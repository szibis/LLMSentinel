# Capture normal Claude Code and Codex sessions

Learning mode keeps the original CLI connected directly to Anthropic or OpenAI. An optional local hook copies conversation evidence to a private JSONL spool, then optionally to Sentinel's learning collector. It never changes authentication, model/provider selection, subscription routing, or provider base URLs, and it never makes another commercial model request. Serving is the separate CLI → Sentinel → local model route. Start with the [mode guide](client-modes.md) to choose the path.

Nothing is enabled or installed automatically. No global CLI configuration or running lab session is modified. `sentinel-tools capture` uses Go's standard library; it does not require Python, credentials, or external packages. Private file handling supports macOS and Linux; other platforms fail closed.

## Preview and enable

Run from the Sentinel repository. Build the utility with `rtk go build -o bin/sentinel-tools ./cmd/sentinel-tools`. Choose an existing private directory and a canonical absolute output filename. On macOS `/tmp` and `/var` are aliases; use their resolved `/private/...` paths. Keep capture data outside tracked source files; `.sentinel-lab/` is suitable for this local lab. Previews embed the executable's absolute path; rebuild it in the same location to preserve installed hooks.

Preview the complete Claude settings fragment (prints JSON, creates no files):

```sh
rtk ./bin/sentinel-tools capture --client claude \
  --output /Users/slawomirskowron/projects/model_training/Sentinel/.sentinel-lab/client-capture.jsonl \
  --collector http://127.0.0.1:19094/sentinel/training/events --preview-config
```

Review and merge its `UserPromptSubmit` and `Stop` handlers into the desired Claude settings scope using `/hooks` or an explicitly chosen settings file. Preserve existing hook entries. A new invocation can load a reviewed settings file with `claude --settings /absolute/path/to/reviewed-settings.json`; do not restart the active local lab. Without `--collector`, capture is entirely offline.

For installed Codex CLI 0.160.1, preview native hook JSON:

```sh
rtk ./bin/sentinel-tools capture --client codex --native-hooks \
  --output /Users/slawomirskowron/projects/model_training/Sentinel/.sentinel-lab/client-capture.jsonl \
  --collector http://127.0.0.1:19094/sentinel/training/events --preview-config
```

Review and merge the generated JSON into the chosen `.codex/hooks.json` scope, preserving existing hooks. Open `/hooks` in Codex to review and trust the exact definitions; project hooks also require a trusted project config layer. Do not bypass hook trust. Native hooks copy the submitted prompt and final response as separate events with shared session/turn IDs. They include the active model. For CLI 0.160.1, Stop additionally parses its explicitly supplied rollout path for reported usage and scoped user/assistant text. Unknown versions or incompatible data retain the hook's text and unknown usage.

Codex also supports a simpler legacy `notify` adapter. Omit `--native-hooks` to preview a TOML `notify = [...]` array. Review it and pass its array as a one-invocation override using Codex's supported `-c 'notify=[...]'`, or merge it into a deliberately selected user config. Codex appends a JSON notification as the final command argument. The notification contains `input-messages` and `last-assistant-message`, thread ID and turn ID, but does not report model, actual tokens or latency. Choose native hooks **or** notify to avoid overlapping records; neither installs itself.

Keep ordinary commercial provider configuration unchanged. Send copies to the dedicated learning collector on port 19094, separate from the serving lab on port 19090. A collector outage does not stop the provider conversation: the hook first appends its spool record, attempts a direct loopback HTTP POST, and retains the spool on failure. The complete HTTP attempt has a 0.3-second timeout; no proxy, DNS lookup, redirect or response body is used. Only literal `http://127.0.0.1/...` and `http://[::1]/...` URLs are accepted. There is no automatic spool replay yet.

## What the evidence contains

Every record includes schema version, event UUID, UTC capture timestamp, source/client/event type, model, role (`conversation`), nullable run/session/turn IDs, `inputs` and `outputs` text arrays, actual `usage`, nullable latency and its source, truncation status, and `quality: {status: "unscored", training_eligible: false}`. Reserved nullable `pair_id`, `reference_event_id`, and `local_event_id` support a later comparison between the commercial reference and local output. Hooks do not grade answers or approve training targets.

For Codex 0.160.1, the adapter prefers durable `token_usage_record` entries with thread/session/turn IDs, response ID and provider-observed usage. It checks identities, deduplicates response IDs and sums matching records. `usage.source = "codex_rollout_token_usage_record"`, `scope = "observed_turn_calls"`, and `reported_calls` identify the measurement. `coverage = "reported_calls_only"` means responses without usage are omitted; do not imply complete turn billing. Input, cached input, output, reasoning output and total tokens retain the CLI's counters. Reasoning is a subset of output and is not added again.

Otherwise a matching turn's final `event_msg`/`token_count` can supply `last_token_usage` with `source = "codex_rollout_token_count"`, `scope = "last_call"`. This is one CLI-reported call, not the whole turn or session total. Validation rejects missing fields, negative/bool counts, inconsistent totals and known synthetic context-fill shapes. Source and scope must accompany these local CLI measurements. Unknown versions, missing metadata or unmatched IDs stay `null`. The bounded reader retains session metadata plus the tail; it does not reconstruct paginated or compressed history. It never tokenizes captured text to manufacture actual counts.

Billing is separate. `billing.class` defaults to `direct_unknown`, and `billing.cost_usd` remains `null`: hooks provide no monetary charge. If the user knows the arrangement, add `--billing-class subscription` or `--billing-class api` to capture/preview commands. Generated hooks preserve that choice with provenance `explicit_cli_option`; no auth files are inspected. Subscription/ChatGPT credit usage must not be priced automatically as retail API usage. Even an explicit API classification needs applicable pricing and billing evidence for USD costs. Future estimates should use separate estimated-cost fields with pricing date/source.

Claude `UserPromptSubmit` captures the raw submitted prompt. `Stop` reads **only** the explicitly supplied hook `transcript_path`, finds the newest text user message in the bounded tail, then captures assistant text blocks and sums reported usage once per unique assistant API message ID. Separate text blocks survive; duplicate rows do not multiply tokens. Claude's `input_tokens` is the reported non-cache input field; cache read and creation tokens remain separately named fields. Missing counts stay `null`, and estimates are never recorded as actual usage. `latency_ms`, when both transcript timestamps exist, is the elapsed user-to-assistant transcript span, including tools and retries; it is not pure model inference latency. Transcript usage is a local CLI record, not independently verified billing evidence. The JSONL transcript schema is not a promised stable public API, so fixture compatibility needs rechecking after CLI upgrades.

These adapters capture conversational text, not the entire wire prompt: hidden system/developer instructions, complete accumulated tool state, binary/images, thinking blocks, and all provider request/response envelopes are not reproduced. Native Codex Stop starts with the last assistant message and can enrich it with known-version rollout text. Bounded reads can omit earlier content; `truncated` identifies clipping, but provider-side omissions are still possible. Prompt and Stop records should be joined by session and turn when provided; Claude prompt events may have no turn ID. Do not claim these are complete reproductions of commercial requests.

## Privacy and resource limits

Capture is opt-in and includes personal text after best-effort redaction. Common API key formats, Bearer tokens, secret assignments and PEM private keys are replaced with `[REDACTED]`. This is not a complete detector for arbitrary sensitive material. Capture only sessions selected for learning and review candidate data before any training export.

The spool is mode 0600, owned by the current user, a regular singly linked file, and protected with an advisory exclusive append lock and fsync. Paths must be absolute and canonical; symlinks are rejected. Use a trusted private parent directory that other users cannot replace. The transcript is opened read-only, must be a user-owned regular file, and is never discovered by scanning home directories. No credentials or unrelated sessions are read. Malformed capture input produces a generic stderr diagnostic without printing private content or injecting hook decisions.

Limits: 1 MiB hook input, final 8 MiB/512 transcript rows plus at most 256 KiB for the first Codex metadata row, 32,768 Unicode characters per captured text string, at most 64 input/output entries per record, and 512 KiB serialized record. Oversized records are skipped rather than written partially. The spool has no automatic rotation: arrange retention explicitly. Concurrent records are serialized by a lock; process crashes or disk errors can still leave an incomplete final line, so downstream importers must validate JSONL records. Tests use fixtures only.

## Official interfaces and remaining telemetry work

Anthropic documents the stdin payloads, settings structure, `UserPromptSubmit`, and `Stop` fields in the [Claude Code hooks reference](https://code.claude.com/docs/en/hooks). Current Stop payloads include `last_assistant_message`; older clients can obtain assistant text from their explicitly supplied transcript.

OpenAI documents hook discovery, trust, stdin fields, model/turn IDs, `UserPromptSubmit` and `Stop`, and the unstable transcript format in [Codex hooks](https://learn.chatgpt.com/docs/hooks). The [advanced configuration reference](https://developers.openai.com/codex/config-advanced) documents legacy notify argv JSON and OpenTelemetry. The [official legacy notify implementation](https://github.com/openai/codex/blob/main/codex-rs/hooks/src/legacy_notify.rs) confirms its text-only payload.

The rollout adapter is pinned to official [0.160.1 protocol definitions](https://github.com/openai/codex/blob/rust-v0.160.1/codex-rs/protocol/src/protocol.rs), [persisted rollout wire format](https://github.com/openai/codex/blob/rust-v0.160.1/codex-rs/history/src/rollout_payload.rs) and the [provider-observed usage persistence test](https://github.com/openai/codex/blob/rust-v0.160.1/codex-rs/core/tests/suite/token_usage_rollout.rs). The upstream test confirms per-response accumulation and absence of a usage record for a response without usage. Fixtures reproduce these source shapes; no private local session contents were inspected.

Codex OTel can export opted-in `codex.user_prompt` content (`log_user_prompt = true`), API request durations and `codex.sse_event` token counts on `response.completed`; model/session metadata accompanies events. The documented telemetry does not promise complete assistant response text or the full wire prompt. A future local OTLP receiver could join reported token counts to native hook text, with explicit source/provenance and correlation checks. This adapter does not enable OTel or conflate estimated tokens with its actual counters. Likewise, collecting all tool events or exact commercial wire prompts is separate work; changing commercial auth or inserting Sentinel as a commercial inference proxy is not required for the implemented copies.

Verification:

```sh
rtk go test -race ./internal/clientcapture
rtk go vet ./internal/clientcapture
```
