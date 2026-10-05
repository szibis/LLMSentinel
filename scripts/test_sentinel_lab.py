import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from types import SimpleNamespace
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("sentinel_lab", Path(__file__).with_name("sentinel_lab.py"))
lab = importlib.util.module_from_spec(spec)
spec.loader.exec_module(lab)


class LabIsolationTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.root_patch = patch.object(lab, "ROOT", Path(self.directory.name) / "lab")
        self.root_patch.start()
        self.addCleanup(self.root_patch.stop)

    def test_prepare_preserves_changed_configuration(self):
        root = lab.prepare()
        config = root / "codex" / "config.toml"
        config.write_text("user edit\n")
        self.assertEqual(lab.prepare(), root)
        self.assertEqual(config.read_text(), "user edit\n")
        self.assertTrue((root / "workspace" / ".git").is_dir())

    def test_prepare_allows_claude_preferences_without_rewriting_them(self):
        root = lab.prepare()
        config = root / "claude" / "settings.json"
        preferences = '{"model":"local","permissions":{"defaultMode":"default"},"theme":"light"}\n'
        config.write_text(preferences)
        before = config.stat().st_mtime_ns
        for _ in range(2):
            self.assertEqual(lab.prepare(), root)
        self.assertEqual(config.read_text(), preferences)
        self.assertEqual(config.stat().st_mtime_ns, before)

    def test_real_child_receives_only_local_credentials_and_own_state(self):
        lab.prepare()
        with patch.dict(os.environ, {
            "OPENAI_API_KEY": "production-marker", "ANTHROPIC_AUTH_TOKEN": "production-marker",
            "GH_TOKEN": "production-marker", "HTTPS_PROXY": "https://production-marker.invalid",
        }):
            for client in ("codex", "claude"):
                child = subprocess.run([
                    "/usr/bin/env",
                ], env=lab.environment(client), capture_output=True, text=True, check=True)
                self.assertNotIn("production-marker", child.stdout)
                own = "CODEX_HOME" if client == "codex" else "CLAUDE_CONFIG_DIR"
                self.assertIn(f"{own}={lab.ROOT / client}\n", child.stdout)

    def test_client_selection_never_falls_back_to_global_installation(self):
        lab.prepare()
        with patch.object(lab.shutil, "which", return_value="/global/claude"):
            with self.assertRaisesRegex(RuntimeError, "lab-clients-update"):
                lab.client_binary("claude")
        binary = lab.ROOT / "clients" / "node_modules" / ".bin" / "claude"
        binary.parent.mkdir(parents=True)
        binary.write_text("#!/bin/sh\nexit 0\n")
        binary.chmod(0o700)
        self.assertEqual(lab.client_binary("claude"), str(binary))

    def test_client_child_path_prefers_lab_tools(self):
        lab.prepare()
        for client in ("codex", "claude"):
            env = lab.environment(client)
            self.assertEqual(env["PATH"].split(os.pathsep)[0], str(lab.ROOT / "clients" / "node_modules" / ".bin"))

    def test_claude_launch_is_interactive_local_and_preserves_permissions(self):
        lab.prepare()
        with patch.object(lab, "client_binary", return_value="/isolated/claude"):
            command, env, workspace = lab.launch_spec("claude")
        self.assertEqual(command[0], "/isolated/claude")
        self.assertNotIn("--print", command)
        self.assertNotIn("-p", command)
        self.assertNotIn("--dangerously-skip-permissions", command)
        self.assertIn("--bare", command)
        self.assertIn("--strict-mcp-config", command)
        self.assertEqual(env["ANTHROPIC_BASE_URL"], lab.ENDPOINT)
        self.assertEqual(env["MAX_THINKING_TOKENS"], "0")
        self.assertEqual(workspace, (lab.ROOT / "workspace").resolve())

    def test_claude_refreshes_only_owned_outdated_gateway(self):
        supervisor=SimpleNamespace(running=lambda root:True, request_stop=unittest.mock.Mock(), start=unittest.mock.Mock())
        old={"capabilities":{"messages":False,"tools":False}}
        new={"capabilities":{"messages":True,"tools":True}}
        with patch.object(lab,"health",side_effect=[old,new]):
            self.assertEqual(lab.ensure_claude_gateway(supervisor),new)
        supervisor.request_stop.assert_called_once_with(lab.ROOT)
        supervisor.start.assert_called_once()

    def test_claude_does_not_restart_unowned_gateway(self):
        supervisor=SimpleNamespace(running=lambda root:False, request_stop=unittest.mock.Mock(), start=unittest.mock.Mock())
        old={"capabilities":{"messages":False,"tools":False}}
        with patch.object(lab,"health",return_value=old):
            self.assertEqual(lab.ensure_claude_gateway(supervisor),old)
        supervisor.request_stop.assert_not_called()
        supervisor.start.assert_not_called()

    def test_role_selection_refreshes_single_model_gateway(self):
        root=lab.prepare()
        (root/"runtime.json").write_text('{"small_model_path":"/local/small"}')
        supervisor=SimpleNamespace(running=lambda root:True, request_stop=unittest.mock.Mock(), start=unittest.mock.Mock())
        old={"capabilities":{"messages":True,"tools":True,"claude_roles":False}}
        new={"capabilities":{"messages":True,"tools":True,"claude_roles":True}}
        with patch.object(lab,"health",side_effect=[old,new]):
            self.assertEqual(lab.ensure_claude_gateway(supervisor),new)
        supervisor.request_stop.assert_called_once_with(root)
        supervisor.start.assert_called_once()

    def test_claude_context_window_comes_from_saved_model_metadata(self):
        root=lab.prepare(); model=root/"model";model.mkdir()
        (model/"config.json").write_text('{"max_position_embeddings":32768}')
        (root/"runtime.json").write_text(json.dumps({"model_path":str(model)}))
        self.assertEqual(lab.environment("claude")["CLAUDE_CODE_MAX_CONTEXT_TOKENS"],"32768")

    def test_claude_roles_hide_qwen_artifacts_from_client(self):
        lab.prepare()
        env = lab.environment("claude")
        self.assertEqual(env["ANTHROPIC_DEFAULT_HAIKU_MODEL"], "sentinel-haiku")
        self.assertEqual(env["ANTHROPIC_DEFAULT_SONNET_MODEL"], "sentinel-sonnet")
        self.assertEqual(env["ANTHROPIC_DEFAULT_OPUS_MODEL"], "sentinel-opus")
        self.assertEqual(env["ANTHROPIC_MODEL"], "opusplan")
        with patch.object(lab,"client_binary",return_value="/isolated/claude"):
            command,_,_ = lab.launch_spec("claude")
        self.assertEqual(command[command.index("--model")+1], "opusplan")
        self.assertIn("Agent", command[command.index("--tools")+1].split(","))
        agents=json.loads(command[command.index("--agents")+1])
        self.assertEqual(agents["Explore"]["model"],"haiku")
        self.assertEqual(agents["Plan"]["model"],"opus")

    def test_two_models_declare_minimum_nested_context(self):
        root = lab.prepare()
        for name,window in (("small",32768),("large",262144)):
            model=root/name; model.mkdir()
            (model/"config.json").write_text(json.dumps({"text_config":{"max_position_embeddings":window}}))
        (root/"runtime.json").write_text(json.dumps({"model_path":str(root/"large"),"small_model_path":str(root/"small")}))
        self.assertEqual(lab.environment("claude")["CLAUDE_CODE_MAX_CONTEXT_TOKENS"],"32768")

    def test_explicit_project_workspace_keeps_separate_client_state(self):
        root=lab.prepare();project=root/"example-project";project.mkdir()
        with patch.dict(os.environ,{"LAB_WORKSPACE":str(project)}),patch.object(lab,"client_binary",return_value="/isolated/claude"):
            command,env,workspace=lab.launch_spec("claude")
        self.assertEqual(workspace,project.resolve())
        self.assertEqual(env["CLAUDE_CONFIG_DIR"],str(root/"claude"))

    def test_updates_install_latest_only_under_lab_without_provider_credentials(self):
        lab.prepare()
        with patch.object(lab.shutil, "which", return_value="/usr/bin/npm"), \
             patch.object(lab.subprocess, "run") as run, \
             patch.object(lab, "client_binary", side_effect=lambda name: str(lab.ROOT / name)), \
             patch.dict(os.environ, {"ANTHROPIC_API_KEY": "production-marker", "NPM_TOKEN": "production-marker"}):
            lab.update_clients()
        install = run.call_args_list[0]
        args = install.args[0]
        self.assertIn("@openai/codex@latest", args)
        self.assertIn("@anthropic-ai/claude-code@latest", args)
        self.assertEqual(args[args.index("--prefix") + 1], str(lab.ROOT / "clients"))
        self.assertNotIn("-g", args)
        self.assertNotIn("production-marker", str(install.kwargs["env"]))
        user_config = next(arg for arg in args if arg.startswith("--userconfig="))
        global_config = next(arg for arg in args if arg.startswith("--globalconfig="))
        self.assertNotEqual(user_config.split("=", 1)[1], global_config.split("=", 1)[1])


if __name__ == "__main__":
    unittest.main()
