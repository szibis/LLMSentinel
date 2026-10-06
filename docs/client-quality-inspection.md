# Client compatibility and measured quality

Sentinel translates the endpoint's wire protocol, not the proprietary model's
internal reasoning. Anthropic Messages supports text, client tool history and
validated tool use. OpenAI Responses supports text, client functions, bounded
one-level tool namespaces and the installed Codex canonical `apply_patch`
grammar. Chat Completions supports text and function tools. Images, provider
built-in tools, arbitrary custom grammars and stored Responses histories are
rejected explicitly. Local streams are buffered until validation succeeds;
this is not native token streaming.

The Qwen bridge accepts trained function/parameter tool blocks and the previous
JSON envelope. It validates complete output, available tool names, input
schemas, tool choice and completed Opus thinking. A formatting correction stays
on the originally selected model and budget. Tool results remain untrusted
evidence. This gate does not check that a path exists or an answer is accurate.

## October 6, 2026 local CLI probes

The real isolated Claude Code 2.1.289 was asked to read a temporary `proof.txt`
containing a random marker, using only its real Read executor, and return the
file contents exactly. Qwen3.5-4B served Haiku; Qwen3.6-35B-A3B served Sonnet and
Opus, with thinking enabled for Opus. These observations used the legacy MLX
runtime before native accounting deployment. They are small regression probes,
not an aggregate benchmark or commercial comparison.

| Role | Valid executed Read calls | Exact final answer | Observed failure |
| --- | --- | --- | --- |
| Haiku | 1 | Failed | Chose `/proof.txt`, then concluded absence instead of correcting the path from the tool error's working-directory evidence. |
| Sonnet | 1 | Failed | Read the correct file but copied displayed line numbers and added Markdown. |
| Opus | 1 | Failed | Read the correct file but copied displayed line numbers and added Markdown. |

An earlier Sonnet probe passed. This variability is why protocol success must
not become a semantic quality label. Formatting instructions and structured
tool-result history remove bridge artifacts, but these probes do not establish
reliable instruction adherence. No learned Jes checkpoint or semantic scorer
is connected. Captures remain unscored and ineligible for training until a
separate reviewed evaluation promotes them.

Real Codex 0.160.1 completed a local Responses request with the exact expected
`SENTINEL_CODEX_READY` answer after namespace support was added. It reported
missing local-model metadata and a transient reconnection. Its displayed zero
usage was not accepted as proof of measured zero tokens: the legacy backend
did not supply reliable prompt accounting. This probe verifies a basic client
turn; it does not establish all Codex features or editing-task quality.

Bare Claude did not expand automatically discovered personal/project command
assets in fixture probes. Explicitly loading the isolated Sentinel plugin
expanded the slash skill correctly without commercial traffic or Metal
generation. New lab sessions use that explicit plugin path.

## Token and cost evidence

Three separate quantities must stay distinct:

* Provider/native measured usage: keep its source, coverage and request/turn
  identity. MLX's formatted prompt count includes the template and tool/history
  text. Native generation includes thinking/control/EOS tokens, even when those
  tokens are absent from visible final text.
* A price estimate: requires an explicit rate card, model/version, currency,
  effective date, cache categories and service tier. It is not a bill. No rate
  card or USD estimator is currently connected.
* Actual billing: subscription inclusion, API credits, discounts and invoices
  depend on the account and authentication path. Capturing tokens alone cannot
  establish the marginal dollars charged to a subscription session.

The native MLX fix replaces the legacy fabricated zero prompt count and
re-tokenized output with final MLX-LM generation metadata. Sentinel recognizes
its capability marker and preserves unknown legacy counts. OpenAI local
responses use null usage when complete counts are unavailable. Anthropic's
required integer usage shape currently exposes zero for unavailable counts;
those wire placeholders must not be treated as accounting evidence. Training
records preserve availability flags and omit unknown native usage fields.

Direct CLI hooks parse bounded transcripts and documented per-response/turn
usage when present, preserving nested cache/reasoning counts and their source.
Reasoning counts are a subset of output, not extra tokens to add again. Hook
coverage can omit hidden wire inputs, discarded retries and unavailable
responses. Missing measurements remain null. Hybrid captures preserve the
vendor's full reported usage separately from the local model's usage; a
comparison must not attribute shadow/local work to vendor billing.

Training capture can be paused without changing inference routing. A paused
collector can acknowledge receipt without persisting it; authoritative client
spools are separate and must be disabled separately when desired.
