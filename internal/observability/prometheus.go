// Package observability provides metrics export for monitoring and observability.
package observability

import (
	"fmt"
	"sync"
	"time"
)

// PrometheusMetrics exposes metrics in Prometheus text format.
type PrometheusMetrics struct {
	mu sync.RWMutex

	// Counters
	TotalRequests     int64
	CacheHits         int64
	CacheMisses       int64
	BatchQueued       int64
	ModelSwitches     int64
	DirectRequests    int64

	// Gauges
	CacheSize         int64
	QueueSize         int64
	CostThisMonth     float64
	ActiveSessions    int64

	// Histogram buckets (latency in milliseconds)
	LatencyBuckets map[float64]int64 // key is bucket upper bound (10, 50, 100, 500, 1000)

	// Histogram buckets (token error)
	TokenErrorBuckets map[float64]int64 // key is bucket upper bound (0.05, 0.10, 0.15, 0.20, 0.50, 1.0)

	// Cost per request tracking
	CostPerRequestSum float64
	CostPerRequestCount int64

	// Model usage breakdown
	ModelUsage map[string]int64 // "haiku", "sonnet", "opus"

	// Task type usage
	TaskTypeUsage map[string]int64

	// Percentiles (computed from recent samples)
	LatencyP50 float64
	LatencyP95 float64
	LatencyP99 float64
}

// NewPrometheusMetrics creates a new metrics collector.
func NewPrometheusMetrics() *PrometheusMetrics {
	return &PrometheusMetrics{
		LatencyBuckets:    make(map[float64]int64),
		TokenErrorBuckets: make(map[float64]int64),
		ModelUsage:        make(map[string]int64),
		TaskTypeUsage:     make(map[string]int64),
	}
}

// Initialize sets up default buckets.
func (pm *PrometheusMetrics) Initialize() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Latency buckets in milliseconds
	latencyBuckets := []float64{10, 50, 100, 500, 1000, 5000}
	for _, b := range latencyBuckets {
		pm.LatencyBuckets[b] = 0
	}

	// Token error buckets
	errorBuckets := []float64{0.05, 0.10, 0.15, 0.20, 0.50, 1.0}
	for _, b := range errorBuckets {
		pm.TokenErrorBuckets[b] = 0
	}

	// Model usage
	pm.ModelUsage["haiku"] = 0
	pm.ModelUsage["sonnet"] = 0
	pm.ModelUsage["opus"] = 0
}

// RecordRequest records a request completion.
func (pm *PrometheusMetrics) RecordRequest(model, taskType string, latencyMs float64, tokenError float64, costUSD float64, cached, batched bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.TotalRequests++

	if cached {
		pm.CacheHits++
	} else {
		pm.CacheMisses++
	}

	if batched {
		pm.BatchQueued++
	} else {
		pm.DirectRequests++
	}

	// Record latency in buckets
	for bucket := range pm.LatencyBuckets {
		if latencyMs <= bucket {
			pm.LatencyBuckets[bucket]++
		}
	}

	// Record token error in buckets
	for bucket := range pm.TokenErrorBuckets {
		if tokenError <= bucket {
			pm.TokenErrorBuckets[bucket]++
		}
	}

	// Track model usage
	if _, ok := pm.ModelUsage[model]; ok {
		pm.ModelUsage[model]++
	}

	// Track task type usage
	if _, ok := pm.TaskTypeUsage[taskType]; ok {
		pm.TaskTypeUsage[taskType]++
	} else {
		pm.TaskTypeUsage[taskType] = 1
	}

	// Track cost per request
	pm.CostPerRequestSum += costUSD
	pm.CostPerRequestCount++
}

// UpdateGauges updates gauge values (call periodically).
func (pm *PrometheusMetrics) UpdateGauges(cacheSize, queueSize, activeSessions int64, costThisMonth float64) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.CacheSize = cacheSize
	pm.QueueSize = queueSize
	pm.ActiveSessions = activeSessions
	pm.CostThisMonth = costThisMonth
}

// RecordModelSwitch records when a model was switched.
func (pm *PrometheusMetrics) RecordModelSwitch() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.ModelSwitches++
}

// SetPercentiles sets calculated percentile values.
func (pm *PrometheusMetrics) SetPercentiles(p50, p95, p99 float64) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.LatencyP50 = p50
	pm.LatencyP95 = p95
	pm.LatencyP99 = p99
}

// ExportPrometheus returns metrics in Prometheus text format.
func (pm *PrometheusMetrics) ExportPrometheus() string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	output := "# HELP claude_escalate_requests_total Total number of requests processed\n"
	output += "# TYPE claude_escalate_requests_total counter\n"
	output += fmt.Sprintf("claude_escalate_requests_total %d\n\n", pm.TotalRequests)

	output += "# HELP claude_escalate_cache_hits_total Total cache hits\n"
	output += "# TYPE claude_escalate_cache_hits_total counter\n"
	output += fmt.Sprintf("claude_escalate_cache_hits_total %d\n\n", pm.CacheHits)

	output += "# HELP claude_escalate_cache_misses_total Total cache misses\n"
	output += "# TYPE claude_escalate_cache_misses_total counter\n"
	output += fmt.Sprintf("claude_escalate_cache_misses_total %d\n\n", pm.CacheMisses)

	output += "# HELP claude_escalate_batch_queued_total Total batch requests queued\n"
	output += "# TYPE claude_escalate_batch_queued_total counter\n"
	output += fmt.Sprintf("claude_escalate_batch_queued_total %d\n\n", pm.BatchQueued)

	output += "# HELP claude_escalate_direct_requests_total Total direct requests\n"
	output += "# TYPE claude_escalate_direct_requests_total counter\n"
	output += fmt.Sprintf("claude_escalate_direct_requests_total %d\n\n", pm.DirectRequests)

	output += "# HELP claude_escalate_model_switches_total Total model switches\n"
	output += "# TYPE claude_escalate_model_switches_total counter\n"
	output += fmt.Sprintf("claude_escalate_model_switches_total %d\n\n", pm.ModelSwitches)

	// Model usage
	output += "# HELP claude_escalate_model_usage_total Total requests per model\n"
	output += "# TYPE claude_escalate_model_usage_total counter\n"
	for model, count := range pm.ModelUsage {
		output += fmt.Sprintf("claude_escalate_model_usage_total{model=\"%s\"} %d\n", model, count)
	}
	output += "\n"

	// Task type usage
	output += "# HELP claude_escalate_task_type_usage_total Total requests per task type\n"
	output += "# TYPE claude_escalate_task_type_usage_total counter\n"
	for taskType, count := range pm.TaskTypeUsage {
		output += fmt.Sprintf("claude_escalate_task_type_usage_total{task_type=\"%s\"} %d\n", taskType, count)
	}
	output += "\n"

	// Gauges
	output += "# HELP claude_escalate_cache_size Current cache size in entries\n"
	output += "# TYPE claude_escalate_cache_size gauge\n"
	output += fmt.Sprintf("claude_escalate_cache_size %d\n\n", pm.CacheSize)

	output += "# HELP claude_escalate_queue_size Current queue size in entries\n"
	output += "# TYPE claude_escalate_queue_size gauge\n"
	output += fmt.Sprintf("claude_escalate_queue_size %d\n\n", pm.QueueSize)

	output += "# HELP claude_escalate_cost_usd_total Total cost in USD\n"
	output += "# TYPE claude_escalate_cost_usd_total gauge\n"
	output += fmt.Sprintf("claude_escalate_cost_usd_total %.2f\n\n", pm.CostThisMonth)

	output += "# HELP claude_escalate_active_sessions Current active sessions\n"
	output += "# TYPE claude_escalate_active_sessions gauge\n"
	output += fmt.Sprintf("claude_escalate_active_sessions %d\n\n", pm.ActiveSessions)

	// Latency histogram
	output += "# HELP claude_escalate_request_duration_seconds Request latency in seconds\n"
	output += "# TYPE claude_escalate_request_duration_seconds histogram\n"
	for _, bucket := range []float64{0.01, 0.05, 0.1, 0.5, 1.0, 5.0} {
		output += fmt.Sprintf("claude_escalate_request_duration_seconds_bucket{le=\"%.2f\"} %d\n",
			bucket/1000, pm.LatencyBuckets[bucket*1000])
	}
	output += fmt.Sprintf("claude_escalate_request_duration_seconds_sum %.2f\n", pm.LatencyP50)
	output += fmt.Sprintf("claude_escalate_request_duration_seconds_count %d\n\n", pm.TotalRequests)

	// Percentiles as gauges
	output += "# HELP claude_escalate_request_duration_p50_seconds P50 request latency\n"
	output += "# TYPE claude_escalate_request_duration_p50_seconds gauge\n"
	output += fmt.Sprintf("claude_escalate_request_duration_p50_seconds %.3f\n\n", pm.LatencyP50/1000)

	output += "# HELP claude_escalate_request_duration_p95_seconds P95 request latency\n"
	output += "# TYPE claude_escalate_request_duration_p95_seconds gauge\n"
	output += fmt.Sprintf("claude_escalate_request_duration_p95_seconds %.3f\n\n", pm.LatencyP95/1000)

	output += "# HELP claude_escalate_request_duration_p99_seconds P99 request latency\n"
	output += "# TYPE claude_escalate_request_duration_p99_seconds gauge\n"
	output += fmt.Sprintf("claude_escalate_request_duration_p99_seconds %.3f\n\n", pm.LatencyP99/1000)

	// Cache hit rate
	hitRate := 0.0
	if pm.TotalRequests > 0 {
		hitRate = float64(pm.CacheHits) / float64(pm.TotalRequests)
	}
	output += "# HELP claude_escalate_cache_hit_rate Cache hit rate\n"
	output += "# TYPE claude_escalate_cache_hit_rate gauge\n"
	output += fmt.Sprintf("claude_escalate_cache_hit_rate %.3f\n\n", hitRate)

	// Average cost per request
	avgCost := 0.0
	if pm.CostPerRequestCount > 0 {
		avgCost = pm.CostPerRequestSum / float64(pm.CostPerRequestCount)
	}
	output += "# HELP claude_escalate_cost_per_request_usd Average cost per request\n"
	output += "# TYPE claude_escalate_cost_per_request_usd gauge\n"
	output += fmt.Sprintf("claude_escalate_cost_per_request_usd %.6f\n\n", avgCost)

	// Timestamp
	output += "# HELP claude_escalate_export_timestamp Export timestamp\n"
	output += "# TYPE claude_escalate_export_timestamp gauge\n"
	output += fmt.Sprintf("claude_escalate_export_timestamp %.0f\n", float64(time.Now().Unix()))

	return output
}

// GetMetricsSnapshot returns current metrics as a map.
func (pm *PrometheusMetrics) GetMetricsSnapshot() map[string]interface{} {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	hitRate := 0.0
	if pm.TotalRequests > 0 {
		hitRate = float64(pm.CacheHits) / float64(pm.TotalRequests)
	}

	avgCost := 0.0
	if pm.CostPerRequestCount > 0 {
		avgCost = pm.CostPerRequestSum / float64(pm.CostPerRequestCount)
	}

	return map[string]interface{}{
		"total_requests":       pm.TotalRequests,
		"cache_hits":           pm.CacheHits,
		"cache_hit_rate":       hitRate,
		"batch_queued":         pm.BatchQueued,
		"model_switches":       pm.ModelSwitches,
		"cache_size":           pm.CacheSize,
		"queue_size":           pm.QueueSize,
		"cost_this_month":      pm.CostThisMonth,
		"cost_per_request":     avgCost,
		"active_sessions":      pm.ActiveSessions,
		"model_usage":          pm.ModelUsage,
		"task_type_usage":      pm.TaskTypeUsage,
		"latency_p50":          pm.LatencyP50,
		"latency_p95":          pm.LatencyP95,
		"latency_p99":          pm.LatencyP99,
	}
}
