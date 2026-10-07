//go:build darwin || linux || freebsd

package taskquality

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Exercise the full runner and real independent Go verifier without a model.
func TestCLICodingRunnerVerifiesMockClientEdit(t *testing.T) {
	root := canonicalRoot(t)
	bin := filepath.Join(root, "clients", "node_modules", ".bin", "codex")
	if err := os.MkdirAll(filepath.Dir(bin), 0700); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
if [ "$1" = "--version" ]; then printf 'mock-codex\n'; exit 0; fi
printf 'package fixture\nfunc Add(a, b int) int { return a+b }\n' > add.go
printf '%s\n' '{"type":"item.completed","item":{"id":"read","type":"command_execution","command":"cat add.go","exit_code":0,"status":"completed","aggregated_output":"func Add(a, b int) int"}}'
printf '%s\n' '{"type":"item.completed","item":{"id":"test","type":"command_execution","command":"go test -timeout 10s ./...","exit_code":0,"status":"completed","aggregated_output":"ok fixture 0.001s"}}'
printf '%s\n' '{"type":"item.completed","item":{"id":"answer","type":"agent_message","text":"FIXED_AND_TESTED"}}' '{"type":"turn.completed"}'
`
	if err := os.WriteFile(bin, []byte(script), 0700); err != nil { // #nosec G306 -- private executable mock client fixture.
		t.Fatal(err)
	}
	var coding Fixture
	for _, f := range fixtures("marker") {
		if f.ID == "coding-fix" {
			coding = f
		}
	}
	r := executeCLI(context.Background(), root, "http://127.0.0.1:19090", "sonnet", "codex", coding, 90*time.Second)
	if r.Workspace != "" {
		t.Cleanup(func() { _ = os.RemoveAll(r.Workspace) })
	}
	if !r.Passed || !r.Checks["independent_tests"] || !r.Checks["edit_and_tests_preserved"] {
		t.Fatalf("full coding verifier failed: %+v", r)
	}
	if r.InputTokens != nil || r.OutputTokens != nil {
		t.Fatal("mock client missing usage was invented")
	}
}
