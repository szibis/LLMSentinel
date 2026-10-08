# Protocol edge and fuzz regression tests

Model-free end-to-end tests use two real HTTP servers: the client-facing
Sentinel API and a synthetic MLX runtime. They cover Anthropic Messages,
OpenAI Responses and Chat Completions, both JSON and buffered SSE. They do
not execute client tools, download models or contact paid providers.

Run deterministic regressions and all fuzz seeds:

```sh
go test ./internal/localgateway -run "TestProtocolEdges|TestNullBackendUsage|Fuzz" -count=1
go test -race ./internal/localgateway
```

Run bounded mutation campaigns, one target at a time:

```sh
for target in FuzzGemmaLiteralRoundTrip FuzzModelOutputBoundary FuzzStrictJSONKeys FuzzProtocolLiteralHTTP FuzzProtocolToolProposalHTTP; do
  go test ./internal/localgateway -run "^$" -fuzz "^${target}$" -parallel=2 -fuzztime=30s -timeout=90s
done
```

CI discovers targets in both `internal/fuzz` and `internal/localgateway`, runs
each for ten seconds with two workers, and uploads minimized failures. Normal
`go test` replays committed seeds and files under `testdata/fuzz/<target>/`.
Use the reproduction command printed by Go, then retain the synthetic input
with its fix. Never commit private conversations, credentials or model weights.

## Properties and regressions

- Duplicate request keys, escaped duplicates and case-aliased protocol fields
  fail with 400 before inference.
- Ambiguous runtime metadata, invalid UTF-8, trailing JSON and negative token
  counts fail with 502 before tool emission or SSE, without correction retries.
- Null or missing usage remains unknown in internal usage provenance.
- All normalized formats reject invalid UTF-8 and limit tool calls to eight,
  including Anthropic-style content blocks.
- Gemma literals preserve typed nested data, Unicode, newlines, NUL, quotes
  and control-token-looking strings.
- Valid arguments survive real JSON/SSE translation unchanged. An independent
  wire reader reconstructs arguments and checks one stream terminal event.
- Adversarial proposals produce registered, schema-valid tool turns or bounded
  JSON errors before streaming. Mixed valid/invalid proposals cannot emit a
  partial tool turn; correction attempts are bounded.
- Case-sensitive dictionary keys remain data: `Type` and `type` can coexist.

These tests found real overwrite, encoding and unknown-usage bugs. Bounded
fuzzing increases regression coverage; it does not prove all possible requests
or semantic model quality. Real cached-model API and native Claude Code/Codex
probes remain separate validation layers.
