# Live backend dashboard

The Task quality view shows the latest explicit [bounded task probe run](task-quality-probes.md),
including failed assertions, tool calls, latency and reported tokens. These
results persist across dashboard restarts and remain dated evidence until another
run completes; dashboard refresh never starts inference.

From Sentinel's repository root:

```sh
rtk make lab-dashboard
```

Open **http://localhost:8077/dashboard**. This serves the isolated lab's backend
telemetry through native Go on 127.0.0.1. Keep the command running; Ctrl+C stops
only this dashboard. The gateway and MLX runtimes remain managed by the lab
supervisor. This live view is separate from the older configuration dashboard
served by `llm-sentinel dashboard`; they cannot occupy the same port.

Live gateway activity refreshes every second by default; choose 1, 2 or 5 seconds
in the browser. Runtime and machine telemetry is cached for four seconds, shared
across browser clients, so faster refresh does not increase heavy system polls.
Navigation links lead to overview, models, history, configuration, attempts and
tools/capabilities. The view shows:

- Actual active local adapter role, selected upstream, thinking profile, elapsed
  time, output budget, waiting count and most recent completed attempt.
- Both Qwen artifacts, completion counters, native decode speed, total generation
  time and native time to first token.
- Shared machine memory, swap and pressure, displayed once.
- Gateway mode, policy, capture state, role budgets and current quality-gate scope.
- Per-runtime prompt cache hits/misses, reused/processed prefill tokens and their
  cumulative reuse ratio, memory/limit, entries and evictions. Last native TTFT
  is accompanied by that prompt's reused, processed and full logical input counts.
- Reported prompt cache, speculative decoding, batching and KV quantization settings.
  Missing cache or optimization fields in older runtimes remain unavailable/unknown.
- Six history graphs: observed completion decode speed, interval output
  throughput, MLX requests per minute, shared available RAM/swap, interval
  prefill token reuse ratio, and observed cold/warm native TTFT.
- Current role profiles, paid startup opt-in and reported adapter capabilities.
- Recent observed local attempts with protocol result, latency and input/output
  usage, without prompts, responses or tool arguments.

History is retained in this dashboard process for 15 minutes (at most 900
telemetry points and 100 observed attempts). It survives page reloads and can
display a five-minute window. It resets on dashboard restart and is populated
only while `/api/status` is polled. A polling dashboard can miss completions
between polls; this is not a durable or exhaustive traffic ledger. It uses no
external chart libraries or assets.

Counter deltas produce interval rates only across fresh, consecutive samples
with the same lab run, runtime endpoint and model. Stale samples, counter or
uptime resets, and sampling gaps over eight seconds leave unknown rate gaps.
Completion decode points are recorded only when the fresh request counter
advances and native completion metadata is present; they are observed sample
times, not invented completion timestamps. The runtime cards still show the
last native completion with unknown age. Graphs do not connect different models.

Prompt cache counters are runtime-lifetime totals and reset on runtime restart.
The interval reuse graph divides the increase in reused tokens by the combined
increase in reused and processed tokens. Intervals without any prompt work are
unknown, rather than zero reuse. Cache counter decreases leave a reuse gap.
Cold TTFT requires explicit `cached_prompt_tokens: 0`; warm TTFT requires a
positive count. Both require fresh native generation metadata and an advancing
request counter. These are observed samples, not an exhaustive completion ledger
or matched cold/warm benchmarks. No latency savings are inferred.

Cache reuse avoids repeated prefill processing. Logical input usage still counts
the complete prompt, and this view makes no billing, cost reduction or fixed
percentage claims. The JSON runtime contract preserves `prompt_cache` and
`optimizations` maps from either the top-level native status or nested `stats`;
top-level values take precedence. History adds nullable `cache_reuse_ratio`
(fraction), `cold_ttft_ms` and `warm_ttft_ms` fields per runtime.

No inference, commercial traffic or control mutations are sent by the dashboard.
It reads only fixed loopback endpoints. Request and response bodies are not
included in activity telemetry. `GET /api/status` returns the displayed JSON;
`GET /health` describes the dashboard process, not model readiness. Status JSON
includes bounded `history` and sanitized `attempts` arrays.

Actual activity comes from `GET /sentinel/activity` on the gateway. Its scope is
local Messages, Responses and Chat adapters. It does not claim to monitor
commercial hybrid calls or raw reverse-proxy traffic. The endpoint keeps one
active local attempt and the last completion in memory; it is not a durable
history. Corrections are distinct attempts with shared request IDs.

Decode tok/s comes from MLX's last native completion and excludes prompt
processing. Total generation time includes prompt processing; gateway attempt
latency also includes adapter validation. Interval throughput includes idle
time. These measurements answer different questions and must not be compared
as if they were the same speed. MLX does not currently timestamp its completion
metadata, so its completion age remains unknown.

Briefly unavailable runtime polls can retain their last successful sample for
up to 15 seconds, visibly marked stale. Data expires rather than showing
fabricated healthy or zero states. Gateway and activity availability remain
explicit. Browser refresh failures mark the displayed sample stale.

Commercial cost stays unknown without a billing feed. Jes is not trained or
connected; the current quality gate checks protocol, tool schemas, repeated
unchanged lookups and short unfinished finals. It does not score reasoning or
factual accuracy. Valid tool calls can still lead to unhelpful exploration.

For repository-specific work, open the isolated client in the intended checkout:

```sh
rtk make lab-claude LAB_WORKSPACE=/absolute/path/to/repository
```

The default workspace is an isolated empty directory, so it does not provide
your project's context. Growing conversations also increase prompt processing:
in one observed Sonnet call, 16,715 prompt tokens required 14.5 seconds to the
first token despite approximately 34 native decode tokens per second. This is
an observed sample, not a benchmark or a speed guarantee.
