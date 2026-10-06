# Live Claude lab status

The isolated lab's native Claude Code status line polls local telemetry every five
seconds through the Go `sentinel-tools statusline` command. New profiles include it automatically. Enable it in an existing profile:

```sh
make lab-statusline
```

Claude reloads the lab settings without restarting the conversation. Installation
preserves permissions, theme and other preferences, and leaves an existing custom
`statusLine` untouched. The known old Sentinel Python command is migrated to the absolute Go binary. This only edits `.sentinel-lab/claude/settings.json`.

The direct equivalent is `bin/sentinel-tools statusline --root /absolute/lab --install`.
Pass `--root` explicitly outside the repository: its default is `.sentinel-lab`
relative to the current directory. Installed commands always use an absolute root
and shell-quoted absolute binary path.

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

MLX statistics can also contain nested native-generation metadata. The status
line retains numeric counters alongside these extensions; optional null or
nonnumeric values remain unknown and do not mark a loaded runtime unavailable.

Inspect the same data as JSON:

```sh
make lab-live-status
```

Polling only reads local health/status endpoints and bounded local logs. It sends
no inference requests or credentials, bypasses proxies, rejects HTTP redirects,
and never displays conversation bodies. Each refresh ends after at most three
concurrent requests with 600 ms deadlines and 64 KiB response limits. Cached
samples younger than four seconds are reused. Cache files use mode 0600 in a
private directory and are replaced atomically; symlink targets are refused.
Samples live in
`.sentinel-lab/tmp/statusline.json`, outside version control. Claude status-line
behavior is documented at <https://code.claude.com/docs/en/statusline>.

Model-free fixtures: `rtk go test -race ./internal/labstatus`.
