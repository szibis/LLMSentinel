# Contributing to claude-escalate

Thank you for considering contributing to claude-escalate! This document provides guidelines for contributing.

## Development Setup

### Prerequisites

- **Go 1.26.2** (required, enforced in `go.mod`)
- Docker (optional, for container testing)
- golangci-lint (for linting, see [CLAUDE.md](CLAUDE.md) for Go 1.26.2 compatibility notes)

**⚠️ Go Version Requirement**: This project requires Go 1.26.2. Do not use earlier versions. See [CLAUDE.md](CLAUDE.md) for detailed setup instructions and known issues.

### Getting Started

```bash
git clone https://github.com/szibis/claude-escalate.git
cd claude-escalate
make build
make test
```

### Available Make Targets

| Target | Description |
|--------|-------------|
| `make build` | Build the binary |
| `make test` | Run all tests |
| `make test-cover` | Run tests with coverage report |
| `make lint` | Run golangci-lint |
| `make fmt` | Format code |
| `make dev` | Start dashboard in development mode |
| `make docker-build` | Build Docker image |
| `make docker-test` | Run integration tests via docker-compose |
| `make clean` | Remove build artifacts |

## Code Style

- Follow [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- Run `make lint` before committing - the CI will enforce this
- Configuration: `.golangci.yml`

## Pull Request Process

1. Fork the repository
2. Create a feature branch from `main` (`git checkout -b feat/my-feature`)
3. Make your changes with tests
4. Run `make test && make lint` locally
5. Commit with conventional commit messages:
   - `feat: add new detection pattern`
   - `fix: correct de-escalation cascade logic`
   - `docs: update integration guide`
   - `refactor: simplify task classification`
6. Push and open a PR against `main`

### PR Requirements

- All CI checks must pass (build, test, lint, security)
- Tests required for new functionality
- Documentation updated if adding features or changing behavior

### Conventional Commits

We use conventional commits for automatic versioning:

| Prefix | Version Bump | Example |
|--------|-------------|---------|
| `feat:` | Minor (0.x.0) | `feat: add budget guard` |
| `fix:` | Patch (0.0.x) | `fix: correct success detection` |
| `feat!:` | Major (x.0.0) | `feat!: change hook output format` |
| `docs:` | No release | `docs: update README` |
| `ci:` | No release | `ci: add CodeQL workflow` |

## Project Structure

```
cmd/claude-escalate/     CLI entry point
internal/
  hook/                  Claude Code JSON protocol
  detect/                Frustration + circular detection
  classify/              Task type classification
  store/                 SQLite persistent storage
  dashboard/             Local web UI
  config/                Configuration
docs/                    Documentation
```

## Adding New Detection Patterns

1. Add pattern to `internal/detect/detect.go`
2. Add test case to `internal/detect/detect_test.go`
3. Run `make test`

## Adding New Task Types

1. Add type constant to `internal/classify/classify.go`
2. Add regex rule to the `rules` slice
3. Add to `AllTaskTypes()`
4. Add test case to `internal/classify/classify_test.go`

## Reporting Issues

Please use the GitHub issue templates for bug reports and feature requests.

## License

By contributing, you agree that your contributions will be licensed under the Apache License 2.0.
