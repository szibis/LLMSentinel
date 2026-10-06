# MLX integration verification

The protected product path is the Go Sentinel gateway and lab talking to the
external MLX-Flash runtime. The legacy standalone semantic cache, compression,
graph and Batch API libraries are not silently enabled by this work. A passing
protocol probe does not establish general model reasoning quality, commercial
model parity or provider-billed cost savings.

## Required evidence

| Contract | Fast regression tests | Real offline Metal proof |
| --- | --- | --- |
| Haiku/small, Sonnet/large, Opus/thinking routing and supported family capabilities | `internal/localgateway/roles_test.go`, `internal/qwensmoke/smoke_test.go` | Existing three role markers, actual native family/profile checks |
| Messages, Responses and Chat Completions JSON/SSE | `internal/localgateway/*protocol_test.go`, `responses_test.go`, `claude_stream_test.go`, `internal/qwensmoke/proofs_test.go` | Exact final marker, response identity, stop/usage and framed stream lifecycle on small and large backends |
| Tool arguments and continuation | `claude_test.go`, `qwen_tools_test.go`, `responses_test.go`, `internal/qwensmoke/proofs_test.go` | Required validated tools in Messages/Responses/Chat JSON and SSE, followed by synthetic tool-result evidence and a completed final answer per protocol |
| Prefix reuse with session/profile isolation | Gateway scope tests plus external MLX `test_prompt_reuse.py`, `test_serve_prompt_reuse.py` | Same session repeats reuse tokens, fresh session does not; native cold/warm/fresh greedy output parity and thinking-profile isolation |
| Honest native usage and stop metadata | MLX `test_generation_measurements.py` and `scripts/test_native_api_proofs.py` | Logical input = reused + processed; completed counters agree; JSON/SSE parity; one-token EOS/budget behavior |
| Cache bounds and lifecycle | MLX prefix-cache/serve regression tests | Entries/bytes within limits; `/release` clears retained prompt state |
| Invalid requests | Gateway schema/family/grammar tests and native serve tests | Invalid sampler/scope requests reject before native generation changes counters |
| Activity and control identity | `activity_test.go`, `control_test.go` | Last completed route matches role, queue/activity is idle and billing remains disabled |
| Dashboard accounting/history and capture/hybrid policy | `internal/labstatus`, `internal/labdashboard`, `training_test.go`, `hybrid_test.go` | Dashboard remains separately verified against the running lab; commercial provider billing is not tested by offline probes |

Model-free tests use synthetic HTTP responses and catch contract/parser failures;
they are not inference evidence. Real proofs start owned native processes on
ports 19190/19191, use a shared host lock and never execute generated client tools
or call commercial providers. They retain per-check success/failure and bounded
synthetic API summaries in `results.json`. Raw private traffic is not collected.
`revisions.json` records the selected immutable gateway/runtime/control commits
in CI; unprovided model revision information is explicitly `unknown`.

The current Sentinel proof suite has 17 named checks on the small backend and
19 on the large backend. Haiku/Sonnet cover all three protocols with marker
JSON/SSE, required-tool JSON/SSE and a tool-result continuation. The large phase
also checks Opus Messages JSON/SSE; the native suite separately verifies both
Gemma thinking profiles. General code quality and arbitrary multi-agent tasks
need separate task evaluations beyond these protocol contracts.

## Run locally

Use already cached weights and an installed MLX runtime. From Sentinel:

```sh
go test -race ./internal/localgateway ./internal/lab ./internal/labstatus ./internal/labdashboard ./internal/qwensmoke
go vet ./internal/localgateway ./internal/qwensmoke
CGO_ENABLED=0 go build -trimpath -o /private/tmp/sentinel-proof-gateway ./cmd/sentinel-gateway
CGO_ENABLED=0 go build -trimpath -o /private/tmp/sentinel-proof-tools ./cmd/sentinel-tools
export QWEN_MLX_FLASH_BIN="$PWD/.sentinel-lab/mlx-runtime/bin/mlx-flash"
export QWEN_SMALL_MODEL_PATH="$PWD/.sentinel-lab/models/LFM2.5-8B-A1B-MLX-4bit"
export QWEN_LARGE_MODEL_PATH="$PWD/.sentinel-lab/models/gemma-4-26b-a4b-it-4bit"
export QWEN_CI_LOCK_PATH="$PWD/../.qwen-ci/metal.lock"
/private/tmp/sentinel-proof-tools smoke --gateway /private/tmp/sentinel-proof-gateway --integration-proofs --artifacts /private/tmp/sentinel-api-proofs
```

From the matching MLX checkout, using those environment variables:

```sh
python -m unittest discover -s scripts -p 'test_*.py' -v
python -m pytest tests/ -q
python scripts/qwen_metal_smoke.py --integration-proofs --artifacts /private/tmp/mlx-api-proofs
```

The native command's Python must import MLX and initialize Metal; choose the
installed MLX environment explicitly. The `QWEN_*` names remain for backward
compatibility; supported family selection includes Qwen, Gemma and LFM. Run GPU
suites sequentially. If RAM is limited, stop the idle owned lab before testing
and restart it afterward. Do not terminate unrelated services to free CI ports.

Without `--integration-proofs`, the earlier marker-only smoke remains available
for historical diagnosis; it does not satisfy the expanded contract gate.

## CI and future changes

PRs run the full fast suites and model-free proof validators. Trusted main/tag
runs, with `QWEN_METAL_ENABLED=true`, run real proofs before release publishing.
An unavailable/disabled Metal job is reported as skipped and proves no inference.
The selected cached model pair is runner configuration: inspect artifact family
capabilities rather than assuming the interactive lab and CI use identical models.

Sentinel pins an immutable MLX commit supporting `cache_scope`; the former older
pin rejected that field and caused HTTP 400 → gateway HTTP 502 on actual main CI.
Changes to the dependency pin must rerun the complete integration gate.

For a new feature or behavior change, add a failure-sensitive regression,
extend its named real proof when it changes native behavior, update this matrix
and link the commands/artifacts in the PR. Keep unsupported paths and unavailable
measurements explicit. Tests safeguard their asserted contracts; they cannot
guarantee every future input or general model answer quality.
