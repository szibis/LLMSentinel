package taskquality

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPublishBoundsAndFilesystemFailures(t *testing.T) {
	report := Report{Version: 1, Scope: "bounded-api-task-probes", FinishedAt: time.Now().UTC(), Results: []Result{{Task: "exact-read", Passed: true}}}
	for _, kind := range []string{"scope", "invalid-json", "archive-file", "archive-symlink", "latest-directory", "summary-limit", "archive-limit"} {
		t.Run(kind, func(t *testing.T) {
			root := canonicalRoot(t)
			r := report
			r.Results = append([]Result(nil), report.Results...)
			switch kind {
			case "scope":
				r.Scope = "untrusted"
			case "invalid-json":
				r.RuntimeSnapshot = json.RawMessage("{")
			case "archive-file":
				if err := os.WriteFile(filepath.Join(root, "task-quality-runs"), []byte("block"), 0600); err != nil {
					t.Fatal(err)
				}
			case "archive-symlink":
				if err := os.Symlink(t.TempDir(), filepath.Join(root, "task-quality-runs")); err != nil {
					t.Fatal(err)
				}
			case "latest-directory":
				if err := os.Mkdir(filepath.Join(root, reportFile), 0700); err != nil {
					t.Fatal(err)
				}
			case "summary-limit":
				r.Results[0].Failure = strings.Repeat("x", (1<<20)+1)
			case "archive-limit":
				r.Results[0].Final = strings.Repeat("x", (64<<20)+1)
			}
			if Save(root, r) == nil {
				t.Fatal("invalid publication succeeded", kind)
			}
			if Load(root) != nil {
				t.Fatal("failed publication invented latest")
			}
			files, _ := filepath.Glob(filepath.Join(root, ".quality-*"))
			if len(files) != 0 {
				t.Fatal("temporary summary leaked", files)
			}
		})
	}
}

func TestLoadRejectsOversizeAndNonreportEvidence(t *testing.T) {
	root := canonicalRoot(t)
	for _, raw := range []string{"not-json", `{"version":2}`, strings.Repeat("x", (1<<20)+1)} {
		if err := os.WriteFile(filepath.Join(root, reportFile), []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if Load(root) != nil {
			t.Fatal("invalid report accepted")
		}
	}
	if err := os.Remove(filepath.Join(root, reportFile)); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, reportFile), 0700); err != nil {
		t.Fatal(err)
	}
	if Load(root) != nil {
		t.Fatal("directory accepted")
	}
}

func TestCodeContractRejectsUnsafeAndChangedFixtureFiles(t *testing.T) {
	for _, source := range []string{
		"package fixture\nfunc Add(a,b int) int {return +a-(-b)}",
		"package fixture\nvar Add=42",
		"package fixture\nfunc Sum(a,b int) int {return a+b}",
		"package fixture\nfunc Add(a int,b int) int {return a+b}",
		"package fixture\nfunc Add(x,b int) int {return x+b}",
		"package fixture\nfunc Add(a,b int64) int {return 0}",
		"package fixture\nfunc Add(a,b int) *int {return nil}",
		"package fixture\nfunc Add(a,b int) (out int) {return a+b}",
		"package fixture\nfunc Add(a,b int) int {panic(\"no\")}",
		"package fixture\nfunc Add(a,b int) int {return a*b}",
		"package fixture\nfunc Add(a,b int) int {return ~a+b}",
		"package fixture\nfunc Add(a,b int) int {return -a+b}",
	} {
		root := canonicalRoot(t)
		for name, raw := range codeFiles() {
			if err := os.WriteFile(filepath.Join(root, name), []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.WriteFile(filepath.Join(root, "add.go"), []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
		err := verifyCode(root)
		if strings.Contains(source, "+a-(-b)") {
			if err != nil {
				t.Fatal("equivalent safe addition rejected", err)
			}
		} else if err == nil {
			t.Fatal("unsafe contract accepted", source)
		}
	}
	root := canonicalRoot(t)
	for _, kind := range []string{"missing", "directory", "symlink", "oversize"} {
		path := filepath.Join(root, kind)
		switch kind {
		case "directory":
			os.Mkdir(path, 0700)
		case "symlink":
			os.Symlink(filepath.Join(root, "missing"), path)
		case "oversize":
			os.WriteFile(path, bytes.Repeat([]byte("x"), (64<<10)+1), 0600)
		}
		if _, err := boundedFile(path); err == nil {
			t.Fatal(kind)
		}
	}
}

func TestPrepareCLIFilesystemFailures(t *testing.T) {
	f := fixtures("marker")[0]
	for _, kind := range []string{"run-file", "claude-settings", "codex-catalog", "codex-config", "missing-workspace", "coding-workspace"} {
		root := canonicalRoot(t)
		run := filepath.Join(root, "run")
		workspace := filepath.Join(root, "workspace")
		os.Mkdir(workspace, 0700)
		client := "claude"
		switch kind {
		case "run-file":
			os.WriteFile(run, []byte("blocked"), 0600)
		case "claude-settings":
			os.MkdirAll(filepath.Join(run, "claude", "settings.json"), 0700)
		case "codex-catalog":
			client = "codex"
			os.MkdirAll(filepath.Join(run, "codex", "model-catalog.json"), 0700)
		case "codex-config":
			client = "codex"
			os.MkdirAll(filepath.Join(run, "codex", "config.toml"), 0700)
		case "missing-workspace":
			workspace = filepath.Join(root, "missing")
		case "coding-workspace":
			f = fixtures("marker")[1]
			workspace = filepath.Join(root, "missing")
		}
		if prepareCLI(run, workspace, client, "http://127.0.0.1:1", f) == nil {
			t.Fatal(kind)
		}
	}
}

func TestInstalledClientRefusesNonexecutables(t *testing.T) {
	for _, kind := range []string{"directory", "nonexecutable"} {
		root := canonicalRoot(t)
		path := filepath.Join(root, "node_modules", ".bin", "claude")
		os.MkdirAll(filepath.Dir(path), 0700)
		if kind == "directory" {
			os.Mkdir(path, 0700)
		} else {
			os.WriteFile(path, []byte("data"), 0600)
		}
		if _, err := installedClientDirectory(root, "claude"); err == nil {
			t.Fatal(kind)
		}
	}
}

func TestQualityCommandsValidationAndSaveFailure(t *testing.T) {
	root := canonicalRoot(t)
	for _, entry := range []func([]string, io.Reader, io.Writer, io.Writer) int{Run, RunCLI} {
		for _, args := range [][]string{{"--unknown"}, {"extra"}, {"--root", root, "--suite", "missing"}, {"--root", root, "--task", "missing"}} {
			if entry(args, nil, io.Discard, io.Discard) != 2 {
				t.Fatal(args)
			}
		}
		link := filepath.Join(root, "link")
		os.Symlink(root, link)
		if entry([]string{"--root", link}, nil, io.Discard, io.Discard) != 2 {
			t.Fatal("symlink root")
		}
		os.Remove(link)
	}
	for _, path := range []string{filepath.Join(root, "missing"), filepath.Join(root, "file")} {
		os.WriteFile(filepath.Join(root, "file"), []byte("x"), 0600)
		if RunCLI([]string{"--root", root, "--clients-root", path}, nil, io.Discard, io.Discard) != 2 {
			t.Fatal("invalid clients root accepted")
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			io.WriteString(w, `{"mode":"serving","controls":{"policy":"local-only","startup_billing_opt_in":false}}`)
			return
		}
		http.Error(w, "synthetic rejection", http.StatusUnprocessableEntity)
	}))
	defer server.Close()
	os.WriteFile(filepath.Join(root, "task-quality-runs"), []byte("block"), 0600)
	if Run([]string{"--root", root, "--endpoint", server.URL, "--task", "exact-read"}, nil, io.Discard, io.Discard) != 1 {
		t.Fatal("publication failure ignored")
	}
}

func TestPostCancellationAndMalformedRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, endpoint := range []string{"http://127.0.0.1:1", "http://["} {
		if _, _, err := post(ctx, &http.Client{}, endpoint, object{}); err == nil {
			t.Fatal("invalid/canceled request succeeded")
		}
	}
}

func FuzzStrictEvidenceJSON(f *testing.F) {
	for _, raw := range []string{`{"a":1}`, `{"a":1,"a":2}`, `[1,{"nested":true}]`, `null`, `{"a":`, `{} {}`} {
		f.Add([]byte(raw))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		var result any
		err := DecodeEvidenceJSON(raw, &result)
		if err == nil {
			if !json.Valid(raw) {
				t.Fatal("accepted invalid JSON")
			}
			encoded, e := json.Marshal(result)
			if e != nil {
				t.Fatal(e)
			}
			var again any
			if DecodeEvidenceJSON(encoded, &again) != nil {
				t.Fatal("valid decoded evidence cannot round trip")
			}
		}
	})
}

func TestLoadRejectsSymlinkAndAmbiguousReport(t *testing.T) {
	root := canonicalRoot(t)
	raw := `{"version":1,"scope":"bounded-api-task-probes","finished_at":"2026-10-08T00:00:00Z","results":[{"task":"exact-read","passed":false,"passed":true}]}`
	if err := os.WriteFile(filepath.Join(root, reportFile), []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	if Load(root) != nil {
		t.Error("duplicate pass keys accepted as score evidence")
	}
	if err := os.Remove(filepath.Join(root, reportFile)); err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(canonicalRoot(t), "report.json")
	raw = `{"version":1,"scope":"bounded-api-task-probes","finished_at":"2026-10-08T00:00:00Z","results":[{"task":"exact-read","passed":true}]}`
	if err := os.WriteFile(external, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(root, reportFile)); err != nil {
		t.Fatal(err)
	}
	if Load(root) != nil {
		t.Error("report symlink outside isolated lab accepted")
	}
}
