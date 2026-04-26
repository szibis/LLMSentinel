package costs

import "fmt"

// ModelCosts defines token costs for each Claude model (USD per 1M tokens).
// Based on current Claude pricing (as of early 2026).
type ModelCosts struct {
	HaikuInput      float64
	HaikuOutput     float64
	SonnetInput     float64
	SonnetOutput    float64
	OpusInput       float64
	OpusOutput      float64
	BatchDiscount   float64 // 0.5 = 50% discount for batch API
	CacheReadCost   float64 // Cost per 1M cache read tokens (typically 10% of input)
	CacheWriteCost  float64 // Cost per 1M cache write tokens (typically 25% of input)
}

// DefaultModelCosts returns the default pricing as of early 2026.
func DefaultModelCosts() ModelCosts {
	return ModelCosts{
		HaikuInput:    0.80,   // $0.80 per 1M input tokens
		HaikuOutput:   4.00,   // $4.00 per 1M output tokens
		SonnetInput:   3.00,   // $3.00 per 1M input tokens
		SonnetOutput:  15.00,  // $15.00 per 1M output tokens
		OpusInput:     15.00,  // $15.00 per 1M input tokens
		OpusOutput:    75.00,  // $75.00 per 1M output tokens
		BatchDiscount: 0.5,    // 50% reduction for batch API
		CacheReadCost: 0.30,   // 10% of input cost for cached reads
		CacheWriteCost: 1.20,  // 25% of input cost for cache writes
	}
}

// TokenCosts stores actual token usage
type TokenCosts struct {
	InputTokens        int
	OutputTokens       int
	CacheReadTokens    int  // Tokens read from prompt cache
	CacheWriteTokens   int  // Tokens written to prompt cache
	CacheHitPercent    float64 // 0.0-1.0, percentage of reads that were cache hits
	IsBatchAPI         bool
	IsCached           bool
}

// CostBreakdown provides detailed cost analysis
type CostBreakdown struct {
	Model               string
	InputCost           float64
	OutputCost          float64
	CacheReadCost       float64
	CacheWriteCost      float64
	TotalCost           float64
	EffectiveCostPerToken float64
	SavingsVsOpus       float64
	CacheSavings        float64
	BatchSavings        float64
	EstimatedAccuracy   float64 // How close estimate was to actual
}

// Calculator performs detailed cost calculations
type Calculator struct {
	costs ModelCosts
}

// NewCalculator creates a new cost calculator with default pricing
func NewCalculator() *Calculator {
	return &Calculator{
		costs: DefaultModelCosts(),
	}
}

// NewCalculatorWithCosts creates a calculator with custom pricing
func NewCalculatorWithCosts(costs ModelCosts) *Calculator {
	return &Calculator{costs: costs}
}

// CalculateCost computes detailed cost breakdown for a transaction
func (c *Calculator) CalculateCost(model string, tokens TokenCosts) (CostBreakdown, error) {
	breakdown := CostBreakdown{Model: model}

	// Get per-token costs for the model
	inputRate, outputRate, err := c.getModelRates(model)
	if err != nil {
		return breakdown, err
	}

	// Calculate input tokens cost
	breakdown.InputCost = float64(tokens.InputTokens) * inputRate / 1_000_000

	// Calculate output tokens cost
	breakdown.OutputCost = float64(tokens.OutputTokens) * outputRate / 1_000_000

	// Calculate cache-related costs
	if tokens.IsCached {
		// Cache reads cost 10% of input
		cacheReadInputRate := inputRate * c.costs.CacheReadCost
		breakdown.CacheReadCost = float64(tokens.CacheReadTokens) * cacheReadInputRate / 1_000_000

		// Cache writes cost 25% of input
		cacheWriteInputRate := inputRate * c.costs.CacheWriteCost
		breakdown.CacheWriteCost = float64(tokens.CacheWriteTokens) * cacheWriteInputRate / 1_000_000

		// Calculate cache savings (avoided read costs)
		baseCacheReadCost := float64(tokens.CacheReadTokens) * inputRate / 1_000_000
		breakdown.CacheSavings = baseCacheReadCost - breakdown.CacheReadCost
	}

	// Calculate base cost before batch discount
	baseCost := breakdown.InputCost + breakdown.OutputCost + breakdown.CacheReadCost + breakdown.CacheWriteCost

	// Apply batch discount if applicable
	if tokens.IsBatchAPI {
		breakdown.BatchSavings = baseCost * (1 - c.costs.BatchDiscount)
		baseCost *= c.costs.BatchDiscount
	}

	breakdown.TotalCost = baseCost

	// Calculate effective cost per token (for comparison)
	totalTokens := tokens.InputTokens + tokens.OutputTokens + tokens.CacheReadTokens + tokens.CacheWriteTokens
	if totalTokens > 0 {
		breakdown.EffectiveCostPerToken = breakdown.TotalCost / float64(totalTokens) * 1_000_000
	}

	// Calculate savings vs Opus baseline (if not already opus)
	if model != "opus" {
		opusInput, opusOutput, _ := c.getModelRates("opus")
		opusCost := float64(tokens.InputTokens)*opusInput/1_000_000 + float64(tokens.OutputTokens)*opusOutput/1_000_000
		if tokens.IsCached {
			opusCacheRead := float64(tokens.CacheReadTokens) * opusInput * c.costs.CacheReadCost / 1_000_000
			opusCacheWrite := float64(tokens.CacheWriteTokens) * opusInput * c.costs.CacheWriteCost / 1_000_000
			opusCost += opusCacheRead + opusCacheWrite
		}
		if tokens.IsBatchAPI {
			opusCost *= c.costs.BatchDiscount
		}
		breakdown.SavingsVsOpus = opusCost - breakdown.TotalCost
	}

	return breakdown, nil
}

// CompareModels returns cost comparison across all three models
func (c *Calculator) CompareModels(tokens TokenCosts) (map[string]CostBreakdown, error) {
	models := []string{"haiku", "sonnet", "opus"}
	results := make(map[string]CostBreakdown)

	for _, model := range models {
		breakdown, err := c.CalculateCost(model, tokens)
		if err != nil {
			return nil, err
		}
		results[model] = breakdown
	}

	return results, nil
}

// EstimateCost estimates cost without actual token data (for pre-request estimation)
func (c *Calculator) EstimateCost(model string, promptLength int, estimatedOutputLength int) (CostBreakdown, error) {
	// Rough heuristics for estimation:
	// - 1 character ≈ 0.25 tokens
	// - Prompt tokens = promptLength * 0.25
	// - Output tokens = estimatedOutputLength * 0.25
	inputTokens := int(float64(promptLength) * 0.25)
	outputTokens := int(float64(estimatedOutputLength) * 0.25)

	tokens := TokenCosts{
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		IsBatchAPI:   false,
		IsCached:     false,
	}

	return c.CalculateCost(model, tokens)
}

// CalculateCostError compares estimated vs actual and returns error percentage
func (c *Calculator) CalculateCostError(estimated, actual float64) float64 {
	if estimated == 0 {
		return 0
	}
	return ((actual - estimated) / estimated) * 100
}

// getModelRates returns input/output rates for a model (USD per 1M tokens)
func (c *Calculator) getModelRates(model string) (inputRate, outputRate float64, err error) {
	switch model {
	case "haiku":
		return c.costs.HaikuInput, c.costs.HaikuOutput, nil
	case "sonnet":
		return c.costs.SonnetInput, c.costs.SonnetOutput, nil
	case "opus":
		return c.costs.OpusInput, c.costs.OpusOutput, nil
	default:
		return 0, 0, fmt.Errorf("unknown model: %s", model)
	}
}

// ROICalculator computes return on investment for cost optimizations
type ROICalculator struct {
	calculator *Calculator
}

// NewROICalculator creates a new ROI calculator
func NewROICalculator() *ROICalculator {
	return &ROICalculator{
		calculator: NewCalculator(),
	}
}

// ROI represents return on investment metrics
type ROI struct {
	TotalUsageCost       float64 // Total cost across all requests
	TotalSavings         float64 // Total saved vs all-Opus baseline
	OptimizationRate     float64 // Percentage saved
	AverageModel         string  // Most frequently used model
	CascadeROI           float64 // ROI from cascade optimizations
	ModelDistribution    map[string]int // Number of requests per model
}

// CalculateROI computes ROI metrics for a series of transactions
func (r *ROICalculator) CalculateROI(breakdowns map[string]CostBreakdown, modelDistribution map[string]int) ROI {
	roi := ROI{
		ModelDistribution: modelDistribution,
	}

	for _, breakdown := range breakdowns {
		roi.TotalUsageCost += breakdown.TotalCost
		roi.TotalSavings += breakdown.SavingsVsOpus
	}

	// Avoid division by zero
	if roi.TotalUsageCost > 0 {
		roi.OptimizationRate = (roi.TotalSavings / (roi.TotalSavings + roi.TotalUsageCost)) * 100
	}

	// Find most frequently used model
	maxCount := 0
	for model, count := range modelDistribution {
		if count > maxCount {
			maxCount = count
			roi.AverageModel = model
		}
	}

	return roi
}
