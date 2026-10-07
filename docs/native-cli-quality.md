# Real Claude Code and Codex task probes

Run against an already serving local lab with its isolated clients installed:

```sh
rtk make lab-cli-quality
rtk ./bin/sentinel-tools cli-quality --root "$PWD/.sentinel-lab" --client claude --role sonnet
rtk ./bin/sentinel-tools cli-quality --root "$PWD/.sentinel-lab" --client codex --role haiku --task exact-read
```

Defaults: both clients, Sonnet, four tasks, three minutes per task. Options include
`--client claude|codex|both`, `--role haiku|sonnet|opus`, `--task exact-read|coding-fix|loki-evidence|planning|all`
and `--timeout 90s` (maximum ten minutes). Exit 0 means all selected tasks passed;
1 means task or operational failure; 2 means invalid options. Runs are sequential,
consume local inference capacity and share the lab with interactive work.

This command uses the installed `.sentinel-lab/clients/node_modules/.bin` executables.
It creates fresh private profiles and synthetic workspaces outside the repository;
it does not install clients, download models or use existing client profiles.
Inherited credentials, proxies and client configuration are excluded. The gateway
must report local-only serving with paid API opt-in disabled. No commercial API
request is part of this benchmark.

Claude uses headless stream JSON, fixed tools and scoped permissions. Successful
tool results are correlated with their calls; a tool proposal alone does not prove
execution. See [Claude headless operation](https://code.claude.com/docs/en/headless)
and [permission modes](https://code.claude.com/docs/en/permissions).
Codex uses ephemeral `exec --json`, workspace-write and approval policy `never`;
the harness requires completed command results and a completed turn. See
[Codex noninteractive operation](https://developers.openai.com/codex/noninteractive/).

| Task | Required evidence |
| --- | --- |
| Exact read | Successful read of a fresh marker; final answer exactly matches it |
| Coding fix | Read `add.go`, edit the subtraction bug, preserve tests/module, run the specified Go test command, return `FIXED_AND_TESTED` |
| Loki evidence | Read the fictional catalog; return the exact complete/partial sets in JSON |
| Planning | Read the dependency file; return the complete valid execution order in JSON |

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
scopes and Unix descendant cleanup. Native runs are explicit experiments, not
release gates. Passing harness tests does not prove model quality.

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
