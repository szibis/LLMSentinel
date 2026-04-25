#!/bin/bash
# Test runner with coverage reporting

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_ROOT"

echo "═══════════════════════════════════════"
echo "Claude Escalate - Test Suite"
echo "═══════════════════════════════════════"
echo ""

# Parse arguments
COVERAGE=false
VERBOSE=false
RACE=false
BENCH=false
TARGET="./..."

while [[ $# -gt 0 ]]; do
    case $1 in
        --coverage)
            COVERAGE=true
            shift
            ;;
        --verbose|-v)
            VERBOSE=true
            shift
            ;;
        --race)
            RACE=true
            shift
            ;;
        --bench)
            BENCH=true
            shift
            ;;
        --package)
            TARGET="$2"
            shift 2
            ;;
        *)
            echo "Unknown option: $1"
            echo "Usage: $0 [--coverage] [--verbose] [--race] [--bench] [--package PATH]"
            exit 1
            ;;
    esac
done

# Build test command
TEST_CMD="go test"

if [ "$VERBOSE" = true ]; then
    TEST_CMD="$TEST_CMD -v"
fi

if [ "$RACE" = true ]; then
    TEST_CMD="$TEST_CMD -race"
fi

if [ "$COVERAGE" = true ]; then
    TEST_CMD="$TEST_CMD -coverprofile=coverage.out -covermode=atomic"
fi

if [ "$BENCH" = true ]; then
    TEST_CMD="$TEST_CMD -bench=. -benchmem"
fi

TEST_CMD="$TEST_CMD $TARGET"

echo "Running: $TEST_CMD"
echo ""

# Run tests
if eval "$TEST_CMD"; then
    echo ""
    echo "✅ All tests passed!"
    echo ""

    if [ "$COVERAGE" = true ]; then
        echo "Coverage Summary:"
        go tool cover -func=coverage.out | tail -1
        echo ""
        echo "Generate HTML coverage report:"
        echo "  go tool cover -html=coverage.out"
    fi
else
    echo ""
    echo "❌ Tests failed!"
    exit 1
fi

# Run benchmarks if requested
if [ "$BENCH" = true ]; then
    echo ""
    echo "═══════════════════════════════════════"
    echo "Benchmark Results"
    echo "═══════════════════════════════════════"
    echo ""
    echo "Key performance targets:"
    echo "  - SentimentDetect: <1ms per prompt"
    echo "  - BudgetCheck: <100µs per request"
    echo "  - RegistryGetBest: <10µs"
    echo ""
fi
