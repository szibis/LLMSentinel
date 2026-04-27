// Package metrics provides comprehensive token usage and cost tracking
package metrics

import (
	"sync"
	"time"
)

// TokenBurned tracks tokens sent to Claude API
type TokenBurned struct {
	InputTokens       int64
	OutputTokens      int64
	CacheReadTokens   int64
	CacheWriteTokens  int64
	TotalTokens       int64
	EstimatedCostUSD  float64
}

// TokenSaved tracks tokens prevented from being sent (optimization savings)
type TokenSaved struct {
	ExactDedupTokens      int64  // 100% savings when cache hit
	SemanticCacheTokens   int64  // 98% savings (embedding cost deducted)
	InputOptimizationTokens int64 // 30-40% savings from input compression
	OutputOptimizationTokens int64 // 30-50% savings from output compression
	RTKSavingsTokens      int64  // 99.4% savings from RTK proxy
	BatchAPISavingsTokens int64  // 50% savings from batch discount
	KnowledgeGraphTokens  int64  // 99% savings from graph queries
	TotalTokensSaved      int64
	SavingsPercent        float64
	EstimatedCostSavedUSD float64
}

// OptimizationBreakdown shows contribution of each layer to total savings
type OptimizationBreakdown struct {
	ExactDedup        TokenOptimizationStat
	SemanticCache     TokenOptimizationStat
	InputOptimization TokenOptimizationStat
	OutputOptimization TokenOptimizationStat
	RTKProxy          TokenOptimizationStat
	BatchAPI          TokenOptimizationStat
	KnowledgeGraph    TokenOptimizationStat
}

// TokenOptimizationStat tracks per-optimization metrics
type TokenOptimizationStat struct {
	HitCount         int64
	TokensSaved      int64
	CostSavedUSD     float64
	SavingsPercent   float64
	AverageSavingsPer int64
	EnabledInConfig  bool
}

// DailyMetrics tracks metrics for a single day
type DailyMetrics struct {
	Date             time.Time
	Burned           TokenBurned
	Saved            TokenSaved
	Breakdown        OptimizationBreakdown
	RequestCount     int64
	CacheHitRate     float64
	FalsePositiveRate float64
}

// MonthlyProjection extrapolates current trends
type MonthlyProjection struct {
	ProjectedTokensBurned   int64
	ProjectedTokensSaved    int64
	ProjectedCostUSD        float64
	ProjectedSavingsUSD     float64
	ProjectedSavingsPercent float64
	BasedOnDays            int
	ProjectionConfidence   float64
}

// SessionMetrics tracks metrics across multiple days
type SessionMetrics struct {
	mu sync.RWMutex

	StartDate    time.Time
	CurrentDate  time.Time
	DailyMetrics map[time.Time]*DailyMetrics

	// Aggregated totals
	TotalBurned           TokenBurned
	TotalSaved            TokenSaved
	TotalBreakdown        OptimizationBreakdown
	TotalRequests         int64
	AverageCacheHitRate   float64
	AvgFalsePositiveRate  float64

	// Projections
	Monthly7Day    MonthlyProjection
	Monthly30Day   MonthlyProjection
	MonthlyFullMonth MonthlyProjection
}

// NewSessionMetrics creates a new metrics tracker
func NewSessionMetrics() *SessionMetrics {
	return &SessionMetrics{
		StartDate:    time.Now(),
		CurrentDate:  time.Now(),
		DailyMetrics: make(map[time.Time]*DailyMetrics),
	}
}

// RecordBurnedTokens records tokens sent to Claude
func (sm *SessionMetrics) RecordBurnedTokens(input, output, cacheRead, cacheWrite int64, costUSD float64) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	today := time.Now().Truncate(24 * time.Hour)
	if _, exists := sm.DailyMetrics[today]; !exists {
		sm.DailyMetrics[today] = &DailyMetrics{Date: today}
	}

	daily := sm.DailyMetrics[today]
	daily.Burned.InputTokens += input
	daily.Burned.OutputTokens += output
	daily.Burned.CacheReadTokens += cacheRead
	daily.Burned.CacheWriteTokens += cacheWrite
	daily.Burned.TotalTokens = daily.Burned.InputTokens + daily.Burned.OutputTokens
	daily.Burned.EstimatedCostUSD += costUSD

	// Update aggregated totals
	sm.TotalBurned.InputTokens += input
	sm.TotalBurned.OutputTokens += output
	sm.TotalBurned.CacheReadTokens += cacheRead
	sm.TotalBurned.CacheWriteTokens += cacheWrite
	sm.TotalBurned.TotalTokens = sm.TotalBurned.InputTokens + sm.TotalBurned.OutputTokens
	sm.TotalBurned.EstimatedCostUSD += costUSD

	sm.TotalRequests++
	daily.RequestCount++
}

// RecordOptimizationSaving records tokens saved by optimization layer
func (sm *SessionMetrics) RecordOptimizationSaving(
	optimizationType string,
	tokensSaved int64,
	costSavedUSD float64,
	hitCount int64,
) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	today := time.Now().Truncate(24 * time.Hour)
	if _, exists := sm.DailyMetrics[today]; !exists {
		sm.DailyMetrics[today] = &DailyMetrics{Date: today}
	}

	daily := sm.DailyMetrics[today]
	daily.Saved.TotalTokensSaved += tokensSaved
	daily.Saved.EstimatedCostSavedUSD += costSavedUSD

	sm.TotalSaved.TotalTokensSaved += tokensSaved
	sm.TotalSaved.EstimatedCostSavedUSD += costSavedUSD

	// Update breakdown by type
	updateBreakdown(&daily.Breakdown, optimizationType, tokensSaved, costSavedUSD, hitCount)
	updateBreakdown(&sm.TotalBreakdown, optimizationType, tokensSaved, costSavedUSD, hitCount)
}

func updateBreakdown(bd *OptimizationBreakdown, optType string, tokens int64, cost float64, hits int64) {
	switch optType {
	case "exact_dedup":
		bd.ExactDedup.TokensSaved += tokens
		bd.ExactDedup.CostSavedUSD += cost
		bd.ExactDedup.HitCount += hits
	case "semantic_cache":
		bd.SemanticCache.TokensSaved += tokens
		bd.SemanticCache.CostSavedUSD += cost
		bd.SemanticCache.HitCount += hits
	case "input_optimization":
		bd.InputOptimization.TokensSaved += tokens
		bd.InputOptimization.CostSavedUSD += cost
		bd.InputOptimization.HitCount += hits
	case "output_optimization":
		bd.OutputOptimization.TokensSaved += tokens
		bd.OutputOptimization.CostSavedUSD += cost
		bd.OutputOptimization.HitCount += hits
	case "rtk_proxy":
		bd.RTKProxy.TokensSaved += tokens
		bd.RTKProxy.CostSavedUSD += cost
		bd.RTKProxy.HitCount += hits
	case "batch_api":
		bd.BatchAPI.TokensSaved += tokens
		bd.BatchAPI.CostSavedUSD += cost
		bd.BatchAPI.HitCount += hits
	case "knowledge_graph":
		bd.KnowledgeGraph.TokensSaved += tokens
		bd.KnowledgeGraph.CostSavedUSD += cost
		bd.KnowledgeGraph.HitCount += hits
	}
}

// GetDailySummary returns metrics for a specific day
func (sm *SessionMetrics) GetDailySummary(date time.Time) *DailyMetrics {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	day := date.Truncate(24 * time.Hour)
	if daily, exists := sm.DailyMetrics[day]; exists {
		return daily
	}
	return nil
}

// CalculateSavingsPercent computes overall savings percentage
func (sm *SessionMetrics) CalculateSavingsPercent() float64 {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if sm.TotalBurned.TotalTokens == 0 {
		return 0
	}

	totalWithoutOptimization := sm.TotalBurned.TotalTokens + sm.TotalSaved.TotalTokensSaved
	if totalWithoutOptimization == 0 {
		return 0
	}

	return float64(sm.TotalSaved.TotalTokensSaved) / float64(totalWithoutOptimization) * 100
}

// ProjectMonthly calculates monthly projections based on current usage
func (sm *SessionMetrics) ProjectMonthly(periodDays int) MonthlyProjection {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if periodDays == 0 {
		return MonthlyProjection{}
	}

	daysPerMonth := 30.0
	multiplier := daysPerMonth / float64(periodDays)
	confidence := 1.0 - (float64(periodDays) / daysPerMonth)

	projection := MonthlyProjection{
		ProjectedTokensBurned:   int64(float64(sm.TotalBurned.TotalTokens) * multiplier),
		ProjectedTokensSaved:    int64(float64(sm.TotalSaved.TotalTokensSaved) * multiplier),
		ProjectedCostUSD:        sm.TotalBurned.EstimatedCostUSD * multiplier,
		ProjectedSavingsUSD:     sm.TotalSaved.EstimatedCostSavedUSD * multiplier,
		BasedOnDays:            periodDays,
		ProjectionConfidence:   confidence,
	}

	totalTokens := projection.ProjectedTokensBurned + projection.ProjectedTokensSaved
	if totalTokens > 0 {
		projection.ProjectedSavingsPercent = float64(projection.ProjectedTokensSaved) / float64(totalTokens) * 100
	}

	return projection
}

// GetJSON returns metrics in JSON-compatible format for export/API
func (sm *SessionMetrics) GetJSON() map[string]interface{} {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	savingsPercent := sm.CalculateSavingsPercent()

	return map[string]interface{}{
		"period": map[string]interface{}{
			"start_date": sm.StartDate,
			"end_date":   sm.CurrentDate,
			"days":       int(sm.CurrentDate.Sub(sm.StartDate).Hours() / 24),
		},
		"burned": map[string]interface{}{
			"input_tokens":        sm.TotalBurned.InputTokens,
			"output_tokens":       sm.TotalBurned.OutputTokens,
			"cache_read_tokens":   sm.TotalBurned.CacheReadTokens,
			"cache_write_tokens":  sm.TotalBurned.CacheWriteTokens,
			"total_tokens":        sm.TotalBurned.TotalTokens,
			"estimated_cost_usd":  sm.TotalBurned.EstimatedCostUSD,
		},
		"saved": map[string]interface{}{
			"exact_dedup_tokens":        sm.TotalSaved.ExactDedupTokens,
			"semantic_cache_tokens":     sm.TotalSaved.SemanticCacheTokens,
			"input_optimization_tokens": sm.TotalSaved.InputOptimizationTokens,
			"output_optimization_tokens": sm.TotalSaved.OutputOptimizationTokens,
			"rtk_savings_tokens":        sm.TotalSaved.RTKSavingsTokens,
			"batch_api_savings_tokens":  sm.TotalSaved.BatchAPISavingsTokens,
			"knowledge_graph_tokens":    sm.TotalSaved.KnowledgeGraphTokens,
			"total_tokens_saved":        sm.TotalSaved.TotalTokensSaved,
			"savings_percent":           savingsPercent,
			"estimated_cost_saved_usd":  sm.TotalSaved.EstimatedCostSavedUSD,
		},
		"breakdown": map[string]interface{}{
			"exact_dedup": map[string]interface{}{
				"tokens_saved":     sm.TotalBreakdown.ExactDedup.TokensSaved,
				"cost_saved_usd":   sm.TotalBreakdown.ExactDedup.CostSavedUSD,
				"hit_count":        sm.TotalBreakdown.ExactDedup.HitCount,
				"savings_percent":  sm.TotalBreakdown.ExactDedup.SavingsPercent,
			},
			"semantic_cache": map[string]interface{}{
				"tokens_saved":     sm.TotalBreakdown.SemanticCache.TokensSaved,
				"cost_saved_usd":   sm.TotalBreakdown.SemanticCache.CostSavedUSD,
				"hit_count":        sm.TotalBreakdown.SemanticCache.HitCount,
				"savings_percent":  sm.TotalBreakdown.SemanticCache.SavingsPercent,
			},
			"input_optimization": map[string]interface{}{
				"tokens_saved":     sm.TotalBreakdown.InputOptimization.TokensSaved,
				"cost_saved_usd":   sm.TotalBreakdown.InputOptimization.CostSavedUSD,
				"hit_count":        sm.TotalBreakdown.InputOptimization.HitCount,
				"savings_percent":  sm.TotalBreakdown.InputOptimization.SavingsPercent,
			},
			"output_optimization": map[string]interface{}{
				"tokens_saved":     sm.TotalBreakdown.OutputOptimization.TokensSaved,
				"cost_saved_usd":   sm.TotalBreakdown.OutputOptimization.CostSavedUSD,
				"hit_count":        sm.TotalBreakdown.OutputOptimization.HitCount,
				"savings_percent":  sm.TotalBreakdown.OutputOptimization.SavingsPercent,
			},
			"rtk_proxy": map[string]interface{}{
				"tokens_saved":     sm.TotalBreakdown.RTKProxy.TokensSaved,
				"cost_saved_usd":   sm.TotalBreakdown.RTKProxy.CostSavedUSD,
				"hit_count":        sm.TotalBreakdown.RTKProxy.HitCount,
				"savings_percent":  sm.TotalBreakdown.RTKProxy.SavingsPercent,
			},
			"batch_api": map[string]interface{}{
				"tokens_saved":     sm.TotalBreakdown.BatchAPI.TokensSaved,
				"cost_saved_usd":   sm.TotalBreakdown.BatchAPI.CostSavedUSD,
				"hit_count":        sm.TotalBreakdown.BatchAPI.HitCount,
				"savings_percent":  sm.TotalBreakdown.BatchAPI.SavingsPercent,
			},
			"knowledge_graph": map[string]interface{}{
				"tokens_saved":     sm.TotalBreakdown.KnowledgeGraph.TokensSaved,
				"cost_saved_usd":   sm.TotalBreakdown.KnowledgeGraph.CostSavedUSD,
				"hit_count":        sm.TotalBreakdown.KnowledgeGraph.HitCount,
				"savings_percent":  sm.TotalBreakdown.KnowledgeGraph.SavingsPercent,
			},
		},
		"requests": sm.TotalRequests,
		"cache_hit_rate": sm.AverageCacheHitRate,
		"false_positive_rate": sm.AvgFalsePositiveRate,
		"projections": map[string]interface{}{
			"7day_monthly": sm.Monthly7Day,
			"30day_monthly": sm.Monthly30Day,
			"full_month": sm.MonthlyFullMonth,
		},
	}
}
