package observability

import (
	"strings"
	"testing"
)

func TestNewPrometheusMetrics(t *testing.T) {
	pm := NewPrometheusMetrics()

	if pm == nil {
		t.Error("expected non-nil metrics")
	}

	pm.Initialize()

	// Check buckets are initialized
	if len(pm.LatencyBuckets) == 0 {
		t.Error("latency buckets not initialized")
	}
	if len(pm.TokenErrorBuckets) == 0 {
		t.Error("token error buckets not initialized")
	}
	if len(pm.ModelUsage) != 3 {
		t.Error("expected 3 models initialized")
	}
}

func TestRecordRequest(t *testing.T) {
	pm := NewPrometheusMetrics()
	pm.Initialize()

	pm.RecordRequest("sonnet", "concurrency", 150.5, 0.08, 0.25, true, false)

	if pm.TotalRequests != 1 {
		t.Errorf("expected 1 total request, got %d", pm.TotalRequests)
	}

	if pm.CacheHits != 1 {
		t.Errorf("expected 1 cache hit, got %d", pm.CacheHits)
	}

	if pm.CacheMisses != 0 {
		t.Errorf("expected 0 cache misses, got %d", pm.CacheMisses)
	}

	if pm.ModelUsage["sonnet"] != 1 {
		t.Errorf("expected sonnet count 1, got %d", pm.ModelUsage["sonnet"])
	}

	if pm.TaskTypeUsage["concurrency"] != 1 {
		t.Errorf("expected concurrency count 1, got %d", pm.TaskTypeUsage["concurrency"])
	}
}

func TestRecordRequestBatched(t *testing.T) {
	pm := NewPrometheusMetrics()
	pm.Initialize()

	pm.RecordRequest("haiku", "parsing", 50.0, 0.05, 0.10, false, true)

	if pm.BatchQueued != 1 {
		t.Errorf("expected 1 batch queued, got %d", pm.BatchQueued)
	}

	if pm.DirectRequests != 0 {
		t.Errorf("expected 0 direct requests, got %d", pm.DirectRequests)
	}

	if pm.CostPerRequestCount != 1 {
		t.Errorf("expected 1 cost sample, got %d", pm.CostPerRequestCount)
	}

	expectedCostSum := 0.10
	if diff := pm.CostPerRequestSum - expectedCostSum; diff < -0.001 || diff > 0.001 {
		t.Errorf("expected cost sum ~%.2f, got %.2f", expectedCostSum, pm.CostPerRequestSum)
	}
}

func TestUpdateGauges(t *testing.T) {
	pm := NewPrometheusMetrics()

	pm.UpdateGauges(500, 25, 10, 150.75)

	if pm.CacheSize != 500 {
		t.Errorf("expected cache size 500, got %d", pm.CacheSize)
	}

	if pm.QueueSize != 25 {
		t.Errorf("expected queue size 25, got %d", pm.QueueSize)
	}

	if pm.ActiveSessions != 10 {
		t.Errorf("expected 10 active sessions, got %d", pm.ActiveSessions)
	}

	if diff := pm.CostThisMonth - 150.75; diff < -0.001 || diff > 0.001 {
		t.Errorf("expected cost 150.75, got %f", pm.CostThisMonth)
	}
}

func TestRecordModelSwitch(t *testing.T) {
	pm := NewPrometheusMetrics()

	pm.RecordModelSwitch()
	pm.RecordModelSwitch()

	if pm.ModelSwitches != 2 {
		t.Errorf("expected 2 model switches, got %d", pm.ModelSwitches)
	}
}

func TestSetPercentiles(t *testing.T) {
	pm := NewPrometheusMetrics()

	pm.SetPercentiles(100.5, 250.3, 500.7)

	if diff := pm.LatencyP50 - 100.5; diff < -0.001 || diff > 0.001 {
		t.Errorf("expected P50=100.5, got %f", pm.LatencyP50)
	}

	if diff := pm.LatencyP95 - 250.3; diff < -0.001 || diff > 0.001 {
		t.Errorf("expected P95=250.3, got %f", pm.LatencyP95)
	}

	if diff := pm.LatencyP99 - 500.7; diff < -0.001 || diff > 0.001 {
		t.Errorf("expected P99=500.7, got %f", pm.LatencyP99)
	}
}

func TestExportPrometheus(t *testing.T) {
	pm := NewPrometheusMetrics()
	pm.Initialize()

	// Record some data
	pm.RecordRequest("sonnet", "test", 100.0, 0.05, 0.15, true, false)
	pm.RecordRequest("haiku", "test", 50.0, 0.10, 0.08, false, false)
	pm.UpdateGauges(100, 5, 2, 50.0)
	pm.SetPercentiles(100, 250, 500)

	output := pm.ExportPrometheus()

	// Check for key metrics
	if !strings.Contains(output, "claude_escalate_requests_total") {
		t.Error("output missing total requests metric")
	}

	if !strings.Contains(output, "claude_escalate_cache_hits_total") {
		t.Error("output missing cache hits metric")
	}

	if !strings.Contains(output, "claude_escalate_model_usage_total") {
		t.Error("output missing model usage metric")
	}

	if !strings.Contains(output, "claude_escalate_cache_size") {
		t.Error("output missing cache size gauge")
	}

	if !strings.Contains(output, "claude_escalate_queue_size") {
		t.Error("output missing queue size gauge")
	}

	// Check for specific values
	if !strings.Contains(output, "claude_escalate_requests_total 2") {
		t.Error("expected total requests=2 in output")
	}

	if !strings.Contains(output, "claude_escalate_cache_size 100") {
		t.Error("expected cache size=100 in output")
	}
}

func TestGetMetricsSnapshot(t *testing.T) {
	pm := NewPrometheusMetrics()
	pm.Initialize()

	pm.RecordRequest("opus", "arch", 200.0, 0.15, 0.50, true, true)
	pm.RecordRequest("sonnet", "arch", 150.0, 0.10, 0.30, false, false)
	pm.UpdateGauges(250, 10, 5, 100.0)
	pm.SetPercentiles(150, 200, 500)

	snapshot := pm.GetMetricsSnapshot()

	if totalReqs, ok := snapshot["total_requests"].(int64); !ok || totalReqs != 2 {
		t.Error("total_requests not in snapshot")
	}

	if hitRate, ok := snapshot["cache_hit_rate"].(float64); !ok || (hitRate < 0.49 || hitRate > 0.51) {
		t.Errorf("cache_hit_rate incorrect: %v", snapshot["cache_hit_rate"])
	}

	if cacheSize, ok := snapshot["cache_size"].(int64); !ok || cacheSize != 250 {
		t.Error("cache_size incorrect in snapshot")
	}

	if modelUsage, ok := snapshot["model_usage"].(map[string]int64); !ok || len(modelUsage) != 3 {
		t.Error("model_usage not correctly in snapshot")
	}
}

func TestLatencyBucketCounting(t *testing.T) {
	pm := NewPrometheusMetrics()
	pm.Initialize()

	// Record requests with different latencies
	pm.RecordRequest("haiku", "test", 5.0, 0.05, 0.05, false, false)    // < 10ms
	pm.RecordRequest("haiku", "test", 25.0, 0.05, 0.05, false, false)   // < 50ms
	pm.RecordRequest("haiku", "test", 150.0, 0.05, 0.05, false, false)  // < 500ms
	pm.RecordRequest("haiku", "test", 2000.0, 0.05, 0.05, false, false) // > 1000ms

	// Verify bucket counts
	if pm.LatencyBuckets[10.0] < 1 {
		t.Error("expected requests in 10ms bucket")
	}

	if pm.LatencyBuckets[50.0] < 2 {
		t.Error("expected 2+ requests in 50ms bucket")
	}
}

func TestPrometheusThreadSafety(t *testing.T) {
	pm := NewPrometheusMetrics()
	pm.Initialize()

	// Simulate concurrent access
	done := make(chan bool, 2)

	go func() {
		for i := 0; i < 100; i++ {
			pm.RecordRequest("haiku", "test", 50.0, 0.05, 0.05, false, false)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 50; i++ {
			_ = pm.ExportPrometheus()
		}
		done <- true
	}()

	<-done
	<-done

	if pm.TotalRequests != 100 {
		t.Errorf("expected 100 requests after concurrent access, got %d", pm.TotalRequests)
	}
}
