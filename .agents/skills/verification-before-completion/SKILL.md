---
name: verification-before-completion
description: "Verification Before Completion. Always — before declaring any task 'done' and before staging."
---

# Verification Before Completion

## When to use

Always — before declaring any task "done" and before staging.

## Procedure

Run through this checklist explicitly. Do not trust the "feels done" feeling.

1. **Re-read the request.** Did you actually do what was asked, or what you'd have preferred to do?
2. **Run the test gate** on a clean tree — not just the test you wrote.
3. **Run the lint / type-check gate** if the project has one.
4. **`git status`** — nothing untracked you forgot to add? Nothing staged you didn't mean to stage?
5. **`git diff --cached`** — read the actual diff, line by line. Catch:
   - leftover `print` / `console.log` / `dbg!()` statements,
   - commented-out blocks,
   - hardcoded paths / secrets / timestamps,
   - files you didn't mean to touch.
6. **Did tests move with code?** If you added behaviour, is there a new test? If you fixed a bug, is there a regression test?
7. **Did you append the tracking row?** `tail -3 docs/tracking/tracking.csv` to confirm.
8. **`make doctor`** (or its project-specific equivalent) green?

Only then: `git add -A` and stop.

## Anti-patterns

- ❌ "The test I wrote passes" — but did all the OTHER tests pass?
- ❌ Skipping `git diff --cached` because "I know what I changed".
- ❌ Staging before verifying — you'll forget which run you meant to stage.
- ❌ Declaring done on a phase when only some bullets are `[x]`.
