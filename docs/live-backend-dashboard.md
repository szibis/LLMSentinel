# Live backend dashboard

From Sentinel's repository root:

```sh
rtk make lab-dashboard
```

Open **http://localhost:8077/dashboard**. This serves the isolated lab's backend
telemetry through native Go on 127.0.0.1. Keep the command running; Ctrl+C stops
only this dashboard. The gateway and MLX runtimes remain managed by the lab
supervisor. This live view is separate from the older configuration dashboard
served by `llm-sentinel dashboard`; they cannot occupy the same port.

The view refreshes every five seconds and shows:

- Actual active local adapter role, selected upstream, thinking profile, elapsed
  time, output budget, waiting count and most recent completed attempt.
- Both Qwen artifacts, completion counters, native decode speed, total generation
  time and native time to first token.
- Shared machine memory, swap and pressure, displayed once.
- Gateway mode, policy, capture state, role budgets and current quality-gate scope.

No inference, commercial traffic or control mutations are sent by the dashboard.
It reads only fixed loopback endpoints. Request and response bodies are not
included in activity telemetry. `GET /api/status` returns the displayed JSON;
`GET /health` describes the dashboard process, not model readiness.

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
connected; the current quality gate checks protocol and tool schemas, not
reasoning quality. Valid tool calls can still lead to repetitive or unhelpful
exploration.

For repository-specific work, open the isolated client in the intended checkout:

```sh
rtk make lab-claude LAB_WORKSPACE=/absolute/path/to/repository
```

The default workspace is an isolated empty directory, so it does not provide
your project's context. Growing conversations also increase prompt processing:
in one observed Sonnet call, 16,715 prompt tokens required 14.5 seconds to the
first token despite approximately 34 native decode tokens per second. This is
an observed sample, not a benchmark or a speed guarantee.
