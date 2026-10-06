#!/usr/bin/env python3
"""Control an already configured Sentinel over local HTTP; no provider calls."""
import argparse
import http.client
import json
import os
from pathlib import Path
import shlex
import sys
from urllib.parse import urlsplit

MAX_RESPONSE_BYTES = 512 * 1024
DEFAULT_ENDPOINT = 'http://127.0.0.1:19090'
FIXED_COMMANDS = {
    'sentinel-status': ('status',),
    'sentinel-training-on': ('training', 'on'),
    'sentinel-training-off': ('training', 'off'),
    'sentinel-policy-local': ('policy', 'local-only'),
    'sentinel-policy-balanced': ('policy', 'balanced'),
    'sentinel-policy-quality': ('policy', 'quality'),
    'sentinel-profile-haiku': ('profile', 'haiku', '1024'),
    'sentinel-profile-sonnet': ('profile', 'sonnet', '4096'),
    'sentinel-profile-opus': ('profile', 'opus', '8192'),
}


def endpoint_address(endpoint):
    address = urlsplit(endpoint)
    if (address.scheme != 'http' or address.hostname not in ('127.0.0.1', '::1')
            or address.username or address.password or address.path not in ('', '/')
            or address.query or address.fragment):
        raise ValueError('endpoint must be a literal HTTP loopback origin without credentials')
    _ = address.port
    return address


def request_spec(arguments):
    if arguments == ['status']:
        return 'GET', None
    if len(arguments) == 2 and arguments[0] == 'training' and arguments[1] in ('on', 'off'):
        return 'POST', {'capture_enabled': arguments[1] == 'on'}
    if len(arguments) == 2 and arguments[0] == 'policy' and arguments[1] in ('local-only', 'balanced', 'quality'):
        return 'POST', {'policy': arguments[1]}
    if len(arguments) == 3 and arguments[0] == 'profile' and arguments[1] in ('haiku', 'sonnet', 'opus'):
        try:
            budget = int(arguments[2])
        except (ValueError, TypeError):
            raise ValueError('profile budget must be an integer') from None
        if 1 <= budget <= 32768:
            return 'POST', {'role_budgets': {arguments[1]: budget}}
    raise ValueError('use status; training on|off; policy local-only|balanced|quality; profile haiku|sonnet|opus 1..32768')


def send(endpoint, method, payload):
    address = endpoint_address(endpoint)
    connection = http.client.HTTPConnection(address.hostname, address.port or 80, timeout=1.0)
    try:
        body = json.dumps(payload).encode() if payload is not None else None
        connection.request(method, '/sentinel/control', body, {'Content-Type': 'application/json'})
        response = connection.getresponse()
        raw = response.read(MAX_RESPONSE_BYTES + 1)
        if len(raw) > MAX_RESPONSE_BYTES:
            raise ValueError('Sentinel control response exceeds limit')
        if not 200 <= response.status < 300:
            # Keep errors bounded and avoid echoing arbitrary server response text.
            raise ValueError(f'Sentinel control HTTP {response.status}; request rejected')
        result = json.loads(raw)
        if not isinstance(result, dict):
            raise ValueError('Sentinel control response must be a JSON object')
        return result
    finally:
        connection.close()


def command_assets(client, endpoint):
    endpoint_address(endpoint)
    if client not in ('claude', 'codex'):
        raise ValueError('client must be claude or codex')
    assets = {}
    for name, arguments in FIXED_COMMANDS.items():
        description = 'Sentinel ' + ' '.join(arguments)
        frontmatter = '---\ndescription: ' + description + '\n'
        if client == 'claude':
            frontmatter += 'disable-model-invocation: true\n'
        frontmatter += '---\n\n'
        command = shlex.join(['rtk', sys.executable, str(Path(__file__).resolve()), '--endpoint', endpoint, *arguments])
        assets[name + '.md'] = (frontmatter +
            'Run the following exact local command once using the shell tool, then report its returned JSON or error briefly.\n'
            'Do not add or interpolate arguments. Do not change provider/authentication settings, start another service, or retry a rejected mutation.\n'
            'These instructions are model-assisted and may consume CLI model tokens; the Python command itself calls no model provider.\n\n'
            '```sh\n' + command + '\n```\n')
    return assets


def write_commands(directory, client, endpoint):
    path = Path(directory)
    if not path.is_absolute() or path.resolve() != path:
        raise ValueError('preview directory must be canonical and absolute without symlinks')
    assets = command_assets(client, endpoint)
    path.mkdir(mode=0o700, parents=True, exist_ok=True)
    targets = [path / name for name in assets]
    if any(target.exists() or target.is_symlink() for target in targets):
        raise FileExistsError('command preview exists; choose a new explicit directory')
    for target in targets:
        fd = os.open(target, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
        with os.fdopen(fd, 'w') as stream:
            stream.write(assets[target.name])
    return targets


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--endpoint', default=DEFAULT_ENDPOINT, help='literal HTTP loopback origin; learning collector usually port 19094')
    parser.add_argument('--client', choices=('claude', 'codex'), default='claude', help='format for preview commands')
    parser.add_argument('--write-commands', metavar='DIR', help='write reviewable Markdown into this explicit directory only; install nothing')
    parser.add_argument('--preview-commands', action='store_true', help='print all reviewable command contents as JSON; install nothing')
    parser.add_argument('arguments', nargs='*')
    args = parser.parse_args()
    try:
        if args.write_commands or args.preview_commands:
            if args.arguments or (args.write_commands and args.preview_commands):
                raise ValueError('command generation cannot be combined with control actions')
            if args.write_commands:
                result = {'written': [str(x) for x in write_commands(args.write_commands, args.client, args.endpoint)], 'installed': False}
            else:
                result = command_assets(args.client, args.endpoint)
        else:
            method, payload = request_spec(args.arguments)
            result = send(args.endpoint, method, payload)
        print(json.dumps(result, indent=2, sort_keys=True))
        return 0
    except (OSError, ValueError, http.client.HTTPException) as error:
        print(f'Sentinel control failed: {error}', file=sys.stderr)
        return 1


if __name__ == '__main__':
    sys.exit(main())
