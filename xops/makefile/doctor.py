"""xops/makefile/doctor.py — `make doctor`.

Verifies the framework files exist, are wired correctly, and have valid
syntax. Returns non-zero if any check fails. Read-only.
"""

from __future__ import annotations

import json
import os
import sys
from pathlib import Path
from typing import List

from _common import (
    REPO_ROOT, TRACKING_CSV, dim, err, info, ok, step, warn,
)


CHECKS_REQUIRED = [
    "AGENTS.md",
    "CLAUDE.md",
    "GEMINI.md",
    "CONVENTIONS.md",
    "README.md",
    "LICENSE",
    "Makefile",
    ".gitignore",
    ".github/copilot-instructions.md",
    "docs/tracking/tracking.csv",
    "docs/tracking/tracking.schema.md",
    "docs/tracking/README.md",
    "xops/lib/log.sh",
    "xops/agent/tracking_append.sh",
    "xops/agent/safe-run.sh",
    "xops/agent/session-bootstrap.sh",
    "xops/makefile/_common.py",
    "xops/makefile/git_ops.py",
    "xops/makefile/track_ops.py",
    "xops/makefile/roadmap_ops.py",
    "xops/makefile/doctor.py",
    "docs/planning/ROADMAP.md",
    ".agents/skills/README.md",
]

EXECUTABLE = [
    "xops/agent/tracking_append.sh",
    "xops/agent/safe-run.sh",
    "xops/agent/session-bootstrap.sh",
    "xops/agent/run-with-retry.sh",
]

JSON_FILES = [
    ".mcp.json",
    ".vscode/mcp.json",
    ".vscode/settings.json",
    ".vscode/tasks.json",
    ".cursor/mcp.json",
    ".claude-plugin/plugin.json",
    ".opencode/config.json",
    "gemini-extension.json",
]

TRACKING_HEADER = "ts_utc,run_id,agent,scope,action,status,summary,refs,commit_sha"


def _check_exists(failures: list[str]) -> None:
    step("📦 required files exist")
    for rel in CHECKS_REQUIRED:
        p = REPO_ROOT / rel
        if p.exists():
            dim(f"  ✓ {rel}")
        else:
            failures.append(f"missing: {rel}")
            err(f"  ✗ {rel}")


def _check_executable(failures: list[str]) -> None:
    step("⚙️  scripts are executable")
    for rel in EXECUTABLE:
        p = REPO_ROOT / rel
        if not p.exists():
            warn(f"  ? {rel} (file missing — checked in 'required files')")
            continue
        if os.access(p, os.X_OK):
            dim(f"  ✓ {rel}")
        else:
            failures.append(f"not executable: {rel}")
            err(f"  ✗ {rel} (run: chmod +x {rel})")


def _check_json(failures: list[str]) -> None:
    step("🔍 JSON files parse")
    for rel in JSON_FILES:
        p = REPO_ROOT / rel
        if not p.exists():
            warn(f"  ? {rel} (file missing — skipped)")
            continue
        text = p.read_text(encoding="utf-8")
        # Allow //... and /*...*/ style comments and $comment keys are fine in JSON.
        # JSONC isn't standard; strip // and /* */ for tolerance.
        stripped = _strip_jsonc(text)
        try:
            json.loads(stripped)
            dim(f"  ✓ {rel}")
        except json.JSONDecodeError as e:
            failures.append(f"invalid JSON: {rel} ({e})")
            err(f"  ✗ {rel}: {e}")


def _strip_jsonc(s: str) -> str:
    out = []
    i = 0
    in_str = False
    escape = False
    while i < len(s):
        ch = s[i]
        if in_str:
            out.append(ch)
            if escape:
                escape = False
            elif ch == "\\":
                escape = True
            elif ch == '"':
                in_str = False
            i += 1
            continue
        if ch == '"':
            in_str = True
            out.append(ch); i += 1; continue
        if ch == "/" and i + 1 < len(s):
            nxt = s[i + 1]
            if nxt == "/":
                # skip to end of line
                j = s.find("\n", i)
                i = j if j != -1 else len(s)
                continue
            if nxt == "*":
                j = s.find("*/", i + 2)
                i = (j + 2) if j != -1 else len(s)
                continue
        out.append(ch); i += 1
    return "".join(out)


def _check_tracking(failures: list[str]) -> None:
    step("📋 tracking.csv has correct header")
    if not TRACKING_CSV.exists():
        failures.append("tracking.csv missing")
        return
    first = TRACKING_CSV.read_text(encoding="utf-8").splitlines()[:1]
    if not first or first[0] != TRACKING_HEADER:
        failures.append(f"tracking.csv header mismatch: got {first}")
        err(f"  ✗ header is {first!r}")
        err(f"  ✗ expected {TRACKING_HEADER!r}")
    else:
        dim(f"  ✓ {TRACKING_HEADER}")


def _check_skill_links(failures: list[str]) -> None:
    step("🧠 skill links referenced from AGENTS.md exist")
    agents_md = REPO_ROOT / "AGENTS.md"
    if not agents_md.exists():
        return
    text = agents_md.read_text(encoding="utf-8")
    # Crude regex-free scan for `.agents/skills/.../<name>.prompt.md` references.
    needles = set()
    needle = ".agents/skills/"
    pos = 0
    while True:
        i = text.find(needle, pos)
        if i == -1:
            break
        end_chars = ") ]\n'\"`"
        j = i
        while j < len(text) and text[j] not in end_chars:
            j += 1
        token = text[i:j]
        # Only consider concrete file references (must end in a known doc suffix),
        # not bare directory mentions like ".agents/skills/".
        if token.endswith(".md"):
            needles.add(token)
        pos = j
    missing = [n for n in sorted(needles) if not (REPO_ROOT / n).exists()]
    for m in missing:
        warn(f"  ? {m} (referenced from AGENTS.md but file missing)")
    if not missing:
        ok("all referenced skill files exist")


def main() -> None:
    info(f"🩺 doctor — checking framework wiring in {REPO_ROOT}")
    failures: list[str] = []
    _check_exists(failures)
    _check_executable(failures)
    _check_json(failures)
    _check_tracking(failures)
    _check_skill_links(failures)
    print()
    if failures:
        err(f"💥 {len(failures)} failure(s):")
        for f in failures:
            err(f"  - {f}")
        sys.exit(1)
    ok("🎉 all checks passed")


if __name__ == "__main__":
    main()
