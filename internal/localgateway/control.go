package localgateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

func (r *trainingRecorder) captureEnabled() bool { return r != nil && !r.paused.Load() }
func (g *Gateway) mode() string {
	if g.cfg.LearningOnly {
		return "learning"
	}
	if g.cfg.Hybrid != nil {
		return "hybrid"
	}
	return "serving"
}
func (g *Gateway) roleBudget(role string, fallback int) int {
	if slot := g.roleBudgets[role]; slot != nil {
		if value := slot.Load(); value > 0 {
			return int(value)
		}
	}
	return fallback
}
func (g *Gateway) controlStatus() map[string]any {
	budgets := map[string]int{}
	for _, role := range []string{"haiku", "sonnet", "opus"} {
		route, _ := (RoleRouter{}).Select(context.Background(), RouteTask{Model: role})
		budgets[role] = g.roleBudget(role, route.MaxTokens)
	}
	policy := "local-only"
	if g.hybrid != nil {
		policy = g.hybrid.policyName()
	}
	return map[string]any{"mode": g.mode(), "capture_enabled": g.training.captureEnabled(), "capture_configured": g.training != nil, "policy": policy, "role_budgets": budgets, "startup_billing_opt_in": g.cfg.Hybrid != nil && g.cfg.Hybrid.AllowPaidAPI, "jes": "not trained/connected", "local_role_recovery": g.cfg.LocalRoleRecovery, "haiku_tool_role": g.cfg.HaikuToolRole, "quality_gate": "protocol/schema validation, explicit JSON/test workflow and progress checks; semantic scoring pending", "quality_checks": map[string]uint64{"rejections": g.qualityRejections.Load(), "recovery_attempts": g.qualityRecoveries.Load(), "local_escalation_attempts": g.localEscalations.Load()}, "mode_change": "requires relaunch; provider authentication is never changed by controls"}
}
func (g *Gateway) control(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		apiError(w, 405, "method_not_allowed", "Use GET or POST")
		return
	}
	if r.Method == http.MethodPost {
		if r.Header.Get("Origin") != "" || !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
			apiError(w, 400, "invalid_control", "Use local application/json controls")
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 8193))
		if err != nil || len(body) > 8192 {
			apiError(w, 413, "control_too_large", "Control exceeds limit")
			return
		}
		var change struct {
			Capture *bool          `json:"capture_enabled"`
			Policy  string         `json:"policy"`
			Budgets map[string]int `json:"role_budgets"`
		}
		decoder := json.NewDecoder(strings.NewReader(string(body)))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&change) != nil || decoder.Decode(new(any)) != io.EOF {
			apiError(w, 400, "invalid_control", "Unknown or invalid control fields")
			return
		}
		if change.Capture != nil && g.training == nil {
			apiError(w, 409, "capture_unconfigured", "Start with a private training directory before toggling capture")
			return
		}
		if change.Policy != "" {
			if g.hybrid == nil {
				apiError(w, 409, "hybrid_unconfigured", "Hybrid providers and paid API permission must be configured at startup")
				return
			}
			if change.Policy != "local-only" && change.Policy != "balanced" && change.Policy != "quality" {
				apiError(w, 400, "invalid_policy", "Use local-only, balanced or quality")
				return
			}
		}
		for role, budget := range change.Budgets {
			if g.roleBudgets[role] == nil || budget < 1 || budget > 32768 {
				apiError(w, 400, "invalid_budget", "Known role budgets must be 1..32768")
				return
			}
		}
		if change.Capture != nil {
			g.training.paused.Store(!*change.Capture)
		}
		if change.Policy != "" {
			_ = g.hybrid.setPolicy(change.Policy)
		}
		for role, budget := range change.Budgets {
			g.roleBudgets[role].Store(int64(budget))
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(g.controlStatus())
}
