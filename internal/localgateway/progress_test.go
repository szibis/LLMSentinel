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

func TestProgressGateChecksExplicitVerbatimNativeRead(t *testing.T) {
	req := claudeRequest{Messages: []claudeMessage{
		{Role: "user", Content: mustJSON("Read evidence.txt, then return its contents exactly.")},
		{Role: "assistant", Content: json.RawMessage(`[{"type":"tool_use","id":"r","name":"functions.exec_command","input":{"cmd":"cat /tmp/evidence.txt"}}]`)},
		{Role: "user", Content: mustJSON([]map[string]any{{"type": "tool_result", "tool_use_id": "r", "content": "Chunk ID: a\nWall time: 1 seconds\nProcess exited with code 0\nOutput:\nArbitrary evidence 9f"}})},
	}}
	final := []claudeBlock{{Type: "text", Text: "Arbitrary evidence 9g"}}
	if issue := progressIssue(req, final); issue != "verbatim_read_mismatch" {
		t.Fatalf("altered evidence accepted: %q", issue)
	}
	final[0].Text = "Arbitrary evidence 9f"
	if issue := progressIssue(req, final); issue != "" {
		t.Fatal(issue)
	}
	for _, command := range []string{"cat /tmp/other.txt", "cat /tmp/evidence.txt /tmp/other.txt", "cat /tmp/evidence.txt; echo extra"} {
		req.Messages[1].Content = mustJSON([]map[string]any{{"type": "tool_use", "id": "r", "name": "exec_command", "input": map[string]any{"cmd": command}}})
		final[0].Text = "summary"
		if issue := progressIssue(req, final); issue != "" {
			t.Fatalf("ambiguous read checked: %s: %s", command, issue)
		}
	}
	req.Messages[0].Content = mustJSON("Read /a/evidence.txt and return its contents exactly.")
	req.Messages[1].Content = json.RawMessage(`[{"type":"tool_use","id":"r","name":"exec_command","input":{"cmd":"cat /b/evidence.txt"}}]`)
	if issue := progressIssue(req, final); issue != "" {
		t.Fatalf("different explicit path became authoritative: %s", issue)
	}
}

func TestProgressGateChecksExplicitFinalFormatAndTestEvidence(t *testing.T) {
	req := claudeRequest{Messages: []claudeMessage{{Role: "user", Content: json.RawMessage(`"Read the evidence. Return only JSON with the answer."`)}}}
	if issue := progressIssue(req, []claudeBlock{{Type: "text", Text: "I have read the file and will now analyze it."}}); issue != "requested_json_missing" {
		t.Fatalf("unformatted ending accepted: %q", issue)
	}
	if issue := progressIssue(req, []claudeBlock{{Type: "text", Text: `{"answer":"done"}`}}); issue != "" {
		t.Fatal(issue)
	}
	req.Tools = []claudeTool{{Name: "Bash"}}
	req.Messages[0].Content = json.RawMessage(`"Fix the function. Run exactly go test -timeout 10s ./... and report the result."`)
	if issue := progressIssue(req, []claudeBlock{{Type: "text", Text: "Fixed and tested."}}); issue != "requested_tests_not_executed" {
		t.Fatalf("invented test completion accepted: %q", issue)
	}
	req.Messages = append(req.Messages,
		claudeMessage{Role: "assistant", Content: json.RawMessage(`[{"type":"tool_use","id":"test","name":"Bash","input":{"command":"go test -timeout 10s ./..."}}]`)},
		claudeMessage{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"test","content":"ok fixture 0.001s"}]`)})
	if issue := progressIssue(req, []claudeBlock{{Type: "text", Text: "Fixed and tested."}}); issue != "" {
		t.Fatal(issue)
	}
}

func TestProgressGateRecognizesWrappedTestsAndWaitsForSessionExit(t *testing.T) {
	for _, command := range []string{"rtk go test ./...", "env GOWORK=off go test ./...", "rtk env GOWORK=off go test ./...", "GOWORK=off go test ./...", "cd pkg && go test ./..."} {
		if !isTestExecution("exec_command", map[string]any{"cmd": command}) {
			t.Errorf("missed test: %s", command)
		}
	}
	req := claudeRequest{Tools: []claudeTool{{Name: "exec_command"}}, Messages: []claudeMessage{
		{Role: "user", Content: json.RawMessage(`"Run go test ./... and report."`)},
		{Role: "assistant", Content: json.RawMessage(`[{"type":"tool_use","id":"one","name":"exec_command","input":{"cmd":"go test ./..."}}]`)},
		{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"one","content":"Chunk ID: abc\nWall time: 1 seconds\nProcess running with session ID 42\nOutput:\n"}]`)},
	}}
	final := []claudeBlock{{Type: "text", Text: "Tests passed."}}
	if issue := progressIssue(req, final); issue != "requested_tests_not_executed" {
		t.Fatalf("ongoing test accepted: %q", issue)
	}
	req.Messages = append(req.Messages,
		claudeMessage{Role: "assistant", Content: json.RawMessage(`[{"type":"tool_use","id":"two","name":"write_stdin","input":{"session_id":42,"chars":""}}]`)},
		claudeMessage{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"two","content":"Chunk ID: def\nWall time: 1 seconds\nProcess exited with code 0\nOutput:\nok fixture"}]`)})
	if issue := progressIssue(req, final); issue != "" {
		t.Fatalf("completed test rejected: %q", issue)
	}
}

func TestProgressGateRejectsReadDisplayPrefixesWhenExplicitlyExcluded(t *testing.T) {
	req := claudeRequest{Messages: []claudeMessage{
		{Role: "user", Content: json.RawMessage(`"Read the file and return its contents without line numbers."`)},
		{Role: "assistant", Content: json.RawMessage(`[{"type":"tool_use","id":"read","name":"Read","input":{"file_path":"file.txt"}}]`)},
		{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"read","content":"1\tactual text"}]`)},
	}}
	if issue := progressIssue(req, []claudeBlock{{Type: "text", Text: "1\tactual text"}}); issue != "read_display_metadata" {
		t.Fatalf("display metadata accepted: %q", issue)
	}
	if issue := progressIssue(req, []claudeBlock{{Type: "text", Text: "actual text"}}); issue != "" {
		t.Fatal(issue)
	}
}

func TestProgressGateRequiresEvidenceForExplicitFileRead(t *testing.T) {
	req := claudeRequest{Tools: []claudeTool{{Name: "exec_command"}}, Messages: []claudeMessage{{Role: "user", Content: json.RawMessage(`"Read dependencies.json and return JSON with the execution order."`)}}}
	if issue := progressIssue(req, []claudeBlock{{Type: "text", Text: `{"order":[]}`}}); issue != "requested_read_not_executed" {
		t.Fatalf("invented file answer accepted: %q", issue)
	}
	if issue := progressIssue(req, []claudeBlock{{Type: "text", Text: `{"error":"I cannot read the file with the available tools."}`}}); issue != "" {
		t.Fatal(issue)
	}
}

func TestProgressGateStopsReadAfterNativeUnchangedNotice(t *testing.T) {
	req := claudeRequest{Messages: []claudeMessage{
		{Role: "user", Content: json.RawMessage(`"Read file.txt and analyze it."`)},
		{Role: "assistant", Content: json.RawMessage(`[{"type":"tool_use","id":"read","name":"Read","input":{"file_path":"file.txt"}}]`)},
		{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"read","content":"Wasted call — file unchanged since your last Read. Refer to that earlier tool_result instead."}]`)},
	}}
	if issue := progressIssue(req, []claudeBlock{{Type: "tool_use", Name: "Read", Input: map[string]any{"file_path": "file.txt"}}}); issue != "repeated_unchanged_lookup" {
		t.Fatalf("wasted read admitted: %q", issue)
	}
}

func TestProgressGatePreservesExactTestCommandAndAllowsExplicitWrappers(t *testing.T) {
	req := claudeRequest{Messages: []claudeMessage{{Role: "user", Content: json.RawMessage(`"Run exactly go test -timeout 10s ./... in the current directory."`)}}}
	call := claudeBlock{Type: "tool_use", Name: "exec_command", Input: map[string]any{"cmd": "cd pkg && go test -timeout 10s ./..."}}
	if issue := progressIssue(req, []claudeBlock{call}); issue != "requested_test_command_wrapped" {
		t.Fatalf("exact command changed: %q", issue)
	}
	req.Messages[0].Content = json.RawMessage(`"Run exactly cd pkg && go test -timeout 10s ./..."`)
	if issue := progressIssue(req, []claudeBlock{call}); issue != "" {
		t.Fatal("explicit wrapper rejected", issue)
	}
}

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

func TestProgressGateRejectsFirstTurnResearchPromises(t *testing.T) {
	req := claudeRequest{Messages: []claudeMessage{{Role: "user", Content: json.RawMessage(`"Review all Loki-compatible logging solutions"`)}}}
	for _, text := range []string{
		"I will research and compare various observability logging solutions to provide you with a comprehensive review.",
		"I will read the fixture file and then create the requested table.",
		"I will read the `loki-options.md` file to extract the solutions and then create a summary table of their Loki logging API compatibility.",
	} {
		if progressIssue(req, []claudeBlock{{Type: "text", Text: text}}) != "unfinished_final_answer" {
			t.Fatalf("initial promise accepted: %s", text)
		}
	}
	for _, prompt := range []string{`"Give me a plan for what you will read next"`, `"Return exactly: I will read the fixture file and then create the requested table."`} {
		req.Messages[0].Content = json.RawMessage(prompt)
		if issue := progressIssue(req, []claudeBlock{{Type: "text", Text: "I will read the fixture file and then create the requested table."}}); issue != "" {
			t.Fatalf("requested planning or literal sentence rejected: %s", issue)
		}
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
