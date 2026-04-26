package intent

import (
	"context"
	"strings"
	"time"
)

// IntentType represents the classified intent
type IntentType string

const (
	IntentQuickAnswer       IntentType = "quick_answer"
	IntentDetailedAnalysis  IntentType = "detailed_analysis"
	IntentRoutine           IntentType = "routine"
	IntentLearning          IntentType = "learning"
	IntentFollowUp          IntentType = "follow_up"
	IntentCacheBypass       IntentType = "cache_bypass"
)

// EffortLevel represents the effort required
type EffortLevel string

const (
	EffortLow    EffortLevel = "low"
	EffortMedium EffortLevel = "medium"
	EffortHigh   EffortLevel = "high"
)

// ModelType represents the recommended model
type ModelType string

const (
	ModelHaiku  ModelType = "haiku"
	ModelSonnet ModelType = "sonnet"
	ModelOpus   ModelType = "opus"
)

// IntentDecision represents the full decision for a query
type IntentDecision struct {
	Intent              IntentType
	CacheSafe           bool
	RecommendedModel    ModelType
	MaxTokens           int
	OptimizeMode        OptimizeLevel
	Confidence          float64
	EffortLevel         EffortLevel
	FeedbackBonus       float64
	Explanation         string
	Timestamp           time.Time
}

// OptimizeLevel represents the optimization aggressiveness
type OptimizeLevel string

const (
	OptimizeLevelAggressive OptimizeLevel = "aggressive"
	OptimizeLevelModerate   OptimizeLevel = "moderate"
	OptimizeLevelMinimal    OptimizeLevel = "minimal"
)

// Classifier classifies query intent and makes coupling decisions
type Classifier struct {
	bypassPatterns    map[string]bool
	detailKeywords    []string
	quickKeywords     []string
	followUpKeywords  []string
	learningKeywords  []string
	userFeedback      map[string]*UserFeedbackPattern
	feedbackHistoryDays int
}

// NewClassifier creates a new intent classifier
func NewClassifier(feedbackHistoryDays int) *Classifier {
	classifier := &Classifier{
		bypassPatterns:     makeBypassPatterns(),
		detailKeywords:     []string{"detailed", "comprehensive", "explain", "why", "how", "deep", "thorough", "analysis"},
		quickKeywords:      []string{"quick", "brief", "summary", "tl;dr", "just", "simply", "shortly"},
		followUpKeywords:   []string{"more", "additional", "also", "what about", "furthermore", "moreover"},
		learningKeywords:   []string{"what if", "try", "experiment", "compare", "explore", "alternative"},
		userFeedback:       make(map[string]*UserFeedbackPattern),
		feedbackHistoryDays: feedbackHistoryDays,
	}
	return classifier
}

// Classify classifies a query and returns the decision
func (c *Classifier) Classify(ctx context.Context, query string, userID string, context *QueryContext) *IntentDecision {
	// Check for explicit cache bypass first (HIGHEST PRIORITY)
	if c.hasExplicitBypass(query) {
		return &IntentDecision{
			Intent:           IntentCacheBypass,
			CacheSafe:        false,
			RecommendedModel: ModelSonnet,
			MaxTokens:        2000,
			OptimizeMode:     OptimizeLevelMinimal,
			Confidence:       1.0,
			EffortLevel:      EffortHigh,
			Explanation:      "User explicitly requested cache bypass (--no-cache, --fresh, !, etc)",
			Timestamp:        time.Now(),
		}
	}

	// Detect base intent from keywords and sentiment
	baseIntent := c.detectBaseIntent(query)
	effortLevel := c.intentToEffort(baseIntent)

	// Get user feedback history to modulate decision
	feedback := c.getUserFeedback(userID)

	// Apply modulation based on user history
	confidence := c.calculateConfidence(baseIntent, feedback, query)
	finalIntent := c.applyFeedbackModulation(baseIntent, feedback, query)
	finalEffort := c.intentToEffort(finalIntent)

	// Generate decision coupling intent + model + cache
	decision := &IntentDecision{
		Intent:           finalIntent,
		CacheSafe:        c.isCacheSafe(finalIntent),
		RecommendedModel: c.effortToModel(finalEffort, feedback),
		MaxTokens:        c.maxTokensForIntent(finalIntent),
		OptimizeMode:     c.optimizeModeForIntent(finalIntent),
		EffortLevel:      finalEffort,
		Confidence:       confidence,
		FeedbackBonus:    feedback.RecentAccuracy,
		Explanation:      c.explainDecision(baseIntent, finalIntent, feedback),
		Timestamp:        time.Now(),
	}

	return decision
}

// hasExplicitBypass checks for explicit cache bypass patterns
func (c *Classifier) hasExplicitBypass(query string) bool {
	lowerQuery := strings.ToLower(query)

	for pattern := range c.bypassPatterns {
		if strings.Contains(lowerQuery, pattern) {
			return true
		}
	}

	return false
}

// detectBaseIntent detects intent from keywords
func (c *Classifier) detectBaseIntent(query string) IntentType {
	lowerQuery := strings.ToLower(query)

	// Check for detailed request
	for _, keyword := range c.detailKeywords {
		if strings.Contains(lowerQuery, keyword) {
			return IntentDetailedAnalysis
		}
	}

	// Check for quick request
	for _, keyword := range c.quickKeywords {
		if strings.Contains(lowerQuery, keyword) {
			return IntentQuickAnswer
		}
	}

	// Check for follow-up
	for _, keyword := range c.followUpKeywords {
		if strings.Contains(lowerQuery, keyword) {
			return IntentFollowUp
		}
	}

	// Check for learning/exploration
	for _, keyword := range c.learningKeywords {
		if strings.Contains(lowerQuery, keyword) {
			return IntentLearning
		}
	}

	// Default to quick answer
	return IntentQuickAnswer
}

// intentToEffort maps intent to effort level
func (c *Classifier) intentToEffort(intent IntentType) EffortLevel {
	switch intent {
	case IntentQuickAnswer:
		return EffortLow
	case IntentRoutine:
		return EffortLow
	case IntentDetailedAnalysis:
		return EffortHigh
	case IntentLearning:
		return EffortHigh
	case IntentFollowUp:
		return EffortMedium
	case IntentCacheBypass:
		return EffortHigh
	default:
		return EffortMedium
	}
}

// effortToModel maps effort level to model recommendation
func (c *Classifier) effortToModel(effort EffortLevel, feedback *UserFeedbackPattern) ModelType {
	switch effort {
	case EffortLow:
		return ModelHaiku
	case EffortMedium:
		return ModelSonnet
	case EffortHigh:
		// Check if user consistently wants Opus
		if feedback.PrefersOpus {
			return ModelOpus
		}
		return ModelSonnet
	default:
		return ModelSonnet
	}
}

// isCacheSafe determines if caching is safe for this intent
func (c *Classifier) isCacheSafe(intent IntentType) bool {
	switch intent {
	case IntentQuickAnswer:
		return true
	case IntentRoutine:
		return true
	case IntentDetailedAnalysis:
		return false
	case IntentLearning:
		return false
	case IntentFollowUp:
		return false
	case IntentCacheBypass:
		return false
	default:
		return false
	}
}

// calculateConfidence calculates confidence in the classification
func (c *Classifier) calculateConfidence(intent IntentType, feedback *UserFeedbackPattern, query string) float64 {
	baseConfidence := 0.7

	// Adjust based on keyword matches
	keywordCount := c.countKeywordMatches(query)
	if keywordCount >= 2 {
		baseConfidence += 0.2
	}

	// Adjust based on feedback history
	if feedback.RecentAccuracy > 0.8 {
		baseConfidence += 0.1
	}

	// Cap at 0.99
	if baseConfidence > 0.99 {
		baseConfidence = 0.99
	}

	return baseConfidence
}

// countKeywordMatches counts how many intent keywords appear in query
func (c *Classifier) countKeywordMatches(query string) int {
	lowerQuery := strings.ToLower(query)
	count := 0

	allKeywords := append(append(append(append(c.detailKeywords, c.quickKeywords...), c.followUpKeywords...), c.learningKeywords...)...)

	for _, keyword := range allKeywords {
		if strings.Contains(lowerQuery, keyword) {
			count++
		}
	}

	return count
}

// applyFeedbackModulation applies user feedback to adjust intent
func (c *Classifier) applyFeedbackModulation(intent IntentType, feedback *UserFeedbackPattern, query string) IntentType {
	// If user consistently provides negative feedback on cached responses for this type of query,
	// escalate intent to prevent caching
	if feedback.PrefersFreshness && strings.Contains(query, "summary") {
		return IntentDetailedAnalysis
	}

	return intent
}

// maxTokensForIntent returns max tokens based on intent
func (c *Classifier) maxTokensForIntent(intent IntentType) int {
	switch intent {
	case IntentQuickAnswer:
		return 256
	case IntentRoutine:
		return 256
	case IntentDetailedAnalysis:
		return 2000
	case IntentLearning:
		return 1024
	case IntentFollowUp:
		return 512
	case IntentCacheBypass:
		return 2000
	default:
		return 512
	}
}

// optimizeModeForIntent returns optimization level
func (c *Classifier) optimizeModeForIntent(intent IntentType) OptimizeLevel {
	switch intent {
	case IntentQuickAnswer:
		return OptimizeLevelAggressive
	case IntentRoutine:
		return OptimizeLevelAggressive
	case IntentDetailedAnalysis:
		return OptimizeLevelMinimal
	case IntentLearning:
		return OptimizeLevelMinimal
	case IntentFollowUp:
		return OptimizeLevelModerate
	case IntentCacheBypass:
		return OptimizeLevelMinimal
	default:
		return OptimizeLevelModerate
	}
}

// explainDecision provides explanation for the decision
func (c *Classifier) explainDecision(baseIntent, finalIntent IntentType, feedback *UserFeedbackPattern) string {
	if baseIntent == finalIntent {
		return "Intent detected: " + string(finalIntent)
	}

	return "Intent detected: " + string(baseIntent) + " → adjusted to " + string(finalIntent) + " based on user history"
}

// getUserFeedback retrieves user feedback pattern
func (c *Classifier) getUserFeedback(userID string) *UserFeedbackPattern {
	if feedback, exists := c.userFeedback[userID]; exists {
		return feedback
	}

	// Default feedback pattern
	return &UserFeedbackPattern{
		RecentAccuracy:   1.0,
		PrefersFreshness: false,
		PrefersOpus:      false,
	}
}

// RecordFeedback records user feedback for learning
func (c *Classifier) RecordFeedback(userID string, decision *IntentDecision, rating string) {
	if _, exists := c.userFeedback[userID]; !exists {
		c.userFeedback[userID] = &UserFeedbackPattern{
			RecentAccuracy: 1.0,
		}
	}

	feedback := c.userFeedback[userID]

	// Update feedback based on rating
	switch rating {
	case "excellent", "perfect":
		feedback.RecentAccuracy = 0.95
		feedback.PositiveFeedbackCount++
	case "good":
		feedback.RecentAccuracy = 0.8
		feedback.PositiveFeedbackCount++
	case "okay":
		feedback.RecentAccuracy = 0.6
	case "poor":
		feedback.RecentAccuracy = 0.2
		feedback.NegativeFeedbackCount++
		// If user gives negative feedback on cached responses, mark preference for freshness
		if !decision.CacheSafe {
			feedback.PrefersFreshness = true
		}
	}

	feedback.LastFeedbackTime = time.Now()
}

// makeBypassPatterns creates the cache bypass patterns
func makeBypassPatterns() map[string]bool {
	return map[string]bool{
		"--no-cache":  true,
		"--fresh":     true,
		"!":           true,
		"(no cache)":  true,
		"(bypass)":    true,
		"(fresh)":     true,
	}
}
