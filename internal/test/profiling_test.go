// Package test provides profiling tests for performance analysis
package test

import (
	"os"
	"runtime"
	"runtime/pprof"
	"testing"
	"time"

	"github.com/szibis/claude-escalate/internal/classify"
	"github.com/szibis/claude-escalate/internal/observability"
)

// TestCPUProfile_Classification profiles CPU usage during classification
func TestCPUProfile_Classification(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping profiling test in short mode")
	}

	// Create profiles directory
	os.MkdirAll("profiles", 0755)

	// Create CPU profile
	f, err := os.Create("profiles/cpu-classify.prof")
	if err != nil {
		t.Fatalf("could not create CPU profile: %v", err)
	}
	defer f.Close()

	if err := pprof.StartCPUProfile(f); err != nil {
		t.Fatalf("could not start CPU profile: %v", err)
	}
	defer pprof.StopCPUProfile()

	// Classify 10K prompts
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

	start := time.Now()
	for i := 0; i < 1000; i++ {
		for _, p := range prompts {
			ec.Classify(p)
		}
	}
	elapsed := time.Since(start)

	t.Logf("Classification CPU profile: 10K operations in %v", elapsed)

	// Assert: CPU time should be reasonable
	if elapsed > 10*time.Second {
		t.Logf("warning: classification taking longer than expected: %v", elapsed)
	}
}

// TestHeapProfile_Metrics profiles memory allocation during metrics recording
func TestHeapProfile_Metrics(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping profiling test in short mode")
	}

	os.MkdirAll("profiles", 0755)

	// Record metrics and then write heap profile
	pm := observability.NewPrometheusMetrics()
	pm.Initialize()

	for i := 0; i < 10000; i++ {
		pm.RecordRequest("sonnet", "test", 100.0+float64(i), 0.05, 0.15, i%2 == 0, i%3 == 0)
		if i%1000 == 0 {
			pm.UpdateGauges(int64(i), int64(i/10), int64(i/100), float64(i)*0.01)
		}
	}

	// Write heap profile
	f, err := os.Create("profiles/heap-metrics.prof")
	if err != nil {
		t.Fatalf("could not create heap profile: %v", err)
	}
	defer f.Close()

	runtime.GC()
	if err := pprof.WriteHeapProfile(f); err != nil {
		t.Fatalf("could not write heap profile: %v", err)
	}

	t.Logf("Heap profile written: profiles/heap-metrics.prof")
}

// TestGoroutineProfile verifies goroutine lifecycle
func TestGoroutineProfile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping profiling test in short mode")
	}

	os.MkdirAll("profiles", 0755)

	baseline := runtime.NumGoroutine()
	t.Logf("baseline goroutines: %d", baseline)

	// Simulate concurrent operations
	pm := observability.NewPrometheusMetrics()
	pm.Initialize()

	done := make(chan bool, 10)

	// Launch 10 concurrent workers
	for worker := 0; worker < 10; worker++ {
		go func(w int) {
			for i := 0; i < 100; i++ {
				pm.RecordRequest("haiku", "test", 50.0+float64(i), 0.05, 0.10, w%2 == 0, false)
			}
			done <- true
		}(worker)
	}

	// Wait for completion
	for i := 0; i < 10; i++ {
		<-done
	}

	// Allow goroutines to clean up
	time.Sleep(100 * time.Millisecond)

	// Write goroutine profile
	f, err := os.Create("profiles/goroutine.prof")
	if err != nil {
		t.Fatalf("could not create goroutine profile: %v", err)
	}
	defer f.Close()

	if err := pprof.Lookup("goroutine").WriteTo(f, 0); err != nil {
		t.Fatalf("could not write goroutine profile: %v", err)
	}

	// Verify cleanup
	final := runtime.NumGoroutine()
	t.Logf("final goroutines: %d (delta: %d)", final, final-baseline)

	if final > baseline+5 {
		t.Logf("warning: goroutine cleanup incomplete (leaked %d)", final-baseline)
	}
}

// TestAllocationProfile tracks memory allocations
func TestAllocationProfile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping profiling test in short mode")
	}

	os.MkdirAll("profiles", 0755)

	// Prepare allocation profile
	runtime.MemProfileRate = 1

	// Perform operations that allocate memory
	learner := classify.NewLearner(100, 1*time.Hour)
	for i := 0; i < 1000; i++ {
		event := classify.LearningEvent{
			ID:             string(rune(i)),
			Prompt:         "test prompt for allocation profiling",
			PredictedTask:  classify.TaskConcurrency,
			ActualTask:     classify.TaskConcurrency,
			Succeeded:      i%2 == 0,
			TokenError:     0.05,
		}
		learner.RecordOutcome(event)
	}

	// Write allocation profile
	f, err := os.Create("profiles/alloc.prof")
	if err != nil {
		t.Fatalf("could not create allocation profile: %v", err)
	}
	defer f.Close()

	runtime.GC()
	profile := pprof.Lookup("alloc_objects")
	if profile == nil {
		profile = pprof.Lookup("heap")
	}

	if profile != nil {
		if err := profile.WriteTo(f, 0); err != nil {
			t.Fatalf("could not write allocation profile: %v", err)
		}
	}

	t.Logf("Allocation profile written: profiles/alloc.prof")
}

// TestLatencyProfileBenchmark measures end-to-end latency
func TestLatencyProfileBenchmark(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping profiling test in short mode")
	}

	ec := classify.NewEmbeddingClassifier()
	prompts := []string{
		"race condition deadlock concurrent",
		"regex parse grammar",
		"optimize performance",
	}

	// Measure classification latency
	var totalTime time.Duration
	iterations := 1000

	for i := 0; i < iterations; i++ {
		start := time.Now()
		ec.Classify(prompts[i%len(prompts)])
		totalTime += time.Since(start)
	}

	avgLatency := totalTime / time.Duration(iterations)
	t.Logf("Classification latency: %v avg (%v total for %d)", avgLatency, totalTime, iterations)

	// SLO: <5ms per classification
	if avgLatency > 5*time.Millisecond {
		t.Logf("warning: classification latency exceeds SLO: %v > 5ms", avgLatency)
	}
}

// BenchmarkClassificationWithProfile runs classification with profiling enabled
func BenchmarkClassificationWithProfile(b *testing.B) {
	ec := classify.NewEmbeddingClassifier()
	prompts := []string{
		"concurrency race condition",
		"parsing regex grammar",
		"optimization performance",
		"debugging segfault",
		"architecture design",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ec.Classify(prompts[i%len(prompts)])
	}
}

// BenchmarkMetricsRecording runs metrics recording benchmark
func BenchmarkMetricsRecording(b *testing.B) {
	pm := observability.NewPrometheusMetrics()
	pm.Initialize()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pm.RecordRequest("sonnet", "test", 100.0+float64(i%1000), 0.05, 0.15, i%2 == 0, i%3 == 0)
	}
}

// BenchmarkMetricsExport runs metrics export benchmark
func BenchmarkMetricsExport(b *testing.B) {
	pm := observability.NewPrometheusMetrics()
	pm.Initialize()

	// Pre-populate with some data
	for i := 0; i < 1000; i++ {
		pm.RecordRequest("sonnet", "test", 100.0, 0.05, 0.15, i%2 == 0, i%3 == 0)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pm.ExportPrometheus()
	}
}

// BenchmarkLearnerRecording runs learner recording benchmark
func BenchmarkLearnerRecording(b *testing.B) {
	learner := classify.NewLearner(100, 1*time.Hour)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		event := classify.LearningEvent{
			ID:             string(rune(i % 100)),
			Prompt:         "test prompt",
			PredictedTask:  classify.TaskConcurrency,
			ActualTask:     classify.TaskConcurrency,
			Succeeded:      i%2 == 0,
			TokenError:     0.05,
		}
		learner.RecordOutcome(event)
	}
}
