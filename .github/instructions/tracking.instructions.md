---
applyTo: '**'
description: Mandatory end-of-turn tracking + staging gate. Always applied.
---

# 📝 Tracking is a MANDATORY end-of-turn gate — not optional

This rule is intentionally short and always-on so it is never skimmed past.
The long form (state machine, forbidden git ops) lives in [`AGENTS.md`](../../AGENTS.md) §2.

## The gate

**When the coordinating parent completes an authorized slice with real changes
and passing gates, it MUST do both of these before handing control back:**

Delegated agents return evidence to the parent without duplicate tracking or
staging. Failed gates enter AGENTS.md §5a recovery first; blocked or reverted
work follows the corresponding terminal state in AGENTS.md §2.

1. **Append exactly one tracking row** to
   [`docs/tracking/tracking.csv`](../../docs/tracking/tracking.csv) via the appender:

   ```bash
   xops/agent/tracking_append.sh \
     --agent=copilot \
     --scope=<short-scope> \
     --action=commit \
     --status=completed \
     --commit-sha=pending \
     --summary="type(scope): imperative description" \
     --refs="path/one;path/two"
   ```

   - `--summary` MUST be Conventional Commits (`feat|fix|docs|style|refactor|perf|test|chore|ci|build|revert`); the appender rejects anything else (exit 65).
   - Use `--action=note` (omit `--commit-sha`) when there is no diff — exploration, findings, a re-index, or a decision worth recording.

2. **Stage the work**: run `git add -A`.

Then STOP. Do **not** `git commit` or `git push` — the human runs `make git`.

## Non-negotiable

- **Never end a turn with a modified working tree that has no matching pending
  tracking row.** `make git` refuses a dirty tree with no pending row, so a
  missing row silently blocks the human's next commit — that is the exact
  failure this file exists to prevent.
- **Do not batch or defer.** Append the row for a slice of work when that slice
  is done, not "later". If you made several unrelated changes, prefer one row
  per logical change. `make git` creates one commit per staging window and
  includes every pending `run_id` in that commit.
- If a gate failed and you reverted, append an `--action=revert --status=failed`
  row instead — never leave the change untracked.

Treat "did I append the tracking row and `git add -A`?" as the last checklist
item of every turn, exactly like running tests before declaring done.
