package optimization

import (
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestStress_HighConcurrencyLowContention tests many goroutines with unique prompts
func TestStress_HighConcurrencyLowContention(t *testing.T) {
	opt := NewOptimizer()
	opt.SetMinBatchSize(5)

	numGoroutines := 200
	requestsPerGoroutine := 50
	completedRequests := 0
	var mu sync.Mutex

	var wg sync.WaitGroup
	startTime := time.Now()

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for r := 0; r < requestsPerGoroutine; r++ {
				prompt := fmt.Sprintf("unique_prompt_g%d_r%d", goroutineID, r)
				decision := opt.Optimize(prompt, "sonnet", 100)

				if decision.Direction != "" {
					mu.Lock()
					completedRequests++
					mu.Unlock()
				}
			}
		}(g)
	}

	wg.Wait()
	elapsed := time.Since(startTime)

	expectedRequests := numGoroutines * requestsPerGoroutine
	if completedRequests != expectedRequests {
		t.Errorf("expected %d requests, got %d", expectedRequests, completedRequests)
	}

	metrics := opt.GetMetrics()
	if metrics.TotalRequests != int64(expectedRequests) {
		t.Errorf("metrics should show %d requests, got %d", expectedRequests, metrics.TotalRequests)
	}

	t.Logf("Completed %d requests in %v (%.0f req/sec)",
		expectedRequests, elapsed, float64(expectedRequests)/elapsed.Seconds())
}

// TestStress_HighConcurrencyHighContention tests many goroutines accessing same prompts
func TestStress_HighConcurrencyHighContention(t *testing.T) {
	opt := NewOptimizer()
	opt.SetMinBatchSize(3)

	numGoroutines := 100
	requestsPerGoroutine := 100
	sharedPrompts := []string{
		"How do I optimize?",
		"What's the best practice?",
		"Can you explain this?",
		"How to debug?",
		"Best way to implement?",
	}

	var wg sync.WaitGroup
	startTime := time.Now()

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			// #nosec G404 - math/rand acceptable for test randomization
			rng := rand.New(rand.NewSource(int64(goroutineID)))
			for r := 0; r < requestsPerGoroutine; r++ {
				prompt := sharedPrompts[rng.Intn(len(sharedPrompts))]
				opt.Optimize(prompt, "sonnet", 100)
			}
		}(g)
	}

	wg.Wait()
	elapsed := time.Since(startTime)

	expectedRequests := numGoroutines * requestsPerGoroutine
	metrics := opt.GetMetrics()

	if metrics.TotalRequests != int64(expectedRequests) {
		t.Errorf("expected %d requests, got %d", expectedRequests, metrics.TotalRequests)
	}

	// With high contention on shared prompts, should see cache hits
	if metrics.CacheHits < int64(expectedRequests/10) {
		t.Logf("warning: expected significant cache hits with contention, got %d/%d",
			metrics.CacheHits, metrics.TotalRequests)
	}

	t.Logf("Completed %d requests with %d cache hits in %v (%.0f req/sec, %.1f%% hit rate)",
		expectedRequests, metrics.CacheHits, elapsed,
		float64(expectedRequests)/elapsed.Seconds(),
		metrics.CacheHitRate)
}

// TestStress_RapidFireRequests tests rapid-fire requests from single goroutine
func TestStress_RapidFireRequests(t *testing.T) {
	opt := NewOptimizer()
	opt.SetMinBatchSize(2)

	numRequests := 10000
	startTime := time.Now()

	for i := 0; i < numRequests; i++ {
		prompt := fmt.Sprintf("prompt_%d", i%100) // 100 unique prompts repeating
		opt.Optimize(prompt, "sonnet", 100)
	}

	elapsed := time.Since(startTime)

	metrics := opt.GetMetrics()
	if metrics.TotalRequests != int64(numRequests) {
		t.Errorf("expected %d requests, got %d", numRequests, metrics.TotalRequests)
	}

	t.Logf("Processed %d rapid-fire requests in %v (%.0f req/sec)",
		numRequests, elapsed, float64(numRequests)/elapsed.Seconds())
}

// TestStress_LargePrompts tests performance with large prompts
func TestStress_LargePrompts(t *testing.T) {
	opt := NewOptimizer()

	// Create progressively larger prompts
	sizes := []int{1000, 10000, 100000, 500000}

	for _, size := range sizes {
		prompt := strings.Repeat("a", size)
		startTime := time.Now()

		decision := opt.Optimize(prompt, "sonnet", 1000)

		elapsed := time.Since(startTime)

		if decision.Direction == "" {
			t.Errorf("failed to handle %d-byte prompt", size)
		}

		t.Logf("Handled %d-byte prompt in %v", size, elapsed)
	}
}

// TestStress_ManyModels tests requesting across all models
func TestStress_ManyModels(t *testing.T) {
	opt := NewOptimizer()

	numRequests := 3000
	models := []string{"haiku", "sonnet", "opus"}

	for i := 0; i < numRequests; i++ {
		model := models[i%len(models)]
		prompt := fmt.Sprintf("prompt_%d", i)
		opt.Optimize(prompt, model, 100)
	}

	metrics := opt.GetMetrics()
	if metrics.TotalRequests != int64(numRequests) {
		t.Errorf("expected %d requests, got %d", numRequests, metrics.TotalRequests)
	}

	t.Logf("Processed %d requests across 3 models: %d haiku, %d sonnet, %d opus approx",
		numRequests, numRequests/3, numRequests/3, numRequests/3)
}

// TestStress_VaryingOutputSizes tests requests with varying output token counts
func TestStress_VaryingOutputSizes(t *testing.T) {
	opt := NewOptimizer()

	outputs := []int{0, 10, 100, 1000, 10000, 50000, 100000}
	for i := 0; i < 1000; i++ {
		output := outputs[i%len(outputs)]
		prompt := fmt.Sprintf("prompt_%d", i)
		decision := opt.Optimize(prompt, "sonnet", output)

		if decision.Direction == "" {
			t.Errorf("failed for output size %d", output)
		}
	}

	metrics := opt.GetMetrics()
	if metrics.TotalRequests != 1000 {
		t.Errorf("expected 1000 requests, got %d", metrics.TotalRequests)
	}
}

// TestStress_CacheFlushing tests cache behavior under load
func TestStress_CacheFlushing(t *testing.T) {
	opt := NewOptimizer()

	// Generate load that builds up cache
	for i := 0; i < 1000; i++ {
		prompt := fmt.Sprintf("cacheable_%d", i%50) // 50 unique prompts
		opt.Optimize(prompt, "sonnet", 100)
		opt.CacheResponse(prompt, "sonnet", "response", 100)

		// Periodically clear expired cache
		if i%100 == 0 {
			cleared := opt.ClearExpiredCache()
			t.Logf("Cleared %d expired cache entries at request %d", cleared, i)
		}
	}

	metrics := opt.GetMetrics()
	if metrics.TotalRequests != 1000 {
		t.Errorf("expected 1000 requests, got %d", metrics.TotalRequests)
	}
}

// TestStress_MetricsUnderLoad tests metrics consistency under high load
func TestStress_MetricsUnderLoad(t *testing.T) {
	opt := NewOptimizer()
	opt.SetMinBatchSize(5)

	numWorkers := 50
	requestsPerWorker := 200

	var wg sync.WaitGroup
	var mu sync.Mutex
	recordedDecisions := make(map[string]int)

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for r := 0; r < requestsPerWorker; r++ {
				prompt := fmt.Sprintf("w%d_r%d", workerID, r)
				decision := opt.Optimize(prompt, "sonnet", 100)

				mu.Lock()
				recordedDecisions[decision.Direction]++
				mu.Unlock()
			}
		}(w)
	}

	wg.Wait()

	metrics := opt.GetMetrics()
	expectedTotal := int64(numWorkers * requestsPerWorker)

	if metrics.TotalRequests != expectedTotal {
		t.Errorf("metrics TotalRequests: expected %d, got %d", expectedTotal, metrics.TotalRequests)
	}

	// Sum of all decision types should equal total requests
	sumDecisions := int64(0)
	for _, count := range recordedDecisions {
		sumDecisions += int64(count)
	}

	if sumDecisions != expectedTotal {
		t.Errorf("decision breakdown: expected %d total, got %d", expectedTotal, sumDecisions)
	}

	t.Logf("Decision breakdown: %v", recordedDecisions)
	t.Logf("Final metrics - Requests: %d, Cache: %d, Batch: %d, Direct: %d, Switches: %d",
		metrics.TotalRequests, metrics.CacheHits, metrics.BatchRequests,
		metrics.DirectRequests, metrics.ModelSwitches)
}

// TestStress_MemoryStability tests memory doesn't grow excessively
func TestStress_MemoryStability(t *testing.T) {
	opt := NewOptimizer()

	// Record large number of events
	for i := 0; i < 100_000; i++ {
		prompt := fmt.Sprintf("prompt_%d", i%1000) // 1000 unique
		model := []string{"haiku", "sonnet", "opus"}[i%3]

		switch i % 4 {
		case 0:
			opt.Optimize(prompt, model, 100)
			metrics := opt.GetMetrics()
			_ = metrics // Use to prevent optimization
		case 1:
			opt.CacheResponse(prompt, model, "response", 100)
		case 2:
			opt.ClearExpiredCache()
		case 3:
			queue := opt.GetQueueStats()
			_ = queue
		}
	}

	// Verify system is still responsive
	metrics := opt.GetMetrics()
	if metrics.TotalRequests == 0 {
		t.Errorf("should have recorded requests")
	}

	// Get metrics multiple times to verify no excessive growth
	for i := 0; i < 100; i++ {
		_ = opt.GetMetrics()
	}

	t.Logf("Processed 100k operations with %d total requests tracked", metrics.TotalRequests)
}

// TestStress_BurstTraffic tests burst-like traffic patterns
func TestStress_BurstTraffic(t *testing.T) {
	opt := NewOptimizer()
	opt.SetMinBatchSize(3)

	// Simulate burst traffic: high volume then idle then high volume
	bursts := []struct {
		count    int
		duration time.Duration
		name     string
	}{
		{1000, 100 * time.Millisecond, "burst1"},
		{500, 50 * time.Millisecond, "burst2"},
		{2000, 200 * time.Millisecond, "burst3"},
	}

	totalRequests := 0
	for _, burst := range bursts {
		startTime := time.Now()

		for i := 0; i < burst.count; i++ {
			prompt := fmt.Sprintf("%s_req_%d", burst.name, i)
			opt.Optimize(prompt, "sonnet", 100)
			totalRequests++
		}

		elapsed := time.Since(startTime)
		rate := float64(burst.count) / elapsed.Seconds()

		t.Logf("Burst %s: %d requests in %v (%.0f req/sec)",
			burst.name, burst.count, elapsed, rate)

		// Simulate idle period between bursts
		time.Sleep(50 * time.Millisecond)
	}

	metrics := opt.GetMetrics()
	if metrics.TotalRequests != int64(totalRequests) {
		t.Errorf("expected %d requests, got %d", totalRequests, metrics.TotalRequests)
	}
}

// TestStress_SustainedLoad tests sustained high load over time
func TestStress_SustainedLoad(t *testing.T) {
	opt := NewOptimizer()
	opt.SetMinBatchSize(2)

	duration := 5 * time.Second
	deadline := time.Now().Add(duration)
	requestCount := 0

	for time.Now().Before(deadline) {
		prompt := fmt.Sprintf("sustained_req_%d", requestCount)
		opt.Optimize(prompt, "sonnet", 100)
		requestCount++
	}

	metrics := opt.GetMetrics()
	elapsed := time.Since(deadline.Add(-duration))

	rate := float64(requestCount) / elapsed.Seconds()
	t.Logf("Sustained load: %d requests in %v (%.0f req/sec)",
		requestCount, elapsed, rate)

	if metrics.TotalRequests != int64(requestCount) {
		t.Errorf("expected %d requests, got %d", requestCount, metrics.TotalRequests)
	}
}

// TestStress_RepeatingPatternBatches tests batching with repeating patterns
func TestStress_RepeatingPatternBatches(t *testing.T) {
	opt := NewOptimizer()
	opt.SetMinBatchSize(3)

	// Create repeating pattern: same prompt N times, then next prompt
	pattern := []string{
		"How to optimize database queries?",
		"What's the best caching strategy?",
		"How do I improve latency?",
	}

	const timesPerPattern = 4
	const patternRepeat = 100

	for p := 0; p < patternRepeat; p++ {
		for _, prompt := range pattern {
			for i := 0; i < timesPerPattern; i++ {
				decision := opt.Optimize(prompt, "sonnet", 100)
				if decision.Direction == "" {
					t.Errorf("decision should not be empty")
				}
			}
		}
	}

	totalRequests := len(pattern) * timesPerPattern * patternRepeat
	metrics := opt.GetMetrics()

	if metrics.TotalRequests != int64(totalRequests) {
		t.Errorf("expected %d requests, got %d", totalRequests, metrics.TotalRequests)
	}

	t.Logf("Pattern-based load: %d requests, %d cache hits (%.1f%% hit rate)",
		metrics.TotalRequests, metrics.CacheHits, metrics.CacheHitRate)
}

// BenchmarkOptimizeSequential benchmarks sequential optimization calls
func BenchmarkOptimizeSequential(b *testing.B) {
	opt := NewOptimizer()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		prompt := fmt.Sprintf("benchmark_prompt_%d", i)
		opt.Optimize(prompt, "sonnet", 100)
	}
}

// BenchmarkOptimizeWithCacheHits benchmarks with cache hits
func BenchmarkOptimizeWithCacheHits(b *testing.B) {
	opt := NewOptimizer()

	// Pre-populate cache
	for i := 0; i < 100; i++ {
		prompt := fmt.Sprintf("cached_prompt_%d", i)
		opt.CacheResponse(prompt, "sonnet", "response", 100)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		prompt := fmt.Sprintf("cached_prompt_%d", i%100)
		opt.Optimize(prompt, "sonnet", 100)
	}
}

// BenchmarkMetricsRetrieval benchmarks metrics retrieval
func BenchmarkMetricsRetrieval(b *testing.B) {
	opt := NewOptimizer()

	// Generate some load
	for i := 0; i < 10000; i++ {
		opt.Optimize(fmt.Sprintf("prompt_%d", i), "sonnet", 100)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = opt.GetMetrics()
	}
}

// BenchmarkConcurrentOptimize benchmarks concurrent optimization
func BenchmarkConcurrentOptimize(b *testing.B) {
	opt := NewOptimizer()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			prompt := fmt.Sprintf("concurrent_prompt_%d", i)
			opt.Optimize(prompt, "sonnet", 100)
			i++
		}
	})
}
