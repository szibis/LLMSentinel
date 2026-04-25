package budgets

import (
	"fmt"
	"time"
)

// BudgetConfig defines all budget parameters.
type BudgetConfig struct {
	// User budgets
	DailyBudgetUSD   float64
	MonthlyBudgetUSD float64
	SessionBudgetTokens int

	// Model daily limits (caps per model per day)
	ModelDailyLimits map[string]float64  // e.g., opus: $5.00

	// Task-type budgets (caps per task type)
	TaskTypeBudgets map[string]int  // e.g., concurrency: 5000 tokens

	// Enforcement
	HardLimit bool  // true: reject if over, false: allow with warning
	SoftLimit bool  // true: warn at thresholds

	// Protective actions
	AutoDowngradeAtPercent float64  // e.g., 0.80 = downgrade when 80% used
	AlertThresholds        map[string]float64  // e.g., 0.5, 0.75, 0.90
}

// BudgetState tracks current spending.
type BudgetState struct {
	DailyUsedUSD     float64
	MonthlyUsedUSD   float64
	SessionUsedTokens int

	ModelDailyUsed   map[string]float64
	TaskTypeUsed     map[string]int

	Timestamp time.Time
	LastReset time.Time
}

// CheckResult is returned by budget checks.
type CheckResult struct {
	IsAllowed         bool
	WithinAllBudgets  bool
	ViolatingBudgets  []string  // Which budgets exceeded?
	RecommendedModel  string    // Alternative model if current exceeds
	WarningLevel      int       // 0: ok, 1: 50%, 2: 75%, 3: 90%, 4: exceeded
	Message           string
	SavingsByDowngrade float64  // Estimated tokens saved if downgrade used
}

// Engine enforces budget constraints.
type Engine struct {
	config BudgetConfig
	state  BudgetState
}

// NewEngine creates a budget engine.
func NewEngine(config BudgetConfig) *Engine {
	if config.ModelDailyLimits == nil {
		config.ModelDailyLimits = make(map[string]float64)
	}
	if config.TaskTypeBudgets == nil {
		config.TaskTypeBudgets = make(map[string]int)
	}
	if config.AlertThresholds == nil {
		config.AlertThresholds = map[string]float64{
			"warn_low":  0.50,
			"warn_med":  0.75,
			"warn_high": 0.90,
		}
	}

	return &Engine{
		config: config,
		state: BudgetState{
			ModelDailyUsed: make(map[string]float64),
			TaskTypeUsed:   make(map[string]int),
			Timestamp:      time.Now(),
			LastReset:      time.Now(),
		},
	}
}

// CheckBudget verifies if operation is within budgets.
func (e *Engine) CheckBudget(model string, estimatedCostUSD float64, taskType string) CheckResult {
	result := CheckResult{
		IsAllowed:        true,
		WithinAllBudgets: true,
	}

	// Check daily budget
	if e.state.DailyUsedUSD+estimatedCostUSD > e.config.DailyBudgetUSD {
		result.ViolatingBudgets = append(result.ViolatingBudgets, "daily")
		result.WithinAllBudgets = false
	}

	// Check monthly budget
	if e.state.MonthlyUsedUSD+estimatedCostUSD > e.config.MonthlyBudgetUSD {
		result.ViolatingBudgets = append(result.ViolatingBudgets, "monthly")
		result.WithinAllBudgets = false
	}

	// Check model-specific limit
	if modelLimit, exists := e.config.ModelDailyLimits[model]; exists {
		if e.state.ModelDailyUsed[model]+estimatedCostUSD > modelLimit {
			result.ViolatingBudgets = append(result.ViolatingBudgets, fmt.Sprintf("%s-daily", model))
			result.WithinAllBudgets = false
		}
	}

	// Determine enforcement action
	if !result.WithinAllBudgets {
		if e.config.HardLimit {
			result.IsAllowed = false
			result.Message = fmt.Sprintf("Hard budget limit exceeded: %v", result.ViolatingBudgets)
		} else if e.config.SoftLimit {
			result.IsAllowed = true
			result.Message = fmt.Sprintf("Warning: would exceed %v (allow with confirmation)", result.ViolatingBudgets)
		}

		// Recommend cheaper model
		result.RecommendedModel = findCheaperModel(model)
		result.SavingsByDowngrade = estimateSavings(estimatedCostUSD, model, result.RecommendedModel)
	}

	// Check warning thresholds
	dailyPercent := (e.state.DailyUsedUSD + estimatedCostUSD) / e.config.DailyBudgetUSD
	if dailyPercent > e.config.AlertThresholds["warn_high"] {
		result.WarningLevel = 3
	} else if dailyPercent > e.config.AlertThresholds["warn_med"] {
		result.WarningLevel = 2
	} else if dailyPercent > e.config.AlertThresholds["warn_low"] {
		result.WarningLevel = 1
	}

	return result
}

// RecordUsage updates state with actual usage.
func (e *Engine) RecordUsage(model string, actualCostUSD float64, taskType string, tokens int) {
	e.state.DailyUsedUSD += actualCostUSD
	e.state.MonthlyUsedUSD += actualCostUSD
	e.state.SessionUsedTokens += tokens

	if model != "" {
		e.state.ModelDailyUsed[model] += actualCostUSD
	}

	if taskType != "" {
		e.state.TaskTypeUsed[taskType] += tokens
	}

	e.state.Timestamp = time.Now()
}

// ShouldDowngrade checks if approaching limit.
func (e *Engine) ShouldDowngrade() bool {
	if e.config.DailyBudgetUSD == 0 {
		return false
	}
	percentUsed := e.state.DailyUsedUSD / e.config.DailyBudgetUSD
	return percentUsed > e.config.AutoDowngradeAtPercent
}

// GetStatus returns current budget status.
func (e *Engine) GetStatus() map[string]interface{} {
	return map[string]interface{}{
		"daily": map[string]interface{}{
			"limit":       e.config.DailyBudgetUSD,
			"used":        e.state.DailyUsedUSD,
			"remaining":   e.config.DailyBudgetUSD - e.state.DailyUsedUSD,
			"percent_used": (e.state.DailyUsedUSD / e.config.DailyBudgetUSD) * 100,
		},
		"monthly": map[string]interface{}{
			"limit":       e.config.MonthlyBudgetUSD,
			"used":        e.state.MonthlyUsedUSD,
			"remaining":   e.config.MonthlyBudgetUSD - e.state.MonthlyUsedUSD,
			"percent_used": (e.state.MonthlyUsedUSD / e.config.MonthlyBudgetUSD) * 100,
		},
		"session": map[string]interface{}{
			"limit":       e.config.SessionBudgetTokens,
			"used":        e.state.SessionUsedTokens,
			"remaining":   e.config.SessionBudgetTokens - e.state.SessionUsedTokens,
		},
		"models":     e.state.ModelDailyUsed,
		"task_types": e.state.TaskTypeUsed,
	}
}

// findCheaperModel suggests a cheaper alternative.
func findCheaperModel(current string) string {
	// Cost hierarchy: haiku < sonnet < opus
	switch current {
	case "opus":
		return "sonnet"  // Try Sonnet (40% cheaper)
	case "sonnet":
		return "haiku"   // Try Haiku (90% cheaper)
	default:
		return "haiku"
	}
}

// estimateSavings calculates cost savings for downgrade.
func estimateSavings(cost float64, current, recommended string) float64 {
	// Rough cost multipliers
	costMultiplier := map[string]float64{
		"haiku":  1.0,
		"sonnet": 5.0,
		"opus":   15.0,
	}

	currentMult := costMultiplier[current]
	recommendedMult := costMultiplier[recommended]

	if currentMult == 0 {
		return 0
	}

	// Estimated savings as percentage
	savedPercent := (currentMult - recommendedMult) / currentMult
	return cost * savedPercent
}
