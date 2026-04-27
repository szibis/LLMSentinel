package costs

import (
	"testing"
)

func TestNewBatchCalculator(t *testing.T) {
	bc := NewBatchCalculator()
	if bc == nil {
		t.Error("expected non-nil calculator")
	}
	if bc.batchDiscountPercent != 0.5 {
		t.Errorf("expected 50%% discount, got %f%%", bc.batchDiscountPercent*100)
	}
}

func TestCalculateRegularCost(t *testing.T) {
	bc := NewBatchCalculator()

	cost, err := bc.CalculateRegularCost("haiku", 1000, 500)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cost <= 0 {
		t.Errorf("expected positive cost, got %f", cost)
	}
}

func TestCalculateBatchCost(t *testing.T) {
	bc := NewBatchCalculator()

	cost, err := bc.CalculateBatchCost("haiku", 1000, 500)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cost <= 0 {
		t.Errorf("expected positive cost, got %f", cost)
	}
}

func TestCompareCosts(t *testing.T) {
	bc := NewBatchCalculator()

	comparison, err := bc.CompareCosts("haiku", 1000, 500)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Batch cost should be less than regular cost
	if comparison.BatchCost >= comparison.RegularCost {
		t.Errorf("expected batch cost < regular cost, got %f >= %f", comparison.BatchCost, comparison.RegularCost)
	}

	// Savings should be positive
	if comparison.SavingsAmount <= 0 {
		t.Errorf("expected positive savings, got %f", comparison.SavingsAmount)
	}

	// Savings percent should be around 50%
	if comparison.SavingsPercent < 40 || comparison.SavingsPercent > 60 {
		t.Errorf("expected savings percent ~50%%, got %.1f%%", comparison.SavingsPercent)
	}

	// Total tokens should match input + output
	if comparison.TotalTokens != 1500 {
		t.Errorf("expected 1500 total tokens, got %d", comparison.TotalTokens)
	}
}

func TestRecordRegularRequest(t *testing.T) {
	bc := NewBatchCalculator()

	err := bc.RecordRegularRequest("haiku", 1000, 500)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	summary := bc.GetCostSummary()
	if summary.TotalRegularCost <= 0 {
		t.Errorf("expected positive regular cost, got %f", summary.TotalRegularCost)
	}
	if summary.TotalBatchCost != 0 {
		t.Errorf("expected 0 batch cost, got %f", summary.TotalBatchCost)
	}
	if summary.RegularRequestsProcessed != 1 {
		t.Errorf("expected 1 regular request processed, got %d", summary.RegularRequestsProcessed)
	}
}

func TestRecordBatchRequest(t *testing.T) {
	bc := NewBatchCalculator()

	err := bc.RecordBatchRequest("haiku", 1000, 500)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	summary := bc.GetCostSummary()
	if summary.TotalBatchCost <= 0 {
		t.Errorf("expected positive batch cost, got %f", summary.TotalBatchCost)
	}
	if summary.BatchRequestsProcessed != 1 {
		t.Errorf("expected 1 batch request processed, got %d", summary.BatchRequestsProcessed)
	}
}

func TestGetCostSummary(t *testing.T) {
	bc := NewBatchCalculator()

	bc.RecordRegularRequest("haiku", 1000, 500)
	bc.RecordBatchRequest("haiku", 1000, 500)

	summary := bc.GetCostSummary()

	if summary.TotalRequestsProcessed != 2 {
		t.Errorf("expected 2 requests processed, got %d", summary.TotalRequestsProcessed)
	}

	if summary.RegularRequestsProcessed != 1 {
		t.Errorf("expected 1 regular request, got %d", summary.RegularRequestsProcessed)
	}

	if summary.BatchRequestsProcessed != 1 {
		t.Errorf("expected 1 batch request, got %d", summary.BatchRequestsProcessed)
	}

	// Batch cost should be less than regular cost
	if summary.TotalBatchCost >= summary.TotalRegularCost {
		t.Errorf("batch cost should be less than regular cost")
	}

	// Should have savings
	if summary.TotalSavings <= 0 {
		t.Errorf("expected positive total savings, got %f", summary.TotalSavings)
	}

	if summary.SavingsPercent <= 0 {
		t.Errorf("expected positive savings percent, got %f", summary.SavingsPercent)
	}
}

func TestEstimateMonthlyBatch(t *testing.T) {
	result, err := EstimateMonthlyBatch(10, 1000, 500, "haiku")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == "" {
		t.Error("expected non-empty result")
	}
}

func TestCalculateBatchROI(t *testing.T) {
	roi := CalculateBatchROI(0.001, 60.0)
	if roi <= 0 || roi > 1.0 {
		t.Errorf("expected ROI in (0, 1], got %f", roi)
	}
}

func TestCalculateBatchROIZeroWait(t *testing.T) {
	roi := CalculateBatchROI(0.001, 0)
	if roi <= 0 || roi > 1.0 {
		t.Errorf("expected positive ROI with zero wait, got %f", roi)
	}
}

func TestCalculateBatchROIClamping(t *testing.T) {
	roi := CalculateBatchROI(100.0, 1.0)
	if roi != 1.0 {
		t.Errorf("expected ROI clamped to 1.0, got %f", roi)
	}
}

func TestIsBatchWorthwhile(t *testing.T) {
	if !IsBatchWorthwhile(0.05, 30.0, 0.1) {
		t.Error("expected batch to be worthwhile")
	}

	if IsBatchWorthwhile(0.001, 100.0, 0.1) {
		t.Error("expected batch to NOT be worthwhile")
	}
}

func TestDifferentModels(t *testing.T) {
	bc := NewBatchCalculator()

	tests := []string{"haiku", "sonnet", "opus"}
	for _, model := range tests {
		comparison, err := bc.CompareCosts(model, 1000, 500)
		if err != nil {
			t.Fatalf("error comparing costs for %s: %v", model, err)
		}

		if comparison.BatchCost >= comparison.RegularCost {
			t.Errorf("batch should be cheaper for %s", model)
		}

		if comparison.SavingsPercent < 40 || comparison.SavingsPercent > 60 {
			t.Errorf("expected savings ~50%% for %s, got %.1f%%", model, comparison.SavingsPercent)
		}
	}
}

func TestVariousTokenSizes(t *testing.T) {
	bc := NewBatchCalculator()

	tests := []struct {
		input  int
		output int
	}{
		{100, 50},
		{1000, 500},
		{10000, 5000},
		{100000, 50000},
	}

	for _, test := range tests {
		comparison, err := bc.CompareCosts("haiku", test.input, test.output)
		if err != nil {
			t.Fatalf("error comparing costs for %d/%d tokens: %v", test.input, test.output, err)
		}

		if comparison.BatchCost >= comparison.RegularCost {
			t.Errorf("batch should be cheaper for %d/%d tokens", test.input, test.output)
		}
	}
}
