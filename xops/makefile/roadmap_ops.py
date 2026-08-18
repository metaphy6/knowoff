"""xops/makefile/roadmap_ops.py — `make roadmap.status`.

Parses Markdown checkboxes in docs/planning/ROADMAP.md and prints a
per-section count of `[ ]` vs `[x]`.
"""

from __future__ import annotations

import re
import sys
from typing import List

from _common import (
    BOLD, DIM, GREEN, RESET, YELLOW, ROADMAP_MD, err, info, ok, step,
)


HEADING_RE = re.compile(r"^(#+)\s+(.*)$")
CHECKBOX_RE = re.compile(r"^\s*-\s*\[([ xX])\]\s*(.*)$")


def cmd_status(_args: List[str]) -> None:
    step("🗺  make roadmap.status")
    if not ROADMAP_MD.exists():
        err(f"{ROADMAP_MD} not found — run `make scaffold` or create the file")
        sys.exit(66)

    current_section = "(no heading)"
    by_section: dict[str, list[int]] = {}  # section -> [done, open]
    total_done = total_open = 0

    for raw in ROADMAP_MD.read_text(encoding="utf-8").splitlines():
        h = HEADING_RE.match(raw)
        if h:
            depth = len(h.group(1))
            if depth <= 3:
                current_section = h.group(2).strip()
                by_section.setdefault(current_section, [0, 0])
            continue
        c = CHECKBOX_RE.match(raw)
        if c:
            mark = c.group(1).lower()
            counts = by_section.setdefault(current_section, [0, 0])
            if mark == "x":
                counts[0] += 1; total_done += 1
            else:
                counts[1] += 1; total_open += 1

    total = total_done + total_open
    if total == 0:
        info("no checkboxes found in ROADMAP.md")
        return

    pct = (total_done * 100) // total
    info(f"{BOLD}overall: {total_done}/{total} ({pct}%) complete{RESET}")
    print()
    for sec, (d, o) in by_section.items():
        if d + o == 0:
            continue
        color = GREEN if o == 0 else YELLOW
        bar_total = d + o
        spct = (d * 100) // bar_total
        print(f"  {color}{d:>3}/{bar_total:<3} ({spct:>3}%){RESET}  {sec}")
    if total_open == 0:
        ok("all roadmap items checked")


TABLE = {"status": cmd_status}

if __name__ == "__main__":
    from _common import dispatch
    dispatch("roadmap_ops", TABLE)
