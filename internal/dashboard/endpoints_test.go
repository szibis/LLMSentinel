package dashboard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/szibis/claude-escalate/internal/config"
	"github.com/szibis/claude-escalate/internal/metrics"
)

func TestHomeExpansionHonorsConfiguredEnvironment(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if got := expandHome("~/test.json"); got != filepath.Join(home, "test.json") {
		t.Fatalf("home expansion bypasses environment: %q", got)
	}
}

func TestSessionMetricsHTTPEndpoints(t *testing.T) {
	sm := metrics.NewSessionMetrics()
	sm.RecordBurnedTokens(100, 50, 10, 20, .3)
	sm.RecordOptimizationSaving("exact_dedup", 30, .1, 2)
	h := NewMetricsHandler(sm)
	mux := http.NewServeMux()
	h.RegisterMetricsEndpoints(mux)
	for _, path := range []string{"/api/metrics/overview", "/api/metrics/daily", "/api/metrics/daily?days=1", "/api/metrics/daily?days=invalid", "/api/metrics/breakdown", "/api/metrics/projections", "/api/metrics/full", "/api/metrics/export/json", "/api/metrics/export/csv"} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 {
			t.Fatal(path, w.Code)
		}
		if strings.HasSuffix(path, "/csv") {
			if !strings.Contains(w.Body.String(), "Tokens Burned (Total),150,tokens") || !strings.Contains(w.Body.String(), "Exact Dedup,30,$0.10,2") {
				t.Fatal(w.Body.String())
			}
			continue
		}
		var response interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(path, err)
		}
		if strings.HasSuffix(path, "overview") {
			m := response.(map[string]interface{})
			if m["requests"] != float64(1) || m["tokens_burned"].(map[string]interface{})["total"] != float64(150) {
				t.Fatal(m)
			}
		}
	}
}

func TestConfigurationAndMetricsRoutes(t *testing.T) {
	s := setupTestServer(t)
	s.metricsCollector.RecordTokens(17, 3)
	s.metricsCollector.SaveSnapshot()
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{{"POST", "/api/config", "{", 400}, {"POST", "/api/config", "{}", 400}, {"POST", "/api/config", `{"gateway":{"port":8077}}`, 501}, {"DELETE", "/api/config", "", 405}, {"POST", "/api/config/reload", "", 200}, {"GET", "/api/metrics/history", "", 200}, {"GET", "/api/metrics/export", "", 200}, {"GET", "/api/metrics/export?format=json", "", 200}, {"GET", "/api/metrics/export?format=bad", "", 400}, {"GET", "/api/metrics/stream", "", 200}, {"GET", "/static/absent", "", 404}, {"POST", "/api/tools", "", 405}, {"POST", "/api/tools/types", "", 405}, {"GET", "/api/tools/unknown", "", 405}} {
		w := httptest.NewRecorder()
		s.httpServer.Handler.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)))
		if w.Code != tc.status {
			t.Fatal(tc, w.Code, w.Body.String())
		}
	}
	if err := s.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.httpServer.Addr = "invalid address"
	if s.Start() == nil {
		t.Fatal("invalid bind succeeded")
	}
}

func TestConfigPOSTReportsUnsupportedPersistence(t *testing.T) {
	s := setupTestServer(t)
	w := httptest.NewRecorder()
	s.handleConfigSet(w, httptest.NewRequest("POST", "/api/config", strings.NewReader(`{"gateway":{"port":8123}}`)))
	if w.Code != http.StatusNotImplemented || !strings.Contains(w.Body.String(), "not implemented") {
		t.Fatal("unsaved config claimed success", w.Code, w.Body.String())
	}
}

func TestConfigurationReadErrorsAndAnalytics(t *testing.T) {
	s := setupTestServer(t)
	s.configLoader = config.NewLoader(filepath.Join(t.TempDir(), "missing.yaml"))
	for _, tc := range []struct{ method, path, body string }{{"GET", "/api/config", ""}, {"POST", "/api/config/reload", ""}, {"GET", "/api/tools", ""}, {"POST", "/api/tools/add", `{"name":"synthetic","type":"cli","path":"/synthetic"}`}, {"PUT", "/api/tools/x", `{}`}, {"DELETE", "/api/tools/x", ""}} {
		w := httptest.NewRecorder()
		s.httpServer.Handler.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)))
		if w.Code != 500 {
			t.Fatal(tc, w.Code, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	s.handleToolTest(w, httptest.NewRequest("POST", "/api/tools/x/test", nil), "x")
	if !strings.Contains(w.Body.String(), "unhealthy") {
		t.Fatal(w.Body.String())
	}
	log := filepath.Join(os.Getenv("HOME"), ".claude", "execution-logs", "project_local.jsonl")
	os.MkdirAll(filepath.Dir(log), 0700)                                                                                   // #nosec G703 -- setupTestServer sets HOME to this test's private t.TempDir; fixed execution-log fixture path.
	os.WriteFile(log, []byte("invalid\n"+`{"operation_type":"synthetic","duration_ms":17,"status":"success"}`+"\n"), 0600) // #nosec G703 -- synthetic fixture written only beneath this test's private HOME.
	w = httptest.NewRecorder()
	s.handleExecutionAnalytics(w, httptest.NewRequest("GET", "/api/analytics", nil))
	if !strings.Contains(w.Body.String(), `"total_operations":1`) {
		t.Fatal(w.Body.String())
	}
	os.Remove(log) // #nosec G703 -- removes the fixed synthetic log created above in the test's private HOME.
	w = httptest.NewRecorder()
	s.handleExecutionAnalytics(w, httptest.NewRequest("GET", "/api/analytics", nil))
	if !strings.Contains(w.Body.String(), `"total_operations":0`) {
		t.Fatal(w.Body.String())
	}
}

func TestToolCRUDPersistsAndHealthFailuresRemainVisible(t *testing.T) {
	s := setupTestServer(t)
	path := filepath.Join(os.Getenv("HOME"), ".claude-escalate", "config.yaml")
	cfg := config.DefaultConfig()
	cfg.Optimizations.MCP.Enabled = true
	cfg.Optimizations.MCP.Tools = []config.MCPTool{{Name: "synthetic", Type: "mcp", Settings: map[string]interface{}{"path": "/synthetic/original"}}}
	cfg.Tools = []config.MCPTool{{Name: "cli-synthetic", Type: "cli"}, {Name: "rest-synthetic", Type: "rest"}, {Name: "unsupported-synthetic", Type: "database"}}
	if err := saveConfigToFile(cfg, path); err != nil {
		t.Fatal(err)
	}
	s.configLoader = config.NewLoader(path)
	for _, tc := range []struct {
		method, url, body string
		status            int
	}{{"GET", "/api/tools", "", 200}, {"POST", "/api/tools/add", `{"name":"synthetic","type":"mcp","path":"/x"}`, 400}, {"PUT", "/api/tools/synthetic", `{"path":"/synthetic/updated","settings":{"limit":7}}`, 200}, {"PUT", "/api/tools/synthetic", "{", 400}} {
		w := httptest.NewRecorder()
		s.httpServer.Handler.ServeHTTP(w, httptest.NewRequest(tc.method, tc.url, strings.NewReader(tc.body)))
		if w.Code != tc.status {
			t.Fatal(tc, w.Code, w.Body.String())
		}
	}
	loaded, err := config.NewLoader(path).Load()
	if err != nil || loaded.Optimizations.MCP.Tools[0].Settings["path"] != "/synthetic/updated" {
		t.Fatal(loaded, err)
	}
	for _, tc := range []struct{ name, status string }{{"synthetic", "unhealthy"}, {"rest-synthetic", "healthy"}, {"cli-synthetic", "healthy"}, {"unsupported-synthetic", "unsupported"}, {"absent", "unknown"}} {
		w := httptest.NewRecorder()
		s.httpServer.Handler.ServeHTTP(w, httptest.NewRequest("POST", "/api/tools/"+tc.name+"/test", nil))
		var response map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || response["status"] != tc.status {
			t.Fatal(tc, response, err)
		}
	}
	w := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(w, httptest.NewRequest("DELETE", "/api/tools/synthetic", nil))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	loaded, err = config.NewLoader(path).Load()
	if err != nil || len(loaded.Optimizations.MCP.Tools) != 0 {
		t.Fatal("delete not persisted", loaded, err)
	}
	w = httptest.NewRecorder()
	s.handleToolEdit(w, httptest.NewRequest("POST", "/x", nil), "x")
	if w.Code != 405 {
		t.Fatal(w.Code)
	}
	w = httptest.NewRecorder()
	s.handleToolDelete(w, httptest.NewRequest("POST", "/x", nil), "x")
	if w.Code != 405 {
		t.Fatal(w.Code)
	}
	if expandHome("relative") != "relative" {
		t.Fatal("relative expansion")
	}
	t.Setenv("HOME", "")
	if expandHome("~/unavailable") != "~/unavailable" {
		t.Fatal("home error hidden")
	}
}
