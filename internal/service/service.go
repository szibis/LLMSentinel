// Package service provides HTTP server for escalation management.
package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/szibis/claude-escalate/internal/config"
	"github.com/szibis/claude-escalate/internal/store"
)

// Service handles all escalation logic via HTTP.
type Service struct {
	db  *store.Store
	cfg *config.Config
}

// New creates a new service instance.
func New(cfg *config.Config) (*Service, error) {
	db, err := store.Open(cfg.DataDir)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	return &Service{db: db, cfg: cfg}, nil
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

	// Dashboard endpoints
	mux.HandleFunc("/", s.handleDashboard)

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	fmt.Printf("claude-escalate service starting on http://%s\n", addr)
	fmt.Printf("  - Hook endpoint: POST /api/hook\n")
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

const dashboardHTML = `<!-- Dashboard HTML would be embedded here -->`
