"""The manual Flutter/Air debug stack and explicit private bot companions."""
from __future__ import annotations

import os
import re
import secrets
import subprocess

from _common import REPO_ROOT, dispatch, info, run

ENV_FILE = REPO_ROOT / 'docs/tracking/state/dev.env'
COMPOSE_DIR = REPO_ROOT / 'infra/compose'


def ensure_environment():
    ENV_FILE.parent.mkdir(parents=True, exist_ok=True)
    try:
        fd = os.open(ENV_FILE, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    except FileExistsError:
        return
    with os.fdopen(fd, 'w') as stream:
        stream.write(f'KNOWOFF_DEV_BOT_KEY={secrets.token_hex(32)}\n')


def compose_command(*args):
    return ['docker', 'compose', '--project-name', 'knowoff',
            '--env-file', str(ENV_FILE), '-f', str(COMPOSE_DIR / 'docker-compose.yaml'),
            '-f', str(COMPOSE_DIR / 'manual.yaml'), '--profile', 'core', *args]


def compose(*args):
    ensure_environment()
    run(compose_command(*args), cwd=REPO_ROOT)


def cmd_up(_args):
    compose('up', '--build', '-d', '--wait', '--wait-timeout', '180')
    info('Private synthetic modes enabled; no earned currency or progression.')
    info('Run make web.run, create a room, then make bots ROOM=<code> COUNT=3 (or 5).')


def cmd_down(_args):
    compose('down')


def cmd_rebuild(_args):
    compose('up', '--build', '-d', '--force-recreate', '--wait', '--wait-timeout', '180', 'server')


def cmd_bots(_args):
    room = os.environ.get('ROOM', '').strip().upper()
    count = os.environ.get('COUNT', '3')
    if not re.fullmatch(r'[A-Z0-9]{6}', room) or count not in {'1', '2', '3', '4', '5'}:
        raise SystemExit('Use make bots ROOM=<room-code> COUNT=3 (or 5 for a six-seat table).')
    if not ENV_FILE.is_file():
        raise SystemExit('Run make up before adding private room bots.')
    entries = dict(line.split('=', 1) for line in ENV_FILE.read_text().splitlines() if '=' in line)
    key = entries.get('KNOWOFF_DEV_BOT_KEY', '')
    if not key:
        raise SystemExit('Local bot credential missing; restore dev.env before starting bots.')
    environment = os.environ | {'KNOWOFF_DEV_BOT_KEY': key}
    result = subprocess.run(['go', 'run', '.', '-room', room, '-count', count],
                            cwd=REPO_ROOT / 'tools/gamebot', env=environment)
    if result.returncode:
        raise SystemExit(result.returncode)


if __name__ == '__main__':
    dispatch('dev_ops', {'up': cmd_up, 'down': cmd_down, 'rebuild': cmd_rebuild, 'bots': cmd_bots})
