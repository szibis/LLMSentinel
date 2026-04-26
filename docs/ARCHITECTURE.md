# Claude Escalate v4.0.0 - Architecture & Technical Design

## System Overview

Claude Escalate v4.0.0 is a cost optimization platform for Claude deployments featuring ML-based task classification, advanced analytics, observability, and a comprehensive web dashboard.

```
┌─────────────────────────────────────────────────────────────┐
│                    Claude Escalate v4.0.0                   │
│                 (http://localhost:9000)                      │
└─────────────────────────────────────────────────────────────┘
        │
        ├─ REST API Layer
        │   ├─ /api/classify/* (Task Classification)
        │   ├─ /api/analytics/* (Advanced Analytics)
        │   ├─ /api/config/* (Configuration Management)
        │   ├─ /api/optimize (Optimization Engine)
        │   ├─ /metrics (Prometheus Pull)
        │   └─ /health (Service Health)
        │
        ├─ Background Services
        │   ├─ Learner (Hourly: ML retraining)
        │   ├─ Analytics Aggregator (Hourly: Time-series buckets)
        │   ├─ Retention Cleanup (Daily: Data cleanup)
        │   └─ OTEL Push Loop (60s: Metrics export)
        │
        ├─ Core Engines
        │   ├─ Classification Engine (Embeddings + Regex Fallback)
        │   ├─ Analytics Engine (Forecasts + Correlations)
        │   ├─ Routing Engine (Model selection)
        │   └─ Cost Optimizer (Caching + Batching)
        │
        └─ Observability Stack
            ├─ Prometheus Exporter (/metrics)
            ├─ OTEL Metrics Push (HTTP OTLP)
            └─ Web Dashboard (React + Vite)

┌─────────────────────────────────────────────────────────────┐
│              External Services (Docker Compose)              │
├─────────────────────────────────────────────────────────────┤
│ VictoriaMetrics    │ Grafana          │ OTel Collector      │
│ (8428)             │ (3000)           │ (4317/4318)         │
└─────────────────────────────────────────────────────────────┘
```

---

## Component Architecture

### 1. Classification Engine

**Purpose**: Classify incoming prompts into task types for intelligent routing.

**Components**:
```
Input Prompt
    ↓
┌──────────────────────────────┐
│ EmbeddingClassifier          │
│ - 384-dim embeddings         │
│ - Cosine similarity matching │
│ - 10 task type categories    │
└──────────────────────────────┘
    ↓ (confidence > 0.75)
Return Task Type + Confidence
    ↓ (confidence < 0.75)
┌──────────────────────────────┐
│ RegexClassifier (Fallback)   │
│ - 40+ semantic patterns      │
│ - Rule-based matching        │
└──────────────────────────────┘
    ↓
Final Task Classification
```

**Files**:
- `internal/classify/embeddings.go` — Embedding generation and similarity
- `internal/classify/classify.go` — Task classification logic
- `internal/classify/learner.go` — Active learning loop

**Task Types**:
```
1. concurrency        6. security
2. parsing           7. database
3. optimization      8. networking
4. debugging         9. testing
5. architecture     10. devops
```

**Active Learning Loop**:
- Records misclassifications hourly
- Retrains embeddings from corrected examples
- Updates confidence thresholds
- Measurable accuracy improvement over time

**Key Metrics**:
- Classification latency: <5ms per prompt
- Embedding confidence range: 0.0–1.0
- Fallback activation rate: ~15% (confidence <0.75)

---

### 2. Analytics Engine

**Purpose**: Extract insights from operational metrics and predict future trends.

**Sub-Components**:

#### 2a. Time-Series Aggregation
```
validation_metrics (raw records)
    ↓ (Hourly cron)
┌──────────────────────────────┐
│ Create hourly bucket         │
│ - TotalRequests              │
│ - CacheHits/Misses           │
│ - BatchQueued, DirectRequests│
│ - Cost, SuccessRate          │
└──────────────────────────────┘
    ↓ (Aggregated)
metrics_hourly table
    ↓ (Daily rollup)
metrics_daily table
    ↓ (Weekly rollup)
metrics_weekly table
```

**Files**: `internal/analytics/timeseries.go`
**Retention**: hourly=7d, daily=90d, weekly=1y (configurable)

#### 2b. Percentile Calculations
```
latency_ms column (from validation_metrics)
    ↓
Calculate: P50, P75, P90, P95, P99
Min, Max, Mean, Std Dev
Sample Count
    ↓
Breakdown by: model, task type, (model, task)
    ↓
LatencyPercentiles struct
```

**Files**: `internal/analytics/percentiles.go`
**Algorithm**: Linear interpolation on sorted samples
**Accuracy**: Within 0.1% of numpy/scipy

#### 2c. Predictive Forecasting
```
Historical cost data (past 30 days)
    ↓
Fit linear regression model
Slope, Intercept, RMSE, R²
    ↓
Generate 7-day forecast
Point estimates + 95% confidence intervals
    ↓
BudgetExceeded() checks if forecast exceeds limit
```

**Files**: `internal/analytics/forecast.go`
**Method**: Linear regression
**Confidence Level**: 95% (adjustable)

#### 2d. Correlation Analysis
```
Variable pairs:
- Task type ↔ Success rate
- Model ↔ Success rate
- Cache hits ↔ Cost savings
- Token error ↔ Satisfaction
    ↓
Pearson correlation coefficient
P-value for statistical significance
    ↓
Return only significant correlations (p < 0.05)
```

**Files**: `internal/analytics/correlations.go`
**Use Case**: "Concurrency tasks need Opus 80% of time"

#### 2e. Per-Task Per-Model Accuracy
```
Query validation_metrics
Group by: (task_type, model)
    ↓
Calculate: success_count, total_count, success_rate
avg_token_error, avg_latency_ms
    ↓
Used by routing engine to select cheapest
model with >80% success rate for task
```

**Files**: `internal/analytics/task_accuracy.go`

---

### 3. Observability Stack

**Purpose**: Export metrics for monitoring and alerting.

#### 3a. Prometheus Metrics (Pull)
```
/metrics endpoint (HTTP GET)
    ↓
Prometheus text format export
    ↓
Metrics types:
- Counters (requests_total, cache_hits_total)
- Gauges (cache_size, queue_size, cost_this_month)
- Histograms (latency, token_error, cost_per_request)
    ↓
Scrape interval: configurable (default: 15s)
```

**Files**: `internal/observability/prometheus.go`
**Buckets**:
- Latency: 10ms, 50ms, 100ms, 500ms, 1s
- Token Error: 0.05, 0.10, 0.15, 0.20, 0.50, 1.0

#### 3b. OTEL Metrics Push
```
Claude Escalate Service
    ↓ (Every 60 seconds)
Build OTLP metrics payload
(same metrics as Prometheus)
    ↓
HTTP POST to OTel Collector
http://otel-collector:4318/v1/metrics
    ↓ (OTel Collector)
Remote write to VictoriaMetrics
or vendor agent (Datadog, Honeycomb, etc.)
```

**Files**: `internal/observability/otel.go`
**Format**: OTLP HTTP
**Scope**: Metrics only (no traces/logs in v4.0.0)

---

### 4. Web Dashboard

**Purpose**: Real-time visualization and configuration management.

**Architecture**:
```
┌─────────────────────────────────────────┐
│     React 18.2.0 + Vite 4.4.0           │
│     (http://localhost:3001)             │
├─────────────────────────────────────────┤
│ 5 Tab Navigation:                       │
│ ├─ Overview (Real-time metrics)         │
│ ├─ Analytics (Trends & forecasts)       │
│ ├─ Config (Settings management)         │
│ ├─ Tasks (Task accuracy analysis)       │
│ └─ Health (Service status)              │
├─────────────────────────────────────────┤
│ Styling:                                │
│ ├─ Tailwind CSS 3.3.2                   │
│ ├─ Dark/Light mode toggle               │
│ ├─ Responsive design (320px–1920px)     │
│ └─ localStorage persistence             │
├─────────────────────────────────────────┤
│ Real-time Updates:                      │
│ ├─ Polling interval: 5 seconds          │
│ ├─ Chart.js 4.3.0 (interactive charts) │
│ └─ Auto-refresh on metric changes       │
└─────────────────────────────────────────┘
        ↓
API Client (api.js)
        ↓
┌───────────────────────────────────────┐
│ Proxy to backend (http://localhost:9000)
│ GET  /api/analytics/timeseries        │
│ GET  /api/analytics/percentiles       │
│ GET  /api/analytics/forecast          │
│ GET  /api/analytics/task-accuracy     │
│ GET  /api/config                      │
│ POST /api/config (update settings)    │
│ GET  /metrics (Prometheus format)     │
└───────────────────────────────────────┘
```

**Tab Breakdown**:

1. **Overview** (3-4 panels)
   - Request volume (24h trend)
   - Cost breakdown (pie: cache vs batch vs direct)
   - Cache hit rate (gauge)
   - Model usage distribution (stacked bar)

2. **Analytics** (6-8 panels)
   - Daily cost trend (30 days)
   - Forecast overlay (7-day with CI)
   - Percentile latencies (P50, P95, P99)
   - Token error distribution
   - Per-task cost breakdown
   - Model success comparison

3. **Config** (Form-based)
   - Budget limits (daily, weekly, monthly)
   - Cache settings (TTL, similarity threshold)
   - Batch settings (min size, max wait)
   - OTEL endpoint configuration
   - Embedding model selection (L6/L12)
   - Submit & validate

4. **Tasks** (Table + Analysis)
   - Success rate by task type
   - Misclassifications (red highlight)
   - Accuracy trend per model
   - Hardest tasks (success <70%)
   - Embedding confidence distribution

5. **Health** (Status Cards)
   - Service status (green/yellow/red)
   - Uptime counter
   - Recent errors (last 10)
   - Component status (DB, cache, OTEL)

**Files**:
```
web/
├── src/
│   ├── App.jsx              (Router, dark mode toggle)
│   ├── components/
│   │   ├── Overview.jsx     (Real-time gauges)
│   │   ├── Analytics.jsx    (Trend charts)
│   │   ├── Config.jsx       (Settings forms)
│   │   ├── Tasks.jsx        (Task analysis)
│   │   └── Health.jsx       (Service status)
│   ├── api.js               (Axios client)
│   ├── index.css            (Tailwind + custom)
│   └── main.jsx
├── vite.config.js
├── tailwind.config.js
└── package.json
```

---

### 5. Docker Compose Stack

**Purpose**: One-command deployment of entire platform.

**Services**:

```
┌─────────────────────────────────────────────┐
│ Claude Escalate (Port 9000)                 │
│ ├─ :9000/api/* (REST endpoints)             │
│ ├─ :9000/metrics (Prometheus export)        │
│ └─ :9000/health (Service health)            │
├─────────────────────────────────────────────┤
│ VictoriaMetrics (Port 8428)                 │
│ ├─ Metrics time-series database             │
│ ├─ Lightweight Prometheus alternative       │
│ └─ Retention policies enforced              │
├─────────────────────────────────────────────┤
│ Grafana (Port 3000)                         │
│ ├─ 5 pre-built dashboards                   │
│ ├─ VictoriaMetrics datasource auto-config  │
│ └─ Alert rules (email/webhook)              │
├─────────────────────────────────────────────┤
│ OTel Collector (Ports 4317/4318)            │
│ ├─ OTLP gRPC receiver (4317)                │
│ ├─ OTLP HTTP receiver (4318)                │
│ └─ Remote write to VictoriaMetrics          │
└─────────────────────────────────────────────┘

Network: escalate-network (bridge)
All services communicate via service names (DNS)
Volumes: Persistent for data, config
```

**Deployment**:
```bash
docker-compose up -d
# Waits 10 seconds for services to boot
./scripts/verify-services.sh
# Outputs: ✓ All services verified
```

**Orchestration**:
- Service dependencies: explicit via `depends_on`
- Health checks: HTTP GET /health (or /metrics, /api/health)
- Volume mapping: Host paths → container paths
- Environment variables: Centralized in docker-compose.yml
- Graceful shutdown: SIGTERM handling on all services

---

## Data Models

### Core Tables

```sql
-- validation_metrics (raw records from inference)
CREATE TABLE validation_metrics (
  id INTEGER PRIMARY KEY,
  timestamp DATETIME,
  task_type VARCHAR(50),
  model VARCHAR(50),
  prompt_tokens INTEGER,
  completion_tokens INTEGER,
  total_tokens INTEGER,
  estimated_cost_usd DECIMAL(10,6),
  actual_cost_usd DECIMAL(10,6),
  cache_hit BOOLEAN,
  batched BOOLEAN,
  latency_ms INTEGER,
  token_error DECIMAL(5,3),     -- |estimated - actual| / actual
  success BOOLEAN
);

-- metrics_hourly (1-hour aggregation)
CREATE TABLE metrics_hourly (
  timestamp DATETIME PRIMARY KEY,
  total_requests INTEGER,
  cache_hits INTEGER,
  cache_misses INTEGER,
  batch_queued INTEGER,
  direct_requests INTEGER,
  total_cost_usd DECIMAL(10,6),
  estimated_cost_usd DECIMAL(10,6),
  success_rate DECIMAL(5,4),
  avg_latency_ms DECIMAL(8,2),
  p50_latency_ms DECIMAL(8,2),
  p95_latency_ms DECIMAL(8,2),
  p99_latency_ms DECIMAL(8,2)
);

-- learning_events (misclassifications for retraining)
CREATE TABLE learning_events (
  id INTEGER PRIMARY KEY,
  timestamp DATETIME,
  prompt TEXT,
  predicted_task VARCHAR(50),
  actual_task VARCHAR(50),
  confidence DECIMAL(5,4),
  succeeded BOOLEAN,
  token_error DECIMAL(5,3)
);

-- task_model_accuracy (per-task per-model stats)
CREATE TABLE task_model_accuracy (
  task_type VARCHAR(50),
  model VARCHAR(50),
  success_count INTEGER,
  total_count INTEGER,
  success_rate DECIMAL(5,4),
  avg_token_error DECIMAL(5,3),
  avg_latency_ms DECIMAL(8,2),
  last_updated DATETIME,
  PRIMARY KEY (task_type, model)
);
```

### Embedding Vectors

```go
type TaskEmbedding struct {
  TaskType  string    // "concurrency", "parsing", ...
  Embedding []float32 // 384-dimensional vector
  UpdatedAt time.Time
}

type PromptEmbedding struct {
  ID        string
  Prompt    string
  Embedding []float32 // 384-dimensional
  TaskType  string
}
```

---

## Request Flow

### Classification Request

```
Client: POST /api/classify/predict
  { "prompt": "race condition deadlock" }
    ↓
Handler: classifyHandler()
    ↓
EmbeddingClassifier.Classify()
  ├─ Generate embedding (all-MiniLM-L6-v2)
  ├─ Cosine similarity vs all task types
  ├─ topMatches(k=3)
  └─ Return highest + confidence
    ↓
If confidence < 0.75:
  ├─ RegexClassifier.Classify() (fallback)
  └─ Return regex result
    ↓
Response: {
  "task_type": "concurrency",
  "confidence": 0.92,
  "embedding_dims": 384,
  "fallback_used": false
}
```

### Analytics Request

```
Client: GET /api/analytics/timeseries?bucket=daily&days=7
    ↓
Handler: timeseriesHandler()
    ↓
Query metrics_daily table
  WHERE timestamp >= now() - 7 days
  ORDER BY timestamp DESC
    ↓
Aggregate results:
  ├─ Total requests per bucket
  ├─ Cache hit rate
  ├─ Cost trends
  └─ Latency percentiles
    ↓
Response: [
  {
    "timestamp": "2026-04-26T00:00:00Z",
    "bucket": "daily",
    "total_requests": 1000,
    "cache_hits": 400,
    ...
  }
]
```

### Optimization Decision

```
Client: GET /api/optimize
  { task_type, estimated_tokens }
    ↓
RoutingEngine.DecideModel()
  ├─ Query TaskModelAccuracy
  │   WHERE task_type = input.task
  ├─ Filter models with success_rate > 0.80
  ├─ Sort by cost (Haiku < Sonnet < Opus)
  └─ Return cheapest viable model
    ↓
Response: {
  "recommended_model": "haiku",
  "expected_cost": 0.04,
  "success_probability": 0.88,
  "reason": "Haiku succeeds 88% of time for parsing tasks"
}
```

---

## Cron Jobs & Background Tasks

### Learner Update (Hourly)

```
Trigger: Every 60 minutes
    ↓
Query learning_events from past hour
    ↓
Analyze misclassifications:
  ├─ Predicted ≠ Actual
  ├─ Group by task type
  └─ Calculate error rate
    ↓
Retrain embeddings:
  ├─ Use corrected examples
  ├─ Update task embeddings
  └─ Adjust confidence thresholds
    ↓
Log changes to audit trail
    ↓
Publish metric: classifier_accuracy (0.0–1.0)
```

### Analytics Aggregation (Hourly)

```
Trigger: Every 60 minutes
    ↓
Read validation_metrics from past hour
    ↓
Aggregate to metrics_hourly:
  ├─ SUM(requests)
  ├─ SUM(cache_hits) / SUM(requests)
  ├─ Percentile calculations (P50, P95, P99)
  └─ MEAN(cost), MIN/MAX
    ↓
Roll up hourly → daily (if day boundary crossed)
Roll up daily → weekly (if week boundary crossed)
    ↓
Enforce retention policies:
  ├─ Delete hourly > 7 days
  ├─ Delete daily > 90 days
  └─ Keep weekly indefinitely
```

### Retention Cleanup (Daily)

```
Trigger: Every 24 hours at 2:00 AM UTC
    ↓
Delete validation_metrics:
  WHERE timestamp < now() - 30 days
    ↓
Delete learning_events:
  WHERE timestamp < now() - 90 days
    ↓
Vacuum database (compact space)
    ↓
Log cleanup summary
```

### OTEL Push Loop (Every 60 seconds)

```
Trigger: Background goroutine
    ↓
Collect current metrics:
  ├─ Counter values (requests, cache hits, costs)
  ├─ Gauge values (cache size, queue size)
  └─ Histogram observations (latency, token error)
    ↓
Build OTLP metrics payload (protobuf)
    ↓
HTTP POST to http://otel-collector:4318/v1/metrics
    ↓
If succeeded:
  ├─ Metrics pushed to Grafana/vendor
  └─ Log success (debug level)
    ↓
If failed:
  ├─ Retry with exponential backoff
  ├─ Log error (warn level)
  └─ Continue (metrics don't block service)
```

---

## Failure Modes & Resilience

### Embedding Model Failure

```
Issue: Embedding inference timeout or OOM
    ↓
Behavior:
  ├─ Catch error in EmbeddingClassifier
  ├─ Log error (with trace)
  ├─ Fallback to RegexClassifier
  └─ Continue serving requests
    ↓
Result: Degraded accuracy but service stays up
```

### Database Connectivity Loss

```
Issue: SQLite connection drops
    ↓
Behavior:
  ├─ Query returns error
  ├─ Retry with exponential backoff (3 attempts)
  ├─ If all fail: return HTTP 500
  └─ Alerting fires (if configured)
    ↓
Impact: Requests fail temporarily (client should retry)
```

### VictoriaMetrics Down

```
Issue: Metrics scrape fails (port 8428 unreachable)
    ↓
Behavior:
  ├─ Claude Escalate continues normally
  ├─ /metrics endpoint still works (pull)
  ├─ OTEL push fails silently (with backoff retry)
  └─ Grafana has stale data (last scraped value)
    ↓
Impact: Monitoring lags but service operational
Recovery: Restart VictoriaMetrics container
  docker-compose up -d victoriametrics
```

### OTel Collector Down

```
Issue: HTTP OTLP endpoint unreachable
    ↓
Behavior:
  ├─ Push fails silently (catch exception)
  ├─ Metrics buffered in memory briefly
  ├─ After 10 failed attempts: stop retrying
  └─ Prometheus pull still works
    ↓
Impact: Vendor integration broken; local monitoring OK
Recovery: Restart OTel Collector
  docker-compose up -d otel-collector
```

### Web Dashboard Unavailable

```
Issue: React app fails to load or API errors
    ↓
Behavior:
  ├─ GET /api/* calls fail with 500
  ├─ Dashboard shows error message
  ├─ User can still use REST API directly
  └─ CLI tools still functional
    ↓
Recovery: Check backend logs
  docker-compose logs -f claude-escalate
```

---

## Security Considerations

### Input Validation

```
All /api/* endpoints validate:
├─ Prompt length: max 10,000 chars
├─ Bucket parameter: enum [hourly, daily, weekly]
├─ Days parameter: range 1–365
├─ Model parameter: enum [haiku, sonnet, opus]
└─ Budget limits: range 0.01–500,000 USD
```

### CORS

```
CORS headers:
├─ Access-Control-Allow-Origin: http://localhost:3001
├─ Access-Control-Allow-Methods: GET, POST, OPTIONS
├─ Access-Control-Allow-Headers: Content-Type
└─ Credentials: omit (no auth cookies)
```

### Rate Limiting

```
Per IP address:
├─ 1,000 requests/minute (global)
├─ 100 requests/second (burst)
└─ Returns 429 Too Many Requests if exceeded
```

### No Secrets in Code

```
All config from environment or files:
├─ OTEL_EXPORTER_OTLP_ENDPOINT (env var)
├─ CONFIG_FILE (yaml, not in repo)
└─ Database file (local filesystem)
```

---

## Monitoring & Alerting

### Key Metrics to Monitor

```
Performance:
├─ Request latency (P50, P95, P99)
├─ Cache hit rate (target: >60%)
└─ Forecast accuracy (RMSE, R²)

Reliability:
├─ Service uptime (target: 99.9%)
├─ Error rate (target: <1%)
└─ Database query latency (target: <200ms)

Cost:
├─ Daily spend vs budget
├─ Spend forecast vs limit
└─ Savings from optimization

Classification:
├─ Embedding classifier accuracy
├─ Fallback activation rate
└─ Misclassification rate
```

### Grafana Dashboards

```
1. Overview (4 panels)
   └─ Volume, cost, cache rate, model distribution

2. Cost Analysis (8 panels)
   ├─ Daily trend
   ├─ Forecast overlay
   ├─ Per-task breakdown
   └─ Savings breakdown

3. Performance (6 panels)
   ├─ Latency percentiles
   ├─ Token error distribution
   └─ Success rate by model

4. Task Classification (5 panels)
   ├─ Accuracy by task
   ├─ Misclassifications
   └─ Confidence distribution

5. Alerts (4 alert rules)
   ├─ High token error (>25%)
   ├─ Low cache rate (<20%)
   ├─ High latency (P99 >1s)
   └─ Budget exceeded
```

---

## Deployment Topology

### Local Development

```
Host Machine
├─ Claude Escalate (go run)
├─ VictoriaMetrics (docker)
├─ Grafana (docker)
├─ OTel Collector (docker)
└─ Web Dashboard (npm run dev)

All communicate via localhost networking
```

### Production (Docker Compose)

```
Host Machine
├─ docker-compose up -d
│  ├─ claude-escalate:4.0.0 (image)
│  ├─ victoriametrics:latest
│  ├─ grafana:latest
│  └─ otel-collector:latest
│
├─ Shared volumes:
│  ├─ escalate-data (sqlite db, embeddings)
│  ├─ vm-data (metrics time-series)
│  └─ grafana-data (dashboards, alerts)
│
└─ Network: escalate-network (bridge)
```

### Horizontal Scaling (Future)

```
Load Balancer (nginx/haproxy)
├─ Claude Escalate Pod 1 (replica)
├─ Claude Escalate Pod 2 (replica)
└─ Claude Escalate Pod N (replica)
    ↓ (all write to)
Shared PostgreSQL
    ↓
Grafana (reads metrics)
```

---

## Performance Characteristics

### Latency Budget

```
Classification Request (POST /api/classify)
├─ Embedding generation: <2ms
├─ Cosine similarity: <1ms
├─ Regex fallback (if needed): <2ms
└─ Total: <5ms

Analytics Query (GET /api/analytics/*)
├─ Database query: <100ms
├─ Aggregation: <50ms
├─ JSON marshaling: <10ms
└─ Total: <200ms

Forecast Calculation (POST /api/analytics/forecast)
├─ Regression fit: <20ms
├─ Confidence intervals: <20ms
└─ Total: <50ms

Metrics Export (GET /metrics)
├─ Collect counter/gauge values: <20ms
├─ Format as Prometheus text: <50ms
└─ Total: <100ms
```

### Throughput

```
Service Capacity (single instance):
├─ Classification: 200+ req/sec
├─ Analytics queries: 50+ req/sec
├─ Metric scrapes: 10+ simultaneous
└─ Total system: 1000+ mixed requests/sec
```

### Memory Usage

```
Service Memory:
├─ Binary + runtime: ~50MB
├─ Database cache: ~100MB (configurable)
├─ Embedding model cache: ~30MB (22MB model + overhead)
└─ Metrics buffer: ~10MB
└─ Total: ~200MB (baseline) + data-dependent

VictoriaMetrics (docker):
├─ Baseline: ~30MB
├─ Per 1M samples: ~50MB
└─ Typical (1 month data): ~500MB

Grafana (docker):
├─ Baseline: ~80MB
├─ Per dashboard: ~10MB
└─ Typical: ~150MB
```

### Disk Usage

```
SQLite database:
├─ Raw records: ~1KB per validation_metric
├─ Aggregated (7d hourly): ~10MB
├─ Aggregated (90d daily): ~5MB
├─ Learning events: ~500B per record
└─ Total (1 month): ~50–100MB

VictoriaMetrics:
├─ 1 month retention: ~500MB–1GB
├─ Full year retention: ~6–12GB

Embeddings:
├─ Task embeddings (10 types): <10KB
├─ Prompt embeddings cache: ~100KB
```

---

## Future Enhancements (v4.1+)

### Phase 1: Distributed Tracing
- OTEL Traces export (OpenTelemetry Protocol)
- Jaeger/Tempo backend for trace visualization
- Request flow tracking end-to-end
- Latency breakdown per component

### Phase 2: Structured Logging
- OTEL Logs export (JSON structured format)
- Loki backend for log aggregation
- Full-text search on logs
- Trace ↔ Log correlation

### Phase 3: Advanced ML
- Multi-model ensemble classification
- Prompt similarity clustering
- Anomaly detection in costs
- Predictive model selection

### Phase 4: Horizontal Scaling
- PostgreSQL shared backend
- Redis cache layer
- Kubernetes deployment
- Multi-region replication

---

**Last Updated**: 2026-04-26  
**Version**: 4.0.0  
**Status**: Production Ready
