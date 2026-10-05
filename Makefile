BINARY=llm-sentinel
.DEFAULT_GOAL := lab-help
GO ?= go
PYTHON ?= python3
export MODEL_PATH SMALL_MODEL_PATH MODEL_ROLE MLX_FLASH_BIN ATTACH_RUNTIME PROMPT LAB_WORKSPACE

.PHONY: lab-clients-update
.PHONY: lab-model-check
lab-model-check:
	$(PYTHON) scripts/check_qwen_roles.py

lab-clients-update:
	$(PYTHON) scripts/sentinel_lab.py clients-update

.PHONY: lab-help lab-init lab-test lab-run lab-start lab-restart lab-foreground lab-rebuild lab-stop lab-status lab-logs lab-follow lab-ask lab-doctor lab-codex lab-claude
lab-help:
	@printf '%s\n' 'Qwen roles: Haiku = small, Sonnet = large, Opus = large with thinking.' '  make lab-model-check  Opt-in real generation check for all three roles' 'Two-model setup: pass MODEL_PATH (large) and SMALL_MODEL_PATH (small); selections are saved.'
	@printf '%s\n' 'Isolated Sentinel lab:' '  make lab-clients-update  Install/update latest isolated Codex and Claude' '  make lab-init     Prepare separate client profiles' '  make lab-test     Fast gateway/race/vet and isolation checks' '  make lab-run      Build and start server in background' '  make lab-rebuild  Stop owned lab, rebuild, and restart server' '  make lab-restart  Restart server without rebuilding' '  make lab-foreground  Optional foreground debugging' '  make lab-stop     Stop only this lab' '  make lab-status   Gateway and real runtime health' '  make lab-logs     Recent gateway/runtime logs' '  make lab-follow   Follow logs' '  make lab-ask      Send PROMPT to the local model' '  make lab-doctor   Isolated client versions and gateway health' '  make lab-claude   Open real interactive Claude Code with the local stack' '' 'First runtime start: make lab-run MODEL_PATH=/absolute/cached/model MLX_FLASH_BIN=/path/to/mlx-flash' 'Selections are saved locally for rebuilds. ATTACH_RUNTIME=1 uses an existing runtime on 19091.' 'No automatic installs or weight downloads.'

lab-init:
	$(PYTHON) scripts/sentinel_lab.py prepare

lab-test: gateway-check
	$(PYTHON) -m unittest discover -s scripts -p 'test_sentinel_*.py' -v

lab-run: lab-start

lab-start: gateway-build
	$(PYTHON) scripts/sentinel_runner.py start

lab-restart:
	$(PYTHON) scripts/sentinel_runner.py restart

lab-foreground: gateway-build
	$(PYTHON) scripts/sentinel_runner.py run

lab-rebuild:
	$(PYTHON) scripts/sentinel_runner.py stop
	$(MAKE) gateway-build
	$(PYTHON) scripts/sentinel_runner.py start

lab-stop:
	$(PYTHON) scripts/sentinel_runner.py stop

lab-status:
	$(PYTHON) scripts/sentinel_runner.py status

lab-logs:
	$(PYTHON) scripts/sentinel_runner.py logs

lab-follow:
	$(PYTHON) scripts/sentinel_runner.py logs --follow

lab-ask:
	$(PYTHON) scripts/sentinel_runner.py ask

lab-doctor:
	$(PYTHON) scripts/sentinel_lab.py doctor

lab-codex:
	$(PYTHON) scripts/sentinel_lab.py codex

lab-claude: lab-start
	$(PYTHON) scripts/sentinel_lab.py claude
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-s -w -X github.com/szibis/claude-escalate/internal/config.Version=$(VERSION)"

.PHONY: build test lint clean install install-hook

.PHONY: gateway-build gateway-check
gateway-build:
	CGO_ENABLED=0 $(GO) build -trimpath -o bin/sentinel-gateway ./cmd/sentinel-gateway

gateway-check:
	$(GO) test -race ./internal/localgateway ./cmd/sentinel-gateway
	$(GO) vet ./internal/localgateway ./cmd/sentinel-gateway

## Build

build:
	CGO_ENABLED=0 go build $(LDFLAGS) -o bin/$(BINARY) ./cmd/llm-sentinel

build-static:
	CGO_ENABLED=0 go build $(LDFLAGS) -a -tags netgo -o bin/$(BINARY) ./cmd/llm-sentinel

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
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2 run ./... --enable gosec --timeout 10m

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
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2 run ./... --timeout 5m

fmt:
	gofmt -w .
	goimports -w .

## Install

install: build
	cp bin/$(BINARY) $(HOME)/.local/bin/$(BINARY)
	@echo "Installed to $(HOME)/.local/bin/$(BINARY)"

install-hook: install
	@echo "Configuring Claude Code hook..."
	@go run ./cmd/llm-sentinel install-hook
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
	@echo "llm-sentinel - Intelligent model escalation for Claude Code"
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
	@echo ""
	@echo "Week 1.5 Verification Targets:"
	@echo "  test-unit          Run unit tests only (fast)"
	@echo "  test-integration   Run integration tests"
	@echo "  test-e2e-week1.5   Run Week 1.5 E2E scenarios"
	@echo "  verify-spec        Check spec compliance"
	@echo "  verify-all         Full verification (unit + integration + coverage + spec)"
	@echo ""
	@echo "ML Models Targets:"
	@echo "  test-models        Run ML model tests (manager, download, inference)"
	@echo "  model-test         Alias for test-models"

## Week 1.5 Verification Targets

.PHONY: test-unit test-integration test-e2e-week1.5 verify-spec verify-all test-models model-test

# Run unit tests for all modules
test-unit:
	@echo "Running Week 1.5 unit tests..."
	@go test -v -race -timeout 30s ./internal/discovery/...
	@go test -v -race -timeout 30s ./internal/intent/...
	@go test -v -race -timeout 30s ./internal/security/...
	@go test -v -race -timeout 30s ./internal/config/...
	@go test -v -race -timeout 30s ./internal/metrics/...
	@go test -v -race -timeout 30s ./internal/models/...
	@echo "✅ Unit tests completed"

# Run ML model-specific tests
test-models:
	@echo "Running ML model tests..."
	@go test -v -race -timeout 60s ./internal/models/...
	@echo "✅ Model tests completed"

# Alias for test-models
model-test: test-models

# Run integration tests
test-integration:
	@echo "Running Week 1.5 integration tests..."
	@RUN_INTEGRATION=1 go test -v -race -timeout 60s -run "Integration" ./internal/test/...
	@echo "✅ Integration tests completed"

# Run Week 1.5 E2E scenarios
test-e2e-week1.5:
	@echo "Running Week 1.5 E2E scenarios..."
	@RUN_E2E_WEEK1_5=1 go test -v -timeout 120s -run "Scenario" ./internal/test/...
	@echo "✅ Week 1.5 E2E tests completed"

# Verify spec compliance
verify-spec:
	@echo "Validating specification compliance..."
	@go run tools/spec_validator.go .
	@echo "✅ Spec validation completed"

# Full verification suite
verify-all: test-unit test-integration test-cover verify-spec
	@echo ""
	@echo "═══════════════════════════════════════════════════════════════"
	@echo "✅ WEEK 1.5 VERIFICATION COMPLETE"
	@echo "═══════════════════════════════════════════════════════════════"
	@echo ""
	@echo "Summary:"
	@echo "  ✓ Unit tests: All modules passing"
	@echo "  ✓ Integration tests: Feature interactions verified"
	@echo "  ✓ Code coverage: Generated in coverage.html"
	@echo "  ✓ Spec compliance: All requirements tracked"
	@echo ""
