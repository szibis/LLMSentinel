# Monitoring & Observability

Monitor Claude Escalate in production.

---

## Health Checks

### Basic Health Endpoint

```bash
# Check service status
curl http://localhost:9000/api/health

# Response:
# {
#   "status": "healthy",
#   "uptime": "2h15m",
#   "database_ok": true,
#   "validations_processed": 245,
#   "last_error": null
# }
```

### Script-Based Health Monitoring

```bash
#!/bin/bash
# Simple health check script

RESPONSE=$(curl -s http://localhost:9000/api/health)
STATUS=$(echo $RESPONSE | jq -r '.status')

if [ "$STATUS" = "healthy" ]; then
  echo "✅ Escalation Manager is healthy"
  exit 0
else
  echo "❌ Escalation Manager is down!"
  echo "Response: $RESPONSE"
  exit 1
fi
```

---

## Logging

### View Real-Time Logs

```bash
# Follow logs
tail -f ~/.claude/data/escalation/escalation.log

# Example output:
# 2024-04-25T10:15:23Z [INFO] Phase 1: Analyzing prompt
# 2024-04-25T10:15:23Z [INFO] Task type: concurrency (complexity: 0.72)
# 2024-04-25T10:15:23Z [INFO] Sentiment: neutral (no frustration)
# 2024-04-25T10:15:23Z [INFO] Budget: $3.50/10.00 remaining
# 2024-04-25T10:15:23Z [INFO] Route decision: Sonnet (confidence: 0.92)
```

### Filter Specific Events

```bash
# Show only errors
grep ERROR ~/.claude/data/escalation/escalation.log

# Show only sentiment events
grep "Sentiment" ~/.claude/data/escalation/escalation.log

# Show only budget warnings
grep "budget\|Budget\|BUDGET" ~/.claude/data/escalation/escalation.log

# Show only escalations
grep "escalat" ~/.claude/data/escalation/escalation.log

# Count validations
grep "Phase 1" ~/.claude/data/escalation/escalation.log | wc -l
```

### Log Rotation

Configure in `~/.claude/escalation/config.yaml`:

```yaml
logging:
  level: info                    # debug, info, warn, error
  file: ~/.claude/data/escalation/escalation.log
  retention_days: 30             # Keep 30 days of logs
  max_file_size_mb: 100          # Rotate at 100MB
```

---

## Metrics & Analytics

### Daily Budget Status

```bash
# Check current spending
escalation-manager dashboard --budget

# Or via API
curl http://localhost:9000/api/analytics/budget-status | jq .

# Response:
# {
#   "daily_budget": 10.0,
#   "daily_used": 3.78,
#   "daily_remaining": 6.22,
#   "daily_percent_used": 37.8,
#   "trend_daily_spending": 0.95,  # $ per hour
#   "estimated_total_today": 7.12,
#   "monthly_budget": 100.0,
#   "monthly_used": 45.20,
#   "monthly_remaining": 54.80
# }
```

### Sentiment Analysis

```bash
# View satisfaction trends
escalation-manager dashboard --sentiment

# Or via API
curl http://localhost:9000/api/analytics/sentiment-trends?hours=24 | jq .

# Response:
# {
#   "period_hours": 24,
#   "satisfaction_rate": 0.873,
#   "satisfied": 62,
#   "neutral": 7,
#   "frustrated": 2,
#   "confused": 0,
#   "impatient": 0,
#   "recent_events": [...]
# }
```

### Model Satisfaction by Task Type

```bash
# See which models work best
curl http://localhost:9000/api/analytics/model-satisfaction | jq .

# Response:
# {
#   "concurrency": {
#     "haiku": {"success_rate": 0.45, "count": 20},
#     "sonnet": {"success_rate": 0.78, "count": 18},
#     "opus": {"success_rate": 0.98, "count": 12}
#   },
#   "parsing": {
#     "haiku": {"success_rate": 0.72, "count": 25},
#     "sonnet": {"success_rate": 0.89, "count": 15}
#   }
# }
```

### Token Efficiency

```bash
# See estimate vs actual
curl http://localhost:9000/api/analytics/token-efficiency | jq .

# Response:
# {
#   "avg_estimation_error": -15.3,  # % (negative = underestimated)
#   "token_efficiency": "GOOD",
#   "estimate_vs_actual": {
#     "estimated_total": 1680,
#     "actual_total": 1420,
#     "error_percent": -15.5
#   }
# }
```

---

## Alerts & Notifications

### Critical Alerts

Set up monitoring for these conditions:

1. **Service Down**
   ```bash
   # Alert if health check fails
   if ! curl -f http://localhost:9000/api/health > /dev/null; then
     send_alert "Escalation Manager is DOWN"
   fi
   ```

2. **Daily Budget Exceeded**
   ```bash
   DAILY_PERCENT=$(curl -s http://localhost:9000/api/analytics/budget-status | jq '.daily_percent_used')
   if (( $(echo "$DAILY_PERCENT > 90" | bc -l) )); then
     send_alert "Daily budget 90%+ used: $DAILY_PERCENT%"
   fi
   ```

3. **High Frustration Rate**
   ```bash
   FRUSTRATION=$(curl -s http://localhost:9000/api/analytics/sentiment-trends | jq '.frustrated')
   if [ "$FRUSTRATION" -gt 5 ]; then
     send_alert "High frustration detected: $FRUSTRATION events"
   fi
   ```

4. **Database Issues**
   ```bash
   if grep "database error\|database locked" ~/.claude/data/escalation/escalation.log; then
     send_alert "Database errors detected"
   fi
   ```

### Alert Integration Examples

**Email Alert**:
```bash
#!/bin/bash
BUDGET=$(curl -s http://localhost:9000/api/analytics/budget-status | jq '.daily_percent_used')
if (( $(echo "$BUDGET > 80" | bc -l) )); then
  mail -s "Escalation Manager Alert" user@example.com << EOF
Daily budget is ${BUDGET}% used.
Remaining: $(curl -s http://localhost:9000/api/analytics/budget-status | jq '.daily_remaining')
EOF
fi
```

**Slack Alert**:
```bash
#!/bin/bash
BUDGET=$(curl -s http://localhost:9000/api/analytics/budget-status | jq '.daily_remaining')
curl -X POST https://hooks.slack.com/services/YOUR/WEBHOOK/URL \
  -d "{\"text\": \"⚠️ Escalation Manager: $BUDGET budget remaining\"}"
```

---

## Performance Monitoring

### Response Times

```bash
# Measure service response time
time curl http://localhost:9000/api/health

# Typical: <100ms for health check
```

### Database Performance

```bash
# Check database size (should stay under 1GB)
du -sh ~/.claude/data/escalation/escalation.db

# Number of validations stored
echo "SELECT COUNT(*) FROM validations;" | \
  sqlite3 ~/.claude/data/escalation/escalation.db
```

### Token Tracking Accuracy

```bash
# Compare estimated vs actual over last 100 validations
curl http://localhost:9000/api/analytics/validation-accuracy | jq .

# Response:
# {
#   "last_100_validations": {
#     "avg_error_percent": -12.5,
#     "accuracy_status": "GOOD",
#     "suggestions": [...]
#   }
# }
```

---

## Dashboards

### CLI Dashboard

```bash
# View all metrics
escalation-manager dashboard

# View specific section
escalation-manager dashboard --sentiment
escalation-manager dashboard --budget
escalation-manager dashboard --optimization
```

### Web Dashboard

```bash
# Open in browser
http://localhost:9000

# Tabs available:
# - Overview: Current status, recent sessions
# - Sentiment: User satisfaction, frustration events
# - Budget: Daily/monthly spending
# - Optimization: Cost-saving recommendations
```

---

## Capacity Planning

### Growth Tracking

Monitor these metrics over time:

```bash
# Weekly budget usage trend
curl http://localhost:9000/api/analytics/budget-history?period=week | jq '.weekly_average'

# Monthly validations
curl http://localhost:9000/api/analytics/statistics | jq '.validations_this_month'

# Database growth
ls -lh ~/.claude/data/escalation/escalation.db
# Should stay under 500MB for years of data
```

### Scaling Recommendations

| Metric | Normal | Warning | Action |
|--------|--------|---------|--------|
| Daily validations | <100 | >500 | Monitor performance |
| Database size | <100MB | >500MB | Archive old data |
| Budget requests/min | <10 | >100 | Consider distributed setup |

---

## Troubleshooting

### Service Not Responding

```bash
# Check if running
ps aux | grep escalation-manager

# Check port
lsof -i :9000

# Check logs for errors
tail -50 ~/.claude/data/escalation/escalation.log | grep ERROR
```

### High Memory Usage

```bash
# Check memory
ps aux | grep escalation-manager | grep -v grep

# If >500MB, may need optimization
# - Reduce log retention
# - Archive old validations
# - Restart service to clear memory
```

### Slow Response Times

```bash
# Check database performance
sqlite3 ~/.claude/data/escalation/escalation.db "PRAGMA database_list;"

# Compact database if slow
sqlite3 ~/.claude/data/escalation/escalation.db "VACUUM;"
```

---

## See Also

- [Deployment](deployment.md) — How to deploy
- [Troubleshooting](troubleshooting.md) — Common issues
- [Dashboards](../analytics/dashboards.md) — Analytics visualization
