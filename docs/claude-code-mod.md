# Native Claude Code Sentinel mod

The isolated lab's `control-plugin` includes a thin JavaScript mod. It calls
existing Go control, snapshot and capture helpers directly. `/sentinel`
commands do not start a model turn. The nine classic `/sentinel:...` skills
remain available.

Use Claude Code terminal 2.1.287 or newer, or Desktop 2.1.286 or newer. The
validated test runner is 2.1.291. The panel supports terminal and Desktop;
headless commands print a text snapshot when a pane cannot be placed. Minimal
lab mode uses `--bare`, which disables installed mods. Use full mode for this
integration. See [Anthropic's Mods overview](https://code.claude.com/docs/en/plugins/mods/overview).

## Use the isolated lab

Build `bin/sentinel-tools` and launch `bin/sentinel-tools lab claude`. Lab
preparation seeds `.sentinel-lab/control-plugin` and loads it with
`--plugin-dir`; no global Claude settings change. Generated
`sentinel-config.json` contains only the absolute helper path, lab root and
literal HTTP loopback origin. Exact owned legacy manifests migrate. Custom
manifests, modules, configuration and skills are preserved; a custom manifest
does not receive native mod files.

| Native command | Result |
| --- | --- |
| `/sentinel` or `/sentinel panel` | Open the panel; Refresh obtains one finite snapshot |
| `/sentinel status` | Read current Go controller JSON |
| `/sentinel failures` | Read retained CLI quality summary and failed checks |
| `/sentinel training on` or `off` | Change gateway collector capture control |
| `/sentinel policy local-only`, `balanced` or `quality` | Request an existing policy change |
| `/sentinel profile haiku 1024` | Set role token budget (1–32768; haiku/sonnet/opus) |
| `/sentinel capture on` or `off` | Toggle local mod recording for this session |

Controls execute argv without a shell. Go still enforces launch mode and
billing authorization. Rejected or unavailable mutations are not retried;
inspect status afterwards. The panel does not poll or initiate inference.
Observation timestamps, stale flags and unknown values remain explicit. A
failed refresh clears the snapshot. Custom endpoints do not borrow telemetry
from the default lab gateway.

Quality counts describe the retained report's suite and selected cases, not a
full-suite promotion result. Failed checks are read-only. The mod cannot
authorize training or promotion and does not relax any quality oracle.

## Opt-in local observations

Local mod capture defaults off and resets at session start and `/clear`.
Enabling it records bounded, redacted prompt and final-answer text plus tool
name, id and host return/error stage in
`.sentinel-lab/captures/claude-mod.jsonl`. Raw tool arguments, results,
transcripts and system prompts are excluded. Capture failures do not replace
the user's prompt, tool result or answer.

The Go writer uses existing private JSONL protections and redaction. Records
have source `claude_code_mod`, provenance `local_cli_observation`, unscored
quality and `training_eligible: false`. Tool stage describes host return only.
Turn usage, when supplied, is labeled `turn_reported`; missing counts remain
unknown. These observations are not complete wire accounting. The adapter
does not forward its local spool to a collector. Gateway `training off` and
local `capture off` are separate controls. See [capture semantics](client-capture.md).

## Verify without inference

```bash
claude plugin validate internal/claudemod/plugin
claude plugin test internal/claudemod/plugin
go test -race ./internal/claudemod ./internal/clientcapture ./internal/lab ./cmd/sentinel-tools
go build -o bin/sentinel-tools ./cmd/sentinel-tools
node scripts/test-claude-mod.cjs bin/sentinel-tools /absolute/path/to/claude
```

CI pins the mod runner to 2.1.291. Framework tests cover native commands,
rejection handling, capture pass-through, reported usage, session reset and
terminal/Desktop pane trees. They require no sign-in or model access; see
[Anthropic's mod test kit](https://code.claude.com/docs/en/plugins/mods/test).
Tree tests do not establish Desktop pixel rendering; inspect the panel in a
real session when changing its layout.

Every Build retains a sealed mod report alongside the API and actual
Claude Code/Codex baseline. Current-commit results appear in the PR proof
comment after the publisher is installed on main; see
[CI compatibility proofs](ci-compatibility-proofs.md).

The actual CLI fixture checks status, headless panel fallback and failure
review against a rejecting local provider endpoint. It requires zero model
turns and only the expected controller GETs. Startup waits boundedly for the
host command list after registration to accommodate the 2.1.291 refresh race.
If the host still treats `/sentinel` as a prompt, the mod drops that submission
with a reload/direct-controller message instead of handing it to a model.
