# Monolithic Service Architecture

**Single Go binary replaces all bash scripts**

## Overview

All escalation logic is now consolidated into one HTTP-based service:

```
Claude Code Hook
    ↓ (stdin)
http-hook.sh (minimal wrapper)
    ↓ POST /api/hook
Go Service (localhost:9000)
    ├─ Parses prompt
    ├─ Detects /escalate, success signals, auto-effort
    ├─ Logs to SQLite database
    ├─ Updates settings.json
    └─ Returns response
    ↓
Dashboard + Stats API
```

## Quick Start

### 1. Start Service

```bash
# Start on default port 9000
escalation-manager service

# Or custom port
escalation-manager service --port 8888
```

### 2. Configure Hook

Edit `~/.claude/settings.json`:

```json
{
  "hooks": {
    "UserPromptSubmit": [
      {
        "type": "command",
        "command": "/path/to/hooks/http-hook.sh",
        "timeout": 10,
        "description": "Escalation service hook"
      }
    ]
  }
}
```

Or set environment variable for custom service URL:

```bash
export ESCALATION_SERVICE_URL="http://localhost:9000"
```

### 3. Access Dashboard

http://localhost:9000/

(Dashboard is served by the service)

## API Endpoints

### POST /api/hook
Processes user prompts. Called by http-hook.sh.

**Request:**
```json
{
  "prompt": "user's message or /escalate command"
}
```

**Response:**
```json
{
  "continue": true,
  "suppressOutput": true,
  "action": "escalate",
  "currentModel": "opus"
}
```

### POST /api/escalate
Manually escalate to a model.

**Request:**
```json
{
  "target": "opus"
}
```

**Response:**
```json
{
  "success": true,
  "model": "opus",
  "timestamp": "2026-04-25T15:34:35Z"
}
```

### POST /api/deescalate
Cascade down one model tier.

**Request:**
```json
{
  "reason": "success_signal"
}
```

**Response:**
```json
{
  "success": true,
  "model": "sonnet",
  "cascaded": true,
  "timestamp": "2026-04-25T15:34:35Z"
}
```

### POST /api/effort
Set effort level and route to appropriate model.

**Request:**
```json
{
  "level": "high"
}
```

**Response:**
```json
{
  "success": true,
  "level": "high",
  "model": "opus"
}
```

### GET /api/stats
Return escalation statistics.

**Response:**
```json
{
  "escalations": 5,
  "de_escalations": 3,
  "turns": 42,
  "current_model": "sonnet",
  "timestamp": "2026-04-25T15:34:35Z"
}
```

### GET /api/health
Check service health.

**Response:**
```json
{
  "status": "healthy",
  "timestamp": "2026-04-25T15:34:35Z"
}
```

### GET /
Serve dashboard UI with all metrics.

## Features

### Prompt Processing (/api/hook)
Automatically detects:
- **`/escalate` commands** → Escalate to Sonnet (or target model)
- **`/escalate to opus`** → Escalate to specific model
- **Success signals** → "works", "perfect", "thanks", etc.
  - Triggers cascade down: Opus → Sonnet → Haiku
- **Auto-effort detection** → Routes by task complexity
  - Simple ("what is") → Haiku (low effort)
  - Medium (refactor) → Sonnet (medium effort)
  - Complex (architecture) → Opus (high effort)

### Database Logging
All events logged to SQLite:
- Escalations (timestamp, from model, to model, reason)
- De-escalations (cascade events)
- Task types (for learning patterns)
- Session tracking (for analytics)

### Statistics
Real-time metrics:
- Total escalations
- Total de-escalations  
- Success rate
- Current model
- Model distribution
- Task type breakdown
- Cost analysis (tokens saved)

### Dashboard
Live web UI showing:
- Current model & effort
- Escalation metrics
- Cost analysis (vs Opus baseline)
- Session history
- Task type performance
- Light/dark mode

## Architecture

### Eliminated Components
✅ Bash escalation logic (in service now)
✅ Bash success detection (in service now)
✅ Bash auto-effort routing (in service now)
✅ Bash stats tracking (in service now)
✅ Multiple scripts (consolidated into one binary)

### New Components
✅ `internal/service/service.go` (341 lines)
  - HTTP server with all endpoint handlers
  - Prompt parsing and detection
  - Database logging
  - Settings.json management

✅ `hooks/http-hook.sh` (12 lines)
  - Minimal wrapper: reads stdin, POSTs to service
  - All logic in Go

✅ Enhanced dashboard
  - Cost analysis
  - Light/dark mode
  - Real-time updates

## Development

### Building
```bash
go build -o escalation-manager ./cmd/claude-escalate
```

### Running
```bash
# Service mode (default port 9000)
./escalation-manager service

# With custom port
./escalation-manager service --port 8888

# Dashboard only (old mode, no service)
./escalation-manager dashboard --port 8077

# Stats only
./escalation-manager stats summary
```

### Testing
```bash
# Start service in background
./escalation-manager service --port 9000 &

# Test endpoints
curl http://localhost:9000/api/health

curl -X POST http://localhost:9000/api/escalate \
  -H "Content-Type: application/json" \
  -d '{"target":"opus"}'

curl http://localhost:9000/api/stats
```

## Configuration

### Environment Variables
```bash
# Service URL for hook
export ESCALATION_SERVICE_URL="http://localhost:9000"

# Service port
export ESCALATION_SERVICE_PORT="9000"

# Data directory
export ESCALATION_DATA_DIR="$HOME/.claude/data/escalation"
```

### Service Settings
In `~/.claude/settings.json`:
```json
{
  "model": "claude-haiku-4-5-20251001",
  "effortLevel": "low",
  "hooks": {
    "UserPromptSubmit": [
      {
        "type": "command",
        "command": "/path/to/hooks/http-hook.sh",
        "timeout": 10
      }
    ]
  }
}
```

## Troubleshooting

### Service won't start
```bash
# Check port is not in use
lsof -i :9000

# Kill existing process
pkill -f "escalation-manager service"

# Try different port
./escalation-manager service --port 9001
```

### Hook not working
```bash
# Verify hook is in settings
grep -A 5 "UserPromptSubmit" ~/.claude/settings.json

# Test hook manually
echo '{"prompt":"/escalate to opus"}' | \
  curl -X POST http://localhost:9000/api/hook \
  -H "Content-Type: application/json" \
  -d @-
```

### Stats not updating
```bash
# Check service is running
curl http://localhost:9000/api/health

# Check database
ls -la ~/.claude/data/escalation/escalation.db

# View stats
curl http://localhost:9000/api/stats
```

## Performance

- **Service startup:** <100ms
- **Hook response time:** <50ms (HTTP request only)
- **Database lookup:** <10ms (SQLite query)
- **Dashboard refresh:** 2 seconds (auto-refresh)
- **Memory usage:** ~25-50MB per instance
- **CPU usage:** <1% idle, <10% processing

## Security

- Service only listens on localhost (127.0.0.1)
- No external network access
- SQLite database file restricted to user
- Settings.json modifications are atomic
- No credentials or sensitive data in API responses

## Future Enhancements

- [ ] Remote service support (non-localhost)
- [ ] Authentication for remote service
- [ ] Load balancing across services
- [ ] Service clustering
- [ ] Advanced analytics dashboard
- [ ] Predictions API
- [ ] Webhook notifications
