package main

import (
	"github.com/szibis/claude-escalate/internal/config"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

type dashboardRT func(*http.Request) (*http.Response, error)

func (f dashboardRT) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestConfigurationCommandPersistence(t *testing.T) {
	commandFixture(t)
	out, code := callCommand(t, []string{"set-budget", "--daily", "12.5", "--monthly", "100", "--session", "2000"}, main)
	if code != 0 || !strings.Contains(out, "$12.50") {
		t.Fatal(code, out)
	}
	cfg, e := config.LoadEscalationConfig()
	if e != nil || cfg.Budgets.DailyUSD != 12.5 || cfg.Budgets.MonthlyUSD != 100 || cfg.Budgets.SessionTokens != 2000 {
		t.Fatal(cfg, e)
	}
	for _, kv := range [][2]string{{"sentiment.enabled", "false"}, {"sentiment.frustration_trigger_escalate", "true"}, {"budgets.daily_usd", "14"}, {"budgets.monthly_usd", "120"}, {"budgets.hard_limit", "true"}, {"budgets.soft_limit", "false"}} {
		out, code = callCommand(t, []string{"config", "set", kv[0], kv[1]}, main)
		if code != 0 || !strings.Contains(out, "Config updated:") {
			t.Fatal(code, out)
		}
	}
	cfg, _ = config.LoadEscalationConfig()
	if cfg.Sentiment.Enabled || !cfg.Sentiment.FrustrationTriggerEscalate || cfg.Budgets.DailyUSD != 14 || cfg.Budgets.MonthlyUSD != 120 || !cfg.Budgets.HardLimit || cfg.Budgets.SoftLimit {
		t.Fatal(cfg)
	}
	for _, args := range [][]string{{"config"}, {"config", "unknown"}, {"monitor"}, {"help"}, {"unknown"}, nil, {"set-budget"}} {
		out, code = callCommand(t, args, main)
		if out == "" {
			t.Fatal(args)
		}
		if len(args) == 0 || args[0] == "unknown" || args[0] == "set-budget" {
			if code != 1 {
				t.Fatal(args, code)
			}
		} else if code != 0 {
			t.Fatal(args, code)
		}
	}
	out, _ = callCommand(t, nil, func() { setConfigValue(cfg, "bad", "true") })
	if !strings.Contains(out, "Invalid key format") {
		t.Fatal(out)
	}
}
func TestDashboardCommandDispatchAndErrors(t *testing.T) {
	commandFixture(t)
	old := http.DefaultClient
	defer func() { http.DefaultClient = old }()
	for _, status := range []int{200, 503} {
		for _, view := range []string{"--sentiment", "--budget", "--optimization", "--server", ""} {
			var paths []string
			http.DefaultClient = &http.Client{Transport: dashboardRT(func(r *http.Request) (*http.Response, error) {
				paths = append(paths, r.URL.Path)
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}, nil
			})}
			args := []string{"dashboard"}
			if view != "" {
				args = append(args, view)
			}
			if view == "--server" {
				args = append(args, "http://fixture.invalid")
			}
			out, code := callCommand(t, args, main)
			if len(paths) == 0 {
				t.Fatal("dashboard not called")
			}
			if status == 200 {
				if code != 0 || out == "" {
					t.Fatal(code, out)
				}
			} else if code != 1 || !strings.Contains(out, "Error:") {
				t.Fatal(code, out)
			}
		}
	}
}
func TestConfigurationReadWriteErrors(t *testing.T) {
	home := commandFixture(t)
	path := filepath.Join(home, ".claude", "escalation", "config.yaml")
	os.MkdirAll(filepath.Dir(path), 0700)
	os.WriteFile(path, []byte("budgets: ["), 0600)
	for _, args := range [][]string{{"set-budget", "--daily", "10"}, {"config"}, {"config", "set", "sentiment.enabled", "true"}} {
		out, code := callCommand(t, args, main)
		if code != 1 || !strings.Contains(out, "Error loading config:") {
			t.Fatal(code, out)
		}
	}
	os.Remove(path)
	os.Mkdir(path, 0700)
	for _, args := range [][]string{{"set-budget", "--daily", "10"}, {"config", "set", "sentiment.enabled", "true"}} {
		out, code := callCommand(t, args, main)
		if code != 1 || out == "" {
			t.Fatal(code, out)
		}
	}
}
