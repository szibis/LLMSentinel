# Docker Service Architecture

**Run escalation system as a remote Docker service** — Hooks communicate via HTTP instead of local binary.

## Quick Start

### 1. Start the Service

```bash
cd claude-escalate/docker-compose
docker-compose up -d
```

Access dashboard: http://localhost:8077

### 2. Configure Claude Code Hooks

Edit `~/.claude/settings.json`:

```json
{
  "hooks": {
    "UserPromptSubmit": [
      {
        "type": "command",
        "command": "/Users/slawomirskowron/.claude/hooks/escalation-service-hook.sh",
        "timeout": 3
      }
    ]
  }
}
```

### 3. Test the Connection

```bash
# Check service health
curl http://localhost:9000/health

# Get current status
curl http://localhost:9000/api/status

# Test escalate
curl -X POST http://localhost:9000/api/escalate \
  -H "Content-Type: application/json" \
  -d '{"target":"opus"}'
```

## Architecture

```
Claude Code (Local)
    ↓
Hooks (escalation-service-hook.sh)
    ↓ HTTP/JSON
Docker Service (Port 9000)
    ├─ Escalation Manager (In-container)
    ├─ Stats Tracking (In-container)
    └─ Dashboard (Port 8077)
    ↓
Persistent Volume (/data/escalation)
```

## Benefits

✅ **Decoupled**: Escalation system runs independently  
✅ **Portable**: Same container across machines  
✅ **Observable**: Centralized logs and metrics  
✅ **Scalable**: Can run multiple services, load-balance requests  
✅ **Easy Updates**: Update container without touching local hooks  

## API Endpoints

### GET /health
Health check response:
```json
{"status":"healthy","timestamp":"2026-04-25T10:00:00Z"}
```

### GET /api/status
Current escalation state:
```json
{
  "currentModel": "sonnet",
  "effort": "medium",
  "sessionActive": true,
  "sessionAge": 120,
  "lastEscalation": "2026-04-25T10:00:00Z"
}
```

### POST /api/escalate
Request:
```json
{"target":"opus"}
```

Response:
```json
{"success":true,"model":"opus","timestamp":"2026-04-25T10:00:00Z"}
```

### POST /api/deescalate
Request:
```json
{"reason":"success_signal"}
```

Response:
```json
{"success":true,"model":"sonnet","timestamp":"2026-04-25T10:00:00Z","cascaded":true}
```

### POST /api/effort
Request:
```json
{"level":"high"}
```

Response:
```json
{"success":true,"level":"high","model":"opus"}
```

### GET /api/stats
Detailed metrics:
```json
{
  "totalEscalations": 5,
  "totalDeescalations": 3,
  "currentModel": "sonnet",
  "sessions": [
    {
      "id": "session-1",
      "startTime": "2026-04-25T09:00:00Z",
      "initialModel": "haiku",
      "finalModel": "sonnet",
      "tokensCost": 250,
      "saved": 150
    }
  ]
}
```

## Configuration

Environment variables in `docker-compose.yml`:

```yaml
environment:
  - PORT=9000                        # Service API port
  - DASHBOARD_PORT=8077              # Web dashboard port
  - DATA_DIR=/data/escalation        # Persistent data directory
  - SERVICE_TIMEOUT=5                # Request timeout (seconds)
  - LOG_LEVEL=info                   # Log verbosity
```

## Fallback Behavior

If the Docker service is unavailable, hooks automatically fall back to the local binary:

```bash
# This hook checks health first
call_service "/api/escalate" ...

# If service down, uses fallback
$FALLBACK_BINARY escalate --to opus
```

This ensures hooks never fail completely — they degrade gracefully.

## Management

### Check Logs
```bash
docker-compose logs -f escalation-service
```

### Restart Service
```bash
docker-compose restart escalation-service
```

### Rebuild from Latest
```bash
docker-compose pull
docker-compose up -d
```

### Inspect Volume Data
```bash
docker volume ls
docker volume inspect claude-escalate_escalation-data
docker run -v claude-escalate_escalation-data:/data alpine ls -la /data/escalation
```

## Monitoring

Dashboard shows real-time metrics:
- Current model and cost
- Active sessions
- Total escalations/de-escalations
- Token cost analysis
- Session history

Access: http://localhost:8077

## Development

To use a local binary while developing:

```bash
# Stop Docker service
docker-compose down

# Run local binary directly
~/.local/bin/escalation-manager dashboard 8077

# Hooks automatically use local binary (no service to connect to)
```

## Troubleshooting

### Service won't start
```bash
docker-compose logs escalation-service
docker ps | grep escalation
```

### Health check failing
```bash
curl -v http://localhost:9000/health
# Should see 200 response with JSON
```

### Hooks not communicating
```bash
# Test manually
curl -X POST http://localhost:9000/api/escalate \
  -H "Content-Type: application/json" \
  -d '{"target":"opus"}'
# Should see success response
```

### Data persistence
```bash
# Verify volume mounted correctly
docker inspect claude-escalation-service | grep -A 5 Mounts
```

## Next Steps

1. Start service: `docker-compose up -d`
2. Verify: `curl http://localhost:9000/health`
3. Update hooks: Add to settings.json
4. Test: Use `/escalate` commands in Claude Code
5. Monitor: Watch dashboard at http://localhost:8077
