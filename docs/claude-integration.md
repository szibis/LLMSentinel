# Claude Code Integration Guide

## Prerequisites

- Claude Code CLI installed
- `claude-escalate` binary in your PATH

## Installation

### Option 1: Download Binary

```bash
curl -sSL https://raw.githubusercontent.com/szibis/claude-escalate/main/scripts/install.sh | bash
```

### Option 2: Build from Source

```bash
go install github.com/szibis/claude-escalate/cmd/claude-escalate@latest
```

### Option 3: Docker

```bash
docker run -p 8077:8077 ghcr.io/szibis/claude-escalate:latest
```

## Hook Configuration

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

This single hook replaces multiple bash scripts. It handles all escalation logic in one binary.

## Commands

Use these in your Claude Code conversation:

| Command | Effect |
|---------|--------|
| `/escalate` | Switch to Sonnet (default) |
| `/escalate to sonnet` | Switch to Sonnet |
| `/escalate to opus` | Switch to Opus |
| `/escalate to haiku` | Downgrade to Haiku |

## What Happens Automatically

### Frustration Detection

When you express frustration after 2+ attempts on the same model, the system suggests escalation:

```
You: "that didn't work, same error again"
System hint: "Haiku seems stuck (3 attempts). Try: /escalate to sonnet"
```

Keywords detected: "didn't work", "still broken", "going in circles", "same error", "try again", etc.

### Circular Reasoning Detection

When the same domain concepts repeat across 4+ conversation turns:

```
Turn 1: "fix the race condition in thread code"
Turn 2: "concurrent access still fails"
Turn 3: "thread synchronization issue"
Turn 4: "mutex not preventing race"
System hint: "Circular pattern detected. Consider: /escalate to sonnet"
```

### Auto De-escalation

When you confirm a problem is solved on an expensive model:

```
You: "That works perfectly, thanks!"
System: "Auto-downgrade: Haiku (problem solved, saving cost)"
```

### Predictive Routing

After 5+ escalations for a task type, the system proactively suggests:

```
You: "fix the deadlock in concurrent code"
System hint: "Predictive: concurrency tasks historically need escalation (7 prior). Consider: /escalate to sonnet"
```

## Dashboard

Start the analytics dashboard:

```bash
claude-escalate dashboard --port 8077
```

Open `http://localhost:8077` to see:
- Current model indicator
- Escalation/de-escalation counts
- Task type performance breakdown
- Predictive escalation status
- Recent history timeline

## Data Storage

All data is stored in SQLite at `~/.claude/data/escalation/escalation.db`:
- Escalation events with timestamps and task types
- Conversation turns with extracted concepts
- Session state for cascade tracking

## Troubleshooting

### Hook not firing

1. Verify binary is in PATH: `which claude-escalate`
2. Test manually: `echo '{"prompt":"test"}' | claude-escalate hook`
3. Check settings.json syntax is valid JSON

### Wrong model selected

The hook modifies `~/.claude/settings.json`. Check current state:

```bash
jq '.model, .effortLevel' ~/.claude/settings.json
```

### Dashboard not loading

1. Check if port is in use: `lsof -i :8077`
2. Try a different port: `claude-escalate dashboard --port 8078`

### Reset all data

```bash
rm ~/.claude/data/escalation/escalation.db
```

Or use the CLI:

```bash
claude-escalate stats reset --confirm
```
