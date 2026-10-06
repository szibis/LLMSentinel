# Real Qwen Metal CI

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
| `QWEN_SMALL_MODEL_PATH` | Absolute cached Qwen3.5 4B MLX 4-bit directory |
| `QWEN_LARGE_MODEL_PATH` | Absolute cached Qwen3.6 35B A3B MLX 4-bit snapshot directory |
| `QWEN_CI_LOCK_PATH` | Shared lock, default `/private/tmp/qwen-metal-ci.lock` |

The inference dependency is pinned to MLX-Flash main commit
`a45ae99464aa2ddd82158042043f91b6966bdec3`. The job checks out that exact source
and installs it editable into a temporary venv with MLX 0.32.3, mlx-lm 0.32.0,
and transformers 5.18.0. It sets `QWEN_MLX_FLASH_BIN` to that job's executable.
Build tools may be downloaded; models must already exist locally, with intact
shard indexes and Qwen thinking templates. Offline mode disables model downloads.
Missing models, missing native profiles, or unavailable Metal fail the enabled
hardware job instead of skipping the generation test.

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
cleans them up on failure or SIGINT/SIGTERM. It leaves the existing lab root and
processes alone. Cross-repository GitHub concurrency is not shared: MLX-Flash
and Sentinel runners must use the same lock path and OS account. The file lock
covers preflight through final process cleanup and waits at most 30 minutes.

Artifacts contain a sanitized health summary, passed check usage, overall
failure/success JSON, and sanitized gateway/runtime logs. A failed check causes
a nonzero exit. The workflow uploads artifacts even after a failed smoke and
removes its temporary venv. Cleanup cannot execute after SIGKILL or host power
loss. Generation, thinking completion and a single validated tool response
do not establish general coding or tool reliability.

Model-free lifecycle tests:

```sh
python3 -m unittest discover -s scripts -p test_qwen_metal_smoke.py -v
```

Direct real smoke, with an already installed pinned inference venv:

```sh
go build -o /tmp/qwen-sentinel-gateway ./cmd/sentinel-gateway
QWEN_MLX_FLASH_BIN=/absolute/venv/bin/mlx-flash \
  python3 scripts/qwen_metal_smoke.py --gateway /tmp/qwen-sentinel-gateway
```
