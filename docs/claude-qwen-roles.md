# Claude lab Qwen roles

Claude uses ordinary Anthropic model settings; Sentinel owns the underlying Qwen identities and endpoint selection. There are two native runtimes, not three copies of model weights.

| Claude role | Internal model | Profile | Local port |
| --- | --- | --- | --- |
| Haiku | Qwen3.5-4B-MLX-4bit | Thinking off, at most 1,024 output tokens | 19092 |
| Sonnet | Qwen3.8-27B-4bit | Thinking off, at most 4,096 output tokens | 19091 |
| Opus | Same large Qwen artifact/process as Sonnet | Thinking on, at most 8,192 output tokens | 19091 |

These names express task roles, not equivalent model quality. Client max_tokens further limits each profile; live controls can adjust its output budget. Each generation and optional format correction retain their route/history. Unknown roles and missing runtimes fail. Local serving has no cloud fallback; hybrid requires explicit paid configuration. Sentinel validates Qwen-native tool calls or the previous JSON envelope after completed Opus thinking; the CLI executes tools. See [current modes](client-modes.md).

These Qwen models generate answers. The separate small decision service will first use a evaluated OSS adapter, with Kev-0.8B proposed as the closest Qwen/scorer match and Jeff as challenger, then our Jes checkpoint. Neither decision adapter is loaded today. See [helper inventory and migration design](jes-decision-design.md).

## Use on this Mac

The large model was explicitly downloaded to ignored `.sentinel-lab/models/Qwen3.8-27B-4bit`, pinned to MLX community revision `10c35caafbb80f7dc6a7a432cdd11af10a6d4818`. All downloaded file sizes and all three weight-shard SHA-256 hashes were checked against the pinned Hub metadata before selection. The small model remains in `.sentinel-lab/models/Qwen3.5-4B-MLX-4bit` at revision `32f3e8ecf65426fc3306969496342d504bfa13f3`. Its weights are 3,034,300,695 bytes with SHA-256 `5fb9acd0246866381cf8c5c354c6db1019f6498eec4ccb4f5edcc71ffeacb2db`. Paths, artifact revisions and the existing MLX-Flash executable are saved in `.sentinel-lab/runtime.json`. The prior large-model selection is recorded in `.sentinel-lab/runtime.before-qwen38.json`; availability of old artifacts is not guaranteed by that historical configuration.

The official [Qwen3.8-27B](https://huggingface.co/Qwen/Qwen3.8-27B) is a dense 27B model based on the `qwen3_5` architecture. The [pinned MLX 4-bit conversion](https://huggingface.co/mlx-community/Qwen3.8-27B-4bit/tree/10c35caafbb80f7dc6a7a432cdd11af10a6d4818) has about 16.1 GB of weights. It is compatible with the lab's existing MLX 0.32.3 / mlx-lm 0.32.0 / transformers 5.18.0 environment for **text** generation. Vision/video and MTP acceleration are not enabled or verified by this lab.

Qwen3.8's thinking template defaults to `xhigh` effort when thinking is enabled. Sentinel currently sends only the request-scoped `enable_thinking` control; it does not expose the newer `reasoning_effort` or `preserve_thinking` template options. Role budgets still bound output. The adapter's existing temperature remains 0.1; this is not a claim that it matches every model-card sampling recommendation.

On 2026-10-06, the running lab passed exact-answer checks for all three roles, completed Opus thinking, a Claude required-tool JSON/schema probe, and an OpenAI Responses final-answer probe after this switch. No client tools or paid providers were executed. These are compatibility smokes, not evidence of commercial-equivalent quality. Dense 27B generation can be slower than the replaced 35B-A3B MoE; model size alone does not predict speed. Use the live dashboard's measured decode, TTFT and interval graphs to evaluate representative tasks.

From a normal Mac terminal with Metal access:

```sh
cd /Users/slawomirskowron/projects/model_training/Sentinel
make lab-rebuild
make lab-model-check
make lab-claude
```

Normal launch never downloads models. Rebuild stops only this lab's owned processes. The lab uses the Go supervisor and role gateway. Occupied unowned ports are refused.

The launcher sets `ANTHROPIC_DEFAULT_HAIKU_MODEL=sentinel-haiku`, `ANTHROPIC_DEFAULT_SONNET_MODEL=sentinel-sonnet`, and `ANTHROPIC_DEFAULT_OPUS_MODEL=sentinel-opus`. Claude sees Haiku/Sonnet/Opus display names. Main mode is `opusplan`: Opus during Plan Mode and Sonnet during implementation. `/model haiku`, `/model sonnet`, `/model opus` and `/model opusplan` use the same mappings. The lab defines a read-only Explore agent on Haiku and a read-only Plan agent on Opus; other agents can select their appropriate role. The Agent tool is enabled alongside repository tools with normal permissions. Production Claude settings stay separate.

Sentinel calls the external `mlx-flash` executable directly. It has no Python profile shim and requires native request-scoped `enable_thinking` support. Missing support produces an actionable update/rebuild refusal; it never restarts an active runtime automatically. Native profile support was merged in [MLX-Flash PR #17](https://github.com/szibis/mlx-flash/pull/17), and native accounting/sampling improvements are in [PR #19](https://github.com/szibis/mlx-flash/pull/19). A merged PR does not update a running installation. Children use cached paths, offline Hub settings and speculation disabled.

For another machine, select existing complete paths:

```sh
make lab-rebuild MODEL_PATH=/absolute/large/qwen SMALL_MODEL_PATH=/absolute/small/qwen MLX_FLASH_BIN=/absolute/venv/bin/mlx-flash
```

Role startup validates weight shard inventory, tokenizer metadata and an embedded/external Qwen thinking template before spawning either native process. It requires only the existing external mlx-flash executable. It preflights ports 19090/19091/19092 and owns both runtime process groups. Logs include `runtime.log` and `runtime-small.log`; `/sentinel/status` reports separate native health for the profiles.

## Evidence and remaining verification

Model-free gateway tests exercise endpoint selection, profile budgets, unknown/remote role refusal, route-pinned correction with actual tool-history evidence, incomplete-thinking refusal and separate native status. Go lab tests exercise isolated settings, nested model context, complete model/template checks, ownership locks, run-specific stop markers and process-group cleanup. Run `make lab-test` for race, vet and isolation checks.

Both tokenizer templates were checked; real Claude Read-tool probes subsequently ran across all three roles. Those probes still exhibited incorrect path recovery and exact-answer formatting. See [the dated client inspection](client-quality-inspection.md). `make lab-model-check` deliberately generates with each profile and checks an exact final answer; it is an opt-in smoke, not a quality benchmark. Full read/edit/check/recovery acceptance remains open and requires inspecting real files/check output.

Local output still buffers, and request-level server-side cancellation is not established. Legacy installations have approximate counters/stop metadata; updated native accounting must be identified by its capability marker. Output bounds do not enforce a physical memory limit or prove safe maximum-context admission. Small/large weights total roughly 18 GiB on disk; disk size is not resident RAM. This remains an experimental non-Claude backend integration.

Claude's role settings and opusplan behavior follow its [model configuration documentation](https://code.claude.com/docs/en/model-config); agent definitions follow the [subagent documentation](https://code.claude.com/docs/en/sub-agents).
