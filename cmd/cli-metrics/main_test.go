package main

import (
	"encoding/json"
	"github.com/szibis/claude-escalate/internal/metrics"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type commandExit int

func commandFixture(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ESCALATE_DATA_DIR", "")
	t.Setenv("CONFIG_FILE", "")
	t.Setenv("ESCALATE_BIND", "")
	t.Chdir(home)
	os.MkdirAll(filepath.Join(home, ".claude"), 0700)
	os.WriteFile(filepath.Join(home, ".claude", "settings.json"), []byte(`{"model":"claude-haiku-4-5"}`), 0600)
	old := exitCommand
	exitCommand = func(n int) { panic(commandExit(n)) }
	t.Cleanup(func() { exitCommand = old })
}
func callCommand(t *testing.T, args []string, fn func()) (string, int) {
	t.Helper()
	oldArgs, oldIn, oldOut, oldErr := os.Args, os.Stdin, os.Stdout, os.Stderr
	os.Args = append([]string{"llm-sentinel"}, args...)
	dir := t.TempDir()
	in, _ := os.Create(filepath.Join(dir, "in"))
	out, _ := os.Create(filepath.Join(dir, "out"))
	errOut, _ := os.Create(filepath.Join(dir, "err"))
	in.Seek(0, 0)
	os.Stdin, os.Stdout, os.Stderr = in, out, errOut
	defer func() {
		os.Args, os.Stdin, os.Stdout, os.Stderr = oldArgs, oldIn, oldOut, oldErr
		in.Close()
		out.Close()
		errOut.Close()
	}()
	code := 0
	func() {
		defer func() {
			if r := recover(); r != nil {
				if e, ok := r.(commandExit); ok {
					code = int(e)
				} else {
					panic(r)
				}
			}
		}()
		fn()
	}()
	out.Seek(0, 0)
	errOut.Seek(0, 0)
	b, _ := io.ReadAll(out)
	return string(b), code
}

func TestMetricsCLIFlagContracts(t *testing.T) {
	commandFixture(t)
	out, code := callCommand(t, []string{"daily", "--format", "json", "--days", "2"}, main)
	if code != 0 {
		t.Fatal(code)
	}
	var rows []interface{}
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatal("JSON format ignored:", out)
	}
}
func TestMetricsViews(t *testing.T) {
	commandFixture(t)
	old := newCommandMetrics
	defer func() { newCommandMetrics = old }()
	sm := metrics.NewSessionMetrics()
	sm.TotalBurned.TotalTokens = 100
	sm.TotalBurned.InputTokens = 80
	sm.TotalBurned.OutputTokens = 20
	sm.TotalSaved.TotalTokensSaved = 50
	sm.TotalRequests = 2
	sm.RecordBurnedTokens(0, 0, 0, 0, 0)
	newCommandMetrics = func() *metrics.SessionMetrics { return sm }
	for _, args := range [][]string{{"overview"}, {"status"}, {"help"}, {"unknown"}, nil} {
		out, code := callCommand(t, args, main)
		if out == "" {
			t.Fatal(args)
		}
		if len(args) == 0 || args[0] == "unknown" {
			if code != 1 {
				t.Fatal(code)
			}
		} else if code != 0 {
			t.Fatal(code)
		}
	}
	for _, format := range []string{"text", "json", "csv"} {
		for _, fn := range []func(){func() { cmdDaily(2, format) }, func() { cmdBreakdown(format) }, func() { cmdProjections(format) }, func() { cmdExport(format) }} {
			out, code := callCommand(t, nil, fn)
			if code != 0 || out == "" {
				t.Fatal(code, out)
			}
		}
	}
	out, _ := callCommand(t, []string{"status"}, main)
	if !strings.Contains(out, "Total Tokens Burned:  100") {
		t.Fatal(out)
	}
}
