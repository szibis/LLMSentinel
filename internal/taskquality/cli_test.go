package taskquality

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func jsonLines(values ...any) []byte {
	var lines []string
	for _, v := range values {
		lines = append(lines, toJSON(v))
	}
	return []byte(strings.Join(lines, "\n"))
}

func TestClaudeCLIRequiresExecutedReadAndSuccessfulFinal(t *testing.T) {
	f := fixtures("fresh-marker")[0]
	workspace := "/private/tmp/fixture"
	call := object{"type": "assistant", "message": object{"content": []any{object{"type": "tool_use", "id": "read1", "name": "Read", "input": object{"file_path": filepath.Join(workspace, f.Path)}}}}}
	output := object{"type": "user", "message": object{"content": []any{object{"type": "tool_result", "tool_use_id": "read1", "content": "1→fresh-marker"}}}}
	final := object{"type": "result", "subtype": "success", "is_error": false, "result": f.Expected, "usage": object{"input_tokens": 12, "output_tokens": 3}}
	evidence, err := decodeCLI("claude", jsonLines(call, output, final), workspace, f)
	if err != nil || evidence.Final != f.Expected || !evidence.Read || evidence.ToolCalls != 1 {
		t.Fatalf("valid CLI evidence lost: %+v %v", evidence, err)
	}
	for _, bad := range []object{
		{"type": "result", "subtype": "error_max_turns", "is_error": true, "result": f.Expected},
		{"type": "result", "subtype": "success", "is_error": false, "result": "Let me read the file."},
	} {
		e, err := decodeCLI("claude", jsonLines(call, output, bad), workspace, f)
		if err == nil && cliFinalPasses(f, e) {
			t.Fatal("failed CLI task accepted")
		}
	}
	e, err := decodeCLI("claude", jsonLines(final), workspace, f)
	if err == nil && cliFinalPasses(f, e) {
		t.Fatal("claimed read without tool execution accepted")
	}
	obj(output["message"])["content"] = []any{object{"type": "tool_result", "tool_use_id": "read1", "content": "fresh-marker", "is_error": true}}
	e, err = decodeCLI("claude", jsonLines(call, output, final), workspace, f)
	if err == nil && cliFinalPasses(f, e) {
		t.Fatal("failed read accepted")
	}
}

func TestCodexCLIRequiresCommandOutcomeAndCompletedTurn(t *testing.T) {
	f := fixtures("fresh-marker")[0]
	workspace := "/private/tmp/fixture"
	read := object{"type": "item.completed", "item": object{"id": "cmd1", "type": "command_execution", "command": "/bin/zsh -lc 'cat proof.txt'", "exit_code": 0, "status": "completed", "aggregated_output": "fresh-marker"}}
	answer := object{"type": "item.completed", "item": object{"id": "msg1", "type": "agent_message", "text": f.Expected}}
	end := object{"type": "turn.completed", "usage": object{"input_tokens": 0, "output_tokens": 0}}
	e, err := decodeCLI("codex", jsonLines(read, answer, end), workspace, f)
	if err != nil || !cliFinalPasses(f, e) || e.InputTokens != nil {
		t.Fatalf("valid evidence lost or zero usage invented: %+v %v", e, err)
	}
	obj(read["item"])["exit_code"] = 1
	e, err = decodeCLI("codex", jsonLines(read, answer, end), workspace, f)
	if err == nil && cliFinalPasses(f, e) {
		t.Fatal("failed command accepted")
	}
	if _, err := decodeCLI("codex", jsonLines(answer), workspace, f); err == nil {
		t.Fatal("unterminated turn accepted")
	}
	if _, err := decodeCLI("codex", jsonLines(answer, object{"type": "turn.failed", "error": object{"message": "HTTP 422"}}), workspace, f); err == nil {
		t.Fatal("terminal failure accepted")
	}
}

func TestCodingProofRejectsTamperedTestsAndUnsafeSource(t *testing.T) {
	root := canonicalRoot(t)
	for _, name := range []string{"add.go", "add_test.go", "go.mod"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(codeFiles()[name]), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := verifyCode(root); err == nil {
		t.Fatal("original subtraction accepted")
	}
	if err := os.WriteFile(filepath.Join(root, "add.go"), []byte("package fixture\nfunc Add(a, b int) int { return b + a }\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifyCode(root); err != nil {
		t.Fatalf("correct equivalent fix rejected: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "add_test.go"), []byte("package fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifyCode(root); err == nil {
		t.Fatal("tampered tests accepted")
	}
	_ = os.WriteFile(filepath.Join(root, "add_test.go"), []byte(codeFiles()["add_test.go"]), 0600)
	_ = os.WriteFile(filepath.Join(root, "add.go"), []byte("package fixture\nimport \"os\"\nfunc Add(a,b int) int { os.Exit(0); return a+b }"), 0600)
	if err := verifyCode(root); err == nil {
		t.Fatal("unsafe generated source accepted")
	}
}

func TestCLIReportDoesNotReplaceAPIBaseline(t *testing.T) {
	root := canonicalRoot(t)
	api := Report{Version: 1, Scope: "bounded-api-task-probes", FinishedAt: nowUTC(), Results: []Result{{Task: "exact-read", Passed: true}}}
	cli := Report{Version: 1, Scope: "real-cli-task-probes", FinishedAt: nowUTC(), Results: []Result{{Task: "exact-read", Client: "claude", Failure: "HTTP 422", CLIEvents: []json.RawMessage{json.RawMessage(`{"type":"error"}`)}}}}
	if err := Save(root, api); err != nil {
		t.Fatal(err)
	}
	if err := Save(root, cli); err != nil {
		t.Fatal(err)
	}
	if r := Load(root); r == nil || !r.Results[0].Passed {
		t.Fatal("API baseline overwritten")
	}
	if r := LoadCLI(root); r == nil || r.Results[0].Failure != "HTTP 422" || len(r.Results[0].CLIEvents) != 0 {
		t.Fatal("CLI summary missing or leaks transcript")
	}
}

func TestCLIEnvironmentHasNoHostCredentialsOrConfig(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "host-secret")
	t.Setenv("ANTHROPIC_API_KEY", "host-secret")
	t.Setenv("CODEX_HOME", "/host/config")
	t.Setenv("HTTP_PROXY", "http://host-proxy")
	env := cliEnvironment("codex", "/private/tmp/run", "http://127.0.0.1:19090")
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "host-secret") || strings.Contains(joined, "/host/") || strings.Contains(joined, "host-proxy") {
		t.Fatal("host settings inherited")
	}
	if !strings.Contains(joined, "CODEX_HOME=/private/tmp/run/codex") {
		t.Fatal("isolated profile missing")
	}
}

func TestNativePromptsNeverReferenceAPIFixtureTool(t *testing.T) {
	for _, f := range fixtures("secret-marker") {
		prompt := cliPrompt(f, "/private/tmp/workspace")
		if strings.Contains(prompt, "read_fixture") {
			t.Errorf("%s requested a nonexistent CLI tool", f.ID)
		}
		if strings.Contains(prompt, "secret-marker") {
			t.Error("answer leaked into the prompt")
		}
	}
}

func TestCommandReadRejectsShellTextThatOnlyMentionsFixture(t *testing.T) {
	f := fixtures("fresh-marker")[0]
	for _, command := range []string{
		"cat /dev/null # proof.txt\nprintf fresh-marker",
		"cat /dev/null; printf fresh-marker proof.txt",
		"cat proof.txt > /dev/null; printf fresh-marker",
		"cat $(printf proof.txt)",
		"awk 'BEGIN { print \"fresh-marker\"; exit }' proof.txt",
	} {
		if commandRead(command, "/private/tmp/fixture", f) {
			t.Errorf("false read evidence: %s", command)
		}
	}
}
