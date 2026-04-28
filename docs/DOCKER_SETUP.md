# Docker Setup Guide

Quick start guide for running Claude Escalate with Docker.

---

## Quick Start (5 minutes)

### 1. Build Image

```bash
docker build -t claude-escalate . 
```

### 2. Run Dashboard (with Tool Configuration UI)

```bash
docker run -d \
  --name claude-escalate \
  -p 9000:8077 \
  -v escalate-data:/data \
  claude-escalate:latest \
  dashboard --port 8077
```

### 3. Access Dashboard

Open your browser:
```
http://localhost:9000/dashboard
```

Click **🔧 Tools** tab to add/manage custom tools via UI.

---

## API Endpoints

All endpoints available at `http://localhost:9000`:

### Tool Management (v0.8.0+)
- `GET /api/tools` — List configured tools
- `POST /api/tools/add` — Add new tool
- `PUT /api/tools/{name}` — Edit tool
- `DELETE /api/tools/{name}` — Delete tool
- `POST /api/tools/{name}/test` — Test tool health
- `GET /api/tools/types` — Available tool types

### Example: Add Tool via API

```bash
curl -X POST http://localhost:9000/api/tools/add \
  -H "Content-Type: application/json" \
  -d '{
    "name": "my_script",
    "type": "cli",
    "path": "/usr/local/bin/my_script",
    "settings": {}
  }'
```

---

## Docker Compose (Full Stack)

For full monitoring stack with Prometheus/Grafana:

```bash
docker-compose up -d
```

Services:
- **Claude Escalate**: http://localhost:9000/dashboard
- **Grafana**: http://localhost:3000 (admin/admin)
- **Prometheus**: http://localhost:9090
- **VictoriaMetrics**: http://localhost:8428

---

## Configuration

### Environment Variables

```bash
docker run -d \
  --name claude-escalate \
  -p 9000:8077 \
  -e LOG_LEVEL=info \
  -e ESCALATE_DATA_DIR=/data \
  -v escalate-data:/data \
  claude-escalate:latest \
  dashboard --port 8077
```

### Volume Mounting

```bash
# Mount config directory
docker run -d \
  -p 9000:8077 \
  -v ./config:/app/config \
  -v escalate-data:/data \
  claude-escalate:latest \
  dashboard --port 8077
```

---

## Commands

### Dashboard
```bash
docker run -d -p 9000:8077 claude-escalate:latest dashboard --port 8077
```

### Service (API only, no UI)
```bash
docker run -d -p 9000:9000 claude-escalate:latest service --port 9000
```

### Hook Mode
```bash
docker run -d claude-escalate:latest hook
```

---

## Troubleshooting

### Port already in use

```bash
# Find and kill existing container
docker ps | grep claude-escalate
docker rm -f <container-id>

# Or use different port
docker run -d -p 9001:8077 claude-escalate:latest dashboard
```

### No data persistence

Ensure volumes are mounted:
```bash
# Named volume
docker run -d -v escalate-data:/data claude-escalate:latest dashboard

# Local directory
docker run -d -v /tmp/escalate:/data claude-escalate:latest dashboard
```

### Can't access from other machines

Service must bind to `0.0.0.0` (all interfaces), not `127.0.0.1`.
This is handled automatically in the binary — service is accessible at:
```
http://<machine-ip>:9000/dashboard
```

---

## Performance

Minimal resource usage:
- **CPU**: <5% idle
- **Memory**: ~50-100MB
- **Disk**: ~1KB per operation

---

## See Also

- [GETTING_STARTED.md](GETTING_STARTED.md) — Installation
- [API.md](API.md) — Full API reference
- [DEPLOYMENT_GUIDE.md](DEPLOYMENT_GUIDE.md) — Production deployment
