package localgateway

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestForcedToolsOmitPreambleButAutoPreservesItAcrossClients(t *testing.T) {
	for _, protocol := range []string{"messages", "responses", "chat/completions"} {
		for _, stream := range []bool{false, true} {
			for _, choice := range []string{"auto", "required", "named"} {
				t.Run(fmt.Sprintf("%s/%t/%s", protocol, stream, choice), func(t *testing.T) {
					s := claudeServer(t, func(w http.ResponseWriter, r *http.Request) {
						envelope := `{"text":"SYNTHETIC_PREAMBLE","tool_calls":[{"name":"Read","input":{"file_path":"fixture.txt"}}]}`
						_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": envelope}, "finish_reason": "stop"}}, "usage": map[string]int{"prompt_tokens": 11, "completion_tokens": 23}})
					})
					s.gateway.cfg.ClaudeBufferedValidation = true
					schema := map[string]any{"type": "object", "properties": map[string]any{"file_path": map[string]string{"type": "string"}}, "required": []string{"file_path"}}
					payload := map[string]any{"model": "local", "stream": stream}
					switch protocol {
					case "messages":
						payload["max_tokens"] = 128
						payload["messages"] = []any{map[string]string{"role": "user", "content": "Read fixture.txt"}}
						payload["tools"] = []any{map[string]any{"name": "Read", "input_schema": schema}}
						payload["tool_choice"] = map[string]string{"type": map[string]string{"auto": "auto", "required": "any", "named": "tool"}[choice]}
						if choice == "named" {
							payload["tool_choice"].(map[string]string)["name"] = "Read"
						}
					case "responses":
						payload["input"] = "Read fixture.txt"
						payload["tools"] = []any{map[string]any{"type": "function", "name": "Read", "parameters": schema}}
						payload["tool_choice"] = choice
						if choice == "named" {
							payload["tool_choice"] = map[string]string{"type": "function", "name": "Read"}
						}
					case "chat/completions":
						if stream {
							payload["stream_options"] = map[string]bool{"include_usage": true}
						}
						payload["messages"] = []any{map[string]string{"role": "user", "content": "Read fixture.txt"}}
						payload["tools"] = []any{map[string]any{"type": "function", "function": map[string]any{"name": "Read", "parameters": schema}}}
						payload["tool_choice"] = choice
						if choice == "named" {
							payload["tool_choice"] = map[string]any{"type": "function", "function": map[string]string{"name": "Read"}}
						}
					}
					encoded, err := json.Marshal(payload)
					if err != nil {
						t.Fatal(err)
					}
					res, err := s.post("/v1/"+protocol, string(encoded))
					if err != nil {
						t.Fatal(err)
					}
					defer res.Body.Close()
					body, err := io.ReadAll(res.Body)
					if err != nil || res.StatusCode != 200 {
						t.Fatalf("response %d %s %v", res.StatusCode, body, err)
					}
					if strings.Contains(string(body), "SYNTHETIC_PREAMBLE") != (choice == "auto") {
						t.Fatalf("tool choice text mismatch: %s", body)
					}
					usageKey := `"output_tokens":23`
					if protocol == "chat/completions" {
						usageKey = `"completion_tokens":23`
					}
					if !strings.Contains(string(body), "Read") || !strings.Contains(string(body), "fixture.txt") || !strings.Contains(string(body), usageKey) {
						t.Fatalf("validated call or full native usage lost: %s", body)
					}
				})
			}
		}
	}
}
