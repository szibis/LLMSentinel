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

The display labels the Claude-selected model/mode separately from the actual
active adapter role and upstream runtime, plus the context percentage,
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
token. The first sample or a restart shows no interval rate. Native `decode tok/s` comes from `stats.last_generation.generation_tps` and
`generation` seconds from `time_s`. Decode speed excludes prompt processing and
validation; generation duration is not Claude turn latency. Native metadata is
retained separately as `last_generation` in status JSON. The native runtime does
not currently publish a completion timestamp, so the display says `age unknown`.
When native speed is absent, a bounded log-tail fallback is explicitly labeled
`legacy log`, also with unknown age.

Memory figures describe the same machine and are displayed once. Gateway JSON
corrections and HTTP errors are counts in the last 64 KiB of logs (or since a
start marker within that tail), not durable lifetime totals. Errors can include
client cancellations. Failed refreshes retain the last successful runtime or activity sample for up
to 15 seconds, visibly marked stale with its age. Gateway failures remain
unavailable, with the last healthy observation age. Stale samples never contribute
interval rates and expire rather than remaining ready indefinitely. Run identity,
endpoint changes and observed runtime uptime resets invalidate retained samples.
Missing measurements are never substituted with zero or a healthy state.

MLX statistics can also contain nested native-generation metadata. The status
line retains numeric counters alongside these extensions; optional null or
nonnumeric values remain unknown and do not mark a loaded runtime unavailable.

Inspect the same data as JSON:

```sh
make lab-live-status
```

Polling only reads local health/status endpoints and bounded local logs. It sends
no inference requests or credentials, bypasses proxies, rejects HTTP redirects,
and never displays conversation bodies. Each refresh ends after concurrent health, activity and runtime
requests with 600 ms deadlines and 64 KiB response limits. Cached
samples younger than four seconds are reused unless the lab run identity changes. Cache files use mode 0600 in a
private directory and are replaced atomically; symlink targets are refused.
Samples live in
`.sentinel-lab/tmp/statusline.json`, outside version control. Claude status-line
behavior is documented at <https://code.claude.com/docs/en/statusline>.

Model-free fixtures: `rtk go test -race ./internal/labstatus`.

Actual activity comes from `/sentinel/activity`, which reports local adapter
attempts (including parallel agents and corrections). An idle gateway displays the
last completed role and route. Selected Claude labels and logs never establish
actual activity. Plain reverse proxy and commercial hybrid calls are outside this
endpoint's scope. Stale activity says `last observed active` to avoid claiming an
attempt is still running. Human output uses ANSI colors for readiness, failures,
and memory pressure; set a nonempty `NO_COLOR` to disable them. JSON has no ANSI
sequences.
