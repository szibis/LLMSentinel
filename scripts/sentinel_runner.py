#!/usr/bin/env python3
"""Manage only this isolated lab's gateway and explicitly selected runtime."""
import argparse
import errno
import fcntl
import json
import os
from pathlib import Path
import re
import shutil
import signal
import socket
import subprocess
import sys
import time
import urllib.error
import urllib.request
import uuid

import sentinel_lab as lab

PROJECT = Path(__file__).resolve().parents[1]


class LabLock:
    def __init__(self, root):
        root.mkdir(parents=True, exist_ok=True, mode=0o700)
        self.file = (root / "run.lock").open("a")

    def __enter__(self):
        try:
            fcntl.flock(self.file, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            self.file.close()
            raise RuntimeError("Lab already running; use make lab-rebuild or make lab-stop")
        return self

    def __exit__(self, *args):
        self.file.close()


def running(root):
    try:
        with LabLock(root):
            return False
    except RuntimeError:
        return True


def write_state(root, state):
    temporary = root / "state.tmp"
    with temporary.open("w") as file:
        os.chmod(temporary, 0o600)
        json.dump(state, file, indent=2)
    temporary.replace(root / "state.json")


def request_stop(root, wait=True):
    if not running(root):
        print("No owned lab is running.")
        return False
    # Lock acquisition precedes state publication. Wait for this run rather than
    # sending a marker for a stale, stopped run during startup.
    deadline = time.monotonic() + 2
    while True:
        if not running(root):
            print("Owned lab already stopped.")
            return False
        try:
            state = json.loads((root / "state.json").read_text())
        except FileNotFoundError:
            state = {}
        if state.get("phase") in ("starting", "running", "stopping"):
            break
        if time.monotonic() >= deadline:
            raise RuntimeError("Lab is still preparing; retry make lab-stop shortly")
        time.sleep(.05)
    if not re.fullmatch(r"[A-Za-z0-9_-]+", str(state.get("run_id", ""))):
        raise RuntimeError("Invalid lab ownership state; no stop request was sent")
    marker = root / ("stop-" + state["run_id"])
    marker.touch(mode=0o600)
    print("Stopping owned lab processes…", flush=True)
    if wait:
        deadline = time.monotonic() + 15
        while running(root):
            if time.monotonic() > deadline:
                raise RuntimeError("Lab did not stop within 15s; inspect logs. No unrelated process was signalled.")
            time.sleep(.1)
    return True


def stop_process(process):
    # The owned leader may have exited while descendants retain its group.
    # Signal that exact group even then; never search or kill by process name.
    try:
        os.killpg(process.pid, signal.SIGTERM)
    except ProcessLookupError:
        return
    except PermissionError:
        if process.poll() is not None:
            return
        raise
    deadline = time.monotonic() + 4
    while True:
        process.poll()
        try:
            os.killpg(process.pid, 0)
        except ProcessLookupError:
            break
        except PermissionError as error:
            # An exited group can no longer be probed in a restricted runner.
            # Do not escalate signals without permission to verify ownership.
            if process.poll() is not None:
                return
            try:
                process.wait(timeout=max(.01, deadline - time.monotonic()))
            except subprocess.TimeoutExpired:
                raise error
            return
        if time.monotonic() >= deadline:
            try:
                os.killpg(process.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            break
        time.sleep(.05)
    process.wait(timeout=3)


def validate_model(value):
    model = Path(value)
    if not model.is_absolute():
        raise RuntimeError("MODEL_PATH must be an absolute cached model directory, not a Hub ID")
    for name in ("config.json", "tokenizer.json"):
        file = model / name
        if not file.is_file() or file.stat().st_size == 0:
            raise RuntimeError(f"Missing or empty {name}: {model}")
    weights = list(model.glob("*.safetensors"))
    if not weights or any(file.stat().st_size == 0 for file in weights):
        raise RuntimeError(f"Missing or empty model weights: {model}")
    index = model / "model.safetensors.index.json"
    if index.exists():
        shards = set(json.loads(index.read_text()).get("weight_map", {}).values())
        if not shards:
            raise RuntimeError("Model index has no weight_map")
        for shard in shards:
            if Path(shard).name != shard or not (model / shard).is_file() or (model / shard).stat().st_size == 0:
                raise RuntimeError(f"Model has missing weight shard: {shard}")
    elif any("-of-" in file.name for file in weights):
        raise RuntimeError("Sharded model needs model.safetensors.index.json to verify completeness")
    return model


def runtime_plan(settings):
    if not settings:
        return []
    models = [("large", 19091, validate_model(settings["model_path"]))]
    if settings.get("small_model_path"):
        models.append(("small", 19092, validate_model(settings["small_model_path"])))
        for _, _, model in models:
            validate_role_model(str(model))
    executable = shutil.which(settings["executable"])
    if not executable:
        raise RuntimeError("MLX-Flash not installed; set MLX_FLASH_BIN to an existing executable. No installation attempted.")
    executable = str(Path(executable).resolve())
    settings["executable"] = executable
    plan = []
    for name, port, model in models:
        prefix = [executable]
        if len(models) == 2:
            python = Path(executable).parent / "python"
            if not python.is_file() or not os.access(python, os.X_OK):
                raise RuntimeError("Role profiles require MLX-Flash's Python environment beside its executable")
            prefix = [str(python), str(PROJECT / "scripts" / "mlx_flash_roles.py")]
        command = prefix + ["--model", str(model), "--host", "127.0.0.1", "--port", str(port), "--speculative", "none"]
        plan.append({"name":name,"port":port,"model_path":str(model),"command":command})
    return plan


def validate_role_model(value):
    model = validate_model(value)
    metadata = model / "tokenizer_config.json"
    if not metadata.is_file() or metadata.stat().st_size == 0:
        raise RuntimeError(f"Missing tokenizer_config.json for role model: {model}")
    try:
        config = json.loads(metadata.read_text())
    except ValueError as error:
        raise RuntimeError(f"Invalid tokenizer_config.json: {model}") from error
    template_path = model / "chat_template.jinja"
    template = template_path.read_text() if template_path.is_file() else config.get("chat_template")
    if not isinstance(template, str) or "enable_thinking" not in template:
        raise RuntimeError(f"Role model needs a Qwen thinking chat template: {model}")
    return model


def gateway_command(binary, settings):
    command = [str(binary)]
    if settings.get("small_model_path"):
        command += ["--role-haiku-upstream","http://127.0.0.1:19092/v1",
                    "--role-sonnet-upstream","http://127.0.0.1:19091/v1",
                    "--role-opus-upstream","http://127.0.0.1:19091/v1"]
    return command


def fetch(path, timeout=2):
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    with opener.open(lab.ENDPOINT + path, timeout=timeout) as response:
        return json.load(response)


def spawn(command, log_path, env):
    with log_path.open("a") as log:
        log.write(f"\n--- Lab start {time.strftime('%Y-%m-%d %H:%M:%S')} ---\n")
        log.flush()
        return subprocess.Popen(command, cwd=lab.ROOT / "workspace", env=env,
                                stdin=subprocess.DEVNULL, stdout=log, stderr=log, start_new_session=True)


def preflight_ports(ports, timeout=3):
    """Allow TCP reuse after shutdown, but never replace an active listener."""
    deadline = time.monotonic() + timeout
    while True:
        probes = []
        failure = None
        try:
            for port in ports:
                sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
                probes.append(sock)
                sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
                sock.bind(("127.0.0.1", port))
                # Binding alone can admit duplicate addresses on some systems.
                # Never use SO_REUSEPORT: an existing server must stay exclusive.
                sock.listen(1)
            return
        except OSError as error:
            failure = error
        finally:
            for sock in probes:
                sock.close()
        if failure.errno == errno.EADDRINUSE and time.monotonic() < deadline:
            time.sleep(.1)
            continue
        if failure.errno == errno.EADDRINUSE:
            reason = "is still in use after waiting for shutdown"
        elif failure.errno in (errno.EPERM, errno.EACCES):
            reason = "could not be checked: permission denied by the OS or sandbox"
        else:
            reason = "could not be checked"
        raise RuntimeError(f"Lab port {port} {reason}: {failure}; existing services were not stopped") from failure


def start(command=None):
    """Detach an owned supervisor and acknowledge its specific startup attempt."""
    root = lab.prepare()
    # Serialize starts separately from the supervisor's lifetime ownership lock.
    with LabLock(root / "control"):
        if running(root):
            print("Lab already running. Use make lab-status or make lab-rebuild.")
            return None
        start_id = uuid.uuid4().hex
        env = {key: os.environ[key] for key in
               ("HOME", "PATH", "LANG", "LC_ALL", "MODEL_PATH", "SMALL_MODEL_PATH", "MLX_FLASH_BIN", "ATTACH_RUNTIME")
               if key in os.environ}
        env["SENTINEL_LAB_START_ID"] = start_id
        log_path = root / "supervisor.log"
        with log_path.open("ab") as log:
            os.chmod(log_path, 0o600)
            log_offset = log.tell()
            log.write(f"\n--- Lab attempt {start_id} {time.strftime('%Y-%m-%d %H:%M:%S')} ---\n".encode())
            log.flush()
            process = subprocess.Popen(command or [sys.executable, str(Path(__file__).resolve()), "run"],
                                       cwd=PROJECT, env=env, stdin=subprocess.DEVNULL,
                                       stdout=log, stderr=log, start_new_session=True)
        deadline = time.monotonic() + 10
        acknowledged = None
        try:
            while time.monotonic() < deadline:
                if process.poll() is not None:
                    with log_path.open("rb") as log:
                        log.seek(max(log_offset, log_path.stat().st_size - 3000))
                        details = log.read().decode("utf-8", errors="replace")
                    raise RuntimeError(f"Lab startup failed ({process.returncode}):\n{details}\nSee make lab-logs")
                path = root / "state.json"
                if path.exists():
                    state = json.loads(path.read_text())
                    if state.get("start_id") == start_id and state.get("phase") == "running":
                        acknowledged = acknowledged or time.monotonic()
                        # Catch immediate child crashes before returning success.
                        if time.monotonic() - acknowledged >= .5:
                            print(f"Lab server started in background: {lab.ENDPOINT}\n"
                                  "Your terminal is free. Use make lab-status, lab-ask, lab-logs or lab-stop.\n"
                                  "Model readiness is separate; inspect status before generation.", flush=True)
                            return process
                time.sleep(.05)
            raise RuntimeError("Lab startup timed out; inspect make lab-logs")
        except BaseException:
            if process.poll() is None:
                # Signal only the supervisor we just created; it owns cleanup.
                process.terminate()
                try:
                    process.wait(timeout=15)
                except subprocess.TimeoutExpired:
                    stop_process(process)
            raise


def run():
    root = lab.prepare()
    with LabLock(root):
        binary = PROJECT / "bin" / "sentinel-gateway"
        if not binary.is_file():
            raise RuntimeError("Missing gateway binary; run make lab-run to build first")
        settings_path = root / "runtime.json"
        settings = json.loads(settings_path.read_text()) if settings_path.exists() else {}
        if os.environ.get("MODEL_PATH"):
            settings = {"model_path": os.environ["MODEL_PATH"], "executable": os.environ.get("MLX_FLASH_BIN") or "mlx-flash"}
            if os.environ.get("SMALL_MODEL_PATH"):
                settings["small_model_path"] = os.environ["SMALL_MODEL_PATH"]
        elif os.environ.get("SMALL_MODEL_PATH"):
            settings["small_model_path"] = os.environ["SMALL_MODEL_PATH"]
        if os.environ.get("ATTACH_RUNTIME") == "1":
            settings = {}
        plan = runtime_plan(settings)
        # Refuse occupied lab ports before starting any child. Never attach silently.
        ports = [19090] + [item["port"] for item in plan]
        preflight_ports(ports)
        if plan:
            with settings_path.open("w") as file:
                os.chmod(settings_path, 0o600)
                json.dump(settings, file, indent=2)
        state = {"run_id": uuid.uuid4().hex, "phase": "starting", "model_path": settings.get("model_path"),
                 "start_id": os.environ.get("SENTINEL_LAB_START_ID"),
                 "runtime": "owned" if plan else "attached", "gateway": lab.ENDPOINT,
                 "upstream": "http://127.0.0.1:19091/v1"}
        state["models"] = [{key:item[key] for key in ("name","port","model_path")} for item in plan]
        state["roles"] = {"haiku":"small","sonnet":"large","opus":"large-thinking"} if settings.get("small_model_path") else {}
        write_state(root, state)
        processes = []
        stopping = False

        def stop_signal(*args):
            nonlocal stopping
            stopping = True

        old_handlers = {sig: signal.signal(sig, stop_signal) for sig in (signal.SIGINT, signal.SIGTERM)}
        env = {key: os.environ[key] for key in ("HOME", "PATH", "LANG", "LC_ALL") if key in os.environ}
        env.update(TMPDIR=str(root / "tmp"), HF_HUB_OFFLINE="1", TRANSFORMERS_OFFLINE="1")
        try:
            for item in plan:
                log = "runtime.log" if item["name"] == "large" else "runtime-small.log"
                processes.append(spawn(item["command"], root / log, env))
            processes.append(spawn(gateway_command(binary,settings), root / "gateway.log", env))
            print(f"Lab processes started: {lab.ENDPOINT}\nRuntime: {state['runtime']} on port 19091\n"
                  f"Logs: {root}\nCtrl+C / make lab-stop to stop; make lab-status to inspect.\n"
                  "Run make lab-status to check readiness before sending a prompt.\n"
                  "Claude Messages/tool bridge is experimental; Codex Responses is pending.", flush=True)
            state["phase"] = "running"
            write_state(root, state)
            while not stopping and not (root / ("stop-" + state["run_id"])).exists():
                for process in processes:
                    if process.poll() is not None:
                        raise RuntimeError(f"Owned lab process exited ({process.returncode}); run make lab-logs")
                time.sleep(.2)
        finally:
            state["phase"] = "stopping"
            write_state(root, state)
            for process in reversed(processes):
                stop_process(process)
            for sig, handler in old_handlers.items():
                signal.signal(sig, handler)
            (root / ("stop-" + state["run_id"])).unlink(missing_ok=True)
            state["phase"] = "stopped"
            write_state(root, state)
            print("Owned lab stopped. Attached services were left running.", flush=True)


def status():
    active = running(lab.ROOT)
    print("Owned lab:", "running" if active else "stopped")
    path = lab.ROOT / "state.json"
    if path.exists():
        print(json.dumps(json.loads(path.read_text()), indent=2))
    try:
        print("Gateway:", json.dumps(fetch("/health"), sort_keys=True))
        try:
            print("Runtime:", json.dumps(fetch("/sentinel/status", timeout=3), sort_keys=True))
        except OSError as error:
            print("Runtime health unavailable:", error)
    except OSError as error:
        print("Gateway unavailable:", error)
        return 1
    return 0


def ask():
    prompt = os.environ.get("PROMPT") or "Reply with a short greeting."
    data = json.dumps({"model": os.environ.get("MODEL_ROLE") or "local", "messages": [{"role": "user", "content": prompt}], "stream": True, "max_tokens": 512}).encode()
    request = urllib.request.Request(lab.ENDPOINT + "/v1/chat/completions", data=data, headers={"Content-Type": "application/json"})
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    complete = False
    print("Sending to the local model; first output may wait for model loading and buffered generation.", file=sys.stderr)
    with opener.open(request, timeout=300) as response:
        for line in response:
            if not line.startswith(b"data:"):
                continue
            body = line[5:].strip()
            if body == b"[DONE]":
                complete = True; break
            if not body:
                continue
            event = json.loads(body)
            if event.get("error"):
                raise RuntimeError("Runtime reported a generation error")
            for choice in event.get("choices", [])[:1]:
                text = choice.get("delta", {}).get("content", "")
                print("".join(char for char in text if char.isprintable() or char in "\n\t"), end="", flush=True)
                reason = choice.get("finish_reason")
                if reason == "stop":
                    complete = True
                elif reason:
                    raise RuntimeError(f"Incomplete response: {reason}")
    print()
    if not complete:
        raise RuntimeError("Stream ended without completion; output is partial")


def logs(follow):
    paths = [lab.ROOT / "supervisor.log", lab.ROOT / "gateway.log", lab.ROOT / "runtime.log", lab.ROOT / "runtime-small.log"]
    offsets = {}
    for path in paths:
        print(f"--- {path.name} ---")
        if path.exists():
            with path.open("rb") as file:
                file.seek(max(0, path.stat().st_size - 16384))
                print(file.read().decode(errors="replace"), end="")
                offsets[path] = file.tell()
        else:
            print("No log yet."); offsets[path] = 0
    while follow:
        for path in paths:
            if path.exists():
                with path.open("rb") as file:
                    file.seek(offsets[path])
                    text = file.read().decode(errors="replace")
                    if text:
                        print(f"[{path.name}] {text}", end="", flush=True)
                    offsets[path] = file.tell()
        time.sleep(.3)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=("start", "run", "restart", "stop", "status", "logs", "ask"))
    parser.add_argument("--follow", action="store_true")
    args = parser.parse_args()
    if args.command == "start":
        start()
    elif args.command == "restart":
        request_stop(lab.ROOT)
        start()
    elif args.command == "run":
        run()
    elif args.command == "stop":
        request_stop(lab.ROOT)
    elif args.command == "status":
        return status()
    elif args.command == "logs":
        logs(args.follow)
    else:
        ask()
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        sys.exit(0)
    except urllib.error.HTTPError as error:
        print(f"Lab HTTP {error.code}: {error.read(8192).decode(errors='replace')}", file=sys.stderr)
        sys.exit(1)
    except (OSError, RuntimeError, ValueError, subprocess.SubprocessError) as error:
        print(f"Sentinel lab: {error}", file=sys.stderr)
        sys.exit(1)
