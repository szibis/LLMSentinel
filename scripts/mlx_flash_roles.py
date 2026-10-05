#!/usr/bin/env python3
"""Lab-local request profiles over the installed MLX-Flash inference server.

No model engine is copied here. This adapter adds the Qwen tokenizer's
enable_thinking option that the pinned MLX-Flash HTTP handler does not forward.
"""
from contextvars import ContextVar
import io
import json
import threading

THINKING = ContextVar("sentinel_qwen_thinking", default=False)
INFERENCE = threading.Lock()


def run_profile(payload, generate):
    options = payload.get("chat_template_kwargs", {"enable_thinking": False})
    if not isinstance(options, dict) or set(options) != {"enable_thinking"} or type(options["enable_thinking"]) is not bool:
        raise ValueError("Only a boolean chat_template_kwargs.enable_thinking is supported")
    with INFERENCE:
        token = THINKING.set(options["enable_thinking"])
        try:
            return generate()
        finally:
            THINKING.reset(token)


def install(serve):
    class RoleState(serve.InferenceState):
        def _format_messages(self, messages):
            # Fail explicitly if the selected tokenizer cannot honor the profile.
            return self.tokenizer.apply_chat_template(
                messages, tokenize=False, add_generation_prompt=True,
                enable_thinking=THINKING.get())

    class RoleHandler(serve.ChatHandler):
        def do_POST(self):
            if self.path != "/v1/chat/completions":
                self._send_json({"error": "Lab runtime only accepts Chat Completions"}, 404)
                return
            self._handle_chat()

        def _handle_chat(self):
            try:
                length = int(self.headers.get("Content-Length", 0))
                if not 0 < length <= 2 * 1024 * 1024:
                    raise ValueError("Request must be 1..2 MiB")
                body = self.rfile.read(length)
                payload = json.loads(body)
                if not isinstance(payload, dict):
                    raise ValueError("Expected a JSON object")
                # The original handler parses and serves the original request.
                # A scoped tokenizer option and lock span all native generation.
                original = self.rfile
                self.rfile = io.BytesIO(body)
                try:
                    run_profile(payload, lambda: super(RoleHandler, self)._handle_chat())
                finally:
                    self.rfile = original
            except (ValueError, TypeError) as error:
                self._send_json({"error": str(error)}, 400)

    serve.InferenceState = RoleState
    serve.ChatHandler = RoleHandler


if __name__ == "__main__":
    from mlx_flash_compress import serve
    # New MLX-Flash releases own the native contract. Retain compatibility with
    # the lab's explicitly pinned older installation until it is upgraded.
    if not hasattr(serve, "validate_chat_template_kwargs"):
        install(serve)
    serve.main()
