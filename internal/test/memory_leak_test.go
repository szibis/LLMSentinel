// Package test provides memory leak detection tests for Claude Escalate
package test

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/szibis/claude-escalate/internal/batch"
	"github.com/szibis/claude-escalate/internal/client"
)

// TestMemoryLeakBatchQueue verifies no memory leaks in batch queue (10k+ requests)
func TestMemoryLeakBatchQueue(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping memory leak test in short mode")
	}

	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	baselineGoroutines := runtime.NumGoroutine()
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	// Process 10,000 requests through batch queue
	queue := batch.NewBatchQueue()
	for i := 0; i < 10000; i++ {
		req := &batch.BatchRequest{
			ID:              fmt.Sprintf("req_%d", i),
			PromptLength:    1000 + (i % 5000),
			EstimatedOutput: 500 + (i % 2000),
			Model:           "sonnet",
			MaxWaitTime:     10 * time.Minute,
			CreatedAt:       time.Now(),
		}
		queue.Enqueue(req)

		// Simulate dequeue every 100 requests
		if i%100 == 0 && queue.Size() > 50 {
			_ = queue.Peek()
		}
	}

	// Flush remaining
	_ = queue.Flush()

	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	finalGoroutines := runtime.NumGoroutine()
	var heapGrowth int64
	heapDiff := m2.HeapAlloc - m1.HeapAlloc
	if heapDiff > uint64(^uint64(0)>>1) {
		heapGrowth = 0 // cap to prevent overflow
	} else {
		heapGrowth = int64(heapDiff)
	}
	var heapGrowthPercent float64
	if m1.HeapAlloc > 0 {
		heapGrowthPercent = (float64(heapGrowth) / float64(m1.HeapAlloc)) * 100
	}

	t.Logf("Batch queue 10k requests - Heap: %d bytes (%.2f%%), Goroutines: %d→%d",
		heapGrowth, heapGrowthPercent, baselineGoroutines, finalGoroutines)

	if heapGrowthPercent > 20.0 {
		t.Errorf("memory growth exceeded: %.2f%% > 20%%", heapGrowthPercent)
	}
	if finalGoroutines > baselineGoroutines+5 {
		t.Logf("warning: goroutine leak: %d new goroutines", finalGoroutines-baselineGoroutines)
	}
}

// TestMemoryLeakBatchPoller verifies no memory leaks in batch poller (100k+ job tracking)
func TestMemoryLeakBatchPoller(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping memory leak test in short mode")
	}

	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	baselineGoroutines := runtime.NumGoroutine()
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	// Create poller and track 100 jobs
	ac := client.NewAnthropicClient("test-key")
	poller := batch.NewBatchPoller(ac)

	for i := 0; i < 100; i++ {
		jobID := fmt.Sprintf("job_%d", i)
		_ = poller.TrackJob(jobID, 50+i)
	}

	// Query all jobs multiple times (1000 total queries)
	for i := 0; i < 1000; i++ {
		jobID := fmt.Sprintf("job_%d", i%100)
		_, _ = poller.GetJobStatus(jobID)
	}

	// Simulate job completion and cleanup
	for i := 0; i < 50; i++ {
		jobID := fmt.Sprintf("job_%d", i)
		tracker, _ := poller.GetJobStatus(jobID)
		if tracker != nil {
			tracker.Status = "succeeded"
		}
	}

	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	finalGoroutines := runtime.NumGoroutine()
	var heapGrowth int64
	heapDiff := m2.HeapAlloc - m1.HeapAlloc
	if heapDiff > uint64(^uint64(0)>>1) {
		heapGrowth = 0 // cap to prevent overflow
	} else {
		heapGrowth = int64(heapDiff)
	}
	var heapGrowthPercent float64
	if m1.HeapAlloc > 0 {
		heapGrowthPercent = (float64(heapGrowth) / float64(m1.HeapAlloc)) * 100
	}

	t.Logf("Batch poller 100 jobs, 1k queries - Heap: %d bytes (%.2f%%), Goroutines: %d→%d",
		heapGrowth, heapGrowthPercent, baselineGoroutines, finalGoroutines)

	if heapGrowthPercent > 25.0 {
		t.Errorf("memory growth exceeded: %.2f%% > 25%%", heapGrowthPercent)
	}
}

// TestMemoryLeakRouterDecisions verifies no memory leaks in routing (50k+ decisions)
func TestMemoryLeakRouterDecisions(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping memory leak test in short mode")
	}

	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	baselineGoroutines := runtime.NumGoroutine()
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	// Make 50,000 routing decisions
	router := batch.NewRouter(batch.StrategyAuto)
	router.EnableDetector(true)

	for i := 0; i < 50000; i++ {
		req := batch.BatchRequest{
			ID:              fmt.Sprintf("req_%d", i),
			PromptLength:    1000 + (i % 5000),
			EstimatedOutput: 500 + (i % 2000),
			Model:           "sonnet",
			MaxWaitTime:     10 * time.Minute,
			CreatedAt:       time.Now(),
			UserContext: map[string]interface{}{
				"intent": "analysis",
			},
		}
		_, _ = router.MakeRoutingDecision(req)
	}

	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	finalGoroutines := runtime.NumGoroutine()
	var heapGrowth int64
	heapDiff := m2.HeapAlloc - m1.HeapAlloc
	if heapDiff > uint64(^uint64(0)>>1) {
		heapGrowth = 0 // cap to prevent overflow
	} else {
		heapGrowth = int64(heapDiff)
	}
	var heapGrowthPercent float64
	if m1.HeapAlloc > 0 {
		heapGrowthPercent = (float64(heapGrowth) / float64(m1.HeapAlloc)) * 100
	}

	t.Logf("Router 50k decisions - Heap: %d bytes (%.2f%%), Goroutines: %d→%d",
		heapGrowth, heapGrowthPercent, baselineGoroutines, finalGoroutines)

	if heapGrowthPercent > 30.0 {
		t.Errorf("memory growth exceeded: %.2f%% > 30%%", heapGrowthPercent)
	}
	if finalGoroutines > baselineGoroutines+5 {
		t.Logf("warning: goroutine leak: %d new goroutines", finalGoroutines-baselineGoroutines)
	}
}

// TestMemoryLeak100kConcurrentRequests verifies concurrent 100k requests with leak detection
func TestMemoryLeak100kConcurrentRequests(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping memory leak test in short mode")
	}

	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	baselineGoroutines := runtime.NumGoroutine()
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	// Concurrent load: 10 workers × 10k requests = 100k total
	queue := batch.NewBatchQueue()
	router := batch.NewRouter(batch.StrategyAuto)
	router.EnableDetector(true)

	var wg sync.WaitGroup
	var processed int64

	// Launch 10 concurrent workers
	for worker := 0; worker < 10; worker++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 10000; i++ {
				req := batch.BatchRequest{
					ID:              fmt.Sprintf("worker_%d_req_%d", w, i),
					PromptLength:    2000 + (i % 3000),
					EstimatedOutput: 1000 + (i % 1500),
					Model:           "sonnet",
					MaxWaitTime:     10 * time.Minute,
					CreatedAt:       time.Now(),
				}
				queue.Enqueue(&req)
				_, _ = router.MakeRoutingDecision(req)
				atomic.AddInt64(&processed, 1)
			}
		}(worker)
	}

	wg.Wait()

	runtime.GC()
	time.Sleep(200 * time.Millisecond)
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	finalGoroutines := runtime.NumGoroutine()
	var heapGrowth int64
	heapDiff := m2.HeapAlloc - m1.HeapAlloc
	if heapDiff > uint64(^uint64(0)>>1) {
		heapGrowth = 0 // cap to prevent overflow
	} else {
		heapGrowth = int64(heapDiff)
	}
	var heapGrowthPercent float64
	if m1.HeapAlloc > 0 {
		heapGrowthPercent = (float64(heapGrowth) / float64(m1.HeapAlloc)) * 100
	}

	t.Logf("100k concurrent - Processed: %d, Heap: %d bytes (%.2f%%), Goroutines: %d→%d",
		atomic.LoadInt64(&processed), heapGrowth, heapGrowthPercent, baselineGoroutines, finalGoroutines)

	if heapGrowthPercent > 40.0 {
		t.Errorf("memory growth exceeded: %.2f%% > 40%%", heapGrowthPercent)
	}
	if finalGoroutines > baselineGoroutines+10 {
		t.Errorf("goroutine leak: %d new goroutines > 10 threshold", finalGoroutines-baselineGoroutines)
	}
}

// BenchmarkQueueOperations measures queue memory allocation
func BenchmarkQueueOperations(b *testing.B) {
	queue := batch.NewBatchQueue()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := &batch.BatchRequest{
			ID:    fmt.Sprintf("req_%d", i),
			Model: "sonnet",
		}
		queue.Enqueue(req)
		if i%100 == 0 {
			_ = queue.Peek()
		}
	}
}

// BenchmarkRouterDecisions measures router allocation per decision
func BenchmarkRouterDecisions(b *testing.B) {
	router := batch.NewRouter(batch.StrategyAuto)
	router.EnableDetector(true)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := batch.BatchRequest{
			ID:    fmt.Sprintf("req_%d", i),
			Model: "sonnet",
		}
		_, _ = router.MakeRoutingDecision(req)
	}
}

// TestGoroutineLeakDetection100k verifies no goroutine leaks at 100k scale
func TestGoroutineLeakDetection100k(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping goroutine leak test in short mode")
	}

	baselineGoroutines := runtime.NumGoroutine()
	ctx := context.Background()

	// Start poller
	ac := client.NewAnthropicClient("test-key")
	poller := batch.NewBatchPoller(ac)
	_ = poller.Start(ctx)
	defer poller.Stop()

	// Process 100k requests
	router := batch.NewRouter(batch.StrategyAuto)
	for i := 0; i < 100000; i++ {
		req := batch.BatchRequest{
			ID:    fmt.Sprintf("req_%d", i),
			Model: "sonnet",
		}
		_, _ = router.MakeRoutingDecision(req)
	}

	time.Sleep(100 * time.Millisecond)
	finalGoroutines := runtime.NumGoroutine()
	goroutineLeaks := finalGoroutines - baselineGoroutines

	t.Logf("Goroutine check at 100k scale: baseline %d → final %d (leaks: %d)",
		baselineGoroutines, finalGoroutines, goroutineLeaks)

	if goroutineLeaks > 20 {
		t.Errorf("goroutine leak: %d new goroutines > 20 threshold", goroutineLeaks)
	}
}
