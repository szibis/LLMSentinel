package taskquality

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestRecoveryCLIPromptRequestsReadBeforeMissingPath(t *testing.T) {
	corpus, err := fixtureSuite("held-out-marker", "extended")
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range corpus {
		if fixture.ID != "tool-recovery" {
			continue
		}
		prompt := cliPrompt(fixture, "/private/tmp/workspace")
		missing := strings.Index(prompt, "unavailable.txt")
		if missing < 0 {
			t.Fatal("recovery task omitted its missing file")
		}
		// The first requested operation must be reading that file. Generic
		// "file tools" can mean ls/stat, which cannot satisfy failed-read evidence.
		if !regexp.MustCompile(`(?i)\b(read|reading|file-reading)\b`).MatchString(prompt[:missing]) {
			t.Fatalf("missing-file operation does not request reading: %s", prompt[:missing])
		}
		return
	}
	t.Fatal("extended suite omitted the recovery task")
}

func TestCLIRecoveryListingCannotReplaceFailedRead(t *testing.T) {
	fixture := Fixture{ID: "tool-recovery", Path: "recovery.json", Content: `{"marker":"held-out"}`, Expected: `{"marker":"held-out"}`}
	// Recorded Codex event shapes: a listing fails, then a real read and exact
	// final answer succeed. This must remain a failed recovery observation.
	raw := []byte(`{"type":"item.completed","item":{"id":"missing","type":"command_execution","command":"/bin/zsh -lc 'ls unavailable.txt'","status":"failed","exit_code":1,"aggregated_output":"ls: unavailable.txt: No such file or directory\n"}}
{"type":"item.completed","item":{"id":"read","type":"command_execution","command":"/bin/zsh -lc 'cat recovery.json'","status":"completed","exit_code":0,"aggregated_output":"{\"marker\":\"held-out\"}"}}
{"type":"item.completed","item":{"id":"answer","type":"agent_message","text":"{\"marker\":\"held-out\"}"}}
{"type":"turn.completed"}`)
	evidence, err := decodeCLI("codex", raw, "/private/tmp/workspace", fixture)
	if err != nil || !evidence.Read || evidence.Final != fixture.Expected {
		t.Fatalf("recorded recovery transcript was not decoded: %+v %v", evidence, err)
	}
	if evidence.FailedRead || cliFinalPasses(fixture, evidence) {
		t.Fatal("failed directory listing counted as a failed file read")
	}
}

func TestClaudeCodingBudgetAllowsFinalAfterSixToolCalls(t *testing.T) {
	args := cliArguments("claude", "opus", "/private/tmp/workspace", Fixture{ID: "coding-fix"})
	for i, arg := range args {
		if arg != "--max-turns" || i+1 == len(args) {
			continue
		}
		turns, err := strconv.Atoi(args[i+1])
		// The recorded workflow legitimately listed the workspace, read three
		// files, edited the source and ran tests. It still needs a final turn.
		if err != nil || turns < 7 || turns > 12 {
			t.Fatalf("coding budget cannot complete the verified workflow or is unbounded: %q", args[i+1])
		}
		return
	}
	t.Fatal("Claude coding probe omitted its turn bound")
}
