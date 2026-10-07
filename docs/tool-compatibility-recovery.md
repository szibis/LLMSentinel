# Tool compatibility and bounded recovery

Messages, Responses and Chat Completions share model-output normalization and
tool validation. Supported formats include the canonical JSON envelope,
OpenAI function-call envelopes, Anthropic tool blocks, Qwen tagged calls and
Gemma calls. Reasoning is separated before public output is validated.

A syntactically complete model response that fails tool formatting, argument
schema, enum, required-tool, tool-choice, empty-turn, patch grammar or parallel
call constraints receives one correction. The correction includes the actual
validation failure. Its route, model and output budget remain pinned.

The gateway buffers tool responses until the complete result passes validation.
It never invents tool arguments, executes tools itself, or exposes rejected
calls in a successful stream. If the correction also fails, the client gets a
provider-shaped error. Truncated generations, cancellation and backend failures
remain errors; recovery does not accept partial tool input.

Successful recovery sums usage from both attempts. Aggregate usage is only
marked known when both attempts reported the required measurements. Learning
capture records each attempt separately and marks rejected attempts unaccepted.

`internal/localgateway/tool_recovery_test.go` covers 120 recovery/exhaustion cases
across three APIs and JSON/SSE, plus Responses patch/parallel constraints and
missing-usage tests. Existing translation tests cover recognized model formats,
ambiguous calls, literal protocol markers, cancellation and truncation.

```sh
go test -race ./internal/localgateway ./internal/qwensmoke
go test ./...
```

The real Metal integration gate also verifies model-backed JSON/SSE, required
tools, tool-result continuation, native usage, activity and session-isolated
prompt cache reuse. Required-tool failures report boolean contract checks and
call counts without persisting arbitrary backend text.

These checks cover recognized contracts; they do not make every possible model
output valid. Keep a rejected tool response as an error rather than emitting an
unverified call. Add a synthetic regression fixture when another model format
is encountered.
