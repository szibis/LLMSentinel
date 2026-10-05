# Initial transport verification — 2026-10-05

## Verified

- Go 1.27.1 binary built with `make gateway-build`.
- Fresh `go test -race -count=1 ./internal/localgateway ./cmd/sentinel-gateway` passed. Real HTTP tests cover unchanged request JSON, immediate forwarding of received SSE before completion, upstream cancellation on disconnect, body bounds, unsupported tool/protocol refusals, local-only URL policy, health/status and upstream error/redirect handling.
- `make gateway-check` and `go vet ./...` passed.
- Full ordinary upstream `go test ./...` completed successfully. Its mock-model tests do not establish real inference, semantic caching or MCP implementation quality.
- Two Python isolation tests passed: existing edited lab config is preserved, and actual child processes do not receive inherited production-token/proxy markers.
- The development workflow's `make lab-test gateway-build` passed: focused Go race/vet checks and all eight Python tests. Additional runner tests cover exclusive ownership, stop markers, invalid ownership state, complete cached model files/shards, and real child-process cleanup while an unrelated process stays alive.
- `make lab-help lab-init lab-stop lab-logs` completed. Separate profiles were prepared; no owned lab was running and no runtime logs existed.
- Installed Codex 0.160.0 and Claude Code 2.1.286 returned their versions using the separate lab environments. No client generation/agent session or inference runtime was launched.
- Jade working tree remained unchanged. Global Codex/Claude settings, login, installation and running sessions were not deliberately modified.

## Restrictions and failures

The lab now selects only lab-owned CLI executables and offers `make lab-clients-update` for latest stable npm packages. Registry metadata checked on 2026-10-05 returned Codex 0.160.0 and Claude Code 2.1.289. Actual package installation was not completed in this session: npm failed with `ENOTFOUND registry.npmjs.org`, and an attempted alternate tarball download also failed. No new CLI version execution or installation success is claimed. The update command must run from a normal terminal with registry access. The installation-destination/PATH/no-global-fallback checks are unit tests, not download or native-binary verification.

A standalone gateway launch was refused by the execution sandbox: `listen tcp 127.0.0.1:19090: bind: operation not permitted`. The owned attempted process exited; no alternate privileged launch was attempted. This leaves standalone lab operation unverified here, despite the passing real loopback tests.

The new `make lab-rebuild ATTACH_RUNTIME=1` completed its stop/build stages, then failed at the supervisor's port preflight with `Operation not permitted`. No runtime or gateway child was started. `make lab-status` reported the supervisor stopped and the sandbox refused the health connection. A successful live run/rebuild with inference remains unverified; execute the documented commands from a normal terminal with an explicit cached model.

The full upstream race run did not pass and was interrupted through its own execution session after these failures:

- `internal/client.TestCreateMessage`: sandbox denied an IPv6 localhost listener.
- `internal/statusline.TestWebhookPoll_PayloadValidation/missing_required_fields`: same listener restriction.
- `internal/dashboard.TestToolsAdd_Returns200OnSuccess`: attempted to save upstream configuration under `~/.claude-escalate/config.yaml`; sandbox refused the write. Do not run the upstream dashboard configuration tests against production settings.

These are separate from the passing gateway race tests. No complete upstream race-suite success is claimed.

## Not yet verified or implemented

The background-server increment passed `make lab-test` with ten Python tests plus the focused gateway race/vet checks. A real detached helper supervisor verified that start returns, repeated start preserves the current owner, and a stop marker terminates the supervisor. A separate failing helper verified immediate error reporting and retained logs. These helper processes are not an inference or HTTP-server proof. The actual background `make lab-run ATTACH_RUNTIME=1` correctly reported the sandbox's port preflight failure, returned an error, and left no owned lab running. Live model readiness and live server rebuild still require verification in a normal Mac terminal.

Real-model transport through this gateway, native tool calling, true token streaming, server-side inference cancellation, Responses and Messages adapters, Codex/Claude agent workflows, offline network isolation, Jes routing and training-platform integration remain open. The lab launcher refuses to start clients until their required protocol/tool capabilities are advertised. Advertising them is not itself proof of compatibility.

Full local CLI proof must use owned runtime/gateway processes on dedicated ports, an explicitly selected small cached model, dedicated configuration directories and a disposable workspace. Test conversation, file read, approved edit, real check and recovery before moving on to multiple models.
