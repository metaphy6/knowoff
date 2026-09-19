"""Reproducible, loopback-only five-mode prototype using the real runtime."""
from __future__ import annotations

import os
from pathlib import Path
import secrets
import shutil

from _common import REPO_ROOT, dispatch, info, run

COMPOSE_FILE = REPO_ROOT / 'infra/compose/playtest.yaml'
ENV_FILE = REPO_ROOT / 'docs/tracking/state/playtest.env'


def ensure_environment():
    """Generate private local credentials once; never print them or rotate a DB password."""
    ENV_FILE.parent.mkdir(parents=True, exist_ok=True)
    try:
        fd = os.open(ENV_FILE, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    except FileExistsError:
        return
    with os.fdopen(fd, 'w') as stream:
        for name in ('KNOWOFF_DB_PASSWORD', 'KNOWOFF_REDIS_PASSWORD', 'KNOWOFF_JWT_KEY'):
            stream.write(f'{name}={secrets.token_hex(32)}\n')


def compose_command(*args):
    return ['docker', 'compose', '--project-name', 'knowoff-playtest',
            '--env-file', str(ENV_FILE), '-f', str(COMPOSE_FILE), *args]


def compose(*args):
    ensure_environment()
    run(compose_command(*args), cwd=REPO_ROOT)


def flutter():
    local = REPO_ROOT / '.tools/flutter/bin/flutter'
    executable = str(local) if local.is_file() else shutil.which('flutter')
    if not executable:
        raise SystemExit('Flutter SDK required: use the project SDK or put a compatible Flutter on PATH.')
    return executable


def build_web():
    run([flutter(), 'build', 'web', '--release', '--no-web-resources-cdn'], cwd=REPO_ROOT / 'client')


def cmd_up(_args):
    ensure_environment()
    build_web()
    compose('up', '--build', '-d', '--wait', '--wait-timeout', '180', 'server', 'web')
    info('Private synthetic prototype; no earned currency or progression. Players:')
    for port in range(8001, 8007):
        info(f'http://localhost:{port} (one origin per player)')


def cmd_down(_args):
    compose('down')


def cmd_reset(_args):
    compose('down', '--volumes')
    info('Only knowoff-playtest data removed. Clear site data on ports 8001–8006, then make playtest.up.')


def cmd_web(_args):
    build_web()
    info('Reload player tabs; use a hard refresh if a previous build remains cached.')


def cmd_android(_args):
    run([flutter(), 'build', 'apk', '--debug'], cwd=REPO_ROOT / 'client')
    info('APK: client/build/app/outputs/flutter-apk/app-debug.apk; Android emulator uses host port 8080.')


if __name__ == '__main__':
    dispatch('playtest_ops', {'up': cmd_up, 'down': cmd_down, 'reset': cmd_reset,
                              'web': cmd_web, 'android': cmd_android})
