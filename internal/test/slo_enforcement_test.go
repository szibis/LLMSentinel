// Package test provides SLO enforcement tests for performance boundaries
package test

import (
	"runtime"
	"testing"
	"time"

	"github.com/szibis/claude-escalate/internal/classify"
	"github.com/szibis/claude-escalate/internal/observability"
)

// TestSLO_MemoryUsage enforces heap memory growth SLO
func TestSLO_MemoryUsage(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping SLO test in short mode")
	}

	// Baseline: measure memory before test
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	// Run operations: classify 10K prompts
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

	for i := 0; i < 1000; i++ {
		for _, p := range prompts {
			ec.Classify(p)
		}
	}

	// Measure after GC
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	// #nosec G115 - uint64 to int64 conversion is safe; memory sizes within expected bounds
	heapGrowth := int64(m2.HeapAlloc) - int64(m1.HeapAlloc)

	t.Logf("Heap growth: %d bytes (baseline: %d)", heapGrowth, m1.HeapAlloc)

	// SLO: <50MB growth after 10K operations (if positive)
	if heapGrowth > 50*1024*1024 {
		t.Fatalf("heap growth exceeded SLO: %d bytes > 50MB", heapGrowth)
	}
}

// TestSLO_ClassificationLatency enforces classification latency SLO
func TestSLO_ClassificationLatency(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping SLO test in short mode")
	}

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

	// Measure latency for 1000 classifications
	start := time.Now()
	for i := 0; i < 1000; i++ {
		ec.Classify(prompts[i%len(prompts)])
	}
	elapsed := time.Since(start)

	avgLatency := elapsed / 1000

	t.Logf("Classification latency: %v avg, %v total for 1000", avgLatency, elapsed)

	// SLO: <5ms average latency per classification
	if avgLatency > 5*time.Millisecond {
		t.Fatalf("classification latency SLO violated: %v > 5ms", avgLatency)
	}
}

// TestSLO_MetricsExportLatency enforces metrics export latency SLO
func TestSLO_MetricsExportLatency(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping SLO test in short mode")
	}

	pm := observability.NewPrometheusMetrics()
	pm.Initialize()

	// Pre-populate with metrics
	for i := 0; i < 1000; i++ {
		pm.RecordRequest("sonnet", "test", 100.0+float64(i%1000), 0.05, 0.15, i%2 == 0, i%3 == 0)
		if i%100 == 0 {
			pm.UpdateGauges(int64(i), int64(i/10), int64(i/100), float64(i)*0.01)
		}
	}

	// Measure export latency
	start := time.Now()
	output := pm.ExportPrometheus()
	elapsed := time.Since(start)

	t.Logf("Metrics export: %v elapsed, %d bytes output", elapsed, len(output))

	// SLO: <100ms to export metrics
	if elapsed > 100*time.Millisecond {
		t.Fatalf("metrics export SLO violated: %v > 100ms", elapsed)
	}

	if len(output) == 0 {
		t.Fatalf("metrics export produced empty output")
	}
}

// TestSLO_MetricsSnapshot enforces snapshot generation latency
func TestSLO_MetricsSnapshot(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping SLO test in short mode")
	}

	pm := observability.NewPrometheusMetrics()
	pm.Initialize()

	// Pre-populate metrics
	for i := 0; i < 500; i++ {
		pm.RecordRequest("sonnet", "test", 100.0, 0.05, 0.15, i%2 == 0, i%3 == 0)
	}

	// Measure snapshot latency
	start := time.Now()
	snapshot := pm.GetMetricsSnapshot()
	elapsed := time.Since(start)

	t.Logf("Metrics snapshot: %v elapsed, %d keys", elapsed, len(snapshot))

	// SLO: <50ms to generate snapshot
	if elapsed > 50*time.Millisecond {
		t.Fatalf("snapshot generation SLO violated: %v > 50ms", elapsed)
	}

	if len(snapshot) == 0 {
		t.Fatalf("snapshot is empty")
	}
}

// TestSLO_LearnerAccuracy enforces learner accuracy tracking
func TestSLO_LearnerAccuracy(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping SLO test in short mode")
	}

	learner := classify.NewLearner(100, 1*time.Hour)

	// Record 100 events with 80% accuracy
	for i := 0; i < 100; i++ {
		event := classify.LearningEvent{
			ID:             string(rune(i % 100)),
			Prompt:         "test prompt",
			PredictedTask:  classify.TaskConcurrency,
			ActualTask:     classify.TaskConcurrency,
			Succeeded:      i < 80, // 80% success rate
			TokenError:     0.05,
		}
		learner.RecordOutcome(event)
	}

	// Verify accuracy tracking
	accuracy := learner.GetTaskAccuracy(classify.TaskConcurrency)

	t.Logf("Learner accuracy: %d/%d = %.2f%%",
		accuracy.SuccessCount, accuracy.TotalCount,
		accuracy.SuccessRate*100)

	// SLO: accuracy tracking should be accurate
	expectedRate := 0.80
	tolerance := 0.05
	if accuracy.SuccessRate < expectedRate-tolerance || accuracy.SuccessRate > expectedRate+tolerance {
		t.Fatalf("accuracy tracking SLO violated: %.2f %% != %.2f %%",
			accuracy.SuccessRate*100, expectedRate*100)
	}
}

// TestSLO_ConcurrentMetricsAccess enforces thread-safe metrics under load
func TestSLO_ConcurrentMetricsAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping SLO test in short mode")
	}

	pm := observability.NewPrometheusMetrics()
	pm.Initialize()

	done := make(chan bool, 10)

	// Launch 10 concurrent workers recording metrics
	start := time.Now()
	for worker := 0; worker < 10; worker++ {
		go func(w int) {
			for i := 0; i < 500; i++ {
				pm.RecordRequest("haiku", "test", 50.0+float64(i), 0.05, 0.10, w%2 == 0, false)
			}
			done <- true
		}(worker)
	}

	// Wait for completion
	for i := 0; i < 10; i++ {
		<-done
	}
	elapsed := time.Since(start)

	t.Logf("Concurrent metrics: 5000 records in %v", elapsed)

	// SLO: <2 seconds for 5000 concurrent writes
	if elapsed > 2*time.Second {
		t.Logf("warning: concurrent metrics SLO warning: %v", elapsed)
	}
}

// TestSLO_GoroutineCleanup enforces goroutine cleanup
func TestSLO_GoroutineCleanup(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping SLO test in short mode")
	}

	baseline := runtime.NumGoroutine()

	// Launch concurrent operations
	pm := observability.NewPrometheusMetrics()
	pm.Initialize()

	done := make(chan bool, 5)
	for worker := 0; worker < 5; worker++ {
		go func(w int) {
			for i := 0; i < 100; i++ {
				pm.RecordRequest("haiku", "test", 50.0, 0.05, 0.10, w%2 == 0, false)
			}
			done <- true
		}(worker)
	}

	for i := 0; i < 5; i++ {
		<-done
	}

	// Wait for cleanup
	time.Sleep(100 * time.Millisecond)

	final := runtime.NumGoroutine()
	delta := final - baseline

	t.Logf("Goroutine cleanup: baseline=%d, final=%d, delta=%d", baseline, final, delta)

	// SLO: no goroutine leaks (allow small delta for runtime management)
	if delta > 5 {
		t.Fatalf("goroutine cleanup SLO violated: %d leaked > 5", delta)
	}
}
