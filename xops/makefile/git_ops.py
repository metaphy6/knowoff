"""xops/makefile/git_ops.py — `make git` and `make git.dry`.

Reads docs/tracking/tracking.csv, finds rows with action=commit, status=completed,
commit_sha=pending whose run_id does NOT already appear in any commit
message, groups them by run_id, then either previews or commits + pushes.

Git has a single staging window, so one `make git` run produces exactly ONE
commit carrying the whole staged tree. Every pending run_id contributes its
summary and `[<run_id>]` trailer to that commit's message. Empty commits are
never created: with a clean tree the pending rows simply wait and fold into
the next real commit.

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


def _build_batch_commit_message(groups: List[List[dict]]) -> str:
    """One commit message for the whole staging window.

    A single group keeps the classic per-run message. Multiple groups share
    the batch's one real commit: first group's summary is the subject, the
    others are listed in the body, and every group's `[<run_id>]` trailer is
    included so each row stays idempotent on re-run.
    """
    if len(groups) == 1:
        return _build_commit_message(groups[0])
    subject = groups[0][0]["summary"]
    body_lines: List[str] = ["", "Also includes:"]
    for group in groups[1:]:
        body_lines.append(f"  - {group[0]['summary']}")
    refs: List[str] = []
    for group in groups:
        for ref in group[0]["refs"].split(";"):
            ref = ref.strip()
            if ref and ref not in refs:
                refs.append(ref)
    if refs:
        body_lines.append("")
        body_lines.append("Refs:")
        for ref in refs:
            body_lines.append(f"  - {ref}")
    body_lines.append("")
    for group in groups:
        body_lines.append(f"[{group[0]['run_id']}]")
    return subject + "\n" + "\n".join(body_lines)


def _working_tree_dirty() -> bool:
    return bool(out(["git", "status", "--porcelain"], check=False).strip())


def _push_needed() -> bool:
    """True if HEAD has commits the configured upstream doesn't have yet.

    Covers the case where a prior `make git` committed locally but the
    `git push` step itself failed (e.g. a transient network/SSH error) —
    the tracking rows already match existing commits on re-run, so without
    this check the commits would be silently left unpushed forever.
    """
    upstream = out(
        ["git", "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"],
        check=False,
    ).strip()
    if not upstream:
        return True  # no upstream configured yet -> first push is needed
    ahead = out(["git", "rev-list", "--count", f"{upstream}..HEAD"], check=False).strip()
    return ahead.isdigit() and int(ahead) > 0


def _push() -> None:
    step("🚀 pushing to upstream")
    branch = out(["git", "rev-parse", "--abbrev-ref", "HEAD"]).strip()
    upstream_check = out(
        ["git", "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"],
        check=False,
    ).strip()
    if not upstream_check:
        run(["git", "push", "--set-upstream", "origin", branch])
    else:
        run(["git", "push"])


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
    new_groups = [g for g in groups if g[0]["run_id"] not in committed]
    info(f"found {len(rows)} pending row(s) in {len(groups)} run_id group(s)")
    for group in groups:
        rid = group[0]["run_id"]
        if rid in committed:
            dim(f"  ⏭  skipping {rid} — already in git log")
            continue
        if not is_conventional_commit(group[0]["summary"]):
            warn(f"  ⚠️  subject is NOT Conventional Commits — `make git` will refuse: {group[0]['summary']!r}")
    if new_groups:
        msg = _build_batch_commit_message(new_groups)
        rids = ", ".join(g[0]["run_id"] for g in new_groups)
        print()
        print(f"{BOLD}── would commit (one commit for the batch): [{rids}] ──{RESET}", file=sys.stderr)
        for line in msg.splitlines():
            print(f"    {line}", file=sys.stderr)
    print()
    if _working_tree_dirty():
        info("staged + unstaged changes (git status --short):")
        run(["git", "status", "--short"])
    else:
        warn("working tree clean — nothing to commit; pending rows fold into the next real commit")


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
        if _push_needed():
            ok("all pending rows already correspond to existing commits — pushing outstanding commits")
            _push()
            return
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

    # Empty commits are never created. With a clean tree the pending rows
    # stay pending and fold into the next real commit's message.
    if not _working_tree_dirty():
        warn("working tree clean — nothing to commit; pending rows will fold into the next real commit")
        if _push_needed():
            _push()
            ok("pushed")
        return

    # Stage everything first (humans may have left things unstaged).
    run(["git", "add", "-A"])

    # One staging window ⇒ one commit. Every pending run_id's summary and
    # [run_id] trailer rides in that commit's message.
    msg = _build_batch_commit_message(new_groups)
    rids = ", ".join(g[0]["run_id"] for g in new_groups)
    info(f"committing [{rids}] (1 commit for {len(new_groups)} run_id group(s))")
    run(["git", "commit", "-m", msg])
    ok(f"committed [{rids}]")

    _push()
    ok("pushed")


TABLE = {
    "dry":  cmd_dry,
    "push": cmd_push,
}

if __name__ == "__main__":
    dispatch("git_ops", TABLE)
