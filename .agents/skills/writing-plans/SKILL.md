---
name: writing-plans
description: "Writing plans. Before any non-trivial implementation. 'Non-trivial' = more than a single"
---

# Writing plans

## When to use

Before any non-trivial implementation. "Non-trivial" = more than a single
file, or any change to a public API.

## Procedure

A good plan has **six sections** and fits on one screen:

1. **Goal** — one sentence. The user-visible outcome.
2. **Non-goals** — bulleted. What this plan explicitly will NOT do.
3. **Touched files** — best-effort list of paths. Catches scope creep early.
4. **Test plan** — for each `[ ]` bullet, what test will prove it works?
5. **Checklist** — the actual `[ ]` bullets the implementer will drain.
   Each bullet should be ≤ 30 minutes of work for a competent agent.
6. **Risks** — what could go sideways, blast radius, mitigation.

Save the plan to `docs/planning/` (for multi-day work) or paste in chat
(for a single phase). The implementer drains every `[ ]` bullet — see
[`phase-persistence`](phase-persistence.prompt.md).

## Anti-patterns

- ❌ A plan that's just a goal — no checklist, no test plan.
- ❌ A plan with checklist items like "implement auth" (too large).
- ❌ A plan written *after* the code (call it "post-hoc rationalisation",
  not a plan).
- ❌ Skipping the non-goals — they're how you bound the work.
- ❌ Writing a plan when the task is one line of code.
