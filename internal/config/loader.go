package config

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Loader handles loading and managing configuration
type Loader struct {
	configPath string
	config     *Config
}

// NewLoader creates a new configuration loader
func NewLoader(configPath string) *Loader {
	return &Loader{
		configPath: configPath,
	}
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

	for _, path := range defaultPaths {
		if err := l.loadFromFile(path); err == nil {
			return l.config, nil
		}
	}

	// No config file found, use auto-detected defaults
	return l.generateDefaultConfig()
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

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return err
	}

	l.config = cfg
	return nil
}

// generateDefaultConfig generates configuration with auto-detected tools
func (l *Loader) generateDefaultConfig() (*Config, error) {
	cfg := &Config{
		Gateway: GatewayConfig{
			Port:            8080,
			Host:            "0.0.0.0",
			SecurityLayer:   true,
			ShutdownTimeout: 30,
			MaxRequestSize:  10485760, // 10MB
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
							"css_selector":   true,
							"markdown_only":  true,
							"cache_responses": true,
						},
					},
					{
						Type: "code_analysis",
						Name: "builtin_lsp",
						Settings: map[string]interface{}{
							"use_lsp":        true,
							"cache_symbols":  true,
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
				Enabled:               true,
				ResponseCompression:   true,
				FieldFiltering:        true,
				DeltaDetection:        true,
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

	l.config = cfg
	return cfg, nil
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

// isToolAvailable checks if a tool is available in PATH or common locations
func isToolAvailable(tool string) bool {
	// Check common tool locations
	commonPaths := []string{
		filepath.Join(os.Getenv("HOME"), ".local", "bin", tool),
		filepath.Join("/usr", "local", "bin", tool),
		filepath.Join("/usr", "bin", tool),
	}

	for _, path := range commonPaths {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}

	// Try to find in PATH
	if path, err := os.LookPath(tool); err == nil && path != "" {
		return true
	}

	return false
}

// expandHome expands ~ in paths
func expandHome(path string) string {
	if path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return home
	}

	if len(path) > 2 && path[0] == '~' && path[1] == '/' {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}

	if len(path) > 1 && path[0] == '~' {
		home, err := user.Current()
		if err != nil {
			return path
		}
		return filepath.Join(home.HomeDir, path[1:])
	}

	return path
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
