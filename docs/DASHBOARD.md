# Dashboard Guide

## Overview

The Escalation Dashboard provides real-time monitoring of your Claude Code usage patterns, model selections, and cost optimization.

**Access**: `http://localhost:8077`

## Starting the Dashboard

```bash
# Default port 8077
~/.claude/bin/escalation-dashboard

# Custom port
~/.claude/bin/escalation-dashboard 8888

# Background (with output redirection)
~/.claude/bin/escalation-dashboard 8077 > /tmp/dashboard.log 2>&1 &
```

Once started:
1. Open browser: `http://localhost:8077`
2. Dashboard auto-refreshes every 2 seconds
3. Click sun (☀️) / moon (🌙) icon to toggle light/dark mode
4. Theme preference saved in browser storage

## Dashboard Metrics

### Current State Section

**📊 Current Model**
- Shows active model: Haiku / Sonnet / Opus
- Color-coded: Green (Haiku) / Orange (Sonnet) / Red (Opus)
- Cost multiplier: 1x / 8x / 30x
- Updates in real-time from settings.json

**🎯 Effort Level**
- Current effort setting: LOW / MEDIUM / HIGH
- Drives auto-effort routing
- Updated by auto-effort or manual `/effort` command

### Key Metrics Section

**📈 Total Escalations**
- Count of `/escalate` commands executed
- Each escalation creates a new session
- Session lasts 30 minutes or until final cascade

**⬇️ Total De-escalations**
- Count of cascade steps (Opus→Sonnet, Sonnet→Haiku)
- Higher = more successful problem resolution
- Indicates cost savings

**📉 Cascade Rate**
- Percentage: (de-escalations / escalations) × 100
- High rate = problems frequently solved (good!)
- Low rate = problems need multiple attempts (use escalation)

**✅ Success Rate**
- Percentage of successful de-escalations
- Based on success signal detection
- High = good problem-solving efficiency

**🔄 Average Cascade Depth**
- Steps per escalation: de-escalations / escalations
- Shows typical cascade path length
- Higher = more thorough cascade before settling

**💰 Token Cost**
- Estimated token consumption
- Calculated: de-escalations × ~200 tokens
- Rough estimate for cost analysis

### Model Distribution (Bar Chart)

**🎛️ Model Usage**
```
Opus (Premium):  ■■░░░░░░░░ 29% (26/89)
Sonnet (Standard): ■■░░░░░░░░ 21% (19/89)
Haiku (Budget):  ■■■■■░░░░░ 50% (45/89)
```

Shows percentage and count for each model used in your session.

### API Endpoint

```bash
curl http://localhost:8077/api/dashboard | jq .
```

**Response Format:**
```json
{
  "timestamp": "2026-04-25T16:24:56.000000",
  "currentState": {
    "model": "Opus",
    "fullModel": "claude-opus-4-6",
    "effort": "HIGH",
    "modelColor": "#FF6B6B",
    "modelCost": "30x"
  },
  "stats": {
    "escalations": 6,
    "deescalations": 51,
    "cascadeCompletion": 850,
    "successRate": 85,
    "avgCascadeDepth": 8
  },
  "modelUsage": {
    "opus": 26,
    "sonnet": 19,
    "haiku": 45
  },
  "costAnalysis": {
    "tokenCost": 10200,
    "costPerEsc": 1700
  }
}
```

## Interpreting the Data

### Healthy Metrics

✅ **Haiku 50%+ of usage**
- Cost-optimized routing working
- Auto-effort correctly identifying simple tasks
- Expected for typical mixed workload

✅ **Cascade Rate 100-500%**
- Problems often solved (good de-escalation)
- Multiple success signals trigger cascades
- Shows cost optimization in action

✅ **Success Rate 80%+**
- High problem-solving efficiency
- Escalations are usually effective
- Cost savings validated by outcomes

### Warning Signs

⚠️ **Haiku < 30% of usage**
- May be over-escalating
- Auto-effort might be too aggressive
- Consider: `/effort low` for simple tasks

⚠️ **Opus > 40% of usage**
- Using expensive model too often
- Check: are tasks really that complex?
- Monitor: are they getting solved?

⚠️ **Success Rate < 50%**
- Problems not being resolved effectively
- Multiple escalations may be needed
- Check: use `/escalate to opus` earlier

⚠️ **Cascade Rate near 0%**
- No de-escalations happening
- Sessions not cascading down
- Verify: success phrases are being detected

## Real-Time Monitoring

### Live Indicators
- Green pulse (● LIVE) = system is active
- Timestamp updates every 2 seconds
- Model color changes instantly when model switches

### Refresh Behavior
- Auto-refresh: every 2 seconds
- Manual refresh: Cmd+R (Ctrl+R on Linux)
- Data source: `~/.claude/bin/escalation-manager stats`
- Fallback: reads from `~/.claude/data/escalation/` logs

## Theme Modes

### Light Mode (Default)
- ☀️ White background, dark text
- Easy on eyes during day
- High contrast for readability

### Dark Mode
- 🌙 Dark background (#1a1a2e), light text
- Easier on eyes in low light
- Reduced blue light

Toggle anytime by clicking sun/moon icon in header.
**Preference persists** in browser localStorage.

## Dashboard Logs

### Data Storage

Dashboard reads live data from:
```
~/.claude/data/escalation/
├── escalations.log      # One line per escalation
├── deescalations.log    # One line per de-escalation
└── last_task_context    # Most recent task type
```

Format example:
```
[2026-04-25 16:24:13] Escalated to Opus (deep reasoning & complex logic) (task: debugging)
[2026-04-25 16:24:56] De-escalated to Sonnet (balanced) (cascade complete) (problem solved, saving cost)
```

### Log Rotation

Automatic rotation keeps latest 200 entries per log:
- When log exceeds 200 lines
- Old entries automatically trimmed
- Prevents disk space issues

## Advanced: Dashboard Customization

### Custom Port

```bash
# Use port 3000 instead
~/.claude/bin/escalation-dashboard 3000
# Access: http://localhost:3000
```

### Disable Auto-Refresh

Edit dashboard HTML locally:
```javascript
// Change interval from 2000ms to never
// Comment out: setInterval(updateSessions, 2000);
```

### Integrate with Monitoring

Get JSON via API for integration:

```bash
# Every 5 seconds, log to file
while true; do
  curl -s http://localhost:8077/api/dashboard >> escalation-metrics.json
  sleep 5
done
```

## Troubleshooting Dashboard

### Dashboard won't start
```bash
# Port already in use?
lsof -i :8077
# Kill process:
kill -9 <PID>
# Try different port:
~/.claude/bin/escalation-dashboard 8888
```

### Shows old/stale data
```bash
# Refresh browser (F5 or Cmd+R)
# Check if binary is running:
ps aux | grep escalation-manager
# Restart dashboard:
pkill -f escalation-dashboard
~/.claude/bin/escalation-dashboard 8077 &
```

### Missing metrics
```bash
# Verify stats command works:
~/.claude/bin/escalation-manager stats | jq .

# Check data directory:
ls -la ~/.claude/data/escalation/

# Check settings:
jq '.model, .effortLevel' ~/.claude/settings.json
```

### Theme not persisting
- Browser localStorage may be disabled
- Check browser privacy settings
- Try incognito/private mode for testing

## Performance

### Resource Usage
- **CPU**: Minimal (runs idle between refreshes)
- **Memory**: ~50-100MB for Python process
- **Disk**: ~1KB per log entry (auto-rotates)
- **Bandwidth**: API response ~1-2KB

### Load Time
- Dashboard load: ~500ms
- API response: ~100-200ms
- Auto-refresh: every 2 seconds

## See Also

- [USAGE.md](USAGE.md) — How to use escalation system
- [ARCHITECTURE.md](ARCHITECTURE.md) — System internals
- [SETUP.md](SETUP.md) — Installation guide
- [TROUBLESHOOTING.md](TROUBLESHOOTING.md) — Common issues

