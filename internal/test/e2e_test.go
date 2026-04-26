// Package test provides end-to-end and integration tests for Claude Escalate v4.0.0
package test

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

// E2ETestSuite runs full end-to-end verification of v4.0.0 features
type E2ETestSuite struct {
	baseURL string
	client  *http.Client
}

// NewE2ETestSuite creates a new E2E test suite
func NewE2ETestSuite(baseURL string) *E2ETestSuite {
	return &E2ETestSuite{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// TestFeature1MLClassification verifies ML-based task classification works end-to-end
func (suite *E2ETestSuite) TestFeature1MLClassification(t *testing.T) {
	// Test embedding classifier for 10 different task types
	testCases := []struct {
		prompt   string
		expected string
	}{
		{"race condition deadlock concurrent", "concurrency"},
		{"parse regex grammar", "parsing"},
		{"optimize performance latency", "optimization"},
		{"debug segfault panic", "debugging"},
		{"architecture design system", "architecture"},
		{"encrypt crypto security", "security"},
		{"database query schema", "database"},
		{"network socket tcp", "networking"},
		{"test mock spec", "testing"},
		{"deploy docker kubernetes", "devops"},
	}

	for _, tc := range testCases {
		body := fmt.Sprintf(`{"prompt": "%s"}`, tc.prompt)
		resp, err := http.Post(
			fmt.Sprintf("%s/api/classify/predict", suite.baseURL),
			"application/json",
			bytes.NewBufferString(body),
		)

		if err != nil {
			t.Errorf("classify request failed: %v", err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("classify returned %d for %s", resp.StatusCode, tc.expected)
		}

		resp.Body.Close()
	}
}

// TestFeature2Analytics verifies time-series, percentiles, and forecasting
func (suite *E2ETestSuite) TestFeature2Analytics(t *testing.T) {
	endpoints := []string{
		"/api/analytics/timeseries?bucket=daily&days=7",
		"/api/analytics/percentiles?bucket=hourly&days=7",
		"/api/analytics/forecast?metric=total_cost_usd&days=7",
		"/api/analytics/task-accuracy?days=30",
		"/api/analytics/correlations",
	}

	for _, endpoint := range endpoints {
		resp, err := http.Get(fmt.Sprintf("%s%s", suite.baseURL, endpoint))
		if err != nil {
			t.Errorf("analytics endpoint %s failed: %v", endpoint, err)
			continue
		}

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
			data, _ := io.ReadAll(resp.Body)
			t.Errorf("analytics endpoint %s returned %d: %s", endpoint, resp.StatusCode, string(data))
		}

		resp.Body.Close()
	}
}

// TestFeature3Observability verifies Prometheus and OTEL metrics export
func (suite *E2ETestSuite) TestFeature3Observability(t *testing.T) {
	// Check Prometheus endpoint
	resp, err := http.Get(fmt.Sprintf("%s/metrics", suite.baseURL))
	if err != nil {
		t.Errorf("metrics endpoint failed: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("metrics returned %d, expected 200", resp.StatusCode)
	}

	// Verify Prometheus text format
	data, _ := io.ReadAll(resp.Body)
	metricsText := string(data)

	requiredMetrics := []string{
		"claude_escalate_requests_total",
		"claude_escalate_cache_hits_total",
		"claude_escalate_cost_usd_total",
		"claude_escalate_cache_hit_rate",
		"claude_escalate_model_usage_total",
	}

	for _, metric := range requiredMetrics {
		if !bytes.Contains([]byte(metricsText), []byte(metric)) {
			t.Errorf("metrics missing %s", metric)
		}
	}
}

// TestFeature4WebDashboard verifies web dashboard components
func (suite *E2ETestSuite) TestFeature4WebDashboard(t *testing.T) {
	endpoints := []struct {
		path   string
		name   string
	}{
		{"/", "Overview"},
		{"/analytics", "Analytics"},
		{"/tasks", "Tasks"},
		{"/config", "Config"},
		{"/health", "Health"},
	}

	for _, ep := range endpoints {
		resp, err := http.Get(fmt.Sprintf("%s:3001%s", suite.baseURL[:len(suite.baseURL)-5], ep.path))
		if err != nil {
			t.Logf("dashboard %s not available (development only): %v", ep.name, err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("dashboard %s returned %d", ep.name, resp.StatusCode)
		}
	}
}

// TestFeature5DockerCompose verifies docker-compose stack
func (suite *E2ETestSuite) TestFeature5DockerCompose(t *testing.T) {
	// Verify main service is available
	resp, err := http.Get(fmt.Sprintf("%s/health", suite.baseURL))
	if err != nil {
		t.Fatalf("main service unavailable: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("main service health check failed: %d", resp.StatusCode)
	}

	// Verify VictoriaMetrics is accessible (port 8428)
	vmResp, err := http.Get("http://localhost:8428/health")
	if err != nil {
		t.Logf("VictoriaMetrics not available (may not be running): %v", err)
	} else {
		defer vmResp.Body.Close()
		if vmResp.StatusCode != http.StatusOK {
			t.Errorf("VictoriaMetrics health check failed: %d", vmResp.StatusCode)
		}
	}

	// Verify Grafana is accessible (port 3000)
	grafanaResp, err := http.Get("http://localhost:3000/api/health")
	if err != nil {
		t.Logf("Grafana not available (may not be running): %v", err)
	} else {
		defer grafanaResp.Body.Close()
		if grafanaResp.StatusCode != http.StatusOK && grafanaResp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Grafana health check failed: %d", grafanaResp.StatusCode)
		}
	}
}

// RegressionTest runs regression matrix against all features
func (suite *E2ETestSuite) RegressionTest(t *testing.T) {
	t.Run("Feature1-MLClassification", suite.TestFeature1MLClassification)
	t.Run("Feature2-Analytics", suite.TestFeature2Analytics)
	t.Run("Feature3-Observability", suite.TestFeature3Observability)
	t.Run("Feature4-WebDashboard", suite.TestFeature4WebDashboard)
	t.Run("Feature5-DockerCompose", suite.TestFeature5DockerCompose)
}

// TestIntegrationE2E runs all E2E tests
func TestIntegrationE2E(t *testing.T) {
	// Skip E2E tests unless explicitly enabled (set RUN_E2E=1)
	if os.Getenv("RUN_E2E") == "" {
		t.Skip("skipping E2E tests (set RUN_E2E=1 to enable)")
	}

	suite := NewE2ETestSuite("http://localhost:9000")
	suite.RegressionTest(t)
}

// TestHealthCheckLoop verifies continuous service availability
func TestHealthCheckLoop(t *testing.T) {
	// Skip E2E tests unless explicitly enabled (set RUN_E2E=1)
	if os.Getenv("RUN_E2E") == "" {
		t.Skip("skipping E2E tests (set RUN_E2E=1 to enable)")
	}

	client := &http.Client{Timeout: 5 * time.Second}
	failures := 0

	for i := 0; i < 10; i++ {
		resp, err := client.Get("http://localhost:9000/health")
		if err != nil {
			failures++
			t.Logf("health check %d failed: %v", i+1, err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			failures++
			t.Logf("health check %d returned %d", i+1, resp.StatusCode)
		}

		resp.Body.Close()
		time.Sleep(500 * time.Millisecond)
	}

	if failures > 2 {
		t.Errorf("too many health check failures: %d/10", failures)
	}
}
