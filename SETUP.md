# Installation & Setup Guide

## Quick Start

### 1. Clone the Repository
```bash
git clone https://github.com/szibis/claude-escalate.git
cd claude-escalate
```

### 2. Install the Binary

#### Option A: Use Pre-built Binary (Recommended)
```bash
./scripts/install.sh
```

This will:
- Download the latest compiled binary for your OS/architecture
- Place it in `~/.local/bin/claude-escalate`
- Verify installation

#### Option B: Use Bash Version (Lightweight)
```bash
chmod +x scripts/bash/escalation-manager
cp scripts/bash/escalation-manager ~/.claude/bin/escalation-manager

# Optional: Copy dashboard tools
cp scripts/bash/dashboard ~/.claude/bin/escalation-dashboard
```

### 3. Configure Claude Code

Edit your `~/.claude/settings.json` and add the hook:

```json
{
  "hooks": {
    "UserPromptSubmit": [
      {
        "type": "command",
        "command": "~/.claude/bin/escalation-manager",
        "timeout": 5,
        "continueOnFailure": true
      }
    ]
  }
}
```

### 4. (Optional) Start the Dashboard

View real-time escalation metrics:

```bash
# Start web dashboard on port 8077
~/.claude/bin/escalation-dashboard 8077

# Open in browser
open http://localhost:8077
```

## System Requirements

- **Bash**: 4.0+ (for bash version)
- **Python**: 3.8+ (for dashboard)
- **Claude Code**: Latest version with hook support
- **OS**: macOS, Linux, or WSL
- **Disk Space**: ~50MB for logs (auto-rotates)

## File Structure After Installation

```
~/.claude/
├── bin/
│   ├── escalation-manager      # Main binary (consolidated)
│   └── escalation-dashboard    # Web dashboard server
├── data/escalation/
│   ├── escalations.log         # Escalation events
│   ├── deescalations.log       # De-escalation events
│   └── last_task_context       # Last task type
├── settings.json               # Claude Code settings (with hooks)
└── hooks/                      # (Legacy - kept for reference)

/tmp/.escalation_$(id -u)/
├── escalation_session          # Current escalation session
├── last_cascade_time           # Cascade timeout tracker
└── deescalation_just_ran       # De-escalation flag
```

## Environment Variables

None required. System uses:
- `$HOME/.claude/` — configuration directory
- `$HOME/.claude/settings.json` — model and effort settings
- `/tmp/.escalation_$(id -u)/` — temporary session state

## Verification

Test the installation:

```bash
# Check binary works
~/.claude/bin/escalation-manager stats | jq .

# Check settings
jq '.model, .effortLevel' ~/.claude/settings.json

# Test escalation command (simulated)
echo '{"prompt": "/escalate to opus"}' | \
  HOOK_TYPE=on-prompt ~/.claude/bin/escalation-manager
```

Expected output: JSON response with escalation acknowledgment

## Troubleshooting Installation

### Binary not found
```bash
which escalation-manager  # Should show ~/.local/bin/escalation-manager
# If not found, add to PATH:
export PATH="$PATH:$HOME/.local/bin"
```

### Permission denied
```bash
chmod +x ~/.claude/bin/escalation-manager
```

### Settings.json not found
```bash
mkdir -p ~/.claude
touch ~/.claude/settings.json
jq '. += {"model": "claude-sonnet-4-6", "effortLevel": "medium"}' \
  ~/.claude/settings.json > /tmp/settings.tmp && \
  mv /tmp/settings.tmp ~/.claude/settings.json
```

### Dashboard won't start
```bash
# Port already in use?
lsof -i :8077
# Try different port:
~/.claude/bin/escalation-dashboard 8888
```

## Next Steps

1. **Read the USAGE guide** to understand how escalation/de-escalation works
2. **View the dashboard** to monitor your sessions
3. **Check ARCHITECTURE.md** for technical details
4. **See TROUBLESHOOTING.md** for common issues

## Support

For issues or questions:
1. Check [TROUBLESHOOTING.md](TROUBLESHOOTING.md)
2. Open an issue on GitHub
3. Check logs: `~/.claude/data/escalation/*.log`

