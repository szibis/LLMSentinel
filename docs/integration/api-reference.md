# API Reference

Complete documentation of all Escalation Service endpoints (20+).

## Base URL

```
http://localhost:9000
```

Default service runs on port 9000. Use `--port` flag to change.

## Phase 1: Estimation Endpoints

### POST /api/hook
Analyze prompt and estimate routing decision.

**Called by**: Hook wrapper (pre-response)  
**Timeout**: 5 seconds

**Request**:
```json
{
  "prompt": "Design a concurrent cache system in Go"
}
```

**Response** (200 OK):
```json
{
  "continue": true,
  "suppressOutput": false,
  "currentModel": "opus",
  "validationId": "uuid-abc123",
  "effort": "high",
  "estimatedTokens": 1200,
  "routing": {
    "model": "opus",
    "reason": "High effort detected (keywords: concurrent, design, system)",
    "confidence": 0.92,
    "alternative": "sonnet"
  }
}
```

**Error** (402 Payment Required - budget exceeded):
```json
{
  "error": "budget_exceeded",
  "message": "Daily budget $10 exhausted. Recommend: Haiku (cheaper)",
  "remaining_budget": 0.0,
  "suggested_model": "haiku"
}
```

---

### POST /api/metrics/hook
Explicit estimate submission (alternative to /api/hook).

**Request**:
```json
{
  "prompt": "What is OAuth?",
  "estimated_input_tokens": 20,
  "estimated_output_tokens": 150,
  "effort": "low",
  "task_type": "explanation"
}
```

**Response** (200 OK):
```json
{
  "validation_id": "uuid-def456",
  "stored": true
}
```

---

## Phase 2: Real-Time Endpoints

### GET /api/statusline
Get current token metrics (polled during generation).

**Called by**: Statusline (Barista, custom, etc.)  
**Frequency**: Every 500ms during response  
**Timeout**: 2 seconds

**Response** (200 OK):
```json
{
  "phase": 2,
  "validation_id": "uuid-abc123",
  "tokens_flowing": {
    "input_tokens_used": 350,
    "output_tokens_so_far": 340,
    "total_so_far": 690,
    "estimated_remaining": 510,
    "trend": "ON_TRACK"
  },
  "budget_status": {
    "daily_used_so_far": 3.74,
    "daily_remaining": 6.26,
    "percentage": 37,
    "warning": null
  },
  "sentiment_signal": {
    "user_pausing": false,
    "frustration_risk": 0.15,
    "current_sentiment": "patient"
  },
  "model": "opus",
  "effort": "high"
}
```

**Error** (404 Not Found - validation not found):
```json
{
  "error": "not_found",
  "message": "Validation ID not found"
}
```

---

## Phase 3: Validation Endpoints

### POST /api/validate
Submit actual token metrics (post-response).

**Called by**: Monitor daemon, post-response hook  
**Timeout**: 10 seconds

**Request**:
```json
{
  "validation_id": "uuid-abc123",
  "actual_input_tokens": 380,
  "actual_output_tokens": 320,
  "actual_cache_hit_tokens": 0,
  "actual_cache_creation_tokens": 0
}
```

**Response** (200 OK):
```json
{
  "success": true,
  "validation_id": "uuid-abc123",
  "comparison": {
    "estimated_total": 1200,
    "actual_total": 700,
    "error_percent": -41.7,
    "error_message": "Excellent accuracy"
  },
  "decision": {
    "action": "cascade",
    "next_model": "sonnet",
    "rationale": "Over-provisioned. User satisfied.",
    "will_save_tokens": 180
  }
}
```

**Error** (422 Unprocessable Entity - validation data conflict):
```json
{
  "error": "validation_error",
  "message": "Actual tokens exceed estimate by >200%. Please verify.",
  "estimated": 1200,
  "actual": 3000,
  "warning": "Possible data error or miscalculation"
}
```

---

## Analytics Endpoints

### GET /api/analytics/phase-1/{validation_id}
Get Phase 1 (estimation) data.

**Response** (200 OK):
```json
{
  "phase": 1,
  "validation_id": "uuid-abc123",
  "timestamp": "2026-04-25T15:52:00Z",
  "prompt_analysis": {
    "prompt": "Design a concurrent cache...",
    "task_type": "architecture",
    "complexity": 0.85,
    "effort_detected": "high"
  },
  "estimation": {
    "estimated_input_tokens": 380,
    "estimated_output_tokens": 1200,
    "estimated_total_tokens": 1580,
    "estimated_cost_usd": 0.096
  },
  "routing_decision": {
    "recommended_model": "opus",
    "confidence": 0.92,
    "reason": "High complexity detected",
    "alternative": "sonnet"
  },
  "budget_check": {
    "within_budget": true,
    "daily_used": 3.50,
    "daily_remaining": 6.50,
    "warning": null
  }
}
```

### GET /api/analytics/phase-2/{validation_id}
Get Phase 2 (real-time) data.

**Response**:
```json
{
  "phase": 2,
  "validation_id": "uuid-abc123",
  "tokens_flowing": {
    "input_tokens_used": 350,
    "output_tokens_so_far": 340,
    "total_so_far": 690
  },
  "sentiment_during": {
    "user_pausing": false,
    "edit_activity": "normal",
    "frustration_risk": 0.12
  },
  "budget_impact": {
    "daily_projected": 3.78,
    "on_track": true
  }
}
```

### GET /api/analytics/phase-3/{validation_id}
Get Phase 3 (validation & learning) data.

**Response**:
```json
{
  "phase": 3,
  "validation_id": "uuid-abc123",
  "actual_tokens": {
    "input_tokens": 380,
    "output_tokens": 320,
    "cache_hit_tokens": 0,
    "total": 700,
    "cost_usd": 0.042
  },
  "accuracy": {
    "estimated_total": 1580,
    "actual_total": 700,
    "error_percent": -55.7,
    "accuracy_rating": "EXCELLENT"
  },
  "user_sentiment": {
    "explicit_signal": "success",
    "explicit_text": "Perfect! That's exactly right.",
    "implicit_sentiment": "satisfied"
  },
  "decision_made": {
    "action": "cascade",
    "next_model": "sonnet",
    "rationale": "User satisfied, model over-provisioned"
  },
  "learning_stored": {
    "task_type": "architecture",
    "model": "opus",
    "sentiment": "satisfied",
    "success": true
  }
}
```

### GET /api/analytics/sentiment-trends
Get user sentiment patterns over time.

**Query Parameters**:
- `hours` — Last N hours (default: 24)
- `task_type` — Filter by task type (optional)

**Response**:
```json
{
  "period_hours": 24,
  "summary": {
    "satisfied": 38,
    "neutral": 3,
    "frustrated": 1,
    "confused": 0,
    "impatient": 0,
    "total": 42,
    "satisfaction_rate": 0.905
  },
  "frustration_events": [
    {
      "timestamp": "2026-04-25T10:30:00Z",
      "sentiment": "frustrated",
      "task_type": "concurrency",
      "initial_model": "haiku",
      "escalated_to": "sonnet",
      "resolved": true
    }
  ],
  "sentiment_timeline": [
    { "hour": 0, "satisfied": 2, "frustrated": 0, "confused": 0 },
    { "hour": 1, "satisfied": 3, "frustrated": 1, "confused": 0 }
  ]
}
```

### GET /api/analytics/budget-status
Get current budget usage and projections.

**Response**:
```json
{
  "daily_budget": {
    "limit_usd": 10.0,
    "used_usd": 3.78,
    "remaining_usd": 6.22,
    "percentage": 37.8,
    "projected_daily": 5.40,
    "warning": null
  },
  "monthly_budget": {
    "limit_usd": 100.0,
    "used_usd": 45.20,
    "remaining_usd": 54.80,
    "percentage": 45.2,
    "days_remaining": 6,
    "projected_monthly": 76.8
  },
  "model_daily_usage": {
    "opus": { "limit": 5.0, "used": 2.10, "percentage": 42 },
    "sonnet": { "limit": 3.0, "used": 1.20, "percentage": 40 },
    "haiku": { "limit": null, "used": 0.48, "percentage": 4 }
  }
}
```

### GET /api/analytics/model-satisfaction
Get (task_type, model) satisfaction rates.

**Query Parameters**:
- `task_type` — Filter to specific type (optional)
- `sort` — satisfaction, count (default: satisfaction)

**Response**:
```json
{
  "period": "all_time",
  "summary": [
    {
      "task_type": "concurrency",
      "model": "opus",
      "satisfaction_rate": 0.98,
      "sample_count": 12,
      "success_count": 12
    },
    {
      "task_type": "concurrency",
      "model": "sonnet",
      "satisfaction_rate": 0.78,
      "sample_count": 9,
      "success_count": 7
    },
    {
      "task_type": "concurrency",
      "model": "haiku",
      "satisfaction_rate": 0.45,
      "sample_count": 20,
      "success_count": 9
    }
  ]
}
```

---

## Query Endpoints

### GET /api/validation/metrics
List validation records with filtering.

**Query Parameters**:
- `limit` — Max records (default: 100, max: 10000)
- `offset` — Pagination offset (default: 0)
- `task_type` — Filter by task type
- `model` — Filter by model (opus, sonnet, haiku)
- `recent_hours` — Show last N hours
- `sort` — Sort by: date (default), error, tokens

**Response**:
```json
{
  "total": 42,
  "returned": 10,
  "records": [
    {
      "id": "uuid-abc123",
      "timestamp": "2026-04-25T15:52:00Z",
      "prompt": "Design a concurrent cache...",
      "task_type": "architecture",
      "model": "opus",
      "estimated_tokens": 1580,
      "actual_tokens": 700,
      "error_percent": -55.7,
      "sentiment": "satisfied",
      "success": true
    }
  ]
}
```

### GET /api/validation/stats
Get aggregated statistics.

**Query Parameters**:
- `task_type` — Filter by type (optional)
- `recent_hours` — Time period (default: all)

**Response**:
```json
{
  "total_validations": 42,
  "total_estimated_tokens": 12340,
  "total_actual_tokens": 11920,
  "tokens_saved": 420,
  "savings_percent": 3.4,
  "average_token_error": -3.2,
  "cost_accuracy": 96.8,
  "success_rate": 90.5,
  "by_model": {
    "opus": { "count": 8, "success_rate": 100, "avg_error": -45.2 },
    "sonnet": { "count": 20, "success_rate": 95, "avg_error": -12.3 },
    "haiku": { "count": 14, "success_rate": 71, "avg_error": 8.5 }
  }
}
```

### GET /api/validation/{id}
Get single validation record with full details.

**Response**: Same as Phase 1 + Phase 2 + Phase 3 combined

---

## Health & Status Endpoints

### GET /api/health
Simple health check.

**Response** (200 OK):
```json
{
  "status": "ok",
  "version": "1.0.0",
  "uptime_seconds": 3600
}
```

### GET /api/config
Get current configuration (read-only).

**Response**:
```json
{
  "service": {
    "port": 9000,
    "bind": "127.0.0.1"
  },
  "sentiment": {
    "enabled": true,
    "frustration_threshold": 0.70
  },
  "budgets": {
    "daily_usd": 10.0,
    "hard_limit": false
  },
  "statusline_sources": ["barista", "envvar"]
}
```

---

## Common Patterns

### Error Handling

All errors return JSON with this structure:
```json
{
  "error": "error_code",
  "message": "Human readable message",
  "details": {}  // optional, context-specific
}
```

HTTP Status Codes:
- `200` — Success
- `400` — Bad request (invalid JSON, missing fields)
- `402` — Payment required (budget exceeded)
- `404` — Not found
- `422` — Unprocessable entity (validation failed)
- `500` — Internal server error

### Pagination

Endpoints returning lists support:
- `limit` — Items per page (default 100)
- `offset` — Starting position (default 0)

Response includes:
```json
{
  "total": 1000,
  "returned": 100,
  "offset": 0,
  "limit": 100,
  "records": [...]
}
```

### Timestamps

All timestamps are ISO 8601 UTC:
```
2026-04-25T15:52:00Z
```

---

## Quick Start

### Basic Flow

```bash
# 1. Hook sends prompt
curl -X POST http://localhost:9000/api/hook \
  -H "Content-Type: application/json" \
  -d '{"prompt":"What is machine learning?"}'
# → Returns: {"continue":true, "validationId":"123", ...}

# 2. Monitor polls for real-time metrics (every 500ms)
curl http://localhost:9000/api/statusline

# 3. Submit actual tokens (post-response)
curl -X POST http://localhost:9000/api/validate \
  -H "Content-Type: application/json" \
  -d '{"validation_id":"123", "actual_total_tokens":493}'
# → Returns: {"success":true, "error":-1.4%, ...}

# 4. Check stats
curl http://localhost:9000/api/validation/stats
```

### Programmatic Usage

See language-specific client libraries in `/clients/` directory.
