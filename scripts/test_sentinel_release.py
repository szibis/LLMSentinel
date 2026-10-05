"""Run the release workflow's shell decisions against local command doubles."""
import os
from pathlib import Path
import subprocess
import tempfile
import textwrap
import unittest

WORKFLOW = Path(__file__).resolve().parents[1] / ".github/workflows/auto-release.yml"


def workflow_script(name, workflow=WORKFLOW):
    lines = workflow.read_text().splitlines()
    start = lines.index("      - name: " + name)
    run = next(i for i in range(start, len(lines)) if lines[i] == "        run: |")
    end = run + 1
    while end < len(lines) and (not lines[end] or lines[end].startswith("          ")):
        end += 1
    return textwrap.dedent("\n".join(lines[run + 1:end]))


class ReleaseDecisionTests(unittest.TestCase):
    def run_script(self, name, workflow=WORKFLOW, **settings):
        with tempfile.TemporaryDirectory() as directory:
            temp = Path(directory)
            doubles = {
                "docker": '#!/bin/sh\nprintf "docker %s\\n" "$*" >> "$MOCK_CALLS"\n',
                "gh": '''#!/bin/sh
case "$*" in
  *branches/main*) printf '%s\\n' "$MOCK_MAIN" ;;
  *matching-refs/tags/v*) printf '%s\\n' "$MOCK_LATEST" ;;
  release*) printf 'gh %s\\n' "$*" >> "$MOCK_CALLS" ;;
  *) printf '%s\\n' "$MOCK_TITLE" ;;
esac
''',
                "git": '''#!/bin/sh
case "$*" in
  "tag --points-at HEAD") printf '%s\\n' "$MOCK_EXISTING" ;;
  "tag --list v* --sort=-version:refname") printf '%s\\n' "$MOCK_LATEST" ;;
  *) printf '%s\\n' "$*" >> "$MOCK_CALLS" ;;
esac
''',
            }
            for command, source in doubles.items():
                executable = temp / command
                executable.write_text(source)
                executable.chmod(0o755)
            output = temp / "outputs"
            output.touch()
            calls = temp / "calls"
            env = dict(os.environ, PATH=str(temp) + os.pathsep + os.environ["PATH"],
                       GITHUB_OUTPUT=str(output), MOCK_CALLS=str(calls),
                       REPOSITORY="example/Sentinel", COMMIT_SHA="abc", **settings)
            subprocess.run(["bash", "-e", "-o", "pipefail", "-c", workflow_script(name, workflow)],
                           env=env, check=True, capture_output=True, text=True)
            values = dict(line.split("=", 1) for line in output.read_text().splitlines())
            return values, calls.read_text() if calls.exists() else ""

    def test_nonrelease_titles_and_missing_pr(self):
        for title in ("", "docs: improve setup", "deps: update library"):
            with self.subTest(title=title):
                outputs, calls = self.run_script("Select release-worthy merged PR", MOCK_TITLE=title)
                self.assertEqual(outputs, {"bump": "none"})
                self.assertEqual(calls, "")

    def test_stale_build_cannot_start_release(self):
        for sha, expected in (("abc", "true"), ("newer", "false")):
            with self.subTest(main=sha):
                outputs, _ = self.run_script("Check current main", MOCK_MAIN=sha)
                self.assertEqual(outputs, {"current": expected})

    def test_title_selects_semantic_bump(self):
        for title, bump in (("fix: release build", "patch"), ("feat: Qwen roles", "minor"),
                            ("feat!: change API", "major")):
            with self.subTest(title=title):
                outputs, _ = self.run_script("Select release-worthy merged PR", MOCK_TITLE=title)
                self.assertEqual(outputs["bump"], bump)

    def test_pr_title_is_data(self):
        outputs, _ = self.run_script("Select release-worthy merged PR",
                                     MOCK_TITLE='fix: $(exit 77) "quotes"')
        self.assertEqual(outputs["bump"], "patch")

    def test_versions_and_first_release(self):
        for bump, current, expected in (("patch", "v4.2.9", "v4.2.10"),
                                         ("minor", "v4.2.9", "v4.3.0"),
                                         ("major", "v4.2.9", "v5.0.0"),
                                         ("patch", "", "v0.0.1")):
            with self.subTest(bump=bump, current=current):
                outputs, calls = self.run_script("Calculate and create version tag", BUMP=bump,
                                                 MOCK_EXISTING="", MOCK_LATEST=current)
                self.assertEqual(outputs, {"version": expected, "release": "true"})
                self.assertIn("push origin " + expected, calls)

    def test_retry_reuses_commit_tag(self):
        outputs, calls = self.run_script("Calculate and create version tag", BUMP="patch",
                                         MOCK_EXISTING="v4.2.9", MOCK_LATEST="v4.2.9")
        self.assertEqual(outputs, {"version": "v4.2.9", "release": "true"})
        self.assertEqual(calls, "")

    def test_older_release_never_overwrites_latest(self):
        workflow = WORKFLOW.with_name("release.yml")
        for version, promoted in (("v4.2.9", False), ("v4.3.0", True)):
            with self.subTest(version=version):
                _, calls = self.run_script("Promote newest semantic version to latest", workflow=workflow,
                                           VERSION=version, IMAGE="ghcr.io/example/sentinel",
                                           MOCK_LATEST="v4.3.0")
                self.assertEqual("docker buildx imagetools create" in calls, promoted)

    def test_older_github_release_does_not_become_latest(self):
        workflow = WORKFLOW.with_name("release.yml")
        for version, promoted in (("v4.2.9", False), ("v4.3.0", True)):
            with self.subTest(version=version):
                _, calls = self.run_script("Publish release binaries", workflow=workflow,
                                           VERSION=version, MOCK_LATEST="v4.3.0")
                self.assertEqual("gh release edit" in calls, promoted)


if __name__ == "__main__":
    unittest.main()
