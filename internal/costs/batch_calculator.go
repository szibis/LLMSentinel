package costs

import (
	"fmt"
	"sync"
)

// BatchCalculator handles batch API cost calculations and comparisons
type BatchCalculator struct {
	mu                     sync.RWMutex
	calc                   *Calculator
	batchDiscountPercent   float64 // Default 50% (0.5)
	totalRegularCost       float64
	totalBatchCost         float64
	totalTokensSaved       int64
	requestsProcessed      int64
	batchRequestsProcessed int64
}

// NewBatchCalculator creates a new batch cost calculator
func NewBatchCalculator() *BatchCalculator {
	return &BatchCalculator{
		calc:                   NewCalculator(),
		batchDiscountPercent:   0.5, // 50% discount
		totalRegularCost:       0,
		totalBatchCost:         0,
		totalTokensSaved:       0,
		requestsProcessed:      0,
		batchRequestsProcessed: 0,
	}
}

// CalculateRegularCost returns cost for direct API call
func (bc *BatchCalculator) CalculateRegularCost(model string, inputTokens, outputTokens int) (float64, error) {
	tokens := TokenCosts{
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		IsBatchAPI:   false,
		IsCached:     false,
	}

	breakdown, err := bc.calc.CalculateCost(model, tokens)
	if err != nil {
		return 0, err
	}

	return breakdown.TotalCost, nil
}

// CalculateBatchCost returns cost for batch API call (applies discount)
func (bc *BatchCalculator) CalculateBatchCost(model string, inputTokens, outputTokens int) (float64, error) {
	tokens := TokenCosts{
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		IsBatchAPI:   true, // Anthropic applies 50% discount
		IsCached:     false,
	}

	breakdown, err := bc.calc.CalculateCost(model, tokens)
	if err != nil {
		return 0, err
	}

	return breakdown.TotalCost, nil
}

// CompareCosts returns detailed comparison between regular and batch processing
func (bc *BatchCalculator) CompareCosts(model string, inputTokens, outputTokens int) (BatchCostComparison, error) {
	regularCost, err := bc.CalculateRegularCost(model, inputTokens, outputTokens)
	if err != nil {
		return BatchCostComparison{}, err
	}

	batchCost, err := bc.CalculateBatchCost(model, inputTokens, outputTokens)
	if err != nil {
		return BatchCostComparison{}, err
	}

	savings := regularCost - batchCost
	savingsPercent := 0.0
	if regularCost > 0 {
		savingsPercent = (savings / regularCost) * 100
	}

	return BatchCostComparison{
		Model:          model,
		RegularCost:    regularCost,
		BatchCost:      batchCost,
		SavingsAmount:  savings,
		SavingsPercent: savingsPercent,
		InputTokens:    inputTokens,
		OutputTokens:   outputTokens,
		TotalTokens:    inputTokens + outputTokens,
	}, nil
}

// BatchCostComparison contains cost analysis for batch vs regular
type BatchCostComparison struct {
	Model          string
	RegularCost    float64
	BatchCost      float64
	SavingsAmount  float64
	SavingsPercent float64
	InputTokens    int
	OutputTokens   int
	TotalTokens    int
}

// RecordRegularRequest tracks a regular (non-batch) request
func (bc *BatchCalculator) RecordRegularRequest(model string, inputTokens, outputTokens int) error {
	cost, err := bc.CalculateRegularCost(model, inputTokens, outputTokens)
	if err != nil {
		return err
	}

	bc.mu.Lock()
	defer bc.mu.Unlock()

	bc.totalRegularCost += cost
	bc.requestsProcessed++

	return nil
}

// RecordBatchRequest tracks a batched request
func (bc *BatchCalculator) RecordBatchRequest(model string, inputTokens, outputTokens int) error {
	comparison, err := bc.CompareCosts(model, inputTokens, outputTokens)
	if err != nil {
		return err
	}

	bc.mu.Lock()
	defer bc.mu.Unlock()

	bc.totalBatchCost += comparison.BatchCost
	bc.totalTokensSaved += int64(comparison.InputTokens) / 2 // Roughly 50% saved
	bc.batchRequestsProcessed++
	bc.requestsProcessed++

	return nil
}

// GetCostSummary returns aggregate cost and savings data
func (bc *BatchCalculator) GetCostSummary() CostSummary {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	totalCost := bc.totalRegularCost + bc.totalBatchCost
	savingsPercent := 0.0
	if bc.totalRegularCost+bc.totalBatchCost > 0 {
		savings := bc.totalRegularCost - bc.totalBatchCost
		savingsPercent = (savings / (bc.totalRegularCost + bc.totalBatchCost)) * 100
	}

	return CostSummary{
		TotalRegularCost:         bc.totalRegularCost,
		TotalBatchCost:           bc.totalBatchCost,
		TotalCost:                totalCost,
		TotalSavings:             bc.totalRegularCost - bc.totalBatchCost,
		SavingsPercent:           savingsPercent,
		TotalTokensSaved:         bc.totalTokensSaved,
		TotalRequestsProcessed:   bc.requestsProcessed,
		BatchRequestsProcessed:   bc.batchRequestsProcessed,
		RegularRequestsProcessed: bc.requestsProcessed - bc.batchRequestsProcessed,
	}
}

// CostSummary contains aggregate cost tracking
type CostSummary struct {
	TotalRegularCost         float64
	TotalBatchCost           float64
	TotalCost                float64
	TotalSavings             float64
	SavingsPercent           float64
	TotalTokensSaved         int64
	TotalRequestsProcessed   int64
	BatchRequestsProcessed   int64
	RegularRequestsProcessed int64
}

// EstimateMonthlyBatch estimates monthly costs for a typical batch workload
func EstimateMonthlyBatch(requestsPerDay int, avgInputTokens, avgOutputTokens int, model string) (string, error) {
	calc := NewBatchCalculator()

	// Estimate daily batch processing
	requestsPerMonth := requestsPerDay * 30

	dailyComparison, err := calc.CompareCosts(model, avgInputTokens, avgOutputTokens)
	if err != nil {
		return "", err
	}

	monthlyRegular := dailyComparison.RegularCost * float64(requestsPerMonth)
	monthlyBatch := dailyComparison.BatchCost * float64(requestsPerMonth)
	monthlySavings := monthlyRegular - monthlyBatch

	result := fmt.Sprintf(
		"Monthly Batch Estimate (%d requests/day, %s model):\n"+
			"  Regular API: $%.2f\n"+
			"  Batch API: $%.2f\n"+
			"  Savings: $%.2f (%.1f%%)\n"+
			"  Tokens/request: %d input + %d output",
		requestsPerDay,
		model,
		monthlyRegular,
		monthlyBatch,
		monthlySavings,
		(monthlySavings/monthlyRegular)*100,
		avgInputTokens,
		avgOutputTokens,
	)

	return result, nil
}

// CalculateBatchROI computes batch ROI score (0.0-1.0)
// Formula: (savings * 100) / max(waitTime, 1.0), clamped to [0, 1]
func CalculateBatchROI(savingsAmount, waitTimeSecs float64) float64 {
	if waitTimeSecs <= 0 {
		waitTimeSecs = 1.0 // Minimum 1 second
	}

	roi := (savingsAmount * 100) / waitTimeSecs
	if roi > 1.0 {
		roi = 1.0
	}
	if roi < 0 {
		roi = 0
	}

	return roi
}

// IsBatchWorthwhile returns true if batching ROI exceeds threshold
func IsBatchWorthwhile(savingsAmount, waitTimeSecs, threshold float64) bool {
	return CalculateBatchROI(savingsAmount, waitTimeSecs) >= threshold
}
