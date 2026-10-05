#!/usr/bin/env python3
"""Opt-in real-generation smoke for all three Claude lab roles."""
import json
import sys
import time
import urllib.error
import urllib.request

import sentinel_lab as lab


def main():
    if not lab.health().get("capabilities", {}).get("claude_roles"):
        raise RuntimeError("Three-role gateway is not running; run make lab-rebuild first")
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    print("Generating with real local models; first requests load weights into RAM.", flush=True)
    for role, budget in (("haiku", 256), ("sonnet", 256), ("opus", 8192)):
        marker = "QWEN_ROLE_READY"
        payload = {"model": "sentinel-" + role, "max_tokens": budget,
                   "messages": [{"role": "user", "content": "Reply with exactly " + marker + " and no other final text."}]}
        request = urllib.request.Request(lab.ENDPOINT + "/v1/messages", data=json.dumps(payload).encode(),
                                         headers={"Content-Type": "application/json"})
        started = time.monotonic()
        with opener.open(request, timeout=600) as response:
            result = json.load(response)
        text = "".join(block.get("text", "") for block in result.get("content", []) if block.get("type") == "text")
        if text.strip() != marker or result.get("stop_reason") != "end_turn":
            raise RuntimeError(f"{role}: native generation did not pass the exact-answer smoke: {text!r}")
        print(f"{role}: real generation passed ({time.monotonic()-started:.1f}s), usage={result.get('usage')}", flush=True)
    print("This verifies role generation, not coding quality or tool reliability.")


if __name__ == "__main__":
    try:
        main()
    except urllib.error.HTTPError as error:
        print(f"Native role check HTTP {error.code}: {error.read(8192).decode(errors='replace')}", file=sys.stderr)
        sys.exit(1)
    except (OSError, RuntimeError, ValueError) as error:
        print(f"Native role check: {error}", file=sys.stderr)
        sys.exit(1)
