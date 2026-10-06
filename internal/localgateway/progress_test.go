package localgateway

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func progressHistory() claudeRequest {
	var req claudeRequest
	_ = json.Unmarshal([]byte(`{"model":"sentinel-sonnet","messages":[{"role":"user","content":"Review Loki alternatives"},{"role":"assistant","content":[{"type":"tool_use","id":"a","name":"Bash","input":{"command":"find . -name '*.md' | xargs grep -li loki"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"a","content":"docs/ARCHITECTURE.md"}]},{"role":"assistant","content":[{"type":"tool_use","id":"b","name":"Bash","input":{"command":"find . -name '*.md' | xargs grep -li loki"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"b","content":"docs/ARCHITECTURE.md"}]}]}`), &req)
	return req
}

func TestProgressGateRejectsRepeatedLookupAndPromiseOnlyFinal(t *testing.T) {
	req := progressHistory()
	call := claudeBlock{Type: "tool_use", Name: "Bash", Input: map[string]any{"command": "find . -name '*.md' | xargs grep -li loki"}}
	for _, blocks := range [][]claudeBlock{{call}, {{Type: "text", Text: "Let me look at the broader project to understand Loki solutions."}}} {
		if progressIssue(req, blocks) == "" {
			t.Fatalf("accepted no-progress turn: %+v", blocks)
		}
	}
	for _, text := range []string{"Here are the alternatives: A and B.", "I cannot verify current alternatives with the available tools.", "Which Loki API features matter to you?", "Let me explain: Loki stores logs.", "Let me search. Here are the alternatives A and B.", "I'll read these results as a tie between A and B.", "I'll look at these results as a tie between A and B."} {
		if issue := progressIssue(req, []claudeBlock{{Type: "text", Text: text}}); issue != "" {
			t.Fatalf("rejected usable answer: %s", issue)
		}
	}
	if !readOnlyLookup("Bash", map[string]any{"command": "find . -name '*.md' | xargs grep -li loki 2>/dev/null"}) {
		t.Fatal("safe stderr suppression must not disable loop detection")
	}
	req.Messages = append(req.Messages, claudeMessage{Role: "user", Content: json.RawMessage(`"Please run that same lookup again."`)})
	if progressIssue(req, []claudeBlock{call}) != "" {
		t.Fatal("explicit new user turn must reset repetition history")
	}
	req = progressHistory()
	req.Messages[0].Content = json.RawMessage(`"Poll this lookup until the expected result appears."`)
	if progressIssue(req, []claudeBlock{call}) != "" {
		t.Fatal("requested polling must allow repeated lookups")
	}
}

func TestProgressGateAllowsChangedEvidenceAndMutations(t *testing.T) {
	req := progressHistory()
	req.Messages[len(req.Messages)-1].Content = json.RawMessage(`[{"type":"tool_result","tool_use_id":"b","content":"new-results.md"}]`)
	call := claudeBlock{Type: "tool_use", Name: "Bash", Input: map[string]any{"command": "find . -name '*.md' | xargs grep -li loki"}}
	if progressIssue(req, []claudeBlock{call}) != "" {
		t.Fatal("changed evidence must allow another lookup")
	}
	for _, command := range []string{"echo changed > file", "find . -delete", "ls; touch file", "sort -o file file", "sort --output=file file"} {
		if readOnlyLookup("Bash", map[string]any{"command": command}) {
			t.Fatalf("mutation classified as lookup: %s", command)
		}
	}
	req = progressHistory()
	req.Messages = append(req.Messages, claudeMessage{Role: "assistant", Content: json.RawMessage(`[{"type":"tool_use","id":"m","name":"Bash","input":{"command":"sort -o file file"}}]`)}, claudeMessage{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"m","content":"done"}]`)})
	if progressIssue(req, []claudeBlock{call}) != "" {
		t.Fatal("intervening mutation did not reset lookup history")
	}
}

func TestProgressRecoveryKeepsOneBoundedAttemptAndReturnsAnswer(t *testing.T) {
	var attempts atomic.Int32
	s := claudeServer(t, func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		content := "Let me search more broadly for Loki alternatives."
		if attempts.Add(1) == 2 {
			if !strings.Contains(string(mustJSON(payload)), "unfinished") {
				t.Error("missing behavior recovery instruction")
			}
			content = "I cannot verify current alternatives with the available tools. Which Loki compatibility requirements should I compare?"
		}
		_, _ = fmt.Fprintf(w, `{"choices":[{"message":{"content":%s},"finish_reason":"stop"}],"usage":{"prompt_tokens":30,"completion_tokens":20},"mlx_flash_compress":{"native_generation_metadata":true}}`, mustJSON(content))
	})
	req := progressHistory()
	req.MaxTokens = 512
	req.Tools = []claudeTool{{Name: "Bash", Schema: map[string]any{"type": "object", "properties": map[string]any{"command": map[string]any{"type": "string"}}, "required": []string{"command"}}}}
	response, err := s.post("/v1/messages", string(mustJSON(req)))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != 200 || attempts.Load() != 2 || !strings.Contains(string(body), "Which Loki") {
		t.Fatalf("failed bounded recovery: %d %s attempts=%d", response.StatusCode, body, attempts.Load())
	}
}

func TestProgressFailureStopsAfterOneRecovery(t *testing.T) {
	var attempts atomic.Int32
	s := claudeServer(t, func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		fmt.Fprint(w, `{"choices":[{"message":{"content":"Let me search the broader project."},"finish_reason":"stop"}],"usage":{"prompt_tokens":30,"completion_tokens":20},"mlx_flash_compress":{"native_generation_metadata":true}}`)
	})
	req := progressHistory()
	req.MaxTokens = 512
	response, err := s.post("/v1/messages", string(mustJSON(req)))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != 422 || attempts.Load() != 2 || !strings.Contains(string(body), "unfinished_final_answer") {
		t.Fatalf("unbounded or fabricated completion: %d %s %d", response.StatusCode, body, attempts.Load())
	}
	status := s.gateway.controlStatus()["quality_checks"].(map[string]uint64)
	if status["rejections"] != 2 || status["recovery_attempts"] != 1 {
		t.Fatalf("wrong quality accounting: %v", status)
	}
}
