# Claude Escalate: Getting Started Guide

**Time to Production**: 10 minutes | **Complexity**: Beginner-friendly

---

## 🚀 Quick Start (5 Minutes)

### 1. Download & Setup
```bash
# Clone or download the escalation system
cd ~/.claude
git clone https://github.com/anthropics/claude-escalate.git
cd claude-escalate

# Build the binary
go build -o escalation-manager ./cmd/escalation-cli/main.go

# Add to PATH (optional)
ln -s $(pwd)/escalation-manager ~/.local/bin/
```

### 2. Configure Basic Budgets
```bash
# Set your daily and monthly budgets
escalation-manager set-budget --daily 10.00 --monthly 100.00

# Verify configuration
escalation-manager config
```

### 3. Start the Service
```bash
# Terminal 1: Start the service (runs on port 9000)
escalation-manager service --port 9000

# Terminal 2: View dashboards
escalation-manager dashboard
```

### 4. Access Web Dashboard
Open your browser: `http://localhost:9000`

**That's it! You're now protecting your tokens and tracking sentiment.**

---

## 📋 Configuration Guide

### Default Configuration Locations

```
~/.claude/escalation/config.yaml       # Main configuration (auto-created)
~/.claude/data/escalation/             # Data directory
  ├── escalation.db                    # SQLite database
  ├── barista-metrics.json             # Token metrics from barista
  └── escalation.log                   # Activity log
```

### Common Configurations

**Development (High Limits)**:
```bash
escalation-manager set-budget --daily 100.00 --monthly 1000.00
```

**Production (Conservative)**:
```bash
escalation-manager set-budget --daily 5.00 --monthly 50.00
```

**Education (No Budget)**:
```bash
escalation-manager config set budgets.daily_usd 0
# Sets budget to unlimited
```

### Enable Features via Config

**Turn on sentiment-driven escalation**:
```bash
escalation-manager config set sentiment.enabled true
escalation-manager config set sentiment.frustration_trigger_escalate true
```

**Set frustration threshold (0.0-1.0)**:
```bash
escalation-manager config set sentiment.frustration_risk_threshold 0.75
# Higher = less likely to escalate, Lower = more sensitive
```

**Change budget enforcement mode**:
```bash
escalation-manager config set budgets.hard_limit true    # Reject over-budget requests
escalation-manager config set budgets.soft_limit true    # Warn but allow
```

---

## 🎯 Use Cases

### Use Case 1: Budget Protection
**Goal**: Prevent overspending on expensive models (Opus)

```bash
# Set daily budget
escalation-manager set-budget --daily 10.00

# Set per-model limits
escalation-manager config set budgets.model_daily_limits.opus 5.00

# Now Opus requests that exceed $5/day auto-downgrade to Sonnet
```

### Use Case 2: Frustration-Aware Escalation
**Goal**: Auto-escalate when you're struggling

```bash
# Enable sentiment detection
escalation-manager config set sentiment.enabled true

# Set threshold (0.70 = escalate at 70% frustration risk)
escalation-manager config set sentiment.frustration_risk_threshold 0.70

# Now system auto-escalates: Haiku → Sonnet → Opus as needed
```

### Use Case 3: Task-Specific Budgets
**Goal**: Limit expensive tasks (like architecture design)

```yaml
# Edit ~/.claude/escalation/config.yaml and set:
budgets:
  task_type_budgets:
    architecture: 5000      # 5k tokens max per architecture task
    concurrency: 5000
    debugging: 4000
```

### Use Case 4: Multi-Source Metrics
**Goal**: Get metrics from multiple sources with fallback

```yaml
# Edit ~/.claude/escalation/config.yaml:
statusline:
  sources:
    - type: barista        # Primary (from RTK)
      enabled: true
    - type: claude-native  # Fallback (from Claude settings)
      enabled: true
    - type: envvar         # Last resort (env variables)
      enabled: true
```

---

## 📊 Dashboard Guide

### Web Dashboard (http://localhost:9000)

**Overview Tab**:
- Current model and effort level
- Escalation/de-escalation counts
- Cost analysis (Haiku/Sonnet/Opus distribution)
- Recent sessions with token savings

**Sentiment Tab**:
- Satisfaction rate (% satisfied)
- 5-sentiment breakdown (satisfied, neutral, frustrated, confused, impatient)
- Frustration events with escalation outcomes
- Model satisfaction rates by task type

**Budget Tab**:
- Daily budget status with color-coded bar
  - 🟢 Green: 0-75% (healthy)
  - 🟡 Yellow: 75-90% (warning)
  - 🔴 Red: 90%+ (critical)
- Monthly budget with days remaining
- Per-model spending breakdown

**Optimization Tab**:
- Cost savings opportunities
- Current model vs recommended model
- Estimated savings percentage
- Average monthly savings

### CLI Dashboard

```bash
# View all dashboards
escalation-manager dashboard

# View specific dashboard
escalation-manager dashboard --sentiment
escalation-manager dashboard --budget
escalation-manager dashboard --optimization

# Connect to custom server
escalation-manager dashboard --server http://other-host:9000
```

---

## 🔧 Advanced Configuration

### Enable Detailed Logging
```yaml
# ~/.claude/escalation/config.yaml
logging:
  level: debug                    # debug, info, warn, error
  file: ~/.claude/data/escalation/escalation.log
  retention_days: 30
```

### Set Per-Model Budgets
```bash
escalation-manager config set budgets.model_daily_limits.opus 5.0
escalation-manager config set budgets.model_daily_limits.sonnet 3.0
escalation-manager config set budgets.model_daily_limits.haiku 0
# 0 = unlimited for that model
```

### Hard vs Soft Limits

**Hard Limit** (Reject over-budget):
```bash
escalation-manager config set budgets.hard_limit true
# Requests that exceed budget are rejected with HTTP 402 error
```

**Soft Limit** (Warn but allow):
```bash
escalation-manager config set budgets.soft_limit true
# Requests over budget show warning but proceed with cheaper model
```

---

## 🚨 Troubleshooting

### Dashboard shows no data
```bash
# Check if service is running
curl http://localhost:9000/api/health

# Check logs
tail -f ~/.claude/data/escalation/escalation.log

# Ensure database exists
ls ~/.claude/data/escalation/escalation.db
```

### Configuration not saving
```bash
# Check file permissions
ls -la ~/.claude/escalation/config.yaml

# Recreate config directory
mkdir -p ~/.claude/escalation
escalation-manager config set budgets.daily_usd 10.0
```

### Budget checks not working
```bash
# Verify budget is set
escalation-manager config

# Check that service loaded config
escalation-manager service --port 9000 2>&1 | grep -i budget
```

### Sentiment escalation not triggering
```bash
# Verify sentiment is enabled
escalation-manager config set sentiment.enabled true

# Check frustration threshold
escalation-manager config | grep frustration_risk_threshold

# View frustration events
escalation-manager dashboard --sentiment
```

---

## 📈 Monitoring & Metrics

### Key Metrics to Watch

**Daily**:
- Budget usage % (should stay <80%)
- Satisfaction rate (target: >80%)
- Model distribution (% Haiku vs Sonnet vs Opus)

**Weekly**:
- Cost trends (is spending increasing or stable?)
- Frustration events (how often auto-escalating?)
- Model satisfaction by task type

**Monthly**:
- Total cost vs budget
- Savings from auto-escalation
- Learning patterns (which tasks use which models?)

### Export Data for Analysis

```bash
# View all sentiment outcomes (CSV-friendly format)
curl http://localhost:9000/api/analytics/sentiment-trends?hours=720 | jq .

# View budget history
curl http://localhost:9000/api/analytics/budget-status | jq .

# Get cost optimization opportunities
curl http://localhost:9000/api/analytics/cost-optimization | jq .
```

---

## 🔌 Integration with Claude Code

### Hook Integration
Claude Escalate automatically integrates with Claude Code hook system:

1. **Phase 1** (Pre-response):
   - Sentiment detection runs
   - Budget check runs
   - Model routing decision made

2. **Phase 2** (During response):
   - Token metrics collected from statusline
   - Sentiment sampled from user behavior

3. **Phase 3** (Post-response):
   - Actual metrics recorded
   - Learning patterns stored
   - Next routing decision made

No additional setup needed - the system works with Claude Code's existing hook infrastructure.

### Environment Variables (Optional)

If you don't have Barista/native statusline, use env vars:

```bash
export CLAUDE_TOKENS_ACTUAL=1500
export CLAUDE_TOKENS_INPUT=400
export CLAUDE_TOKENS_OUTPUT=1100
export CLAUDE_CACHE_HIT_TOKENS=0
export CLAUDE_CACHE_CREATION_TOKENS=0
export CLAUDE_MODEL="claude-sonnet-4-6"

escalation-manager dashboard
```

---

## 📚 Next Steps

1. **Set your budgets** (use `set-budget` command)
2. **Configure sentiment** (enable in config)
3. **Monitor dashboards** (web or CLI)
4. **Adjust thresholds** as needed based on your usage patterns

### Learn More

- **Configuration Details**: See `IMPLEMENTATION_STATUS.md`
- **API Reference**: See `docs/integration/api-reference.md`
- **Architecture**: See `docs/architecture/overview.md`

---

## ⚡ Pro Tips

- **Start conservative** with budgets, relax them as you understand usage
- **Monitor sentiment trends** weekly - adjust frustration threshold if needed
- **Use task-type budgets** for expensive operations (architecture, optimization)
- **Set hard limits** in production, soft limits in development
- **Review optimization suggestions** - free tokens by switching to cheaper models

---

## 🆘 Support

**Configuration issues**?
```bash
escalation-manager config
# Shows your current configuration
```

**Service not starting?**
```bash
escalation-manager service --port 9000 2>&1 | head -20
# Shows startup errors
```

**Want to reset everything?**
```bash
rm ~/.claude/escalation/config.yaml
rm ~/.claude/data/escalation/escalation.db
# Restart - will recreate with defaults
```

**Questions?**
- Check the documentation: `docs/` folder
- Review examples in this guide
- See completion docs: `PHASE_*_COMPLETION.md`

---

**Ready to optimize your Claude usage? Start with `escalation-manager set-budget`! 🚀**
