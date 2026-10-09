package budgets

import (
	"testing"
	"time"
)

func TestUsageStatusAndDowngradeBoundaries(t *testing.T) {
	e := NewEngine(BudgetConfig{DailyBudgetUSD: 10, MonthlyBudgetUSD: 100, SessionBudgetTokens: 1000, AutoDowngradeAtPercent: .8, HardLimit: true})
	before := time.Now()
	e.RecordUsage("sonnet", 8, "code", 300)
	if e.ShouldDowngrade() {
		t.Fatal("threshold equality should not downgrade")
	}
	e.RecordUsage("", .1, "", 10)
	if !e.ShouldDowngrade() {
		t.Fatal("above threshold must downgrade")
	}
	status := e.GetStatus()
	daily := status["daily"].(map[string]interface{})
	monthly := status["monthly"].(map[string]interface{})
	session := status["session"].(map[string]interface{})
	if daily["used"] != 8.1 || monthly["used"] != 8.1 || session["used"] != 310 || session["remaining"] != 690 {
		t.Fatalf("status=%+v", status)
	}
	if e.state.ModelDailyUsed["sonnet"] != 8 || e.state.TaskTypeUsed["code"] != 300 || len(e.state.ModelDailyUsed) != 1 || e.state.Timestamp.Before(before) {
		t.Fatal("usage attribution incorrect")
	}
	if r := e.CheckBudget("sonnet", 2, "code"); r.IsAllowed || r.RecommendedModel != "haiku" || r.SavingsByDowngrade <= 0 {
		t.Fatalf("check=%+v", r)
	}
	if r := e.CheckBudget("unknown", 2, "code"); r.SavingsByDowngrade != 0 {
		t.Fatal("unknown model savings invented")
	}
	if NewEngine(BudgetConfig{}).ShouldDowngrade() {
		t.Fatal("unlimited budget must not downgrade")
	}
}
