"""xops/makefile/track_ops.py — `make track.add` and `make track.list`.

Thin wrapper around xops/agent/tracking_append.sh. Reads variables from the
environment (set by the Makefile from ACTION=, STATUS=, etc.).
"""

from __future__ import annotations

import os
import sys
from typing import List

from _common import (
    REPO_ROOT, TRACKING_APPEND_SH, TRACKING_CSV, dispatch, err, info, ok, run, step,
)


REQUIRED_VARS = ("ACTION", "STATUS", "SUMMARY")
OPTIONAL_VARS = {
    "AGENT": "--agent",
    "SCOPE": "--scope",
    "REFS": "--refs",
    "COMMIT_SHA": "--commit-sha",
    "RUN_ID": "--run-id",
}


def cmd_add(_args: List[str]) -> None:
    step("📝 make track.add")
    for v in REQUIRED_VARS:
        if not os.environ.get(v):
            err(f"missing {v}= (required: {', '.join(REQUIRED_VARS)})")
            err("example: make track.add ACTION=note STATUS=completed SUMMARY='chore: tidy README'")
            sys.exit(64)
    args = [
        str(TRACKING_APPEND_SH),
        f"--action={os.environ['ACTION']}",
        f"--status={os.environ['STATUS']}",
        f"--summary={os.environ['SUMMARY']}",
    ]
    for env_key, flag in OPTIONAL_VARS.items():
        if os.environ.get(env_key):
            args.append(f"{flag}={os.environ[env_key]}")
    run(args)


def cmd_list(_args: List[str]) -> None:
    step("📋 make track.list — last 20 rows of docs/tracking/tracking.csv")
    if not TRACKING_CSV.exists():
        err(f"{TRACKING_CSV} not found")
        sys.exit(66)
    lines = TRACKING_CSV.read_text(encoding="utf-8").splitlines()
    header, *rows = lines
    info(f"total rows: {len(rows)}")
    print(header)
    for r in rows[-20:]:
        print(r)


TABLE = {
    "add":  cmd_add,
    "list": cmd_list,
}

if __name__ == "__main__":
    dispatch("track_ops", TABLE)
