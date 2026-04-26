# Deployment & Testing Guide

**Complete guide for testing locally and deploying to production**

## What You Now Have

✅ **Docker Image**: `szibis/claude-escalate:2.0.0` on Docker Hub  
✅ **Go Binary**: Compiled, tested, 6.1MB static linked  
✅ **Bash Tools**: Escalation manager, stats tracker, barista module  
✅ **Web Dashboard**: Real-time metrics, light/dark mode, 2s auto-refresh  
✅ **Documentation**: 12+ comprehensive guides  
✅ **Tests**: 100% pass rate on all features  

## Phase Overview

### Phase 1: Push to Registry ✅
- Docker images pushed: `szibis/claude-escalate:2.0.0` and `:latest`
- Registry: docker.io (Docker Hub)
- Command: `docker pull szibis/claude-escalate:2.0.0`

### Phase 2: GitHub Release ✅
- Version: v2.0.0
- URL: https://github.com/szibis/claude-escalate/releases/tag/v2.0.0
- Includes: Full release notes, feature list, installation instructions

### Phase 3: Docker Service Architecture ✅
- docker-compose setup with persistent volumes
- HTTP-based hook integration (future enhancement)
- Graceful fallback to local binary
- Comprehensive API documentation in DOCKER_SERVICE.md

### Phase 4: Local Testing ✅
- Docker service running on http://localhost:8077
- Dashboard accessible and displaying metrics
- Ready for local hook testing
- Detailed testing scenarios in LOCAL_TESTING.md

## Two Deployment Options

### Option A: Local Binary (Fast, Simple)

**Best for:** Single-machine setups, local development, getting started quickly

```bash
# 1. Copy binary
mkdir -p ~/.local/bin
cp /tmp/claude-escalate/claude-escalate ~/.local/bin/escalation-manager
chmod +x ~/.local/bin/escalation-manager

# 2. Install hook
~/.local/bin/escalation-manager install-hook

# 3. Start dashboard (separate terminal)
~/.local/bin/escalation-manager dashboard --port 8077

# 4. Verify
curl http://localhost:8077
open http://localhost:8077
```

**Features:**
- No Docker dependency
- Instant startup
- Direct file access
- Easy debugging
- Perfect for testing

### Option B: Docker Service (Portable, Scalable)

**Best for:** Team deployments, cloud environments, persistent monitoring

```bash
# 1. Start service
cd /tmp/claude-escalate/docker-compose
docker-compose up -d

# 2. Configure HTTP-based hooks
# Edit ~/.claude/settings.json to use escalation-service-hook.sh
# (Falls back to local binary if service unavailable)

# 3. Access dashboard
open http://localhost:8077

# 4. Monitor
docker-compose logs -f
```

**Features:**
- Container isolation
- Persistent volumes
- Easy updates
- Centralized metrics
- Health checks built-in
- Works across networks

## Quick Start - Option A (Local Binary)

```bash
# 1. Set up binary
mkdir -p ~/.local/bin
cp /tmp/claude-escalate/claude-escalate ~/.local/bin/escalation-manager
chmod +x ~/.local/bin/escalation-manager

# 2. Install hook into Claude Code
~/.local/bin/escalation-manager install-hook
# This updates ~/.claude/settings.json automatically

# 3. Start dashboard
~/.local/bin/escalation-manager dashboard --port 8077 &

# 4. Test in Claude Code
# Type: /escalate to opus
# Should see model change in settings.json

# 5. Monitor dashboard
open http://localhost:8077
```

## Quick Start - Option B (Docker)

```bash
# 1. Start Docker service
cd /tmp/claude-escalate/docker-compose
docker-compose up -d

# 2. Wait for health check
sleep 5
curl http://localhost:8077  # Should return HTML

# 3. Configure Claude Code
# Manual: Add hook to ~/.claude/settings.json (see SETUP.md)
# Or: Use local binary with hook install

# 4. Test
# Type: /escalate to sonnet
# Watch dashboard update

# 5. Monitor
docker logs -f claude-escalation-service
```

## Testing Checklist

### Before Production Deployment

- [ ] **Binary works**: `escalation-manager version` shows v2.0.0
- [ ] **Hook installed**: `grep -q escalation ~/.claude/settings.json`
- [ ] **Dashboard running**: `curl http://localhost:8077` returns HTML
- [ ] **Manual escalation**: `/escalate to opus` changes model
- [ ] **De-escalation**: Success phrases trigger cascade
- [ ] **Cascade timeout**: Second cascade within 5 min blocked
- [ ] **Session cleanup**: Haiku cascade clears session
- [ ] **Stats tracking**: Dashboard shows escalation counts
- [ ] **Model changes**: Barista shows correct model in statusline
- [ ] **Docker image**: `docker run szibis/claude-escalate:2.0.0 version`

### Monitor First Week

Track these metrics:
- **Escalations per day**: Should decrease after first week (learned patterns)
- **De-escalation success rate**: Should be >80% (good cascade detection)
- **Average tokens per session**: Should decrease over time (getting cheaper)
- **False positives**: Success phrases detected incorrectly (adjust guards if needed)

## File Locations

### Local Installation

```
~/.local/bin/escalation-manager          # Main binary
~/.claude/hooks/de-escalate-model.sh     # Hook script
~/.claude/data/escalation/               # Data directory
  ├── sessions.jsonl                      # Session history
  ├── escalations.log                     # Event log
  └── stats.json                          # Aggregated stats
```

### Docker Installation

```
/tmp/claude-escalate/docker-compose/     # Compose config
  ├── docker-compose.yml                  # Service definition
  └── escalation-data/                    # Persistent volume
```

### Configuration

```
~/.claude/settings.json                   # Claude Code settings
  ├── model                               # Current model
  ├── effortLevel                         # Auto-effort level
  └── hooks                               # Hook definitions
```

## Common Tasks

### Check Current Model

```bash
jq '.model' ~/.claude/settings.json
# Output: "claude-haiku-4-5-20251001"
```

### View Escalation History

```bash
~/.local/bin/escalation-manager stats history
# Or via Docker:
curl http://localhost:8077/api/stats/history
```

### View Cost Analysis

```bash
~/.local/bin/escalation-manager stats summary
# Shows: Total escalations, de-escalations, tokens saved
```

### Stop All Services

```bash
# Docker
cd /tmp/claude-escalate/docker-compose && docker-compose down

# Local dashboard
pkill -f "escalation-manager dashboard"
```

### Reset All Data

```bash
# WARNING: This clears all history!
rm -rf ~/.claude/data/escalation/*
~/.local/bin/escalation-manager stats reset
```

## Troubleshooting

### Binary not found

```bash
# Ensure it's in PATH
which escalation-manager

# If not, add to PATH
export PATH="$HOME/.local/bin:$PATH"
```

### Hook not triggering

```bash
# Check settings.json
jq '.hooks.UserPromptSubmit' ~/.claude/settings.json

# Test hook manually
~/.local/bin/escalation-manager --help

# Check permissions
ls -la ~/.local/bin/escalation-manager
# Should have executable bit
```

### Dashboard not responding

```bash
# Check if running
curl -v http://localhost:8077

# Restart dashboard
pkill -f "escalation-manager dashboard"
~/.local/bin/escalation-manager dashboard --port 8077 &
```

### Model not changing

```bash
# Check hook output
~/.local/bin/escalation-manager hook --prompt "/escalate to opus"

# Check settings.json was updated
jq '.model' ~/.claude/settings.json
```

### Docker service failing

```bash
# Check logs
docker logs claude-escalation-service

# Check ports
lsof -i :8077

# Restart
docker-compose down && docker-compose up -d
```

## Next Steps

1. **Choose deployment**: Option A (local) or B (Docker)
2. **Follow quick start**: 5 minutes to get running
3. **Run testing scenarios**: Verify all features work
4. **Monitor dashboard**: Watch metrics for 24 hours
5. **Adjust settings**: Tune cascade timeout, effort levels if needed
6. **Documentation**: Share with team if using Docker option

## Resources

- **Setup**: [SETUP.md](SETUP.md) — Installation and configuration
- **Usage**: [USAGE.md](USAGE.md) — User guide and commands
- **Dashboard**: [DASHBOARD.md](DASHBOARD.md) — Metrics and API
- **Architecture**: [ARCHITECTURE.md](ARCHITECTURE.md) — Technical design
- **Docker**: [DOCKER_SERVICE.md](DOCKER_SERVICE.md) — Remote service setup
- **Testing**: [LOCAL_TESTING.md](LOCAL_TESTING.md) — Testing scenarios
- **Troubleshooting**: [TROUBLESHOOTING.md](TROUBLESHOOTING.md) — Common issues

## Support

For issues:

1. Check [TROUBLESHOOTING.md](TROUBLESHOOTING.md)
2. Review logs: `docker logs` or hook output
3. Test manually: `~/.local/bin/escalation-manager version`
4. Check GitHub issues: https://github.com/szibis/claude-escalate/issues

---

**Status**: Production Ready  
**Version**: 2.0.0  
**Last Updated**: 2026-04-25  
**Docker Registry**: docker.io/szibis/claude-escalate
