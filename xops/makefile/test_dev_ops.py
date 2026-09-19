"""Manual debug startup and explicit private bot companions."""
import importlib
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parent))


class DevOpsTests(unittest.TestCase):
    def setUp(self):
        self.ops = importlib.import_module('dev_ops')

    def test_key_is_private_stable_and_not_in_command(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'dev.env'
            with patch.object(self.ops, 'ENV_FILE', path):
                self.ops.ensure_environment()
                first = path.read_text()
                self.ops.ensure_environment()
                self.assertEqual(first, path.read_text())
                self.assertEqual(path.stat().st_mode & 0o777, 0o600)
                key = first.strip().split('=', 1)[1]
                self.assertGreaterEqual(len(key), 32)
                self.assertNotIn(key, ' '.join(self.ops.compose_command('up')))

    def test_up_and_rebuild_keep_manual_overlay_and_down_preserves_data(self):
        with patch.object(self.ops, 'ensure_environment'), patch.object(self.ops, 'run') as run:
            self.ops.cmd_up([])
            self.assertEqual(run.call_args.args[0], self.ops.compose_command('up', '--build', '-d', '--wait', '--wait-timeout', '180'))
            self.ops.cmd_rebuild([])
            self.assertEqual(run.call_args.args[0], self.ops.compose_command('up', '--build', '-d', '--force-recreate', '--wait', '--wait-timeout', '180', 'server'))
            self.ops.cmd_down([])
            self.assertEqual(run.call_args.args[0], self.ops.compose_command('down'))

    def test_bots_get_key_only_through_environment_and_validate_room_first(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'dev.env'
            path.write_text('KNOWOFF_DEV_BOT_KEY=private-test-value\n')
            with patch.object(self.ops, 'ENV_FILE', path), patch.dict(os.environ, {'ROOM': 'ABC123', 'COUNT': '3'}), patch.object(self.ops.subprocess, 'run') as run:
                run.return_value.returncode = 0
                self.ops.cmd_bots([])
                command = run.call_args.args[0]
                self.assertEqual(command, ['go', 'run', '.', '-room', 'ABC123', '-count', '3'])
                self.assertEqual(run.call_args.kwargs['env']['KNOWOFF_DEV_BOT_KEY'], 'private-test-value')
                self.assertNotIn('private-test-value', ' '.join(command))
                for room, count in [('', '3'), ('bad;room', '3'), ('ABC123', '0'), ('ABC123', '6')]:
                    with self.subTest(room=room, count=count), patch.dict(os.environ, {'ROOM': room, 'COUNT': count}):
                        with self.assertRaises(SystemExit):
                            self.ops.cmd_bots([])
                self.assertEqual(run.call_count, 1)

    def test_manual_compose_enables_only_private_runtime(self):
        with tempfile.TemporaryDirectory() as directory:
            with patch.object(self.ops, 'ENV_FILE', Path(directory) / 'dev.env'):
                self.ops.ensure_environment()
                result = subprocess.run(self.ops.compose_command('config', '--format', 'json'), check=True, capture_output=True, text=True, timeout=30)
        model = json.loads(result.stdout)
        self.assertEqual(model['name'], 'knowoff')
        server = model['services']['server']
        self.assertEqual(server['environment']['KNOWOFF_TEXT_PROTOTYPE_PACK'], '/prototype')
        self.assertEqual(server['environment']['KNOWOFF_CONFIG'], 'configs/local.yaml')
        self.assertTrue(server['environment']['KNOWOFF_DEV_BOT_KEY'])
        mounts = {m['target']: m for m in server['volumes']}
        self.assertTrue(mounts['/prototype']['read_only'])
        self.assertTrue(mounts['/app']['source'].endswith('/server'))
        for service in model['services'].values():
            for port in service.get('ports', []):
                self.assertEqual(port['host_ip'], '127.0.0.1')
        self.assertNotIn('KNOWOFF_TEXT_PROTOTYPE_PACK', model['services']['migrate']['environment'])


if __name__ == '__main__':
    unittest.main()
