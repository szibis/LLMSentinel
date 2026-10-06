package localgateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClaudeEmptyToolArgumentsRemainAnObject(t *testing.T) {
	encoded, err := json.Marshal(claudeBlock{Type: "tool_use", ID: "toolu_test", Name: "Status", Input: map[string]any{}})
	if err != nil || !strings.Contains(string(encoded), `"input":{}`) {
		t.Fatalf("invalid empty tool block: %s %v", encoded, err)
	}
}

func TestClaudeBufferedValidationRejectsTruncationBeforeOpeningStream(t *testing.T) {
	s := claudeServer(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"content":"partial"},"finish_reason":"length"}]}`)
	})
	s.gateway.cfg.ClaudeBufferedValidation = true
	res, _ := s.post("/v1/messages", `{"model":"local","max_tokens":128,"stream":true,"messages":[{"role":"user","content":"Read"}]}`)
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != 422 || res.Header.Get("x-should-retry") != "false" || strings.Contains(string(body), "event:") {
		t.Fatalf("opened stream/retryable response for invalid output: %d %s", res.StatusCode, body)
	}
}

func TestClaudeOutputBudgetBoundsLongClientRequests(t *testing.T) {
	s := claudeServer(t, func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)
		if payload["max_tokens"] != float64(768) {
			t.Errorf("long request not bounded: %v", payload["max_tokens"])
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"Hello"},"finish_reason":"stop"}]}`)
	})
	res, _ := s.post("/v1/messages", `{"model":"local","max_tokens":32768,"messages":[{"role":"user","content":"Hello"}]}`)
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("status %d", res.StatusCode)
	}
}

func TestClaudeBufferedValidationDeliversValidatedToolStream(t *testing.T) {
	s := claudeServer(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"text\":\"Reading\",\"tool_calls\":[{\"name\":\"Read\",\"input\":{\"file_path\":\"hello.txt\"}}]}"},"finish_reason":"stop"}],"usage":{"completion_tokens":20}}`)
	})
	s.gateway.cfg.ClaudeBufferedValidation = true
	res, _ := s.post("/v1/messages", `{"model":"local","max_tokens":128,"stream":true,"messages":[{"role":"user","content":"Read hello.txt"}],"tools":[{"name":"Read","input_schema":{"type":"object","required":["file_path"],"properties":{"file_path":{"type":"string"}}}}]}`)
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("invalid stream: %d %s", res.StatusCode, body)
	}
	for _, expected := range []string{"event: message_start", `"partial_json":"{\"file_path\":\"hello.txt\"}"`, `"stop_reason":"tool_use"`, "event: message_stop"} {
		if !strings.Contains(string(body), expected) {
			t.Errorf("missing %s: %s", expected, body)
		}
	}
}

func TestClaudeCancellationAndStreamErrorDoNotInventCompletion(t *testing.T) {
	g, err := New(Config{Upstream: "http://127.0.0.1:19091/v1", Timeout: time.Second, MaxRequestBytes: 4096, ClaudeAdapter: true})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	canceled := make(chan struct{})
	started := make(chan struct{})
	g.claudeTransport = cancelTransport{canceled, started}
	ctx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(`{"model":"local","max_tokens":128,"stream":true,"messages":[{"role":"user","content":"Hello"}]}`)).WithContext(ctx)
	recorder := httptest.NewRecorder()
	finished := make(chan struct{})
	go func() { g.ServeHTTP(recorder, request); close(finished) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		cancel()
		t.Fatal("backend not started")
	}
	cancel()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("handler did not cancel")
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("backend request did not cancel")
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "event: error") || strings.Contains(body, "message_stop") {
		t.Fatalf("canceled response claimed completion: %s", body)
	}
}

type cancelTransport struct{ canceled, started chan struct{} }

func (c cancelTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	close(c.started)
	<-r.Context().Done()
	close(c.canceled)
	return nil, r.Context().Err()
}

func TestClaudeTruncationNeverEmitsTools(t *testing.T) {
	s := claudeServer(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"tool_calls\":[{\"name\":\"Read\",\"input\":{\"file_path\":\"x\"}}]}"},"finish_reason":"length"}]}`)
	})
	res, _ := s.post("/v1/messages", `{"model":"local","max_tokens":128,"stream":true,"messages":[{"role":"user","content":"Read"}],"tools":[{"name":"Read","input_schema":{"type":"object"}}]}`)
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if !strings.Contains(string(body), "event: error") || strings.Contains(string(body), "content_block_start") {
		t.Fatalf("partial tools emitted: %s", body)
	}
}

type handlerTransport struct{ handler http.HandlerFunc }

func (h handlerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	recorder := httptest.NewRecorder()
	h.handler(recorder, r)
	return recorder.Result(), nil
}

type claudeHarness struct {
	gateway *Gateway
	URL     string
}

//nolint:unparam // Match the HTTP client API used by these protocol tests.
func (h *claudeHarness) post(path, body string) (*http.Response, error) {
	request := httptest.NewRequest("POST", h.URL+path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	h.gateway.ServeHTTP(recorder, request)
	return recorder.Result(), nil
}
func claudeServer(t *testing.T, handler http.HandlerFunc) *claudeHarness {
	t.Helper()
	g, err := New(Config{Upstream: "http://127.0.0.1:19091/v1", Timeout: 3 * time.Second, MaxRequestBytes: 65536, ClaudeAdapter: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(g.Close)
	g.claudeTransport = handlerTransport{handler}
	return &claudeHarness{g, "http://127.0.0.1:19090"}
}

func TestClaudeToolRoundTripAndStream(t *testing.T) {
	calls := 0
	server := claudeServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path %s", r.URL.Path)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["stream"] != false || body["model"] != "local" {
			t.Errorf("payload %v", body)
		}
		transcript, _ := json.Marshal(body["messages"])
		if !strings.Contains(string(transcript), "Read") {
			t.Error("tools absent from prompt")
		}
		output := `{"text":"","tool_calls":[{"name":"Read","input":{"file_path":"hello.txt"}}]}`
		if strings.Contains(string(transcript), "file evidence") {
			output = `{"text":"I read the file.","tool_calls":[]}`
		}
		calls++
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": output}, "finish_reason": "stop"}}, "usage": map[string]int{"prompt_tokens": 12, "completion_tokens": 8}})
	})
	tool := `"tools":[{"name":"Read","input_schema":{"type":"object","properties":{"file_path":{"type":"string"}},"required":["file_path"]}}]`
	payload := `{"model":"local","max_tokens":512,"messages":[{"role":"user","content":"Read hello.txt"}],` + tool + `}`
	res, err := server.post("/v1/messages?beta=true", payload)
	if err != nil {
		t.Fatal(err)
	}
	var message map[string]any
	json.NewDecoder(res.Body).Decode(&message)
	res.Body.Close()
	if res.StatusCode != 200 || message["stop_reason"] != "tool_use" {
		t.Fatalf("response %d %v", res.StatusCode, message)
	}
	blocks := message["content"].([]any)
	block := blocks[0].(map[string]any)
	if block["name"] != "Read" || block["type"] != "tool_use" {
		t.Fatal(block)
	}
	history := []any{map[string]any{"role": "user", "content": "Read hello.txt"}, map[string]any{"role": "assistant", "content": blocks}, map[string]any{"role": "user", "content": []any{map[string]any{"type": "tool_result", "tool_use_id": block["id"], "content": "file evidence"}}}}
	var next map[string]any
	json.Unmarshal([]byte(payload), &next)
	next["messages"] = history
	next["stream"] = true
	data, _ := json.Marshal(next)
	res, err = server.post("/v1/messages", string(data))
	if err != nil {
		t.Fatal(err)
	}
	stream, _ := io.ReadAll(res.Body)
	res.Body.Close()
	for _, event := range []string{"message_start", "content_block_start", "text_delta", "content_block_stop", "message_delta", "message_stop", "I read the file."} {
		if !strings.Contains(string(stream), event) {
			t.Errorf("missing %s: %s", event, stream)
		}
	}
	if calls != 2 {
		t.Fatalf("backend calls %d", calls)
	}
}

func TestClaudeRejectsInvalidModelToolOutput(t *testing.T) {
	for _, output := range []string{`{not JSON`, `{"tool_calls":[{"name":"DeleteEverything","input":{}}]}`, `{"tool_calls":[{"name":"Read","input":{"file_path":42}}]}`} {
		t.Run(output, func(t *testing.T) {
			s := claudeServer(t, func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": output}, "finish_reason": "stop"}}})
			})
			res, err := s.post("/v1/messages", `{"model":"local","max_tokens":128,"messages":[{"role":"user","content":"Read"}],"tools":[{"name":"Read","input_schema":{"type":"object","properties":{"file_path":{"type":"string"}},"required":["file_path"]}}]}`)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			if res.StatusCode != 422 {
				b, _ := io.ReadAll(res.Body)
				t.Fatalf("invalid tool accepted: %d %s", res.StatusCode, b)
			}
		})
	}
}

func TestClaudeCorrectsMalformedJSONOnceWithoutInventingTools(t *testing.T) {
	for _, recover := range []bool{true, false} {
		t.Run(fmt.Sprint(recover), func(t *testing.T) {
			calls := 0
			s := claudeServer(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				var payload struct {
					Messages []map[string]string `json:"messages"`
				}
				json.NewDecoder(r.Body).Decode(&payload)
				output := `{"text":"Hello","tool_calls=[]}`
				if calls == 2 {
					last := payload.Messages[len(payload.Messages)-1]
					if last["role"] != "user" || !strings.Contains(last["content"], "invalid JSON") {
						t.Error("missing explicit format correction")
					}
					if recover {
						output = `{"text":"Hello","tool_calls":[]}`
					}
				}
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": output}, "finish_reason": "stop"}}})
			})
			res, _ := s.post("/v1/messages", `{"model":"local","max_tokens":128,"messages":[{"role":"user","content":"Say hello"}],"tools":[{"name":"Read","input_schema":{"type":"object"}}]}`)
			defer res.Body.Close()
			body, _ := io.ReadAll(res.Body)
			if calls != 2 {
				t.Fatalf("correction attempts not bounded: %d", calls)
			}
			if recover {
				if res.StatusCode != 200 || !strings.Contains(string(body), `"text":"Hello"`) || strings.Contains(string(body), `"type":"tool_use"`) {
					t.Fatalf("invalid corrected response: %d %s", res.StatusCode, body)
				}
			} else if res.StatusCode != 422 || !strings.Contains(string(body), `"type":"invalid_request_error"`) {
				t.Fatalf("persistent format error reported as retryable outage: %d %s", res.StatusCode, body)
			}
		})
	}
}

func TestClaudeRejectsImagesAndUnknownToolResultsBeforeInference(t *testing.T) {
	s := claudeServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("unsupported input reached inference") })
	for _, content := range []string{`[{"type":"image","source":{}}]`, `[{"type":"tool_result","tool_use_id":"unknown","content":"x"}]`} {
		payload := fmt.Sprintf(`{"model":"local","max_tokens":128,"messages":[{"role":"user","content":%s}]}`, content)
		res, err := s.post("/v1/messages", payload)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != 400 {
			t.Errorf("unsupported input status %d", res.StatusCode)
		}
	}
}
