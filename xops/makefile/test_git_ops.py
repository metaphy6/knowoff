"""Tests for `make git` batch commit-message construction."""

from __future__ import annotations

import unittest

from git_ops import _build_batch_commit_message, _build_commit_message


def _row(run_id: str, summary: str, refs: str = "") -> dict:
    return {
        "ts_utc": "2026-09-09T12:00:00Z",
        "run_id": run_id,
        "agent": "copilot",
        "scope": "test",
        "action": "commit",
        "status": "completed",
        "summary": summary,
        "refs": refs,
        "commit_sha": "pending",
    }


class BuildBatchCommitMessageTest(unittest.TestCase):
    def test_single_group_matches_per_group_message(self) -> None:
        group = [_row("run-a", "feat(x): add thing", "a.go;b.go")]
        self.assertEqual(
            _build_batch_commit_message([group]),
            _build_commit_message(group),
        )

    def test_multi_group_uses_first_subject_and_lists_others(self) -> None:
        groups = [
            [_row("run-a", "feat(x): add thing")],
            [_row("run-b", "fix(y): correct thing")],
            [_row("run-c", "docs(z): explain thing")],
        ]
        msg = _build_batch_commit_message(groups)
        lines = msg.splitlines()
        self.assertEqual(lines[0], "feat(x): add thing")
        self.assertIn("Also includes:", lines)
        self.assertIn("  - fix(y): correct thing", lines)
        self.assertIn("  - docs(z): explain thing", lines)

    def test_multi_group_carries_every_run_id_marker_once(self) -> None:
        groups = [
            [_row("run-a", "feat(x): add thing")],
            [_row("run-b", "fix(y): correct thing")],
        ]
        msg = _build_batch_commit_message(groups)
        for marker in ("[run-a]", "[run-b]"):
            self.assertEqual(msg.count(marker), 1, marker)

    def test_multi_group_unions_refs_in_order_without_duplicates(self) -> None:
        groups = [
            [_row("run-a", "feat(x): add thing", "a.go;shared.go")],
            [_row("run-b", "fix(y): correct thing", "shared.go;b.go")],
        ]
        msg = _build_batch_commit_message(groups)
        lines = msg.splitlines()
        refs_at = lines.index("Refs:")
        self.assertEqual(
            lines[refs_at + 1 : refs_at + 4],
            ["  - a.go", "  - shared.go", "  - b.go"],
        )
        self.assertEqual(msg.count("shared.go"), 1)

    def test_multi_group_omits_refs_section_when_no_refs(self) -> None:
        groups = [
            [_row("run-a", "feat(x): add thing")],
            [_row("run-b", "fix(y): correct thing")],
        ]
        self.assertNotIn("Refs:", _build_batch_commit_message(groups))


if __name__ == "__main__":
    unittest.main()
