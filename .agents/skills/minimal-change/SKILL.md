---
name: minimal-change
description: "Minimal Change. Always. Especially when:"
---

# Minimal Change

## When to use

Always. Especially when:

- The task is "fix X" — but you're tempted to also clean up Y and Z.
- A file you're touching has style you disagree with.
- You see a "better" way to structure the surrounding code.

## Procedure

1. **State the minimum change** that satisfies the request.
2. Make exactly that change. Nothing more.
3. If you spot drive-by improvements, record them as `action=note` tracking
   rows or a `docs/planning/` follow-up — **don't bundle them**.
4. Re-read the diff: anything that isn't strictly required for the stated
   goal → revert it.

## Why

- Reviewers can review a focused diff. They cannot review a 2000-line
  "while I was in there" PR.
- Bisecting a bug to a focused commit is trivial; to a kitchen-sink commit
  it's a nightmare.
- The blast radius of a small change is small. Rollback is cheap.

## Anti-patterns

- ❌ "While I was in there I also refactored the auth module."
- ❌ Reformatting a whole file because your editor's autoformatter ran.
- ❌ Renaming a function because you prefer the new name.
- ❌ "Cleaning up" imports / removing "dead" code you didn't verify is dead.
- ❌ Adding error handling for cases that can't happen.
- ❌ Adding abstractions ("this might be useful later") with one caller.
