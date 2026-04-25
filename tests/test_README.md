# Test Coverage Guide

Claude Escalate includes comprehensive test coverage with unit tests, integration tests, benchmarks, and regression tests.

---

## Running Tests

### All Tests

```bash
# Run all tests
go test ./...

# Run with verbose output
go test -v ./...

# Run with race condition detection
go test -race ./...
```

### Specific Packages

```bash
# Test sentiment detection
go test -v ./internal/sentiment/

# Test budget enforcement
go test -v ./internal/budgets/

# Test statusline sources
go test -v ./internal/statusline/

# Test service integration
go test -v ./internal/service/

# Run integration tests
go test -v ./tests/
```

### Test Coverage

```bash
# Generate coverage report
go test -cover ./...

# Detailed coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out  # Open in browser

# Coverage by function
go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out
```

---

## Test Structure

### Unit Tests

Each package includes comprehensive unit tests:

**Sentiment Detection** (`internal/sentiment/sentiment.go`):
- Frustration detection (keywords, patterns)
- Confusion signals
- Impatience/caution detection
- Confidence scoring (0.0-1.0)
- Edge cases (empty prompts, special characters)
- Multiple signal compounding
- Benchmarks

**Budget System** (`internal/budgets/budgets.go`):
- Daily/monthly limit enforcement
- Per-model daily limits
- Task-type budgets
- Hard vs soft limit modes
- Auto-downgrade logic
- Warning thresholds (50%, 75%, 90%)
- Compound budget violations
- Benchmarks

**Statusline Sources** (`internal/statusline/statusline.go`):
- Source availability checking
- Registry fallback chain
- Token metric parsing
- Cache metrics handling
- Timestamp ordering
- Benchmarks

### Integration Tests (`tests/integration_test.go`)

End-to-end scenarios testing multiple systems together:

1. **Full 3-Phase Flow**: Prompt analysis → budget check → usage recording → escalation decision
2. **Budget Enforcement Sequence**: Multiple requests within limits, exceeding limits, downgrading
3. **Sentiment Learning Pattern**: User progress from initial question through frustration to satisfaction
4. **Model Selection Logic**: Combining sentiment detection with budget constraints
5. **Cascade After Success**: Escalation and de-escalation based on outcomes

### Regression Tests

Tests for known past issues:

- **Sentiment false positives**: "thanks" in frustrated context
- **Frustration signal bleeding**: /escalate command shouldn't trigger frustration
- **Budget rounding errors**: Floating-point accumulation across many small requests
- **Zero cost requests**: Always allowed regardless of budget
- **Large token counts**: Handles 100M+ tokens without overflow

---

## Test Cases by Scenario

### Scenario: User Gets Frustrated and Escalates

```go
// Test: frustration detection → escalation → success
t.Run("Frustrated user escalates successfully", func(t *testing.T) {
    prompt1 := "tried haiku, still broken"
    score1 := detector.Detect(prompt1, false, 0)
    // Should detect frustration
    
    // System escalates to Sonnet
    budgetEngine.CheckBudget("sonnet", 0.30, "")
    
    prompt2 := "sonnet fixed it! thanks"
    score2 := detector.Detect(prompt2, false, 0)
    // Should detect satisfaction
})
```

### Scenario: Budget Constraint Forces Downgrade

```go
// Test: approaching budget triggers model downgrade
t.Run("Auto-downgrade on budget constraint", func(t *testing.T) {
    engine.state.DailyUsedUSD = 8.5  // 85% of $10 budget
    
    result := engine.CheckBudget("opus", 0.60, "")
    // Should recommend Sonnet instead
    
    engine.RecordUsage("sonnet", 0.30, "")
    // Total: $8.80, still within budget
})
```

### Scenario: Multiple Violations Detected

```go
// Test: request violates daily + monthly + model limits
t.Run("Multiple budget violations", func(t *testing.T) {
    engine.CheckBudget("opus", 0.60, "")
    // Should report: daily, monthly, opus-daily violations
})
```

---

## Benchmarks

Performance benchmarks ensure system scales efficiently:

```bash
# Run benchmarks
go test -bench=. -benchmem ./...

# Run specific benchmark
go test -bench=BenchmarkDetect -benchmem ./internal/sentiment/

# Compare benchmarks
go test -bench=. -benchmem ./... | tee old.txt
# [Make code changes]
go test -bench=. -benchmem ./... | tee new.txt
benchstat old.txt new.txt
```

### Key Benchmarks

- **SentimentDetect**: <1ms per prompt (should be <50µs)
- **BudgetCheck**: <100µs per request
- **RegistryGetBest**: <10µs (statusline source selection)

---

## Code Coverage Goals

| Module | Target | Status |
|--------|--------|--------|
| Sentiment detection | >80% | ✅ |
| Budget enforcement | >85% | ✅ |
| Statusline sources | >75% | ✅ |
| Service integration | >70% | ✅ |
| CLI commands | >60% | ⚠️ (UI heavy) |
| **Overall** | **>75%** | ✅ |

---

## Continuous Integration

### GitHub Actions

Tests run automatically on:
- Pull requests
- Commits to main
- Manual workflow dispatch

```yaml
# .github/workflows/test.yml
name: Tests
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      - run: go test -v -race -coverprofile=coverage.out ./...
      - run: go tool cover -html=coverage.out
```

---

## Adding New Tests

### Test File Naming

- Unit tests: `*_test.go` in same package
- Integration tests: `tests/*.go`
- Examples: `*_example_test.go`

### Test Structure

```go
package mypackage_test

import "testing"

func TestFeatureName(t *testing.T) {
    // Setup
    system := New()
    
    // Test
    result := system.DoSomething()
    
    // Verify
    if result != expected {
        t.Errorf("expected %v, got %v", expected, result)
    }
}

func TestFeatureName_EdgeCase(t *testing.T) {
    // Use subtests for related cases
    tests := []struct {
        name   string
        input  string
        expected string
    }{
        {"case1", "input1", "output1"},
        {"case2", "input2", "output2"},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
        })
    }
}

// Benchmarks
func BenchmarkFeature(b *testing.B) {
    system := New()
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        system.DoSomething()
    }
}
```

### Regression Test Template

```go
// Document why test exists
func TestRegressionIssue123(t *testing.T) {
    // Issue: [description of bug]
    // Expected: [what should happen]
    // Test: [verify it works now]
}
```

---

## Test Maintenance

### Updating Tests

When you modify code:

1. **Run tests**: `go test -v ./...`
2. **Fix failures**: Update tests or code as needed
3. **Check coverage**: `go test -cover ./...`
4. **Commit**: Include test changes in commit

### Debugging Test Failures

```bash
# Run specific test with output
go test -v -run TestFeatureName ./...

# Run with logging
go test -v -run TestFeatureName -timeout 30s ./...

# Debug with prints
t.Logf("debug value: %v", variable)  # Only shows on failure or with -v
```

### Skipping Tests Temporarily

```go
// Skip entire test
func TestFeature(t *testing.T) {
    t.Skip("TODO: implement")
}

// Skip specific case
if testing.Short() {
    t.Skip("Skipping in short mode")
}
```

---

## See Also

- [Deployment Guide](../docs/operations/deployment.md) — Production testing
- [Quality Standards](QUALITY.md) — Code quality metrics
- [Contributing](../CONTRIBUTING.md) — Testing requirements for PRs
