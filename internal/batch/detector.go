package batch

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// WorkloadDetectionResult represents the output of workload detection analysis
type WorkloadDetectionResult struct {
	ShouldBatch       bool
	Confidence        float64 // 0.0-1.0
	Reason            string
	EstimatedWaitTime time.Duration
}

// WorkloadMetrics tracks request patterns for batch eligibility detection
type WorkloadMetrics struct {
	RecentRequestCount     int64
	AverageLatency         time.Duration
	LastPeakTime           time.Time
	BulkProcessingDetected bool
}

// NonInteractiveDetector determines if requests are non-interactive (batch-eligible)
type NonInteractiveDetector struct {
	mu                    sync.RWMutex
	requestCountWindow    time.Duration
	requestCountThreshold int64
	expectedResponseTime  time.Duration
	metrics               *WorkloadMetrics
	lastRequestTime       time.Time
	recentRequests        []time.Time
}

// NewNonInteractiveDetector creates a new detector with default settings
func NewNonInteractiveDetector() *NonInteractiveDetector {
	return &NonInteractiveDetector{
		requestCountWindow:    30 * time.Second,
		requestCountThreshold: 5, // 5+ requests in 30 seconds = bulk workload
		expectedResponseTime:  5 * time.Minute,
		metrics: &WorkloadMetrics{
			RecentRequestCount: 0,
			LastPeakTime:       time.Now(),
		},
		recentRequests: make([]time.Time, 0, 100),
	}
}

// IsNonInteractive evaluates if a request is non-interactive and suitable for batching
func (nid *NonInteractiveDetector) IsNonInteractive(ctx context.Context, query string, intent string) WorkloadDetectionResult {
	nid.mu.Lock()
	defer nid.mu.Unlock()

	now := time.Now()
	nid.recordRequest(now)

	decision := WorkloadDetectionResult{
		ShouldBatch: false,
		Confidence:  0.0,
		Reason:      "Interactive request",
	}

	// Check 1: Intent-based heuristics
	if nid.isNonInteractiveIntent(intent) {
		decision.Confidence += 0.7
		decision.Reason = fmt.Sprintf("Non-interactive intent: %s", intent)
	}

	// Check 2: Request volume heuristics
	recentCount := nid.countRecentRequests(now)
	if recentCount >= nid.requestCountThreshold {
		decision.Confidence += 0.6
		if decision.Reason == "Interactive request" {
			decision.Reason = fmt.Sprintf("High request volume: %d in %v", recentCount, nid.requestCountWindow)
		}
	}

	// Check 3: Query pattern analysis
	if nid.detectsBulkQuery(query) {
		decision.Confidence += 0.6
		if decision.Reason == "Interactive request" {
			decision.Reason = "Detected bulk processing pattern in query"
		}
	}

	// Finalize decision: batch if confidence >= 0.6
	decision.ShouldBatch = decision.Confidence >= 0.6

	// Set expected wait time for batch processing
	if decision.ShouldBatch {
		decision.EstimatedWaitTime = nid.expectedResponseTime
	}

	return decision
}

// recordRequest adds a request timestamp to the tracking window
func (nid *NonInteractiveDetector) recordRequest(now time.Time) {
	nid.lastRequestTime = now

	// Clean up old requests outside the window
	cutoffTime := now.Add(-nid.requestCountWindow)
	startIdx := 0
	for i, t := range nid.recentRequests {
		if t.After(cutoffTime) {
			startIdx = i
			break
		}
	}
	nid.recentRequests = nid.recentRequests[startIdx:]

	// Add new request
	nid.recentRequests = append(nid.recentRequests, now)

	// Update atomic counter
	atomic.StoreInt64(&nid.metrics.RecentRequestCount, int64(len(nid.recentRequests)))
}

// countRecentRequests returns the number of requests in the time window
func (nid *NonInteractiveDetector) countRecentRequests(now time.Time) int64 {
	cutoffTime := now.Add(-nid.requestCountWindow)
	count := int64(0)
	for _, t := range nid.recentRequests {
		if t.After(cutoffTime) {
			count++
		}
	}
	return count
}

// isNonInteractiveIntent checks if the intent type suggests non-interactive usage
func (nid *NonInteractiveDetector) isNonInteractiveIntent(intent string) bool {
	nonInteractiveIntents := map[string]bool{
		"batch_analysis":    true,
		"bulk_processing":   true,
		"overnight_job":     true,
		"scheduled_task":    true,
		"report_generation": true,
		"data_export":       true,
	}
	return nonInteractiveIntents[intent]
}

// detectsBulkQuery looks for keywords indicating bulk processing
func (nid *NonInteractiveDetector) detectsBulkQuery(query string) bool {
	bulkKeywords := []string{
		"analyze all",
		"process all",
		"bulk",
		"batch",
		"all files",
		"all functions",
		"multiple",
		"export all",
		"generate all",
		"all requests",
	}

	lowerQuery := toLower(query)
	for _, keyword := range bulkKeywords {
		if contains(lowerQuery, keyword) {
			return true
		}
	}
	return false
}

// Helper function to check substring (case-insensitive via toLower)
func contains(s, substr string) bool {
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Helper function to convert to lowercase
func toLower(s string) string {
	result := make([]byte, len(s))
	for i, b := range []byte(s) {
		if b >= 'A' && b <= 'Z' {
			result[i] = b + 32
		} else {
			result[i] = b
		}
	}
	return string(result)
}

// BatchEligibilityChecker determines if a request can be batched
type BatchEligibilityChecker struct {
	maxBatchSize   int
	minBatchSize   int
	maxWaitTime    time.Duration
	allowedIntents map[string]bool
	blockedIntents map[string]bool
}

// NewBatchEligibilityChecker creates a new eligibility checker
func NewBatchEligibilityChecker(minBatchSize, maxBatchSize int, maxWaitTime time.Duration) *BatchEligibilityChecker {
	return &BatchEligibilityChecker{
		maxBatchSize: maxBatchSize,
		minBatchSize: minBatchSize,
		maxWaitTime:  maxWaitTime,
		allowedIntents: map[string]bool{
			"batch_analysis": true,
			"bulk_process":   true,
			"overnight":      true,
			"scheduled":      true,
		},
		blockedIntents: map[string]bool{
			"interactive": true,
			"real_time":   true,
			"synchronous": true,
			"urgent":      true,
		},
	}
}

// CanBatch checks if a request meets all eligibility criteria
func (bec *BatchEligibilityChecker) CanBatch(intent string, estimatedTokens int, responseTimeExpectation time.Duration) bool {
	// Check intent
	if bec.blockedIntents[intent] {
		return false
	}

	if !bec.allowedIntents[intent] && !isNeutralIntent(intent) {
		return false // Not explicitly allowed and not neutral
	}

	// Check estimated tokens
	if estimatedTokens < bec.minBatchSize*100 {
		return false // Too small for batching overhead
	}

	// Check expected response time
	if responseTimeExpectation > 0 && responseTimeExpectation < 30*time.Second {
		return false // User expects immediate response
	}

	return true
}

// Helper to identify neutral intents that can go either way
func isNeutralIntent(intent string) bool {
	neutralIntents := map[string]bool{
		"analysis":   true,
		"processing": true,
		"lookup":     true,
		"search":     true,
	}
	return neutralIntents[intent]
}

// WorkloadAnalyzer provides detailed workload pattern analysis
type WorkloadAnalyzer struct {
	detector *NonInteractiveDetector
	checker  *BatchEligibilityChecker
}

// NewWorkloadAnalyzer creates a new analyzer
func NewWorkloadAnalyzer() *WorkloadAnalyzer {
	return &WorkloadAnalyzer{
		detector: NewNonInteractiveDetector(),
		checker:  NewBatchEligibilityChecker(10, 100, 5*time.Minute),
	}
}

// AnalyzeRequest returns a comprehensive decision on batch eligibility
func (wa *WorkloadAnalyzer) AnalyzeRequest(ctx context.Context, query string, intent string, estimatedTokens int, responseTimeExpectation time.Duration) WorkloadDetectionResult {
	// Get non-interactive detection result
	decision := wa.detector.IsNonInteractive(ctx, query, intent)

	// Check eligibility constraints
	if !wa.checker.CanBatch(intent, estimatedTokens, responseTimeExpectation) {
		decision.ShouldBatch = false
		decision.Confidence = 0.0
		decision.Reason = "Request does not meet batch eligibility criteria"
	}

	return decision
}

// GetMetrics returns current workload metrics
func (wa *WorkloadAnalyzer) GetMetrics() map[string]interface{} {
	wa.detector.mu.RLock()
	defer wa.detector.mu.RUnlock()

	return map[string]interface{}{
		"recent_request_count": atomic.LoadInt64(&wa.detector.metrics.RecentRequestCount),
		"average_latency_ms":   wa.detector.metrics.AverageLatency.Milliseconds(),
		"last_peak_time":       wa.detector.metrics.LastPeakTime,
		"bulk_detected":        wa.detector.metrics.BulkProcessingDetected,
	}
}
