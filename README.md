# claude-escalate

**Intelligent model escalation for Claude Code** — automatically detect when your AI is stuck, escalate to more capable models, and downgrade when done to save cost.

[![Build](https://github.com/szibis/claude-escalate/actions/workflows/build.yml/badge.svg)](https://github.com/szibis/claude-escalate/actions/workflows/build.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/szibis/claude-escalate)](https://goreportcard.com/report/github.com/szibis/claude-escalate)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

---

## The Problem

When using Claude Code with cost-optimized models (Haiku), you hit a common pattern:

```
You: "debug this race condition"
Haiku: [attempts fix] -> wrong approach
You: "that didn't work"
Haiku: [same approach again] -> circular reasoning
You: "still broken, going in circles"
Haiku: [repeats itself] -> tokens wasted, no progress
```

**Result:** 1,500+ tokens wasted, problem unsolved, manual intervention required.

## The Solution

`claude-escalate` detects when models are stuck and manages intelligent model switching:

```
You: "debug this race condition"
Haiku: [attempts fix] -> wrong approach
You: "that didn't work"
claude-escalate: "Haiku seems stuck (2 attempts). Try: /escalate to sonnet"
You: "/escalate to sonnet"
Sonnet: [solves it]
You: "Perfect, that works!"
claude-escalate: "Auto-downgrade: Haiku (problem solved, saving cost)"
```

**Result:** 1,300 tokens, problem solved, automatically back to cheap model.

---

## Features

### Five-Layer Intelligence

| Layer | What | How |
|-------|------|-----|
| **Frustration Detection** | Detects when you're stuck | Monitors for "didn't work", "still broken", "going in circles" |
| **Circular Reasoning** | Detects repeated concepts | Tracks domain keywords across conversation turns |
| **Manual Escalation** | User-controlled switching | `/escalate to sonnet`, `/escalate to opus` |
| **Auto De-escalation** | Downgrades on success | Detects "works!", "perfect", "thanks" and switches to cheaper model |
| **Predictive Routing** | Learns from history | "Concurrency tasks usually need Sonnet" and suggests proactively |

### Local Dashboard

Real-time analytics at `http://localhost:8077`:
- Current model indicator
- Escalation/de-escalation counts and success rate
- Task type performance breakdown
- Predictive escalation status
- Recent history timeline

### One Binary, One Hook

Replaces 6 separate bash scripts with a single Go binary:

```json
{
  "hooks": {
    "UserPromptSubmit": [{
      "hooks": [{
        "type": "command",
        "command": "claude-escalate hook",
        "timeout": 5
      }]
    }]
  }
}
```

---

## Quick Start

### Install

```bash
# From source
go install github.com/szibis/claude-escalate/cmd/claude-escalate@latest

# Or download binary
curl -sSL https://github.com/szibis/claude-escalate/releases/latest/download/claude-escalate-$(uname -s | tr A-Z a-z)-$(uname -m) \
  -o ~/.local/bin/claude-escalate
chmod +x ~/.local/bin/claude-escalate
```

### Configure Claude Code

Add to `~/.claude/settings.json`:

```json
{
  "hooks": {
    "UserPromptSubmit": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "claude-escalate hook",
            "timeout": 5
          }
        ]
      }
    ]
  }
}
```

### Start Dashboard

```bash
claude-escalate dashboard --port 8077
```

### View Statistics

```bash
claude-escalate stats summary
claude-escalate stats types
claude-escalate stats predictions
claude-escalate stats history
```

---

## How It Works

### Escalation Flow

```
User sends prompt to Claude Code
                |
                v
claude-escalate hook (single binary, <5ms)

  1. Is this /escalate command?
     YES -> Switch model, log event, respond

  2. Predictive check: known-hard task type?
     YES -> Suggest escalation proactively

  3. Frustration detected? (keywords + retry count)
     YES -> Suggest: "Haiku stuck. Try /escalate"

  4. Circular reasoning? (repeated concepts across turns)
     YES -> Suggest: "Pattern detected. Consider /escalate"

  5. Success signal? (on expensive model)
     YES -> Auto-downgrade to cheaper model

  6. None of the above -> pass through silently
```

### Cost Optimization

| Scenario | Tokens | Cost | Outcome |
|----------|--------|------|---------|
| Haiku circles (5 attempts) | ~2,500 | High waste | Problem unsolved |
| Haiku then escalate to Sonnet | ~1,300 | Lower | **Problem solved** |
| Escalate then solve then downgrade | ~1,500 | Optimal | **Solved + ready for cheap tasks** |

### Model Cascade

```
Opus ($$$)  <-  For deep reasoning, architecture
  ^ escalate / v de-escalate
Sonnet ($$) <-  For precision code, debugging
  ^ escalate / v de-escalate
Haiku ($)   <-  For simple tasks, search, lookup
```

---

## Commands

### User Commands (in Claude Code)

| Command | Effect |
|---------|--------|
| `/escalate` | Escalate to Sonnet (default) |
| `/escalate to sonnet` | Escalate to Sonnet explicitly |
| `/escalate to opus` | Escalate to Opus (deep reasoning) |
| `/escalate to haiku` | Downgrade to Haiku (cost saving) |

### CLI Commands

| Command | Description |
|---------|-------------|
| `claude-escalate hook` | Run as Claude Code hook (stdin/stdout) |
| `claude-escalate dashboard` | Start local web dashboard |
| `claude-escalate stats summary` | Overall statistics |
| `claude-escalate stats types` | Task type breakdown |
| `claude-escalate stats predictions` | Predictive routing status |
| `claude-escalate stats history` | Recent events |
| `claude-escalate version` | Show version |

---

## Architecture

```
claude-escalate/
  cmd/claude-escalate/       # CLI entry point
  internal/
    hook/                    # Claude Code JSON protocol
    detect/                  # Frustration + circular detection
    classify/                # Task type classification
    store/                   # SQLite persistent storage
    dashboard/               # Local web UI
    config/                  # Configuration
  docs/                      # Documentation
  .github/workflows/         # CI/CD
  Makefile                   # Build targets
  Dockerfile                 # Container image
```

### Storage

All data persists in SQLite at `~/.claude/data/escalation/escalation.db`:
- Escalation events (from/to model, task type, reason, timestamp)
- Conversation turns (model, extracted concepts)
- Session state (active escalation markers)

### Performance

| Metric | Value |
|--------|-------|
| Hook startup | <5ms |
| Decision time | <2ms |
| Total overhead per prompt | **<10ms** |
| Binary size | ~8MB |
| Memory usage | ~5MB |

---

## Comparison

| Feature | claude-escalate | Tokenwise | Smart Router | LiteLLM |
|---------|:-:|:-:|:-:|:-:|
| Frustration detection | Yes | -- | -- | -- |
| Circular reasoning detection | Yes | -- | -- | -- |
| Auto de-escalation | Yes | -- | -- | -- |
| Predictive routing | Yes | Partial | -- | -- |
| Native Claude Code hooks | Yes | -- | -- | -- |
| Local dashboard | Yes | Yes | -- | Yes |
| SQLite analytics | Yes | Yes | -- | Yes |
| Task classification | Yes | Yes | Yes | -- |
| Single binary | Yes | -- | -- | -- |
| Zero configuration | Yes | -- | -- | -- |

---

## Development

```bash
# Clone
git clone https://github.com/szibis/claude-escalate.git
cd claude-escalate

# Build
make build

# Test
make test

# Lint
make lint

# Run dashboard locally
make dev

# Run full test suite with coverage
make test-cover
```

---

## Contributing

We welcome contributions! Please:

1. Fork the repository
2. Create a feature branch (`git checkout -b feat/my-feature`)
3. Run tests (`make test`)
4. Commit your changes
5. Open a Pull Request

---

## License

Apache License 2.0 -- see [LICENSE](LICENSE).

---

## Acknowledgments

Built for the [Claude Code](https://docs.anthropic.com/en/docs/claude-code) ecosystem. Inspired by the cost optimization patterns in [Tokenwise](https://github.com/tanishmisra9/tokenwise) and the routing concepts in [Smart Router](https://github.com/MatthdV/smart-router).
