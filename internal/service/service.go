// Package service provides HTTP server for escalation management.
package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/szibis/claude-escalate/internal/budgets"
	"github.com/szibis/claude-escalate/internal/config"
	"github.com/szibis/claude-escalate/internal/decisions"
	"github.com/szibis/claude-escalate/internal/sentiment"
	"github.com/szibis/claude-escalate/internal/signals"
	"github.com/szibis/claude-escalate/internal/store"
)

// Service handles all escalation logic via HTTP.
type Service struct {
	db                *store.Store
	cfg               *config.Config
	escCfg            *config.EscalationConfig
	sentimentDetector *sentiment.Detector
	budgetEngine      *budgets.Engine
}

// New creates a new service instance.
func New(cfg *config.Config) (*Service, error) {
	db, err := store.Open(cfg.DataDir)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Load escalation configuration
	escCfg, err := config.LoadEscalationConfig()
	if err != nil {
		fmt.Printf("Warning: failed to load escalation config, using defaults: %v\n", err)
		escCfg = config.DefaultEscalationConfig()
	}

	// Initialize sentiment detector
	sentimentDetector := sentiment.NewDetector()

	// Initialize budget engine
	budgetCfg := budgets.BudgetConfig{
		DailyBudgetUSD:         escCfg.Budgets.DailyUSD,
		MonthlyBudgetUSD:       escCfg.Budgets.MonthlyUSD,
		SessionBudgetTokens:    escCfg.Budgets.SessionTokens,
		ModelDailyLimits:       escCfg.Budgets.ModelDailyLimits,
		TaskTypeBudgets:        escCfg.Budgets.TaskTypeBudgets,
		HardLimit:              escCfg.Budgets.HardLimit,
		SoftLimit:              escCfg.Budgets.SoftLimit,
		AutoDowngradeAtPercent: escCfg.Budgets.AutoDowngradeAt,
		AlertThresholds:        escCfg.Budgets.AlertThresholds,
	}
	budgetEngine := budgets.NewEngine(budgetCfg)

	return &Service{
		db:                db,
		cfg:               cfg,
		escCfg:            escCfg,
		sentimentDetector: sentimentDetector,
		budgetEngine:      budgetEngine,
	}, nil
}

// Start begins the HTTP server.
func (s *Service) Start(addr string) error {
	mux := http.NewServeMux()

	// Hook endpoint: receives prompts and handles escalation logic
	mux.HandleFunc("/api/hook", s.handleHook)
	mux.HandleFunc("/api/escalate", s.handleEscalate)
	mux.HandleFunc("/api/deescalate", s.handleDeescalate)
	mux.HandleFunc("/api/effort", s.handleEffort)
	mux.HandleFunc("/api/stats", s.handleStats)
	mux.HandleFunc("/api/health", s.handleHealth)

	// Validation endpoints
	mux.HandleFunc("/api/validate", s.handleValidate)
	mux.HandleFunc("/api/validation/metrics", s.handleValidationMetrics)
	mux.HandleFunc("/api/validation/stats", s.handleValidationStats)
	mux.HandleFunc("/api/metrics/hook", s.handleHookMetrics)

	// Signal detection and decision-making endpoints
	mux.HandleFunc("/api/signals/detect", s.handleDetectSignal)
	mux.HandleFunc("/api/decisions/make", s.handleMakeDecision)
	mux.HandleFunc("/api/decisions/learning", s.handleDecisionLearning)

	// Statusline integration endpoint (for barista, plugins, etc.)
	mux.HandleFunc("/api/statusline", s.handleStatusline)

	// Analytics endpoints (3-phase data) - TODO: implement with BoltDB instead of SQL
	// mux.HandleFunc("/api/analytics/phase-1", s.handleAnalyticsPhase1)
	// mux.HandleFunc("/api/analytics/phase-2", s.handleAnalyticsPhase2)
	// mux.HandleFunc("/api/analytics/phase-3", s.handleAnalyticsPhase3)
	// mux.HandleFunc("/api/analytics/sentiment-trends", s.handleSentimentTrends)
	// mux.HandleFunc("/api/analytics/budget-status", s.handleBudgetStatus)
	// mux.HandleFunc("/api/analytics/model-satisfaction", s.handleModelSatisfaction)
	// mux.HandleFunc("/api/analytics/frustration-events", s.handleFrustrationEvents)
	// mux.HandleFunc("/api/analytics/cost-optimization", s.handleCostOptimization)

	// Dashboard endpoints
	mux.HandleFunc("/", s.handleDashboard)

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	fmt.Printf("claude-escalate service starting on http://%s\n", addr)
	fmt.Printf("  - Hook endpoint: POST /api/hook\n")
	fmt.Printf("  - Analytics (3-phase): GET /api/analytics/phase-{1,2,3}\n")
	fmt.Printf("  - Analytics (trends): GET /api/analytics/{sentiment-trends,budget-status,model-satisfaction,frustration-events,cost-optimization}\n")
	fmt.Printf("  - Dashboard: GET /\n")
	fmt.Printf("  - Stats: GET /api/stats\n")

	return srv.ListenAndServe()
}

// HookRequest is the structure for hook calls.
type HookRequest struct {
	Prompt string `json:"prompt"`
}

// HookResponse is returned after processing a hook.
type HookResponse struct {
	Continue       bool   `json:"continue"`
	SuppressOutput bool   `json:"suppressOutput"`
	Action         string `json:"action,omitempty"`
	Message        string `json:"message,omitempty"`
	CurrentModel   string `json:"currentModel,omitempty"`
}

// handleHook processes user prompts for /escalate commands, success signals, and task detection.
func (s *Service) handleHook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req HookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	prompt := req.Prompt
	response := HookResponse{Continue: true, SuppressOutput: true}

	// Phase 1: Sentiment Detection (if enabled)
	if s.escCfg.Sentiment.Enabled {
		sentimentScore := s.sentimentDetector.Detect(prompt, false, 0)

		// If high frustration risk and enabled, offer escalation
		if sentimentScore.FrustrationRisk > s.escCfg.Sentiment.FrustrationRiskThreshold &&
			s.escCfg.Sentiment.FrustrationTriggerEscalate {
			settings, _ := config.ReadClaudeSettings()
			currentModel := "haiku"
			if settings != nil {
				currentModel = modelShortName(settings.Model)
			}

			// Auto-escalate on frustration
			nextModel := escalateByOne(currentModel)
			response.Action = "escalate_on_frustration"
			response.CurrentModel = nextModel
			if err := s.db.LogEscalation(currentModel, nextModel, "auto", "frustration_detected"); err != nil {
				fmt.Printf("error logging frustration escalation: %v\n", err)
			}
			if err := updateClaudeSettings(modelToFull(nextModel)); err != nil {
				fmt.Printf("error updating settings: %v\n", err)
			}
		}
	}

	// Phase 1: Budget Check (if enabled)
	if s.escCfg.Budgets.DailyUSD > 0 {
		settings, _ := config.ReadClaudeSettings()
		currentModel := "haiku"
		if settings != nil {
			currentModel = modelShortName(settings.Model)
		}

		// Estimate cost for this request (rough estimate)
		estimatedCost := 0.01 // placeholder

		budgetCheck := s.budgetEngine.CheckBudget(currentModel, estimatedCost, "")

		if !budgetCheck.IsAllowed && budgetCheck.RecommendedModel != "" {
			response.Action = "downgrade_for_budget"
			response.CurrentModel = budgetCheck.RecommendedModel
			response.Message = budgetCheck.Message
			if err := updateClaudeSettings(modelToFull(budgetCheck.RecommendedModel)); err != nil {
				fmt.Printf("error updating settings: %v\n", err)
			}
		}
	}

	// Check for /escalate commands
	if isEscalateCommand(prompt) {
		target := extractEscalateTarget(prompt)
		response.Action = "escalate"
		response.CurrentModel = target
		// Log escalation to database
		if err := s.db.LogEscalation("haiku", target, "manual", "user_command"); err != nil {
			fmt.Printf("error logging escalation: %v\n", err)
		}
		if err := updateClaudeSettings(target); err != nil {
			fmt.Printf("error updating settings: %v\n", err)
		}
	}

	// Check for success signals (de-escalation triggers)
	if hasSuccessSignal(prompt) {
		response.Action = "deescalate"
		// Get current model and cascade down
		settings, _ := config.ReadClaudeSettings()
		if settings != nil && settings.Model != "" {
			nextModel := cascadeDown(settings.Model)
			response.CurrentModel = nextModel
			if err := s.db.LogEscalation(settings.Model, nextModel, "success", "success_signal"); err != nil {
				fmt.Printf("error logging deescalation: %v\n", err)
			}
			if err := updateClaudeSettings(nextModel); err != nil {
				fmt.Printf("error updating settings: %v\n", err)
			}
		}
	}

	// Auto-effort detection
	effort := detectEffort(prompt)
	if effort != "" {
		model := effortToModel(effort)
		response.CurrentModel = model
		if err := s.db.LogEscalation("haiku", model, "auto", effort); err != nil {
			fmt.Printf("error logging effort detection: %v\n", err)
		}
		if err := updateClaudeSettings(model); err != nil {
			fmt.Printf("error updating settings: %v\n", err)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		fmt.Printf("error encoding response: %v\n", err)
	}
}

// handleEscalate manually escalates to a target model.
func (s *Service) handleEscalate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Target string `json:"target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	target := req.Target
	if target == "" {
		target = "sonnet"
	}

	settings, _ := config.ReadClaudeSettings()
	from := "haiku"
	if settings != nil && settings.Model != "" {
		from = modelShortName(settings.Model)
	}

	if err := s.db.LogEscalation(from, target, "manual", "user_command"); err != nil {
		fmt.Printf("error logging escalation: %v\n", err)
	}
	if err := updateClaudeSettings(modelToFull(target)); err != nil {
		fmt.Printf("error updating settings: %v\n", err)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"model":     target,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		fmt.Printf("error encoding response: %v\n", err)
	}
}

// handleDeescalate cascades down one model tier.
func (s *Service) handleDeescalate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	settings, _ := config.ReadClaudeSettings()
	if settings == nil || settings.Model == "" {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]interface{}{"error": "no current model"}); err != nil {
			fmt.Printf("error encoding response: %v\n", err)
		}
		return
	}

	from := settings.Model
	to := cascadeDown(from)

	if err := s.db.LogEscalation(modelShortName(from), to, "cascade", req.Reason); err != nil {
		fmt.Printf("error logging deescalation: %v\n", err)
	}
	if err := updateClaudeSettings(modelToFull(to)); err != nil {
		fmt.Printf("error updating settings: %v\n", err)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"model":     to,
		"cascaded":  true,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		fmt.Printf("error encoding response: %v\n", err)
	}
}

// handleEffort sets effort level and routes to appropriate model.
func (s *Service) handleEffort(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Level string `json:"level"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	model := effortToModel(req.Level)
	if err := updateClaudeSettings(modelToFull(model)); err != nil {
		fmt.Printf("error updating settings: %v\n", err)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"level":   req.Level,
		"model":   model,
	}); err != nil {
		fmt.Printf("error encoding response: %v\n", err)
	}
}

// handleStats returns current statistics.
func (s *Service) handleStats(w http.ResponseWriter, r *http.Request) {
	esc, deesc, turns, _ := s.db.TotalStats()
	settings, _ := config.ReadClaudeSettings()
	currentModel := "unknown"
	if settings != nil {
		currentModel = modelShortName(settings.Model)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"escalations":    esc,
		"de_escalations": deesc,
		"turns":          turns,
		"current_model":  currentModel,
		"timestamp":      time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		fmt.Printf("error encoding response: %v\n", err)
	}
}

// handleHealth returns service health status.
func (s *Service) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{
		"status":    "healthy",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		fmt.Printf("error encoding response: %v\n", err)
	}
}

// handleDashboard serves the dashboard UI.
func (s *Service) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	// Dashboard HTML would be served here (same as dashboard package)
	if _, err := w.Write([]byte(dashboardHTML)); err != nil {
		fmt.Printf("error writing response: %v\n", err)
	}
}

// handleValidate accepts actual token metrics and compares with estimates.
func (s *Service) handleValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ActualInputTokens         int     `json:"actual_input_tokens"`
		ActualCacheCreationTokens int     `json:"actual_cache_creation_tokens"`
		ActualCacheReadTokens     int     `json:"actual_cache_read_tokens"`
		ActualOutputTokens        int     `json:"actual_output_tokens"`
		ActualTotalTokens         int     `json:"actual_total_tokens"`
		ActualCost                float64 `json:"actual_cost"`
		ValidationID              int64   `json:"validation_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	// Create validation metric record
	metric := store.ValidationMetric{
		ActualInputTokens:  req.ActualInputTokens,
		ActualOutputTokens: req.ActualOutputTokens,
		ActualTotalTokens:  req.ActualTotalTokens,
		ActualCost:         req.ActualCost,
		Validated:          true,
	}

	if err := s.db.LogValidationMetric(metric); err != nil {
		http.Error(w, "failed to log metric", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"success":         true,
		"validation_id":   metric.ID,
		"tokens_recorded": metric.ActualTotalTokens,
		"timestamp":       time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		fmt.Printf("error encoding response: %v\n", err)
	}
}

// handleValidationMetrics returns recent validation metrics.
func (s *Service) handleValidationMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	metrics, err := s.db.GetValidationMetrics(100)
	if err != nil {
		http.Error(w, "failed to get metrics", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"metrics":   metrics,
		"count":     len(metrics),
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		fmt.Printf("error encoding response: %v\n", err)
	}
}

// handleValidationStats returns validation statistics summary.
func (s *Service) handleValidationStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats, err := s.db.GetValidationStats()
	if err != nil {
		http.Error(w, "failed to get stats", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(stats); err != nil {
		fmt.Printf("error encoding response: %v\n", err)
	}
}

// handleStatusline returns current metrics in statusline-friendly JSON format.
// Any statusline plugin (barista, custom plugins, etc.) can query this endpoint
// to get live escalation and validation statistics for display.
func (s *Service) handleStatusline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get current model from settings
	settings, _ := config.ReadClaudeSettings()
	currentModel := "unknown"
	effortLevel := "medium"
	if settings != nil {
		currentModel = modelShortName(settings.Model)
		effortLevel = settings.EffortLevel
	}

	// Get escalation stats
	esc, deesc, turns, _ := s.db.TotalStats()

	// Get validation stats
	valStats, _ := s.db.GetValidationStats()
	totalValidations := valStats["total_metrics"].(int)
	estimatedTotal := valStats["estimated_total"].(int)
	actualTotal := valStats["actual_total"].(int)
	estimatedCost := valStats["estimated_cost_total"].(float64)
	actualCost := valStats["actual_cost_total"].(float64)
	avgTokenError := valStats["avg_token_error"].(float64)

	// Calculate savings
	tokensSaved := estimatedTotal - actualTotal
	costSaved := estimatedCost - actualCost
	savingsPercent := 0.0
	if estimatedTotal > 0 {
		savingsPercent = float64(tokensSaved) / float64(estimatedTotal) * 100
	}

	// Calculate accuracy percentage
	accuracy := 100.0
	if avgTokenError != 0 {
		// Convert error percentage to accuracy (e.g., -3% error = 103% accuracy)
		accuracy = 100.0 - avgTokenError
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		// Current state
		"model":     currentModel,
		"effort":    effortLevel,
		"timestamp": time.Now().UTC().Format(time.RFC3339),

		// Escalation metrics
		"escalations":    esc,
		"de_escalations": deesc,
		"turns":          turns,

		// Validation metrics
		"validations":     totalValidations,
		"accuracy":        accuracy,
		"avg_token_error": avgTokenError,

		// Token statistics
		"estimated_tokens": estimatedTotal,
		"actual_tokens":    actualTotal,
		"tokens_saved":     tokensSaved,
		"savings_percent":  savingsPercent,

		// Cost statistics
		"estimated_cost": estimatedCost,
		"actual_cost":    actualCost,
		"cost_saved":     costSaved,

		// Integration info
		"service": "escalation-manager",
		"version": "2.0",
	}); err != nil {
		fmt.Printf("error encoding response: %v\n", err)
	}
}

// handleHookMetrics records estimated metrics from the hook (pre-response).
// This creates the "estimate side" of the validation pair.
func (s *Service) handleHookMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Prompt                string  `json:"prompt"`
		DetectedTaskType      string  `json:"detected_task_type"`
		DetectedEffort        string  `json:"detected_effort"`
		RoutedModel           string  `json:"routed_model"`
		EstimatedInputTokens  int     `json:"estimated_input_tokens"`
		EstimatedOutputTokens int     `json:"estimated_output_tokens"`
		EstimatedTotalTokens  int     `json:"estimated_total_tokens"`
		EstimatedCost         float64 `json:"estimated_cost"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	// Create validation metric record with estimates
	metric := store.ValidationMetric{
		Prompt:                req.Prompt,
		DetectedTaskType:      req.DetectedTaskType,
		DetectedEffort:        req.DetectedEffort,
		RoutedModel:           req.RoutedModel,
		EstimatedInputTokens:  req.EstimatedInputTokens,
		EstimatedOutputTokens: req.EstimatedOutputTokens,
		EstimatedTotalTokens:  req.EstimatedTotalTokens,
		EstimatedCost:         req.EstimatedCost,
		Validated:             false, // Not validated yet (waiting for barista)
	}

	if err := s.db.LogValidationMetric(metric); err != nil {
		http.Error(w, "failed to log metric", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"success":       true,
		"validation_id": metric.ID,
		"estimated":     metric.EstimatedTotalTokens,
		"effort":        metric.DetectedEffort,
		"model":         metric.RoutedModel,
		"timestamp":     time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		fmt.Printf("error encoding response: %v\n", err)
	}
}

// Helper functions

func isEscalateCommand(prompt string) bool {
	return len(prompt) >= 9 && prompt[:9] == "/escalate"
}

func extractEscalateTarget(prompt string) string {
	// /escalate to opus → opus
	if len(prompt) > 13 && prompt[:13] == "/escalate to " {
		return prompt[13:]
	}
	return "sonnet" // default
}

func hasSuccessSignal(prompt string) bool {
	signals := []string{
		"works", "works great", "works perfectly",
		"perfect", "perfectly",
		"thanks", "thank you",
		"great", "excellent",
		"solved", "fixed", "done",
		"got it", "got it!",
		"appreciate", "appreciate it",
		"exactly", "exactly right",
		"correct", "that's correct",
		"right", "that's right",
		"success", "successful",
	}
	for _, s := range signals {
		if len(prompt) >= len(s) && prompt[:len(s)] == s {
			return true
		}
	}
	return false
}

func detectEffort(prompt string) string {
	// Simple heuristic: word count and keywords
	wordCount := len(prompt) / 5 // rough estimate

	hasComplexKeywords := containsAny(prompt, []string{
		"architecture", "design", "complex", "system",
		"refactor", "optimize", "migrate", "rewrite",
	})

	hasSimpleKeywords := containsAny(prompt, []string{
		"what is", "how do", "help", "explain",
		"define", "list", "quick",
	})

	if hasComplexKeywords || wordCount > 50 {
		return "high"
	}
	if hasSimpleKeywords && wordCount < 20 {
		return "low"
	}
	return "medium"
}

func containsAny(text string, keywords []string) bool {
	for _, k := range keywords {
		if len(text) >= len(k) {
			for i := 0; i <= len(text)-len(k); i++ {
				if text[i:i+len(k)] == k {
					return true
				}
			}
		}
	}
	return false
}

func effortToModel(effort string) string {
	switch effort {
	case "low":
		return "haiku"
	case "medium":
		return "sonnet"
	case "high":
		return "opus"
	default:
		return "sonnet"
	}
}

func cascadeDown(model string) string {
	short := modelShortName(model)
	switch short {
	case "opus":
		return "sonnet"
	case "sonnet":
		return "haiku"
	default:
		return "haiku"
	}
}

func modelShortName(fullModel string) string {
	if len(fullModel) >= 6 && fullModel[:6] == "claude" {
		if len(fullModel) >= 15 && fullModel[14:15] == "4" {
			if len(fullModel) >= 22 && fullModel[20:22] == "op" {
				return "opus"
			}
			if len(fullModel) >= 20 && fullModel[18:20] == "on" {
				return "sonnet"
			}
		}
		if len(fullModel) >= 12 && fullModel[10:12] == "ha" {
			return "haiku"
		}
	}
	return "haiku"
}

func modelToFull(short string) string {
	switch short {
	case "opus":
		return "claude-opus-4-7"
	case "sonnet":
		return "claude-sonnet-4-6"
	case "haiku":
		return "claude-haiku-4-5-20251001"
	default:
		return "claude-haiku-4-5-20251001"
	}
}

func updateClaudeSettings(model string) error {
	// Default effort level based on model
	effort := "medium"
	switch modelShortName(model) {
	case "haiku":
		effort = "low"
	case "opus":
		effort = "high"
	}
	return config.WriteClaudeSettings(model, effort)
}

// Signal detection request/response types
type DetectSignalRequest struct {
	Text string `json:"text"`
}

type DetectSignalResponse struct {
	SignalType SignalType `json:"signal_type"`
	Confidence float64    `json:"confidence"`
	Pattern    string     `json:"pattern"`
	Text       string     `json:"text"`
}

type SignalType string

const (
	SignalSuccess       SignalType = "success"
	SignalFailure       SignalType = "failure"
	SignalEscalation    SignalType = "escalation"
	SignalClarification SignalType = "clarification"
	SignalEffortLow     SignalType = "effort_low"
	SignalEffortHigh    SignalType = "effort_high"
	SignalNone          SignalType = "none"
)

// handleDetectSignal analyzes text for user signals
func (s *Service) handleDetectSignal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req DetectSignalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	detector := signals.NewDetector()
	signal := detector.DetectSignal(req.Text)

	resp := DetectSignalResponse{
		SignalType: SignalType(signal.Type),
		Confidence: signal.Confidence,
		Pattern:    signal.Pattern,
		Text:       signal.Text,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		fmt.Printf("error encoding response: %v\n", err)
	}
}

// Decision request/response types
type MakeDecisionRequest struct {
	ValidationID string               `json:"validation_id"`
	Signal       DetectSignalResponse `json:"signal"`
}

type MakeDecisionResponse struct {
	Action              string  `json:"action"`
	NextModel           string  `json:"next_model"`
	NextEffort          string  `json:"next_effort"`
	Reason              string  `json:"reason"`
	Confidence          float64 `json:"confidence"`
	CascadeAvailable    bool    `json:"cascade_available"`
	EscalateAvailable   bool    `json:"escalate_available"`
	CostSavingsEstimate float64 `json:"cost_savings_estimate"`
}

// handleMakeDecision makes optimization decisions based on validation metrics and signals
func (s *Service) handleMakeDecision(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req MakeDecisionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	validation, err := s.db.GetValidationMetric(req.ValidationID)
	if err != nil {
		http.Error(w, fmt.Sprintf("validation not found: %v", err), http.StatusNotFound)
		return
	}

	// Convert response signal to internal signal type
	internalSignal := signals.Signal{
		Type:       signals.SignalType(req.Signal.SignalType),
		Confidence: req.Signal.Confidence,
		Pattern:    req.Signal.Pattern,
		Text:       req.Signal.Text,
	}

	engine := decisions.NewEngine()
	decision := engine.MakeDecision(validation, internalSignal)

	resp := MakeDecisionResponse{
		Action:              decision.Action,
		NextModel:           decision.NextModel,
		NextEffort:          decision.NextEffort,
		Reason:              decision.Reason,
		Confidence:          decision.Confidence,
		CascadeAvailable:    decision.CascadeAvailable,
		EscalateAvailable:   decision.EscalateAvailable,
		CostSavingsEstimate: decision.CostSavingsEstimate,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		fmt.Printf("error encoding response: %v\n", err)
	}
}

// Learning analysis request/response
type DecisionLearningResponse struct {
	LowEffort    map[string]interface{} `json:"low_effort"`
	MediumEffort map[string]interface{} `json:"medium_effort"`
	HighEffort   map[string]interface{} `json:"high_effort"`
}

// handleDecisionLearning provides learning analysis across task types
func (s *Service) handleDecisionLearning(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	validations, err := s.db.GetAllValidationMetrics()
	if err != nil {
		http.Error(w, fmt.Sprintf("error retrieving validations: %v", err), http.StatusInternalServerError)
		return
	}

	engine := decisions.NewEngine()
	learning := engine.CalculateLearning(validations)

	resp := DecisionLearningResponse{
		LowEffort:    learning["low_effort"].(map[string]interface{}),
		MediumEffort: learning["medium_effort"].(map[string]interface{}),
		HighEffort:   learning["high_effort"].(map[string]interface{}),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		fmt.Printf("error encoding response: %v\n", err)
	}
}

// escalateByOne escalates from current model to next tier
func escalateByOne(currentModel string) string {
	switch currentModel {
	case "haiku":
		return "sonnet"
	case "sonnet":
		return "opus"
	case "opus":
		return "opus" // Already at top
	default:
		return "sonnet" // Default escalation
	}
}

const dashboardHTML = `<!-- Dashboard HTML would be embedded here -->`
