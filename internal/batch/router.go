package batch

import (
	"fmt"
	"sync"
	"time"

	"github.com/szibis/claude-escalate/internal/costs"
)

// BatchStrategy defines how to route requests to batch API
type BatchStrategy string

const (
	// StrategyNever - never use batch API
	StrategyNever BatchStrategy = "never"
	// StrategyAuto - automatically batch when cost-effective
	StrategyAuto BatchStrategy = "auto"
	// StrategyAlways - always batch requests
	StrategyAlways BatchStrategy = "always"
	// StrategyUserChoice - let user decide per request
	StrategyUserChoice BatchStrategy = "user_choice"
)

// BatchRequest represents a request queued for batch processing
type BatchRequest struct {
	ID                    string
	PromptLength          int
	EstimatedOutput       int
	Model                 string
	Priority              int // 0 = low, 1 = medium, 2 = high (processed first)
	MaxWaitTime           time.Duration
	CreatedAt             time.Time
	EstimatedCost         float64
	EstimatedBatchSavings float64
	UserContext           map[string]interface{}
}

// BatchDecision represents the routing decision for a request
type BatchDecision struct {
	RequestID          string
	UsesBatchAPI       bool
	Reason             string
	EstimatedSavings   float64
	EstimatedWaitTime  time.Duration
	ROIScore           float64 // 0.0-1.0: benefit of batching vs waiting
	AlternativeModel   string  // Suggested cheaper model
	AlternativeSavings float64
	UserCanOverride    bool
}

// Router manages batch request routing and queue decisions
type Router struct {
	strategy           BatchStrategy
	queue              []*BatchRequest
	calculator         *costs.Calculator
	analyzer           *WorkloadAnalyzer // Detector for non-interactive workloads
	maxQueueSize       int
	maxBatchWaitTime   time.Duration
	minBatchSize       int
	minSavingsPercent  float64
	useDetector        bool // Enable/disable workload detection
	detectorConfidence float64 // Minimum confidence for detector-based batching
	mu                 sync.RWMutex
}

// NewRouter creates a new batch router with default settings
func NewRouter(strategy BatchStrategy) *Router {
	return &Router{
		strategy:           strategy,
		queue:              make([]*BatchRequest, 0, 100),
		calculator:         costs.NewCalculator(),
		analyzer:           NewWorkloadAnalyzer(),
		maxQueueSize:       100,
		maxBatchWaitTime:   5 * time.Minute,
		minBatchSize:       3,
		minSavingsPercent:  5.0, // Only batch if saving 5%+
		useDetector:        true,
		detectorConfidence: 0.6,
	}
}

// EnableDetector enables/disables non-interactive workload detection
func (r *Router) EnableDetector(enabled bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.useDetector = enabled
}

// SetDetectorConfidence sets minimum confidence threshold for detector-based batching
func (r *Router) SetDetectorConfidence(confidence float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if confidence < 0.0 {
		confidence = 0.0
	}
	if confidence > 1.0 {
		confidence = 1.0
	}
	r.detectorConfidence = confidence
}

// MakeRoutingDecision determines whether to batch a request
func (r *Router) MakeRoutingDecision(req BatchRequest) (BatchDecision, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	decision := BatchDecision{
		RequestID:       req.ID,
		UsesBatchAPI:    false,
		UserCanOverride: true,
	}

	// Calculate costs for direct vs batch processing
	tokens := costs.TokenCosts{
		InputTokens:  req.PromptLength,
		OutputTokens: req.EstimatedOutput,
		IsBatchAPI:   false,
		IsCached:     false,
	}

	directBreakdown, err := r.calculator.CalculateCost(req.Model, tokens)
	if err != nil {
		return decision, fmt.Errorf("failed to calculate direct cost: %w", err)
	}

	// Calculate batch cost (with 50% discount)
	batchTokens := tokens
	batchTokens.IsBatchAPI = true
	batchBreakdown, _ := r.calculator.CalculateCost(req.Model, batchTokens)

	savingsAmount := directBreakdown.TotalCost - batchBreakdown.TotalCost
	savingsPercent := (savingsAmount / directBreakdown.TotalCost) * 100

	decision.EstimatedSavings = savingsAmount
	decision.AlternativeSavings = directBreakdown.SavingsVsOpus

	// Check workload detection if enabled
	var detectionResult *WorkloadDetectionResult
	if r.useDetector {
		// Extract intent and query from user context if available
		intent := "general"
		query := ""
		if ctx, ok := req.UserContext["intent"]; ok {
			if intentStr, ok := ctx.(string); ok {
				intent = intentStr
			}
		}
		if ctx, ok := req.UserContext["query"]; ok {
			if queryStr, ok := ctx.(string); ok {
				query = queryStr
			}
		}

		// Run workload analysis
		detectionResult = &WorkloadDetectionResult{}
		*detectionResult = r.analyzer.AnalyzeRequest(
			nil, // context.Background() would be better, but we don't have it here
			query,
			intent,
			req.PromptLength+req.EstimatedOutput,
			req.MaxWaitTime,
		)
	}

	// Route decision based on strategy
	switch r.strategy {
	case StrategyNever:
		decision.UsesBatchAPI = false
		decision.Reason = "batch API disabled by strategy"

	case StrategyAlways:
		decision.UsesBatchAPI = true
		decision.Reason = fmt.Sprintf("batch API enabled (saves $%.4f, %.1f%%)", savingsAmount, savingsPercent)

	case StrategyAuto:
		// Use batch if:
		// 1. Cost savings >= minSavingsPercent AND queue fits, OR
		// 2. Detector identifies non-interactive workload with confidence >= threshold
		queueLen := len(r.queue)

		// Cost-based batching decision
		costBatchingWorth := savingsPercent >= r.minSavingsPercent &&
			queueLen < r.maxQueueSize

		// Detection-based batching decision
		detectionBasedBatch := false
		detectionReason := ""
		if detectionResult != nil && detectionResult.ShouldBatch && detectionResult.Confidence >= r.detectorConfidence {
			detectionBasedBatch = true
			detectionReason = fmt.Sprintf(" (detected: %s, confidence=%.1f%%)", detectionResult.Reason, detectionResult.Confidence*100)
		}

		shouldBatch := (costBatchingWorth || detectionBasedBatch) && queueLen < r.maxQueueSize
		decision.UsesBatchAPI = shouldBatch

		if shouldBatch {
			// Estimate wait time based on queue
			avgProcessTime := 10 * time.Second
			waitTime := time.Duration(queueLen) * avgProcessTime
			if waitTime > r.maxBatchWaitTime {
				waitTime = r.maxBatchWaitTime
			}
			decision.EstimatedWaitTime = waitTime

			// Reason includes both cost and detection
			baseReason := fmt.Sprintf("auto-batch queued (saves $%.4f, queue=%d/%d)", savingsAmount, queueLen, r.minBatchSize)
			decision.Reason = baseReason + detectionReason

			// Calculate ROI score: benefit vs cost of waiting
			// ROI = (savings / wait_time_seconds) normalized to 0-1
			waitSeconds := float64(waitTime.Seconds())
			if waitSeconds > 0 {
				decision.ROIScore = (savingsAmount * 100) / waitSeconds // Normalized by wait time
				if decision.ROIScore > 1.0 {
					decision.ROIScore = 1.0
				}
			} else {
				// No queue: immediate processing with full savings benefit
				decision.ROIScore = savingsAmount * 100 / 1.0 // Treat as 1-second wait
				if decision.ROIScore > 1.0 {
					decision.ROIScore = 1.0
				}
			}
		} else {
			// Suggest alternative model if not batching
			comparison, _ := r.calculator.CompareModels(tokens)
			for _, model := range []string{"haiku", "sonnet"} {
				if model != req.Model {
					if modelBreakdown, ok := comparison[model]; ok {
						if modelBreakdown.TotalCost < directBreakdown.TotalCost {
							decision.AlternativeModel = model
							decision.AlternativeSavings = directBreakdown.TotalCost - modelBreakdown.TotalCost
							decision.Reason = fmt.Sprintf("use %s instead (saves $%.4f)", model, decision.AlternativeSavings)
							break
						}
					}
				}
			}
			if decision.Reason == "" {
				reason := fmt.Sprintf("direct API (savings < %.1f%% or queue too small)", r.minSavingsPercent)
				if detectionResult != nil && !detectionResult.ShouldBatch {
					reason += fmt.Sprintf(" (not detected as batch-worthy: %s)", detectionResult.Reason)
				}
				decision.Reason = reason
			}
		}

	case StrategyUserChoice:
		decision.UsesBatchAPI = false
		decision.UserCanOverride = true
		decision.Reason = fmt.Sprintf("user choice: batch saves $%.4f (%.1f%%)", savingsAmount, savingsPercent)
	}

	// Add to queue if batching
	if decision.UsesBatchAPI {
		r.queue = append(r.queue, &req)
	}

	return decision, nil
}

// QueueSize returns current queue size
func (r *Router) QueueSize() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.queue)
}

// QueueStats returns statistics about the current queue
func (r *Router) QueueStats() QueueStatistics {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := QueueStatistics{
		Size:             len(r.queue),
		MaxSize:          r.maxQueueSize,
		OldestRequestAge: 0,
		AverageWaitTime:  0,
		TotalPendingCost: 0,
		EstimatedSavings: 0,
	}

	if len(r.queue) == 0 {
		return stats
	}

	now := time.Now()
	totalWait := time.Duration(0)

	for _, req := range r.queue {
		age := now.Sub(req.CreatedAt)
		if stats.OldestRequestAge == 0 || age > stats.OldestRequestAge {
			stats.OldestRequestAge = age
		}
		totalWait += age
		stats.TotalPendingCost += req.EstimatedCost
		stats.EstimatedSavings += req.EstimatedBatchSavings
	}

	stats.AverageWaitTime = totalWait / time.Duration(len(r.queue))

	return stats
}

// QueueStatistics contains queue metrics
type QueueStatistics struct {
	Size             int
	MaxSize          int
	OldestRequestAge time.Duration
	AverageWaitTime  time.Duration
	TotalPendingCost float64
	EstimatedSavings float64
}

// FlushQueue processes all queued requests
func (r *Router) FlushQueue() ([]*BatchRequest, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.queue) == 0 {
		return []*BatchRequest{}, nil
	}

	// Sort by priority (high first)
	// Simple bubble sort for small queues
	for i := 0; i < len(r.queue); i++ {
		for j := i + 1; j < len(r.queue); j++ {
			if r.queue[j].Priority > r.queue[i].Priority {
				r.queue[i], r.queue[j] = r.queue[j], r.queue[i]
			}
		}
	}

	processed := make([]*BatchRequest, len(r.queue))
	copy(processed, r.queue)
	r.queue = r.queue[:0] // Clear queue

	return processed, nil
}

// CanAddToQueue checks if a request can be added to the queue
func (r *Router) CanAddToQueue() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.queue) < r.maxQueueSize
}

// SetStrategy changes the batch routing strategy
func (r *Router) SetStrategy(strategy BatchStrategy) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.strategy = strategy
}

// SetMinBatchSize sets the minimum queue size before processing
func (r *Router) SetMinBatchSize(size int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.minBatchSize = size
}

// SetMinSavingsPercent sets minimum savings threshold for auto-batching
func (r *Router) SetMinSavingsPercent(percent float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.minSavingsPercent = percent
}

// SetMaxBatchWaitTime sets maximum acceptable wait time for batching
func (r *Router) SetMaxBatchWaitTime(duration time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.maxBatchWaitTime = duration
}

// RecommendBatching analyzes if batching is worthwhile
func (r *Router) RecommendBatching(model string, inputTokens, outputTokens int, waitTimeAcceptable time.Duration) BatchRecommendation {
	r.mu.RLock()
	queueLen := len(r.queue)
	r.mu.RUnlock()

	tokens := costs.TokenCosts{
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		IsBatchAPI:   false,
	}

	directBreakdown, _ := r.calculator.CalculateCost(model, tokens)

	batchTokens := tokens
	batchTokens.IsBatchAPI = true
	batchBreakdown, _ := r.calculator.CalculateCost(model, batchTokens)

	savings := directBreakdown.TotalCost - batchBreakdown.TotalCost
	savingsPercent := (savings / directBreakdown.TotalCost) * 100

	recommendation := BatchRecommendation{
		RecommendBatch:    false,
		EstimatedSavings:  savings,
		SavingsPercent:    savingsPercent,
		QueueSize:         queueLen,
		EstimatedWaitTime: time.Duration(queueLen) * 10 * time.Second,
		Rationale:         "",
	}

	// Recommend if savings are significant and wait is acceptable
	if savingsPercent >= r.minSavingsPercent && recommendation.EstimatedWaitTime <= waitTimeAcceptable {
		recommendation.RecommendBatch = true
		recommendation.Rationale = fmt.Sprintf("batch saves %.1f%%, wait ~%v", savingsPercent, recommendation.EstimatedWaitTime)
	} else if savingsPercent < r.minSavingsPercent {
		recommendation.Rationale = fmt.Sprintf("savings (%.1f%%) below threshold (%.1f%%)", savingsPercent, r.minSavingsPercent)
	} else {
		recommendation.Rationale = fmt.Sprintf("wait time (%v) exceeds acceptable (%v)", recommendation.EstimatedWaitTime, waitTimeAcceptable)
	}

	return recommendation
}

// BatchRecommendation provides batching advice
type BatchRecommendation struct {
	RecommendBatch    bool
	EstimatedSavings  float64
	SavingsPercent    float64
	QueueSize         int
	EstimatedWaitTime time.Duration
	Rationale         string
}
