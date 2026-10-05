# Interactive Claude lab evidence — 2026-10-05

The real lab-installed Claude Code 2.1.289 executable was launched in a PTY with the same interactive arguments and environment used by `launch_spec`. It displayed its native theme selector, local-placeholder API-key confirmation, disposable-workspace trust prompt and the full Claude Code input UI showing model `local` and the lab workspace. Onboarding selections were confined to the lab profile; manual permissions were retained. The owned session exited normally with Ctrl+C. No inference task or tool operation was sent during this UI check.

Fresh `go test -race ./internal/localgateway -run Claude -count=1` passed. These are hermetic HTTP handler/transport tests, not real TCP or GPU inference tests. They cover conversation/tool-result round trips, complete tool input events, final stream events, heartbeat delivery before generation finishes, upstream cancellation, unknown tools, invalid basic argument types, empty argument objects, invalid history/images and rejection of truncated tool output. The native Claude SDK has not yet consumed a generated local-model tool turn in this execution environment.

The focused gateway binary built and vet checks passed. The fresh whole-package race test was attempted and failed when the sandbox refused `httptest` listeners (`bind: operation not permitted`). Existing TCP tests remain in the suite; no complete fresh package-race success is claimed here. See [earlier transport evidence](local-gateway-validation.md).

The launcher is configured to use the isolated installed client, bootstrap only its lab-owned server, wait for gateway availability, and refresh only an owned older gateway. It keeps native interactive input, ordinary permissions, separate config paths, empty MCP configuration and lab binaries ahead of global PATH. A chosen `LAB_WORKSPACE` retains the separate client profile. Thinking is disabled and known model context limits come from saved model metadata. Python tests exercise these contracts and process ownership.

The current tool bridge is prompt-based. It validates available names and basic schema properties/types/required arguments, not all JSON Schema keywords. Advanced thinking, images, server tools, native function calling, exact input token counts and prompt caching are not provided. MLX-Flash buffers generation; pings communicate connection liveness, not fabricated model tokens or download progress. HTTP cancellation does not establish that core inference stops without terminating the runtime.

Jes is not trained or connected. `DecisionRouter` provides the integration seam; the active policy explicitly selects the single configured local backend. Codex Responses is still pending. This increment does not claim Anthropic-model quality, fully offline client behavior, or a successful real-model read/edit/test/recovery cycle.

Final focused verification passed: the fresh Claude protocol race tests, all 18 Python tests, gateway build and vet checks. The tests do not establish native-model tool selection quality or successful execution by the real Claude CLI.

Run `make lab-claude` in a normal Mac terminal to complete that cycle. Review file changes and actual test output, then record errors or successful model/backend details before expanding tools, plugins, routing or model sizes.

## Restart regression checks

The runner now enables TCP address reuse during preflight, checks exclusive listening without `SO_REUSEPORT`, and retries address conflicts for up to three seconds. Permission denials fail immediately. Errors identify the affected port; existing services are never terminated to free it. Startup failures include only the current attempt's log output while retaining historical logs on disk.

All 24 Python tests passed after this change. Socket regression tests simulate kernel outcomes because this execution sandbox refuses real TCP listeners. A fresh `make lab-test` failed in `TestPayloadAndStreamArriveBeforeCompletion` with `bind: operation not permitted`; full TCP and native Claude/model verification remain pending in the normal Mac terminal. No listener on ports 19090 or 19091 was observed during investigation; a transient post-shutdown conflict is a possible explanation, not an observed kernel state.

## Model JSON failure investigation

The running local runtime successfully answered direct synthetic requests. A request through the gateway with a synthetic greeting and a Read tool returned `local model did not return the required tool JSON`. Reproducing the adapter's instruction directly against MLX produced malformed JSON containing the literal assignment `tool_calls=[]`. The instruction now gives valid JSON examples instead, discourages unnecessary tool calls, and permits one explicit format correction with the original conversation retained. Tools still undergo name and argument validation; malformed output is never executed or silently converted into a tool call.

Model output failures use HTTP 422 / `invalid_request_error` for non-streaming requests and the corresponding error type in SSE. Backend transport failures retain API error classification. Gateway logs include failure reasons without conversation or model-output bodies. Native Claude retry behavior for an error inside an already-started stream remains to be verified.

A corrected-instruction synthetic runtime response contained valid JSON but unnecessarily requested Read with path `stdin`. That request was not sent to Claude for execution. This is evidence of remaining tool selection limitations in the configured Llama 3.2 3B model, not a successful coding-agent turn.

## Long generation investigation

The normal-terminal lab subsequently logged a 4,097-token generation at 28.5 tokens/sec, approximately 144 seconds, rejected by the adapter's then-current 4,096-token ceiling. Native session records contain two completed text-only replies (5.453s / 157 output tokens and 12.171s / 171 output tokens), plus an earlier failed 216.567s turn. No tool-use or tool-result blocks were recorded in those completed replies.

The gateway default output ceiling is now 768 tokens, configurable at gateway startup. Production lab defaults validate the buffered response before opening SSE, so output validation failures return HTTP 422 and an explicit no-retry header. Tests cover bounded output requests, rejection before any stream opens, and successful validated tool streams. The historical heartbeat tests still exercise the optional earlier mode. Native Claude behavior with these updated defaults requires a lab rebuild in the normal terminal; output-token limits do not guarantee elapsed time or cancel core GPU inference on client disconnection.
