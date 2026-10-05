# Sentinel upstream provenance

Source: https://github.com/szibis/LLMSentinel

Revision: `2c4b43e4beddad296dc93541f1b80611c492bf4d` (2026-04-29).

The original snapshot was obtained through the GitHub API archive because shell Git network access was unavailable. Local Git now records the canonical signed upstream base and lab commits, with a shallow history boundary at the imported base. The upstream Apache-2.0 license and source attribution are preserved. The remote is the existing `szibis/LLMSentinel`; no ReliablyObserve repository was created.

The extension adds an independently buildable local model transport gateway, an isolated Claude Code lab, Qwen role routing, and build/release repairs. Existing escalation/tool modules are preserved; the local gateway uses its configured native runtime. See [local gateway setup](docs/local-gateway.md) and [Qwen roles](docs/claude-qwen-roles.md). These changes are proposed back to the existing `szibis/LLMSentinel` repository.
