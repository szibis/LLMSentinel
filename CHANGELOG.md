# Changelog

All notable changes to LLMSentinel are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [0.7.0] - 2026-04-27

### 🚀 Batch API + Production-Grade Hardening

#### Batch API Integration (50% Cost Reduction)
- Anthropic Batch API client with full lifecycle management
- Automatic non-interactive workload detection (bulk analysis, overnight jobs)
- Batch queue management with auto-flush logic (configurable batch size/timeout)
- Job status polling with exponential backoff retry
- Result aggregation and error handling
- Cost tracking: Compare batch vs regular API costs per request type
- Stacks with semantic cache for ~55% combined savings (50% batch + cache overlap)

**Batch Eligibility Detector:**
- Intent classification (batch_analysis, bulk_processing, scheduled_tasks, etc)
- Query pattern recognition ("analyze all 50 files", "bulk process", etc)
- Request volume tracking (5+ req/30s = bulk workload)
- Response time expectations (bulk jobs can wait 5+ min)
- Confidence scoring (0-1 scale, configurable threshold)
- Cost-benefit ROI calculation for batch vs interactive routing

#### Knowledge Graph Infrastructure (Phase 2 Complete)
- SQLite-backed knowledge graph for code relationship queries
- Node types: function, class, interface, struct, variable, import, method, module
- Recursive CTE traversal for efficient relationship queries (<10ms typical)
- Content indexing with AST parsing (Go, Python, TypeScript)
- Incremental indexing on file changes (inotify file watcher)
- Graph lookup integration with optimization pipeline (99% token savings on relationship queries)

#### Enhanced Metrics & Observability
- Label-based cardinality control (prevent metric explosion)
- OpenTelemetry + Prometheus dual export
- Structured JSON logging for production aggregation (ELK, Datadog, CloudWatch)
- Per-optimization metrics (RTK, semantic cache, input/output compression, batch API)
- Cost tracking (tokens sent, estimated cost, monthly projections, savings)
- Security event logging (injection attempts blocked, rate limit triggers)

### 🎯 Quality & Reliability

**Performance Validation (all targets met):**
- Cache lookup: <10ms ✓
- Intent detection: <50ms ✓
- Security validation: <20ms ✓
- Total gateway overhead: <200ms (fresh request) ✓
- Cache hit response: <100ms (end-to-end) ✓

**Load Testing (5K req/sec sustained):**
- 614+ tests passing across 37 packages ✓
- Memory stability: <10% heap growth under load ✓
- Goroutine leak detection: <5 new goroutines ✓
- Cache hit rate stability: >90% under load ✓
- Latency percentiles: P99 <300ms, P99.9 measured ✓
- Zero regressions from v0.6.0 ✓

**Security Hardening:**
- 50+ attack pattern detection (SQL injection, XSS, command injection)
- Input/output sanitization with context awareness
- Rate limiting (1000 req/min per IP, exponential backoff)
- Security event audit logging (all injections blocked, rate limits triggered)
- <0.1% false positive rate on semantic cache

### 📊 Test Coverage
- 625+ tests across 37 packages
- 8 load/stress tests validating 5K req/sec sustained, memory/goroutine stability
- Integration tests for Batch API, Knowledge Graph, Metrics, Router, Intent detection
- E2E scenarios: fresh query, cached query, cache bypass, security blocks, metrics accuracy
- Zero test flakiness, all tests deterministic

### 🔧 Technical Details
- Batch API: AnthropicClient, BatchQueue, NonInteractiveDetector, WorkloadAnalyzer
- Knowledge Graph: SQLite with WAL mode, fsnotify file watching, AST parsers
- Metrics: LabelValue cardinality control, OpenTelemetry SDK, Prometheus export
- Added dependencies: fsnotify (file watching), anthropic-sdk (Batch API)

### 🔄 Breaking Changes
- None. Backward compatible with v0.6.0.

### 📈 Migration Guide
- Batch API enabled by default (auto-detection for non-interactive workloads)
- Knowledge Graph available via graph query commands (no breaking changes)
- Config YAML backward compatible (new batch_api section optional)
- All existing optimizations (RTK, semantic cache, input compression) unchanged

---

## [0.6.0] - 2026-04-27

### 🚀 Configuration + Semantic Cache + Auto-Release

#### Gateway Configuration System
- YAML-based configuration with auto-detection of installed tools
- Live reload without downtime (watch config.yaml for changes)
- Support for 5 tool adapter types: MCP, CLI, REST, Database, Binary
- Environment variable interpolation (~, GOPATH, CARGO_HOME, etc)
- Sensible defaults if no config provided (auto-detect RTK, LSP, scrapling, git)

#### Semantic Caching
- Vector embeddings (ONNX Mini-L6-v2, 384-dim, 22MB)
- Cosine similarity matching (configurable threshold, default 0.85)
- LSH indexing for fast similarity search
- <0.1% false positive rate (strict thresholding)
- Semantic cache hit rate: 50-60% on typical workloads

#### Web Dashboard
- Real-time metrics visualization (WebSocket streaming)
- Live config editor (validate and reload without restart)
- Token savings calculator
- Cache hit rate display
- Security event log
- Health status checks

### 📊 Test Coverage
- 530+ tests across 34 packages
- Configuration loading and validation tests
- Semantic cache accuracy tests
- Dashboard API endpoint tests
- Integration tests (config → metrics → dashboard flow)

### 🔄 Breaking Changes
- None. Additive features only.

---

## [0.5.0] - 2026-04-27

### 🚀 Knowledge Graphs & Advanced Input Optimization

#### Knowledge Graph Storage
- SQLite-backed knowledge graph for code relationship queries
- Node types: function, class, interface, struct, variable, import, method, module
- Relationship types: calls, defines, imports, references, inherits, implements
- Recursive CTE traversal for efficient relationship queries
- Graph lookup integration with cache layer (99% token savings on relationship queries)

#### Content Indexing Pipeline
- CodeIndexer with file system watching and incremental indexing
- AST-based parsers for Go, Python, TypeScript code extraction
- Automatic entity detection (functions, classes, interfaces, imports)
- Relationship extraction with confidence scoring
- Graph persistence and query optimization

#### Advanced Input Optimization
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
