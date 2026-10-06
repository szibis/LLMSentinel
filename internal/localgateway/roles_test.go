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

func TestReasoningNeverLeaksFromAnyRole(t *testing.T) {
	for _, tc := range []struct {
		text, want        string
		thinking, invalid bool
	}{
		{"<think>private</think>answer", "answer", false, false},
		{"private</think>answer", "answer", false, false},
		{"<think>unfinished", "", false, true},
		{"<|channel>thought\nprivate\n<channel|>answer", "answer", true, false},
		{"<|channel>thought\nprivate\n<channel|>answer", "answer", false, false},
		{"<|channel>thought\nunfinished", "", false, true},
		{"<|channel>thought\nprivate<channel|><|channel>analysis", "", true, true},
	} {
		t.Run(tc.text, func(t *testing.T) {
			got, err := finalRoleText(tc.text, tc.thinking)
			if (err != nil) != tc.invalid || got != tc.want {
				t.Fatalf("got %q, %v; want %q invalid=%t", got, err, tc.want, tc.invalid)
			}
		})
	}
	if _, err := finalRoleText("private</think>answer", true, "gemma"); err == nil {
		t.Fatal("Gemma accepted a Qwen reasoning delimiter")
	}
}

func TestCacheScopePartitionsSessionsAndKeepsCorrectionTogether(t *testing.T) {
	var scopes []string
	g, err := New(Config{Upstream: "http://127.0.0.1:19091/v1", Timeout: time.Second, MaxRequestBytes: 4096, ClaudeAdapter: true})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	g.claudeTransport = handlerTransport{func(w http.ResponseWriter, r *http.Request) {
		var p map[string]any
		json.NewDecoder(r.Body).Decode(&p)
		scope, _ := p["cache_scope"].(string)
		scopes = append(scopes, scope)
		answer := `{"text":"done","tool_calls":[]}`
		if len(scopes) == 1 {
			answer = "<tool_call>incomplete"
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": answer}, "finish_reason": "stop"}}})
	}}
	for _, session := range []string{"private-session-a", "private-session-a", "private-session-b", "", ""} {
		r := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(`{"model":"local","max_tokens":512,"messages":[{"role":"user","content":"Finish"}],"tools":[{"name":"Read","input_schema":{"type":"object"}}]}`))
		r.Header.Set("X-Session-ID", session)
		w := httptest.NewRecorder()
		g.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
	}
	if len(scopes) != 6 || scopes[0] == "" || scopes[0] != scopes[1] || scopes[1] != scopes[2] || scopes[2] == scopes[3] || scopes[4] == scopes[5] {
		t.Fatalf("session partition/recovery scopes: %q", scopes)
	}
	for _, scope := range scopes {
		if strings.Contains(scope, "private-session") {
			t.Fatal("raw session exposed")
		}
	}
}

func TestRoleHealthFailureDoesNotGuessThinkingSupport(t *testing.T) {
	g, err := New(Config{Upstream: "http://127.0.0.1:19091/v1", RoleUpstreams: map[string]string{"haiku": "http://127.0.0.1:19091/v1"}, Timeout: time.Second, MaxRequestBytes: 4096, ClaudeAdapter: true})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	inferences := 0
	g.claudeTransport = handlerTransport{func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			inferences++
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}}
	w := httptest.NewRecorder()
	g.ServeHTTP(w, httptest.NewRequest("POST", "/v1/messages", strings.NewReader(`{"model":"sentinel-haiku","max_tokens":512,"messages":[{"role":"user","content":"Hello"}]}`)))
	if w.Code < 400 || inferences != 0 {
		t.Fatalf("health failure dispatched %d requests; status %d", inferences, w.Code)
	}
}

func TestRoleHealthRejectsUnsupportedOrInconsistentFamilies(t *testing.T) {
	for _, body := range []string{
		`{"capabilities":{"model_family":"unknown","thinking_control":true,"reasoning_format":"think"}}`,
		`{"capabilities":{"model_family":"gemma4","thinking_control":true,"reasoning_format":"think"}}`,
		`{"capabilities":{"model_family":"lfm2_moe","thinking_control":true,"reasoning_format":"think"}}`,
		`{"capabilities":{"model_family":"qwen3_5","thinking_control":false,"reasoning_format":"think"}}`,
	} {
		g, err := New(Config{Upstream: "http://127.0.0.1:19091/v1", RoleUpstreams: map[string]string{"haiku": "http://127.0.0.1:19091/v1"}, Timeout: time.Second, MaxRequestBytes: 4096, ClaudeAdapter: true})
		if err != nil {
			t.Fatal(err)
		}
		inferences := 0
		g.claudeTransport = handlerTransport{func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/health" {
				fmt.Fprint(w, body)
				return
			}
			inferences++
			fmt.Fprint(w, `{"choices":[{"message":{"content":"Hello"},"finish_reason":"stop"}]}`)
		}}
		w := httptest.NewRecorder()
		g.ServeHTTP(w, httptest.NewRequest("POST", "/v1/messages", strings.NewReader(`{"model":"sentinel-haiku","max_tokens":512,"messages":[{"role":"user","content":"Hello"}]}`)))
		g.Close()
		if w.Code < 400 || inferences != 0 {
			t.Errorf("accepted %s: status %d, inference calls %d", body, w.Code, inferences)
		}
	}
}

func TestPlainRoleChatBuffersAndValidatesReasoning(t *testing.T) {
	for _, family := range []string{"lfm2_moe", "gemma4"} {
		for _, stream := range []bool{false, true} {
			for _, complete := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/stream=%t/complete=%t", family, stream, complete), func(t *testing.T) {
					backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.URL.Path == "/health" {
							format := "think"
							if family == "gemma4" {
								format = "gemma"
							}
							json.NewEncoder(w).Encode(map[string]any{"capabilities": map[string]any{"model_family": family, "thinking_control": family == "gemma4", "reasoning_format": format}})
							return
						}
						answer := "<think>private-reasoning"
						if family == "gemma4" {
							answer = "<|channel>thought\nprivate-reasoning"
						}
						if complete {
							if family == "gemma4" {
								answer += "\n<channel|>Hello"
							} else {
								answer += "</think>Hello"
							}
						}
						json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": answer}, "finish_reason": "stop"}}})
					}))
					defer backend.Close()
					role := "haiku"
					if family == "gemma4" {
						role = "opus"
					}
					g, err := New(Config{Upstream: backend.URL + "/v1", RoleUpstreams: map[string]string{role: backend.URL + "/v1"}, Timeout: time.Second, MaxRequestBytes: 4096})
					if err != nil {
						t.Fatal(err)
					}
					defer g.Close()
					w := httptest.NewRecorder()
					g.ServeHTTP(w, httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(fmt.Sprintf(`{"model":"sentinel-%s","max_tokens":512,"stream":%t,"messages":[{"role":"user","content":"Hello"}]}`, role, stream))))
					if strings.Contains(w.Body.String(), "private-reasoning") {
						t.Fatal("raw role reasoning leaked")
					}
					if complete {
						if w.Code != 200 || !strings.Contains(w.Body.String(), "Hello") {
							t.Fatalf("%d %s", w.Code, w.Body.String())
						}
					} else {
						if w.Code != 422 || strings.Contains(w.Body.String(), "data:") {
							t.Fatalf("incomplete output accepted: %d %s", w.Code, w.Body.String())
						}
					}
				})
			}
		}
	}
}

func TestModelFamiliesUseAdvertisedThinkingAndJSONTools(t *testing.T) {
	for _, family := range []string{"lfm2_moe", "gemma4"} {
		t.Run(family, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/health" {
					format := "think"
					if family == "gemma4" {
						format = "gemma"
					}
					json.NewEncoder(w).Encode(map[string]any{"capabilities": map[string]any{"model_family": family, "thinking_control": family == "gemma4", "reasoning_format": format}})
					return
				}
				var p map[string]any
				json.NewDecoder(r.Body).Decode(&p)
				if family == "lfm2_moe" && p["chat_template_kwargs"] != nil {
					t.Error("LFM received unsupported thinking option")
				}
				if family == "gemma4" && p["chat_template_kwargs"].(map[string]any)["enable_thinking"] != true {
					t.Error("Gemma thinking not enabled")
				}
				msgs, _ := json.Marshal(p["messages"])
				if strings.Contains(string(msgs), "Qwen tool format") || !strings.Contains(string(msgs), "tool_calls") {
					t.Errorf("wrong family instructions: %s", msgs)
				}
				answer := `<think>private</think>{"text":"","tool_calls":[{"name":"Read","input":{"file_path":"hello.txt"}}]}`
				if family == "gemma4" {
					answer = "<|channel>thought\nprivate\n<channel|>" + `{"text":"","tool_calls":[{"name":"Read","input":{"file_path":"hello.txt"}}]}`
				}
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": answer}, "finish_reason": "stop"}}})
			})
			role := "haiku"
			if family == "gemma4" {
				role = "opus"
			}
			g, err := New(Config{Upstream: "http://127.0.0.1:19091/v1", RoleUpstreams: map[string]string{role: "http://127.0.0.1:19091/v1"}, Timeout: time.Second, MaxRequestBytes: 4096, ClaudeAdapter: true})
			if err != nil {
				t.Fatal(err)
			}
			defer g.Close()
			g.claudeTransport = handlerTransport{handler}
			w := httptest.NewRecorder()
			g.ServeHTTP(w, httptest.NewRequest("POST", "/v1/messages", strings.NewReader(fmt.Sprintf(`{"model":"sentinel-%s","max_tokens":512,"messages":[{"role":"user","content":"Read hello.txt"}],"tools":[{"name":"Read","input_schema":{"type":"object","properties":{"file_path":{"type":"string"}},"required":["file_path"]}}]}`, role))))
			if w.Code != 200 || !strings.Contains(w.Body.String(), `"type":"tool_use"`) || strings.Contains(w.Body.String(), "private") {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
		})
	}
}

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
		if r.URL.Path == "/health" {
			fmt.Fprint(w, `{"capabilities":{"model_family":"qwen3_5","thinking_control":true,"reasoning_format":"think"}}`)
			return
		}
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
		if r.URL.Path == "/health" {
			fmt.Fprint(w, `{"capabilities":{"model_family":"qwen3_5","thinking_control":true,"reasoning_format":"think"}}`)
			return
		}
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
			if r.URL.Path == "/health" {
				fmt.Fprint(w, `{"capabilities":{"model_family":"qwen3_5","thinking_control":true,"reasoning_format":"think"}}`)
				return
			}
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
		if r.URL.Path == "/health" {
			fmt.Fprint(w, `{"capabilities":{"model_family":"qwen3_5","thinking_control":true,"reasoning_format":"think"}}`)
			return
		}
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
