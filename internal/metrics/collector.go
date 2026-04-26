package metrics

import (
	"sync"
	"time"
)

// MetricType represents different metric types
type MetricType string

const (
	MetricTypeGauge     MetricType = "gauge"
	MetricTypeCounter   MetricType = "counter"
	MetricTypeHistogram MetricType = "histogram"
)

// MetricValue represents a single metric value
type MetricValue struct {
	Name      string
	Type      MetricType
	Value     float64
	Labels    map[string]string
	Timestamp time.Time
}

// CacheMetrics tracks caching performance
type CacheMetrics struct {
	HitRate           float64
	FalsePositiveRate float64
	TotalHits         int64
	TotalMisses       int64
	FalsePositives    int64
	LastUpdated       time.Time
}

// SecurityMetrics tracks security events
type SecurityMetrics struct {
	InjectionAttemptsBlocked int64
	RateLimitTriggered       int64
	ValidationFailures       int64
	UnauthorizedAttempts     int64
	LastUpdated              time.Time
}

// TokenMetrics tracks token usage and savings
type TokenMetrics struct {
	TotalInputTokens       int64
	TotalOutputTokens      int64
	TokensSavedByOptimization int64
	SavingsPercent         float64
	CostUSD                float64
	EstimatedSavingsUSD    float64
	LastUpdated            time.Time
}

// LatencyMetrics tracks latency by component
type LatencyMetrics struct {
	CacheLookupMs        float64
	SecurityValidationMs float64
	IntentDetectionMs    float64
	OptimizationMs       float64
	ClaudeAPICallMs      float64
	ResponseCompressionMs float64
	TotalMs              float64
	LastUpdated          time.Time
}

// OptimizerMetrics tracks per-optimizer savings
type OptimizerMetrics struct {
	OptimizerName      string
	TokensSaved        int64
	SavingsPercent     float64
	RequestsProcessed  int64
	CacheHitRate       float64
	Errors             int64
	LastUpdated        time.Time
}

// MetricsCollector collects and aggregates metrics
type MetricsCollector struct {
	mu                 sync.RWMutex
	cacheMetrics       *CacheMetrics
	securityMetrics    *SecurityMetrics
	tokenMetrics       *TokenMetrics
	latencyMetrics     *LatencyMetrics
	optimizerMetrics   map[string]*OptimizerMetrics
	requestCount       int64
	errorCount         int64
	startTime          time.Time
	metricsHistory     []MetricSnapshot
	maxHistorySize     int
}

// MetricSnapshot represents a point-in-time snapshot of metrics
type MetricSnapshot struct {
	Timestamp          time.Time
	CacheMetrics       *CacheMetrics
	SecurityMetrics    *SecurityMetrics
	TokenMetrics       *TokenMetrics
	LatencyMetrics     *LatencyMetrics
	OptimizerMetrics   map[string]*OptimizerMetrics
	RequestCount       int64
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		cacheMetrics:    &CacheMetrics{},
		securityMetrics: &SecurityMetrics{},
		tokenMetrics:    &TokenMetrics{},
		latencyMetrics:  &LatencyMetrics{},
		optimizerMetrics: make(map[string]*OptimizerMetrics),
		startTime:       time.Now(),
		metricsHistory:  make([]MetricSnapshot, 0),
		maxHistorySize:  1440, // Keep 24 hours at 1-minute intervals
	}
}

// RecordCacheHit records a cache hit
func (mc *MetricsCollector) RecordCacheHit() {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.cacheMetrics.TotalHits++
	mc.updateCacheRates()
}

// RecordCacheMiss records a cache miss
func (mc *MetricsCollector) RecordCacheMiss() {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.cacheMetrics.TotalMisses++
	mc.updateCacheRates()
}

// RecordFalsePositive records a false positive in semantic cache
func (mc *MetricsCollector) RecordFalsePositive() {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.cacheMetrics.FalsePositives++
	mc.updateCacheRates()
}

// RecordTokens records token usage
func (mc *MetricsCollector) RecordTokens(inputTokens, outputTokens int64) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.tokenMetrics.TotalInputTokens += inputTokens
	mc.tokenMetrics.TotalOutputTokens += outputTokens
}

// RecordTokenSavings records token savings from optimization
func (mc *MetricsCollector) RecordTokenSavings(tokensSaved int64) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.tokenMetrics.TokensSavedByOptimization += tokensSaved
	mc.updateTokenSavingsPercent()
}

// RecordSecurityEvent records a security event
func (mc *MetricsCollector) RecordSecurityEvent(eventType string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	switch eventType {
	case "injection_attempt":
		mc.securityMetrics.InjectionAttemptsBlocked++
	case "rate_limit":
		mc.securityMetrics.RateLimitTriggered++
	case "validation_failure":
		mc.securityMetrics.ValidationFailures++
	case "unauthorized_access":
		mc.securityMetrics.UnauthorizedAttempts++
	}

	mc.securityMetrics.LastUpdated = time.Now()
}

// RecordLatency records latency for a component
func (mc *MetricsCollector) RecordLatency(cacheMs, securityMs, intentMs, optimizeMs, claudeMs, compressMs float64) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	mc.latencyMetrics.CacheLookupMs = cacheMs
	mc.latencyMetrics.SecurityValidationMs = securityMs
	mc.latencyMetrics.IntentDetectionMs = intentMs
	mc.latencyMetrics.OptimizationMs = optimizeMs
	mc.latencyMetrics.ClaudeAPICallMs = claudeMs
	mc.latencyMetrics.ResponseCompressionMs = compressMs
	mc.latencyMetrics.TotalMs = cacheMs + securityMs + intentMs + optimizeMs + claudeMs + compressMs
	mc.latencyMetrics.LastUpdated = time.Now()
}

// RecordRequest records a request
func (mc *MetricsCollector) RecordRequest(success bool) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	mc.requestCount++
	if !success {
		mc.errorCount++
	}
}

// RecordOptimizerMetric records metrics for a specific optimizer
func (mc *MetricsCollector) RecordOptimizerMetric(optimizerName string, tokensSaved int64, savingsPercent float64, success bool) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if _, exists := mc.optimizerMetrics[optimizerName]; !exists {
		mc.optimizerMetrics[optimizerName] = &OptimizerMetrics{
			OptimizerName: optimizerName,
		}
	}

	optimizer := mc.optimizerMetrics[optimizerName]
	optimizer.TokensSaved += tokensSaved
	optimizer.RequestsProcessed++
	optimizer.SavingsPercent = (float64(optimizer.TokensSaved) / float64(optimizer.RequestsProcessed))
	if !success {
		optimizer.Errors++
	}
	optimizer.LastUpdated = time.Now()
}

// GetMetrics returns current metrics snapshot
func (mc *MetricsCollector) GetMetrics() MetricSnapshot {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	snapshot := MetricSnapshot{
		Timestamp:        time.Now(),
		CacheMetrics:     mc.cacheMetrics,
		SecurityMetrics:  mc.securityMetrics,
		TokenMetrics:     mc.tokenMetrics,
		LatencyMetrics:   mc.latencyMetrics,
		OptimizerMetrics: make(map[string]*OptimizerMetrics),
		RequestCount:     mc.requestCount,
	}

	// Deep copy optimizer metrics
	for name, metrics := range mc.optimizerMetrics {
		snapshot.OptimizerMetrics[name] = &OptimizerMetrics{
			OptimizerName:     metrics.OptimizerName,
			TokensSaved:       metrics.TokensSaved,
			SavingsPercent:    metrics.SavingsPercent,
			RequestsProcessed: metrics.RequestsProcessed,
			CacheHitRate:      metrics.CacheHitRate,
			Errors:            metrics.Errors,
			LastUpdated:       metrics.LastUpdated,
		}
	}

	return snapshot
}

// GetMetricsHistory returns historical metrics
func (mc *MetricsCollector) GetMetricsHistory() []MetricSnapshot {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	history := make([]MetricSnapshot, len(mc.metricsHistory))
	copy(history, mc.metricsHistory)
	return history
}

// SaveSnapshot saves a metrics snapshot to history
func (mc *MetricsCollector) SaveSnapshot() {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	snapshot := MetricSnapshot{
		Timestamp:        time.Now(),
		CacheMetrics:     copyMetrics(mc.cacheMetrics),
		SecurityMetrics:  copySecurityMetrics(mc.securityMetrics),
		TokenMetrics:     copyTokenMetrics(mc.tokenMetrics),
		LatencyMetrics:   copyLatencyMetrics(mc.latencyMetrics),
		OptimizerMetrics: make(map[string]*OptimizerMetrics),
		RequestCount:     mc.requestCount,
	}

	for name, metrics := range mc.optimizerMetrics {
		snapshot.OptimizerMetrics[name] = &OptimizerMetrics{
			OptimizerName:     metrics.OptimizerName,
			TokensSaved:       metrics.TokensSaved,
			SavingsPercent:    metrics.SavingsPercent,
			RequestsProcessed: metrics.RequestsProcessed,
			CacheHitRate:      metrics.CacheHitRate,
			Errors:            metrics.Errors,
			LastUpdated:       metrics.LastUpdated,
		}
	}

	mc.metricsHistory = append(mc.metricsHistory, snapshot)

	// Keep only recent history
	if len(mc.metricsHistory) > mc.maxHistorySize {
		mc.metricsHistory = mc.metricsHistory[1:]
	}
}

// Uptime returns the collector uptime
func (mc *MetricsCollector) Uptime() time.Duration {
	return time.Since(mc.startTime)
}

// Helper functions

func (mc *MetricsCollector) updateCacheRates() {
	total := mc.cacheMetrics.TotalHits + mc.cacheMetrics.TotalMisses
	if total > 0 {
		mc.cacheMetrics.HitRate = float64(mc.cacheMetrics.TotalHits) / float64(total)
		mc.cacheMetrics.FalsePositiveRate = float64(mc.cacheMetrics.FalsePositives) / float64(total)
	}
	mc.cacheMetrics.LastUpdated = time.Now()
}

func (mc *MetricsCollector) updateTokenSavingsPercent() {
	total := mc.tokenMetrics.TotalInputTokens + mc.tokenMetrics.TotalOutputTokens
	if total > 0 {
		mc.tokenMetrics.SavingsPercent = float64(mc.tokenMetrics.TokensSavedByOptimization) / float64(total)
	}
}

func copyMetrics(m *CacheMetrics) *CacheMetrics {
	return &CacheMetrics{
		HitRate:           m.HitRate,
		FalsePositiveRate: m.FalsePositiveRate,
		TotalHits:         m.TotalHits,
		TotalMisses:       m.TotalMisses,
		FalsePositives:    m.FalsePositives,
		LastUpdated:       m.LastUpdated,
	}
}

func copySecurityMetrics(m *SecurityMetrics) *SecurityMetrics {
	return &SecurityMetrics{
		InjectionAttemptsBlocked: m.InjectionAttemptsBlocked,
		RateLimitTriggered:       m.RateLimitTriggered,
		ValidationFailures:       m.ValidationFailures,
		UnauthorizedAttempts:     m.UnauthorizedAttempts,
		LastUpdated:              m.LastUpdated,
	}
}

func copyTokenMetrics(m *TokenMetrics) *TokenMetrics {
	return &TokenMetrics{
		TotalInputTokens:          m.TotalInputTokens,
		TotalOutputTokens:         m.TotalOutputTokens,
		TokensSavedByOptimization: m.TokensSavedByOptimization,
		SavingsPercent:            m.SavingsPercent,
		CostUSD:                   m.CostUSD,
		EstimatedSavingsUSD:       m.EstimatedSavingsUSD,
		LastUpdated:               m.LastUpdated,
	}
}

func copyLatencyMetrics(m *LatencyMetrics) *LatencyMetrics {
	return &LatencyMetrics{
		CacheLookupMs:        m.CacheLookupMs,
		SecurityValidationMs: m.SecurityValidationMs,
		IntentDetectionMs:    m.IntentDetectionMs,
		OptimizationMs:       m.OptimizationMs,
		ClaudeAPICallMs:      m.ClaudeAPICallMs,
		ResponseCompressionMs: m.ResponseCompressionMs,
		TotalMs:              m.TotalMs,
		LastUpdated:          m.LastUpdated,
	}
}
