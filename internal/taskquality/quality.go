// Package taskquality evaluates bounded tasks through client wire protocols.
// These fixture checks do not claim general semantic or commercial equivalence.
package taskquality

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/szibis/claude-escalate/internal/clientcontrol"
	"github.com/szibis/claude-escalate/internal/labstatus"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"
)

type object = map[string]any

type Fixture struct{ ID, Path, Content, Prompt, Expected string }

func fixtures(marker string) []Fixture {
	return []Fixture{
		{"exact-read", "proof.txt", marker, "Read proof.txt using read_fixture, then return its contents exactly, without line numbers, Markdown or explanation.", marker},
		{"coding-fix", "add.go", "package example\nfunc Add(a, b int) int { return a - b }\n", `Read add.go. Add should return the sum, not the difference. Return only a JSON object with key "expression" containing the corrected Go return expression.`, `{"expression":"a + b"}`},
		{"loki-evidence", "catalog.json", `{"scope":"synthetic fixture, not real products","solutions":[{"name":"Cedar","push":true,"query":true},{"name":"Birch","push":true,"query":false},{"name":"Elm","push":false,"query":true}]}`, `Review all entries in catalog.json using read_fixture. These are synthetic observability solutions. Which support both Loki push and query protocols? Return only JSON {"compatible":[names],"partial":[names]}, names sorted alphabetically. Base the answer only on this evidence.`, `{"compatible":["Cedar"],"partial":["Birch","Elm"]}`},
		{"planning", "dependencies.json", `{"tasks":[{"name":"deploy","after":["verify"]},{"name":"verify","after":["build"]},{"name":"build","after":["design"]},{"name":"design","after":[]}]}`, `Read dependencies.json with read_fixture. Plan an execution order respecting every dependency. Return only JSON {"order":[task names]}.`, `{"order":["design","build","verify","deploy"]}`},
	}
}

type Exchange struct {
	Request  json.RawMessage `json:"request"`
	Response json.RawMessage `json:"response"`
	Status   int             `json:"http_status"`
}
type Result struct {
	Task          string            `json:"task"`
	Role          string            `json:"role"`
	Protocol      string            `json:"protocol"`
	Passed        bool              `json:"passed"`
	Failure       string            `json:"failure,omitempty"`
	Requests      int               `json:"requests,omitempty"`
	ToolCalls     int               `json:"tool_calls"`
	LatencyMS     int64             `json:"latency_ms"`
	InputTokens   *int64            `json:"input_tokens"`
	OutputTokens  *int64            `json:"output_tokens"`
	UsageSource   string            `json:"usage_source"`
	Final         string            `json:"final"`
	Exchanges     []Exchange        `json:"exchanges,omitempty"`
	Client        string            `json:"client,omitempty"`
	ClientVersion string            `json:"client_version,omitempty"`
	CLIExitCode   *int              `json:"cli_exit_code,omitempty"`
	CLIEvents     []json.RawMessage `json:"cli_events,omitempty"`
	CLIStderr     string            `json:"cli_stderr,omitempty"`
	Workspace     string            `json:"workspace,omitempty"`
	Checks        map[string]bool   `json:"checks,omitempty"`
}
type Report struct {
	Version         int             `json:"version"`
	Scope           string          `json:"scope"`
	StartedAt       time.Time       `json:"started_at"`
	FinishedAt      time.Time       `json:"finished_at"`
	Endpoint        string          `json:"endpoint"`
	Results         []Result        `json:"results"`
	GatewayHealth   object          `json:"gateway_health,omitempty"`
	RuntimeSnapshot json.RawMessage `json:"runtime_snapshot,omitempty"`
}

func toJSON(v any) string { b, _ := json.Marshal(v); return string(b) }
func validEndpoint(endpoint string) bool {
	u, err := url.Parse(endpoint)
	return err == nil && u.Scheme == "http" && u.User == nil && net.ParseIP(u.Hostname()).IsLoopback() && u.Port() != "" && u.Path == "" && u.RawQuery == "" && u.Fragment == ""
}
func matches(f Fixture, answer string) bool {
	if f.ID == "exact-read" {
		return strings.TrimSpace(answer) == f.Expected
	}
	var actual, expected any
	if f.ID == "coding-fix" {
		var result map[string]string
		if len(answer) > 8192 || json.Unmarshal([]byte(answer), &result) != nil || len(result) != 1 {
			return false
		}
		expression, err := parser.ParseExpr(result["expression"])
		if err != nil {
			return false
		}
		a, b, ok := coefficients(expression)
		return ok && a == 1 && b == 1
	}
	return json.Unmarshal([]byte(answer), &actual) == nil && json.Unmarshal([]byte(f.Expected), &expected) == nil && reflect.DeepEqual(actual, expected)
}

// Symbolic coefficients accept equivalent addition expressions without executing model code.
func coefficients(expression ast.Expr) (int, int, bool) {
	switch e := expression.(type) {
	case *ast.Ident:
		if e.Name == "a" {
			return 1, 0, true
		}
		if e.Name == "b" {
			return 0, 1, true
		}
	case *ast.ParenExpr:
		return coefficients(e.X)
	case *ast.UnaryExpr:
		a, b, ok := coefficients(e.X)
		if e.Op == token.ADD {
			return a, b, ok
		}
		if e.Op == token.SUB {
			return -a, -b, ok
		}
	case *ast.BinaryExpr:
		a, b, ok := coefficients(e.X)
		c, d, valid := coefficients(e.Y)
		if e.Op == token.ADD {
			return a + c, b + d, ok && valid
		}
		if e.Op == token.SUB {
			return a - c, b - d, ok && valid
		}
	}
	return 0, 0, false
}
func post(ctx context.Context, client *http.Client, endpoint string, payload any) (object, Exchange, error) {
	raw := []byte(toJSON(payload))
	exchange := Exchange{Request: raw}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, exchange, err
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := client.Do(req)
	if err != nil {
		return nil, exchange, err
	}
	defer response.Body.Close()
	exchange.Status = response.StatusCode
	body, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(body) > 1<<20 {
		excerpt := body
		if len(excerpt) > 4096 {
			excerpt = excerpt[:4096]
		}
		exchange.Response = json.RawMessage(toJSON(object{"body_excerpt": string(excerpt), "truncated": true, "bytes_read": len(body)}))
		return nil, exchange, errors.New("response unavailable or oversized")
	}
	if json.Valid(body) {
		exchange.Response = body
	} else {
		exchange.Response = []byte(toJSON(string(body)))
	}
	if response.StatusCode != 200 {
		return nil, exchange, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	var result object
	if json.Unmarshal(body, &result) != nil || result == nil {
		return nil, exchange, errors.New("invalid response JSON")
	}
	return result, exchange, nil
}
func array(v any) []any { a, _ := v.([]any); return a }
func obj(v any) object  { o, _ := v.(map[string]any); return o }
func str(v any) string  { s, _ := v.(string); return s }
func count(v any) (int64, bool) {
	n, ok := v.(float64)
	return int64(n), ok && n > 0 && n < 1e12 && float64(int64(n)) == n
}

func execute(ctx context.Context, client *http.Client, base, role, protocol string, f Fixture, timeout time.Duration) (result Result) {
	result = Result{Task: f.ID, Role: role, Protocol: protocol, UsageSource: "unknown"}
	started := time.Now()
	defer func() { result.LatencyMS = time.Since(started).Milliseconds() }()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	schema := object{"type": "object", "properties": object{"path": object{"type": "string"}}, "required": []string{"path"}, "additionalProperties": false}
	history := []any{object{"role": "user", "content": f.Prompt}}
	var totalIn, totalOut int64
	known := true
	for step := 0; step < 4; step++ {
		payload := object{"model": "sentinel-" + role, "stream": false}
		if protocol == "messages" {
			payload["messages"] = history
			payload["max_tokens"] = 1024
			payload["tools"] = []any{object{"name": "read_fixture", "description": "Read an in-memory fixture by exact relative path. No shell execution.", "input_schema": schema}}
		} else {
			payload["input"] = history
			payload["max_output_tokens"] = 1024
			payload["tools"] = []any{object{"type": "function", "name": "read_fixture", "description": "Read an in-memory fixture by exact relative path.", "parameters": schema}}
		}
		response, exchange, err := post(ctx, client, base+"/v1/"+protocol, payload)
		result.Requests++
		result.Exchanges = append(result.Exchanges, exchange)
		if err != nil {
			result.InputTokens = nil
			result.OutputTokens = nil
			result.UsageSource = "unknown"
			result.Failure = err.Error()
			return
		}
		usage := obj(response["usage"])
		in, inOK := count(usage["input_tokens"])
		out, outOK := count(usage["output_tokens"])
		known = known && inOK && outOK
		totalIn += in
		totalOut += out
		if known {
			a, b := totalIn, totalOut
			result.InputTokens = &a
			result.OutputTokens = &b
			result.UsageSource = "wire-reported; native reconciliation not performed"
		} else {
			result.InputTokens = nil
			result.OutputTokens = nil
			result.UsageSource = "unknown"
		}
		blocks := array(response["content"])
		if protocol == "responses" {
			blocks = array(response["output"])
			if response["status"] != "completed" {
				result.Failure = "incomplete response"
				return
			}
		}
		var calls []object
		var final strings.Builder
		for _, raw := range blocks {
			block := obj(raw)
			switch block["type"] {
			case "text":
				final.WriteString(str(block["text"]))
			case "tool_use":
				calls = append(calls, block)
			case "function_call":
				var arguments object
				if json.Unmarshal([]byte(str(block["arguments"])), &arguments) != nil {
					result.Failure = "invalid function arguments"
					return
				}
				calls = append(calls, object{"id": block["call_id"], "name": block["name"], "input": arguments})
			case "message":
				for _, part := range array(block["content"]) {
					p := obj(part)
					if p["type"] == "output_text" {
						final.WriteString(str(p["text"]))
					}
				}
			}
		}
		result.Final = final.String()
		if len(calls) == 0 {
			if protocol == "messages" && response["stop_reason"] != "end_turn" {
				result.Failure = "incomplete final answer"
				return
			}
			if result.ToolCalls == 0 {
				result.Failure = "required fixture was not read"
				return
			}
			result.Passed = matches(f, result.Final)
			if !result.Passed {
				result.Failure = "final answer failed fixture assertions"
			}
			return
		}
		if protocol == "messages" && response["stop_reason"] != "tool_use" {
			result.Failure = "invalid tool stop reason"
			return
		}
		if len(calls) != 1 {
			result.Failure = "expected one fixture read per turn"
			return
		}
		call := calls[0]
		input := obj(call["input"])
		if call["name"] != "read_fixture" || str(call["id"]) == "" || len(input) != 1 || input["path"] != f.Path {
			result.Failure = "invalid fixture tool or path"
			return
		}
		result.ToolCalls++
		if result.ToolCalls > 1 {
			result.Failure = "repeated unchanged fixture read"
			return
		}
		if protocol == "messages" {
			history = append(history, object{"role": "assistant", "content": blocks}, object{"role": "user", "content": []any{object{"type": "tool_result", "tool_use_id": call["id"], "content": f.Content}}})
		} else {
			history = append(history, blocks...)
			history = append(history, object{"type": "function_call_output", "call_id": call["id"], "output": f.Content})
		}
	}
	result.Failure = "turn limit exceeded"
	return
}

const reportFile = "task-quality-latest.json"
const cliReportFile = "task-cli-quality-latest.json"

func reportFilename(scope string) (string, error) {
	switch scope {
	case "bounded-api-task-probes":
		return reportFile, nil
	case "real-cli-task-probes":
		return cliReportFile, nil
	}
	return "", errors.New("unknown task report scope")
}

// Save atomically publishes a private report. Only synthetic fixture traffic is retained.
func Save(root string, report Report) error {
	filename, err := reportFilename(report.Scope)
	if err != nil {
		return err
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if len(raw) > 64<<20 {
		return errors.New("archive exceeds 64 MiB")
	}
	archive := filepath.Join(root, "task-quality-runs")
	if err := clientcontrol.ValidateDirectory(archive); err != nil {
		return err
	}
	if err := os.MkdirAll(archive, 0700); err != nil {
		return err
	}
	record, err := os.CreateTemp(archive, "run-*.json")
	if err != nil {
		return err
	}
	if _, err = record.Write(raw); err != nil {
		record.Close()
		os.Remove(record.Name())
		return err
	}
	if err = record.Close(); err != nil {
		os.Remove(record.Name())
		return err
	}
	// Latest evidence is a small summary; complete transcripts stay in the archive.
	summary := report
	summary.Results = append([]Result(nil), report.Results...)
	for i := range summary.Results {
		summary.Results[i].Exchanges = nil
		summary.Results[i].Final = ""
		summary.Results[i].CLIEvents = nil
		summary.Results[i].CLIStderr = ""
	}
	raw, err = json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}
	if len(raw) > 1<<20 {
		return errors.New("summary exceeds 1 MiB")
	}
	temp, err := os.CreateTemp(root, ".quality-*")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	if _, err = temp.Write(raw); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Close(); err != nil {
		return err
	}
	return os.Rename(name, filepath.Join(root, filename))
}

// Load returns nil when no valid report exists; the dashboard never fabricates results.
func Load(root string) *Report {
	return loadReport(root, reportFile, "bounded-api-task-probes")
}

func LoadCLI(root string) *Report { return loadReport(root, cliReportFile, "real-cli-task-probes") }

func loadReport(root, filename, scope string) *Report {
	file, err := os.Open(filepath.Join(root, filename))
	if err != nil {
		return nil
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return nil
	}
	var report Report
	if json.Unmarshal(raw, &report) != nil || report.Version != 1 || report.Scope != scope || report.FinishedAt.IsZero() || len(report.Results) == 0 || len(report.Results) > 24 {
		return nil
	}
	return &report
}

// Run sends explicit synthetic local requests, retaining failures and exiting nonzero on any failed task.
func Run(args []string, _ io.Reader, out, stderr io.Writer) int {
	flags := flag.NewFlagSet("quality", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".sentinel-lab", "existing lab root for evidence")
	endpoint := flags.String("endpoint", "http://127.0.0.1:19090", "literal loopback gateway")
	role := flags.String("role", "sonnet", "haiku, sonnet or opus")
	protocol := flags.String("protocol", "both", "messages, responses or both")
	task := flags.String("task", "all", "all, exact-read, coding-fix, loki-evidence or planning")
	timeout := flags.Duration("timeout", 2*time.Minute, "whole-task timeout")
	if flags.Parse(args) != nil || flags.NArg() != 0 || !validEndpoint(*endpoint) || (*role != "haiku" && *role != "sonnet" && *role != "opus") || (*protocol != "both" && *protocol != "messages" && *protocol != "responses") || *timeout <= 0 || *timeout > 10*time.Minute {
		fmt.Fprintln(stderr, "invalid quality options")
		return 2
	}
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	var selected []Fixture
	for _, f := range fixtures("QUALITY_" + hex.EncodeToString(nonce)) {
		if *task == "all" || *task == f.ID {
			selected = append(selected, f)
		}
	}
	if len(selected) == 0 {
		fmt.Fprintln(stderr, "unknown task")
		return 2
	}
	if stat, err := os.Stat(*root); err != nil || !stat.IsDir() {
		fmt.Fprintln(stderr, "lab root must exist")
		return 2
	}
	absolute, err := filepath.Abs(*root)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if err = clientcontrol.ValidateDirectory(absolute); err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	*root = absolute
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: *timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect refused") }}
	// Paid provider routing must be disabled before probes. The active lab is never reconfigured.
	response, err := client.Get(*endpoint + "/health")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	var health object
	err = json.NewDecoder(io.LimitReader(response.Body, 65536)).Decode(&health)
	response.Body.Close()
	controls := obj(health["controls"])
	if err != nil || response.StatusCode != 200 || health["mode"] != "serving" || controls["policy"] != "local-only" || controls["startup_billing_opt_in"] != false {
		fmt.Fprintln(stderr, "quality requires serving mode, local-only policy and disabled paid API opt-in")
		return 1
	}
	report := Report{Version: 1, Scope: "bounded-api-task-probes", Endpoint: *endpoint, StartedAt: time.Now().UTC(), GatewayHealth: health}
	// Model identities are configuration/status evidence, not inferred from role aliases.
	var snapshot, diagnostic bytes.Buffer
	if *endpoint == "http://127.0.0.1:19090" && labstatus.Run([]string{"--root", absolute, "--json"}, nil, &snapshot, &diagnostic) == 0 && json.Valid(snapshot.Bytes()) {
		report.RuntimeSnapshot = append(json.RawMessage(nil), snapshot.Bytes()...)
	}
	protocols := []string{*protocol}
	if *protocol == "both" {
		protocols = []string{"messages", "responses"}
	}
	passed := true
	for _, p := range protocols {
		for _, f := range selected {
			fmt.Fprintf(out, "Checking %s / %s / %s\n", *role, p, f.ID)
			result := execute(context.Background(), client, *endpoint, *role, p, f, *timeout)
			report.Results = append(report.Results, result)
			fmt.Fprintf(out, "  pass=%t calls=%d latency=%dms %s\n", result.Passed, result.ToolCalls, result.LatencyMS, result.Failure)
			passed = passed && result.Passed
		}
	}
	report.FinishedAt = time.Now().UTC()
	if err := Save(*root, report); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(out, "Evidence:", filepath.Join(*root, reportFile))
	if !passed {
		return 1
	}
	return 0
}
