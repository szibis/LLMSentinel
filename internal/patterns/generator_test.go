package patterns

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/szibis/claude-escalate/internal/execlog"
)

func TestPatternsReflectSyntheticExecutionHistory(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "events.jsonl")
	var data []byte
	for _, command := range []string{"fast", "slow-a", "slow-b", strings.Repeat("x", 80)} {
		for i := 0; i < 3; i++ {
			duration := int64(3000)
			if command == "fast" {
				duration = 100
			}
			b, err := json.Marshal(execlog.Entry{CommandNormalized: command, DurationMS: duration, Status: "success"})
			if err != nil {
				t.Fatal(err)
			}
			data = append(data, b...)
			data = append(data, '\n')
		}
	}
	if err := os.WriteFile(log, data, 0600); err != nil {
		t.Fatal(err)
	}
	r, err := execlog.NewReader(log)
	if err != nil {
		t.Fatal(err)
	}
	g := New(r)
	md := g.Generate()
	for _, fragment := range []string{"from 12 operations", "**fast** — 100ms", "slow-a", "Many slow operations detected (3)", "Repetitions: 3", "Potential Savings: 6000ms", strings.Repeat("x", 60)} {
		if !strings.Contains(md, fragment) {
			t.Errorf("missing %q", fragment)
		}
	}
	out := filepath.Join(dir, "patterns.md")
	if err := g.WriteFile(out); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(out)
	if err != nil || st.Mode().Perm() != 0600 {
		t.Fatal(st, err)
	}
	if err := g.WriteFile(filepath.Join(dir, "missing", "patterns.md")); err == nil {
		t.Fatal("write failure hidden")
	}
	if truncate("short", 60) != "short" {
		t.Fatal("short text changed")
	}
}

func TestTruncateRespectsRequestedDisplayLimit(t *testing.T) {
	for _, tc := range []struct {
		text string
		max  int
		want string
	}{{"operation", 4, "oper"}, {"abcd", 4, "abcd"}, {"short", 60, "short"}, {"hidden", 0, ""}} {
		if got := truncate(tc.text, tc.max); got != tc.want {
			t.Fatalf("truncate(%q,%d)=%q want %q", tc.text, tc.max, got, tc.want)
		}
	}
}
