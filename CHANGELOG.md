# Changelog

All notable changes to Claude Escalate are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [0.5.0] - 2026-04-27

### 🚀 Phase 2: Knowledge Graphs & Advanced Input Optimization

#### Feature 1: Knowledge Graph Storage
- SQLite-backed knowledge graph for code relationship queries
- Node types: function, class, interface, struct, variable, import, method, module
- Relationship types: calls, defines, imports, references, inherits, implements
- Recursive CTE traversal for efficient relationship queries
- Graph lookup integration with cache layer (99% token savings on relationship queries)

#### Feature 2: Content Indexing Pipeline
- CodeIndexer with file system watching and incremental indexing
- AST-based parsers for Go, Python, TypeScript code extraction
- Automatic entity detection (functions, classes, interfaces, imports)
- Relationship extraction with confidence scoring
- Graph persistence and query optimization

#### Feature 3: Advanced Input Optimization (Layer 4)
- RequestDeduplicator: Hash-based request caching (30-40% savings)
- InputFormatter: Structured input conversion, whitespace removal, term shortening
- ParameterCompressor: Key abbreviation and default value removal (20-35% savings)
- Unified InputOptimizer coordinating all techniques
- Combined input optimization: 40-60% token savings measured

#### Performance Improvements
- Exact cache hits: 100% token savings
- Semantic cache hits: 98% token savings  
- Graph query hits: 99% token savings
- Input optimization: 40-60% token savings on input layer
- Overall realistic mixed workload: 54-60% token savings

### 📊 Test Coverage
- 12 content indexing tests (AST parsing, graph integration, file watching)
- 15 input optimization tests (deduplication, formatting, compression)
- 530 total tests passing across 34 packages
- Zero regressions detected

### 🔧 Technical Details
- Added fsnotify dependency for file watching
- SQLite with WAL mode for concurrent access
- JSON marshaling for metadata storage
- Concurrent-safe deduplication with RWMutex
- Thread-safe parameter compression

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
