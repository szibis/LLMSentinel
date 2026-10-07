# Real Qwen Metal CI

Hardware jobs put Go build and module caches in their temporary job directory
and remove them during cleanup. Actions cache upload is disabled on this runner;
the Mac's shared Go caches are never archived by these jobs. Hosted build jobs
retain their existing caching.

For release retries, the gateway builds from the selected tested source SHA,
while the smoke controls come from the publishing workflow's commit. This allows
testing releases that predate the CI harness without changing their source.

`Real Qwen Metal roles` verifies the freshly built Sentinel gateway against
real, offline Qwen generation. Trusted CI/release workflows invoke it through
`workflow_call`, passing their validated source commit as `source_ref` (default:
the calling event's `github.sha`). Both hosted and hardware jobs use this exact
checkout. Manual dispatches on main or a trusted tag may also run the opt-in
Apple Silicon job. Pull-request code never runs
on this Mac; ordinary tests and the new lifecycle harness tests run on hosted
runners. A hosted status job explicitly reports disabled hardware CI.

Register a repository-specific runner with labels
`self-hosted`, `macOS`, `ARM64`, `qwen-metal`. Enable repository variable
`QWEN_METAL_ENABLED=true` only once its cached runtime and models are ready.
Machine environment settings stay outside Git:

| Variable | Meaning |
| --- | --- |
| `QWEN_CI_PYTHON` | Absolute Python executable in the cached native MLX runtime |
| `QWEN_SMALL_MODEL_PATH` | Absolute cached small instruction model; the current lab uses LFM2.5-8B-A1B |
| `QWEN_LARGE_MODEL_PATH` | Absolute cached large instruction model; the current lab uses Gemma 4 26B-A4B |
| `QWEN_CI_LOCK_PATH` | Shared lock, default `/private/tmp/qwen-metal-ci.lock` |

The inference dependency is pinned to MLX-Flash main commit
`626d702970ef46284185401a1144af1607b0fe72` (merged MLX-Flash PR #25). The job checks out that exact source
and installs it editable into a temporary venv with MLX 0.32.3, mlx-lm 0.32.0,
and transformers 5.18.0. It sets `QWEN_MLX_FLASH_BIN` to that job's executable.
Build tools may be downloaded; models must already exist locally, with intact
shard indexes and Qwen thinking templates. Offline mode disables model downloads.
Missing models, missing native profiles, or unavailable Metal fail the enabled
hardware job instead of skipping the generation test.

Sentinel's smoke harness is the native Go `sentinel-tools smoke` command. It
invokes only the external `mlx-flash` executable and its HTTP API. The workflow's
temporary Python environment belongs solely to that separately checked-out
MLX-Flash dependency; Sentinel contains no Python source, hook, adapter or test.
The Go harness establishes readiness and performs real generation without
importing MLX or running inline Python preflight code.

The small model serves Haiku first. The process is cleaned up before the large
model serves Sonnet and Opus. Each request goes through the actual Claude
Messages adapter and RoleRouter, with an exact final marker required. Opus uses
the native thinking profile and must finish reasoning before returning a final
answer. A Sonnet request must produce one schema-valid `record_marker` tool call;
the harness inspects the returned tool block and executes no tool. For memory
isolation each phase maps configured role endpoints to its one active worker;
the suite checks role profiles, not simultaneous resident models or distinct
upstream routing. Hosted gateway unit tests verify distinct upstream routing.

Ports 19190/19191 isolate this job from the interactive lab's 19090/19091/19092.
The harness refuses occupied CI ports, starts only owned process groups, and
cleans them up on failure or SIGINT/SIGTERM. By default it leaves the lab alone.
Cross-repository GitHub concurrency is not shared: MLX-Flash
and Sentinel runners must use the same lock path and OS account. The file lock
covers preflight through final process cleanup and waits at most 30 minutes.

## Optional lab memory handoff

Set repository variables `SENTINEL_CI_PROJECT_ROOT` and `SENTINEL_CI_LAB_ROOT`
to absolute paths on the self-hosted Mac. Install the current Go `sentinel-tools`
in `<project>/bin/sentinel-tools` before enabling them. While holding the shared
Metal lock, CI checks that the lab is idle and owned by the Go supervisor, pauses
its processes, runs its isolated models, and restores the lab before unlocking.
The gateway atomically reserves admission only when no client request is accepted;
new inference returns 503 while reserved. An already stopped lab gets a lease and
stays stopped. Busy, unknown, attached or already reserved
labs fail the job before interruption. A manual `runner stop` during CI cancels
restoration; starting a lab during the lease is refused. Restoration failure fails
CI even when generation passed. Both repository runners must use this handoff to
avoid competing with an interactive lab for model memory.

The private `ci-pause.json` contains only pause ownership and restoration intent.
An incomplete shutdown also retains the lease for explicit recovery. After
SIGKILL or host power loss, inspect that lease and the owned processes;
automatic cleanup cannot run. Do not remove a lease while its hardware job is
still running. The dashboard can remain running during the pause.

Native accounting proofs snapshot cumulative requests, generated tokens, and
processed/reused prompt tokens before and after each client exchange. Exact
provider totals must match the entire window, including the bounded correction
attempt. Missing, reset, fractional or inconsistent counters fail the proof;
the latest generation must still carry native metadata and a normal stop.
The shared lock is close-on-exec, so a restored supervisor cannot retain it and
block the next hardware job. Forced tool choices omit a generated text preamble
while preserving schema-validated calls and all native usage. Automatic tool
choice preserves text and calls. See [Anthropic's forced tool semantics](https://platform.claude.com/docs/en/agents-and-tools/tool-use/define-tools#forcing-tool-use).

Artifacts contain a sanitized health summary, passed check usage, overall
failure/success JSON, and sanitized gateway/runtime logs. A failed check causes
a nonzero exit. The workflow uploads artifacts even after a failed smoke and
removes its temporary venv. Cleanup cannot execute after SIGKILL or host power
loss. Generation, thinking completion and a single validated tool response
do not establish general coding or tool reliability.

Model-free lifecycle tests:

```sh
go test -race ./internal/qwensmoke ./internal/release
```

Direct real smoke, with an already installed pinned inference venv:

```sh
go build -o /tmp/qwen-sentinel-gateway ./cmd/sentinel-gateway
go build -o /tmp/qwen-sentinel-tools ./cmd/sentinel-tools
QWEN_MLX_FLASH_BIN=/absolute/venv/bin/mlx-flash \
  /tmp/qwen-sentinel-tools smoke --gateway /tmp/qwen-sentinel-gateway
```
