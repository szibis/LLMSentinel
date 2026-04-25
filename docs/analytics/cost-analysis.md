# Cost Analysis Guide

Understand and optimize your Claude token spending.

---

## Token Pricing

Claude Escalate tracks spending across three models with different cost tiers:

| Model | Input Cost | Output Cost | Relative Cost | Speed |
|-------|-----------|-------------|---|---|
| **Haiku 4.5** | $0.80/MTok | $4.00/MTok | **1x (baseline)** | ⚡ Instant |
| **Sonnet 4.6** | $3.00/MTok | $15.00/MTok | **5x** | 🚗 Normal |
| **Opus 4.7** | $15.00/MTok | $75.00/MTok | **15x** | 🐢 Careful |

*MTok = Million tokens*

### Example: Cost per Request

```
Request: "Debug my race condition"
Estimate: 400 input + 1200 output = 1600 tokens

Model    | Input Cost | Output Cost | Total Cost
---------|-----------|-------------|----------
Haiku    | $0.32     | $4.80       | $5.12
Sonnet   | $1.20     | $18.00      | $19.20  (3.75x)
Opus     | $6.00     | $90.00      | $96.00  (18.75x)

Real-world impact:
  1 Opus request = 19 Haiku requests
```

---

## Daily Budget Examples

### $5/Day Budget (Tight)

```
Allocation Strategy:
- Mostly Haiku (cheap, fast)
- Sonnet for medium tasks
- No Opus unless critical

Typical day:
  10 Haiku requests @ $0.20 avg = $2.00
  3 Sonnet requests @ $0.80 avg = $2.40
  1 Opus request (critical)     = $0.60
  Total: $5.00 ✅

Usage: 50-70 requests/day possible
Frustration risk: Medium (Haiku succeeds 70% of time)
Recommendation: Good for casual use
```

### $10/Day Budget (Standard)

```
Allocation Strategy:
- Balance Haiku + Sonnet
- Limited Opus (expensive problems only)
- Room for escalations when frustrated

Typical day:
  15 Haiku requests @ $0.20 avg = $3.00
  8 Sonnet requests @ $0.60 avg = $4.80
  2 Opus requests (hard problems)= $1.20
  Total: $9.00 ✅

Usage: 100-150 requests/day
Frustration risk: Low (Sonnet succeeds 85% of time)
Recommendation: Good for active development
```

### $25/Day Budget (Generous)

```
Allocation Strategy:
- Use best model for each task
- Frequent Opus for complex work
- No constraints on escalations

Typical day:
  10 Haiku requests @ $0.20 avg = $2.00
  10 Sonnet requests @ $0.60 avg = $6.00
  10 Opus requests @ $1.20 avg  = $12.00
  Other tasks                    = $5.00
  Total: $25.00 ✅

Usage: 200+ requests/day
Frustration risk: Very low (use Opus liberally)
Recommendation: For serious development work
```

---

## Cost Optimization Strategies

### Strategy 1: Haiku-First for Simple Tasks

Test if Haiku works before escalating:

```
Question: "What's the syntax for defer in Go?"

Attempt 1: Haiku ($0.02)
  Output: "The defer statement..."
  User reaction: "Perfect, thanks"
  Total cost: $0.02

vs.

Attempt 1: Opus ($0.30)
  Output: Same answer + more explanation
  Total cost: $0.30 ❌

Savings: $0.28 (14x cheaper!)
Strategy: Haiku-first, escalate only if needed
```

### Strategy 2: Sonnet for 80% of Work

Sonnet is the sweet spot for most tasks:

```
Task Type      | Haiku Success | Sonnet Success | Opus Success | Cost Ratio
---------------|--------------|----------------|-------------|----------
Simple Q&A     | 95%           | 99%            | 100%        | 1x / 5x / 15x
Debugging      | 60%           | 92%            | 98%         | 1x / 5x / 15x
Architecture   | 30%           | 85%            | 98%         | 1x / 5x / 15x
Refactoring    | 85%           | 95%            | 98%         | 1x / 5x / 15x

Recommendation: Use Sonnet by default
  - Haiku for simple questions (high success rate)
  - Opus only for architecture / hard problems
```

### Strategy 3: Identify Expensive Task Types

Track which tasks cost most and optimize:

```
Your task breakdown (monthly):

1. Concurrency problems: 25 tasks × $0.80 avg = $20.00 (37%)
   - Currently: Opus for all
   - Learning: Sonnet succeeds 78% of time
   - Optimization: Sonnet-first ($0.60), escalate if needed
   - Savings: ~$5.00/month (25%)

2. Parsing tasks: 50 tasks × $0.40 avg = $20.00 (37%)
   - Currently: Sonnet for all
   - Learning: Haiku succeeds 72% of time
   - Optimization: Haiku-first ($0.05), escalate if needed
   - Savings: ~$17.50/month (87%)

3. Debugging: 15 tasks × $0.75 avg = $11.25 (21%)
   - Currently: Sonnet + occasional Opus
   - Learning: Sonnet 92% success (good enough)
   - Optimization: Sonnet for all
   - Savings: ~$1.50/month (13%)

4. Other: 10 tasks × $0.30 avg = $3.00 (5%)

Total current: $54.25/month
Optimized: $36.00/month
Total savings: $18.25 (34% reduction) 💰
```

---

## Monthly Budget Planning

### Low Usage (< 50 requests/month)

```
Recommended budget: $5-10/month

Why: Small number of requests means predictable spending
     Failures/escalations manageable

Example allocation:
  - Daily limit: $1-2
  - Monthly limit: $10
  - Model limits: Opus $1/day, Sonnet $0.50/day

Expected outcomes:
  - 50 requests @ $0.12 avg = $6/month
  - Room for 3-4 escalations without exceeding budget
```

### Medium Usage (50-200 requests/month)

```
Recommended budget: $15-30/month

Why: Regular work, enough volume to learn patterns
     Can optimize by task type

Example allocation:
  - Daily limit: $1.50-2.00
  - Monthly limit: $30
  - Model limits: Opus $1/day, Sonnet $1.50/day, Haiku unlimited

Expected outcomes:
  - 100 requests @ $0.20 avg = $20/month
  - 4-5 escalations included
  - Learning data available for optimization
```

### High Usage (> 200 requests/month)

```
Recommended budget: $30-100+/month

Why: Heavy development, want flexibility
     Enough data to optimize extensively

Example allocation:
  - Daily limit: $3-5
  - Monthly limit: $100
  - Model limits: High limits to not constrain

Expected outcomes:
  - 250+ requests @ $0.15-0.20 avg = $40-50/month
  - Optimized distribution based on learning
  - Most frustration cases handled
```

---

## Calculating ROI of Better Models

**Question**: Is Opus worth 15x the cost of Haiku?

**Answer**: Depends on context.

### Case 1: Debugging Complex Issue

```
Problem: Race condition in Go code
Time to solve: Critical (blocks release)

Option A: Haiku (15 attempts needed, failures)
  Cost: 15 × $0.10 = $1.50
  Time: 30 minutes (multiple escalations)
  Success: Maybe

Option B: Opus (1 attempt, right answer)
  Cost: $0.30
  Time: 5 minutes
  Success: Yes

ROI: $1.50 - $0.30 = $1.20 saved PLUS 25 min saved ✅
Conclusion: Opus worth it for blocking issues
```

### Case 2: Simple Code Explanation

```
Problem: "What's Go's context package?"
Time to solve: Not urgent

Option A: Haiku (direct answer, good)
  Cost: $0.02
  Time: 2 seconds
  Success: Yes

Option B: Opus (detailed explanation, better)
  Cost: $0.30
  Time: 3 seconds
  Success: Yes (extra details)

ROI: You pay $0.28 for extra details you don't need ❌
Conclusion: Haiku adequate for simple questions
```

**General Rule**:
- Use **Haiku**: For simple, time-insensitive questions
- Use **Sonnet**: For most development work (best balance)
- Use **Opus**: For complex problems, important decisions, architecture

---

## Real-World Cost Scenarios

### Scenario 1: Full-Time Developer

```
Usage: 300 requests/month

Distribution (optimized):
  - Haiku (simple): 100 @ $0.05 = $5.00
  - Sonnet (medium): 150 @ $0.50 = $75.00
  - Opus (hard): 50 @ $1.00 = $50.00
  Total: $130/month

Daily average: $4.30/day
Weekly cost: $30

Frustration impact: Low (good model selection)
Optimization: Could save $10-15 by switching some Opus → Sonnet
```

### Scenario 2: Side Project Developer

```
Usage: 50 requests/month

Distribution:
  - Haiku: 30 @ $0.05 = $1.50
  - Sonnet: 15 @ $0.50 = $7.50
  - Opus: 5 @ $1.00 = $5.00
  Total: $14/month

Daily average: $0.47/day
With $5/day budget: Room for 10x more requests

Frustration impact: Medium (sometimes hit Haiku limitations)
Optimization: Try Haiku-first more aggressively
```

### Scenario 3: AI Research / Complex Work

```
Usage: 500 requests/month

Distribution:
  - Haiku: 50 @ $0.05 = $2.50
  - Sonnet: 150 @ $0.50 = $75.00
  - Opus: 300 @ $1.00 = $300.00
  Total: $377.50/month

Daily average: $12.50/day
Weekly cost: $87.50

Frustration impact: Minimal (use best model for everything)
Optimization: Worth the cost given complexity of work
```

---

## Cost Tracking Over Time

### Weekly Review

```bash
# Check weekly spending
curl http://localhost:9000/api/analytics/budget-status | jq .

# Analyze trends
escalation-manager dashboard --budget
```

### Monthly Analysis

```bash
# Extract data for analysis
curl http://localhost:9000/api/analytics/cost-trends?period=month | jq .

# Typical format:
# {
#   "period": "2024-04",
#   "total_spent": 157.50,
#   "by_model": {
#     "haiku": 12.50,
#     "sonnet": 87.50,
#     "opus": 57.50
#   },
#   "by_task_type": {...},
#   "average_per_request": 0.63,
#   "request_count": 250
# }
```

### Yearly Budget

```
Monthly average: $50/month
Yearly: $600/year

Historical trend: Decreasing as you learn
  Month 1: $80 (learning)
  Month 2: $65 (optimizing)
  Month 3: $50 (converged)
  Month 4+: $50 (stable)
```

---

## See Also

- [Budget Configuration](../integration/budgets.md) — Set spending limits
- [Dashboards](dashboards.md) — View cost analysis
- [Sentiment Detection](../integration/sentiment-detection.md) — Minimize expensive failures
