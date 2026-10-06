#!/usr/bin/env python3
"""Read-only local telemetry for the isolated Claude lab's native status line."""
import argparse
from concurrent.futures import ThreadPoolExecutor
import json
import os
from pathlib import Path
import re
import shlex
import sys
import tempfile
import time
import urllib.request

ROOT = Path(__file__).resolve().parents[1] / ".sentinel-lab"


def read_json(path):
    try:
        value = json.loads(path.read_text())
        return value if isinstance(value, dict) else {}
    except (OSError, ValueError):
        return {}


def atomic_json(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    fd, temporary = tempfile.mkstemp(dir=path.parent, prefix=".status-")
    try:
        with os.fdopen(fd, "w") as file:
            json.dump(value, file)
            file.write("\n")
        os.replace(temporary, path)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def install(root=ROOT):
    path = root / "claude" / "settings.json"
    settings = json.loads(path.read_text())
    if "statusLine" not in settings:
        command = shlex.join([sys.executable, str(Path(__file__).resolve()), "--root", str(root.resolve())])
        settings["statusLine"] = {"type": "command", "command": command, "refreshInterval": 5}
        atomic_json(path, settings)


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def fetch(url):
    try:
        # Bypass inherited proxies and reject redirects. Never send credentials.
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect())
        with opener.open(url, timeout=.6) as response:
            body = response.read(65537)
        if len(body) > 65536:
            return None
        value = json.loads(body)
        return value if isinstance(value, dict) else None
    except (OSError, ValueError):
        return None


def tail(path):
    try:
        with path.open("rb") as file:
            file.seek(0, 2)
            file.seek(max(0, file.tell() - 65536))
            return file.read().decode("utf-8", errors="replace").rsplit("--- Lab start ", 1)[-1]
    except OSError:
        return ""


def collect(root):
    state = read_json(root / "state.json")
    endpoints = {"gateway": "http://127.0.0.1:19090/health"}
    models = state.get("models") or [{"name": "large", "port": 19091}]
    for model in models:
        name, port = model.get("name"), model.get("port")
        if name in ("large", "small") and type(port) is int and 1 <= port <= 65535:
            endpoints[name] = f"http://127.0.0.1:{port}/status"
    with ThreadPoolExecutor(max_workers=3) as executor:
        values = dict(zip(endpoints, executor.map(fetch, endpoints.values())))
    gateway = values.pop("gateway")
    snapshot = {"sample_time": time.time(), "run_id": state.get("run_id"),
                "gateway": bool(gateway and gateway.get("status") == "ok"), "runtimes": values}
    log = tail(root / "gateway.log")
    snapshot["recent_corrections"] = log.count("attempting one format correction")
    snapshot["recent_errors"] = len(re.findall(r"Claude adapter HTTP [45]\d\d:", log))
    snapshot["last_speed"] = {}
    for name in values:
        log_name = "runtime-small.log" if name == "small" else "runtime.log"
        matches = re.findall(r"Inference complete tokens=(\d+) tok_per_s=([\d.]+)", tail(root / log_name))
        if matches:
            snapshot["last_speed"][name] = float(matches[-1][1])
    return snapshot


def add_rates(current, previous):
    current["rates"] = {}
    elapsed = current.get("sample_time", 0) - previous.get("sample_time", 0)
    if not 0 < elapsed <= 30 or current.get("run_id") != previous.get("run_id"):
        return
    tokens = requests = 0
    runtimes = current.get("runtimes", {})
    if not runtimes or set(runtimes) != set(previous.get("runtimes", {})):
        return
    for name, runtime in runtimes.items():
        old = previous.get("runtimes", {}).get(name)
        if not runtime or not old:
            return
        now_stats, old_stats = runtime.get("stats", {}), old.get("stats", {})
        required = ("uptime_s", "tokens_generated", "requests")
        if not all(isinstance(stats.get(key), (int, float)) for stats in (now_stats, old_stats) for key in required):
            return
        if any(now_stats[key] < old_stats[key] for key in required):
            return
        tokens += now_stats["tokens_generated"] - old_stats["tokens_generated"]
        requests += now_stats["requests"] - old_stats["requests"]
    current["rates"] = {"tokens_per_s": tokens / elapsed, "requests_per_min": requests * 60 / elapsed}


def clean(value):
    return re.sub(r"[^\w .%+:/-]", "", str(value))[:80]


def model_name(value):
    value = str(value)
    if "models--" in value:
        value = value.split("models--", 1)[1].split("/", 1)[0].split("--")[-1]
    else:
        value = Path(value).name
    return clean(value)


def render(session, snapshot):
    model = session.get("model") or {}
    label = clean(model.get("display_name") or model.get("id") or "Claude")
    label = label.replace("sentinel-sonnet", "Sonnet").replace("sentinel-haiku", "Haiku").replace("sentinel-opus", "Opus")
    context = (session.get("context_window") or {}).get("used_percentage")
    first = ["LAB", label, "Sentinel online" if snapshot.get("gateway") else "gateway offline"]
    if isinstance(context, (int, float)):
        first.append(f"ctx {context:.0f}%")
    runtimes = snapshot.get("runtimes", {})
    ready = []
    for name, runtime in runtimes.items():
        if not runtime:
            first.append(f"{name} unavailable")
        elif not runtime.get("model_loaded"):
            first.append(f"{name} loading")
        else:
            artifact = model_name(runtime.get("model", ""))
            first.append(f"{name} {artifact} ready" if artifact else f"{name} ready")
            ready.append(runtime)
    second = []
    if ready and len(ready) == len(runtimes) and all("requests" in r.get("stats", {}) and "tokens_generated" in r.get("stats", {}) for r in ready):
        second += [f"{sum(r['stats']['requests'] for r in ready)} MLX req",
                   f"{sum(r['stats']['tokens_generated'] for r in ready)} tok"]
    rates = snapshot.get("rates", {})
    if "tokens_per_s" in rates:
        second += [f"{rates['tokens_per_s']:.1f} tok/s interval", f"{rates['requests_per_min']:.1f} req/min"]
    for name, speed in snapshot.get("last_speed", {}).items():
        second.append(f"{name} last {speed:.1f} tok/s")
    # Both models report the same machine's memory; display one snapshot.
    memory = next((r.get("memory") for r in runtimes.values() if r and r.get("memory")), {})
    third = []
    if memory:
        for key, suffix in (("available_gb", "GB available"), ("swap_used_gb", "GB swap")):
            if key in memory:
                third.append(f"{clean(memory[key])} {suffix}")
        third.append("pressure " + clean(memory.get("pressure", "unknown")))
    if "recent_corrections" in snapshot:
        third.append(f"Format corrections {snapshot['recent_corrections']} / errors {snapshot['recent_errors']} (log tail)")
    return "\n".join(" | ".join(row) for row in (first, second, third) if row)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=ROOT)
    parser.add_argument("--install", action="store_true")
    parser.add_argument("--json", action="store_true")
    args = parser.parse_args()
    if args.install:
        install(args.root)
        return
    session = {}
    if not args.json:
        try:
            session = json.loads(sys.stdin.read(65536))
            if not isinstance(session, dict):
                session = {}
        except ValueError:
            pass
    cache = args.root / "tmp" / "statusline.json"
    previous = read_json(cache)
    if 0 <= time.time() - previous.get("sample_time", 0) < 4:
        snapshot = previous
    else:
        snapshot = collect(args.root)
        add_rates(snapshot, previous)
        try:
            atomic_json(cache, snapshot)
        except OSError:
            pass
    print(json.dumps(snapshot) if args.json else render(session, snapshot))


if __name__ == "__main__":
    main()
