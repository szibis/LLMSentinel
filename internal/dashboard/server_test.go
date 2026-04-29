package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/szibis/claude-escalate/internal/config"
	"github.com/szibis/claude-escalate/internal/gateway"
	"github.com/szibis/claude-escalate/internal/metrics"
)

// Test helpers
func setupTestServer(_ *testing.T) *Server {
	loader := config.NewLoader("")
	collector := metrics.NewMetricsCollector()
	publisher := metrics.NewMetricsPublisher(collector, 1*time.Minute)
	factory := gateway.NewAdapterFactory()

	s := NewServer("127.0.0.1", 8077, loader, collector, publisher, factory)
	return s
}

// ============================================================================
// Dashboard HTML Tests
// ============================================================================

func TestDashboardHTML_Returns200(t *testing.T) {
	s := setupTestServer(t)
	req := httptest.NewRequest("GET", "/dashboard", nil)
	w := httptest.NewRecorder()

	s.handleDashboard(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestDashboardHTML_ContainsRequiredTabs(t *testing.T) {
	s := setupTestServer(t)
	req := httptest.NewRequest("GET", "/dashboard", nil)
	w := httptest.NewRecorder()

	s.handleDashboard(w, req)

	body := w.Body.String()
	requiredTabs := []string{
		"Configuration",
		"Tools",
		"Security",
		"Feedback",
		"Analytics",
		"Metrics",
	}

	for _, tab := range requiredTabs {
		if !strings.Contains(body, tab) {
			t.Errorf("Dashboard missing required tab: %s", tab)
		}
	}
}

func TestDashboardHTML_ContainsConfigEditor(t *testing.T) {
	s := setupTestServer(t)
	req := httptest.NewRequest("GET", "/dashboard", nil)
	w := httptest.NewRecorder()

	s.handleDashboard(w, req)

	body := w.Body.String()
	requiredElements := []string{
		`id="config-editor"`,
		`id="config-highlight"`,
		`id="config-hints"`,
		`id="tools-list-table"`,
		`id="available-tools"`,
	}

	for _, elem := range requiredElements {
		if !strings.Contains(body, elem) {
			t.Errorf("Dashboard missing required element: %s", elem)
		}
	}
}

func TestDashboardHTML_ContainsJavaScriptFunctions(t *testing.T) {
	s := setupTestServer(t)
	req := httptest.NewRequest("GET", "/dashboard", nil)
	w := httptest.NewRecorder()

	s.handleDashboard(w, req)

	body := w.Body.String()
	requiredFunctions := []string{
		"function switchTab",
		"function loadConfig",
		"function saveConfig",
		"function highlightYAMLSyntax",
		"function getYAMLPath",
		"function getNestedConfigHints",
		"function quickJump",
		"function loadTools",
		"function loadKnownTools",
		"function addTool",
		"function validateToolForm",
	}

	for _, fn := range requiredFunctions {
		if !strings.Contains(body, fn) {
			t.Errorf("Dashboard missing required function: %s", fn)
		}
	}
}

// ============================================================================
// Config API Tests
// ============================================================================

func TestConfigAPI_Get_Returns200(t *testing.T) {
	s := setupTestServer(t)
	req := httptest.NewRequest("GET", "/api/config", nil)
	w := httptest.NewRecorder()

	s.handleConfig(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestConfigAPI_Get_ReturnsValidJSON(t *testing.T) {
	s := setupTestServer(t)
	req := httptest.NewRequest("GET", "/api/config", nil)
	w := httptest.NewRecorder()

	s.handleConfig(w, req)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Errorf("Config response is not valid JSON: %v", err)
	}

	if _, ok := response["config"]; !ok {
		t.Error("Config response missing 'config' key")
	}
}

// ============================================================================
// Config Spec Tests
// ============================================================================

func TestConfigSpec_Returns200(t *testing.T) {
	s := setupTestServer(t)
	req := httptest.NewRequest("GET", "/api/config/spec", nil)
	w := httptest.NewRecorder()

	s.handleConfigSpec(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestConfigSpec_ReturnsValidJSON(t *testing.T) {
	s := setupTestServer(t)
	req := httptest.NewRequest("GET", "/api/config/spec", nil)
	w := httptest.NewRecorder()

	s.handleConfigSpec(w, req)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Errorf("Config spec response is not valid JSON: %v", err)
	}
}

// ============================================================================
// Tools API Tests
// ============================================================================

func TestToolsList_Returns200(t *testing.T) {
	s := setupTestServer(t)
	req := httptest.NewRequest("GET", "/api/tools", nil)
	w := httptest.NewRecorder()

	s.handleTools(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestToolsList_ReturnsValidJSON(t *testing.T) {
	s := setupTestServer(t)
	req := httptest.NewRequest("GET", "/api/tools", nil)
	w := httptest.NewRecorder()

	s.handleTools(w, req)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Errorf("Tools list response is not valid JSON: %v", err)
	}

	if _, ok := response["tools"]; !ok {
		t.Error("Tools list response missing 'tools' key")
	}
}

// ============================================================================
// Tools Known Tests
// ============================================================================

func TestToolsKnown_Returns200(t *testing.T) {
	s := setupTestServer(t)
	req := httptest.NewRequest("GET", "/api/tools/known", nil)
	w := httptest.NewRecorder()

	s.handleToolsKnown(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

// ============================================================================
// Tools Discover Tests
// ============================================================================

func TestToolsDiscover_Returns200(t *testing.T) {
	s := setupTestServer(t)
	req := httptest.NewRequest("GET", "/api/tools/discover", nil)
	w := httptest.NewRecorder()

	s.handleToolsDiscover(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

// ============================================================================
// Tools Types Tests
// ============================================================================

func TestToolsTypes_Returns200(t *testing.T) {
	s := setupTestServer(t)
	req := httptest.NewRequest("GET", "/api/tools/types", nil)
	w := httptest.NewRecorder()

	s.handleToolsTypes(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

// ============================================================================
// Metrics Tests
// ============================================================================

func TestMetrics_Returns200(t *testing.T) {
	s := setupTestServer(t)
	req := httptest.NewRequest("GET", "/api/metrics", nil)
	w := httptest.NewRecorder()

	s.handleMetrics(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

// ============================================================================
// Health Check Tests
// ============================================================================

func TestHealth_Returns200(t *testing.T) {
	s := setupTestServer(t)
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	s.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

// ============================================================================
// Content Type Tests
// ============================================================================

func TestContentType_API_ReturnsJSON(t *testing.T) {
	s := setupTestServer(t)
	endpoints := []string{
		"/api/config",
		"/api/config/spec",
		"/api/tools",
		"/api/tools/known",
		"/api/metrics",
		"/health",
	}

	for _, endpoint := range endpoints {
		req := httptest.NewRequest("GET", endpoint, nil)
		w := httptest.NewRecorder()

		// Route to appropriate handler
		switch endpoint {
		case "/api/config":
			s.handleConfig(w, req)
		case "/api/config/spec":
			s.handleConfigSpec(w, req)
		case "/api/tools":
			s.handleTools(w, req)
		case "/api/tools/known":
			s.handleToolsKnown(w, req)
		case "/api/metrics":
			s.handleMetrics(w, req)
		case "/health":
			s.handleHealth(w, req)
		}

		contentType := w.Header().Get("Content-Type")
		if !strings.Contains(contentType, "application/json") {
			t.Errorf("Endpoint %s returned content-type %s, expected application/json", endpoint, contentType)
		}
	}
}

func TestContentType_Dashboard_ReturnsHTML(t *testing.T) {
	s := setupTestServer(t)
	req := httptest.NewRequest("GET", "/dashboard", nil)
	w := httptest.NewRecorder()

	s.handleDashboard(w, req)

	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		t.Errorf("Dashboard returned content-type %s, expected text/html", contentType)
	}
}

// ============================================================================
// Tool Management API Tests
// ============================================================================

func TestToolsAdd_Returns200OnSuccess(t *testing.T) {
	s := setupTestServer(t)
	// Use unique tool name to avoid conflicts with previous test runs
	toolName := fmt.Sprintf("test_tool_%d", time.Now().UnixNano())
	reqBody := fmt.Sprintf(`{"name":"%s","type":"cli","path":"/usr/bin/test","settings":{}}`, toolName)
	req := httptest.NewRequest("POST", "/api/tools/add", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.handleToolsAdd(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusCreated {
		t.Logf("Response body: %s", w.Body.String())
		t.Errorf("Expected status 200 or 201, got %d", w.Code)
	}
}

func TestToolsAdd_RejectsEmptyName(t *testing.T) {
	s := setupTestServer(t)
	reqBody := `{"name":"","type":"cli","path":"/usr/bin/test","settings":{}}`
	req := httptest.NewRequest("POST", "/api/tools/add", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.handleToolsAdd(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for empty name, got %d", w.Code)
	}
}

func TestToolsAdd_RejectsEmptyType(t *testing.T) {
	s := setupTestServer(t)
	reqBody := `{"name":"test_tool","type":"","path":"/usr/bin/test","settings":{}}`
	req := httptest.NewRequest("POST", "/api/tools/add", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.handleToolsAdd(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for empty type, got %d", w.Code)
	}
}

func TestToolsAdd_RejectsEmptyPath(t *testing.T) {
	s := setupTestServer(t)
	reqBody := `{"name":"test_tool","type":"cli","path":"","settings":{}}`
	req := httptest.NewRequest("POST", "/api/tools/add", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.handleToolsAdd(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for empty path, got %d", w.Code)
	}
}

func TestToolsAdd_RejectsInvalidJSON(t *testing.T) {
	s := setupTestServer(t)
	reqBody := `{invalid json}`
	req := httptest.NewRequest("POST", "/api/tools/add", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.handleToolsAdd(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for invalid JSON, got %d", w.Code)
	}
}

func TestToolsAdd_RejectsOnlyWithPOST(t *testing.T) {
	s := setupTestServer(t)
	req := httptest.NewRequest("GET", "/api/tools/add", nil)
	w := httptest.NewRecorder()

	s.handleToolsAdd(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405 for GET, got %d", w.Code)
	}
}

func TestToolsDelete_Returns200OnSuccess(t *testing.T) {
	s := setupTestServer(t)
	req := httptest.NewRequest("DELETE", "/api/tools/nonexistent", nil)
	w := httptest.NewRecorder()

	s.handleToolDelete(w, req, "nonexistent")

	// Will fail because tool doesn't exist, but handler should respond with 404
	if w.Code != http.StatusNotFound && w.Code != http.StatusOK {
		t.Errorf("expected 404 or 200, got %d", w.Code)
	}
}

func TestToolsEdit_Returns405OnNonPUT(t *testing.T) {
	s := setupTestServer(t)
	req := httptest.NewRequest("GET", "/api/tools/test", nil)
	w := httptest.NewRecorder()

	s.handleToolEdit(w, req, "test")

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

func TestToolsTest_Returns200(t *testing.T) {
	s := setupTestServer(t)
	req := httptest.NewRequest("POST", "/api/tools/test/test", nil)
	w := httptest.NewRecorder()

	s.handleToolTest(w, req, "test")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Errorf("Response is not valid JSON: %v", err)
	}

	if _, ok := response["status"]; !ok {
		t.Error("Response missing 'status' key")
	}
}
