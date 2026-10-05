import importlib.util
import errno
import json
import os
from pathlib import Path
import subprocess
import socket
import sys
import tempfile
import unittest
from unittest.mock import Mock, patch

spec = importlib.util.spec_from_file_location("sentinel_runner", Path(__file__).with_name("sentinel_runner.py"))
runner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(runner)


class RunnerTests(unittest.TestCase):
    def test_role_runtime_plan_uses_two_owned_processes_and_shared_large_artifact(self):
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory)
            executable=root/"bin"/"mlx-flash"; executable.parent.mkdir()
            for name in ("mlx-flash","python"):
                file=executable.parent/name; file.write_text("#!/bin/sh\nexit 0\n"); file.chmod(0o700)
            for name in ("small","large"):
                model=root/name; model.mkdir()
                for file in ("config.json","tokenizer.json","model.safetensors"):
                    (model/file).write_text("{}")
                (model/"tokenizer_config.json").write_text('{"chat_template":"{{ messages }} {{ enable_thinking }}"}')
            settings={"model_path":str(root/"large"),"small_model_path":str(root/"small"),"executable":str(executable)}
            plan=runner.runtime_plan(settings)
            self.assertEqual([item["port"] for item in plan],[19091,19092])
            self.assertEqual([item["model_path"] for item in plan],[str(root/"large"),str(root/"small")])
            for item in plan:
                self.assertEqual(Path(item["command"][0]).resolve(),(executable.parent/"python").resolve())
                self.assertIn("mlx_flash_roles.py",item["command"][1])
            self.assertEqual(runner.gateway_command(root/"gateway",settings)[-6:],
                ["--role-haiku-upstream","http://127.0.0.1:19092/v1","--role-sonnet-upstream","http://127.0.0.1:19091/v1","--role-opus-upstream","http://127.0.0.1:19091/v1"])

    def test_role_runtime_plan_refuses_partial_small_snapshot_before_spawn(self):
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory); large=root/"large";small=root/"small";large.mkdir();small.mkdir()
            for name in ("config.json","tokenizer.json","model.safetensors"):
                (large/name).write_text("{}")
            with self.assertRaisesRegex(RuntimeError,"config.json"):
                runner.runtime_plan({"model_path":str(large),"small_model_path":str(small),"executable":"/missing"})

    def test_background_start_returns_and_stop_cleans_up_supervisor(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            helper = root / "helper.py"
            helper.write_text('''import os, sys, time
from pathlib import Path
sys.path.insert(0, sys.argv[1])
import sentinel_runner as r
root = Path(sys.argv[2])
with r.LabLock(root):
    state = {"run_id":"fixture", "phase":"running", "start_id":os.environ["SENTINEL_LAB_START_ID"]}
    r.write_state(root, state)
    while not (root / "stop-fixture").exists(): time.sleep(.02)
    state["phase"] = "stopped"
    r.write_state(root, state)
''')
            command = [sys.executable, str(helper), str(Path(__file__).parent), str(root)]
            with patch.object(runner.lab, "ROOT", root), patch.object(runner.lab, "prepare", return_value=root):
                process = runner.start(command)
                try:
                    self.assertIsNone(process.poll())
                    self.assertTrue(runner.running(root))
                    self.assertIsNone(runner.start(command))
                    runner.request_stop(root)
                    self.assertEqual(process.wait(timeout=3), 0)
                finally:
                    if process.poll() is None:
                        runner.stop_process(process)

    def test_background_start_reports_early_failure_and_keeps_log(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            with patch.object(runner.lab, "ROOT", root), patch.object(runner.lab, "prepare", return_value=root):
                with self.assertRaisesRegex(RuntimeError, "startup failed"):
                    runner.start([sys.executable, "-c", "print('missing model', flush=True); raise SystemExit(1)"])
                self.assertIn("missing model", (root / "supervisor.log").read_text())

    def test_failed_start_reports_only_current_attempt_without_erasing_history(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            log = root / "supervisor.log"
            log.write_text("old sandbox denial\nold successful plain-text startup\n")
            with patch.object(runner.lab, "prepare", return_value=root):
                with self.assertRaises(RuntimeError) as caught:
                    runner.start([sys.executable, "-c", "print('current failure', flush=True); raise SystemExit(1)"])
            self.assertIn("current failure", str(caught.exception))
            self.assertNotIn("old sandbox denial", str(caught.exception))
            self.assertNotIn("old successful", str(caught.exception))
            self.assertIn("old sandbox denial", log.read_text())

    def test_port_preflight_allows_reusable_addresses_and_checks_listeners(self):
        probe = Mock()
        def bind(address):
            if (socket.SOL_SOCKET, socket.SO_REUSEADDR, 1) not in [call.args for call in probe.setsockopt.call_args_list]:
                raise OSError(errno.EADDRINUSE, "recently closed connection")
        probe.bind.side_effect = bind
        with patch("socket.socket", return_value=probe):
            runner.preflight_ports([19090], timeout=0)
        probe.listen.assert_called_once_with(1)
        probe.close.assert_called_once()

    def test_port_preflight_retries_brief_shutdown_race_and_closes_probes(self):
        busy, free = Mock(), Mock()
        busy.bind.side_effect = OSError(errno.EADDRINUSE, "closing listener")
        with patch("socket.socket", side_effect=[busy, free]), patch.object(runner.time, "sleep"):
            runner.preflight_ports([19091])
        busy.close.assert_called_once()
        free.close.assert_called_once()

    def test_port_preflight_refuses_active_listener_and_names_port(self):
        gateway, runtime = Mock(), Mock()
        runtime.listen.side_effect = OSError(errno.EADDRINUSE, "active listener")
        with patch("socket.socket", side_effect=[gateway, runtime]):
            with self.assertRaisesRegex(RuntimeError, "19091.*in use"):
                runner.preflight_ports([19090, 19091], timeout=0)
        gateway.close.assert_called_once()
        runtime.close.assert_called_once()

    def test_port_preflight_reports_permission_denial_without_retry(self):
        probe = Mock()
        probe.bind.side_effect = OSError(errno.EPERM, "Operation not permitted")
        with patch("socket.socket", return_value=probe), patch.object(runner.time, "sleep") as sleep:
            with self.assertRaisesRegex(RuntimeError, "19090.*permission"):
                runner.preflight_ports([19090])
        sleep.assert_not_called()
        probe.close.assert_called_once()

    def test_only_one_supervisor_can_own_a_lab(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            with runner.LabLock(root):
                self.assertTrue(runner.running(root))
                with self.assertRaisesRegex(RuntimeError, "already running"):
                    with runner.LabLock(root):
                        self.fail("second owner admitted")
            self.assertFalse(runner.running(root))

    def test_stop_request_targets_current_run_without_signalling_processes(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            with runner.LabLock(root):
                runner.write_state(root, {"run_id": "current", "phase": "running"})
                runner.request_stop(root, wait=False)
                self.assertTrue((root / "stop-current").exists())
                self.assertFalse((root / "stop-other").exists())
            self.assertFalse(runner.request_stop(root, wait=False))

    def test_owned_process_stop_preserves_unrelated_process(self):
        owned = subprocess.Popen([sys.executable, "-c", "import time;time.sleep(30)"], start_new_session=True)
        unrelated = subprocess.Popen([sys.executable, "-c", "import time;time.sleep(30)"], start_new_session=True)
        try:
            runner.stop_process(owned)
            self.assertIsNotNone(owned.poll())
            self.assertIsNone(unrelated.poll())
        finally:
            for process in (owned, unrelated):
                if process.poll() is None:
                    process.terminate()
                    process.wait(timeout=3)

    def test_exited_owned_group_permission_denial_does_not_escalate(self):
        for returncode in (0, None):
            with self.subTest(returncode=returncode):
                owned = Mock(pid=98765)
                owned.poll.return_value = returncode
                with patch.object(runner.os, "killpg", side_effect=[None, PermissionError()]) as kill:
                    if returncode is None:
                        with self.assertRaises(PermissionError):
                            runner.stop_process(owned)
                    else:
                        runner.stop_process(owned)
                    self.assertEqual(kill.call_count, 2)

    def test_owned_descendant_is_stopped_after_leader_exits(self):
        with tempfile.TemporaryDirectory() as directory:
            pid_file=Path(directory)/"child.pid";stopped=Path(directory)/"stopped"
            child_code="import os,time,signal,sys;from pathlib import Path;signal.signal(signal.SIGTERM,lambda *_:(Path("+repr(str(stopped))+").write_text('stopped'),sys.exit(0)));Path("+repr(str(pid_file))+").write_text(str(os.getpid()));time.sleep(30)"
            leader_code="import subprocess,sys,time;from pathlib import Path;subprocess.Popen([sys.executable,'-c',"+repr(child_code)+"]);p=Path("+repr(str(pid_file))+");\nwhile not p.exists(): time.sleep(.01)"
            leader=subprocess.Popen([sys.executable,"-c",leader_code],start_new_session=True)
            try:
                leader.wait(timeout=3)
                runner.stop_process(leader)
                deadline=runner.time.monotonic()+1
                while not stopped.exists() and runner.time.monotonic()<deadline: runner.time.sleep(.01)
                self.assertTrue(stopped.exists(),"owned descendant never received termination")
            finally:
                try: os.killpg(leader.pid,9)
                except ProcessLookupError: pass

    def test_role_snapshot_requires_thinking_template_before_spawn(self):
        with tempfile.TemporaryDirectory() as directory:
            model=Path(directory)
            for name in ("config.json","tokenizer.json","model.safetensors"):
                (model/name).write_text("{}")
            with self.assertRaisesRegex(RuntimeError,"tokenizer_config.json"):
                runner.validate_role_model(str(model))
            (model/"tokenizer_config.json").write_text("{}")
            with self.assertRaisesRegex(RuntimeError,"thinking.*template"):
                runner.validate_role_model(str(model))
            (model/"chat_template.jinja").write_text("{{ messages }} {{ enable_thinking }}")
            self.assertEqual(runner.validate_role_model(str(model)),model)

    def test_invalid_ownership_cannot_create_a_stop_marker(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            with runner.LabLock(root):
                runner.write_state(root, {"run_id": "../other", "phase": "running"})
                with self.assertRaisesRegex(RuntimeError, "Invalid lab ownership"):
                    runner.request_stop(root, wait=False)
                self.assertFalse(list(root.glob("stop-*")))

    def test_local_model_requires_complete_files_and_no_remote_id(self):
        with self.assertRaisesRegex(RuntimeError, "absolute"):
            runner.validate_model("mlx-community/model")
        with tempfile.TemporaryDirectory() as directory:
            model = Path(directory)
            with self.assertRaisesRegex(RuntimeError, "config.json"):
                runner.validate_model(str(model))
            for name in ("config.json", "tokenizer.json", "model.safetensors"):
                (model / name).write_text('{}')
            self.assertEqual(runner.validate_model(str(model)), model)

    def test_incomplete_indexed_shards_are_refused(self):
        with tempfile.TemporaryDirectory() as directory:
            model = Path(directory)
            for name in ("config.json", "tokenizer.json", "model-00001-of-00002.safetensors"):
                (model / name).write_text('{}')
            (model / "model.safetensors.index.json").write_text('{"weight_map":{"a":"model-00001-of-00002.safetensors","b":"model-00002-of-00002.safetensors"}}')
            with self.assertRaisesRegex(RuntimeError, "missing weight shard"):
                runner.validate_model(str(model))


if __name__ == "__main__":
    unittest.main()
