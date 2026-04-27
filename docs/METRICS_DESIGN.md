# Claude Escalate Metrics Design (OTEL/Prometheus)

## Philosophy: Labels Not Metric Explosion

**Problem**: Creating separate metrics for each component leads to cardinality explosion.
- ❌ Bad: `cache_hit_rate_exact`, `cache_hit_rate_semantic`, `cache_hit_rate_graph` = 3 metrics
- ✅ Good: `cache_hit_rate{layer="exact"}`, `cache_hit_rate{layer="semantic"}` = 1 metric, 2 label values

**Rule**: Core metric names only. Dimensions added via labels.

---

## Core Metrics (Minimal Set)

### 1. Cache Metrics
```
claude_escalate_cache_operations_total (counter)
  Labels: layer (exact, semantic, graph), operation (hit, miss, false_positive)
  Example: claude_escalate_cache_operations_total{layer="semantic", operation="hit"} 450

claude_escalate_cache_hit_rate (gauge)
  Labels: layer (exact, semantic, graph, overall)
  Range: 0.0-1.0
  Example: claude_escalate_cache_hit_rate{layer="semantic"} 0.52

claude_escalate_cache_false_positive_rate (gauge)
  Labels: layer (semantic, graph)
  Range: 0.0-1.0
  Example: claude_escalate_cache_false_positive_rate{layer="semantic"} 0.0008
```

### 2. Token Metrics
```
claude_escalate_tokens_total (counter)
  Labels: type (input, output, cache_read, cache_write, saved), layer (exact, semantic, rtk, graph, input_opt, output_opt, batch)
  Example: claude_escalate_tokens_total{type="saved", layer="semantic"} 180000
           claude_escalate_tokens_total{type="input"} 1000000

claude_escalate_token_savings_percent (gauge)
  Labels: aggregation (overall, layer), layer (exact, semantic, rtk, graph, input_opt, output_opt, batch, compound)
  Range: 0.0-100.0
  Example: claude_escalate_token_savings_percent{aggregation="overall"} 65.2
           claude_escalate_token_savings_percent{aggregation="layer", layer="semantic"} 12.8
```

### 3. Cost Metrics
```
claude_escalate_cost_usd_total (counter)
  Labels: type (burned, saved), model (haiku, sonnet, opus), phase (request, response)
  Example: claude_escalate_cost_usd_total{type="burned", model="sonnet"} 15.50
           claude_escalate_cost_usd_total{type="saved"} 4.50

claude_escalate_cost_per_request_usd (gauge)
  Labels: model (haiku, sonnet, opus)
  Example: claude_escalate_cost_per_request_usd{model="haiku"} 0.0005
```

### 4. Latency Metrics (Histogram + Percentiles)
```
claude_escalate_latency_seconds (histogram)
  Labels: stage (cache_lookup, security_validation, intent_detection, optimization, claude_api, response_compression, total), percentile (p50, p95, p99, p99_9)
  Example: claude_escalate_latency_seconds_bucket{stage="cache_lookup", le="0.010"} 450
           claude_escalate_latency_seconds{stage="cache_lookup", quantile="0.95"} 0.008
```

### 5. Quality Metrics
```
claude_escalate_quality_score (gauge)
  Labels: dimension (accuracy, false_positives, user_satisfaction)
  Range: 0.0-1.0
  Example: claude_escalate_quality_score{dimension="accuracy"} 0.996
           claude_escalate_quality_score{dimension="false_positives"} 0.999

claude_escalate_cache_confidence (gauge)
  Labels: layer (semantic, graph)
  Range: 0.0-1.0
  Example: claude_escalate_cache_confidence{layer="semantic"} 0.95
```

### 6. Request Metrics
```
claude_escalate_requests_total (counter)
  Labels: status (success, error, cached, fresh), intent (quick, detailed, routine, learning, follow_up), model (haiku, sonnet, opus)
  Example: claude_escalate_requests_total{status="success", intent="quick", model="haiku"} 300
           claude_escalate_requests_total{status="cached"} 450

claude_escalate_request_duration_seconds (histogram)
  Labels: intent (quick, detailed, routine, learning, follow_up), cached (true, false)
  Example: claude_escalate_request_duration_seconds_bucket{intent="quick", cached="true", le="0.1"} 200
```

### 7. Security Metrics
```
claude_escalate_security_events_total (counter)
  Labels: type (injection_blocked, rate_limit, validation_failure, unauthorized), pattern (sql, xss, cmd, other)
  Example: claude_escalate_security_events_total{type="injection_blocked", pattern="sql"} 12
           claude_escalate_security_events_total{type="rate_limit"} 3

claude_escalate_security_alerts_total (counter)
  Labels: severity (low, medium, high, critical), category (false_positive_spike, savings_drop, latency_spike, error_rate)
  Example: claude_escalate_security_alerts_total{severity="high", category="false_positive_spike"} 1
```

### 8. Operational Metrics
```
claude_escalate_gateway_status (gauge)
  Labels: component (cache, security, intent, optimizer, claude_api, dashboard)
  Values: 0 (down), 1 (up)
  Example: claude_escalate_gateway_status{component="cache"} 1

claude_escalate_uptime_seconds (counter)
  No labels
  Example: claude_escalate_uptime_seconds 86400

claude_escalate_memory_bytes (gauge)
  Labels: type (heap, cache, embeddings)
  Example: claude_escalate_memory_bytes{type="cache"} 52428800
```

---

## Label Dimensions

### By Component/Layer
- **layer**: exact, semantic, rtk, graph, input_opt, output_opt, batch, security, intent

### By Type/Category
- **type**: input, output, cache_read, cache_write, saved, burned, hit, miss
- **status**: success, error, cached, fresh, partial
- **intent**: quick, detailed, routine, learning, follow_up
- **model**: haiku, sonnet, opus
- **severity**: low, medium, high, critical
- **dimension**: accuracy, false_positives, user_satisfaction

### By Cardinality Control
- Keep label cardinality <10 per dimension
- Use aggregation labels sparingly (overall, layer, compound)
- Avoid unbounded dimensions (user_id, request_id, query_hash)

---

## Histogram Metrics (For Percentiles)

Use histograms with buckets instead of multiple gauge metrics:

```go
// Instead of:
latency_cache_lookup_p50
latency_cache_lookup_p95
latency_cache_lookup_p99

// Use:
latency_seconds{stage="cache_lookup", quantile="0.50"}
latency_seconds{stage="cache_lookup", quantile="0.95"}
latency_seconds{stage="cache_lookup", quantile="0.99"}

// Or histogram buckets:
latency_seconds_bucket{stage="cache_lookup", le="0.005"} 200
latency_seconds_bucket{stage="cache_lookup", le="0.010"} 400
latency_seconds_bucket{stage="cache_lookup", le="0.050"} 450
latency_seconds_count{stage="cache_lookup"} 450
latency_seconds_sum{stage="cache_lookup"} 2.3
```

---

## OTEL Attributes (Labels)

Map Prometheus labels to OTEL attributes:

```go
type MetricWithAttributes struct {
  Name       string            // e.g., "cache_hit_rate"
  Type       string            // counter, gauge, histogram
  Value      float64
  Attributes map[string]string // {layer: "semantic", aggregation: "overall"}
  Unit       string            // tokens, seconds, percent, usd
  Timestamp  int64
}

// Example emission:
collector.RecordMetric(&MetricWithAttributes{
  Name:  "cache_hit_rate",
  Type:  "gauge",
  Value: 0.52,
  Attributes: map[string]string{
    "layer": "semantic",
    "aggregation": "overall",
  },
  Unit: "percent",
})
```

---

## Query Examples (Prometheus)

```promql
# Cache hit rate for semantic cache
claude_escalate_cache_hit_rate{layer="semantic"}

# Total tokens saved by all layers
sum(claude_escalate_tokens_total{type="saved"}) by (layer)

# Cost savings by model
sum(claude_escalate_cost_usd_total{type="saved"}) by (model)

# p95 latency for fresh requests
histogram_quantile(0.95, claude_escalate_request_duration_seconds{cached="false"})

# Injection attacks blocked per pattern
sum(claude_escalate_security_events_total{type="injection_blocked"}) by (pattern)

# Query: Show all metrics by layer (no explosion)
claude_escalate_tokens_total{type="saved"}
```

---

## Scrape Configuration (Prometheus)

```yaml
scrape_configs:
  - job_name: 'claude-escalate'
    static_configs:
      - targets: ['localhost:8080']
    metrics_path: '/metrics'
    scrape_interval: 30s
    scrape_timeout: 10s
```

---

## OTEL Collector Configuration

```yaml
receivers:
  otlp:
    protocols:
      http:
        endpoint: 0.0.0.0:4318

processors:
  batch:
    send_batch_size: 512
    timeout: 5s

exporters:
  prometheus:
    endpoint: "0.0.0.0:8888"
  jaeger:
    endpoint: "jaeger:14250"

service:
  pipelines:
    metrics:
      receivers: [otlp]
      processors: [batch]
      exporters: [prometheus, jaeger]
```

---

## Cardinality Analysis

**Total label combinations estimate:**

```
cache_hit_rate: 4 values (exact, semantic, graph, overall) = 4
tokens_total: 7 layers × 5 types = 35
cost_usd_total: 3 models × 2 types × 2 phases = 12
latency_seconds: 6 stages × 4 percentiles = 24
requests_total: 3 intents × 2 cached × 3 models × 2 status = 36
security_events: 4 types × 4 patterns = 16

Total cardinality: ~127 unique combinations (vs 1000+ with metric explosion)
```

---

## Migration Path

1. Keep existing metrics for backward compatibility
2. Add new label-based metrics in parallel
3. Deprecate old metric names (30-day warning)
4. Sunset old metrics (v0.7.0+)

---

## Dashboard Integration (Grafana)

Key panels should use label queries:
- Cache hit rate by layer (group by `layer`)
- Token savings breakdown (group by `layer`)
- Latency percentiles (histogram quantile queries)
- Cost by model (group by `model`)
- Security events by type (group by `type`)

---

## Example: Cost Tracking Query

```promql
# Actual cost vs projected cost
(sum(claude_escalate_cost_usd_total{type="burned"}) by (model)) /
(sum(rate(claude_escalate_requests_total{status="success"}[5m])) by (model))
= cost per request by model
```

