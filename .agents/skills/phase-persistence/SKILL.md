---
name: phase-persistence
description: "Phase persistence. You've been asked to implement a phase, sub-phase, or any named scope with"
---

# Phase persistence

## When to use

You've been asked to implement a phase, sub-phase, or any named scope with
multiple `[ ]` bullets. **This is the most common place agents drop work.**

## Procedure

1. **Read the entire phase** before starting. Count the `[ ]` bullets.
2. **Drain them in order.** For each bullet:
   - implement the change (with the test, per
     [`test-driven-development`](../coding/test-driven-development.prompt.md)),
   - run the gate,
   - append a tracking row,
   - mark the bullet `[x]` in the ROADMAP.
3. **Do not stop between bullets** to ask permission. The named scope IS
   the permission.
4. **Stop only on a real blocker** — undefined behaviour, a decision that
   needs the user, scope outside the allow-list, a failing gate you can't
   diagnose within the rules. Write a `checkpoint.json`, append a
   `action=block, status=blocked` row, surface the blocker clearly.
5. **When every bullet is `[x]`**, run the phase's *Test plan* line one
   final time, then end in `staged` (see AGENTS.md §2).

## Anti-patterns

- ❌ "I did bullet 1 — should I continue?" → No. Drain the scope.
- ❌ "I did bullet 1; bullets 2–5 are easy but I'll let you confirm" → Same failure.
- ❌ Implementing bullets in random order, leaving "the hard one for last", giving up on the hard one.
- ❌ Re-planning mid-phase when nothing surprising happened.
- ❌ Skipping bullet 3 because "it's covered by bullet 5" — without saying so in chat.
- ❌ Marking a bullet `[x]` without the test + commit row to back it.
