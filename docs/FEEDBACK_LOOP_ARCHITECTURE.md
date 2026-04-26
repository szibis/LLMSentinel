# Complete Feedback Loop Architecture

## Executive Summary

A **closed-loop feedback system** that captures data at every stage:

```
USER INPUT → ANALYZE → ESTIMATE → ROUTE → CLAUDE → ACTUAL → VALIDATE → LEARN → REACT
   ↓         ↓         ↓         ↓      ↓      ↓       ↓       ↓       ↓
 PHASE 1                                   PHASE 2           PHASE 3
(Pre-Response)                         (Response)        (Post-Response)
```

Each phase captures specific data → correlate across phases → analytics → optimization decisions.

---

## Part 1: Data Capture Points (Where & When)

### PHASE 1: PRE-RESPONSE (Happens BEFORE Claude generates anything)

**T = -5sec to 0sec** (User types prompt)

#### 1.1: User Input Capture
```
WHEN: Hook runs (UserPromptSubmit)
DATA CAPTURED:
  - Raw prompt text
  - Prompt length (characters, words, lines)
  - Timestamp (T0)
  - Session context (task type, current model, effort level)

EXAMPLE:
{
  "phase": "input",
  "timestamp": "2026-04-25T16:00:00Z",
  "prompt": "What is REST API?",
  "prompt_length_chars": 18,
  "prompt_length_words": 4,
  "session_context": {
    "current_model": "haiku",
    "current_effort": "low",
    "previous_satisfaction": "high"
  }
}
```

#### 1.2: Prompt Analysis
```
WHEN: Service /api/hook processes prompt
DATA CAPTURED:
  - Detected effort level (low/medium/high)
  - Detected keywords (complexity signals)
  - Command detection (/escalate, success signals)
  - Signal confidence

EXAMPLE:
{
  "phase": "analysis",
  "timestamp": "2026-04-25T16:00:01Z",
  "analysis": {
    "detected_effort": "low",
    "effort_confidence": 0.92,
    "keywords": ["simple", "overview"],
    "command_detected": null,
    "signals_detected": [],
    "signal_confidence": 0
  }
}
```

#### 1.3: Token Estimation
```
WHEN: Service estimates from prompt
DATA CAPTURED:
  - Estimated input tokens (from prompt length + complexity)
  - Estimated output tokens (from effort level)
  - Total estimated tokens
  - Estimation method/formula used
  - Estimation confidence

EXAMPLE:
{
  "phase": "estimation",
  "timestamp": "2026-04-25T16:00:01Z",
  "estimation": {
    "method": "heuristic",
    "prompt_length_chars": 18,
    "estimated_input_tokens": 7,
    "estimated_output_tokens": 450,
    "estimated_total_tokens": 457,
    "estimation_confidence": 0.65,
    "estimation_formula": "input=(len/4)+2, output=base(effort)+variance"
  }
}
```

#### 1.4: Model Selection
```
WHEN: Service makes routing decision
DATA CAPTURED:
  - Selected model (haiku/sonnet/opus)
  - Selection reason (effort level, cascade state, explicit command)
  - Alternative models considered
  - Selection confidence
  - Cost estimate for this model

EXAMPLE:
{
  "phase": "routing",
  "timestamp": "2026-04-25T16:00:01Z",
  "routing": {
    "selected_model": "haiku",
    "selection_reason": "low_effort_detected",
    "alternatives_considered": ["sonnet", "opus"],
    "selection_confidence": 0.92,
    "estimated_cost": 0.0046,
    "estimated_cost_vs_opus": 0.1408,
    "savings_vs_opus_percent": 96.7
  }
}
```

#### 1.5: Validation Record Creation
```
WHEN: Service creates record in database
DATA CAPTURED:
  - Unique validation ID
  - All above data stored together
  - Status: "estimate_only"

CREATES DB RECORD:
validation_metric #42 {
  id: 42,
  timestamp: "2026-04-25T16:00:01Z",
  prompt: "What is REST API?",
  prompt_length: 18,
  detected_effort: "low",
  routed_model: "haiku",
  estimated_input_tokens: 7,
  estimated_output_tokens: 450,
  estimated_total_tokens: 457,
  estimated_cost: 0.0046,
  status: "estimate_only"
}
```

**PHASE 1 OUTPUT**: Everything ready for Claude to process with model routing decision.

---

### PHASE 2: RESPONSE (Happens DURING Claude processing)

**T = 0sec to 2sec** (Claude generates response)

#### 2.1: Response Generation
```
WHEN: Claude Code processes request with selected model
DATA AVAILABLE (but not captured yet):
  - Response text being generated
  - Token count streaming (if available)
  - Model being used (haiku)
  - Start time (T0 = 0sec)

ISSUE: Hook runs BEFORE response, so can't access this data yet
SOLUTION: Monitor or post-hook needed
```

#### 2.2: Token Streaming (Optional - for real-time feedback)
```
WHEN: Claude Code generates tokens
DATA AVAILABLE:
  - Input tokens count
  - Output tokens count (streaming)
  - Cache creation tokens (if using cache)
  - Cache read tokens (if using cache)
  - Response generation time

IF CAPTURED:
  - Can show real-time token count to user
  - Can start early validations
  - Can trigger early reactions
```

**PHASE 2 LIMITATION**: No direct hook access to response data. Need external monitor or post-response integration.

---

### PHASE 3: POST-RESPONSE (Happens AFTER Claude generates response)

**T = 2-3sec** (After Claude finishes, before user sees output)

#### 3.1: Response Metadata Capture
```
WHEN: Claude Code has response metrics available
WHERE: Barista statusline provides .context_window.current_usage
HOW: Monitor daemon or post-hook queries this data

DATA CAPTURED:
{
  "phase": "post_response",
  "timestamp": "2026-04-25T16:00:03Z",
  "response_metadata": {
    "input_tokens": 7,
    "output_tokens": 447,
    "cache_creation_input_tokens": 0,
    "cache_read_input_tokens": 0,
    "total_tokens": 454,
    "generation_time_ms": 2100,
    "model_used": "haiku",
    "stop_reason": "end_turn",
    "response_length_chars": 1342
  }
}
```

#### 3.2: Validation Comparison
```
WHEN: Service receives actual tokens and matches to estimate
DATA CALCULATED:
{
  "phase": "validation",
  "timestamp": "2026-04-25T16:00:03Z",
  "validation": {
    "validation_id": 42,
    "estimated_total": 457,
    "actual_total": 454,
    "input_token_error": 0,  // (7 - 7) / 7 = 0%
    "output_token_error": -0.67,  // (447 - 450) / 450 = -0.67%
    "total_token_error": -0.66,  // (454 - 457) / 457 = -0.66%
    "cost_error": -0.66,
    "accuracy_score": 99.34,  // 100 - abs(error)
    "validation_status": "validated"
  }
}
```

#### 3.3: User Satisfaction Signal (Early - 2 seconds after response)
```
WHEN: User immediately reacts while reading response
TIMING: T = 2.5 seconds (before token data available)
DATA CAPTURED:
{
  "phase": "early_signal",
  "timestamp": "2026-04-25T16:00:02.5Z",
  "signal": {
    "text": "Perfect! That's exactly what I needed",
    "detected_type": "success",
    "confidence": 0.95,
    "patterns_matched": ["perfect", "exactly"],
    "timing_seconds": 2.5
  }
}
```

#### 3.4: Final Decision (Late - 3 seconds after response)
```
WHEN: Decision engine has both signal + token data
TIMING: T = 3 seconds
DATA CALCULATED:
{
  "phase": "decision",
  "timestamp": "2026-04-25T16:00:03Z",
  "decision": {
    "validation_id": 42,
    "signal_type": "success",
    "signal_confidence": 0.95,
    "token_accuracy": 99.34,
    "combined_confidence": 0.97,
    
    "decision": "stay",
    "reason": "User satisfied, model choice was excellent",
    "next_model": "haiku",
    "next_effort": "low",
    
    "cascade_available": false,
    "explanation": "Already on cheapest model (haiku)"
  }
}
```

#### 3.5: Learning Record
```
WHEN: All data available, stored for future learning
DATA STORED:
{
  "validation_id": 42,
  "effort_level": "low",
  "model_used": "haiku",
  "user_satisfaction": "high",
  "token_accuracy": 99.34,
  "model_correctness": true,
  
  // For pattern learning
  "is_success": true,
  "should_cascade": false,
  "cascade_would_fail": false,
  
  // For task-type learning
  "task_keywords": ["simple", "overview", "api"],
  "task_type": "definition_request",
  "optimal_model": "haiku"
}
```

---

## Part 2: Data Correlation & Analytics

### 2.1: Individual Record Analytics

**Per Validation Record** (42):
```
Metrics Calculated:
- Token Accuracy: 99.34% (within ±15% target ✅)
- Cost Accuracy: 99.34% (within ±10% target ✅)
- Model Choice: CORRECT (haiku was right for low effort)
- User Satisfaction: HIGH (from signal)
- Quality Assessment: EXCELLENT (accuracy + satisfaction)

Insights:
- Effort detection was accurate (low effort → haiku)
- Token estimation was accurate (-0.66% error)
- User happy with response quality
- No escalation needed
- Cascade not available (already cheapest)
```

### 2.2: Effort Level Analytics

**Pattern: LOW Effort Tasks** (e.g., "What is X?")

After 25 low-effort records:
```
Statistics:
- Count: 25 records
- Best Model: Haiku (23/25 = 92% success)
- Average Token Error: -1.2% (under-estimate slightly)
- User Satisfaction: 94% (satisfied/happy)
- Cost per task: $0.0045 average
- Cascade Success: 100% (when offered)

Insights:
- Haiku is optimal for low-effort (94% success)
- Token estimation slightly conservative (good for safety)
- Users very satisfied with haiku responses
- Cascade to haiku from sonnet always works

Recommendation:
- Auto-route low-effort to haiku immediately
- Don't bother with sonnet (waste 8x cost)
```

**Pattern: MEDIUM Effort Tasks** (e.g., "How do I...?")

After 18 medium-effort records:
```
Statistics:
- Count: 18 records
- Best Model: Sonnet (14/18 = 78% success, 4/18 haiku failures)
- Average Token Error: +1.8% (slight over-estimate)
- User Satisfaction: 87% (but failures hurt this)
- Cost per task: $0.038 average
- Cascade Success: 65% (haiku sometimes not enough)

Insights:
- Sonnet is needed for medium-effort (haiku fails 22%)
- Token estimation slightly aggressive (not bad)
- Cascade to haiku works 65% of time (risky)
- Better to stay on sonnet (avoid re-escalation)

Recommendation:
- Route medium-effort to sonnet first
- Don't cascade to haiku (high re-escalation risk)
- Keep on sonnet until user says "perfect"
```

**Pattern: HIGH Effort Tasks** (e.g., "Design a system...")

After 12 high-effort records:
```
Statistics:
- Count: 12 records
- Best Model: Opus (11/12 = 92% success, 1/12 sonnet failure)
- Average Token Error: +4.2% (over-estimate more)
- User Satisfaction: 91% (high complexity needs opus)
- Cost per task: $0.531 average
- Cascade Success: 0% (never cascade from opus)

Insights:
- Opus is essential for high-effort (92% success)
- Token estimation conservative but reasonable
- User very satisfied with opus quality
- Never cascade from opus (always needed)

Recommendation:
- Route high-effort to opus directly
- No cascading from opus
- Investment in opus pays for itself in quality
```

---

## Part 3: Real-Time Feedback Loop (Complete Cycle)

### Timeline: Single User Interaction

```
T=-1sec:  User thinks about question
          Status: No data yet

T=0sec:   User types: "What is REST API?"
          Hook captures: prompt text, length, timestamp
          ↓ POST /api/hook

T=0.1sec: Service receives prompt
          Analysis: "low effort" (confidence: 0.92)
          Estimation: 457 tokens estimated
          Routing: haiku selected
          DB: Create validation #42 (status: estimate_only)
          ↓ Return: {model: haiku, validationId: 42}

T=0.2sec: Claude Code receives routing decision
          Model selection: haiku
          Ready for processing

T=0.5sec: Claude starts generating response
          (Token streaming available but not captured yet)

T=2sec:   Claude finishes response
          Response complete (454 tokens used)
          Statusline updates with actual tokens
          User reads response

T=2.5sec: User immediately says: "Perfect!"
          ↓ Hook detects signal
          Service detects: success signal (confidence: 0.95)
          EARLY REACTION (T=2.5sec):
            "✅ You're happy! Great result."
            (Don't wait for token validation)

T=2.7sec: Token capture background process
          Queries statusline: 454 total tokens
          ↓ POST /api/validate {actual_total_tokens: 454}

T=2.8sec: Service validates
          Compare: estimate (457) vs actual (454)
          Error: -0.66% (excellent! within ±15%)
          Update DB: validation #42 {actual_tokens: 454, error: -0.66}
          
          Make decision with signal + tokens:
          Decision: "stay" (already on cheapest)
          Confidence: 0.97 (0.95 signal × accuracy)
          
          ↓ Store learning record

T=3sec:   Dashboard updates
          Shows: Est 457 vs Act 454 (-0.66%) ✅
          User satisfaction: High ✅
          Model accuracy: Excellent ✅
          
          Analytics instantly updated:
          - Low-effort: Now 26/25 (one more success)
          - Haiku: 92% → 92% (unchanged, but confidence ++)
          
T=3.5sec: Next prompt comes in
          System applies learned pattern:
          "Low effort detected → use haiku"
          (Based on previous 26 validations)

TOTAL FEEDBACK LOOP: 3 seconds end-to-end
REACTION TIME: 2.5 seconds (before token data)
VALIDATION TIME: 0.3 seconds after tokens available
LEARNING APPLIED: <1 second to next task
```

---

## Part 4: Complete Data Flow Diagram

```
╔════════════════════════════════════════════════════════════════╗
║                   COMPLETE FEEDBACK LOOP                       ║
╚════════════════════════════════════════════════════════════════╝

USER INPUT (Prompt)
  ↓ [Hook: UserPromptSubmit]
  
PHASE 1: PRE-RESPONSE (T=0-0.2sec)
┌──────────────────────────────────────┐
│ Prompt Analysis Service              │
├──────────────────────────────────────┤
│ • Parse prompt                       │
│ • Detect effort (low/medium/high)   │
│ • Estimate input tokens             │
│ • Estimate output tokens            │
│ • Select model (haiku/sonnet/opus) │
│ • Calculate cost estimate           │
└──────────────────────────────────────┘
  ↓ [DB Write]
  
VALIDATION METRIC (estimate_only)
┌──────────────────────────────────────┐
│ Database Record #42                  │
├──────────────────────────────────────┤
│ • ID, Timestamp                      │
│ • Prompt + analysis                  │
│ • Estimated tokens + cost            │
│ • Selected model                     │
│ • Status: estimate_only              │
└──────────────────────────────────────┘
  ↓ [Return routing]
  
CLAUDE PROCESSING (T=0.2-2sec)
┌──────────────────────────────────────┐
│ Claude Code + Selected Model         │
├──────────────────────────────────────┤
│ • Generate response                  │
│ • Actual tokens: input + output      │
│ • Response text                      │
│ • Generation time                    │
└──────────────────────────────────────┘
  ↓ [Parallel: Signal + Tokens]
  
PHASE 2A: EARLY SIGNAL (T=2-2.5sec)
┌──────────────────────────────────────┐
│ User Reaction Analysis               │
├──────────────────────────────────────┤
│ • Detect user text                   │
│ • Match against 40+ patterns         │
│ • Calculate confidence               │
│ • Signal type (success/failure/etc)  │
│ • IMMEDIATE FEEDBACK                 │
└──────────────────────────────────────┘
  ↓
  
PHASE 2B: TOKEN CAPTURE (T=2-3sec)
┌──────────────────────────────────────┐
│ Actual Token Extraction              │
├──────────────────────────────────────┤
│ • Query: .context_window.current_    │
│   usage (from barista statusline)   │
│ • Actual input tokens                │
│ • Actual output tokens               │
│ • Total actual tokens                │
│ • Cache tokens (if any)              │
└──────────────────────────────────────┘
  ↓ [POST /api/validate]
  
PHASE 3: VALIDATION & DECISION (T=3sec)
┌──────────────────────────────────────┐
│ Service Decision Engine              │
├──────────────────────────────────────┤
│ 1. Compare estimates vs actual       │
│ 2. Calculate error %                 │
│ 3. Combine with signal confidence    │
│ 4. Apply 5-level priority rules      │
│ 5. Make decision (cascade/escalate/  │
│    stay/adjust_effort)               │
│ 6. Calculate confidence of decision  │
└──────────────────────────────────────┘
  ↓ [DB Update]
  
VALIDATION METRIC (validated)
┌──────────────────────────────────────┐
│ Database Record #42 UPDATED          │
├──────────────────────────────────────┤
│ • Add: actual tokens                 │
│ • Add: token error %                 │
│ • Add: signal data                   │
│ • Add: decision made                 │
│ • Status: validated                  │
│ • Store: learning record             │
└──────────────────────────────────────┘
  ↓
  
LEARNING & ANALYTICS (T=3+sec)
┌──────────────────────────────────────┐
│ Pattern Learning                     │
├──────────────────────────────────────┤
│ Group by effort level:               │
│ • Low: 26 samples, 92% haiku success │
│ • Medium: 18 samples, 78% sonnet     │
│ • High: 12 samples, 92% opus success │
│                                      │
│ Group by model:                      │
│ • Haiku: 92% accuracy, 3.2% cheaper  │
│ • Sonnet: 87% accuracy, 8x base      │
│ • Opus: 91% accuracy, 30x base       │
│                                      │
│ Group by satisfaction:               │
│ • High: 94% low-effort, 65% medium   │
│ • Medium: 22% medium with haiku      │
│ • Low: Only opus failures (rare)     │
└──────────────────────────────────────┘
  ↓
  
NEXT INTERACTION (T=3.5sec)
┌──────────────────────────────────────┐
│ Auto-Routing from Pattern            │
├──────────────────────────────────────┤
│ New prompt: "What is caching?"       │
│ Analysis: low effort                 │
│ Pattern match: → haiku (92% success) │
│ Route: haiku (learned from 26 records│
│                                      │
│ Alternative without learning:        │
│ Would route to sonnet (8x cost)      │
└──────────────────────────────────────┘
  ↓
  ↓ (Loop continues)
```

---

## Part 5: Data Requirements & Flow

### What Data Must Be Captured

**REQUIRED (for closed loop)**:
- ✅ User prompt (input)
- ✅ Effort detection confidence
- ✅ Model selected
- ✅ Estimated tokens (input + output)
- ✅ Actual tokens (input + output)
- ✅ User signal (satisfaction)
- ✅ Response generation time
- ✅ Validation ID (to match pre ↔ post)

**OPTIONAL (for richer analytics)**:
- ⊙ Response text (for quality analysis)
- ⊙ Cache tokens (cache hit rate)
- ⊙ Stop reason (natural end vs limit)
- ⊙ User corrections (follow-up edits)
- ⊙ Task category (inferred from keywords)
- ⊙ Session context (previous satisfaction, model streak)

### How to Get This Data

| Data | Source | Timing | Method |
|------|--------|--------|--------|
| **Prompt** | User input | T=0 | Hook stdin |
| **Effort** | Service analysis | T=0.1 | Hook /api/hook |
| **Model** | Service routing | T=0.1 | Hook response |
| **Est Tokens** | Service estimation | T=0.1 | Hook response |
| **Act Tokens** | Barista statusline | T=2-3 | Monitor/subprocess |
| **Signal** | User text | T=2-2.5 | Detector.DetectSignal |
| **Gen Time** | Barista metrics | T=2-3 | Monitor/subprocess |

### Data Completeness Check

```
✅ CLOSED LOOP possible? 
   - Pre-response (estimate): ✅ Available at T=0.1
   - Post-response (actual): ✅ Available at T=3
   - Validation ID: ✅ Correlates pre ↔ post
   - Signal: ✅ Available at T=2.5
   
✅ LEARNING possible?
   - Group by effort: ✅ Have detected_effort
   - Group by model: ✅ Have routed_model
   - Group by satisfaction: ✅ Have signal_type
   - Calculate accuracy: ✅ Have est vs actual tokens
   
✅ REACTIONS possible?
   - Early (signal only): ✅ At T=2.5
   - Late (signal + tokens): ✅ At T=3
   - Next-interaction (learned): ✅ Use patterns
```

---

## Part 6: Analytics Dashboard Queries

### Real-Time Query Examples

```sql
-- Q1: Overall accuracy this session
SELECT 
  COUNT(*) as total,
  AVG(ABS(token_error)) as avg_error,
  AVG(accuracy_score) as avg_accuracy
FROM validation_metrics 
WHERE validated = true
```

**Result**:
```
total: 42
avg_error: 2.1%
avg_accuracy: 97.9%
```

---

```sql
-- Q2: Model performance by effort level
SELECT 
  detected_effort,
  routed_model,
  COUNT(*) as count,
  AVG(accuracy_score) as accuracy,
  COUNT(CASE WHEN accuracy_score > 95 THEN 1 END) as excellent_count
FROM validation_metrics
GROUP BY detected_effort, routed_model
```

**Result**:
```
low,    haiku:    23, 98.2% accuracy, 21 excellent
low,    sonnet:   2,  96.1% accuracy, 1 excellent
medium, sonnet:   14, 94.3% accuracy, 11 excellent
medium, haiku:    4,  87.2% accuracy, 1 excellent (failures!)
high,   opus:     11, 94.7% accuracy, 10 excellent
high,   sonnet:   1,  89.1% accuracy, 0 excellent (failed)
```

---

```sql
-- Q3: Signal accuracy (do user signals predict accuracy?)
SELECT 
  signal_type,
  COUNT(*) as count,
  AVG(accuracy_score) as avg_accuracy,
  COUNT(CASE WHEN accuracy_score > 95 THEN 1 END) as excellent_percent
FROM validation_metrics
WHERE signal_type IS NOT NULL
GROUP BY signal_type
```

**Result**:
```
success:  22, 98.7% accuracy, 21 excellent (95%)
failure:  3,  78.2% accuracy, 0 excellent (cascades failed)
none:     17, 96.4% accuracy, 15 excellent
```

---

```sql
-- Q4: Cascade effectiveness (when we downgrade, does it work?)
SELECT 
  cascaded_from,
  cascaded_to,
  COUNT(*) as count,
  COUNT(CASE WHEN accuracy_score > 95 THEN 1 END) as success_count,
  (success_count * 100.0 / COUNT(*)) as success_percent
FROM validation_metrics
WHERE cascaded = true
GROUP BY cascaded_from, cascaded_to
```

**Result**:
```
opus,   sonnet: 12, 11, 91% success
sonnet, haiku:  18, 17, 94% success
```

---

## Part 7: Complete Example: One Day of Data

### 42 Validations Over 1 Day

**Input**: 42 user interactions with escalation system

**Output: Analytics Dashboard**

```
╔═══════════════════════════════════════════════════════════╗
║              CLAUDE ESCALATE - DAY 1 ANALYSIS             ║
╚═══════════════════════════════════════════════════════════╝

OVERALL METRICS
├─ Total Interactions: 42
├─ Validated Records: 39
├─ Cascades Offered: 18
├─ Escalations Used: 4
├─ Re-escalations: 0 (100% cascade success)
└─ User Satisfaction: 94%

ACCURACY & COSTS
├─ Token Estimate Accuracy: 97.9% (avg error: 2.1%)
├─ Cost Estimate Accuracy: 97.8% (avg error: 2.2%)
├─ Cost Savings vs All-Opus: 73.4% ($0.387 saved of $1.45)
└─ Tokens Saved: 847 tokens

EFFORT-BASED ROUTING
│
├─ LOW EFFORT (23 records)
│  ├─ Best Model: Haiku (92% success, 21/23)
│  ├─ Cost per: $0.0046 avg
│  ├─ User Satisfaction: 96%
│  ├─ Tokens Saved vs Sonnet: 320 tokens (60% reduction)
│  └─ Recommendation: AUTO-HAIKU for all low-effort ✅
│
├─ MEDIUM EFFORT (14 records)
│  ├─ Best Model: Sonnet (78% success, 11/14)
│  ├─ Haiku Failures: 3/4 (75% fail rate) ⚠️
│  ├─ Cost per: $0.038 avg
│  ├─ User Satisfaction: 87%
│  ├─ Cascade to Haiku: 65% success (risky)
│  └─ Recommendation: STAY-SONNET (avoid cascade) ✅
│
└─ HIGH EFFORT (5 records)
   ├─ Best Model: Opus (92% success, 4/5)
   ├─ Sonnet Failures: 1/5 (20% fail rate)
   ├─ Cost per: $0.531 avg
   ├─ User Satisfaction: 91%
   ├─ Cascade from Opus: Never attempted (correct)
   └─ Recommendation: AUTO-OPUS for all high-effort ✅

LEARNING PATTERNS
├─ Pattern 1: Low-effort keywords (simple, quick, easy)
│  └─ Always route to Haiku (100% confidence after 23 samples)
│
├─ Pattern 2: Definition requests (What is X?)
│  └─ Always low-effort (detection accuracy 100%)
│
├─ Pattern 3: User says "Perfect!"
│  └─ Accuracy 98.7%, Cascade safe (94% success)
│
├─ Pattern 4: Architecture questions
│  └─ Always high-effort, always need Opus
│
└─ Pattern 5: "Try again" means current model insufficient
   └─ Escalate immediately (100% of attempts)

SIGNAL EFFECTIVENESS
├─ Success Signals (22): 98.7% avg accuracy ✅
├─ Failure Signals (3): 78.2% avg accuracy (re-escalations)
├─ No Signal (17): 96.4% avg accuracy (good defaults)
└─ Early Reaction Time: 2.4 seconds average

CASCADING PERFORMANCE
├─ Total Cascades: 18
├─ Successful: 17 (94% success rate)
├─ Failed (re-escalations): 1 (sonnet cascade failed)
├─ Cost Savings from Cascade: $0.087 (8.7 cents saved)
└─ User Satisfaction after Cascade: 95%

COST ANALYSIS
├─ Estimated Cost (if all-Opus): $1.45
├─ Actual Cost (with routing): $0.387
├─ Savings: $1.063 (73.4% reduction) 💰
├─ Per Interaction: $0.00923 avg (vs $0.0345 for Opus)
└─ ROI: For every $1 system cost, save $3.46 ✅

NEXT DAY RECOMMENDATIONS
├─ Apply Learning #1: Auto-route low-effort to haiku
├─ Apply Learning #2: Keep medium-effort on sonnet (don't cascade)
├─ Apply Learning #3: Trust success signals (98.7% accurate)
├─ Apply Learning #4: Trigger on /escalate immediately
└─ Collect 58 more records for statistical significance (100 total)
```

---

## Part 8: Feedback Loop Closed

**Complete cycle achieved**:

```
INPUT (Prompt) 
  → ANALYSIS (Effort + Model)
  → ESTIMATE (Tokens + Cost)
  → ROUTE (Claude with model)
  → RESPONSE (Actually generation)
  → CAPTURE (Actual tokens)
  → VALIDATE (Compare est vs actual)
  → SIGNAL (User satisfaction)
  → DECIDE (Cascade/escalate/stay)
  → LEARN (Pattern extraction)
  → REACT (Apply to next task)
  → INPUT (Loop continues)
```

**Success Indicators**:
- ✅ All pre/post data captured
- ✅ Correlation ID (validation_id) connects pre ↔ post
- ✅ Analytics possible at granular level
- ✅ Learning extracted after 10+ samples per pattern
- ✅ Reactions deployed after 20 samples
- ✅ Cost savings calculated and verified
- ✅ User satisfaction measurable
- ✅ System improves over time

**Time to Insights**:
- Early signals: 2.5 seconds (before tokens)
- Full validation: 3 seconds (with tokens)
- Pattern recognition: 20+ samples (1-2 hours of use)
- Optimization learned: 50+ samples (3-4 hours of use)
- Statistical significance: 100+ samples (1 day of use)
