# Verified local quality foundation

## Intent and constraints

Implement the five user-authorized stages through reviewable pull requests:
release/lifecycle reliability; broader native quality tests; repeatable speed/cache
benchmarks; dashboard histories; a Jes quality-gate foundation. Sentinel remains
pure Go. Preserve existing client wire protocols, local-only defaults, explicit
hybrid billing opt-in and private learning capture. Never fabricate measurements,
tool execution, semantic scores, trained Jes availability or model parity.

User requests 90% test coverage, E2E hardening, extended fuzzing and real CI
evidence. The implemented default is a repository-wide 90% floor, enforced on
actual Go statement counts and accompanied by the complete repository report. No denominator
reduction, hidden excludes, threshold ratchets below 90%, or skipped failures.

## Architecture

Extend existing packages rather than add a parallel gateway. Release preparation
uses the higher of the tested repository VERSION and stable tags, preserving
historical notes, immutable commits and reviewed publication. Restart holds the
existing control lock across its CI-pause check and stop/start transition; only
explicit stop may cancel restoration.

Versioned synthetic fixtures expand taskquality. Every result records assertions,
actual tool evidence, client/runtime versions, code SHA and reproducible task
identity. Cancellation, partial streams, tool failures, long context, parallel
calls and prompt-injection-like tool data have independent behavioral oracles.
Model-free E2E/fuzz complements real-model/native CLI evidence.

A Go benchmark runner captures paired repetitions of a fixed synthetic prefix
through the existing gateway and runtime metrics. Preserve first/repeat provenance,
actual model and route, retries, token counts, cache counters, native TTFT/decode
when available, and whole-task latency percentiles. Unknown stays null. Runs are
bounded and local-only. Counter-window checks mark conflicting observed traffic
unknown but cannot exclude traffic invisible to sampled counters. Never flush unrelated
user caches or claim provider dollar savings from local cache reuse.

Dashboard shows benchmark history, latency/quality distributions and routing/
recovery evidence from versioned reports and bounded activity history. Reports
retain run identity and freshness; comparisons do not cross runtime resets or
model changes. Existing historical graphs remain available.

Jes foundation supplies versioned evaluation cases, explicit scored comparisons
and a policy decision interface with deterministic checks as the temporary
backend. Persist provenance and reasons; reject ambiguous/unsupported judgments.
Training admission requires configured criteria plus human approval, and route
promotion remains advisory until explicitly enabled. No trained Jes checkpoint,
commercial traffic replay or automatic model training is invented.

## CI evidence and failure protection

Fast tests, race tests, coverage thresholds and bounded fuzz campaigns gate PRs.
Trusted Apple Silicon CI captures real cached-model proofs and native task
evidence into synthetic-only artifacts tied to an immutable code/runtime/client
identity. Preserve success and failure evidence, minimized fuzz corpus and
coverage reports. A validation summary distinguishes unavailable hardware from
a passed execution; real proof failures fail the job.

## Review criteria

All five stages connected to usable commands/UI, documented and verified.
Show actual coverage percentages, CI results and real-model/native task outcomes
for each PR. Retain regression cases for every discovered failure. No claims of
universal correctness or commercial equivalence.
