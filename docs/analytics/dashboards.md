# Analytics Dashboards

View your Claude usage patterns, spending, sentiment, and optimization opportunities.

---

## Dashboard Access

### Web Dashboard

```bash
# Start the service
escalation-manager service --port 9000

# Open browser: http://localhost:9000
# or: open http://localhost:9000  (macOS)
# or: start http://localhost:9000  (Windows)
```

### CLI Dashboard

```bash
# View all dashboards
escalation-manager dashboard

# View specific section
escalation-manager dashboard --sentiment
escalation-manager dashboard --budget
escalation-manager dashboard --optimization
```

---

## Dashboard Tabs

### Overview Tab

**Purpose**: Quick status snapshot of your Claude usage

Shows:
- **Current Model**: Which model is set for escalation (Haiku/Sonnet/Opus)
- **Effort Level**: Current task effort setting (Low/Medium/High)
- **Escalation Count**: How many times you've escalated this session
- **Cost Analysis**: Pie chart of Haiku vs Sonnet vs Opus spending
- **Recent Sessions**: List of recent requests with outcomes (5-10 most recent)

**Key Metrics**:
- Total tokens used today
- Total cost today
- Average tokens per request
- Average cost per request

**Use Case**: 
- "Am I spending too much on expensive models?"
- "How many times have I escalated today?"
- "Is my distribution of models reasonable?"

---

### Sentiment Tab

**Purpose**: Track user satisfaction and frustration patterns

Shows:
- **Satisfaction Rate**: % of requests where user was satisfied (target: >80%)
- **Sentiment Breakdown**: Pie chart showing:
  - 😊 Satisfied (green)
  - 😐 Neutral (gray)
  - 😤 Frustrated (red)
  - 🤔 Confused (yellow)
  - ⏰ Impatient (orange)

- **Frustration Events**: Recent times user got frustrated + what helped
  - Time: when it happened
  - Context: what task type
  - Initial Model: what you started with
  - Outcome: how it was resolved

- **Model Satisfaction by Task Type**: Table showing:
  - Task Type (concurrency, parsing, debugging, etc.)
  - Haiku success rate (%)
  - Sonnet success rate (%)
  - Opus success rate (%)
  - Recommendation: which model works best

**Key Metrics**:
- Satisfaction rate (target: >85%)
- Frustration events this week
- Most effective model for each task type
- Trends over time (improving or declining satisfaction?)

**Use Case**:
- "Am I getting frustrated less often?"
- "Which models work best for my concurrency questions?"
- "Should I escalate more aggressively or less?"

---

### Budget Tab

**Purpose**: Monitor token spending against limits

Shows:
- **Daily Budget**: 
  - Bar chart showing: Used / Limit
  - Color coded: 🟢 <75%, 🟡 75-90%, 🔴 90%+
  - Numbers: $X.XX used of $Y.YY daily limit
  - Remaining: $Z.ZZ
  - Trend: ↑ increasing, ↓ decreasing, → stable

- **Monthly Budget**:
  - Same format as daily
  - Shows days remaining in month
  - Projected total if trend continues

- **Per-Model Spending**:
  - Opus: $X.XX / $Y.YY daily limit (percent used)
  - Sonnet: $X.XX / $Y.YY daily limit
  - Haiku: $X.XX (unlimited)

- **Task Type Spending**:
  - Shows token usage by task type
  - Highlights which tasks cost most
  - Identifies optimization opportunities

**Key Metrics**:
- % of daily budget used
- Remaining budget
- Spending trend (on pace for budget?)
- Per-model utilization
- Most expensive task types

**Use Case**:
- "Am I on track to exceed my budget today?"
- "Which models am I spending most on?"
- "Where can I cut costs?"

---

### Optimization Tab

**Purpose**: Identify cost-saving opportunities

Shows:
- **Cost Savings Opportunities**: Numbered list of suggestions:
  1. "Concurrency tasks: 98% success on Sonnet (vs 45% on Haiku) but 1/15 cost"
     - Impact: Save $0.45/month if you switch
     - Status: HIGH CONFIDENCE (15 samples)
  
  2. "Parsing tasks waste 20% on Opus, switch to Haiku"
     - Impact: Save $0.15/month
     - Status: MEDIUM CONFIDENCE (8 samples)

  3. "Debugging on Opus: success rate same as Sonnet, but 3x cost"
     - Impact: Save $2.10/month
     - Status: HIGH CONFIDENCE (25 samples)

- **Current Model Distribution**:
  - Haiku: 15% of requests, 2% of cost
  - Sonnet: 50% of requests, 45% of cost
  - Opus: 35% of requests, 53% of cost

- **Recommended Distribution** (based on learning):
  - Haiku: 25% → +10% (better for simple tasks)
  - Sonnet: 60% → +10% (balanced, good success rates)
  - Opus: 15% → -20% (use only for hard problems)

- **Estimated Savings**: $X.XX/month (~Y% reduction)

- **Frustration Impact**: 
  - Current frustration rate: X%
  - If you follow recommendations: Y% (lower is better)

**Key Metrics**:
- Monthly savings opportunity ($)
- Percentage cost reduction (%)
- Frustration rate impact
- Confidence level of each recommendation

**Use Case**:
- "Where am I wasting money?"
- "Should I upgrade my hardware instead of using Opus?"
- "Can I be more frustrated but save more?"

---

## CLI Dashboard Output Examples

### Sentiment Dashboard

```
=== Sentiment Dashboard ===
Satisfaction Rate: 87.3% (62/71) ✅

Sentiment Breakdown:
  😊 Satisfied:  62 (87%)  ███████████████████
  😐 Neutral:     7 (10%)  ██
  😤 Frustrated:  2 (3%)   █
  🤔 Confused:    0 (0%)
  ⏰ Impatient:   0 (0%)

Recent Frustration Events (last 24h):
  [2h ago] Concurrency on Haiku
    → Auto-escalated to Sonnet
    → Result: RESOLVED ✅

  [30m ago] Parsing on Haiku
    → Succeeded immediately
    → No escalation needed ✅

Model Satisfaction (by task type):
  Task Type     | Haiku  | Sonnet | Opus
  --------------|--------|--------|------
  concurrency   | 45%    | 78%    | 98%
  parsing       | 72%    | 89%    | 96%
  debugging     | 88%    | 92%    | 95%
  architecture  | 30%    | 85%    | 98%
```

### Budget Dashboard

```
=== Budget Dashboard ===

Daily Budget: $10.00
  Used: $3.78 (38%) 🟢
  Remaining: $6.22
  Trend: Stable (used $0.94/hour)
  Estimated total today: $6.83

Monthly Budget: $100.00
  Used: $45.20 (45%) 🟢
  Remaining: $54.80
  Days remaining: 6
  Projected total: $102.50 ⚠️

Per-Model Daily Limits:
  Opus:   $5.00/day  Used: $2.10 (42%) 🟢
  Sonnet: $3.00/day  Used: $1.20 (40%) 🟢
  Haiku:  Unlimited  Used: $0.48

Task Type Spending:
  Task Type   | Tokens | Cost  | Avg/Request
  ------------|--------|-------|------------
  concurrency | 4,200  | $2.52 | $0.63
  debugging   | 2,100  | $0.84 | $0.28
  parsing     | 1,800  | $0.36 | $0.06
  general     | 2,300  | $0.06 | $0.01
```

### Optimization Dashboard

```
=== Optimization Dashboard ===

Cost Savings Opportunities:

1. Concurrency on Sonnet (HIGH CONFIDENCE ⭐⭐⭐)
   → Success rate: 78% on Sonnet vs 45% on Haiku
   → Cost difference: 1/15th (Sonnet cheaper in practice due to fewer retries)
   → Your usage: 8 concurrency questions (2 frustrated on Haiku, 5 satisfied on Sonnet)
   → Potential savings: $0.36/month
   → Recommendation: DEFAULT to Sonnet for concurrency

2. Parsing on Haiku (MEDIUM CONFIDENCE ⭐⭐)
   → Success rate: 72% on Haiku, 89% on Sonnet
   → Trade-off: 17% lower success, but 8x cheaper
   → Your usage: 25 parsing tasks (20 succeeded on Haiku, 5 on Sonnet)
   → Potential savings: $1.20/month
   → Recommendation: Try Haiku first, escalate if needed

3. Debugging on Sonnet (HIGH CONFIDENCE ⭐⭐⭐)
   → Success rate: 92% on Sonnet vs 95% on Opus (tiny difference)
   → Cost difference: 3x (Opus costs 15x Haiku, Sonnet costs 5x)
   → Your usage: 6 debugging tasks on Opus (all succeeded)
   → Potential savings: $1.80/month
   → Recommendation: DEFAULT to Sonnet, reserve Opus for complex issues

Estimated Total Savings: $3.36/month (~7% reduction)
Frustration Impact: Current 13% frustration → projected 8% (-5%)

Would You Save Money or Frustration?
  → SAVE BOTH! (more savings + fewer frustrations)
```

---

## Customizing Dashboard

### Change Dashboard Refresh Rate

Edit `~/.claude/escalation/config.yaml`:

```yaml
display:
  refresh_interval_ms: 500    # Update every 500ms
```

### Disable Certain Metrics

```yaml
display:
  display_model: true
  display_effort: true
  display_tokens: true
  display_sentiment: true        # Hide if not interested
  display_budget_remaining: true
```

### Export Dashboard Data

```bash
# Get raw JSON for analysis
curl http://localhost:9000/api/analytics/budget-status | jq . > budget.json
curl http://localhost:9000/api/analytics/sentiment-trends | jq . > sentiment.json
curl http://localhost:9000/api/analytics/cost-optimization | jq . > optimization.json

# Import into analytics tool (Excel, etc.)
```

---

## Dashboard in Barista Statusline

If using Barista integration, the statusline shows a mini-dashboard:

```
🚀 OPUS(15x) • Effort: HIGH 🔥 • Tokens: 1.6k | Budget: $6.22 left | Sentiment: 😐 neutral
```

Breaking it down:
- **🚀 OPUS(15x)**: Current model (cost multiplier)
- **Effort: HIGH 🔥**: Task complexity setting
- **Tokens: 1.6k**: Estimated tokens for this request
- **Budget: $6.22 left**: Daily budget remaining
- **Sentiment: 😐 neutral**: Current sentiment

---

## Interpreting Dashboard Metrics

### Satisfaction Rate

| Rate | Status | Action |
|------|--------|--------|
| >90% | Excellent | Keep current settings |
| 80-90% | Good | Minor tweaks if needed |
| 70-80% | Fair | Consider lowering frustration threshold |
| <70% | Poor | Lower threshold, escalate more aggressively |

### Budget Usage

| % Used | Status | Action |
|--------|--------|--------|
| 0-50% | Healthy | No action needed |
| 50-75% | Caution | Monitor usage |
| 75-90% | Warning | Consider de-escalating |
| >90% | Critical | Auto-downgrade enabled |

### Model Distribution

| Model % | Interpretation |
|---------|---|
| Haiku >40% | Good! Using cheap model frequently |
| Sonnet 30-50% | Balanced, most common pattern |
| Opus >30% | Consider switching some to Sonnet |
| Even distribution | Optimal based on task needs |

---

## See Also

- [Budget Configuration](../integration/budgets.md) — Understand spending
- [Sentiment Detection](../integration/sentiment-detection.md) — Satisfaction metrics
- [Monitoring](operations/monitoring.md) — Production monitoring
