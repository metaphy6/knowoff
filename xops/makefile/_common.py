"""xops/makefile/_common.py — shared helpers for Make dispatchers.

stdlib-only. Cross-platform. Emoji conventions match xops/lib/log.sh.
"""

from __future__ import annotations

import os
import shutil
import subprocess
import sys
from pathlib import Path
from typing import Iterable, Sequence

# ── paths ────────────────────────────────────────────────────────────────
THIS_FILE = Path(__file__).resolve()
REPO_ROOT = THIS_FILE.parents[2]  # xops/makefile/_common.py → repo root
TRACKING_CSV = REPO_ROOT / "docs" / "tracking" / "tracking.csv"
TRACKING_APPEND_SH = REPO_ROOT / "xops" / "agent" / "tracking_append.sh"
ROADMAP_MD = REPO_ROOT / "docs" / "planning" / "ROADMAP.md"

# ── color / emoji ────────────────────────────────────────────────────────
_USE_COLOR = sys.stderr.isatty() and os.environ.get("NO_COLOR") is None

def _c(code: str) -> str:
    return code if _USE_COLOR else ""

RESET  = _c("\033[0m")
DIM    = _c("\033[2m")
BOLD   = _c("\033[1m")
RED    = _c("\033[31m")
GREEN  = _c("\033[32m")
YELLOW = _c("\033[33m")
BLUE   = _c("\033[34m")
CYAN   = _c("\033[36m")

E_OK   = "✅"
E_ERR  = "❌"
E_WARN = "⚠️ "
E_INFO = "ℹ️ "
E_STEP = "▶️ "
E_GIT  = "🔧"
E_BOOK = "📋"
E_FIRE = "💥"
E_NOTE = "📝"
E_BOOM = "🚀"


def info(msg: str) -> None:
    print(f"{BLUE}{E_INFO}{RESET} {msg}", file=sys.stderr)

def ok(msg: str) -> None:
    print(f"{GREEN}{E_OK}{RESET} {msg}", file=sys.stderr)

def warn(msg: str) -> None:
    print(f"{YELLOW}{E_WARN}{RESET} {msg}", file=sys.stderr)

def err(msg: str) -> None:
    print(f"{RED}{E_ERR}{RESET} {msg}", file=sys.stderr)

def step(msg: str) -> None:
    print(f"{CYAN}{E_STEP}{RESET} {BOLD}{msg}{RESET}", file=sys.stderr)

def dim(msg: str) -> None:
    print(f"{DIM}{msg}{RESET}", file=sys.stderr)


# ── subprocess helpers ───────────────────────────────────────────────────
def run(cmd: Sequence[str], *, check: bool = True, cwd: Path | None = None) -> int:
    """Run a command, stream output, return exit code."""
    proc = subprocess.run(list(cmd), cwd=str(cwd) if cwd else None)
    if check and proc.returncode != 0:
        raise SystemExit(proc.returncode)
    return proc.returncode

def out(cmd: Sequence[str], *, check: bool = True, cwd: Path | None = None) -> str:
    """Run a command, capture stdout (text), return it."""
    proc = subprocess.run(
        list(cmd),
        cwd=str(cwd) if cwd else None,
        capture_output=True,
        text=True,
    )
    if check and proc.returncode != 0:
        err(f"command failed (rc={proc.returncode}): {' '.join(cmd)}")
        if proc.stderr:
            sys.stderr.write(proc.stderr)
        raise SystemExit(proc.returncode)
    return proc.stdout


# ── dispatch helper ──────────────────────────────────────────────────────
def dispatch(name: str, table: dict) -> None:
    """Pick a subcommand from sys.argv and run it. `table` is {name: callable}."""
    if len(sys.argv) < 2 or sys.argv[1] in ("-h", "--help"):
        print(f"usage: {name} <subcommand>", file=sys.stderr)
        print(f"  subcommands: {', '.join(sorted(table))}", file=sys.stderr)
        sys.exit(0 if len(sys.argv) >= 2 else 64)
    sub = sys.argv[1]
    if sub not in table:
        err(f"unknown {name} subcommand: {sub!r} (try: {', '.join(sorted(table))})")
        sys.exit(64)
    table[sub](sys.argv[2:])


def have(binary: str) -> bool:
    return shutil.which(binary) is not None
