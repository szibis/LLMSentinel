package localgateway

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHybridPaidRoutingIsExplicit(t *testing.T) {
	if h, err := newHybridRouter(nil, nil); err != nil || h != nil {
		t.Fatalf("nil configuration enabled hybrid: %v %v", h, err)
	}
	cfg := &HybridConfig{OpenAIBaseURL: "https://api.openai.com/v1", OpenAIModel: "commercial-test", OpenAIAPIKeyEnv: "SENTINEL_TEST_OPENAI"}
	if _, err := newHybridRouter(cfg, nil); err == nil {
		t.Fatal("configured paid API accepted without opt-in")
	}
	cfg.AllowPaidAPI = true
	h, err := newHybridRouter(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ path, body, provider string }{
		{"/v1/responses", `{"model":"local","reasoning":{"effort":"high"},"input":"x"}`, "openai"},
		{"/v1/chat/completions", `{"model":"sentinel-opus","messages":[]}`, "openai"},
		{"/v1/responses", `{"model":"sentinel-sonnet","input":"x"}`, ""},
		{"/v1/messages", `{"model":"sentinel-opus","messages":[]}`, ""},
	} {
		decision, err := h.decide(tc.path, []byte(tc.body))
		if err != nil {
			t.Fatal(err)
		}
		if decision.Provider != tc.provider {
			t.Errorf("wrong routing %s: %v", tc.body, decision)
		}
		if tc.provider != "" && (decision.Model != "commercial-test" || decision.BillingClass != "api_usage") {
			t.Errorf("missing explicit model/billing: %v", decision)
		}
	}
}

func TestHybridRejectsUnsafeProviderConfiguration(t *testing.T) {
	for _, base := range []string{"http://api.openai.com/v1", "https://user:password@api.openai.com/v1", "https://api.openai.com/v1?key=x", "https://api.openai.com/v1#fragment"} {
		_, err := newHybridRouter(&HybridConfig{AllowPaidAPI: true, OpenAIBaseURL: base, OpenAIModel: "test", OpenAIAPIKeyEnv: "TEST_KEY"}, nil)
		if err == nil {
			t.Errorf("unsafe base accepted: %s", base)
		}
	}
	if _, err := newHybridRouter(&HybridConfig{AllowPaidAPI: true, OpenAIBaseURL: "https://api.openai.com/v1", OpenAIAPIKeyEnv: "TEST_KEY"}, nil); err == nil {
		t.Fatal("missing commercial model accepted")
	}
}

func TestHybridLeavesCheapRequestsAndCredentialsLocal(t *testing.T) {
	h, err := newHybridRouter(&HybridConfig{AllowPaidAPI: true, OpenAIBaseURL: "https://api.openai.com/v1", OpenAIModel: "test", OpenAIAPIKeyEnv: "TEST_KEY"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	body := `{"model":"sentinel-haiku","input":"Hi"}`
	r := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer user-subscription-token")
	if h.serve(httptest.NewRecorder(), r) {
		t.Fatal("cheap request escaped local route")
	}
	restored, _ := io.ReadAll(r.Body)
	if string(restored) != body {
		t.Fatalf("local body consumed: %s", restored)
	}
}

func TestHybridForwardsNativeProtocolWithSeparateAPIKeyAndCapture(t *testing.T) {
	t.Setenv("SENTINEL_HYBRID_TEST_KEY", "test-api-key-marker")
	recorder, err := newTrainingRecorder(&TrainingConfig{Directory: filepath.Join(t.TempDir(), "private")})
	if err != nil {
		t.Fatal(err)
	}
	h, err := newHybridRouter(&HybridConfig{AllowPaidAPI: true, AnthropicBaseURL: "https://api.anthropic.com", AnthropicModel: "vendor-opus-test", AnthropicAPIKeyEnv: "SENTINEL_HYBRID_TEST_KEY"}, recorder)
	if err != nil {
		t.Fatal(err)
	}
	h.transport = handlerTransport{func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-api-key-marker" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" || r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Errorf("wrong credentials/headers: %v", r.Header)
		}
		if r.URL.String() != "https://api.anthropic.com/v1/messages" {
			t.Errorf("wrong vendor URL: %s", r.URL)
		}
		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)
		if payload["model"] != "vendor-opus-test" {
			t.Errorf("model not replaced: %v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"msg_vendor","type":"message","model":"vendor-opus-test","content":[{"type":"text","text":"Answer"}],"stop_reason":"end_turn","usage":{"input_tokens":12,"output_tokens":4,"cache_read_input_tokens":3}}`)
	}}
	r := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(`{"model":"sentinel-opus","messages":[{"role":"user","content":"Question"}],"max_tokens":128}`))
	r.Header.Set("Authorization", "Bearer subscription-marker")
	r.Header.Set("x-api-key", "client-marker")
	r.Header.Set("Cookie", "private-marker")
	w := httptest.NewRecorder()
	if !h.serve(w, r) || w.Code != 200 || !strings.Contains(w.Body.String(), "msg_vendor") {
		t.Fatalf("native response lost: %d %s", w.Code, w.Body.String())
	}
	data, err := os.ReadFile(filepath.Join(recorder.directory, "training.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var capture map[string]any
	json.Unmarshal(data, &capture)
	if capture["provider"] != "anthropic" || capture["model"] != "vendor-opus-test" {
		t.Fatalf("wrong capture: %s", data)
	}
	quality := capture["quality"].(map[string]any)
	if quality["billing_class"] != "api_usage" || quality["status"] != "unscored" || quality["training_eligible"] != false {
		t.Fatalf("wrong billing/quality: %s", data)
	}
	if strings.Contains(string(data), "subscription-marker") || strings.Contains(string(data), "test-api-key-marker") {
		t.Fatalf("credentials leaked: %s", data)
	}
}

func TestHybridPreservesCompletedSSEAndExactUsage(t *testing.T) {
	t.Setenv("SENTINEL_HYBRID_TEST_KEY", "test-key-marker")
	recorder, err := newTrainingRecorder(&TrainingConfig{Directory: filepath.Join(t.TempDir(), "private")})
	if err != nil {
		t.Fatal(err)
	}
	h, err := newHybridRouter(&HybridConfig{AllowPaidAPI: true, OpenAIBaseURL: "https://api.openai.com/v1", OpenAIModel: "vendor-test", OpenAIAPIKeyEnv: "SENTINEL_HYBRID_TEST_KEY"}, recorder)
	if err != nil {
		t.Fatal(err)
	}
	stream := "event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_test\"}}\n\nevent: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_test\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":10,\"output_tokens\":5,\"total_tokens\":15,\"input_tokens_details\":{\"cached_tokens\":2}}}}\n\n"
	h.transport = handlerTransport{func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key-marker" {
			t.Error("configured key absent")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, stream)
	}}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"opus","input":"x","stream":true}`))
	if !h.serve(w, r) || w.Body.String() != stream {
		t.Fatalf("SSE changed: %s", w.Body.String())
	}
	data, _ := os.ReadFile(filepath.Join(recorder.directory, "training.jsonl"))
	var capture map[string]any
	json.Unmarshal(data, &capture)
	if capture["accepted"] != true || !strings.Contains(string(data), `"cached_tokens":2`) {
		t.Fatalf("completed usage missing: %s", data)
	}
}

func TestHybridNeverRetriesOrFallsBackOnVendorFailure(t *testing.T) {
	t.Setenv("SENTINEL_HYBRID_TEST_KEY", "test-key-marker")
	h, err := newHybridRouter(&HybridConfig{AllowPaidAPI: true, OpenAIBaseURL: "https://api.openai.com/v1", OpenAIModel: "test", OpenAIAPIKeyEnv: "SENTINEL_HYBRID_TEST_KEY"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	h.transport = handlerTransport{func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(429)
		fmt.Fprint(w, `{"error":{"type":"rate_limit_error","message":"Try later"}}`)
	}}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"opus","input":"x"}`))
	if !h.serve(w, r) || w.Code != 429 || calls != 1 || !strings.Contains(w.Body.String(), "rate_limit_error") {
		t.Fatalf("vendor error changed/retried: %d %d %s", w.Code, calls, w.Body.String())
	}
}

func TestHybridRedirectAndResponseLimitsFailClosed(t *testing.T) {
	t.Setenv("SENTINEL_HYBRID_TEST_KEY", "test-key-marker")
	for _, status := range []int{302, 200} {
		h, err := newHybridRouter(&HybridConfig{AllowPaidAPI: true, OpenAIBaseURL: "https://api.openai.com/v1", OpenAIModel: "test", OpenAIAPIKeyEnv: "SENTINEL_HYBRID_TEST_KEY"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		h.maxResponseBytes = 16
		calls := 0
		h.transport = handlerTransport{func(w http.ResponseWriter, r *http.Request) {
			calls++
			w.Header().Set("Location", "https://other.invalid")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			fmt.Fprint(w, `{"status":"completed","output":[]}`)
		}}
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"opus","input":"x"}`))
		if !h.serve(w, r) || w.Code != 502 || calls != 1 {
			t.Fatalf("unsafe vendor response accepted: %d calls=%d", w.Code, calls)
		}
	}
}

func TestHybridIncompleteStreamIsNotCapturedAsAnAnswer(t *testing.T) {
	t.Setenv("SENTINEL_HYBRID_TEST_KEY", "test-key-marker")
	recorder, err := newTrainingRecorder(&TrainingConfig{Directory: filepath.Join(t.TempDir(), "private")})
	if err != nil {
		t.Fatal(err)
	}
	h, err := newHybridRouter(&HybridConfig{AllowPaidAPI: true, OpenAIBaseURL: "https://api.openai.com/v1", OpenAIModel: "test", OpenAIAPIKeyEnv: "SENTINEL_HYBRID_TEST_KEY"}, recorder)
	if err != nil {
		t.Fatal(err)
	}
	stream := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n"
	h.transport = handlerTransport{func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, stream)
	}}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"opus","input":"x","stream":true}`))
	h.serve(w, r)
	data, _ := os.ReadFile(filepath.Join(recorder.directory, "training.jsonl"))
	var capture map[string]any
	json.Unmarshal(data, &capture)
	if capture["accepted"] != false || capture["output"] != "" {
		t.Fatalf("partial answer captured as completed: %s", data)
	}
}

func TestHybridCaptureLimitDoesNotChangeValidVendorStream(t *testing.T) {
	t.Setenv("SENTINEL_HYBRID_TEST_KEY", "test-key-marker")
	recorder, err := newTrainingRecorder(&TrainingConfig{Directory: filepath.Join(t.TempDir(), "private")})
	if err != nil {
		t.Fatal(err)
	}
	h, err := newHybridRouter(&HybridConfig{AllowPaidAPI: true, OpenAIBaseURL: "https://api.openai.com/v1", OpenAIModel: "test", OpenAIAPIKeyEnv: "SENTINEL_HYBRID_TEST_KEY"}, recorder)
	if err != nil {
		t.Fatal(err)
	}
	h.maxCaptureBytes = 8
	stream := "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"usage\":null}}\n\n"
	h.transport = handlerTransport{func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, stream)
	}}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"opus","input":"x","stream":true}`))
	h.serve(w, r)
	data, _ := os.ReadFile(filepath.Join(recorder.directory, "training.jsonl"))
	var capture map[string]any
	json.Unmarshal(data, &capture)
	if w.Body.String() != stream || capture["accepted"] != false || capture["quality"].(map[string]any)["capture_complete"] != false {
		t.Fatalf("capture overflow misreported/changedstream: %s %s", w.Body.String(), data)
	}
}

func TestHybridLivePoliciesRespectPaidConfiguration(t *testing.T) {
	h, err := newHybridRouter(&HybridConfig{AllowPaidAPI: true, OpenAIBaseURL: "https://api.openai.com/v1", OpenAIModel: "test", OpenAIAPIKeyEnv: "SENTINEL_HYBRID_TEST_KEY"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	cheap := []byte(`{"model":"sentinel-sonnet","input":"x"}`)
	if h.policyName() != "balanced" {
		t.Fatal("wrong default policy")
	}
	if err := h.setPolicy("quality"); err != nil {
		t.Fatal(err)
	}
	decision, err := h.decide("/v1/responses", cheap)
	if err != nil || decision.Provider != "openai" || decision.BillingClass != "api_usage" {
		t.Fatalf("quality not commercial: %v %v", decision, err)
	}
	if err := h.setPolicy("local-only"); err != nil {
		t.Fatal(err)
	}
	decision, err = h.decide("/v1/responses", []byte(`{"model":"opus","input":"x"}`))
	if err != nil || decision.Provider != "" {
		t.Fatalf("local-only used paid API: %v %v", decision, err)
	}
	if err := h.setPolicy("unknown"); err == nil || h.policyName() != "local-only" {
		t.Fatal("invalid policy mutated routing")
	}
	if err := h.setPolicy("quality"); err != nil {
		t.Fatal(err)
	}
	decision, err = h.decide("/v1/messages", []byte(`{"model":"opus","messages":[]}`))
	if err != nil || decision.Provider != "" {
		t.Fatal("quality invented absent Anthropic provider")
	}
}

func TestHybridPreservesLargeNumericArguments(t *testing.T) {
	t.Setenv("SENTINEL_HYBRID_TEST_KEY", "test-key-marker")
	h, err := newHybridRouter(&HybridConfig{AllowPaidAPI: true, AnthropicBaseURL: "https://api.anthropic.com", AnthropicModel: "test", AnthropicAPIKeyEnv: "SENTINEL_HYBRID_TEST_KEY"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	h.transport = handlerTransport{func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "9007199254740993") {
			t.Errorf("numeric argument changed: %s", body)
		}
		fmt.Fprint(w, `{"type":"message","stop_reason":"end_turn","content":[]}`)
	}}
	r := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(`{"model":"opus","messages":[{"role":"assistant","content":[{"type":"tool_use","input":{"id":9007199254740993}}]}]}`))
	h.serve(httptest.NewRecorder(), r)
}

func TestHybridUnterminatedSSECompletionIsNotAccepted(t *testing.T) {
	for _, suffix := range []string{"", "\n"} {
		body := `data: {"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}}}` + suffix
		_, completed := hybridSSEUsage("/v1/responses", body)
		if completed {
			t.Fatal("unterminated SSE frame treated as delivered completion")
		}
	}
	_, completed := hybridSSEUsage("/v1/responses", `data: {"type":"response.completed","response":{"status":"completed"}}`+"\n\n")
	if !completed {
		t.Fatal("terminated completion frame rejected")
	}
}
