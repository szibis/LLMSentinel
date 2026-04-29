package config

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/szibis/claude-escalate/internal/discovery"
)

// expandHome expands ~ to user home directory
func expandHome(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	usr, err := user.Current()
	if err != nil {
		return path
	}
	return filepath.Join(usr.HomeDir, path[1:])
}

// isToolAvailable checks if tool exists in PATH
func isToolAvailable(toolName string) bool {
	_, err := exec.LookPath(toolName)
	return err == nil
}

// DefaultConfig returns config with auto-detected tools
func DefaultConfig() *Config {
	return &Config{
		Gateway: GatewayConfig{
			Port:            8077,
			Host:            "0.0.0.0",
			SecurityLayer:   true,
			ShutdownTimeout: 30,
			MaxRequestSize:  10485760,
			DataDir:         expandHome("~/.claude-escalate/data"),
		},
	}
}

// Loader handles loading and managing configuration
type Loader struct {
	configPath string
	config     *Config
	loadedPath string
}

// NewLoader creates a new configuration loader
func NewLoader(configPath string) *Loader {
	return &Loader{
		configPath: configPath,
	}
}

// GetLoadedPath returns the path of the config file that was loaded
func (l *Loader) GetLoadedPath() string {
	return l.loadedPath
}

// Load loads configuration from file, auto-detecting tools if no config exists
func (l *Loader) Load() (*Config, error) {
	// Try to load from provided path
	if l.configPath != "" {
		if err := l.loadFromFile(l.configPath); err == nil {
			return l.config, nil
		}
	}

	// Try default locations
	defaultPaths := []string{
		"./config.yaml",
		"./configs/config.yaml",
		expandHome("~/.claude-escalate/config.yaml"),
	}

	// Add CONFIG_FILE environment variable if set
	if envPath := os.Getenv("CONFIG_FILE"); envPath != "" {
		// Check if it's not already in the default paths
		found := false
		for _, p := range defaultPaths {
			if p == envPath {
				found = true
				break
			}
		}
		if !found {
			defaultPaths = append(defaultPaths, envPath)
		}
	}

	for _, path := range defaultPaths {
		if err := l.loadFromFile(path); err == nil {
			return l.config, nil
		}
	}

	// No config file found, use auto-detected defaults
	cfg := DefaultConfig()
	l.config = cfg
	return cfg, nil
}

// loadFromFile loads configuration from a YAML file
func (l *Loader) loadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return err
	}

	// Apply defaults for any missing configuration sections
	applyNewConfigDefaults(cfg)

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return err
	}

	l.config = cfg
	l.loadedPath = path
	return nil
}

// generateDefaultConfig generates configuration with auto-detected tools
func (l *Loader) generateDefaultConfig() *Config {
	cfg := &Config{
		Gateway: GatewayConfig{
			Port:            8080,
			Host:            "0.0.0.0",
			SecurityLayer:   true,
			ShutdownTimeout: 30,
			MaxRequestSize:  10485760, // 10MB
			DataDir:         expandHome("~/.claude-escalate/data"),
		},
		Optimizations: OptimizationsConfig{
			RTK: RTKConfig{
				Enabled:             isToolAvailable("rtk"),
				CommandProxySavings: 99.4,
				Models: map[string]string{
					"low_effort":    "haiku",
					"medium_effort": "sonnet",
					"high_effort":   "opus",
				},
				CacheSavings: true,
			},
			MCP: MCPConfig{
				Enabled: true,
				Tools: []MCPTool{
					{
						Type: "web_scraping",
						Name: "scrapling",
						Settings: map[string]interface{}{
							"css_selector":    true,
							"markdown_only":   true,
							"cache_responses": true,
						},
					},
					{
						Type: "code_analysis",
						Name: "builtin_lsp",
						Settings: map[string]interface{}{
							"use_lsp":       true,
							"cache_symbols": true,
						},
					},
				},
			},
			SemanticCache: SemanticCacheConfig{
				Enabled:             true,
				EmbeddingModel:      "onnx-mini-l6",
				SimilarityThreshold: 0.85,
				HitRateTarget:       60,
				FalsePositiveLimit:  0.5,
				MaxCacheSize:        500,
			},
			KnowledgeGraph: KnowledgeGraphConfig{
				Enabled:         false, // Phase 2
				IndexLocalCode:  false,
				IndexWebContent: false,
				DBPath:          expandHome("~/.claude-escalate/graph.db"),
			},
			InputOptimization: InputOptimizationConfig{
				Enabled:                  true,
				StripUnusedTools:         true,
				CompressParameters:       true,
				DeduplicateExactRequests: true,
			},
			OutputOptimization: OutputOptimizationConfig{
				Enabled:             true,
				ResponseCompression: true,
				FieldFiltering:      true,
				DeltaDetection:      true,
			},
			BatchAPI: BatchAPIConfig{
				Enabled:          false,
				MinBatchSize:     10,
				MaxBatchSize:     100,
				AutoBatchSimilar: true,
			},
		},
		IntentDetection: IntentConfig{
			Enabled: true,
			CacheBypassPatterns: []string{
				"--no-cache",
				"--fresh",
				"!",
				"(no cache)",
				"(bypass)",
			},
			Personalization: PersonalizationConfig{
				LearnFromFeedback:   true,
				AdaptPerUser:        true,
				FeedbackHistoryDays: 90,
			},
		},
		Security: SecurityConfig{
			Enabled:                   true,
			SQLInjectionDetection:     true,
			XSSPrevention:             true,
			CommandInjectionDetection: true,
			RateLimiting: RateLimitConfig{
				RequestsPerMinute: 1000,
				PerIP:             true,
			},
			AuditLogging: true,
		},
		Metrics: MetricsConfig{
			Enabled: true,
			PublishTo: PublishTargets{
				Prometheus: PrometheusTarget{
					Enabled: true,
					Port:    9090,
					Path:    "/metrics",
				},
				Grafana: GrafanaTarget{
					Enabled: false,
				},
				CloudWatch: CloudWatchTarget{
					Enabled: false,
				},
				DebugLogs: DebugLogsTarget{
					Enabled: true,
					Dir:     expandHome("~/.claude-escalate/metrics"),
				},
			},
		},
	}

	// Apply defaults for all configuration sections
	applyNewConfigDefaults(cfg)

	l.config = cfg
	return cfg
}

// generateDefaultConfigWithDiscovery generates configuration using YAML-based tool discovery
func (l *Loader) generateDefaultConfigWithDiscovery() *Config {
	// Try to load discovery config
	discoveryConfigPath := findDiscoveryConfig()
	var detectedTools *discovery.DetectedTools

	if discoveryConfigPath != "" {
		tools, err := discovery.DetectToolsWithConfig(discoveryConfigPath)
		if err == nil {
			detectedTools = tools
		}
	}

	// Fallback to hardcoded detection if YAML-based fails
	if detectedTools == nil {
		fallback := discovery.DetectTools()
		detectedTools = fallback
	}

	// Generate config with detected tools
	cfg := &Config{
		Gateway: GatewayConfig{
			Port:            8080,
			Host:            "0.0.0.0",
			SecurityLayer:   true,
			ShutdownTimeout: 30,
			MaxRequestSize:  10485760, // 10MB
			DataDir:         expandHome("~/.claude-escalate/data"),
		},
		Optimizations: OptimizationsConfig{
			RTK: RTKConfig{
				Enabled:             detectedTools.RTKPath != "",
				CommandProxySavings: 99.4,
				Models: map[string]string{
					"low_effort":    "haiku",
					"medium_effort": "sonnet",
					"high_effort":   "opus",
				},
				CacheSavings: true,
			},
			MCP: MCPConfig{
				Enabled: detectedTools.ScraplingPath != "",
				Tools: []MCPTool{
					{
						Type: "web_scraping",
						Name: "scrapling",
						Settings: map[string]interface{}{
							"css_selector":    true,
							"markdown_only":   true,
							"cache_responses": true,
						},
					},
					{
						Type: "code_analysis",
						Name: "builtin_lsp",
						Settings: map[string]interface{}{
							"use_lsp":       true,
							"cache_symbols": true,
						},
					},
				},
			},
			SemanticCache: SemanticCacheConfig{
				Enabled:             true,
				EmbeddingModel:      "onnx-mini-l6",
				SimilarityThreshold: 0.85,
				HitRateTarget:       60,
				FalsePositiveLimit:  0.5,
				MaxCacheSize:        500,
			},
			KnowledgeGraph: KnowledgeGraphConfig{
				Enabled:         false, // Phase 2
				IndexLocalCode:  false,
				IndexWebContent: false,
				DBPath:          expandHome("~/.claude-escalate/graph.db"),
			},
			InputOptimization: InputOptimizationConfig{
				Enabled:                  true,
				StripUnusedTools:         true,
				CompressParameters:       true,
				DeduplicateExactRequests: true,
			},
			OutputOptimization: OutputOptimizationConfig{
				Enabled:             true,
				ResponseCompression: true,
				FieldFiltering:      true,
				DeltaDetection:      true,
			},
			BatchAPI: BatchAPIConfig{
				Enabled:          false,
				MinBatchSize:     10,
				MaxBatchSize:     100,
				AutoBatchSimilar: true,
			},
		},
		IntentDetection: IntentConfig{
			Enabled: true,
			CacheBypassPatterns: []string{
				"--no-cache",
				"--fresh",
				"!",
				"(no cache)",
				"(bypass)",
			},
			Personalization: PersonalizationConfig{
				LearnFromFeedback:   true,
				AdaptPerUser:        true,
				FeedbackHistoryDays: 90,
			},
		},
		Security: SecurityConfig{
			Enabled:                   true,
			SQLInjectionDetection:     true,
			XSSPrevention:             true,
			CommandInjectionDetection: true,
			RateLimiting: RateLimitConfig{
				RequestsPerMinute: 1000,
				PerIP:             true,
			},
			AuditLogging: true,
		},
		Metrics: MetricsConfig{
			Enabled: true,
			PublishTo: PublishTargets{
				Prometheus: PrometheusTarget{
					Enabled: true,
					Port:    9090,
					Path:    "/metrics",
				},
				Grafana: GrafanaTarget{
					Enabled: false,
				},
				CloudWatch: CloudWatchTarget{
					Enabled: false,
				},
				DebugLogs: DebugLogsTarget{
					Enabled: true,
					Dir:     expandHome("~/.claude-escalate/metrics"),
				},
			},
		},
	}

	// Apply defaults for all configuration sections
	applyNewConfigDefaults(cfg)

	l.config = cfg
	return cfg
}

// findDiscoveryConfig searches for discovery.yaml configuration file
func findDiscoveryConfig() string {
	searchPaths := []string{
		"./configs/discovery.yaml",
		"./discovery.yaml",
		expandHome("~/.claude-escalate/discovery.yaml"),
	}

	for _, path := range searchPaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

// SaveDefault saves the current configuration as default
func (l *Loader) SaveDefault() error {
	if l.config == nil {
		return fmt.Errorf("no configuration to save")
	}

	configDir := expandHome("~/.claude-escalate")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return err
	}

	configPath := filepath.Join(configDir, "config.yaml")
	data, err := yaml.Marshal(l.config)
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0600)
}

// GetConfig returns the loaded configuration
func (l *Loader) GetConfig() *Config {
	return l.config
}

// Note: isToolAvailable() and expandHome() are defined in defaults.go
// and used throughout the package

// applyNewConfigDefaults applies default values to new configuration sections
func applyNewConfigDefaults(cfg *Config) {
	// Apply Thresholds defaults
	if cfg.Thresholds.CacheSimilarity == 0 {
		cfg.Thresholds.CacheSimilarity = 0.85
	}
	if cfg.Thresholds.ConfidenceScores.High == 0 {
		cfg.Thresholds.ConfidenceScores.High = 0.95
	}
	if cfg.Thresholds.ConfidenceScores.Medium == 0 {
		cfg.Thresholds.ConfidenceScores.Medium = 0.85
	}
	if cfg.Thresholds.ConfidenceScores.Low == 0 {
		cfg.Thresholds.ConfidenceScores.Low = 0.70
	}
	if cfg.Thresholds.ModelAccuracy == 0 {
		cfg.Thresholds.ModelAccuracy = 0.85
	}

	// Apply Keywords defaults
	if len(cfg.Keywords.Detail) == 0 {
		cfg.Keywords.Detail = []string{"detailed", "comprehensive", "explain", "why", "how", "deep", "thorough", "analysis"}
	}
	if len(cfg.Keywords.Quick) == 0 {
		cfg.Keywords.Quick = []string{"quick", "brief", "summary", "tl;dr", "just", "simply", "shortly"}
	}
	if len(cfg.Keywords.FollowUp) == 0 {
		cfg.Keywords.FollowUp = []string{"more", "additional", "also", "what about", "furthermore", "moreover"}
	}
	if len(cfg.Keywords.Learning) == 0 {
		cfg.Keywords.Learning = []string{"what if", "try", "experiment", "compare", "explore", "alternative"}
	}
	if len(cfg.Keywords.Suspicious) == 0 {
		cfg.Keywords.Suspicious = []string{"DROP TABLE", "DELETE FROM", "INSERT INTO", "UPDATE", "TRUNCATE", "UNION SELECT"}
	}

	// Apply Signals defaults
	if cfg.Signals.SuccessSignals == nil {
		cfg.Signals.SuccessSignals = map[string]float64{
			"perfect":   0.95,
			"thanks":    0.85,
			"thank_you": 0.85,
			"solved":    0.90,
			"works":     0.85,
			"got_it":    0.85,
		}
	}
	if cfg.Signals.FailureSignals == nil {
		cfg.Signals.FailureSignals = map[string]float64{
			"error":       0.90,
			"broken":      0.90,
			"failed":      0.90,
			"doesnt_work": 0.85,
			"issue":       0.80,
		}
	}
	if cfg.Signals.FrustrationKeywords == nil {
		cfg.Signals.FrustrationKeywords = map[string]float64{
			"still_broken":     0.90,
			"going_in_circles": 0.85,
			"stuck":            0.85,
			"frustrated":       0.80,
		}
	}

	// Apply TokenLimits defaults
	if cfg.TokenLimits.QuickAnswer == 0 {
		cfg.TokenLimits.QuickAnswer = 256
	}
	if cfg.TokenLimits.DetailedAnalysis == 0 {
		cfg.TokenLimits.DetailedAnalysis = 2000
	}
	if cfg.TokenLimits.Routine == 0 {
		cfg.TokenLimits.Routine = 256
	}
	if cfg.TokenLimits.Learning == 0 {
		cfg.TokenLimits.Learning = 1024
	}
	if cfg.TokenLimits.FollowUp == 0 {
		cfg.TokenLimits.FollowUp = 512
	}
	if cfg.TokenLimits.CacheBypass == 0 {
		cfg.TokenLimits.CacheBypass = 2000
	}
	if cfg.TokenLimits.MaxCacheSize == nil {
		cfg.TokenLimits.MaxCacheSize = map[string]int{
			"haiku":  5000,
			"sonnet": 10000,
			"opus":   50000,
		}
	}

	// Apply Timeouts defaults (in milliseconds)
	if cfg.Timeouts.IndividualToolCheckMs == 0 {
		cfg.Timeouts.IndividualToolCheckMs = 500
	}
	if cfg.Timeouts.TotalDiscoveryMs == 0 {
		cfg.Timeouts.TotalDiscoveryMs = 5000
	}
	if cfg.Timeouts.EscalationMs == 0 {
		cfg.Timeouts.EscalationMs = 2000
	}
	if cfg.Timeouts.IntentDetectionMs == 0 {
		cfg.Timeouts.IntentDetectionMs = 50
	}
	if cfg.Timeouts.SecurityValidationMs == 0 {
		cfg.Timeouts.SecurityValidationMs = 20
	}
	if cfg.Timeouts.CacheLookupMs == 0 {
		cfg.Timeouts.CacheLookupMs = 10
	}

	// Apply Paths defaults
	if cfg.Paths.ConfigDir == "" {
		cfg.Paths.ConfigDir = expandHome("~/.claude-escalate")
	}
	if cfg.Paths.DataDir == "" {
		cfg.Paths.DataDir = expandHome("~/.claude-escalate/data")
	}
	if cfg.Paths.GraphDBPath == "" {
		cfg.Paths.GraphDBPath = expandHome("~/.claude-escalate/graph.db")
	}
	if cfg.Paths.MetricsDir == "" {
		cfg.Paths.MetricsDir = expandHome("~/.claude-escalate/metrics")
	}
	if cfg.Paths.LogDir == "" {
		cfg.Paths.LogDir = expandHome("~/.claude-escalate/logs")
	}
	if cfg.Paths.CacheDir == "" {
		cfg.Paths.CacheDir = expandHome("~/.claude-escalate/cache")
	}
	if cfg.Paths.ClaudeHome == "" {
		cfg.Paths.ClaudeHome = expandHome("~/.claude")
	}

	// Apply Models defaults
	if cfg.Models.Haiku.ID == "" {
		cfg.Models.Haiku.ID = "claude-haiku-4-5-20251001"
		cfg.Models.Haiku.CostPer1KInput = 0.0008
		cfg.Models.Haiku.CostPer1KOutput = 0.0004
		cfg.Models.Haiku.ContextWindow = 200000
	}
	if cfg.Models.Sonnet.ID == "" {
		cfg.Models.Sonnet.ID = "claude-sonnet-4-6"
		cfg.Models.Sonnet.CostPer1KInput = 0.003
		cfg.Models.Sonnet.CostPer1KOutput = 0.015
		cfg.Models.Sonnet.ContextWindow = 200000
	}
	if cfg.Models.Opus.ID == "" {
		cfg.Models.Opus.ID = "claude-opus-4-6"
		cfg.Models.Opus.CostPer1KInput = 0.015
		cfg.Models.Opus.CostPer1KOutput = 0.075
		cfg.Models.Opus.ContextWindow = 200000
	}

	// Apply Security defaults
	if cfg.Security.RateLimiting.RequestsPerMinute <= 0 {
		cfg.Security.RateLimiting.RequestsPerMinute = 1000
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Gateway.Port <= 0 || c.Gateway.Port > 65535 {
		return fmt.Errorf("invalid gateway port: %d", c.Gateway.Port)
	}

	if c.Optimizations.SemanticCache.Enabled {
		if c.Optimizations.SemanticCache.SimilarityThreshold < 0.0 || c.Optimizations.SemanticCache.SimilarityThreshold > 1.0 {
			return fmt.Errorf("semantic cache similarity threshold must be between 0.0 and 1.0, got %f", c.Optimizations.SemanticCache.SimilarityThreshold)
		}
		if c.Optimizations.SemanticCache.FalsePositiveLimit < 0.0 || c.Optimizations.SemanticCache.FalsePositiveLimit > 100.0 {
			return fmt.Errorf("false positive limit must be between 0 and 100, got %f", c.Optimizations.SemanticCache.FalsePositiveLimit)
		}
	}

	if c.Security.RateLimiting.RequestsPerMinute <= 0 {
		return fmt.Errorf("rate limiting requests per minute must be positive, got %d", c.Security.RateLimiting.RequestsPerMinute)
	}

	return nil
}
