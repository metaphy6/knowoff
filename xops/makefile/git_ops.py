"""xops/makefile/git_ops.py — `make git` and `make git.dry`.

Reads docs/tracking/tracking.csv, finds rows with action=commit, status=completed,
commit_sha=pending whose run_id does NOT already appear in any commit
message, groups them by run_id (one commit per run_id), then either previews
or commits + pushes.

The row's `summary` column is used VERBATIM as the commit subject. A
`[<run_id>]` trailer is appended so repeat invocations are idempotent.

Refuses to commit if the working tree is dirty AND no pending row exists
(catches the "agent forgot to track.add" footgun).
"""

from __future__ import annotations

import csv
import re
import sys
from pathlib import Path
from typing import Iterable, List

from _common import (
    BOLD, DIM, RESET, REPO_ROOT, TRACKING_CSV, dim, dispatch, err, info, ok,
    out, run, step, warn,
)

# Conventional Commits subject: <type>(<scope>)?(!)?: <description>.
# Mirrors the gate in xops/agent/tracking_append.sh. The commit subject is the
# tracking row's `summary` verbatim, so this is the last line of defense that
# keeps the commit log Conventional-Commits-clean even if tracking.csv was
# hand-edited around the appender.
_CC_RE = re.compile(
    r"^(feat|fix|docs|style|refactor|perf|test|chore|ci|build|revert)"
    r"(\([^)]+\))?!?:\s.+"
)


def is_conventional_commit(subject: str) -> bool:
    """True iff `subject` is a valid Conventional Commits subject line."""
    return bool(_CC_RE.match(subject))


def _read_pending_rows() -> List[dict]:
    if not TRACKING_CSV.exists():
        err(f"{TRACKING_CSV} not found")
        sys.exit(66)
    rows = []
    with TRACKING_CSV.open(newline="", encoding="utf-8") as f:
        reader = csv.DictReader(f)
        expected = ["ts_utc", "run_id", "agent", "scope", "action", "status",
                    "summary", "refs", "commit_sha"]
        if reader.fieldnames != expected:
            err(f"tracking.csv header mismatch: got {reader.fieldnames}")
            sys.exit(66)
        for r in reader:
            if (r["action"] == "commit"
                    and r["status"] == "completed"
                    and r["commit_sha"] == "pending"):
                rows.append(r)
    return rows


def _already_committed_run_ids() -> set[str]:
    """Return run_ids already mentioned in any commit message anywhere in repo."""
    if not (REPO_ROOT / ".git").exists():
        return set()
    log = out(["git", "log", "--all", "--format=%B"], check=False)
    seen = set()
    for line in log.splitlines():
        # match a [run-id] trailer anywhere in the line
        i = line.find("[")
        while i != -1:
            j = line.find("]", i + 1)
            if j == -1:
                break
            cand = line[i + 1 : j]
            if cand.replace("-", "").isalnum():
                seen.add(cand)
            i = line.find("[", j + 1)
    return seen


def _group_by_run_id(rows: List[dict]) -> List[List[dict]]:
    groups: dict[str, List[dict]] = {}
    order: List[str] = []
    for r in rows:
        rid = r["run_id"]
        if rid not in groups:
            groups[rid] = []
            order.append(rid)
        groups[rid].append(r)
    return [groups[rid] for rid in order]


def _build_commit_message(group: List[dict]) -> str:
    primary = group[0]
    subject = primary["summary"]
    body_lines: List[str] = []
    if len(group) > 1:
        body_lines.append("")
        body_lines.append("Additional tracking rows:")
        for r in group[1:]:
            body_lines.append(f"  - {r['action']}/{r['status']}: {r['summary']}")
    if primary["refs"]:
        body_lines.append("")
        body_lines.append("Refs:")
        for ref in primary["refs"].split(";"):
            ref = ref.strip()
            if ref:
                body_lines.append(f"  - {ref}")
    body_lines.append("")
    body_lines.append(f"[{primary['run_id']}]")
    return subject + "\n" + "\n".join(body_lines)


def _working_tree_dirty() -> bool:
    return bool(out(["git", "status", "--porcelain"], check=False).strip())


# ── subcommands ───────────────────────────────────────────────────────────

def cmd_dry(_args: List[str]) -> None:
    step("🔧 make git.dry — preview what would be committed")
    rows = _read_pending_rows()
    if not rows:
        ok("no pending commit rows in tracking.csv")
        if _working_tree_dirty():
            warn("⚠️  working tree IS dirty — agent forgot to track.add?")
            run(["git", "status", "--short"])
        return
    committed = _already_committed_run_ids()
    groups = _group_by_run_id(rows)
    info(f"found {len(rows)} pending row(s) in {len(groups)} run_id group(s)")
    for group in groups:
        rid = group[0]["run_id"]
        if rid in committed:
            dim(f"  ⏭  skipping {rid} — already in git log")
            continue
        msg = _build_commit_message(group)
        print()
        print(f"{BOLD}── would commit: [{rid}] ──{RESET}", file=sys.stderr)
        for line in msg.splitlines():
            print(f"    {line}", file=sys.stderr)
        if not is_conventional_commit(group[0]["summary"]):
            warn(f"  ⚠️  subject is NOT Conventional Commits — `make git` will refuse: {group[0]['summary']!r}")
    print()
    if _working_tree_dirty():
        info("staged + unstaged changes (git status --short):")
        run(["git", "status", "--short"])
    else:
        warn("working tree clean — `make git` would create empty commit(s)")


def cmd_push(_args: List[str]) -> None:
    step("🔧 make git — commit pending tracking rows + push")
    rows = _read_pending_rows()
    if not rows:
        if _working_tree_dirty():
            err("working tree has changes but tracking.csv has no pending row.")
            err("→ agent should append a row first (see AGENTS.md §2).")
            sys.exit(2)
        ok("nothing to commit (no pending rows, clean tree)")
        return

    committed = _already_committed_run_ids()
    groups = _group_by_run_id(rows)
    new_groups = [g for g in groups if g[0]["run_id"] not in committed]
    if not new_groups:
        ok("all pending rows already correspond to existing commits — nothing to do")
        return

    # Final Conventional-Commits gate before anything is committed. Validate
    # every subject up front and refuse the whole batch on the first offender,
    # so a bad subject never reaches the commit log and we never commit a
    # partial batch.
    offenders = [
        g[0] for g in new_groups if not is_conventional_commit(g[0]["summary"])
    ]
    if offenders:
        err("refusing to commit: non-Conventional-Commits subject(s) in tracking.csv")
        for r in offenders:
            err(f"  [{r['run_id']}] {r['summary']!r}")
        err("  required: <type>(<scope>)?(!)?: <description>")
        err("  fix the row's summary (append a corrective row) and re-run.")
        sys.exit(65)

    # Stage everything first (humans may have left things unstaged).
    run(["git", "add", "-A"])

    for i, group in enumerate(new_groups):
        rid = group[0]["run_id"]
        msg = _build_commit_message(group)
        info(f"committing [{rid}] ({i + 1}/{len(new_groups)})")
        # First group consumes the staged tree; subsequent groups become
        # --allow-empty so multiple run_ids can share one staging window.
        cmd = ["git", "commit", "-m", msg]
        if i > 0:
            cmd.insert(2, "--allow-empty")
        run(cmd)
        ok(f"committed [{rid}]")

    step("🚀 pushing to upstream")
    branch = out(["git", "rev-parse", "--abbrev-ref", "HEAD"]).strip()
    # --set-upstream-on-first-push, otherwise plain push.
    upstream_check = out(["git", "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"], check=False).strip()
    if not upstream_check:
        run(["git", "push", "--set-upstream", "origin", branch])
    else:
        run(["git", "push"])
    ok("pushed")


TABLE = {
    "dry":  cmd_dry,
    "push": cmd_push,
}

if __name__ == "__main__":
    dispatch("git_ops", TABLE)
