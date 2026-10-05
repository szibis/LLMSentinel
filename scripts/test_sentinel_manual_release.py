"""Verify that manual publishing cannot tag an untested or conflicting commit."""
import unittest
from unittest.mock import patch

import manual_release as release


class ManualReleaseTests(unittest.TestCase):
    def test_semver_is_strict(self):
        self.assertEqual(release.version_key("v3.1.0"), (3, 1, 0))
        for value in ("3.1.0", "v03.1.0", "v3.1.0-rc1", "v3.1.0\n"):
            with self.subTest(value=value), self.assertRaises(ValueError):
                release.version_key(value)

    def prepare(self, version="v3.1.0", existing=None, runs=None):
        calls = []
        def api(path, **fields):
            calls.append((path, fields))
            if path.endswith("commits/main"):
                return {"sha": "a" * 40}
            if "matching-refs" in path:
                return [{"ref": "refs/tags/v3.0.0", "object": {"sha": "old", "type": "commit"}}] + (existing or [])
            if "actions/runs" in path:
                return {"workflow_runs": runs if runs is not None else [
                    {"name": "Build", "event": "push", "head_branch": "main", "head_sha": "a" * 40,
                     "status": "completed", "conclusion": "success"}]}
            if path.endswith("git/refs"):
                return {}
            raise AssertionError(path)
        with patch.object(release, "api", side_effect=api):
            return release.publish_metadata("example/Sentinel", version, "main"), calls

    def test_tested_sha_is_used_for_both_tag_and_publisher(self):
        result, calls = self.prepare()
        self.assertEqual(result, {"version": "v3.1.0", "ref": "a" * 40})
        self.assertEqual(calls[-1][1], {"ref": "refs/tags/v3.1.0", "sha": "a" * 40})

    def test_missing_or_failed_build_cannot_create_tag(self):
        for runs in ([], [{"name": "Build", "status": "completed", "conclusion": "failure"}]):
            with self.subTest(runs=runs), self.assertRaises(RuntimeError):
                self.prepare(runs=runs)

    def test_existing_matching_tag_can_be_republished_without_mutation(self):
        _, calls = self.prepare(existing=[{"ref": "refs/tags/v3.1.0", "object": {"sha": "a" * 40, "type": "commit"}}])
        self.assertFalse(any(fields for _, fields in calls))

    def test_existing_tag_cannot_move(self):
        with self.assertRaises(RuntimeError):
            self.prepare(existing=[{"ref": "refs/tags/v3.1.0", "object": {"sha": "other", "type": "commit"}}])

    def test_new_version_must_increase(self):
        with self.assertRaises(ValueError):
            self.prepare(version="v2.9.0")


if __name__ == "__main__":
    unittest.main()
