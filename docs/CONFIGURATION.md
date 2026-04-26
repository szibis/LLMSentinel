# Configuration Guide (v0.5.0)

## Overview

Claude Escalate is configured via `config.yaml`. Default: `~/.claude-escalate/config.yaml`

Settings support **live reload** — changes apply immediately without restart.

## Quick Start

```yaml
gateway:
  port: 8080
  host: 127.0.0.1

claude_api:
  api_key: ${CLAUDE_API_KEY}

cache:
  enabled: true
  semantic_enabled: true

knowledge_graph:
  enabled: true
  index_paths: [./src, ./lib]

metrics:
  prometheus: true
  otel:
    enabled: true
    endpoint: "http://localhost:4317"
```

## Core Configuration Sections

### Gateway

```yaml
gateway:
  host: 127.0.0.1
  port: 8080
  log_level: "info"              # debug, info, warn, error
  request_timeout_seconds: 30
  max_request_body_mb: 10
  
  # TLS (optional)
  tls_enabled: false
  tls_cert_path: ""
  tls_key_path: ""
```

### Claude API

```yaml
claude_api:
  api_key: ${CLAUDE_API_KEY}
  default_model: "claude-3-5-haiku-20241022"
  timeout_seconds: 60
  max_tokens: 4096
  rate_limit_rpm: 60
  max_retries: 3
```

### Cache

```yaml
cache:
  enabled: true
  exact_dedup_enabled: true
  semantic_enabled: true
  semantic_threshold: 0.85            # Cosine similarity (0-1)
  database: "~/.claude-escalate/cache.db"
  max_cache_size_mb: 500
  ttl_hours: 168                      # 7 days
  false_positive_rate_limit: 0.005    # 0.5%
```

### Knowledge Graph

```yaml
knowledge_graph:
  enabled: true
  index_paths: [./src, ./lib, ./app]
  exclude_patterns: [node_modules, vendor, .venv]
  database: "~/.claude-escalate/graph.db"
  enable_file_watching: true
  debounce_ms: 500
  max_traversal_depth: 10
  supported_languages: [go, python, typescript]
```

### Input Optimization

```yaml
input_optimization:
  enabled: true
  layers:
    tool_stripping: true
    parameter_compression: true
    structured_formatting: true
    whitespace_removal: true
  aggressiveness: "moderate"          # conservative, moderate, aggressive
```

### Intent Detection

```yaml
intent_detection:
  enabled: true
  learn_from_feedback: true
  feedback_history_days: 90
  cache_bypass_patterns:
    - "--no-cache"
    - "--fresh"
    - "!"
```

### Security

```yaml
security:
  enabled: true                        # ALWAYS ON
  validate_sql_injection: true
  validate_xss: true
  validate_command_injection: true
  rate_limit_rps: 1000
  rate_limit_per_ip_rpm: 1000
```

### Metrics (Prometheus + OTEL)

```yaml
metrics:
  enabled: true
  collection_interval_seconds: 1
  retention_days: 90
  
  # Export formats
  prometheus: true                    # /metrics endpoint
  
  # OpenTelemetry push (to collector)
  otel:
    enabled: true
    endpoint: "http://localhost:4317"
    service_name: "claude-escalate"
    environment: "production"
    push_interval_seconds: 30
    
    # Optional: TLS
    tls_enabled: false
    
    # Optional: Authentication headers
    headers: {}
      # Authorization: "Bearer YOUR_TOKEN"
```

### Dashboard

```yaml
dashboard:
  enabled: true
  port: 8080
  host: 127.0.0.1
  refresh_interval_ms: 1000
```

## Environment Variables

```bash
export CLAUDE_API_KEY="sk-ant-..."
export OTEL_ENDPOINT="http://otel-collector:4317"
```

```yaml
claude_api:
  api_key: ${CLAUDE_API_KEY}

metrics:
  otel:
    endpoint: ${OTEL_ENDPOINT:-http://localhost:4317}
```

## Examples

### Development

```yaml
gateway:
  log_level: "debug"
  host: 0.0.0.0

cache:
  semantic_threshold: 0.95

knowledge_graph:
  debounce_ms: 200

metrics:
  prometheus: true
```

### Production with OTEL Push

```yaml
metrics:
  prometheus: true
  otel:
    enabled: true
    endpoint: "http://otel-collector:4317"
    service_name: "claude-escalate-prod"
    headers:
      Authorization: "Bearer ${OTEL_TOKEN}"

security:
  rate_limit_per_ip_rpm: 500

gateway:
  tls_enabled: true
  tls_cert_path: /etc/ssl/certs/server.crt
  tls_key_path: /etc/ssl/private/server.key
```

### Strict Security (No Caching)

```yaml
cache:
  enabled: false

knowledge_graph:
  enabled: false

security:
  rate_limit_rps: 100
```

## Management Commands

```bash
# Show configuration
claude-escalate config show

# Validate syntax
claude-escalate config validate

# Update setting (live reload)
claude-escalate config set cache.semantic_threshold 0.90

# View metrics
claude-escalate metrics --now
```

## Prometheus Integration

Scrape metrics from `http://localhost:8080/metrics`:

```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'claude-escalate'
    static_configs:
      - targets: ['localhost:8080']
    metrics_path: '/metrics'
    scrape_interval: 15s
```

## OpenTelemetry Integration

Push metrics to OTEL Collector:

```yaml
# In config.yaml
metrics:
  otel:
    enabled: true
    endpoint: "http://otel-collector:4317"
    push_interval_seconds: 30
```

**Supported OTEL backends**:
- Datadog (via OTLP)
- Prometheus (via OTLP)
- Jaeger
- New Relic
- Any OpenTelemetry Collector

## Troubleshooting

```bash
# Validate configuration
claude-escalate config validate --verbose

# Check Prometheus endpoint
curl http://localhost:8080/metrics | head -20

# Check OTEL connectivity
telnet localhost 4317

# View configuration
claude-escalate config get metrics.otel
```
