# Model translation and CLI feature reuse

Sentinel keeps the installed CLI in charge of its agent loop, tool execution,
approvals, sandbox and local integrations. A backend model supplies text and tool
intent; Sentinel converts recognized output into validated calls in the endpoint
protocol. The model alias selects a route, not an output parser. The same
backend can serve Anthropic Messages, OpenAI Responses and Chat Completions.
This does not establish equivalent proprietary-model quality.

## Translation boundary

The Go gateway normalizes the canonical `text`/`tool_calls` envelope, OpenAI
function calls with serialized arguments, Anthropic text/tool-use content,
Qwen function/parameter blocks, tagged JSON calls, and Gemma 4 native calls.
Plain text and ordinary JSON final answers remain final answers. String arguments
are preserved, including protocol examples inside JSON strings. Numeric decoding
currently uses float64: integers beyond its exact range are not lossless and must
not be used for precise identifiers. Duplicate keys,
competing envelopes, incomplete frames and invented tool-result payloads fail
validation. Available-name, schema and tool-choice checks still run before a
call reaches the client. Streams are buffered until validation succeeds;
this is not native token streaming.

Gemma syntax follows Google's [function-calling specification](https://ai.google.dev/gemma/docs/capabilities/text/function-calling-gemma4).
Future formats need a decoder, negative fixtures and client protocol proofs
before being advertised. Prose is never converted into a tool call by guessing
names, paths or arguments.

Claude Code text-only `system` messages within conversation input are merged
with client context ahead of the output contract. Tool blocks in that context
are rejected. Short promises to read/research a requested deliverable without
a call or answer enter the existing bounded quality recovery path, including
the first turn. Requested plans and literal replies are exempt. This is a
narrow progress heuristic, not a factual or semantic evaluator.

## Compatibility matrix

The isolated Claude launcher defaults to full CLI mode rather than `--bare`
with a seven-tool allowlist. Its normal tool catalog, profile skills/commands
and configured hooks can participate. Set `LAB_CLAUDE_FEATURES=minimal` for the
previous bare mode. Both retain normal permissions, explicit lab controls,
the isolated profile and empty MCP configuration. Provider tool search and
thinking betas remain disabled. Configure and verify MCP separately; the
launcher does not automatically import personal integrations.

| Capability | Current local adapter | Evidence or remaining work |
| --- | --- | --- |
| Local read, shell, edit and function tools | CLI executes validated advertised functions | Unit/API gates; real Claude Read and Codex read-only shell probes passed |
| Codex apply_patch | Canonical installed custom grammar and one-level namespaces supported | Existing grammar/namespace tests; latest real probe exercised shell, not editing |
| Instructions, skills, slash commands and hooks | CLI-owned; text context carried through gateway | Isolated controls tested; each integration needs a representative end-to-end task |
| MCP tools exposed as supported functions | Names and schemas use the normal tool path | Discovery/authentication belong to CLI; per-server behavior not certified |
| Subagents, planning and agent teams | Advertised functions use normal validators and role routes | Full delegated workflows not certified; planning quality not guaranteed |
| Permissions and sandbox | Original CLI executor remains responsible | Gateway never executes calls; add client proofs for denied operations |
| Conversation and tool-result continuation | Explicit history and call IDs translated | Existing API tests; real probes completed tool + answer turns |
| Compaction, resume and long context | Client-supplied text history within backend limits | Provider compaction APIs and opaque/stored histories not implemented; large-context retention needs proofs |
| Images, audio and arbitrary custom grammars | Explicitly unsupported locally | Add backend capability and client fixtures before enabling |
| Provider web/file search and hosted tools | Explicitly unsupported locally | Use a configured client-executed tool or compatible commercial route; never invent results |
| Provider reasoning signatures, native cache billing and cloud sessions | No local equivalence claimed | Local thinking/cache telemetry is separate; vendor-only behavior needs a supported vendor route |

See [modes](client-modes.md), [capture](client-capture.md),
[measured quality](client-quality-inspection.md) and
[API/Metal verification](mlx-integration-verification.md). Direct learning mode
keeps the original provider connection and captures eligible local transcript
observations. Hybrid routing must satisfy the endpoint's actual capabilities;
unsupported provider features must not be silently downgraded.

Official references: [Claude Code gateway compatibility](https://code.claude.com/docs/en/llm-gateway-protocol)
and [Codex custom providers](https://developers.openai.com/codex/config-advanced).
Success depends on client version, tool catalog and backend capacity. Reusing
a CLI feature does not prove that a small model uses it well.

## Verification and future adapters

Run from the repository root:

```sh
go test -race ./internal/localgateway ./internal/qwensmoke ./cmd/sentinel-gateway
go vet ./internal/localgateway ./internal/qwensmoke ./cmd/sentinel-gateway
```

Go tests cover recognized formats, literals, malformed calls, context-only
requests and JSON/SSE translation across all three endpoints, including invalid
arguments rejected before streaming. Normal CI runs these regressions without
model downloads or commercial API calls.

October 6 local real-client probes used a synthetic Loki/VictoriaLogs/Tempo
fixture: full Claude Code selected Read and returned a comparison; Codex executed
`cat ./loki-options.md` and answered from its contents. The full Claude request
included a 22-tool catalog and about 26k native prompt tokens. These manually
run probes are separate from automated mock/API gates and the existing real
Metal smoke matrix. They do not certify every CLI feature or answer accuracy.

For another CLI, add its request/history adapter and response serializer around
the shared tool envelope, preserve client tool identity and policy, and test a
complete real tool-result turn. Add capability fixtures for each extra request
field. Keep tool decoding independent of commercial-looking model aliases.
