package config

import (
	"time"
)

// Config represents the complete Claude Escalate configuration
type Config struct {
	Gateway        GatewayConfig        `yaml:"gateway"`
	Optimizations  OptimizationsConfig  `yaml:"optimizations"`
	IntentDetection IntentConfig        `yaml:"intent_detection"`
	Security       SecurityConfig       `yaml:"security"`
	Metrics        MetricsConfig        `yaml:"metrics"`
}

// GatewayConfig configures the gateway server
type GatewayConfig struct {
	Port             int    `yaml:"port"`
	Host             string `yaml:"host"`
	SecurityLayer    bool   `yaml:"security_layer"`
	ShutdownTimeout  int    `yaml:"shutdown_timeout_seconds"`
	MaxRequestSize   int    `yaml:"max_request_size_bytes"`
}

// OptimizationsConfig configures all optimization layers
type OptimizationsConfig struct {
	RTK              RTKConfig              `yaml:"rtk"`
	MCP              MCPConfig              `yaml:"mcp"`
	SemanticCache    SemanticCacheConfig    `yaml:"semantic_cache"`
	KnowledgeGraph   KnowledgeGraphConfig   `yaml:"knowledge_graph"`
	InputOptimization InputOptimizationConfig `yaml:"input_optimization"`
	OutputOptimization OutputOptimizationConfig `yaml:"output_optimization"`
	BatchAPI         BatchAPIConfig         `yaml:"batch_api"`
}

// RTKConfig configures RTK optimization
type RTKConfig struct {
	Enabled           bool              `yaml:"enabled"`
	CommandProxySavings float64         `yaml:"command_proxy_savings"`
	Models            map[string]string `yaml:"models"`
	CacheSavings      bool              `yaml:"cache_savings"`
}

// MCPConfig configures MCP tools
type MCPConfig struct {
	Enabled bool      `yaml:"enabled"`
	Tools   []MCPTool `yaml:"tools"`
}

// MCPTool represents a single MCP tool configuration
type MCPTool struct {
	Type     string                 `yaml:"type"`
	Name     string                 `yaml:"name"`
	Settings map[string]interface{} `yaml:"settings"`
}

// SemanticCacheConfig configures semantic caching
type SemanticCacheConfig struct {
	Enabled              bool    `yaml:"enabled"`
	EmbeddingModel       string  `yaml:"embedding_model"`
	SimilarityThreshold  float64 `yaml:"similarity_threshold"`
	HitRateTarget        float64 `yaml:"hit_rate_target"`
	FalsePositiveLimit   float64 `yaml:"false_positive_limit"`
	MaxCacheSize         int     `yaml:"max_cache_size_mb"`
}

// KnowledgeGraphConfig configures knowledge graph
type KnowledgeGraphConfig struct {
	Enabled          bool   `yaml:"enabled"`
	IndexLocalCode   bool   `yaml:"index_local_code"`
	IndexWebContent  bool   `yaml:"index_web_content"`
	CacheLookups     bool   `yaml:"cache_lookups"`
	DBPath           string `yaml:"db_path"`
}

// InputOptimizationConfig configures input optimization
type InputOptimizationConfig struct {
	Enabled                 bool `yaml:"enabled"`
	StripUnusedTools        bool `yaml:"strip_unused_tools"`
	CompressParameters      bool `yaml:"compress_parameters"`
	DeduplicateExactRequests bool `yaml:"dedup_exact_requests"`
}

// OutputOptimizationConfig configures output optimization
type OutputOptimizationConfig struct {
	Enabled         bool `yaml:"enabled"`
	ResponseCompression bool `yaml:"response_compression"`
	FieldFiltering  bool `yaml:"field_filtering"`
	DeltaDetection  bool `yaml:"delta_detection"`
}

// BatchAPIConfig configures batch processing
type BatchAPIConfig struct {
	Enabled           bool `yaml:"enabled"`
	MinBatchSize      int  `yaml:"min_batch_size"`
	MaxBatchSize      int  `yaml:"max_batch_size"`
	AutoBatchSimilar  bool `yaml:"auto_batch_similar"`
}

// IntentConfig configures intent detection
type IntentConfig struct {
	Enabled              bool     `yaml:"enabled"`
	CacheBypassPatterns []string `yaml:"cache_bypass_patterns"`
	Personalization      PersonalizationConfig `yaml:"personalization"`
}

// PersonalizationConfig configures user preference learning
type PersonalizationConfig struct {
	LearnFromFeedback  bool `yaml:"learn_from_feedback"`
	AdaptPerUser       bool `yaml:"adapt_per_user"`
	FeedbackHistoryDays int `yaml:"feedback_history_depth"`
}

// SecurityConfig configures security validation
type SecurityConfig struct {
	Enabled                  bool `yaml:"enabled"`
	SQLInjectionDetection    bool `yaml:"sql_injection_detection"`
	XSSPrevention            bool `yaml:"xss_prevention"`
	CommandInjectionDetection bool `yaml:"command_injection_detection"`
	RateLimiting             RateLimitConfig `yaml:"rate_limiting"`
	AuditLogging             bool `yaml:"audit_logging"`
}

// RateLimitConfig configures rate limiting
type RateLimitConfig struct {
	RequestsPerMinute int  `yaml:"requests_per_minute"`
	PerIP             bool `yaml:"per_ip"`
}

// MetricsConfig configures metrics collection and publishing
type MetricsConfig struct {
	Enabled   bool              `yaml:"enabled"`
	PublishTo PublishTargets    `yaml:"publish_to"`
	Track     MetricsTracking   `yaml:"track"`
}

// PublishTargets configures where metrics are published
type PublishTargets struct {
	Prometheus PrometheusTarget `yaml:"prometheus"`
	Grafana    GrafanaTarget    `yaml:"grafana"`
	CloudWatch CloudWatchTarget `yaml:"cloudwatch"`
	DebugLogs  DebugLogsTarget  `yaml:"debug_logs"`
}

// PrometheusTarget configures Prometheus export
type PrometheusTarget struct {
	Enabled bool   `yaml:"enabled"`
	Port    int    `yaml:"port"`
	Path    string `yaml:"path"`
}

// GrafanaTarget configures Grafana integration
type GrafanaTarget struct {
	Enabled bool   `yaml:"enabled"`
	URL     string `yaml:"url"`
}

// CloudWatchTarget configures CloudWatch integration
type CloudWatchTarget struct {
	Enabled   bool   `yaml:"enabled"`
	Namespace string `yaml:"namespace"`
}

// DebugLogsTarget configures local debug logging
type DebugLogsTarget struct {
	Enabled bool   `yaml:"enabled"`
	Dir     string `yaml:"dir"`
}

// MetricsTracking configures which metrics to track
type MetricsTracking struct {
	CacheHitRate             MetricConfig `yaml:"cache_hit_rate"`
	CacheFalsePositiveRate   MetricConfig `yaml:"cache_false_positive_rate"`
	TokenSavingsPercent      MetricConfig `yaml:"token_savings_percent"`
	LatencyByLayer           MetricConfig `yaml:"latency_by_layer"`
	PerOptimizationSavings   MetricConfig `yaml:"per_optimization_savings"`
	SecurityEvents           MetricConfig `yaml:"security_events"`
	CostTracking             MetricConfig `yaml:"cost_tracking"`
}

// MetricConfig represents a single metric configuration
type MetricConfig struct {
	Enabled        bool          `yaml:"enabled"`
	Interval       time.Duration `yaml:"interval"`
	AlertIfAbove   *float64      `yaml:"alert_if_above,omitempty"`
	AlertIfBelow   *float64      `yaml:"alert_if_below,omitempty"`
	CustomTags     map[string]string `yaml:"custom_tags,omitempty"`
}
