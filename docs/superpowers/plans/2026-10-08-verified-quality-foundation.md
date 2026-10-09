# Verified Quality Foundation Implementation Plan

> **For agentic workers:** Use superpowers:executing-plans to implement this plan task by task, with a final independent review.

**Goal:** Deliver all five requested improvements with measured 90% coverage, hardened E2E/fuzz and reproducible real CI evidence.

**Architecture:** Extend release, lab, taskquality and labdashboard, adding focused Go packages for benchmark statistics, evidence manifests and Jes evaluation policy. Wire commands through sentinel-tools and CI through existing trusted hardware workflows.

**Tech Stack:** Go standard library and existing dependencies, GitHub Actions, cached MLX-Flash, installed isolated Claude Code/Codex.

**Spec:** ../specs/2026-10-08-verified-quality-foundation-design.md

## Global Constraints

- Pure Go in Sentinel; no new Python source or automatic weights/downloads.
- Default local-only serving; no paid requests or production transcript artifacts.
- Coverage floor 90% in the user-selected scope; publish complete repository coverage.
- Unknown usage/timing/cost remains null; synthetic-only CI capture.
- Preserve immutable release provenance, owned-process safety and client protocols.

## Review Focus

- Restart races with CI pause/restoration must retain restoration and never signal unrelated processes.
- Unpublished reviewed VERSION must not reuse a merged release branch or downgrade a version.
- Failed/interrupted tool streams must never appear as completed execution evidence.
- Cache comparisons across resets/models must be unknown, never misleading savings.
- Malformed judgments or incomplete evidence must not admit training data or promote routes.

## Execution ledger

Track task status, concrete test failures, fixes, commands and measured results in
the PR descriptions and this plan. Existing artifacts remain private outside Git.

### Continuation review, 2026-10-09

- Independent read-only review identified three Important gaps: missing uptime
  admitted cache attribution; saved Jes outcomes lost audit inputs and trusted
  eligibility flags; benchmark history was overwritten by the next run.
- Final: fixed missing continuity — live and persisted benchmark regressions and
  dashboard missing-uptime regression RED→GREEN; missing uptime now leaves
  derived rates/cache/TTFT unknown.
- Final: fixed Jes audit inputs — lossless decision round-trip and unsupported
  eligibility regressions RED→GREEN; loading recomputes the fixture judgment
  from retained sample and policy, rejecting edited or incomplete summaries.
- Final: fixed persisted benchmark history — retention/identity/API/DOM
  regressions RED→GREEN; up to 20 separate immutable runs survive restarts.
- Final: minor (deferred): per-request retry totals are unavailable from sampled
  adapter activity. Documentation states this; no total is inferred.
- Final: Ruling: observed nondecreasing uptime on both sides is the available
  continuity signal, rather than a fabricated runtime-instance identifier.
  Hidden resets/traffic between snapshots cannot be excluded; cost if wrong is
  overstated cache attribution, bounded by the explicitly documented limitation.
- The independent review was interrupted by the daemon restart after delivering
  these findings. The author finishes the validation pass; no complete fresh
  reviewer verdict or visual browser result is claimed.
- Final verification: `go test -json -race -coverprofile=coverage-final.out
  -count=1 -timeout=15m ./...` exited 0 across 56 packages and 2,133 passing
  test cases. Coverage is 20,858/22,454 statements (92.892135%, global 90% gate
  passed). Pinned lint returned 0 issues; actionlint and all four DOM contracts
  passed. Fresh bounded fuzz campaigns passed 596,955 admission, 389,754
  persisted-attribution and 728,430 coverage-profile executions.
- Local read-only preview is available on port 8078 with 48 native observations
  and 48 advisory outcomes, zero eligible for training. Both existing Apple
  Silicon runner registrations were restored with their prior labels after
  the restart, and GitHub reports them online. Expanded merged-main hardware
  execution and the five native extended-quality failures remain pending.

### Patched toolchain and hosted evidence, 2026-10-09

- The first PR Build run 37901269295 passed test, global coverage, compression,
  evidence verification and upload. Downloaded artifacts verified locally at
  merge source/control `978fbd5eafc6381836728a30b12bba9fef2bfe59`; coverage
  was 20,858/22,454 statements (92.892135%). Security failed on nine reachable
  Go standard-library vulnerabilities fixed by Go 1.27.2.
- Updated module/container/docs to Go 1.27.2. Linter 2.13.2 could not decode
  its compiler export format; 2.14.0 passes. The newer gosec's G703 report on
  LAB_WORKSPACE Stat is suppressed only at that operation: selecting the local
  caller's existing project directory is intentional, not a remote sandbox.
- Patched local full race suite: exit 0, 56 packages, 2,133 cases; global
  coverage 15,403/16,935 statements (90.953646%), unchanged 90% gate passed.
  Same 150 files and 11,552 blocks as the earlier profile; 1,839 block statement
  counts changed and two coverage-presence bits changed. The toolchain includes
  a cover fix; percentages across toolchains are not directly comparable.
- Go 1.27.2 govulncheck: no vulnerabilities. Linter 2.14.0: zero issues.
  actionlint and four DOM contracts passed. Complete event logs compressed
  losslessly and retained with status, coverage, security and lint output.
- Patched Build security, lint, four platform builds, Docker and web passed.
  The separate Security & Performance workflow still pinned linter 2.13.2;
  its job 113728437863 reproduced the export-version-5 decoder failure.
  Updated that remaining workflow pin to 2.14.0 and checked all active build
  and security entry points for stale pins. No runtime code changed.

### Review checkpoint, 2026-10-09

All five implementation areas are connected. The repository-wide local race
run measured 20,755/22,339 statements (92.909%); hosted CI enforces the whole
repository's unrounded 90% floor. Regression tests also uncovered and corrected
cache-key collisions, retry body replay, metrics races and fabricated legacy
measurements. The foundation guide records compatibility changes.

Native installed-client evidence recorded 24/24 baseline and 43/48 extended
passes. Five extended failures are retained and block promotion; they are not
declared fixed. All 48 are collected by the hardware gate, with optional strict
failure through the validated repository variable. A corrected six-request
benchmark used isolated pair sessions and observed zero cached tokens; pressure
and cache residency remain relevant. Jes is advisory fixture evidence only,
with synthetic training admission disabled.

Additional measured fuzz campaigns completed 566,829 training-admission,
281,013 coverage-parser and 205,239 extended-evidence executions, alongside the
protocol/report campaigns. CI retains campaign logs and minimized failures.
Test execution logs are compressed losslessly before bounded manifest creation.

Dashboard HTTP and three DOM rendering contracts passed. The review preview is
http://127.0.0.1:8078/dashboard. Visual browser inspection was unavailable because
Computer Use permission was denied; it is not reported as a passed browser E2E.
Independent review corrected strict-gate forwarding and the oversized raw-log
artifact problem. The next merged trusted-main Metal run remains necessary to
establish expanded hardware CI evidence. Release publication awaits the reviewed
workflow; historical failed release sources are not tagged without verification.

The task list below is the original work breakdown. This checkpoint and the
reviewed CI artifacts distinguish completed implementation from pending live
publication/visual checks; unchecked publication steps are not success claims.

### 1. Release and lifecycle reliability

- [ ] Test restart during a CI lease preserves Resume=true and sends no stop/start.
- [ ] Implement restart under the existing control lock, using CI-safe stop and startLocked.
- [ ] Test semantic version floor for older/newer tags, unpublished VERSION, malformed versions and overflow.
- [ ] Add release preparation --version-floor and wire tested VERSION into auto-release.
- [ ] Execute workflow fixtures, race tests and retry eligible failed release-build jobs.
- [ ] Prepare focused reviewed PR with exact red/green evidence.

### 2. Coverage and broader quality fixtures

- [ ] Measure total/per-package baseline with go test -coverprofile.
- [ ] Add tested Go coverage enforcement, actual statement accounting and complete reports.
- [ ] Expand versioned fixtures and independent native evidence assertions.
- [ ] Add HTTP E2E/fuzz for cancellation, fragmented/multiple calls, large history, tool failures and untrusted tool data.
- [ ] Increase meaningful behavior coverage to >=90% in the selected scope; enforce it in CI.
- [ ] Preserve reproducible minimized failures; run native fixtures against cached models.

### 3. Performance/cache evidence

- [ ] Specify bounded benchmark report with nullable native metrics and paired-run provenance.
- [ ] Test percentile calculations, resets, missing metrics, correction accounting and cancellation.
- [ ] Implement local-only benchmark command and immutable evidence manifests.
- [ ] Verify cold/warm measurements using real cached runtimes, without altering unrelated caches.
- [ ] Connect trusted hardware CI, always-upload evidence and threshold assertions.

### 4. Dashboard histories

- [ ] Test report loading, freshness, bounded history, reset handling and unknown measurements.
- [ ] Expose read-only performance/quality/history API and charts with actual model/routes/retries.
- [ ] Verify HTTP behavior, browser rendering and synthetic graph fixtures.
- [ ] Document sampling, retention and comparison boundaries.

### 5. Jes quality foundation and final proof

- [ ] Add versioned evaluation corpus, scored comparison records and advisory policy decisions.
- [ ] Test fail-closed admission, provenance, malformed scores, missing observations and human approval.
- [ ] Connect capture metadata, commands and dashboard without enabling untrained Jes or paid replay.
- [ ] Extend fuzz and E2E across evaluation/evidence/report boundaries.
- [ ] Run complete tests, >=90% coverage gates, race, lint, fuzz and real/native CI proofs.
- [ ] Independent whole-change review; fix important findings, publish reviewable PRs, leave lab healthy.
