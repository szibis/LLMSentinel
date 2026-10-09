package main

import (
	"github.com/szibis/claude-escalate/internal/discovery"
	"io"
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

func TestToolDiscoveryPresentation(t *testing.T) {
	commandFixture(t)
	old := detectCommandTools
	defer func() { detectCommandTools = old }()
	for _, available := range []bool{false, true} {
		tools := &discovery.DetectedTools{LSPServers: map[string]string{}}
		if available {
			tools.RTKPath = "/fixture/rtk"
			tools.ScraplingPath = "/fixture/scrapling"
			tools.GitPath = "/fixture/git"
			tools.LSPServers["go"] = "/fixture/gopls"
		}
		detectCommandTools = func() *discovery.DetectedTools { return tools }
		for _, args := range [][]string{{"discover", "-v"}, {"status"}} {
			out, _, code := callCommand(t, args, "", main)
			if code != 0 {
				t.Fatal(code)
			}
			want := "Not found"
			if args[0] == "status" {
				want = "MISSING"
			}
			if available {
				want = "/fixture/rtk"
			}
			if !strings.Contains(out, want) {
				t.Fatal(out)
			}
		}
	}
}
func TestToolCommandValidation(t *testing.T) {
	home := commandFixture(t)
	path := filepath.Join(home, "tool")
	os.WriteFile(path, []byte("#!/bin/sh"), 0600)
	if err := os.Chmod(path, 0700); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{nil, {"help"}, {"unknown"}, {"validate", "--config", path}, {"validate", "--config", path + "-missing"}, {"config"}, {"config", "--add-tool"}, {"config", "--add-tool", "--name", "fixture"}, {"config", "--add-tool", "--name", "fixture", "--type", "cli"}, {"config", "--add-tool", "--name", "fixture", "--type", "cli", "--path", path + "-missing"}, {"config", "--add-tool", "--name", "fixture", "--type", "cli", "--path", path}} {
		out, errOut, code := callCommand(t, args, "", main)
		bad := len(args) > 0 && (args[0] == "unknown" || (args[0] == "validate" && strings.HasSuffix(args[len(args)-1], "-missing")) || (args[0] == "config" && len(args) > 1 && args[len(args)-1] != path))
		if bad {
			if code != 1 || out+errOut == "" {
				t.Fatal(args, code, out, errOut)
			}
		} else if code != 0 {
			t.Fatal(args, code, out, errOut)
		}
	}
}
