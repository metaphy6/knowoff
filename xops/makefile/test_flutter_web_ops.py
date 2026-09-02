"""Tests for Flutter web-server process detection."""

from __future__ import annotations

import unittest

from flutter_web_ops import is_flutter_web_server_command


class FlutterWebOpsTest(unittest.TestCase):
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
