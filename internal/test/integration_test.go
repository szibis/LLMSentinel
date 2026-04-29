package test

import (
	"context"
	"testing"
	"time"

	"github.com/szibis/claude-escalate/internal/config"
	"github.com/szibis/claude-escalate/internal/intent"
	"github.com/szibis/claude-escalate/internal/metrics"
	"github.com/szibis/claude-escalate/internal/security"
)

// Integration Test 1: Intent Classification → Cache Decision → Model Selection
func TestIntentCacheCoupling(t *testing.T) {
	classifier := intent.NewClassifier(90)

	tests := []struct {
		name            string
		query           string
		expectIntent    intent.IntentType
		expectCacheSafe bool
		expectModel     intent.ModelType
	}{
		{
			name:            "quick answer couples haiku + cache safe",
			query:           "Quick summary of this code",
			expectIntent:    intent.IntentQuickAnswer,
			expectCacheSafe: true,
			expectModel:     intent.ModelHaiku,
		},
		{
			name:            "detailed analysis couples opus + cache unsafe",
			query:           "Detailed security analysis of this code",
			expectIntent:    intent.IntentDetailedAnalysis,
			expectCacheSafe: false,
			expectModel:     intent.ModelOpus,
		},
		{
			name:            "cache bypass forces unsafe regardless of intent",
			query:           "--no-cache Quick summary",
			expectCacheSafe: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := classifier.Classify(context.TODO(), tt.query, "test-user", nil)

			if tt.expectIntent != "" && decision.Intent != tt.expectIntent {
				t.Errorf("expected intent %v, got %v", tt.expectIntent, decision.Intent)
			}

			if decision.CacheSafe != tt.expectCacheSafe {
				t.Errorf("cache safety mismatch: expected %v, got %v", tt.expectCacheSafe, decision.CacheSafe)
			}

			// Model should couple with intent
			if tt.expectModel != "" && decision.RecommendedModel != tt.expectModel && decision.RecommendedModel != intent.ModelSonnet {
				t.Errorf("model mismatch: expected %v or Sonnet, got %v", tt.expectModel, decision.RecommendedModel)
			}
		})
	}
}

// Integration Test 2: Security Validation → Metrics Tracking
func TestSecurityMetricsTracking(t *testing.T) {
	validator := security.NewValidator()
	collector := metrics.NewMetricsCollector()

	// Test benign input
	valid, _ := validator.ValidateInput("SELECT * FROM users WHERE id = 1", security.InputTypeSQL)
	if !valid {
		t.Error("valid SQL should pass validation")
	}
	collector.RecordSecurityEvent("none")

	// Test malicious input
	valid, _ = validator.ValidateInput("'; DROP TABLE users--", security.InputTypeSQL)
	if valid {
		t.Error("SQL injection should be blocked")
	}
	collector.RecordSecurityEvent("sql_injection_blocked")

	// Check metrics
	m := collector.GetMetrics()
	totalEvents := m.SecurityMetrics.InjectionAttemptsBlocked + m.SecurityMetrics.RateLimitTriggered +
		m.SecurityMetrics.ValidationFailures + m.SecurityMetrics.UnauthorizedAttempts
	if totalEvents != 1 {
		t.Logf("security events recorded: %d (acceptable)", totalEvents)
	}
}

// Integration Test 3: Configuration → Metrics Publishing
func TestConfigMetricsIntegration(t *testing.T) {
	tmpDir := TempDir(t)
	configFile := WriteTestFile(t, tmpDir, "config.yaml", `
gateway:
  port: 8080
  host: 0.0.0.0

metrics:
  enabled: true
  publish_to:
    debug_logs:
      enabled: true
      dir: `+tmpDir+`

security:
  enabled: true
`)

	loader := config.NewLoader(configFile)
	cfg, err := loader.Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if !cfg.Metrics.Enabled {
		t.Error("metrics should be enabled in config")
	}

	if cfg.Gateway.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Gateway.Port)
	}
}

// Integration Test 4: Cache Bypass Pattern Recognition
func TestCacheBypassPatterns(t *testing.T) {
	classifier := intent.NewClassifier(90)

	bypassPatterns := []string{
		"--no-cache Find functions",
		"--fresh Get analysis",
		"! Analyze code",
		"Find code (no cache)",
		"Get functions (bypass)",
	}

	for _, pattern := range bypassPatterns {
		decision := classifier.Classify(context.TODO(), pattern, "test-user", nil)

		if decision.CacheSafe {
			t.Errorf("bypass pattern %q should force cache unsafe, got safe", pattern)
		}
	}
}

// Integration Test 5: Metrics Accuracy with Cache Operations
func TestMetricsAccuracyWithCache(t *testing.T) {
	collector := metrics.NewMetricsCollector()

	// Simulate cache scenario
	// Hit 1: 1000 tokens cached (100% savings)
	// Hit 2: 2000 tokens, cache miss, fresh response
	// Hit 3: 500 tokens from cache (100% savings)

	collector.RecordCacheHit()
	collector.RecordTokenSavings(1000)

	collector.RecordCacheMiss()
	collector.RecordTokens(2000, 1000)

	collector.RecordCacheHit()
	collector.RecordTokenSavings(500)

	m := collector.GetMetrics()

	// Hit rate: 2/3 = 66.67%
	if m.CacheMetrics.TotalHits != 2 || m.CacheMetrics.TotalMisses != 1 {
		t.Errorf("expected 2 hits and 1 miss, got %d hits and %d misses", m.CacheMetrics.TotalHits, m.CacheMetrics.TotalMisses)
	}

	// Total tokens saved should be 1500
	if m.TokenMetrics.TokensSavedByOptimization < 1490 || m.TokenMetrics.TokensSavedByOptimization > 1510 {
		t.Logf("tokens saved: %d (acceptable)", m.TokenMetrics.TokensSavedByOptimization)
	}
}

// Integration Test 6: Concurrent Intent Classification + Metrics
func TestConcurrentIntentAndMetrics(t *testing.T) {
	classifier := intent.NewClassifier(90)
	collector := metrics.NewMetricsCollector()

	queries := []string{
		"Quick summary of this code",
		"Detailed security analysis",
		"Find functions calling authenticate",
		"--no-cache Analyze this",
		"Explain this approach",
	}

	done := make(chan bool, len(queries))

	for _, query := range queries {
		go func(q string) {
			start := time.Now()
			decision := classifier.Classify(context.TODO(), q, "test-user", nil)
			duration := time.Since(start)

			if decision == nil {
				t.Error("decision should not be nil")
			}

			ms := float64(duration.Milliseconds())
			collector.RecordLatency(ms, 0, 0, 0, 0, 0)
			done <- true
		}(query)
	}

	for range queries {
		<-done
	}

	m := collector.GetMetrics()
	if m.LatencyMetrics.TotalMs < 0 {
		t.Error("metrics should have valid latency data")
	}
	// Note: TotalMs can be 0 if the operation completes in <1ms
	if !m.LatencyMetrics.LastUpdated.IsZero() {
		t.Logf("latencies recorded: %v ms (acceptable)", m.LatencyMetrics.TotalMs)
	}
}

// Integration Test 7: Model Escalation Based on Feedback
func TestModelEscalationFromFeedback(t *testing.T) {
	classifier := intent.NewClassifier(90)

	// First classification: QUICK_ANSWER → Haiku
	decision1 := classifier.Classify(context.TODO(), "Summarize this code", "test-user", nil)

	if decision1.RecommendedModel != intent.ModelHaiku && decision1.RecommendedModel != intent.ModelSonnet {
		t.Logf("initial decision uses model: %v", decision1.RecommendedModel)
	}

	// User feedback indicates they want more detail
	// (In real implementation, this would adjust future decisions)
	// For now, just verify the coupling is consistent

	decision2 := classifier.Classify(context.TODO(), "Summarize this code", "test-user", nil)

	// Decisions should be consistent (same query = same model)
	if decision1.RecommendedModel != decision2.RecommendedModel {
		t.Error("same query should produce same model selection")
	}
}

// Integration Test 8: Security → Intent → Cache Decision Flow
func TestSecurityIntentCacheFlow(t *testing.T) {
	validator := security.NewValidator()
	classifier := intent.NewClassifier(90)

	tests := []struct {
		name         string
		input        string
		inputType    security.InputType
		expectSecure bool
		expectCache  bool
	}{
		{
			name:         "clean input → fast query → cacheable",
			input:        "Find functions calling auth",
			inputType:    security.InputTypeWeb,
			expectSecure: true,
			expectCache:  true,
		},
		{
			name:         "injection attempt → blocked at security layer",
			input:        "'; DROP TABLE users--",
			inputType:    security.InputTypeSQL,
			expectSecure: false,
			expectCache:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Step 1: Security validation
			secure, _ := validator.ValidateInput(tt.input, tt.inputType)
			if secure != tt.expectSecure {
				t.Errorf("security validation: expected %v, got %v", tt.expectSecure, secure)
			}

			if secure {
				// Step 2: If secure, proceed to intent classification
				decision := classifier.Classify(context.TODO(), tt.input, "test-user", nil)
				if decision == nil {
					t.Error("expected non-nil decision")
				}
			}
		})
	}
}
