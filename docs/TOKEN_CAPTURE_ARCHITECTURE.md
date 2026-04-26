# Token Capture & Early Reaction Architecture

## Part 1: Token Data Sources & Capture Mechanisms

### Source 1: Barista Statusline Data
**Where**: Claude Code → Barista every 15 seconds  
**Data Available**: `.context_window.current_usage` JSON  
**Fields**:
```json
{
  "input_tokens": 1234,
  "cache_creation_input_tokens": 0,
  "cache_read_input_tokens": 0,
  "output_tokens": 567
}
```

**Total Actual Tokens** = input_tokens + cache_creation_input_tokens + cache_read_input_tokens + output_tokens

**How to Capture** (Binary-Only, No Scripts):
```go
// In escalation-manager binary:
// 1. Accept POST /api/validate with actual tokens from any source
// 2. Alternative: Read from environment variable set by Claude Code
// 3. Alternative: Query barista data directly via subprocess
```

### Source 2: Claude Code Response Metadata
**Timing**: Available AFTER response generation, BEFORE user sees output  
**Access Method**: 
- PostToolUse hook (doesn't capture response tokens)
- Statusline refresh interval (async, every 15s)
- Environment variables passed by Claude Code

**Capture Strategy**:
- Hook into statusline refresh cycle
- Extract `.context_window.current_usage`
- POST immediately to `/api/validate`
- No shell script needed — binary subprocess call

### Source 3: Manual Entry (For Testing/Verification)
**CLI Command**:
```bash
escalation-manager validate --tokens 567 --input 1234 --output 567
```

**Direct HTTP**:
```bash
curl -X POST http://localhost:9000/api/validate \
  -d '{"actual_total_tokens":567,"actual_input_tokens":1234,"actual_output_tokens":567}'
```

---

## Part 2: Early Reaction Signals & Detection

### User Signals = Cost/Quality Decision Points

**SUCCESS SIGNALS** (User is happy → can cascade down):
```
"Perfect!"
"Works great!"
"Thank you"
"That fixed it"
"Excellent"
"Exactly what I needed"
"Solved"
"Appreciate it"
```
**Action**: De-escalate to cheaper model (Opus→Sonnet, Sonnet→Haiku)

**FAILURE SIGNALS** (User is unhappy → need better model):
```
"Didn't work"
"Still broken"
"That's wrong"
"Try again"
"Not correct"
"Incomplete"
"Missing something"
"Going in circles"
```
**Action**: Escalate to better model (Haiku→Sonnet, Sonnet→Opus)

**CLARIFICATION SIGNALS** (User needs model to understand context):
```
"Can you explain..."
"Why did you..."
"How does..."
"What about..."
```
**Action**: Stay at current model, add context

**EFFORT SIGNALS** (Task is harder/easier than expected):
```
"This is complex"          → Upgrade effort estimate
"Actually simple"          → Downgrade effort estimate
"Multiple steps needed"    → Increase estimated tokens
"Just a quick fix"        → Decrease estimated tokens
```
**Action**: Adjust effort level and model routing

---

## Part 3: Event-Based Optimization Pipeline

### Event Stream Architecture

```
USER INPUT
    ↓
HOOK: Analyze prompt
    ├─ Extract signals: "perfect", "broken", "escalate"
    ├─ Detect effort: low/medium/high
    ├─ Route model: haiku/sonnet/opus
    └─ Create validation record (estimate phase)
    ↓
CLAUDE RESPONSE
    ├─ Generate response
    └─ Produce tokens: input_tokens + output_tokens
    ↓
SIGNAL DETECTION: Analyze response for user satisfaction
    ├─ Pattern: "perfect!" → success signal
    ├─ Pattern: "didn't work" → failure signal
    ├─ Pattern: "escalate" → manual escalation
    └─ Estimate signal confidence (0-100%)
    ↓
TOKEN CAPTURE: Extract actual tokens from Claude
    ├─ Source: Statusline refresh
    ├─ POST /api/validate
    └─ Update validation record (actual phase)
    ↓
DECISION MAKING
    ├─ Compare: estimate vs actual
    ├─ Calculate: error %, accuracy
    ├─ Evaluate: did routing decision work?
    ├─ Check: is cascade appropriate?
    └─ Decide: next model + effort level
    ↓
OPTIMIZATION ACTION
    ├─ IF success signal → cascade down (save cost)
    ├─ IF failure signal → escalate up (improve quality)
    ├─ IF tokens < estimate → model was right
    ├─ IF tokens > estimate → need better model
    └─ Store in database for learning
```

### Real-Time Decision Rules

```
RULE 1: Early Success Detection
  IF user says "Perfect!" before validation complete
    AND current_model == "opus"
    THEN immediately offer de-escalation
    (User satisfaction doesn't require full token data)

RULE 2: Token-Based Accuracy
  IF actual_tokens vs estimated_tokens
    AND error % within ±15% target
    THEN routing decision was correct (store as success)

RULE 3: Model Right-Sizing
  IF error % > 20%
    AND tokens exceeded significantly
    THEN model was underestimated
    ACTION: Increase base model for this task type

RULE 4: Cost Optimization
  IF success signal + token error < 5%
    AND cascade available
    THEN recommend cascade immediately
    (Proven cost savings)

RULE 5: Circular Reasoning Detection
  IF same tokens used 3+ times
    AND user keeps saying "try again"
    THEN escalate (current model insufficient)
```

---

## Part 4: Data Collection for Optimization

### What To Track Per Event

```
ValidationMetric {
  // Identification
  id: 42
  timestamp: "2026-04-25T16:00:00Z"
  
  // Phase 1: Pre-Response (Estimate)
  prompt: "What is ML?"
  detected_effort: "low"
  routed_model: "haiku"
  estimated_input_tokens: 25
  estimated_output_tokens: 500
  estimated_total_tokens: 525
  estimated_cost: 0.005
  
  // Early Signals (Captured During Response)
  user_signal: "Perfect!"
  signal_type: "success"
  signal_confidence: 0.95
  signal_timing: 2.5 seconds after response start
  
  // Phase 2: Post-Response (Actual)
  actual_input_tokens: 23
  actual_output_tokens: 487
  actual_total_tokens: 510
  actual_cost: 0.0051
  
  // Calculated Metrics
  token_error: -2.9% (estimate was 3% high)
  cost_error: -2.0%
  model_accuracy: true (haiku was right choice)
  cascade_applied: true
  cascade_savings: 3.2%
  
  // Outcomes
  validation_status: "complete"
  user_satisfaction: "high" (from signal)
  quality_acceptable: true (from signal)
  cost_efficient: true (from error %)
  
  // Follow-up Actions
  next_model: "haiku" (cascade down worked)
  next_effort: "low" (confirmed)
  cascaded_from: "sonnet"
  cascaded_to: "haiku"
}
```

### Statistics for Optimization

```
AggregatedStats {
  // Task Accuracy by Effort Level
  low_effort: {
    avg_token_error: -2.1%,
    success_rate: 94%,
    cascade_success: 92%,
    best_model: "haiku"
  },
  medium_effort: {
    avg_token_error: 1.8%,
    success_rate: 87%,
    cascade_success: 65%,
    best_model: "sonnet"
  },
  high_effort: {
    avg_token_error: 4.2%,
    success_rate: 91%,
    cascade_success: 12% (opus rarely cascades)
    best_model: "opus"
  },
  
  // Token Prediction Accuracy
  input_token_accuracy: 96% (estimate vs actual)
  output_token_accuracy: 92%
  total_token_accuracy: 94%
  
  // Cost Savings Realized
  estimated_total_cost: $1.234
  actual_total_cost: $1.192
  cost_saved: $0.042 (3.4%)
  
  // Early Signal Accuracy
  success_signal_accuracy: 0.97 (98% of "perfect!" led to satisfaction)
  failure_signal_accuracy: 0.94 (93% of "didn't work" required re-attempt)
  
  // Cascade Effectiveness
  successful_cascades: 42
  failed_cascades: 3
  cascade_success_rate: 93%
  avg_tokens_saved_per_cascade: 47
}
```

---

## Part 5: Implementation Strategy (Binary-Only, No Scripts)

### Step 1: Signal Detection in Hook
```go
// In escalation-manager hook subcommand
type SignalDetector struct {
  successPatterns   []string  // "perfect", "works", "thanks"
  failurePatterns   []string  // "broken", "wrong", "again"
  escalationPattern string    // "/escalate"
  effortPatterns    map[string][]string
}

func (sd *SignalDetector) AnalyzePrompt(prompt string) Signal {
  // Returns: {type: "success|failure|effort|escalate", confidence: 0.0-1.0}
}
```

### Step 2: Token Capture from Statusline
```go
// In escalation-manager service
// Option A: Subprocess call to extract statusline data
func GetActualTokensFromStatusline() (tokens int, err error) {
  // Call barista.sh, extract .context_window.current_usage
  // Parse output, return actual_total_tokens
}

// Option B: Environment variable from Claude Code
func GetActualTokensFromEnv() (tokens int, err error) {
  // Read CLAUDE_CONTEXT_USAGE env var (if Claude Code sets it)
}

// Option C: Binary API endpoint (manual, for testing)
// POST /api/validate with actual tokens
```

### Step 3: Decision Engine
```go
// In escalation-manager service
type DecisionEngine struct {
  db *Store
}

func (de *DecisionEngine) DecideNextAction(
  validation ValidationMetric,
  signal Signal,
) Action {
  // Inputs:
  // - validation: estimated vs actual tokens, error %
  // - signal: user said "perfect" or "broken"
  
  // Logic:
  // - IF signal=success AND tokens_error<15% → CASCADE
  // - IF signal=failure OR tokens_error>25% → ESCALATE
  // - IF signal=success AND tokens_error>20% → DOWNGRADE MODEL
  
  // Output:
  // - next_model: haiku|sonnet|opus
  // - next_effort: low|medium|high
  // - reason: why this decision
  // - confidence: 0.0-1.0
}
```

### Step 4: Early Reaction API
```go
// New binary subcommand
escalation-manager signal --text "Perfect!" --type success
// OR: escalation-manager signal --prompt-id 42 --satisfaction high

// Returns: {action: "cascade_to_haiku", confidence: 0.95, reason: "..."}
```

---

## Part 6: Event Sequence - Complete Example

```
T0: USER PROMPT
   Input: "What is REST API? Just brief overview"
   Hook analyzes: effort=low, signals: none, model_suggestion: haiku
   
   POST /api/hook
   Response: {continue: true, model: haiku, validation_id: 42}
   
   DB Create: validation_metric #42 {
     prompt: "What is REST API?...",
     estimated_tokens: 450,
     detected_effort: "low",
     routed_model: "haiku"
   }

T0-2sec: CLAUDE PROCESSING
   Claude: Generating response with haiku model
   Tokens used: input 32, output 412, total 444

T2sec: USER SEES RESPONSE
   User reads Claude's answer
   User types: "Perfect! Exactly what I needed"
   
   EARLY SIGNAL DETECTION:
   Hook detects: "Perfect!" + "Exactly"
   Signal: {type: success, confidence: 0.98}
   Timing: 2 seconds after response
   
   ACTION: Suggest cascade before token data available
   "This can cascade to Haiku (3x cheaper) next time"

T3sec: TOKEN CAPTURE
   Statusline refreshes, provides actual tokens
   actual_input: 32
   actual_output: 412
   actual_total: 444
   
   POST /api/validate {actual_total_tokens: 444, validation_id: 42}
   
   DB Update: validation_metric #42 {
     actual_tokens: 444,
     token_error: -1.3% (estimate 450 vs actual 444)
     validated: true
   }

T3.5sec: OPTIMIZATION DECISION
   Engine compares:
   - estimated: 450 tokens
   - actual: 444 tokens
   - error: -1.3% ✅ (within ±15% target)
   - signal: success (user said "perfect")
   - current_model: haiku
   - cascade_available: false (already on cheapest)
   
   Decision:
   - Validation_status: SUCCESS
   - Model_accuracy: CORRECT (haiku was right choice)
   - Tokens_efficient: YES
   - Savings: 0 (already on haiku, can't go cheaper)
   - Next_action: STAY_ON_HAIKU (best choice)
   - Store: Record success for "low_effort" pattern

NEXT PROMPT (T5sec+): 
   Similar low-effort task → Auto-route to haiku again
   (Learning from previous validation)
```

---

## Part 7: Dashboard Metrics

### Real-Time Display
```
┌─ Token Validation ────────────────┐
│ Est:     450 tokens               │
│ Act:     444 tokens               │
│ Error:   -1.3% ✅                 │
│ Accur:   98.7%                    │
├─ Early Signals ───────────────────┤
│ User Signal: "Perfect!"           │
│ Confidence: 98%                   │
│ Type: Success                     │
│ Timing: 2.1s after response       │
├─ Decision ────────────────────────┤
│ Model Choice: ✅ CORRECT (haiku)  │
│ Cascade Offered: No (already low) │
│ Cost Efficiency: EXCELLENT        │
│ Next Model: Haiku                 │
└───────────────────────────────────┘
```

### Learning View
```
Effort Type: LOW (e.g., "What is X?")
  ├─ Best Model: Haiku (94% success)
  ├─ Token Accuracy: 96%
  ├─ Avg Cascade: 100% → 0% (already cheapest)
  ├─ Cost Saved: $0.043 vs all-opus baseline
  └─ Sample Prompts: 23 collected

Effort Type: MEDIUM (e.g., "How do I...?")
  ├─ Best Model: Sonnet (91% success)
  ├─ Token Accuracy: 93%
  ├─ Cascade Rate: 87% → 92% success
  ├─ Cost Saved: $0.142 vs all-opus baseline
  └─ Sample Prompts: 18 collected

Effort Type: HIGH (e.g., "Design a system...")
  ├─ Best Model: Opus (94% success)
  ├─ Token Accuracy: 91%
  ├─ Cascade Rate: Never (stays on opus)
  ├─ Cost: $0.531 (necessary for quality)
  └─ Sample Prompts: 12 collected
```

---

## Summary: Token Capture Without Scripts

1. **Token Source**: `.context_window.current_usage` from Claude Code
2. **Capture Methods**: 
   - Statusline subprocess call (binary only)
   - Environment variable (if Claude Code provides)
   - Manual HTTP POST
3. **Early Signals**: Detect user satisfaction before token data
4. **Decision Engine**: Rules-based optimization (cascade vs escalate)
5. **Learning**: Store all validations for task-type accuracy
6. **Actions**: Automatic model routing based on patterns

**No shell scripts required** — all logic in Go binary.
