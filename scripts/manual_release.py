"""Resolve a manual release to an immutable, successfully built main commit."""
import argparse
import json
import os
import re
import subprocess
from urllib.parse import quote


def version_key(version):
    if not re.fullmatch(r"v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)", version):
        raise ValueError("Expected a stable semantic version such as v3.1.0")
    return tuple(int(part) for part in version[1:].split("."))


def api(path, **fields):
    command = ["gh", "api", path]
    if fields:
        command.extend(["--method", "POST"])
        for name, value in fields.items():
            command.extend(["-f", f"{name}={value}"])
    return json.loads(subprocess.check_output(command, text=True))


def publish_metadata(repository, version, ref):
    requested = version_key(version)
    prefix = f"repos/{repository}"
    sha = api(f"{prefix}/commits/{quote(ref, safe='')}")["sha"]
    tags = api(f"{prefix}/git/matching-refs/tags/v")
    existing = next((tag for tag in tags if tag["ref"] == "refs/tags/" + version), None)
    if existing:
        target = existing["object"]
        while target["type"] == "tag":
            target = api(f"{prefix}/git/tags/{target['sha']}")["object"]
        if target["sha"] != sha:
            raise RuntimeError("Existing release tag points to a different commit; tags never move")
    else:
        previous = []
        for tag in tags:
            try:
                previous.append(version_key(tag["ref"].removeprefix("refs/tags/")))
            except ValueError:
                continue
        if previous and requested <= max(previous):
            raise ValueError("A new release version must exceed existing stable tags")
    runs = api(f"{prefix}/actions/runs?head_sha={sha}&event=push&per_page=100")["workflow_runs"]
    builds = [run for run in runs if run.get("name") == "Build" and run.get("event") == "push"
              and run.get("head_branch") == "main" and run.get("head_sha") == sha]
    if not builds or builds[0].get("status") != "completed" or builds[0].get("conclusion") != "success":
        raise RuntimeError("The selected commit must have a successful completed Build on main")
    if not existing:
        api(f"{prefix}/git/refs", ref="refs/tags/" + version, sha=sha)
    return {"version": version, "ref": sha}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repository", required=True)
    parser.add_argument("--version", required=True)
    parser.add_argument("--ref", default="main")
    args = parser.parse_args()
    result = publish_metadata(args.repository, args.version, args.ref)
    with open(os.environ["GITHUB_OUTPUT"], "a") as output:
        for key, value in result.items():
            print(f"{key}={value}", file=output)
    print(f"Publishing {result['version']} from tested commit {result['ref']}")


if __name__ == "__main__":
    main()
