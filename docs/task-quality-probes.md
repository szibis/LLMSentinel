# Bounded task-quality probes

Run synthetic tasks against the live local lab:

```sh
rtk make tools-build
rtk ./bin/sentinel-tools quality --root "$PWD/.sentinel-lab" --role sonnet
rtk ./bin/sentinel-tools quality --root "$PWD/.sentinel-lab" --role haiku
```

The default runs four tasks through both Anthropic Messages and OpenAI
Responses. Use `--protocol messages|responses`, `--task exact-read|coding-fix|loki-evidence|planning`,
or `--timeout 90s` to select a smaller run. Exit status is 0 only when every
selected task passes, 1 for task or operational failures, and 2 for invalid options.
Tests run sequentially and consume local inference capacity. They do not stop,
reconfigure or reserve the lab; concurrent user work can affect measured latency.

| Task | Required evidence | Success assertion |
| --- | --- | --- |
| Exact read | Read a fresh random marker from `proof.txt` | Exact marker without Markdown or line numbers |
| Coding correction | Read a small Go function with a subtraction bug | A JSON return expression symbolically equivalent to `a + b` |
| Loki evidence | Read a synthetic three-entry protocol support catalog | Correct complete/partial sets; no invented entries |
| Planning | Read a four-task dependency chain | Correct complete execution order |

`read_fixture` executes only an in-memory lookup. It cannot execute shell commands,
write files or access user documents. A skipped read, repeated unchanged read,
invalid tool/path, HTTP 422, incomplete final answer or incorrect result fails
the task. Final-answer checks are task-specific deterministic assertions. The
coding check accepts equivalent addition expressions without executing generated
code; it is not a repository editing or compilation benchmark. The Loki catalog
uses fictional products and makes no claims about real products or current APIs.

These are API conversation probes with a small fixture executor, **not real
Claude Code/Codex CLI executions**, a broad semantic benchmark, or proof of
commercial model equivalence. Existing native protocol/streaming smoke tests
remain separate. No Jes scorer or automatic routing promotion is added here.
The output budget is 1024 tokens per request; Opus probes using `--role opus`
can fail this deliberately bounded budget while thinking. Interpret them as
bounded-budget evidence rather than the model's maximum capability.

## Reports and dashboard

Each completed invocation writes a private report to
`.sentinel-lab/task-quality-runs/run-*.json` and atomically replaces
`.sentinel-lab/task-quality-latest.json`. Reports contain synthetic request/response
exchanges, final answers, failures, role/protocol, tool/request counts, and whole-task
latency. Interrupted runs do not replace the latest completed report. Keep the
archives to investigate regressions; delete them manually when no longer needed.
They are local artifacts and are not committed or automatically promoted into
training data.

Complete archives are bounded to 64 MiB per invocation. The latest file is a
bounded summary without exchanges or final answers. Oversized/unreadable HTTP
responses retain an explicitly truncated body excerpt and fail the probe.

Gateway health and, for the default lab endpoint, a pre-run runtime snapshot
record configuration and model-status evidence. Role aliases alone do not establish
the artifact actually used; the snapshot is not request-correlated route proof.
Token totals are labeled wire-reported and are available only when **every**
response reports positive integer input/output counts. Missing counts and legacy
zero placeholders make the whole-task totals null. Native reconciliation, cache
savings, commercial prices and actual billing are not measured by this suite.

The [live dashboard](live-backend-dashboard.md) shows the latest completed run's
timestamp, per-task result, role/protocol, tools, latency and usage. It hides raw
exchanges and final answers. No report means unknown evidence, not zero failures.
Dashboard refresh sends no inference requests and never runs the probes itself.
An old report remains dated evidence until another explicit probe run finishes.

## Regression verification

```sh
rtk go test -race ./internal/taskquality ./internal/labdashboard
```

CI runs deterministic mock-server tests covering both tool-history protocols and
the original failure patterns: HTTP 422, search-only endings, incorrect copying,
skipped reads, repeated reads, truncated answers and unknown usage across a
continuation. Mock-server success validates the harness, not a model's quality.
Live quality runs are explicit experiments; they are not release gates until
reviewed baselines and promotion thresholds are agreed.

## Initial local baseline — October 7, 2026

With the cached Gemma 4 26B-A4B large artifact and LFM2.5-8B-A1B small artifact,
Sonnet passed 8/8 fixtures across Messages and Responses. Haiku passed 0/8:
five HTTP 422 failures and three final-answer assertion failures. A planning
answer returned the right words without the requested JSON, another violated
a dependency, and a catalog answer incorrectly returned empty sets.

This is one run per role under shared local load, not a stable success-rate
estimate. An earlier coding prompt that included the desired expression also
exposed skipped fixture reads; the finalized prompt asks the model to derive
the expression. These observations identify small-model tool and instruction
adherence as follow-up work. They do not change production routing or admit
any captured output to training.
