package config

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/szibis/claude-escalate/internal/discovery"
)

// DefaultConfig returns a base configuration with auto-detected tools
func DefaultConfig() *Config {
	tools := detectAndCreateTools()

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
				CacheSavings:        true,
			},
			MCP: MCPConfig{
				Enabled: true,
				Tools:   tools,
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
				Enabled:         false,
				IndexLocalCode:  false,
				IndexWebContent: false,
				CacheLookups:    false,
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

	return cfg
}

// detectAndCreateTools auto-detects installed tools and creates MCPTool entries
func detectAndCreateTools() []MCPTool {
	tools := discovery.DetectTools()
	var mcpTools []MCPTool

	// RTK
	if tools.RTKPath != "" {
		mcpTools = append(mcpTools, MCPTool{
			Type: "cli",
			Name: "rtk",
			Settings: map[string]interface{}{
				"path":        tools.RTKPath,
				"description": "Real Token Killer - Command output optimization (99.4% savings)",
				"enabled":     true,
			},
		})
	}

	// Scrapling
	if tools.ScraplingPath != "" {
		mcpTools = append(mcpTools, MCPTool{
			Type: "mcp",
			Name: "scrapling",
			Settings: map[string]interface{}{
				"path":        tools.ScraplingPath,
				"description": "Web scraping and content extraction (85-94% savings)",
				"enabled":     true,
			},
		})
	}

	// Git
	if tools.GitPath != "" {
		mcpTools = append(mcpTools, MCPTool{
			Type: "cli",
			Name: "git",
			Settings: map[string]interface{}{
				"path":        tools.GitPath,
				"description": "Version control and diff operations",
				"enabled":     true,
			},
		})
	}

	// LSP Servers
	for lang, path := range tools.LSPServers {
		mcpTools = append(mcpTools, MCPTool{
			Type: "lsp",
			Name: lang + "-lsp",
			Settings: map[string]interface{}{
				"path":        path,
				"language":    lang,
				"description": "Language server for " + lang + " (code navigation)",
				"enabled":     true,
			},
		})
	}

	return mcpTools
}

// isToolAvailable checks if a tool is available in PATH
func isToolAvailable(toolName string) bool {
	_, err := exec.LookPath(toolName)
	return err == nil
}

// expandHome expands ~ to home directory
func expandHome(path string) string {
	if len(path) > 0 && path[0] == '~' {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[1:])
		}
	}
	return path
}
