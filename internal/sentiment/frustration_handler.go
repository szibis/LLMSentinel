package sentiment

import (
	"fmt"
)

// Decision represents a routing decision based on sentiment.
type Decision struct {
	Action          string  // "escalate", "de-escalate", "adjust_effort", "continue"
	NextModel       string  // haiku, sonnet, opus
	NextEffort      string  // low, medium, high
	Rationale       string
	Confidence      float64
	Warning         string  // Optional warning to user
	ShouldInterrupt bool    // If true, stop current operation
}

// FrustrationHandler makes escalation decisions based on sentiment.
type FrustrationHandler struct {
	sentimentDetector *Detector
	maxAttemptsBeforeOpus int
}

// NewFrustrationHandler creates a handler.
func NewFrustrationHandler() *FrustrationHandler {
	return &FrustrationHandler{
		sentimentDetector:     NewDetector(),
		maxAttemptsBeforeOpus: 2,
	}
}

// HandleFrustration determines if escalation is needed based on sentiment.
func (fh *FrustrationHandler) HandleFrustration(
	sentiment Score,
	currentModel string,
	attemptCount int,
	taskType string,
) *Decision {

	// If frustration risk is high, escalate immediately
	if sentiment.FrustrationRisk > 0.70 {
		return fh.escalateOnFrustration(sentiment, currentModel, attemptCount, taskType)
	}

	// If user is impatient, use faster model
	if sentiment.Primary == SentimentImpatient {
		return &Decision{
			Action:     "adjust_effort",
			NextModel:  "haiku",
			NextEffort: "low",
			Rationale:  "User impatient, switching to fastest model (Haiku)",
			Confidence: 0.8,
			Warning:    "Note: Haiku is fastest but less capable. May need escalation if it fails.",
		}
	}

	// If user is confused, escalate from Haiku for better explanations
	if sentiment.Primary == SentimentConfused && currentModel == "haiku" {
		return &Decision{
			Action:     "escalate",
			NextModel:  "sonnet",
			NextEffort: "medium",
			Rationale:  "User confused, Sonnet better at explanations",
			Confidence: 0.85,
		}
	}

	// No frustration-driven action needed
	return nil
}

// escalateOnFrustration handles escalation when user is frustrated.
func (fh *FrustrationHandler) escalateOnFrustration(
	sentiment Score,
	currentModel string,
	attemptCount int,
	taskType string,
) *Decision {

	// Already on Opus? No further escalation possible
	if currentModel == "opus" {
		return nil
	}

	// First or second attempt: escalate by one level
	if attemptCount < fh.maxAttemptsBeforeOpus {
		nextModel := escalateByOne(currentModel)
		return &Decision{
			Action:     "escalate",
			NextModel:  nextModel,
			NextEffort: effortForModel(nextModel),
			Rationale: fmt.Sprintf(
				"User frustrated (risk: %.0f%%). Task type: %s. Escalating from %s.",
				sentiment.FrustrationRisk*100, taskType, currentModel,
			),
			Confidence: sentiment.FrustrationRisk,
		}
	}

	// Multiple failed attempts + frustration: go to Opus
	return &Decision{
		Action:     "escalate",
		NextModel:  "opus",
		NextEffort: "high",
		Rationale: fmt.Sprintf(
			"User frustrated after %d attempts. Task: %s. Using Opus for highest capability.",
			attemptCount, taskType,
		),
		Confidence: 1.0,
	}
}

// escalateByOne steps up one model tier.
func escalateByOne(current string) string {
	switch current {
	case "haiku":
		return "sonnet"
	case "sonnet":
		return "opus"
	default:
		return "opus"
	}
}

// effortForModel returns effort level for given model.
func effortForModel(model string) string {
	switch model {
	case "haiku":
		return "low"
	case "sonnet":
		return "medium"
	case "opus":
		return "high"
	default:
		return "medium"
	}
}

// ShouldDeEscalate checks if successful completion enables downgrading.
func (fh *FrustrationHandler) ShouldDeEscalate(
	sentiment Score,
	taskWasSuccess bool,
	currentModel string,
	budgetPercentageUsed float64,
) *Decision {

	// Must be successful and have positive sentiment
	if !taskWasSuccess || sentiment.Primary != SentimentSatisfied {
		return nil
	}

	// Already on Haiku? Can't downgrade further
	if currentModel == "haiku" {
		return nil
	}

	// If budget usage is healthy, offer downgrade
	if budgetPercentageUsed < 0.80 {
		nextModel := deEscalateByOne(currentModel)
		return &Decision{
			Action:     "de-escalate",
			NextModel:  nextModel,
			NextEffort: effortForModel(nextModel),
			Rationale: fmt.Sprintf(
				"Task successful on %s. Problem solved. Budget healthy (%.0f%%). Downgrading to %s to save tokens.",
				currentModel, budgetPercentageUsed*100, nextModel,
			),
			Confidence: 0.95,
		}
	}

	// Budget is tight: downgrade to save money
	nextModel := deEscalateByOne(currentModel)
	return &Decision{
		Action:     "de-escalate",
		NextModel:  nextModel,
		NextEffort: effortForModel(nextModel),
		Rationale: fmt.Sprintf(
			"Task successful. Budget approaching limit (%.0f%%). Downgrading to %s for cost savings.",
			budgetPercentageUsed*100, nextModel,
		),
		Confidence: 0.90,
	}
}

// deEscalateByOne steps down one model tier.
func deEscalateByOne(current string) string {
	switch current {
	case "opus":
		return "sonnet"
	case "sonnet":
		return "haiku"
	default:
		return "haiku"
	}
}

// GetDecisionString returns human-readable decision string.
func (d *Decision) String() string {
	if d == nil {
		return "No action needed"
	}
	return fmt.Sprintf("%s to %s: %s (confidence: %.0f%%)",
		d.Action, d.NextModel, d.Rationale, d.Confidence*100)
}

// ShouldShowWarning checks if user should be warned.
func (d *Decision) ShouldShowWarning() bool {
	return d != nil && d.Warning != ""
}
