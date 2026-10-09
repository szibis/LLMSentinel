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
	Type       string            `json:"type"` // counter, gauge, histogram
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

// snapshotToMetrics converts MetricSnapshot to OTelMetrics with label-based cardinality control
func (oe *OpenTelemetryExporter) snapshotToMetrics(snapshot MetricSnapshot) []OTelMetric {
	metrics := make([]OTelMetric, 0)
	now := time.Now().UnixMilli()

	add := func(name, kind string, value float64, labels map[string]string) {
		metrics = append(metrics, OTelMetric{Name: name, Type: kind, Value: value, Timestamp: now, Attributes: labels})
	}
	if c := snapshot.CacheMetrics; c != nil {
		add("cache_hit_rate", "gauge", c.HitRate, map[string]string{"layer": "overall"})
		add("cache_false_positive_rate", "gauge", c.FalsePositiveRate, map[string]string{"layer": "overall"})
		for operation, count := range map[string]int64{"hit": c.TotalHits, "miss": c.TotalMisses, "false_positive": c.FalsePositives} {
			add("cache_operations_total", "counter", float64(count), map[string]string{"layer": "overall", "operation": operation, "unit": "count"})
		}
	}
	if t := snapshot.TokenMetrics; t != nil {
		for kind, count := range map[string]int64{"input": t.TotalInputTokens, "output": t.TotalOutputTokens, "saved": t.TokensSavedByOptimization} {
			add("tokens_total", "counter", float64(count), map[string]string{"type": kind, "unit": "tokens"})
		}
		add("token_savings_percent", "gauge", t.SavingsPercent*100, map[string]string{"aggregation": "overall", "unit": "percent"})
	}
	if l := snapshot.LatencyMetrics; l != nil {
		for stage, ms := range map[string]float64{"cache_lookup": l.CacheLookupMs, "security_validation": l.SecurityValidationMs, "intent_detection": l.IntentDetectionMs, "optimization": l.OptimizationMs, "claude_api": l.ClaudeAPICallMs, "response_compression": l.ResponseCompressionMs, "total": l.TotalMs} {
			add("latency_seconds", "gauge", ms/1000, map[string]string{"stage": stage, "unit": "seconds"})
		}
	}
	add("requests_total", "counter", float64(snapshot.RequestCount), map[string]string{"unit": "count"})
	if s := snapshot.SecurityMetrics; s != nil {
		for kind, count := range map[string]int64{"injection_blocked": s.InjectionAttemptsBlocked, "rate_limit": s.RateLimitTriggered, "validation_failure": s.ValidationFailures, "unauthorized": s.UnauthorizedAttempts} {
			add("security_events_total", "counter", float64(count), map[string]string{"type": kind, "unit": "count"})
		}
	}
	add("uptime_seconds", "gauge", oe.collector.Uptime().Seconds(), map[string]string{"unit": "seconds"})

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
