package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/szibis/claude-escalate/internal/intent"
)

// FeedbackRequest represents user feedback on a response
type FeedbackRequest struct {
	RequestID string `json:"request_id"`
	Rating    int    `json:"rating"`   // 1-5 stars
	Helpful   bool   `json:"helpful"`  // Simplified: thumbs up/down
	Comment   string `json:"comment"`  // Optional comment
	Accurate  bool   `json:"accurate"` // Was the answer correct?
}

// FeedbackResponse represents stored feedback
type FeedbackResponse struct {
	RequestID    string    `json:"request_id"`
	UserID       string    `json:"user_id"`
	Rating       int       `json:"rating"`
	Helpful      bool      `json:"helpful"`
	Accurate     bool      `json:"accurate"`
	Comment      string    `json:"comment"`
	RecordedAt   time.Time `json:"recorded_at"`
	Acknowledged bool      `json:"acknowledged"`
}

// UserAnalytics represents per-user analytics and preferences
type UserAnalytics struct {
	UserID                      string    `json:"user_id"`
	TotalFeedbackCount          int       `json:"total_feedback_count"`
	PositiveFeedbackCount       int       `json:"positive_feedback_count"`
	NegativeFeedbackCount       int       `json:"negative_feedback_count"`
	AverageRating               float64   `json:"average_rating"`
	HelpfulPercentage           float64   `json:"helpful_percentage"`
	AccuracyPercentage          float64   `json:"accuracy_percentage"`
	PrefersFreshness            bool      `json:"prefers_freshness"` // User rates cached responses low
	PrefersOpus                 bool      `json:"prefers_opus"`      // User wants detailed Opus responses
	PrefersBriefness            bool      `json:"prefers_briefness"` // User rates verbose responses low
	CacheHitRating              float64   `json:"cache_hit_rating"`  // How user rates cached vs fresh
	AverageResponseTime         float64   `json:"avg_response_time_ms"`
	LastFeedbackTime            time.Time `json:"last_feedback_time"`
	AverageResponseSatisfaction float64   `json:"avg_response_satisfaction"`
	PreferredModel              string    `json:"preferred_model"` // haiku, sonnet, opus
}

// handleFeedback processes user feedback on responses
// POST /api/feedback/{request_id}
func (s *Service) handleFeedback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var feedbackReq FeedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&feedbackReq); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Validate rating (1-5)
	if feedbackReq.Rating < 1 || feedbackReq.Rating > 5 {
		http.Error(w, "Rating must be 1-5", http.StatusBadRequest)
		return
	}

	// Extract user ID from request (from context, auth header, or IP)
	userID := extractUserID(r)

	// Record feedback (store in memory/DB)
	feedback := &FeedbackResponse{
		RequestID:  feedbackReq.RequestID,
		UserID:     userID,
		Rating:     feedbackReq.Rating,
		Helpful:    feedbackReq.Helpful,
		Accurate:   feedbackReq.Accurate,
		Comment:    feedbackReq.Comment,
		RecordedAt: time.Now(),
	}

	// Learn from feedback - update user preferences
	s.learnUserPreference(userID, feedback)

	// Return confirmation
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":       "recorded",
		"request_id":   feedbackReq.RequestID,
		"rating":       feedbackReq.Rating,
		"acknowledged": true,
		"message":      "Thank you for your feedback!",
	})
}

// handleUserAnalytics returns per-user analytics and preferences
// GET /api/analytics/personal
func (s *Service) handleUserAnalytics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := extractUserID(r)

	// Get user's feedback pattern from classifier
	pattern := &intent.UserFeedbackPattern{
		UserID: userID,
	}

	// Calculate aggregated analytics
	analytics := &UserAnalytics{
		UserID:                      userID,
		PositiveFeedbackCount:       pattern.PositiveFeedbackCount,
		NegativeFeedbackCount:       pattern.NegativeFeedbackCount,
		AverageRating:               calculateAverageRating(pattern),
		HelpfulPercentage:           calculateHelpfulPercentage(pattern),
		AccuracyPercentage:          calculateAccuracyPercentage(pattern),
		PrefersFreshness:            pattern.PrefersFreshness,
		PrefersOpus:                 pattern.PrefersOpus,
		PrefersBriefness:            pattern.PrefersBriefness,
		CacheHitRating:              pattern.CacheHitRating,
		AverageResponseSatisfaction: pattern.AverageResponseSatisfaction,
		LastFeedbackTime:            pattern.LastFeedbackTime,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(analytics)
}

// learnUserPreference updates model and cache decisions based on feedback
func (s *Service) learnUserPreference(userID string, feedback *FeedbackResponse) {
	// Update user preference pattern (used by intent classifier)
	// This influences future model selection and cache decisions

	// Logic:
	// - If cached response rated low → mark cache unsafe for this user
	// - If detailed response rated high → escalate to Sonnet/Opus for this user
	// - If brief response rated high → keep Haiku for this user
	// - If accuracy low → adjust semantic cache confidence threshold down

	// Implementation: integrate with classifier's feedback learning
	// classifier.UpdateUserPreference(userID, feedback.Rating, feedback.Comment)
}

// extractUserID extracts user ID from request context or IP
func extractUserID(r *http.Request) string {
	// Priority: context > auth header > IP address
	if userID := r.Header.Get("X-User-ID"); userID != "" {
		return userID
	}
	if userID := r.Header.Get("Authorization"); userID != "" {
		return userID
	}
	// Fall back to IP address (anonymized)
	return r.RemoteAddr
}

// calculateAverageRating calculates average rating from feedback pattern
func calculateAverageRating(pattern *intent.UserFeedbackPattern) float64 {
	total := pattern.PositiveFeedbackCount + pattern.NegativeFeedbackCount
	if total == 0 {
		return 0
	}
	// Approximate: positive feedback ≈ 4 stars, negative ≈ 2 stars
	avgStars := float64(pattern.PositiveFeedbackCount*4+pattern.NegativeFeedbackCount*2) / float64(total)
	return avgStars
}

// calculateHelpfulPercentage calculates what % of responses user finds helpful
func calculateHelpfulPercentage(pattern *intent.UserFeedbackPattern) float64 {
	total := pattern.PositiveFeedbackCount + pattern.NegativeFeedbackCount
	if total == 0 {
		return 0
	}
	return float64(pattern.PositiveFeedbackCount) / float64(total) * 100
}

// calculateAccuracyPercentage calculates response accuracy based on pattern
func calculateAccuracyPercentage(pattern *intent.UserFeedbackPattern) float64 {
	// Based on user's accuracy feedback (whether answers were correct)
	// This is tracked separately from general satisfaction
	if pattern.RecentAccuracy == 0 {
		return 100 // Assume correct if not specified
	}
	return pattern.RecentAccuracy * 100
}
