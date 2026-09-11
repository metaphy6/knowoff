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

### Commit candidate handoff

`make git.dry` selects rows with **all three** values:
`action=commit`, `status=completed`, `commit_sha=pending`. It then excludes
run IDs found as `[run_id]` anywhere in `git log --all` commit messages. `note`, `implement`,
`review`, `test` and `block` rows never create commit candidates, even when
their status says `completed`. A later block/note row does not cancel a pending
commit row; append completion rows only for the work being handed over.

For every completed repository change, the coordinating agent must:

1. Review the exact change set and record truthful validation results under
   [AGENTS.md](../../AGENTS.md). A completed draft/document/skill change is
   distinct from content approval, technical certification or app activation.
   Keep unperformed release checks and existing failures explicit; a commit
   candidate is not evidence that those checks passed.
2. Append a completion row through the script below, using the task's existing
   `run_id` when it already has tracking rows. Never edit historical CSV rows.
   Use a fresh ID for new work if an earlier ID already appears in Git history.
3. Run `git add -A` after checking the full set for unrelated or sensitive files.
4. Run `make git.dry` and verify the intended summary and `[run_id]` appear in
   its proposed commit message. Report that evidence rather than assuming a
   tracking append succeeded. Agents stop here; the human runs `make git`.

Example from the repository root, after the applicable checks/authorization:
replace the example ID, summary and refs with the actual task's values.
Codex uses `--agent=local` in this repository's current schema; `codex` is not
an accepted enum value.

```bash
pwd
bash xops/agent/tracking_append.sh \
  --run-id=content-batch-20260911 --agent=local --scope=content \
  --action=commit --status=completed --commit-sha=pending \
  --summary='docs(content): add reviewed draft candidates' \
  --refs='content/README.md'
pwd
git add -A
pwd
make git.dry
```

Use `type(scope): description` for the actual change; keep the summary within
200 characters and refs separated by semicolons. Multiple pending run IDs in
one staging window produce **one combined commit**, with the first summary as
the subject and the others under `Also includes:`. `refs` describe evidence;
they do not select which files are committed. Preview does not stage or run
tests, and the human's `make git` stages all current changes before committing.
No empty commit is created. CSV rows stay `pending` after the human commits;
the recorded Git message IDs prevent them from becoming candidates again.
If a real blocker prevents the handoff under current instructions, explicitly
say that **no commit candidate was registered**, name the blocker and preserve
the checkpoint; do not describe the change as ready for `make git`.

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
