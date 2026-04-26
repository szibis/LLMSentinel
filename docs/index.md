# Claude Escalate Documentation

Complete guides and API reference for Claude Escalate v4.0.0.

**Version**: 4.0.0 | **Release**: 2026-04-26 | **Status**: Production Ready

---

## 🚀 Getting Started (Start Here!)

### New Users
1. **[Installation & Setup](GETTING_STARTED.md)** — Install and configure (2 min)
2. **[Quick Start Guide](QUICK_START.md)** — Get running in 5 minutes  
3. **[Architecture Overview](ARCHITECTURE.md)** — Understand v4.0.0 components
4. **[Web Dashboard](DASHBOARD.md)** — Access analytics and monitoring

### First Steps
- **[ML Classification Guide](how-it-works.md)** — Understand task routing
- **[Budget Management](quick-start/budgets-setup.md)** — Set spending limits
- **[Analytics Basics](analytics/dashboards.md)** — View timeseries and forecasts

---

## 📚 Core Documentation (v4.0.0)

### New Features in v4.0.0
- **[ML Task Classification](how-it-works.md)** — Automatic task complexity detection
- **[Advanced Analytics](analytics/dashboards.md)** — Timeseries, percentiles, forecasts
- **[Dynamic Budgets](quick-start/budgets-setup.md)** — Multi-tier budget management
- **[Web Dashboard](DASHBOARD.md)** — React-based real-time monitoring
- **[Observability](operations/monitoring.md)** — Prometheus metrics & health checks
- **[Security Hardening](security/SECURITY.md)** — OWASP Top 10 compliance

### Architecture & Design
- **[System Architecture](ARCHITECTURE.md)** — v4.0.0 component design
- **[ML Classification Pipeline](how-it-works.md)** — Task detection & routing
- **[Analytics Engine](analytics/dashboards.md)** — Metrics collection & forecasting
- **[Three-Layer Optimization](how-it-works.md)** — Cache → Batch → Model selection

### Features & Integration
- **[REST API Reference](API.md)** — All v4.0.0 endpoints
- **[Budgets & Cost Control](quick-start/budgets-setup.md)** — Dynamic enforcement
- **[Task Classification](how-it-works.md)** — ML-based routing
- **[Configuration API](API.md)** — Runtime settings management

### Analytics & Monitoring
- **[Dashboards Guide](DASHBOARD.md)** — Web UI features and usage
- **[Metrics & Health](operations/monitoring.md)** — Prometheus & health endpoints
- **[Cost Analysis](analytics/cost-analysis.md)** — Savings breakdown & forecasts
- **[Performance Profiling](TESTING.md)** — CPU/memory/goroutine profiles

---

## 🛠️ Operations & Deployment

### Setup & Configuration
- **[Local Testing](LOCAL_TESTING.md)** — Development setup
- **[Environment Variables](operations/deployment.md#environment-variables)** — Configuration options

### Deployment
- **[Deployment Guide](operations/deployment.md)** — Production setup
- **[Docker Deployment](DOCKER_SERVICE.md)** — Containerized deployment
- **[Monitoring](operations/monitoring.md)** — Health checks & metrics

### Troubleshooting
- **[Troubleshooting Guide](TROUBLESHOOTING.md)** — Common issues & solutions
- **[FAQ](TROUBLESHOOTING.md#faq)** — Frequently asked questions

---

## 👨‍💻 Development & Quality

### Contributing & Setup
- **[Developer Guide](../CLAUDE.md)** — Local development setup *(in root)*
- **[Contributing Guide](../CONTRIBUTING.md)** — How to contribute *(in root)*
- **[Build Instructions](../BUILD.md)** — Build from source *(in root)*

### Testing & Quality
- **[Testing Guide](TESTING.md)** — Run unit, integration, and stress tests
- **[Memory Leak Detection](TESTING.md)** — Runtime leak analysis
- **[Performance Profiling](TESTING.md)** — pprof integration (CPU, heap, goroutine)
- **[Security Testing](security/SECURITY.md)** — OWASP Top 10 coverage

### Code Quality Standards
- **[Quality Standards](QUALITY.md)** — Code quality gates & requirements
- **[Race Detection](TESTING.md)** — Concurrent safety verification
- **[SLO Enforcement](TESTING.md)** — Performance boundaries

---

## 🔒 Security (OWASP Top 10)

### Security Features
- **[Security Policy](security/SECURITY.md)** — OWASP Top 10 hardening
- **[Input Validation](security/SECURITY.md)** — SQL injection, XSS, command injection prevention
- **[Memory Safety](security/SECURITY.md)** — Bounds checking, leak detection
- **[Cryptography](security/SECURITY.md)** — Secure hashing and key management
- **[Fuzzing Tests](TESTING.md)** — Native Go 1.18+ fuzz testing

---

## 📖 Reference

### API Documentation
- **[REST API Reference](API.md)** — Complete v4.0.0 endpoints
- **[Configuration API](API.md)** — Runtime config management
- **[Health & Metrics](API.md)** — Status and Prometheus endpoints

### Configuration & Setup
- **[Setup Guide](SETUP.md)** — Configuration file format & options
- **[Docker Setup](DOCKER_SERVICE.md)** — Container deployment
- **[Deployment Guide](operations/deployment.md)** — Production setup

### Logs & Debugging
- **[Troubleshooting Guide](TROUBLESHOOTING.md)** — Common issues & solutions
- **[Logs & Debugging](TROUBLESHOOTING.md#logs-and-debugging)** — Logging setup
- **[FAQ](TROUBLESHOOTING.md#faq)** — Frequently asked questions

---

## 📊 Advanced Topics

### Analytics & Reporting
- **[Cost Analysis](analytics/cost-analysis.md)** — Detailed savings breakdown
- **[Timeseries Analytics](analytics/dashboards.md)** — Metrics collection & aggregation
- **[Forecasting](analytics/dashboards.md)** — Cost and usage predictions
- **[Correlation Analysis](analytics/dashboards.md)** — Task-accuracy relationships

### Real-World Usage
- **[Usage Patterns](USAGE.md)** — Common implementation patterns
- **[Budget Strategies](quick-start/budgets-setup.md)** — Cost control patterns
- **[Optimization Examples](how-it-works.md)** — Real-world use cases

### v4.0.0 Updates
- **[Changelog](../CHANGELOG.md)** — Full v4.0.0 release notes
- **[Migration Guide](../CHANGELOG.md)** — Upgrading from v3.0.0
- **[Feature Overview](ARCHITECTURE.md)** — New v4.0.0 components

---

## 🔄 Changelog & Updates

- **[Changelog](../CHANGELOG.md)** — Version history & features *(in root)*
- **[Recent Updates](FINAL_STATUS.md)** — Latest changes
- **[Improvements](IMPROVEMENTS_APPLIED.md)** — Recent optimizations

---

## 📋 Quick Reference

| Task | Document |
|------|----------|
| **Set up in 5 minutes** | [5-Minute Setup](quick-start/5-minute-setup.md) |
| **Understand how it works** | [System Overview](architecture/overview.md) |
| **Deploy to production** | [Deployment Guide](operations/deployment.md) |
| **Monitor savings** | [Dashboards](analytics/dashboards.md) |
| **Set cost limits** | [Budgets Setup](quick-start/budgets-setup.md) |
| **Fix a problem** | [Troubleshooting](TROUBLESHOOTING.md) |
| **Integrate API** | [API Reference](integration/api-reference.md) |
| **Contribute code** | [Contributing Guide](../CONTRIBUTING.md) |
| **Report security issue** | [Security Policy](security/SECURITY.md) |
| **See cost savings** | [Cost Analysis](analytics/cost-analysis.md) |

---

## 📁 Documentation Structure

```
docs/
├── index.md (you are here)
├── quick-start/              # Getting started guides
│   ├── 5-minute-setup.md    # Installation & first run
│   ├── first-escalation.md  # Try it out
│   └── budgets-setup.md     # Cost control
├── architecture/             # System design & concepts
│   ├── overview.md          # High-level architecture
│   ├── 3-phase-flow.md      # Optimization flow
│   └── sentiment-detection.md # Task classification
├── integration/              # APIs & integrations
│   ├── api-reference.md     # HTTP API endpoints
│   ├── budgets.md           # Budget APIs
│   └── sentiment-detection.md # Sentiment APIs
├── analytics/                # Monitoring & metrics
│   ├── dashboards.md        # Dashboard features
│   └── cost-analysis.md     # Savings analysis
├── operations/               # Deployment & ops
│   ├── deployment.md        # Production setup
│   └── monitoring.md        # Health & metrics
├── security/                 # Security & vulnerability reporting
│   └── SECURITY.md          # Security policy
├── SETUP.md                 # Full setup guide
├── TROUBLESHOOTING.md       # Problem solving
├── TESTING.md               # Test suite
└── ARCHITECTURE.md          # Detailed architecture
```

---

## 🎯 Learning Paths

### For Users
1. [5-Minute Setup](quick-start/5-minute-setup.md)
2. [How It Works](architecture/overview.md)
3. [Dashboard Guide](analytics/dashboards.md)
4. [Budget Setup](quick-start/budgets-setup.md)
5. [Cost Analysis](analytics/cost-analysis.md)

### For Developers
1. [Developer Guide](../CLAUDE.md) *(root)*
2. [Architecture Overview](ARCHITECTURE.md)
3. [API Reference](integration/api-reference.md)
4. [Testing Guide](TESTING.md)
5. [Contributing Guide](../CONTRIBUTING.md) *(root)*

### For DevOps/Operations
1. [Deployment Guide](operations/deployment.md)
2. [Docker Setup](DOCKER_SERVICE.md)
3. [Monitoring](operations/monitoring.md)
4. [Troubleshooting](TROUBLESHOOTING.md)
5. [Configuration](SETUP.md)

---

## ❓ Need Help?

| Need | Where to Look |
|------|---|
| Something not working? | [Troubleshooting](TROUBLESHOOTING.md) |
| Want to deploy? | [Deployment Guide](operations/deployment.md) |
| How do I...? | Check the [5-Minute Setup](quick-start/5-minute-setup.md) |
| Found a bug? | [GitHub Issues](https://github.com/szibis/claude-escalate/issues) |
| Security issue? | [Security Policy](security/SECURITY.md) |
| Want to contribute? | [Contributing Guide](../CONTRIBUTING.md) *(root)* |

---

## 🔗 External Links

- **[GitHub Repository](https://github.com/szibis/claude-escalate)**
- **[Releases](https://github.com/szibis/claude-escalate/releases)**
- **[Issues](https://github.com/szibis/claude-escalate/issues)**
- **[Discussions](https://github.com/szibis/claude-escalate/discussions)**

---

**Last Updated**: April 2026  
**Version**: 3.0.0  
**Status**: Production Ready ✅
