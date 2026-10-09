# Verified quality foundation

The foundation combines repository-wide Go regression coverage, bounded native
client task evidence, synthetic request measurements and verifiable artifact
manifests. Each answers a different question. A passing Go test does not prove
model quality; a native task pass does not establish stable performance, cost
savings or commercial-model equivalence.

PR #66 merged as `ef3a63d690efe6ac08d5ab1296953ee1e8ccb505` on October 9.
The merged hardware run retained failures and failed its baseline gate; no
successful merged hardware result or route promotion is established.

## Repository coverage and reproducible evidence

Build CI runs `go test -json -race -coverprofile=coverage.out -count=1 ./...` and
enforces **at least 90% unrounded statement coverage across the whole Go
repository**. The global gate includes command packages and legacy modules; it
does not substitute a selected-package profile for the repository total.
Missing, malformed or empty profiles fail. CI requires the whole-repository
test command to succeed; the coverage parser alone cannot prove that a supplied
profile contains every package. Repeatable `--package` flags instead enforce
selected exact-package percentages while retaining the global totals in the
report. The hosted global gate uses no package selector.

The local foundation verification passed race testing at 92.909% statement
coverage (20,755 of 22,339 statements). This describes that checked revision;
the CI artifact from the reviewed commit is the evidence for future changes.
Tests use local HTTP fixtures, synthetic subprocesses and temporary directories;
they do not require a paid provider, model download or GPU.

The continuation's Go 1.27.1 race run passed 56 packages and 2,133 test cases at
**92.892135%** global coverage (20,858/22,454 statements), including the subsequent
history and report-validation fixes. Pinned lint, workflow syntax and four DOM
rendering contracts passed. Fresh bounded fuzz runs passed 596,955 admission,
389,754 persisted-attribution and 728,430 coverage-profile executions.

The first PR Build run, [37901269295](https://github.com/szibis/LLMSentinel/actions/runs/37901269295),
passed its test, global coverage, complete-log compression, manifest verification
and artifact-upload steps. The downloaded manifest also verified locally;
coverage was 20,858/22,454 statements (92.892135%). Its source and control
revision is the PR merge commit `978fbd5eafc6381836728a30b12bba9fef2bfe59`.
That run failed security scanning because nine reachable standard-library
vulnerabilities required [Go 1.27.2](https://go.dev/doc/devel/release#go1.27.2).
The module and container now use that patch. The linter pin is 2.14.0 because
2.13.2 could not decode the patched compiler's export format.

The patched-toolchain local race run passed the same 56 packages and 2,133
test cases at **90.953646%** global coverage (15,403/16,935 statements), above
the unchanged 90% gate. Both profiles contain the same 150 files and 11,552
block locations, while 1,839 block statement counts changed; two blocks changed
between covered and uncovered. The Go release also contains a cover-tool fix.
The changed statement accounting therefore prevents a direct percentage
comparison with the earlier profile. Go 1.27.2 govulncheck found no
vulnerabilities; golangci-lint 2.14.0 reported zero issues; actionlint and all
four DOM contracts passed. Complete logs remain preserved losslessly.

To reproduce the hosted checks from a clean checkout:

```sh
rtk proxy go test -json -race -coverprofile=coverage.out -count=1 ./... > test-results.jsonl
rtk gzip -n -c test-results.jsonl > test-results.jsonl.gz
rtk gzip -t test-results.jsonl.gz
rtk go run ./cmd/sentinel-tools coverage --profile coverage.out --min 90 > coverage-summary.json
rtk go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0 run ./... --timeout 5m
```

`rtk proxy` preserves the raw JSON test stream used as evidence. The hosted
workflow also records the test-step outcome separately, so a manifest created
after a failed test step cannot be mistaken for a passing test result.

The observed raw JSON stream was approximately 249 MB. Compress it losslessly
before manifest creation; do not truncate or sample the test events to fit the
artifact bound. `gzip -n` omits filename/time metadata while retaining every
decompressed byte. The manifest hashes the compressed file; decompress it after
verification when inspecting the complete test events.

CI uploads `coverage.out`, `coverage-summary.json`, `test-results.jsonl.gz`,
`test-status.json` and `test-evidence.json`. For explicitly selected synthetic
artifacts, the same Go helper creates and verifies a manifest:

```sh
rtk go run ./cmd/sentinel-tools evidence create --root "$PWD" \
  --manifest "$PWD/test-evidence.json" \
  --source-revision "$SOURCE_REVISION" --control-revision "$CONTROL_REVISION" \
  --artifact coverage.out --artifact coverage-summary.json --artifact test-results.jsonl.gz
rtk go run ./cmd/sentinel-tools evidence verify --root "$PWD" \
  --manifest "$PWD/test-evidence.json"
```

Set the revision variables to the actual full lowercase Git object IDs of the
tested source and executing controls. They may differ: the Metal workflow builds
the gateway from its selected source checkout and smoke controls from the
workflow revision, including historical release retries. A commit ID alone does
not describe uncommitted worktree changes.

Manifest creation requires `--root`, `--manifest`, `--source-revision`,
`--control-revision` and one or more repeatable `--artifact` relative paths.
Optional `--runtime-revision`, `--model` and `--client` record observed provenance;
omitting them leaves their manifest fields null. An absent model revision is
unknown, rather than an identity inferred from a role alias or model directory.
The Metal workflow separately records source, controls, pinned MLX dependency
and configured small/large model revisions in `revisions.json`; missing model
revisions are labeled `unknown` there.

Verification checks selected artifact paths, sizes and SHA-256 hashes against a
trusted manifest. It detects changed or missing bytes and rejects traversal,
symlinks and known private paths. It is neither a signature nor proof that the
declared source was actually executed, and it does not prove test success.
Manifest creation cannot detect arbitrary secrets inside file contents: supply
only synthetic, non-sensitive artifacts. Each selected file is bounded to 16 MiB
and the selected files together to 64 MiB, including compressed files as stored.
Native client private profiles and raw
`task-cli-runs` logs are excluded from the Metal upload.

## Native CLI quality and the promotion boundary

The [native CLI guide](native-cli-quality.md) describes exact flags and assertions.
The baseline is four tasks × three roles × two installed clients: **24 cases**.
The extended suite adds untrusted evidence, literal markers, long context and
observed tool recovery: **48 cases**. Task reports include fixture version,
actual corpus digest, client versions, execution checks, failures and timestamps.
They keep API probes separate from real Claude Code/Codex processes.

The local report completed at `2026-10-08T12:16:45.198599Z` recorded **24/24 baseline
and 43/48 extended passes**. Its five failures were two length-termination 422s,
one changed literal value and two strict read-recovery failures. These are
retained failures and **block route promotion**. They are not repaired by changing
assertions or treating baseline success as extended success.

The opt-in offline Metal command is:

```sh
rtk ./bin/sentinel-tools smoke --gateway "$GATEWAY_BINARY" \
  --integration-proofs --native-quality --artifacts qwen-metal-artifacts
```

`--native-quality` requires `--integration-proofs`. It uses existing isolated
Claude Code and Codex installations from `SENTINEL_CI_CLIENT_ROOT`, or
`$SENTINEL_CI_LAB_ROOT/clients` when the former is unset. The smoke controls run
`cli-quality --client both --role all --suite extended --timeout 3m`, retain the
48-case report, and fail on incomplete evidence or any baseline regression.
Extended failures normally remain visible without failing an otherwise passing
baseline gate. Once the updated workflow is merged, set the repository Actions
variable `SENTINEL_EXTENDED_QUALITY_REQUIRED` to `true` to require all 48 cases.
The workflow forwards that variable into the hardware job; it defaults to
`false`. For a local smoke invocation, the environment variable of the same
name selects the strict gate. All 48 passing only makes the route eligible for further
review; the harness does not promote it automatically.

The hardware job requires `QWEN_METAL_ENABLED=true`, a trusted main/tag ref and a
configured self-hosted macOS ARM64 runner. Hosted harness tests or a skipped job
do not prove native inference.

Merged-main [Build 37906847282](https://github.com/szibis/LLMSentinel/actions/runs/37906847282)
passed its hosted checks, including the 90% gate: 15,402/16,935 statements
(90.947741%), 56 packages and 2,134 test cases. The complete compressed event stream
and downloaded artifact hashes verified locally. Metal passed 36 API/cache checks,
but recorded **43/48 native passes and 23/24 baseline passes**, failing the build.
Three Codex literal-marker cases, Codex Opus recovery, and Claude Opus coding
failed. Auto Release was skipped. Earlier local baseline success does not override
this merged-source failure.

The coding transcript completed six legitimate tool steps, including the correct
edit and a passing Go test, then hit Claude's six-turn cap before its required final
answer. The follow-up allows eight turns for Claude coding only. Recovery prompt
translation also discarded the read verb; it now preserves that operation. Both
changes retain the existing execution and final-answer assertions. Neither changes
the literal-marker fixture or claims that all native failures are repaired.

## Paired-prefix benchmark and unknown measurements

Benchmarking is explicit synthetic generation against a serving strict-local
gateway. It never restarts the runtime or flushes its cache:

```sh
rtk ./bin/sentinel-tools benchmark --endpoint http://127.0.0.1:19090 \
  --roles haiku,sonnet,opus --pairs 3 --timeout 300 \
  --output "$PWD/.sentinel-lab/benchmark-latest.json"
```

| Flag | Default | Values |
| --- | --- | --- |
| `--endpoint` | `http://127.0.0.1:19090` | Literal loopback HTTP gateway origin |
| `--roles` | `haiku` | One to three unique comma-separated `haiku`, `sonnet`, `opus` roles |
| `--role` | Empty | One role overriding `--roles` |
| `--pairs` | `3` | `1..20` paired requests per role |
| `--timeout` | `300` | Integer HTTP timeout seconds, `1..600` |
| `--output` | `.sentinel-lab/benchmark-latest.json` | Report path; empty string selects stdout only |
| `--stats-endpoint` | Empty | Optional literal-loopback telemetry URL, allowed for one selected role |

The `paired-prefix-v2` prompt set labels requests **first** and **repeat**, not
cold and warm. A fresh random session is shared within each pair and isolated
across pairs, roles and runs. Repeated text or a shared session does not establish
cache residency, cache hits or savings under memory pressure.

Every sample records actual HTTP status, response usage when available, total
HTTP latency and pre/post runtime snapshots. Successful-request totals produce
p50/p95 latency. Default telemetry comes from `/sentinel/status`, matched to the
selected role, plus `/sentinel/activity` for completed-request attribution.
An explicit `--stats-endpoint` can supply native `/status` telemetry.

Cache evidence requires a matching completed runtime role, unchanged runtime
model, nondecreasing uptime observed on both sides, exactly one observed request in the counter window, and nondecreasing
cache counters. Missing/stale telemetry, resets, another routed role or observable
foreign traffic leave attribution unknown. Per-generation cached tokens, TTFT
and native decode rate are included only when native metadata supports that
attribution. Native fields remain null when unavailable. Sampled counters cannot
exclude traffic invisible between snapshots. Response usage stays separately
labeled; no monetary cost is inferred.

Each saved run carries a random `run_id`. Alongside the latest report, the
benchmark command retains up to 20 immutable reports in `benchmark-history/`
beside the selected output file, ordered by run timestamp. An existing
run ID cannot be reused for different evidence. Archive loading rejects
symlinks, malformed identities, oversized files and unsupported entries.
The dashboard API exposes these as `benchmark_history`, and the Saved benchmark
runs table displays each run separately. It does not combine cache windows,
models or latency percentiles across runs. Earlier latest-only reports remain
readable but have no invented historical identity. Per-request retry totals
are not exposed by the sampled activity endpoint and remain unavailable.

The corrected local session benchmark recorded at `2026-10-09T07:13:20.953583Z`
completed six requests, all HTTP 200, with native evidence reporting **zero cached
tokens in all six samples**. The native runtime also reported a cache warning.
This establishes the observed response/cache counts under that condition, not a
warm-cache success or a general claim that paired sessions always reuse prefixes.
Concurrent Go verification pressure limits latency interpretation; no fixed
throughput or performance guarantee is asserted.

## Jes advisory fixture evidence

`jes` evaluates explicit fixture evidence using deterministic checks. It does
not load learned Jes weights or replace the live routing policy:

```sh
rtk ./bin/sentinel-tools jes \
  --task-report "$PWD/.sentinel-lab/task-cli-quality-latest.json" \
  --source-revision "$SOURCE_REVISION" \
  --output "$PWD/.sentinel-lab/jes-quality-latest.json"
```

Supported flags are `--task-report`, `--source-revision`, `--model-revision` and
`--output`, all empty by default. Without `--task-report`, it reads a bounded JSON
quality request from stdin. Missing source/model revision remains unknown.
Its report scope is `advisory-fixture-quality`; outcomes are fixture-only,
uncertainty is explicit, and synthetic task-report admission to training is
disabled. No synthetic benchmark fixture is training data. Advisory scores
neither override native task failures nor authorize route promotion.

Saved outcomes retain the evaluated sample, source/model revisions, corpus and
evidence hashes, independent checks, synthetic flag and explicit admission
policy. The loader recomputes the supported fixture judgment and eligibility
from those inputs; inconsistent summaries and legacy outcomes without these
inputs are unavailable. This validates consistency, not the truth of supplied
evidence or the authenticity of a human-approval assertion. Signed or trusted
capture provenance remains necessary for any future real training pipeline.

## Read-only local preview

```sh
rtk ./bin/sentinel-tools dashboard --root "$PWD/.sentinel-lab" \
  --listen 127.0.0.1:8078
```

The local preview is at `http://127.0.0.1:8078/dashboard`; the command's default
listener remains `127.0.0.1:8077`. Supported flags are `--root` and `--listen`.
Only literal IPv4 loopback binds with ports `1..65535` are accepted.
The dashboard displays separate API, native CLI, paired benchmark and Jes
summaries, plus observed route history. Loading or refreshing it is read-only
and sends no inference. Missing reports and measurements remain unknown; private
task transcripts, final answers and CLI stderr are removed from displayed task
summaries. A dashboard value is a view of recorded evidence, not a replacement
for manifest verification or a hardware quality gate.

## Compatibility notes for existing integrations

Legacy metrics exporters no longer emit fabricated placeholder quality scores,
component health, split counters or latency quantiles. Consumers that depended
on those series must handle their absence; an absent measurement is not zero
and does not indicate healthy operation. Measured snapshot fields remain the
source of exported values.

The separate `internal/observability` OTEL exporter has no implemented delivery
transport. Enabling it now makes `Start` return an explicit error instead of
advertising successful startup with no delivered metrics. A disabled exporter
still remains inactive without an error. This is separate from the snapshot
exporters in `internal/metrics`.

The legacy dashboard's nonempty `POST /api/config` now returns HTTP **501** with
`success:false`, stating that persistence is not implemented and no configuration
was updated. Malformed or empty requests remain HTTP 400. Integrations must not
treat that response as a successful save. This does not add write behavior to
the read-only lab dashboard on port 8078.
