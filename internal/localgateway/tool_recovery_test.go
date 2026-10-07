package localgateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestRecoveryUsageRequiresBothAttemptsToReportUsage(t *testing.T) {
	for _, missing := range []string{"none", "prompt_tokens", "completion_tokens"} {
		t.Run(missing, func(t *testing.T) {
			attempts := 0
			s := claudeServer(t, func(w http.ResponseWriter, r *http.Request) {
				attempts++
				raw := `{"tool_calls":`
				usage := map[string]int{"prompt_tokens": 20, "completion_tokens": 12}
				if attempts == 1 {
					delete(usage, missing)
				} else {
					raw = `{"tool_calls":[{"name":"record_marker","input":{"marker":"READY"}}]}`
				}
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": raw}, "finish_reason": "stop"}}, "usage": usage})
			})
			var req claudeRequest
			if err := json.Unmarshal([]byte(`{"model":"local","max_tokens":128,"messages":[{"role":"user","content":"Record READY."}],"tools":[{"name":"record_marker","input_schema":{"type":"object"}}],"tool_choice":{"type":"tool","name":"record_marker"}}`), &req); err != nil {
				t.Fatal(err)
			}
			messages, err := prepareClaude(req)
			if err != nil {
				t.Fatal(err)
			}
			res, err := s.gateway.inferClaude(context.Background(), req, messages)
			if err != nil || attempts != 2 || res.UsageKnown != (missing == "none") {
				t.Fatalf("incorrect retry availability: known=%t attempts=%d err=%v", res.UsageKnown, attempts, err)
			}
			input, output := 40, 24
			if missing == "prompt_tokens" {
				input = 20
			}
			if missing == "completion_tokens" {
				output = 12
			}
			if res.Usage["input_tokens"] != input || res.Usage["output_tokens"] != output {
				t.Fatalf("incorrect retry accounting: %v", res.Usage)
			}
		})
	}
}

func TestToolValidationRecoveryAcrossProtocols(t *testing.T) {
	valid := `{"text":"","tool_calls":[{"name":"record_marker","input":{"marker":"READY"}}]}`
	cases := map[string]string{
		"wrong-choice":     `{"tool_calls":[{"name":"other_marker","input":{"marker":"READY"}}]}`,
		"empty-output":     "",
		"missing-argument": `{"tool_calls":[{"name":"record_marker","input":{}}]}`,
		"extra-argument":   `{"tool_calls":[{"name":"record_marker","input":{"marker":"READY","extra":true}}]}`,
		"duplicate-key":    `{"tool_calls":[{"name":"record_marker","input":{"marker":"READY","marker":"WRONG"}}]}`,
		"wrong-schema":     `{"tool_calls":[{"name":"record_marker","input":{"marker":17}}]}`,
		"wrong-enum":       `{"tool_calls":[{"name":"record_marker","input":{"marker":"WRONG"}}]}`,
		"unknown-tool":     `{"tool_calls":[{"name":"invented","input":{"marker":"READY"}}]}`,
		"required-absent":  `{"text":"I will call the tool.","tool_calls":[]}`,
		"invalid-json":     `{"tool_calls":`,
	}
	for _, protocol := range []string{"messages", "responses", "chat/completions"} {
		for _, stream := range []bool{false, true} {
			for name, bad := range cases {
				for _, recovers := range []bool{false, true} {
					t.Run(protocol+"/"+name+"/"+map[bool]string{true: "stream", false: "json"}[stream]+"/"+map[bool]string{true: "recovers", false: "exhausted"}[recovers], func(t *testing.T) {
						attempts := 0
						s := claudeServer(t, func(w http.ResponseWriter, r *http.Request) {
							attempts++
							raw := bad
							if attempts == 2 && recovers {
								raw = valid
							}
							json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": raw}, "finish_reason": "stop"}}, "usage": map[string]int{"prompt_tokens": 20, "completion_tokens": 12}})
						})
						s.gateway.cfg.ClaudeBufferedValidation = true
						schema := map[string]any{"type": "object", "properties": map[string]any{"marker": map[string]any{"type": "string", "enum": []string{"READY"}}}, "required": []string{"marker"}, "additionalProperties": false}
						request := map[string]any{"model": "local", "stream": stream}
						switch protocol {
						case "messages":
							request["max_tokens"] = 128
							request["messages"] = []any{map[string]any{"role": "user", "content": "Record READY."}}
							request["tools"] = []any{map[string]any{"name": "record_marker", "input_schema": schema}, map[string]any{"name": "other_marker", "input_schema": schema}}
							request["tool_choice"] = map[string]any{"type": "tool", "name": "record_marker"}
						case "responses":
							request["max_output_tokens"] = 128
							request["input"] = "Record READY."
							request["tools"] = []any{map[string]any{"type": "function", "name": "record_marker", "parameters": schema}, map[string]any{"type": "function", "name": "other_marker", "parameters": schema}}
							request["tool_choice"] = map[string]any{"type": "function", "name": "record_marker"}
						case "chat/completions":
							request["max_tokens"] = 128
							request["messages"] = []any{map[string]any{"role": "user", "content": "Record READY."}}
							request["tools"] = []any{map[string]any{"type": "function", "function": map[string]any{"name": "record_marker", "parameters": schema}}, map[string]any{"type": "function", "function": map[string]any{"name": "other_marker", "parameters": schema}}}
							request["tool_choice"] = map[string]any{"type": "function", "function": map[string]any{"name": "record_marker"}}
						}
						response, err := s.post("/v1/"+protocol, string(mustJSON(request)))
						if err != nil {
							t.Fatal(err)
						}
						defer response.Body.Close()
						body, _ := io.ReadAll(response.Body)
						if attempts != 2 {
							t.Fatalf("expected exactly two bounded attempts; got %d: %s", attempts, body)
						}
						if recovers {
							if response.StatusCode != 200 || !strings.Contains(string(body), "READY") {
								t.Fatalf("recovery failed: %d %s", response.StatusCode, body)
							}
							if !stream {
								var d map[string]any
								if err := json.Unmarshal(body, &d); err != nil {
									t.Fatal(err)
								}
								u := d["usage"].(map[string]any)
								input, output := "input_tokens", "output_tokens"
								if protocol == "chat/completions" {
									input, output = "prompt_tokens", "completion_tokens"
								}
								if u[input] != float64(40) || u[output] != float64(24) {
									t.Fatalf("retry usage omitted: %v", u)
								}
							}
						} else {
							if response.StatusCode != 422 || strings.Contains(response.Header.Get("Content-Type"), "event-stream") || strings.Contains(string(body), "toolu_") {
								t.Fatalf("invalid tool exposed: %d %s", response.StatusCode, body)
							}
						}
					})
				}
			}
		}
	}
}

func TestResponsesContractFailuresReceiveOneCorrection(t *testing.T) {
	for _, kind := range []string{"parallel", "patch"} {
		for _, stream := range []bool{false, true} {
			t.Run(kind+"/"+map[bool]string{true: "stream", false: "json"}[stream], func(t *testing.T) {
				attempts := 0
				patch := "*** Begin Patch\n*** Add File: fixture.txt\n+READY\n*** End Patch"
				valid := `{"tool_calls":[{"name":"record_marker","input":{"marker":"READY"}}]}`
				bad := `{"tool_calls":[{"name":"record_marker","input":{"marker":"READY"}},{"name":"record_marker","input":{"marker":"READY"}}]}`
				tool := map[string]any{"type": "function", "name": "record_marker", "parameters": map[string]any{"type": "object", "properties": map[string]any{"marker": map[string]string{"type": "string"}}, "required": []string{"marker"}}}
				if kind == "patch" {
					valid = string(mustJSON(map[string]any{"tool_calls": []any{map[string]any{"name": "apply_patch", "input": map[string]string{"input": patch}}}}))
					bad = `{"tool_calls":[{"name":"apply_patch","input":{"input":"not a patch"}}]}`
					tool = map[string]any{"type": "custom", "name": "apply_patch", "format": map[string]string{"type": "grammar", "syntax": "lark", "definition": testPatchGrammar}}
				}
				s := claudeServer(t, func(w http.ResponseWriter, r *http.Request) {
					attempts++
					raw := bad
					if attempts == 2 {
						raw = valid
					}
					json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": raw}, "finish_reason": "stop"}}, "usage": map[string]int{"prompt_tokens": 20, "completion_tokens": 12}})
				})
				s.gateway.cfg.ClaudeBufferedValidation = true
				r, err := s.post("/v1/responses", string(mustJSON(map[string]any{"model": "local", "input": "Record READY using the tool.", "stream": stream, "parallel_tool_calls": false, "tools": []any{tool}})))
				if err != nil {
					t.Fatal(err)
				}
				defer r.Body.Close()
				b, _ := io.ReadAll(r.Body)
				if r.StatusCode != 200 || attempts != 2 || !strings.Contains(string(b), "READY") {
					t.Fatalf("contract recovery failed: %d attempts=%d %s", r.StatusCode, attempts, b)
				}
			})
		}
	}
}
