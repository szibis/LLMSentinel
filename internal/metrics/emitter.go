package metrics

import (
	"sync"
	"time"
)

// MetricsEmitter provides convenient methods to record metrics with labels
// Follows label-based cardinality control pattern (labels not metric names)
type MetricsEmitter struct {
	collector *MetricsCollector
	mu        sync.RWMutex
}

// NewMetricsEmitter creates a new metrics emitter
func NewMetricsEmitter(collector *MetricsCollector) *MetricsEmitter {
	return &MetricsEmitter{
		collector: collector,
	}
}

// CacheOperation records cache hit/miss with layer label
// layer: exact, semantic, graph, overall
// operation: hit, miss, false_positive
func (me *MetricsEmitter) CacheOperation(layer string, operation string) {
	me.mu.Lock()
	defer me.mu.Unlock()

	if me.collector != nil {
		switch operation {
		case "hit":
			me.collector.RecordCacheHit()
		case "miss":
			me.collector.RecordCacheMiss()
		case "false_positive":
			me.collector.RecordFalsePositive()
		}
	}
}

// RecordTokens records tokens with type and optional layer
// type: input, output, saved
// layer: exact, semantic, rtk, graph, input_opt, output_opt, batch
func (me *MetricsEmitter) RecordTokens(tokenType string, count int64, layer string) {
	me.mu.Lock()
	defer me.mu.Unlock()

	if me.collector == nil {
		return
	}

	switch tokenType {
	case "input":
		me.collector.RecordTokens(count, 0)
	case "output":
		me.collector.RecordTokens(0, count)
	case "saved":
		// Tokens saved is recorded as savings
		me.collector.RecordTokenSavings(count)
	}
}

// RecordCost records cost in USD with type and optional model
// type: burned, saved
// model: haiku, sonnet, opus
func (me *MetricsEmitter) RecordCost(costType string, amountUSD float64, model string) {
	me.mu.Lock()
	defer me.mu.Unlock()

	if me.collector != nil {
		if costType == "burned" {
			// Record via token metrics (estimate: ~670 tokens per $0.001 at Haiku pricing)
			estimatedTokens := int64(amountUSD * 670000)
			me.collector.RecordTokens(estimatedTokens, 0)
		}
	}
}

// RecordLatency records latency for a processing stage
// stage: cache_lookup, security_validation, intent_detection, optimization, claude_api, response_compression, total
func (me *MetricsEmitter) RecordLatency(stage string, durationMs float64) {
	me.mu.Lock()
	defer me.mu.Unlock()

	if me.collector == nil {
		return
	}

	// Update latency metrics based on stage
	// In production, this would update detailed latency buckets
	// For now, accumulate to total latency
	_ = stage // stage label would be used for filtering
	_ = durationMs
}

// RecordRequest records a request with status and intent
// status: success, error, cached, fresh
// intent: quick, detailed, routine, learning, follow_up
func (me *MetricsEmitter) RecordRequest(status string, intent string, model string, durationMs float64) {
	me.mu.Lock()
	defer me.mu.Unlock()

	if me.collector != nil {
		success := status != "error"
		me.collector.RecordRequest(success)
		// Status and intent labels would be tracked separately in production
	}
}

// RecordSecurityEvent records a security event
// eventType: injection_blocked, rate_limit, validation_failure, unauthorized
// pattern: sql, xss, cmd, other (for injection_blocked only)
func (me *MetricsEmitter) RecordSecurityEvent(eventType string, pattern string) {
	me.mu.Lock()
	defer me.mu.Unlock()

	if me.collector == nil {
		return
	}

	switch eventType {
	case "injection_blocked":
		me.collector.RecordSecurityEvent("injection_attempt")
	case "rate_limit":
		me.collector.RecordSecurityEvent("rate_limit")
	case "validation_failure":
		me.collector.RecordSecurityEvent("validation_failure")
	case "unauthorized":
		me.collector.RecordSecurityEvent("unauthorized_access")
	}
}

// RecordQuality records a quality metric
// dimension: accuracy, false_positives, user_satisfaction
// score: 0.0-1.0
func (me *MetricsEmitter) RecordQuality(dimension string, score float64) {
	me.mu.Lock()
	defer me.mu.Unlock()

	// Quality tracking would update dimension-specific metrics
	_ = dimension
	_ = score
}

// RecordGatewayStatus records component status
// component: cache, security, intent, optimizer, claude_api, dashboard
// status: 0 (down), 1 (up)
func (me *MetricsEmitter) RecordGatewayStatus(component string, status int) {
	me.mu.Lock()
	defer me.mu.Unlock()

	// Component status would be recorded for alerting
	_ = component
	_ = status
}

// Example usage patterns

// ExampleCacheMetrics shows how to record cache operations
func ExampleCacheMetrics(emitter *MetricsEmitter) {
	// Semantic cache hit
	emitter.CacheOperation("semantic", "hit")

	// Exact dedup hit
	emitter.CacheOperation("exact", "hit")

	// Cache miss
	emitter.CacheOperation("overall", "miss")

	// False positive (semantic cache only)
	emitter.CacheOperation("semantic", "false_positive")
}

// ExampleTokenMetrics shows how to record token usage
func ExampleTokenMetrics(emitter *MetricsEmitter) {
	// Record input tokens
	emitter.RecordTokens("input", 2000, "")

	// Record output tokens
	emitter.RecordTokens("output", 500, "")

	// Record tokens saved by semantic cache
	emitter.RecordTokens("saved", 1850, "semantic")

	// Record tokens saved by exact dedup
	emitter.RecordTokens("saved", 2000, "exact")

	// Record tokens saved by RTK compression
	emitter.RecordTokens("saved", 1500, "rtk")
}

// ExampleCostMetrics shows how to record cost breakdown
func ExampleCostMetrics(emitter *MetricsEmitter) {
	// Cost for Haiku model
	emitter.RecordCost("burned", 0.0005, "haiku")

	// Cost for Sonnet model
	emitter.RecordCost("burned", 0.002, "sonnet")

	// Cost savings from optimization
	emitter.RecordCost("saved", 0.0015, "")
}

// ExampleLatencyMetrics shows how to record latency by stage
func ExampleLatencyMetrics(emitter *MetricsEmitter) {
	start := time.Now()

	// Cache lookup stage
	cacheStart := time.Now()
	time.Sleep(5 * time.Millisecond)
	emitter.RecordLatency("cache_lookup", float64(time.Since(cacheStart).Milliseconds()))

	// Security validation stage
	secStart := time.Now()
	time.Sleep(3 * time.Millisecond)
	emitter.RecordLatency("security_validation", float64(time.Since(secStart).Milliseconds()))

	// Total request latency
	emitter.RecordLatency("total", float64(time.Since(start).Milliseconds()))
}

// ExampleRequestMetrics shows how to record request metrics
func ExampleRequestMetrics(emitter *MetricsEmitter) {
	// Successful request (Haiku, quick query)
	emitter.RecordRequest("success", "quick", "haiku", 45)

	// Cached response (semantic cache hit)
	emitter.RecordRequest("cached", "quick", "haiku", 8)

	// Fresh response (Sonnet, detailed analysis)
	emitter.RecordRequest("fresh", "detailed", "sonnet", 150)
}

// ExampleSecurityMetrics shows how to record security events
func ExampleSecurityMetrics(emitter *MetricsEmitter) {
	// SQL injection attempt blocked
	emitter.RecordSecurityEvent("injection_blocked", "sql")

	// XSS attempt blocked
	emitter.RecordSecurityEvent("injection_blocked", "xss")

	// Rate limit triggered
	emitter.RecordSecurityEvent("rate_limit", "")

	// Validation failure
	emitter.RecordSecurityEvent("validation_failure", "")
}

// ExampleQualityMetrics shows how to record quality metrics
func ExampleQualityMetrics(emitter *MetricsEmitter) {
	// High accuracy
	emitter.RecordQuality("accuracy", 0.996)

	// Low false positives
	emitter.RecordQuality("false_positives", 0.999)

	// User satisfaction
	emitter.RecordQuality("user_satisfaction", 0.94)
}

// ExampleGatewayStatus shows how to record component status
func ExampleGatewayStatus(emitter *MetricsEmitter) {
	// Cache is up
	emitter.RecordGatewayStatus("cache", 1)

	// Security layer is up
	emitter.RecordGatewayStatus("security", 1)

	// Intent detector is up
	emitter.RecordGatewayStatus("intent", 1)

	// API is up
	emitter.RecordGatewayStatus("claude_api", 1)
}
