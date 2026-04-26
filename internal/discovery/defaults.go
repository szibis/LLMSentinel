package discovery

import (
	"fmt"
	"os"
	"path/filepath"
)

// GenerateDefaultConfig creates a sensible default configuration based on detected tools
func GenerateDefaultConfig(detectedTools *DetectedTools) string {
	config := `# Claude Escalate v4.1.0 - Auto-Detected Configuration
# Generated at startup based on available tools

gateway:
  port: 8080
  host: "127.0.0.1"
  security_layer: enabled  # ALWAYS on, cannot be disabled
  timeout_ms: 30000

optimizations:
`

	// RTK optimization
	if detectedTools.RTKPath != "" {
		config += fmt.Sprintf(`  # RTK (99.4%% savings on command output)
  rtk:
    enabled: true
    path: "%s"
    config:
      command_proxy_savings: 99
      models:
        low_effort: haiku
        medium_effort: sonnet
        high_effort: opus

`, detectedTools.RTKPath)
	} else {
		config += `  # RTK not detected - install at ~/.local/bin/rtk to enable
  rtk:
    enabled: false

`
	}

	// Semantic caching
	config += `  # Semantic caching (98%% savings on similar queries)
  semantic_cache:
    enabled: true
    settings:
      embedding_model: "onnx-mini-l6"
      similarity_threshold: 0.85  # Strict threshold for <0.1%% false positives
      hit_rate_target: 60
      false_positive_limit: 0.5   # Kill cache if >0.5%% wrong answers

`

	// Input optimization
	config += `  # Input optimization (30-40%% savings)
  input_optimization:
    enabled: true
    settings:
      strip_unused_tools: true
      compress_parameters: true
      dedup_exact_requests: true

`

	// Output optimization
	config += `  # Output optimization (30-50%% savings)
  output_optimization:
    enabled: true
    settings:
      response_compression: true
      field_filtering: true
      delta_detection: true

`

	// Knowledge graph (Phase 2)
	config += `  # Knowledge graph (Phase 2 - optional)
  knowledge_graph:
    enabled: false

  # Batch API (Phase 2 - optional)
  batch_api:
    enabled: false

`

	// Intent detection
	config += `intent_detection:
  enabled: true
  settings:
    cache_bypass_patterns:
      - "--no-cache"
      - "--fresh"
      - "!"
      - "(no cache)"
      - "(bypass)"
    personalization:
      learn_from_feedback: true
      adapt_per_user: true
      feedback_history_depth: 90

`

	// Security
	config += `security:
  enabled: true                 # ALWAYS on, cannot be disabled
  settings:
    sql_injection_detection: true
    xss_prevention: true
    command_injection_detection: true
    rate_limiting:
      requests_per_minute: 1000
      per_ip: true
    audit_logging: true

`

	// Metrics
	config += `metrics:
  enabled: true
  publish_to:
    - prometheus:
        enabled: true
        port: 9090
        path: /metrics
    - debug_logs:
        enabled: true
        dir: ~/.claude-escalate/metrics

  track:
    - cache_hit_rate:
        enabled: true
        interval: 60s
    - cache_false_positive_rate:
        enabled: true
        interval: 60s
        alert_if_above: 0.5
    - token_savings_percent:
        enabled: true
        interval: 60s
    - latency_by_layer:
        enabled: true
    - security_events:
        enabled: true
    - cost_tracking:
        enabled: true

`

	// Tools/adapters
	config += `# Tool adapters (auto-detected)\n`

	if detectedTools.GitAvailable {
		config += `tools:
  cli:
    enabled: true
    type: "cli"
    whitelist:
      - "git"
      - "ls"
      - "cat"
      - "grep"
      - "find"
      - "head"
      - "tail"
      - "wc"
      - "stat"

`
	}

	if detectedTools.ScraplingAvailable {
		config += `  scrapling:
    enabled: true
    type: "mcp"
    protocol: "json-rpc"
    settings:
      css_selector: true
      markdown_only: true
      cache_responses: true

`
	}

	if detectedTools.LSPAvailable {
		config += `  lsp:
    enabled: true
    type: "mcp"
    protocol: "json-rpc"
    settings:
      use_structured_search: true
      cache_symbols: true

`
	}

	config += `# Logging
logging:
  level: "info"
  format: "json"
  output: "stdout"
  audit_log_dir: "~/.claude-escalate/audit"

# Dashboard
dashboard:
  enabled: true
  host: "127.0.0.1"
  port: 8080
  path: "/dashboard"

# Live reload
live_reload:
  enabled: true
  watch_file: true
  auto_save: true
`

	return config
}

// SaveDefaultConfig writes default configuration to ~/.claude-escalate/config.yaml
func SaveDefaultConfig(config string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	configDir := filepath.Join(homeDir, ".claude-escalate")

	// Create directory if it doesn't exist
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create config directory: %w", err)
	}

	configPath := filepath.Join(configDir, "config.yaml")

	// Check if config already exists
	if _, err := os.Stat(configPath); err == nil {
		// Config exists, create config-auto.yaml instead
		configPath = filepath.Join(configDir, "config-auto.yaml")
	}

	// Write config file
	if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
		return "", fmt.Errorf("failed to write config file: %w", err)
	}

	return configPath, nil
}

// LoadOrDetect attempts to load config from file, falls back to auto-detection
func LoadOrDetect(configPath string) (string, *DetectedTools, error) {
	// If configPath provided, try to load it
	if configPath != "" {
		if data, err := os.ReadFile(configPath); err == nil {
			return string(data), nil, nil
		}
	}

	// Fall back to auto-detection
	detectedTools := DetectTools()
	defaultConfig := GenerateDefaultConfig(detectedTools)

	// Save auto-detected config
	savedPath, err := SaveDefaultConfig(defaultConfig)
	if err != nil {
		return defaultConfig, detectedTools, fmt.Errorf("auto-detected but failed to save: %w", err)
	}

	return defaultConfig, detectedTools, nil
}
