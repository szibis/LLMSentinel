package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// EscalationConfig is the complete configuration for the escalation system.
type EscalationConfig struct {
	// Statusline sources (priority order)
	Statusline StatuslineConfig `yaml:"statusline"`

	// Token budgets
	Budgets BudgetConfig `yaml:"budgets"`

	// Sentiment detection
	Sentiment SentimentConfig `yaml:"sentiment"`

	// Decision engine
	Decisions DecisionConfig `yaml:"decisions"`

	// Statusline display
	Display DisplayConfig `yaml:"display"`

	// Logging
	Logging LoggingConfig `yaml:"logging"`
}

// StatuslineConfig defines which statusline sources to use and their priority.
type StatuslineConfig struct {
	Sources []StatuslineSourceConfig `yaml:"sources"`
}

// StatuslineSourceConfig defines a single statusline source.
type StatuslineSourceConfig struct {
	Type    string `yaml:"type"` // barista, claude-native, webhook, file, envvar
	Enabled bool   `yaml:"enabled"`
	Path    string `yaml:"path"`       // For file sources
	URL     string `yaml:"url"`        // For webhook sources
	Token   string `yaml:"token"`      // For webhook auth
	Timeout int    `yaml:"timeout_ms"` // Timeout in milliseconds (default: 2000)
}

// BudgetConfig defines token budget parameters.
type BudgetConfig struct {
	DailyUSD         float64            `yaml:"daily_usd"`
	MonthlyUSD       float64            `yaml:"monthly_usd"`
	SessionTokens    int                `yaml:"session_tokens"`
	HardLimit        bool               `yaml:"hard_limit"`
	SoftLimit        bool               `yaml:"soft_limit"`
	AutoDowngradeAt  float64            `yaml:"auto_downgrade_at"` // e.g., 0.80 = 80%
	ModelDailyLimits map[string]float64 `yaml:"model_daily_limits"`
	TaskTypeBudgets  map[string]int     `yaml:"task_type_budgets"`
	AlertThresholds  map[string]float64 `yaml:"alert_thresholds"`
}

// SentimentConfig defines sentiment detection parameters.
type SentimentConfig struct {
	Enabled                    bool    `yaml:"enabled"`
	FrustrationTriggerEscalate bool    `yaml:"frustration_trigger_escalate"`
	FrustrationRiskThreshold   float64 `yaml:"frustration_risk_threshold"` // 0.0-1.0
	LearningEnabled            bool    `yaml:"learning_enabled"`
	TrackSatisfaction          bool    `yaml:"track_satisfaction"`
}

// DecisionConfig defines the decision engine parameters.
type DecisionConfig struct {
	SuccessSignalThreshold    float64 `yaml:"success_signal_threshold"`
	FailureSignalThreshold    float64 `yaml:"failure_signal_threshold"`
	TokenErrorThreshold       float64 `yaml:"token_error_threshold"` // percent
	AutoEscalateOnFrustration bool    `yaml:"auto_escalate_on_frustration"`
	MaxAttemptsBeforeOpus     int     `yaml:"max_attempts_before_opus"`
}

// DisplayConfig defines statusline display options.
type DisplayConfig struct {
	DisplayModel           bool `yaml:"display_model"`
	DisplayEffort          bool `yaml:"display_effort"`
	DisplayTokens          bool `yaml:"display_tokens"`
	DisplaySentiment       bool `yaml:"display_sentiment"`
	DisplayBudgetRemaining bool `yaml:"display_budget_remaining"`
	RefreshIntervalMs      int  `yaml:"refresh_interval_ms"`
}

// LoggingConfig defines logging parameters.
type LoggingConfig struct {
	Level         string `yaml:"level"` // debug, info, warn, error
	File          string `yaml:"file"`
	RetentionDays int    `yaml:"retention_days"`
}

// LoadEscalationConfig loads configuration from ~/.claude/escalation/config.yaml
func LoadEscalationConfig() (*EscalationConfig, error) {
	configPath := filepath.Join(homeDir(), ".claude", "escalation", "config.yaml")

	// Check if file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Return default config if file doesn't exist
		return DefaultEscalationConfig(), nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg EscalationConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Apply defaults to empty fields
	applyDefaults(&cfg)

	return &cfg, nil
}

// DefaultEscalationConfig returns the default configuration.
func DefaultEscalationConfig() *EscalationConfig {
	return &EscalationConfig{
		Statusline: StatuslineConfig{
			Sources: []StatuslineSourceConfig{
				{
					Type:    "barista",
					Enabled: true,
					Path:    filepath.Join(homeDir(), ".claude", "data", "escalation", "barista-metrics.json"),
					Timeout: 2000,
				},
				{
					Type:    "claude-native",
					Enabled: true,
					Path:    filepath.Join(homeDir(), ".claude", "statusline.json"),
					Timeout: 2000,
				},
				{
					Type:    "envvar",
					Enabled: true,
					Timeout: 2000,
				},
			},
		},
		Budgets: BudgetConfig{
			DailyUSD:        10.0,
			MonthlyUSD:      100.0,
			SessionTokens:   10000,
			HardLimit:       false,
			SoftLimit:       true,
			AutoDowngradeAt: 0.80,
			ModelDailyLimits: map[string]float64{
				"opus":   5.0,
				"sonnet": 3.0,
				"haiku":  0, // unlimited
			},
			TaskTypeBudgets: map[string]int{
				"concurrency":  5000,
				"parsing":      3000,
				"debugging":    4000,
				"architecture": 6000,
				"optimization": 5000,
				"testing":      4000,
				"security":     5000,
			},
			AlertThresholds: map[string]float64{
				"warn_low":  0.50,
				"warn_med":  0.75,
				"warn_high": 0.90,
			},
		},
		Sentiment: SentimentConfig{
			Enabled:                    true,
			FrustrationTriggerEscalate: true,
			FrustrationRiskThreshold:   0.70,
			LearningEnabled:            true,
			TrackSatisfaction:          true,
		},
		Decisions: DecisionConfig{
			SuccessSignalThreshold:    0.80,
			FailureSignalThreshold:    0.80,
			TokenErrorThreshold:       15.0,
			AutoEscalateOnFrustration: true,
			MaxAttemptsBeforeOpus:     2,
		},
		Display: DisplayConfig{
			DisplayModel:           true,
			DisplayEffort:          true,
			DisplayTokens:          true,
			DisplaySentiment:       true,
			DisplayBudgetRemaining: true,
			RefreshIntervalMs:      500,
		},
		Logging: LoggingConfig{
			Level:         "info",
			File:          filepath.Join(homeDir(), ".claude", "data", "escalation", "escalation.log"),
			RetentionDays: 30,
		},
	}
}

// applyDefaults fills in missing values with defaults.
func applyDefaults(cfg *EscalationConfig) {
	defaults := DefaultEscalationConfig()

	// Statusline: use defaults if empty
	if len(cfg.Statusline.Sources) == 0 {
		cfg.Statusline = defaults.Statusline
	}

	// Budgets: merge with defaults
	if cfg.Budgets.DailyUSD == 0 {
		cfg.Budgets.DailyUSD = defaults.Budgets.DailyUSD
	}
	if cfg.Budgets.MonthlyUSD == 0 {
		cfg.Budgets.MonthlyUSD = defaults.Budgets.MonthlyUSD
	}
	if cfg.Budgets.SessionTokens == 0 {
		cfg.Budgets.SessionTokens = defaults.Budgets.SessionTokens
	}
	if cfg.Budgets.AutoDowngradeAt == 0 {
		cfg.Budgets.AutoDowngradeAt = defaults.Budgets.AutoDowngradeAt
	}
	if cfg.Budgets.ModelDailyLimits == nil {
		cfg.Budgets.ModelDailyLimits = defaults.Budgets.ModelDailyLimits
	}
	if cfg.Budgets.TaskTypeBudgets == nil {
		cfg.Budgets.TaskTypeBudgets = defaults.Budgets.TaskTypeBudgets
	}
	if cfg.Budgets.AlertThresholds == nil {
		cfg.Budgets.AlertThresholds = defaults.Budgets.AlertThresholds
	}

	// Sentiment: use defaults if zero-value
	if !cfg.Sentiment.Enabled && !cfg.Sentiment.FrustrationTriggerEscalate {
		cfg.Sentiment = defaults.Sentiment
	}
	if cfg.Sentiment.FrustrationRiskThreshold == 0 {
		cfg.Sentiment.FrustrationRiskThreshold = defaults.Sentiment.FrustrationRiskThreshold
	}

	// Decisions: use defaults if zero-value
	if cfg.Decisions.SuccessSignalThreshold == 0 {
		cfg.Decisions = defaults.Decisions
	}

	// Display: use defaults if zero-value
	if cfg.Display.RefreshIntervalMs == 0 {
		cfg.Display = defaults.Display
	}

	// Logging: use defaults if empty
	if cfg.Logging.Level == "" {
		cfg.Logging = defaults.Logging
	}
}

// SaveEscalationConfig saves configuration to ~/.claude/escalation/config.yaml
func SaveEscalationConfig(cfg *EscalationConfig) error {
	configDir := filepath.Join(homeDir(), ".claude", "escalation")
	if err := os.MkdirAll(configDir, 0750); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	configPath := filepath.Join(configDir, "config.yaml")

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0640); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
