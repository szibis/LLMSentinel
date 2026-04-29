package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Version is set at build time via ldflags.
var Version = "dev"

// Model IDs for Claude models.
const (
	ModelHaiku  = "claude-haiku-4-5-20251001"
	ModelSonnet = "claude-sonnet-4-6"
	ModelOpus   = "claude-opus-4-6"
)

// ModelTier represents the cost tier of a model.
type ModelTier int

const (
	TierHaiku  ModelTier = 1
	TierSonnet ModelTier = 2
	TierOpus   ModelTier = 3
)

// LegacyConfig holds legacy configuration (deprecated, use Config from types.go instead)
type LegacyConfig struct {
	DataDir            string  `json:"data_dir"`
	DashboardPort      int     `json:"dashboard_port"`
	DashboardBind      string  `json:"dashboard_bind"`      // Bind address (default 127.0.0.1)
	FrustrationRetries int     `json:"frustration_retries"` // Min retries before suggesting escalation
	SessionTimeout     int     `json:"session_timeout"`     // Escalation session timeout in seconds
	PredictThreshold   int     `json:"predict_threshold"`   // Min escalations to enable prediction
	CircularTurns      int     `json:"circular_turns"`      // Min turns to detect circular reasoning
	DailyBudgetUSD     float64 `json:"daily_budget_usd"`    // Daily spend guard (0 = unlimited)
}

// DefaultConfig is defined in defaults.go
// It returns a base configuration with auto-detected tools and optimizations

// DefaultLegacyConfig returns the default legacy configuration (deprecated).
func DefaultLegacyConfig() *LegacyConfig {
	return &LegacyConfig{
		DataDir:            filepath.Join(homeDir(), ".claude", "data", "escalation"),
		DashboardPort:      8077,
		FrustrationRetries: 2,
		SessionTimeout:     1800,
		PredictThreshold:   5,
		CircularTurns:      4,
		DailyBudgetUSD:     0,
	}
}

func homeDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return "/tmp"
	}
	return h
}

// ClaudeSettings represents the relevant fields in ~/.claude/settings.json.
type ClaudeSettings struct {
	Model       string `json:"model"`
	EffortLevel string `json:"effortLevel"`
}

// ReadClaudeSettings reads the current model and effort from settings.json.
func ReadClaudeSettings() (*ClaudeSettings, error) {
	path := filepath.Join(homeDir(), ".claude", "settings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s ClaudeSettings
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// WriteClaudeSettings updates model and effort in settings.json atomically.
func WriteClaudeSettings(model, effort string) error {
	path := filepath.Join(homeDir(), ".claude", "settings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	raw["model"] = model
	raw["effortLevel"] = effort

	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}

	tmpFile := path + ".tmp"
	if err := os.WriteFile(tmpFile, out, 0600); err != nil {
		return err
	}
	return os.Rename(tmpFile, path)
}

// ModelShortName returns a human-friendly name for a model ID.
func ModelShortName(modelID string) string {
	switch {
	case contains(modelID, "haiku"):
		return "haiku"
	case contains(modelID, "sonnet"):
		return "sonnet"
	case contains(modelID, "opus"):
		return "opus"
	default:
		return modelID
	}
}

// ModelTierOf returns the tier for a model ID.
func ModelTierOf(modelID string) ModelTier {
	switch ModelShortName(modelID) {
	case "haiku":
		return TierHaiku
	case "sonnet":
		return TierSonnet
	case "opus":
		return TierOpus
	default:
		return TierHaiku
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsSubstring(s, sub))
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
