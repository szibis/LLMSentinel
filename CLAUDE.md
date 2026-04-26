# Claude Escalate Developer Guide

## System Requirements

- **Go 1.26.2** (required, enforced in `go.mod`)
- **SQLite 3.x** (for analytics storage)
- **Make** (optional, for helper targets)

## Quick Start (5 min)

```bash
# 1. Clone and enter the project
git clone <repo-url>
cd claude-escalate

# 2. Verify Go version
go version  # Should be go1.26.2

# 3. Download dependencies
go mod download

# 4. Build the binary
go build -o escalate ./cmd/claude-escalate

# 5. Run tests
go test ./...

# 6. Start the service
./escalate service --port 9000
```

## Local Development

### Build & Test

```bash
# Build binary
go build -o escalate ./cmd/claude-escalate

# Run tests with coverage
go test -v -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Run specific test
go test -v -run TestSentimentDetector ./internal/sentiment

# Run tests with race detector
go test -race ./...
```

### Linting & Type Checking

The project uses golangci-lint for linting. Due to Go 1.26.2 compatibility constraints:

```bash
# Run linting (uses build-from-source workaround)
# See .github/workflows/build.yml for the full approach
make lint  # If Makefile available, otherwise use manual approach

# Manual linting (build from source)
git clone --depth 1 https://github.com/golangci/golangci-lint.git /tmp/golangci-lint
cd /tmp/golangci-lint
make build
./golangci-lint run ../..
```

### Type Checking

```bash
# Go vet (always clean before committing)
go vet ./...

# Check for static issues
staticcheck ./...
```

### Debugging

```bash
# Enable debug logging
ESCALATION_LOG_LEVEL=debug ./escalate service --port 9000

# Get analytics for validation ID
curl http://localhost:9000/api/analytics/phase-1/{validation_id}
curl http://localhost:9000/api/analytics/phase-2/{validation_id}
curl http://localhost:9000/api/analytics/phase-3/{validation_id}

# Sentiment trends
curl http://localhost:9000/api/analytics/sentiment-trends?hours=24

# Budget status
curl http://localhost:9000/api/analytics/budget-status
```

## Architecture Overview

### Key Modules

- **`internal/config/`** - Configuration loading (YAML, environment variables)
- **`internal/sentiment/`** - Sentiment detection engine with pattern matching
- **`internal/budgets/`** - Token budget enforcement and tracking
- **`internal/statusline/`** - Multi-source statusline integration (webhook, file, native, etc.)
- **`internal/analytics/`** - Analytics persistence (SQLite)
- **`internal/decisions/`** - Model routing and escalation decisions
- **`cmd/claude-escalate/`** - Main CLI entry point

### Multi-Source Statusline Integration

The statusline system supports multiple data sources (Barista, Claude native, webhooks, environment variables, files). Sources are prioritized, with fallback if primary unavailable:

```bash
# Register a source in config.yaml
statusline:
  sources:
    - type: barista
      enabled: true
      path: ~/.claude/barista.conf
    - type: webhook
      enabled: true
      url: https://your-service.com/metrics
      token: your-auth-token
    - type: file
      enabled: true
      path: ~/.claude/data/escalation/statusline.json
```

### Adding a New Statusline Source

To add a new statusline source:

1. Implement the `StatuslineSource` interface in `internal/statusline/statusline.go`:
   ```go
   type StatuslineSource interface {
     Name() string              // e.g., "barista", "webhook"
     IsAvailable() bool         // Is this source working?
     Priority() int             // Higher = tried first
     Poll() (StatuslineData, error)  // Get current metrics
   }
   ```

2. Create a new file `internal/statusline/yoursource.go` with implementation

3. Register it in the registry (typically in service initialization)

4. Add configuration support in `internal/config/config.go`

5. Add tests in `internal/statusline/yoursource_test.go`

## Known Issues & Workarounds

### Go 1.26.2 Lint Compatibility

**Issue**: golangci-lint v1.x (released binaries) were built with Go 1.23-1.24, incompatible with Go 1.26.2 compiler.

**Current Workaround**: `.github/workflows/build.yml` builds golangci-lint from source at CI time:
```bash
git clone --depth 1 https://github.com/golangci/golangci-lint.git /tmp/golangci-lint
cd /tmp/golangci-lint
make build
./golangci-lint run
```

**Cost**: Adds 5-10 minutes to each CI run.

**Longer-term**: Monitor [golangci-lint releases](https://github.com/golangci/golangci-lint/releases) for v2.0 with native Go 1.26 support (likely Q2-Q3 2026).

**Disabled Linters** (planned for re-enabling in v3.0.1):

- **`errcheck`** - ~30 unchecked errors in v3.0.0 code. Requires refactoring error handling throughout sentiment, budgets, and analytics modules.
  - Planned fix: Add proper error returns and logging, remove silent error suppressions.

- **`gosec`** - Security linter disabled to prioritize security fixes separately. 5 security findings identified:
  - See [docs/security/SECURITY.md](docs/security/SECURITY.md) for remediation roadmap
  - Planned re-enable: After CRITICAL and HIGH severity fixes applied (v3.0.1)

## Contributing

### Code Review Checklist

Before submitting a PR, ensure:

- [ ] `go vet ./...` passes (zero warnings)
- [ ] Tests added for new features (run with `go test -v ./...`)
- [ ] No unchecked errors (will be enforced in v3.0.1 with errcheck linter)
- [ ] No new gosec warnings (security must be validated first)
- [ ] Sentiment detection tested with diverse input vectors
- [ ] Budget calculations tested with edge cases (zero, negative, overflow)
- [ ] New code follows existing patterns (see similar files for style)

### Security Considerations

**Webhook URLs**:
- Must be HTTPS only (localhost/127.0.0.1 rejected for SSRF protection)
- Validated at configuration time

**JSON Deserialization**:
- Use `json.Decoder` with `DisallowUnknownFields()`
- Validate all numeric fields for range and sign before use
- Check nil pointers before dereferencing

**File Paths**:
- Restricted to `~/.claude/data/escalation/` directory (path traversal protection)
- Use `filepath.Abs` and validate before opening

**Sentiment Detection**:
- Regex patterns should be pre-compiled and timeout-protected

For full security remediation roadmap, see [docs/security/SECURITY.md](docs/security/SECURITY.md).

## Configuration

### Basic Setup

```yaml
# ~/.claude/escalation/config.yaml

statusline:
  sources:
    - type: barista
      enabled: true
    - type: webhook
      enabled: false
      url: https://example.com/metrics
      token: bearer_token_here

budgets:
  daily_usd: 10.00
  monthly_usd: 100.00
  session_tokens: 10000
  hard_limit: false
  soft_limit: true
  auto_downgrade_at: 0.80

sentiment:
  enabled: true
  frustration_trigger_escalate: true
  learning_enabled: true

decisions:
  auto_escalate_on_frustration: true
  max_attempts_before_opus: 2

logging:
  level: info
```

## Environment Variables

```bash
ESCALATION_LOG_LEVEL=debug  # Set logging level
ESCALATION_CONFIG_PATH=...  # Override config file path
ESCALATION_DB_PATH=...      # Override analytics database path
```

## Testing

### Unit Tests

```bash
go test -v ./...
```

### Integration Tests

```bash
# Run with database
go test -v -run Integration ./internal/analytics
```

### Sentiment Detection Tests

```bash
go test -v ./internal/sentiment -run TestDetect
```

### Budget Calculation Tests

```bash
go test -v ./internal/budgets -run TestBudget
```

## Troubleshooting

### "Build failed: golangci-lint not found"

Run from inside the repository:
```bash
git clone --depth 1 https://github.com/golangci/golangci-lint.git /tmp/golangci-lint
cd /tmp/golangci-lint && make build && cd -
/tmp/golangci-lint/golangci-lint run ./...
```

### "Webhook validation failed: URL must be HTTPS"

Ensure webhook URLs use HTTPS:
```bash
# Wrong
webhook_url: http://localhost:8080/metrics  # ❌ HTTP not allowed

# Right
webhook_url: https://api.example.com/metrics  # ✅ HTTPS required
```

### "SQLite database locked"

Ensure only one service instance is running:
```bash
pkill -f "escalate service"
./escalate service --port 9000
```

## Resources

- [Claude Escalate README](README.md) - Project overview
- [Contributing Guide](CONTRIBUTING.md) - Development standards
- [Security Documentation](docs/security/SECURITY.md) - Security considerations
- [API Documentation](docs/api/endpoints.md) - REST API reference
