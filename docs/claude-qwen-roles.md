# Claude lab Qwen roles

Claude uses ordinary Anthropic model settings; Sentinel owns the underlying Qwen identities and endpoint selection. There are two native runtimes, not three copies of model weights.

| Claude role | Internal model | Profile | Local port |
| --- | --- | --- | --- |
| Haiku | Qwen3.5-4B-MLX-4bit | Thinking off, at most 1,024 output tokens | 19092 |
| Sonnet | Qwen3.6-35B-A3B-4bit | Thinking off, at most 4,096 output tokens | 19091 |
| Opus | Same large Qwen artifact/process as Sonnet | Thinking on, at most 8,192 output tokens | 19091 |

These names express task roles, not equivalent model quality. Client max_tokens further limits each profile. Each generation and its optional JSON correction retain their route and complete tool history. Unknown roles and missing runtimes fail; no cloud fallback exists. Sentinel validates final tool JSON after completed Opus thinking; it never executes client tools.

## Use on this Mac

The large model was already cached. The small model was explicitly downloaded to ignored `.sentinel-lab/models/Qwen3.5-4B-MLX-4bit` at revision `32f3e8ecf65426fc3306969496342d504bfa13f3`. Its weights are 3,034,300,695 bytes with SHA-256 `5fb9acd0246866381cf8c5c354c6db1019f6498eec4ccb4f5edcc71ffeacb2db`, verified against the pinned Hub file. Large revision: `38740b847e4cb78f352aba30aa41c76e08e6eb46`. Both paths and the existing MLX-Flash executable are saved in `.sentinel-lab/runtime.json`. Previous selection: `.sentinel-lab/runtime.before-qwen-roles.json`.

From a normal Mac terminal with Metal access:

```sh
cd /Users/slawomirskowron/projects/model_training/Sentinel
make lab-rebuild
make lab-model-check
make lab-claude
```

Normal launch never downloads models. Rebuild stops only this lab's owned processes. The currently running old lab was preserved during setup; the next rebuild/Claude launch refreshes it to the role gateway. Occupied unowned ports are refused.

The launcher sets `ANTHROPIC_DEFAULT_HAIKU_MODEL=sentinel-haiku`, `ANTHROPIC_DEFAULT_SONNET_MODEL=sentinel-sonnet`, and `ANTHROPIC_DEFAULT_OPUS_MODEL=sentinel-opus`. Claude sees Haiku/Sonnet/Opus display names. Main mode is `opusplan`: Opus during Plan Mode and Sonnet during implementation. `/model haiku`, `/model sonnet`, `/model opus` and `/model opusplan` use the same mappings. The lab defines a read-only Explore agent on Haiku and a read-only Plan agent on Opus; other agents can select their appropriate role. The Agent tool is enabled alongside repository tools with normal permissions. Production Claude settings stay separate.

The existing pinned MLX-Flash server does not forward tokenizer options. `scripts/mlx_flash_roles.py` adds request-scoped `enable_thinking` through its actual tokenizer and serializes native generation inside each runtime. Inference remains in the installed MLX-Flash/MLX libraries. With native support installed, the launcher uses the server's own profile handling instead of the compatibility adapter. Native support is proposed in [MLX-Flash PR #17](https://github.com/szibis/mlx-flash/pull/17); the installed runtime has not been changed. All native children use local paths, offline Hub settings and speculation disabled.

For another machine, select existing complete paths:

```sh
make lab-rebuild MODEL_PATH=/absolute/large/qwen SMALL_MODEL_PATH=/absolute/small/qwen MLX_FLASH_BIN=/absolute/venv/bin/mlx-flash
```

Role startup validates weight shard inventory, tokenizer metadata and an embedded/external Qwen thinking template before spawning either native process. It requires the installation's Python executable beside mlx-flash. It preflights ports 19090/19091/19092 and owns both runtime process groups. Logs include `runtime.log` and `runtime-small.log`; `/sentinel/status` reports separate native health for the profiles.

## Evidence and remaining verification

Model-free gateway tests exercise endpoint selection, profile budgets, unknown/remote role refusal, route-pinned correction with actual tool-history evidence, incomplete-thinking refusal and separate native status. Python tests exercise role settings, opusplan/agent definitions, nested model context, complete template checks, request-scoped effort and cleanup of descendants after an owned leader exits. Run `make lab-test` for race, vet and isolation checks.

Both actual tokenizer templates were tested locally without generation: thinking off closes the Qwen thinking prefix; thinking on opens it. This establishes template compatibility, not weight-loading or output quality. MLX import in this execution session failed with `[metal::load_device] No Metal device available`. Real Haiku/Sonnet/Opus generation, agent tool use, memory residency and the interactive read/edit/check/recovery workflow therefore remain unverified. `make lab-model-check` deliberately generates with each profile and checks an exact final answer; it is an opt-in smoke, not a quality benchmark. Follow it with an actual coding workflow in Claude and inspect files/check output.

The pinned runtime still buffers generation, lacks verified request-level native cancellation and reports approximate completion counters/finish reasons. Role output bounds do not enforce a physical memory limit or prove safe maximum-context admission. Small and large weights total roughly 22 GiB on disk; measured RAM demand remains unverified here. This lab remains an experimental non-Claude backend integration.

Claude's role settings and opusplan behavior follow its [model configuration documentation](https://code.claude.com/docs/en/model-config); agent definitions follow the [subagent documentation](https://code.claude.com/docs/en/sub-agents).
