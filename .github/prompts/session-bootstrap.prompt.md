---
agent: ask
description: Print a session-bootstrap report so the agent knows where the previous session left off.
---

# Session bootstrap

Run [`xops/agent/session-bootstrap.sh`](../../xops/agent/session-bootstrap.sh)
and report what it prints. Specifically surface, at the top of your reply:

1. Any unresolved `docs/tracking/state/last_failure.json` — read it and propose a
   recovery step.
2. Any present `docs/tracking/state/checkpoint.json` — read it and propose whether
   to resume or restart.
3. Any `current.json` task in `in_progress` whose commit did **not** land
   in `git log` — orphaned, propose how to handle it.
4. Branch + last commit (one line).
5. The last 5 rows of [`docs/tracking/tracking.csv`](../../docs/tracking/tracking.csv).

Do not start new work in this prompt — only orient.
