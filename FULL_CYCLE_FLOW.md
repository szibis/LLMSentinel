# Complete Full-Cycle Validation Flow

**Goal**: Both hook AND barista report metrics to same endpoint for complete visibility.

---

## End-to-End Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│ USER INTERACTION                                                    │
│                                                                     │
│ User: "What is machine learning?" (simple question)               │
└──────────────────────────┬──────────────────────────────────────────┘
                           │
                ┌──────────▼──────────┐
                │ Claude Code Detects │
                │    User Prompt      │
                └──────────┬──────────┘
                           │
┌──────────────────────────▼────────────────────────────────────────┐
│ PHASE 1: PRE-RESPONSE (Hook Executes)                            │
├──────────────────────────────────────────────────────────────────┤
│                                                                  │
│ http-hook.sh receives: "What is machine learning?"              │
│                                                                  │
│ STEP 1A: Parse & Estimate                                       │
│ ├─ Detects: low effort (keyword "what is")                     │
│ ├─ Detects: no /escalate command                               │
│ ├─ Detects: not success signal                                 │
│ ├─ Estimates input tokens: 27 chars / 4 = ~7 tokens           │
│ ├─ Estimates output tokens: 7 / 4 + base = ~500 tokens        │
│ └─ Estimates total: 507 tokens @ Haiku price = ~$0.005         │
│                                                                  │
│ STEP 1B: Report Estimated Metrics to Service                    │
│ ├─ POST /api/metrics/hook                                       │
│ └─ {                                                             │
│     "prompt": "What is machine learning?",                      │
│     "detected_task_type": "general",                            │
│     "detected_effort": "low",                                   │
│     "routed_model": "haiku",                                    │
│     "estimated_input_tokens": 27,                               │
│     "estimated_output_tokens": 500,                             │
│     "estimated_total_tokens": 527,                              │
│     "estimated_cost": 0.005                                     │
│   }                                                              │
│                                                                  │
│ SERVICE RESPONSE:                                                │
│ {                                                                │
│   "success": true,                                              │
│   "validation_id": 42,                                          │
│   "estimated": 527,                                             │
│   "effort": "low",                                              │
│   "model": "haiku"                                              │
│ }                                                                │
│                                                                  │
│ STEP 1C: Return Routing Decision to Claude Code                 │
│ ├─ POST /api/hook (routing endpoint)                            │
│ └─ Returns: {"continue": true, "currentModel": "haiku"}         │
│                                                                  │
│ [Hook completes - message sent to Claude]                       │
│                                                                  │
└───────────────────────────┬─────────────────────────────────────┘
                            │
            ┌───────────────▼───────────────┐
            │  Claude Code Processes        │
            │  (2-3 seconds)                │
            │                               │
            │  • Loads prompt               │
            │  • Generates response         │
            │  • Counts tokens              │
            │  • Actual output: 468 tokens  │
            │                               │
            └───────────────┬───────────────┘
                            │
┌───────────────────────────▼────────────────────────────────────────┐
│ PHASE 2: POST-RESPONSE (Barista/Integration Reports)              │
├────────────────────────────────────────────────────────────────────┤
│                                                                    │
│ [Option A] Barista Module Executes                                │
│ ├─ Reads Claude's statusline JSON                                │
│ │  {                                                              │
│ │    "context_window": {                                         │
│ │      "current_usage": {                                        │
│ │        "input_tokens": 25,                                     │
│ │        "cache_creation_input_tokens": 0,                       │
│ │        "cache_read_input_tokens": 0,                           │
│ │        "output_tokens": 468                                    │
│ │      }                                                          │
│ │    }                                                            │
│ │  }                                                              │
│ │                                                                │
│ ├─ Calculates total: 25 + 0 + 0 + 468 = 493 tokens             │
│ ├─ Calculates cost: 493 * 0.00001 = $0.00493                   │
│ │                                                                │
│ ├─ POST /api/validate (actual metrics)                           │
│ └─ {                                                             │
│     "actual_input_tokens": 25,                                   │
│     "actual_cache_creation_tokens": 0,                           │
│     "actual_cache_read_tokens": 0,                               │
│     "actual_output_tokens": 468,                                 │
│     "actual_total_tokens": 493,                                  │
│     "actual_cost": 0.00493                                       │
│   }                                                              │
│                                                                  │
│ [Option B] Post-Response Hook / CLI / Daemon (Alternative)       │
│ ├─ Any of these can ALSO POST /api/validate                      │
│ ├─ Same endpoint, same data format                               │
│ └─ Service deduplicates if multiple sources report               │
│                                                                  │
│ SERVICE RESPONSE:                                                 │
│ {                                                                 │
│   "success": true,                                               │
│   "validation_id": 42,                                           │
│   "tokens_recorded": 493                                         │
│ }                                                                 │
│                                                                  │
└───────────────────────────┬────────────────────────────────────────┘
                            │
┌───────────────────────────▼────────────────────────────────────────┐
│ PHASE 3: SERVICE MATCHING & CALCULATION                           │
├────────────────────────────────────────────────────────────────────┤
│                                                                    │
│ SERVICE LOGIC (in memory):                                        │
│                                                                    │
│ LOOKUP validation_id=42 in database                               │
│ ├─ Found estimate: 527 tokens, cost=$0.005                       │
│ ├─ Received actual: 493 tokens, cost=$0.00493                    │
│ │                                                                │
│ CALCULATE ERROR:                                                  │
│ ├─ Token error = (493 - 527) / 527 = -6.5%   ✅ (within ±15%)   │
│ ├─ Cost error = (0.00493 - 0.005) / 0.005 = -1.4% ✅            │
│ │                                                                │
│ UPDATE VALIDATION RECORD:                                         │
│ └─ ValidationMetric {                                            │
│     ID: 42,                                                       │
│     Timestamp: 2026-04-25T15:52:00Z,                             │
│     Prompt: "What is machine learning?",                         │
│     DetectedTaskType: "general",                                 │
│     DetectedEffort: "low",                                       │
│     RoutedModel: "haiku",                                        │
│     EstimatedInputTokens: 27,                                    │
│     EstimatedOutputTokens: 500,                                  │
│     EstimatedTotalTokens: 527,                                   │
│     EstimatedCost: 0.005,                                        │
│     ActualInputTokens: 25,                                       │
│     ActualOutputTokens: 468,                                     │
│     ActualTotalTokens: 493,                                      │
│     ActualCost: 0.00493,                                         │
│     TokenError: -6.5,                                            │
│     CostError: -1.4,                                             │
│     Validated: true                                              │
│   }                                                              │
│                                                                  │
│ AGGREGATE STATS:                                                  │
│ ├─ Total validations: 42                                         │
│ ├─ Average token error: -3.2% (within ±15%) ✅                   │
│ ├─ Average cost error: -2.1% (within ±10%) ✅                    │
│ ├─ Total estimated tokens: 12,340 (all sessions)                │
│ ├─ Total actual tokens: 11,920 (all sessions)                   │
│ ├─ Token savings vs estimate: 420 tokens saved! 📊                │
│ └─ Cost savings vs estimate: $0.0042 saved! 💰                   │
│                                                                  │
└───────────────────────────┬────────────────────────────────────────┘
                            │
┌───────────────────────────▼────────────────────────────────────────┐
│ PHASE 4: DASHBOARD DISPLAYS EVERYTHING                            │
├────────────────────────────────────────────────────────────────────┤
│                                                                    │
│ GET /api/validation/metrics → Returns all 42 records             │
│ GET /api/validation/stats → Returns aggregate statistics         │
│                                                                    │
│ DASHBOARD DISPLAYS:                                               │
│                                                                    │
│ ┌─ CURRENT SESSION ───────────────────────────────────┐          │
│ │ Prompt: "What is machine learning?"                 │          │
│ │ Effort: Low 💡                                       │          │
│ │ Model: Haiku (fastest, cheapest) ⚡                  │          │
│ │                                                      │          │
│ │ Estimated: 527 tokens ($0.005)                      │          │
│ │ Actual:    493 tokens ($0.00493)                    │          │
│ │ ✅ Accuracy: -6.5% (excellent!)                     │          │
│ └──────────────────────────────────────────────────────┘          │
│                                                                    │
│ ┌─ VALIDATION METRICS TABLE (Last 10) ─────────────┐            │
│ │ # │ Prompt │ Est. │ Act. │ Error │ Cost │ Status │            │
│ │──────────────────────────────────────────────────│            │
│ │42 │ ML?... │ 527  │ 493  │ -6.5% │ ✅  │ Valid  │            │
│ │41 │ Fix... │ 280  │ 312  │+11.4% │ ✅  │ Valid  │            │
│ │40 │ Arch..│ 1200 │ 1150 │ -4.2% │ ✅  │ Valid  │            │
│ │... │ ...   │ ...  │ ...  │  ... │ ... │  ...  │            │
│ └──────────────────────────────────────────────────────┘          │
│                                                                    │
│ ┌─ ACCURACY STATISTICS ────────────────────────────┐            │
│ │ Total Sessions: 42                               │            │
│ │ Validated: 42 (100%)                             │            │
│ │                                                  │            │
│ │ Token Estimation Error: -3.2% (avg)              │            │
│ │ ✅ Target: ±15%  Status: EXCELLENT               │            │
│ │                                                  │            │
│ │ Cost Estimation Error: -2.1% (avg)               │            │
│ │ ✅ Target: ±10%  Status: EXCELLENT               │            │
│ │                                                  │            │
│ │ Model Accuracy: 95% correct routing              │            │
│ │ ✅ Target: 85%+  Status: EXCELLENT               │            │
│ └──────────────────────────────────────────────────┘            │
│                                                                    │
│ ┌─ SAVINGS ANALYSIS ───────────────────────────────┐            │
│ │ Estimated Total: 12,340 tokens                   │            │
│ │ Actual Total:    11,920 tokens                   │            │
│ │ Difference:      -420 tokens saved! 📊             │            │
│ │                                                  │            │
│ │ Estimated Cost:  $0.1234                         │            │
│ │ Actual Cost:     $0.1192                         │            │
│ │ Difference:      -$0.0042 saved! 💰                │            │
│ │                                                  │            │
│ │ vs All-Opus Baseline:                            │            │
│ │ • Would have cost: $0.3702                       │            │
│ │ • Actually cost:   $0.1192                       │            │
│ │ • Total savings:   $0.2510 (67% cheaper!) 🚀      │            │
│ └──────────────────────────────────────────────────┘            │
│                                                                    │
└────────────────────────────────────────────────────────────────────┘
```

---

## What Gets Reported Where

### Hook Reports (Pre-Response)

**Endpoint**: `POST /api/metrics/hook`

```json
{
  "prompt": "User's actual input",
  "detected_task_type": "general|escalation|success_signal",
  "detected_effort": "low|medium|high",
  "routed_model": "haiku|sonnet|opus",
  "estimated_input_tokens": 27,
  "estimated_output_tokens": 500,
  "estimated_total_tokens": 527,
  "estimated_cost": 0.005
}
```

**What it provides**:
- ✅ What the hook predicted
- ✅ Why it made that decision
- ✅ Token estimate methodology
- ✅ Model routing reasoning

**Stored in DB**: `validation_metrics` with `validated=false` (waiting for actual)

---

### Barista Reports (Post-Response)

**Endpoint**: `POST /api/validate`

```json
{
  "actual_input_tokens": 25,
  "actual_cache_creation_tokens": 0,
  "actual_cache_read_tokens": 0,
  "actual_output_tokens": 468,
  "actual_total_tokens": 493,
  "actual_cost": 0.00493
}
```

**What it provides**:
- ✅ What Claude actually used
- ✅ Cache impact (if any)
- ✅ Real cost charged
- ✅ Breakdown by token type

**Stored in DB**: Updates same record with `validated=true`, calculates errors

---

## Data Flow: From Hook to Dashboard

```
Hook Estimation        Service Database       Barista Update       Dashboard View
┌─────────────┐        ┌──────────────┐       ┌────────────┐       ┌────────────┐
│ Prompt      │        │ Validation   │       │ Actual:    │       │ Estimated  │
│ Low effort  │──────→ │ Metric #42   │       │ 493 tokens │──────→│ 527 tokens │
│ 527 tokens  │  POST  │              │  POST │ $0.00493   │       │            │
│ Model: Hk   │        │ status: est  │       │ validated! │       │ Actual     │
└─────────────┘        └──────────────┘       └────────────┘       │ 493 tokens │
                            │                                       │            │
                            └──────────────┬──────────────┬─────────→│ Error:     │
                                      SERVICE CALCULATES           │ -6.5%  ✅  │
                            • Token error: (493-527)/527 = -6.5%   │            │
                            • Cost error: -1.4%                    │ Confidence:│
                            • Accuracy: Excellent                   │ 98%        │
                            • Stores complete record                └────────────┘
```

---

## Query Patterns: What Dashboard Can Show

```bash
# Get all validation records
curl http://localhost:9000/api/validation/metrics

# Get validation statistics
curl http://localhost:9000/api/validation/stats

# Filter by effort level (app-side)
GET /api/validation/metrics | jq '.[] | select(.detected_effort=="low")'

# Analyze by model
GET /api/validation/metrics | jq 'group_by(.routed_model)'

# Calculate cumulative savings
GET /api/validation/metrics | jq '[.[] | (.estimated_total - .actual_total)] | add'

# Check validation completeness
GET /api/validation/stats | jq '.validated / .total_metrics'
```

---

## Complete Information Available in Dashboard

### Per-Record (42 metrics shown)
- ✅ What user asked (prompt)
- ✅ What effort level detected
- ✅ What model routed to (and why)
- ✅ What tokens estimated (before generation)
- ✅ What tokens actually used (after generation)
- ✅ How accurate the estimate was (error %)
- ✅ Cost comparison (estimate vs actual)
- ✅ Timestamp and complete audit trail

### Aggregated (Summary stats)
- ✅ Total sessions validated
- ✅ Average estimation accuracy
- ✅ Token savings vs baseline
- ✅ Cost savings achieved
- ✅ Model distribution
- ✅ Effort level accuracy
- ✅ Success/cascade rates

---

## Why This Works Better

✅ **Two-sided visibility**:
- Hook side: Shows what we predicted and why
- Barista side: Shows what Claude actually did
- Service: Matches them and calculates truth

✅ **Flexible integration**:
- Barista (primary)
- Post-hook (backup)
- Daemon (fallback)
- Any source works

✅ **Complete audit trail**:
- Prompt captured
- Decision logic visible
- Actual usage measured
- Error quantified

✅ **Real cost validation**:
- Estimates vs actuals
- Model routing accuracy
- Cascade effectiveness
- Savings proven

---

## Next: Implement & Deploy

1. **Rebuild binary** (includes hook metrics endpoint)
   ```bash
   go build -o claude-escalate ./cmd/claude-escalate
   ```

2. **Install barista module** (or choose alternative)
   ```bash
   cp escalation-validation.sh ~/.claude/barista/modules/
   ```

3. **Update hook** (now reports metrics too)
   ```bash
   cp http-hook.sh ~/.claude/hooks/
   ```

4. **Restart service**
   ```bash
   escalation-manager service --port 9000 &
   ```

5. **Use normally** — Full cycle automatic!

