package costs

import (
	"math"
	"testing"
)

func TestCalculateCost_Haiku(t *testing.T) {
	calc := NewCalculator()
	tokens := TokenCosts{
		InputTokens:  1000,
		OutputTokens: 500,
		IsBatchAPI:   false,
		IsCached:     false,
	}

	breakdown, err := calc.CalculateCost("haiku", tokens)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Haiku: 0.80 per 1M input + 4.00 per 1M output
	expectedInput := 1000 * 0.80 / 1_000_000
	expectedOutput := 500 * 4.00 / 1_000_000
	expectedTotal := expectedInput + expectedOutput

	if math.Abs(breakdown.InputCost-expectedInput) > 1e-9 {
		t.Errorf("input cost: expected %f, got %f", expectedInput, breakdown.InputCost)
	}
	if math.Abs(breakdown.OutputCost-expectedOutput) > 1e-9 {
		t.Errorf("output cost: expected %f, got %f", expectedOutput, breakdown.OutputCost)
	}
	if math.Abs(breakdown.TotalCost-expectedTotal) > 1e-9 {
		t.Errorf("total cost: expected %f, got %f", expectedTotal, breakdown.TotalCost)
	}
}

func TestCalculateCost_WithCache(t *testing.T) {
	calc := NewCalculator()
	tokens := TokenCosts{
		InputTokens:     1000,
		OutputTokens:    500,
		CacheReadTokens: 200,  // 10% of input cost
		IsBatchAPI:      false,
		IsCached:        true,
	}

	breakdown, err := calc.CalculateCost("sonnet", tokens)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have cache read cost savings
	if breakdown.CacheReadCost <= 0 {
		t.Error("expected cache read cost to be calculated")
	}
	if breakdown.CacheSavings <= 0 {
		t.Error("expected cache savings to be positive")
	}
}

func TestCalculateCost_WithBatch(t *testing.T) {
	calc := NewCalculator()
	tokens := TokenCosts{
		InputTokens:  1000,
		OutputTokens: 500,
		IsBatchAPI:   true,
		IsCached:     false,
	}

	breakdown, err := calc.CalculateCost("sonnet", tokens)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Batch API should have 50% discount
	if breakdown.BatchSavings <= 0 {
		t.Error("expected batch savings to be positive")
	}

	// Calculate expected cost without batch discount
	inputCost := 1000 * 3.0 / 1_000_000
	outputCost := 500 * 15.0 / 1_000_000
	baseCost := inputCost + outputCost
	expectedCost := baseCost * 0.5 // 50% discount

	if math.Abs(breakdown.TotalCost-expectedCost) > 1e-9 {
		t.Errorf("batch cost: expected %f, got %f", expectedCost, breakdown.TotalCost)
	}
}

func TestCompareModels(t *testing.T) {
	calc := NewCalculator()
	tokens := TokenCosts{
		InputTokens:  1000,
		OutputTokens: 500,
		IsBatchAPI:   false,
		IsCached:     false,
	}

	results, err := calc.CompareModels(tokens)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("expected 3 models, got %d", len(results))
	}

	// Haiku should be cheapest, Opus most expensive
	haiku := results["haiku"]
	sonnet := results["sonnet"]
	opus := results["opus"]

	if haiku.TotalCost >= sonnet.TotalCost || sonnet.TotalCost >= opus.TotalCost {
		t.Error("expected haiku < sonnet < opus in cost")
	}

	// Savings vs opus should be accurate
	if haiku.SavingsVsOpus != opus.TotalCost-haiku.TotalCost {
		t.Error("savings vs opus calculation incorrect")
	}
}

func TestEstimateCost(t *testing.T) {
	calc := NewCalculator()

	// Estimate for a small prompt
	breakdown, err := calc.EstimateCost("sonnet", 1000, 500)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if breakdown.TotalCost <= 0 {
		t.Error("expected positive cost estimate")
	}

	// Check that cost increases with more tokens
	breakdown2, _ := calc.EstimateCost("sonnet", 5000, 2500)
	if breakdown2.TotalCost <= breakdown.TotalCost {
		t.Error("expected larger prompt to cost more")
	}
}

func TestCalculateCostError(t *testing.T) {
	calc := NewCalculator()

	// 20% under-estimate
	error := calc.CalculateCostError(100, 120)
	if error != 20.0 {
		t.Errorf("expected 20%% error, got %f%%", error)
	}

	// 10% over-estimate
	error = calc.CalculateCostError(100, 90)
	if error != -10.0 {
		t.Errorf("expected -10%% error, got %f%%", error)
	}

	// Perfect estimate
	error = calc.CalculateCostError(100, 100)
	if error != 0 {
		t.Errorf("expected 0%% error, got %f%%", error)
	}

	// Zero estimate should return 0
	error = calc.CalculateCostError(0, 100)
	if error != 0 {
		t.Errorf("expected 0%% for zero estimate, got %f%%", error)
	}
}

func TestROICalculation(t *testing.T) {
	roi := NewROICalculator()

	// Create sample breakdown data
	breakdowns := map[string]CostBreakdown{
		"txn1": {TotalCost: 0.10, SavingsVsOpus: 0.25},
		"txn2": {TotalCost: 0.05, SavingsVsOpus: 0.30},
		"txn3": {TotalCost: 0.15, SavingsVsOpus: 0.20},
	}

	distribution := map[string]int{
		"haiku":  2,
		"sonnet": 1,
	}

	result := roi.CalculateROI(breakdowns, distribution)

	if result.TotalUsageCost != 0.30 {
		t.Errorf("expected total usage cost 0.30, got %f", result.TotalUsageCost)
	}

	if result.TotalSavings != 0.75 {
		t.Errorf("expected total savings 0.75, got %f", result.TotalSavings)
	}

	expectedRate := (0.75 / (0.75 + 0.30)) * 100
	if math.Abs(result.OptimizationRate-expectedRate) > 0.01 {
		t.Errorf("expected optimization rate %f%%, got %f%%", expectedRate, result.OptimizationRate)
	}

	if result.AverageModel != "haiku" {
		t.Errorf("expected most used model haiku, got %s", result.AverageModel)
	}
}

func TestUnknownModel(t *testing.T) {
	calc := NewCalculator()
	tokens := TokenCosts{
		InputTokens:  1000,
		OutputTokens: 500,
	}

	_, err := calc.CalculateCost("unknown", tokens)
	if err == nil {
		t.Error("expected error for unknown model")
	}
}

func TestEffectiveCostPerToken(t *testing.T) {
	calc := NewCalculator()
	tokens := TokenCosts{
		InputTokens:  1_000_000, // 1M tokens
		OutputTokens: 1_000_000, // 1M tokens
		IsBatchAPI:   false,
		IsCached:     false,
	}

	breakdown, _ := calc.CalculateCost("haiku", tokens)

	// For Haiku: cost = 0.80 + 4.00 = 4.80 for 2M tokens
	// EffectiveCostPerToken = 4.80 / 2M * 1M = 2.40 per million tokens
	expectedRate := 2.40

	if math.Abs(breakdown.EffectiveCostPerToken-expectedRate) > 1e-9 {
		t.Errorf("expected effective rate %f, got %f", expectedRate, breakdown.EffectiveCostPerToken)
	}
}
