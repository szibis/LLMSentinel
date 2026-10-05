import concurrent.futures
import importlib.util
from pathlib import Path
import threading
import time
import unittest

spec=importlib.util.spec_from_file_location("mlx_flash_roles",Path(__file__).with_name("mlx_flash_roles.py"))
roles=importlib.util.module_from_spec(spec)
spec.loader.exec_module(roles)


class ThinkingProfileTests(unittest.TestCase):
    def test_thinking_is_request_scoped_and_restored_after_failure(self):
        self.assertFalse(roles.THINKING.get())
        def fail():
            self.assertTrue(roles.THINKING.get())
            raise RuntimeError("generation failure")
        with self.assertRaisesRegex(RuntimeError,"generation failure"):
            roles.run_profile({"chat_template_kwargs":{"enable_thinking":True}},fail)
        self.assertFalse(roles.THINKING.get())
        self.assertFalse(roles.run_profile({"chat_template_kwargs":{"enable_thinking":False}},roles.THINKING.get))

    def test_overlapping_requests_never_mix_effort_or_inference(self):
        active=0; lock=threading.Lock()
        def run(thinking):
            def generate():
                nonlocal active
                with lock:
                    active+=1; self.assertEqual(active,1)
                time.sleep(.02)
                observed=roles.THINKING.get()
                with lock: active-=1
                return observed
            return roles.run_profile({"chat_template_kwargs":{"enable_thinking":thinking}},generate)
        with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
            a=pool.submit(run,True); b=pool.submit(run,False)
            self.assertTrue(a.result()); self.assertFalse(b.result())

    def test_invalid_template_options_never_enter_generation(self):
        for options in ({"enable_thinking":"false"},{"unknown":True},[],None):
            with self.assertRaises(ValueError):
                roles.run_profile({"chat_template_kwargs":options},lambda:self.fail("generation admitted"))


if __name__=="__main__": unittest.main()
