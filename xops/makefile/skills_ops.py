"""xops/makefile/skills_ops.py — `make skills.status` and `make skills.find`.

Reads every *.prompt.md under .agents/skills/, parses optional YAML-like
front-matter (tags, vendor blocks), and produces:

  make skills.status          — table of skill files: lines, last-modified,
                                which AGENTS.md section references each.
  make skills.find TAG=debug  — list skills whose tags include TAG.

Front-matter format (optional, at the top of the skill file after the h1):
---
tags: debugging, testing, reliability
claude: |
  Claude-specific note for this skill.
gpt: |
  GPT-specific note.
gemini: |
  Gemini-specific note.
---
"""

from __future__ import annotations

import os
import re
import sys
from datetime import datetime, timezone
from pathlib import Path
from typing import Dict, List, Optional, Tuple

from _common import REPO_ROOT, err, info, ok, step, warn


SKILLS_DIR = REPO_ROOT / ".agents" / "skills"
AGENTS_MD  = REPO_ROOT / "AGENTS.md"


# ── front-matter parsing ─────────────────────────────────────────────────

def _parse_frontmatter(text: str) -> Tuple[Dict[str, str], str]:
    """Return (frontmatter_dict, body_without_frontmatter).

    Looks for a YAML-like block delimited by --- after the first h1 heading.
    Keys with a block scalar (|) are returned verbatim (no YAML library needed).
    """
    # Skip the first heading line if present.
    lines = text.splitlines()
    start_idx = 0
    for i, line in enumerate(lines):
        if line.startswith("#"):
            start_idx = i + 1
            break

    # Look for --- ... --- block.
    fm: Dict[str, str] = {}
    body_start = start_idx
    if start_idx < len(lines) and lines[start_idx].strip() == "---":
        end_idx = None
        for j in range(start_idx + 1, len(lines)):
            if lines[j].strip() == "---":
                end_idx = j
                break
        if end_idx is not None:
            fm_lines = lines[start_idx + 1 : end_idx]
            body_start = end_idx + 1
            key: Optional[str] = None
            accumulator: List[str] = []
            for line in fm_lines:
                if re.match(r"^[a-zA-Z_][a-zA-Z0-9_]*\s*:\s*\|", line):
                    if key and accumulator:
                        fm[key] = "\n".join(accumulator).strip()
                    key = line.split(":")[0].strip()
                    accumulator = []
                elif re.match(r"^[a-zA-Z_][a-zA-Z0-9_]*\s*:", line):
                    if key and accumulator:
                        fm[key] = "\n".join(accumulator).strip()
                    parts = line.split(":", 1)
                    key = parts[0].strip()
                    value = parts[1].strip() if len(parts) > 1 else ""
                    if value:
                        fm[key] = value
                        key = None
                    accumulator = []
                elif key is not None:
                    accumulator.append(line)
            if key and accumulator:
                fm[key] = "\n".join(accumulator).strip()

    body = "\n".join(lines[body_start:])
    return fm, body


def _tags(fm: Dict[str, str]) -> List[str]:
    raw = fm.get("tags", "")
    if not raw:
        return []
    return [t.strip().lower() for t in raw.split(",") if t.strip()]


# ── AGENTS.md section detection ──────────────────────────────────────────

def _build_agents_refs() -> Dict[str, List[str]]:
    """Return mapping skill_name → [section_header, ...] referenced in AGENTS.md."""
    if not AGENTS_MD.exists():
        return {}
    text = AGENTS_MD.read_text(encoding="utf-8")
    # Find all markdown links that look like skill references.
    links = re.findall(r"\[([^\]]+)\]\(.agents/skills/[^\)]+\.skill\.md\)", text)
    # Also bare paths like `.agents/skills/tdd/SKILL.md`.
    bare  = re.findall(r".agents/skills/([\w/\-]+)\.skill\.md", text)
    refs: Dict[str, List[str]] = {}
    # Map link text → partial path segment.
    for link_text in links:
        slug = link_text.lower().replace(" ", "-")
        refs.setdefault(slug, []).append("AGENTS.md")
    for b in bare:
        slug = b.split("/")[-1]
        refs.setdefault(slug, []).append("AGENTS.md")
    return refs


# ── skill discovery ──────────────────────────────────────────────────────

def _discover_skills() -> List[Dict]:
    """Return list of dicts with keys: path, rel, name, lines, modified, tags, fm."""
    results: List[Dict] = []
    if not SKILLS_DIR.exists():
        return results
    for path in sorted(SKILLS_DIR.rglob("*.prompt.md")):
        rel = path.relative_to(REPO_ROOT)
        text = path.read_text(encoding="utf-8")
        lines = len(text.splitlines())
        mtime = datetime.fromtimestamp(path.stat().st_mtime, tz=timezone.utc)
        fm, _ = _parse_frontmatter(text)
        # Extract skill name from first h1 heading.
        name_match = re.search(r"^#\s+🧠\s+Skill:\s+(.+)", text, re.MULTILINE)
        name = name_match.group(1).strip() if name_match else path.stem.replace("-", " ").title()
        results.append({
            "path": path,
            "rel": str(rel),
            "name": name,
            "lines": lines,
            "modified": mtime.strftime("%Y-%m-%d"),
            "tags": _tags(fm),
            "fm": fm,
        })
    return results


# ── commands ─────────────────────────────────────────────────────────────

def cmd_status(_args: List[str]) -> None:
    step("📋 make skills.status")
    skills = _discover_skills()
    agents_refs = _build_agents_refs()

    if not skills:
        warn(f"No *.prompt.md files found under {SKILLS_DIR}")
        return

    # Header
    col_w = [40, 6, 12, 10, 30]
    hdr = (
        f"{'Skill':<{col_w[0]}} {'Lines':>{col_w[1]}} "
        f"{'Modified':<{col_w[2]}} {'Tags':<{col_w[3]}} {'AGENTS.md refs':<{col_w[4]}}"
    )
    print(hdr)
    print("-" * sum(col_w))

    for s in skills:
        slug = s["path"].stem
        refs = agents_refs.get(slug, agents_refs.get(s["name"].lower().replace(" ", "-"), []))
        tags_str = ",".join(s["tags"]) if s["tags"] else "-"
        refs_str = ",".join(set(refs)) if refs else "-"
        name_trunc = s["name"][:col_w[0] - 1]
        print(
            f"{name_trunc:<{col_w[0]}} {s['lines']:>{col_w[1]}} "
            f"{s['modified']:<{col_w[2]}} {tags_str:<{col_w[3]}} {refs_str:<{col_w[4]}}"
        )

    print("-" * sum(col_w))
    ok(f"{len(skills)} skills found in {SKILLS_DIR.relative_to(REPO_ROOT)}")


def cmd_find(args: List[str]) -> None:
    step("🔍 make skills.find")
    tag = os.environ.get("TAG", "")
    if not tag and args:
        tag = args[0]
    if not tag:
        err("TAG is required: make skills.find TAG=debugging")
        sys.exit(64)
    tag = tag.lower().strip()

    skills = _discover_skills()
    matches = [s for s in skills if tag in s["tags"] or tag in s["name"].lower()]

    if not matches:
        warn(f"No skills found with tag or name containing '{tag}'")
        return

    ok(f"Skills matching '{tag}':")
    for s in matches:
        tags_str = ",".join(s["tags"]) if s["tags"] else "(no tags)"
        print(f"  {s['rel']}")
        print(f"    {s['name']}  [{tags_str}]")
        # Show vendor overlays if present.
        for vendor in ("claude", "gpt", "gemini"):
            if s["fm"].get(vendor):
                print(f"    {vendor}: {s['fm'][vendor][:80]}…" if len(s["fm"][vendor]) > 80 else f"    {vendor}: {s['fm'][vendor]}")


TABLE = {
    "status": cmd_status,
    "find":   cmd_find,
}


def _dispatch() -> None:
    from _common import dispatch
    dispatch("skills_ops", TABLE)


if __name__ == "__main__":
    _dispatch()
