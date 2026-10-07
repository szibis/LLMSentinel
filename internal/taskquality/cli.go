// Native CLI probes use installed isolated clients and synthetic workspaces.
package taskquality

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/szibis/claude-escalate/internal/clientcontrol"
	"github.com/szibis/claude-escalate/internal/labstatus"
)

func nowUTC() time.Time { return time.Now().UTC() }

type cliEvidence struct {
	Final                     string
	Read                      bool
	TestRan                   bool
	ToolCalls                 int
	InputTokens, OutputTokens *int64
	Events                    []json.RawMessage
}

func textContent(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	var out strings.Builder
	for _, raw := range array(v) {
		part := obj(raw)
		if part["type"] == "text" {
			out.WriteString(str(part["text"]))
		}
	}
	return out.String()
}
func fixturePath(path, workspace string, f Fixture) bool {
	if path == "" {
		return false
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(workspace, path)
	}
	return filepath.Clean(path) == filepath.Join(workspace, f.Path)
}
func readEvidence(output string, f Fixture) bool {
	if f.ID == "coding-fix" {
		return strings.Contains(output, "func Add(")
	}
	return strings.Contains(output, f.Content)
}
func unwrapShell(command string) string {
	command = strings.TrimSpace(command)
	for _, shell := range []string{"/bin/zsh", "/bin/bash", "/bin/sh", "bash", "sh", "zsh"} {
		prefix := shell + " -lc "
		if strings.HasPrefix(command, prefix) {
			body := strings.TrimSpace(strings.TrimPrefix(command, prefix))
			if len(body) >= 2 && body[0] == '\'' && body[len(body)-1] == '\'' {
				return strings.ReplaceAll(body[1:len(body)-1], "'\\''", "'")
			}
			if decoded, err := strconv.Unquote(body); err == nil {
				return decoded
			}
			return body
		}
	}
	return command
}
func isTestCommand(command string) bool {
	return unwrapShell(command) == "go test -timeout 10s ./..."
}

func successfulTestOutput(output string) bool {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "ok" && fields[1] == "fixture" {
			return true
		}
	}
	return false
}
func commandRead(command, workspace string, f Fixture) bool {
	command = unwrapShell(command)
	if strings.ContainsAny(command, "\n\r;|&<>#$`()\\") {
		return false
	}
	fields := strings.Fields(command)
	if len(fields) < 2 {
		return false
	}
	switch fields[0] {
	case "cat":
		fields = fields[1:]
		if fields[0] == "--" {
			fields = fields[1:]
		}
	case "head":
		if len(fields) != 4 || fields[1] != "-n" {
			return false
		}
		if n, err := strconv.Atoi(fields[2]); err != nil || n <= 0 {
			return false
		}
		fields = fields[3:]
	case "sed":
		if len(fields) != 4 || fields[1] != "-n" || !regexp.MustCompile(`^'?\d+(,\d+)?p'?$`).MatchString(fields[2]) {
			return false
		}
		fields = fields[3:]
	default:
		return false
	}
	found := false
	for _, field := range fields {
		path := strings.Trim(field, "\"'")
		if path == "" || strings.HasPrefix(path, "-") || strings.ContainsAny(path, "\"' *?[]{}") {
			return false
		}
		if fixturePath(path, workspace, f) {
			found = true
		}
	}
	return found
}

// decodeCLI only promotes successful tool outcomes, not assistant tool proposals.
func decodeCLI(client string, raw []byte, workspace string, f Fixture) (cliEvidence, error) {
	var evidence cliEvidence
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	calls := map[string]object{}
	seen := map[string]bool{}
	completed := false
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var event object
		if json.Unmarshal(line, &event) != nil {
			return evidence, errors.New("invalid CLI JSONL event")
		}
		evidence.Events = append(evidence.Events, append(json.RawMessage(nil), line...))
		if client == "claude" {
			switch event["type"] {
			case "assistant":
				for _, raw := range array(obj(event["message"])["content"]) {
					block := obj(raw)
					id := str(block["id"])
					if block["type"] == "tool_use" && id != "" && !seen[id] {
						calls[id] = block
						seen[id] = true
						evidence.ToolCalls++
					}
				}
			case "user":
				for _, raw := range array(obj(event["message"])["content"]) {
					block := obj(raw)
					call := calls[str(block["tool_use_id"])]
					if block["type"] != "tool_result" || block["is_error"] == true || call == nil {
						continue
					}
					input := obj(call["input"])
					output := textContent(block["content"])
					if call["name"] == "Read" && fixturePath(str(input["file_path"]), workspace, f) && readEvidence(output, f) {
						evidence.Read = true
					}
					if call["name"] == "Bash" && isTestCommand(str(input["command"])) && successfulTestOutput(output) {
						evidence.TestRan = true
					}
				}
			case "result":
				if event["is_error"] != false || event["subtype"] != "success" {
					detail := str(event["result"])
					if len(detail) > 300 {
						detail = detail[:300]
					}
					return evidence, fmt.Errorf("CLI error (%s): %s", str(event["subtype"]), detail)
				}
				completed = true
				evidence.Final = str(event["result"])
				setCLIUsage(&evidence, obj(event["usage"]))
			}
		} else {
			switch event["type"] {
			case "turn.failed":
				detail := str(obj(event["error"])["message"])
				if len(detail) > 300 {
					detail = detail[:300]
				}
				return evidence, fmt.Errorf("CLI turn failed: %s", detail)
			case "turn.completed":
				completed = true
				setCLIUsage(&evidence, obj(event["usage"]))
			case "item.completed":
				item := obj(event["item"])
				id := str(item["id"])
				if id == "" || seen[id] {
					continue
				}
				seen[id] = true
				switch item["type"] {
				case "agent_message":
					evidence.Final = str(item["text"])
				case "command_execution":
					evidence.ToolCalls++
					if item["status"] != "completed" || item["exit_code"] != float64(0) {
						continue
					}
					command, output := str(item["command"]), str(item["aggregated_output"])
					if commandRead(command, workspace, f) && readEvidence(output, f) {
						evidence.Read = true
					}
					if isTestCommand(command) && successfulTestOutput(output) {
						evidence.TestRan = true
					}
				case "file_change":
					evidence.ToolCalls++
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return evidence, errors.New("CLI event oversized or unreadable")
	}
	if !completed {
		return evidence, errors.New("CLI completion event missing")
	}
	return evidence, nil
}
func setCLIUsage(e *cliEvidence, usage object) {
	a, ok := count(usage["input_tokens"])
	b, valid := count(usage["output_tokens"])
	e.InputTokens = nil
	e.OutputTokens = nil
	if ok && valid {
		e.InputTokens = &a
		e.OutputTokens = &b
	}
}
func cliFinalPasses(f Fixture, e cliEvidence) bool { return e.Read && matches(f, e.Final) }

func codeFiles() map[string]string {
	return map[string]string{
		"go.mod": "module fixture\n\ngo 1.20\n",
		"add.go": "package fixture\n\nfunc Add(a, b int) int { return a - b }\n",
		"add_test.go": `package fixture
import "testing"
func TestAdd(t *testing.T) {
 for _,v:=range [][3]int{{2,3,5},{-3,4,1},{0,0,0},{7,-2,5}} {
  if got:=Add(v[0],v[1]);got!=v[2] {t.Fatalf("Add(%d,%d)=%d want %d",v[0],v[1],got,v[2])}
 }
}
`,
	}
}
func boundedFile(path string) ([]byte, error) {
	stat, err := os.Lstat(path)
	if err != nil || !stat.Mode().IsRegular() || stat.Size() > 64<<10 {
		return nil, errors.New("fixture file missing, nonregular or oversized")
	}
	return os.ReadFile(path)
}

// verifyCode validates the edit and immutable tests before executing any generated code.
func verifyCode(workspace string) error {
	for _, name := range []string{"go.mod", "add_test.go"} {
		data, err := boundedFile(filepath.Join(workspace, name))
		if err != nil || string(data) != codeFiles()[name] {
			return errors.New("supplied tests or module were changed")
		}
	}
	data, err := boundedFile(filepath.Join(workspace, "add.go"))
	if err != nil {
		return err
	}
	source, err := parser.ParseFile(token.NewFileSet(), "add.go", data, 0)
	if err != nil || source.Name.Name != "fixture" || len(source.Imports) != 0 || len(source.Decls) != 1 {
		return errors.New("edited source is outside the bounded function contract")
	}
	function, ok := source.Decls[0].(*ast.FuncDecl)
	if !ok || function.Name.Name != "Add" || function.Recv != nil || function.Type.TypeParams != nil || function.Body == nil || len(function.Body.List) != 1 {
		return errors.New("unexpected function")
	}
	params := function.Type.Params.List
	if len(params) != 1 || len(params[0].Names) != 2 || params[0].Names[0].Name != "a" || params[0].Names[1].Name != "b" {
		return errors.New("unexpected function parameters")
	}
	typ, ok := params[0].Type.(*ast.Ident)
	if !ok || typ.Name != "int" || function.Type.Results == nil || len(function.Type.Results.List) != 1 {
		return errors.New("unexpected function types")
	}
	retType, ok := function.Type.Results.List[0].Type.(*ast.Ident)
	if !ok || retType.Name != "int" || len(function.Type.Results.List[0].Names) != 0 {
		return errors.New("unexpected return type")
	}
	statement, ok := function.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(statement.Results) != 1 {
		return errors.New("unexpected function body")
	}
	a, b, valid := coefficients(statement.Results[0])
	if !valid || a != 1 || b != 1 {
		return errors.New("edited function does not compute addition")
	}
	return nil
}
func cliEnvironment(client, run, endpoint string) []string {
	env := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + filepath.Join(run, "home"), "TMPDIR=" + filepath.Join(run, "tmp"), "GOTOOLCHAIN=local", "GOCACHE=" + filepath.Join(run, "workspace-cache"), "LANG=C", "LC_ALL=C"}
	if client == "codex" {
		return append(env, "CODEX_HOME="+filepath.Join(run, "codex"), "SENTINEL_LAB_TOKEN=sentinel-cli-probe")
	}
	return append(env, "CLAUDE_CONFIG_DIR="+filepath.Join(run, "claude"), "ANTHROPIC_BASE_URL="+endpoint, "ANTHROPIC_API_KEY=sentinel-cli-probe", "ANTHROPIC_DEFAULT_HAIKU_MODEL=sentinel-haiku", "ANTHROPIC_DEFAULT_SONNET_MODEL=sentinel-sonnet", "ANTHROPIC_DEFAULT_OPUS_MODEL=sentinel-opus", "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1", "DISABLE_AUTOUPDATER=1", "DISABLE_UPDATES=1", "DISABLE_TELEMETRY=1", "DISABLE_ERROR_REPORTING=1", "MAX_THINKING_TOKENS=0", "CLAUDE_CODE_DISABLE_ADAPTIVE_THINKING=1", "CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS=1", "ENABLE_TOOL_SEARCH=false")
}
func prepareCLI(run, workspace, client, endpoint string, f Fixture) error {
	for _, name := range []string{"home", "tmp", "claude", "codex", "workspace-cache"} {
		if err := os.MkdirAll(filepath.Join(run, name), 0700); err != nil {
			return err
		}
	}
	if client == "claude" {
		if err := os.WriteFile(filepath.Join(run, "claude", "settings.json"), []byte("{}"), 0600); err != nil {
			return err
		}
	} else {
		catalogPath := filepath.Join(run, "codex", "model-catalog.json")
		if err := os.WriteFile(catalogPath, clientcontrol.CodexModelCatalog(), 0600); err != nil {
			return err
		}
		config := "model_provider = \"sentinel\"\napproval_policy = \"never\"\nsandbox_mode = \"workspace-write\"\nweb_search = \"disabled\"\ncli_auth_credentials_store = \"file\"\n\n[analytics]\nenabled = false\n\n[model_providers.sentinel]\nname = \"Sentinel CLI probe\"\nbase_url = " + strconv.Quote(endpoint+"/v1") + "\nenv_key = \"SENTINEL_LAB_TOKEN\"\nwire_api = \"responses\"\nrequires_openai_auth = false\nrequest_max_retries = 0\nstream_max_retries = 0\nstream_idle_timeout_ms = 120000\n"
		config = "model_catalog_json = " + strconv.Quote(catalogPath) + "\n" + config
		config += "\n" // The header belongs to the provider table, before its fields.
		config = strings.Replace(config, "[model_providers.sentinel]\n", "[model_providers.sentinel]\nhttp_headers = { \"X-Session-ID\" = "+strconv.Quote(filepath.Base(run))+" }\n", 1)
		if err := os.WriteFile(filepath.Join(run, "codex", "config.toml"), []byte(config), 0600); err != nil {
			return err
		}
	}
	if f.ID == "coding-fix" {
		for name, data := range codeFiles() {
			if err := os.WriteFile(filepath.Join(workspace, name), []byte(data), 0600); err != nil {
				return err
			}
		}
		return nil
	}
	return os.WriteFile(filepath.Join(workspace, f.Path), []byte(f.Content), 0600)
}
func cliPrompt(f Fixture, workspace string) string {
	if f.ID == "coding-fix" {
		return "Read " + filepath.Join(workspace, "add.go") + " and add_test.go. Fix Add to return the sum. Edit only add.go; preserve the tests and go.mod. Run exactly go test -timeout 10s ./... in the current directory. After successful tests, reply exactly FIXED_AND_TESTED. Do not just describe a fix."
	}
	return strings.ReplaceAll(f.Prompt, "read_fixture", "your real file tools") + " The absolute fixture path is " + filepath.Join(workspace, f.Path) + "."
}
func cliArguments(client, role, workspace string, f Fixture) []string {
	prompt := cliPrompt(f, workspace)
	if client == "codex" {
		return []string{"exec", "--ephemeral", "--skip-git-repo-check", "--json", "--sandbox", "workspace-write", "-c", "approval_policy=\"never\"", "-c", "model_reasoning_effort=\"medium\"", "-m", "sentinel-" + role, "-C", workspace, prompt}
	}
	tools := "Read"
	allowed := []string{"Read(/" + filepath.Join(workspace, f.Path) + ")"}
	if f.ID == "coding-fix" {
		tools = "Read,Edit,Write,Bash"
		allowed = []string{"Read(/" + workspace + "/*)", "Edit(/" + filepath.Join(workspace, "add.go") + ")", "Write(/" + filepath.Join(workspace, "add.go") + ")", "Bash(go test -timeout 10s ./...)"}
	}
	args := []string{"--bare", "--model", role, "--print", "--verbose", "--output-format", "stream-json", "--no-session-persistence", "--setting-sources", "user", "--strict-mcp-config", "--mcp-config", `{"mcpServers":{}}`, "--permission-mode", "dontAsk", "--tools", tools, "--max-turns", "6", "--allowedTools"}
	args = append(args, allowed...)
	// -- ends the variadic tool allow-list so the prompt cannot be parsed as another rule.
	return append(args, "--", prompt)
}
func installedClient(root, client string) (string, error) {
	path := filepath.Join(root, "clients", "node_modules", ".bin", client)
	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", errors.New("isolated CLI is not installed; no installation attempted")
	}
	relative, err := filepath.Rel(filepath.Join(root, "clients"), target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("isolated CLI resolves outside the lab client directory")
	}
	stat, err := os.Stat(target)
	if err != nil || !stat.Mode().IsRegular() || stat.Mode().Perm()&0111 == 0 {
		return "", errors.New("isolated CLI is not executable")
	}
	return target, nil
}

type cappedBuffer struct {
	bytes.Buffer
	Limit    int
	Overflow bool
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	remaining := b.Limit - b.Len()
	if len(p) > remaining {
		b.Overflow = true
		if remaining > 0 {
			_, _ = b.Buffer.Write(p[:remaining])
		}
		return len(p), nil
	}
	return b.Buffer.Write(p)
}
func runCLIProcess(ctx context.Context, binary string, args, env []string, workspace string) ([]byte, string, int, error) {
	cmd := exec.CommandContext(ctx, binary, args...) // #nosec G204 -- installed isolated client or resolved Go tool; fixed arguments, no shell interpolation.
	cmd.Dir = workspace
	cmd.Env = env
	cmd.Stdin = nil
	cmd.WaitDelay = time.Second
	configureCLIProcess(cmd)
	stdout, stderr := &cappedBuffer{Limit: 8 << 20}, &cappedBuffer{Limit: 1 << 20}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	finishCLIProcess(cmd)
	exit := -1
	if cmd.ProcessState != nil {
		exit = cmd.ProcessState.ExitCode()
	}
	if stdout.Overflow || stderr.Overflow {
		err = errors.New("CLI log limit exceeded")
	}
	return stdout.Bytes(), stderr.String(), exit, err
}

func executeCLI(ctx context.Context, root, base, role, client string, f Fixture, timeout time.Duration) (result Result) {
	result = Result{Task: f.ID, Role: role, Client: client, Protocol: client + "-cli", UsageSource: "unknown", Checks: map[string]bool{}}
	started := time.Now()
	defer func() { result.LatencyMS = time.Since(started).Milliseconds() }()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	binary, err := installedClient(root, client)
	if err != nil {
		result.Failure = err.Error()
		return
	}
	artifacts := filepath.Join(root, "task-cli-runs")
	if err = clientcontrol.ValidateDirectory(artifacts); err == nil {
		err = os.MkdirAll(artifacts, 0700)
	}
	if err != nil {
		result.Failure = err.Error()
		return
	}
	run, err := os.MkdirTemp(artifacts, client+"-")
	if err != nil {
		result.Failure = err.Error()
		return
	}
	workspace, err := os.MkdirTemp("", "sentinel-cli-"+client+"-")
	if err != nil {
		result.Failure = err.Error()
		return
	}
	workspace, err = filepath.EvalSymlinks(workspace)
	if err != nil {
		result.Failure = err.Error()
		return
	}
	result.Workspace = workspace
	if err = prepareCLI(run, workspace, client, base, f); err != nil {
		result.Failure = err.Error()
		return
	}
	env := cliEnvironment(client, run, base)
	// Test cache must be writable inside the Codex workspace sandbox.
	for i, v := range env {
		if strings.HasPrefix(v, "GOCACHE=") {
			env[i] = "GOCACHE=" + filepath.Join(workspace, ".go-cache")
		}
	}
	version, versionErr, code, err := runCLIProcess(ctx, binary, []string{"--version"}, env, workspace)
	if err != nil || code != 0 {
		result.Failure = "CLI version probe failed: " + versionErr
		return
	}
	result.ClientVersion = strings.TrimSpace(string(version))
	raw, stderr, exit, processErr := runCLIProcess(ctx, binary, cliArguments(client, role, workspace, f), env, workspace)
	result.CLIExitCode = &exit
	result.CLIStderr = stderr
	if err := os.WriteFile(filepath.Join(run, "stdout.jsonl"), raw, 0600); err != nil {
		result.Failure = "persist CLI transcript: " + err.Error()
		return
	}
	if err := os.WriteFile(filepath.Join(run, "stderr.log"), []byte(stderr), 0600); err != nil {
		result.Failure = "persist CLI stderr: " + err.Error()
		return
	}
	evidence, decodeErr := decodeCLI(client, raw, workspace, f)
	result.CLIEvents = evidence.Events
	result.Final = evidence.Final
	result.ToolCalls = evidence.ToolCalls
	result.Checks["fixture_read"] = evidence.Read
	if processErr != nil || exit != 0 {
		result.Failure = "CLI process failed"
		if decodeErr != nil {
			result.Failure = decodeErr.Error()
		}
		if ctx.Err() != nil {
			result.Failure = "CLI deadline/cancellation"
		}
		return
	}
	if decodeErr != nil {
		result.Failure = decodeErr.Error()
		return
	}
	result.InputTokens = evidence.InputTokens
	result.OutputTokens = evidence.OutputTokens
	if result.InputTokens != nil {
		result.UsageSource = "CLI-reported; retries/cache coverage and native reconciliation not established"
	}
	if f.ID != "coding-fix" {
		result.Passed = cliFinalPasses(f, evidence)
		if !result.Passed {
			result.Failure = "CLI read or final assertion failed"
		}
		return
	}
	result.Checks["client_test_command"] = evidence.TestRan
	if err = verifyCode(workspace); err != nil {
		result.Failure = err.Error()
		return
	}
	result.Checks["edit_and_tests_preserved"] = true
	if !evidence.Read || !evidence.TestRan || strings.TrimSpace(evidence.Final) != "FIXED_AND_TESTED" {
		result.Failure = "CLI coding evidence incomplete"
		return
	}
	goBinary, err := exec.LookPath("go")
	if err != nil {
		result.Failure = "Go verifier unavailable"
		return
	}
	verifyCtx, verifyCancel := context.WithTimeout(ctx, 20*time.Second)
	defer verifyCancel()
	// Run only the validated function and trusted tests, never arbitrary extra Go files.
	verifyDir, err := os.MkdirTemp(run, "verify-")
	if err != nil {
		result.Failure = err.Error()
		return
	}
	data, err := boundedFile(filepath.Join(workspace, "add.go"))
	if err != nil {
		result.Failure = err.Error()
		return
	}
	for name, contents := range codeFiles() {
		if name == "add.go" {
			contents = string(data)
		}
		if err = os.WriteFile(filepath.Join(verifyDir, name), []byte(contents), 0600); err != nil {
			result.Failure = err.Error()
			return
		}
	}
	if err = verifyCode(verifyDir); err != nil {
		result.Failure = err.Error()
		return
	}
	// These cases are not supplied to the client and are checked independently.
	heldout := `package fixture
import "testing"
func TestHeldOutAdd(t *testing.T) { for a:=-31;a<=31;a++ {for b:=-17;b<=17;b++ {if got:=Add(a,b);got!=a+b {t.Fatalf("Add(%d,%d)=%d",a,b,got)}}} }
`
	if err = os.WriteFile(filepath.Join(verifyDir, "heldout_test.go"), []byte(heldout), 0600); err != nil {
		result.Failure = err.Error()
		return
	}
	_, verifyStderr, status, verifyErr := runCLIProcess(verifyCtx, goBinary, []string{"test", "-timeout", "10s", "./..."}, env, verifyDir)
	if verifyErr != nil || status != 0 {
		result.Failure = "independent Go tests failed: " + verifyStderr
		return
	}
	result.Checks["independent_tests"] = true
	result.Passed = true
	return
}

// RunCLI benchmarks real local CLIs without using the user's existing profiles.
func cliRoles(role string) []string {
	if role == "all" {
		return []string{"haiku", "sonnet", "opus"}
	}
	if role == "haiku" || role == "sonnet" || role == "opus" {
		return []string{role}
	}
	return nil
}

func RunCLI(args []string, _ io.Reader, out, stderr io.Writer) int {
	flags := flag.NewFlagSet("cli-quality", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".sentinel-lab", "existing isolated lab root")
	base := flags.String("endpoint", "http://127.0.0.1:19090", "literal loopback gateway")
	role := flags.String("role", "sonnet", "haiku, sonnet, opus or all")
	clientName := flags.String("client", "both", "claude, codex or both")
	task := flags.String("task", "all", "all, exact-read, coding-fix, loki-evidence or planning")
	timeout := flags.Duration("timeout", 3*time.Minute, "whole-task deadline")
	if flags.Parse(args) != nil || flags.NArg() != 0 || !validEndpoint(*base) || len(cliRoles(*role)) == 0 || (*clientName != "claude" && *clientName != "codex" && *clientName != "both") || *timeout <= 0 || *timeout > 10*time.Minute {
		fmt.Fprintln(stderr, "invalid CLI probe options")
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
	if stat, err := os.Stat(absolute); err != nil || !stat.IsDir() {
		fmt.Fprintln(stderr, "lab root must exist")
		return 2
	}
	nonce := make([]byte, 12)
	if _, err = rand.Read(nonce); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	var selected []Fixture
	for _, f := range fixtures("CLI_" + hex.EncodeToString(nonce)) {
		if *task == "all" || f.ID == *task {
			selected = append(selected, f)
		}
	}
	if len(selected) == 0 {
		fmt.Fprintln(stderr, "unknown CLI fixture")
		return 2
	}
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	httpClient := &http.Client{Transport: transport, Timeout: 2 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect refused") }}
	response, err := httpClient.Get(*base + "/health")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	var health object
	err = json.NewDecoder(io.LimitReader(response.Body, 65536)).Decode(&health)
	response.Body.Close()
	controls := obj(health["controls"])
	if err != nil || response.StatusCode != http.StatusOK || health["mode"] != "serving" || controls["policy"] != "local-only" || controls["startup_billing_opt_in"] != false {
		fmt.Fprintln(stderr, "CLI probes require local-only serving with paid API opt-in disabled")
		return 1
	}
	report := Report{Version: 1, Scope: "real-cli-task-probes", Endpoint: *base, StartedAt: nowUTC(), GatewayHealth: health}
	var snapshot, diagnostic bytes.Buffer
	if *base == "http://127.0.0.1:19090" && labstatus.Run([]string{"--root", absolute, "--json"}, nil, &snapshot, &diagnostic) == 0 && json.Valid(snapshot.Bytes()) {
		report.RuntimeSnapshot = append(json.RawMessage(nil), snapshot.Bytes()...)
	}
	clients := []string{*clientName}
	if *clientName == "both" {
		clients = []string{"claude", "codex"}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	passed := true
outer:
	for _, selectedRole := range cliRoles(*role) {
		for _, client := range clients {
			for _, f := range selected {
				fmt.Fprintf(out, "Checking real %s / %s / %s\n", client, selectedRole, f.ID)
				result := executeCLI(ctx, absolute, *base, selectedRole, client, f, *timeout)
				report.Results = append(report.Results, result)
				passed = passed && result.Passed
				fmt.Fprintf(out, "  pass=%t tools=%d latency=%dms %s\n", result.Passed, result.ToolCalls, result.LatencyMS, result.Failure)
				if ctx.Err() != nil {
					break outer
				}
			}
		}
	}
	report.FinishedAt = nowUTC()
	if ctx.Err() != nil {
		fmt.Fprintln(stderr, "CLI probes interrupted; private logs retained, latest report unchanged")
		return 1
	}
	if err := Save(absolute, report); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(out, "CLI evidence:", filepath.Join(absolute, cliReportFile))
	if !passed || ctx.Err() != nil {
		return 1
	}
	return 0
}
