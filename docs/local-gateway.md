# Local gateway and isolated client lab

The lab exposes an experimental Claude Messages/tool bridge with three logical Claude roles over two local Qwen models. `make lab-claude` opens the real, separately installed Claude Code interactive UI. Model-free protocol tests pass; Qwen generation and a complete coding task remain unverified in this execution sandbox because MLX cannot access Metal. Codex Responses and trained Jes routing remain future work. See [role setup and verification](claude-qwen-roles.md).

## Interactive Claude Code

Your lab clients and cached model are already configured on this Mac. From `Sentinel/`, run:

```sh
make lab-claude
```

This builds the gateway, starts the lab server if needed, refreshes an older owned gateway when its required capabilities are absent, and replaces the launcher with the actual lab Claude Code executable. Server processes stay in the background. Claude receives normal terminal input directly; this is not print mode. Its built-in Read, Write, Edit, Bash, Glob, Grep and Agent tools are enabled with normal permission prompts. Initial theme/API-key/workspace dialogs belong to the separate lab profile. The API key is a local placeholder, not a paid provider credential.

The default project is disposable `.sentinel-lab/workspace`. To work on a project of your choosing with the same separate profile:

```sh
make lab-claude LAB_WORKSPACE=/absolute/path/to/your/project
```

To test the agent, ask it inside the UI to create `hello.py` and `test_hello.py`, run the test, deliberately change the expected result, then diagnose and fix the failure. Review tool requests and approve operations as usual. Inspect the actual files and test output; assistant claims alone do not establish success. Exit Claude normally; `make lab-stop` shuts down the lab server.

The adapter renders client tool definitions/results into the local model's conversation and accepts only complete JSON tool requests with known names and basic schema validation. Claude Code executes them; Sentinel never executes model commands itself. This is an experimental prompt-based tool protocol, not native MLX-Flash function calling or a claim of commercial-model quality. Malformed or truncated output produces an error, without executing partial tool input. Responses remain buffered. Role output limits are 1,024/4,096/8,192 tokens for Haiku/Sonnet/Opus, further capped by the client request. Exact token counting is absent; usage reflects available upstream counters, including its currently unavailable input count. Images, Anthropic thinking blocks and advanced server tools are rejected. Opus's internal Qwen reasoning is isolated from final client text and tool input. Prompt-cache metadata is not implemented.

Bare mode keeps the real coding UI while isolating inherited hooks, plugins, keychain access and ancestor instruction discovery. MCP is explicitly empty; advanced Claude features are not all enabled. Your global installation/profile is not used. The launcher declares the minimum configured model context window, including nested Qwen metadata. Sentinel owns the explicit role routing policy through `DecisionRouter`; this is not trained Jes inference. The lab starts in `opusplan`, with Haiku Explore and Opus Plan agents.

## Everyday development workflow

From `Sentinel/`, run `make` to see the lab commands. Requirements are Go 1.27.1, Python 3, Git, and an existing MLX-Flash installation for owned inference. Go may fetch the declared toolchain and modules on the first build; the lab does not install clients or download model weights.

```sh
cd /Users/slawomirskowron/projects/model_training/Sentinel
make lab-test
make lab-run MODEL_PATH=/absolute/cached/model MLX_FLASH_BIN=/absolute/path/to/mlx-flash
```

Replace both paths with your existing runtime and a complete local model directory. Model selection is saved in ignored `.sentinel-lab/runtime.json`; later starts and rebuilds reuse it. The runtime receives offline Hub settings and speculation disabled. This prevents implicit Hub downloads, but is not a network sandbox. No model is selected automatically. Without a saved selection, `make lab-run` starts the gateway expecting an already running runtime on port 19091.

Startup returns your terminal prompt; run the following commands in the same terminal:

| Command | Purpose |
| --- | --- |
| `make lab-status` | Inspect supervisor state, gateway health and real runtime health. |
| `make lab-ask PROMPT='Explain this system briefly'` | Send a plain-text prompt and print received output. |
| `make lab-rebuild` | Stop this lab, rebuild the gateway, and restart with saved settings. |
| `make lab-restart` | Restart the server with saved settings, without rebuilding. |
| `make lab-logs` / `make lab-follow` | Read recent logs or follow new output. |
| `make lab-stop` | Stop only children owned by this lab supervisor. |
| `make lab-doctor` | Inspect isolated client versions and gateway health. |

`lab-run` (also `lab-start`) and `lab-rebuild` start a detached supervisor. Closing the terminal leaves the server running; `make lab-stop` stops its owned gateway/runtime processes. Startup acknowledges the new supervisor and catches immediate child exits; this is separate from model readiness. Preparation errors are printed and saved in `.sentinel-lab/supervisor.log`, alongside gateway/runtime logs. This is a local background server, not a login/boot service: it does not automatically restart after a crash or Mac reboot. `make lab-foreground` is available for debugging, where Ctrl+C stops the lab.

An exclusive lock prevents two supervisors; occupied ports cause startup to fail without stopping existing services. `ATTACH_RUNTIME=1` temporarily uses your existing runtime on 19091 and leaves it running on shutdown. Gateway health alone does not mean a model is loaded: inspect runtime health/logs and verify a real prompt. First generation may wait for loading and the runtime's internal buffering.

`make lab-test` runs the focused gateway race tests, vet checks, and Python isolation/process-ownership tests without requiring model weights. `make lab-codex` remains guarded because the Responses adapter is pending. Claude launch requires the new gateway's Messages and tools capabilities; gateway health does not prove model quality/readiness.

## Build and verify

Use Go 1.27.1:

```sh
make gateway-check
make gateway-build
./bin/sentinel-gateway -upstream http://127.0.0.1:19091/v1
```

The runtime must already be running on the separate lab port. This command starts only Sentinel on 127.0.0.1:19090; it does not start/stop other services, install dependencies or download weights. The binary forwards received SSE immediately, but the audited runtime buffers generation internally. HTTP cancellation reaches the upstream request; this does not prove server-side inference stops.

```sh
curl http://127.0.0.1:19090/health
curl http://127.0.0.1:19090/sentinel/status
curl http://127.0.0.1:19090/v1/models
curl -N http://127.0.0.1:19090/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{"model":"local","messages":[{"role":"user","content":"Hello"}],"stream":true}'
```

Health describes the gateway only; status probes actual runtime health. There is no fabricated model-ready or download-progress indicator. Requests are bounded to 2 MiB and five minutes by default. Strict local policy rejects remote upstream URLs and redirects. The listener is loopback only and unauthenticated; it is intended for this user's isolated local lab, not a shared server.

The legacy single-runtime gateway defaults to 768 output tokens per Claude generation (`-claude-max-tokens`). Three-role mode uses the profile limits above. One JSON-format correction can cause a second generation on the same selected route. Output limits may reject long file edits and are not latency guarantees.

By default, `-claude-buffered-validation=true` validates model output before opening the Claude SSE stream. Invalid output returns HTTP 422 with `x-should-retry: false`. Valid responses retain the Messages/tool event sequence. Since MLX buffers generation, the UI waits until validation completes; no heartbeats or live model tokens are emitted in this mode. The experimental heartbeat mode remains available with `-claude-buffered-validation=false`, but failures then occur inside an already-open stream. Client retry behavior should be checked in the native CLI.

## Preserve current Codex and Claude work

```sh
make lab-clients-update
make lab-doctor
```

`lab-clients-update` installs the official `@openai/codex@latest` and `@anthropic-ai/claude-code@latest` npm packages under `.sentinel-lab/clients`, including their platform binaries. It requires Node.js/npm and registry access. Run it again to update the lab to the current stable versions. npm's cache and lockfile stay inside the lab; global installations and shell PATH configuration are not modified. Installation does not start an agent or install model weights. This explicit update command uses the network; ordinary server startup does not install clients.

The launcher and doctor use only `.sentinel-lab/clients/node_modules/.bin/{codex,claude}`, and child PATH puts these first. They never fall back to an older global CLI. Doctor reports each lab executable path and installed version; it exits unsuccessfully if either client is missing. Config isolation tests cover the installation destination and credential exclusions; they do not prove a new CLI release is fully compatible with our gateway.

Lab state lives in ignored `.sentinel-lab/codex`, `.sentinel-lab/claude`, `.sentinel-lab/tmp` and `.sentinel-lab/workspace`. Codex uses its own `CODEX_HOME`; Claude uses its own `CLAUDE_CONFIG_DIR`. Child environments exclude inherited provider credentials and proxy variables. Claude updates/nonessential traffic and Codex analytics are disabled by configuration. No production settings, authentication, shell profiles, CLI installations or running processes are changed. Changed lab configs are preserved instead of overwritten.

`doctor` invokes only installed clients' version commands in those environments. Launch commands (`python3 scripts/sentinel_lab.py codex` / `claude`) refuse to start until the gateway advertises the required protocol and tools. The Claude local-model path is experimental and officially unsupported by Anthropic. Machine-managed policy can still override application settings; separate config directories are not an OS sandbox or proof of no external network traffic. Do not claim full offline operation until it is observed.

Next work: complete the actual isolated conversation/read/approved-edit/check/recovery proof, improve native runtime tool/stream/cancellation contracts, connect trained Jes decisions, and implement Responses for Codex. See the canonical [architecture](../../LLM_SSD/docs/superpowers/specs/2026-10-05-sentinel-local-gateway-design.md). The adapter follows the [Claude gateway endpoint guide](https://code.claude.com/docs/en/llm-gateway-protocol) and [Messages event format](https://platform.claude.com/docs/en/build-with-claude/streaming). Routing non-Claude models remains experimental and unsupported by Anthropic.

See [interactive Claude validation](claude-lab-validation.md) and [earlier transport evidence](local-gateway-validation.md) for what actually ran on this Mac.
