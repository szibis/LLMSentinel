package taskquality

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type cancelTaskWriter struct {
	cancel context.CancelFunc
	starts int
}

func (w *cancelTaskWriter) Write(raw []byte) (int, error) {
	if strings.Contains(string(raw), "Checking real ") {
		w.starts++
		w.cancel()
	}
	return len(raw), nil
}

func TestCLIContextCanceledPreflightSendsNoHTTP(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); t.Error("canceled command sent HTTP") }))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if code := RunCLIContext(ctx, []string{"--root", canonicalRoot(t), "--endpoint", server.URL}, nil, io.Discard, io.Discard); code != 1 || calls.Load() != 0 {
		t.Fatal("canceled preflight continued", code, calls.Load())
	}
}

func TestCLIContextCancellationStopsHealthAndNextTasks(t *testing.T) {
	t.Run("health", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		seen := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(seen); <-r.Context().Done() }))
		defer server.Close()
		done := make(chan int, 1)
		root := canonicalRoot(t)
		go func() {
			done <- RunCLIContext(ctx, []string{"--root", root, "--endpoint", server.URL}, nil, io.Discard, io.Discard)
		}()
		select {
		case <-seen:
		case <-time.After(time.Second):
			t.Fatal("preflight not started")
		}
		cancel()
		select {
		case code := <-done:
			if code != 1 {
				t.Fatal(code)
			}
		case <-time.After(time.Second):
			t.Fatal("health ignored parent cancellation")
		}
	})
	t.Run("tasks", func(t *testing.T) {
		root := installedFixtureClients(t)
		previous := Report{Version: 1, Scope: "real-cli-task-probes", FinishedAt: time.Now().UTC(), Results: []Result{{Task: "prior-evidence", Passed: true}}}
		if err := Save(root, previous); err != nil {
			t.Fatal(err)
		}
		before, err := os.ReadFile(filepath.Join(root, cliReportFile))
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/health" {
				t.Error("unexpected inference")
			}
			io.WriteString(w, `{"mode":"serving","controls":{"policy":"local-only","startup_billing_opt_in":false}}`)
		}))
		defer server.Close()
		out := &cancelTaskWriter{cancel: cancel}
		code := RunCLIContext(ctx, []string{"--root", root, "--endpoint", server.URL, "--role", "all", "--client", "both", "--suite", "extended"}, nil, out, io.Discard)
		after, err := os.ReadFile(filepath.Join(root, cliReportFile))
		if err != nil || code != 1 || out.starts != 1 || string(after) != string(before) {
			t.Fatalf("canceled run kept starting tasks: code=%d starts=%d", code, out.starts)
		}
	})
}

func TestCLIRunnerFilesystemAndIndependentVerifierFailures(t *testing.T) {
	for _, kind := range []string{"artifact-file", "artifact-symlink", "temp-missing", "coding-no-read", "go-unavailable", "go-failed"} {
		t.Run(kind, func(t *testing.T) {
			root := installedFixtureClients(t)
			f := fixtures("marker")[0]
			switch kind {
			case "artifact-file":
				if err := os.WriteFile(filepath.Join(root, "task-cli-runs"), []byte("blocked"), 0600); err != nil {
					t.Fatal(err)
				}
			case "artifact-symlink":
				if err := os.Symlink(t.TempDir(), filepath.Join(root, "task-cli-runs")); err != nil {
					t.Fatal(err)
				}
			case "temp-missing":
				t.Setenv("TMPDIR", filepath.Join(root, "missing"))
			case "coding-no-read":
				f = fixtures("marker")[1]
				if err := os.WriteFile(filepath.Join(root, "clients", "node_modules", ".bin", "mode"), []byte("no-read"), 0600); err != nil {
					t.Fatal(err)
				}
			case "go-unavailable":
				f = fixtures("marker")[1]
				t.Setenv("PATH", root)
			case "go-failed":
				f = fixtures("marker")[1]
				path := filepath.Join(root, "go")
				if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf 'synthetic verifier rejection' >&2\nexit 1\n"), 0700); err != nil { // #nosec G306 -- owner-only executable synthetic subprocess fixture under t.TempDir; fixed test content, no real clients or downloads.
					t.Fatal(err)
				}
				t.Setenv("PATH", root)
			}
			r := executeCLI(context.Background(), root, "http://127.0.0.1:1", "sonnet", "claude", f, 3*time.Second)
			if r.Workspace != "" {
				t.Cleanup(func() { os.RemoveAll(r.Workspace) })
			}
			if r.Passed || r.Failure == "" {
				t.Fatal("runner accepted failure", kind, r)
			}
			if kind == "coding-no-read" && r.Failure != "CLI coding evidence incomplete" {
				t.Fatal(r.Failure)
			}
			if kind == "go-unavailable" && r.Failure != "Go verifier unavailable" {
				t.Fatal(r.Failure)
			}
			if kind == "go-failed" && !strings.Contains(r.Failure, "independent Go tests failed: synthetic verifier rejection") {
				t.Fatal(r.Failure)
			}
		})
	}
}

func TestCLICommandExplicitClientsDirectoryAndPublicationFailure(t *testing.T) {
	root := installedFixtureClients(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Error("unexpected inference")
		}
		io.WriteString(w, `{"mode":"serving","controls":{"policy":"local-only","startup_billing_opt_in":false}}`)
	}))
	defer server.Close()
	args := []string{"--root", root, "--endpoint", server.URL, "--clients-root", filepath.Join(root, "clients"), "--task", "exact-read", "--client", "codex"}
	if code := RunCLI(args, nil, io.Discard, io.Discard); code != 0 {
		t.Fatal("explicit fixture clients failed", code)
	}
	report := LoadCLI(root)
	if report == nil || len(report.Results) != 1 || !report.Results[0].Passed {
		t.Fatal(report)
	}
	if err := os.RemoveAll(filepath.Join(root, "task-quality-runs")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "task-quality-runs"), []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	if code := RunCLI(args, nil, io.Discard, io.Discard); code != 1 {
		t.Fatal("failed report publication accepted", code)
	}
}

func TestQualityCommandBothProtocolsSuccessfulConversation(t *testing.T) {
	root := canonicalRoot(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			io.WriteString(w, `{"mode":"serving","controls":{"policy":"local-only","startup_billing_opt_in":false}}`)
			return
		}
		var req object
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			http.Error(w, "invalid request", 400)
			return
		}
		if r.URL.Path == "/v1/messages" {
			history := array(req["messages"])
			blocks := []any{object{"type": "tool_use", "id": "read", "name": "read_fixture", "input": object{"path": "proof.txt"}}}
			stop := "tool_use"
			if len(history) > 1 {
				last := obj(history[len(history)-1])
				result := obj(array(last["content"])[0])
				blocks = []any{object{"type": "text", "text": result["content"]}}
				stop = "end_turn"
			}
			json.NewEncoder(w).Encode(object{"content": blocks, "stop_reason": stop, "usage": object{"input_tokens": 10, "output_tokens": 2}})
		} else {
			history := array(req["input"])
			blocks := []any{object{"type": "function_call", "call_id": "read", "name": "read_fixture", "arguments": `{"path":"proof.txt"}`}}
			if len(history) > 1 {
				last := obj(history[len(history)-1])
				blocks = []any{object{"type": "message", "role": "assistant", "content": []any{object{"type": "output_text", "text": last["output"]}}}}
			}
			json.NewEncoder(w).Encode(object{"output": blocks, "status": "completed", "usage": object{"input_tokens": 10, "output_tokens": 2}})
		}
	}))
	defer server.Close()
	if code := Run([]string{"--root", root, "--endpoint", server.URL, "--task", "exact-read"}, nil, io.Discard, io.Discard); code != 0 {
		t.Fatal("local two-protocol quality failed", code)
	}
	report := Load(root)
	if report == nil || len(report.Results) != 2 {
		t.Fatal(report)
	}
	for _, result := range report.Results {
		if !result.Passed || result.Requests != 2 || result.ToolCalls != 1 || result.InputTokens == nil || *result.InputTokens != 20 {
			t.Fatal("lost real continuation evidence", result)
		}
	}
}

func TestNativeEvidenceOversizeFailuresAndReadBoundaries(t *testing.T) {
	for _, client := range []string{"claude", "codex"} {
		raw := jsonLines(object{"type": "result", "subtype": "failed", "is_error": true, "result": strings.Repeat("x", 400)})
		if client == "codex" {
			raw = jsonLines(object{"type": "turn.failed", "error": object{"message": strings.Repeat("x", 400)}})
		}
		if _, err := decodeCLI(client, raw, "/fixture", fixtures("marker")[0]); err == nil || len(err.Error()) > 400 {
			t.Fatal("unbounded/accepted failure", err)
		}
		if _, err := decodeCLI(client, []byte(strings.Repeat("x", (1<<20)+1)), "/fixture", fixtures("marker")[0]); err == nil {
			t.Fatal("oversized event accepted")
		}
	}
	if fixturePath("", "/fixture", fixtures("marker")[0]) || successfulTestOutput("FAIL fixture\nok unrelated 0.1s") {
		t.Fatal("invented proof")
	}
	if unwrapShell("sh -lc cat proof.txt") != "cat proof.txt" {
		t.Fatal("unquoted shell decoding")
	}
}

func FuzzExtendedNativeEvidence(f *testing.F) {
	f.Add(`{"type":"turn.completed"}`, uint8(0))
	f.Add(`{"type":"result","subtype":"success","is_error":false,"result":"forged"}`, uint8(1))
	corpus, _ := fixtureSuite("marker", "extended")
	for i, fixture := range corpus[4:] {
		raw := string(jsonLines(object{"type": "item.completed", "item": object{"id": "read", "type": "command_execution", "command": "cat " + fixture.Path, "status": "completed", "exit_code": 0, "aggregated_output": fixture.Content}}, object{"type": "item.completed", "item": object{"id": "answer", "type": "agent_message", "text": fixture.Expected}}, object{"type": "turn.completed"}))
		f.Add(raw, uint8(i*2))
	}
	f.Fuzz(func(t *testing.T, raw string, kind uint8) {
		if len(raw) > 65536 {
			t.Skip()
		}
		fixture := corpus[4+int(kind/2)%4]
		client := []string{"codex", "claude"}[kind%2]
		e, err := decodeCLI(client, []byte(raw), "/fixture", fixture)
		if err == nil && cliFinalPasses(fixture, e) {
			if !e.Read || !matches(fixture, e.Final) || (fixture.ID == "tool-recovery" && !e.FailedRead) {
				t.Fatal("passed without independent fixture proof")
			}
		}
	})
}
