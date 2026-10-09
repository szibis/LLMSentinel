package main

import (
	"context"
	"errors"
	"flag"
	"github.com/szibis/claude-escalate/internal/config"
	"github.com/szibis/claude-escalate/internal/gateway"
	"github.com/szibis/claude-escalate/internal/intent"
	"io"
	"os"
	"path/filepath"
	"reflect"
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
func callCommand(t *testing.T, args []string, fn func()) (string, string, int) {
	t.Helper()
	oldFlags := flag.CommandLine
	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	defer func() { flag.CommandLine = oldFlags }()
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
	e, _ := io.ReadAll(errOut)
	return string(b), string(e), code
}

type fixtureAdapter struct {
	response *gateway.ToolResponse
	err      error
	req      *gateway.ToolRequest
}

func (a *fixtureAdapter) Name() string                         { return "fixture" }
func (a *fixtureAdapter) Type() gateway.ToolType               { return gateway.ToolType("cli") }
func (a *fixtureAdapter) GetSignature() *gateway.ToolSignature { return nil }
func (a *fixtureAdapter) Health(context.Context) error         { return nil }
func (a *fixtureAdapter) Close() error                         { return nil }
func (a *fixtureAdapter) Execute(_ context.Context, r *gateway.ToolRequest) (*gateway.ToolResponse, error) {
	a.req = r
	return a.response, a.err
}

type fixtureFactory struct {
	adapter   *fixtureAdapter
	createErr error
	missing   map[string]bool
	closed    bool
}

func (f *fixtureFactory) CreateFromConfig(*config.Config) error { return f.createErr }
func (f *fixtureFactory) GetAdapter(name string) (gateway.ToolAdapter, error) {
	if f.missing[name] {
		return nil, errors.New("missing adapter")
	}
	return f.adapter, nil
}
func (f *fixtureFactory) Close() error { f.closed = true; return nil }
func TestCLIParameterContracts(t *testing.T) {
	got := parseToolParams([]string{"query", "--limit", "10", "--fresh", "-v", "tail"})
	want := map[string]interface{}{"query": "query", "limit": "10", "fresh": true, "v": true, "arg5": "tail"}
	if !reflect.DeepEqual(got, want) {
		t.Fatal(got)
	}
}
func TestCLIExecutionAndFormatting(t *testing.T) {
	commandFixture(t)
	oldF, oldC := newCommandFactory, classifyCommandIntent
	defer func() { newCommandFactory, classifyCommandIntent = oldF, oldC }()
	classifyCommandIntent = func(_ context.Context, q, u string, c *intent.QueryContext) *intent.IntentDecision {
		if q == "" || u == "" || c.UserSessionID == "" {
			t.Fatal("classification context missing")
		}
		return &intent.IntentDecision{Explanation: "fixture"}
	}
	for _, data := range []interface{}{map[string]string{"output": "fixture result"}, map[string]interface{}{"value": "fixture result"}, "fixture result"} {
		a := &fixtureAdapter{response: &gateway.ToolResponse{Success: true, Data: data}}
		f := &fixtureFactory{adapter: a, missing: map[string]bool{"git": true}}
		newCommandFactory = func() commandFactory { return f }
		out, _, code := callCommand(t, []string{"--fresh", "-v", "git", "status"}, main)
		if code != 0 || !strings.Contains(out, "fixture result") || !f.closed || !a.req.NoCacheBypassed || a.req.Params["command"] != "git status" {
			t.Fatal(code, out, a.req, f.closed)
		}
	}
	a := &fixtureAdapter{response: &gateway.ToolResponse{Success: true, Data: "ok"}}
	f := &fixtureFactory{adapter: a}
	newCommandFactory = func() commandFactory { return f }
	out, _, code := callCommand(t, []string{"fixture", "search", "--limit", "2"}, main)
	if code != 0 || out != "ok\n" || a.req.Params["query"] != "search" || a.req.Params["limit"] != "2" {
		t.Fatal(code, out, a.req)
	}
}
func TestCLIErrorBoundaries(t *testing.T) {
	commandFixture(t)
	oldF, oldC := newCommandFactory, classifyCommandIntent
	defer func() { newCommandFactory, classifyCommandIntent = oldF, oldC }()
	classifyCommandIntent = func(context.Context, string, string, *intent.QueryContext) *intent.IntentDecision {
		return &intent.IntentDecision{}
	}
	a := &fixtureAdapter{response: &gateway.ToolResponse{Success: true}}
	f := &fixtureFactory{adapter: a}
	newCommandFactory = func() commandFactory { return f }
	for _, args := range [][]string{nil, {"--config", "missing.yaml", "fixture"}} {
		_, errOut, code := callCommand(t, args, main)
		if code != 1 || errOut == "" {
			t.Fatal(code, errOut)
		}
	}
	for _, failure := range []string{"create", "missing", "fallback", "execute", "unsuccessful"} {
		a.err = nil
		a.response = &gateway.ToolResponse{Success: true}
		f.createErr = nil
		f.missing = map[string]bool{}
		args := []string{"fixture"}
		switch failure {
		case "create":
			f.createErr = errors.New("fixture create failure")
		case "missing":
			f.missing["fixture"] = true
		case "fallback":
			args = []string{"git"}
			f.missing["git"] = true
			f.missing["cli"] = true
		case "execute":
			a.err = errors.New("fixture execution failure")
		case "unsuccessful":
			a.response = &gateway.ToolResponse{Error: "fixture failure", Data: map[string]interface{}{"stderr": "fixture stderr"}}
		}
		_, errOut, code := callCommand(t, args, main)
		if code != 1 || errOut == "" {
			t.Fatal(failure, code, errOut)
		}
	}
}
