# Local Testing Guide

**Test escalation system locally with your Claude Code instance**

## Quick Setup (5 min)

### 1. Start Docker Service

```bash
cd /tmp/claude-escalate/docker-compose
docker-compose up -d

# Verify it's running
curl http://localhost:8077
# Should show HTML dashboard
```

### 2. Install Local Binary

```bash
# Option A: Use the Go binary (pre-built, faster)
mkdir -p ~/.local/bin
cp /tmp/claude-escalate/claude-escalate ~/.local/bin/escalation-manager
chmod +x ~/.local/bin/escalation-manager

# Verify
~/.local/bin/escalation-manager version
# Should show: claude-escalate 2.0.0
```

### 3. Configure Claude Code Hook

Edit `~/.claude/settings.json`:

```json
{
  "hooks": {
    "UserPromptSubmit": [
      {
        "type": "command",
        "command": "/Users/slawomirskowron/.claude/hooks/de-escalate-model.sh",
        "timeout": 3,
        "description": "Escalation/de-escalation handler"
      }
    ]
  }
}
```

Or use the install command:

```bash
~/.local/bin/escalation-manager install-hook
# This automatically configures ~/.claude/settings.json
```

### 4. Verify Hook is Installed

```bash
jq '.hooks.UserPromptSubmit' ~/.claude/settings.json
# Should show the hook entry
```

## Testing Scenarios

### Scenario 1: Manual Escalation

1. Start a new Claude Code session
2. Type: `/escalate to opus`
3. Observe:
   - Settings.json model changes to `claude-opus-4-7`
   - Dashboard shows "Opus" model
   - Cost multiplier shows "30x"

```bash
# Verify manually
jq '.model' ~/.claude/settings.json
# Should show: "claude-opus-4-7"

# Check stats
curl http://localhost:8077/api/stats 2>/dev/null | jq '.totalEscalations'
# Should increment
```

### Scenario 2: De-escalation via Success Signal

1. In current session, type: `Perfect! That works great.`
2. Observe:
   - Hook detects success phrase
   - De-escalation triggers (Opus → Sonnet)
   - Dashboard model downgrade animates
   - After 5+ min and another success: Sonnet → Haiku

```bash
# Watch dashboard for real-time updates
open http://localhost:8077

# Or poll stats
watch -n 1 'curl -s http://localhost:8077/api/stats | jq .totalDeescalations'
```

### Scenario 3: Auto-Effort Routing

Create a new session with different prompts:

**Simple task (should route to Haiku):**
```
What's the capital of France?
```

**Medium complexity (should route to Sonnet):**
```
Refactor this function to use promises instead of callbacks
```

**Complex task (should route to Opus):**
```
Design a distributed cache invalidation system with conflict resolution
```

Observe dashboard effort level changing automatically.

### Scenario 4: Cascade Timeout Prevention

1. Escalate to Opus: `/escalate to opus`
2. Send success signal: `This is perfect!`
3. System cascades: Opus → Sonnet
4. Send another success signal immediately: `Thanks!`
5. Expected: **No second cascade** (within 5-minute window)

Verify in logs:
```bash
docker logs claude-escalation-service | grep -i cascade
# Should see cascade timestamp and timeout message
```

### Scenario 5: Session Lifecycle

1. Start new session, escalate to Opus
2. Let session run for 30 minutes
3. Expected: Session automatically cleared, back to Haiku for next task

Check session file:
```bash
ls -la ~/.claude/data/escalation/
# Session files created
# After 30 min: Session files cleaned up
```

## Dashboard Monitoring

Access: **http://localhost:8077**

Watch these metrics:

| Metric | What to Watch | Normal Range |
|--------|---------------|--------------|
| **Current Model** | Changes on `/escalate` commands | Haiku → Sonnet → Opus |
| **Effort Level** | Auto-updates for task complexity | low/medium/high |
| **Escalations** | Counts `/escalate` commands | 0-50+ |
| **De-escalations** | Counts success signals | 0-30+ |
| **Cascade Success Rate** | % of cascades completing | 80-100% |
| **Tokens Saved** | vs using Opus for all tasks | 40-80% |

## Real-Time Testing

### Watch Dashboard Live

```bash
# Terminal 1: Watch dashboard
open http://localhost:8077

# Terminal 2: Send test commands
for i in {1..5}; do
  echo "Test $i: $(date)"
  sleep 2
done

# Dashboard updates every 2 seconds
```

### Test Cascade Chain

```bash
# Terminal 1: Watch logs
docker logs -f claude-escalation-service

# Terminal 2: Trigger escalations
echo "/escalate to opus" | xclip
# Paste into Claude Code

echo "Perfect! Works great." | xclip
# Paste 1+ min later
# Watch dashboard cascade through Sonnet → Haiku
```

## Troubleshooting

### Docker service not responding

```bash
docker-compose ps
# Should show: Up

docker logs claude-escalation-service | tail -20
# Check for errors

curl http://localhost:8077
# Should return HTML
```

### Hook not running

```bash
# Check hook is in settings
jq '.hooks.UserPromptSubmit' ~/.claude/settings.json

# Test hook manually
~/.local/bin/escalation-manager --help
# Should show version info

# Check permissions
ls -la ~/.local/bin/escalation-manager
# Should show: -rwxr-xr-x
```

### Model not changing

```bash
# Check settings.json
jq '.model' ~/.claude/settings.json

# Verify hook is being called
# Add debug to hook script:
echo "Hook called at $(date)" >> /tmp/escalation-debug.log

# Check for errors
docker logs claude-escalation-service | grep -i error
```

### Dashboard shows no stats

```bash
# Verify stats file exists
ls -la ~/.claude/data/escalation/sessions.jsonl

# Check file contents
cat ~/.claude/data/escalation/sessions.jsonl | head

# Manually log a session
~/.local/bin/escalation-manager stats summary
```

## Stopping Services

### Clean Shutdown

```bash
# Stop Docker service
cd /tmp/claude-escalate/docker-compose
docker-compose down

# Remove hook from settings (optional)
jq 'del(.hooks.UserPromptSubmit[0])' ~/.claude/settings.json > /tmp/settings.json
mv /tmp/settings.json ~/.claude/settings.json
```

### Preserve Data

Data persists in `/tmp/claude-escalate/docker-compose/escalation-data` volume.

To keep data while stopping:
```bash
docker-compose down
# Data remains in Docker volume

# To access:
docker run -v docker-compose_escalation-data:/data alpine ls -la /data
```

## Next Steps After Testing

1. **If everything works:**
   - Push to production: Move binary to `/usr/local/bin`
   - Update dashboard port if needed: Edit `docker-compose.yml`
   - Set up persistent monitoring

2. **If you find issues:**
   - Document the issue
   - Check logs: `docker logs -f claude-escalation-service`
   - Try fallback: Use local binary without Docker

3. **For team deployment:**
   - Use DOCKER_SERVICE.md for HTTP API setup
   - Configure hooks to hit Docker service
   - Centralize metrics in shared dashboard

## Performance Notes

- Dashboard refresh: 2 seconds
- Hook timeout: 3 seconds (automatic fallback if longer)
- Model switch latency: <500ms
- Memory usage: 25-50MB per session
- CPU: <1% idle, <10% during processing
