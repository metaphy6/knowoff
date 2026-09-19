"""Tests for Flutter web-server process detection."""

from __future__ import annotations

import unittest
from unittest.mock import patch

from flutter_web_ops import REPO_ROOT, cmd_run, is_flutter_web_server_command


class FlutterWebOpsTest(unittest.TestCase):
    def test_run_preserves_interactive_debug_server_and_uses_selected_sdk(self):
        with patch('flutter_web_ops.flutter', return_value='/project/flutter'), patch('flutter_web_ops.run') as run:
            cmd_run([])
        run.assert_called_once_with(['/project/flutter', 'run', '-d', 'web-server',
                                     '--web-hostname=0.0.0.0', '--web-port=8000', '--no-web-resources-cdn'],
                                    cwd=REPO_ROOT / 'client')

    def test_identifies_flutter_web_server_commands(self) -> None:
        self.assertTrue(
            is_flutter_web_server_command(
                ("/opt/flutter/bin/cache/dart-sdk/bin/dart", "flutter_tools.snapshot", "run", "-d", "web-server")
            )
        )
        self.assertTrue(
            is_flutter_web_server_command(
                ("flutter", "run", "--device-id=web-server")
            )
        )
        self.assertFalse(
            is_flutter_web_server_command(("flutter", "build", "web"))
        )
        self.assertFalse(
            is_flutter_web_server_command(("python3", "-m", "http.server", "8000"))
        )


if __name__ == "__main__":
    unittest.main()
