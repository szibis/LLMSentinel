# Configuration Examples

This directory contains example configurations for different use cases.

## Quick Setup

Choose your use case and copy the example config:

```bash
# Production setup (tight budget, high safety)
cp examples/config-production.yaml ~/.claude/escalation/config.yaml

# Development setup (generous budget, full features)
cp examples/config-development.yaml ~/.claude/escalation/config.yaml

# Research setup (maximum capabilities, high spend)
cp examples/config-research.yaml ~/.claude/escalation/config.yaml
```

Then start the service:

```bash
escalation-manager service --port 9000
```

## Config Comparison

### Budget Limits

| Scenario | Daily | Monthly | Opus/Day | Sentiment | Hard Limit |
|----------|-------|---------|----------|-----------|---|
| **Production** | $5 | $50 | $2 | Yes | ✅ Strict |
| **Development** | $50 | $500 | $30 | Yes | ⚠️ Warn only |
| **Research** | $100 | $1000 | $80 | Yes | ⚠️ Warn only |

### Model Preferences

| Scenario | Haiku | Sonnet | Opus | Strategy |
|----------|-------|--------|------|----------|
| **Production** | ✅ Preferred | Limited | Rare | Cost-first |
| **Development** | Good | Balanced | Liberal | Balanced |
| **Research** | Minimal | Medium | Liberal | Capability-first |

### Sentiment & Escalation

All scenarios have sentiment detection enabled, but frustration thresholds vary:

| Scenario | Frustration Threshold | Auto-Escalate | Max Attempts |
|----------|-----|---|---|
| **Production** | 0.70 (high) | Yes | 2 |
| **Development** | 0.65 (moderate) | Yes | 3 |
| **Research** | 0.75 (high) | Yes | 1 |

### Learning & Analytics

All scenarios have learning enabled to improve routing over time.

## Per-Task-Type Budgets

### Production (Conservative)
- Concurrency: 3000 tokens (complex, needs power)
- Parsing: 1000 tokens (simple extraction)
- Debugging: 2000 tokens (moderate)
- Architecture: 2500 tokens (design work)

### Development (Generous)
- Concurrency: 15000 tokens
- Parsing: 10000 tokens
- Debugging: 15000 tokens
- Architecture: 20000 tokens

### Research (Maximum)
- Concurrency: 20000 tokens
- Parsing: 15000 tokens
- Debugging: 20000 tokens
- Architecture: 50000 tokens (deep design work)

## Statusline Integration

### Production
- Primary: Barista (actual token metrics)
- Fallback: Claude native statusline
- Tertiary: Environment variables
- Timeout: 2 seconds

### Development
- Primary: Environment variables (flexible)
- No external integrations

### Research
- Primary: Barista (accurate metrics)
- Fallback: Webhook (custom integration)
- Tertiary: Environment variables

## Customization

Copy an example config and customize for your needs:

```bash
cp examples/config-production.yaml ~/.claude/escalation/config.yaml
# Edit config file
nano ~/.claude/escalation/config.yaml
```

Then reload the service:

```bash
# Restart to pick up config changes
pkill escalation-manager
escalation-manager service --port 9000
```

## Environment Variables

You can also set budget via CLI:

```bash
# Set daily budget
escalation-manager set-budget --daily 10.00

# Set monthly budget
escalation-manager set-budget --monthly 100.00

# Or both
escalation-manager set-budget --daily 10.00 --monthly 100.00
```

This creates/updates `~/.claude/escalation/config.yaml`.

## Monitoring Your Config

After setup, verify it's working:

```bash
# View current config
escalation-manager config

# View dashboard
escalation-manager dashboard --budget

# Check logs
tail -f ~/.claude/data/escalation/escalation.log
```

## See Also

- [Budget Configuration Guide](../docs/integration/budgets.md) — Detailed explanation
- [Sentiment Detection Guide](../docs/integration/sentiment-detection.md) — How detection works
- [Deployment Guide](../docs/operations/deployment.md) — Production setup
