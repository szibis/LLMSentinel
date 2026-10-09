package labdashboard

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/szibis/claude-escalate/internal/jes"
)

func TestSavedEvidenceRedactionAndUnknown(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"api", "cli", "benchmark", "judge", "unknown"} {
		if loadEvidence(root, kind) != nil {
			t.Fatal("invented evidence", kind)
		}
	}
	reports := map[string]string{
		"task-quality-latest.json":     `{"version":1,"scope":"bounded-api-task-probes","finished_at":"2026-10-08T00:00:00Z","results":[{"task":"exact-read","passed":false,"failure":"HTTP 422","final":"PRIVATE","exchanges":[{"response":{"secret":"PRIVATE"}}]}]}`,
		"task-cli-quality-latest.json": `{"version":1,"scope":"real-cli-task-probes","finished_at":"2026-10-08T00:00:00Z","results":[{"task":"exact-read","passed":true,"final":"PRIVATE","cli_stderr":"PRIVATE","cli_events":[{"secret":"PRIVATE"}]}]}`,
		"benchmark-latest.json":        `{"schema_version":1,"scope":"synthetic-benchmark","timestamp":"2026-10-08T00:00:00Z","samples":[]}`,
	}
	for name, raw := range reports {
		if err := os.WriteFile(filepath.Join(root, name), []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if code := jes.Run([]string{"--output", filepath.Join(root, "jes-quality-latest.json")}, strings.NewReader(`{"version":1,"samples":[{"id":"unknown-fixture"}],"policy":{"minimum_score":1}}`), io.Discard, io.Discard); code != 0 {
		t.Fatal("advisory fixture failed", code)
	}
	for _, kind := range []string{"api", "cli", "benchmark", "judge"} {
		raw := loadEvidence(root, kind)
		if !json.Valid(raw) || strings.Contains(string(raw), "PRIVATE") {
			t.Fatalf("%s missing/redaction failed: %s", kind, raw)
		}
	}
}

func TestDashboardLoadsBenchmarkHistorySeparately(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "benchmark-history")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"} {
		raw := `{"schema_version":1,"scope":"synthetic-benchmark","run_id":"` + id + `","timestamp":"2026-10-08T00:00:00Z","samples":[]}`
		name := "20261008T000000.000000000Z-" + id + ".json"
		if err := os.WriteFile(filepath.Join(dir, name), []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var reports []map[string]any
	raw := loadEvidence(root, "benchmark-history")
	if err := json.Unmarshal(raw, &reports); err != nil || len(reports) != 2 {
		t.Fatalf("saved histories unavailable: %s %v", raw, err)
	}
	if reports[0]["run_id"] == reports[1]["run_id"] {
		t.Fatal("run identities collapsed")
	}
	none := func() json.RawMessage { return nil }
	handler := newHandlerWithHistory(&http.Client{Timeout: 10 * time.Millisecond}, func() (json.RawMessage, error) { return nil, nil }, none, none, none, none, func() json.RawMessage { return loadEvidence(root, "benchmark-history") })
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/status", nil))
	var response struct {
		Runs []map[string]any `json:"benchmark_history"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || w.Code != 200 || len(response.Runs) != 2 {
		t.Fatalf("HTTP history unavailable: %s %v", w.Body.String(), err)
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/status", nil))
	if w.Code != 405 {
		t.Fatal("dashboard accepted a write")
	}
	files, _ := os.ReadDir(dir)
	if len(files) != 2 {
		t.Fatal("dashboard request mutated archived reports")
	}
}

type notifyWriter chan struct{}

func (w notifyWriter) Write(p []byte) (int, error) {
	select {
	case w <- struct{}{}:
	default:
	}
	return len(p), nil
}

func TestRunLifecycleOnlyEphemeralHealth(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	started := make(notifyWriter, 1)
	done := make(chan int, 1)
	var diagnostic bytes.Buffer
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	go func() { done <- Run([]string{"--root", root, "--listen", address}, nil, started, &diagnostic) }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("startup timed out")
	}
	client := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 100 * time.Millisecond}
	defer client.CloseIdleConnections()
	deadline := time.Now().Add(3 * time.Second)
	healthy := false
	for time.Now().Before(deadline) {
		response, err := client.Get("http://" + address + "/health")
		if err == nil {
			raw, _ := io.ReadAll(response.Body)
			response.Body.Close()
			healthy = response.StatusCode == 200 && strings.Contains(string(raw), "lab-dashboard")
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case code := <-done:
		if code != 0 || !healthy {
			t.Fatalf("health=%t code=%d %s", healthy, code, diagnostic.String())
		}
	case <-time.After(4 * time.Second):
		t.Fatal("shutdown timed out")
	}
}

func TestRunValidationAndOccupiedListener(t *testing.T) {
	for _, args := range [][]string{{"--unknown"}, {"extra"}, {"--listen", "0.0.0.0:8077"}} {
		if Run(args, nil, io.Discard, io.Discard) != 2 {
			t.Fatal(args)
		}
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "linked")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	if Run([]string{"--root", link}, nil, io.Discard, io.Discard) != 1 {
		t.Fatal("symlink root accepted")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if Run([]string{"--root", root, "--listen", listener.Addr().String()}, nil, io.Discard, io.Discard) != 1 {
		t.Fatal("occupied bind accepted")
	}
}

type errorBody struct{}

func (errorBody) Read([]byte) (int, error) { return 0, errors.New("fixture disconnect") }
func (errorBody) Close() error             { return nil }
func TestReadEndpointFailuresAndHealthRoute(t *testing.T) {
	for _, raw := range []string{"null", "[]", "{", strings.Repeat("x", 65537)} {
		client := &http.Client{Transport: testTransport(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(raw))}, nil
		})}
		if readEndpoint(client, "/health") != nil {
			t.Fatal("invalid endpoint evidence accepted")
		}
	}
	for _, transport := range []testTransport{func(*http.Request) (*http.Response, error) { return nil, errors.New("offline") }, func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: errorBody{}}, nil
	}} {
		if readEndpoint(&http.Client{Transport: transport}, "/health") != nil {
			t.Fatal("failed endpoint invented")
		}
	}
}
