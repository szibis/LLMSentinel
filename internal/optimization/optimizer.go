package optimization

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/szibis/claude-escalate/internal/batch"
	"github.com/szibis/claude-escalate/internal/costs"
)

// OptimizationDecision represents the full optimization analysis
type OptimizationDecision struct {
	// Layer results
	CacheHit               bool
	CachedResponseHash     string
	UseBatch               bool
	SwitchModel            string
	Direction              string // "cache_hit" | "batch" | "model_switch" | "direct"

	// Cost breakdown
	DirectCost             float64
	OptimizedCost          float64
	TotalSavings           float64
	SavingsPercent         float64

	// Recommendations
	Rationale              string
	EstimatedWaitTime      time.Duration

	// Metadata for logging
	CacheAge               time.Duration
	BatchQueueSize         int
}

// Optimizer coordinates cache, batch, and model optimization decisions
type Optimizer struct {
	cache                *batch.CacheManager
	router               *batch.Router
	calculator           *costs.Calculator
	metrics              *Metrics
	mu                   sync.RWMutex
}

// NewOptimizer creates a new optimization engine
func NewOptimizer() *Optimizer {
	return &Optimizer{
		cache:      batch.NewCacheManager(),
		router:     batch.NewRouter(batch.StrategyAuto),
		calculator: costs.NewCalculator(),
		metrics:    NewMetrics(),
	}
}

// Optimize analyzes a request and recommends the best optimization strategy
func (o *Optimizer) Optimize(prompt, model string, estimatedOutput int) OptimizationDecision {
	// Input validation
	if err := o.validateInput(prompt, model, estimatedOutput); err != nil {
		return OptimizationDecision{
			Direction: "direct",
			Rationale: fmt.Sprintf("validation error: %v", err),
		}
	}

	o.mu.Lock()
	defer o.mu.Unlock()

	decision := OptimizationDecision{
		Direction: "direct",
		Rationale: "no optimization",
	}

	// Layer 1: Cache check (highest savings potential: 99.8%)
	if cachedHash := o.cache.FindSimilarPrompt(prompt, model, 0.85); cachedHash != "" {
		decision.CacheHit = true
		decision.CachedResponseHash = cachedHash
		decision.Direction = "cache_hit"
		decision.SavingsPercent = 99.8
		decision.Rationale = "cache hit: return stored response (99.8% savings)"
		decision.TotalSavings = o.estimateCost(prompt, model, estimatedOutput) * 0.998
		o.metrics.RecordCacheHit(prompt, model)
		return decision
	}

	// Layer 2: Batch check (moderate savings: 50%)
	batchReq := batch.BatchRequest{
		ID:              fmt.Sprintf("opt-%d", time.Now().UnixNano()),
		PromptLength:    len(prompt),
		EstimatedOutput: estimatedOutput,
		Model:           model,
		MaxWaitTime:     5 * time.Minute,
		CreatedAt:       time.Now(),
	}

	batchDecision, err := o.router.MakeRoutingDecision(batchReq)
	if err != nil {
		// Log error but continue to fallback strategies
		decision.Rationale = fmt.Sprintf("batch decision error (using fallback): %v", err)
	} else if batchDecision.UsesBatchAPI {
		decision.UseBatch = true
		decision.Direction = "batch"
		decision.TotalSavings = batchDecision.EstimatedSavings
		decision.EstimatedWaitTime = batchDecision.EstimatedWaitTime
		directCost := o.estimateCost(prompt, model, estimatedOutput)
		if directCost > 0 {
			decision.SavingsPercent = (batchDecision.EstimatedSavings / directCost) * 100
			// Cap savings at 100% (logical maximum)
			if decision.SavingsPercent > 100.0 {
				decision.SavingsPercent = 100.0
			}
		}
		decision.Rationale = batchDecision.Reason
		decision.BatchQueueSize = o.router.QueueSize()
		o.metrics.RecordBatchDecision(prompt, model, true)
		return decision
	}

	// Layer 3: Model switch check (mild savings: 10-50%)
	if batchDecision.AlternativeModel != "" {
		decision.SwitchModel = batchDecision.AlternativeModel
		decision.Direction = "model_switch"
		decision.TotalSavings = batchDecision.AlternativeSavings
		directCost := o.estimateCost(prompt, model, estimatedOutput)
		if directCost > 0 {
			decision.SavingsPercent = (batchDecision.AlternativeSavings / directCost) * 100
			// Cap savings at 100%
			if decision.SavingsPercent > 100.0 {
				decision.SavingsPercent = 100.0
			}
		}
		decision.Rationale = fmt.Sprintf("switch to %s: save %.1f%% ($%.4f)",
			batchDecision.AlternativeModel, decision.SavingsPercent, decision.TotalSavings)
		o.metrics.RecordModelSwitch(prompt, model, batchDecision.AlternativeModel)
		return decision
	}

	// Layer 4: Direct call (no optimization)
	decision.Direction = "direct"
	decision.TotalSavings = 0
	decision.SavingsPercent = 0
	decision.Rationale = "direct API call (no optimization available)"
	o.metrics.RecordDirect(prompt, model)

	return decision
}

// CacheResponse stores a successful response for future reuse
func (o *Optimizer) CacheResponse(prompt, model string, response string, estimatedTokens int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.cache.CachePrompt(prompt, model, estimatedTokens)
}

// GetMetrics returns current optimization metrics
func (o *Optimizer) GetMetrics() MetricsSummary {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.metrics.GetSummary()
}

// GetQueueStats returns current batch queue statistics
func (o *Optimizer) GetQueueStats() batch.QueueStatistics {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.router.QueueStats()
}

// FlushBatch processes queued requests
func (o *Optimizer) FlushBatch() ([]*batch.BatchRequest, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.router.FlushQueue()
}

// ClearExpiredCache removes old entries
func (o *Optimizer) ClearExpiredCache() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.cache.ClearExpiredEntries()
}

// estimateCost calculates direct API cost, returns 0 on error
func (o *Optimizer) estimateCost(prompt, model string, estimatedOutput int) float64 {
	breakdown, err := o.calculator.EstimateCost(model, len(prompt), estimatedOutput)
	if err != nil {
		return 0
	}
	return breakdown.TotalCost
}

// validateInput checks prompt, model, and output for invalid/dangerous values
func (o *Optimizer) validateInput(prompt, model string, estimatedOutput int) error {
	// Validate prompt
	if len(strings.TrimSpace(prompt)) == 0 {
		return fmt.Errorf("prompt cannot be empty")
	}

	const maxPromptLength = 1_000_000 // 1M characters max
	if len(prompt) > maxPromptLength {
		return fmt.Errorf("prompt too large: %d > %d characters", len(prompt), maxPromptLength)
	}

	// Validate model
	model = strings.ToLower(strings.TrimSpace(model))
	validModels := map[string]bool{"haiku": true, "sonnet": true, "opus": true}
	if !validModels[model] {
		return fmt.Errorf("invalid model: %q (must be haiku, sonnet, or opus)", model)
	}

	// Validate estimatedOutput
	if estimatedOutput < 0 {
		return fmt.Errorf("estimatedOutput cannot be negative: %d", estimatedOutput)
	}

	const maxOutputLength = 100_000 // 100k tokens max
	if estimatedOutput > maxOutputLength {
		return fmt.Errorf("estimatedOutput too large: %d > %d tokens", estimatedOutput, maxOutputLength)
	}

	// Check for integer overflow in token calculations
	if int64(len(prompt))+int64(estimatedOutput) > math.MaxInt64>>16 {
		return fmt.Errorf("prompt + output would cause integer overflow in token calculation")
	}

	return nil
}

// SetBatchStrategy changes the batching strategy
func (o *Optimizer) SetBatchStrategy(strategy batch.BatchStrategy) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.router.SetStrategy(strategy)
}

// SetMinBatchSize configures minimum queue size before processing
func (o *Optimizer) SetMinBatchSize(size int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.router.SetMinBatchSize(size)
}

// SetMaxBatchWaitTime configures maximum acceptable wait time
func (o *Optimizer) SetMaxBatchWaitTime(duration time.Duration) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.router.SetMaxBatchWaitTime(duration)
}

// SetMinSavingsPercent configures minimum savings threshold
func (o *Optimizer) SetMinSavingsPercent(percent float64) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.router.SetMinSavingsPercent(percent)
}
