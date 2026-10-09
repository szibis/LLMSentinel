# PR compatibility proofs

Every Build run checks the API regression baseline, actual pinned Claude Code
and Codex clients, and the Claude mod. The checks run on pull requests and main,
including documentation changes. Existing whole-repository race tests and the
90% statement-coverage gate remain required.

| Check | Measured evidence |
| --- | --- |
| `api-regression` | Complete uncached Go JSON events from six packages, with named Messages, Responses and Chat Completions tool/stream regressions plus capture, controls, mod, quality-harness and telemetry checks |
| `client-compatibility` | Claude Code 2.1.291 and Codex 0.160.1 execute a native file-read tool through the production gateway, send the correlated result, and complete the exact expected final answer |
| `claude-mod` | Plugin validation, native command/capture/pane framework tests, and actual CLI status/panel/failures commands with zero model turns |
| `test` | Whole-repository race tests, complete compressed execution log, global coverage profile and summary |

The native-client fixture measures `/v1/messages` and `/v1/responses` with the
`sentinel-sonnet` alias routed to the local Sonnet role. It uses a synthetic
loopback backend without a model download, GPU or provider inference. It proves
these client versions' read/tool protocol compatibility. Other roles, tools,
interactive rendering and model answer quality need their relevant checks.

Each public proof artifact contains a versioned report, tested/source and
control revisions, and a SHA-256 manifest. PR head and tested merge-checkout
revisions are distinct. The publisher checks hashes and confirms that a tested
merge checkout contains the current PR head. Missing reports, failed jobs,
unfinished logs, wrong revisions and stale run attempts cannot become a pass.
Raw transcripts, subprocess diagnostics and private lab files are excluded
from public client summaries and comments.

The `PR compatibility proofs` workflow runs after Build completes. It uses
default-branch publisher code, downloads evidence as data, and updates one
`github-actions[bot]` comment per PR. It checks the current PR head again before
writing. Human comments and other bots' comments are preserved. The comment
shows change scope, API/Claude/Codex/mod verdicts, coverage, source/control
revisions and links to the run and artifacts. Failed, skipped and unavailable
real-model results stay explicit; synthetic compatibility cannot establish
model quality or authorize training/promotion.

GitHub activates a `workflow_run` workflow only after its file exists on the
default branch. The PR introducing this publisher therefore has a manually
posted current-run proof comment; automatic updates start after merge. See
[GitHub's workflow event documentation](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows#workflow_run).

To reproduce the hosted baseline after building the gateway and helper:

```bash
go test -json -race -count=1 ./internal/localgateway ./internal/clientcapture ./internal/clientcontrol ./internal/claudemod ./internal/taskquality ./internal/labstatus > regression-events.jsonl
sentinel-tools evidence regression --events regression-events.jsonl --report regression.json --source-revision FULL_TESTED_SHA --control-revision FULL_CONTROLS_SHA
node --test scripts/test-ci-proof-record.cjs scripts/test-ci-proof-comment.cjs scripts/test-native-clients.test.cjs
node scripts/test-native-clients.cjs /absolute/path/to/sentinel-gateway /absolute/path/to/pinned/claude /absolute/path/to/pinned/codex native-clients.json
```

Use complete immutable Git hashes. A locally assigned revision identifies the
caller-supplied checkout; the manifest is an integrity check, not a signature
or independent proof that tests executed. Review the corresponding trusted CI
job and its complete execution log.
