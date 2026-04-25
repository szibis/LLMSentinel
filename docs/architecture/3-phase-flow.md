# Three-Phase Validation Flow

Claude Escalate uses a three-phase approach to model routing and token management: **Phase 1 (Estimation)**, **Phase 2 (Monitoring)**, and **Phase 3 (Validation & Learning)**.

---

## Phase 1: Pre-Response (Estimation & Routing Decision)

**When it happens**: Immediately when you submit a prompt to Claude Code  
**Duration**: <50ms  
**Key task**: Decide which model to use

### What Happens

1. **Prompt Analysis**
   - Detect task type (concurrency, parsing, debugging, etc.)
   - Estimate complexity (0.0-1.0)
   - Analyze user sentiment (baseline from prior interactions)

2. **Token Estimation**
   - Estimate input tokens (usually ~20-30% of prompt)
   - Estimate output tokens (task-type dependent, historical data)
   - Calculate estimated total cost in USD

3. **Sentiment Detection**
   - Check if user shows signs of frustration, confusion, impatience
   - If frustration detected: escalate model proactively
   - Store baseline sentiment for learning

4. **Budget Checking**
   - Verify request fits within daily/monthly budget
   - Check per-model daily limits
   - Check per-task-type token limits
   - If over budget: recommend cheaper model or reject

5. **Routing Decision**
   - Select best model: Haiku (fast/cheap) vs Sonnet (balanced) vs Opus (capable)
   - Choose based on: task type, frustration, budget, historical success
   - Store decision with confidence score

6. **Create Validation Record**
   - Assign unique validation_id
   - Store Phase 1 data (all estimates, decision rationale)
   - Mark status: "estimate_only"

### Data Available

```json
{
  "phase": 1,
  "validation_id": "uuid-abc123",
  "task_analysis": {
    "detected_type": "concurrency",
    "complexity": 0.72,
    "sentiment_baseline": "neutral"
  },
  "estimation": {
    "estimated_input_tokens": 400,
    "estimated_output_tokens": 1200,
    "estimated_total_tokens": 1600,
    "estimated_cost_usd": 0.096
  },
  "budget_check": {
    "daily_budget": 10.0,
    "daily_used": 3.50,
    "daily_remaining": 6.50,
    "within_budget": true,
    "warnings": []
  },
  "routing_decision": {
    "recommended_model": "sonnet",
    "confidence": 0.92,
    "rationale": "Balanced cost-quality for concurrency tasks",
    "alternative_if_limited": "haiku"
  }
}
```

---

## Phase 2: During-Response (Real-Time Monitoring)

**When it happens**: While Claude is generating the response  
**Duration**: ~0.5-5 seconds (typical response time)  
**Key task**: Track tokens and monitor for issues

### What Happens

1. **Real-Time Token Tracking**
   - Poll statusline source (Barista, Claude native, webhook, etc.)
   - Extract tokens flowing: input used, output generated so far
   - Calculate total and remaining estimate
   - Compare actual vs predicted

2. **Budget Monitoring**
   - Track spending against daily/monthly limits
   - Check if on track to exceed any budget
   - Prepare protective actions if needed

3. **Implicit Sentiment Sampling**
   - Detect user activity (pause, edits, rapid follow-ups)
   - Infer frustration from interaction patterns
   - Flag if sentiment deteriorating

4. **Protective Actions** (if needed)
   - Warn in statusline if approaching budget
   - Log potential budget violations
   - Prepare to cascade/downgrade if needed

### Data Available

```json
{
  "phase": 2,
  "validation_id": "uuid-abc123",
  "tokens_flowing": {
    "input_tokens_used": 350,
    "output_tokens_so_far": 340,
    "total_so_far": 690,
    "estimated_remaining": 910,
    "trend": "ON_TRACK"
  },
  "budget_status": {
    "daily_used_so_far": 3.74,
    "daily_remaining": 6.26,
    "on_track_for_daily": true,
    "warnings": []
  },
  "sentiment_signal": {
    "user_pausing": false,
    "edit_activity": "normal",
    "frustration_risk": 0.15
  }
}
```

---

## Phase 3: Post-Response (Validation & Learning)

**When it happens**: After Claude finishes responding  
**Duration**: ~100-200ms (analysis + storage)  
**Key task**: Record actual results and learn patterns

### What Happens

1. **Extract Actual Metrics**
   - Get final token counts from Claude
   - Calculate actual cost (input + output tokens × prices)
   - Compare estimate accuracy

2. **Assess User Sentiment**
   - Detect success signals: "works", "perfect", "thanks", "exactly"
   - Detect failure signals: "broken", "still failing", "doesn't work"
   - Detect confusion: "why", "confused", "don't understand"
   - Calculate sentiment confidence

3. **Record Outcome**
   - Token efficiency (estimated vs actual)
   - User satisfaction (success/failure/confusion)
   - Task type performance
   - Model effectiveness for this task type

4. **Update Learning Patterns**
   - Store: (task_type, model, sentiment_initial, sentiment_final) → success_rate
   - Update model satisfaction statistics
   - Track frustration events (if user got frustrated)
   - Update cost trends

5. **Make Next Routing Decision**
   - Based on results, what should next task use?
   - If success: cascade to cheaper model
   - If failure: escalate to better model
   - Store decision with learning confidence

### Data Available

```json
{
  "phase": 3,
  "validation_id": "uuid-abc123",
  "actual_tokens": {
    "input_tokens": 380,
    "output_tokens": 320,
    "total_actual": 700,
    "cost_usd": 0.042
  },
  "accuracy": {
    "estimated_total": 1600,
    "actual_total": 700,
    "error_percent": -56.2,
    "efficiency": "EXCELLENT"
  },
  "user_sentiment": {
    "final_sentiment": "satisfied",
    "success_detected": true,
    "success_signal": "Perfect! That's exactly what I needed.",
    "confidence": 0.95
  },
  "learning": {
    "task_type": "concurrency",
    "model_used": "sonnet",
    "sentiment_initial": "neutral",
    "sentiment_final": "satisfied",
    "success": true,
    "confidence": 0.95
  },
  "next_decision": {
    "action": "cascade",
    "recommended_model": "haiku",
    "rationale": "User satisfied, model over-provisioned for this task type",
    "estimated_savings": 0.038
  }
}
```

---

## Complete Flow Diagram

```
User types prompt
    ↓
Phase 1: ESTIMATION (50ms)
├─ Analyze task type & complexity
├─ Detect sentiment (baseline)
├─ Estimate tokens (input, output)
├─ Check budgets (daily, monthly, per-model)
├─ Route decision (Haiku/Sonnet/Opus)
└─ Create validation record
    ↓
Phase 2: MONITORING (0.5-5s)
├─ Poll statusline source
├─ Track real-time tokens
├─ Monitor budget spending
├─ Sample implicit sentiment
└─ Log metrics every 100ms
    ↓
Claude generates response...
    ↓
Phase 3: VALIDATION (100-200ms)
├─ Extract actual token counts
├─ Assess user sentiment
├─ Record outcome (success/failure)
├─ Update learning patterns
└─ Make next routing decision
    ↓
Analytics Dashboard
├─ Shows all 3 phases
├─ Tracks sentiment trends
├─ Displays budget usage
└─ Recommends optimizations
```

---

## Key Insights

### Estimation Accuracy
- **Phase 1**: Based on task type + historical average
- **Phase 2**: Real-time tracking shows actual vs estimated
- **Phase 3**: Actual tokens recorded, accuracy feedback stored
- **Learning**: Improves Phase 1 estimates for future similar tasks

### Sentiment Flow
```
Phase 1: Baseline sentiment (from prior interactions)
         ↓
Phase 2: Implicit signals (activity patterns)
         ↓
Phase 3: Explicit signal (user message feedback)
         ↓
Learning: Store (task_type, model, sentiment_initial → sentiment_final)
```

### Budget Protection
```
Phase 1: Check if request fits within limits
         ↓
Phase 2: Monitor if on track to exceed
         ↓
Phase 3: Record actual spending + update limits
         ↓
Phase 1 (next request): Use updated remaining budget
```

---

## See Also

- [Sentiment Detection](sentiment-detection.md) — How frustration is detected and handled
- [Token Validation](token-validation.md) — Estimation accuracy and learning
- [API Reference](../integration/api-reference.md) — Query any phase's data
