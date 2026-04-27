# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.8.0] - 2026-04-27

### Added
- Visual tool configuration dashboard tab (🔧 Tools)
  - Add, edit, delete custom CLI/MCP/REST/Database/Binary tools via web UI
  - Tool health status indicators with real-time checks
  - Type-specific path validation (CLI vs MCP socket vs REST API)
  - Settings editor with JSON support
  - Config auto-save with YAML persistence (survives restart)
- Tool management API endpoints
  - GET /api/tools — List configured tools
  - POST /api/tools/add — Add new tool
  - PUT /api/tools/{name} — Update tool configuration
  - DELETE /api/tools/{name} — Remove tool
  - POST /api/tools/{name}/test — Health check
  - GET /api/tools/types — Available tool types

### Changed
- Dashboard now includes 6 tabs: Metrics, Configuration, Security, Tools (NEW), Feedback, Analytics
- Tool configuration no longer requires YAML editing for end users
- Version bumped from v0.7.0 to v0.8.0

### Fixed
- Tool management route ordering for proper HTTP method dispatch

## [0.7.0] - 2026-04-15

### Added
- Five-layer intelligent model escalation system
  - Frustration signal detection (20+ patterns)
  - Circular reasoning detection (concept tracking across turns)
  - Manual escalation via `/escalate` commands
  - Automatic de-escalation on success signals
  - Predictive routing from escalation history
- Task type classification (11 domains: concurrency, parsing, debugging, etc.)
- SQLite-backed persistent storage for all escalation data
- Local web dashboard at port 8077 with real-time analytics
- CLI statistics tool (`stats summary`, `types`, `predictions`, `history`)
- Single binary replaces 6 separate bash hook scripts
- Pure Go build (no CGO required) for easy cross-compilation
- Docker support with multi-stage build
- docker-compose for local testing
- GitHub Actions CI/CD pipeline with auto-release on PR merge
- GHCR container image publishing
