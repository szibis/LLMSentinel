package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/szibis/claude-escalate/internal/classify"
	"github.com/szibis/claude-escalate/internal/config"
	"github.com/szibis/claude-escalate/internal/dashboard"
	"github.com/szibis/claude-escalate/internal/execlog"
	"github.com/szibis/claude-escalate/internal/hook"
	"github.com/szibis/claude-escalate/internal/store"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type commandExit int

func commandFixture(t *testing.T) string {
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
	return home
}
func callCommand(t *testing.T, args []string, input string, fn func()) (string, string, int) {
	t.Helper()
	oldArgs, oldIn, oldOut, oldErr := os.Args, os.Stdin, os.Stdout, os.Stderr
	os.Args = append([]string{"llm-sentinel"}, args...)
	dir := t.TempDir()
	in, _ := os.Create(filepath.Join(dir, "in"))
	out, _ := os.Create(filepath.Join(dir, "out"))
	errOut, _ := os.Create(filepath.Join(dir, "err"))
	in.WriteString(input)
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
	e, _ := io.ReadAll(errOut)
	return string(b), string(e), code
}
func TestCommandUsageAndInformationalCommands(t *testing.T) {
	commandFixture(t)
	for _, args := range [][]string{nil, {"unknown"}, {"version"}, {"install-hook"}} {
		out, errOut, code := callCommand(t, args, "", main)
		if len(args) == 0 || args[0] == "unknown" {
			if code != 1 || !strings.Contains(errOut, "Usage:") {
				t.Fatal(code, errOut)
			}
		} else if code != 0 || out == "" {
			t.Fatal(code, out)
		}
	}
	for input, want := range map[string]string{"": "", "haiku": "Haiku", "sonnet": "Sonnet"} {
		if got := capitalize(input); got != want {
			t.Fatal(got)
		}
	}
	if parseIntFlag("bad") != 0 || parseIntFlag("12") != 12 {
		t.Fatal("integer flags")
	}
}
func decodeCommandHook(t *testing.T, out string) *hook.Output {
	t.Helper()
	var v hook.Output
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatal(out, err)
	}
	if !v.Continue || !v.SuppressOutput {
		t.Fatal(v)
	}
	return &v
}
func runPrompt(t *testing.T, prompt string) *hook.Output {
	t.Helper()
	b, _ := json.Marshal(hook.Input{Prompt: prompt})
	out, _, code := callCommand(t, []string{"hook"}, string(b), main)
	if code != 0 {
		t.Fatal(code)
	}
	return decodeCommandHook(t, out)
}
func TestHookInputAndSettingsFailures(t *testing.T) {
	home := commandFixture(t)
	for _, input := range []string{"{", `{"prompt":""}`} {
		out, _, _ := callCommand(t, []string{"hook"}, input, main)
		if decodeCommandHook(t, out).HookOutput != nil {
			t.Fatal(out)
		}
	}
	os.MkdirAll(filepath.Join(home, ".claude"), 0700)
	os.WriteFile(filepath.Join(home, ".claude", "settings.json"), []byte("{"), 0600)
	if runPrompt(t, "hello").HookOutput != nil {
		t.Fatal("bad settings must pass through")
	}
	os.RemoveAll(filepath.Join(home, ".claude"))
	os.WriteFile(filepath.Join(home, ".claude"), []byte("blocked"), 0600)
	if runPrompt(t, "hello").HookOutput != nil {
		t.Fatal("bad database path must pass through")
	}
}
func TestHookModelSwitchingAndPredictions(t *testing.T) {
	commandFixture(t)
	config.WriteClaudeSettings(config.ModelHaiku, "low")
	if runPrompt(t, "/help").HookOutput != nil {
		t.Fatal("meta command hint")
	}
	for _, target := range []string{"opus", "sonnet", "haiku"} {
		v := runPrompt(t, "/escalate to "+target)
		if !strings.Contains(fmt.Sprint(v.HookOutput), "Escalated:") {
			t.Fatal(v)
		}
	}
	db, err := store.Open(config.DefaultConfig().Gateway.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		db.LogEscalation("haiku", "sonnet", string(classify.Classify("debug this panic")), "user_command")
	}
	db.Close()
	config.WriteClaudeSettings(config.ModelHaiku, "low")
	if !strings.Contains(fmt.Sprint(runPrompt(t, "debug this panic").HookOutput), "Predictive:") {
		t.Fatal("predictive escalation missing")
	}
	config.WriteClaudeSettings(config.ModelOpus, "high")
	if !strings.Contains(fmt.Sprint(runPrompt(t, "thanks, it works!").HookOutput), "Auto-downgrade:") {
		t.Fatal("downgrade missing")
	}
}
func TestDirectEscalationContracts(t *testing.T) {
	home := commandFixture(t)
	db, e := store.Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	for _, target := range []string{"opus", "sonnet", "haiku", "unknown"} {
		out, _, _ := callCommand(t, nil, "", func() { handleEscalate(db, config.ModelHaiku, target) })
		if !strings.Contains(fmt.Sprint(decodeCommandHook(t, out).HookOutput), "Escalated:") {
			t.Fatal(out)
		}
	}
	for _, model := range []string{config.ModelOpus, config.ModelSonnet, config.ModelHaiku} {
		db.SetSession("escalation_active", "true")
		out, _, _ := callCommand(t, nil, "", func() { handleDeEscalate(db, model, "coding") })
		v := decodeCommandHook(t, out)
		if model != config.ModelHaiku && !strings.Contains(fmt.Sprint(v.HookOutput), "Auto-downgrade") {
			t.Fatal(out)
		}
	}
	db.DeleteSession("escalation_active")
	out, _, _ := callCommand(t, nil, "", func() { handleDeEscalate(db, config.ModelSonnet, "coding") })
	if decodeCommandHook(t, out).HookOutput != nil {
		t.Fatal(out)
	}
	db.LogTurn("sonnet", "code")
	db.LogTurn("sonnet", "code")
	out, _, _ = callCommand(t, nil, "", func() { handleDeEscalate(db, config.ModelSonnet, "coding") })
	if decodeCommandHook(t, out).HookOutput == nil {
		t.Fatal("repeated costly turns")
	}
	os.RemoveAll(filepath.Join(home, ".claude"))
	os.WriteFile(filepath.Join(home, ".claude"), []byte("blocked"), 0600)
	out, _, _ = callCommand(t, nil, "", func() { handleEscalate(db, config.ModelHaiku, "opus") })
	if decodeCommandHook(t, out).HookOutput != nil {
		t.Fatal("failed settings")
	}
	db.SetSession("escalation_active", "true")
	out, _, _ = callCommand(t, nil, "", func() { handleDeEscalate(db, config.ModelOpus, "coding") })
	if decodeCommandHook(t, out).HookOutput != nil {
		t.Fatal("failed downgrade")
	}
}
func TestStatsCommands(t *testing.T) {
	commandFixture(t)
	db, e := store.Open(config.DefaultConfig().Gateway.DataDir)
	if e != nil {
		t.Fatal(e)
	}
	db.LogTurn("haiku", "code")
	for _, task := range []struct {
		kind string
		n    int
	}{{"coding", 5}, {"debugging", 3}} {
		for i := 0; i < task.n; i++ {
			db.LogEscalation("haiku", "sonnet", task.kind, "user_command")
		}
	}
	db.LogEscalation("sonnet", "haiku", "coding", "success")
	db.Close()
	for _, sub := range []string{"summary", "types", "predictions", "history", "reset", "unknown"} {
		out, errOut, code := callCommand(t, []string{"stats", sub}, "", main)
		if code != 0 {
			t.Fatal(code)
		}
		if sub == "unknown" {
			if !strings.Contains(errOut, "Unknown stats") {
				t.Fatal(errOut)
			}
		} else if out == "" {
			t.Fatal(sub)
		}
	}
	out, _, _ := callCommand(t, []string{"stats"}, "", main)
	if !strings.Contains(out, "Success rate") {
		t.Fatal(out)
	}
	empty, _ := store.Open(filepath.Join(t.TempDir(), "db"))
	defer empty.Close()
	out, _, _ = callCommand(t, nil, "", func() { printStatsPredictions(empty, nil) })
	if !strings.Contains(out, "No task types") {
		t.Fatal(out)
	}
}
func writeExecutionFixture(t *testing.T, path string) {
	t.Helper()
	var lines []string
	for i := 0; i < 4; i++ {
		e := execlog.Entry{Timestamp: time.Now(), SessionID: "fixture", OperationType: "bash", OperationID: fmt.Sprint(i), Command: "go test", CommandNormalized: "go test", Status: "success", DurationMS: 1200, CacheKey: "test", RepetitionsThisSession: i}
		b, _ := json.Marshal(e)
		lines = append(lines, string(b))
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
}
func TestAnalyticsAndPatternsCommands(t *testing.T) {
	home := commandFixture(t)
	log := filepath.Join(home, ".execution-log.jsonl")
	writeExecutionFixture(t, log)
	for _, args := range [][]string{{"analytics"}, {"analytics", "--log-file", log, "--all"}, {"analytics", "--summary", "--slowest", "2", "--duplicates", "2", "--recommendations", "--json"}, {"analytics", "--slowest", "0", "--duplicates", "0"}} {
		out, _, code := callCommand(t, args, "", main)
		if code != 0 {
			t.Fatal(code)
		}
		if len(args) < 3 && !strings.Contains(out, "Total Operations: 4") {
			t.Fatal(out)
		}
	}
	output := filepath.Join(home, "patterns.md")
	out, _, code := callCommand(t, []string{"generate-patterns", "--log-file", log, "--output", output}, "", main)
	if code != 0 || !strings.Contains(out, "4 operations") {
		t.Fatal(code, out)
	}
	b, _ := os.ReadFile(output)
	if !strings.Contains(string(b), "Execution Patterns") {
		t.Fatal(string(b))
	}
	for _, root := range []string{t.TempDir(), home} {
		out, _, code = callCommand(t, []string{"session-startup", "--project-root", root}, "", main)
		if code != 0 || !strings.Contains(out, "✅") {
			t.Fatal(code, out)
		}
		out, _, _ = callCommand(t, []string{"session-startup", "--project-root", root}, "", main)
		if out == "" {
			t.Fatal("repeat startup")
		}
	}
	var errOut string
	for _, command := range []string{"analytics", "generate-patterns"} {
		_, errOut, code = callCommand(t, []string{command, "--log-file", home}, "", main)
		if code != 1 || errOut == "" {
			t.Fatal(code, errOut)
		}
	}
	_, errOut, code = callCommand(t, []string{"generate-patterns", "--output", home}, "", main)
	if code != 1 || errOut == "" {
		t.Fatal(code, errOut)
	}
	out, errOut, _ = callCommand(t, []string{"session-startup", "--project-root", filepath.Join(home, "missing", "root")}, "", main)
	if !strings.Contains(out, "Session initialized") || !strings.Contains(errOut, "Warning:") {
		t.Fatal(out, errOut)
	}
}

type fakeCommandService struct {
	addr string
	err  error
}

func (s *fakeCommandService) Start(addr string) error { s.addr = addr; return s.err }
func TestDaemonCommandOrchestration(t *testing.T) {
	commandFixture(t)
	oldService, oldDash, oldWait := newCommandService, startCommandDashboard, waitMonitor
	defer func() { newCommandService, startCommandDashboard, waitMonitor = oldService, oldDash, oldWait }()
	fake := &fakeCommandService{}
	newCommandService = func(cfg *config.Config) (commandService, error) {
		if cfg.Gateway.DataDir == "" {
			t.Fatal("missing data directory")
		}
		return fake, nil
	}
	startCommandDashboard = func(*dashboard.Server) error { return nil }
	waitMonitor = func() {}
	for _, args := range [][]string{{"service", "--port", "9100"}, {"dashboard", "--port", "8100", "--bind", "127.0.0.1", "--config", "missing.yaml"}, {"monitor", "--port", "9100"}} {
		_, _, code := callCommand(t, args, "", main)
		if code != 0 {
			t.Fatal(args, code)
		}
	}
	if fake.addr != "0.0.0.0:9100" {
		t.Fatal(fake.addr)
	}
	t.Setenv("ESCALATE_BIND", "127.0.0.1")
	t.Setenv("CONFIG_FILE", "fixture.yaml")
	t.Setenv("ESCALATE_DATA_DIR", t.TempDir())
	callCommand(t, []string{"dashboard"}, "", main)
	fake.err = errors.New("fixture start failure")
	_, errOut, code := callCommand(t, []string{"service"}, "", main)
	if code != 1 || !strings.Contains(errOut, "fixture start failure") {
		t.Fatal(code, errOut)
	}
	newCommandService = func(*config.Config) (commandService, error) { return nil, errors.New("fixture construction failure") }
	_, errOut, code = callCommand(t, []string{"service"}, "", main)
	if code != 1 || !strings.Contains(errOut, "fixture construction failure") {
		t.Fatal(code, errOut)
	}
	startCommandDashboard = func(*dashboard.Server) error { return errors.New("fixture dashboard failure") }
	_, errOut, code = callCommand(t, []string{"dashboard"}, "", main)
	if code != 1 || !strings.Contains(errOut, "fixture dashboard failure") {
		t.Fatal(code, errOut)
	}
}
