---
agent: ask
description: Summarise current ROADMAP.md progress — checked / unchecked / blocked bullets per phase.
---

# Roadmap status

Read [`docs/planning/ROADMAP.md`](../../docs/planning/ROADMAP.md) and produce:

1. **Phase summary table** — for each top-level phase: total bullets,
   `[x]` count, `[~]` count, `[ ]` count.
2. **Active phase** — which phase is currently "in progress" (has any `[~]`
   or partial `[x]` checkboxes).
3. **Next-up bullets** — the first 3 `[ ]` bullets the implementer should
   tackle next, with their file location.
4. **Blocked items** — any bullets explicitly noted as blocked, with the
   reason.

Cross-reference with recent rows in
[`docs/tracking/tracking.csv`](../../docs/tracking/tracking.csv) (last 20) to flag any
checkbox that should be `[x]` based on a `status=completed` commit row but
isn't yet.

Do not edit any files — this is a read-only report.
