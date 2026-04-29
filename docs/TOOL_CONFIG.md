# Tool Configuration Guide

This guide shows how to configure custom tools via the dashboard and CLI, and how the system validates and tests them.

## Quick Start: Add a Tool via Dashboard

1. Open dashboard: `http://localhost:9000/dashboard`
2. Click **🔧 Tools** tab
3. Fill the "Add Tool" form:
   - **Tool Type**: Select from dropdown (CLI, MCP, REST, Database, Binary)
   - **Tool Name**: Enter alphanumeric + underscore name
   - **Path/Socket**: Path to executable (CLI) or socket/URL (MCP, REST, Database)
   - **Settings**: Optional key-value pairs (timeout, auth, etc.)
4. Click **Test Connection** → see health status
5. Click **Add Tool** → tool appears in table and persists to `config.yaml`

---

## Tool Types & Configuration

### CLI Tools
Run shell commands or scripts.

**Configuration Fields**:
- **Name**: Unique identifier (e.g., `my_script`)
- **Path**: Full path to executable or script (e.g., `/usr/local/bin/analyze`)
- **Args** (optional): Default command-line arguments (e.g., `["--json", "--output=stdout"]`)
- **Working Dir** (optional): Directory to run in
- **Timeout Override** (optional): Max seconds for execution (default: 30s)

**Example YAML**:
```yaml
tools:
  - name: my_analyzer
    type: cli
    path: /usr/local/bin/my-analyzer
    settings:
      args:
        - --json
        - --verbose
      timeout: 60
```

**Health Check**: Runs command with `--health-check` flag, expects exit code 0

**Invocation**: `subprocess(path, args, timeout=settings.timeout)`

---

### MCP Tools
Model Context Protocol servers via Unix socket.

**Configuration Fields**:
- **Name**: Unique identifier (e.g., `custom_mcp`)
- **Socket Path**: Unix socket location (e.g., `~/.sockets/my-mcp.sock` or `/tmp/mcp.sock`)
- **Protocol Version** (optional): "1.0" (default)
- **Auth Token** (optional): Bearer token for authentication
- **Capabilities** (optional): Declared capabilities (auto-detected if empty)

**Example YAML**:
```yaml
tools:
  - name: custom_mcp
    type: mcp
    path: ~/.sockets/custom.sock
    settings:
      protocol_version: "1.0"
      auth_token: "secret_token_here"
```

**Health Check**: Sends MCP `ping` request, expects `pong` response (timeout: 5s)

**Invocation**: `socket_connect(path, auth_token) → send request → collect response`

---

### REST Tools
HTTP REST API endpoints.

**Configuration Fields**:
- **Name**: Unique identifier (e.g., `api_analyzer`)
- **Base URL**: API endpoint (e.g., `http://localhost:8000` or `https://api.example.com`)
- **Auth Type**: "none", "bearer", "api_key", or "basic"
- **Auth Token/API Key** (conditional): Required if auth_type != "none"
- **Headers** (optional): Custom HTTP headers
- **Query Params** (optional): Default query parameters

**Example YAML**:
```yaml
tools:
  - name: api_analyzer
    type: rest
    path: https://api.example.com
    settings:
      auth_type: bearer
      api_key: "sk_prod_xxxxx"
      headers:
        X-Custom-Header: "value"
```

**Health Check**: GET `{base_url}/health`, expects HTTP 200 (timeout: 10s)

**Invocation**: `http_request(method, path, body, headers, auth, timeout)`

---

### Database Tools
SQL databases for querying.

**Configuration Fields**:
- **Name**: Unique identifier (e.g., `audit_db`)
- **Connection String**: Database URL (e.g., `postgresql://user:pass@host/db`)
- **Driver**: "postgres", "mysql", "sqlite", "sqlserver"
- **Pool Size** (optional): Connection pool size (default: 5, max: 100)
- **Query Timeout** (optional): Max seconds per query (default: 120s)

**Example YAML**:
```yaml
tools:
  - name: audit_db
    type: database
    path: postgresql://user:password@localhost/audit_db
    settings:
      driver: postgres
      pool_size: 10
      query_timeout: 120
```

**Health Check**: `SELECT 1`, expects successful result (timeout: 10s)

**Invocation**: `db_query(connection_string, query, timeout)`

---

### Binary Tools
Standalone executables (similar to CLI but without shell interpretation).

**Configuration Fields**:
- **Name**: Unique identifier (e.g., `standalone_tool`)
- **Path**: Path to binary (e.g., `/opt/tools/my-binary`)
- **Args** (optional): Command-line arguments
- **Stdin Data** (optional): Input to pass via stdin

**Example YAML**:
```yaml
tools:
  - name: standalone_tool
    type: binary
    path: /opt/tools/my-binary
    settings:
      args:
        - --analyze
        - --format=json
```

**Health Check**: Runs binary with `--health` flag, expects exit code 0

**Invocation**: `subprocess(path, args, stdin_data, timeout)`

---

## Configuration via CLI

### Add Tool
```bash
escalate-tools config add-tool \
  --name my_script \
  --type cli \
  --path /usr/local/bin/my-script \
  --setting timeout=60 \
  --setting args="--json"
```

### Edit Tool
```bash
escalate-tools config edit-tool my_script \
  --setting timeout=90
```

### Delete Tool
```bash
escalate-tools config delete-tool my_script --confirm
```

### List Tools
```bash
escalate-tools config list-tools
```

### Test Tool
```bash
escalate-tools tools test my_script
```

---

## Configuration via config.yaml

Edit `config.yaml` directly (YAML format):

```yaml
tools:
  - name: quick_grep
    type: cli
    path: /usr/bin/grep
    settings:
      args: ["-n", "--color=never"]
      timeout: 5
      working_dir: /code
      
  - name: code_mcp
    type: mcp
    path: ~/.sockets/code-analyzer.sock
    settings:
      protocol_version: "1.0"
      auth_token: "token_xyz"
      
  - name: web_api
    type: rest
    path: https://api.internal.com
    settings:
      auth_type: bearer
      api_key: "sk_live_xxxxx"
      headers:
        X-Service: "escalate"
        
  - name: metrics_db
    type: database
    path: postgresql://metrics:pass@db.internal/metrics
    settings:
      driver: postgres
      pool_size: 20
      query_timeout: 60
```

Then restart the gateway for changes to take effect:
```bash
escalate-tools restart
```

---

## Validation Rules

### Tool Name
- **Required**: Yes
- **Pattern**: `^[a-z_][a-z0-9_]*$` (lowercase alphanumeric + underscore)
- **Unique**: Must not match existing tool name
- **Length**: 1-50 characters

**Examples**: ✓ `my_analyzer`, ✓ `tool_2`, ✓ `a`, ✗ `My-Tool`, ✗ `tool-name`

### Tool Type
- **Required**: Yes
- **Valid Values**: `cli`, `mcp`, `rest`, `database`, `binary`
- **Case Sensitive**: Lowercase only

### Path/Socket
- **CLI**: Full path to executable, must exist and be executable
- **MCP**: Unix socket path (validated format), doesn't need to exist yet
- **REST**: Valid HTTP/HTTPS URL
- **Database**: Valid connection string for driver type
- **Binary**: Full path to executable, must exist and be executable

### Settings
- **Type**: Object (key-value pairs)
- **Validation**: Per-type schema (see above)
- **Optional Fields**: Can be omitted (defaults used)
- **Extra Fields**: Silently ignored (forward compatible)

---

## Health Check Behavior

### Automatic Health Checks
- **On Registration**: When tool first added (must pass to accept)
- **Periodic**: Every 5 minutes (configurable in CONFIG_SPEC.yaml)
- **On Test Click**: When user clicks "Test Connection" in dashboard
- **On Invocation**: Before using tool in Phase 2/4 (skip if recent check passed)

### Health Status Indicators
| Status | Meaning | Action |
|--------|---------|--------|
| 🟢 **OK** | Healthy, ready for use | Invoke normally |
| 🟡 **WARN** | Degraded (slow/flaky) | Use with caution |
| 🔴 **FAIL** | Unhealthy, cannot connect | Remove from rotation temporarily |
| ⚪ **UNKNOWN** | Not checked yet | Will check before first use |

### Failure Scenarios & Recovery

**Connection Refused**
- **Cause**: Tool socket not listening, endpoint down, wrong path
- **Recovery**: Retry 3 times with 5s delay; mark unhealthy; retry after 5 minutes
- **User Action**: Start tool service, verify path/endpoint

**Timeout**
- **Cause**: Tool slow to respond (network latency, processing delay)
- **Recovery**: Kill request, try next tool in selection list
- **User Action**: Increase timeout in settings, optimize tool performance

**Invalid Response**
- **Cause**: Tool returned unexpected format
- **Recovery**: Log error, try next tool
- **User Action**: Update tool to return proper format

**Authentication Failed (401/403)**
- **Cause**: Auth token expired or invalid
- **Recovery**: Log warning, disable tool until token refreshed
- **User Action**: Update auth token in settings

---

## Common Use Cases

### Add System Monitoring Tool
```yaml
tools:
  - name: system_stats
    type: cli
    path: /usr/bin/top
    settings:
      args: ["-b", "-n", "1", "-u", "$USER"]
      timeout: 10
```

### Add Web Content Fetcher
```yaml
tools:
  - name: fetch_docs
    type: rest
    path: https://docs.internal.com
    settings:
      auth_type: none
      headers:
        User-Agent: "escalate-tool/1.0"
```

### Add Code Analysis Tool
```yaml
tools:
  - name: lint_checker
    type: cli
    path: /usr/local/bin/eslint
    settings:
      args: ["--format=json"]
      working_dir: /code
      timeout: 30
```

### Add Database Query Tool
```yaml
tools:
  - name: query_logs
    type: database
    path: postgresql://loguser:pass@logs.internal/production
    settings:
      driver: postgres
      query_timeout: 60
      pool_size: 5
```

### Add MCP Code Analysis Server
```yaml
tools:
  - name: code_analyzer_mcp
    type: mcp
    path: ~/.sockets/code-analyzer.sock
    settings:
      protocol_version: "1.0"
      capabilities: ["search_code", "get_definition", "find_references"]
```

---

## Troubleshooting Tool Configuration

### Tool Shows "Path Does Not Exist"
**Problem**: Dashboard rejects CLI tool path
**Solution**:
1. Verify path is absolute: `/usr/local/bin/tool` not `./tool`
2. Check file exists: `ls -l /usr/local/bin/tool`
3. Check executable: `file /usr/local/bin/tool` should show "executable"
4. Check permissions: `chmod +x /usr/local/bin/tool`

### Health Check Fails Immediately
**Problem**: "Test Connection" shows failure
**Solution**:
1. **CLI Tool**: Run manually to verify: `/path/to/tool --health-check`
2. **MCP Tool**: Check socket exists: `ls -l ~/.sockets/tool.sock`
3. **REST Tool**: Test endpoint: `curl -H "Authorization: Bearer TOKEN" https://endpoint/health`
4. **Database**: Test connection: `psql -c "SELECT 1" postgresql://...`

### Tool Settings Not Applied
**Problem**: Changed settings via dashboard, but not used
**Solution**:
1. Click "Test Connection" after updating settings
2. Restart gateway: `escalate-tools restart`
3. Verify in config.yaml: `cat ~/.escalate/config.yaml | grep tool_name -A 5`

### Tool Invocation Timeouts
**Problem**: Tool takes too long, request times out
**Solution**:
1. Increase timeout: Edit settings, raise `timeout_override` value
2. Check tool performance: Run tool manually with `time` command
3. Check network: `ping` remote endpoint if REST/Database tool
4. Scale up: Increase connection pool or worker threads

---

## Best Practices

### 1. Use Meaningful Names
❌ Bad: `t1`, `tool`, `x`
✅ Good: `security_scanner`, `web_fetcher`, `metrics_db`

### 2. Test Before Adding to Config
```bash
# Manual test
/path/to/tool --health-check && echo "OK" || echo "FAIL"

# Then add via dashboard and click "Test Connection"
```

### 3. Set Appropriate Timeouts
- **Fast tools** (grep, simple API): 5-10 seconds
- **Medium tools** (full analysis, deep queries): 30-60 seconds
- **Slow tools** (comprehensive scans): 120+ seconds

### 4. Monitor Tool Health
```bash
# Check all tools
escalate-tools tools status

# Get audit log
escalate-tools tools audit-log | tail -20
```

### 5. Document Custom Tools
Add comments to `config.yaml`:
```yaml
# Analytics database - queries production metrics
# Owner: analytics-team
# On-call runbook: https://wiki.internal/analytics-runbook
- name: metrics_db
  type: database
  path: postgresql://...
```

---

## What's Next

See [TOOL_INVOCATION.md](./TOOL_INVOCATION.md) for how tools are selected and invoked during escalation.

See [CONFIG_SPEC.yaml](./CONFIG_SPEC.yaml) for complete specification reference.
