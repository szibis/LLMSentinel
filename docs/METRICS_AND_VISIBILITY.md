# Claude Escalate Metrics & Visibility

Complete observability into token optimization, cost savings, and performance metrics—matching codeburn's format.

## Overview

Claude Escalate tracks every optimization layer and exposes comprehensive metrics through:

1. **Web Dashboard** - HTTP API endpoints for real-time metrics
2. **CLI Viewer** - Command-line interface for quick metrics lookup
3. **Exports** - JSON/CSV export for analysis and reporting

## Metrics Tracked

### Tokens Burned
Input and output tokens sent to Claude API:
- **Input Tokens**: Tokens in user requests
- **Output Tokens**: Tokens in Claude responses
- **Cache Read Tokens**: Tokens from Anthropic prompt cache
- **Cache Write Tokens**: Tokens written to Anthropic prompt cache
- **Estimated Cost**: USD cost (calculated using LiteLLM pricing)

### Tokens Saved
Tokens prevented from being sent (optimization impact):
- **Exact Dedup**: 100% savings when cache hit
- **Semantic Cache**: 98% savings (embedding cost deducted)
- **Input Optimization**: 30-40% savings from compression
- **Output Optimization**: 30-50% savings from compression
- **RTK Proxy**: 99.4% savings on command output
- **Batch API**: 50% savings from batch discount
- **Knowledge Graph**: 99% savings from graph lookups

### Quality Metrics
- **Cache Hit Rate**: Percentage of requests served from cache
- **False Positive Rate**: Percentage of cache hits that were wrong answers
- **Requests per Second**: Throughput tracking
- **Monthly Projections**: Extrapolate 7-day and 30-day trends

## Web Dashboard API

### Overview Endpoint
```bash
curl http://localhost:8080/api/metrics/overview
```

Returns:
```json
{
  "tokens_burned": {
    "input": 1000000,
    "output": 500000,
    "cache_read": 250000,
    "cache_write": 100000,
    "total": 1500000,
    "estimated_cost": 15.50
  },
  "tokens_saved": {
    "total": 450000,
    "savings_percent": 30.0,
    "estimated_savings": 4.50
  },
  "requests": 1500,
  "cache_hit_rate": 52.3,
  "false_positive_rate": 0.08,
  "timestamp": "2026-04-27T10:30:00Z"
}
```

### Daily Breakdown
```bash
curl http://localhost:8080/api/metrics/daily?days=7
```

Returns daily metrics for each day in the range.

### Optimization Breakdown
```bash
curl http://localhost:8080/api/metrics/breakdown
```

Shows per-optimization-layer savings:
```json
{
  "exact_dedup": {
    "tokens_saved": 50000,
    "cost_saved": 0.50,
    "hit_count": 250
  },
  "semantic_cache": {
    "tokens_saved": 180000,
    "cost_saved": 1.80,
    "hit_count": 400
  },
  "input_optimization": {
    "tokens_saved": 120000,
    "cost_saved": 1.20,
    "hit_count": 1500
  },
  "rtk_proxy": {
    "tokens_saved": 100000,
    "cost_saved": 1.00,
    "hit_count": 50
  }
}
```

### Monthly Projections
```bash
curl http://localhost:8080/api/metrics/projections
```

Extrapolate current trends to monthly costs and savings.

### Export Endpoints
```bash
# JSON export
curl http://localhost:8080/api/metrics/export/json > metrics.json

# CSV export
curl http://localhost:8080/api/metrics/export/csv > metrics.csv
```

## CLI Metrics Viewer

### Overview Command
```bash
claude-escalate-metrics overview
```

Terminal dashboard output:
```
╔══════════════════════════════════════════════════════════════╗
║           Claude Escalate Metrics Overview                   ║
╚══════════════════════════════════════════════════════════════╝

📊 TOKENS BURNED (sent to Claude API)
   Input Tokens:        1000000
   Output Tokens:        500000
   Cache Read Tokens:    250000
   Cache Write Tokens:   100000
   ├─ Total:           1500000
   └─ Est. Cost:       $15.50

💾 TOKENS SAVED (optimization impact)
   Total Saved:         450000
   Savings:              30.0%
   Est. Cost Saved:      $4.50

📈 REQUEST STATISTICS
   Total Requests:       1500
   Cache Hit Rate:       52.3%
   False Positive Rate:   0.1%

✅ Monthly Projection (7-day extrapolation)
   Projected Tokens:    6450000
   Projected Savings:   1935000
   Projected Cost:      $66.45
   Projected Savings:   $19.35
   Confidence:           89.0%
```

### Daily Breakdown
```bash
# Last 7 days (default)
claude-escalate-metrics daily

# Last 30 days
claude-escalate-metrics daily -days 30

# JSON output
claude-escalate-metrics daily -format json
```

Output:
```
📅 DAILY BREAKDOWN (Last 7 days)
──────────────────────────────────────────────────────────────
Date          Burned   Saved   Cache Hit  Requests
──────────────────────────────────────────────────────────────
2026-04-27   214285   64285     52.3%      214
2026-04-26   185714   55714     51.8%      186
2026-04-25   200000   60000     53.1%      200
...
```

### Optimization Breakdown
```bash
# Text format
claude-escalate-metrics breakdown

# JSON format
claude-escalate-metrics breakdown -format json
```

Shows contribution of each optimization layer:
```
🔍 OPTIMIZATION BREAKDOWN
──────────────────────────────────────────────────────────────
Layer                  Tokens Saved   Cost Saved   Hits   Savings Type
──────────────────────────────────────────────────────────────
exact_dedup                 50000      $0.50       250    100% (cache hit)
semantic_cache             180000      $1.80       400    98% (embedding cost deducted)
input_optimization         120000      $1.20      1500    30-40% (compression)
output_optimization         50000      $0.50       600    30-50% (compression)
rtk_proxy                  100000      $1.00        50    99.4% (RTK proxy)
batch_api                  60000       $0.60       150    50% (batch discount)
knowledge_graph             30000      $0.30        10    99% (graph lookup)
```

### Monthly Projections
```bash
claude-escalate-metrics projections
```

Shows 7-day and 30-day extrapolations with confidence scores.

### Export Metrics
```bash
# Export as JSON
claude-escalate-metrics export -format json > metrics.json

# Export as CSV
claude-escalate-metrics export -format csv > metrics.csv
```

### Quick Status
```bash
claude-escalate-metrics status
```

One-liner output:
```
Claude Escalate Metrics Status
─────────────────────────────────────────
Total Tokens Burned:  1500000
Total Tokens Saved:   450000
Savings Percentage:   30.0%
Total Cost:           $15.50
Cost Saved:           $4.50
Requests:             1500
```

## Dashboard Integration

The metrics endpoints integrate with web dashboards:

1. **Real-Time Updates**: WebSocket streaming for live metrics
2. **Charts & Graphs**: Visualize daily trends, savings breakdown
3. **Projections**: Show extrapolated monthly costs
4. **Alerts**: Trigger if savings drop below threshold or false positive rate spikes
5. **Exports**: Download JSON/CSV for reporting

### Example Dashboard Queries

```javascript
// Fetch overview metrics
const response = await fetch('/api/metrics/overview');
const metrics = await response.json();

console.log(`Today's Cost: $${metrics.tokens_burned.estimated_cost}`);
console.log(`Savings: ${metrics.tokens_saved.savings_percent}%`);
```

## Metrics Format (Codeburn Compatible)

Claude Escalate metrics match [codeburn](https://github.com/getagentseal/codeburn) format:

```json
{
  "period": {
    "start_date": "2026-04-20",
    "end_date": "2026-04-27",
    "days": 7
  },
  "burned": {
    "input_tokens": 1000000,
    "output_tokens": 500000,
    "cache_read_tokens": 250000,
    "cache_write_tokens": 100000,
    "total_tokens": 1500000,
    "estimated_cost_usd": 15.50
  },
  "saved": {
    "exact_dedup_tokens": 50000,
    "semantic_cache_tokens": 180000,
    "input_optimization_tokens": 120000,
    "output_optimization_tokens": 50000,
    "rtk_savings_tokens": 100000,
    "batch_api_savings_tokens": 60000,
    "knowledge_graph_tokens": 30000,
    "total_tokens_saved": 450000,
    "savings_percent": 30.0,
    "estimated_cost_saved_usd": 4.50
  },
  "breakdown": {
    "exact_dedup": {
      "tokens_saved": 50000,
      "cost_saved_usd": 0.50,
      "hit_count": 250
    },
    "semantic_cache": {
      "tokens_saved": 180000,
      "cost_saved_usd": 1.80,
      "hit_count": 400
    }
  },
  "requests": 1500,
  "cache_hit_rate": 52.3,
  "false_positive_rate": 0.08,
  "projections": {
    "7day_monthly": {
      "projected_tokens_burned": 6450000,
      "projected_tokens_saved": 1935000,
      "projected_cost_usd": 66.45,
      "projected_savings_usd": 19.35,
      "projected_savings_percent": 30.0
    }
  }
}
```

## Performance Monitoring

Use metrics to monitor:

### Cache Health
- **Cache Hit Rate**: Target 50%+ (60%+ for production)
- **False Positive Rate**: Target <0.5% (alert if >0.5%)
- **Hit Count by Layer**: Identify which layers are most effective

### Savings Validation
- **Actual vs Projected**: Compare real savings to projections
- **Per-Optimization Breakdown**: See which layers contribute most
- **Monthly Trends**: Identify patterns and anomalies

### Quality Metrics
- **False Positive Spikes**: Indicate misconfigured cache threshold
- **Low Cache Hit Rate**: May indicate unstable system prompt or context
- **High Cost per Request**: May indicate overpowered model selection

## Integration with Observability

Metrics can be integrated with:

- **Prometheus**: `/metrics` endpoint for scraping
- **Grafana**: Pre-built dashboards using metrics API
- **CloudWatch**: Push metrics to AWS CloudWatch
- **Datadog**: Custom metrics agent
- **ELK Stack**: Log metrics alongside request logs

## Cost Projection Confidence

Monthly projections include confidence scoring:

- **7-day projection**: 75% confidence (more variable)
- **14-day projection**: 83% confidence
- **30-day projection**: 90% confidence (more stable)

Higher confidence = more reliable extrapolation for budgeting.

## API Reference

All endpoints support:

- **JSON output** (default): `Content-Type: application/json`
- **CSV export**: Add `?format=csv` to any endpoint
- **Date filtering**: `?from=2026-04-01&to=2026-04-30`
- **Custom periods**: `/metrics/daily?days=14`

Response codes:
- `200`: Success
- `400`: Invalid parameters
- `404`: Metrics not found
- `500`: Server error

## Next Steps

1. Start the gateway: `./claude-escalate`
2. View metrics via CLI: `claude-escalate-metrics overview`
3. Open dashboard: `http://localhost:8080/dashboard`
4. Monitor `/api/metrics/overview` for cost trends
5. Set up alerts on savings drop or false positive spikes
