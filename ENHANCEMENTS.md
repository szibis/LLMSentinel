# Advanced Enhancements (Phase 2+)

This directory contains optional enhancements for the escalation system.

## 1. Enhanced Stats Tracking

**File:** `tools/stats-tracking/escalation-stats-enhanced.sh`

Tracks detailed session history with token estimation and cost analysis.

### Features
- Session-by-session logging
- Token cost estimation (Opus=500t, Sonnet=200t, Haiku=50t)
- Savings calculation (vs using Opus for all tasks)
- Session history in JSON Lines format
- Cost analysis and breakdown

### Usage
```bash
# Enable enhanced tracking
chmod +x ~/.claude/bin/escalation-stats-enhanced
ln -s ~/.claude/bin/escalation-stats-enhanced /path/to/enhanced

# Log a session event
escalation-stats-enhanced log escalate claude-opus-4-6 claude-sonnet-4-6 debugging

# Get session history (last 20)
escalation-stats-enhanced history 20

# Get cost analysis
escalation-stats-enhanced cost

# Get summary stats
escalation-stats-enhanced summary
```

### Data Location
Sessions logged to: `~/.claude/data/escalation/sessions.jsonl`

Each line is a JSON object with:
```json
{
  "timestamp": "2026-04-25 16:30:00",
  "epoch": 1777127400,
  "type": "escalate|deescalate|success",
  "from": "claude-sonnet-4-6",
  "to": "claude-haiku-4-5-20251001",
  "task": "debugging",
  "tokens": 50,
  "savings": 450
}
```

## 2. Enhanced Web Dashboard

**File:** `tools/dashboard-enhanced/dashboard-enhanced.sh`

Advanced dashboard with cost analysis, session history, and savings tracking.

### Features
- Real-time cost savings display (total tokens saved)
- Session history with detailed breakdown
- Model distribution charts
- Savings vs potential waste analysis
- Per-session token tracking
- Cost per session metrics

### Usage
```bash
chmod +x ~/.claude/bin/dashboard-enhanced
~/.claude/bin/dashboard-enhanced 8077
# Open: http://localhost:8077
```

### Displays
- **💰 Total Tokens Saved** vs using Opus for all
- **📈 Savings Percentage** of costs
- **✅ Success Rate** of problem resolution
- **📊 Model Distribution** with breakdown
- **💡 Cost Breakdown** actual vs estimated
- **📋 Session History** last 10 sessions

### API Endpoint
```bash
curl http://localhost:8077/api/dashboard | jq .
```

Returns:
```json
{
  "currentState": {...},
  "stats": {...},
  "costAnalysis": {
    "actualTokensCost": 5000,
    "estimatedWithoutEscalation": 10000,
    "costSaved": 5000,
    "costSavedPercent": 50,
    "avgCostPerSession": 312
  },
  "sessionHistory": [...],
  "metrics": {...}
}
```

## 3. Barista Statusline Module

**File:** `tools/barista-modules/escalation-status.sh`

Real-time display of current model and effort in Claude Code's statusline.

### Features
- Shows active model (Haiku/Sonnet/Opus)
- Displays effort level (low/medium/high)
- Shows cost multiplier (1x/8x/30x)
- Displays session duration if in escalation context
- Color-coded by model

### Installation
See `BARISTA_INTEGRATION.md` for detailed setup instructions.

### Output Examples
```
🚀 Escalation: Haiku(1x) • Effort: low ⚡
🚀 Escalation: Opus(30x) • Effort: high 🔥 ⏱ 15m
```

## Integration Notes

### When to Use Each

**Use Dashboard-Enhanced when:**
- You want to see cost savings over time
- You need detailed session history
- You want to understand token usage patterns
- You're analyzing cost-benefit of escalations

**Use Stats-Enhanced when:**
- You're building custom analytics
- You need raw session data in JSON format
- You're integrating with external tools
- You want programmatic access to metrics

**Use Barista Module when:**
- You want quick at-a-glance status
- You need real-time model display
- You want effort level visible always
- You're in terminal-heavy workflows

## Enabling All Three

```bash
# 1. Install enhanced stats
cp tools/stats-tracking/escalation-stats-enhanced.sh ~/.claude/bin/
chmod +x ~/.claude/bin/escalation-stats-enhanced

# 2. Install enhanced dashboard
cp tools/dashboard-enhanced/dashboard-enhanced.sh ~/.claude/bin/
chmod +x ~/.claude/bin/dashboard-enhanced

# 3. Install barista module
cp tools/barista-modules/escalation-status.sh ~/.claude/barista/modules/

# 4. Update barista config
cp .barista.conf.example ~/.claude/barista/barista.conf
# Edit to enable MODULE_ESCALATION_STATUS="true"

# 5. Start dashboard
~/.claude/bin/dashboard-enhanced 8077 &

# 6. Restart Claude Code
```

## Future Improvements

- [ ] Historical trending (24h, 7d, 30d)
- [ ] Per-task-type cost analysis
- [ ] Predictive escalation suggestions
- [ ] Cost alerts and thresholds
- [ ] Export to CSV for analysis
- [ ] Integration with cost tracking systems

