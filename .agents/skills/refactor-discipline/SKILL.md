---
name: refactor-discipline
description: "Refactor discipline. You want to change *shape* without changing *behaviour*. Renames,"
---

# Refactor discipline

## When to use

You want to change *shape* without changing *behaviour*. Renames,
extractions, splits, merges, dead-code removal.

## Procedure

1. **Confirm test coverage** for the surface area you're touching. If it's
   thin, *add tests first* (a separate commit), then refactor.
2. **One refactor per commit.** Mixing two refactors hides what broke when
   things break.
3. **Tests must stay green on every step**, not just the final one.
4. **Don't change behaviour.** If you find a bug while refactoring, fix it
   in a separate commit — *after* the refactor commit, so reviewers can
   tell them apart.
5. **Use the IDE's rename / extract operations** when possible. They're
   safer than hand-editing.
6. **Run the full gate**, not just the unit tests — refactors often break
   integration tests that exercise the old shape.

## Anti-patterns

- ❌ "Refactor + bugfix" in one commit.
- ❌ Refactoring code with no tests, "trusting the diff".
- ❌ Renaming a public symbol without grepping for downstream consumers.
- ❌ Deleting "dead code" without confirming it's actually dead (reflection, tests, scripts).
- ❌ Adding new features mid-refactor.
