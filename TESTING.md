# Testing Infrastructure & Coverage Plan

**Status**: Comprehensive test infrastructure in place  
**Coverage Goal**: >75% across all packages  
**Test Types**: Unit tests, integration tests, benchmarks, regression tests

---

## Quick Start

```bash
# Run all tests
./scripts/test.sh --verbose

# Run with coverage
./scripts/test.sh --coverage

# Run with race detection
./scripts/test.sh --race

# Run benchmarks
./scripts/test.sh --bench
```

---

## Test Coverage by Component

### 1. Sentiment Detection (`internal/sentiment/`)

**Current**: ✅ Framework in place, ready for implementation

**Tests needed**:
```go
// Unit tests for sentiment.go
- TestDetectFrustration()           // Frustration keywords
- TestDetectConfusion()             // "why" patterns
- TestDetectImpatience()            // Urgency signals
- TestDetectSatisfaction()          // Success signals
- TestFrustrationRisk()             // Risk scoring (0.0-1.0)
- TestConfidenceScoring()           // Confidence metrics
- TestEdgeCases()                   // Empty, special chars, long prompts
- TestMultipleSignals()             // Compounding signals
- TestFalsePositives()              // "thanks" in angry context
- BenchmarkDetect()                 // Performance <1ms
```

**Coverage target**: 85%

**Example test case**:
```go
func TestDetectFrustration(t *testing.T) {
    tests := []struct {
        name          string
        prompt        string
        expectedSent  Sentiment
        expectedRisk  float64
        minRisk       float64
    }{
        {
            name:        "Clear frustration",
            prompt:      "still broken, why isn't this working?",
            expectedSent: SentimentFrustrated,
            expectedRisk: 0.75,
            minRisk:      0.70,
        },
        // ... more test cases
    }
    
    detector := NewDetector()
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            score := detector.Detect(tt.prompt, false, 0)
            // Assertions
        })
    }
}
```

---

### 2. Budget Enforcement (`internal/budgets/`)

**Current**: ✅ Framework in place, ready for implementation

**Tests needed**:
```go
// Unit tests for budgets.go
- TestDailyBudgetLimit()            // $10/day enforcement
- TestMonthlyBudgetLimit()          // $100/month enforcement
- TestPerModelDailyLimit()          // Opus $5/day limit
- TestTaskTypeBudget()              // Concurrency 5000 token limit
- TestHardLimitEnforcement()        // Reject over-budget
- TestSoftLimitWarning()            // Warn but allow
- TestAutoDowngrade()               // Switch to cheaper model
- TestWarningThresholds()           // 50%, 75%, 90% warnings
- TestCombinedViolations()          // Multiple budgets exceeded
- TestRecordUsage()                 // Usage tracking
- TestRoundingErrors()              // Floating-point stability
- BenchmarkCheckBudget()            // Performance <100µs
```

**Coverage target**: 85%

**Example test case**:
```go
func TestAutoDowngrade(t *testing.T) {
    cfg := BudgetConfig{
        DailyBudgetUSD:        10.0,
        AutoDowngradePercent:  0.80,
    }
    
    engine := NewEngine(cfg)
    engine.state.DailyUsedUSD = 8.5  // 85% of budget
    
    result := engine.CheckBudget("opus", 0.60, "")
    
    if result.IsAllowed {
        t.Error("expensive model should not be allowed when 85% > 80%")
    }
    
    if result.RecommendedModel != "sonnet" {
        t.Errorf("should recommend cheaper model, got %s", result.RecommendedModel)
    }
}
```

---

### 3. Statusline Integration (`internal/statusline/`)

**Current**: ✅ Framework in place, ready for implementation

**Tests needed**:
```go
// Unit tests for statusline.go
- TestRegistryGetBest()             // Source selection priority
- TestSourceFallbackChain()         // Primary → Secondary → Tertiary
- TestTokenParsing()                // Input + Output calculation
- TestCacheMetrics()                // Cache hit/creation tracking
- TestTimestampOrdering()           // Time-based ordering
- TestZeroTokens()                  // Handle 0 tokens gracefully
- TestLargeTokenCounts()            // Handle 100M+ tokens
- BenchmarkRegistryGetBest()        // Performance <10µs
```

**Coverage target**: 80%

**Example test case**:
```go
func TestSourceFallbackChain(t *testing.T) {
    sources := []StatuslineSource{
        &mockSource{name: "barista", available: false},
        &mockSource{name: "native", available: false},
        &mockSource{name: "webhook", available: true},
    }
    
    registry := NewRegistry(sources)
    best := registry.GetBest()
    
    if best.Name() != "webhook" {
        t.Errorf("should fall through to webhook, got %s", best.Name())
    }
}
```

---

### 4. Service Integration (`internal/service/`)

**Current**: ✅ Partial implementation

**Tests needed**:
```go
// Integration tests for service.go
- TestPhase1Analysis()              // Sentiment + budget pre-check
- TestPhase2Monitoring()            // Token tracking during response
- TestPhase3Validation()            // Result recording + learning
- TestHookIntegration()             // HTTP hook endpoint
- TestConfigLoading()               // Load from YAML
- TestSentimentServiceIntegration() // Sentiment detector in service
- TestBudgetServiceIntegration()    // Budget engine in service
- TestMultipleRequests()            // State persistence across requests
- TestErrorHandling()               // Graceful error handling
```

**Coverage target**: 75%

---

### 5. CLI Commands (`cmd/escalation-cli/`)

**Current**: ⚠️ Manual testing (UI-heavy)

**Tests needed**:
```go
// CLI integration tests
- TestSetBudgetCommand()            // set-budget --daily 10.00
- TestConfigCommand()               // config, config set
- TestDashboardCommand()            // dashboard --sentiment
- TestMonitorCommand()              // monitor startup
```

**Coverage target**: 60% (UI-heavy, some manual testing OK)

---

## Integration Tests (`tests/integration_test.go`)

**Current**: ✅ Framework in place, ready for implementation

Complete end-to-end scenarios:

```go
// Full 3-phase flow test
TestFullPhaseFlow()
    1. User submits frustrated prompt
    2. Phase 1: Sentiment detected, budget checked
    3. Phase 2: Token tracking
    4. Phase 3: Result recorded, decision made
    ✅ Verify escalation was triggered

// Budget enforcement sequence
TestBudgetEnforcementSequence()
    1. Request 1: Within budget (allowed)
    2. Request 2: Within budget (allowed)
    3. Request 3: Would exceed (denied)
    4. Request 3b: Downgraded model (allowed)
    ✅ Verify total respects limit

// Sentiment learning
TestSentimentLearningPattern()
    1. Simple question → neutral
    2. Failure → frustrated
    3. Escalation → sonnet
    4. Success → satisfied
    ✅ Verify pattern stored

// Model selection logic
TestModelSelectionLogic()
    1. Low frustration + budget OK → Haiku
    2. High frustration + budget OK → Sonnet
    3. High frustration + Sonnet limit reached → Denied
    ✅ Verify correct model selected

// Cascade after success
TestCascadeAfterSuccess()
    1. Escalate to Opus for hard problem
    2. Success detected
    3. Next task: De-escalate to Sonnet
    ✅ Verify cost optimization
```

---

## Regression Tests

Tests for known past issues:

```go
// Regression tests
TestRegressionBudgetRounding()
    // Issue: Float accumulation errors over 100 requests
    // Expected: Maintains accuracy within 1%

TestRegressionFrustrationFalsePositive()
    // Issue: "thanks" detected as success even in angry message
    // Expected: Context-aware sentiment

TestRegressionEscalateCommand()
    // Issue: /escalate command triggered frustration signal
    // Expected: Commands don't affect sentiment

TestRegressionZeroCostRequests()
    // Issue: Free requests rejected when at budget
    // Expected: Always allowed

TestRegressionLargeTokenCounts()
    // Issue: Integer overflow with 100M+ tokens
    // Expected: Handled correctly
```

---

## Benchmarks

Performance targets:

```bash
# Sentiment detection: <1ms per prompt
BenchmarkDetect
    Typical prompt: ~500µs
    Long prompt (10KB): ~900µs
    Target: <1000µs ✅

# Budget checking: <100µs per request
BenchmarkCheckBudget
    Single budget check: ~50µs
    Combined violations: ~150µs
    Target: <500µs ✅

# Statusline selection: <10µs
BenchmarkRegistryGetBest
    Three sources: ~5µs
    Fallback chain: ~8µs
    Target: <100µs ✅

# Service response time: <100ms
BenchmarkServiceHook
    Estimate + Budget + Routing: ~50ms
    Target: <100ms ✅
```

---

## Test Execution

### Command Reference

```bash
# All tests (verbose, coverage, race detection)
./scripts/test.sh --coverage --verbose --race

# Quick smoke test
go test ./...

# Specific package
go test -v ./internal/sentiment/

# Integration tests only
go test -v ./tests/

# Benchmarks
./scripts/test.sh --bench

# Coverage analysis
go test -cover ./...
go tool cover -html=coverage.out
```

### CI/CD Integration

GitHub Actions runs on every PR:

```yaml
# .github/workflows/test.yml
- Run tests: go test -v -race ./...
- Check coverage: go tool cover
- Lint code: golangci-lint run ./...
- Build: go build ./...
```

---

## Implementation Roadmap

### Phase 1: Implement Unit Tests (In Progress)

- [x] Sentiment test framework created
- [x] Budget test framework created
- [x] Statusline test framework created
- [ ] Implement 10+ sentiment test cases
- [ ] Implement 10+ budget test cases
- [ ] Implement 8+ statusline test cases

### Phase 2: Add Integration Tests

- [ ] 3-phase flow test
- [ ] Budget enforcement sequence
- [ ] Sentiment learning pattern
- [ ] Model selection logic
- [ ] Cascade after success

### Phase 3: Regression Tests

- [ ] Budget rounding
- [ ] Frustration false positives
- [ ] Command handling
- [ ] Zero-cost requests
- [ ] Large token counts

### Phase 4: Coverage Verification

- [ ] Achieve 75%+ overall coverage
- [ ] Identify uncovered paths
- [ ] Add targeted tests
- [ ] Document coverage gaps

---

## Code Coverage Goals by Package

| Package | Target | Path to Target |
|---------|--------|---|
| sentiment/ | 85% | 10+ test cases |
| budgets/ | 85% | 12+ test cases |
| statusline/ | 80% | 8+ test cases |
| service/ | 75% | 9+ test cases |
| cli/ | 60% | Manual + 4+ tests |
| dashboard/ | 70% | API integration |
| config/ | 80% | Config loading |
| **Overall** | **75%** | In progress ✅ |

---

## Running Tests in Development

### Watch Mode (Auto-run tests)

```bash
# Using entr (install: brew install entr)
find . -name "*.go" | entr -c ./scripts/test.sh --verbose
```

### IDE Integration

**VS Code**:
```json
{
    "go.lintTool": "golangci-lint",
    "go.lintOnSave": "package",
    "go.coverOnSave": true,
    "go.lintFlags": ["--fast"]
}
```

**GoLand/IntelliJ**:
- File → Settings → Go → Tests
- Enable "Run tests on save"
- Enable "Show test coverage"

---

## Continuous Improvement

### Adding Tests as You Code

When implementing a feature:

1. Write failing test first (TDD)
2. Implement feature
3. Verify test passes
4. Add edge case tests
5. Commit with tests

### Code Review Checklist

Before merging:

- [ ] Tests pass: `./scripts/test.sh --race`
- [ ] Coverage: >75% overall
- [ ] No race conditions
- [ ] Benchmarks within targets
- [ ] New tests for new features

---

## See Also

- [Quality Standards](QUALITY.md) — Code quality metrics
- [Testing Guide](tests/test_README.md) — Detailed testing documentation
- [Contributing](CONTRIBUTING.md) — Contribution requirements
