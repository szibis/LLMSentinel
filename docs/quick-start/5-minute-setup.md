# 5-Minute Setup Guide

Get Claude Escalate running in under 5 minutes.

## Prerequisites
- Claude Code installed and configured
- Go 1.21+ (if building from source)
- Basic terminal knowledge

## Step 1: Get the Binary

### Option A: Download Prebuilt (Fastest)
```bash
# Downloads to ~/.local/bin/escalation-manager
curl -L https://github.com/szibis/claude-escalate/releases/latest/download/escalation-manager-darwin-arm64 \
  -o ~/.local/bin/escalation-manager
chmod +x ~/.local/bin/escalation-manager
```

### Option B: Build from Source
```bash
git clone https://github.com/szibis/claude-escalate.git
cd claude-escalate
go build -o escalation-manager ./cmd/claude-escalate
mv escalation-manager ~/.local/bin/
```

## Step 2: Create Hook (3 Lines)
```bash
cat > ~/.claude/hooks/http-hook.sh << 'EOF'
#!/bin/bash
read -r PROMPT
curl -s -X POST http://localhost:9000/api/hook -d "{\"prompt\":\"$PROMPT\"}"
EOF
chmod +x ~/.claude/hooks/http-hook.sh
```

## Step 3: Configure Hook in Settings
Edit `~/.claude/settings.json`:
```json
{
  "hooks": {
    "UserPromptSubmit": [
      {
        "type": "command",
        "command": "~/.claude/hooks/http-hook.sh",
        "timeout": 5
      }
    ]
  }
}
```

## Step 4: Start Service
```bash
# Start in background
escalation-manager service --port 9000 &

# Or in a separate terminal
escalation-manager service --port 9000
```

Service is now listening on `http://localhost:9000`

## Step 5: Verify Installation
```bash
# Should show help
escalation-manager --help

# Should show version
escalation-manager version

# Check service is running
curl http://localhost:9000/api/health
```

## You're Done! 🎉

The system is now active:
- ✅ Hook integrated with Claude Code
- ✅ Service running and ready
- ✅ Ready to detect model escalation needs
- ✅ Dashboard available at http://localhost:9000

## Next Steps

1. **Try an Escalation**: Ask Claude something complex, then say it's not working. System will suggest escalation.
2. **Enable Sentiment Detection** (optional):
   ```bash
   escalation-manager config set sentiment.enabled true
   ```
3. **Set Budget Limits** (optional):
   ```bash
   escalation-manager set-budget --daily 10.00 --monthly 100.00
   ```
4. **Open Dashboard** (optional):
   ```bash
   open http://localhost:9000
   ```

## Troubleshooting

**Hook not firing?**
- Check hook file exists: `ls ~/.claude/hooks/http-hook.sh`
- Check settings.json is valid JSON: `cat ~/.claude/settings.json | jq .`
- Verify service is running: `curl http://localhost:9000/api/health`

**Service won't start?**
- Port 9000 already in use? Try: `escalation-manager service --port 9001`
- Check logs: `tail -f ~/.claude/data/escalation/escalation.log`

**Curl fails?**
- Service running? Check: `lsof -i :9000`
- Network issues? Try: `curl -v http://localhost:9000/api/health`

See [Troubleshooting](../operations/troubleshooting.md) for more help.
