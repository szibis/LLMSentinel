# Deployment Guide - v1.0.0-rc1

Production deployment procedures for LLMSentinel Unified Gateway.

## Quick Staging Deploy (5 minutes)

```bash
# 1. Download
wget https://github.com/szibis/claude-escalate/releases/download/v1.0.0-rc1/escalate-gateway-linux-amd64
chmod +x escalate-gateway-linux-amd64

# 2. Start (mock API for staging)
./escalate-gateway-linux-amd64 -provider mock -api-key "staging-key"

# 3. Test
curl http://localhost:8080/health
```

## Production Deploy (15 minutes)

```bash
# 1. Download release binaries
cd /opt
wget https://github.com/szibis/claude-escalate/releases/download/v1.0.0-rc1/escalate-gateway-linux-amd64
chmod +x escalate-gateway-linux-amd64

# 2. Create service user
sudo useradd -r -s /bin/false escalate
sudo mkdir -p /var/lib/escalate
sudo chown escalate:escalate /var/lib/escalate

# 3. Create systemd service
sudo systemctl enable /opt/escalate-gateway-linux-amd64
sudo systemctl start escalate-gateway

# 4. Verify running
sudo systemctl status escalate-gateway
curl http://localhost:8080/health
```

## Nginx Reverse Proxy (Optional)

```nginx
upstream escalate {
    server 127.0.0.1:8080;
}

server {
    listen 443 ssl http2;
    server_name api.example.com;
    
    location / {
        proxy_pass http://escalate;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

## Backup Strategy

```bash
# Daily backup
escalate-db-backup -cmd backup -db /var/lib/escalate/escalate.db

# Verify backup
escalate-db-backup -cmd verify -file escalate.db.backup

# Restore if needed
escalate-db-backup -cmd restore -file escalate.db.backup -db /var/lib/escalate/escalate.db
```

## Monitoring

```bash
# Health check
curl http://localhost:8080/health

# Metrics (requires auth)
curl -H "Authorization: Bearer api-key" http://localhost:8080/metrics

# Database check
curl -H "Authorization: Bearer api-key" http://localhost:8080/health/db
```

## Scaling

**Horizontal**: Deploy multiple instances behind load balancer  
**Vertical**: Increase memory/CPU allocation  
**Database**: Backup daily, archive monthly

---

See full deployment guide in DEPLOYMENT_GUIDE.md for comprehensive instructions.

**Staging Deployment**: 2026-05-22 (30-day test)  
**Production Deployment**: 2026-06-09  
