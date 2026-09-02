"""xops/makefile/codegraph_ops.py - `make codeg`.

Initializes or synchronizes the repository-local CodeGraph index. Uses the
installed CLI when available and falls back to npx without invoking a shell.
"""

from __future__ import annotations

import subprocess
import sys
from typing import List

from _common import REPO_ROOT, err, have, ok, step, warn


CODEGRAPH_DB = REPO_ROOT / ".codegraph" / "codegraph.db"
CODEGRAPH_PACKAGE = "@colbymchenry/codegraph"


def _is_noop_sync(output: str) -> bool:
    return "already up to date" in output.casefold()


def _action() -> str:
    return "sync" if CODEGRAPH_DB.is_file() else "init"


def _run(command: List[str]) -> tuple[int, str]:
    process = subprocess.run(
        command,
        check=False,
        cwd=REPO_ROOT,
        stderr=subprocess.STDOUT,
        stdout=subprocess.PIPE,
        text=True,
    )
    sys.stdout.write(process.stdout)
    sys.stdout.flush()
    return process.returncode, process.stdout


def _run_codegraph(action: str) -> tuple[int, str]:
    if have("codegraph"):
        step(f"CodeGraph: {action} local index with codegraph")
        return _run(["codegraph", action, "."])

    warn("CodeGraph: CLI not found; trying npx fallback")
    if not have("npx"):
        err("CodeGraph: neither codegraph nor npx is available")
        return 1, ""

    step(f"CodeGraph: {action} local index with npx")
    return _run(["npx", "-y", CODEGRAPH_PACKAGE, action, "."])


def cmd_update(_args: List[str]) -> None:
    step("make codeg")
    action = _action()
    result, output = _run_codegraph(action)

    if result != 0 and have("codegraph") and have("npx"):
        warn("CodeGraph: installed CLI failed; trying npx fallback")
        result, output = _run(["npx", "-y", CODEGRAPH_PACKAGE, action, "."])

    if result != 0:
        err(f"CodeGraph: {action} failed (exit code {result})")
        sys.exit(result)

    if action == "sync" and _is_noop_sync(output):
        ok("CodeGraph: index already up to date")
        return

    ok("CodeGraph: index updated successfully")


if __name__ == "__main__":
    if len(sys.argv) > 1 and sys.argv[1] in ("-h", "--help"):
        print("usage: codegraph_ops.py update")
        sys.exit(0)
    if len(sys.argv) != 2 or sys.argv[1] != "update":
        err("usage: codegraph_ops.py update")
        sys.exit(64)
    cmd_update([])
