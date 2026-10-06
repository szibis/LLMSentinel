#!/usr/bin/env python3
"""Opt-in local conversation evidence capture. No provider calls or global installs."""
import argparse
from datetime import datetime, timezone
import fcntl
import http.client
import json
import os
from pathlib import Path
import re
import shlex
import stat
import sys
import uuid
from urllib.parse import urlsplit

MAX_EVENT_BYTES = 1024 * 1024
MAX_TRANSCRIPT_BYTES = 8 * 1024 * 1024
MAX_ROWS = 512
MAX_TEXT = 32768
MAX_RECORD_BYTES = 512 * 1024
SECRET_PATTERNS = [
    r'(?i)\bBearer\s+[A-Za-z0-9._~+/=-]+',
    r'\b(?:sk-[A-Za-z0-9_-]{8,}|gh[pousr]_[A-Za-z0-9_]{8,}|github_pat_[A-Za-z0-9_]{8,}|AKIA[A-Z0-9]{16})\b',
    r'(?i)\b(?:api[_-]?key|access[_-]?token|password|secret|authorization)["\x27]?\s*[:=]\s*["\x27]?[^\s,"\x27}]+',
    r'-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----',
]


def redact(value):
    if isinstance(value, str):
        for pattern in SECRET_PATTERNS:
            value = re.sub(pattern, '[REDACTED]', value)
        return value[:MAX_TEXT]
    if isinstance(value, list):
        return [redact(x) for x in value[:MAX_ROWS]]
    if isinstance(value, dict):
        return {k: redact(v) for k, v in value.items()}
    return value


def canonical_path(value):
    path = Path(value)
    if not path.is_absolute():
        raise ValueError('capture paths must be absolute')
    # Callers must canonicalize normal /tmp and macOS /var aliases explicitly.
    # No implicit home expansion, globbing, or symlink traversal.
    if path.resolve() != path:
        raise ValueError('capture paths must be canonical and contain no symlinks')
    return path


def read_transcript(value, include_header=False):
    path = canonical_path(value)
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    with os.fdopen(fd, 'rb') as stream:
        info = os.fstat(stream.fileno())
        if not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid():
            raise ValueError('transcript must be a regular file owned by this user')
        start = max(0, info.st_size - MAX_TRANSCRIPT_BYTES)
        header = stream.readline(256 * 1024) if include_header else b''
        stream.seek(start)
        raw = stream.read(MAX_TRANSCRIPT_BYTES)
    lines = raw.splitlines()
    if start and lines:
        lines.pop(0)
    rows = []
    for line in lines[-MAX_ROWS:]:
        try:
            row = json.loads(line)
            if isinstance(row, dict):
                rows.append(row)
        except (ValueError, UnicodeDecodeError):
            continue
    if header:
        try:
            metadata = json.loads(header)
            if isinstance(metadata, dict) and metadata.get('type') == 'session_meta':
                rows.insert(0, metadata)
        except (ValueError, UnicodeDecodeError):
            pass
    return rows, bool(start or len(lines) > MAX_ROWS)


def text_content(content):
    if isinstance(content, str):
        return content
    if isinstance(content, list):
        return '\n'.join(x.get('text', '') for x in content[:MAX_ROWS]
                         if isinstance(x, dict) and x.get('type') == 'text' and isinstance(x.get('text'), str))
    return ''


def codex_usage(value):
    keys = ('input_tokens', 'cached_input_tokens', 'output_tokens', 'reasoning_output_tokens', 'total_tokens')
    if not isinstance(value, dict) or not all(isinstance(value.get(k), int) and not isinstance(value[k], bool) and value[k] >= 0 for k in keys):
        return None
    # Reject known synthetic context-fill counters and internally inconsistent data.
    if value['input_tokens'] + value['output_tokens'] != value['total_tokens'] or value['cached_input_tokens'] > value['input_tokens'] or value['reasoning_output_tokens'] > value['output_tokens']:
        return None
    return {k: value[k] for k in keys}


def enrich_codex(record, event):
    rows, clipped = read_transcript(event['transcript_path'], include_header=True)
    record['truncated'] |= clipped
    metadata = next((x.get('payload', {}) for x in rows if x.get('type') == 'session_meta'), {})
    if not isinstance(metadata, dict):
        return
    record['capture_metadata'] = {'cli_version': metadata.get('cli_version'), 'model_provider': metadata.get('model_provider')}
    if (metadata.get('cli_version') != '0.160.1' or metadata.get('id') != record['session_id']
            or not isinstance(record['session_id'], str) or not isinstance(record['turn_id'], str)):
        return
    # No home discovery: version/session identity comes from this explicit hook file only.
    scoped, current = [], None
    for row in rows:
        payload = row.get('payload')
        if not isinstance(payload, dict):
            continue
        if row.get('type') == 'event_msg' and payload.get('type') in ('task_started', 'turn_started'):
            current = payload.get('turn_id')
        elif row.get('type') == 'turn_context':
            current = payload.get('turn_id')
            if current == record['turn_id'] and isinstance(payload.get('model'), str):
                record['model'] = payload['model']
        if current == record['turn_id']:
            scoped.append(row)
    texts = {'user': [], 'assistant': []}
    for row in scoped:
        payload = row['payload']
        if row.get('type') == 'response_item' and payload.get('type') == 'message' and payload.get('role') in texts:
            content = payload.get('content')
            if isinstance(content, list):
                kind = 'input_text' if payload['role'] == 'user' else 'output_text'
                text = '\n'.join(x['text'] for x in content[:MAX_ROWS] if isinstance(x, dict) and x.get('type') == kind and isinstance(x.get('text'), str))
                if text and text not in texts[payload['role']]:
                    texts[payload['role']].append(text)
    if texts['user']:
        record['inputs'] = texts['user']
    if texts['assistant']:
        record['outputs'] = texts['assistant']
    responses = {}
    for row in rows:
        payload = row.get('payload', {})
        if (row.get('type') == 'token_usage_record' and isinstance(payload, dict)
                and payload.get('thread_id') == record['session_id']
                and payload.get('session_id') == metadata.get('session_id', metadata.get('id'))
                and payload.get('turn_id') == record['turn_id'] and isinstance(payload.get('response_id'), str)):
            usage = codex_usage(payload.get('usage'))
            if usage:
                responses[payload['response_id']] = usage
    usage, source, scope = None, None, None
    if responses:
        usage = {key: sum(value[key] for value in responses.values()) for key in next(iter(responses.values()))}
        source, scope = 'codex_rollout_token_usage_record', 'observed_turn_calls'
    else:
        counts = [row['payload'].get('info') for row in scoped if row.get('type') == 'event_msg' and row['payload'].get('type') == 'token_count']
        if counts and isinstance(counts[-1], dict):
            usage = codex_usage(counts[-1].get('last_token_usage'))
            source, scope = 'codex_rollout_token_count', 'last_call'
    if usage:
        record['usage'].update(input_tokens=usage['input_tokens'], output_tokens=usage['output_tokens'],
                               cache_read_input_tokens=usage['cached_input_tokens'],
                               reasoning_output_tokens=usage['reasoning_output_tokens'], total_tokens=usage['total_tokens'],
                               source=source, scope=scope, reported_calls=len(responses) if responses else None,
                               coverage='reported_calls_only', provenance='local_cli_record')


def normalize(client, event, billing_class='direct_unknown'):
    if billing_class not in ('direct_unknown', 'subscription', 'api'):
        raise ValueError('unsupported billing class')
    record = {'schema_version': 1, 'event_id': str(uuid.uuid4()),
              'timestamp': datetime.now(timezone.utc).isoformat(), 'source': 'client_hook',
              'client': client, 'model': None, 'role': 'conversation', 'run_id': None,
              'session_id': None, 'turn_id': None, 'inputs': [], 'outputs': [],
              'usage': {'input_tokens': None, 'output_tokens': None,
                        'cache_read_input_tokens': None, 'cache_creation_input_tokens': None,
                        'source': 'unavailable'},
              'latency_ms': None, 'latency_source': 'unavailable', 'truncated': False,
              'billing': {'class': billing_class, 'source': 'explicit_cli_option' if billing_class != 'direct_unknown' else 'unknown', 'cost_usd': None, 'cost_source': 'unavailable'},
              'quality': {'status': 'unscored', 'training_eligible': False},
              'pair_id': None, 'reference_event_id': None, 'local_event_id': None}
    if client == 'codex':
        if event.get('hook_event_name') in ('UserPromptSubmit', 'Stop'):
            name = event['hook_event_name']
            record.update(event_type=name, session_id=event.get('session_id'),
                          turn_id=event.get('turn_id'), model=event.get('model'))
            if name == 'UserPromptSubmit' and isinstance(event.get('prompt'), str):
                record['inputs'] = [event['prompt']]
            elif name == 'Stop' and isinstance(event.get('last_assistant_message'), str):
                record['outputs'] = [event['last_assistant_message']]
            if name == 'Stop' and event.get('transcript_path'):
                enrich_codex(record, event)
        elif event.get('type') != 'agent-turn-complete':
            raise ValueError('unsupported Codex notify event')
        else:
            record.update(event_type=event['type'], session_id=event.get('thread-id'), turn_id=event.get('turn-id'))
            inputs = event.get('input-messages', [])
            if not isinstance(inputs, list) or not all(isinstance(x, str) for x in inputs):
                raise ValueError('Codex input-messages must be strings')
            record['inputs'] = inputs[:MAX_ROWS]
            record['truncated'] = len(inputs) > MAX_ROWS
            output = event.get('last-assistant-message')
            if isinstance(output, str):
                record['outputs'] = [output]
    else:
        name = event.get('hook_event_name')
        if name not in ('UserPromptSubmit', 'Stop'):
            raise ValueError('unsupported Claude hook event')
        record.update(event_type=name, session_id=event.get('session_id'))
        if name == 'UserPromptSubmit':
            if not isinstance(event.get('prompt'), str):
                raise ValueError('Claude prompt must be a string')
            record['inputs'] = [event['prompt']]
        else:
            rows, clipped = read_transcript(event['transcript_path']) if event.get('transcript_path') else ([], False)
            record['truncated'] = clipped
            start = next((i for i in range(len(rows) - 1, -1, -1)
                          if rows[i].get('type') == 'user' and text_content(rows[i].get('message', {}).get('content'))), None)
            if start is not None:
                rows = rows[start:]
                record['inputs'] = [text_content(rows[0]['message']['content'])]
                record['turn_id'] = rows[0].get('uuid')
            else:
                rows = []  # Never pair an incomplete tail with an unrelated old input.
            messages = {}
            seen_text = set()
            for index, row in enumerate(rows):
                if row.get('type') == 'assistant' and isinstance(row.get('message'), dict):
                    message = row['message']
                    message_id = message.get('id') or row.get('uuid') or str(index)
                    text = text_content(message.get('content'))
                    if text and (message_id, text) not in seen_text:
                        record['outputs'].append(text)
                        seen_text.add((message_id, text))
                    messages[message_id] = row
            for row in messages.values():
                message = row['message']
                record['model'] = message.get('model') or record['model']
            if not record['outputs'] and isinstance(event.get('last_assistant_message'), str):
                record['outputs'] = [event['last_assistant_message']]
            if messages:
                for key in ('input_tokens', 'output_tokens', 'cache_read_input_tokens', 'cache_creation_input_tokens'):
                    values = [row['message'].get('usage', {}).get(key) for row in messages.values()]
                    if all(isinstance(v, int) and not isinstance(v, bool) and v >= 0 for v in values):
                        record['usage'][key] = sum(values)
                if any(record['usage'][key] is not None for key in ('input_tokens', 'output_tokens')):
                    record['usage']['source'] = 'claude_transcript_message_usage'
                try:
                    first = datetime.fromisoformat(rows[0]['timestamp'].replace('Z', '+00:00'))
                    last = datetime.fromisoformat(list(messages.values())[-1]['timestamp'].replace('Z', '+00:00'))
                    duration = (last - first).total_seconds() * 1000
                    if duration >= 0:
                        record['latency_ms'] = duration
                        record['latency_source'] = 'transcript_turn_span'
                except (KeyError, ValueError, TypeError):
                    pass
    record['truncated'] |= any(len(x) > MAX_TEXT for x in record['inputs'] + record['outputs'])
    if len(record['inputs']) + len(record['outputs']) > 64:
        record['inputs'] = record['inputs'][-32:]
        record['outputs'] = record['outputs'][-32:]
        record['truncated'] = True
    return redact(record)


def append_private(value, record):
    path = canonical_path(value)
    payload = (json.dumps(record, ensure_ascii=True, separators=(',', ':')) + '\n').encode()
    if len(payload) > MAX_RECORD_BYTES:
        raise ValueError('capture record exceeds limit')
    fd = os.open(path, os.O_WRONLY | os.O_APPEND | os.O_CREAT | os.O_NOFOLLOW | os.O_NONBLOCK, 0o600)
    try:
        fcntl.flock(fd, fcntl.LOCK_EX)
        info = os.fstat(fd)
        if not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid() or info.st_nlink != 1:
            raise ValueError('output must be a regular singly linked file owned by this user')
        os.fchmod(fd, 0o600)
        view = memoryview(payload)
        while view:
            count = os.write(fd, view)
            if not count:
                raise OSError('capture write made no progress')
            view = view[count:]
        os.fsync(fd)
    finally:
        os.close(fd)


def decode_event(raw):
    if len(raw.encode('utf-8')) > MAX_EVENT_BYTES:
        raise ValueError('hook input exceeds limit')
    event = json.loads(raw)
    if not isinstance(event, dict):
        raise ValueError('hook input must be an object')
    return event


def collector_address(url):
    address = urlsplit(url)
    if (address.scheme != 'http' or address.hostname not in ('127.0.0.1', '::1')
            or address.username or address.password or address.query or address.fragment):
        raise ValueError('collector must be a literal HTTP loopback URL without credentials')
    _ = address.port
    return address


def deliver(url, record):
    address = collector_address(url)
    connection = http.client.HTTPConnection(address.hostname, address.port or 80, timeout=0.3)
    try:
        # Direct connection: no env proxy lookup, DNS, redirects, or response body read.
        connection.request('POST', address.path or '/', json.dumps(record).encode(), {'Content-Type': 'application/json'})
        return 200 <= connection.getresponse().status < 300
    except (OSError, http.client.HTTPException):
        return False
    finally:
        connection.close()


def preview_config(client, output, native_hooks=False, collector=None, billing_class='direct_unknown'):
    path = canonical_path(output)
    command = [sys.executable, str(Path(__file__).resolve()), '--client', client, '--output', str(path)]
    if billing_class != 'direct_unknown':
        command += ['--billing-class', billing_class]
    if collector:
        collector_address(collector)
        command += ['--collector', collector]
    if client == 'codex' and not native_hooks:
        return 'notify = ' + json.dumps(command) + '\n'
    return {'hooks': {name: [{'hooks': [{'type': 'command', 'command': shlex.join(command), 'timeout': 5}]}]
                      for name in ('UserPromptSubmit', 'Stop')}}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--client', choices=('claude', 'codex'), required=True)
    parser.add_argument('--output', required=True, help='explicit canonical absolute capture JSONL file')
    parser.add_argument('--preview-config', action='store_true', help='print configuration only; install nothing')
    parser.add_argument('--native-hooks', action='store_true', help='preview Codex hooks.json instead of notify TOML')
    parser.add_argument('--collector', help='optional literal HTTP loopback URL; private spool written first')
    parser.add_argument('--billing-class', choices=('direct_unknown', 'subscription', 'api'), default='direct_unknown', help='explicit billing provenance; never inferred from credentials or token counts')
    parser.add_argument('event', nargs='?', help='Codex notify appends JSON as its final argv argument; stdin otherwise')
    args = parser.parse_args()
    try:
        if args.collector:
            collector_address(args.collector)
        if args.preview_config:
            config = preview_config(args.client, args.output, args.native_hooks, args.collector, args.billing_class)
            print(config if isinstance(config, str) else json.dumps(config, indent=2))
        else:
            raw = args.event if args.event is not None else sys.stdin.buffer.read(MAX_EVENT_BYTES + 1).decode('utf-8')
            record = normalize(args.client, decode_event(raw), args.billing_class)
            append_private(args.output, record)
            if args.collector and not deliver(args.collector, record):
                print('Sentinel collector unavailable; private capture spool retained', file=sys.stderr)
        return 0
    except (OSError, ValueError, TypeError, KeyError, RecursionError):
        # Never echo private event data, and never emit Claude hook decision JSON.
        print('Sentinel capture skipped: invalid event or inaccessible private file', file=sys.stderr)
        return 1


if __name__ == '__main__':
    sys.exit(main())
