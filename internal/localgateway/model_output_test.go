package localgateway

import (
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestModelJSONFailurePreservesParserDiagnosis(t *testing.T) {
	_, err := normalizeModelOutput(claudeRequest{}, `{"text":"","tool_calls":[{"name":"Read","input":{"file_path":"a.txt"}}}]}`)
	if err == nil || !strings.Contains(err.Error(), "after array element") {
		t.Fatalf("correction lost syntax diagnosis: %v", err)
	}
}

func TestActionPreambleCanPrecedeCompleteToolEnvelope(t *testing.T) {
	envelope := `{"text":"","tool_calls":[{"name":"Read","input":{"file_path":"a.txt"}}]}`
	for _, raw := range []string{"I'll read the file.\n" + envelope, "I'll start by reading the file.\n\n```json\n" + envelope + "\n```"} {
		out, err := normalizeModelOutput(claudeRequest{JSONTools: true}, raw)
		if err != nil || len(out.Calls) != 1 || out.Calls[0].Input["file_path"] != "a.txt" {
			t.Fatalf("complete prefaced call lost: %+v %v", out, err)
		}
	}
	for _, raw := range []string{"Here is an example:\n" + envelope, "I'll show an example:\n" + envelope, "I'll read the file.\n" + envelope + " extra text"} {
		out, err := normalizeModelOutput(claudeRequest{JSONTools: true}, raw)
		if err == nil && len(out.Calls) > 0 {
			t.Fatal("example or trailing prose executed")
		}
	}
}

func TestModelOutputTranslationDoesNotDependOnClientModelAlias(t *testing.T) {
	for _, model := range []string{"sentinel-sonnet", "local", "another-model"} {
		req := claudeRequest{Model: model, JSONTools: true, Tools: []claudeTool{{Name: "Read", Schema: map[string]any{"type": "object"}}}}
		for name, raw := range map[string]string{
			"canonical":           `{"text":"","tool_calls":[{"name":"Read","input":{"file_path":"fixture.md"}}]}`,
			"openai_null_content": `{"content":null,"tool_calls":[{"type":"function","function":{"name":"Read","arguments":"{\"file_path\":\"fixture.md\"}"}}]}`,
			"openai_text_content": `{"content":"Reading fixture.","tool_calls":[{"type":"function","function":{"name":"Read","arguments":"{\"file_path\":\"fixture.md\"}"}}]}`,
			"openai":              `{"tool_calls":[{"id":"call1","type":"function","function":{"name":"Read","arguments":"{\"file_path\":\"fixture.md\"}"}}]}`,
			"arguments":           `{"tool_calls":[{"name":"Read","arguments":{"file_path":"fixture.md"}}]}`,
			"anthropic":           `{"content":[{"type":"tool_use","id":"tool1","name":"Read","input":{"file_path":"fixture.md"}}]}`,
			"tagged_json":         `<tool_call>{"name":"Read","arguments":{"file_path":"fixture.md"}}</tool_call>`,
			"tagged_parameters":   `<tool_call><function=Read><parameter=file_path>fixture.md</parameter></function></tool_call>`,
		} {
			t.Run(model+"/"+name, func(t *testing.T) {
				out, err := decodeToolOutput(req, raw)
				if err != nil || len(out.Calls) != 1 || out.Calls[0].Name != "Read" || !reflect.DeepEqual(out.Calls[0].Input, map[string]any{"file_path": "fixture.md"}) {
					t.Fatalf("translation failed: %#v %v", out, err)
				}
			})
		}
		for _, raw := range []string{"Here is the requested completed review.", `{"result":"final answer"}`} {
			out, err := decodeToolOutput(req, raw)
			if err != nil || out.Text != raw || len(out.Calls) != 0 {
				t.Fatalf("valid final text rejected: %#v %v", out, err)
			}
		}
	}
}

func TestModelToolTranslationAcrossClientProtocols(t *testing.T) {
	for _, protocol := range []string{"messages", "responses", "chat/completions"} {
		for _, stream := range []bool{false, true} {
			for _, format := range []string{"json", "gemma"} {
				for _, valid := range []bool{false, true} {
					t.Run(protocol+"/"+format+"/"+map[bool]string{true: "stream", false: "json"}[stream]+"/"+map[bool]string{true: "valid", false: "invalid"}[valid], func(t *testing.T) {
						args := `{"file_path":"fixture.md"}`
						if !valid {
							args = `{"file_path":42}`
						}
						output := string(mustJSON(map[string]any{"tool_calls": []any{map[string]any{"type": "function", "function": map[string]any{"name": "Read", "arguments": args}}}}))
						if format == "gemma" {
							value := "fixture.md"
							if !valid {
								value = "42"
							}
							output = "<|tool_call>call:Read{file_path:" + value + "}<tool_call|>"
						}
						s := claudeServer(t, func(w http.ResponseWriter, r *http.Request) {
							json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": output}, "finish_reason": "stop"}}, "usage": map[string]int{"prompt_tokens": 20, "completion_tokens": 12}})
						})
						s.gateway.cfg.ClaudeBufferedValidation = true
						schema := map[string]any{"type": "object", "properties": map[string]any{"file_path": map[string]any{"type": "string"}}, "required": []string{"file_path"}, "additionalProperties": false}
						request := map[string]any{"model": "local", "stream": stream}
						switch protocol {
						case "messages":
							request["max_tokens"] = 128
							request["messages"] = []any{map[string]any{"role": "user", "content": "Read fixture.md"}}
							request["tools"] = []any{map[string]any{"name": "Read", "input_schema": schema}}
						case "responses":
							request["max_output_tokens"] = 128
							request["input"] = "Read fixture.md"
							request["tools"] = []any{map[string]any{"type": "function", "name": "Read", "parameters": schema}}
						case "chat/completions":
							request["max_tokens"] = 128
							request["messages"] = []any{map[string]any{"role": "user", "content": "Read fixture.md"}}
							request["tools"] = []any{map[string]any{"type": "function", "function": map[string]any{"name": "Read", "parameters": schema}}}
						}
						response, err := s.post("/v1/"+protocol, string(mustJSON(request)))
						if err != nil {
							t.Fatal(err)
						}
						defer response.Body.Close()
						body, _ := io.ReadAll(response.Body)
						if valid {
							if response.StatusCode != 200 || !strings.Contains(string(body), "Read") || !strings.Contains(string(body), "fixture.md") {
								t.Fatalf("translated tool absent: %d %s", response.StatusCode, body)
							}
						} else if response.StatusCode != 422 || strings.Contains(response.Header.Get("Content-Type"), "event-stream") {
							t.Fatalf("invalid tool reached client stream: %d %s", response.StatusCode, body)
						}
					})
				}
			}
		}
	}
}

func TestModelOutputTranslationRejectsAmbiguousOrIncompleteCalls(t *testing.T) {
	req := claudeRequest{Model: "another-model", JSONTools: true}
	for _, raw := range []string{
		`{"tool_calls":null}`,
		`{"name":"Read","arguments":{"file_path":"a"},"content":[{"type":"tool_use","name":"Read","input":{"file_path":"b"}}]}`,
		`{"text":"one","content":"two","tool_calls":[]}`,
		`{"tool_calls":[],"content":[{"type":"text","text":"conflict"}]}`,
		`{"tool_calls":[{"function":{"name":"Read","arguments":{}},"input":null}]}`,
		`{"tool_calls":[{"name":"Read","input":{},"arguments":{"other":true}}]}`,
		`{"tool_calls":[{"function":{"name":"Read","arguments":"not-json"}}]}`,
		`{"content":[{"type":"tool_use","name":"Read"}]}`,
		`<tool_call>{"name":"Read","arguments":{}}`,
		`<tool_call>{"name":"Read","arguments":{}}</tool_call> trailing prose`,
		`{"tool_calls":[]}{"tool_calls":[]}`,
		`{"tool_calls":[],"tool_calls":[]}`,
		`{"tool_calls":[{"name":"Read","input":{"file_path":"a","file_path":"b"}}]}`,
	} {
		if _, err := decodeToolOutput(req, raw); err == nil {
			t.Fatalf("malformed call accepted: %s", raw)
		}
	}
}

func TestModelToolArgumentsKeepLiteralProtocolMarkers(t *testing.T) {
	raw := `{"tool_calls":[{"name":"Write","input":{"content":"<tool_call><function=literal></tool_call>"}}]}`
	out, err := decodeToolOutput(claudeRequest{JSONTools: true}, raw)
	if err != nil || len(out.Calls) != 1 || out.Calls[0].Input["content"] != "<tool_call><function=literal></tool_call>" {
		t.Fatalf("literal argument content interpreted as a call: %#v %v", out, err)
	}
}

func TestTaggedJSONArgumentsKeepLiteralProtocolMarkers(t *testing.T) {
	for _, content := range []string{"</tool_call>", "<function=literal>", "<parameter=literal>", "<tool_call></tool_call>", "<|tool_call>", "<tool_call|>", "<|tool_response>"} {
		raw := "<tool_call>" + strings.NewReplacer(`\u003c`, "<", `\u003e`, ">").Replace(string(mustJSON(map[string]any{"name": "Write", "arguments": map[string]any{"content": content}}))) + "</tool_call>"
		out, err := decodeToolOutput(claudeRequest{JSONTools: true}, raw)
		if err != nil || len(out.Calls) != 1 || out.Calls[0].Input["content"] != content {
			t.Fatalf("literal content changed: %#v %v", out, err)
		}
	}
}
