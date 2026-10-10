package claudemod

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func canonicalRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func runSnapshot(t *testing.T, root, endpoint string) (map[string]any, string) {
	t.Helper()
	var out, diagnostic bytes.Buffer
	if code := Run([]string{"--root", root, "--endpoint", endpoint, "status"}, nil, &out, &diagnostic); code != 0 {
		t.Fatalf("snapshot returned %d: %s", code, &diagnostic)
	}
	var snapshot map[string]any
	if err := json.Unmarshal(out.Bytes(), &snapshot); err != nil {
		t.Fatalf("invalid snapshot: %v", err)
	}
	if diagnostic.Len() != 0 {
		t.Fatalf("component failures must remain unknown without private diagnostics: %s", &diagnostic)
	}
	return snapshot, out.String()
}

// Dropping unknown control fields, inventing absent values, querying another
// endpoint or declaring collected data promotable would break this contract.
func TestStatusPreservesControlUnknownsAndMissingQuality(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/sentinel/control" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		io.WriteString(w, `{"capture_enabled":false,"future_control":{"known":null},"counter":9007199254740993}`)
	}))
	defer server.Close()
	before := time.Now().UTC()
	snapshot, raw := runSnapshot(t, canonicalRoot(t), server.URL)
	if snapshot["scope"] != "claude-mod-status" || snapshot["endpoint"] != server.URL || snapshot["training_eligible"] != false || snapshot["promotion"] != "not_authorized" {
		t.Fatalf("incorrect snapshot envelope: %s", raw)
	}
	sampledAt, err := time.Parse(time.RFC3339Nano, snapshot["sampled_at"].(string))
	if err != nil || sampledAt.Before(before) || sampledAt.After(time.Now().UTC()) {
		t.Fatalf("invalid sampling timestamp: %v", snapshot["sampled_at"])
	}
	control, ok := snapshot["control"].(map[string]any)
	if !ok || control["capture_enabled"] != false || !reflect.DeepEqual(control["future_control"], map[string]any{"known": nil}) || !strings.Contains(raw, "9007199254740993") {
		t.Fatalf("control extensions or numeric precision lost: %s", raw)
	}
	if _, fabricated := control["policy"]; fabricated {
		t.Fatal("missing policy was fabricated")
	}
	for _, key := range []string{"telemetry", "cli_quality"} {
		if value, exists := snapshot[key]; !exists || value != nil {
			t.Fatalf("%s must explicitly be unknown: %s", key, raw)
		}
	}
}

// Surfacing failures as healthy or echoing response/transport error content
// would make these real loopback fixtures fail.
func TestStatusUnavailableControlRemainsUnknown(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"http-rejected", 500, "PRIVATE_RESPONSE /private/secret"},
		{"malformed", 200, "{PRIVATE_RESPONSE"},
		{"null", 200, "null"},
		{"array", 200, "[]"},
		{"trailing-json", 200, "{} {}"},
		{"oversized", 200, `{"private":"` + strings.Repeat("x", 512*1024) + `"}`},
		{"redirect", 302, "PRIVATE_RESPONSE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.status == 302 {
					w.Header().Set("Location", "http://127.0.0.1:1/private")
				}
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			}))
			defer server.Close()
			snapshot, raw := runSnapshot(t, canonicalRoot(t), server.URL)
			if snapshot["control"] != nil || strings.Contains(raw, "PRIVATE_RESPONSE") || strings.Contains(raw, "/private/secret") {
				t.Fatalf("control failure leaked or became healthy: %s", raw)
			}
		})
	}
	t.Run("connection-refused", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		endpoint := server.URL
		server.Close()
		snapshot, _ := runSnapshot(t, canonicalRoot(t), endpoint)
		if snapshot["control"] != nil {
			t.Fatal("offline endpoint became known")
		}
	})
}

// Accepting remote endpoints or normalizing untrusted paths would allow reads
// and network requests outside the explicitly selected canonical lab root.
func TestStatusRejectsInvalidConfigWithoutDisclosingIt(t *testing.T) {
	root := canonicalRoot(t)
	symlink := filepath.Join(root, "alias")
	if err := os.Symlink(root, symlink); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"--root", root, "--endpoint", "https://127.0.0.1:19090", "status"},
		{"--root", root, "--endpoint", "http://localhost:19090", "status"},
		{"--root", root, "--endpoint", "http://192.168.1.1:19090", "status"},
		{"--root", root, "--endpoint", "http://PRIVATE_CREDENTIAL@127.0.0.1:19090", "status"},
		{"--root", root, "--endpoint", "http://127.0.0.1:19090/PRIVATE_PATH", "status"},
		{"--root", root, "--endpoint", "http://127.0.0.1:65536", "status"},
		{"--root", root, "--endpoint", "http://127.0.0.1:19090?PRIVATE_QUERY", "status"},
		{"--root", "relative/PRIVATE_PATH", "status"},
		{"--root", root + "/../PRIVATE_PATH", "status"},
		{"--root", symlink, "status"},
		{"--root", symlink + "/missing", "status"},
		{"--root", root, "PRIVATE_ACTION"},
		{"--root", root, "status", "--endpoint", "http://127.0.0.1:1"},
		{"--root", root, "--PRIVATE_FLAG"},
		{"--root"},
		{"status"},
	} {
		var out, diagnostic bytes.Buffer
		code := Run(args, nil, &out, &diagnostic)
		if (code != 1 && code != 2) || out.Len() != 0 || diagnostic.Len() == 0 {
			t.Fatalf("invalid options accepted or no diagnostic: %q => %d %s %s", args, code, &out, &diagnostic)
		}
		if strings.Contains(diagnostic.String(), "PRIVATE_") || strings.Contains(diagnostic.String(), root) {
			t.Fatalf("private options disclosed: %s", &diagnostic)
		}
	}
}

// Counting a full assumed corpus or serializing report objects would leak
// private task evidence and misrepresent the tested subset.
func TestStatusSummarizesOnlyStoredCLICasesAndSupportedFailures(t *testing.T) {
	root := canonicalRoot(t)
	report := `{
 "version":1,"scope":"real-cli-task-probes","suite":"extended",
 "finished_at":"2026-10-09T07:42:00Z","corpus_sha256":"0123456789abcdef",
 "endpoint":"http://127.0.0.1:19090","gateway_health":{"private":"PRIVATE_HEALTH"},
 "runtime_snapshot":{"private":"PRIVATE_RUNTIME"},"future_report":"PRIVATE_REPORT",
 "results":[
  {"task":"exact-read","role":"haiku","client":"claude","passed":true,"final":"PRIVATE_FINAL","checks":{"fixture_read":true}},
  {"task":"coding-fix","role":"sonnet","client":"codex","passed":false,
   "failure":"PRIVATE_FAILURE","final":"PRIVATE_ANSWER","workspace":"/PRIVATE_WORKSPACE",
   "cli_stderr":"PRIVATE_STDERR","cli_events":[{"private":"PRIVATE_EVENT"}],
   "exchanges":[{"request":{"private":"PRIVATE_REQUEST"},"response":{"private":"PRIVATE_RESPONSE"}}],
   "checks":{"protocol_valid":true,"fixture_read":false,"failed_read_before_success":false,"client_test_command":false,"independent_tests":false,"edit_and_tests_preserved":true,"final_assertion":false,"PRIVATE_CHECK":false}},
  {"task":"tool-recovery","role":"opus","client":"claude","passed":false,
   "checks":{"protocol_valid":true,"fixture_read":true,"failed_read_before_success":false,"final_assertion":false}},
  {"task":"planning","role":"haiku","client":"claude","passed":false}
 ]}`
	if err := os.WriteFile(filepath.Join(root, "task-cli-quality-latest.json"), []byte(report), 0600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `{}`) }))
	defer server.Close()
	snapshot, raw := runSnapshot(t, root, server.URL)
	quality, ok := snapshot["cli_quality"].(map[string]any)
	if !ok || quality["suite"] != "extended" || quality["finished_at"] != "2026-10-09T07:42:00Z" || quality["corpus_sha256"] != "0123456789abcdef" || quality["passed"] != float64(1) || quality["total"] != float64(4) {
		t.Fatalf("stored subset not summarized accurately: %s", raw)
	}
	want := []any{
		map[string]any{"task": "coding-fix", "role": "sonnet", "client": "codex", "failed_checks": []any{"client_test_command", "final_assertion", "fixture_read", "independent_tests"}},
		map[string]any{"task": "tool-recovery", "role": "opus", "client": "claude", "failed_checks": []any{"failed_read_before_success", "final_assertion"}},
		map[string]any{"task": "planning", "role": "haiku", "client": "claude", "failed_checks": []any{}},
	}
	if !reflect.DeepEqual(quality["failed_cases"], want) {
		t.Fatalf("failed cases must whitelist only explicit supported failures: %#v", quality["failed_cases"])
	}
	if strings.Contains(raw, "PRIVATE_") || strings.Contains(raw, root) {
		t.Fatalf("private report evidence leaked: %s", raw)
	}
}

func TestStatusInvalidOrSymlinkedCLIReportIsUnknown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `{}`) }))
	defer server.Close()
	for _, body := range []string{`{"version":1`, `{"version":1,"scope":"bounded-api-task-probes","finished_at":"2026-10-09T07:42:00Z","results":[{}]}`, strings.Repeat("x", 1<<20+1)} {
		root := canonicalRoot(t)
		if err := os.WriteFile(filepath.Join(root, "task-cli-quality-latest.json"), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		snapshot, _ := runSnapshot(t, root, server.URL)
		if snapshot["cli_quality"] != nil {
			t.Fatal("invalid report became known")
		}
	}
	root := canonicalRoot(t)
	if err := os.WriteFile(filepath.Join(root, "private.json"), []byte(`{"version":1,"scope":"real-cli-task-probes","finished_at":"2026-10-09T07:42:00Z","results":[{"passed":true}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "private.json"), filepath.Join(root, "task-cli-quality-latest.json")); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := runSnapshot(t, root, server.URL)
	if snapshot["cli_quality"] != nil {
		t.Fatal("symlinked report read")
	}
}

type rejectingWriter struct{}

func (rejectingWriter) Write([]byte) (int, error) { return 0, errors.New("PRIVATE_OUTPUT_ERROR") }

func TestStatusOutputFailureDoesNotLeakWriterError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `{}`) }))
	defer server.Close()
	var diagnostic bytes.Buffer
	if code := Run([]string{"--root", canonicalRoot(t), "--endpoint", server.URL, "status"}, nil, rejectingWriter{}, &diagnostic); code != 1 || diagnostic.Len() == 0 || strings.Contains(diagnostic.String(), "PRIVATE_OUTPUT_ERROR") {
		t.Fatalf("output failure result: %d %s", code, &diagnostic)
	}
}
