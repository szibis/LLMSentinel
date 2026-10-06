import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch


class StatuslineTests(unittest.TestCase):
    def setUp(self):
        spec = importlib.util.spec_from_file_location("statusline", Path(__file__).with_name("sentinel_statusline.py"))
        self.module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(self.module)

    def test_shared_runtime_is_counted_once_and_memory_is_machine_wide(self):
        snapshot = {"gateway": True, "runtimes": {
            "large": {"model_loaded": True, "stats": {"requests": 25, "tokens_generated": 5439},
                      "memory": {"available_gb": 6.8, "swap_used_gb": 17.7, "pressure": "warning"}},
            "small": {"model_loaded": True, "stats": {"requests": 2, "tokens_generated": 24},
                      "memory": {"available_gb": 6.8, "swap_used_gb": 17.7, "pressure": "warning"}},
        }}
        output = self.module.render({"model": {"id": "sentinel-sonnet"}, "context_window": {"used_percentage": 12}}, snapshot)
        self.assertIn("27 MLX req", output)
        self.assertIn("5463 tok", output)
        self.assertIn("6.8 GB available", output)
        self.assertIn("17.7 GB swap", output)
        self.assertIn("ctx 12%", output)
        self.assertIn("Sonnet", output)
        self.assertIn("pressure warning", output)
        self.assertNotIn("13.6", output)

    def test_unavailable_data_is_not_reported_as_zero_or_healthy(self):
        output = self.module.render({}, {"gateway": False, "runtimes": {"large": None, "small": None}})
        self.assertIn("gateway offline", output)
        self.assertIn("large unavailable", output)
        self.assertNotIn("0 tok", output)

    def test_loading_model_still_reports_machine_pressure(self):
        output = self.module.render({}, {"gateway": True, "runtimes": {
            "large": {"model_loaded": False, "memory": {"pressure": "critical", "available_gb": 1.2}},
        }})
        self.assertIn("large loading", output)
        self.assertIn("pressure critical", output)
        self.assertIn("1.2 GB available", output)

    def test_rate_uses_sample_deltas_and_resets_with_runtime(self):
        previous = {"sample_time": 100, "runtimes": {"large": {"stats": {"uptime_s": 50, "tokens_generated": 100, "requests": 4}}}}
        current = {"sample_time": 105, "runtimes": {"large": {"stats": {"uptime_s": 55, "tokens_generated": 200, "requests": 5}}}}
        self.module.add_rates(current, previous)
        self.assertEqual(current["rates"]["tokens_per_s"], 20)
        self.assertEqual(current["rates"]["requests_per_min"], 12)
        current["runtimes"]["large"]["stats"]["uptime_s"] = 1
        self.module.add_rates(current, previous)
        self.assertNotIn("tokens_per_s", current["rates"])

    def test_settings_install_is_idempotent_and_preserves_preferences(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "claude").mkdir()
            config = root / "claude/settings.json"
            config.write_text(json.dumps({"theme": "dark", "permissions": {"defaultMode": "default"}}))
            self.module.install(root)
            settings = json.loads(config.read_text())
            self.assertEqual(settings["theme"], "dark")
            self.assertEqual(settings["permissions"], {"defaultMode": "default"})
            self.assertEqual(settings["statusLine"]["refreshInterval"], 5)
            before = config.read_bytes()
            self.module.install(root)
            self.assertEqual(config.read_bytes(), before)
            settings["statusLine"] = {"type": "command", "command": "custom"}
            config.write_text(json.dumps(settings))
            self.module.install(root)
            self.assertEqual(json.loads(config.read_text())["statusLine"]["command"], "custom")

    def test_poll_does_not_follow_remote_state_endpoints(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "state.json").write_text(json.dumps({"gateway": "https://example.com", "models": [{"name": "large", "port": "443/redirect"}]}))
            with patch.object(self.module, "fetch", return_value=None) as fetch:
                self.module.collect(root)
            for call in fetch.call_args_list:
                self.assertTrue(call.args[0].startswith("http://127.0.0.1:"))


if __name__ == "__main__":
    unittest.main()
