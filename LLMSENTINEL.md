# LLMSentinel Developer Guide

Multi-provider LLM orchestration platform for Claude, Gemini, OpenAI, and Copilot.

## System Requirements

- **Go 1.26.2** (required, enforced in `go.mod`)
- **SQLite 3.x** (for analytics storage)
- **Make** (optional, for helper targets)

## Quick Start (5 min)

```bash
# 1. Clone and enter the project
git clone https://github.com/szibis/LLMSentinel.git
cd LLMSentinel

# 2. Verify Go version
go version  # Should be go1.26.2

# 3. Download dependencies
go mod download

# 4. Build the binary
go build -o llm-sentinel ./cmd/llm-sentinel

# 5. Run tests
go test ./...

# 6. Start the service
./llm-sentinel service --port 9000
```

## Local Development

### Build & Test

```bash
# Build binary
go build -o llm-sentinel ./cmd/llm-sentinel

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
LLMSENTINEL_LOG_LEVEL=debug ./llm-sentinel service --port 9000

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
- **`internal/providers/`** - Provider implementations (Claude, Gemini, OpenAI, Copilot)
- **`internal/sentiment/`** - Multi-provider sentiment detection
- **`internal/budgets/`** - Per-provider budget enforcement
- **`internal/execlog/`** - Unified execution logging
- **`internal/analytics/`** - Cross-provider analytics (SQLite)
- **`internal/patterns/`** - Auto-generated optimization patterns
- **`cmd/llm-sentinel/`** - Main CLI entry point

### Multi-Provider Architecture

LLMSentinel uses a provider abstraction layer to support multiple cloud AI CLIs:

```
User Application
    ↓
Provider Interface (abstract)
    ├─ ClaudeProvider
    ├─ GeminiProvider
    ├─ OpenAIProvider
    └─ CopilotProvider
    ↓
Unified Execution Engine
    ├─ Logging
    ├─ Analytics
    ├─ Sentiment Detection
    ├─ Model Escalation
    └─ Budget Enforcement
```

Each provider implements the `Provider` interface with model-specific logic. The execution engine remains provider-agnostic.

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

## Contributing

### Code Review Checklist

Before submitting a PR, ensure:

- [ ] `go vet ./...` passes (zero warnings)
- [ ] Tests added for new features (run with `go test -v ./...`)
- [ ] Provider-agnostic code when applicable
- [ ] Multi-provider testing (at least 2 providers tested)
- [ ] New code follows existing patterns (see similar files for style)

### Security Considerations

**API Keys & Tokens**:
- Never log authentication credentials
- Store API keys in environment variables
- Use secure credential storage per provider

**Cross-Provider Data**:
- Sanitize logs across all providers
- Validate token counts from each provider API
- Document cost calculation methodology per provider

**Configuration**:
- Validate provider configurations at startup
- Support provider-specific security settings
- Test authentication for all enabled providers

For full security documentation, see [docs/security/SECURITY.md](docs/security/SECURITY.md).

## Configuration

### Basic Setup (Multi-Provider)

```yaml
# ~/.llmsentinel/config.yaml

providers:
  claude:
    enabled: true
    auth_key_var: ANTHROPIC_API_KEY
    budgets:
      daily_usd: 5.00
      monthly_usd: 100.00
  
  gemini:
    enabled: true
    auth_key_var: GCLOUD_API_KEY
    budgets:
      daily_usd: 10.00
      monthly_usd: 200.00
  
  openai:
    enabled: false
    auth_key_var: OPENAI_API_KEY
    budgets:
      daily_usd: 2.00
      monthly_usd: 50.00
  
  copilot:
    enabled: false
    auth_key_var: GH_TOKEN
    budgets:
      daily_requests: 100

execution:
  fallback:
    enabled: true
    order: ["gemini", "openai"]
    max_depth: 2
```

## Environment Variables

```bash
LLMSENTINEL_LOG_LEVEL=debug              # Set logging level
LLMSENTINEL_CONFIG_PATH=...              # Override config file path
LLMSENTINEL_DB_PATH=...                  # Override analytics database path
ANTHROPIC_API_KEY=...                    # Claude authentication
GCLOUD_API_KEY=...                       # Gemini authentication
OPENAI_API_KEY=...                       # OpenAI authentication
GH_TOKEN=...                             # Copilot authentication
```

## Testing

### Unit Tests

```bash
go test -v ./...
```

### Integration Tests (Multi-Provider)

```bash
# Run with all enabled providers
go test -v -run Integration ./internal/analytics
```

### Sentiment Detection Tests

```bash
go test -v ./internal/sentiment -run TestDetect
```

### Budget Calculation Tests (Per-Provider)

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

### "Provider authentication failed"

Ensure environment variables are set:
```bash
# Check Claude
echo $ANTHROPIC_API_KEY

# Check Gemini
echo $GCLOUD_API_KEY

# Check OpenAI
echo $OPENAI_API_KEY

# Check Copilot
echo $GH_TOKEN
```

### "SQLite database locked"

Ensure only one service instance is running:
```bash
pkill -f "llm-sentinel service"
./llm-sentinel service --port 9000
```

## Execution Feedback Loop: Patterns & Optimization

### Auto-Generated Execution Patterns

The system logs all direct operations (across all providers) to `.execution-log.jsonl`. This creates project-specific optimization guides:

**Auto-Generated Files:**
- **`.execution-log.jsonl`** (gitignored) — Unified operation log with provider, cost, duration
- **`EXECUTION_PATTERNS.md`** (checked in) — Cross-provider optimization guide

**Pattern Generation:**
```bash
# Auto-triggers after 50 operations or on-demand:
llm-sentinel generate-patterns  # Reads .execution-log.jsonl, generates EXECUTION_PATTERNS.md
```

### Multi-Provider Patterns

Patterns include provider-specific insights:
- Fast operations per provider (cache candidates)
- Slow operations per provider (optimization targets)
- Cost comparison (which provider is cheapest for this task?)
- Cross-provider trends (when to escalate between providers)

### Session Startup Integration

At conversation start:
1. **Reads** `EXECUTION_PATTERNS.md` if it exists
2. **Learns** cross-provider patterns and cost tradeoffs
3. **Adapts** provider selection based on learned patterns

**Example patterns:**
- "Claude Haiku costs 0.003/1K, Gemini Flash costs 0.075/1M (60x cheaper for simple tasks)"
- "Claude Opus takes 2.3s avg, Gemini Pro takes 1.5s (faster but more expensive)"
- "Budget hit on Claude? Fallback to Gemini for 100x cost savings"

### Analytics Dashboard

Execution analytics available at `http://localhost:9000/analytics`:
- Real-time operation metrics (per provider)
- Cost breakdown by provider
- Cross-provider comparison
- Optimization opportunities
- Performance trends

### Analytics CLI

```bash
# Session summary (all providers)
llm-sentinel analytics --summary

# Show slowest 10 operations (all providers)
llm-sentinel analytics --slowest 10

# Show by provider
llm-sentinel analytics --by-provider

# Show cost breakdown
llm-sentinel analytics --costs

# Show optimization recommendations
llm-sentinel analytics --recommendations
```

### Privacy & Security

Execution logging respects user privacy:
- **Auto-filtered**: Commands with API keys, tokens, passwords are redacted
- **Path normalization**: File paths normalized to `<path>`
- **Gitignore**: `.execution-log.jsonl` is gitignored (contains runtime data)
- **Provider-aware**: Respects each provider's privacy requirements
- **User control**: Configure exclusion patterns via environment variables

## Resources

- [LLMSentinel README](README.md) - Project overview
- [Multi-CLI Feasibility Analysis](docs/MULTI_CLI_FEASIBILITY_ANALYSIS.md) - Detailed provider analysis
- [Multi-CLI Architecture](docs/MULTI_CLI_ARCHITECTURE.md) - Technical architecture
- [Implementation Roadmap](docs/IMPLEMENTATION_ROADMAP.md) - Development timeline
- [Contributing Guide](CONTRIBUTING.md) - Development standards
- [Security Documentation](docs/security/SECURITY.md) - Security considerations
- [API Documentation](docs/api/endpoints.md) - REST API reference
