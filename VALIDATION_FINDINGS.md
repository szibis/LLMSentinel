# Cost Validation Framework - Research Findings

**Date**: April 25, 2026  
**Objective**: Investigate where Claude Code tracks token metrics and how to validate our estimated cost savings against actual usage  
**Status**: ✅ Research Complete - Implementation Plan Established

---

## Key Discovery: Token Metrics Location

### Where Claude Code Exposes Token Data ✅

**Found in**: `~/.claude/barista/modules/context.sh`

Claude Code **DOES expose actual token metrics**, but in a very specific context:

```javascript
// Claude's statusline JSON structure (passed to barista modules)
{
  "context_window": {
    "context_window_size": 200000,
    "current_usage": {
      "input_tokens": 2500,
      "cache_creation_input_tokens": 300,
      "cache_read_input_tokens": 800,
      "output_tokens": 450
    }
  }
}
```

**Source**: This data is available to barista modules during statusline rendering (AFTER Claude generates responses)

**Key insight**: This is the authoritative source for actual token consumption. Barista modules successfully parse and display this data.

---

## The Hook Timing Problem

### Hooks Run BEFORE Response Generation

```
Timeline:
┌─────────────────────────────────────────┐
│  User types: "/escalate to opus"        │
├─────────────────────────────────────────┤
│  ⏱️ Hook runs (UserPromptSubmit)        │
│     - Can see: prompt text only         │
│     - Cannot see: output tokens yet     │
│     - Estimates: based on prompt length │
├─────────────────────────────────────────┤
│  ⏳ Claude processes prompt              │
│  ⏳ Claude generates response            │
├─────────────────────────────────────────┤
│  ⏱️ Barista runs (statusline render)    │
│     - Can see: ACTUAL token counts      │
│     - This is when we can validate      │
└─────────────────────────────────────────┘
```

**Implication**: We can't validate in the hook. We need a POST-generation mechanism.

---

## Solution: Two-Phase Validation Strategy

### Phase 1: Pre-Response (Hook)
✅ **What we do now**:
1. Hook receives prompt
2. Detects effort level
3. **Estimates output tokens** based on prompt complexity
4. Records to database as "validation_estimate"
5. Returns model routing decision

### Phase 2: Post-Response (Barista Module)
⚠️ **What we need to add**:
1. Custom barista module captures actual tokens
2. **Extracts from Claude's statusline JSON**
3. Sends to validation endpoint
4. Service **matches with estimate** and calculates error
5. Dashboard shows comparison

---

## Implementation Approach

### Option A: Custom Barista Module (RECOMMENDED)

**Advantages**:
- ✅ Accesses actual token data directly
- ✅ Automatic per-response
- ✅ No user setup needed
- ✅ Lightweight (~50 lines of bash)

**Disadvantages**:
- ❌ Requires barista to be installed
- ❌ Requires adding new module file

**File to create**:
```bash
~/.claude/barista/modules/escalation-validation.sh
```

**What it does**:
```bash
# Extract actual tokens from Claude's statusline
input_tokens=$(echo "$input" | jq '.context_window.current_usage.input_tokens')
output_tokens=$(echo "$input" | jq '.context_window.current_usage.output_tokens')
cache_tokens=$(...)

# Send to our service
curl -X POST http://localhost:9000/api/validate \
  -d "{actual_tokens: $total, ...}"
```

### Option B: Enhanced Hook (Fallback)

**Advantages**:
- ✅ No additional setup needed
- ✅ Works immediately

**Disadvantages**:
- ❌ Can't get actual output tokens (runs before generation)
- ❌ Must estimate output tokens (heuristic-based)
- ❌ Less accurate validation

**Approach**:
```bash
# In hook, estimate output based on prompt
estimated_output = (prompt_length / 4) + 100
```

---

## What We Already Have

### ✅ Store Layer
- New `bucketValidation` in database
- `ValidationMetric` struct with all fields
- Methods: `LogValidationMetric()`, `GetValidationMetrics()`, `GetValidationStats()`

### ✅ Service API Endpoints
- `POST /api/validate` — Accept actual token data
- `GET /api/validation/metrics` — Retrieve validation records
- `GET /api/validation/stats` — Summary statistics

### ✅ Documentation
- `VALIDATION_INTEGRATION.md` — Complete implementation guide
- Step-by-step setup instructions
- Code examples for barista module
- Troubleshooting guide

---

## What Needs Implementation

### Phase 1: Barista Module (30 min)
- [ ] Create `~/.claude/barista/modules/escalation-validation.sh`
- [ ] Parse Claude's statusline JSON
- [ ] Send actual tokens to service
- [ ] Verify module runs successfully

### Phase 2: Dashboard Integration (45 min)
- [ ] Add validation section to dashboard
- [ ] Display validation metrics table
- [ ] Create comparison charts (estimated vs actual)
- [ ] Show accuracy statistics

### Phase 3: Data Collection (1 week)
- [ ] Install barista module
- [ ] Run system normally
- [ ] Collect 100+ validation records
- [ ] Analyze patterns

### Phase 4: Validation Report (30 min)
- [ ] Generate accuracy report
- [ ] Calculate error percentages
- [ ] Identify estimation gaps
- [ ] Recommend adjustments

---

## Expected Metrics to Track

### For Each Validation Record

```json
{
  "session_id": "abc123",
  "timestamp": "2026-04-25T15:30:00Z",
  
  "hook_data": {
    "prompt": "What is machine learning?",
    "detected_effort": "low",
    "estimated_input_tokens": 270,
    "estimated_output_tokens": 500,
    "estimated_total_tokens": 770,
    "estimated_cost": "$0.0077"
  },
  
  "actual_data": {
    "actual_input_tokens": 268,
    "actual_cache_creation_tokens": 0,
    "actual_cache_read_tokens": 0,
    "actual_output_tokens": 474,
    "actual_total_tokens": 742,
    "actual_cost": "$0.0074"
  },
  
  "validation": {
    "token_error_percent": -3.6,
    "cost_error_percent": -3.9,
    "model_match": true,
    "validation_id": 42
  }
}
```

---

## Success Criteria

### Accuracy Targets
- ✅ Task classification: **85%+** (low/medium/high effort)
- ✅ Token estimation: **±15%** error
- ✅ Cost estimation: **±10%** error
- ✅ Cascade savings: **40%+** reduction vs baseline

### Data Quality
- ✅ 100+ validation records per week
- ✅ 95%+ validation completion rate
- ✅ Proper timestamping and linking
- ✅ No missing token values

### Performance
- ✅ Zero latency impact (<1ms per validation)
- ✅ Barista module runs without errors
- ✅ Dashboard updates in real-time
- ✅ No database corruption

---

## Known Limitations

### Prompt Context Not Captured
Currently, we can estimate output tokens but can't see:
- Conversation history length
- System prompt complexity
- File context size
- Tool definitions

**Workaround**: Barista gives us actual tokens, so we can compare estimates to reality.

### Cache Effects Not Estimated
Hook doesn't know about:
- Prompt cache hits (cache_read_input_tokens)
- Cache creation overhead
- Cache validity

**Workaround**: Actual tokens include cache effects, so error metrics will show cache impact.

### Per-Request Cost Invisible
Claude doesn't expose:
- Per-token billing rates
- Discount eligibility
- Rate limit consumption

**Workaround**: Use published pricing rates and calculate cost from token counts.

---

## Architecture: Validation Flow

```
Hook (Pre-response)
  ├─ Receives: prompt text
  ├─ Detects: effort level, /escalate command
  ├─ Estimates: input/output tokens
  ├─ Records: validation_metric (estimated only)
  └─ Returns: model routing decision

[Claude processes and generates response]

Barista Module (Post-response)
  ├─ Receives: Claude's statusline JSON
  ├─ Extracts: actual token counts
  ├─ Calls: POST /api/validate
  └─ Service updates: validation_metric (adds actual data)

Service Validation Endpoint
  ├─ Receives: actual token metrics
  ├─ Finds: matching estimate record
  ├─ Calculates: error percentages
  ├─ Stores: comparison results
  └─ Returns: validation_id

Dashboard
  ├─ Queries: GET /api/validation/metrics
  ├─ Displays: estimated vs actual tables
  ├─ Shows: accuracy statistics
  └─ Updates: every 2 seconds
```

---

## Next Immediate Steps

### 1. Install Barista Module (10 minutes)
```bash
# Create the validation module
mkdir -p ~/.claude/barista/modules
cat > ~/.claude/barista/modules/escalation-validation.sh << 'EOF'
#!/bin/bash
module_escalation_validation() {
  local input="$1"
  local total=$(echo "$input" | jq '.context_window.current_usage | ... sum ...')
  curl -s -X POST http://localhost:9000/api/validate \
    -H "Content-Type: application/json" \
    -d "{\"actual_total_tokens\": $total}"
}
EOF
chmod +x ~/.claude/barista/modules/escalation-validation.sh

# Enable in barista config
echo 'MODULE_ESCALATION_VALIDATION="true"' >> ~/.claude/barista/barista.conf
```

### 2. Rebuild Binary (5 minutes)
```bash
cd /tmp/claude-escalate
go build -o claude-escalate ./cmd/claude-escalate
cp claude-escalate ~/.local/bin/escalation-manager
```

### 3. Restart Service (1 minute)
```bash
pkill -f "escalation-manager service"
escalation-manager service --port 9000 &
```

### 4. Use Normally (ongoing)
- Type commands with `/escalate`
- Say success phrases like "works!"
- Dashboard collects validation data automatically

### 5. Check Dashboard (2 minutes)
- Go to http://localhost:9000/
- Look for new "Validation" section
- Watch metrics accumulate in real-time

---

## Success Indicators

### After 1 hour
- ✅ Barista module installed and running
- ✅ Service receives actual token data
- ✅ 5-10 validation records in database

### After 1 day
- ✅ 50+ validation records
- ✅ Dashboard showing comparison charts
- ✅ Patterns starting to emerge

### After 1 week
- ✅ 300+ validation records
- ✅ 85%+ accuracy statistics
- ✅ Clear cost savings validation

---

## Deliverables Summary

### Code Changes
- ✅ `internal/store/store.go` — Add validation tables & methods
- ✅ `internal/service/service.go` — Add validation endpoints
- ✅ `hooks/http-hook.sh` — Ready for token estimates (optional enhancement)

### Documentation
- ✅ `VALIDATION_INTEGRATION.md` — Complete implementation guide (470+ lines)
- ✅ `VALIDATION_FINDINGS.md` — This document (research summary)
- ✅ `README.md` — Updated with validation feature
- ✅ `COST_VALIDATION.md` — Original framework (reference)

### Next Files Needed
- ⚠️ `~/.claude/barista/modules/escalation-validation.sh` — (User creates from guide)
- ⚠️ Dashboard validation section — (UI enhancement, in separate PR)

---

## Questions Answered

### Q: Where does Claude Code track token metrics?
**A**: In the statusline input JSON passed to barista modules, at `.context_window.current_usage`. Found in barista's context.sh module.

### Q: Can hooks access actual token counts?
**A**: No - hooks run before response generation. But barista modules (which run after) can access them.

### Q: How do we compare estimates to actuals?
**A**: Barista module sends actual tokens to our validation endpoint, which matches them with the earlier estimate and calculates error percentages.

### Q: What accuracy can we expect?
**A**: For estimation errors, ±10-20% is typical depending on conversation history and cache effects. Our targets are ±15% for tokens and ±10% for cost.

### Q: How long to implement?
**A**: ~1-2 hours total (barista module + dashboard updates + testing). Data collection takes 1 week for statistical significance.

---

## Conclusion

✅ **Research complete**: We found the token metrics source  
✅ **Solution designed**: Two-phase validation strategy with barista integration  
✅ **Code ready**: Store layer and API endpoints implemented  
✅ **Documentation complete**: Step-by-step guide provided  
⏭️ **Ready for implementation**: User can follow VALIDATION_INTEGRATION.md to deploy

The validation framework will provide **objective proof** of cost savings, or identify where we need to adjust our estimates.

