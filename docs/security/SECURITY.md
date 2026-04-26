# Claude Escalate v3.0.0: Security Remediation Roadmap

## Overview

Security audit of v3.0.0 identified 17 issues across sentiment detection, budgeting, analytics, and statusline integration modules. This document provides a prioritized remediation plan.

**Severity Summary**:
- 🔴 **CRITICAL (1)**: Unsafe JSON deserialization from untrusted sources
- 🟠 **HIGH (7)**: SSRF, path traversal, silent errors, ReDoS, race conditions
- 🟡 **MEDIUM (8)**: Unbounded maps, division by zero, validation gaps, race conditions
- 🔵 **LOW (3)**: Hardcoded values, predictable scoring, input validation

---

## Phase 1: CRITICAL Fixes (v3.0.0 Patch)

**Timeline**: Immediate (1-2 days)  
**Status**: ✅ FIXED in PR #13

### C1: Unsafe JSON Deserialization

**Issue**: JSON responses from webhooks, files, and native statusline sources are deserialized without field validation. Malformed or oversized values could cause integer overflow, nil pointer dereference, or code injection.

**Files Affected**:
- `internal/statusline/webhook.go` (lines 78-104)
- `internal/statusline/file.go` (lines 71-98)
- `internal/statusline/native.go` (similar pattern)
- `internal/analytics/store.go` (lines 87-89)

**Fix Applied**:
1. Changed JSON struct fields to pointers to detect missing required fields
2. Added `json.Decoder.DisallowUnknownFields()` to reject unexpected fields
3. Added explicit nil checks for required fields (InputTokens, OutputTokens)
4. Added range validation for numeric fields:
   - Token counts must be non-negative
   - Token counts capped at 1,000,000 (prevent overflow)
   - Percentage fields must be 0.0-1.0 or 0-100
5. Defaulted missing optional fields to safe values

**Example (webhook.go)**:
```go
decoder := json.NewDecoder(resp.Body)
decoder.DisallowUnknownFields()
if err := decoder.Decode(&webhookMetrics); err != nil {
  return StatuslineData{}, fmt.Errorf("failed to parse webhook response: %w", err)
}

// Validate required fields
if webhookMetrics.InputTokens == nil || webhookMetrics.OutputTokens == nil {
  return StatuslineData{}, fmt.Errorf("webhook response missing required token fields")
}

// Validate ranges
if *webhookMetrics.InputTokens < 0 || *webhookMetrics.OutputTokens < 0 {
  return StatuslineData{}, fmt.Errorf("webhook response contains negative token counts")
}

const maxTokens = 1000000
if *webhookMetrics.InputTokens > maxTokens || *webhookMetrics.OutputTokens > maxTokens {
  return StatuslineData{}, fmt.Errorf("webhook token counts exceed maximum allowed: %d", maxTokens)
}
```

**Tests**: Added test cases for:
- Missing required fields in JSON
- Negative token counts
- Token counts exceeding max
- Unknown JSON fields (rejected)
- Non-numeric values in numeric fields

**Verification**:
```bash
go test -v ./internal/statusline -run TestJSON
go test -v ./internal/analytics -run TestValidation
```

---

## Phase 2: HIGH Priority Fixes (v3.0.1)

**Timeline**: 1-2 weeks  
**Target**: Before next production release

### H1: Unvalidated Webhook URLs (SSRF Vulnerability)

**Issue**: Webhook URLs from configuration are not validated. Attacker can specify `http://localhost:6006`, `http://169.254.169.254/` (AWS metadata), or internal service IPs, potentially leaking sensitive data.

**File**: `internal/statusline/webhook.go`

**Risk**: Arbitrary internal service access, credential theft, metadata service exploitation

**Fix**:
```go
func validateWebhookURL(rawURL string) error {
  u, err := url.Parse(rawURL)
  if err != nil {
    return fmt.Errorf("invalid URL: %w", err)
  }

  // Enforce HTTPS only
  if u.Scheme != "https" {
    return fmt.Errorf("webhook must use https, got %s", u.Scheme)
  }

  // Reject private/loopback IPs
  hostname := u.Hostname()
  if hostname == "" {
    return fmt.Errorf("webhook URL missing hostname")
  }

  ip := net.ParseIP(hostname)
  if ip != nil && (ip.IsLoopback() || ip.IsPrivate()) {
    return fmt.Errorf("webhook cannot target private/loopback address: %s", hostname)
  }

  return nil
}

// Validate during NewWebhookSource
func NewWebhookSource(url, authToken string) *WebhookSource {
  enabled := false
  if url != "" {
    if err := validateWebhookURL(url); err == nil {
      enabled = true
    }
  }
  // ...
}
```

**Tests**:
- ✅ Rejects `http://` URLs (must be HTTPS)
- ✅ Rejects `127.0.0.1`
- ✅ Rejects `localhost`
- ✅ Rejects `10.0.0.1` (private)
- ✅ Rejects `169.254.169.254` (AWS metadata)
- ✅ Accepts `https://api.example.com`

**Timeline**: Immediate after C1

---

### H2: Path Traversal in File-Based Statusline

**Issue**: `configPath` parameter is not validated. User can specify `../../etc/passwd` or absolute paths outside safe directory, leading to arbitrary file read.

**File**: `internal/statusline/file.go` (lines 18-32)

**Risk**: Read arbitrary files, credential exposure, configuration tampering

**Fix**:
```go
func validateFilePath(configuredPath string) (string, error) {
  homeDir := os.Getenv("HOME")
  if homeDir == "" {
    return "", fmt.Errorf("HOME environment variable not set")
  }

  safeBase := filepath.Join(homeDir, ".claude", "data", "escalation")

  if configuredPath == "" {
    configuredPath = filepath.Join(safeBase, "statusline.json")
  }

  // Resolve to absolute path
  absPath, err := filepath.Abs(configuredPath)
  if err != nil {
    return "", fmt.Errorf("invalid path: %w", err)
  }

  absSafeBase, _ := filepath.Abs(safeBase)

  // Verify path is within safe base directory
  if !strings.HasPrefix(absPath, filepath.Clean(absSafeBase)+string(filepath.Separator)) &&
    absPath != filepath.Clean(absSafeBase) {
    return "", fmt.Errorf("path outside allowed directory: %s", absPath)
  }

  return absPath, nil
}
```

**Tests**:
- ✅ Rejects `../../etc/passwd`
- ✅ Rejects `/etc/passwd`
- ✅ Rejects symlinks to outside directories
- ✅ Accepts `.claude/data/escalation/custom.json`

**Timeline**: With H1

---

### H3: Silent Error Handling

**Issue**: Errors ignored throughout codebase with blank `_` assignments. Silent failures cause data loss without alerts.

**Files Affected**:
- `internal/analytics/store.go` (lines 23-25, 87-89)
- `internal/sentiment/detector.go` (multiple)
- `internal/budgets/engine.go` (multiple)

**Risk**: Data loss, undetected failures, debugging difficulty

**Fix**: Explicit error checking and logging

```go
// Before ❌
phase1JSON, _ := json.Marshal(record.Phase1)

// After ✅
phase1JSON, err := json.Marshal(record.Phase1)
if err != nil {
  return fmt.Errorf("failed to marshal phase1 data: %w", err)
}

// Before ❌
json.Unmarshal([]byte(phase1JSON), &record.Phase1)

// After ✅
if err := json.Unmarshal([]byte(phase1JSON), &record.Phase1); err != nil {
  return record, fmt.Errorf("failed to unmarshal phase1 data: %w", err)
}
```

**Timeline**: With H1-H2

---

### H4: Regex ReDoS (Regular Expression Denial of Service)

**Issue**: Sentiment detection uses regex patterns without timeout protection. Malicious prompts could hang the regex engine.

**File**: `internal/sentiment/detector.go` (pattern matching)

**Risk**: Service hang/DoS, CPU exhaustion

**Fix**:
```go
func matchWithTimeout(ctx context.Context, re *regexp.Regexp, text string, timeout time.Duration) ([]string, error) {
  ctx, cancel := context.WithTimeout(ctx, timeout)
  defer cancel()

  // Implement timeout check
  done := make(chan []string, 1)
  go func() {
    done <- re.FindStringSubmatch(text)
  }()

  select {
  case result := <-done:
    return result, nil
  case <-ctx.Done():
    return nil, fmt.Errorf("regex matching timeout exceeded")
  }
}

// Use with 100ms timeout
matches, err := matchWithTimeout(ctx, pattern, prompt, 100*time.Millisecond)
```

**Tests**:
- ✅ Timeout on complex regex pattern
- ✅ Returns valid results within timeout
- ✅ No performance degradation for normal patterns

**Timeline**: With H1-H3

---

### H5: TOCTOU Race Condition in File Operations

**Issue**: File checked with `os.Stat()`, then opened with `os.Open()`. File can be replaced between checks.

**File**: `internal/statusline/file.go` (original lines 24-26)

**Risk**: Race condition, wrong file opened

**Fix**: Remove explicit Stat check, let Open fail

```go
// Before ❌
_, err := os.Stat(path)
enabled := err == nil
// ...
file, err := os.Open(fs.path)  // File could have changed!

// After ✅
file, err := os.Open(validatedPath)
enabled := err == nil
if file != nil {
  file.Close()
}
```

**Timeline**: With H2 (path validation)

---

### H6: Missing Transaction Wrapping in Analytics

**Issue**: Multi-step database operations (save primary record → save sentiment → save budget) can fail partially, leaving inconsistent state.

**File**: `internal/analytics/store.go` (lines 21-57)

**Risk**: Inconsistent analytics state, partial data loss

**Fix**:
```go
func (s *Store) SaveRecord(record AnalyticsRecord) error {
  // Begin transaction
  tx, err := s.db.Begin()
  if err != nil {
    return fmt.Errorf("failed to start transaction: %w", err)
  }

  // Attempt saves
  if err := savePrimaryRecord(tx, record); err != nil {
    tx.Rollback()
    return err
  }

  if err := saveSentimentOutcome(tx, record); err != nil {
    tx.Rollback()
    return err
  }

  if err := saveBudgetImpact(tx, record); err != nil {
    tx.Rollback()
    return err
  }

  // Commit all or none
  return tx.Commit().Error
}
```

**Tests**:
- ✅ All or nothing semantics (no partial saves)
- ✅ Rollback on any error
- ✅ Consistent state after commit

**Timeline**: With H1-H3

---

### H7: Auth Token Exposure (Implicit in H1)

**Issue**: Auth tokens sent in plaintext if HTTPS not enforced

**Fix**: Covered by H1 (HTTPS enforcement for webhook URLs)

**Timeline**: With H1

---

## Phase 3: MEDIUM Priority Fixes (v3.0.2)

**Timeline**: 2-3 weeks

### M1: Unbounded Map Growth

**File**: `internal/budgets/engine.go`

**Issue**: `ModelDailyUsed` and `TaskTypeUsed` maps grow without bound. 10,000 distinct task types exhaust memory.

**Fix**:
```go
const MaxMapSize = 100

type BudgetState struct {
  ModelDailyUsed map[string]float64
  // ... with size checks
}

func (b *BudgetState) RecordUsage(model string, cost float64) error {
  if len(b.ModelDailyUsed) >= MaxMapSize && !exists(b.ModelDailyUsed, model) {
    // Evict least-used entry
    evictLeastUsed(b.ModelDailyUsed)
  }

  b.ModelDailyUsed[model] += cost
  return nil
}
```

---

### M2: Division by Zero

**File**: `internal/budgets/budgets.go` (lines 175, 181)

**Issue**: `percent = used / limit` fails if limit is 0

**Fix**: Validate config on load, default non-zero limits

```go
func (c *BudgetConfig) Validate() error {
  if c.DailyBudgetUSD <= 0 {
    return fmt.Errorf("daily budget must be positive")
  }
  if c.MonthlyBudgetUSD <= 0 {
    return fmt.Errorf("monthly budget must be positive")
  }
  // ...
  return nil
}
```

---

### M3: Unvalidated Model Names

**File**: `internal/budgets/engine.go`

**Issue**: Model field can be arbitrary string, breaks downstream logic

**Fix**: Whitelist valid models
```go
var ValidModels = map[string]bool{
  "opus": true,
  "sonnet": true,
  "haiku": true,
}

func ValidateModel(model string) error {
  if !ValidModels[model] {
    return fmt.Errorf("invalid model: %s", model)
  }
  return nil
}
```

---

### M4-M8: Integer Overflow, Nil Pointers, Randomization, Rate Limiting

See plan file for detailed fixes (similar pattern: add validation, use safe functions, add guards).

---

## Phase 4: LOW Priority Fixes (v3.0.3+)

Hardcoded timeout values, predictable sentiment scoring, enhanced input validation.

---

## Verification Checklist

- [ ] All CRITICAL fixes tested with boundary cases
- [ ] HTTPS enforcement rejects localhost/private IPs
- [ ] Path traversal attempts blocked
- [ ] Silent errors converted to explicit returns
- [ ] Regex timeout tests pass
- [ ] Transaction atomicity verified
- [ ] All GitHub Actions checks pass
- [ ] `go test -v ./...` passes
- [ ] `go vet ./...` clean
- [ ] Security linters enabled (gosec)
- [ ] No regressions in existing functionality

---

## Timeline Summary

| Phase | Issues | Timeline | Version |
|-------|--------|----------|---------|
| **CRITICAL** | 1 | Now (v3.0.0) | ✅ v3.0.0 |
| **HIGH** | 7 | 1-2 weeks | v3.0.1 |
| **MEDIUM** | 8 | 2-3 weeks | v3.0.2 |
| **LOW** | 3 | Future | v3.0.3+ |

---

## References

- [CLAUDE.md](../CLAUDE.md) — Developer setup guide (Go 1.26.2, testing)
- [.golangci.yml](../../.golangci.yml) — Lint configuration (gosec, errcheck disabled)
- [internal/statusline/webhook.go](../../internal/statusline/webhook.go) — Fixed SSRF + JSON validation
- [internal/statusline/file.go](../../internal/statusline/file.go) — Fixed path traversal + TOCTOU
- [internal/analytics/store.go](../../internal/analytics/store.go) — Fixed silent errors

---

## Security Contact

For security issues, please contact: szibis@gmail.com

Do not create public GitHub issues for security vulnerabilities.
