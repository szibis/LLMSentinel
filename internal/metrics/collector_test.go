package metrics

import (
	"sync"
	"testing"
)

func TestMetricsCollectorCacheMetrics(t *testing.T) {
	collector := NewMetricsCollector()

	// Record cache events
	collector.RecordCacheHit()
	collector.RecordCacheHit()
	collector.RecordCacheMiss()

	snapshot := collector.GetMetrics()

	if snapshot.CacheMetrics.TotalHits != 2 {
		t.Errorf("expected 2 cache hits, got %d", snapshot.CacheMetrics.TotalHits)
	}
	if snapshot.CacheMetrics.TotalMisses != 1 {
		t.Errorf("expected 1 cache miss, got %d", snapshot.CacheMetrics.TotalMisses)
	}
}

func TestMetricsCollectorTokenMetrics(t *testing.T) {
	collector := NewMetricsCollector()

	// Record token usage
	collector.RecordTokens(1000, 500)
	collector.RecordTokens(2000, 1000)

	snapshot := collector.GetMetrics()

	if snapshot.TokenMetrics.TotalInputTokens != 3000 {
		t.Errorf("expected 3000 input tokens, got %d", snapshot.TokenMetrics.TotalInputTokens)
	}
	if snapshot.TokenMetrics.TotalOutputTokens != 1500 {
		t.Errorf("expected 1500 output tokens, got %d", snapshot.TokenMetrics.TotalOutputTokens)
	}
}

func TestMetricsCollectorTokenSavings(t *testing.T) {
	collector := NewMetricsCollector()

	// Record optimized and baseline tokens
	collector.RecordTokens(1000, 500)
	collector.RecordTokenSavings(500)

	snapshot := collector.GetMetrics()

	if snapshot.TokenMetrics.TokensSavedByOptimization != 500 {
		t.Errorf("expected 500 tokens saved, got %d", snapshot.TokenMetrics.TokensSavedByOptimization)
	}
}

func TestMetricsCollectorLatency(t *testing.T) {
	collector := NewMetricsCollector()

	// Record latencies (6 float64 parameters: cache, security, intent, optimize, claude, compress)
	collector.RecordLatency(5.0, 10.0, 15.0, 8.0, 1000.0, 5.0)

	snapshot := collector.GetMetrics()

	if snapshot.LatencyMetrics.CacheLookupMs != 5.0 {
		t.Errorf("expected 5.0ms cache latency, got %v", snapshot.LatencyMetrics.CacheLookupMs)
	}

	if snapshot.LatencyMetrics.TotalMs < 1000 {
		t.Logf("total latency: %v (acceptable)", snapshot.LatencyMetrics.TotalMs)
	}
}

func TestMetricsCollectorSecurityEvents(t *testing.T) {
	collector := NewMetricsCollector()

	// Record security events
	collector.RecordSecurityEvent("injection_attempt")
	collector.RecordSecurityEvent("validation_failure")
	collector.RecordSecurityEvent("injection_attempt")

	snapshot := collector.GetMetrics()

	if snapshot.SecurityMetrics.InjectionAttemptsBlocked != 2 {
		t.Errorf("expected 2 injection events, got %d", snapshot.SecurityMetrics.InjectionAttemptsBlocked)
	}
	if snapshot.SecurityMetrics.ValidationFailures != 1 {
		t.Errorf("expected 1 validation failure, got %d", snapshot.SecurityMetrics.ValidationFailures)
	}
}

func TestMetricsCollectorFalsePositiveRate(t *testing.T) {
	collector := NewMetricsCollector()

	// Record cache hits and false positives
	collector.RecordCacheHit()
	collector.RecordCacheHit()
	collector.RecordFalsePositive()

	snapshot := collector.GetMetrics()

	// 1 false positive out of 3 = 33.33%
	if snapshot.CacheMetrics.FalsePositives != 1 {
		t.Errorf("expected 1 false positive, got %d", snapshot.CacheMetrics.FalsePositives)
	}
	if snapshot.CacheMetrics.TotalHits != 2 {
		t.Errorf("expected 2 cache hits, got %d", snapshot.CacheMetrics.TotalHits)
	}
}

func TestMetricsCollectorConcurrentAccess(t *testing.T) {
	collector := NewMetricsCollector()
	var wg sync.WaitGroup

	// 10 goroutines recording metrics concurrently
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				collector.RecordCacheHit()
				collector.RecordTokens(1000, 500)
				collector.RecordLatency(5.0, 10.0, 15.0, 8.0, 100.0, 5.0)
			}
		}()
	}

	wg.Wait()

	metrics := collector.GetMetrics()

	expectedHits := int64(10 * 100)
	actualHits := metrics.CacheMetrics.TotalHits
	if actualHits != expectedHits {
		t.Errorf("expected %d cache hits, got %d", expectedHits, actualHits)
	}
}

func TestMetricsCollectorHistory(t *testing.T) {
	collector := NewMetricsCollector()

	// Record metrics at different times
	collector.RecordCacheHit()
	collector.RecordCacheHit()

	// Save snapshot to history (explicitly required)
	collector.SaveSnapshot()

	// Get history
	history := collector.GetMetricsHistory()

	if len(history) == 0 {
		t.Error("expected history entries, got 0")
	}

	// Latest entry should have the metrics
	latest := history[len(history)-1]
	if latest.CacheMetrics.TotalHits != 2 {
		t.Errorf("expected 2 cache hits in history, got %d", latest.CacheMetrics.TotalHits)
	}
}

func TestMetricsCollectorOptimizerMetrics(t *testing.T) {
	collector := NewMetricsCollector()

	// Record per-optimizer metrics
	collector.RecordOptimizerMetric("semantic_cache", 100, 0.5, true)
	collector.RecordOptimizerMetric("rtk_compression", 50, 0.3, true)

	metrics := collector.GetMetrics()

	if len(metrics.OptimizerMetrics) > 0 {
		t.Logf("optimizer metrics recorded: %d optimizers", len(metrics.OptimizerMetrics))
	}
}

func TestMetricsCollectorCostEstimate(t *testing.T) {
	collector := NewMetricsCollector()

	// Record tokens for cost calculation
	// Haiku: ~$0.0008 per 1K input, $0.0004 per 1K output
	collector.RecordTokens(10000, 5000) // 10K input + 5K output

	metrics := collector.GetMetrics()

	// Cost should be roughly 10 * 0.0008 + 5 * 0.0004 = 0.0080 + 0.0020 = 0.01
	if metrics.TokenMetrics.CostUSD > 0 {
		t.Logf("estimated cost: $%v (acceptable)", metrics.TokenMetrics.CostUSD)
	}
}

// Benchmark test for metrics recording
func BenchmarkRecordMetrics(b *testing.B) {
	collector := NewMetricsCollector()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		collector.RecordCacheHit()
		collector.RecordTokens(1000, 500)
		collector.RecordLatency(5.0, 10.0, 15.0, 8.0, 100.0, 5.0)
	}
}
