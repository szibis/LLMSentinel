package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestDispatchCommandsValidateBeforeSideEffects(t *testing.T) {
	for _, command := range []string{"capture", "control", "statusline", "dashboard", "lab", "runner", "ci-lab", "role-check", "smoke", "release", "quality", "cli-quality", "coverage", "evidence", "benchmark", "jes"} {
		var out, diagnostic bytes.Buffer
		code := run([]string{command, "--invalid-option"}, strings.NewReader("{}"), &out, &diagnostic)
		if code == 0 {
			t.Errorf("%s accepted unknown option", command)
		}
	}
	for _, args := range [][]string{nil, {"--help"}, {"help"}} {
		var out bytes.Buffer
		if code := run(args, nil, &out, &bytes.Buffer{}); code != 0 || !strings.Contains(out.String(), "sentinel-tools") {
			t.Fatal(args, code, out.String())
		}
	}
	var diagnostic bytes.Buffer
	if code := run([]string{"unknown"}, nil, &bytes.Buffer{}, &diagnostic); code != 2 || !strings.Contains(diagnostic.String(), "unknown") {
		t.Fatal(code, diagnostic.String())
	}
}
