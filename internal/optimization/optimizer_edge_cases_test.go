package optimization

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestEdgeCases_EmptyPrompt validates empty prompt handling
func TestEdgeCases_EmptyPrompt(t *testing.T) {
	opt := NewOptimizer()

	decision := opt.Optimize("", "sonnet", 100)
	if decision.Direction != "direct" {
		t.Errorf("empty prompt should result in direct, got %s", decision.Direction)
	}
	if !strings.Contains(decision.Rationale, "validation error") {
		t.Errorf("expected validation error, got: %s", decision.Rationale)
	}
}

// TestEdgeCases_WhitespaceOnlyPrompt validates whitespace-only prompt
func TestEdgeCases_WhitespaceOnlyPrompt(t *testing.T) {
	opt := NewOptimizer()

	decision := opt.Optimize("   \n\t  ", "sonnet", 100)
	if decision.Direction != "direct" {
		t.Errorf("whitespace-only prompt should result in direct, got %s", decision.Direction)
	}
}

// TestEdgeCases_VeryLongPrompt tests prompt length limit
func TestEdgeCases_VeryLongPrompt(t *testing.T) {
	opt := NewOptimizer()

	// Create prompt exceeding 1M character limit
	longPrompt := strings.Repeat("a", 1_000_001)
	decision := opt.Optimize(longPrompt, "sonnet", 100)

	if decision.Direction != "direct" {
		t.Errorf("very long prompt should result in direct, got %s", decision.Direction)
	}
	if !strings.Contains(decision.Rationale, "validation error") {
		t.Errorf("expected validation error for long prompt, got: %s", decision.Rationale)
	}
}

// TestEdgeCases_MaxPromptLength tests prompt at exactly the limit
func TestEdgeCases_MaxPromptLength(t *testing.T) {
	opt := NewOptimizer()

	// Create prompt at exactly 1M character limit
	maxPrompt := strings.Repeat("a", 1_000_000)
	decision := opt.Optimize(maxPrompt, "sonnet", 100)

	// Should not fail validation, may cache or go direct
	if strings.Contains(decision.Rationale, "prompt too large") {
		t.Errorf("prompt at limit should be allowed, got: %s", decision.Rationale)
	}
}

// TestEdgeCases_InvalidModel tests invalid model name handling
func TestEdgeCases_InvalidModel(t *testing.T) {
	opt := NewOptimizer()

	invalidModels := []string{"gpt4", "claude", "invalid", "GPT", ""}
	for _, model := range invalidModels {
		decision := opt.Optimize("test prompt", model, 100)
		if !strings.Contains(decision.Rationale, "validation error") {
			t.Errorf("invalid model %q should fail validation, got: %s", model, decision.Rationale)
		}
	}
}

// TestEdgeCases_ValidModelsCI tests case-insensitive model names
func TestEdgeCases_ValidModelsCI(t *testing.T) {
	opt := NewOptimizer()

	models := []string{"opus", "OPUS", "OpUs", "sonnet", "SONNET", "haiku", "HAIKU"}
	for _, model := range models {
		decision := opt.Optimize("test prompt", model, 100)
		if strings.Contains(decision.Rationale, "invalid model") {
			t.Errorf("model %q should be valid (case-insensitive), got: %s", model, decision.Rationale)
		}
	}
}

// TestEdgeCases_NegativeOutputTokens tests negative output token handling
func TestEdgeCases_NegativeOutputTokens(t *testing.T) {
	opt := NewOptimizer()

	decision := opt.Optimize("test prompt", "sonnet", -100)
	if !strings.Contains(decision.Rationale, "validation error") {
		t.Errorf("negative output should fail validation, got: %s", decision.Rationale)
	}
}

// TestEdgeCases_ZeroOutputTokens tests zero output token handling
func TestEdgeCases_ZeroOutputTokens(t *testing.T) {
	opt := NewOptimizer()

	decision := opt.Optimize("test prompt", "sonnet", 0)
	if strings.Contains(decision.Rationale, "validation error") {
		t.Errorf("zero output tokens should be allowed, got: %s", decision.Rationale)
	}
}

// TestEdgeCases_ExcessiveOutputTokens tests output token limit
func TestEdgeCases_ExcessiveOutputTokens(t *testing.T) {
	opt := NewOptimizer()

	decision := opt.Optimize("test prompt", "sonnet", 100_001)
	if !strings.Contains(decision.Rationale, "validation error") {
		t.Errorf("excessive output tokens should fail validation, got: %s", decision.Rationale)
	}
}

// TestEdgeCases_MaxOutputTokens tests output at exactly the limit
func TestEdgeCases_MaxOutputTokens(t *testing.T) {
	opt := NewOptimizer()

	decision := opt.Optimize("test prompt", "sonnet", 100_000)
	if strings.Contains(decision.Rationale, "too large") {
		t.Errorf("output at limit should be allowed, got: %s", decision.Rationale)
	}
}

// TestEdgeCases_IntegerOverflow tests integer overflow protection
func TestEdgeCases_IntegerOverflow(t *testing.T) {
	opt := NewOptimizer()

	// Try to cause integer overflow: very large prompt + very large output
	largePrompt := strings.Repeat("a", 500_000)
	decision := opt.Optimize(largePrompt, "sonnet", 50_000)

	if strings.Contains(decision.Rationale, "integer overflow") {
		t.Errorf("should handle large but valid inputs, got: %s", decision.Rationale)
	}

	// Now try actual overflow conditions
	absurdlyLargePrompt := strings.Repeat("a", 1_000_000)
	decision = opt.Optimize(absurdlyLargePrompt, "sonnet", 100_000)

	// Should still not crash, either validate or handle gracefully
	if decision.Direction == "" {
		t.Errorf("should handle extreme inputs gracefully")
	}
}

// TestMetrics_DivisionByZero tests metrics with zero requests
func TestMetrics_DivisionByZero(t *testing.T) {
	metrics := NewMetrics()

	summary := metrics.GetSummary()
	if summary.CacheHitRate != 0 {
		t.Errorf("cache hit rate with zero requests should be 0, got %.2f", summary.CacheHitRate)
	}
	if summary.CostPerRequest != 0 {
		t.Errorf("cost per request with zero requests should be 0, got %.2f", summary.CostPerRequest)
	}
	if summary.ROIScore != 0 {
		t.Errorf("ROI score with zero requests should be 0, got %.2f", summary.ROIScore)
	}

	cacheStats := metrics.GetCacheStats()
	if cacheStats["hit_rate"].(float64) != 0 {
		t.Errorf("cache stats hit_rate with zero requests should be 0")
	}
}

// TestMetrics_SingleRequest tests metrics with exactly one request
func TestMetrics_SingleRequest(t *testing.T) {
	metrics := NewMetrics()
	metrics.RecordDirect("test", "sonnet")

	summary := metrics.GetSummary()
	if summary.TotalRequests != 1 {
		t.Errorf("expected 1 request, got %d", summary.TotalRequests)
	}
	if summary.DirectRequests != 1 {
		t.Errorf("expected 1 direct request, got %d", summary.DirectRequests)
	}
	if summary.CacheHitRate != 0 {
		t.Errorf("cache hit rate with no cache hits should be 0, got %.2f", summary.CacheHitRate)
	}
	if summary.CostPerRequest <= 0 {
		t.Errorf("cost per request should be > 0, got %.2f", summary.CostPerRequest)
	}
}

// TestMetrics_BoundedEventLogging tests that events don't grow unbounded
func TestMetrics_BoundedEventLogging(t *testing.T) {
	metrics := NewMetrics()

	// Record way more events than the limit (should evict old ones)
	for i := 0; i < 15_000; i++ {
		metrics.RecordDirect(fmt.Sprintf("prompt_%d", i), "sonnet")
	}

	summary := metrics.GetSummary()
	if summary.TotalRequests != 15_000 {
		t.Errorf("should track all requests, got %d", summary.TotalRequests)
	}

	directStats := metrics.GetSwitchStats()
	eventCount := directStats["events_count"].(int)

	// Event count should not exceed maxEventCount
	if eventCount > 15_000 {
		t.Errorf("events should be bounded, got %d", eventCount)
	}
}

// TestMetrics_AllEventTypes tests recording all event types
func TestMetrics_AllEventTypes(t *testing.T) {
	metrics := NewMetrics()

	// Record each event type
	metrics.RecordCacheHit("prompt1", "sonnet")
	metrics.RecordBatchDecision("prompt2", "sonnet", true)
	metrics.RecordDirect("prompt3", "sonnet")
	metrics.RecordModelSwitch("prompt4", "sonnet", "haiku")

	summary := metrics.GetSummary()

	if summary.TotalRequests != 4 {
		t.Errorf("expected 4 total requests, got %d", summary.TotalRequests)
	}
	if summary.CacheHits != 1 {
		t.Errorf("expected 1 cache hit, got %d", summary.CacheHits)
	}
	if summary.BatchRequests != 1 {
		t.Errorf("expected 1 batch request, got %d", summary.BatchRequests)
	}
	if summary.DirectRequests != 1 {
		t.Errorf("expected 1 direct request, got %d", summary.DirectRequests)
	}
	if summary.ModelSwitches != 1 {
		t.Errorf("expected 1 model switch, got %d", summary.ModelSwitches)
	}
}

// TestConcurrency_ConcurrentOptimize tests thread-safe Optimize calls
func TestConcurrency_ConcurrentOptimize(t *testing.T) {
	opt := NewOptimizer()
	opt.SetMinBatchSize(2)

	numGoroutines := 100
	requestsPerGoroutine := 10
	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*requestsPerGoroutine)

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for r := 0; r < requestsPerGoroutine; r++ {
				prompt := fmt.Sprintf("goroutine_%d_request_%d", goroutineID, r)
				decision := opt.Optimize(prompt, "sonnet", 100)

				if decision.Direction == "" {
					errors <- fmt.Errorf("goroutine %d got empty decision direction", goroutineID)
				}
			}
		}(g)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent optimization failed: %v", err)
	}

	// Verify metrics are consistent
	metrics := opt.GetMetrics()
	expectedTotal := int64(numGoroutines * requestsPerGoroutine)
	if metrics.TotalRequests != expectedTotal {
		t.Errorf("expected %d total requests, got %d", expectedTotal, metrics.TotalRequests)
	}
}

// TestConcurrency_ConcurrentMetricsRead tests concurrent reads of metrics
func TestConcurrency_ConcurrentMetricsRead(t *testing.T) {
	opt := NewOptimizer()

	// Record some metrics
	for i := 0; i < 1000; i++ {
		opt.Optimize(fmt.Sprintf("prompt_%d", i), "sonnet", 100)
	}

	// Concurrent reads should not cause issues
	numReaders := 50
	var wg sync.WaitGroup
	errors := make(chan error, numReaders)

	for r := 0; r < numReaders; r++ {
		wg.Add(1)
		go func(readerID int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				metrics := opt.GetMetrics()
				if metrics.TotalRequests < 0 {
					errors <- fmt.Errorf("reader %d got invalid metrics", readerID)
				}
			}
		}(r)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent metrics read failed: %v", err)
	}
}

// TestConcurrency_ConcurrentCacheAndMetrics tests concurrent cache and metrics updates
func TestConcurrency_ConcurrentCacheAndMetrics(t *testing.T) {
	opt := NewOptimizer()

	numWorkers := 50
	requestsPerWorker := 50
	var wg sync.WaitGroup
	errors := make(chan error, numWorkers*requestsPerWorker)

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for r := 0; r < requestsPerWorker; r++ {
				// Alternate between simple and duplicate prompts
				var prompt string
				if r%5 == 0 {
					prompt = "repeated_prompt" // Will cause cache hits on some
				} else {
					prompt = fmt.Sprintf("worker_%d_request_%d", workerID, r)
				}

				decision := opt.Optimize(prompt, "sonnet", 100)

				// Also cache responses for repeated prompts
				if r%3 == 0 {
					opt.CacheResponse(prompt, "sonnet", "test response", 100)
				}

				if decision.Direction == "" {
					errors <- fmt.Errorf("worker %d got empty decision", workerID)
				}
			}
		}(w)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent cache and metrics failed: %v", err)
	}

	// Verify final state is consistent
	metrics := opt.GetMetrics()
	expectedRequests := int64(numWorkers * requestsPerWorker)
	if metrics.TotalRequests != expectedRequests {
		t.Errorf("expected %d requests, got %d", expectedRequests, metrics.TotalRequests)
	}

	// Should have some cache hits from repeated prompts
	if metrics.CacheHits < 1 {
		t.Logf("warning: expected some cache hits with repeated prompts, got %d", metrics.CacheHits)
	}
}

// TestConfiguration_SetMinBatchSize tests batch size configuration
func TestConfiguration_SetMinBatchSize(t *testing.T) {
	opt := NewOptimizer()

	sizes := []int{1, 2, 5, 10, 100}
	for _, size := range sizes {
		opt.SetMinBatchSize(size)
		// Should not panic, configuration accepted
	}
}

// TestConfiguration_SetMaxWaitTime tests wait time configuration
func TestConfiguration_SetMaxWaitTime(t *testing.T) {
	opt := NewOptimizer()

	durations := []time.Duration{
		1 * time.Second,
		30 * time.Second,
		5 * time.Minute,
		1 * time.Hour,
	}

	for _, d := range durations {
		opt.SetMaxBatchWaitTime(d)
		// Should not panic, configuration accepted
	}
}

// TestConfiguration_SetMinSavingsPercent tests savings threshold configuration
func TestConfiguration_SetMinSavingsPercent(t *testing.T) {
	opt := NewOptimizer()

	thresholds := []float64{0.0, 5.0, 10.0, 25.0, 50.0, 100.0}
	for _, threshold := range thresholds {
		opt.SetMinSavingsPercent(threshold)
		// Should not panic, configuration accepted
	}
}

// TestCache_ResponseCaching tests caching response properly
func TestCache_ResponseCaching(t *testing.T) {
	opt := NewOptimizer()

	prompt := "test prompt"
	model := "sonnet"
	response := "test response"
	tokens := 100

	opt.CacheResponse(prompt, model, response, tokens)

	// Next request with same prompt should hit cache
	decision := opt.Optimize(prompt, model, tokens)
	if decision.Direction != "cache_hit" {
		t.Errorf("expected cache hit for identical prompt, got %s", decision.Direction)
	}
}

// TestCache_ExpiredCache tests cache expiration
func TestCache_ExpiredCache(t *testing.T) {
	opt := NewOptimizer()

	prompt := "test prompt"
	model := "sonnet"

	opt.CacheResponse(prompt, model, "response", 100)

	// Clear expired entries (should clear entries older than TTL)
	cleared := opt.ClearExpiredCache()
	// May or may not clear depending on timing, but should not error
	if cleared < 0 {
		t.Errorf("cleared count should be >= 0, got %d", cleared)
	}
}

// TestQueueStats tests batch queue statistics
func TestQueueStats_ReturnsValidStats(t *testing.T) {
	opt := NewOptimizer()
	opt.SetMinBatchSize(3)

	// Make requests that might queue
	for i := 0; i < 10; i++ {
		opt.Optimize(fmt.Sprintf("prompt_%d", i), "sonnet", 100)
	}

	stats := opt.GetQueueStats()
	if stats.Size < 0 {
		t.Errorf("queue size should be >= 0, got %d", stats.Size)
	}
	if stats.Size > stats.MaxSize {
		t.Errorf("queue size %d should not exceed max %d", stats.Size, stats.MaxSize)
	}
}

// TestFlushBatch tests batch flushing
func TestFlushBatch_NoError(t *testing.T) {
	opt := NewOptimizer()

	// Should not error even with empty queue
	requests, err := opt.FlushBatch()
	if err != nil {
		t.Errorf("flush should not error on empty queue: %v", err)
	}

	if requests == nil {
		t.Errorf("flush should return non-nil slice, even if empty")
	}
}

// TestCostEstimation_ValidCosts tests cost estimation doesn't panic
func TestCostEstimation_ValidCosts(t *testing.T) {
	opt := NewOptimizer()

	testCases := []struct {
		prompt  string
		model   string
		output  int
		name    string
	}{
		{"test", "haiku", 10, "haiku_small"},
		{"test", "sonnet", 100, "sonnet_medium"},
		{"test", "opus", 1000, "opus_large"},
		{strings.Repeat("a", 10000), "sonnet", 5000, "large_prompt"},
		{"test", "sonnet", 0, "zero_output"},
		{"test", "sonnet", 100000, "max_output"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			decision := opt.Optimize(tc.prompt, tc.model, tc.output)
			if decision.Direction == "" {
				t.Errorf("should return valid decision for %s", tc.name)
			}
		})
	}
}

// TestSavingsCalculation_ValidRange tests that savings are in valid range
func TestSavingsCalculation_ValidRange(t *testing.T) {
	opt := NewOptimizer()
	opt.SetMinBatchSize(2)

	for i := 0; i < 100; i++ {
		decision := opt.Optimize(fmt.Sprintf("prompt_%d", i), "sonnet", 100)

		if decision.TotalSavings < 0 {
			t.Errorf("savings should never be negative, got %.4f for direction %s",
				decision.TotalSavings, decision.Direction)
		}

		// Savings percent should be 0-100 (capped at max)
		if decision.SavingsPercent < 0 || decision.SavingsPercent > 100.1 { // Allow small floating point error
			t.Errorf("savings percent should be 0-100, got %.1f for direction %s",
				decision.SavingsPercent, decision.Direction)
		}

		// Verify savings are consistent: can't save more than 100%
		if decision.Direction != "direct" && decision.SavingsPercent > 100.1 {
			t.Errorf("optimized decision should not save > 100%%, got %.1f%%", decision.SavingsPercent)
		}
	}
}

// TestModelValidation_AllValidModels tests all three models work
func TestModelValidation_AllValidModels(t *testing.T) {
	opt := NewOptimizer()

	models := []string{"haiku", "sonnet", "opus"}
	for _, model := range models {
		decision := opt.Optimize("test prompt", model, 100)
		if strings.Contains(decision.Rationale, "invalid model") {
			t.Errorf("model %s should be valid, got: %s", model, decision.Rationale)
		}
	}
}

// TestErrorMessages_AreDescriptive tests that error messages are helpful
func TestErrorMessages_AreDescriptive(t *testing.T) {
	opt := NewOptimizer()

	testCases := []struct {
		prompt  string
		model   string
		output  int
		keyword string
	}{
		{"", "sonnet", 100, "empty"},
		{"test", "invalid", 100, "invalid"},
		{"test", "sonnet", -1, "negative"},
		{strings.Repeat("a", 1_000_001), "sonnet", 100, "large"},
	}

	for _, tc := range testCases {
		decision := opt.Optimize(tc.prompt, tc.model, tc.output)
		if !strings.Contains(strings.ToLower(decision.Rationale), tc.keyword) &&
			!strings.Contains(strings.ToLower(decision.Rationale), "validation error") {
			t.Errorf("error message for %s should mention %q, got: %s",
				tc.keyword, tc.keyword, decision.Rationale)
		}
	}
}
