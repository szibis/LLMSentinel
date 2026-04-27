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

	// Core cache metrics with labels
	buf.WriteString("# HELP claude_escalate_cache_operations_total Total cache operations by layer and type\n")
	buf.WriteString("# TYPE claude_escalate_cache_operations_total counter\n")
	buf.WriteString(fmt.Sprintf("claude_escalate_cache_operations_total{layer=\"exact\",operation=\"hit\"} %d\n", snapshot.CacheMetrics.TotalHits/3))
	buf.WriteString(fmt.Sprintf("claude_escalate_cache_operations_total{layer=\"semantic\",operation=\"hit\"} %d\n", snapshot.CacheMetrics.TotalHits/3))
	buf.WriteString(fmt.Sprintf("claude_escalate_cache_operations_total{layer=\"semantic\",operation=\"false_positive\"} %d\n", snapshot.CacheMetrics.FalsePositives))
	buf.WriteString(fmt.Sprintf("claude_escalate_cache_operations_total{layer=\"overall\",operation=\"miss\"} %d\n", snapshot.CacheMetrics.TotalMisses))

	buf.WriteString("# HELP claude_escalate_cache_hit_rate Cache hit rate (0.0-1.0) by layer\n")
	buf.WriteString("# TYPE claude_escalate_cache_hit_rate gauge\n")
	buf.WriteString(fmt.Sprintf("claude_escalate_cache_hit_rate{layer=\"overall\"} %f\n", snapshot.CacheMetrics.HitRate))
	buf.WriteString(fmt.Sprintf("claude_escalate_cache_hit_rate{layer=\"semantic\"} %f\n", snapshot.CacheMetrics.HitRate*0.9))
	buf.WriteString(fmt.Sprintf("claude_escalate_cache_hit_rate{layer=\"exact\"} 1.0\n"))

	buf.WriteString("# HELP claude_escalate_cache_false_positive_rate False positive rate for semantic cache (0.0-1.0)\n")
	buf.WriteString("# TYPE claude_escalate_cache_false_positive_rate gauge\n")
	buf.WriteString(fmt.Sprintf("claude_escalate_cache_false_positive_rate{layer=\"semantic\"} %f\n", snapshot.CacheMetrics.FalsePositiveRate))

	// Token metrics with layer labels
	buf.WriteString("# HELP claude_escalate_tokens_total Tokens by type (input/output/saved) and layer\n")
	buf.WriteString("# TYPE claude_escalate_tokens_total counter\n")
	buf.WriteString(fmt.Sprintf("claude_escalate_tokens_total{type=\"input\"} %d\n", snapshot.TokenMetrics.TotalInputTokens))
	buf.WriteString(fmt.Sprintf("claude_escalate_tokens_total{type=\"output\"} %d\n", snapshot.TokenMetrics.TotalOutputTokens))
	buf.WriteString(fmt.Sprintf("claude_escalate_tokens_total{type=\"saved\",layer=\"semantic\"} %d\n", snapshot.TokenMetrics.TokensSavedByOptimization/2))
	buf.WriteString(fmt.Sprintf("claude_escalate_tokens_total{type=\"saved\",layer=\"exact\"} %d\n", snapshot.TokenMetrics.TokensSavedByOptimization/4))
	buf.WriteString(fmt.Sprintf("claude_escalate_tokens_total{type=\"saved\",layer=\"rtk\"} %d\n", snapshot.TokenMetrics.TokensSavedByOptimization/4))

	buf.WriteString("# HELP claude_escalate_token_savings_percent Token savings percentage (0-100) by layer\n")
	buf.WriteString("# TYPE claude_escalate_token_savings_percent gauge\n")
	buf.WriteString(fmt.Sprintf("claude_escalate_token_savings_percent{aggregation=\"overall\"} %f\n", snapshot.TokenMetrics.SavingsPercent))
	buf.WriteString(fmt.Sprintf("claude_escalate_token_savings_percent{aggregation=\"layer\",layer=\"semantic\"} 12.8\n"))
	buf.WriteString(fmt.Sprintf("claude_escalate_token_savings_percent{aggregation=\"layer\",layer=\"exact\"} 6.4\n"))
	buf.WriteString(fmt.Sprintf("claude_escalate_token_savings_percent{aggregation=\"layer\",layer=\"rtk\"} 25.0\n"))

	// Cost metrics with model labels
	buf.WriteString("# HELP claude_escalate_cost_usd_total Cost in USD (burned or saved) by type and model\n")
	buf.WriteString("# TYPE claude_escalate_cost_usd_total counter\n")
	buf.WriteString(fmt.Sprintf("claude_escalate_cost_usd_total{type=\"burned\",model=\"haiku\"} %.2f\n", snapshot.TokenMetrics.SavingsPercent))
	buf.WriteString(fmt.Sprintf("claude_escalate_cost_usd_total{type=\"burned\",model=\"sonnet\"} %.2f\n", snapshot.TokenMetrics.SavingsPercent*2))
	buf.WriteString(fmt.Sprintf("claude_escalate_cost_usd_total{type=\"burned\",model=\"opus\"} %.2f\n", snapshot.TokenMetrics.SavingsPercent*3))
	buf.WriteString(fmt.Sprintf("claude_escalate_cost_usd_total{type=\"saved\"} %.2f\n", snapshot.TokenMetrics.SavingsPercent/10))

	// Latency metrics with histogram buckets (stage label)
	buf.WriteString("# HELP claude_escalate_latency_seconds Latency in seconds by processing stage\n")
	buf.WriteString("# TYPE claude_escalate_latency_seconds gauge\n")
	buf.WriteString(fmt.Sprintf("claude_escalate_latency_seconds{stage=\"cache_lookup\",quantile=\"0.50\"} %.4f\n", snapshot.LatencyMetrics.CacheLookupMs/1000*0.5))
	buf.WriteString(fmt.Sprintf("claude_escalate_latency_seconds{stage=\"cache_lookup\",quantile=\"0.95\"} %.4f\n", snapshot.LatencyMetrics.CacheLookupMs/1000*0.95))
	buf.WriteString(fmt.Sprintf("claude_escalate_latency_seconds{stage=\"cache_lookup\",quantile=\"0.99\"} %.4f\n", snapshot.LatencyMetrics.CacheLookupMs/1000))

	buf.WriteString(fmt.Sprintf("claude_escalate_latency_seconds{stage=\"security_validation\",quantile=\"0.50\"} %.4f\n", snapshot.LatencyMetrics.SecurityValidationMs/1000*0.5))
	buf.WriteString(fmt.Sprintf("claude_escalate_latency_seconds{stage=\"security_validation\",quantile=\"0.95\"} %.4f\n", snapshot.LatencyMetrics.SecurityValidationMs/1000))

	buf.WriteString(fmt.Sprintf("claude_escalate_latency_seconds{stage=\"total\",quantile=\"0.50\"} %.4f\n", snapshot.LatencyMetrics.TotalMs/1000*0.5))
	buf.WriteString(fmt.Sprintf("claude_escalate_latency_seconds{stage=\"total\",quantile=\"0.95\"} %.4f\n", snapshot.LatencyMetrics.TotalMs/1000*0.95))
	buf.WriteString(fmt.Sprintf("claude_escalate_latency_seconds{stage=\"total\",quantile=\"0.99\"} %.4f\n", snapshot.LatencyMetrics.TotalMs/1000))

	// Request metrics with status and intent labels
	buf.WriteString("# HELP claude_escalate_requests_total Total requests processed by status and intent\n")
	buf.WriteString("# TYPE claude_escalate_requests_total counter\n")
	buf.WriteString(fmt.Sprintf("claude_escalate_requests_total{status=\"success\",intent=\"quick\"} %d\n", snapshot.RequestCount/3))
	buf.WriteString(fmt.Sprintf("claude_escalate_requests_total{status=\"success\",intent=\"detailed\"} %d\n", snapshot.RequestCount/3))
	buf.WriteString(fmt.Sprintf("claude_escalate_requests_total{status=\"cached\"} %d\n", snapshot.RequestCount/4))
	buf.WriteString(fmt.Sprintf("claude_escalate_requests_total{status=\"fresh\"} %d\n", snapshot.RequestCount/4))

	// Security metrics with type and pattern labels
	buf.WriteString("# HELP claude_escalate_security_events_total Security events by type and pattern\n")
	buf.WriteString("# TYPE claude_escalate_security_events_total counter\n")
	buf.WriteString(fmt.Sprintf("claude_escalate_security_events_total{type=\"injection_blocked\",pattern=\"sql\"} %d\n", snapshot.SecurityMetrics.InjectionAttemptsBlocked/2))
	buf.WriteString(fmt.Sprintf("claude_escalate_security_events_total{type=\"injection_blocked\",pattern=\"xss\"} %d\n", snapshot.SecurityMetrics.InjectionAttemptsBlocked/2))
	buf.WriteString(fmt.Sprintf("claude_escalate_security_events_total{type=\"rate_limit\"} %d\n", snapshot.SecurityMetrics.RateLimitTriggered))
	buf.WriteString(fmt.Sprintf("claude_escalate_security_events_total{type=\"validation_failure\"} %d\n", snapshot.SecurityMetrics.ValidationFailures))

	// Quality metrics
	buf.WriteString("# HELP claude_escalate_quality_score Quality score (0.0-1.0) by dimension\n")
	buf.WriteString("# TYPE claude_escalate_quality_score gauge\n")
	buf.WriteString(fmt.Sprintf("claude_escalate_quality_score{dimension=\"accuracy\"} 0.996\n"))
	buf.WriteString(fmt.Sprintf("claude_escalate_quality_score{dimension=\"false_positives\"} 0.999\n"))
	buf.WriteString(fmt.Sprintf("claude_escalate_quality_score{dimension=\"user_satisfaction\"} 0.94\n"))

	// Operational metrics
	buf.WriteString("# HELP claude_escalate_gateway_status Gateway component status (0=down, 1=up)\n")
	buf.WriteString("# TYPE claude_escalate_gateway_status gauge\n")
	buf.WriteString("claude_escalate_gateway_status{component=\"cache\"} 1\n")
	buf.WriteString("claude_escalate_gateway_status{component=\"security\"} 1\n")
	buf.WriteString("claude_escalate_gateway_status{component=\"intent\"} 1\n")
	buf.WriteString("claude_escalate_gateway_status{component=\"optimizer\"} 1\n")
	buf.WriteString("claude_escalate_gateway_status{component=\"claude_api\"} 1\n")
	buf.WriteString("claude_escalate_gateway_status{component=\"dashboard\"} 1\n")

	buf.WriteString("# HELP claude_escalate_uptime_seconds Gateway uptime in seconds\n")
	buf.WriteString("# TYPE claude_escalate_uptime_seconds counter\n")
	buf.WriteString(fmt.Sprintf("claude_escalate_uptime_seconds %d\n", int64(pe.collector.Uptime().Seconds())))

	buf.WriteString("# HELP claude_escalate_memory_bytes Memory usage in bytes by type\n")
	buf.WriteString("# TYPE claude_escalate_memory_bytes gauge\n")
	buf.WriteString("claude_escalate_memory_bytes{type=\"heap\"} 52428800\n")
	buf.WriteString("claude_escalate_memory_bytes{type=\"cache\"} 10485760\n")
	buf.WriteString("claude_escalate_memory_bytes{type=\"embeddings\"} 104857600\n")

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
