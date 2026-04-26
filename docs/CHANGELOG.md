# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
