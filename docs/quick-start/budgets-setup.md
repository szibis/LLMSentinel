# Token Budget Setup Guide

Protect yourself from unexpected costs with intelligent token budgets.

## Why Budgets?

You want to:
- **Control spending** — Set maximum daily/monthly budget
- **Prevent surprises** — No more "$50 surprise bill"
- **Optimize models** — Automatically use cheaper models when approaching limit
- **Per-model limits** — Cap expensive models (Opus) while letting cheap ones (Haiku) run free

## Quick Setup (2 minutes)

### Set Daily Budget
```bash
escalation-manager set-budget --daily 10.00
```

This sets a hard limit of $10/day. When approaching, system will:
1. Show warning in statusline (at 50%, 75%, 90%)
2. Auto-downgrade models if needed
3. Reject new requests if budget fully exceeded

### Set Monthly Budget (Optional)
```bash
escalation-manager set-budget --daily 10.00 --monthly 100.00
```

Now you have:
- Daily limit: $10
- Monthly limit: $100
- System respects whichever is stricter

## Advanced Configuration

Create a config file for granular control:

```yaml
# ~/.claude/escalation/config.yaml

budgets:
  daily_usd: 10.00
  monthly_usd: 100.00
  session_tokens: 5000       # Max per session
  
  hard_limit: false          # true = reject, false = warn
  soft_limit: true           # Warn at thresholds
  
  auto_downgrade_at: 0.80    # Downgrade when 80% used
  
  model_daily_limits:
    opus: 5.00               # Max $5/day on Opus
    sonnet: 3.00             # Max $3/day on Sonnet
    haiku: unlimited          # Haiku always allowed
  
  task_type_budgets:
    concurrency: 5000        # Max 5000 tokens on concurrency
    parsing: 3000
    debugging: 4000
    architecture: 6000
```

Load with:
```bash
escalation-manager config load ~/.claude/escalation/config.yaml
```

## Budget Levels Explained

### Daily Budget
- Resets every 24 hours (midnight UTC)
- Hard limit per calendar day
- Example: `--daily 10.00` = $10/day

### Monthly Budget
- Resets on 1st of each month
- Accumulates across all days
- Example: `--monthly 100.00` = $100/month

### Per-Model Limits
- Set maximum spend for each model per day
- Opus can be limited to $5, while Sonnet $3, Haiku unlimited
- System routes to cheaper model when expensive model limit reached

### Per-Task-Type Budgets
- Set maximum tokens for specific task types
- "concurrency" tasks limited to 5000 tokens
- "parsing" tasks limited to 3000 tokens
- Helps prevent runaway tokens on specific task patterns

### Session Budget
- Maximum tokens for current session
- Resets on new session/day
- Example: `session_tokens: 10000` = 10k tokens max this session

## Hard vs Soft Limits

### Soft Limit (Default: Recommended)
```yaml
hard_limit: false
soft_limit: true
```

When approaching budget:
1. Show warning (yellow) — "75% of daily budget used"
2. Auto-downgrade to cheaper model
3. Allow request but notify user
4. Continue operations normally

**Best for**: Normal users who want protection + flexibility

### Hard Limit
```yaml
hard_limit: true
soft_limit: false
```

When budget exceeded:
1. Reject request entirely
2. Return HTTP 402 (Payment Required)
3. Show: "Daily budget exceeded: $10.00"
4. Stop escalation cold

**Best for**: Strict cost control, testing, or managed environments

## How System Uses Budgets

### Phase 1: Pre-Response Check
```
User types prompt
  ↓
System checks: "Is this within budget?"
  ├─ YES → Route normally
  ├─ NO (soft limit) → Warn + recommend cheaper model
  └─ NO (hard limit) → Reject request
```

### Phase 2: Real-Time Monitoring
```
Claude generating response
  ↓
System tracks: "Are tokens tracking toward over-budget?"
  ├─ YES → Show warning in statusline
  └─ NO → Continue normally
```

### Phase 3: Decision Making
```
Response complete
  ↓
System calculates: "Approaching limit?"
  ├─ YES → Next request uses cheaper model
  └─ NO → Continue normal routing
```

## Common Scenarios

### Scenario 1: $10/day Casual User
```bash
escalation-manager set-budget --daily 10.00
```

Good for: Light usage, personal projects  
Cost: ~0.33/day ≈ $10/month

### Scenario 2: $30/day Active Developer
```bash
escalation-manager set-budget --daily 30.00 --monthly 500.00
```

Good for: Active development, frequent escalations  
Cost: ~1/day ≈ $30/month, capped at $500/month

### Scenario 3: Per-Model Limits (Conservative)
```yaml
budgets:
  daily_usd: 50.00
  model_daily_limits:
    opus: 5.00       # Only $5/day on Opus (expensive)
    sonnet: 20.00    # $20/day on Sonnet (medium)
    haiku: unlimited # Unlimited Haiku (cheap)
  auto_downgrade_at: 0.70
```

Good for: Careful cost management, prefer Haiku first  
Behavior: Opus used sparingly, Sonnet for medium tasks, Haiku as default

### Scenario 4: Testing (Hard Limit)
```bash
escalation-manager set-budget --daily 5.00  # Small test budget
escalation-manager config set budgets.hard_limit true
```

Good for: Testing before production, catching regressions  
Behavior: Hard stop at $5, no exceptions

## Monitoring Budget Usage

### Check Current Status
```bash
escalation-manager budget status
```

Output:
```
Daily Budget: $10.00
├─ Used: $3.78 (38%)
├─ Remaining: $6.22
└─ Projected: $5.40/day (based on usage trend)

Monthly Budget: $100.00
├─ Used: $45.20 (45%)
└─ Remaining: $54.80
```

### View Spending by Model
```bash
escalation-manager budget breakdown
```

Output:
```
Opus:   $2.10 (56% of daily limit)
Sonnet: $1.20 (40% of daily limit)
Haiku:  $0.48 (4% of spend)
```

### List Recent Transactions
```bash
escalation-manager budget transactions --recent 10
```

### Export Budget History
```bash
escalation-manager budget export --format csv --output spending.csv
escalation-manager budget export --format json --output spending.json
```

## Dashboard View

Open web dashboard to see budget graphically:
```bash
escalation-manager dashboard
```

Navigate to "Budget & Spending" tab:
- Visual progress bars (daily, monthly)
- Model breakdown pie chart
- Daily trend line (is usage increasing?)
- Recommendations (e.g., "Switch concurrency to Sonnet to save 40%")

## Alerts & Notifications

System shows alerts when:

| Usage Level | Alert Level | Action |
|------------|-----------|--------|
| 0-50% | ✅ OK | None |
| 50-75% | 🟡 YELLOW | Warn in statusline |
| 75-90% | 🟠 ORANGE | Strong warning |
| 90-100% | 🔴 RED | Active downgrade to Haiku |
| >100% | ⛔ BLOCKED | Hard stop (if hard_limit: true) |

Example statusline display:
```
🔴 Budget Alert: 92% used ($9.20/$10) | Switched to Haiku to save money
```

## Reset & Troubleshooting

### Reset Budget (Start Over)
```bash
escalation-manager budget reset --confirm
```

### Change Budget Retroactively
```bash
escalation-manager set-budget --daily 20.00  # Increase limit
escalation-manager set-budget --daily 5.00   # Decrease limit
```

### Check Budget File
```bash
cat ~/.claude/escalation/config.yaml | grep budgets -A 20
```

### Disable Budget Enforcement (Careful!)
```bash
escalation-manager config set budgets.soft_limit false
escalation-manager config set budgets.hard_limit false
```

## Integration with Auto-Effort

Budget system works with auto-effort hook:

```
Easy task + High budget → Estimate with Opus
Easy task + Low budget (10%) → Estimate with Haiku
Hard task + High budget → Estimate with Opus
Hard task + Low budget → Estimate with Sonnet
```

The system balances:
- **Task difficulty** (effort estimation)
- **Budget remaining** (percentage)
- **Historical success** (learned patterns)

## Next Steps

1. Set your budget: `escalation-manager set-budget --daily 10.00`
2. Monitor spending: `escalation-manager dashboard`
3. Review recommendations: Check "Cost Analysis" tab
4. Adjust as needed: Create `config.yaml` for fine control

See also:
- [System Overview](../architecture/overview.md)
- [Cost Analysis](../analytics/cost-analysis.md)
- [Troubleshooting](../operations/troubleshooting.md)
