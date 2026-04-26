BINARY=claude-escalate
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-s -w -X github.com/szibis/claude-escalate/internal/config.Version=$(VERSION)"

.PHONY: build test lint clean install install-hook

## Build

build:
	CGO_ENABLED=0 go build $(LDFLAGS) -o bin/$(BINARY) ./cmd/claude-escalate

build-static:
	CGO_ENABLED=0 go build $(LDFLAGS) -a -tags netgo -o bin/$(BINARY) ./cmd/claude-escalate

## Test

test:
	go test -v -race -count=1 ./...

test-cover:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

bench:
	go test -bench=. -benchmem ./...

## Profiling & Memory Leak Detection

profile-cpu:
	mkdir -p profiles
	go test -bench=. -cpuprofile=profiles/cpu.prof ./... 2>&1 | tee profiles/bench.txt
	@echo "CPU profile generated: profiles/cpu.prof"
	@echo "View with: go tool pprof -http=:8080 profiles/cpu.prof"

profile-mem:
	mkdir -p profiles
	go test -bench=. -memprofile=profiles/mem.prof ./...
	@echo "Memory profile generated: profiles/mem.prof"
	@echo "View with: go tool pprof -http=:8080 profiles/mem.prof"

profile-all: profile-cpu profile-mem
	@echo "✓ Profiles generated in ./profiles/"

memory-leak-test:
	go test -v -run TestMemoryLeak ./internal/test/...

profiling-test:
	go test -v -run "TestCPU|TestHeap|TestGoroutine|TestAllocation" ./internal/test/...

slo-test:
	go test -v -run "TestSLO" ./internal/test/...

## Security Testing

security-lint:
	go run github.com/golangci/golangci-lint/cmd/golangci-lint@latest run ./... --enable gosec --timeout 10m

security-test:
	go test -v ./internal/security/...

security-scan:
	trivy fs .
	go list -json -m all | nancy sleuth

fuzz-all:
	go test -fuzz=Fuzz ./internal/fuzz/... -fuzztime=5m -timeout=15m

ci-local: security-lint memory-leak-test slo-test
	@echo "✓ Full CI simulation passed locally"

## Lint

lint:
	golangci-lint run ./...

fmt:
	gofmt -w .
	goimports -w .

## Install

install: build
	cp bin/$(BINARY) $(HOME)/.local/bin/$(BINARY)
	@echo "Installed to $(HOME)/.local/bin/$(BINARY)"

install-hook: install
	@echo "Configuring Claude Code hook..."
	@go run ./cmd/claude-escalate install-hook
	@echo "Hook installed. Restart Claude Code to activate."

## Clean

clean:
	rm -rf bin/ coverage.out coverage.html

## Docker

docker-build:
	docker build -t $(BINARY):$(VERSION) .

## Development

dev: build
	./bin/$(BINARY) dashboard --port 8077

## Help

help:
	@echo "claude-escalate - Intelligent model escalation for Claude Code"
	@echo ""
	@echo "Build Targets:"
	@echo "  build         Build binary"
	@echo "  install       Build and install to ~/.local/bin"
	@echo "  install-hook  Install + configure Claude Code hook"
	@echo "  dev           Start dashboard in development mode"
	@echo ""
	@echo "Testing Targets:"
	@echo "  test          Run tests with race detection"
	@echo "  test-cover    Run tests with coverage report"
	@echo "  bench         Run benchmarks"
	@echo "  memory-leak-test  Run memory leak detection tests"
	@echo "  profiling-test    Run profiling tests"
	@echo "  slo-test      Run SLO enforcement tests"
	@echo ""
	@echo "Profiling Targets:"
	@echo "  profile-cpu   Generate CPU profile"
	@echo "  profile-mem   Generate memory profile"
	@echo "  profile-all   Generate all profiles"
	@echo ""
	@echo "Security Targets:"
	@echo "  security-lint Run gosec + golangci-lint"
	@echo "  security-test Run security test suite"
	@echo "  security-scan Scan dependencies with Trivy/Nancy"
	@echo "  fuzz-all      Run fuzzing tests"
	@echo "  ci-local      Simulate full CI locally"
	@echo ""
	@echo "Other Targets:"
	@echo "  lint          Run linters"
	@echo "  fmt           Format code"
	@echo "  clean         Remove build artifacts"
