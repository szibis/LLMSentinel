package metrics

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// PrometheusExporter exports metrics in Prometheus format
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

	// HELP and TYPE comments
	buf.WriteString("# HELP claude_escalate_cache_hit_rate Cache hit rate (0.0-1.0)\n")
	buf.WriteString("# TYPE claude_escalate_cache_hit_rate gauge\n")

	buf.WriteString("# HELP claude_escalate_cache_false_positive_rate False positive rate for semantic cache\n")
	buf.WriteString("# TYPE claude_escalate_cache_false_positive_rate gauge\n")

	buf.WriteString("# HELP claude_escalate_cache_hits Total cache hits\n")
	buf.WriteString("# TYPE claude_escalate_cache_hits counter\n")

	buf.WriteString("# HELP claude_escalate_cache_misses Total cache misses\n")
	buf.WriteString("# TYPE claude_escalate_cache_misses counter\n")

	buf.WriteString("# HELP claude_escalate_tokens_saved_total Total tokens saved by optimizations\n")
	buf.WriteString("# TYPE claude_escalate_tokens_saved_total counter\n")

	buf.WriteString("# HELP claude_escalate_token_savings_percent Token savings percentage\n")
	buf.WriteString("# TYPE claude_escalate_token_savings_percent gauge\n")

	buf.WriteString("# HELP claude_escalate_security_injections_blocked Total injection attempts blocked\n")
	buf.WriteString("# TYPE claude_escalate_security_injections_blocked counter\n")

	buf.WriteString("# HELP claude_escalate_security_rate_limits Total rate limit triggers\n")
	buf.WriteString("# TYPE claude_escalate_security_rate_limits counter\n")

	buf.WriteString("# HELP claude_escalate_requests_total Total requests processed\n")
	buf.WriteString("# TYPE claude_escalate_requests_total counter\n")

	buf.WriteString("# HELP claude_escalate_errors_total Total errors\n")
	buf.WriteString("# TYPE claude_escalate_errors_total counter\n")

	buf.WriteString("# HELP claude_escalate_latency_ms Latency by component\n")
	buf.WriteString("# TYPE claude_escalate_latency_ms gauge\n")

	buf.WriteString("# HELP claude_escalate_optimizer_tokens_saved Tokens saved by optimizer\n")
	buf.WriteString("# TYPE claude_escalate_optimizer_tokens_saved counter\n")

	buf.WriteString("# HELP claude_escalate_optimizer_savings_percent Savings percentage by optimizer\n")
	buf.WriteString("# TYPE claude_escalate_optimizer_savings_percent gauge\n")

	buf.WriteString("# HELP claude_escalate_uptime_seconds Uptime in seconds\n")
	buf.WriteString("# TYPE claude_escalate_uptime_seconds counter\n")

	// Actual metrics
	buf.WriteString(fmt.Sprintf("claude_escalate_cache_hit_rate %f\n", snapshot.CacheMetrics.HitRate))
	buf.WriteString(fmt.Sprintf("claude_escalate_cache_false_positive_rate %f\n", snapshot.CacheMetrics.FalsePositiveRate))
	buf.WriteString(fmt.Sprintf("claude_escalate_cache_hits_total %d\n", snapshot.CacheMetrics.TotalHits))
	buf.WriteString(fmt.Sprintf("claude_escalate_cache_misses_total %d\n", snapshot.CacheMetrics.TotalMisses))
	buf.WriteString(fmt.Sprintf("claude_escalate_tokens_saved_total %d\n", snapshot.TokenMetrics.TokensSavedByOptimization))
	buf.WriteString(fmt.Sprintf("claude_escalate_token_savings_percent %f\n", snapshot.TokenMetrics.SavingsPercent))
	buf.WriteString(fmt.Sprintf("claude_escalate_security_injections_blocked_total %d\n", snapshot.SecurityMetrics.InjectionAttemptsBlocked))
	buf.WriteString(fmt.Sprintf("claude_escalate_security_rate_limits_total %d\n", snapshot.SecurityMetrics.RateLimitTriggered))
	buf.WriteString(fmt.Sprintf("claude_escalate_requests_total %d\n", snapshot.RequestCount))
	buf.WriteString(fmt.Sprintf("claude_escalate_latency_cache_lookup_ms %f\n", snapshot.LatencyMetrics.CacheLookupMs))
	buf.WriteString(fmt.Sprintf("claude_escalate_latency_security_validation_ms %f\n", snapshot.LatencyMetrics.SecurityValidationMs))
	buf.WriteString(fmt.Sprintf("claude_escalate_latency_intent_detection_ms %f\n", snapshot.LatencyMetrics.IntentDetectionMs))
	buf.WriteString(fmt.Sprintf("claude_escalate_latency_optimization_ms %f\n", snapshot.LatencyMetrics.OptimizationMs))
	buf.WriteString(fmt.Sprintf("claude_escalate_latency_claude_api_ms %f\n", snapshot.LatencyMetrics.ClaudeAPICallMs))
	buf.WriteString(fmt.Sprintf("claude_escalate_latency_response_compression_ms %f\n", snapshot.LatencyMetrics.ResponseCompressionMs))
	buf.WriteString(fmt.Sprintf("claude_escalate_latency_total_ms %f\n", snapshot.LatencyMetrics.TotalMs))

	// Per-optimizer metrics
	for name, metrics := range snapshot.OptimizerMetrics {
		safeLabel := strings.ReplaceAll(name, "-", "_")
		safeLabel = strings.ReplaceAll(safeLabel, ".", "_")
		buf.WriteString(fmt.Sprintf("claude_escalate_optimizer_tokens_saved_total{optimizer=\"%s\"} %d\n", safeLabel, metrics.TokensSaved))
		buf.WriteString(fmt.Sprintf("claude_escalate_optimizer_savings_percent{optimizer=\"%s\"} %f\n", safeLabel, metrics.SavingsPercent))
		buf.WriteString(fmt.Sprintf("claude_escalate_optimizer_requests{optimizer=\"%s\"} %d\n", safeLabel, metrics.RequestsProcessed))
	}

	buf.WriteString(fmt.Sprintf("claude_escalate_uptime_seconds %d\n", int64(pe.collector.Uptime().Seconds())))

	return buf.String()
}

// ExportJSON returns metrics as JSON (for API endpoints)
func (pe *PrometheusExporter) ExportJSON() map[string]interface{} {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	snapshot := pe.collector.GetMetrics()

	result := map[string]interface{}{
		"timestamp": snapshot.Timestamp.Format(time.RFC3339),
		"uptime_seconds": int64(pe.collector.Uptime().Seconds()),
		"cache": map[string]interface{}{
			"hit_rate":             snapshot.CacheMetrics.HitRate,
			"false_positive_rate":  snapshot.CacheMetrics.FalsePositiveRate,
			"total_hits":           snapshot.CacheMetrics.TotalHits,
			"total_misses":         snapshot.CacheMetrics.TotalMisses,
			"false_positives":      snapshot.CacheMetrics.FalsePositives,
		},
		"tokens": map[string]interface{}{
			"input_total":                 snapshot.TokenMetrics.TotalInputTokens,
			"output_total":                snapshot.TokenMetrics.TotalOutputTokens,
			"saved_by_optimization":       snapshot.TokenMetrics.TokensSavedByOptimization,
			"savings_percent":             snapshot.TokenMetrics.SavingsPercent,
		},
		"security": map[string]interface{}{
			"injections_blocked":  snapshot.SecurityMetrics.InjectionAttemptsBlocked,
			"rate_limits_triggered": snapshot.SecurityMetrics.RateLimitTriggered,
			"validation_failures":   snapshot.SecurityMetrics.ValidationFailures,
			"unauthorized_attempts": snapshot.SecurityMetrics.UnauthorizedAttempts,
		},
		"latency_ms": map[string]interface{}{
			"cache_lookup":            snapshot.LatencyMetrics.CacheLookupMs,
			"security_validation":     snapshot.LatencyMetrics.SecurityValidationMs,
			"intent_detection":        snapshot.LatencyMetrics.IntentDetectionMs,
			"optimization":            snapshot.LatencyMetrics.OptimizationMs,
			"claude_api":              snapshot.LatencyMetrics.ClaudeAPICallMs,
			"response_compression":    snapshot.LatencyMetrics.ResponseCompressionMs,
			"total":                   snapshot.LatencyMetrics.TotalMs,
		},
		"requests": map[string]interface{}{
			"total": snapshot.RequestCount,
		},
		"optimizers": snapshot.OptimizerMetrics,
	}

	return result
}
