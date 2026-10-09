package metrics

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// PrometheusExporter exports metrics in Prometheus format with label-based cardinality control
type PrometheusExporter struct {
	collector *MetricsCollector
	mu        sync.RWMutex
}

// NewPrometheusExporter creates a new Prometheus exporter
func NewPrometheusExporter(collector *MetricsCollector) *PrometheusExporter {
	return &PrometheusExporter{
		collector: collector,
	}
}

// Export generates Prometheus-format metrics output
func (pe *PrometheusExporter) Export() string {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	snapshot := pe.collector.GetMetrics()

	var buf strings.Builder

	// Export only dimensions actually recorded by the collector.
	buf.WriteString("# TYPE claude_escalate_cache_operations_total counter\n")
	for operation, count := range map[string]int64{"hit": snapshot.CacheMetrics.TotalHits, "miss": snapshot.CacheMetrics.TotalMisses, "false_positive": snapshot.CacheMetrics.FalsePositives} {
		fmt.Fprintf(&buf, "claude_escalate_cache_operations_total{layer=\"overall\",operation=\"%s\"} %d\n", operation, count)
	}
	buf.WriteString("# TYPE claude_escalate_cache_hit_rate gauge\n")
	fmt.Fprintf(&buf, "claude_escalate_cache_hit_rate{layer=\"overall\"} %f\n", snapshot.CacheMetrics.HitRate)
	fmt.Fprintf(&buf, "claude_escalate_cache_false_positive_rate{layer=\"overall\"} %f\n", snapshot.CacheMetrics.FalsePositiveRate)
	buf.WriteString("# TYPE claude_escalate_tokens_total counter\n")
	for kind, count := range map[string]int64{"input": snapshot.TokenMetrics.TotalInputTokens, "output": snapshot.TokenMetrics.TotalOutputTokens, "saved": snapshot.TokenMetrics.TokensSavedByOptimization} {
		fmt.Fprintf(&buf, "claude_escalate_tokens_total{type=\"%s\"} %d\n", kind, count)
	}
	fmt.Fprintf(&buf, "claude_escalate_token_savings_percent{aggregation=\"overall\"} %f\n", snapshot.TokenMetrics.SavingsPercent*100)
	buf.WriteString("# TYPE claude_escalate_latency_seconds gauge\n")
	for stage, ms := range map[string]float64{"cache_lookup": snapshot.LatencyMetrics.CacheLookupMs, "security_validation": snapshot.LatencyMetrics.SecurityValidationMs, "intent_detection": snapshot.LatencyMetrics.IntentDetectionMs, "optimization": snapshot.LatencyMetrics.OptimizationMs, "claude_api": snapshot.LatencyMetrics.ClaudeAPICallMs, "response_compression": snapshot.LatencyMetrics.ResponseCompressionMs, "total": snapshot.LatencyMetrics.TotalMs} {
		fmt.Fprintf(&buf, "claude_escalate_latency_seconds{stage=\"%s\"} %f\n", stage, ms/1000)
	}
	fmt.Fprintf(&buf, "claude_escalate_requests_total %d\n", snapshot.RequestCount)
	for kind, count := range map[string]int64{"injection_blocked": snapshot.SecurityMetrics.InjectionAttemptsBlocked, "rate_limit": snapshot.SecurityMetrics.RateLimitTriggered, "validation_failure": snapshot.SecurityMetrics.ValidationFailures, "unauthorized": snapshot.SecurityMetrics.UnauthorizedAttempts} {
		fmt.Fprintf(&buf, "claude_escalate_security_events_total{type=\"%s\"} %d\n", kind, count)
	}
	fmt.Fprintf(&buf, "claude_escalate_uptime_seconds %f\n", pe.collector.Uptime().Seconds())

	return buf.String()
}

// ExportJSON returns metrics as JSON (for API endpoints)
func (pe *PrometheusExporter) ExportJSON() map[string]interface{} {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	snapshot := pe.collector.GetMetrics()

	result := map[string]interface{}{
		"timestamp":      snapshot.Timestamp.Format(time.RFC3339),
		"uptime_seconds": int64(pe.collector.Uptime().Seconds()),
		"cache": map[string]interface{}{
			"hit_rate":            snapshot.CacheMetrics.HitRate,
			"false_positive_rate": snapshot.CacheMetrics.FalsePositiveRate,
			"total_hits":          snapshot.CacheMetrics.TotalHits,
			"total_misses":        snapshot.CacheMetrics.TotalMisses,
			"false_positives":     snapshot.CacheMetrics.FalsePositives,
		},
		"tokens": map[string]interface{}{
			"input_total":           snapshot.TokenMetrics.TotalInputTokens,
			"output_total":          snapshot.TokenMetrics.TotalOutputTokens,
			"saved_by_optimization": snapshot.TokenMetrics.TokensSavedByOptimization,
			"savings_percent":       snapshot.TokenMetrics.SavingsPercent,
		},
		"security": map[string]interface{}{
			"injections_blocked":    snapshot.SecurityMetrics.InjectionAttemptsBlocked,
			"rate_limits_triggered": snapshot.SecurityMetrics.RateLimitTriggered,
			"validation_failures":   snapshot.SecurityMetrics.ValidationFailures,
			"unauthorized_attempts": snapshot.SecurityMetrics.UnauthorizedAttempts,
		},
		"latency_ms": map[string]interface{}{
			"cache_lookup":         snapshot.LatencyMetrics.CacheLookupMs,
			"security_validation":  snapshot.LatencyMetrics.SecurityValidationMs,
			"intent_detection":     snapshot.LatencyMetrics.IntentDetectionMs,
			"optimization":         snapshot.LatencyMetrics.OptimizationMs,
			"claude_api":           snapshot.LatencyMetrics.ClaudeAPICallMs,
			"response_compression": snapshot.LatencyMetrics.ResponseCompressionMs,
			"total":                snapshot.LatencyMetrics.TotalMs,
		},
		"requests": map[string]interface{}{
			"total": snapshot.RequestCount,
		},
		"optimizers": snapshot.OptimizerMetrics,
	}

	return result
}
