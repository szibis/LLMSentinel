# Live Claude lab status

The isolated lab's native Claude Code status line polls local telemetry every five
seconds. New profiles include it automatically. Enable it in an existing profile:

```sh
make lab-statusline
```

Claude reloads the lab settings without restarting the conversation. Installation
preserves permissions, theme and other preferences, and leaves an existing custom
`statusLine` untouched. This only edits `.sentinel-lab/claude/settings.json`.

The display includes the Claude-selected model/mode and context percentage,
Sentinel reachability, both loaded Qwen artifacts, MLX call and generated-token
counts, recent throughput, the last completed generation's speed, machine memory
availability, swap and pressure, and JSON corrections/errors in the gateway log
tail. Haiku uses the small runtime; Sonnet and Opus share the large runtime, with
thinking enabled for Opus. Claude's selected mode is session metadata, not proof
of which role a background agent is currently executing.

Counters come from each distinct runtime and reset with its process. One Claude
turn can make multiple MLX calls, including format corrections, so `MLX req` is
not a completed-turn count. Interval rates use differences between successful
samples and include idle/prompt-processing time. Counters are published on
completion; these are not instantaneous streaming decode rates or time to first
token. The first sample or a restart shows no interval rate. `last` speeds come
from the most recent completion in each runtime's bounded log tail and may be
older than the current sampling interval.

Memory figures describe the same machine and are displayed once. Gateway JSON
corrections and HTTP errors are counts in the last 64 KiB of logs (or since a
start marker within that tail), not durable lifetime totals. Errors can include
client cancellations. Unavailable endpoints are explicitly marked; missing
measurements are never substituted with zero or a healthy state.

Inspect the same data as JSON:

```sh
make lab-live-status
```

Polling only reads local health/status endpoints and bounded local logs. It sends
no inference requests or credentials, bypasses proxies, rejects HTTP redirects,
and never displays conversation bodies. Samples live in
`.sentinel-lab/tmp/statusline.json`, outside version control. Claude status-line
behavior is documented at <https://code.claude.com/docs/en/statusline>.
