# Configuring Sentiment Detection

This guide walks you through enabling and configuring sentiment-aware routing to minimize frustration.

---

## Quick Setup (2 minutes)

```bash
# 1. Enable sentiment detection
escalation-manager config set sentiment.enabled true

# 2. Set frustration threshold (0.0-1.0, default 0.70)
escalation-manager config set sentiment.frustration_risk_threshold 0.70

# 3. Enable auto-escalation on frustration
escalation-manager config set sentiment.frustration_trigger_escalate true

# 4. Enable learning from patterns
escalation-manager config set sentiment.learning_enabled true

# 5. Verify configuration
escalation-manager config
```

---

## Configuration Options

### `sentiment.enabled` (boolean, default: `true`)

Enable or disable sentiment detection entirely.

```bash
# Enable (detect user sentiment from prompts)
escalation-manager config set sentiment.enabled true

# Disable (static routing, no frustration detection)
escalation-manager config set sentiment.enabled false
```

### `sentiment.frustration_risk_threshold` (float 0.0-1.0, default: `0.70`)

The frustration level at which the system auto-escalates to a better model.

```bash
# Sensitive: escalate earlier (at 60% frustration)
escalation-manager config set sentiment.frustration_risk_threshold 0.60

# Default: escalate at 70% frustration
escalation-manager config set sentiment.frustration_risk_threshold 0.70

# Conservative: only escalate when very frustrated (>80%)
escalation-manager config set sentiment.frustration_risk_threshold 0.80
```

**How to choose**:
- **0.50-0.60**: Very proactive, escalate frequently, higher costs but fewer frustrations
- **0.70** (default): Balanced, escalate when clearly frustrated
- **0.80-0.90**: Conservative, only escalate when very frustrated, lower costs but more frustrations

### `sentiment.frustration_trigger_escalate` (boolean, default: `true`)

When frustration is detected, automatically escalate to a higher model.

```bash
# Enable auto-escalation
escalation-manager config set sentiment.frustration_trigger_escalate true

# Disable (detect frustration but just alert, don't escalate)
escalation-manager config set sentiment.frustration_trigger_escalate false
```

### `sentiment.learning_enabled` (boolean, default: `true`)

Store sentiment outcomes to improve future routing decisions.

```bash
# Enable learning (track which models work best by task type + sentiment)
escalation-manager config set sentiment.learning_enabled true

# Disable (static assignments, no learning)
escalation-manager config set sentiment.learning_enabled false
```

### `sentiment.track_satisfaction` (boolean, default: `true`)

Track overall satisfaction rates for analytics and dashboards.

```bash
escalation-manager config set sentiment.track_satisfaction true
```

---

## Complete Configuration Example

Edit `~/.claude/escalation/config.yaml`:

```yaml
# Sentiment Detection Settings
sentiment:
  # Enable/disable sentiment detection
  enabled: true

  # When detected: 0.70 = escalate when 70% frustrated
  frustration_risk_threshold: 0.70

  # Auto-escalate on frustration detection
  frustration_trigger_escalate: true

  # Learn patterns (store sentiment → success correlation)
  learning_enabled: true

  # Track satisfaction rate for dashboards
  track_satisfaction: true

  # Frustration signal timeout (seconds)
  # How long to remember frustration signals
  memory_duration_minutes: 30
```

---

## How It Works in Practice

### Scenario 1: Frustrated User Gets Auto-Escalated

```
You: "Debug this race condition"
System: Phase 1 → Estimates Haiku sufficiency
         ↓
[Haiku responds with explanation]
         ↓
You: "Still broken, still getting the race condition"
         ↓
System detects:
  - "Still broken" (frustration keyword)
  - "Still getting" (repeated failure signal)
  - Frustration risk: 0.75
  ↓
Threshold check: 0.75 > 0.70 ✓ AUTO-ESCALATE
  ↓
Action: Haiku → Sonnet (auto-escalate)
  ↓
[Sonnet responds with detailed solution]
  ↓
You: "Perfect, that fixed it!"
  ↓
System: Record success, learn that "race conditions" → Sonnet/Opus
         De-escalate for next task (if simple)
```

### Scenario 2: Confused User Gets Better Explanation

```
You: "Explain middleware in Go"
System: Phase 1 → Haiku sufficient
         ↓
[Haiku gives basic explanation]
         ↓
You: "I'm confused about the return type, can you explain more?"
         ↓
System detects:
  - "confused" (explicit signal)
  - Follow-up question (implicit signal)
  - Sentiment: confused (not frustrated)
  ↓
Action: Haiku → Sonnet (better explanations)
  ↓
[Sonnet gives detailed walkthrough with examples]
  ↓
You: "Got it, thanks!"
  ↓
System: Record success, learn "explanation" questions → Sonnet
```

### Scenario 3: Learning from Success

```
Multiple interactions:
1. Concurrency question on Haiku → User frustrated (0.75) → Escalated
2. Concurrency question on Sonnet → User satisfied (0.90)
3. Concurrency question on Sonnet → User satisfied (0.95)
4. Concurrency question on Opus → User satisfied (0.98)

System learns:
  Concurrency + Haiku = 20% success (users frustrated)
  Concurrency + Sonnet = 85% success (good balance)
  Concurrency + Opus = 98% success (overkill for budget)

Next concurrency question:
  Phase 1 recommendation: Sonnet (good cost-quality balance)
  Reason: Historical success 85% on Sonnet > 20% on Haiku
```

---

## Monitoring Sentiment

### View Current Sentiment Status

```bash
# Show sentiment dashboard
escalation-manager dashboard --sentiment

# Output:
# Satisfaction Rate: 87.3% (62/71)
# Recent Frustration Events: 2
#   [2h ago] Concurrency on Haiku → Escalated
#   [30m ago] Parsing on Haiku → Resolved
#
# Model Satisfaction (by task type):
#   concurrency: Opus=98%, Sonnet=78%, Haiku=45%
#   parsing: Opus=96%, Sonnet=89%, Haiku=72%
```

### Query Sentiment Trends

```bash
# Get sentiment data from API
curl http://localhost:9000/api/analytics/sentiment-trends?hours=24 | jq .

# Response:
# {
#   "satisfaction_rate": 0.873,
#   "satisfied": 62,
#   "neutral": 7,
#   "frustrated": 2,
#   "confused": 0,
#   "impatient": 0,
#   "recent_frustration_events": [...]
# }
```

---

## Fine-Tuning

### Make Sentiment Detection More Sensitive

If you get frustrated more easily, lower the threshold:

```bash
escalation-manager config set sentiment.frustration_risk_threshold 0.60

# Now system escalates earlier (at 60% frustration instead of 70%)
# Cost: slightly higher token usage, but fewer frustrations
```

### Make Sentiment Detection Less Sensitive

If you want to keep costs down and don't mind frustration occasionally:

```bash
escalation-manager config set sentiment.frustration_risk_threshold 0.85

# Now system only escalates when you're very clearly frustrated
# Cost: lower token usage, potential for more frustrations
```

### Disable Auto-Escalation (Alerts Only)

If you want to be alerted to frustration but decide when to escalate:

```bash
escalation-manager config set sentiment.frustration_trigger_escalate false

# System detects frustration and shows warning, but doesn't auto-escalate
# You use /escalate command manually to switch models
```

### Reset to Defaults

```bash
# Restore default sentiment configuration
escalation-manager config set sentiment.frustration_risk_threshold 0.70
escalation-manager config set sentiment.frustration_trigger_escalate true
escalation-manager config set sentiment.learning_enabled true
```

---

## Troubleshooting

### Sentiment Not Being Detected

**Problem**: You're clearly frustrated but system isn't escalating

**Check**:
1. Sentiment detection is enabled: `escalation-manager config | grep "Sentiment"`
2. You're using frustration keywords the system recognizes
3. Your frustration risk is actually > threshold

**Solution**:
```bash
# Lower threshold temporarily for testing
escalation-manager config set sentiment.frustration_risk_threshold 0.50

# Try again with a clearly frustrated prompt: "Still broken, why isn't this working?"
```

### System Escalating Too Often

**Problem**: Escalating to Opus on every minor issue, wasting budget

**Check**:
1. Current threshold: `escalation-manager config | grep frustration_risk_threshold`
2. Are you using trigger words frequently?

**Solution**:
```bash
# Raise threshold to be less sensitive
escalation-manager config set sentiment.frustration_risk_threshold 0.80

# Or disable auto-escalation entirely
escalation-manager config set sentiment.frustration_trigger_escalate false
```

### Learning Not Improving Over Time

**Problem**: System still recommends wrong models after many interactions

**Check**:
1. Learning is enabled: `escalation-manager config | grep learning_enabled`
2. You have enough data (system needs 5+ similar interactions to learn)

**Solution**:
```bash
# Make sure learning is on
escalation-manager config set sentiment.learning_enabled true

# Give it more time (need ~5-10 similar interactions per task type)
```

---

## See Also

- [Sentiment Detection Overview](../architecture/sentiment-detection.md) — How sentiment is detected
- [Token Budgets](budgets.md) — Set spending limits
- [3-Phase Flow](../architecture/3-phase-flow.md) — How sentiment is used in routing
