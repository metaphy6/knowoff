---
name: non-zero-exit-recovery
description: "Non-zero exit recovery. A command exited non-zero. Or a command produced no output (a killed"
---

# Non-zero exit recovery

## When to use

A command exited non-zero. Or a command produced no output (a killed
terminal counts as failure, not as no-op).

## Procedure

**Order matters.** Do not skip a step.

1. **Read the log.** Use the `.log` from
   [`safe-run.sh`](../tooling/safe-run-wrapper.prompt.md):
   `tail -200 /tmp/agent-runs/<run-id>.log`, then full if needed. Never
   guess at the cause.
2. **Diagnose the root cause.** Missing dep? Unset env var? Real test
   failure? OOM? Syntax error? Be specific.
3. **Fix that root cause** within the rules (no system-level changes
   without confirmation; no silencing the test; no `|| true`).
4. **Resume the interrupted task.** If `docs/tracking/state/checkpoint.json`
   exists, restart from there.
5. **Mark resolved.** Either delete `docs/tracking/state/last_failure.json` or
   edit `"resolved": true`.

## Anti-patterns

- ❌ Retrying the same command without reading the log.
- ❌ "I'll just disable that test."
- ❌ `try / except: pass` around the failing line.
- ❌ Re-running with `set +e` to hide the exit code.
- ❌ Treating a silent / killed terminal as success.
- ❌ Saying "done" with `last_failure.json` still `resolved: false`.
