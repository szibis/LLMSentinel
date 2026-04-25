# How It Works

## Architecture

claude-escalate runs as a single Claude Code `UserPromptSubmit` hook. On every user prompt, it:

1. Reads the prompt from stdin (JSON)
2. Runs through a decision pipeline (<5ms total)
3. Outputs a JSON response to stdout

```
stdin → [parse] → [classify] → [detect] → [decide] → [act] → stdout
```

## Decision Pipeline

The pipeline evaluates in priority order. First match wins:

### 1. Meta-command Check
If the prompt starts with `/escalate`, `/effort`, or `/model`, handle it directly. No further checks needed.

### 2. Predictive Routing (Phase 5)
Query the SQLite database for this task type's escalation history. If the type has been escalated 5+ times in the past, proactively suggest escalation.

### 3. Frustration Detection (Phase 2)
Check the prompt against 30+ frustration patterns. If matched AND the user has sent 2+ prompts on the same model recently, suggest escalation.

### 4. Circular Reasoning Detection (Phase 4)
Extract domain concepts from the prompt. Compare against the last 6 conversation turns. If 2+ concepts appear in 3+ turns, suggest escalation.

### 5. Success Signal Detection (Phase 3)
Check for success phrases ("works great", "thanks", "perfect"). If on an expensive model with an active escalation session, auto-downgrade one tier.

### 6. Pass Through
If none of the above matched, silently allow the prompt to continue.

## Task Classification

Prompts are classified into 11 domain types using regex patterns:

| Domain | Example Keywords |
|--------|------------------|
| concurrency | race, thread, mutex, deadlock, goroutine |
| parsing | regex, parse, grammar, AST, tokenize |
| optimization | optimize, performance, cache, latency |
| debugging | debug, traceback, segfault, panic |
| architecture | design, microservice, domain-driven |
| security | crypto, auth, TLS, JWT, XSS |
| database | SQL, migration, schema, transaction |
| networking | socket, TCP, HTTP, gRPC, proxy |
| testing | test, mock, assert, coverage |
| devops | docker, kubernetes, CI/CD, terraform |
| general | (anything else) |

## Model Cascade

```
Opus ($$$)  ← Deep reasoning, architecture
  ↕
Sonnet ($$) ← Precision code, debugging
  ↕
Haiku ($)   ← Simple tasks, search, lookup
```

Escalation moves up the chain. De-escalation moves down.
Opus → Sonnet → Haiku (one step at a time for cascade support).

## Concept Extraction

For circular reasoning detection, domain-specific keywords are extracted from each prompt:

- Concurrency: race, concurrent, thread, mutex, lock, async, parallel
- Performance: optimize, speed, cache, memory, latency, profile
- Errors: error, bug, crash, exception, fail, broke, stuck

These concepts are stored per-turn in SQLite. The detector counts how many turns share the same concepts.

## Session Management

Escalation sessions are tracked in SQLite:
- Created when `/escalate` is used
- Checked during de-escalation (must have active session or 2+ turns on expensive model)
- Opus → Sonnet keeps session alive (for cascade)
- Sonnet → Haiku clears session (bottom of chain)

## Storage Schema

```sql
-- Escalation/de-escalation events
escalations(id, timestamp, from_model, to_model, task_type, reason)

-- Conversation turns with concepts
turns(id, timestamp, model, concepts)

-- Session state (key-value)
sessions(key, value, updated_at)
```
