package gateway

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCLIRejectsShellCompositionBeforeExecution(t *testing.T) {
	a := NewCLIAdapter()
	canary := filepath.Join(t.TempDir(), "canary")
	for _, command := range []string{"echo ok; touch " + canary, "echo ok && touch " + canary, "echo $(touch " + canary + ")", "echo `touch " + canary + "`", "echo value > " + canary, "cat /dev/null | sh", "echo ok\ntouch " + canary, "find . -exec sh -c 'echo unsafe' ;", "sed -n '1e echo unsafe' file", "git -c alias.run='!echo unsafe' run", "sort --compress-program=sh"} {
		result, err := a.Execute(context.Background(), &ToolRequest{Params: map[string]interface{}{"command": command}})
		if err != nil || result.Success {
			t.Errorf("unsafe command accepted: %q %+v %v", command, result, err)
		}
	}
	if _, err := os.Stat(canary); !os.IsNotExist(err) {
		t.Fatal("rejected shell created canary", err)
	}
}

func TestCLIPreservesQuotedLiteralPaths(t *testing.T) {
	file := filepath.Join(t.TempDir(), "literal file.txt")
	if err := os.WriteFile(file, []byte("literal\n"), 0600); err != nil {
		t.Fatal(err)
	}
	a := NewCLIAdapter()
	for _, command := range []string{"cat '" + file + "'", "cat \"" + file + "\""} {
		result, err := a.Execute(context.Background(), &ToolRequest{Params: map[string]interface{}{"command": command}})
		if err != nil || !result.Success || result.Data.(map[string]string)["output"] != "literal\n" {
			t.Fatal(result, err)
		}
	}
	for _, command := range []string{"echo 'unterminated", "echo \"unterminated", "echo a\\ b", "date -s now"} {
		if a.isCommandAllowed(command) {
			t.Fatal("unsupported command accepted", command)
		}
	}
}
