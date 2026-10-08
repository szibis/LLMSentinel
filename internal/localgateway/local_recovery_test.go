package localgateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLocalRecoveryEscalatesTruncatedSmallOutputWithoutLeakingIt(t *testing.T) {
	smallCalls, largeCalls := 0, 0
	small := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			fmt.Fprint(w, `{"capabilities":{"model_family":"lfm2_moe","thinking_control":false,"reasoning_format":"think"}}`)
			return
		}
		smallCalls++
		fmt.Fprint(w, `{"choices":[{"message":{"content":"<think>PRIVATE_UNFINISHED"},"finish_reason":"length"}],"usage":{"prompt_tokens":7,"completion_tokens":512}}`)
	}))
	defer small.Close()
	large := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			fmt.Fprint(w, `{"capabilities":{"model_family":"gemma4","thinking_control":true,"reasoning_format":"gemma"}}`)
			return
		}
		largeCalls++
		var p map[string]any
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			t.Error(err)
		}
		encoded, _ := json.Marshal(p)
		if strings.Contains(string(encoded), "PRIVATE_UNFINISHED") {
			t.Error("unfinished private reasoning entered fallback history")
		}
		if p["max_tokens"] != float64(512) {
			t.Errorf("original budget exceeded: %v", p["max_tokens"])
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"<|channel>thought\n<channel|>done"},"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":5}}`)
	}))
	defer large.Close()
	g, err := New(Config{Upstream: large.URL + "/v1", RoleUpstreams: map[string]string{"haiku": small.URL + "/v1", "sonnet": large.URL + "/v1"}, LocalRoleRecovery: true, ClaudeAdapter: true, Timeout: time.Second, MaxRequestBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	w := httptest.NewRecorder()
	g.ServeHTTP(w, httptest.NewRequest("POST", "/v1/messages", strings.NewReader(`{"model":"sentinel-haiku","max_tokens":512,"messages":[{"role":"user","content":"Say done"}],"tools":[{"name":"Read","input_schema":{"type":"object"}}]}`)))
	if w.Code != 200 || smallCalls != 1 || largeCalls != 1 {
		t.Fatalf("bounded fallback failed: %d small=%d large=%d %s", w.Code, smallCalls, largeCalls, w.Body.String())
	}
	var response claudeResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Model != "sentinel-haiku" || response.Usage["input_tokens"] != 18 || response.Usage["output_tokens"] != 517 || strings.Contains(w.Body.String(), "PRIVATE") {
		t.Fatalf("alias/usage/privacy lost: %s", w.Body.String())
	}
}

func TestLocalRecoveryIsBoundedAndMissingUsageStaysUnknown(t *testing.T) {
	for _, failStrong := range []bool{false, true} {
		t.Run(fmt.Sprint(failStrong), func(t *testing.T) {
			calls := 0
			g, err := New(Config{Upstream: "http://127.0.0.1:19091/v1", RoleUpstreams: map[string]string{"haiku": "http://127.0.0.1:19092/v1", "sonnet": "http://127.0.0.1:19091/v1"}, LocalRoleRecovery: true, ClaudeAdapter: true, Timeout: time.Second, MaxRequestBytes: 4096})
			if err != nil {
				t.Fatal(err)
			}
			defer g.Close()
			g.claudeTransport = handlerTransport{func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/health" {
					family := `{"capabilities":{"model_family":"gemma4","thinking_control":true,"reasoning_format":"gemma"}}`
					if r.URL.Port() == "19092" {
						family = `{"capabilities":{"model_family":"lfm2_moe","thinking_control":false,"reasoning_format":"think"}}`
					}
					fmt.Fprint(w, family)
					return
				}
				calls++
				if r.URL.Port() == "19092" || failStrong {
					fmt.Fprint(w, `{"choices":[{"message":{"content":"<think>unfinished"},"finish_reason":"length"}]}`)
					return
				}
				fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"text\":\"done\",\"tool_calls\":[]}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":5}}`)
			}}
			req := claudeRequest{Model: "sentinel-haiku", MaxTokens: 512, Messages: []claudeMessage{{Role: "user", Content: json.RawMessage(`"Say done"`)}}}
			messages, err := prepareClaude(req)
			if err != nil {
				t.Fatal(err)
			}
			response, err := g.inferClaude(t.Context(), req, messages)
			if calls != 2 || g.localEscalations.Load() != 1 {
				t.Fatalf("unbounded recovery: %d", calls)
			}
			if failStrong && err == nil {
				t.Fatal("unfinished strong output accepted")
			}
			if !failStrong && (err != nil || response.UsageKnown) {
				t.Fatalf("missing rejected usage invented: %+v %v", response, err)
			}
		})
	}
}

func TestSuccessfulRecoveryKeepsSessionOnRecoveredRoute(t *testing.T) {
	g, err := New(Config{Upstream: "http://127.0.0.1:19091/v1", RoleUpstreams: map[string]string{"haiku": "http://127.0.0.1:19092/v1", "sonnet": "http://127.0.0.1:19091/v1"}, LocalRoleRecovery: true, ClaudeAdapter: true, Timeout: time.Second, MaxRequestBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	small, large := 0, 0
	g.claudeTransport = handlerTransport{func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			fmt.Fprint(w, `{"capabilities":{"model_family":"gemma4","thinking_control":true,"reasoning_format":"gemma"}}`)
			return
		}
		if r.URL.Port() == "19092" {
			small++
			fmt.Fprint(w, `{"choices":[{"message":{"content":"unfinished"},"finish_reason":"length"}]}`)
			return
		}
		large++
		var payload struct {
			MaxTokens int `json:"max_tokens"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if payload.MaxTokens != 1024 {
			t.Errorf("retained recovery exceeded original role cap: %d", payload.MaxTokens)
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"text\":\"done\",\"tool_calls\":[]}"},"finish_reason":"stop"}]}`)
	}}
	req := claudeRequest{Model: "sentinel-haiku", MaxTokens: 8192, Messages: []claudeMessage{{Role: "user", Content: json.RawMessage(`"Say done"`)}}}
	req.Metadata.UserID = "session-one"
	for i := 0; i < 2; i++ {
		messages, _ := prepareClaude(req)
		if _, err := g.inferClaude(t.Context(), req, messages); err != nil {
			t.Fatal(err)
		}
	}
	if small != 1 || large != 2 {
		t.Fatalf("recovered session bounced back: small=%d large=%d", small, large)
	}
	req.Metadata.UserID = "session-two"
	messages, _ := prepareClaude(req)
	if _, err := g.inferClaude(t.Context(), req, messages); err != nil {
		t.Fatal(err)
	}
	if small != 2 {
		t.Fatal("new session inherited recovery")
	}
	req.Metadata.UserID = "session-one"
	key := recoverySessionKey(t.Context(), req)
	g.recoveredSessions[key] = recoveredSession{role: "sonnet", expires: time.Now().Add(-time.Second)}
	messages, _ = prepareClaude(req)
	if _, err := g.inferClaude(t.Context(), req, messages); err != nil {
		t.Fatal(err)
	}
	if small != 3 {
		t.Fatal("expired session remained promoted")
	}
}

func TestHaikuToolAdmissionUsesStrongerRoleAndPreservesAliasAndBudget(t *testing.T) {
	g, err := New(Config{Upstream: "http://127.0.0.1:19091/v1", RoleUpstreams: map[string]string{"haiku": "http://127.0.0.1:19092/v1", "sonnet": "http://127.0.0.1:19091/v1"}, HaikuToolRole: "sonnet", ClaudeAdapter: true, Timeout: time.Second, MaxRequestBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	small, large := 0, 0
	g.claudeTransport = handlerTransport{func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			family, format := "gemma4", "gemma"
			if r.URL.Port() == "19092" {
				family, format = "lfm2_moe", "think"
			}
			json.NewEncoder(w).Encode(map[string]any{"capabilities": map[string]any{"model_family": family, "reasoning_format": format, "thinking_control": family == "gemma4"}})
			return
		}
		if r.URL.Port() == "19092" {
			small++
			fmt.Fprint(w, `{"choices":[{"message":{"content":"small plain"},"finish_reason":"stop"}]}`)
			return
		}
		large++
		var payload struct {
			MaxTokens int `json:"max_tokens"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if payload.MaxTokens != 1024 {
			t.Errorf("Haiku budget changed: %d", payload.MaxTokens)
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"<|tool_call>call:Read{file_path:<|\"|>a.txt<|\"|>}<tool_call|>"},"finish_reason":"stop"}]}`)
	}}
	req := claudeRequest{Model: "sentinel-haiku", MaxTokens: 4096, Tools: []claudeTool{{Name: "Read", Schema: map[string]any{"type": "object"}}}, Messages: []claudeMessage{{Role: "user", Content: json.RawMessage(`"Read a.txt"`)}}}
	messages, _ := prepareClaude(req)
	response, err := g.inferClaude(t.Context(), req, messages)
	if err != nil || large != 1 || small != 0 || response.Model != "sentinel-haiku" || len(response.Content) != 1 || response.Content[0].Type != "tool_use" {
		t.Fatalf("tool admission failed: %+v %v small=%d large=%d", response, err, small, large)
	}
	req.Tools = nil
	req.Messages[0].Content = json.RawMessage(`"Say hello"`)
	messages, _ = prepareClaude(req)
	if _, err = g.inferClaude(t.Context(), req, messages); err != nil {
		t.Fatal(err)
	}
	if small != 1 {
		t.Fatal("plain Haiku stopped using small model")
	}
}
