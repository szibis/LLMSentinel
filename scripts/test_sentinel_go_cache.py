"""Ensure native CI can remove read-only Go modules without touching host caches."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

from test_sentinel_release import workflow_script, WORKFLOW


class GoCacheCleanupTests(unittest.TestCase):
    def test_removes_readonly_job_modules_and_preserves_unrelated_cache(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            cache = root / "job-go-cache"
            module = cache / "mod" / "example.com" / "library@v1.0.0"
            module.mkdir(parents=True)
            (module / "source.go").write_text("package library\n")
            unrelated = root / "host-go-cache"
            unrelated.mkdir()
            (unrelated / "keep").write_text("host cache")
            module.chmod(0o555)
            env = dict(os.environ, QWEN_JOB_GO_ROOT=str(cache), QWEN_JOB_VENV="")
            script = workflow_script("Remove job runtime", WORKFLOW.with_name("qwen-metal.yml"))
            try:
                result = subprocess.run(["bash", "-e", "-c", script], env=env, capture_output=True, text=True)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertFalse(cache.exists())
                self.assertEqual((unrelated / "keep").read_text(), "host cache")
            finally:
                if module.exists():
                    module.chmod(0o755)


if __name__ == "__main__":
    unittest.main()
