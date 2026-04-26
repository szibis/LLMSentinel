package metrics

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/szibis/claude-escalate/internal/config"
)

// OpenTelemetryExporter exports metrics via OpenTelemetry protocol
type OpenTelemetryExporter struct {
	collector      *MetricsCollector
	config         *config.OpenTelemetryTarget
	client         *http.Client
	mu             sync.RWMutex
	batch          []OTelMetric
	batchMu        sync.Mutex
	lastFlushTime  time.Time
	lastFlushCount int
}

// OTelMetric represents a single OpenTelemetry metric
type OTelMetric struct {
	Name       string            `json:"name"`
	Type       string            `json:"type"`       // counter, gauge, histogram
	Value      float64           `json:"value"`
	Timestamp  int64             `json:"timestamp_ms"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// OTelPayload represents the complete OTLP payload
type OTelPayload struct {
	ServiceName    string       `json:"service_name"`
	ServiceVersion string       `json:"service_version"`
	Environment    string       `json:"environment"`
	Metrics        []OTelMetric `json:"metrics"`
	Timestamp      int64        `json:"timestamp_ms"`
}

// NewOpenTelemetryExporter creates a new OpenTelemetry exporter
func NewOpenTelemetryExporter(collector *MetricsCollector, cfg *config.OpenTelemetryTarget) *OpenTelemetryExporter {
	if cfg == nil || !cfg.Enabled {
		return nil
	}

	// Default values
	if cfg.ServiceName == "" {
		cfg.ServiceName = "claude-escalate"
	}
	if cfg.ServiceVersion == "" {
		cfg.ServiceVersion = "v4.1.0"
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 512
	}
	if cfg.BatchTimeout <= 0 {
		cfg.BatchTimeout = 5000
	}
	if cfg.ExporterType == "" {
		cfg.ExporterType = "otlp"
	}

	return &OpenTelemetryExporter{
		collector:     collector,
		config:        cfg,
		client:        &http.Client{Timeout: 10 * time.Second},
		batch:         make([]OTelMetric, 0, cfg.BatchSize),
		lastFlushTime: time.Now(),
	}
}

// Export exports metrics to OpenTelemetry endpoint
func (oe *OpenTelemetryExporter) Export() error {
	if oe == nil || !oe.config.Enabled {
		return nil
	}

	oe.mu.RLock()
	snapshot := oe.collector.GetMetrics()
	oe.mu.RUnlock()

	metrics := oe.snapshotToMetrics(snapshot)
	if len(metrics) > 0 {
		return oe.send(metrics)
	}
	return nil
}

// AddMetric adds a metric to the batch
func (oe *OpenTelemetryExporter) AddMetric(metric OTelMetric) error {
	if oe == nil || !oe.config.Enabled {
		return nil
	}

	oe.batchMu.Lock()
	defer oe.batchMu.Unlock()

	oe.batch = append(oe.batch, metric)
	oe.lastFlushCount++

	// Flush if batch size reached or timeout exceeded
	if len(oe.batch) >= oe.config.BatchSize ||
		time.Since(oe.lastFlushTime) > time.Duration(oe.config.BatchTimeout)*time.Millisecond {
		oe.flushBatch()
	}

	return nil
}

// flushBatch sends accumulated metrics to OTEL endpoint (non-blocking)
func (oe *OpenTelemetryExporter) flushBatch() {
	if len(oe.batch) == 0 {
		return
	}

	payload := &OTelPayload{
		ServiceName:    oe.config.ServiceName,
		ServiceVersion: oe.config.ServiceVersion,
		Environment:    oe.config.Environment,
		Metrics:        oe.batch,
		Timestamp:      time.Now().UnixMilli(),
	}

	oe.batch = make([]OTelMetric, 0, oe.config.BatchSize)
	oe.lastFlushTime = time.Now()
	oe.lastFlushCount = 0

	// Send in background to avoid blocking
	go func() {
		_ = oe.sendPayload(payload)
	}()
}

// snapshotToMetrics converts MetricSnapshot to OTelMetrics
func (oe *OpenTelemetryExporter) snapshotToMetrics(snapshot MetricSnapshot) []OTelMetric {
	metrics := make([]OTelMetric, 0)
	now := time.Now().UnixMilli()

	// Cache metrics
	if snapshot.CacheMetrics != nil {
		metrics = append(metrics, OTelMetric{
			Name:      "claude_escalate_cache_hit_rate",
			Type:      "gauge",
			Value:     snapshot.CacheMetrics.HitRate,
			Timestamp: now,
		})

		metrics = append(metrics, OTelMetric{
			Name:      "claude_escalate_cache_hits_total",
			Type:      "counter",
			Value:     float64(snapshot.CacheMetrics.TotalHits),
			Timestamp: now,
		})
	}

	// Token metrics
	if snapshot.TokenMetrics != nil {
		metrics = append(metrics, OTelMetric{
			Name:      "claude_escalate_tokens_saved_total",
			Type:      "counter",
			Value:     float64(snapshot.TokenMetrics.TokensSavedByOptimization),
			Timestamp: now,
		})

		metrics = append(metrics, OTelMetric{
			Name:      "claude_escalate_token_savings_percent",
			Type:      "gauge",
			Value:     snapshot.TokenMetrics.SavingsPercent,
			Timestamp: now,
		})
	}

	// Latency metrics
	if snapshot.LatencyMetrics != nil && snapshot.LatencyMetrics.TotalMs > 0 {
		metrics = append(metrics, OTelMetric{
			Name:      "claude_escalate_latency_total_ms",
			Type:      "gauge",
			Value:     snapshot.LatencyMetrics.TotalMs,
			Timestamp: now,
		})

		metrics = append(metrics, OTelMetric{
			Name:      "claude_escalate_cache_lookup_latency_ms",
			Type:      "gauge",
			Value:     snapshot.LatencyMetrics.CacheLookupMs,
			Timestamp: now,
		})

		metrics = append(metrics, OTelMetric{
			Name:      "claude_escalate_security_validation_latency_ms",
			Type:      "gauge",
			Value:     snapshot.LatencyMetrics.SecurityValidationMs,
			Timestamp: now,
		})
	}

	// Security metrics
	if snapshot.SecurityMetrics != nil && snapshot.SecurityMetrics.InjectionAttemptsBlocked > 0 {
		metrics = append(metrics, OTelMetric{
			Name:      "claude_escalate_security_injections_blocked",
			Type:      "counter",
			Value:     float64(snapshot.SecurityMetrics.InjectionAttemptsBlocked),
			Timestamp: now,
		})
	}

	return metrics
}

// sendPayload sends the OTEL payload to the configured endpoint
func (oe *OpenTelemetryExporter) sendPayload(payload *OTelPayload) error {
	if oe == nil || !oe.config.Enabled {
		return nil
	}

	// Marshal payload
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal OTEL payload: %w", err)
	}

	// Determine endpoint based on exporter type
	var endpoint string
	switch oe.config.ExporterType {
	case "otlp":
		endpoint = oe.config.OTLPEndpoint
		if endpoint == "" {
			endpoint = "http://localhost:4317"
		}
		endpoint += "/v1/metrics"
	case "jaeger":
		endpoint = oe.config.JaegerEndpoint
		if endpoint == "" {
			endpoint = "http://localhost:14268/api/traces"
		}
	default:
		endpoint = oe.config.OTLPEndpoint
		if endpoint == "" {
			endpoint = "http://localhost:4317/v1/metrics"
		}
	}

	// Create request
	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create OTEL request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Add custom headers (e.g., API keys)
	for key, value := range oe.config.Headers {
		req.Header.Set(key, value)
	}

	// Send request
	resp, err := oe.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send OTEL metrics: %w", err)
	}
	defer resp.Body.Close()

	// Read response for debugging
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("OTEL export failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// send is a helper that sends metrics in the appropriate format
func (oe *OpenTelemetryExporter) send(metrics []OTelMetric) error {
	payload := &OTelPayload{
		ServiceName:    oe.config.ServiceName,
		ServiceVersion: oe.config.ServiceVersion,
		Environment:    oe.config.Environment,
		Metrics:        metrics,
		Timestamp:      time.Now().UnixMilli(),
	}

	return oe.sendPayload(payload)
}
