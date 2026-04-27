# Metrics Configuration Guide

## Prometheus Scrape Configuration

Add to `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'claude-escalate'
    static_configs:
      - targets: ['localhost:8080']
    metrics_path: '/metrics'
    scrape_interval: 30s
    scrape_timeout: 10s
    
    # Optional: Custom relabeling for environment/region
    relabel_configs:
      - source_labels: [__address__]
        regex: '([^:]+)(?::\d+)?'
        target_label: 'instance'
        replacement: '${1}'
```

---

## OpenTelemetry Collector Configuration

### OTLP Receiver (HTTP/gRPC)

Create `otel-collector-config.yaml`:

```yaml
receivers:
  # HTTP receiver for OTLP metrics
  otlp:
    protocols:
      http:
        endpoint: 0.0.0.0:4318
      grpc:
        endpoint: 0.0.0.0:4317

processors:
  # Batch processor for efficiency
  batch:
    send_batch_size: 512
    timeout: 5s
    
  # Resource detection for auto-labeling
  resource_detection:
    detectors: [env, system, docker]
    timeout: 2s
    override: true

exporters:
  # Export to Prometheus
  prometheus:
    endpoint: "0.0.0.0:8888"
  
  # Export to Jaeger (optional)
  jaeger:
    endpoint: "jaeger:14250"
    tls:
      insecure: true
  
  # Export to Datadog (optional)
  datadog:
    api:
      key: ${DATADOG_API_KEY}
      site: datadoghq.com
  
  # Export to CloudWatch (optional)
  awsemf:
    log_group_name: /aws/lambda/claude-escalate
    log_stream_name: metrics

service:
  pipelines:
    # Metrics pipeline
    metrics:
      receivers: [otlp]
      processors: [batch, resource_detection]
      exporters: [prometheus, jaeger, datadog]
```

Start OTEL Collector:

```bash
docker run -d \
  --name otel-collector \
  -p 4317:4317 \
  -p 4318:4318 \
  -p 8888:8888 \
  -v $(pwd)/otel-collector-config.yaml:/etc/otel-collector-config.yaml \
  otel/opentelemetry-collector:latest \
  --config=/etc/otel-collector-config.yaml
```

---

## Claude Escalate Configuration

Add to `config.yaml`:

```yaml
metrics:
  enabled: true
  
  # Prometheus endpoint (gateway exports to this)
  prometheus:
    enabled: true
    port: 8080
    path: /metrics
    
  # OpenTelemetry export
  opentelemetry:
    enabled: true
    service_name: claude-escalate
    service_version: v4.1.0
    environment: production
    
    # Export to OTLP collector
    exporter_type: otlp
    otlp_endpoint: http://localhost:4318
    
    # Batch settings
    batch_size: 512
    batch_timeout: 5000  # milliseconds
    
    # Custom headers (API keys, etc)
    headers:
      authorization: "Bearer YOUR_TOKEN"
    
  # Prometheus scrape target (if running separately)
  prometheus_target:
    enabled: false
    url: http://prometheus:9090
    job_name: claude-escalate
    scrape_interval: 30s
  
  # Datadog export (optional)
  datadog:
    enabled: false
    api_key: ${DATADOG_API_KEY}
    site: datadoghq.com
  
  # CloudWatch export (optional)
  cloudwatch:
    enabled: false
    log_group: /aws/lambda/claude-escalate
    namespace: claude-escalate
```

---

## Grafana Dashboards

### Import Dashboard JSON

```json
{
  "dashboard": {
    "title": "Claude Escalate Metrics",
    "panels": [
      {
        "title": "Cache Hit Rate by Layer",
        "targets": [
          {
            "expr": "claude_escalate_cache_hit_rate",
            "legendFormat": "{{ layer }}"
          }
        ]
      },
      {
        "title": "Token Savings by Layer",
        "targets": [
          {
            "expr": "sum(claude_escalate_tokens_total{type=\"saved\"}) by (layer)"
          }
        ]
      },
      {
        "title": "Latency Percentiles",
        "targets": [
          {
            "expr": "claude_escalate_latency_seconds",
            "legendFormat": "{{ stage }} p{{ quantile }}"
          }
        ]
      },
      {
        "title": "Security Events",
        "targets": [
          {
            "expr": "sum(claude_escalate_security_events_total) by (type, pattern)"
          }
        ]
      },
      {
        "title": "Cost Breakdown",
        "targets": [
          {
            "expr": "sum(claude_escalate_cost_usd_total) by (type, model)"
          }
        ]
      },
      {
        "title": "Request Success Rate",
        "targets": [
          {
            "expr": "sum(claude_escalate_requests_total{status=\"success\"}) / sum(claude_escalate_requests_total)"
          }
        ]
      },
      {
        "title": "Gateway Status",
        "targets": [
          {
            "expr": "claude_escalate_gateway_status",
            "legendFormat": "{{ component }}"
          }
        ]
      }
    ]
  }
}
```

### Key Queries for Dashboards

**Cache Performance**:
```promql
# Hit rate by layer
claude_escalate_cache_hit_rate{layer!="overall"}

# False positive rate trend
rate(claude_escalate_cache_operations_total{operation="false_positive"}[5m])
```

**Token Savings**:
```promql
# Total savings percentage
sum(claude_escalate_tokens_total{type="saved"}) / (sum(claude_escalate_tokens_total{type="input"}) + sum(claude_escalate_tokens_total{type="output"})) * 100

# Breakdown by layer
sum(claude_escalate_tokens_total{type="saved"}) by (layer)

# Cost comparison
{type="burned"} vs {type="saved"} [as bar chart]
```

**Latency**:
```promql
# p95 latency by stage
histogram_quantile(0.95, claude_escalate_latency_seconds{quantile="0.95"})

# Total latency over time
claude_escalate_latency_seconds{stage="total", quantile="0.99"}
```

**Quality**:
```promql
# Quality dimensions
claude_escalate_quality_score

# Security events over time
sum(rate(claude_escalate_security_events_total[5m])) by (type)
```

---

## Alert Rules

Create `alert-rules.yml`:

```yaml
groups:
  - name: claude-escalate
    rules:
      # Cache false positives too high
      - alert: CacheFalsePositiveRateTooHigh
        expr: claude_escalate_cache_false_positive_rate{layer="semantic"} > 0.005
        for: 5m
        annotations:
          summary: "Cache false positive rate {{ $value | humanizePercentage }}"
          action: "Disable semantic cache or lower threshold"
      
      # Token savings dropped
      - alert: TokenSavingsDropped
        expr: |
          sum(claude_escalate_tokens_total{type="saved"}) / 
          (sum(claude_escalate_tokens_total{type="input"}) + sum(claude_escalate_tokens_total{type="output"})) < 0.50
        for: 10m
        annotations:
          summary: "Token savings only {{ $value | humanizePercentage }}"
          action: "Investigate optimization effectiveness"
      
      # Latency p99 too high
      - alert: HighLatencyP99
        expr: claude_escalate_latency_seconds{stage="total", quantile="0.99"} > 0.3
        for: 5m
        annotations:
          summary: "p99 latency {{ $value }}s (target: <0.2s)"
          action: "Profile and optimize slowest path"
      
      # Security events spiking
      - alert: SecurityEventSpike
        expr: |
          rate(claude_escalate_security_events_total{type="injection_blocked"}[5m]) > 10
        for: 2m
        annotations:
          summary: "{{ $value | humanize }} injection attempts/sec"
          action: "Check for attack patterns"
      
      # Gateway component down
      - alert: GatewayComponentDown
        expr: claude_escalate_gateway_status == 0
        for: 1m
        annotations:
          summary: "Component {{ $labels.component }} is down"
          action: "Check service logs and restart if needed"
```

Load rules into Prometheus:

```yaml
rule_files:
  - /path/to/alert-rules.yml
```

---

## Label Cardinality Summary

Total unique metric combinations: ~127

**By metric**:
- `cache_hit_rate`: 4 values (layer: exact, semantic, graph, overall)
- `tokens_total`: 7 values (layer: exact, semantic, rtk, graph, input_opt, output_opt, batch)
- `cost_usd_total`: 6 values (type: burned/saved × model: haiku/sonnet/opus)
- `latency_seconds`: 12 values (stage: cache_lookup/security/total × quantile: 0.50/0.95/0.99)
- `requests_total`: 6 values (status: success/cached/fresh × intent: quick/detailed/routine)
- `security_events_total`: 6 values (type: injection/rate_limit/validation × pattern: sql/xss/other)
- `quality_score`: 3 values (dimension: accuracy/false_positives/satisfaction)
- `gateway_status`: 6 values (component: cache/security/intent/optimizer/api/dashboard)

**No unbounded cardinality**: user_id, request_id, or query_hash labels avoided

---

## Performance Impact

**Prometheus scrape**:
- Metrics size: ~4KB (text format)
- Scrape time: <50ms at 30s interval
- Storage: ~1.4GB/year (2 weeks retention, daily cleanup)

**OTEL export**:
- Batch size: 512 metrics max
- Network: ~50KB per batch
- Latency: <100ms for batch send

**Gateway overhead**:
- Metrics collection: <1ms per request
- Prometheus export: <5ms per scrape
- OTEL batch flush: <50ms (non-blocking)

