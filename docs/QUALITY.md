# Code Quality Standards

Claude Escalate maintains high quality standards across testing, documentation, and code style.

---

## Testing Requirements

### Coverage Targets

| Component | Minimum | Current Status |
|-----------|---------|---|
| **Sentiment Detection** | 80% | ✅ Comprehensive |
| **Budget Enforcement** | 85% | ✅ Comprehensive |
| **Statusline Integration** | 75% | ✅ Good |
| **Service/API** | 70% | ✅ Good |
| **Overall** | 75% | ✅ On Target |

### Test Types

1. **Unit Tests** (test each function in isolation)
   - Sentiment pattern matching
   - Budget calculation accuracy
   - Statusline source selection
   - Configuration loading/saving

2. **Integration Tests** (test systems working together)
   - Full 3-phase flow (estimate → track → validate)
   - Sentiment + Budget combined logic
   - Multi-source statusline fallback
   - End-to-end CLI commands

3. **Regression Tests** (prevent known issues)
   - Frustration false positives ("thanks" in angry context)
   - Budget rounding errors
   - Statusline timing issues
   - Configuration edge cases

4. **Benchmarks** (performance verification)
   - Sentiment detection latency
   - Budget check performance
   - Registry source selection speed

### Running Tests

```bash
# Full test suite with coverage
./scripts/test.sh --coverage --verbose

# Just run tests
go test -v ./...

# With race condition detection
go test -race ./...

# Run specific package
go test -v ./internal/sentiment/

# Generate coverage report
go test -cover ./... && go tool cover -html=coverage.out
```

### Test Requirements for PRs

Before submitting a pull request:

- [ ] All existing tests pass: `go test -v ./...`
- [ ] No race conditions: `go test -race ./...`
- [ ] Coverage maintained or improved
- [ ] New features include tests
- [ ] Regression tests added for bug fixes

---

## Code Style Standards

### Go Code Style

Follow [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments):

- Use meaningful variable names
- Keep functions focused (single responsibility)
- Avoid unexported functions when public needed
- Document exported functions
- Use interfaces for abstraction

### Example: Good Style

```go
// Good: Clear intent, single responsibility
func (d *Detector) Detect(prompt string) Score {
    score := Score{Primary: SentimentNeutral}
    
    for sentiment, patterns := range d.patterns {
        if d.matchesPatterns(prompt, patterns) {
            score.Primary = sentiment
            score.Confidence = 0.8
            break
        }
    }
    
    return score
}

// Good: Helper function is unexported, clear name
func (d *Detector) matchesPatterns(prompt string, patterns []*regexp.Regexp) bool {
    for _, p := range patterns {
        if p.MatchString(prompt) {
            return true
        }
    }
    return false
}
```

### Example: Avoid

```go
// Avoid: Unclear intent, mixed concerns
func D(p string) int {
    r := 0
    for _, pat := range pats {
        if pat.Match([]byte(p)) {
            r++
        }
    }
    return r
}
```

### Documentation Standards

Every exported function must have a comment:

```go
// Detect analyzes a prompt for sentiment signals.
// Returns a Score with Primary sentiment and FrustrationRisk (0.0-1.0).
func (d *Detector) Detect(prompt string) Score {
    // ...
}
```

### Error Handling

Handle errors explicitly:

```go
// Good: Clear error handling
budget, err := LoadBudgetConfig()
if err != nil {
    log.Fatalf("failed to load budget config: %v", err)
}

// Avoid: Silent failures
budget, _ := LoadBudgetConfig()  // Wrong! Error ignored
```

---

## Performance Standards

### Target Latencies

| Operation | Target | Acceptable |
|-----------|--------|---|
| Sentiment detection | <1ms | <5ms |
| Budget check | <100µs | <500µs |
| Service response | <100ms | <500ms |
| Dashboard load | <2s | <5s |

### Memory Usage

| Component | Target | Acceptable |
|-----------|--------|---|
| Service binary | <100MB | <200MB |
| Runtime memory | <100MB | <300MB |
| Database per 1k records | <10MB | <50MB |

### Testing Performance

```bash
# Run benchmarks
go test -bench=. -benchmem ./...

# Profile memory
go test -bench=. -memprofile=mem.prof ./...
go tool pprof mem.prof
```

---

## Documentation Standards

### Required Documentation

1. **README.md** - Project overview and quick start
2. **docs/** - Organized documentation by topic
3. **Code comments** - Every exported function and package
4. **Commit messages** - Clear, descriptive
5. **API documentation** - Endpoint specs in code

### Doc Structure

```
docs/
├── README.md                    # Main index
├── quick-start/                 # 5-min setup guides
│   ├── 5-minute-setup.md
│   ├── first-escalation.md
│   └── budgets-setup.md
├── architecture/                # How it works
│   ├── overview.md
│   ├── 3-phase-flow.md
│   └── sentiment-detection.md
├── integration/                 # Configuration
│   ├── sentiment-detection.md
│   ├── budgets.md
│   └── api-reference.md
├── operations/                  # Deployment & monitoring
│   ├── deployment.md
│   ├── monitoring.md
│   └── troubleshooting.md
└── analytics/                   # Data & visualization
    ├── dashboards.md
    ├── cost-analysis.md
    └── recommendations.md
```

---

## Linting & Formatting

### Go Lint

```bash
# Install
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Run linter
golangci-lint run ./...

# Fix issues
golangci-lint run --fix ./...
```

### Format Code

```bash
# Format all files
gofmt -w .

# Or use gopls
gopls format -w .
```

### Configuration

```yaml
# .golangci.yml
linters:
  enable:
    - vet
    - fmt
    - staticcheck
    - errcheck
    - ineffassign
```

---

## Commit Quality

### Commit Message Format

```
Summarize changes in one line (50 chars or less)

More detailed explanation of your changes. Explain
what problem this solves and how it solves it.

Related issues: Closes #123
```

### Good Commit Examples

✅ Good:
```
Add sentiment-based escalation for frustrated users

Detect frustration keywords and automatically escalate
to Sonnet/Opus when user frustration_risk > 0.70.
Includes learning patterns for future optimization.

Closes #45
```

❌ Avoid:
```
Fixed stuff
Update code
WIP
asdf
```

---

## Code Review Checklist

When reviewing code:

- [ ] All tests pass: `go test -v ./...`
- [ ] No race conditions: `go test -race ./...`
- [ ] Coverage maintained/improved
- [ ] Functions documented
- [ ] Error handling present
- [ ] Variable names clear
- [ ] No unnecessary complexity
- [ ] Performance acceptable
- [ ] Consistent with style guide

---

## Continuous Integration

### GitHub Actions

Automatic checks on every PR:

1. **Tests**: `go test -v -race ./...`
2. **Coverage**: Must maintain or improve
3. **Lint**: Must pass golangci-lint
4. **Build**: Must compile for all platforms

### Local Pre-Commit

```bash
# Run before committing
go test -race ./... && golangci-lint run ./...
```

---

## Releasing

### Release Checklist

- [ ] All tests pass
- [ ] Coverage >= 75%
- [ ] No linting issues
- [ ] Documentation updated
- [ ] Changelog updated
- [ ] Version bumped (semantic versioning)
- [ ] Build successful for all platforms
- [ ] Release notes written

### Version Numbers

Use [Semantic Versioning](https://semver.org/):
- **MAJOR**: Breaking API changes (e.g., 1.0.0 → 2.0.0)
- **MINOR**: New features, backward compatible (e.g., 1.0.0 → 1.1.0)
- **PATCH**: Bug fixes (e.g., 1.0.0 → 1.0.1)

---

## See Also

- [Testing Guide](tests/test_README.md) — Detailed testing documentation
- [Contributing](CONTRIBUTING.md) — Contribution guidelines
- [API Reference](docs/integration/api-reference.md) — API specifications
