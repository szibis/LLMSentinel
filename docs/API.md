# Claude Escalate v4.0.0 - API Specification

## Overview

Complete REST API specification for Claude Escalate v4.0.0. All endpoints require no authentication for local deployment, support JSON request/response format, and follow HTTP conventions.

---

## Base URL

```
http://localhost:9000/api
```

## Health & Status

### GET /health
Returns service health status.

**Response** (200 OK):
```json
{
  "status": "healthy",
  "uptime_seconds": 3600,
  "version": "4.0.0",
  "timestamp": "2026-04-26T12:00:00Z"
}
```

---

## Classification API

### POST /classify/predict
Classify a prompt into a task type using ML embeddings.

**Request**:
```json
{
  "prompt": "race condition deadlock concurrent",
  "use_fallback": false
}
```

**Response** (200 OK):
```json
{
  "task_type": "concurrency",
  "confidence": 0.92,
  "embedding_dims": 384,
  "fallback_used": false
}
```

**Status Codes**:
- 200: Successfully classified
- 400: Invalid prompt (empty or >10000 chars)
- 500: Internal error

---

## Analytics API

### GET /analytics/timeseries
Retrieve time-series metrics for a specific bucket.

**Query Parameters**:
- `bucket` (string, required): "hourly", "daily", or "weekly"
- `days` (integer, optional): Days to look back (default: 7)

**Response** (200 OK):
```json
[
  {
    "timestamp": "2026-04-26T00:00:00Z",
    "bucket": "daily",
    "total_requests": 1000,
    "cache_hits": 400,
    "cache_misses": 600,
    "batch_queued": 150,
    "direct_requests": 250,
    "total_cost_usd": 15.50,
    "success_rate": 0.95,
    "avg_latency_ms": 125.3,
    "p50_latency_ms": 100.0,
    "p95_latency_ms": 250.5,
    "p99_latency_ms": 500.0
  }
]
```

**Example**:
```bash
curl "http://localhost:9000/api/analytics/timeseries?bucket=daily&days=30"
```

---

### GET /analytics/percentiles
Retrieve latency percentile distributions.

**Query Parameters**:
- `bucket` (string, optional): "hourly" or "daily" (default: "daily")
- `days` (integer, optional): Days to look back (default: 7)

**Response** (200 OK):
```json
{
  "overall": {
    "p50": 100.5,
    "p75": 150.3,
    "p90": 250.0,
    "p95": 400.5,
    "p99": 750.0,
    "min": 10.0,
    "max": 2000.0,
    "mean": 175.0,
    "std_dev": 250.0,
    "sample_count": 10000
  },
  "by_model": {
    "haiku": { "p50": 50.0, "p95": 200.0, ... },
    "sonnet": { "p50": 100.0, "p95": 300.0, ... },
    "opus": { "p50": 150.0, "p95": 500.0, ... }
  },
  "by_task": {
    "concurrency": { "p50": 120.0, "p95": 350.0, ... },
    "parsing": { "p50": 80.0, "p95": 150.0, ... }
  }
}
```

---

### GET /analytics/forecast
Generate cost forecast with confidence intervals.

**Query Parameters**:
- `metric` (string, optional): "total_cost_usd" (default)
- `days` (integer, optional): Days to forecast (default: 7)

**Response** (200 OK):
```json
{
  "metric": "total_cost_usd",
  "forecasts": [
    {
      "timestamp": "2026-04-27T00:00:00Z",
      "point": 16.50,
      "lower_bound": 14.20,
      "upper_bound": 18.80
    },
    {
      "timestamp": "2026-04-28T00:00:00Z",
      "point": 17.25,
      "lower_bound": 14.75,
      "upper_bound": 19.75
    }
  ],
  "model_quality": {
    "rmse": 0.85,
    "r_squared": 0.92,
    "slope": 0.75,
    "intercept": 10.5
  }
}
```

---

### GET /analytics/task-accuracy
Retrieve per-task-per-model accuracy metrics.

**Query Parameters**:
- `days` (integer, optional): Days to look back (default: 30)
- `min_samples` (integer, optional): Minimum samples required (default: 5)

**Response** (200 OK):
```json
[
  {
    "task_type": "concurrency",
    "model": "sonnet",
    "success_count": 95,
    "total_count": 100,
    "success_rate": 0.95,
    "avg_token_error": 0.08,
    "avg_latency_ms": 125.5
  },
  {
    "task_type": "parsing",
    "model": "haiku",
    "success_count": 88,
    "total_count": 100,
    "success_rate": 0.88,
    "avg_token_error": 0.12,
    "avg_latency_ms": 75.3
  }
]
```

**Sorting**: Results sorted by task_type then success_rate descending

---

### GET /analytics/correlations
Analyze statistical correlations between metrics.

**Response** (200 OK):
```json
[
  {
    "variable1": "success_rate",
    "variable2": "latency_ms",
    "coefficient": -0.45,
    "p_value": 0.001,
    "significant": true
  },
  {
    "variable1": "cache_hits",
    "variable2": "actual_cost_usd",
    "coefficient": -0.89,
    "p_value": 0.0001,
    "significant": true
  }
]
```

**Note**: Only significant correlations (p < 0.05) returned

---

## Configuration API

### GET /config
Retrieve current configuration.

**Response** (200 OK):
```json
{
  "cache": {
    "enabled": true,
    "ttl_seconds": 604800,
    "similarity_threshold": 0.85,
    "max_entries": 10000
  },
  "batch": {
    "enabled": true,
    "strategy": "auto",
    "min_size": 3,
    "max_wait_seconds": 300,
    "min_savings_percent": 5.0
  },
  "budgets": {
    "daily_limit": 10.00,
    "weekly_limit": 50.00,
    "monthly_limit": 200.00
  },
  "observability": {
    "prometheus": {
      "enabled": true,
      "port": 9090,
      "path": "/metrics"
    },
    "otel": {
      "enabled": true,
      "endpoint": "http://otel-collector:4317",
      "interval_seconds": 60
    }
  }
}
```

---

### POST /config
Update configuration.

**Request**:
```json
{
  "budgets": {
    "daily_limit": 15.00,
    "weekly_limit": 75.00
  }
}
```

**Response** (200 OK):
```json
{
  "status": "config_updated",
  "affected_fields": ["budgets.daily_limit", "budgets.weekly_limit"]
}
```

**Validation**:
- daily_limit: 0.01 - 10000.00
- weekly_limit: 0.01 - 50000.00
- monthly_limit: 0.01 - 500000.00

---

### GET /config/budgets
Get budget status and usage.

**Response** (200 OK):
```json
{
  "daily": {
    "limit": 10.00,
    "used": 7.25,
    "remaining": 2.75,
    "percentage": 72.5,
    "reset_at": "2026-04-27T00:00:00Z"
  },
  "weekly": {
    "limit": 50.00,
    "used": 35.80,
    "remaining": 14.20,
    "percentage": 71.6,
    "reset_at": "2026-05-03T00:00:00Z"
  },
  "monthly": {
    "limit": 200.00,
    "used": 120.50,
    "remaining": 79.50,
    "percentage": 60.25,
    "reset_at": "2026-05-26T00:00:00Z"
  }
}
```

---

### POST /config/budgets
Set budget limit.

**Request**:
```json
{
  "type": "daily",
  "limit": 12.50
}
```

**Response** (200 OK):
```json
{
  "type": "daily",
  "limit": 12.50,
  "previous_limit": 10.00
}
```

---

## Metrics API

### GET /metrics
Export metrics in Prometheus text format.

**Response** (200 OK, text/plain):
```
# HELP claude_escalate_requests_total Total number of requests processed
# TYPE claude_escalate_requests_total counter
claude_escalate_requests_total 15234

# HELP claude_escalate_cache_hits_total Total cache hits
# TYPE claude_escalate_cache_hits_total counter
claude_escalate_cache_hits_total 6093

# HELP claude_escalate_model_usage_total Total requests per model
# TYPE claude_escalate_model_usage_total counter
claude_escalate_model_usage_total{model="haiku"} 9140
claude_escalate_model_usage_total{model="sonnet"} 4321
claude_escalate_model_usage_total{model="opus"} 1773

# HELP claude_escalate_cache_hit_rate Cache hit rate
# TYPE claude_escalate_cache_hit_rate gauge
claude_escalate_cache_hit_rate 0.399
```

**Format**: [Prometheus Exposition Format](https://prometheus.io/docs/instrumenting/exposition_formats/)

---

### GET /metrics/snapshot
Get metrics as JSON snapshot.

**Response** (200 OK):
```json
{
  "total_requests": 15234,
  "cache_hits": 6093,
  "cache_hit_rate": 0.399,
  "batch_queued": 2341,
  "model_switches": 156,
  "cache_size": 2150,
  "queue_size": 0,
  "cost_this_month": 847.32,
  "cost_per_request": 0.0556,
  "active_sessions": 3,
  "model_usage": {
    "haiku": 9140,
    "sonnet": 4321,
    "opus": 1773
  },
  "latency_p50": 125.0,
  "latency_p95": 350.5,
  "latency_p99": 750.0
}
```

---

## Error Responses

### 400 Bad Request
```json
{
  "error": "invalid_parameter",
  "message": "Parameter 'days' must be between 1 and 365",
  "status": 400
}
```

### 404 Not Found
```json
{
  "error": "resource_not_found",
  "message": "No data available for requested period",
  "status": 404
}
```

### 500 Internal Server Error
```json
{
  "error": "internal_error",
  "message": "Database connection failed",
  "status": 500,
  "trace_id": "abc123def456"
}
```

---

## Rate Limiting

- **Default**: 1000 requests/minute per client IP
- **Burst**: 100 requests/second
- **Response Header**: `X-RateLimit-Remaining: 999`

---

## API Versioning

Current API version: **v1** (implicit in base URL)

Future versions will use `/api/v2`, `/api/v3`, etc.

---

## OpenAPI Specification

**File**: `docs/openapi.yaml`

```yaml
openapi: 3.0.0
info:
  title: Claude Escalate API
  version: 4.0.0
  description: Cost optimization API for Claude deployments

servers:
  - url: http://localhost:9000/api
    description: Local development

paths:
  /health:
    get:
      summary: Health check
      responses:
        '200':
          description: Service healthy

  /classify/predict:
    post:
      summary: Classify prompt into task type
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              properties:
                prompt:
                  type: string
      responses:
        '200':
          description: Classification successful

  /analytics/timeseries:
    get:
      summary: Get time-series metrics
      parameters:
        - name: bucket
          in: query
          schema:
            type: string
            enum: [hourly, daily, weekly]
        - name: days
          in: query
          schema:
            type: integer
      responses:
        '200':
          description: Time-series data

# ... additional paths
```

---

## SDK & Client Libraries

### Go Client
```go
import "github.com/szibis/claude-escalate/sdk"

client := sdk.NewClient("http://localhost:9000")
metrics, err := client.Analytics.GetTimeseries("daily", 7)
```

### Python Client (Planned for v4.1)
```python
from claude_escalate import Client

client = Client("http://localhost:9000")
metrics = client.analytics.get_timeseries("daily", days=7)
```

### JavaScript Client
See `web/src/api.js` for reference implementation.

---

**Last Updated**: 2026-04-26  
**Status**: Stable (v4.0.0)  
**Next Review**: 2026-06-26
