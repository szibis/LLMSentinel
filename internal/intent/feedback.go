package intent

import (
	"time"
)

// UserFeedbackPattern tracks user feedback and preferences
type UserFeedbackPattern struct {
	UserID                   string
	PositiveFeedbackCount    int
	NegativeFeedbackCount    int
	RecentAccuracy           float64  // 0.0-1.0
	PrefersFreshness         bool     // True if user marks cached responses negatively
	PrefersOpus              bool     // True if user often requests detailed analysis
	PrefersBriefness         bool     // True if user marks verbose responses negatively
	CacheHitRating           float64  // How user rates cached hits vs fresh
	LastFeedbackTime         time.Time
	AverageResponseSatisfaction float64
}

// SentimentAnalyzer analyzes query sentiment
type SentimentAnalyzer struct {
	urgencyKeywords  []string
	urgencyIndicators map[string]float64
}

// NewSentimentAnalyzer creates a new sentiment analyzer
func NewSentimentAnalyzer() *SentimentAnalyzer {
	return &SentimentAnalyzer{
		urgencyKeywords: []string{
			"urgent", "asap", "quickly", "now", "immediately", "fast",
			"slow", "stuck", "blocked", "breaking", "critical",
		},
		urgencyIndicators: map[string]float64{
			"!":    0.2,  // Single exclamation
			"!!":   0.5,  // Double exclamation
			"!!!":  0.8,  // Triple or more
			"????": 0.3,  // Multiple questions
		},
	}
}

// AnalyzeSentiment analyzes query sentiment and returns urgency score
func (sa *SentimentAnalyzer) AnalyzeSentiment(query string) *SentimentResult {
	result := &SentimentResult{
		Timestamp: time.Now(),
	}

	// Check urgency keywords
	for _, keyword := range sa.urgencyKeywords {
		if contains(query, keyword) {
			result.UrgencyScore += 0.15
			result.Urgent = true
		}
	}

	// Check urgency indicators
	for indicator, score := range sa.urgencyIndicators {
		if contains(query, indicator) {
			result.UrgencyScore += score
		}
	}

	// Cap urgency score at 1.0
	if result.UrgencyScore > 1.0 {
		result.UrgencyScore = 1.0
	}

	// Determine complexity based on query length and punctuation
	result.ComplexityScore = calculateComplexity(query)

	// Determine tone
	result.Tone = determineTone(query)

	return result
}

// SentimentResult represents sentiment analysis result
type SentimentResult struct {
	UrgencyScore    float64  // 0.0-1.0
	ComplexityScore float64  // 0.0-1.0
	Tone            string   // "neutral", "polite", "casual", "urgent", "frustrated"
	Urgent          bool
	Timestamp       time.Time
}

// QueryContext provides context for intent classification
type QueryContext struct {
	PreviousQuery   string
	PreviousIntent  IntentType
	PreviousModel   ModelType
	IsCachedHit     bool
	UserSessionID   string
	QueryHistory    []QueryRecord
}

// QueryRecord represents a single query record
type QueryRecord struct {
	Query      string
	Intent     IntentType
	Model      ModelType
	Cached     bool
	Satisfaction float64 // User rating 0-1
	Timestamp  time.Time
}

// Helper functions

func contains(str, substr string) bool {
	// Case-insensitive contains
	lowerStr := toLower(str)
	lowerSubstr := toLower(substr)
	return indexOf(lowerStr, lowerSubstr) >= 0
}

func toLower(s string) string {
	result := make([]byte, len(s))
	for i, b := range []byte(s) {
		if b >= 'A' && b <= 'Z' {
			result[i] = b + 32
		} else {
			result[i] = b
		}
	}
	return string(result)
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func calculateComplexity(query string) float64 {
	// Base complexity on query length
	length := len(query)
	complexity := float64(length) / 1000.0 // Normalized to 1.0 at 1000 chars

	if complexity > 1.0 {
		complexity = 1.0
	}

	// Increase complexity if contains special characters
	specialChars := 0
	for _, c := range query {
		if !isAlphanumeric(c) {
			specialChars++
		}
	}

	specialCharRatio := float64(specialChars) / float64(len(query))
	if specialCharRatio > 0.2 {
		complexity += 0.2
	}

	if complexity > 1.0 {
		complexity = 1.0
	}

	return complexity
}

func isAlphanumeric(c rune) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == ' '
}

func determineTone(query string) string {
	lowerQuery := toLower(query)

	// Check for polite indicators
	if indexOf(lowerQuery, "please") >= 0 || indexOf(lowerQuery, "thanks") >= 0 || indexOf(lowerQuery, "could you") >= 0 {
		return "polite"
	}

	// Check for casual indicators
	if indexOf(lowerQuery, "hey") >= 0 || indexOf(lowerQuery, "btw") >= 0 || indexOf(lowerQuery, "lol") >= 0 {
		return "casual"
	}

	// Check for urgent indicators
	if indexOf(lowerQuery, "urgent") >= 0 || indexOf(lowerQuery, "asap") >= 0 || indexOf(lowerQuery, "critical") >= 0 {
		return "urgent"
	}

	// Check for frustrated indicators
	if indexOf(lowerQuery, "frustrated") >= 0 || indexOf(lowerQuery, "stuck") >= 0 || indexOf(lowerQuery, "can't") >= 0 {
		return "frustrated"
	}

	return "neutral"
}
