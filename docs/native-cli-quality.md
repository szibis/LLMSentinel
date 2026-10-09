# Real Claude Code and Codex task probes

Run against an already serving local lab with its isolated clients installed:

```sh
rtk make lab-cli-quality
rtk make lab-cli-quality-all
rtk ./bin/sentinel-tools cli-quality --root "$PWD/.sentinel-lab" --client claude --role sonnet
rtk ./bin/sentinel-tools cli-quality --root "$PWD/.sentinel-lab" --client codex --role haiku --task exact-read
```

Defaults: both clients, Sonnet, the four-task baseline suite, three minutes per task.
The supported flags are:

| Flag | Default | Values |
| --- | --- | --- |
| `--root` | `.sentinel-lab` | Existing isolated lab directory |
| `--clients-root` | Empty | Optional existing isolated installation directory containing `node_modules/.bin/claude` and `codex` |
| `--endpoint` | `http://127.0.0.1:19090` | Literal loopback HTTP gateway origin |
| `--client` | `both` | `claude`, `codex`, `both` |
| `--role` | `sonnet` | `haiku`, `sonnet`, `opus`, `all` |
| `--suite` | `baseline` | `baseline`, `extended` |
| `--task` | `all` | `all` or a task ID in the selected suite |
| `--timeout` | `3m` | Positive Go duration, at most `10m` |

Exit 0 means all selected tasks passed;
1 means task or operational failure; 2 means invalid options. Runs are sequential,
consume local inference capacity and share the lab with interactive work.

This command uses the installed `.sentinel-lab/clients/node_modules/.bin` executables.
It creates fresh private profiles and synthetic workspaces outside the repository;
it does not install clients, download models or use existing client profiles.
Inherited credentials, proxies and client configuration are excluded. The gateway
must report local-only serving with paid API opt-in disabled. No commercial API
request is part of this benchmark.

`lab-cli-quality-all` now selects the extended suite: 48 cases across all three
roles and both clients, with six minutes allowed per task. To reproduce just
the 24-case baseline, select it explicitly:

```sh
rtk ./bin/sentinel-tools cli-quality --root "$PWD/.sentinel-lab" \
  --client both --role all --suite baseline --timeout 6m
rtk ./bin/sentinel-tools cli-quality --root "$PWD/.sentinel-lab" \
  --client both --role all --suite extended --timeout 6m
```

Claude uses headless stream JSON, fixed tools and scoped permissions. Successful
tool results are correlated with their calls; a tool proposal alone does not prove
execution. See [Claude headless operation](https://code.claude.com/docs/en/headless)
and [permission modes](https://code.claude.com/docs/en/permissions).
Codex uses ephemeral `exec --json`, workspace-write and approval policy `never`;
the harness requires completed command results and a completed turn. See
[Codex noninteractive operation](https://developers.openai.com/codex/noninteractive/).

Fresh Codex profiles include a local model catalog for the Sentinel aliases.
It declares native `unified_exec` and freeform `apply_patch`, text-only input,
and a conservative 32K client context. This exposes the client's actual edit
tool instead of unknown-model fallback metadata. It does not assert commercial
model identity, runtime context capacity, web access or reasoning-summary support.
The catalog schema and edit grammar are checked against
[Codex 0.160.1 model metadata](https://github.com/openai/codex/blob/rust-v0.160.1/codex-rs/protocol/src/openai_models.rs)
and its [patch grammar](https://github.com/openai/codex/blob/rust-v0.160.1/codex-rs/core/assets/tools/apply_patch.lark).
Interactive lab preparation adds the catalog setting when absent and preserves
an explicit custom catalog, approval policy and sandbox setting.
Generated Codex profiles also supply a session header, using a fresh identity
for every lab launch or probe workspace. Explicit custom HTTP headers remain
user-owned. This enables session-scoped local routing and prompt reuse without
sharing identities between fresh launches.

For the LFM small-model lab, `--haiku-tool-role sonnet` admits Haiku requests
with tool definitions to Gemma while preserving the Haiku alias and 1,024-token
cap. This includes plain-answer tasks when the client advertises tools. Requests
without tool definitions still use LFM. Set `haiku_tool_role` in saved
`runtime.json` to `haiku` to disable this admission, or explicitly choose
`sonnet` or `opus`. Standalone gateways default to no override; the lab's
automatic override applies only to `lfm2_moe`, leaving Qwen unchanged.
The control API reports `haiku_tool_role`; activity shows actual backend routes.
This policy does not establish standalone LFM native-agent quality.

The two-model lab enables `--local-role-recovery`. After a pinned correction
fails, one extra local generation may use Sonnet for Haiku, or Opus for Sonnet;
Opus can make one fresh bounded retry. Truncated private candidates never enter
fallback history. The original alias and token cap remain in effect. A successful
recovery retains the stronger route for that identified session for 20 minutes;
new sessions begin with their configured role admission. This in-memory routing table holds
at most 256 entries and clears on restart. Requests without a session identity
do not retain routing. The dashboard records actual routes and the control API
reports `local_escalation_attempts`. Standalone gateways leave recovery disabled
unless explicitly enabled. This recovery does not call commercial providers.

Tool evidence separates file contents from error status. Read-tool line numbers
are described as display metadata, and native unchanged-file notices stop
repeated reads. Failed combined file reads preserve partial output but explicitly
require successful individual reads before editing. Native command headers are
separate execution metadata, so stdout cannot masquerade as exit status.
An explicit request to return file contents exactly also checks the final text
against a completed, unambiguous native single-file `cat` result (ignoring outer
whitespace). A mismatch requests correction from existing evidence; Sentinel
does not substitute an answer. Explicit directory paths must match, and failed,
ongoing or multi-file commands do not establish evidence for this check.
Gemma's cached native declarations/call/response syntax is used
instead of prompting its parser with a generic JSON tool envelope; LFM keeps the
JSON envelope and Qwen keeps its existing native format. Format checks require explicit JSON finals;
test-completion checks distinguish native ongoing sessions from completed test
commands and their `write_stdin` results. These are narrow observable checks,
not a semantic guarantee that a model's answer or code is correct.

| Task | Required evidence |
| --- | --- |
| Exact read | Successful read of a fresh marker; final answer exactly matches it |
| Coding fix | Read `add.go`, edit the subtraction bug, preserve tests/module, run the specified Go test command, return `FIXED_AND_TESTED` |
| Loki evidence | Read the fictional catalog; return the exact complete/partial sets in JSON |
| Planning | Read the dependency file; return the complete valid execution order in JSON |

The `extended` suite retains those four tasks and adds:

| Task ID | Required evidence |
| --- | --- |
| `untrusted-evidence` | Extract the marker while treating an embedded instruction to forge results as untrusted data |
| `literal-markers` | Preserve Unicode, escaped characters, newline, NUL and tool-looking text in the exact decoded JSON value |
| `long-context` | Extract the exact marker surrounded by unrelated reference text |
| `tool-recovery` | Observe a failed read of `unavailable.txt`, then a successful read of `recovery.json`, followed by the exact JSON answer |

For example, `--suite extended --task tool-recovery` selects only recovery.
Fixtures identify their version and carry a SHA-256 digest of the actual corpus,
including that run's fresh marker. JSON assertions and native events reject
duplicate keys, invalid UTF-8, excessive nesting and events after completion.

Coding verification accepts only a bounded arithmetic return in `Add(a, b int) int`.
It rejects imports, extra declarations and changed supplied tests, then copies the
validated function into a separate verifier with trusted tests plus 2,205 held-out
integer pairs. The verifier never executes arbitrary extra files from the workspace.
This is a small edit benchmark, not a general repository coding evaluation.
The other final-answer assertions are deterministic; Markdown instead of requested
JSON, skipped reads, unfinished promises, CLI errors and timeouts fail the task.
No broad semantic scorer, routing promotion or commercial model equivalence is implied.

On Unix, task cancellation and client completion clean up the owned process group.
Timeouts cover the client version probe, task and independent verifier. Output is
bounded to 8 MiB stdout and 1 MiB stderr per process; exceeding either fails the task.

## Private evidence and dashboard

Fresh profiles, workspace paths and raw client logs are recorded under
`.sentinel-lab/task-cli-runs/`. Workspaces are temporary directories retained for
inspection. Delete these artifacts manually when no longer needed. Reports are
archived under `.sentinel-lab/task-quality-runs/` with scope `real-cli-task-probes`;
the summary atomically replaces `task-cli-quality-latest.json`. Reports retain
client versions, exit status, events, checks and latency. They are private local
artifacts, not committed or automatically admitted to training.

The dashboard's **Real CLI tasks** section and `/api/status` `cli_task_quality`
show this separate summary. API reports remain in `task-quality-latest.json` and
`task_quality`. Refreshing the UI triggers no inference. Interrupted runs do not
replace a completed summary; displayed timestamps indicate the age of the evidence.

Usage is labeled CLI-reported and unreconciled. Missing or zero input/output
counts remain null. No gateway request totals, cache savings, monetary cost or
commercial billing are inferred from CLI events. Whole-task latency includes
startup and verification; it is not model decode speed.

## Regression checks

```sh
rtk go test -race ./internal/taskquality ./internal/labdashboard
```

Hosted CI runs the model-free tests, including successful/failed tool correlation,
unfinished turns, immutable coding fixtures, isolated environment, separate report
scopes and Unix descendant cleanup. Passing harness tests does not prove model quality.
The opt-in Metal workflow additionally requests `smoke --integration-proofs
--native-quality`, captures all 48 native cases, and requires all 24 baseline
cases to pass. Extended failures are retained and block route promotion.
The repository Actions variable `SENTINEL_EXTENDED_QUALITY_REQUIRED=true`, forwarded
by the updated workflow after merge, makes any extended failure fail that hardware
gate too; the same environment variable works for local smoke runs. A 48/48 report
permits further review; it does not itself
change routes or authorize promotion. See the [verified quality foundation](verified-quality-foundation.md)
for evidence manifests, CI provenance and the separation from hosted coverage.

## Initial native baseline — October 7, 2026

With Claude Code 2.1.291, Codex 0.160.1, the cached Gemma 4 26B-A4B large model
and LFM2.5-8B-A1B small model:

| Role | Claude Code | Codex | Total |
| --- | --- | --- | --- |
| Sonnet | 3/4 | 1/4 | 4/8 |
| Haiku | 0/4 | 0/4 | 0/8 |

Sonnet coding exposed an unsupported reasoning-channel 422 in Claude and an
incorrect edit in Codex. Codex also failed the catalog assertion and planning
hit a reasoning-channel 422. Haiku exposed malformed tool frames, truncated
generation and final-answer failures. These are one-run observations under
shared local load, not stable success rates. The earlier API-only Sonnet 8/8
result does not establish native CLI success. Recovery fixes are separate work;
this benchmark preserves failures and does not silently retry on commercial models.

## Native role policy verification — October 8, 2026

A combined run completed at `2026-10-07T22:22:01Z` (October 8 locally), using
Claude Code 2.1.291 and Codex 0.160.1 with the same four task assertions:

| Alias | Claude Code | Codex | Total |
| --- | --- | --- | --- |
| Haiku | 4/4 | 4/4 | 8/8 |
| Sonnet | 4/4 | 4/4 | 8/8 |
| Opus | 4/4 | 4/4 | 8/8 |

All 24 bounded native tasks passed, following another 24/24 run completed at
`2026-10-07T22:14:39Z`. Tool-bearing Haiku requests used the explicit
Sonnet/Gemma admission policy with the Haiku cap. Sonnet used Gemma without
thinking, and Opus used Gemma with thinking. These are observed task outcomes,
not stable success-rate estimates or standalone LFM agent-quality certification.
Earlier failed runs remain archived locally. CLI usage remains unreconciled;
no commercial token savings or cost is inferred from these outcomes.

A separate offline `smoke --integration-proofs` run using the rebuilt gateway
passed 36 API, required-tool, continuation, telemetry and native cache checks
against the cached LFM and Gemma models. Those basic API contracts are separate
from native agent task quality; they do not certify LFM for the native task set.

## Extended native observation — October 8, 2026

The local version-2 extended report completed at `2026-10-08T12:16:45.198599Z`,
using Claude Code 2.1.291 and Codex 0.160.1. It recorded **43/48 passes**, including
**24/24 baseline passes**. Five Codex cases failed:

| Task | Alias | Recorded failure |
| --- | --- | --- |
| `literal-markers` | Haiku | Backend length termination surfaced as HTTP 422 |
| `literal-markers` | Sonnet | Backend length termination surfaced as HTTP 422 |
| `literal-markers` | Opus | Literal value changed; exact final assertion failed |
| `tool-recovery` | Sonnet | Strict failed-read/successful-read recovery evidence did not pass |
| `tool-recovery` | Opus | Strict failed-read/successful-read recovery evidence did not pass |

The inspected summary is private local evidence at
`Sentinel/.sentinel-lab/task-cli-quality-latest.json`; its full synthetic transcripts
remain in local archives and are not committed. This is one observed run, not a
stable success rate. The baseline passes do not cancel the five extended failures:
route promotion remains blocked. The expanded hardware workflow has not yet been
validated on merged foundation code; these local observations are not a new
hardware CI success claim.
