// Package qwensmoke verifies an external MLX-Flash runtime without embedding it.
package qwensmoke

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"
)

const marker = "QWEN_ROLE_READY"
const runtimeURL = "http://127.0.0.1:19191"
const gatewayURL = "http://127.0.0.1:19190"

type document map[string]any

func mapping(v any) document {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	if m, ok := v.(document); ok {
		return m
	}
	return document{}
}
func list(v any) []any  { a, _ := v.([]any); return a }
func text(v any) string { s, _ := v.(string); return s }

func request(ctx context.Context, endpoint string, payload any, timeout time.Duration) (document, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "http" || u.User != nil || !net.ParseIP(u.Hostname()).IsLoopback() {
		return nil, errors.New("smoke requests require a literal loopback HTTP endpoint")
	}
	var data io.Reader
	method := http.MethodGet
	if payload != nil {
		encoded, e := json.Marshal(payload)
		if e != nil {
			return nil, e
		}
		data = bytes.NewReader(encoded)
		method = http.MethodPost
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, endpoint, data)
	if err != nil {
		return nil, errors.New("invalid smoke request")
	}
	req.Header.Set("Content-Type", "application/json")
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	if err != nil {
		return nil, errors.New("smoke HTTP request timed out or failed")
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("smoke HTTP request failed (%d)", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 8<<20+1))
	if err != nil || len(body) > 8<<20 {
		return nil, errors.New("smoke HTTP response is unavailable or oversized")
	}
	var result document
	if err = json.Unmarshal(body, &result); err != nil {
		return nil, errors.New("smoke HTTP response is invalid JSON")
	}
	return result, nil
}

func waitReady(ctx context.Context, process *ownedProcess, endpoint string, timeout time.Duration) (document, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		select {
		case <-process.done:
			return nil, errors.New("owned server exited before becoming ready")
		case <-ctx.Done():
			return nil, errors.New("timed out waiting for owned server readiness")
		default:
		}
		if state, err := request(ctx, endpoint, nil, 2*time.Second); err == nil && state["status"] == "ok" {
			return state, nil
		}
		select {
		case <-ctx.Done():
			return nil, errors.New("timed out waiting for owned server readiness")
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func validateModel(path string) error {
	if !filepath.IsAbs(path) {
		return errors.New("model paths must be absolute cached directories")
	}
	for _, name := range []string{"config.json", "tokenizer.json", "tokenizer_config.json"} {
		info, err := os.Stat(filepath.Join(path, name)) // #nosec G703 -- Explicit operator-selected model/runtime path and fixed metadata or validated shard basename.
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			return fmt.Errorf("cached model is missing nonempty %s", name)
		}
	}
	weights, err := filepath.Glob(filepath.Join(path, "*.safetensors"))
	if err != nil || len(weights) == 0 {
		return errors.New("cached model has missing weights")
	}
	sharded := false
	for _, file := range weights {
		info, e := os.Stat(file) // #nosec G703 -- Explicit operator-selected model/runtime path and fixed metadata or validated shard basename.
		if e != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			return errors.New("cached model has missing or empty weights")
		}
		sharded = sharded || strings.Contains(filepath.Base(file), "-of-")
	}
	index, err := os.ReadFile(filepath.Join(path, "model.safetensors.index.json")) // #nosec G703 -- Explicit operator-selected model/runtime path and fixed metadata or validated shard basename.
	if err == nil {
		var value struct {
			Weights map[string]string `json:"weight_map"`
		}
		if json.Unmarshal(index, &value) != nil || len(value.Weights) == 0 {
			return errors.New("cached model index has invalid shards")
		}
		for _, shard := range value.Weights {
			if filepath.Base(shard) != shard || shard == "." || strings.ContainsAny(shard, "/\\") {
				return errors.New("cached model index has invalid shards")
			}
			info, e := os.Stat(filepath.Join(path, shard)) // #nosec G703 -- Explicit operator-selected model/runtime path and fixed metadata or validated shard basename.
			if e != nil || !info.Mode().IsRegular() || info.Size() == 0 {
				return errors.New("cached model index has missing shards")
			}
		}
	} else if !os.IsNotExist(err) || sharded {
		return errors.New("sharded model requires a readable weights index")
	}
	template, err := os.ReadFile(filepath.Join(path, "chat_template.jinja")) // #nosec G703 -- Explicit operator-selected model/runtime path and fixed metadata or validated shard basename.
	if os.IsNotExist(err) {
		var cfg struct {
			Template string `json:"chat_template"`
		}
		data, e := os.ReadFile(filepath.Join(path, "tokenizer_config.json")) // #nosec G703 -- Explicit operator-selected model/runtime path and fixed metadata or validated shard basename.
		if e != nil || json.Unmarshal(data, &cfg) != nil {
			return errors.New("cached model has invalid tokenizer configuration")
		}
		template = []byte(cfg.Template)
	} else if err != nil {
		return errors.New("cached model has unreadable thinking template")
	}
	if !bytes.Contains(template, []byte("enable_thinking")) {
		return errors.New("cached model must have a Qwen thinking template")
	}
	return nil
}

func preflightPorts(ports []int) error {
	var listeners []net.Listener
	defer func() {
		for _, listener := range listeners {
			_ = listener.Close()
		}
	}()
	for _, port := range ports {
		listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			return errors.New("CI ports are occupied; existing services were not stopped")
		}
		listeners = append(listeners, listener)
	}
	return nil
}

func finalText(value string, thinking bool) error {
	_, after, found := strings.Cut(value, "</think>")
	if thinking && !found {
		return errors.New("qwen thinking generation did not finish reasoning")
	}
	if found {
		value = after
	}
	if strings.TrimSpace(value) != marker {
		return errors.New("qwen generation failed the exact final-answer marker")
	}
	return nil
}

func generateRole(ctx context.Context, base, role string, tool bool) (document, error) {
	budget := 512
	if role == "opus" {
		budget = 8192
	}
	message := document{"role": "user", "content": "Reply with exactly " + marker + " and no other final text."}
	payload := document{"model": "sentinel-" + role, "max_tokens": budget, "messages": []any{message}}
	if tool {
		message["content"] = "Call record_marker once with marker " + marker + ". Do not answer in text."
		payload["tools"] = []any{document{"name": "record_marker", "description": "Record a smoke-test marker; no side effects.", "input_schema": document{"type": "object", "properties": document{"marker": document{"type": "string", "enum": []string{marker}}}, "required": []string{"marker"}, "additionalProperties": false}}}
		payload["tool_choice"] = document{"type": "tool", "name": "record_marker"}
	}
	result, err := request(ctx, base+"/v1/messages", payload, 10*time.Minute)
	if err != nil {
		return nil, err
	}
	if tool {
		var calls []document
		for _, block := range list(result["content"]) {
			b := mapping(block)
			if b["type"] == "tool_use" {
				calls = append(calls, b)
			}
		}
		if result["stop_reason"] != "tool_use" || len(calls) != 1 || calls[0]["name"] != "record_marker" || len(mapping(calls[0]["input"])) != 1 || mapping(calls[0]["input"])["marker"] != marker {
			return nil, errors.New("sentinel tool bridge failed the validated marker call")
		}
	} else {
		if result["stop_reason"] != "end_turn" {
			return nil, errors.New("sentinel role generation did not finish")
		}
		var answer strings.Builder
		for _, block := range list(result["content"]) {
			b := mapping(block)
			if b["type"] == "text" {
				answer.WriteString(text(b["text"]))
			}
		}
		if err = finalText(answer.String(), false); err != nil {
			return nil, err
		}
	}
	return mapping(result["usage"]), nil
}

func generateChat(ctx context.Context, thinking bool) (document, error) {
	budget := 256
	if thinking {
		budget = 8192
	}
	result, err := request(ctx, runtimeURL+"/v1/chat/completions", document{"model": "local", "messages": []any{document{"role": "user", "content": "Reply with exactly " + marker + " and no other final text."}}, "max_tokens": budget, "temperature": 0, "chat_template_kwargs": document{"enable_thinking": thinking}, "stream": false}, 10*time.Minute)
	if err != nil {
		return nil, err
	}
	return validateChatResult(result, budget, thinking)
}

func validateChatResult(result document, budget int, thinking bool) (document, error) {
	choices := list(result["choices"])
	usage := mapping(result["usage"])
	nativeEOS := mapping(result["mlx_flash_compress"])["native_generation_metadata"] == true
	count, known := usage["completion_tokens"].(float64)
	if len(choices) != 1 || mapping(choices[0])["finish_reason"] != "stop" || !known || count < 0 || count > float64(budget) || (count == float64(budget) && !nativeEOS) {
		return nil, errors.New("qwen chat generation was truncated or lacked token accounting")
	}
	if err := finalText(text(mapping(mapping(choices[0])["message"])["content"]), thinking); err != nil {
		return nil, err
	}
	return usage, nil
}

func childEnv() []string {
	var env []string
	for _, key := range []string{"HOME", "PATH", "LANG", "LC_ALL", "TMPDIR"} {
		if value, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+value)
		}
	}
	return append(env, "HF_HUB_OFFLINE=1", "TRANSFORMERS_OFFLINE=1", "HF_HUB_DISABLE_TELEMETRY=1")
}

func runSmoke(ctx context.Context, gateway, raw string, result document, out io.Writer) error {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		return errors.New("real Qwen CI requires macOS Apple Silicon")
	}
	executable := os.Getenv("QWEN_MLX_FLASH_BIN")
	info, err := os.Stat(executable) // #nosec G703 -- Explicit operator-selected model/runtime path and fixed metadata or validated shard basename.
	if err != nil || !filepath.IsAbs(executable) || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
		return errors.New("QWEN_MLX_FLASH_BIN must name an installed external executable")
	}
	models := [][2]string{{"small", os.Getenv("QWEN_SMALL_MODEL_PATH")}}
	if gateway != "" || strings.EqualFold(os.Getenv("QWEN_RUN_LARGE"), "true") || os.Getenv("QWEN_RUN_LARGE") == "" {
		models = append(models, [2]string{"large", os.Getenv("QWEN_LARGE_MODEL_PATH")})
	}
	for _, model := range models {
		if err = validateModel(model[1]); err != nil {
			return err
		}
	}
	if err = preflightPorts([]int{19190, 19191}); err != nil {
		return err
	}
	for _, model := range models {
		err = func() (phaseErr error) {
			fmt.Fprintf(out, "Starting owned %s Qwen runtime on isolated CI ports.\n", model[0])
			process, e := startOwned([]string{executable, "--model", model[1], "--host", "127.0.0.1", "--port", "19191", "--speculative", "none", "--request-timeout", "600"}, filepath.Join(raw, model[0]+"-runtime.log"), childEnv())
			if e != nil {
				return e
			}
			defer func() {
				if cleanupErr := process.close(); cleanupErr != nil {
					phaseErr = cleanupErr
				}
			}()
			health, e := waitReady(ctx, process, runtimeURL+"/health", 2*time.Minute)
			if e != nil {
				return e
			}
			capabilities := mapping(health["capabilities"])
			thinking := false
			for _, key := range list(capabilities["chat_template_kwargs"]) {
				thinking = thinking || key == "enable_thinking"
			}
			if !thinking {
				return errors.New("native runtime does not advertise Qwen thinking profiles")
			}
			result["health"] = append(result["health"].([]any), document{"model_size": model[0], "status": health["status"], "capabilities": capabilities})
			if gateway == "" {
				for _, profile := range []bool{false, true} {
					usage, e := generateChat(ctx, profile)
					if e != nil {
						return e
					}
					result["checks"] = append(result["checks"].([]any), document{"model_size": model[0], "thinking": profile, "usage": usage, "passed": true})
				}
				return nil
			}
			upstream := runtimeURL + "/v1"
			gw, e := startOwned([]string{gateway, "--listen", "127.0.0.1:19190", "--upstream", upstream, "--role-haiku-upstream", upstream, "--role-sonnet-upstream", upstream, "--role-opus-upstream", upstream, "--claude-max-tokens", "8192", "--timeout", "10m"}, filepath.Join(raw, model[0]+"-gateway.log"), childEnv())
			if e != nil {
				return e
			}
			defer func() {
				if cleanupErr := gw.close(); cleanupErr != nil {
					phaseErr = cleanupErr
				}
			}()
			state, e := waitReady(ctx, gw, gatewayURL+"/health", 2*time.Minute)
			if e != nil {
				return e
			}
			if mapping(state["capabilities"])["claude_roles"] != true {
				return errors.New("gateway did not advertise all Claude roles")
			}
			result["health"] = append(result["health"].([]any), document{"model_size": model[0], "scope": "gateway", "capabilities": state["capabilities"]})
			roles := []string{"haiku"}
			if model[0] == "large" {
				roles = []string{"sonnet", "opus"}
			}
			for _, role := range roles {
				fmt.Fprintf(out, "Generating real Messages response: %s.\n", role)
				usage, e := generateRole(ctx, gatewayURL, role, false)
				if e != nil {
					return e
				}
				result["checks"] = append(result["checks"].([]any), document{"role": role, "model_size": model[0], "usage": usage, "passed": true})
			}
			if model[0] == "large" {
				usage, e := generateRole(ctx, gatewayURL, "sonnet", true)
				if e != nil {
					return e
				}
				result["checks"] = append(result["checks"].([]any), document{"role": "sonnet", "tool_bridge": true, "usage": usage, "passed": true})
			}
			return nil
		}()
		if err != nil {
			return err
		}
	}
	return nil
}

func sanitizedLogs(raw, artifacts string) error {
	replacements := []string{raw}
	for _, item := range os.Environ() {
		key, value, _ := strings.Cut(item, "=")
		if value != "" && (strings.HasSuffix(key, "_PATH") || key == "HOME" || key == "QWEN_MLX_FLASH_BIN") {
			replacements = append(replacements, value)
		}
	}
	sort.Slice(replacements, func(i, j int) bool { return len(replacements[i]) > len(replacements[j]) })
	logs, err := filepath.Glob(filepath.Join(raw, "*.log"))
	if err != nil {
		return err
	}
	for _, path := range logs {
		f, e := os.Open(path)
		if e != nil {
			return errors.New("could not read private smoke logs")
		}
		body, e := io.ReadAll(io.LimitReader(f, 16<<20))
		_ = f.Close()
		if e != nil {
			return errors.New("could not read private smoke logs")
		}
		content := string(bytes.ToValidUTF8(body, []byte("?")))
		for _, value := range replacements {
			content = strings.ReplaceAll(content, value, "<local-path>")
		}
		if e = os.WriteFile(filepath.Join(artifacts, filepath.Base(path)), []byte(content), 0600); e != nil {
			return errors.New("could not save sanitized smoke logs")
		}
	}
	return nil
}

// Run executes the offline hardware smoke on ports separate from the live lab.
func Run(args []string, _ io.Reader, out, stderr io.Writer) int {
	flags := flag.NewFlagSet("smoke", flag.ContinueOnError)
	flags.SetOutput(stderr)
	gateway := flags.String("gateway", "", "fresh gateway binary")
	artifacts := flags.String("artifacts", "qwen-metal-artifacts", "sanitized output directory")
	if flags.Parse(args) != nil || flags.NArg() != 0 {
		return 2
	}
	if err := os.MkdirAll(*artifacts, 0700); err != nil {
		fmt.Fprintln(stderr, "cannot create smoke artifacts")
		return 1
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	result := document{"passed": false, "health": []any{}, "checks": []any{}}
	err := func() error {
		lock := os.Getenv("QWEN_CI_LOCK_PATH")
		if lock == "" {
			lock = "/private/tmp/qwen-metal-ci.lock"
		}
		unlock, e := machineLock(ctx, lock, 30*time.Minute)
		if e != nil {
			return e
		}
		defer unlock()
		raw, e := os.MkdirTemp("", "qwen-ci-")
		if e != nil {
			return errors.New("cannot create private smoke logs")
		}
		defer func() { _ = os.RemoveAll(raw) }()
		e = runSmoke(ctx, *gateway, raw, result, out)
		logsErr := sanitizedLogs(raw, *artifacts)
		if e == nil {
			e = logsErr
		}
		return e
	}()
	if err != nil {
		result["error"] = err.Error()
		fmt.Fprintf(stderr, "Real Qwen smoke failed: %s; see sanitized server logs.\n", err)
	} else {
		result["passed"] = true
	}
	body, e := json.MarshalIndent(result, "", "  ")
	if e != nil || os.WriteFile(filepath.Join(*artifacts, "results.json"), append(body, '\n'), 0600) != nil {
		fmt.Fprintln(stderr, "cannot save smoke results")
		return 1
	}
	if err != nil {
		return 1
	}
	fmt.Fprintln(out, "Real Qwen generation checks passed; all owned processes cleaned up.")
	return 0
}

// RunRoles performs opt-in real inference against an already running lab.
func RunRoles(args []string, _ io.Reader, out, stderr io.Writer) int {
	flags := flag.NewFlagSet("role-check", flag.ContinueOnError)
	flags.SetOutput(stderr)
	base := flags.String("endpoint", "http://127.0.0.1:19090", "lab gateway")
	if flags.Parse(args) != nil || flags.NArg() != 0 {
		return 2
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	health, err := request(ctx, *base+"/health", nil, 5*time.Second)
	if err != nil || mapping(health["capabilities"])["claude_roles"] != true {
		fmt.Fprintln(stderr, "lab is unavailable or does not advertise Claude roles")
		return 1
	}
	for _, role := range []string{"haiku", "sonnet", "opus"} {
		start := time.Now()
		usage, err := generateRole(ctx, *base, role, false)
		if err != nil {
			fmt.Fprintf(stderr, "%s: %s\n", role, err)
			return 1
		}
		encoded, _ := json.Marshal(usage)
		fmt.Fprintf(out, "%s passed in %.2fs; usage=%s\n", role, time.Since(start).Seconds(), encoded)
	}
	return 0
}
