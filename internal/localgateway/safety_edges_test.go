package localgateway

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSafetyRuntimeCapabilitiesFailClosed(t *testing.T) {
	g, err := New(edgeLocalConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	for _, body := range []string{`{`, strings.Repeat("x", 65537), `{}`, `{"capabilities":{"family":"unknown"}}`, `{"capabilities":{"family":"gemma4","thinking_control":false,"reasoning_format":"gemma"}}`} {
		g.claudeTransport = handlerTransport{handler: func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/health" {
				t.Fatal("inference dispatched", r.URL.Path)
			}
			_, _ = io.WriteString(w, body)
		}}
		if _, err := g.modelCapabilities(context.Background(), g.upstream); err == nil {
			t.Fatal("unsafe capabilities accepted", body)
		}
	}
	g.claudeTransport = handlerTransport{handler: func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"capabilities":{"chat_template_kwargs":["other","enable_thinking"]}}`)
	}}
	if caps, err := g.modelCapabilities(context.Background(), g.upstream); err != nil || caps.Family != "qwen" {
		t.Fatal(caps, err)
	}
}

func TestSafetyStatusCannotAdvertiseInvalidRuntimeHealth(t *testing.T) {
	for _, body := range []string{"not json", strings.Repeat("x", 65537), "unavailable"} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if body == "unavailable" {
				w.WriteHeader(503)
			}
			_, _ = io.WriteString(w, body)
		}))
		cfg := edgeLocalConfig()
		cfg.Upstream = srv.URL + "/v1"
		g, err := New(cfg)
		if err != nil {
			srv.Close()
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		g.status(w, httptest.NewRequest("GET", "/sentinel/status", nil))
		if w.Code != 502 || strings.Contains(w.Body.String(), `"scope":"runtime"`) {
			t.Fatal(w.Code, w.Body.String())
		}
		g.Close()
		srv.Close()
	}
	srv := httptest.NewServer(http.NotFoundHandler())
	cfg := edgeLocalConfig()
	cfg.Upstream = srv.URL + "/v1"
	srv.Close()
	g, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	w := httptest.NewRecorder()
	g.status(w, httptest.NewRequest("GET", "/sentinel/status", nil))
	if w.Code != 502 || !strings.Contains(w.Body.String(), "unavailable") {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestSafetyHybridRejectsUnreadableBodiesBeforeDispatch(t *testing.T) {
	h, err := newHybridRouter(&HybridConfig{AllowPaidAPI: true, OpenAIBaseURL: "https://api.openai.com/v1", OpenAIModel: "test", OpenAIAPIKeyEnv: "SENTINEL_SAFETY_UNSET"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	for _, path := range []string{"/v1/messages", "/v1/responses"} {
		r := httptest.NewRequest("POST", path, nil)
		r.Body = failedReader{}
		w := httptest.NewRecorder()
		if !h.serve(w, r) || w.Code != 413 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}

func TestSafetyContentBlocksRejectUnsupportedAndExcessCalls(t *testing.T) {
	for _, blocks := range [][]any{{42}, {map[string]any{"type": "text", "text": 42}}, {map[string]any{"type": "image"}}, {map[string]any{"type": "tool_use", "name": 42}}} {
		if _, err := normalizeContentBlocks(blocks); err == nil {
			t.Fatal("invalid blocks accepted", blocks)
		}
	}
	blocks := []any{}
	for i := 0; i < 9; i++ {
		blocks = append(blocks, map[string]any{"type": "tool_use", "name": "read", "input": map[string]any{"path": "literal"}})
	}
	if _, err := normalizeContentBlocks(blocks); err == nil || !strings.Contains(err.Error(), "too many") {
		t.Fatal(err)
	}
}

func TestSafetyChatRejectsUnsupportedHistoryAndFormats(t *testing.T) {
	for _, body := range []string{`{"messages":[{"role":"user","content":[{"type":"image"}]}]}`, `{"messages":[{"role":"unknown"}]}`, `{"messages":[{"role":"user","tool_calls":[{"type":"function"}]}]}`, `{"messages":[{"role":"assistant","tool_calls":[{"type":"custom"}]}]}`, `{"tools":[{"type":"custom"}]}`, `{"functions":[]}`, `{"response_format":{"type":"json_object"}}`} {
		var req chatProtocolRequest
		if err := json.Unmarshal([]byte(body), &req); err != nil {
			t.Fatal(err)
		}
		if _, _, err := prepareChatProtocol(req, false, 1024); err == nil {
			t.Fatal("unsupported protocol accepted", body)
		}
	}
}

func TestSafetyTrainingFailureKeepsPrivateCapturesIntact(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	r, err := newTrainingRecorder(&TrainingConfig{Directory: dir, MaxBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "training.jsonl")
	r.record(context.Background(), trainingEvent{Quality: map[string]any{"unsupported": make(chan int)}})
	if data, err := os.ReadFile(path); err != nil || len(data) != 0 {
		t.Fatal("unsupported data wrote capture", err)
	}
	r.record(context.Background(), trainingEvent{Quality: map[string]any{"large": strings.Repeat("x", 2000)}})
	if data, err := os.ReadFile(path); err != nil || len(data) != 0 {
		t.Fatal("oversize data wrote capture", err)
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("a", 1024)), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path+".1", 0700); err != nil {
		t.Fatal(err)
	}
	if err := r.append([]byte("unsafe rotation")); err == nil {
		t.Fatal("unsafe rotation accepted")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != strings.Repeat("a", 1024) {
		t.Fatal("capture corrupted", err)
	}
	if err := os.Chmod(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := r.append([]byte("unsafe directory")); err == nil {
		t.Fatal("public directory accepted")
	}
}

type failedReader struct{}

func (failedReader) Read([]byte) (int, error) { return 0, errors.New("synthetic read error") }
func (failedReader) Close() error             { return nil }

func edgeLocalConfig() Config {
	return Config{Upstream: "http://127.0.0.1:19099/v1", Timeout: time.Second, MaxRequestBytes: 4096}
}

func TestSafetyGatewayRejectsMalformedAndUnavailableContracts(t *testing.T) {
	g, err := New(edgeLocalConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{{"POST", "/health", "", 405}, {"POST", "/sentinel/status", "", 405}, {"GET", "/v1/messages", "", 501}, {"POST", "/v1/responses", "", 501}, {"HEAD", "/api/hello", "", 200}, {"GET", "/api/hello", "", 405}, {"POST", "/v1/messages/count_tokens", "", 501}, {"POST", "/v1/models", "", 405}, {"GET", "/v1/chat/completions", "", 405}, {"POST", "/v1/chat/completions", "", 400}, {"POST", "/v1/chat/completions", "[]", 400}, {"POST", "/v1/chat/completions", "{", 400}, {"POST", "/v1/chat/completions", `{"messages":[{"role":"function"}]}`, 501}, {"GET", "/missing", "", 404}, {"PUT", "/sentinel/control", "", 405}, {"POST", "/sentinel/control", "{}", 400}, {"GET", "/sentinel/training/events", "", 405}, {"POST", "/sentinel/training/events", "{}", 409}} {
		w := httptest.NewRecorder()
		g.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)))
		if w.Code != tc.status {
			t.Fatal(tc, w.Code, w.Body.String())
		}
	}
	r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	r.Body = failedReader{}
	w := httptest.NewRecorder()
	g.ServeHTTP(w, r)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "Cannot read") {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, mutate := range []func(*Config){func(c *Config) { c.LearningOnly = true }, func(c *Config) { c.RoleUpstreams = map[string]string{"unknown": "http://127.0.0.1:1"} }, func(c *Config) { c.RoleUpstreams = map[string]string{"opus": "https://remote.example"} }, func(c *Config) { c.HaikuToolRole = "opus" }, func(c *Config) { c.ClaudeMaxTokens = -1 }, func(c *Config) { c.ClaudeMaxTokens = 32769 }, func(c *Config) { c.Timeout = 11 * time.Minute }, func(c *Config) { c.MaxRequestBytes = 0 }} {
		cfg := edgeLocalConfig()
		mutate(&cfg)
		if _, err := New(cfg); err == nil {
			t.Fatal("invalid config accepted", cfg)
		}
	}
}

func TestSafetyTrainingAndControlFailuresDoNotMutate(t *testing.T) {
	cfg := edgeLocalConfig()
	cfg.Training = &TrainingConfig{Directory: filepath.Join(t.TempDir(), "training")}
	g, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	for _, tc := range []struct {
		path, body, contentType, origin string
		status                          int
	}{{"/sentinel/training/events", "{}", "text/plain", "", 400}, {"/sentinel/training/events", "{}", "application/json", "browser", 400}, {"/sentinel/training/events", "null", "application/json", "", 400}, {"/sentinel/training/events", "{", "application/json", "", 400}, {"/sentinel/training/events", `{"client":"unknown"}`, "application/json", "", 400}, {"/sentinel/training/events", `{"client":"claude","source":"wrong"}`, "application/json", "", 400}, {"/sentinel/training/events", `{"client":"codex","source":"client_hook"}`, "application/json", "", 400}, {"/sentinel/training/events", strings.Repeat("x", 5000), "application/json", "", 413}, {"/sentinel/control", "{}", "text/plain", "", 400}, {"/sentinel/control", "{}", "application/json", "browser", 400}, {"/sentinel/control", "{} {}", "application/json", "", 400}, {"/sentinel/control", strings.Repeat("x", 9000), "application/json", "", 413}, {"/sentinel/control", `{"policy":"balanced"}`, "application/json", "", 409}} {
		r := httptest.NewRequest("POST", tc.path, strings.NewReader(tc.body))
		r.Header.Set("Content-Type", tc.contentType)
		r.Header.Set("Origin", tc.origin)
		w := httptest.NewRecorder()
		g.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatal(tc, w.Code, w.Body.String())
		}
	}
	for _, path := range []string{"/sentinel/control", "/sentinel/training/events"} {
		r := httptest.NewRequest("POST", path, nil)
		r.Header.Set("Content-Type", "application/json")
		r.Body = failedReader{}
		w := httptest.NewRecorder()
		g.ServeHTTP(w, r)
		if w.Code != 413 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if !g.training.captureEnabled() {
		t.Fatal("failed control disabled capture")
	}
}

func TestSafetyToolSchemaRejectsMalformedNestedArguments(t *testing.T) {
	for _, tc := range []struct {
		schema         map[string]any
		valid, invalid any
	}{
		{map[string]any{"type": "string"}, "value", float64(1)}, {map[string]any{"type": "boolean"}, true, "true"}, {map[string]any{"type": "integer"}, float64(2), float64(2.5)}, {map[string]any{"type": "number"}, float64(2.5), "2.5"}, {map[string]any{"type": "null"}, nil, "null"},
		{map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, []any{"x"}, []any{true}},
		{map[string]any{"type": "object", "required": []any{"key"}, "properties": map[string]any{"key": map[string]any{"type": "boolean"}}, "additionalProperties": false}, map[string]any{"key": true}, map[string]any{"key": "true"}},
		{map[string]any{"type": "string", "enum": []any{"allowed"}}, "allowed", "not-allowed"},
	} {
		if err := validateInput(tc.schema, tc.valid); err != nil {
			t.Fatal(tc, err)
		}
		if err := validateInput(tc.schema, tc.invalid); err == nil {
			t.Fatal("invalid argument accepted", tc)
		}
	}
	for _, value := range []any{nil, map[string]any{}, map[string]any{"key": true, "extra": 1}} {
		if err := validateInput(map[string]any{"type": "object", "required": []any{"key"}, "additionalProperties": false}, value); err == nil {
			t.Fatal(value)
		}
	}
	if err := validateInput(map[string]any{"type": "array"}, "not-array"); err == nil {
		t.Fatal("nonarray accepted")
	}
}

func TestSafetyPatchContractFailures(t *testing.T) {
	for _, tc := range []struct {
		patch string
		valid bool
	}{
		{"*** Begin Patch\n*** Add File: x\n+literal\n*** End Patch", true},
		{"*** Begin Patch\n*** Delete File: x\n*** End Patch", true},
		{"*** Begin Patch\n*** Update File: x\n*** Move to: y\n@@ context\n-old\n+new\n*** End of File\n*** End Patch", true},
		{"*** Begin Patch\n*** Add File: x\n*** End Patch", false},
		{"*** Begin Patch\n*** Update File: x\n*** End of File\n*** End Patch", false},
		{"*** Begin Patch\n--- x\n+++ y\n*** End Patch", false},
		{"*** Begin Patch\n*** End Patch", false},
	} {
		if err := validatePatchSyntax(tc.patch); (err == nil) != tc.valid {
			t.Fatal(tc, err)
		}
	}
}

func TestSafetyHybridUsageNeedsActualCompletion(t *testing.T) {
	for _, path := range []string{"/v1/messages", "/v1/responses", "/v1/chat/completions", "/unsupported"} {
		if usage, ok := hybridJSONUsage(path, []byte("{")); usage != nil || ok {
			t.Fatal(path, usage, ok)
		}
		if _, ok := hybridJSONUsage(path, []byte(`{"usage":{"input_tokens":17}}`)); ok {
			t.Fatal("usage alone claimed completion")
		}
		if _, ok := hybridSSEUsage(path, "data: invalid\n\n:heartbeat\n\n"); ok {
			t.Fatal("invalid SSE completed")
		}
	}
	message := `{"type":"message","stop_reason":"end_turn","usage":{"input_tokens":17}}`
	if usage, ok := hybridJSONUsage("/v1/messages", []byte(message)); !ok || usage["input_tokens"] != json.Number("17") {
		t.Fatal(usage, ok)
	}
	frames := "data: " + `{"type":"message_start","message":{"usage":{"input_tokens":17}}}` + "\n\ndata: " + `{"type":"message_delta","usage":{"output_tokens":3}}` + "\n\ndata: " + `{"type":"message_stop"}` + "\n\n"
	usage, ok := hybridSSEUsage("/v1/messages", frames)
	if !ok || usage["input_tokens"] != json.Number("17") || usage["output_tokens"] != json.Number("3") {
		t.Fatal(usage, ok)
	}
	if _, ok := hybridSSEUsage("/v1/messages", strings.TrimSuffix(frames, "\n\n")); ok {
		t.Fatal("unterminated completion accepted")
	}
	completed := "data: " + `{"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":17}}}` + "\n\n"
	if _, ok := hybridSSEUsage("/v1/responses", completed); !ok {
		t.Fatal("complete response not recognized")
	}
}

func TestSafetyInferenceNetworkFailureKeepsUsageUnknown(t *testing.T) {
	g, err := New(edgeLocalConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	g.claudeTransport = handlerTransport{handler: func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
		io.WriteString(w, "runtime unavailable")
	}}
	req := claudeRequest{Model: "local", MaxTokens: 128, Messages: []claudeMessage{{Role: "user", Content: json.RawMessage(`"hello"`)}}}
	messages, err := prepareClaude(req)
	if err != nil {
		t.Fatal(err)
	}
	result, err := g.inferClaude(context.Background(), req, messages)
	if err == nil || result.UsageKnown || len(result.Content) != 0 {
		t.Fatal("failed inference invented output", result, err)
	}
}
