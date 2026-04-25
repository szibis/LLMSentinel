# 🚀 Quick Start (5 Minutes)

**Get escalation system running locally with your Claude Code**

## Choose Your Setup

### 🔵 Option A: Local Binary (RECOMMENDED FOR TESTING)

Fastest, simplest setup. No Docker dependency.

```bash
# Step 1: Copy binary (30 seconds)
mkdir -p ~/.local/bin
cp /tmp/claude-escalate/claude-escalate ~/.local/bin/escalation-manager
chmod +x ~/.local/bin/escalation-manager

# Step 2: Verify binary works (10 seconds)
~/.local/bin/escalation-manager version
# Output: claude-escalate 2.0.0 ✅

# Step 3: Install hook into Claude Code (20 seconds)
~/.local/bin/escalation-manager install-hook
# Automatically updates ~/.claude/settings.json

# Step 4: Start dashboard in background (10 seconds)
~/.local/bin/escalation-manager dashboard --port 8077 &

# Step 5: Open dashboard (10 seconds)
open http://localhost:8077
```

**Total time: ~2 minutes**

### 🐳 Option B: Docker (For Team/Persistent Deployments)

```bash
# Step 1: Start service (30 seconds)
cd /tmp/claude-escalate/docker-compose
docker-compose up -d

# Step 2: Wait for health (30 seconds)
sleep 5
curl http://localhost:8077  # Should show HTML

# Step 3: Configure hook (20 seconds)
# Either auto-install via binary:
~/.local/bin/escalation-manager install-hook
# Or manually add to ~/.claude/settings.json (see SETUP.md)

# Step 4: Open dashboard (10 seconds)
open http://localhost:8077
```

**Total time: ~2 minutes**

---

## Test It (Pick One Scenario)

### Test 1: Manual Escalation (60 seconds)

```bash
# 1. In Claude Code, type:
/escalate to opus

# 2. Verify model changed:
jq '.model' ~/.claude/settings.json
# Should show: "claude-opus-4-7"

# 3. Check dashboard:
open http://localhost:8077
# Should show: "Model: Opus", "Cost: 30x"
```

### Test 2: Auto De-escalation (2 minutes)

```bash
# 1. Make sure you're on Opus:
/escalate to opus

# 2. Type success phrase:
Perfect! That works great.

# 3. Wait ~30 seconds for cascade:
# Opus → Sonnet

# 4. Type another success phrase:
Thanks so much!

# 5. Wait ~30 seconds:
# Sonnet → Haiku (back to cheap!)

# 6. Verify final model:
jq '.model' ~/.claude/settings.json
# Should show: "claude-haiku-4-5-20251001"

# 7. Check dashboard:
# Should show successful cascade: Opus (30x) → Sonnet (8x) → Haiku (1x)
```

### Test 3: Auto-Effort Routing (90 seconds)

Create new sessions with different prompts:

```bash
# Simple task (should route to Haiku - 1x cost)
What is 2+2?
# Check dashboard: Effort: low, Model: Haiku

# Medium task (should route to Sonnet - 8x cost)
Refactor this function to use async/await
# Check dashboard: Effort: medium, Model: Sonnet

# Complex task (should route to Opus - 30x cost)
Design a distributed cache system with consensus
# Check dashboard: Effort: high, Model: Opus
```

---

## Verify Everything Works

### Dashboard Checklist

Open http://localhost:8077 and verify:

- [ ] **Page loads** without errors
- [ ] **Current Model** section shows your model
- [ ] **Effort Level** shows (low/medium/high)
- [ ] **Cost Multiplier** shows (1x, 8x, or 30x)
- [ ] **Auto-refresh** works (page updates every 2 seconds)
- [ ] **Light/Dark mode** toggle works

### Hook Checklist

```bash
# 1. Hook is installed
grep -q escalation ~/.claude/settings.json && echo "✅ Hook installed" || echo "❌ Hook missing"

# 2. Settings.json is valid JSON
jq empty ~/.claude/settings.json && echo "✅ Valid JSON" || echo "❌ Invalid JSON"

# 3. Binary is executable
[ -x ~/.local/bin/escalation-manager ] && echo "✅ Executable" || echo "❌ Not executable"

# 4. Version works
~/.local/bin/escalation-manager version && echo "✅ Binary works"
```

### Metrics Checklist

```bash
# Check stats are being recorded
~/.local/bin/escalation-manager stats summary

# Sample output should show:
# - Total Escalations: [count]
# - Total De-escalations: [count]
# - Average Tokens Saved: [count]
```

---

## Next Steps

### ✅ Everything Works?

1. **Test real tasks** with escalations
2. **Monitor first day** of usage
3. **Adjust settings** if needed (see SETUP.md)
4. Share with team if desired

### ❌ Something Broken?

1. Check [TROUBLESHOOTING.md](TROUBLESHOOTING.md)
2. Review logs:
   ```bash
   # For local binary
   cat ~/.claude/data/escalation/escalations.log | tail -20
   
   # For Docker
   docker logs -f claude-escalation-service
   ```
3. Verify manually:
   ```bash
   ~/.local/bin/escalation-manager hook --prompt "/escalate to sonnet"
   ```

---

## Files Created

After setup, you'll have:

```
~/.local/bin/
  └── escalation-manager              # The binary (6.1MB)

~/.claude/
  ├── settings.json                   # Updated with hook
  └── data/escalation/
      ├── sessions.jsonl              # Session history
      ├── escalations.log             # Event log
      └── stats.json                  # Aggregated stats

http://localhost:8077/               # Web dashboard
```

---

## Commands You Can Use Now

### In Claude Code

```
/escalate              # Escalate to Sonnet (default)
/escalate to opus      # Escalate to Opus
/escalate to haiku     # Downgrade to Haiku

[any success phrase]   # Trigger auto de-escalation
"works great"
"perfect!"
"thanks"
"solved"
```

### Via CLI

```bash
# Show current stats
~/.local/bin/escalation-manager stats summary

# View history
~/.local/bin/escalation-manager stats history

# Reset all data
~/.local/bin/escalation-manager stats reset

# Test dashboard
~/.local/bin/escalation-manager dashboard --port 8077

# Install hook again if needed
~/.local/bin/escalation-manager install-hook
```

---

## Stopping Services

```bash
# Stop dashboard
pkill -f "escalation-manager dashboard"

# Stop Docker service
cd /tmp/claude-escalate/docker-compose
docker-compose down

# Keep data? It's stored in Docker volume (persistent)
docker volume ls | grep escalation
```

---

## How to Get Help

1. **Quick questions**: Check [USAGE.md](USAGE.md)
2. **Setup issues**: Check [SETUP.md](SETUP.md)
3. **Problems**: Check [TROUBLESHOOTING.md](TROUBLESHOOTING.md)
4. **Details**: Check [DASHBOARD.md](DASHBOARD.md)
5. **Deep dive**: Check [ARCHITECTURE.md](ARCHITECTURE.md)

---

**Status**: ✅ Ready to use  
**Time to setup**: ~5 minutes  
**Docker Hub**: docker.io/szibis/claude-escalate:2.0.0  
**GitHub**: https://github.com/szibis/claude-escalate  
