---
name: safe-run-wrapper
description: "Safe-run wrapper. Any command where a non-zero exit, a hung process, or a killed terminal"
---

# Safe-run wrapper

## When to use

Any command where a non-zero exit, a hung process, or a killed terminal
would lose information you'd need to recover. In practice: tests, builds,
long downloads, anything > a few seconds.

## Procedure

Wrap the command with [`xops/agent/safe-run.sh`](../../../xops/agent/safe-run.sh):

```bash
xops/agent/safe-run.sh tests -- pytest -q tests/
xops/agent/safe-run.sh build -- make build
xops/agent/safe-run.sh deps  -- npm install
```

The wrapper guarantees:

- `/tmp/agent-runs/<run-id>.cmd`  — exact command + cwd snapshot
- `/tmp/agent-runs/<run-id>.log`  — combined stdout+stderr, tee'd live
- `/tmp/agent-runs/<run-id>.exit` — exit code (absence = killed mid-run)
- a heartbeat to stderr every 30s so the UI doesn't go silent.
- `docs/tracking/state/last_failure.json` on non-zero exit (with `resolved: false`).

If the command failed, follow
[`non-zero-exit-recovery`](../reliability/non-zero-exit-recovery.prompt.md).

## Anti-patterns

- ❌ Running raw `npm test` and losing the log when the terminal scrolls.
- ❌ Suppressing failure with `|| true` to keep the loop going.
- ❌ Piping into `head` or `wc` and discarding the rest of the output
  before reading it.
- ❌ Ignoring `last_failure.json` and retrying blindly.
