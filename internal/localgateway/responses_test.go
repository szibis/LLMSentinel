package localgateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResponsesTextProtocol(t *testing.T) {
	s := claudeServer(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"content":"Hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":12,"completion_tokens":3}}`)
	})
	res, _ := s.post("/v1/responses", `{"model":"local","input":"Hi","store":false,"max_output_tokens":128}`)
	defer res.Body.Close()
	var response map[string]any
	json.NewDecoder(res.Body).Decode(&response)
	if res.StatusCode != 200 || response["object"] != "response" || response["status"] != "completed" {
		t.Fatalf("Responses protocol unavailable: HTTP %d %v", res.StatusCode, response)
	}
	output := response["output"].([]any)[0].(map[string]any)
	part := output["content"].([]any)[0].(map[string]any)
	if output["type"] != "message" || output["role"] != "assistant" || part["type"] != "output_text" || part["text"] != "Hello" {
		t.Fatalf("invalid output %v", output)
	}
	usage := response["usage"].(map[string]any)
	if usage["input_tokens"] != float64(12) || usage["output_tokens"] != float64(3) || usage["total_tokens"] != float64(15) {
		t.Fatalf("backend usage lost: %v", usage)
	}
}

func TestResponsesRejectsUnsupportedInputsBeforeInference(t *testing.T) {
	s := claudeServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("invalid request reached backend") })
	for _, extra := range []string{
		`"input":[{"role":"user","content":[{"type":"input_image","image_url":"x"}]}]`,
		`"input":"x","tools":[{"type":"web_search"}]`,
		`"input":"x","tools":[{"type":"custom","name":"apply_patch"}]`,
		`"input":"x","previous_response_id":"resp_old"`,
		`"input":"x","store":true`,
		`"input":"x","max_output_tokens":32769`,
		`"input":[{"type":"function_call_output","call_id":"missing","output":"x"}]`,
	} {
		res, _ := s.post("/v1/responses", `{"model":"local",`+extra+`}`)
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != 400 || strings.Contains(string(body), "event:") || !strings.Contains(string(body), `"type":"invalid_request_error"`) {
			t.Errorf("request accepted/misreported: %s: %d %s", extra, res.StatusCode, body)
		}
	}
}

func TestResponsesValidatedToolStreamAndHistory(t *testing.T) {
	calls := 0
	s := claudeServer(t, func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)
		calls++
		content := `{"text":"Reading","tool_calls":[{"name":"read_file","input":{"path":"a.txt"}}]}`
		if calls == 2 {
			transcript := string(mustJSON(payload["messages"]))
			if !strings.Contains(transcript, "file evidence") || !strings.Contains(transcript, "a.txt") {
				t.Errorf("history missing: %s", transcript)
			}
			content = `{"text":"Done","tool_calls":[]}`
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": content}, "finish_reason": "stop"}}, "usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 8}})
	})
	tool := `"tools":[{"type":"function","name":"read_file","description":"Read a file","parameters":{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}}]`
	res, _ := s.post("/v1/responses", `{"model":"local","input":"Read a.txt","stream":true,`+tool+`}`)
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	events := responseEvents(t, string(body))
	expected := []string{"response.created", "response.in_progress", "response.output_item.added", "response.content_part.added", "response.output_text.delta", "response.output_text.done", "response.content_part.done", "response.output_item.done", "response.output_item.added", "response.function_call_arguments.delta", "response.function_call_arguments.done", "response.output_item.done", "response.completed"}
	if len(events) != len(expected) {
		t.Fatalf("bad event stream HTTP %d: %s", res.StatusCode, body)
	}
	for i, event := range events {
		if event["type"] != expected[i] || event["sequence_number"] != float64(i) {
			t.Fatalf("wrong event %d: %v", i, event)
		}
	}
	final := events[len(events)-1]["response"].(map[string]any)
	output := final["output"].([]any)
	call := output[1].(map[string]any)
	if call["type"] != "function_call" || call["name"] != "read_file" || call["arguments"] != `{"path":"a.txt"}` || call["call_id"] == "" {
		t.Fatalf("bad function call: %v", call)
	}
	input := []any{map[string]any{"role": "user", "content": "Read a.txt"}, output[0], call, map[string]any{"type": "function_call_output", "call_id": call["call_id"], "output": "file evidence"}}
	res, _ = s.post("/v1/responses", `{"model":"local","input":`+string(mustJSON(input))+`,`+tool+`}`)
	defer res.Body.Close()
	body, _ = io.ReadAll(res.Body)
	if res.StatusCode != 200 || !strings.Contains(string(body), `"text":"Done"`) || calls != 2 {
		t.Fatalf("tool continuation: %d %s", res.StatusCode, body)
	}
}

func responseEvents(t *testing.T, body string) []map[string]any {
	t.Helper()
	var events []map[string]any
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
			t.Fatal(err)
		}
		events = append(events, event)
	}
	return events
}

func TestResponsesTruncationFailsBeforeStreamStarts(t *testing.T) {
	s := claudeServer(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"content":"partial"},"finish_reason":"length"}]}`)
	})
	res, _ := s.post("/v1/responses", `{"model":"local","input":"Hi","stream":true}`)
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != 422 || strings.Contains(string(body), "event:") {
		t.Fatalf("unvalidated stream: %d %s", res.StatusCode, body)
	}
}

func TestResponsesRequestLimitAndMissingUsage(t *testing.T) {
	s := claudeServer(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"content":"Hello"},"finish_reason":"stop"}]}`)
	})
	s.gateway.cfg.MaxRequestBytes = 128
	res, _ := s.post("/v1/responses", `{"model":"local","input":"`+strings.Repeat("x", 256)+`"}`)
	res.Body.Close()
	if res.StatusCode != 413 {
		t.Fatalf("request limit ignored: %d", res.StatusCode)
	}
	res, _ = s.post("/v1/responses", `{"model":"local","input":"Hi"}`)
	defer res.Body.Close()
	var response map[string]any
	json.NewDecoder(res.Body).Decode(&response)
	if res.StatusCode != 200 || response["usage"] != nil {
		t.Fatalf("invented usage: %d %v", res.StatusCode, response)
	}
}

func TestResponsesCancellationNeverClaimsCompletion(t *testing.T) {
	s := claudeServer(t, func(w http.ResponseWriter, r *http.Request) { t.Fatal("unused") })
	started, canceled := make(chan struct{}), make(chan struct{})
	s.gateway.claudeTransport = cancelTransport{canceled: canceled, started: started}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"local","input":"Hi","stream":true}`)).WithContext(ctx)
	recorder := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { s.gateway.ServeHTTP(recorder, request); close(done) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("backend not called")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancellation not propagated")
	}
	if recorder.Code != 502 || strings.Contains(recorder.Body.String(), "response.completed") || strings.Contains(recorder.Body.String(), "event:") {
		t.Fatalf("canceled completion emitted: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestResponsesCustomPatchProtocolAndHistory(t *testing.T) {
	patch := "*** Begin Patch\n*** Add File: a.txt\n+hello\n*** End Patch\n"
	tool := map[string]any{"type": "custom", "name": "apply_patch", "description": "Apply a patch", "format": map[string]any{"type": "grammar", "syntax": "lark", "definition": testPatchGrammar}}
	calls := 0
	s := claudeServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)
		content := string(mustJSON(map[string]any{"text": "", "tool_calls": []any{map[string]any{"name": "apply_patch", "input": map[string]string{"input": patch}}}}))
		if calls == 2 {
			if !strings.Contains(string(mustJSON(payload)), "patch evidence") {
				t.Error("custom output history missing")
			}
			content = `{"text":"Done","tool_calls":[]}`
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": content}, "finish_reason": "stop"}}})
	})
	req := map[string]any{"model": "local", "input": "Add a.txt", "tools": []any{tool}, "stream": true}
	res, _ := s.post("/v1/responses", string(mustJSON(req)))
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	events := responseEvents(t, string(body))
	if len(events) == 0 {
		t.Fatalf("custom tool unsupported: %d %s", res.StatusCode, body)
	}
	completed := events[len(events)-1]["response"].(map[string]any)
	call := completed["output"].([]any)[0].(map[string]any)
	if call["type"] != "custom_tool_call" || call["input"] != patch || call["name"] != "apply_patch" {
		t.Fatalf("custom protocol lost: %v", call)
	}
	if !strings.Contains(string(body), "response.custom_tool_call_input.delta") || !strings.Contains(string(body), "response.custom_tool_call_input.done") {
		t.Fatalf("custom SSE missing: %s", body)
	}
	req["stream"] = false
	req["input"] = []any{map[string]any{"role": "user", "content": "Add a.txt"}, call, map[string]any{"type": "custom_tool_call_output", "call_id": call["call_id"], "output": "patch evidence"}}
	res, _ = s.post("/v1/responses", string(mustJSON(req)))
	defer res.Body.Close()
	body, _ = io.ReadAll(res.Body)
	if res.StatusCode != 200 || !strings.Contains(string(body), `"text":"Done"`) {
		t.Fatalf("custom continuation rejected: %d %s", res.StatusCode, body)
	}
}

func TestResponsesMalformedPatchNeverOpensStream(t *testing.T) {
	for _, patch := range []string{"*** Begin Patch\n*** Add File: a\n+partial", "not a patch", "*** Begin Patch\n*** Add File: a\nhello\n*** End Patch"} {
		s := claudeServer(t, func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": string(mustJSON(map[string]any{"text": "", "tool_calls": []any{map[string]any{"name": "apply_patch", "input": map[string]string{"input": patch}}}}))}, "finish_reason": "stop"}}})
		})
		req := map[string]any{"model": "local", "input": "Patch a", "stream": true, "tools": []any{map[string]any{"type": "custom", "name": "apply_patch", "format": map[string]any{"type": "grammar", "syntax": "lark", "definition": testPatchGrammar}}}}
		res, _ := s.post("/v1/responses", string(mustJSON(req)))
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != 422 || strings.Contains(string(body), "event:") {
			t.Fatalf("malformed patch emitted: %d %s", res.StatusCode, body)
		}
	}
}

func TestResponsesCallOutputTypeMustMatchHistory(t *testing.T) {
	s := claudeServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("mismatched call types reached backend") })
	res, _ := s.post("/v1/responses", `{"model":"local","input":[{"type":"function_call","name":"read_file","call_id":"call_1","arguments":"{}"},{"type":"custom_tool_call_output","call_id":"call_1","output":"evidence"}]}`)
	defer res.Body.Close()
	if res.StatusCode != 400 {
		t.Fatalf("custom output accepted for function call: %d", res.StatusCode)
	}
}

func TestResponsesConfiguredRoleDefaultBudget(t *testing.T) {
	for _, effort := range []string{"", "high"} {
		s := claudeServer(t, func(w http.ResponseWriter, r *http.Request) {
			var payload map[string]any
			json.NewDecoder(r.Body).Decode(&payload)
			want := 4096
			if effort == "high" {
				want = 8192
			}
			if payload["max_tokens"] != float64(want) {
				t.Errorf("role budget clipped: got %v want %d", payload["max_tokens"], want)
			}
			content := "Hello"
			if effort == "high" {
				content = "Reasoning</think>Hello"
			}
			json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": content}, "finish_reason": "stop"}}})
		})
		s.gateway.roles["sonnet"] = s.gateway.upstream
		s.gateway.roles["opus"] = s.gateway.upstream
		s.gateway.cfg.Router = RoleRouter{}
		res, _ := s.post("/v1/responses", `{"model":"local","input":"Hi","reasoning":{"effort":"`+effort+`"}}`)
		res.Body.Close()
		if res.StatusCode != 200 {
			t.Fatalf("role request failed: %d", res.StatusCode)
		}
	}
}

func TestResponsesNamedChoiceMustMatchToolType(t *testing.T) {
	s := claudeServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("mismatched choice reached inference") })
	res, _ := s.post("/v1/responses", `{"model":"local","input":"Read","tools":[{"type":"function","name":"read_file","parameters":{"type":"object"}}],"tool_choice":{"type":"custom","name":"read_file"}}`)
	defer res.Body.Close()
	if res.StatusCode != 400 {
		t.Fatalf("custom choice accepted for function: %d", res.StatusCode)
	}
}

func TestResponsesRejectedAdapterResultNeverCapturesAcceptance(t *testing.T) {
	for _, custom := range []bool{true, false} {
		output := `{"text":"","tool_calls":[{"name":"read_file","input":{}},{"name":"read_file","input":{}}]}`
		if custom {
			output = `{"text":"","tool_calls":[{"name":"apply_patch","input":{"input":"not a patch"}}]}`
		}
		s := claudeServer(t, func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": output}, "finish_reason": "stop"}}})
		})
		recorder, err := newTrainingRecorder(&TrainingConfig{Directory: filepath.Join(t.TempDir(), "private")})
		if err != nil {
			t.Fatal(err)
		}
		s.gateway.training = recorder
		tool := map[string]any{"type": "function", "name": "read_file", "parameters": map[string]any{"type": "object"}}
		if custom {
			tool = map[string]any{"type": "custom", "name": "apply_patch", "format": map[string]any{"type": "grammar", "syntax": "lark", "definition": testPatchGrammar}}
		}
		req := map[string]any{"model": "local", "input": "Use the tool", "parallel_tool_calls": false, "tools": []any{tool}, "stream": true}
		res, _ := s.post("/v1/responses", string(mustJSON(req)))
		res.Body.Close()
		if res.StatusCode != 422 {
			t.Fatalf("invalid output accepted: %d", res.StatusCode)
		}
		data, err := os.ReadFile(filepath.Join(recorder.directory, "training.jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		var event map[string]any
		if json.Unmarshal(data, &event) != nil {
			t.Fatalf("expected one failed attempt: %s", data)
		}
		if event["accepted"] != false || event["quality"].(map[string]any)["protocol_valid"] != false {
			t.Fatalf("rejected adapter output capturedaccepted: %s", data)
		}
	}
}

func TestResponsesPartialUsageStaysUnknown(t *testing.T) {
	for _, usage := range []string{`{"completion_tokens":3}`, `{"prompt_tokens":12}`} {
		s := claudeServer(t, func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, `{"choices":[{"message":{"content":"Hello"},"finish_reason":"stop"}],"usage":%s}`, usage)
		})
		res, _ := s.post("/v1/responses", `{"model":"local","input":"Hi"}`)
		defer res.Body.Close()
		var response map[string]any
		json.NewDecoder(res.Body).Decode(&response)
		if res.StatusCode != 200 || response["usage"] != nil {
			t.Fatalf("partial usage became invented totals: %d %v", res.StatusCode, response)
		}
	}
}

func TestResponsesNamespaceFunctionRoundTrip(t *testing.T) {
	calls := 0
	s := claudeServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)
		transcript := string(mustJSON(payload["messages"]))
		if !strings.Contains(transcript, "functions.read_file") {
			t.Errorf("qualified function omitted: %s", transcript)
		}
		content := `{"text":"","tool_calls":[{"name":"functions.read_file","input":{"path":"a.txt"}}]}`
		if calls == 2 {
			if !strings.Contains(transcript, "file evidence") {
				t.Error("namespaced tool history missing")
			}
			content = `{"text":"Done","tool_calls":[]}`
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": content}, "finish_reason": "stop"}}})
	})
	tool := map[string]any{"type": "namespace", "name": "functions", "description": "Local functions", "tools": []any{map[string]any{"type": "function", "name": "read_file", "parameters": map[string]any{"type": "object", "properties": map[string]any{"path": map[string]string{"type": "string"}}, "required": []string{"path"}}}}}
	req := map[string]any{"model": "local", "input": "Read a.txt", "stream": true, "tools": []any{tool}, "tool_choice": map[string]string{"type": "function", "name": "read_file", "namespace": "functions"}}
	res, _ := s.post("/v1/responses", string(mustJSON(req)))
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	events := responseEvents(t, string(body))
	if res.StatusCode != 200 || len(events) == 0 {
		t.Fatalf("namespace unavailable: %d %s", res.StatusCode, body)
	}
	completed := events[len(events)-1]["response"].(map[string]any)
	call := completed["output"].([]any)[0].(map[string]any)
	if call["name"] != "read_file" || call["namespace"] != "functions" || call["type"] != "function_call" {
		t.Fatalf("namespace wire identity lost: %v", call)
	}
	if completed["tools"].([]any)[0].(map[string]any)["type"] != "namespace" {
		t.Fatalf("namespace definitions lost: %v", completed["tools"])
	}
	req["stream"] = false
	req["tool_choice"] = "auto"
	req["input"] = []any{map[string]any{"role": "user", "content": "Read a.txt"}, call, map[string]any{"type": "function_call_output", "call_id": call["call_id"], "namespace": "functions", "name": "read_file", "output": "file evidence"}}
	res, _ = s.post("/v1/responses", string(mustJSON(req)))
	defer res.Body.Close()
	body, _ = io.ReadAll(res.Body)
	if res.StatusCode != 200 || !strings.Contains(string(body), `"text":"Done"`) {
		t.Fatalf("namespaced continuation failed: %d %s", res.StatusCode, body)
	}
}

func TestResponsesNamespaceCustomPatchIdentity(t *testing.T) {
	patch := "*** Begin Patch\n*** Add File: a.txt\n+hello\n*** End Patch\n"
	s := claudeServer(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": string(mustJSON(map[string]any{"text": "", "tool_calls": []any{map[string]any{"name": "functions.apply_patch", "input": map[string]string{"input": patch}}}}))}, "finish_reason": "stop"}}})
	})
	req := map[string]any{"model": "local", "input": "Add a.txt", "tools": []any{map[string]any{"type": "namespace", "name": "functions", "tools": []any{map[string]any{"type": "custom", "name": "apply_patch", "format": map[string]string{"type": "grammar", "syntax": "lark", "definition": testPatchGrammar}}}}}}
	res, _ := s.post("/v1/responses", string(mustJSON(req)))
	defer res.Body.Close()
	var response map[string]any
	json.NewDecoder(res.Body).Decode(&response)
	if res.StatusCode != 200 {
		t.Fatalf("namespaced patch rejected: %d %v", res.StatusCode, response)
	}
	call := response["output"].([]any)[0].(map[string]any)
	if call["type"] != "custom_tool_call" || call["name"] != "apply_patch" || call["namespace"] != "functions" || call["input"] != patch {
		t.Fatalf("custom namespace identity lost: %v", call)
	}
}

func TestResponsesNamespaceRejectsUnsupportedContentsAndIdentity(t *testing.T) {
	s := claudeServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("unsupported namespace reached inference") })
	for _, body := range []string{
		`{"model":"local","input":"x","tools":[{"type":"namespace","name":"functions","tools":[{"type":"web_search"}]}]}`,
		`{"model":"local","input":"x","tools":[{"type":"namespace","name":"functions","tools":[{"type":"namespace","name":"nested","tools":[]}]}]}`,
		`{"model":"local","input":"x","tools":[{"type":"namespace","name":"functions","tools":[{"type":"function","name":"read","defer_loading":true,"parameters":{"type":"object"}}]}]}`,
		`{"model":"local","input":[{"type":"function_call","namespace":"functions","name":"read","call_id":"c","arguments":"{}"},{"type":"function_call_output","namespace":"other","call_id":"c","output":"x"}]}`,
	} {
		res, _ := s.post("/v1/responses", body)
		res.Body.Close()
		if res.StatusCode != 400 {
			t.Errorf("unsupportednamespace accepted: %d %s", res.StatusCode, body)
		}
	}
}

const testPatchGrammar = `start: begin_patch hunk+ end_patch
begin_patch: "*** Begin Patch" LF
end_patch: "*** End Patch" LF?

hunk: add_hunk | delete_hunk | update_hunk
add_hunk: "*** Add File: " filename LF add_line+
delete_hunk: "*** Delete File: " filename LF
update_hunk: "*** Update File: " filename LF change_move? change?

filename: /(.+)/
add_line: "+" /(.*)/ LF -> line
change_move: "*** Move to: " filename LF
change: (change_context | change_line)+ eof_line?
change_context: ("@@" | "@@ " /(.+)/) LF
change_line: ("+" | "-" | " ") /(.*)/ LF
eof_line: "*** End of File" LF

%import common.LF
`
