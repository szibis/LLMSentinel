package taskquality

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A subprocess fixture tests process/config/report behavior; this is never
// counted as native model evidence. Production evidence uses installed CLIs.
func TestMain(m *testing.M) {
	modeRaw, _ := os.ReadFile(filepath.Join(filepath.Dir(os.Args[0]), "mode")) // #nosec G703 -- test binary and copied synthetic clients are executed only from isolated test directories; fixed mode filename.
	mode := strings.TrimSpace(string(modeRaw))
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		if mode == "version-error" {
			os.Exit(1)
		}
		fmt.Println("synthetic-test-client 1")
		os.Exit(0)
	}
	if len(os.Args) > 1 && (os.Args[1] == "--bare" || os.Args[1] == "exec") {
		if mode == "deadline" {
			time.Sleep(time.Minute)
		}
		if mode == "process-error" {
			fmt.Fprintln(os.Stderr, "synthetic failure")
			os.Exit(1)
		}
		if mode == "malformed" {
			fmt.Println("broken-json")
			os.Exit(0)
		}
		client := "claude"
		if os.Args[1] == "exec" {
			client = "codex"
		}
		path := "proof.txt"
		result := ""
		if raw, err := os.ReadFile(path); err == nil {
			result = string(raw)
		} else if raw, err := os.ReadFile("add.go"); err == nil {
			path = "add.go"
			result = "FIXED_AND_TESTED"
			_ = os.WriteFile(path, []byte("package fixture\nfunc Add(a,b int) int {return a+b}\n"), 0600)
			_ = raw
		} else {
			fmt.Fprintln(os.Stderr, "fixture missing")
			os.Exit(1)
		}
		content, _ := os.ReadFile(path)
		if mode == "wrong-answer" {
			result = "WRONG"
		}
		if mode == "no-read" {
			content = []byte("unrelated")
		}
		if mode == "tampered-tests" {
			_ = os.WriteFile("add_test.go", []byte("package fixture"), 0600)
		}
		if mode == "unsafe-code" {
			_ = os.WriteFile("add.go", []byte("package fixture\nimport \"os\"\nfunc Add(a,b int) int {os.Exit(0);return a+b}"), 0600)
		}
		var events []any
		if client == "claude" {
			events = []any{
				object{"type": "assistant", "message": object{"content": []any{object{"type": "tool_use", "id": "read", "name": "Read", "input": object{"file_path": path}}, object{"type": "tool_use", "id": "test", "name": "Bash", "input": object{"command": "go test -timeout 10s ./..."}}}}},
				object{"type": "user", "message": object{"content": []any{object{"type": "tool_result", "tool_use_id": "read", "content": string(content)}, object{"type": "tool_result", "tool_use_id": "test", "content": "ok fixture 0.1s"}}}},
				object{"type": "result", "subtype": "success", "is_error": false, "result": result, "usage": object{"input_tokens": 10, "output_tokens": 5}},
			}
		} else {
			events = []any{
				object{"type": "item.completed", "item": object{"id": "read", "type": "command_execution", "command": "cat " + path, "status": "completed", "exit_code": 0, "aggregated_output": string(content)}},
				object{"type": "item.completed", "item": object{"id": "test", "type": "command_execution", "command": "go test -timeout 10s ./...", "status": "completed", "exit_code": 0, "aggregated_output": "ok fixture 0.1s"}},
				object{"type": "item.completed", "item": object{"id": "answer", "type": "agent_message", "text": result}},
				object{"type": "turn.completed", "usage": object{"input_tokens": 10, "output_tokens": 5}},
			}
		}
		_, _ = os.Stdout.Write(jsonLines(events...))
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestSyntheticCLIProcessFailuresRemainFailures(t *testing.T) {
	root := installedFixtureClients(t)
	f := fixtures("marker")[0]
	for _, mode := range []string{"version-error", "deadline", "process-error", "malformed", "wrong-answer", "no-read", "tampered-tests", "unsafe-code"} {
		if err := os.WriteFile(filepath.Join(root, "clients", "node_modules", ".bin", "mode"), []byte(mode), 0600); err != nil {
			t.Fatal(err)
		}
		selected := f
		if mode == "tampered-tests" || mode == "unsafe-code" {
			selected = fixtures("marker")[1]
		}
		timeout := time.Second
		if mode == "deadline" {
			timeout = 100 * time.Millisecond
		}
		result := executeCLI(context.Background(), root, "http://127.0.0.1:19090", "sonnet", "claude", selected, timeout)
		if result.Passed || result.Failure == "" {
			t.Fatalf("%s accepted: %+v", mode, result)
		}
	}
}

func TestQualityCommandRejectsUnsafeHealthAndFilesystem(t *testing.T) {
	root := canonicalRoot(t)
	for _, health := range []string{`{"mode":"serving","controls":{"policy":"hybrid","startup_billing_opt_in":true}}`, `not-json`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = fmt.Fprint(w, health) }))
		for _, entry := range []func([]string, io.Reader, io.Writer, io.Writer) int{Run, RunCLI} {
			if code := entry([]string{"--root", root, "--endpoint", server.URL}, nil, &bytes.Buffer{}, &bytes.Buffer{}); code != 1 {
				t.Fatal("unsafe health accepted", code)
			}
		}
		server.Close()
	}
	for _, entry := range []func([]string, io.Reader, io.Writer, io.Writer) int{Run, RunCLI} {
		if code := entry([]string{"--root", filepath.Join(root, "missing"), "--endpoint", "http://127.0.0.1:1"}, nil, &bytes.Buffer{}, &bytes.Buffer{}); code != 2 {
			t.Fatal("missing root accepted", code)
		}
		if code := entry([]string{"--root", root, "--endpoint", "http://127.0.0.1:1"}, nil, &bytes.Buffer{}, &bytes.Buffer{}); code != 1 {
			t.Fatal("offline endpoint", code)
		}
	}
}

func installedFixtureClients(t *testing.T) string {
	t.Helper()
	root := canonicalRoot(t)
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "clients", "node_modules", ".bin")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, client := range []string{"claude", "codex"} {
		if err := os.WriteFile(filepath.Join(dir, client), data, 0700); err != nil { // #nosec G306 G703 -- owner-only copy of the running test executable in t.TempDir; client is a fixed claude/codex test literal, never an external path.
			t.Fatal(err)
		}
	}
	return root
}

func TestCLICommandSyntheticSubprocessContracts(t *testing.T) {
	root := installedFixtureClients(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("unexpected inference from subprocess fixture: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(object{"mode": "serving", "controls": object{"policy": "local-only", "startup_billing_opt_in": false}})
	}))
	defer server.Close()
	for _, task := range []string{"exact-read", "coding-fix"} {
		var out, diagnostic bytes.Buffer
		code := RunCLI([]string{"--root", root, "--endpoint", server.URL, "--role", "all", "--client", "both", "--task", task}, nil, &out, &diagnostic)
		if code != 0 {
			t.Fatalf("synthetic CLI process flow: %d %s %s", code, out.String(), diagnostic.String())
		}
		report := LoadCLI(root)
		if report == nil || len(report.Results) != 6 || report.FixtureVersion != fixtureVersion || report.Suite != "baseline" {
			t.Fatalf("report lost: %+v", report)
		}
		for _, r := range report.Results {
			if !r.Passed || r.ClientVersion != "synthetic-test-client 1" || !r.Checks["fixture_read"] {
				t.Fatalf("subprocess assertion: %+v", r)
			}
		}
	}
	for _, args := range [][]string{{"--role", "invalid"}, {"--timeout", "0"}, {"--endpoint", "https://example.com"}, {"--root", root, "--endpoint", server.URL, "--suite", "invalid"}, {"--root", root, "--endpoint", server.URL, "--task", "invalid"}} {
		var diagnostic bytes.Buffer
		if RunCLI(args, nil, &bytes.Buffer{}, &diagnostic) != 2 {
			t.Fatal("invalid options accepted", args)
		}
	}
}

func TestEvidenceHelpersBoundaries(t *testing.T) {
	buffer := &cappedBuffer{Limit: 3}
	if n, err := buffer.Write([]byte("ab")); n != 2 || err != nil {
		t.Fatal(n, err)
	}
	_, _ = buffer.Write([]byte("cde"))
	_, _ = buffer.Write([]byte("f"))
	if !buffer.Overflow || buffer.String() != "abc" {
		t.Fatal(buffer)
	}
	if textContent([]any{object{"type": "text", "text": "one"}, object{"type": "other", "text": "ignore"}, object{"type": "text", "text": "two"}}) != "onetwo" {
		t.Fatal("text blocks")
	}
	for _, command := range []string{"cat -- proof.txt", "head -n 1 proof.txt", "sed -n '1,2p' proof.txt", "sh -lc \"cat proof.txt\"", "bash -lc 'cat proof.txt'"} {
		if !commandRead(command, "/private/tmp/fixture", fixtures("x")[0]) {
			t.Error(command)
		}
	}
	for _, command := range []string{"cat", "cat --", "head -n 0 proof.txt", "head -n bad proof.txt", "sed -n bad proof.txt", "cat -n proof.txt", "cat ''", "cat 'proof.txt' '*'"} {
		if commandRead(command, "/private/tmp/fixture", fixtures("x")[0]) {
			t.Error(command)
		}
	}
	for _, raw := range []string{"null", "{} {}", `{"a":{"b":1,"b":2}}`, strings.Repeat("[", 70) + "0" + strings.Repeat("]", 70)} {
		var obj object
		if strictJSON([]byte(raw), &obj) == nil && raw != "null" {
			t.Error("ambiguous JSON", raw)
		}
	}
}
