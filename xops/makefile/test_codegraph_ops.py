"""Tests for CodeGraph operation reporting."""

from __future__ import annotations

import unittest
from unittest.mock import patch

from codegraph_ops import _is_noop_sync, cmd_update


class CodegraphOpsTest(unittest.TestCase):
    def test_identifies_an_already_up_to_date_sync(self) -> None:
        self.assertTrue(_is_noop_sync("\nAlready up to date\n"))
        self.assertFalse(_is_noop_sync("\nIndexed 24 files\n"))

    @patch("codegraph_ops.ok")
    @patch("codegraph_ops._run_codegraph", return_value=(0, "Already up to date"))
    @patch("codegraph_ops._action", return_value="sync")
    def test_reports_an_unchanged_index(self, _action, _run_codegraph, ok) -> None:
        cmd_update([])

        ok.assert_called_once_with("CodeGraph: index already up to date")


if __name__ == "__main__":
    unittest.main()
