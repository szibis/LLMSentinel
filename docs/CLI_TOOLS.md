# Claude Escalate CLI Tools Guide

Complete reference for CLI commands and utilities.

## Available CLI Tools

### 1. `claude-escalate` — Main Gateway

**Start the gateway with configuration:**
```bash
claude-escalate --config config.yaml --port 9000
```

**Options:**
- `--config <file>` — Configuration file path
- `--port <port>` — HTTP server port (default: 9000)
- `--dashboard` — Enable dashboard (default: true)
- `--metrics` — Enable metrics collection (default: true)
- `--verbose` — Verbose logging (default: false)

### 2. `escalate-tools` — Tool Management & Diagnostics

**Discover installed tools:**
```bash
escalate-tools discover          # List all detected tools
escalate-tools discover -v       # Verbose (show purposes and usage)
```

**Check tool status:**
```bash
escalate-tools status            # Health check all tools
```

**Validate configuration:**
```bash
escalate-tools validate --config config.yaml
```

**Manage tool configuration:**
```bash
escalate-tools config --add-tool cli --name my_script --path ~/scripts/my_script.sh
escalate-tools config --add-tool mcp --name custom_mcp --path ~/.sockets/custom.sock
```

### 3. `escalate-metrics` — Metrics & Analytics

**Export metrics:**
```bash
escalate-metrics export --format json --output metrics.json
escalate-metrics export --format csv  # Export to CSV
```

**View real-time metrics:**
```bash
escalate-metrics watch               # Live metrics stream
escalate-metrics dashboard          # Browser dashboard
```

**Cost analysis:**
```bash
escalate-metrics cost --since today
escalate-metrics cost --since 7days  # Last 7 days
escalate-metrics cost --period month # This month
```

### 4. `escalate-cli` — Direct API Client

**Simple request:**
```bash
escalate-cli --query "Find all functions calling authenticate()"
```

**With cache bypass:**
```bash
escalate-cli --query "Analyze code for security" --no-cache
escalate-cli --query "Quick summary" --fresh
```

**Batch submission:**
```bash
escalate-cli batch --file requests.json
```

**Check batch status:**
```bash
escalate-cli batch status batch_abc123xyz
escalate-cli batch results batch_abc123xyz
```

### 5. `escalate-load-test` — Load Testing

**Run sustained load test:**
```bash
escalate-load-test --duration 5m --rate 5000 --workers 100
```

**Memory stability test:**
```bash
escalate-load-test --test memory --duration 30s --rate 2000
```

**Cache hit rate test:**
```bash
escalate-load-test --test cache --duration 30s --rate 1000
```

**Latency percentiles:**
```bash
escalate-load-test --test latency --duration 20s --rate 2000
```

---

## Configuration Management

### Config Locations (auto-detected order)

1. `./config.yaml` (current directory)
2. `./configs/config.yaml` (project directory)
3. `~/.claude-escalate/config.yaml` (home directory)
4. `~/.claude-escalate/config-auto.yaml` (auto-discovered)

### Auto-Discovery

If no config file exists, tools are auto-detected:
```bash
escalate-tools discover > config-auto.yaml
# Then copy to expected location
```

### Live Reload

Edit config and reload without restart:
```bash
# Dashboard: http://localhost:9000/dashboard → Save & Reload
# Or via API:
curl -X POST http://localhost:9000/api/reload
```

---

## Examples

### Example 1: Bulk Code Analysis (Batch API)

```bash
# Analyze 50 files in background
escalate-cli batch \
  --query "Analyze file1.go for security" \
  --query "Analyze file2.go for security" \
  ... (50 files total) \
  --submit

# Returns job ID: batch_abc123xyz789

# Check progress
escalate-cli batch status batch_abc123xyz789
# Status: processing (45/50 completed)

# Get results (when complete)
escalate-cli batch results batch_abc123xyz789
# Savings: 50% cost reduction
```

### Example 2: Tool Diagnostics

```bash
# Quick health check
$ escalate-tools discover
🔍 Tool Discovery Results
============================================================

📦 RTK (Real Token Killer)
  ✓ Found at: /Users/user/.local/bin/rtk
  Purpose: Command output optimization (99.4% savings)

🕸️  Scrapling (Web Scraping MCP)
  ✓ Found at: /Users/user/.local/bin/scrapling
  Token savings: 85-94% with CSS selectors

🔤 LSP Servers (Code Analysis)
  ✓ Found 3 LSP servers:
    • go: ~/.local/bin/gopls
    • python: ~/.local/bin/pylsp
    • typescript: ~/.local/bin/typescript-language-server

📚 Git
  ✓ Found at: /usr/bin/git

📊 Summary
  Discovered: 4/4 tools available
  Recommendation: Run 'escalate-tools status' to check health
```

### Example 3: Metrics Export

```bash
# Daily cost breakdown
$ escalate-metrics cost --since today
Daily Cost Report
======================
Date        Cost      Savings   Token Usage
2026-04-27  $1.45     $1.67     118,500 tokens
           (Regular would be $3.12)

# Weekly trends
$ escalate-metrics export --format json --since 7days
{
  "daily_metrics": [
    {"date": "2026-04-21", "cost": "$1.23", "tokens": 105000},
    {"date": "2026-04-22", "cost": "$1.45", "tokens": 118500},
    ...
  ],
  "weekly_total": "$9.87",
  "average_daily": "$1.41"
}
```

### Example 4: Load Testing

```bash
# Sustained 5K req/sec for 5 minutes
$ escalate-load-test --duration 5m --rate 5000 --workers 100

Load Testing Results
====================
Duration: 5m 0s
Target Rate: 5000 req/sec
Workers: 100
Total Requests: 1,500,000

Performance:
  Actual Rate: 4,995 req/sec ✓
  P50 Latency: 12ms
  P99 Latency: 47ms (target: <300ms) ✓
  P99.9 Latency: 89ms ✓

Memory:
  Baseline: 45.2 MB
  Peak: 52.8 MB
  Growth: 7.6 MB (16.8%) ✓

Goroutines:
  Baseline: 12
  Final: 14
  Growth: 2 (threshold: 5) ✓

Cache Stability:
  Hit Rate: 52.3% ✓
  False Positive Rate: 0.08% ✓

✓ All targets met. Load test passed.
```

---

## Troubleshooting

### "command not found: escalate-tools"

Install the tool:
```bash
go install github.com/szibis/claude-escalate/cmd/escalate-tools@latest
```

### "Config file not found"

Ensure config exists at one of these locations:
```bash
./config.yaml
./configs/config.yaml
~/.claude-escalate/config.yaml
```

Or auto-discover:
```bash
escalate-tools discover -v > config.yaml
```

### "RTK not found"

Install RTK (Real Token Killer):
```bash
go install github.com/szibis/rtk@latest
```

Verify:
```bash
escalate-tools status
```

### Gateway not responding

Check if gateway is running:
```bash
curl http://localhost:9000/health
```

Start gateway:
```bash
claude-escalate --config config.yaml --port 9000
```

---

## Environment Variables

```bash
# Override config location
export ESCALATE_CONFIG=~/.my-config.yaml

# Override port
export ESCALATE_PORT=8080

# Enable debug logging
export ESCALATE_DEBUG=1

# Custom tool paths
export RTK_PATH=/custom/path/to/rtk
export SCRAPLING_PATH=/custom/path/to/scrapling
```

---

## See Also

- [BATCH_API.md](BATCH_API.md) — Batch API usage guide
- [API.md](API.md) — HTTP API endpoints
- [DASHBOARD.md](DASHBOARD.md) — Dashboard features
