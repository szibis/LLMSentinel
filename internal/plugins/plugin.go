package plugins

import (
	"context"
)

// OptimizationPlugin defines the interface for optimization plugins
type OptimizationPlugin interface {
	// Name returns the plugin name
	Name() string

	// Description returns a human-readable description
	Description() string

	// Version returns the plugin version
	Version() string

	// OptimizeInput applies optimization to request before Claude
	OptimizeInput(ctx context.Context, req interface{}) (interface{}, *Metrics, error)

	// OptimizeOutput applies optimization to response after Claude
	OptimizeOutput(ctx context.Context, resp interface{}) (interface{}, *Metrics, error)

	// GetMetrics returns current metrics
	GetMetrics() *PluginMetrics

	// Initialize initializes the plugin with config
	Initialize(config map[string]interface{}) error

	// Close cleans up plugin resources
	Close() error
}

// Metrics represents token/cost savings from an optimization
type Metrics struct {
	TokensIn             int64
	TokensOut            int64
	TokensSaved          int64
	SavingsPercent       float64
	ProcessingTimeMs     int64
	CacheHit             bool
	CacheConfidence      float64
	RecommendedFreshness bool
}

// PluginMetrics represents aggregated plugin metrics
type PluginMetrics struct {
	Name                   string
	TotalRequestsProcessed int64
	TotalTokensSaved       int64
	AverageSavingsPercent  float64
	CacheHitRate           float64
	LastExecutionMs        int64
	Errors                 int64
}

// Config represents plugin configuration
type Config struct {
	Name     string
	Version  string
	Enabled  bool
	Settings map[string]interface{}
}

// RTKOptimizationPlugin represents RTK optimization plugin
type RTKOptimizationPlugin struct {
	name       string
	version    string
	enabled    bool
	savingRate float64
	metrics    *PluginMetrics
}

// NewRTKOptimizationPlugin creates a new RTK plugin
func NewRTKOptimizationPlugin() *RTKOptimizationPlugin {
	return &RTKOptimizationPlugin{
		name:       "rtk-optimizer",
		version:    "1.0.0",
		enabled:    true,
		savingRate: 99.4,
		metrics: &PluginMetrics{
			Name: "rtk-optimizer",
		},
	}
}

// Name returns the plugin name
func (p *RTKOptimizationPlugin) Name() string {
	return p.name
}

// Description returns a human-readable description
func (p *RTKOptimizationPlugin) Description() string {
	return "RTK token optimization for command output compression"
}

// Version returns the plugin version
func (p *RTKOptimizationPlugin) Version() string {
	return p.version
}

// OptimizeInput applies RTK optimization to request
func (p *RTKOptimizationPlugin) OptimizeInput(ctx context.Context, req interface{}) (interface{}, *Metrics, error) {
	// RTK optimization primarily affects output, not input
	return req, &Metrics{}, nil
}

// OptimizeOutput applies RTK compression to output
func (p *RTKOptimizationPlugin) OptimizeOutput(ctx context.Context, resp interface{}) (interface{}, *Metrics, error) {
	// This adapter currently passes through output. Until it measures an
	// actual transformation, no token usage or savings can be attributed.
	metrics := &Metrics{}

	p.metrics.TotalRequestsProcessed++

	return resp, metrics, nil
}

// GetMetrics returns current metrics
func (p *RTKOptimizationPlugin) GetMetrics() *PluginMetrics {
	return p.metrics
}

// Initialize initializes the plugin
func (p *RTKOptimizationPlugin) Initialize(config map[string]interface{}) error {
	if enabled, ok := config["enabled"].(bool); ok {
		p.enabled = enabled
	}
	if rate, ok := config["saving_rate"].(float64); ok {
		p.savingRate = rate
	}
	return nil
}

// Close cleans up plugin resources
func (p *RTKOptimizationPlugin) Close() error {
	return nil
}
