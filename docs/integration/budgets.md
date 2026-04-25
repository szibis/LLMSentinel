# Token Budget Configuration Guide

Protect yourself from unexpected token spending with hierarchical budgets.

---

## Quick Setup (2 minutes)

```bash
# 1. Set daily budget
escalation-manager set-budget --daily 10.00

# 2. Set monthly budget  
escalation-manager set-budget --monthly 100.00

# 3. Verify it saved
escalation-manager config

# 4. Start the service
escalation-manager service --port 9000

# Done! System now protects your budgets
```

---

## Budget Levels

Claude Escalate supports **5 levels of budgets** for fine-grained spending control:

### 1. Daily Budget (Primary)

Total USD you're willing to spend per calendar day.

```bash
# Set daily limit
escalation-manager set-budget --daily 10.00

# This means: "Never spend more than $10 per day"
# If limit reached: system auto-downgrades to Haiku or rejects request
```

### 2. Monthly Budget (Ceiling)

Total USD you're willing to spend per calendar month.

```bash
# Set monthly limit  
escalation-manager set-budget --monthly 100.00

# This means: "Never spend more than $100 per month"
# Provides a safety ceiling even if daily limits are high
```

### 3. Per-Model Daily Limits

Maximum daily spending for specific models:

```yaml
# Edit ~/.claude/escalation/config.yaml
budgets:
  model_daily_limits:
    opus: 5.00      # Max $5/day on Opus (expensive)
    sonnet: 3.00    # Max $3/day on Sonnet
    haiku: 0        # 0 = unlimited on Haiku
```

**Example**: Opus costs 15x more than Haiku. You might want to limit Opus to prevent accidental expensive requests.

### 4. Per-Task-Type Token Limits

Maximum tokens for specific task types:

```yaml
budgets:
  task_type_budgets:
    concurrency: 5000      # Limit concurrency problems to 5k tokens
    parsing: 3000          # Parsing max 3k tokens
    debugging: 4000
    architecture: 6000
    # Tokens are cheaper than USD but useful for specific tasks
```

### 5. Session Budget (Optional)

Maximum tokens per Claude Code session:

```bash
escalation-manager set-budget --session 10000

# This means: "Total tokens across all requests in this session < 10k"
```

---

## Hard vs Soft Limits

### Hard Limit (Default: Reject Over-Budget)

```bash
escalation-manager config set budgets.hard_limit true

# Behavior: If request would exceed budget → **REJECT** with error
# Error returned: HTTP 402 (Payment Required)
# User must escalate manually or switch to cheaper model
```

**When to use**:
- You have a strict budget ceiling
- You want to guarantee no surprises
- In production or shared accounts

### Soft Limit (Warn but Allow)

```bash
escalation-manager config set budgets.soft_limit true

# Behavior: If request would exceed budget → **WARN** but allow with confirmation
# Warning shown: "This request would exceed your budget"
# User can: confirm (spend anyway) or cancel
```

**When to use**:
- You want flexibility for important requests
- You prefer warnings over hard blocks
- In development/testing

---

## Enforcement Points

### Phase 1: Pre-Request Check

Before Claude even responds, the system checks:

```
User submits prompt
  ↓
Phase 1: Check budgets
  ├─ Daily budget remaining?
  ├─ Monthly budget remaining?
  ├─ Model daily limit for requested model?
  └─ Task-type token limit?
  ↓
  ├─ ALL OK? → Proceed with Opus (if requested)
  ├─ Daily exceeded? → Downgrade to Sonnet
  ├─ Model limit exceeded? → Downgrade to Haiku
  └─ Hard limit violated? → Reject with error
```

### Phase 2: Real-Time Tracking

If tokens are running higher than expected:

```
During response generation
  ↓
System polls token metrics
  ↓
Actual tokens approaching estimate → Warning in statusline
  ↓
If on track to exceed budget:
  ├─ Show warning: "⚠️ Approaching daily limit"
  ├─ Don't cancel (response already started)
  └─ Prepare to downgrade next request
```

### Phase 3: Post-Response Update

After response completes:

```
Response finished
  ↓
Record actual token usage
  ↓
Update daily/monthly remaining budget
  ↓
If new remaining < 30%:
  └─ Next requests auto-downgrade to Haiku
```

---

## Auto-Downgrade Strategy

When approaching budget limits, system intelligently downgrades:

```yaml
# Trigger at 80% of budget (default)
budgets:
  auto_downgrade_at: 0.80

# Downgrade chain:
# Opus → Sonnet (5x cheaper)
# Sonnet → Haiku (8x cheaper)
# Haiku → Haiku (can't go cheaper)
```

**Example**:
```
Daily budget: $10.00
Current spending: $8.00 (80% of budget)
  ↓
Next request would normally use Opus ($0.60)
  ↓
Check: $8.00 + $0.60 = $8.60 (86% of budget)
  ↓
Trigger: 86% > 80% → AUTO-DOWNGRADE
  ↓
Use Sonnet instead ($0.12 instead of $0.60)
  ↓
New total: $8.12 (81% of budget)
```

---

## Configuration Examples

### Conservative Budget (Tight Spending)

```bash
# Set tight daily budget
escalation-manager set-budget --daily 5.00

# Limit expensive models
escalation-manager config set budgets.model_daily_limits.opus 2.00
escalation-manager config set budgets.model_daily_limits.sonnet 2.00

# Use hard limits (reject over-budget)
escalation-manager config set budgets.hard_limit true

# Auto-downgrade early (at 70% instead of 80%)
escalation-manager config set budgets.auto_downgrade_at 0.70

# Result: Tight control, mostly Haiku/Sonnet, no surprises
```

### Flexible Budget (Development)

```bash
# Generous daily budget
escalation-manager set-budget --daily 50.00

# High per-model limits
escalation-manager config set budgets.model_daily_limits.opus 30.00

# Use soft limits (warn but allow)
escalation-manager config set budgets.soft_limit true

# Auto-downgrade late (at 95%)
escalation-manager config set budgets.auto_downgrade_at 0.95

# Result: Mostly use what you want, warnings only
```

### Balanced Budget (Recommended)

```bash
# Reasonable daily budget
escalation-manager set-budget --daily 10.00

# Reasonable per-model limits
escalation-manager config set budgets.model_daily_limits.opus 5.00
escalation-manager config set budgets.model_daily_limits.sonnet 3.00

# Soft limits (warn + allow flexibility)
escalation-manager config set budgets.soft_limit true

# Auto-downgrade at normal point (80%)
escalation-manager config set budgets.auto_downgrade_at 0.80

# Result: Balanced cost protection + flexibility
```

### Per-Task-Type Limits

```yaml
# ~/.claude/escalation/config.yaml
budgets:
  task_type_budgets:
    concurrency:   5000   # Complex problem, allow more tokens
    parsing:       2000   # Simple extraction, limit tokens
    debugging:     3000   # Medium complexity
    architecture:  6000   # Design problems, need deep reasoning
    refactoring:   2000   # Straightforward changes
    performance:   4000   # Needs analysis
    security:      5000   # Very important, allow more
```

---

## Monitoring Budget

### View Current Status

```bash
# Show all budget info
escalation-manager config

# Output shows:
# Budgets:
#   Daily:   $10.00
#   Monthly: $100.00
#   Session: 10000 tokens
#   Hard Limit: false, Soft Limit: true
```

### Check Budget Dashboard

```bash
escalation-manager dashboard --budget

# Output:
# Daily Budget: $10.00 | Used: $3.78 (38%) | Remaining: $6.22
# Monthly Budget: $100.00 | Used: $45.20 (45%) | Remaining: $54.80
# 
# Model Daily Limits:
#   Opus: $5.00/day | Used: $2.10 (42%)
#   Sonnet: $3.00/day | Used: $1.20 (40%)
#   Haiku: Unlimited | Used: $0.48
```

### Query Budget API

```bash
# Get budget status programmatically
curl http://localhost:9000/api/analytics/budget-status | jq .

# Output:
# {
#   "daily_budget": 10.0,
#   "daily_used": 3.78,
#   "daily_remaining": 6.22,
#   "daily_percent_used": 37.8,
#   "monthly_budget": 100.0,
#   "monthly_used": 45.20,
#   "monthly_remaining": 54.80,
#   "model_usage": {...}
# }
```

---

## Warning Thresholds

System shows warnings at 3 levels:

```yaml
budgets:
  alert_thresholds:
    warn_low: 0.50      # 🟡 Warning at 50% of budget
    warn_med: 0.75      # 🟡 Warning at 75% (stronger)
    warn_high: 0.90     # 🔴 Critical at 90%
```

**What happens**:
- **50%**: Subtle notification, just informational
- **75%**: Prominent warning, statusline shows 🟡
- **90%**: Critical alert, 🔴 in statusline, consider de-escalating

---

## Common Scenarios

### Scenario 1: Daily Budget Approaching Limit

```
9:00 AM: Use Opus for complex architecture problem ($1.50)
2:00 PM: Use Sonnet for debugging issue ($0.30)
4:00 PM: Daily budget $10.00, used $7.80 (78%)

At 78%: System warns 🟡 "Approaching daily limit"

5:00 PM: You want to use Opus again, but:
  → Phase 1 detects: $7.80 + $0.60 (Opus) = $8.40 (84%)
  → Exceeds 80% threshold
  → AUTO-DOWNGRADE to Sonnet ($0.12)

Result: Keeps you under budget while still helping
```

### Scenario 2: Monthly Ceiling Protection

```
Mid-month spending: $60 / $100 (60% of monthly budget)

User sets per-model Opus limit: $8/day
But we're 15 days in: ($60 / 15) * 15 = on track for $120/month

Phase 1 checks:
  ├─ Daily OK? Yes, we spent $4 today
  ├─ Monthly on track? No, trending to 20% over
  └─ Action: Auto-downgrade future Opus requests

Result: Monthly ceiling protected while daily looks fine
```

### Scenario 3: Task-Type Specific Limits

```
Task: Parse large JSON file (typically 1000 tokens)
Task-type budget: parsing: 2000 tokens

Request 1: Actual usage 450 tokens (OK, 22% of task budget)
Request 2: Actual usage 520 tokens (OK, 47% total)
Request 3: Would use 1200 tokens (EXCEEDS, 135% total)

Phase 1 detects:
  → This parsing request would exceed task budget
  → AUTO-DOWNGRADE to Haiku (cheaper, faster)
  → Haiku uses 280 tokens (OK, 62% of task budget)
```

---

## Troubleshooting

### Hard Limit Rejecting Requests

**Problem**: "Payment Required (402)" errors on requests

**Check**:
```bash
# See current spending
escalation-manager dashboard --budget

# Check if hard_limit enabled
escalation-manager config | grep hard_limit
```

**Solutions**:
```bash
# Option 1: Increase daily budget
escalation-manager set-budget --daily 15.00

# Option 2: Switch to soft limit (warn but allow)
escalation-manager config set budgets.hard_limit false
escalation-manager config set budgets.soft_limit true

# Option 3: Reset budgets for today
escalation-manager config set budgets.daily_usd 20.00
```

### Auto-Downgrade Too Aggressive

**Problem**: System keeps using Haiku when you want Opus

**Check**:
```bash
# See auto-downgrade threshold
escalation-manager config | grep auto_downgrade_at
```

**Solutions**:
```bash
# Increase the threshold (downgrade later)
escalation-manager config set budgets.auto_downgrade_at 0.90

# Or increase daily budget
escalation-manager set-budget --daily 20.00

# Or increase Opus daily limit
escalation-manager config set budgets.model_daily_limits.opus 10.00
```

---

## See Also

- [Sentiment Detection](sentiment-detection.md) — Configure frustration escalation
- [3-Phase Flow](../architecture/3-phase-flow.md) — How budgets are checked
- [Dashboards](../analytics/dashboards.md) — View spending analytics
