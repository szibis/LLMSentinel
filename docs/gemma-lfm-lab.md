# Gemma and LFM role lab

The lab can serve Gemma 4 26B-A4B Instruct as the Sonnet/Opus backend and
LFM2.5-8B-A1B as Haiku. Claude Code and Codex retain their normal Anthropic or
OpenAI protocol formats. These roles are local effort profiles, not claims of
commercial-model quality equivalence. Sentinel remains pure Go; MLX-Flash is
the external native inference runtime.

| Role | Artifact | Profile |
|---|---|---|
| Haiku | LFM for requests without tool definitions; Gemma for tool-bearing requests | 1,024 output-token limit, original Haiku alias retained |
| Sonnet | `mlx-community/gemma-4-26b-a4b-it-4bit` | Thinking disabled, 4,096 output-token limit |
| Opus | Same Gemma artifact | Thinking enabled, 8,192 output-token limit |

The local runtime advertises its actual family and reasoning controls. Sentinel
pins those capabilities for a request and its format recovery, selects the
appropriate template control and accepts only completed final output. Gemma
thought channels and LFM/Qwen think sections are filtered before text or tool
arguments reach clients. All role Chat Completions, including streams, use the
buffered validation path. Unknown or inconsistent families fail before model
dispatch. Existing Qwen profiles remain supported.

The LFM lab defaults to `--haiku-tool-role sonnet`: requests advertising tools
use Gemma even when the model ultimately answers without a tool call. LFM did
not pass every native agent task, so this is an explicit quality routing policy,
not evidence that LFM matches the larger model. Set `haiku_tool_role` to `haiku`
in the saved `runtime.json` to use LFM for these requests, or select `sonnet` or
`opus` explicitly. The automatic default applies only to the cached `lfm2_moe`
small model; Qwen configurations retain their existing routing. The control API
exposes this setting and activity records the actual backend route.

The lab also enables bounded local role recovery. A failed validated generation
can escalate to a stronger local role while preserving the client alias and
original token cap; successful recovery retains that route for the identified
session for 20 minutes. Fresh sessions start with the configured role admission. See
[native client checks and recovery](native-cli-quality.md) for limits and evidence.

Gemma uses its native tool declarations, typed call frames and tool-response
history, rendered by Sentinel to match the cached chat template. Its final text
or requested JSON is returned directly. Marker-bearing file evidence is escaped
so it cannot create control frames. LFM retains the validated JSON tool envelope.
Tool names, required arguments and schemas are validated in both paths;
malformed or incomplete outputs do not execute tools. Their tool languages are
not advertised as interchangeable Qwen formats. LFM's recommended repetition penalty is
applied by the native runtime; Sentinel does not pretend its reasoning can be
disabled. Its thinking may consume more output tokens than a nonreasoning small
model, so route quality and total task usage need continued evaluation.

## Cache identity and dashboard

Sentinel sends a hashed stable session cache scope when Claude metadata
`user_id`, Responses metadata `session_id`, or an explicit session header is
available. Otherwise, the request ID partitions the cache and only recovery
within that request can reuse state. Never infer cross-session reuse from a
shared model or equal user text.

MLX-Flash retains bounded, exact native token-prefix snapshots within each
model/profile/session partition. This avoids recomputing shared prompt history
without reusing an old answer or tool result. The legacy semantic response
cache is not connected to the role gateway: its original key omits parameters
and its fixed savings estimate is unsuitable for live agent accounting.

At [the local dashboard](http://localhost:8077/dashboard), model cards show cache
hits/misses, actual reused and processed input counts, retained bytes and limit,
entries/evictions, last prompt counts and time to first token. History adds
interval reuse and cold/warm first-token graphs. Active runtime optimizations
are visible beside native generation throughput. Unknown fields remain unknown.
The one-second browser refresh shares a four-second telemetry sample; graphs
are observed history, not a complete request ledger. Runtime restart resets
cache counters; dashboard restart resets its in-memory history.

Local cache reuse reduces prefill work; it does not shrink logical context or
prove commercial billing savings. Costs remain unknown unless an actual
provider reports the relevant usage/pricing evidence.

## Verification and rollout

Pinned native artifacts were checked against Hugging Face sizes and SHA256
weights: Gemma revision `0d77464eeb233a2da68ebf9d7dc4edaac7db956d`, LFM revision
`2e92b640a63d47ad4dcf81a19a366b902356b3bc`.

Native greedy cold/cached/fresh checks returned identical completed outputs.
For approximately 1,200 input tokens, warm uncached versus cached first-token
time was 488 ms versus 28 ms for Gemma and 184 ms versus 12 ms for LFM. These are
single synthetic marker checks, not general latency or quality guarantees.

Live Anthropic Haiku/Sonnet/Opus marker requests, required-tool calls on both
Haiku and Sonnet, and an OpenAI Responses marker request passed. A generated Go
deduplication function passed behavior tests for ordering, duplicates, empty
values, case sensitivity and input/output independence. Broader repository
research, factual quality and long-running agent evaluations remain necessary.

Keep prior `runtime.json` settings and cached Qwen artifacts for rollback.
The existing Go `qwen-metal` smoke command retains its CLI/env names while
checking the selected architecture, supported reasoning profiles and real
role/tool behavior. No commercial credentials or provider calls are needed.
