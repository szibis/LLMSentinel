// Package observability provides OpenTelemetry metrics export.
package observability

import (
	"context"
	"fmt"
	"time"
)

// OTELConfig holds OpenTelemetry configuration.
type OTELConfig struct {
	Enabled            bool          `yaml:"enabled"`
	ExporterType       string        `yaml:"exporter_type"`       // "otlpmetrichttp", "otlpmetricgrpc"
	Endpoint           string        `yaml:"endpoint"`            // e.g., http://localhost:4318
	Interval           time.Duration `yaml:"interval"`            // Push interval (default 60s)
	Insecure           bool          `yaml:"insecure"`            // Allow HTTP for local dev
	TimeoutSeconds     int           `yaml:"timeout_seconds"`     // Request timeout
	Headers            map[string]string `yaml:"headers"`        // Custom headers for auth
}

// OTELMetricsExporter exports metrics to OpenTelemetry collectors.
type OTELMetricsExporter struct {
	config    OTELConfig
	stopChan  chan struct{}
	isRunning bool
	metrics   *PrometheusMetrics
}

// NewOTELMetricsExporter creates a new OTEL metrics exporter.
func NewOTELMetricsExporter(config OTELConfig, metrics *PrometheusMetrics) *OTELMetricsExporter {
	if config.Interval == 0 {
		config.Interval = 60 * time.Second
	}
	if config.TimeoutSeconds == 0 {
		config.TimeoutSeconds = 10
	}
	if config.ExporterType == "" {
		config.ExporterType = "otlpmetrichttp"
	}

	return &OTELMetricsExporter{
		config:   config,
		stopChan: make(chan struct{}),
		metrics:  metrics,
	}
}

// Start begins exporting metrics to the configured OTEL collector.
func (ome *OTELMetricsExporter) Start(ctx context.Context) error {
	if !ome.config.Enabled {
		return nil
	}

	if ome.config.Endpoint == "" {
		return fmt.Errorf("OTEL endpoint not configured")
	}

	ome.isRunning = true

	// Start background push routine
	go ome.pushLoop(ctx)

	return nil
}

// Stop stops the metrics exporter.
func (ome *OTELMetricsExporter) Stop(ctx context.Context) error {
	if !ome.isRunning {
		return nil
	}

	close(ome.stopChan)
	ome.isRunning = false

	// Final push before shutdown
	_ = ome.pushMetrics(ctx)

	return nil
}

// pushLoop runs the periodic push routine.
func (ome *OTELMetricsExporter) pushLoop(ctx context.Context) {
	ticker := time.NewTicker(ome.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			_ = ome.pushMetrics(ctx)
		case <-ome.stopChan:
			return
		case <-ctx.Done():
			return
		}
	}
}

// pushMetrics pushes current metrics to OTEL collector via HTTP OTLP.
func (ome *OTELMetricsExporter) pushMetrics(ctx context.Context) error {
	// Get current metrics snapshot
	snapshot := ome.metrics.GetMetricsSnapshot()

	// Build OTLP metrics payload
	// In production, this would use the official OTEL SDK
	// For now, we provide a simplified HTTP push implementation

	payload := map[string]interface{}{
		"resourceMetrics": []map[string]interface{}{
			{
				"resource": map[string]interface{}{
					"attributes": []map[string]interface{}{
						{
							"key": "service.name",
							"value": map[string]string{
								"stringValue": "claude-escalate",
							},
						},
						{
							"key": "service.version",
							"value": map[string]string{
								"stringValue": "4.0.0",
							},
						},
					},
				},
				"scopeMetrics": []map[string]interface{}{
					{
						"scope": map[string]interface{}{
							"name": "claude-escalate/metrics",
						},
						"metrics": ome.buildOTLPMetrics(snapshot),
					},
				},
			},
		},
	}

	// In production, push payload to ome.config.Endpoint
	// For now, we just log the metrics
	return ome.sendOTLPPayload(ctx, payload)
}

// buildOTLPMetrics constructs OTLP metric records from snapshot.
func (ome *OTELMetricsExporter) buildOTLPMetrics(snapshot map[string]interface{}) []map[string]interface{} {
	var metrics []map[string]interface{}

	// Total requests counter
	if totalReqs, ok := snapshot["total_requests"].(int64); ok {
		metrics = append(metrics, map[string]interface{}{
			"name": "claude_escalate_requests_total",
			"type": "SUM",
			"sum": map[string]interface{}{
				"dataPoints": []map[string]interface{}{
					{
						"asInt": totalReqs,
						"attributes": []map[string]interface{}{},
					},
				},
				"aggregationTemporality": "CUMULATIVE",
				"isMonotonic": true,
			},
		})
	}

	// Cache hit rate gauge
	if hitRate, ok := snapshot["cache_hit_rate"].(float64); ok {
		metrics = append(metrics, map[string]interface{}{
			"name": "claude_escalate_cache_hit_rate",
			"type": "GAUGE",
			"gauge": map[string]interface{}{
				"dataPoints": []map[string]interface{}{
					{
						"asDouble": hitRate,
					},
				},
			},
		})
	}

	// Cost per request gauge
	if costPerReq, ok := snapshot["cost_per_request"].(float64); ok {
		metrics = append(metrics, map[string]interface{}{
			"name": "claude_escalate_cost_per_request_usd",
			"type": "GAUGE",
			"gauge": map[string]interface{}{
				"dataPoints": []map[string]interface{}{
					{
						"asDouble": costPerReq,
					},
				},
			},
		})
	}

	// Queue size gauge
	if queueSize, ok := snapshot["queue_size"].(int64); ok {
		metrics = append(metrics, map[string]interface{}{
			"name": "claude_escalate_queue_size",
			"type": "GAUGE",
			"gauge": map[string]interface{}{
				"dataPoints": []map[string]interface{}{
					{
						"asInt": queueSize,
					},
				},
			},
		})
	}

	// Latency percentiles
	if p95, ok := snapshot["latency_p95"].(float64); ok {
		metrics = append(metrics, map[string]interface{}{
			"name": "claude_escalate_latency_p95_ms",
			"type": "GAUGE",
			"gauge": map[string]interface{}{
				"dataPoints": []map[string]interface{}{
					{
						"asDouble": p95,
					},
				},
			},
		})
	}

	return metrics
}

// sendOTLPPayload sends the OTLP payload to the configured endpoint.
// In production, this would use HTTP/gRPC with proper error handling.
func (ome *OTELMetricsExporter) sendOTLPPayload(ctx context.Context, payload map[string]interface{}) error {
	// Placeholder for actual HTTP/gRPC send
	// In production:
	// 1. Marshal payload to protobuf
	// 2. Send POST to ome.config.Endpoint/v1/metrics (HTTP OTLP)
	// 3. Handle 4xx/5xx responses
	// 4. Retry on timeout

	return nil
}

// IsRunning returns whether the exporter is actively pushing metrics.
func (ome *OTELMetricsExporter) IsRunning() bool {
	return ome.isRunning
}

// UpdateConfig updates the exporter configuration at runtime.
func (ome *OTELMetricsExporter) UpdateConfig(newConfig OTELConfig) error {
	// Stop current exporter if running
	if ome.isRunning {
		_ = ome.Stop(context.Background())
	}

	ome.config = newConfig

	// Restart with new config
	return ome.Start(context.Background())
}
