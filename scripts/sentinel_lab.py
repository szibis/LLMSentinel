#!/usr/bin/env python3
"""Prepare separate client state; launch only against a capable local gateway."""
import argparse
import json
import os
from pathlib import Path
import shutil
import shlex
import subprocess
import sys
import time
import urllib.request
from sentinel_control import command_assets

ROOT = Path(__file__).resolve().parents[1] / ".sentinel-lab"
ENDPOINT = "http://127.0.0.1:19090"
TOKEN = "sentinel-local-lab"  # Local placeholder, never a production credential.


def owned_directory(directory):
    """Check every owned ancestor before creating anything beneath it."""
    directory.relative_to(ROOT)
    for ancestor in (directory, *directory.parents):
        if ancestor.is_symlink():
            raise RuntimeError(f'Refusing symlink lab directory: {ancestor}')
        if ancestor == ROOT:
            break
    directory.mkdir(parents=True, exist_ok=True, mode=0o700)


def prepare():
    for name in ("codex", "claude", "workspace", "tmp"):
        owned_directory(ROOT / name)
    # A workspace root prevents inheriting Sentinel's project settings/instructions.
    if not (ROOT / "workspace" / ".git").exists():
        subprocess.run(["git", "init", "-q", str(ROOT / "workspace")], check=True)
    configs = {
        ROOT / "codex" / "config.toml": '''model = "local"
model_provider = "sentinel"
approval_policy = "on-request"
sandbox_mode = "read-only"
web_search = "disabled"
cli_auth_credentials_store = "file"

[model_providers.sentinel]
name = "Sentinel isolated local lab"
base_url = "http://127.0.0.1:19090/v1"
env_key = "SENTINEL_LAB_TOKEN"
wire_api = "responses"
requires_openai_auth = false
request_max_retries = 0
stream_max_retries = 0
stream_idle_timeout_ms = 300000

[analytics]
enabled = false
''',
        ROOT / "claude" / "settings.json": json.dumps({
            "model": "local", "permissions": {"defaultMode": "default"},
            "enableAllProjectMcpServers": False,
            "statusLine": {"type": "command", "command": shlex.join([
                sys.executable, str(Path(__file__).with_name("sentinel_statusline.py").resolve()),
                "--root", str(ROOT.resolve()),
            ]), "refreshInterval": 5},
        }, indent=2) + "\n",
    }
    for path, content in configs.items():
        if path.exists():
            # Seed defaults once. Native clients and users own their subsequent
            # preferences; ordinary changes must not block server startup.
            continue
        with path.open("x") as file:
            os.chmod(path, 0o600)
            file.write(content)
    for client, subdirectory in (("claude", "commands"), ("codex", "prompts")):
        directory = ROOT / client / subdirectory
        owned_directory(directory)
        for name, content in command_assets(client, ENDPOINT).items():
            path = directory / name
            if path.exists() or path.is_symlink():
                continue  # Client/user edits remain owned by them.
            fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
            with os.fdopen(fd, "w") as file:
                file.write(content)
    plugin = ROOT / 'control-plugin'
    files = {'.claude-plugin/plugin.json': json.dumps({'name': 'sentinel', 'version': '1.0.0'}) + '\n'}
    for name, content in command_assets('claude', ENDPOINT).items():
        skill = name.removeprefix('sentinel-').removesuffix('.md')
        files[f'skills/{skill}/SKILL.md'] = content
    for relative, content in files.items():
        path = plugin / relative
        owned_directory(path.parent)
        if path.exists() or path.is_symlink():
            continue
        fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
        with os.fdopen(fd, 'w') as file:
            file.write(content)
    return ROOT


def environment(client):
    # Keep the real HOME meaning; client-specific variables own their test state.
    # Do not copy provider keys, proxy variables, plugins or production CLI env.
    env = {key: os.environ[key] for key in ("HOME", "PATH", "TERM", "COLORTERM", "LANG", "LC_ALL") if key in os.environ}
    env["PATH"] = str(ROOT / "clients" / "node_modules" / ".bin") + os.pathsep + env.get("PATH", os.defpath)
    env["TMPDIR"] = str(ROOT / "tmp")
    if client == "codex":
        env.update(CODEX_HOME=str(ROOT / "codex"), SENTINEL_LAB_TOKEN=TOKEN)
    else:
        env.update(CLAUDE_CONFIG_DIR=str(ROOT / "claude"),
                   ANTHROPIC_BASE_URL=ENDPOINT, ANTHROPIC_API_KEY=TOKEN,
                   ANTHROPIC_MODEL="opusplan", ANTHROPIC_DEFAULT_HAIKU_MODEL="sentinel-haiku",
                   ANTHROPIC_DEFAULT_SONNET_MODEL="sentinel-sonnet", ANTHROPIC_DEFAULT_OPUS_MODEL="sentinel-opus",
                   ANTHROPIC_DEFAULT_HAIKU_MODEL_NAME="Haiku",
                   ANTHROPIC_DEFAULT_SONNET_MODEL_NAME="Sonnet",
                   ANTHROPIC_DEFAULT_OPUS_MODEL_NAME="Opus",
                   CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC="1", DISABLE_AUTOUPDATER="1",
                   DISABLE_UPDATES="1", DISABLE_TELEMETRY="1", DISABLE_ERROR_REPORTING="1")
        env.update(MAX_THINKING_TOKENS="0", CLAUDE_CODE_DISABLE_ADAPTIVE_THINKING="1",
                   CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS="1", ENABLE_TOOL_SEARCH="false")
        try:
            selection = json.loads((ROOT / "runtime.json").read_text())
            windows = []
            for key in ("model_path", "small_model_path"):
                if not selection.get(key):
                    continue
                model_config = json.loads((Path(selection[key]) / "config.json").read_text())
                window = model_config.get("max_position_embeddings") or model_config.get("text_config", {}).get("max_position_embeddings")
                if type(window) is int and window > 0:
                    windows.append(window)
            if windows:
                env["CLAUDE_CODE_MAX_CONTEXT_TOKENS"] = str(min(windows))
        except (OSError, ValueError, KeyError):
            pass  # An attached runtime has no local model metadata to declare.
    return env


def client_binary(client):
    binary = ROOT / "clients" / "node_modules" / ".bin" / client
    if not binary.is_file() or not os.access(binary, os.X_OK):
        raise RuntimeError(f"Lab {client} is not installed; run make lab-clients-update. Global installations are not used.")
    return str(binary)


def launch_spec(client):
    binary = client_binary(client)
    command = [binary]
    if client == "claude":
        # The actual Claude UI and tool executor; no print mode or bypass flags.
        # Bare mode isolates hooks, keychain, plugins and ancestor CLAUDE.md.
        agents = {
            "Explore": {"description":"Fast read-only file discovery and code search.",
                        "prompt":"Explore the repository using Read, Glob and Grep. Report concrete file evidence concisely. Do not edit files or execute commands.",
                        "tools":["Read","Glob","Grep"],"model":"haiku"},
            "Plan": {"description":"Read-only research, reasoning and implementation planning.",
                     "prompt":"Read relevant files, reason carefully and propose a concrete implementation plan grounded in repository evidence. Do not modify files or run commands.",
                     "tools":["Read","Glob","Grep"],"model":"opus"},
        }
        command += ["--bare", "--model", "opusplan", "--setting-sources", "user",
                    "--plugin-dir", str(ROOT / "control-plugin"),
                    "--strict-mcp-config", "--mcp-config", '{"mcpServers":{}}',
                    "--tools", "Read,Write,Edit,Bash,Glob,Grep,Agent", "--agents",json.dumps(agents), "--permission-mode", "default"]
    workspace = Path(os.environ.get("LAB_WORKSPACE") or ROOT / "workspace").resolve()
    if not workspace.is_dir():
        raise RuntimeError(f"LAB_WORKSPACE must be an existing directory: {workspace}")
    return command, environment(client), workspace


def update_clients():
    npm = shutil.which("npm")
    if not npm:
        raise RuntimeError("Node.js/npm is required to install the isolated lab clients")
    clients = ROOT / "clients"
    clients.mkdir(mode=0o700, exist_ok=True)
    # npm refuses loading the same path as both user and global configuration.
    # Use two separate empty lab files rather than inheriting production npmrc.
    configs = [ROOT / "npm-user.npmrc", ROOT / "npm-global.npmrc"]
    for config in configs:
        if config.exists() and config.read_text():
            raise RuntimeError(f"Preserving changed npm config: {config}; expected an empty lab config")
        config.touch(mode=0o600)
    env = {key: os.environ[key] for key in ("HOME", "PATH", "LANG", "LC_ALL") if key in os.environ}
    env["TMPDIR"] = str(ROOT / "tmp")
    print("Installing latest stable official clients into the lab only…", flush=True)
    subprocess.run([npm, "install", "--prefix", str(clients), "--registry=https://registry.npmjs.org",
                    f"--userconfig={configs[0]}", f"--globalconfig={configs[1]}", "--cache", str(ROOT / "npm-cache"),
                    "--no-audit", "--no-fund", "--fetch-retries=0", "--fetch-timeout=30000",
                    "@openai/codex@latest", "@anthropic-ai/claude-code@latest"],
                   env=env, cwd=clients, check=True)
    for client in ("codex", "claude"):
        subprocess.run([client_binary(client), "--version"], env=environment(client),
                       cwd=ROOT / "workspace", check=True, timeout=15)


def health():
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    with opener.open(ENDPOINT + "/health", timeout=2) as response:
        return json.load(response)


def ensure_claude_gateway(supervisor=None):
    if supervisor is None:
        import sentinel_runner as supervisor
    restarted = False
    required_roles = roles_configured()
    deadline = time.monotonic() + 10
    while True:
        try:
            state = health()
        except OSError:
            if not supervisor.running(ROOT):
                supervisor.start()
            if time.monotonic() >= deadline:
                raise RuntimeError("Gateway did not become reachable; run make lab-logs")
            time.sleep(.1)
            continue
        capabilities = state.get("capabilities", {})
        if capabilities.get("messages") and capabilities.get("tools") and (not required_roles or capabilities.get("claude_roles")):
            return state
        if restarted or not supervisor.running(ROOT):
            return state
        print("Updating this lab's older gateway to the Claude adapter…", flush=True)
        supervisor.request_stop(ROOT)
        supervisor.start()
        restarted = True


def roles_configured():
    try:
        return bool(json.loads((ROOT / "runtime.json").read_text()).get("small_model_path"))
    except FileNotFoundError:
        return False


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=("prepare", "clients-update", "doctor", "codex", "claude"))
    parser.add_argument("args", nargs=argparse.REMAINDER)
    args = parser.parse_args()
    prepare()
    if args.command == "clients-update":
        update_clients()
        return 0
    if args.command == "prepare":
        print(f"Lab prepared: {ROOT}\nProduction client configuration was not changed.")
        return 0
    if args.command == "doctor":
        clients_ok = True
        for client in ("codex", "claude"):
            try:
                binary = client_binary(client)
            except RuntimeError as error:
                print(error)
                clients_ok = False
                continue
            version = subprocess.run([binary, "--version"], env=environment(client),
                                     cwd=ROOT / "workspace", capture_output=True, text=True, timeout=10)
            print(f"{client}: {version.stdout.strip() or version.stderr.strip()}\n  Lab executable: {binary}")
            clients_ok = clients_ok and version.returncode == 0
        try:
            print("Gateway:", json.dumps(health(), sort_keys=True))
        except Exception as error:
            print(f"Gateway unavailable: {error}")
            return 1
        return 0 if clients_ok else 1
    required = "responses" if args.command == "codex" else "messages"
    # Refuse missing lab clients/extra flags before touching the lab server.
    command, env, workspace = launch_spec(args.command)
    if args.args:
        raise RuntimeError("Lab launch does not accept arbitrary flags yet; preserve its isolation settings")
    state = ensure_claude_gateway() if args.command == "claude" else health()
    capabilities = state.get("capabilities", {})
    if not capabilities.get(required) or not capabilities.get("tools"):
        print(f"Not launching {args.command}: Sentinel needs {required} and tools support. "
              "Current gateway is a plain-text transport proof; no model request was sent.", file=sys.stderr)
        return 2
    if args.command == "claude" and roles_configured() and not capabilities.get("claude_roles"):
        raise RuntimeError("Selected Claude roles require the new gateway; run make lab-rebuild. An unowned gateway was not restarted.")
    print(f"Opening real {args.command} interactively in {workspace}\n"
          "Backend: Sentinel → MLX-Flash → your local model. Tool permissions remain enabled.\n"
          "Models: Claude roles map internally in Sentinel; no trained Jes checkpoint is connected.", flush=True)
    os.chdir(workspace)
    os.execve(command[0], command, env)


if __name__ == "__main__":
    try:
        sys.exit(main())
    except (OSError, RuntimeError, ValueError, subprocess.SubprocessError) as error:
        print(f"Sentinel lab: {error}", file=sys.stderr)
        sys.exit(1)
