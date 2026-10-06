package localgateway

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestChatCompletionsFunctionRoundTrip(t *testing.T) {
	calls := 0
	s := claudeServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		text := `{"text":"","tool_calls":[{"name":"Read","input":{"file_path":"a.txt"}}]}`
		if calls == 2 {
			var p map[string]any
			json.NewDecoder(r.Body).Decode(&p)
			if !strings.Contains(string(mustJSON(p)), "FILE_EVIDENCE") {
				t.Error("lost tool evidence")
			}
			text = `{"text":"FILE_EVIDENCE","tool_calls":[]}`
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": text}, "finish_reason": "stop"}}, "usage": map[string]int{"prompt_tokens": 30, "completion_tokens": 10}})
	})
	tools := `"tools":[{"type":"function","function":{"name":"Read","parameters":{"type":"object","required":["file_path"],"properties":{"file_path":{"type":"string"}}}}}]`
	res, _ := s.post("/v1/chat/completions", `{"model":"local","max_tokens":128,"messages":[{"role":"user","content":"Read a.txt"}],`+tools+`}`)
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	var response struct {
		Choices []struct {
			Message map[string]any `json:"message"`
			Finish  string         `json:"finish_reason"`
		} `json:"choices"`
	}
	json.Unmarshal(body, &response)
	if res.StatusCode != 200 || len(response.Choices) != 1 || response.Choices[0].Finish != "tool_calls" {
		t.Fatalf("bad Chat response %d %s", res.StatusCode, body)
	}
	toolCall := response.Choices[0].Message["tool_calls"].([]any)[0].(map[string]any)
	messages := []any{map[string]any{"role": "user", "content": "Read a.txt"}, response.Choices[0].Message, map[string]any{"role": "tool", "tool_call_id": toolCall["id"], "content": "FILE_EVIDENCE"}}
	res, _ = s.post("/v1/chat/completions", fmt.Sprintf(`{"model":"local","messages":%s,%s}`, mustJSON(messages), tools))
	body, _ = io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !strings.Contains(string(body), `"content":"FILE_EVIDENCE"`) {
		t.Fatalf("bad continuation %d %s", res.StatusCode, body)
	}
}

func TestChatStreamingUsesCompletionChunks(t *testing.T) {
	s := claudeServer(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"content":"Hi"},"finish_reason":"stop"}],"usage":{"completion_tokens":1}}`)
	})
	res, _ := s.post("/v1/chat/completions", `{"model":"local","messages":[{"role":"user","content":"Hi"}],"stream":true}`)
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !strings.Contains(string(body), `"object":"chat.completion.chunk"`) || !strings.Contains(string(body), "data: [DONE]") || strings.Contains(string(body), "event:") {
		t.Fatalf("wrong stream %d %s", res.StatusCode, body)
	}
}
