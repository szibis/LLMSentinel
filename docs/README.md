# Claude Escalate Documentation

**Complete guide to intelligent model escalation, sentiment-aware routing, token budgeting, and analytics for Claude Code.**

---

## 🚀 Quick Start

New to Claude Escalate? Start here:

- **[5-Minute Setup](quick-start/5-minute-setup.md)** — Install and configure in under 5 minutes
- **[First Escalation](quick-start/first-escalation.md)** — Try your first model escalation
- **[Token Budgets Guide](quick-start/budgets-setup.md)** — Set up spending limits and protections

---

## 🏗️ Architecture

Understand how the system works:

- **[System Overview](architecture/overview.md)** — High-level architecture and components
- **[3-Phase Flow](architecture/3-phase-flow.md)** — Complete validation workflow with analytics
- **[Signal Detection](architecture/signal-detection.md)** — How frustration, success, and confusion are detected
- **[Token Validation](architecture/token-validation.md)** — Estimation accuracy and learning
- **[Sentiment Detection](architecture/sentiment-detection.md)** — Frustration minimization and user sentiment analysis

---

## 🔌 Integration

Connect with your Claude environment:

- **[Barista Statusline](integration/barista-statusline.md)** — Display escalation metrics in your statusline
- **[Sentiment Detection](integration/sentiment-detection.md)** — Configure frustration detection and anti-frustration escalation
- **[Token Budgets](integration/budgets.md)** — Set daily/monthly/per-model spending limits
- **[API Reference](integration/api-reference.md)** — Complete endpoint documentation (20+ endpoints)

---

## 🛠️ Operations

Deploy, monitor, and maintain:

- **[Deployment Guide](operations/deployment.md)** — Production setup and configuration
- **[Monitoring](operations/monitoring.md)** — Health checks, logging, and alerting
- **[Troubleshooting](operations/troubleshooting.md)** — Common issues and solutions

---

## 📊 Analytics

Understand your usage and optimize:

- **[Dashboards](analytics/dashboards.md)** — Web and CLI analytics dashboards
- **[Cost Analysis](analytics/cost-analysis.md)** — Token spend, model usage, and optimization tips
- **[Recommendations](analytics/recommendations.md)** — Data-driven suggestions for cost and frustration reduction

---

## Key Features

### Intelligent Model Escalation
- **Automatic detection** of frustration, confusion, and impatience
- **Predictive routing** based on task type and historical success rates
- **Auto-downgrade** to cheaper models after solving problems

### Sentiment-Aware System
- Detects frustrated, confused, and impatient users
- Escalates automatically to prevent frustration
- Learns which models work best for different user sentiments
- Minimizes user frustration while protecting token budget

### Token Budget Protection
- **Hierarchical budgets**: Daily, monthly, per-model, per-task-type
- **Hard and soft limits**: Stop or warn when approaching budget
- **Auto-downgrade**: Intelligently switch to cheaper models when near limits
- **Real-time tracking**: Monitor spending during response generation

### Multi-Source Statusline
- **Barista** integration (default)
- **Claude native** statusline (coming)
- **Webhook** endpoints (custom integrations)
- **Environment variables** (flexible)
- **File polling** (advanced)

### Complete Analytics
- **Phase 1**: Pre-response estimation and routing decisions
- **Phase 2**: Real-time token tracking during generation
- **Phase 3**: Post-response validation and learning
- **APIs**: Full analytics endpoints for every phase

---

## System Flows

### Basic: Frustration Detection & Auto-Escalation
```
User types prompt
  ↓
System detects frustration signals (repeated attempts, "still broken", etc.)
  ↓
System escalates model automatically (Haiku → Sonnet → Opus)
  ↓
Problem solved
  ↓
System learns: this task type needs higher model
```

### Budget Protection: Approaching Limit
```
User starts session with $10/day budget
  ↓
After 3 requests, used $7.50 (75% of budget)
  ↓
System shows warning: "75% of daily budget used"
  ↓
Next request routes to cheaper model (Haiku) instead of Opus
  ↓
Saves money while staying within limits
```

### Learning: Sentiment → Success Correlation
```
Record: (task_type="concurrency", model="haiku", sentiment="frustrated", success=false)
Record: (task_type="concurrency", model="sonnet", sentiment="satisfied", success=true)
Record: (task_type="concurrency", model="opus", sentiment="satisfied", success=true)
  ↓
Learn: concurrency on Haiku has 40% success, Sonnet 80%, Opus 98%
  ↓
Next concurrency task: recommend Sonnet first (good balance of cost/success)
```

---

## Command Reference

### Main Service
```bash
# Start the HTTP service (listens on :9000)
escalation-manager service --port 9000

# Start token metrics monitor
escalation-manager monitor --port 9000 --method env

# Report actual token metrics
escalation-manager report-metrics --validation-id 42 --input-tokens 268 --output-tokens 474
```

### Queries & Analytics
```bash
# View overall statistics
escalation-manager validation stats

# List validation records
escalation-manager validation metrics --limit 100

# Compare estimate vs actual
escalation-manager validation compare --validation-id 42

# Show satisfaction rates by model
escalation-manager dashboard --sentiment
```

### Configuration
```bash
# Set token budgets
escalation-manager set-budget --daily 10.00 --monthly 100.00

# Configure statusline sources
escalation-manager config set statusline.sources.barista.enabled true

# View current configuration
escalation-manager config show
```

---

## Common Scenarios

### Scenario 1: "I get frustrated when code keeps failing"
→ Enable sentiment detection (default: enabled)  
→ System detects frustration keywords and escalates model automatically  
→ See: [Sentiment Detection Guide](architecture/sentiment-detection.md)

### Scenario 2: "I want to spend max $10/day on Claude"
→ Set daily budget: `escalation-manager set-budget --daily 10.00`  
→ System tracks spending and auto-downgrades to cheaper models when needed  
→ See: [Token Budgets Guide](quick-start/budgets-setup.md)

### Scenario 3: "I want to see what models work best for different tasks"
→ Open the web dashboard: `escalation-manager dashboard`  
→ Check "Model Satisfaction" tab to see success rates  
→ Use recommendations to optimize your workflow  
→ See: [Analytics Dashboards](analytics/dashboards.md)

### Scenario 4: "I'm using Barista and want real-time metrics"
→ Configure Barista integration (default)  
→ Statusline shows Phase 1 (estimate), Phase 2 (in-progress), Phase 3 (final)  
→ See: [Barista Integration](integration/barista-statusline.md)

---

## Architecture at a Glance

```
Claude Code
    ↓ (UserPromptSubmit hook)
HTTP Hook (3 lines)
    ↓ (HTTP POST)
Escalation Service (:9000)
    ├─ Phase 1: Analyze & Estimate
    ├─ Phase 2: Real-time Monitoring
    ├─ Phase 3: Validation & Learning
    └─ Dashboard: Analytics & Metrics
    ↓
SQLite Database (validation.db)
    └─ Metrics, learning, statistics
```

---

## Support & Contribution

- **Issue Tracker**: [GitHub Issues](https://github.com/szibis/claude-escalate/issues)
- **Discussions**: [GitHub Discussions](https://github.com/szibis/claude-escalate/discussions)
- **Contributing**: See [CONTRIBUTING.md](../CONTRIBUTING.md)

---

## License

MIT License. See [LICENSE](../LICENSE) file.
