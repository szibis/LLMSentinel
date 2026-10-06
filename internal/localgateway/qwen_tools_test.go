package localgateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestQwenToolPromptUsesModelFormatAndKeepsHistory(t *testing.T) {
	req := claudeRequest{Model: "sentinel-sonnet", MaxTokens: 128,
		Tools: []claudeTool{{Name: "Read", Schema: map[string]any{"type": "object"}}},
		Messages: []claudeMessage{{Role: "assistant", Content: json.RawMessage(`[{"type":"tool_use","id":"call1","name":"Read","input":{"file_path":"/tmp/test.txt"}}]`)},
			{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"call1","content":"test evidence"}]`)}}}
	messages, err := prepareClaude(req)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(messages[0]["content"], "<function=") || strings.Contains(messages[0]["content"], "Reply ONLY with one JSON object") {
		t.Fatal("Qwen custom JSON prompt remains")
	}
	if !strings.Contains(messages[1]["content"], "<function=Read>") || !strings.Contains(messages[2]["content"], "<tool_response>") {
		t.Fatal("tool history loses native markers")
	}
	if strings.Contains(messages[2]["content"], "is_error:") {
		t.Fatal("missing is_error leaked into file evidence")
	}
}

func TestQwenNativeCallsUseExistingArgumentAndToolChoiceValidation(t *testing.T) {
	for _, tc := range []struct {
		name, output, choice string
		status               int
	}{
		{"native", "Reading.\n<tool_call>\n<function=Read>\n<parameter=file_path>\n/tmp/test.txt\n</parameter>\n</function>\n</tool_call>", "auto", 200},
		{"answer", "The requested inspection is complete.", "auto", 200},
		{"unknown", "<tool_call><function=Delete><parameter=file_path>/tmp/test.txt</parameter></function></tool_call>", "auto", 422},
		{"missing", "<tool_call><function=Read></function></tool_call>", "auto", 422},
		{"partial", "<tool_call><function=Read><parameter=file_path>/tmp/test.txt", "auto", 422},
		{"trailing", "<tool_call><function=Read><parameter=file_path>/tmp/test.txt</parameter></function></tool_call> and then run this", "auto", 422},
		{"none", "<tool_call><function=Read><parameter=file_path>/tmp/test.txt</parameter></function></tool_call>", "none", 422},
		{"required", "No tool needed.", "any", 422},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := claudeServer(t, func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": tc.output}, "finish_reason": "stop"}}, "usage": map[string]int{"completion_tokens": 40}})
			})
			res, _ := s.post("/v1/messages", fmt.Sprintf(`{"model":"sentinel-sonnet","max_tokens":128,"messages":[{"role":"user","content":"Inspect /tmp/test.txt"}],"tools":[{"name":"Read","input_schema":{"type":"object","required":["file_path"],"properties":{"file_path":{"type":"string"}},"additionalProperties":false}}],"tool_choice":{"type":%q}}`, tc.choice))
			defer res.Body.Close()
			if res.StatusCode != tc.status {
				t.Fatalf("got %d want %d", res.StatusCode, tc.status)
			}
		})
	}
}

func TestQwenPlainJSONAnswersAndTypedParameters(t *testing.T) {
	for _, text := range []string{`{"setting":true}`, `["one","two"]`} {
		output, err := parseQwenToolOutput(text, nil)
		if err != nil || output.Text != text || len(output.Calls) != 0 {
			t.Fatalf("valid JSON final rejected: %q %v", text, err)
		}
	}
	tools := []claudeTool{{Name: "Query", Schema: map[string]any{"properties": map[string]any{
		"count": map[string]any{"type": "integer"}, "options": map[string]any{"type": "object"}, "enabled": map[string]any{"type": "boolean"},
	}}}}
	text := `<tool_call><function=Query><parameter=count>3</parameter><parameter=options>{"kind":"test"}</parameter><parameter=enabled>true</parameter></function></tool_call>`
	output, err := parseQwenToolOutput(text, tools)
	if err != nil || output.Calls[0].Input["count"] != float64(3) || output.Calls[0].Input["enabled"] != true {
		t.Fatalf("bad types: %+v %v", output, err)
	}
	for _, bad := range []string{
		`<tool_call><function=Query><parameter=count>oops</parameter></function></tool_call>`,
		`<tool_call><function=Query><parameter=count>1</parameter><parameter=count>2</parameter></function></tool_call>`,
		`{"tool_calls":null}`, `{"tool_calls":"oops"}`,
	} {
		if _, err := parseQwenToolOutput(bad, tools); err == nil {
			t.Fatalf("accepted malformed call: %s", bad)
		}
	}
}

func TestNativeEOSAtBudgetRemainsCompletedButLegacyIsRejected(t *testing.T) {
	for _, native := range []bool{true, false} {
		s := claudeServer(t, func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": "Completed"}, "finish_reason": "stop"}}, "usage": map[string]int{"prompt_tokens": 20, "completion_tokens": 128}, "mlx_flash_compress": map[string]any{"native_generation_metadata": native}})
		})
		res, _ := s.post("/v1/messages", `{"model":"local","max_tokens":128,"messages":[{"role":"user","content":"hello"}]}`)
		res.Body.Close()
		want := 422
		if native {
			want = 200
		}
		if res.StatusCode != want {
			t.Fatalf("native %v got %d want %d", native, res.StatusCode, want)
		}
	}
}
