# Changelog

All notable changes to Claude Escalate are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [4.0.0] - 2026-04-26

### 🚀 Major Features Added

#### Feature 1: ML-Based Task Classification
- Automatic task complexity detection using embedding-based classification
- Support for 7+ task types (concurrency, parsing, optimization, database, architecture, simple_qa, classification, summarization)
- Learned accuracy tracking from feedback loop
- Embedding cache for performance optimization
- Task type routing to optimal Claude model (Haiku/Sonnet/Opus)

#### Feature 2: Advanced Analytics Engine
- Timeseries aggregation (hourly, daily, weekly, monthly buckets)
- Latency percentile tracking (P50, P95, P99)
- Cost forecasting (linear regression models)
- Task-accuracy correlation matrix analysis
- Anomaly detection with sentiment-aware baseline
- Data retention policies (automatic cleanup of old metrics)

#### Feature 3: Dynamic Budget Management
- Multi-tier budgets (daily, weekly, monthly)
- Automatic budget enforcement with cost-aware routing
- Budget tracking and alerts
- Burndown rate calculation
- Remaining budget forecasting

#### Feature 4: Real-Time Web Dashboard
- React 18 + Vite + Tailwind CSS frontend
- 5 main tabs: Overview, Analytics, Tasks, Config, Health
- Dark mode support with localStorage persistence
- Real-time metrics refresh (5s polling)
- Interactive analytics with trend charts
- Mobile-responsive design

#### Feature 5: Observability & Monitoring
- Prometheus metrics export (/metrics endpoint)
- Health check endpoints (/health/live, /health/ready)
- Request latency histograms
- Model distribution tracking
- Cache hit rate monitoring
- Budget status metrics
- Goroutine and memory usage tracking

#### Feature 6: Enterprise Security Hardening
- 30+ security tests (OWASP Top 10 coverage)
- Memory leak detection (runtime analysis)
- CPU/heap/goroutine profiling infrastructure
- SLO enforcement (memory <50MB, latency <5ms)
- Input validation for SQL injection, XSS, command injection
- Fuzzing tests (Go 1.18+ native fuzzing, 7 fuzz targets)
- Race condition detection (all tests with -race flag)
- Gosec security linting (6 intentional suppressions, documented)

### ✨ Enhancements

- **CLI Dashboard**: Terminal-based dashboard for remote viewing
- **Configuration API**: REST endpoints for budget and settings management
- **Batch Processing**: Improved batch queue with cost optimization
- **Caching**: Similarity-based prompt matching with configurable thresholds
- **Cost Calculations**: Per-model pricing with budget enforcement
- **Error Handling**: Comprehensive error messages with context
- **Documentation**: 100+ page comprehensive docs with examples
- **Testing**: 312 unit tests + 30 security tests + 7 fuzz targets
- **CI/CD**: GitHub Actions with security scanning, memory leak detection, profiling

### 🐛 Bug Fixes

- Fixed database connection pooling issues
- Fixed metrics export race conditions
- Fixed cache eviction logic
- Fixed budget enforcement on concurrent requests
- Fixed goroutine cleanup in health checks
- Removed Claude co-authoring from commits (no-attribution policy)
- Resolved merge conflicts in security infrastructure

### 🔒 Security Fixes

- Input validation on all API endpoints
- Integer overflow protection in cost calculations
- Memory bounds checking in cache
- No secrets in metrics/logs
- HTTPS enforcement for webhook URLs
- Cryptographic validation for sensitive operations
- Path traversal protection
- SQL injection prevention (parameterized queries)

### ⚙️ Technical Changes

- Upgraded Go to 1.26
- Upgraded golangci-lint to v7
- Implemented pprof profiling infrastructure
- Added runtime.MemStats tracking for leak detection
- Implemented SLO enforcement tests
- Added gosec security linting to CI/CD
- All commits now SSH-signed (ED25519 key)
- Structured logging with context

### 📊 Performance Improvements

- <5ms per-request latency (SLO enforced)
- <50MB memory usage with 10K+ cached entries
- Efficient timeseries aggregation
- Optimized embedding lookups
- Concurrent request handling with race detection
- CPU profiles generated for bottleneck analysis

### 🔄 Breaking Changes

- Configuration schema updated for new features
- API endpoints reorganized for clarity
- Database schema migrated (automatic migration included)
- CLI commands updated with new options

### 📦 Dependencies

- All dependencies updated to latest versions
- Removed deprecated packages
- Zero breaking security issues
- Go modules locked for reproducible builds

### 🎯 Known Limitations

- ML classification trained on internal dataset (future: community models)
- Single-node deployment (future: distributed support)
- SQLite backend (future: PostgreSQL option)
- Hourly+ timeseries aggregation (future: minute-level)
