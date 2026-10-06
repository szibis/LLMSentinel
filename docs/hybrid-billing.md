# Local, learning and hybrid billing

Sentinel separates inference routing from private training capture. Capturing a
conversation does not change how the CLI authenticates or how a vendor bills it.

| Mode | Inference path | Commercial billing | Capture |
| --- | --- | --- | --- |
| Serving | CLI → Sentinel → local Qwen runtime | No commercial API request | Optional local candidates |
| Learning | CLI → vendor directly; hooks copy completed local transcripts | The CLI's actual vendor authentication determines billing | Optional private reference candidates |
| Hybrid | CLI → Sentinel → local runtime or explicitly configured vendor API | Vendor API usage for commercial routes | Optional private local/commercial candidates |

For subscription-backed learning, keep the commercial CLI pointed directly at
its vendor and use the capture hooks. Sentinel does not read, collect, forward or
reuse subscription OAuth credentials. Claude Code API-key environment variables
take priority over subscription login and cause API billing. Keep
`ANTHROPIC_API_KEY` unset in the commercial Claude Code process when you intend
to use the subscription. See [Anthropic's authentication guidance](https://support.claude.com/en/articles/12304248-manage-api-key-environment-variables-in-claude-code).

Codex supports ChatGPT sign-in and API-key sign-in as distinct authentication
paths. API-key usage is billed through the OpenAI Platform account at API rates,
instead of included ChatGPT plan credits. A custom gateway URL does not make a
gateway's Platform API calls subscription-backed. See [OpenAI's authentication
guide](https://learn.chatgpt.com/docs/auth).

## Explicit paid hybrid configuration

Hybrid requires `--mode hybrid` and `--allow-paid-api` before any configured
commercial provider can be enabled. Choose commercial models explicitly;
Sentinel has no implicit vendor model or automatic paid fallback.

| Configuration field | CLI flag | Meaning |
| --- | --- | --- |
| `AllowPaidAPI` | `--allow-paid-api` | Explicit permission for separately billed vendor API calls |
| `AnthropicBaseURL` | `--anthropic-base-url` | HTTPS API base, normally `https://api.anthropic.com` |
| `AnthropicModel` | `--anthropic-model` | Exact commercial Anthropic model ID |
| `AnthropicAPIKeyEnv` | `--anthropic-api-key-env` | Name of the environment variable containing the Anthropic API key |
| `OpenAIBaseURL` | `--openai-base-url` | HTTPS API base, normally `https://api.openai.com/v1` |
| `OpenAIModel` | `--openai-model` | Exact commercial OpenAI model ID |
| `OpenAIAPIKeyEnv` | `--openai-api-key-env` | Name of the environment variable containing the OpenAI Platform API key |

Set the chosen key variables only in the Sentinel gateway process. Custom names
such as `SENTINEL_ANTHROPIC_API_KEY` and `SENTINEL_OPENAI_API_KEY` keep gateway
API credentials separate from a commercial CLI's subscription environment. The
flags contain variable names, never secret values. The gateway does not consult
client Authorization, API-key or cookie headers for commercial authentication.
An unset configured key produces an error when a commercial route is selected;
it never falls back to another paid provider or silently changes to local work.

Only HTTPS bases without credentials, query strings or fragments are accepted.
Certificates are verified. HTTP redirects and environment proxy settings are
disabled. Vendor requests retain their native Anthropic Messages, OpenAI
Responses or OpenAI Chat Completions body shape, with the model replaced by the
configured commercial model. Required authentication comes from the configured
API-key environment variable. Tools still execute in the CLI under its existing
permissions.

## Live policy

The paid-provider configuration and opt-in are immutable for the gateway
process. Live policy changes choose among already configured paths:

| Policy | Decision |
| --- | --- |
| `local-only` | Every supported request remains local |
| `balanced` (default) | `opus`/`sentinel-opus` requests use the configured matching vendor; OpenAI Responses `reasoning.effort` high/xhigh also selects the commercial path |
| `quality` | Every supported request uses its configured matching vendor |

Requests without a matching configured vendor remain local, including under
`quality`. Anthropic routes apply only to `/v1/messages`; OpenAI routes apply to
`/v1/responses` and `/v1/chat/completions`. Under `balanced`, Haiku/Sonnet and
ordinary local requests stay local unless a Responses request explicitly asks
for high/xhigh effort. Vendor failures and rate limits pass back to the caller;
the gateway neither retries nor changes provider. Switching policy affects new
requests, not an already running generation.

This is a deterministic role/effort policy. A trained Jes checkpoint is not
connected to these decisions, and the gateway does not label them as Jes model
inference.

## Capture and cost evidence

To retain both local and commercial candidates in hybrid mode, add
`--training-mode --training-dir /private/tmp/sentinel-hybrid-learning` to the
hybrid startup command. Capture is opt-in and can then be paused/resumed through
`/sentinel/control` without changing routing or billing authorization.

Commercial capture records provider, configured model, request/attempt identity,
protocol and `billing_class: api_usage`. Complete vendor usage is retained in
`reference.vendor_usage`, including nested cache or reasoning details. Integer
usage fields are also available in the existing usage map. Missing usage stays
unknown; Sentinel does not invent token counts, subscription entitlements,
dollar estimates or quality scores.

Candidates remain `quality.status: unscored` and
`quality.training_eligible: false`. A successful API response is evidence of
protocol completion, not proof that the answer is good enough for training.
Capture uses the private recorder's permissions, redaction and rotation. The
gateway never records authentication headers.

Requests are bounded to 2 MiB, nonstream responses to 8 MiB, forwarded streams
to 64 MiB and accumulated stream capture to 8 MiB. Valid streams pass through
unchanged, with no fabricated completion events. Capture overflow does not
change a stream below the forwarding limit, but records incomplete capture and
unknown usage. Streams ending without their protocol's terminal completion
record failure metadata without a partial training answer. A forwarding limit
or connection failure terminates the stream; the gateway does not claim it
completed. These limits mean some successful large vendor streams will not
produce a training reference.
