"""Private prototype launch/reset contract; infrastructure checks require Docker."""
from pathlib import Path
import json
import os
import subprocess
import time
from urllib.request import urlopen
from urllib.error import HTTPError
import sys
import tempfile
import unittest
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parent))


class PlaytestOpsTests(unittest.TestCase):
    def module(self):
        import playtest_ops
        return playtest_ops

    def test_secrets_are_private_stable_and_generated(self):
        module = self.module()
        with tempfile.TemporaryDirectory() as temporary:
            path = Path(temporary) / 'playtest.env'
            with patch.object(module, 'ENV_FILE', path):
                module.ensure_environment()
                first = path.read_text()
                module.ensure_environment()
                self.assertEqual(first, path.read_text())
                self.assertEqual(path.stat().st_mode & 0o777, 0o600)
                self.assertEqual(len(first.splitlines()), 3)
                self.assertTrue(all(len(line.split('=', 1)[1]) >= 32 for line in first.splitlines()))

    def test_up_builds_web_before_waiting_for_private_services(self):
        module = self.module()
        with patch.object(module, 'ensure_environment'), patch.object(module, 'build_web') as build, patch.object(module, 'compose') as compose:
            module.cmd_up([])
        build.assert_called_once_with()
        compose.assert_called_once_with('up', '--build', '-d', '--wait', '--wait-timeout', '180', 'server', 'web')

    def test_down_preserves_data_reset_only_deletes_named_project_volumes(self):
        module = self.module()
        with patch.object(module, 'compose') as compose:
            module.cmd_down([])
            compose.assert_called_with('down')
            module.cmd_reset([])
            compose.assert_called_with('down', '--volumes')
        command = module.compose_command('down', '--volumes')
        self.assertIn('knowoff-playtest', command)
        self.assertIn(str(module.COMPOSE_FILE), command)
        self.assertNotIn('--remove-orphans', command)

    def test_workspace_sdk_is_preferred(self):
        module = self.module()
        with patch.object(module.Path, 'is_file', return_value=True):
            self.assertEqual(module.flutter(), str(module.REPO_ROOT / '.tools/flutter/bin/flutter'))


class PlaytestInfrastructureTests(unittest.TestCase):
    def test_resolved_compose_is_private_and_synthetic(self):
        import playtest_ops as module
        environment = os.environ | {name: 'isolated-config-test' for name in
                                    ('KNOWOFF_DB_PASSWORD', 'KNOWOFF_REDIS_PASSWORD', 'KNOWOFF_JWT_KEY')}
        result = subprocess.run(['docker', 'compose', '-f', str(module.COMPOSE_FILE),
                                 'config', '--format', 'json'], env=environment,
                                check=True, capture_output=True, text=True, timeout=30)
        model = json.loads(result.stdout)
        self.assertEqual(model['name'], 'knowoff-playtest')
        self.assertEqual(set(model['volumes']), {'playtest_pg'})
        services = model['services']
        self.assertEqual(set(services), {'postgres', 'redis', 'migrate', 'server', 'web'})
        for service in services.values():
            for port in service.get('ports', []):
                self.assertEqual(port['host_ip'], '127.0.0.1')
        self.assertEqual({p['published'] for p in services['web']['ports']},
                         {str(p) for p in range(8001, 8007)})
        self.assertEqual([p['published'] for p in services['server']['ports']], ['8080'])
        self.assertEqual(services['server']['environment']['KNOWOFF_TEXT_PROTOTYPE_PACK'], '/prototype')
        self.assertEqual(services['server']['environment']['KNOWOFF_CONFIG'], 'configs/playtest.yaml')
        self.assertFalse(services['postgres'].get('ports'))
        self.assertFalse(services['redis'].get('ports'))
        mounts = {v['target']: v for v in services['server']['volumes']}
        self.assertTrue(mounts['/prototype']['read_only'])
        self.assertTrue(mounts['/prototype']['source'].endswith('/server/pkg/media/testdata/text-en'))
        self.assertNotIn('/prototype', {v['target'] for v in services['web']['volumes']})

    def test_real_web_host_serves_bootstrap_module_as_javascript(self):
        import playtest_ops as module
        with tempfile.TemporaryDirectory() as temporary:
            Path(temporary).chmod(0o755)
            Path(temporary, 'text_generation.mjs').write_text('export const generation = 2;')
            result = subprocess.run(['docker', 'run', '--rm', '-d', '-p', '127.0.0.1::80',
                                     '-v', f'{temporary}:/usr/share/nginx/html:ro',
                                     '-v', f'{module.REPO_ROOT}/infra/compose/playtest-web.conf:/etc/nginx/conf.d/default.conf:ro',
                                     'nginx:alpine'], check=True, capture_output=True, text=True, timeout=60)
            container = result.stdout.strip()
            try:
                published = subprocess.run(['docker', 'port', container, '80'], check=True,
                                           capture_output=True, text=True, timeout=10).stdout.strip()
                deadline = time.monotonic() + 15
                while True:
                    try:
                        with urlopen(f'http://{published}/text_generation.mjs?generation=2', timeout=2) as response:
                            self.assertEqual(response.headers.get_content_type(), 'application/javascript')
                            self.assertEqual(response.headers['Cache-Control'], 'no-store')
                            self.assertIn(b'export const generation', response.read())
                        break
                    except HTTPError as error:
                        error.close()
                        raise
                    except OSError:
                        if time.monotonic() >= deadline:
                            raise
                        time.sleep(0.1)
            finally:
                subprocess.run(['docker', 'stop', container], check=True,
                               capture_output=True, text=True, timeout=30)


if __name__ == '__main__':
    unittest.main()
