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
	@echo "Targets:"
	@echo "  build         Build binary"
	@echo "  test          Run tests"
	@echo "  test-cover    Run tests with coverage"
	@echo "  lint          Run linters"
	@echo "  install       Build and install to ~/.local/bin"
	@echo "  install-hook  Install + configure Claude Code hook"
	@echo "  dev           Start dashboard in development mode"
	@echo "  clean         Remove build artifacts"
