# Sentiment Detection & Anti-Frustration System

Claude Escalate detects user sentiment (frustrated, confused, impatient, cautious, satisfied) and uses it to make intelligent routing decisions. The goal: **minimize frustration while protecting your token budget**.

---

## Sentiment Types

The system recognizes six sentiment signals:

| Sentiment | Signal | Response |
|-----------|--------|----------|
| **Frustrated** | "still broken", "not working", repeated escalations | Escalate to higher model (Haiku → Sonnet → Opus) |
| **Confused** | "why", "don't understand", follow-up questions | Escalate to Sonnet for better explanations |
| **Impatient** | "hurry", "ASAP", rapid follow-ups | Switch to Haiku (faster) or show warning |
| **Cautious** | "be careful", "slowly", "don't break" | Stay on current model or de-escalate carefully |
| **Satisfied** | "perfect", "thanks", "works", "exactly" | Cascade to cheaper model (Opus → Sonnet → Haiku) |
| **Neutral** | No emotional signal | Use budget/task-type routing decision |

---

## Detection Methods

### 1. Explicit Signals (Text Patterns)

The system scans your prompts for keywords and phrases:

**Frustration Keywords**:
```
"still broken", "not working", "doesn't work", "still failing",
"can't get it to work", "tried again", "again", "retry",
"keep getting error", "same issue", "broken again"
```

**Success Keywords**:
```
"perfect", "works", "thanks", "thank you", "exactly", "got it",
"that's it", "solved", "fixed", "appreciate"
```

**Confusion Keywords**:
```
"why", "confused", "don't understand", "explain", "how do I",
"what do I", "how come", "clarify", "more details"
```

**Impatience Keywords**:
```
"hurry", "fast", "ASAP", "urgent", "now", "quick"
```

**Caution Keywords**:
```
"careful", "slowly", "don't break", "safe", "gentle", "gentle approach"
```

### 2. Implicit Signals (Interaction Patterns)

Beyond keywords, the system detects behavior patterns:

**Rapid Escalations**:
- Multiple `/escalate` commands in < 5 minutes → likely frustrated
- Confidence increases with each additional escalation

**Repeated Attempts**:
- Same type of request within < 2 minutes → might be confused or frustrated
- Previous request apparently didn't solve the problem

**Editing Behavior**:
- Frequent prompt edits before submitting → uncertainty or confusion
- Multiple quick follow-up questions → confusion signal

**Timing Patterns**:
- Very short response times → might indicate impatience ("why is this taking so long?")
- Long pauses before responses → might indicate confusion or thinking

### 3. Response Quality Feedback

The system learns from outcomes:

**Success Indicators**:
- User immediately copies/uses the response
- User marks solution as helpful
- No follow-up questions within 10 minutes

**Failure Indicators**:
- User immediately escalates or re-prompts
- User deletes/ignores the response
- User asks "why didn't this work?"

---

## Frustration Risk Score

The system calculates a **frustration risk score** (0.0 - 1.0) as a separate dimension from the primary sentiment:

**How it's calculated**:
- Each signal contributes points (0.0 to 1.0)
- Frustration keywords: +0.3 to 0.5
- Confused patterns: +0.2 to 0.3
- Rapid escalations: +0.4 to 0.5 per escalation (max 1.0)
- Combined score: 0.0 (perfectly content) to 1.0 (maximally frustrated)

**Example**:
```
User prompt: "still broken, tried again"
- Contains "still broken": +0.4 frustration
- Contains "tried again": +0.2 frustration
- Total frustration risk: 0.6

Threshold for auto-escalation: 0.70 (configurable)
Status: Not escalating yet (0.6 < 0.70)

[Next prompt: user escalates]
- New frustration score: 0.85 (multiple failures)
- Status: AUTO-ESCALATE (0.85 > 0.70)
```

---

## Automatic Escalation Strategy

When frustration risk > 0.70 (configurable threshold):

### First Failure: One Model Up
```
Current model: Haiku
Frustration: 0.75
Action: Haiku → Sonnet (4-5x more capable)
Rationale: Haiku likely insufficient, Sonnet should handle it
```

### Multiple Failures: Go to Opus
```
Attempt 1: Haiku → failed (frustration: 0.75)
Attempt 2: Sonnet → failed (frustration: 0.85)
Action: Sonnet → Opus (deep reasoning)
Rationale: User stuck on hard problem, need best model
```

### Already on Opus: Manual Help Needed
```
Attempt 1, 2, 3: All escalated to Opus, still failing
Frustration: 0.95
Action: Suggest manual debugging / consultation
Reason: Best model couldn't solve it, might need human help
```

### Special Cases

**Confused Users**:
```
Sentiment: confused (why, explain, don't understand)
Current: Haiku
Action: Escalate to Sonnet
Reason: Sonnet better at clear explanations
```

**Impatient Users**:
```
Sentiment: impatient (hurry, ASAP, rapid follow-ups)
Current: Opus
Action: Suggest Haiku (4x faster)
Warning: "Note: Haiku is faster but less capable"
Reason: Balance speed with capability
```

---

## De-Escalation (Automatic Cost Reduction)

When a problem is **solved** and user sentiment turns **satisfied**:

```
Opus response
↓
User says: "Perfect! That works."
↓
System detects success signal
↓
Auto-downgrade: Opus → Sonnet
↓
Next task uses Sonnet by default (cheaper, still capable)
↓
If Sonnet succeeds too: cascade to Haiku
```

**Cost Impact Example**:
```
Problem: Race condition (complex)
Escalation: Haiku → Sonnet (cost: 5x)
Solution found: User says "thanks, that's it"
De-escalation: Sonnet → Haiku (saves 4x on next task)

Net: Paid 5x for one complex task, then 1x for next simple task
Result: Frustration minimized, budget balanced
```

---

## Learning from Sentiment

Every interaction teaches the system which models work best for different scenarios:

```
Record each outcome:
{
  "task_type": "concurrency",
  "model_used": "sonnet",
  "user_sentiment_initial": "neutral",
  "user_sentiment_final": "satisfied",
  "success": true,
  "frustration_detected": false
}

Aggregate over time:
- Concurrency on Haiku: 45% satisfaction (users frustrated)
- Concurrency on Sonnet: 78% satisfaction
- Concurrency on Opus: 98% satisfaction

Learn: For concurrency, prefer Sonnet or Opus
       (Haiku often fails, causing frustration)
```

Next time you ask a concurrency question:
1. Phase 1 suggests Sonnet (based on learning)
2. You're less likely to get frustrated (better success rate)
3. Still cheaper than Opus, but better than Haiku

---

## Configuration

### Enable/Disable Sentiment Detection
```bash
# Enable (default: true)
escalation-manager config set sentiment.enabled true

# Disable
escalation-manager config set sentiment.enabled false
```

### Adjust Frustration Threshold
```bash
# Default: 0.70 (escalate when 70% frustrated)
escalation-manager config set sentiment.frustration_risk_threshold 0.70

# More sensitive (escalate earlier)
escalation-manager config set sentiment.frustration_risk_threshold 0.60

# Less sensitive (only escalate when very frustrated)
escalation-manager config set sentiment.frustration_risk_threshold 0.80
```

### Enable/Disable Auto-Escalation
```bash
# Enable automatic escalation on frustration
escalation-manager config set sentiment.frustration_trigger_escalate true

# Disable (alerts only)
escalation-manager config set sentiment.frustration_trigger_escalate false
```

### Enable Learning
```bash
# Learn patterns (default: true)
escalation-manager config set sentiment.learning_enabled true

# Disable learning (static model assignments)
escalation-manager config set sentiment.learning_enabled false
```

---

## Viewing Sentiment Data

### Web Dashboard
```
Open: http://localhost:9000
Go to: Sentiment Tab
Shows:
  - Satisfaction rate (%)
  - Breakdown: satisfied, neutral, confused, frustrated, impatient
  - Recent frustration events
  - Model satisfaction by task type
```

### CLI Dashboard
```bash
escalation-manager dashboard --sentiment
```

Output:
```
=== Sentiment Dashboard ===
Satisfaction Rate: 87.3% (62/71)
Recent Frustration Events: 2
  [2h ago] Concurrency on Haiku → Escalated to Sonnet → Resolved ✅
  [30m ago] Parsing on Haiku → Success, no escalation needed ✅

Model Satisfaction (by task type):
  concurrency: Opus=98%, Sonnet=78%, Haiku=45%
  parsing: Opus=96%, Sonnet=89%, Haiku=72%
  debugging: Opus=95%, Sonnet=92%, Haiku=88%
```

---

## Examples

### Example 1: Frustration Detection & Auto-Escalation
```
You: "Debug this race condition in my Go code"
System: Phase 1 → Haiku (simple question estimate)

[Haiku generates response]
You: "This doesn't actually solve the problem"

System detects:
- Keyword: "doesn't solve"
- Frustration risk: 0.72
- Threshold: 0.70
- Action: AUTO-ESCALATE to Sonnet

[Sonnet generates response]
You: "Perfect! That's the issue. Thanks."

System detects success:
- Auto-cascade: Sonnet → Haiku
- Learn: concurrency problems need Sonnet
```

### Example 2: Confused User Gets Better Explanation
```
You: "What's the difference between channels and mutexes?"
System: Phase 1 → Haiku (educational question)

[Haiku explains]
You: "I'm still confused, can you explain with an example?"

System detects:
- Keyword: "confused"
- Sentiment: confused
- Action: Escalate to Sonnet (better at explaining)

[Sonnet explains with detailed example]
You: "Ah, got it! That makes sense now."

System learns: "explain" questions work better on Sonnet
```

### Example 3: Impatient User Gets Fast Response
```
You: "I need to fix this bug ASAP before standup"
System detects:
- Keywords: "ASAP"
- Sentiment: impatient
- Suggestion: Use Haiku for speed

You confirm: yes, go with Haiku
[Haiku responds in 0.3s]
You: "Good enough, moving on"

Budget saved: Haiku costs 1/15th of Opus
```

---

## See Also

- [3-Phase Flow](3-phase-flow.md) — How sentiment is captured across phases
- [Token Validation](token-validation.md) — Learning from outcomes
- [Dashboards](../analytics/dashboards.md) — Viewing sentiment analytics
