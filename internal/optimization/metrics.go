package optimization

import (
	"sync"
	"time"
)

// MetricsSummary provides real-world impact analysis
type MetricsSummary struct {
	// Cache metrics
	TotalRequests           int64
	CacheHits               int64
	CacheMisses             int64
	CacheHitRate            float64

	// Batch metrics
	BatchDecisions          int64
	BatchRequests           int64
	DirectRequests          int64
	ModelSwitches           int64

	// Cost savings
	TotalEstimatedCost      float64
	TotalActualCost         float64
	TotalSavings            float64
	OverallSavingsPercent   float64

	// Timing
	AverageBatchWaitTime    time.Duration
	CacheAverageAge         time.Duration

	// ROI analysis
	ROIScore                float64
	CostPerRequest          float64
	BreakdownByStrategy     map[string]StrategyMetrics
}

// StrategyMetrics shows performance per optimization type
type StrategyMetrics struct {
	Count              int64
	EstimatedCost      float64
	ActualCost         float64
	Savings            float64
	SavingsPercent     float64
	AverageWaitTime    time.Duration
}

// Metrics tracks real-world optimization impact
type Metrics struct {
	mu sync.RWMutex

	totalRequests      int64
	cacheHits          int64
	cacheMisses        int64
	batchDecisions     int64
	batchRequests      int64
	directRequests     int64
	modelSwitches      int64

	totalEstimatedCost float64
	totalActualCost    float64
	totalSavings       float64

	cacheEvents        []CacheEvent
	batchEvents        []BatchEvent
	directEvents       []DirectEvent
	switchEvents       []SwitchEvent

	// Event logging limits to prevent memory exhaustion
	maxEventCount      int
}

// Event types for detailed tracking
type CacheEvent struct {
	Timestamp      time.Time
	Prompt         string
	Model          string
	SavedCost      float64
	Age            time.Duration
}

type BatchEvent struct {
	Timestamp      time.Time
	Prompt         string
	Model          string
	SavedCost      float64
	WaitTime       time.Duration
	EstimatedCost  float64
}

type DirectEvent struct {
	Timestamp      time.Time
	Prompt         string
	Model          string
	ActualCost     float64
}

type SwitchEvent struct {
	Timestamp      time.Time
	FromModel      string
	ToModel        string
	SavedCost      float64
}

// NewMetrics creates a new metrics tracker
func NewMetrics() *Metrics {
	return &Metrics{
		cacheEvents:   make([]CacheEvent, 0, 100),
		batchEvents:   make([]BatchEvent, 0, 100),
		directEvents:  make([]DirectEvent, 0, 100),
		switchEvents:  make([]SwitchEvent, 0, 100),
		maxEventCount: 10000, // Cap total events to prevent unbounded growth
	}
}

// evictOldEvents removes oldest events if count exceeds limit (called within lock)
func (m *Metrics) evictOldEvents() {
	totalEvents := len(m.cacheEvents) + len(m.batchEvents) + len(m.directEvents) + len(m.switchEvents)
	if totalEvents <= m.maxEventCount {
		return
	}

	// Evict oldest 10% of events from each type
	evictCount := totalEvents / 10
	if len(m.cacheEvents) > 0 && evictCount > 0 {
		keep := len(m.cacheEvents) - evictCount/4
		if keep < 0 {
			keep = 0
		}
		m.cacheEvents = m.cacheEvents[len(m.cacheEvents)-keep:]
	}
	if len(m.batchEvents) > 0 && evictCount > 0 {
		keep := len(m.batchEvents) - evictCount/4
		if keep < 0 {
			keep = 0
		}
		m.batchEvents = m.batchEvents[len(m.batchEvents)-keep:]
	}
	if len(m.directEvents) > 0 && evictCount > 0 {
		keep := len(m.directEvents) - evictCount/4
		if keep < 0 {
			keep = 0
		}
		m.directEvents = m.directEvents[len(m.directEvents)-keep:]
	}
	if len(m.switchEvents) > 0 && evictCount > 0 {
		keep := len(m.switchEvents) - evictCount/4
		if keep < 0 {
			keep = 0
		}
		m.switchEvents = m.switchEvents[len(m.switchEvents)-keep:]
	}
}

// RecordCacheHit records a cache hit with savings
func (m *Metrics) RecordCacheHit(prompt, model string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.totalRequests++
	m.cacheHits++
	// Cache hit costs ~$0.00015 per request vs direct cost (~$0.008)
	// Savings is approximately 99.8% of direct cost
	const averageDirectCost = 0.008
	const cacheSavingsPercent = 0.998
	savedCost := averageDirectCost * cacheSavingsPercent
	m.totalSavings += savedCost

	m.cacheEvents = append(m.cacheEvents, CacheEvent{
		Timestamp: time.Now(),
		Prompt:    prompt,
		Model:     model,
		SavedCost: savedCost,
	})

	m.evictOldEvents()
}

// RecordBatchDecision records a batch queuing decision
func (m *Metrics) RecordBatchDecision(prompt, model string, queued bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.totalRequests++
	if queued {
		m.batchRequests++
		// Batch saves 50% vs direct
		const estimatedCost = 0.016
		const batchDiscount = 0.5
		actualCost := estimatedCost * batchDiscount
		savedCost := estimatedCost - actualCost
		m.totalSavings += savedCost

		m.batchEvents = append(m.batchEvents, BatchEvent{
			Timestamp:     time.Now(),
			Prompt:        prompt,
			Model:         model,
			SavedCost:     savedCost,
			EstimatedCost: estimatedCost,
			WaitTime:      5 * time.Second, // Average wait
		})

		m.evictOldEvents()
	}
}

// RecordDirect records a direct API call without optimization
func (m *Metrics) RecordDirect(prompt, model string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.totalRequests++
	m.directRequests++
	const directCost = 0.016
	m.totalActualCost += directCost

	m.directEvents = append(m.directEvents, DirectEvent{
		Timestamp: time.Now(),
		Prompt:    prompt,
		Model:     model,
		ActualCost: directCost,
	})

	m.evictOldEvents()
}

// RecordModelSwitch records a model downgrade
func (m *Metrics) RecordModelSwitch(prompt, fromModel, toModel string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.totalRequests++
	m.modelSwitches++
	// Model switch (Sonnet to Haiku) saves ~75%
	const estimatedCost = 0.016
	const estimatedDowngradeCost = 0.004 // Approximate for Haiku
	savedCost := estimatedCost - estimatedDowngradeCost
	m.totalSavings += savedCost

	m.switchEvents = append(m.switchEvents, SwitchEvent{
		Timestamp: time.Now(),
		FromModel: fromModel,
		ToModel:   toModel,
		SavedCost: savedCost,
	})

	m.evictOldEvents()
}

// GetSummary returns a metrics summary for analysis
func (m *Metrics) GetSummary() MetricsSummary {
	m.mu.RLock()
	defer m.mu.RUnlock()

	summary := MetricsSummary{
		TotalRequests:      m.totalRequests,
		CacheHits:          m.cacheHits,
		CacheMisses:        m.cacheMisses,
		BatchDecisions:     m.batchDecisions,
		BatchRequests:      m.batchRequests,
		DirectRequests:     m.directRequests,
		ModelSwitches:      m.modelSwitches,
		TotalEstimatedCost: m.totalEstimatedCost,
		TotalActualCost:    m.totalActualCost,
		TotalSavings:       m.totalSavings,
	}

	// Calculate derived metrics
	if m.totalRequests > 0 {
		summary.CacheHitRate = float64(m.cacheHits) / float64(m.totalRequests) * 100
		estimatedCost := float64(m.totalRequests) * 0.016 // Average cost per request
		if estimatedCost > 0 {
			summary.OverallSavingsPercent = (summary.TotalSavings / estimatedCost) * 100
		}
		summary.CostPerRequest = summary.TotalActualCost / float64(m.totalRequests)
	}

	// Calculate ROI (avoid division by zero)
	if summary.TotalActualCost > 0 && summary.TotalSavings > 0 {
		summary.ROIScore = summary.TotalSavings / summary.TotalActualCost
	}

	// Breakdown by strategy
	summary.BreakdownByStrategy = map[string]StrategyMetrics{
		"cache": {
			Count:          m.cacheHits,
			Savings:        float64(m.cacheHits) * 0.008, // Approximation
			SavingsPercent: 99.8,
		},
		"batch": {
			Count:          m.batchRequests,
			SavingsPercent: 50.0,
		},
		"model_switch": {
			Count:          m.modelSwitches,
			SavingsPercent: 75.0,
		},
		"direct": {
			Count:          m.directRequests,
			SavingsPercent: 0.0,
		},
	}

	return summary
}

// GetCacheStats returns cache-specific statistics
func (m *Metrics) GetCacheStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	totalCacheSavings := 0.0
	for _, event := range m.cacheEvents {
		totalCacheSavings += event.SavedCost
	}

	avgAge := time.Duration(0)
	if len(m.cacheEvents) > 0 {
		totalAge := time.Duration(0)
		for _, event := range m.cacheEvents {
			totalAge += event.Age
		}
		avgAge = totalAge / time.Duration(len(m.cacheEvents))
	}

	hitRate := 0.0
	if m.totalRequests > 0 {
		hitRate = float64(m.cacheHits) / float64(m.totalRequests) * 100
	}

	return map[string]interface{}{
		"total_hits":       m.cacheHits,
		"hit_rate":         hitRate,
		"total_savings":    totalCacheSavings,
		"average_age":      avgAge,
		"events_count":     len(m.cacheEvents),
	}
}

// GetBatchStats returns batch-specific statistics
func (m *Metrics) GetBatchStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	totalBatchSavings := 0.0
	avgWaitTime := time.Duration(0)

	if len(m.batchEvents) > 0 {
		for _, event := range m.batchEvents {
			totalBatchSavings += event.SavedCost
			avgWaitTime += event.WaitTime
		}
		avgWaitTime = avgWaitTime / time.Duration(len(m.batchEvents))
	}

	batchRate := 0.0
	if m.totalRequests > 0 {
		batchRate = float64(m.batchRequests) / float64(m.totalRequests) * 100
	}

	return map[string]interface{}{
		"total_batched":    m.batchRequests,
		"batch_rate":       batchRate,
		"total_savings":    totalBatchSavings,
		"average_wait":     avgWaitTime,
		"events_count":     len(m.batchEvents),
	}
}

// GetSwitchStats returns model switch statistics
func (m *Metrics) GetSwitchStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	totalSwitchSavings := 0.0
	for _, event := range m.switchEvents {
		totalSwitchSavings += event.SavedCost
	}

	switchRate := 0.0
	if m.totalRequests > 0 {
		switchRate = float64(m.modelSwitches) / float64(m.totalRequests) * 100
	}

	return map[string]interface{}{
		"total_switches":   m.modelSwitches,
		"switch_rate":      switchRate,
		"total_savings":    totalSwitchSavings,
		"events_count":     len(m.switchEvents),
	}
}

// ExportRealWorldAnalysis produces detailed impact report
func (m *Metrics) ExportRealWorldAnalysis() map[string]interface{} {
	summary := m.GetSummary()

	return map[string]interface{}{
		// Volume metrics
		"total_requests": summary.TotalRequests,
		"cache_hits":     summary.CacheHits,
		"cache_hit_rate": summary.CacheHitRate,
		"batch_requests": summary.BatchRequests,
		"model_switches": summary.ModelSwitches,
		"direct_calls":   summary.DirectRequests,

		// Cost metrics
		"estimated_total_cost": summary.TotalEstimatedCost,
		"actual_total_cost":    summary.TotalActualCost,
		"total_savings":        summary.TotalSavings,
		"savings_percent":      summary.OverallSavingsPercent,
		"cost_per_request":     summary.CostPerRequest,
		"roi_score":            summary.ROIScore,

		// Strategy breakdown
		"cache_stats":  m.GetCacheStats(),
		"batch_stats":  m.GetBatchStats(),
		"switch_stats": m.GetSwitchStats(),

		// Timestamp
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
}
