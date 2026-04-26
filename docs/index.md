# Claude Escalate Documentation

Complete guides and API reference for Claude Escalate v3.0.0.

---

## 🚀 Getting Started (Start Here!)

### New Users
1. **[5-Minute Setup](quick-start/5-minute-setup.md)** — Install and run in 5 minutes
2. **[How It Works](architecture/overview.md)** — Understand the three optimization layers
3. **[Dashboard Guide](analytics/dashboards.md)** — Monitor savings in real-time

### First Steps
- **[First Escalation](quick-start/first-escalation.md)** — Try the optimization
- **[Budget Setup](quick-start/budgets-setup.md)** — Set spending limits

---

## 📚 Core Documentation

### Architecture & Design
- **[System Overview](architecture/overview.md)** — High-level architecture
- **[Three-Phase Flow](architecture/3-phase-flow.md)** — Cache → Batch → Model selection
- **[Sentiment Detection](architecture/sentiment-detection.md)** — Task complexity analysis

### Features & Integration
- **[API Reference](integration/api-reference.md)** — HTTP endpoints & responses
- **[Budgets & Limits](integration/budgets.md)** — Cost enforcement
- **[Sentiment Detection](integration/sentiment-detection.md)** — Task routing

### Analytics & Monitoring
- **[Dashboards](analytics/dashboards.md)** — Real-time metrics & charts
- **[Cost Analysis](analytics/cost-analysis.md)** — Savings breakdown

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

## 👨‍💻 Development

### Contributing
- **[Developer Guide](../CLAUDE.md)** — Local development setup *(in root)*
- **[Contributing Guide](../CONTRIBUTING.md)** — How to contribute *(in root)*
- **[Testing](TESTING.md)** — How to run tests

### Code Quality
- **[Architecture Details](ARCHITECTURE.md)** — Deep dive into design
- **[Quality Standards](QUALITY.md)** — Code quality & testing

---

## 🔒 Security

- **[Security Policy](security/SECURITY.md)** — Vulnerability reporting & fixes
- **[Hardening Guide](security/SECURITY.md)** — Input validation, overflow protection

---

## 📖 Reference

### API Documentation
- **[API Reference](integration/api-reference.md)** — Complete API endpoints

### Configuration
- **[Config Guide](SETUP.md)** — Configuration file format & options

### Logs & Debugging
- **[Logs & Debugging](TROUBLESHOOTING.md#logs-and-debugging)** — Where to find logs

---

## 📊 Detailed Guides

### Real-World Usage
- **[Real World Impact](REAL_WORLD_IMPACT.md)** — Savings examples
- **[Conservative Analysis](CONSERVATIVE_ANALYSIS.md)** — Realistic projections
- **[Optimization Analysis](OPTIMIZATION_ANALYSIS.md)** — Deep dive

### Implementation Details
- **[Phase Completion](PHASE_7_COMPLETION.md)** — Latest updates
- **[Integration Status](INTEGRATION_COMPLETE.md)** — Feature status
- **[Validation Progress](VALIDATION_PROGRESS.md)** — Quality metrics

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
