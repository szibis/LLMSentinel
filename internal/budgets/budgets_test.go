package budgets

import (
	"testing"
)

func TestCheckBudgetWithinLimit(t *testing.T) {
	cfg := BudgetConfig{
		DailyBudgetUSD:   10.0,
		MonthlyBudgetUSD: 100.0,
		HardLimit:        true,
	}

	engine := NewEngine(cfg)
	result := engine.CheckBudget("sonnet", 0.50, "")

	if !result.IsAllowed {
		t.Errorf("request within budget should be allowed")
	}

	if !result.WithinAllBudgets {
		t.Errorf("should report within all budgets")
	}

	if len(result.ViolatingBudgets) > 0 {
		t.Errorf("no budgets should be violated, got: %v", result.ViolatingBudgets)
	}
}

func TestCheckBudgetDailyExceeded(t *testing.T) {
	cfg := BudgetConfig{
		DailyBudgetUSD:   10.0,
		MonthlyBudgetUSD: 100.0,
		HardLimit:        true,
	}

	engine := NewEngine(cfg)
	// Simulate usage
	engine.state.DailyUsedUSD = 9.5

	result := engine.CheckBudget("opus", 0.60, "")

	if result.IsAllowed {
		t.Errorf("request exceeding daily budget should not be allowed with hard limit")
	}

	if !contains(result.ViolatingBudgets, "daily") {
		t.Errorf("daily should be in violated budgets, got: %v", result.ViolatingBudgets)
	}
}

func TestCheckBudgetMonthlyExceeded(t *testing.T) {
	cfg := BudgetConfig{
		DailyBudgetUSD:   10.0,
		MonthlyBudgetUSD: 100.0,
		HardLimit:        true,
	}

	engine := NewEngine(cfg)
	engine.state.MonthlyUsedUSD = 99.5

	result := engine.CheckBudget("opus", 0.60, "")

	if result.IsAllowed {
		t.Errorf("request exceeding monthly budget should not be allowed")
	}

	if !contains(result.ViolatingBudgets, "monthly") {
		t.Errorf("monthly should be in violated budgets")
	}
}

func TestSoftLimitAllows(t *testing.T) {
	cfg := BudgetConfig{
		DailyBudgetUSD:   10.0,
		MonthlyBudgetUSD: 100.0,
		HardLimit:        false,
		SoftLimit:        true,
	}

	engine := NewEngine(cfg)
	engine.state.DailyUsedUSD = 9.5

	result := engine.CheckBudget("opus", 0.60, "")

	if !result.IsAllowed {
		t.Errorf("soft limit should allow over-budget requests")
	}

	if result.WithinAllBudgets {
		t.Errorf("should report exceeding budgets even with soft limit")
	}
}

func TestPerModelLimit(t *testing.T) {
	cfg := BudgetConfig{
		DailyBudgetUSD:   20.0,
		MonthlyBudgetUSD: 200.0,
		HardLimit:        true,
		ModelDailyLimits: map[string]float64{
			"opus":   5.0,
			"sonnet": 3.0,
			"haiku":  0, // unlimited
		},
	}

	engine := NewEngine(cfg)
	engine.state.ModelDailyUsed["opus"] = 4.9

	result := engine.CheckBudget("opus", 0.2, "")

	if result.IsAllowed {
		t.Errorf("request exceeding per-model limit should fail")
	}

	if !contains(result.ViolatingBudgets, "opus-daily") {
		t.Errorf("model-specific limit should be reported")
	}
}

func TestWarningLevels(t *testing.T) {
	cfg := BudgetConfig{
		DailyBudgetUSD:   10.0,
		MonthlyBudgetUSD: 100.0,
	}

	tests := []struct {
		dailyUsed        float64
		requestCost      float64
		expectedWarning  int
	}{
		{2.0, 1.0, 0},    // 30% - no warning
		{5.0, 0.5, 1},    // 55% - low warning
		{7.5, 0.5, 2},    // 80% - medium warning
		{9.0, 0.5, 3},    // 95% - high warning
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			engine := NewEngine(cfg)
			engine.state.DailyUsedUSD = tt.dailyUsed

			result := engine.CheckBudget("sonnet", tt.requestCost, "")

			if result.WarningLevel != tt.expectedWarning {
				t.Logf("at %.1f%% usage, expected warning %d, got %d",
					((tt.dailyUsed+tt.requestCost)/cfg.DailyBudgetUSD)*100,
					tt.expectedWarning, result.WarningLevel)
			}
		})
	}
}

func TestZeroBudgetAllows(t *testing.T) {
	cfg := BudgetConfig{
		DailyBudgetUSD:   0, // Zero budget (unlimited)
		MonthlyBudgetUSD: 0,
		HardLimit:        false,
	}

	engine := NewEngine(cfg)
	result := engine.CheckBudget("opus", 100.0, "")

	if !result.IsAllowed {
		t.Errorf("zero budget should allow any amount")
	}
}

func BenchmarkCheckBudget(b *testing.B) {
	cfg := BudgetConfig{
		DailyBudgetUSD:   10.0,
		MonthlyBudgetUSD: 100.0,
	}

	engine := NewEngine(cfg)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine.CheckBudget("sonnet", 0.30, "concurrency")
	}
}

func BenchmarkCheckBudgetComplex(b *testing.B) {
	cfg := BudgetConfig{
		DailyBudgetUSD:   10.0,
		MonthlyBudgetUSD: 100.0,
		ModelDailyLimits: map[string]float64{
			"opus":   5.0,
			"sonnet": 3.0,
		},
		TaskTypeBudgets: map[string]int{
			"concurrency": 5000,
			"parsing":     2000,
		},
	}

	engine := NewEngine(cfg)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine.CheckBudget("opus", 0.30, "concurrency")
	}
}

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
