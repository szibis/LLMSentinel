---
name: 🔒 Security Issue
about: Report a security vulnerability
title: "Security Issue: [Brief Description]"
labels: ["security", "triage"]
assignees: []
---

## Severity Level
<!-- Select one -->
- [ ] CRITICAL (Blocks release, immediate fix required)
- [ ] HIGH (Fix before RC1)
- [ ] MEDIUM (Fix before v1.0.0)
- [ ] LOW (Nice to have)

## Issue Type
<!-- Select one or more -->
- [ ] Injection (SQL, Command, Template, NoSQL)
- [ ] Authentication/Authorization
- [ ] Cryptographic Failure
- [ ] Insecure Configuration
- [ ] Data Exposure
- [ ] Dependency Vulnerability
- [ ] Rate Limiting Bypass
- [ ] Error Message Leak
- [ ] Hardcoded Secrets
- [ ] Other: _______________

## Vulnerability Description

### Location
**File**: [path/to/file.go]  
**Line**: [line number]  
**Function**: [function name]  

### Current Code
```go
// Paste the vulnerable code here
```

### Description
[Clear description of the vulnerability]

### Impact
- **Severity**: [CRITICAL|HIGH|MEDIUM|LOW]
- **Affected Component**: [e.g., Authentication, API, Database]
- **Potential Attacker Action**: [What could be exploited?]
- **Data at Risk**: [What data could be compromised?]

### Proof of Concept / Reproduction
```
Steps to reproduce or demonstrate the vulnerability:
1. ...
2. ...
3. ...
```

## Proposed Remediation

### Recommended Fix
```go
// Corrected code
```

### Alternatives Considered
- [ ] Option A: [description]
- [ ] Option B: [description]

### Verification Steps
- [ ] Code review completed
- [ ] Test case added
- [ ] All security scanners pass
- [ ] All unit tests pass
- [ ] Integration tests pass (if applicable)

## References

### Related Standards
- [ ] OWASP Top 10
- [ ] CWE
- [ ] CVE
- [ ] NIST Cybersecurity Framework

### Documentation
- [OWASP Top 10](https://owasp.org/www-project-top-ten/)
- [CWE/SANS Top 25](https://cwe.mitre.org/top25/)
- [Security Audit Guide](docs/PHASE_3_SECURITY_AUDIT.md)

### Similar Issues
<!-- Link to related security issues if any -->
- Closes/Related: #___

## Acceptance Criteria

- [ ] Fix implemented
- [ ] Test case covers the vulnerability
- [ ] Code review approved
- [ ] All scanners pass (gosec, govulncheck, nancy)
- [ ] No new vulnerabilities introduced
- [ ] Documentation updated
- [ ] Team trained on the issue

## Timeline

**Priority**: [1-7 days | 1-2 weeks | 1 month]  
**Assigned to**: [Team member]  
**Target Date**: [Date]  

---

## Security Audit Tracking

- **Scanner**: [gosec|manual|snyk|govulncheck|other]
- **First Identified**: [Date]
- **Status**: [Open|In Progress|Remediated|Deferred]
- **CVSS Score**: [0-10]
