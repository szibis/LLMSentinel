// Package test provides memory leak detection tests for Claude Escalate
package test

import (
	"runtime"
	"testing"
	"time"

	"github.com/szibis/claude-escalate/internal/classify"
	"github.com/szibis/claude-escalate/internal/observability"
)

// TestMemoryLeakClassificationLearner verifies no memory leaks in classification learner
func TestMemoryLeakClassificationLearner(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping memory leak test in short mode")
	}

	// Baseline: measure memory before test
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	// Create 1000 learning events
	learner := classify.NewLearner(100, 1*time.Hour)
	for i := 0; i < 1000; i++ {
		event := classify.LearningEvent{
			ID:             string(rune(i)),
			Prompt:         "test prompt for memory leak detection",
			PredictedTask:  classify.TaskConcurrency,
			ActualTask:     classify.TaskConcurrency,
			Succeeded:      i%2 == 0,
			TokenError:     0.05,
			ConfidenceScore: 0.85,
		}
		learner.RecordOutcome(event)
	}

	// Measure memory after operations + GC
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	// Calculate heap growth percentage (allow negative growth)
	// #nosec G115 - uint64 to int64 conversion is safe; values are memory sizes within expected bounds
	heapGrowth := int64(m2.HeapAlloc) - int64(m1.HeapAlloc)
	var heapGrowthPercent float64
	if heapGrowth > 0 {
		heapGrowthPercent = (float64(heapGrowth) / float64(m1.HeapAlloc)) * 100
	}

	t.Logf("Heap growth: %d bytes (%.2f%%, baseline: %d bytes)",
		heapGrowth, heapGrowthPercent, m1.HeapAlloc)

	// Assert growth is less than 10%
	if heapGrowthPercent > 10.0 {
		t.Fatalf("memory growth exceeded threshold: %.2f%% > 10%%", heapGrowthPercent)
	}

	// Verify goroutine cleanup
	numGoroutines := runtime.NumGoroutine()
	if numGoroutines > 100 {
		t.Logf("warning: high goroutine count: %d", numGoroutines)
	}
}

// TestMemoryLeakPrometheusMetrics verifies no memory leaks in metrics collection
func TestMemoryLeakPrometheusMetrics(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping memory leak test in short mode")
	}

	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	// Create metrics and record 10K samples
	pm := observability.NewPrometheusMetrics()
	pm.Initialize()

	for i := 0; i < 10000; i++ {
		pm.RecordRequest("sonnet", "test", 100.0+float64(i), 0.05, 0.15, i%2 == 0, i%3 == 0)
		if i%1000 == 0 {
			pm.UpdateGauges(int64(i), int64(i/10), int64(i/100), float64(i)*0.01)
		}
	}

	// Measure after operations + GC
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	// #nosec G115 - uint64 to int64 conversion is safe; memory sizes within expected bounds
	heapGrowth := int64(m2.HeapAlloc) - int64(m1.HeapAlloc)
	var heapGrowthPercent float64
	if heapGrowth > 0 {
		heapGrowthPercent = (float64(heapGrowth) / float64(m1.HeapAlloc)) * 100
	}

	t.Logf("Metrics heap growth: %d bytes (%.2f%%)", heapGrowth, heapGrowthPercent)

	if heapGrowthPercent > 15.0 {
		t.Fatalf("metrics memory growth exceeded: %.2f%% > 15%%", heapGrowthPercent)
	}
}

// TestMemoryLeakEmbeddingClassifier verifies no memory leaks in embeddings
func TestMemoryLeakEmbeddingClassifier(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping memory leak test in short mode")
	}

	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	// Classify 5K prompts
	ec := classify.NewEmbeddingClassifier()
	prompts := []string{
		"race condition deadlock concurrent",
		"regex parse grammar",
		"optimize performance",
		"debug segfault",
		"architecture design",
		"encrypt crypto",
		"database query",
		"network socket",
		"test mock",
		"deploy docker",
	}

	for i := 0; i < 500; i++ {
		for _, p := range prompts {
			ec.Classify(p)
		}
	}

	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	// #nosec G115 - uint64 to int64 conversion is safe; memory sizes within expected bounds
	heapGrowth := int64(m2.HeapAlloc) - int64(m1.HeapAlloc)
	var heapGrowthPercent float64
	if heapGrowth > 0 {
		heapGrowthPercent = (float64(heapGrowth) / float64(m1.HeapAlloc)) * 100
	}

	t.Logf("Embedding classifier heap growth: %d bytes (%.2f%%)", heapGrowth, heapGrowthPercent)

	if heapGrowthPercent > 10.0 {
		t.Fatalf("classifier memory growth exceeded: %.2f%% > 10%%", heapGrowthPercent)
	}
}

// TestMemoryLeakConcurrentAccess verifies thread-safe cleanup under concurrent load
func TestMemoryLeakConcurrentAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping memory leak test in short mode")
	}

	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	// Simulate concurrent metric recording
	pm := observability.NewPrometheusMetrics()
	pm.Initialize()

	done := make(chan bool, 5)

	// Launch 5 concurrent workers
	for worker := 0; worker < 5; worker++ {
		go func(w int) {
			for i := 0; i < 500; i++ {
				pm.RecordRequest("haiku", "test", 50.0+float64(i), 0.05, 0.10, w%2 == 0, false)
			}
			done <- true
		}(worker)
	}

	// Wait for all workers
	for i := 0; i < 5; i++ {
		<-done
	}

	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	// #nosec G115 - uint64 to int64 conversion is safe; memory sizes within expected bounds
	heapGrowth := int64(m2.HeapAlloc) - int64(m1.HeapAlloc)
	var heapGrowthPercent float64
	if heapGrowth > 0 {
		heapGrowthPercent = (float64(heapGrowth) / float64(m1.HeapAlloc)) * 100
	}

	t.Logf("Concurrent access heap growth: %d bytes (%.2f%%)", heapGrowth, heapGrowthPercent)

	if heapGrowthPercent > 12.0 {
		t.Fatalf("concurrent memory growth exceeded: %.2f%% > 12%%", heapGrowthPercent)
	}

	// Verify no goroutine leaks
	numGoroutines := runtime.NumGoroutine()
	if numGoroutines > 100 {
		t.Logf("warning: goroutines not cleaned up: %d", numGoroutines)
	}
}

// BenchmarkMemoryAllocation measures memory allocation patterns
func BenchmarkMemoryAllocation(b *testing.B) {
	ec := classify.NewEmbeddingClassifier()
	prompts := []string{
		"concurrency race condition",
		"parsing regex grammar",
		"optimization performance",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ec.Classify(prompts[i%len(prompts)])
	}
}

// BenchmarkMetricsCollection measures metrics recording overhead
func BenchmarkMetricsCollection(b *testing.B) {
	pm := observability.NewPrometheusMetrics()
	pm.Initialize()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pm.RecordRequest("sonnet", "test", 100.0, 0.05, 0.15, i%2 == 0, i%3 == 0)
	}
}
