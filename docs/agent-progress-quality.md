# Agent progress checks

The local adapter validates tool schemas and output structure, but valid output
can still fail the task. A captured Claude lab turn issued the same shell lookup
twice with unchanged results, then finished with only “Let me look at the broader
project…”. Its normal end-turn and zero format errors did not mean it delivered
the requested review.

Sentinel now adds explicit completion instructions and two narrow progress checks:

- A third consecutive identical read-only lookup with two successful identical
  results in the current user turn is rejected. Read/Glob/Grep and a conservative
  shell lookup subset are covered. Changed evidence, failed results, intervening
  mutations and a new user instruction reset the repetition evidence. Explicit
  polling, monitoring and repetition requests allow repeated lookups.
- After tool results, a short English final answer consisting only of a promised
  lookup (“Let me search…”, “I will inspect…”) is rejected. Questions, limitations
  and substantive answers are allowed.

Sentinel makes at most one recovery generation on the same selected local route
and output budget, shared with the existing one-correction limit. It asks for an
answer, relevant new evidence, or an honest limitation/clarification. It never
executes rejected calls, escalates to paid providers, or invents tool results.
If recovery also fails, HTTP 422 reports the progress failure before tool dispatch.

These are deterministic failure-pattern checks, not Jes inference or semantic
scoring. They do not prove that a comparison is complete, current, factual or
source-backed. A replay produced a substantive comparison from model knowledge;
it performed no external research, so those claims remain unverified.

The gateway health/control JSON includes process-lifetime `quality_checks`
counts for rejections and recovery attempts. The browser dashboard and Claude
statusline display these separately from format corrections. Counts reset on
gateway restart. Capture records distinguish a progress failure from invalid
protocol; rejected candidates remain ineligible for training.

The lab exposes only its configured CLI tools. A request for agentic research
does not automatically add browser/search capabilities. Use an appropriate
research integration and verify its sources before treating generated comparisons
as researched conclusions. Start a fresh conversation to remove earlier repeated
exploration from the model context.
