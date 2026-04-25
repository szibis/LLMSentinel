# Barista Statusline Integration

The escalation system integrates with Barista (Claude Code's statusline) to display real-time model and effort information.

## Real-Time Display

The `escalation-status` barista module shows:
- **🚀 Escalation:** Current model (Haiku/Sonnet/Opus)
- **Cost:** 1x / 8x / 30x multiplier
- **⚡ Effort:** low/medium/high
- **⏱:** Session duration if active (e.g., "5m" if in escalation context)

### Example Output
```
🚀 Escalation: Haiku(1x) • Effort: low ⚡
🚀 Escalation: Opus(30x) • Effort: high 🔥 ⏱ 12m
```

## Installation

1. **Copy module to barista:**
```bash
cp tools/barista-modules/escalation-status.sh ~/.claude/barista/modules/
```

2. **Update barista.conf:**
```bash
# Set the new module
MODULE_ESCALATION_STATUS="true"

# Disable old modules (optional)
MODULE_AUTO_EFFORT="false"
MODULE_MODEL_ROUTING="false"

# Add to MODULE_ORDER
MODULE_ORDER="...,escalation-status,..."
```

3. **Apply config:**
```bash
cp .barista.conf.example ~/.claude/barista/barista.conf
```

4. **Restart Claude Code** to load the new statusline

## Configuration

In `barista.conf`:
```bash
# Icon to display (default: 🚀)
ESCALATION_ICON="🚀"

# Compact mode (true/false, default: false)
ESCALATION_COMPACT="false"

# Include in statusline
MODULE_ESCALATION_STATUS="true"
```

## How It Works

The module:
1. Calls `escalation-manager stats` to get current state
2. Falls back to reading `~/.claude/settings.json` if binary unavailable
3. Checks for active session in `/tmp/.escalation_$(id -u)/`
4. Displays model, cost, effort, and session duration
5. Updates every barista refresh cycle (usually instant)

## Troubleshooting

**Module not showing:**
- Verify `MODULE_ESCALATION_STATUS="true"` in barista.conf
- Check module is in `MODULE_ORDER`
- Restart Claude Code

**Wrong model displayed:**
- Verify `~/.claude/settings.json` has correct model
- Check `escalation-manager stats` works: `~/.claude/bin/escalation-manager stats | jq .currentState`

**Always shows "Sonnet":**
- Binary may not be found
- Check path: `which escalation-manager` or `ls ~/.claude/bin/escalation-manager`

