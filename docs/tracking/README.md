# 📋 `docs/tracking/` — agent runtime + tracking log

This folder is the agent's working memory **and** the documentation for the
tracking workflow. The schema for the log itself lives next door at
[`tracking.schema.md`](tracking.schema.md).

## Layout

| Path | Purpose | Source-controlled? |
|---|---|---|
| [`tracking.csv`](tracking.csv) | Append-only log of every agent action. | ✅ Yes — header + every row. |
| [`tracking.schema.md`](tracking.schema.md) | Schema for `tracking.csv`. | ✅ Yes. |
| [`context.md`](context.md) | Shared project context pack — single place for project-specific overrides. | ✅ Yes. |
| `state/current.json` | The task the agent is currently running. | ❌ No (gitignored). |
| `state/checkpoint.json` | Mid-task interrupt point (written on SIGINT / 429). | ❌ No. |
| `state/last_failure.json` | Last non-zero-exit breadcrumb from `safe-run.sh`. | ❌ No. |
| `state/log.jsonl` | Append-only per-event log; tail with `tail -f`. | ❌ No. |
| `state/notes/` | Long-lived per-repo notes the agent wants to keep. | ❌ No. |

## The contract

> Agents append rows. Humans push via `make git`. No exceptions.

Every meaningful action by an agent ends with a row in
[`tracking.csv`](tracking.csv), appended atomically via
[`../../xops/agent/tracking_append.sh`](../../xops/agent/tracking_append.sh).
The row's `summary` column on `action=commit` is used **verbatim** as the
commit message by `make git`.

## The loop

```mermaid
flowchart LR
    plan[plan row] --> implement[implement rows]
    implement --> test[test row]
    test -->|green| commit[commit row\ncommit_sha=pending]
    test -->|red|   revert[revert row]
    commit --> stage[git add -A]
    stage --> human{human}
    human --> push[make git]
```

## Common patterns

### "I just finished a feature"

```bash
make track.add \
  ACTION=commit STATUS=completed \
  AGENT=copilot SCOPE=phase-2 \
  SUMMARY='feat(auth): add JWT validation middleware' \
  REFS='src/auth/mw.go;src/auth/mw_test.go' \
  COMMIT_SHA=pending
git add -A
# done — human runs `make git`
```

### "I ran the tests"

```bash
make track.add \
  ACTION=test STATUS=passed \
  AGENT=copilot SCOPE=phase-2 \
  SUMMARY='test: full auth suite passes (42 tests)'
```

### "Something blocked me"

```bash
make track.add \
  ACTION=block STATUS=blocked \
  AGENT=copilot SCOPE=phase-2 \
  SUMMARY='block: rebase needed — main moved 14 commits ahead' \
  REFS='docs/tracking/state/checkpoint.json'
```

### "I made a mistake in a prior row"

Rows are append-only. **Append a corrective row** (`action=note`) with
`refs` pointing to the prior `run_id`:

```bash
make track.add \
  ACTION=note STATUS=completed \
  AGENT=human SCOPE=phase-2 \
  SUMMARY='note: prior commit row had wrong scope (was phase-1)' \
  REFS='run-20260601T123000Z-1234'
```

## Reading the log

```bash
make track.list                          # last 20 rows
tail -n 50 docs/tracking/tracking.csv    # raw
```

## Why this design

- **One source of truth** for "what did the agent do today".
- **Reproducible commits**: `summary` → commit message, `run_id` → idempotency.
- **No magic**: humans can read the CSV; no DB, no daemon.
- **Multi-agent safe**: `flock(1)` serialises concurrent appends.

Read first: [`../../AGENTS.md`](../../AGENTS.md) +
[`tracking.schema.md`](tracking.schema.md). Use
[`../../xops/agent/tracking_append.sh`](../../xops/agent/tracking_append.sh) —
never hand-edit `tracking.csv`.
