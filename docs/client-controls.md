# Sentinel controls from terminal or normal CLI commands

`scripts/sentinel_control.py` controls an already configured local Sentinel process. Its direct terminal commands make no model/provider requests, require no credentials and do not change Claude/Codex provider configuration. The server decides whether a requested update is allowed. Status reports its startup mode, billing opt-in state, training capture state, profiles and hybrid policy.

From the repository root:

```sh
rtk python3 scripts/sentinel_control.py status
rtk python3 scripts/sentinel_control.py training off
rtk python3 scripts/sentinel_control.py training on
rtk python3 scripts/sentinel_control.py policy local-only
rtk python3 scripts/sentinel_control.py policy balanced
rtk python3 scripts/sentinel_control.py policy quality
rtk python3 scripts/sentinel_control.py profile sonnet 2048
```

Default serving endpoint: `http://127.0.0.1:19090`. Use `--endpoint http://127.0.0.1:19094` for the separate learning collector. The endpoint accepts only literal HTTP loopback origins (`127.0.0.1` or `::1`), without credentials, paths or query parameters. Requests use direct HTTP without environment proxies, redirects or DNS, a one-second socket timeout and a 512 KiB response limit.

Status sends `GET /sentinel/control`. Mutations send POST JSON: `training on|off` sets `capture_enabled`; `policy local-only|balanced|quality` sets `policy`; `profile haiku|sonnet|opus N` sets `role_budgets` for that role with `1 <= N <= 32768`. Profile budgets govern Sentinel's role output limits; they do not select the original commercial CLI model in learning mode.

Mode and paid-provider startup authorization are immutable through this endpoint. Training-on requires a storage directory already configured at startup. Balanced/quality policy requires hybrid routing and paid opt-in already configured at startup; these commands cannot install credentials, enable paid requests by themselves, or turn a learning collector into an inference proxy. Unsupported changes return a nonzero controller exit status. Policy changes in serving mode can affect later inference requests under the previously authorized startup policy; the control command itself performs no inference.

## Reviewable slash command assets

Generate Markdown for a chosen normal CLI, with no global installation or HTTP calls:

```sh
rtk python3 scripts/sentinel_control.py --client claude --preview-commands
rtk python3 scripts/sentinel_control.py --client codex --endpoint http://127.0.0.1:19094 --preview-commands
```

`--preview-commands` prints JSON containing every command's filename and contents. `--write-commands /canonical/absolute/review-directory` writes those assets instead. It refuses existing files and symlinks and never silently replaces commands. Choose a new preview directory for each regeneration.

Claude commands use the official `.claude/commands/<name>.md` format, with `disable-model-invocation: true` so only explicit user invocation triggers them. For normal installations, copy files into the chosen project or personal commands directory. See [Claude Code skills/custom commands](https://code.claude.com/docs/en/slash-commands). Preparing the isolated lab seeds files into its client profiles, preserving existing files. Bare Claude Code 2.1.289 did not expand those automatically in the real CLI probe. The launcher therefore explicitly loads its generated local `control-plugin` with `--plugin-dir`; its nine skills use `/sentinel:status`, `/sentinel:training-on`, `/sentinel:training-off`, `/sentinel:policy-local`, `/sentinel:policy-balanced`, `/sentinel:policy-quality`, `/sentinel:profile-haiku`, `/sentinel:profile-sonnet`, and `/sentinel:profile-opus`. This leaves bare-mode authentication isolation and disabled hooks intact. A new lab launch is required to add the plugin to an existing session.

Codex assets use its documented Markdown custom prompt format. After review, place files directly under the explicitly chosen `CODEX_HOME/prompts` directory (normally `~/.codex/prompts`) and open a new Codex chat/session to load them. Invocation is `/prompts:sentinel-status`, **not** an invented native `/sentinel-status` command. Custom prompts are deprecated but still documented and present in the installed 0.160.1 source; OpenAI recommends skills for future reusable extensions. See [official custom prompts](https://developers.openai.com/codex/custom-prompts) and [0.160.1 custom prompt tests](https://github.com/openai/codex/blob/rust-v0.160.1/codex-rs/tui/src/bottom_pane/custom_prompt_view_tests.rs). The generator does not install global prompts or restart an active session.

Generated names and fixed effects:

| Asset name | Claude invocation | Codex invocation | Effect |
| --- | --- | --- | --- |
| sentinel-status | `/sentinel-status` | `/prompts:sentinel-status` | Show returned status |
| sentinel-training-on | `/sentinel-training-on` | `/prompts:sentinel-training-on` | Enable configured training capture |
| sentinel-training-off | `/sentinel-training-off` | `/prompts:sentinel-training-off` | Disable collector capture |
| sentinel-policy-local | `/sentinel-policy-local` | `/prompts:sentinel-policy-local` | Set local-only policy |
| sentinel-policy-balanced | `/sentinel-policy-balanced` | `/prompts:sentinel-policy-balanced` | Set authorized balanced policy |
| sentinel-policy-quality | `/sentinel-policy-quality` | `/prompts:sentinel-policy-quality` | Set authorized quality policy |
| sentinel-profile-haiku | `/sentinel-profile-haiku` | `/prompts:sentinel-profile-haiku` | Set Haiku output budget to 1024 |
| sentinel-profile-sonnet | `/sentinel-profile-sonnet` | `/prompts:sentinel-profile-sonnet` | Set Sonnet output budget to 4096 |
| sentinel-profile-opus | `/sentinel-profile-opus` | `/prompts:sentinel-profile-opus` | Set Opus output budget to 8192 |

Each asset instructs the CLI to run one exact shell-quoted Python command and report its JSON/error. No `$ARGUMENTS` or positional substitutions enter shell code. For arbitrary budgets, use the validated terminal `profile` command. Generated slash commands are model-assisted instructions, so they can consume the normal CLI's model tokens and need its ordinary tool permissions; they are not a zero-token native CLI extension.

Training-off pauses persistence in Sentinel; ingestion may acknowledge receipt without storing an event. It does not remove capture hooks or stop their authoritative private JSONL spool; disable those hooks separately if local capture must stop. Existing files remain subject to explicit retention/review. No automatic mode change, provider switch or live-lab restart happens through these commands.

Verification uses fixture HTTP responses and generated temporary files only:

```sh
rtk python3 -m unittest discover -s scripts -p test_sentinel_control.py
```
