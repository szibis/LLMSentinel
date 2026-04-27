package batch

import (
	"context"
	"testing"
	"time"
)

func TestNewNonInteractiveDetector(t *testing.T) {
	detector := NewNonInteractiveDetector()
	if detector.requestCountWindow != 30*time.Second {
		t.Errorf("expected window 30s, got %v", detector.requestCountWindow)
	}
	if detector.requestCountThreshold != 5 {
		t.Errorf("expected threshold 5, got %d", detector.requestCountThreshold)
	}
}

func TestIsNonInteractiveByIntent(t *testing.T) {
	detector := NewNonInteractiveDetector()
	ctx := context.Background()

	tests := []struct {
		intent   string
		expected bool
	}{
		{"batch_analysis", true},
		{"bulk_processing", true},
		{"overnight_job", true},
		{"interactive", false},
		{"real_time", false},
	}

	for _, test := range tests {
		decision := detector.IsNonInteractive(ctx, "test query", test.intent)
		if test.expected && !decision.ShouldBatch {
			t.Errorf("intent %q should be non-interactive", test.intent)
		}
	}
}

func TestIsNonInteractiveByQuery(t *testing.T) {
	detector := NewNonInteractiveDetector()
	ctx := context.Background()

	tests := []struct {
		query    string
		expected bool
	}{
		{"analyze all files", true},
		{"process all requests", true},
		{"bulk export", true},
		{"find one function", false},
		{"what is this", false},
	}

	for _, test := range tests {
		decision := detector.IsNonInteractive(ctx, test.query, "analysis")
		if test.expected && !decision.ShouldBatch {
			t.Errorf("query %q should detect bulk processing", test.query)
		}
	}
}

func TestIsNonInteractiveByVolume(t *testing.T) {
	detector := NewNonInteractiveDetector()
	detector.requestCountThreshold = 2
	ctx := context.Background()

	// Send 3 requests rapidly
	now := time.Now()
	detector.mu.Lock()
	for i := 0; i < 3; i++ {
		detector.recordRequest(now.Add(time.Duration(i) * 100 * time.Millisecond))
	}
	detector.mu.Unlock()

	// Check that high volume is detected
	decision := detector.IsNonInteractive(ctx, "query", "neutral_intent")
	if !decision.ShouldBatch {
		t.Error("high request volume should trigger batching")
	}
}

func TestRecordRequest(t *testing.T) {
	detector := NewNonInteractiveDetector()
	now := time.Now()

	detector.mu.Lock()
	detector.recordRequest(now)
	detector.recordRequest(now.Add(100 * time.Millisecond))
	detector.recordRequest(now.Add(200 * time.Millisecond))
	detector.mu.Unlock()

	if len(detector.recentRequests) != 3 {
		t.Errorf("expected 3 requests, got %d", len(detector.recentRequests))
	}
}

func TestCountRecentRequests(t *testing.T) {
	detector := NewNonInteractiveDetector()
	detector.requestCountWindow = 1 * time.Second
	now := time.Now()

	detector.mu.Lock()
	// Add requests within window
	detector.recordRequest(now)
	detector.recordRequest(now.Add(500 * time.Millisecond))

	// Add request outside window
	detector.recordRequest(now.Add(-2 * time.Second))
	detector.mu.Unlock()

	detector.mu.RLock()
	count := detector.countRecentRequests(now)
	detector.mu.RUnlock()

	if count != 2 {
		t.Errorf("expected 2 requests in window, got %d", count)
	}
}

func TestNewBatchEligibilityChecker(t *testing.T) {
	checker := NewBatchEligibilityChecker(10, 100, 5*time.Minute)
	if checker.minBatchSize != 10 {
		t.Errorf("expected minBatchSize 10, got %d", checker.minBatchSize)
	}
	if checker.maxBatchSize != 100 {
		t.Errorf("expected maxBatchSize 100, got %d", checker.maxBatchSize)
	}
}

func TestCanBatchByIntent(t *testing.T) {
	checker := NewBatchEligibilityChecker(10, 100, 5*time.Minute)

	tests := []struct {
		intent   string
		expected bool
	}{
		{"batch_analysis", true},
		{"bulk_process", true},
		{"interactive", false},
		{"urgent", false},
		{"analysis", true}, // neutral intent
	}

	for _, test := range tests {
		result := checker.CanBatch(test.intent, 5000, 5*time.Minute)
		if result != test.expected {
			t.Errorf("CanBatch(%q) = %v, expected %v", test.intent, result, test.expected)
		}
	}
}

func TestCanBatchByTokenSize(t *testing.T) {
	checker := NewBatchEligibilityChecker(10, 100, 5*time.Minute)

	// Too small for batching
	result := checker.CanBatch("batch_analysis", 100, 5*time.Minute)
	if result {
		t.Error("small token size should not be eligible for batching")
	}

	// Adequate size
	result = checker.CanBatch("batch_analysis", 5000, 5*time.Minute)
	if !result {
		t.Error("adequate token size should be eligible for batching")
	}
}

func TestCanBatchByResponseTime(t *testing.T) {
	checker := NewBatchEligibilityChecker(10, 100, 5*time.Minute)

	// User expects immediate response
	result := checker.CanBatch("batch_analysis", 5000, 5*time.Second)
	if result {
		t.Error("low response time expectation should not be eligible for batching")
	}

	// User expects delayed response
	result = checker.CanBatch("batch_analysis", 5000, 10*time.Minute)
	if !result {
		t.Error("high response time expectation should be eligible for batching")
	}
}

func TestNewWorkloadAnalyzer(t *testing.T) {
	analyzer := NewWorkloadAnalyzer()
	if analyzer.detector == nil {
		t.Error("detector should be initialized")
	}
	if analyzer.checker == nil {
		t.Error("checker should be initialized")
	}
}

func TestAnalyzeRequest(t *testing.T) {
	analyzer := NewWorkloadAnalyzer()
	ctx := context.Background()

	// Non-interactive request that meets all criteria
	decision := analyzer.AnalyzeRequest(ctx, "analyze all files", "batch_analysis", 5000, 10*time.Minute)
	if !decision.ShouldBatch {
		t.Error("request should be eligible for batching")
	}

	// Interactive request
	decision = analyzer.AnalyzeRequest(ctx, "what is this", "interactive", 5000, 1*time.Second)
	if decision.ShouldBatch {
		t.Error("interactive request should not be eligible for batching")
	}
}

func TestGetMetrics(t *testing.T) {
	analyzer := NewWorkloadAnalyzer()
	ctx := context.Background()

	// Generate some requests
	for i := 0; i < 3; i++ {
		analyzer.AnalyzeRequest(ctx, "test query", "batch_analysis", 5000, 10*time.Minute)
	}

	metrics := analyzer.GetMetrics()
	if metrics["recent_request_count"] == nil {
		t.Error("metrics should contain recent_request_count")
	}
}

func TestBatchDecision(t *testing.T) {
	detector := NewNonInteractiveDetector()
	ctx := context.Background()

	decision := detector.IsNonInteractive(ctx, "analyze all files", "batch_analysis")
	if decision.Confidence <= 0 {
		t.Error("confidence should be positive for batch decision")
	}
	if decision.EstimatedWaitTime == 0 {
		t.Error("estimated wait time should be set for batch decision")
	}
}
