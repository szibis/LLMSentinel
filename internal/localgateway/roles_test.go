package localgateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type changingRoleRouter struct{ calls atomic.Int32 }

func (r *changingRoleRouter) Name() string { return "test-changing-route" }
func (r *changingRoleRouter) Select(_ context.Context, _ RouteTask) (RouteDecision, error) {
	role := "haiku"
	if r.calls.Add(1) > 1 {
		role = "sonnet"
	}
	return (RoleRouter{}).Select(context.Background(), RouteTask{Model: role})
}

func TestRoleCorrectionPinsEndpointAndPreservesToolEvidence(t *testing.T) {
	var calls atomic.Int32
	small := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p struct {
			Messages []map[string]string `json:"messages"`
		}
		json.NewDecoder(r.Body).Decode(&p)
		encoded, _ := json.Marshal(p.Messages)
		if !strings.Contains(string(encoded), "toolu_original") || !strings.Contains(string(encoded), "FILE_EVIDENCE") {
			t.Errorf("lost tool history: %s", encoded)
		}
		text := "<tool_call><function=Read>"
		if calls.Add(1) == 2 {
			text = `{"text":"FILE_EVIDENCE","tool_calls":[]}`
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": text}, "finish_reason": "stop"}}})
	}))
	defer small.Close()
	large := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("correction changed model"); fmt.Fprint(w, `{}`) }))
	defer large.Close()
	router := &changingRoleRouter{}
	g, err := New(Config{Upstream: large.URL + "/v1", RoleUpstreams: map[string]string{"haiku": small.URL + "/v1", "sonnet": large.URL + "/v1"}, Router: router, Timeout: time.Second, MaxRequestBytes: 8192, ClaudeAdapter: true})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	payload := `{"model":"sentinel-haiku","max_tokens":512,"messages":[{"role":"user","content":"Read hello.txt"},{"role":"assistant","content":[{"type":"tool_use","id":"toolu_original","name":"Read","input":{"file_path":"hello.txt"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_original","content":"FILE_EVIDENCE"}]}],"tools":[{"name":"Read","input_schema":{"type":"object","properties":{"file_path":{"type":"string"}},"required":["file_path"]}}]}`
	w := httptest.NewRecorder()
	g.ServeHTTP(w, httptest.NewRequest("POST", "/v1/messages", strings.NewReader(payload)))
	if w.Code != 200 || calls.Load() != 2 || router.calls.Load() != 1 || !strings.Contains(w.Body.String(), "FILE_EVIDENCE") {
		t.Fatalf("unpinned correction: %d calls=%d routes=%d %s", w.Code, calls.Load(), router.calls.Load(), w.Body.String())
	}
}

func TestIncompleteOpusReasoningNeverDispatchesToolInput(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": `<think>Perhaps {"text":"","tool_calls":[{"name":"Read","input":{"file_path":"invented"}}]}`}, "finish_reason": "stop"}}})
	}))
	defer backend.Close()
	g, err := New(Config{Upstream: backend.URL + "/v1", RoleUpstreams: map[string]string{"opus": backend.URL + "/v1"}, Router: RoleRouter{}, Timeout: time.Second, MaxRequestBytes: 4096, ClaudeAdapter: true, ClaudeBufferedValidation: true})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	w := httptest.NewRecorder()
	g.ServeHTTP(w, httptest.NewRequest("POST", "/v1/messages", strings.NewReader(`{"model":"sentinel-opus","max_tokens":1024,"stream":true,"messages":[{"role":"user","content":"Read a file"}],"tools":[{"name":"Read","input_schema":{"type":"object"}}]}`)))
	if w.Code != 422 || strings.Contains(w.Body.String(), "event:") || strings.Contains(w.Body.String(), `"type":"tool_use"`) {
		t.Fatalf("partial reasoning dispatched: %d %s", w.Code, w.Body.String())
	}
}

func TestConfiguredRoleDiscoveryAndSeparateRuntimeHealth(t *testing.T) {
	small := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"model":"SMALL","model_loaded":true}`) }))
	defer small.Close()
	large := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"model":"LARGE","model_loaded":false}`) }))
	defer large.Close()
	g, err := New(Config{Upstream: large.URL + "/v1", RoleUpstreams: map[string]string{"haiku": small.URL + "/v1", "sonnet": large.URL + "/v1", "opus": large.URL + "/v1"}, Timeout: time.Second, MaxRequestBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	for _, path := range []string{"/v1/models", "/health", "/sentinel/status"} {
		w := httptest.NewRecorder()
		g.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		if path == "/sentinel/status" {
			for _, want := range []string{"SMALL", "LARGE", "model_loaded"} {
				if !strings.Contains(w.Body.String(), want) {
					t.Errorf("missing runtime %s: %s", want, w.Body.String())
				}
			}
		} else if !strings.Contains(w.Body.String(), "sentinel-haiku") || !strings.Contains(w.Body.String(), "sentinel-opus") {
			t.Errorf("missing roles: %s", w.Body.String())
		}
	}
}

func TestClaudeRolesSelectEndpointsAndEffort(t *testing.T) {
	var mu sync.Mutex
	calls := map[string][]map[string]any{}
	backend := func(name string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var p map[string]any
			json.NewDecoder(r.Body).Decode(&p)
			mu.Lock()
			calls[name] = append(calls[name], p)
			mu.Unlock()
			answer := "Hello"
			if name == "large" && p["chat_template_kwargs"].(map[string]any)["enable_thinking"] == true {
				answer = "Reasoning\n</think>\n\nHello"
			}
			json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": answer}, "finish_reason": "stop"}}, "usage": map[string]int{"completion_tokens": 32}})
		}))
	}
	small, large := backend("small"), backend("large")
	defer small.Close()
	defer large.Close()
	g, err := New(Config{Upstream: large.URL + "/v1", RoleUpstreams: map[string]string{"haiku": small.URL + "/v1", "sonnet": large.URL + "/v1", "opus": large.URL + "/v1"}, Timeout: time.Second, MaxRequestBytes: 8192, ClaudeAdapter: true, Router: RoleRouter{}})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	for _, role := range []string{"haiku", "sonnet", "opus"} {
		r := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(fmt.Sprintf(`{"model":"sentinel-%s","max_tokens":20000,"messages":[{"role":"user","content":"Hello"}]}`, role)))
		w := httptest.NewRecorder()
		g.ServeHTTP(w, r)
		if w.Code != 200 || strings.Contains(w.Body.String(), "Reasoning") || !strings.Contains(w.Body.String(), "Hello") {
			t.Fatalf("%s: %d %s", role, w.Code, w.Body.String())
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(calls["small"]) != 1 || len(calls["large"]) != 2 {
		t.Fatalf("wrong endpoints: %v", calls)
	}
	for i, want := range []float64{4096, 8192} {
		if calls["large"][i]["max_tokens"] != want {
			t.Fatalf("large budget: %v", calls)
		}
	}
	if calls["small"][0]["max_tokens"] != float64(1024) || calls["small"][0]["model"] != "local" {
		t.Fatalf("small request: %v", calls)
	}
}

func TestUnknownRoleNeverReachesBackend(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("unexpected inference"); fmt.Fprint(w, `{}`) }))
	defer backend.Close()
	g, err := New(Config{Upstream: backend.URL + "/v1", Timeout: time.Second, MaxRequestBytes: 4096, ClaudeAdapter: true, Router: RoleRouter{}})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	w := httptest.NewRecorder()
	g.ServeHTTP(w, httptest.NewRequest("POST", "/v1/messages", strings.NewReader(`{"model":"unknown","max_tokens":128,"messages":[{"role":"user","content":"Hello"}]}`)))
	if w.Code < 400 {
		t.Fatalf("unknown role accepted: %s", w.Body.String())
	}
}

func TestRoleEndpointsRemainStrictLocal(t *testing.T) {
	for _, bad := range []string{"https://example.com/v1", "http://127.0.0.1:19092/v1?secret=x", "http://user:pass@127.0.0.1/v1"} {
		g, err := New(Config{Upstream: "http://127.0.0.1:19091/v1", RoleUpstreams: map[string]string{"haiku": bad}, Timeout: time.Second, MaxRequestBytes: 4096})
		if err == nil {
			g.Close()
			t.Fatalf("accepted %s", bad)
		}
	}
}

func TestRoleChatCompletionsUseSmallEndpoint(t *testing.T) {
	small := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p map[string]any
		json.NewDecoder(r.Body).Decode(&p)
		if p["model"] != "local" || p["max_tokens"] != float64(1024) {
			t.Errorf("payload: %v", p)
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"small"},"finish_reason":"stop"}]}`)
	}))
	defer small.Close()
	large := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("large backend selected"); fmt.Fprint(w, `{}`) }))
	defer large.Close()
	g, err := New(Config{Upstream: large.URL + "/v1", RoleUpstreams: map[string]string{"haiku": small.URL + "/v1"}, Router: RoleRouter{}, Timeout: time.Second, MaxRequestBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	server := httptest.NewServer(g)
	defer server.Close()
	res, err := http.Post(server.URL+"/v1/chat/completions", "application/json", strings.NewReader(`{"model":"sentinel-haiku","max_tokens":10000,"messages":[{"role":"user","content":"Hello"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 || !strings.Contains(string(body), "small") {
		t.Fatalf("response: %d %s", res.StatusCode, body)
	}
}
