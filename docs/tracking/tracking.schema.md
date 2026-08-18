# `docs/tracking/tracking.csv` — schema

> **Single source of truth for every agent-driven action in this repository.**
> Agents append rows; humans push via `make git`. Append-only, validated,
> RFC 4180.

## File location & format

- Path: [`tracking.csv`](tracking.csv).
- Encoding: UTF-8, LF line endings, no BOM.
- Format: RFC 4180 CSV. The header is the first line and is fixed.
- **Append-only.** Existing rows are immutable. Corrections land as a *new*
  row with `action=note` and a `refs` link to the prior `run_id`.
- Append via [`xops/agent/tracking_append.sh`](../xops/agent/tracking_append.sh) —
  it enforces every invariant below and is atomic under `flock(1)`.

## Columns (9, in order)

| # | Column | Type | Required | Description |
|---|---|---|---|---|
| 1 | `ts_utc` | ISO-8601 UTC, `YYYY-MM-DDTHH:MM:SSZ` | yes | Wall clock at row append. Strictly ≥ previous row's `ts_utc`. |
| 2 | `run_id` | `[a-z0-9-]{4,40}` | yes | Stable id for this agent run / slash-command invocation. Reused across every row from the same run. Used by `make git` for idempotency. |
| 3 | `agent` | enum | yes | `copilot \| claude \| gemini \| codex \| cursor \| opencode \| aider \| local \| human`. |
| 4 | `scope` | string (≤ 40 chars) | yes | What the work touches — a phase id (`phase-1.2`), module slug (`auth`), area (`docs`), or chore tag (`tooling`). Free-form but short. |
| 5 | `action` | enum | yes | `plan \| implement \| test \| review \| commit \| revert \| note \| block`. |
| 6 | `status` | enum | yes | `started \| in_progress \| passed \| failed \| blocked \| completed`. |
| 7 | `summary` | string (≤ 200 chars), quoted | yes | Conventional Commits format **required** on `action=commit`: `type(scope)?(!)?: description`. The appender rejects (exit 65) a non-conforming commit summary. Free-form otherwise. `make git` uses this verbatim as the commit message. |
| 8 | `refs` | semi-colon-separated string | no | File paths, issue links, prior `run_id`s. Example: `"src/auth.go;docs/design/AUTH.md;#42"`. |
| 9 | `commit_sha` | empty \| `pending` \| 7–40 hex | depends | On `action=commit`: must be `pending` (until `make git` runs) or a hex SHA. On `action=revert`: must be a hex SHA (of the reverted commit). All other actions: must be empty. |

## Enum cheat-sheet

| `action` | When to use | Typical `status` |
|---|---|---|
| `plan` | You wrote / updated a plan in `docs/planning/` | `completed` |
| `implement` | Mid-task progress note (no commit yet) | `in_progress` |
| `test` | You ran a gate (tests, lint, type-check) | `passed` \| `failed` |
| `review` | You reviewed staged or recent code | `completed` |
| `commit` | Real diff staged; awaiting `make git` | `completed` + `commit_sha=pending` |
| `revert` | A gate failed; you `git restore`d / `git reset --hard`d | `failed` |
| `note` | Informational — anything else worth recording | `completed` |
| `block` | Real blocker hit; checkpoint written | `blocked` |

## Invariants the appender enforces

1. Header is exactly:
   `ts_utc,run_id,agent,scope,action,status,summary,refs,commit_sha`.
2. Every row has exactly **9** columns.
3. `ts_utc` strictly ≥ previous row's `ts_utc` (clock-monotone).
4. `action`, `status`, `agent` are one of the enum values above.
5. `commit_sha` rules:
   - `action=commit` → `commit_sha` ∈ {`pending`} ∪ `[0-9a-f]{7,40}`.
   - `action=revert` → `commit_sha` matches `[0-9a-f]{7,40}`.
   - all other actions → `commit_sha` is empty.
6. `summary` on `action=commit` **must** be Conventional Commits format
   (`type(scope)?(!)?: description`, types: `feat fix docs style refactor
   perf test chore ci build revert`) — the appender **rejects** a
   non-conforming summary with exit 65 (the row is not appended). `make git`
   re-validates each subject and refuses to commit a non-conforming one.
7. `run_id` matches `[a-z0-9-]{4,40}`.
8. Atomic write under `flock(1)` so concurrent agent runs cannot interleave.

## How `make git` reads this file

See [`xops/makefile/git_ops.py`](../xops/makefile/git_ops.py):

1. Find rows where `action=commit`, `status=completed`,
   `commit_sha=pending` AND `run_id` is NOT already mentioned in any
   existing commit message (`git log --all --format=%B`).
2. Group consecutive rows by `run_id` (one commit per `run_id`).
3. For each group, create one commit:
   - **subject** = the row's `summary` field, verbatim,
   - **trailer** = `[<run_id>]` (used for idempotency on re-run).
4. Push to upstream.
5. **Refuse** to run if the working tree is dirty *but* no pending row
   exists (smart guard — agent forgot to `track.add`).

`make git.dry` performs steps 1–3 read-only and prints what *would* happen.

## Example rows

```csv
2026-06-01T09:12:33Z,bootstrap-2026-06-01-1,copilot,scaffold,note,completed,"chore(scaffold): initial framework drop","README.md;AGENTS.md;Makefile",
2026-06-01T09:45:10Z,phase-1-2026-06-01,claude,phase-1,plan,completed,"plan: phase 1 auth design","docs/planning/ROADMAP.md;docs/design/AUTH.md",
2026-06-01T10:30:55Z,phase-1-2026-06-01,claude,phase-1,test,passed,"test: full auth suite passes (42 tests)",,
2026-06-01T10:31:02Z,phase-1-2026-06-01,claude,phase-1,commit,completed,"feat(auth): add JWT validation middleware","src/auth/mw.go;src/auth/mw_test.go",pending
```

After `make git`, the last row's `[run_id]` trailer appears in the real
commit message — making the row idempotent on the next `make git`.
