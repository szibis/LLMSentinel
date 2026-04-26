# Escalation System Cost Validation Framework

**Objective**: Compare our hook's estimated cost savings vs. Claude's actual token usage

## Problem Statement

Our escalation system claims:
- ✅ Haiku routes simple tasks (1x cost)
- ✅ Sonnet routes medium tasks (8x cost)
- ✅ Opus routes complex tasks (30x cost)
- ✅ Cascades save tokens by downgrading after solving

But we need to **validate these claims against reality**:
1. Does Claude actually report different token costs for each model?
2. Are our task classifications accurate?
3. Is the cost calculation correct?
4. How much are we actually saving?

## Data We Need to Collect

### From Claude Code (Actual Metrics)
```json
{
  "session_id": "...",
  "timestamp": "2026-04-25T10:00:00Z",
  "model_used": "claude-haiku-4-5-20251001",
  "input_tokens": 450,
  "output_tokens": 320,
  "total_tokens": 770,
  "cost_cents": 0.77,
  "task_type": "search"
}
```

### From Our Hook (Estimated Metrics)
```json
{
  "session_id": "...",
  "timestamp": "2026-04-25T10:00:00Z",
  "prompt": "What is the capital of France?",
  "detected_task_type": "search",
  "detected_effort": "low",
  "routed_model": "haiku",
  "estimated_tokens": 770,
  "estimated_cost_cents": 0.77,
  "escalations": 0,
  "cascades": 0
}
```

## Comparison Metrics

### 1. Task Classification Accuracy

**Hypothesis**: Our auto-effort detection correctly classifies task types

```
For 100 sessions, measure:
├─ Accuracy: % of tasks routed to expected model
├─ Precision: When we say "low", is it actually simple?
├─ Recall: Do we catch all simple tasks?
└─ F1-score: Balanced accuracy metric

Target: 85%+ accuracy
```

### 2. Token Cost Correlation

**Hypothesis**: Actual tokens match our estimates

```
For each (task_type, model) pair:
├─ Estimate: What we predicted
├─ Actual: What Claude reported
├─ Error: Estimate vs Actual (%)
└─ Confidence: How confident are we?

Model token baselines:
├─ Haiku: 50 tokens/session ± 30%
├─ Sonnet: 200 tokens/session ± 20%
└─ Opus: 500 tokens/session ± 10%

Target: ±15% error
```

### 3. Cost Savings Validation

**Hypothesis**: Cascading saves tokens

```
Scenario: Opus → Sonnet → Haiku cascade

Without cascade:
  Opus for all: 3 × 500 = 1,500 tokens

With cascade (on success):
  Opus (solve): 500 tokens
  Sonnet (verify): 200 tokens
  Haiku (document): 50 tokens
  Total: 750 tokens
  
Savings: 1,500 - 750 = 750 tokens (50%)
```

### 4. Model Cost Multipliers

**Hypothesis**: Cost ratios match reality

```
If Haiku = $0.80/M input, $2.40/M output
If Sonnet = $3/M input, $15/M output
If Opus = $15/M input, $60/M output

Then:
├─ Haiku ÷ Sonnet = 0.125x (our assumption: 1/8)
├─ Sonnet ÷ Opus = 0.267x (our assumption: 8/30)
└─ Haiku ÷ Opus = 0.033x (our assumption: 1/30)

Need to verify against actual pricing
```

## Implementation Plan

### Phase 1: Add Metrics Collection (Service)

Update `internal/service/service.go` to log:
```go
type SessionMetrics struct {
    SessionID       string
    Timestamp       time.Time
    Prompt          string
    TaskType        string
    DetectedEffort  string
    RoutedModel     string
    EstimatedTokens int
    EstimatedCost   float64
}

// Log after each escalation
s.db.LogMetrics(metrics)
```

### Phase 2: Capture Claude's Actual Data

Option A: Parse Claude Code's logs
```bash
# If Claude Code exposes token metrics
~/.claude/metrics/session-*.json

Extract:
├─ model_used
├─ input_tokens
├─ output_tokens
├─ total_tokens
└─ estimated_cost
```

Option B: Add to Hook Output
```json
{
  "continue": true,
  "suppressOutput": true,
  "metrics": {
    "actual_model": "claude-haiku-4-5-20251001",
    "actual_tokens": 770,
    "actual_cost": 0.0077
  }
}
```

Option C: Monitor Via Barista
```bash
# Barista might expose Claude's current metrics
# Check ~/.claude/barista/ cache files
```

### Phase 3: Dashboard Comparison View

Add to dashboard:
```
┌──────────────────────────────────────────┐
│  COST VALIDATION DASHBOARD               │
├──────────────────────────────────────────┤
│                                          │
│  Estimated vs Actual Cost                │
│  ├─ Estimated: 750 tokens               │
│  ├─ Actual: 742 tokens                  │
│  ├─ Error: -1.1% ✓                      │
│  └─ Confidence: 98%                     │
│                                          │
│  Task Classification Accuracy             │
│  ├─ Low effort: 87% correct             │
│  ├─ Medium effort: 82% correct          │
│  ├─ High effort: 91% correct            │
│  └─ Overall: 87%                        │
│                                          │
│  Cascade Effectiveness                   │
│  ├─ Sessions with cascade: 23           │
│  ├─ Avg savings per cascade: 45%        │
│  └─ Total saved: 12,340 tokens          │
│                                          │
└──────────────────────────────────────────┘
```

### Phase 4: Validation Reports

Generate weekly reports:
```markdown
# Escalation System Validation Report
## Week of April 21-27, 2026

### Task Classification
- Low effort: 87% accuracy (89 correct / 102 total)
- Medium effort: 82% accuracy (72 correct / 88 total)
- High effort: 91% accuracy (104 correct / 114 total)
- **Overall: 87%** ✓ (above 85% target)

### Token Cost Estimation
- Haiku: 52 tokens ± 28% (target: 50 ± 30%) ✓
- Sonnet: 198 tokens ± 18% (target: 200 ± 20%) ✓
- Opus: 485 tokens ± 12% (target: 500 ± 10%) ✗
  (Opus is consistently lower than expected)

### Cost Savings
- Total escalations: 12
- Total cascades: 8
- Cascade success rate: 67%
- Average savings per cascade: 45%
- Total tokens saved: 4,230 tokens
- Total cost saved: ~$0.021

### Findings
1. ✓ Task classification is accurate (87%)
2. ✓ Haiku/Sonnet cost estimates are correct
3. ⚠ Opus might be cheaper than estimated
4. ✓ Cascading is working as expected

### Recommendations
- Update Opus estimate to 450 tokens (was 500)
- Increase cascade timeout confidence
- Continue monitoring classification accuracy
```

## Validation Checklist

### Week 1
- [ ] Add metrics logging to service
- [ ] Identify where Claude reports token data
- [ ] Create sample comparison for one session
- [ ] Document actual vs estimated for 10 sessions

### Week 2
- [ ] Build dashboard comparison view
- [ ] Collect data from 100 sessions
- [ ] Calculate accuracy metrics
- [ ] Compare against targets

### Week 3
- [ ] Generate validation report
- [ ] Identify discrepancies
- [ ] Adjust estimates if needed
- [ ] Update documentation

## Success Criteria

✓ Task classification: 85%+ accuracy  
✓ Token estimation: ±15% error  
✓ Cascade effectiveness: 40%+ savings  
✓ Cost multipliers: Match actual pricing  
✓ Confidence intervals: 95%+  

## Example Validation Session

### Input
```
User: "What is machine learning?"  → Detected: low effort
Hook: Route to Haiku
Claude Code: Uses Haiku model
```

### Metrics Collected
```
Estimated:
├─ Tokens: 770
├─ Cost: $0.0077
└─ Model: Haiku

Actual (from Claude):
├─ Tokens: 742
├─ Cost: $0.0074
└─ Model: Haiku

Validation:
├─ Accuracy: ✓ (correct model)
├─ Error: -3.6% (within ±15%)
└─ Confidence: 98%
```

## Data Collection Points

For each session, capture:

```json
{
  "session": {
    "id": "sess_abc123",
    "timestamp": "2026-04-25T10:00:00Z",
    "duration_ms": 4500
  },
  "prompt": {
    "text": "What is machine learning?",
    "length_chars": 27,
    "keywords": ["machine", "learning"]
  },
  "hook_estimation": {
    "detected_task_type": "search",
    "detected_effort": "low",
    "routed_model": "haiku",
    "estimated_input_tokens": 270,
    "estimated_output_tokens": 500,
    "estimated_total_tokens": 770,
    "estimated_cost_cents": 0.77
  },
  "actual_usage": {
    "model_used": "claude-haiku-4-5-20251001",
    "actual_input_tokens": 268,
    "actual_output_tokens": 474,
    "actual_total_tokens": 742,
    "actual_cost_cents": 0.74
  },
  "validation": {
    "model_match": true,
    "token_error_percent": -3.6,
    "cost_error_percent": -3.9,
    "accuracy_score": 98,
    "confidence": 0.98
  }
}
```

## Implementation in Service

```go
// In internal/service/service.go
type CostValidator struct {
    EstimatedTokens  int     // What we predicted
    ActualTokens     int     // What Claude used
    EstimatedCost    float64 // What we predicted
    ActualCost       float64 // What Claude charged
    Confidence       float32 // How confident (0-100%)
}

func (s *Service) ValidateCosts(sessionID string) error {
    // Compare estimated vs actual
    // Calculate errors
    // Log validation results
    // Update confidence intervals
}
```

## Open Questions

1. **Where can we get Claude's actual token metrics?**
   - Does Claude Code expose this in settings.json?
   - Is it in barista-cache?
   - Can we get it from Claude's API logs?

2. **How do we get output token count?**
   - Our system only sees the prompt (input tokens)
   - Need output tokens to validate total cost
   - Where is this information available?

3. **What's the actual pricing model?**
   - Our assumptions may be outdated
   - Need current pricing for Haiku/Sonnet/Opus
   - Are there input/output token differences?

4. **How often should we validate?**
   - Per session? (too granular)
   - Per day? (reasonable)
   - Per week? (summary view)

## Next Actions

1. **Research**: Find where Claude Code logs token metrics
2. **Implement**: Add metrics collection to service
3. **Validate**: Compare 100+ sessions
4. **Report**: Generate validation dashboard
5. **Adjust**: Update estimates based on findings

This framework will give us **objective proof** that our escalation system is actually saving costs or reveal where we need to adjust our estimates.
