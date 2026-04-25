package decisions

import (
	"fmt"

	"github.com/szibis/claude-escalate/internal/signals"
	"github.com/szibis/claude-escalate/internal/store"
)

// Decision represents an optimization decision
type Decision struct {
	Action              string // "cascade", "escalate", "stay", "adjust_effort"
	NextModel           string // "haiku", "sonnet", "opus"
	NextEffort          string // "low", "medium", "high"
	Reason              string
	Confidence          float64 // 0.0-1.0
	CascadeAvailable    bool
	EscalateAvailable   bool
	CostSavingsEstimate float64
}

// Engine makes optimization decisions based on tokens + signals
type Engine struct {
	// Thresholds
	TokenErrorThreshold    float64 // ±15% default
	CostErrorThreshold     float64 // ±10% default
	ModelAccuracyThreshold float64 // 85% default
	SuccessSignalThreshold float64 // 0.80 confidence default
	FailureSignalThreshold float64 // 0.80 confidence default
}

// NewEngine creates a new decision engine with default thresholds
func NewEngine() *Engine {
	return &Engine{
		TokenErrorThreshold:    15.0,
		CostErrorThreshold:     10.0,
		ModelAccuracyThreshold: 0.85,
		SuccessSignalThreshold: 0.80,
		FailureSignalThreshold: 0.80,
	}
}

// MakeDecision analyzes validation metrics and signals to make an optimization decision
func (e *Engine) MakeDecision(
	validation store.ValidationMetric,
	signal signals.Signal,
) Decision {
	decision := Decision{
		NextModel:  validation.RoutedModel,
		NextEffort: validation.DetectedEffort,
		Reason:     "No change needed",
		Confidence: 0.0,
	}

	// PRIORITY 1: Explicit escalation signal
	if signal.Type == signals.SignalEscalation {
		return Decision{
			Action:            "escalate",
			NextModel:         "opus", // Default to Opus on /escalate
			NextEffort:        validation.DetectedEffort,
			Reason:            "User explicitly requested escalation",
			Confidence:        signal.Confidence,
			EscalateAvailable: true,
		}
	}

	// PRIORITY 2: Success signal + tokens are within target
	if signal.Type == signals.SignalSuccess && signal.Confidence >= e.SuccessSignalThreshold {
		// User is happy. Can we cascade down?
		switch validation.RoutedModel {
		case "opus":
			return Decision{
				Action:           "cascade",
				NextModel:        "sonnet",
				NextEffort:       validation.DetectedEffort,
				Reason:           "User satisfied + model was over-provisioned",
				Confidence:       signal.Confidence,
				CascadeAvailable: true,
			}
		case "sonnet":
			return Decision{
				Action:           "cascade",
				NextModel:        "haiku",
				NextEffort:       validation.DetectedEffort,
				Reason:           "User satisfied + model was over-provisioned",
				Confidence:       signal.Confidence,
				CascadeAvailable: true,
			}
		}
		// Already on haiku, can't cascade further
		decision.Action = "stay"
		decision.Reason = "User satisfied, already on cheapest model"
		decision.Confidence = signal.Confidence
		return decision
	}

	// PRIORITY 3: Failure signal + current model insufficient
	if signal.Type == signals.SignalFailure && signal.Confidence >= e.FailureSignalThreshold {
		switch validation.RoutedModel {
		case "haiku":
			return Decision{
				Action:            "escalate",
				NextModel:         "sonnet",
				NextEffort:        "medium", // Upgrade effort too
				Reason:            "User unsatisfied, Haiku insufficient",
				Confidence:        signal.Confidence,
				EscalateAvailable: true,
			}
		case "sonnet":
			return Decision{
				Action:            "escalate",
				NextModel:         "opus",
				NextEffort:        "high", // Upgrade effort too
				Reason:            "User unsatisfied, Sonnet insufficient",
				Confidence:        signal.Confidence,
				EscalateAvailable: true,
			}
		}
		// Already on Opus
		decision.Action = "stay"
		decision.Reason = "Already on highest model, suggest manual escalation"
		decision.Confidence = signal.Confidence
		return decision
	}

	// PRIORITY 4: Token-based accuracy without explicit signal
	if validation.Validated {
		tokenError := validation.TokenError

		// If estimate was too low (used more tokens than predicted)
		if tokenError > e.TokenErrorThreshold {
			// Model was under-provisioned
			switch validation.RoutedModel {
			case "haiku":
				return Decision{
					Action:            "escalate",
					NextModel:         "sonnet",
					NextEffort:        "medium",
					Reason:            "Token usage exceeded estimate by " + formatPercent(tokenError),
					Confidence:        0.75,
					EscalateAvailable: true,
				}
			case "sonnet":
				return Decision{
					Action:            "escalate",
					NextModel:         "opus",
					NextEffort:        "high",
					Reason:            "Token usage exceeded estimate by " + formatPercent(tokenError),
					Confidence:        0.75,
					EscalateAvailable: true,
				}
			}
		}

		// If estimate was too high (used fewer tokens than predicted)
		if tokenError < -(e.TokenErrorThreshold) {
			// Model was over-provisioned
			switch validation.RoutedModel {
			case "opus":
				return Decision{
					Action:           "cascade",
					NextModel:        "sonnet",
					NextEffort:       validation.DetectedEffort,
					Reason:           "Token usage " + formatPercent(tokenError) + " under estimate, can downgrade",
					Confidence:       0.80,
					CascadeAvailable: true,
				}
			case "sonnet":
				return Decision{
					Action:           "cascade",
					NextModel:        "haiku",
					NextEffort:       "low",
					Reason:           "Token usage " + formatPercent(tokenError) + " under estimate, can downgrade",
					Confidence:       0.80,
					CascadeAvailable: true,
				}
			}
		}

		// Tokens within acceptable range
		if tokenError >= -(e.TokenErrorThreshold) && tokenError <= e.TokenErrorThreshold {
			decision.Action = "stay"
			decision.Reason = "Token estimate accurate within ±" + formatPercent(e.TokenErrorThreshold)
			decision.Confidence = 0.90
			return decision
		}
	}

	// PRIORITY 5: Effort signal
	if signal.Type == signals.SignalEffortHigh {
		return Decision{
			Action:     "adjust_effort",
			NextModel:  e.escalateByEffort(validation.RoutedModel),
			NextEffort: "high",
			Reason:     "User indicates high complexity",
			Confidence: signal.Confidence,
		}
	}

	if signal.Type == signals.SignalEffortLow {
		return Decision{
			Action:     "adjust_effort",
			NextModel:  e.deescalateByEffort(validation.RoutedModel),
			NextEffort: "low",
			Reason:     "User indicates low complexity",
			Confidence: signal.Confidence,
		}
	}

	return decision
}

// escalateByEffort returns a more capable model
func (e *Engine) escalateByEffort(currentModel string) string {
	switch currentModel {
	case "haiku":
		return "sonnet"
	case "sonnet":
		return "opus"
	}
	return "opus"
}

// deescalateByEffort returns a cheaper model
func (e *Engine) deescalateByEffort(currentModel string) string {
	switch currentModel {
	case "opus":
		return "sonnet"
	case "sonnet":
		return "haiku"
	}
	return "haiku"
}

// CalculateLearning analyzes a set of validations to extract patterns
func (e *Engine) CalculateLearning(validations []store.ValidationMetric) map[string]interface{} {
	learning := map[string]interface{}{
		"low_effort":    e.analyzeEffortLevel(validations, "low"),
		"medium_effort": e.analyzeEffortLevel(validations, "medium"),
		"high_effort":   e.analyzeEffortLevel(validations, "high"),
	}
	return learning
}

// analyzeEffortLevel extracts statistics for a specific effort level
func (e *Engine) analyzeEffortLevel(validations []store.ValidationMetric, effort string) map[string]interface{} {
	var effortValidations []store.ValidationMetric
	for _, v := range validations {
		if v.DetectedEffort == effort {
			effortValidations = append(effortValidations, v)
		}
	}

	if len(effortValidations) == 0 {
		return map[string]interface{}{
			"count":   0,
			"samples": "insufficient",
		}
	}

	// Calculate statistics
	totalError := 0.0
	successCount := 0
	modelCounts := make(map[string]int)

	for _, v := range effortValidations {
		if v.Validated {
			totalError += v.TokenError
			modelCounts[v.RoutedModel]++

			// Simple success detection: error within threshold and positive outcome
			if v.TokenError >= -e.TokenErrorThreshold && v.TokenError <= e.TokenErrorThreshold {
				successCount++
			}
		}
	}

	avgError := totalError / float64(len(effortValidations))
	successRate := float64(successCount) / float64(len(effortValidations))

	// Find best model for this effort level
	bestModel := "haiku"
	bestCount := 0
	for model, count := range modelCounts {
		if count > bestCount {
			bestCount = count
			bestModel = model
		}
	}

	return map[string]interface{}{
		"count":           len(effortValidations),
		"avg_token_error": roundFloat(avgError, 2),
		"success_rate":    roundFloat(successRate*100, 1),
		"best_model":      bestModel,
		"model_counts":    modelCounts,
	}
}

// Helper functions

func formatPercent(value float64) string {
	if value > 0 {
		return "+" + roundFloatStr(value, 1) + "%"
	}
	return roundFloatStr(value, 1) + "%"
}

func roundFloat(value float64, decimals int) float64 {
	multiplier := 1.0
	for i := 0; i < decimals; i++ {
		multiplier *= 10
	}
	return float64(int(value*multiplier)) / multiplier
}

func roundFloatStr(value float64, decimals int) string {
	format := fmt.Sprintf("%%.%df", decimals)
	return fmt.Sprintf(format, value)
}
