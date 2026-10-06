import importlib.util
import json
from pathlib import Path
import stat
import subprocess
import sys
import tempfile
import unittest


class CaptureTests(unittest.TestCase):
    def setUp(self):
        spec = importlib.util.spec_from_file_location('capture', Path(__file__).with_name('sentinel_capture.py'))
        self.capture = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(self.capture)

    def test_codex_notification_has_unknown_usage_and_redacts_secrets(self):
        record = self.capture.normalize('codex', {'type': 'agent-turn-complete', 'thread-id': 's',
            'turn-id': 't', 'input-messages': ['api_key=secret-value', '{"api_key": "json-secret"}'],
            'last-assistant-message': 'Bearer abcdefghijklmnop'})
        self.assertEqual(record['session_id'], 's')
        self.assertEqual(record['turn_id'], 't')
        self.assertIsNone(record['usage']['input_tokens'])
        self.assertIsNone(record['model'])
        self.assertEqual(record['quality']['status'], 'unscored')
        self.assertNotIn('secret-value', json.dumps(record))
        self.assertNotIn('json-secret', json.dumps(record))
        self.assertNotIn('abcdefghijklmnop', json.dumps(record))

    def test_claude_stop_only_captures_latest_turn_and_unique_message_usage(self):
        with tempfile.TemporaryDirectory() as tmp:
            transcript = Path(tmp).resolve() / 'session.jsonl'
            rows = [{'type': 'user', 'uuid': 'old', 'message': {'content': 'old prompt'}},
                    {'type': 'assistant', 'message': {'id': 'old', 'content': 'old answer', 'usage': {'input_tokens': 90}}},
                    {'type': 'user', 'uuid': 'new', 'timestamp': '2026-10-06T00:00:00Z', 'message': {'content': 'new prompt'}},
                    {'type': 'assistant', 'timestamp': '2026-10-06T00:00:02Z', 'message': {'id': 'm1', 'model': 'fixture-model', 'content': [{'type': 'text', 'text': 'answer'}], 'usage': {'input_tokens': 10, 'output_tokens': 3}}}]
            transcript.write_text('\n'.join(map(json.dumps, rows + [rows[-1]])))
            record = self.capture.normalize('claude', {'hook_event_name': 'Stop', 'session_id': 's', 'transcript_path': str(transcript)})
            self.assertEqual(record['inputs'], ['new prompt'])
            self.assertEqual(record['outputs'], ['answer'])
            self.assertEqual(record['usage']['input_tokens'], 10)
            self.assertEqual(record['usage']['output_tokens'], 3)
            self.assertEqual(record['latency_ms'], 2000)
            self.assertEqual(record['turn_id'], 'new')

    def test_private_append_and_symlink_refusal(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp).resolve() / 'events.jsonl'
            self.capture.append_private(path, {'x': 1})
            self.capture.append_private(path, {'x': 2})
            self.assertEqual([json.loads(x)['x'] for x in path.read_text().splitlines()], [1, 2])
            self.assertEqual(stat.S_IMODE(path.stat().st_mode), 0o600)
            link = Path(tmp).resolve() / 'link'
            link.symlink_to(path)
            with self.assertRaises((ValueError, OSError)):
                self.capture.append_private(link, {'x': 3})
            with self.assertRaises((ValueError, OSError)):
                self.capture.read_transcript(str(link))

    def test_claude_preserves_separate_text_blocks_of_one_api_message(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp).resolve() / 'transcript.jsonl'
            rows = [{'type': 'user', 'message': {'content': 'fixture'}}]
            for text, tokens in [('first block', 2), ('second block', 5), ('second block', 5)]:
                rows.append({'type': 'assistant', 'message': {'id': 'same',
                    'content': [{'type': 'text', 'text': text}], 'usage': {'input_tokens': 10, 'output_tokens': tokens}}})
            path.write_text('\n'.join(map(json.dumps, rows)))
            record = self.capture.normalize('claude', {'hook_event_name': 'Stop', 'transcript_path': str(path)})
            self.assertEqual(record['outputs'], ['first block', 'second block'])
            self.assertEqual(record['usage']['output_tokens'], 5)

    def test_prompt_and_bounded_invalid_inputs(self):
        record = self.capture.normalize('claude', {'hook_event_name': 'UserPromptSubmit', 'prompt': 'hello', 'session_id': 's'})
        self.assertEqual(record['inputs'], ['hello'])
        self.assertEqual(record['outputs'], [])
        with self.assertRaises(ValueError):
            self.capture.normalize('codex', {'type': 'approval-requested'})
        with self.assertRaises(ValueError):
            self.capture.decode_event('x' * (self.capture.MAX_EVENT_BYTES + 1))

    def test_native_codex_hook_and_collector_address_validation(self):
        record = self.capture.normalize('codex', {'hook_event_name': 'Stop', 'session_id': 's',
            'turn_id': 't', 'model': 'fixture-model', 'last_assistant_message': 'native answer'})
        self.assertEqual(record['outputs'], ['native answer'])
        self.assertEqual(record['model'], 'fixture-model')
        self.assertIsNone(record['usage']['output_tokens'])
        for url in ('https://127.0.0.1:19090/x', 'http://example.com/x', 'http://localhost/x', 'http://user:password@127.0.0.1/x'):
            with self.assertRaises(ValueError):
                self.capture.collector_address(url)
        self.assertEqual(self.capture.collector_address('http://127.0.0.1:19090/sentinel/training/events').hostname, '127.0.0.1')

    def codex_rollout_fixture(self, path, version='0.160.1', record_usage=True):
        # Wire shapes from openai/codex rust-v0.160.1 protocol and history/rollout_payload.rs.
        usage = {'input_tokens': 100, 'cached_input_tokens': 60, 'output_tokens': 20,
                 'reasoning_output_tokens': 5, 'total_tokens': 120}
        rows = [{'type': 'session_meta', 'payload': {'id': 's', 'session_id': 's', 'cli_version': version, 'model_provider': 'openai'}},
                {'type': 'event_msg', 'payload': {'type': 'task_started', 'turn_id': 't'}},
                {'type': 'turn_context', 'payload': {'turn_id': 't', 'model': 'fixture-model'}},
                {'type': 'response_item', 'payload': {'type': 'message', 'role': 'user', 'content': [{'type': 'input_text', 'text': 'fixture input'}]}},
                {'type': 'response_item', 'payload': {'type': 'message', 'role': 'assistant', 'content': [{'type': 'output_text', 'text': 'fixture output'}]}},
                {'type': 'event_msg', 'payload': {'type': 'token_count', 'info': {'last_token_usage': usage, 'total_token_usage': usage}}}]
        if record_usage:
            row = {'type': 'token_usage_record', 'payload': {'thread_id': 's', 'session_id': 's', 'turn_id': 't', 'response_id': 'r', 'usage': usage}}
            rows += [row, row]
        path.write_text('\n'.join(map(json.dumps, rows)))

    def test_codex_reported_rollout_usage_matches_turn_and_deduplicates_responses(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp).resolve() / 'rollout.jsonl'
            self.codex_rollout_fixture(path)
            event = {'hook_event_name': 'Stop', 'session_id': 's', 'turn_id': 't', 'transcript_path': str(path)}
            record = self.capture.normalize('codex', event, billing_class='subscription')
            self.assertEqual(record['usage']['input_tokens'], 100)
            self.assertEqual(record['usage']['cache_read_input_tokens'], 60)
            self.assertEqual(record['usage']['output_tokens'], 20)
            self.assertEqual(record['usage']['reasoning_output_tokens'], 5)
            self.assertEqual(record['usage']['total_tokens'], 120)
            self.assertEqual(record['usage']['scope'], 'observed_turn_calls')
            self.assertEqual(record['usage']['reported_calls'], 1)
            self.assertEqual(record['billing']['class'], 'subscription')
            self.assertIsNone(record['billing']['cost_usd'])
            self.assertEqual(record['inputs'], ['fixture input'])
            self.assertEqual(record['outputs'], ['fixture output'])
            event['turn_id'] = 'unrelated'
            self.assertIsNone(self.capture.normalize('codex', event)['usage']['input_tokens'])
            event['turn_id'], event['session_id'] = 't', 'unrelated'
            self.assertIsNone(self.capture.normalize('codex', event)['usage']['input_tokens'])

    def test_codex_token_count_fallback_is_last_call_and_unknown_version_stays_unknown(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp).resolve() / 'rollout.jsonl'
            self.codex_rollout_fixture(path, record_usage=False)
            event = {'hook_event_name': 'Stop', 'session_id': 's', 'turn_id': 't', 'transcript_path': str(path)}
            record = self.capture.normalize('codex', event)
            self.assertEqual(record['usage']['source'], 'codex_rollout_token_count')
            self.assertEqual(record['usage']['scope'], 'last_call')
            self.assertEqual(record['usage']['input_tokens'], 100)
            self.codex_rollout_fixture(path, version='unknown')
            record = self.capture.normalize('codex', event)
            self.assertIsNone(record['usage']['input_tokens'])
            self.assertEqual(record['billing']['class'], 'direct_unknown')

    def test_codex_malformed_and_synthetic_usage_is_unknown_and_metadata_survives_tail_limit(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp).resolve() / 'rollout.jsonl'
            self.codex_rollout_fixture(path)
            original = path.read_text().splitlines()
            path.write_text('\n'.join([original[0]] + [json.dumps({'type': 'event_msg', 'payload': {'type': 'unrelated'}})] * 600 + original[1:]))
            event = {'hook_event_name': 'Stop', 'session_id': 's', 'turn_id': 't', 'transcript_path': str(path)}
            record = self.capture.normalize('codex', event)
            self.assertEqual(record['usage']['input_tokens'], 100)
            self.assertTrue(record['truncated'])
            self.assertIsNone(self.capture.codex_usage({'input_tokens': 0, 'cached_input_tokens': 0, 'output_tokens': 0, 'reasoning_output_tokens': 0, 'total_tokens': 500}))
            self.assertIsNone(self.capture.codex_usage({'input_tokens': True, 'cached_input_tokens': 0, 'output_tokens': 1, 'reasoning_output_tokens': 0, 'total_tokens': 2}))
            self.assertIsNone(self.capture.codex_usage({'input_tokens': 100}))

    def test_collector_receives_same_record_and_failed_delivery_preserves_spool(self):
        from unittest.mock import patch
        record = self.capture.normalize('codex', {'type': 'agent-turn-complete', 'input-messages': ['fixture']})
        with patch.object(self.capture.http.client, 'HTTPConnection') as transport:
            transport.return_value.getresponse.return_value.status = 204
            self.assertTrue(self.capture.deliver('http://127.0.0.1:19090/events', record))
            transport.assert_called_once_with('127.0.0.1', 19090, timeout=0.3)
            self.assertEqual(json.loads(transport.return_value.request.call_args.args[2]), record)
            transport.return_value.request.side_effect = OSError('offline')
            self.assertFalse(self.capture.deliver('http://127.0.0.1:19090/events', record))
        with tempfile.TemporaryDirectory() as tmp:
            output = Path(tmp).resolve() / 'events.jsonl'
            proc = subprocess.run([sys.executable, str(Path(__file__).with_name('sentinel_capture.py')),
                '--client', 'codex', '--output', str(output), '--collector', 'http://127.0.0.1:1/events',
                json.dumps({'type': 'agent-turn-complete', 'input-messages': ['fixture']})], capture_output=True, text=True)
            self.assertEqual(proc.returncode, 0)
            self.assertEqual(json.loads(output.read_text())['inputs'], ['fixture'])

    def test_preview_configs_and_cli_argv_notification(self):
        with tempfile.TemporaryDirectory() as tmp:
            output = Path(tmp).resolve() / 'events.jsonl'
            preview = self.capture.preview_config('claude', output)
            self.assertEqual(set(preview['hooks']), {'UserPromptSubmit', 'Stop'})
            self.assertIn('notify = [', self.capture.preview_config('codex', output))
            proc = subprocess.run([sys.executable, str(Path(__file__).with_name('sentinel_capture.py')),
                '--client', 'codex', '--output', str(output), json.dumps({'type': 'agent-turn-complete', 'input-messages': ['fixture'], 'last-assistant-message': 'ok'})], capture_output=True, text=True)
            self.assertEqual(proc.returncode, 0, proc.stderr)
            self.assertEqual(proc.stdout, '')
            self.assertEqual(json.loads(output.read_text())['inputs'], ['fixture'])


if __name__ == '__main__':
    unittest.main()
