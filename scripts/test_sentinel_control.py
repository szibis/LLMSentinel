import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch


class ControlTests(unittest.TestCase):
    def setUp(self):
        spec = importlib.util.spec_from_file_location('control', Path(__file__).with_name('sentinel_control.py'))
        self.control = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(self.control)

    def test_fixed_requests_and_budget_validation(self):
        self.assertEqual(self.control.request_spec(['status']), ('GET', None))
        self.assertEqual(self.control.request_spec(['training', 'off']), ('POST', {'capture_enabled': False}))
        self.assertEqual(self.control.request_spec(['policy', 'local-only']), ('POST', {'policy': 'local-only'}))
        self.assertEqual(self.control.request_spec(['profile', 'sonnet', '2048']), ('POST', {'role_budgets': {'sonnet': 2048}}))
        for args in (['profile', 'sonnet', '0'], ['profile', 'sonnet', '32769'], ['profile', 'secret', '1'], ['policy', 'paid'], ['training', 'yes']):
            with self.assertRaises(ValueError):
                self.control.request_spec(args)

    def test_literal_loopback_only_and_transport_uses_no_redirect_or_proxy(self):
        for endpoint in ('https://127.0.0.1:19090', 'http://localhost:19090', 'http://user:secret@127.0.0.1:19090', 'http://example.com', 'http://127.0.0.1/path'):
            with self.assertRaises(ValueError):
                self.control.endpoint_address(endpoint)
        with patch.object(self.control.http.client, 'HTTPConnection') as transport:
            response = transport.return_value.getresponse.return_value
            response.status = 200
            response.read.return_value = b'{"mode":"learning"}'
            result = self.control.send('http://127.0.0.1:19094', 'POST', {'capture_enabled': True})
            self.assertEqual(result, {'mode': 'learning'})
            transport.assert_called_once_with('127.0.0.1', 19094, timeout=1.0)
            call = transport.return_value.request.call_args
            self.assertEqual(call.args[:2], ('POST', '/sentinel/control'))
            self.assertEqual(json.loads(call.args[2]), {'capture_enabled': True})
            response.status = 302
            with self.assertRaises(ValueError):
                self.control.send('http://127.0.0.1:19094', 'GET', None)

    def test_generated_commands_are_fixed_and_never_overwrite(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp).resolve() / 'preview'
            files = self.control.write_commands(root, 'claude', 'http://127.0.0.1:19094')
            self.assertEqual(len(files), 9)
            status = (root / 'sentinel-status.md').read_text()
            self.assertIn('disable-model-invocation: true', status)
            self.assertIn('/sentinel_control.py', status)
            self.assertNotIn('$ARGUMENTS', status)
            self.assertNotIn('$1', status)
            self.assertIn('http://127.0.0.1:19094', status)
            with self.assertRaises(FileExistsError):
                self.control.write_commands(root, 'claude', 'http://127.0.0.1:19094')
            other = Path(tmp).resolve() / 'codex-preview'
            self.control.write_commands(other, 'codex', 'http://127.0.0.1:19090')
            self.assertNotIn('disable-model-invocation', (other / 'sentinel-status.md').read_text())

    def test_server_errors_and_response_limits_are_not_reported_as_success(self):
        with patch.object(self.control.http.client, 'HTTPConnection') as transport:
            response = transport.return_value.getresponse.return_value
            response.status = 409
            response.read.return_value = b'{"error":"hybrid unavailable"}'
            with self.assertRaisesRegex(ValueError, '409'):
                self.control.send('http://127.0.0.1:19090', 'POST', {'policy': 'quality'})
            response.status = 200
            response.read.return_value = b'x' * (self.control.MAX_RESPONSE_BYTES + 1)
            with self.assertRaises(ValueError):
                self.control.send('http://127.0.0.1:19090', 'GET', None)


if __name__ == '__main__':
    unittest.main()
