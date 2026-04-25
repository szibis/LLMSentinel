# Deployment Guide

Deploy Claude Escalate in your environment.

---

## System Requirements

- **Go 1.21+** (if building from source)
- **2GB disk space** (for SQLite database, logs)
- **Port 9000** available (configurable)
- **macOS, Linux, Windows** supported

---

## Installation

### Option 1: Pre-Built Binary

```bash
# Download latest release
curl -L https://github.com/szibis/claude-escalate/releases/latest/download/escalation-manager -o ~/.local/bin/escalation-manager
chmod +x ~/.local/bin/escalation-manager

# Verify
escalation-manager --help
```

### Option 2: Build from Source

```bash
# Clone repository
git clone https://github.com/szibis/claude-escalate.git
cd claude-escalate

# Build CLI
go build -o escalation-manager ./cmd/escalation-cli/main.go

# Move to PATH
mv escalation-manager ~/.local/bin/
```

---

## Initial Setup

### 1. Create Configuration Directory

```bash
mkdir -p ~/.claude/escalation
mkdir -p ~/.claude/data/escalation
```

### 2. Set Token Budgets

```bash
# Daily and monthly limits
escalation-manager set-budget --daily 10.00 --monthly 100.00

# This creates: ~/.claude/escalation/config.yaml
```

### 3. Enable Sentiment Detection (Recommended)

```bash
escalation-manager config set sentiment.enabled true
escalation-manager config set sentiment.frustration_risk_threshold 0.70
```

### 4. Start the Service

```bash
# Terminal 1: Start HTTP service
escalation-manager service --port 9000

# Terminal 2: Start token metrics monitor
escalation-manager monitor --port 9000 --method env

# Or if using Barista:
escalation-manager monitor --port 9000 --method barista
```

### 5. Verify It's Running

```bash
# Check service health
curl http://localhost:9000/api/health

# Expected response:
# {"status":"healthy","uptime":"5s"}

# View dashboard
escalation-manager dashboard --sentiment
```

---

## Production Deployment

### Docker (Recommended)

```dockerfile
# Dockerfile
FROM golang:1.21-alpine
WORKDIR /app
COPY . .
RUN go build -o escalation-manager ./cmd/escalation-cli/main.go
EXPOSE 9000
ENTRYPOINT ["./escalation-manager", "service", "--port", "9000"]
```

Build and run:
```bash
docker build -t escalation-manager .
docker run -p 9000:9000 \
  -v ~/.claude/escalation:/root/.claude/escalation \
  -v ~/.claude/data/escalation:/root/.claude/data/escalation \
  escalation-manager
```

### Systemd Service

Create `/etc/systemd/system/escalation-manager.service`:

```ini
[Unit]
Description=Claude Escalation Manager
After=network.target

[Service]
Type=simple
User=claude
ExecStart=/usr/local/bin/escalation-manager service --port 9000
Restart=on-failure
RestartSec=10
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
```

Enable and start:
```bash
sudo systemctl enable escalation-manager
sudo systemctl start escalation-manager
sudo systemctl status escalation-manager
```

### Environment Variables

```bash
# Set paths
export ESCALATION_CONFIG=~/.claude/escalation/config.yaml
export ESCALATION_DATA=~/.claude/data/escalation

# Set logging
export ESCALATION_LOG_LEVEL=info
export ESCALATION_LOG_FILE=~/.claude/data/escalation/escalation.log

# Start service
escalation-manager service --port 9000
```

---

## Configuration Management

### Using Configuration File

Edit `~/.claude/escalation/config.yaml`:

```yaml
# Budgets
budgets:
  daily_usd: 10.0
  monthly_usd: 100.0
  hard_limit: false
  soft_limit: true

# Sentiment Detection
sentiment:
  enabled: true
  frustration_risk_threshold: 0.70
  learning_enabled: true

# Statusline Sources
statusline:
  sources:
    - type: barista
      enabled: true
    - type: envvar
      enabled: true

# Logging
logging:
  level: info
  file: ~/.claude/data/escalation/escalation.log
```

### CLI Configuration

```bash
# View current config
escalation-manager config

# Set values
escalation-manager config set sentiment.enabled true
escalation-manager config set budgets.daily_usd 15.0

# Check single setting
escalation-manager config | grep frustration_risk
```

### Secrets Management

For sensitive data (API tokens for webhooks):

```bash
# Set via environment
export WEBHOOK_AUTH_TOKEN="secret-token-here"

# Reference in config
statusline:
  sources:
    - type: webhook
      url: https://api.example.com/metrics
      auth_token: ${WEBHOOK_AUTH_TOKEN}
```

---

## Monitoring & Health Checks

### Health Endpoint

```bash
# Check service health
curl http://localhost:9000/api/health

# Response:
# {
#   "status": "healthy",
#   "uptime": "45m23s",
#   "database": "ok",
#   "validation_count": 234
# }
```

### Logs

```bash
# View logs
tail -f ~/.claude/data/escalation/escalation.log

# Filter errors
grep ERROR ~/.claude/data/escalation/escalation.log

# Count validations
grep "Phase 1" ~/.claude/data/escalation/escalation.log | wc -l
```

### Metrics

```bash
# Get metrics
curl http://localhost:9000/api/analytics/budget-status | jq .

# Get validation count
curl http://localhost:9000/api/statistics | jq '.validation_count'
```

---

## Upgrading

### From Older Version

```bash
# Stop service
pkill escalation-manager

# Backup database
cp ~/.claude/data/escalation/escalation.db ~/.claude/data/escalation/escalation.db.backup

# Update binary
curl -L https://github.com/szibis/claude-escalate/releases/latest/download/escalation-manager -o ~/.local/bin/escalation-manager
chmod +x ~/.local/bin/escalation-manager

# Verify backward compatibility
escalation-manager --version

# Restart service
escalation-manager service --port 9000
```

---

## Troubleshooting Deployment

### Port Already in Use

```bash
# Find process using port 9000
lsof -i :9000

# Kill existing process
kill -9 <PID>

# Or use different port
escalation-manager service --port 9001
```

### Configuration Not Loading

```bash
# Check config file exists
ls -la ~/.claude/escalation/config.yaml

# Check permissions
cat ~/.claude/escalation/config.yaml

# Recreate if missing
escalation-manager set-budget --daily 10.00
```

### Database Locked

```bash
# Check for stale process
ps aux | grep escalation-manager

# Force reset database
rm ~/.claude/data/escalation/escalation.db

# Restart service (will recreate database)
escalation-manager service --port 9000
```

---

## Backup & Recovery

### Backup Configuration & Database

```bash
# Backup script
#!/bin/bash
BACKUP_DIR=~/escalation-backups/$(date +%Y%m%d-%H%M%S)
mkdir -p $BACKUP_DIR

cp ~/.claude/escalation/config.yaml $BACKUP_DIR/
cp ~/.claude/data/escalation/escalation.db $BACKUP_DIR/
cp ~/.claude/data/escalation/escalation.log $BACKUP_DIR/

echo "Backed up to $BACKUP_DIR"
```

### Restore from Backup

```bash
# Stop service
pkill escalation-manager

# Restore files
cp ~/escalation-backups/latest/config.yaml ~/.claude/escalation/
cp ~/escalation-backups/latest/escalation.db ~/.claude/data/escalation/

# Restart service
escalation-manager service --port 9000
```

---

## Performance Tuning

### Database Optimization

```bash
# Regular maintenance
# SQLite compaction (done automatically, but can force)
sqlite3 ~/.claude/data/escalation/escalation.db "VACUUM;"

# Check database size
ls -lh ~/.claude/data/escalation/escalation.db
```

### Resource Limits

```bash
# Set max memory for service (if running in Docker)
docker run -m 256m escalation-manager

# Or in systemd:
[Service]
MemoryLimit=256M
```

---

## See Also

- [Monitoring](monitoring.md) — Production monitoring
- [Troubleshooting](troubleshooting.md) — Common issues
- [Configuration](../architecture/overview.md) — System architecture
