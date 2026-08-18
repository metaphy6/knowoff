---
name: git-bisect-strategy
description: "Git Bisect Strategy. You need to find which commit introduced a bug, regression, or performance"
---

# Git Bisect Strategy

## When to use

- You need to find which commit introduced a bug, regression, or performance
  degradation and the history is longer than ~10 commits.
- `git blame` and manual inspection have not narrowed it down.
- You have a reliable way to reproduce the failure (script, test, command).

## Procedure

1. **Confirm you have a good / bad boundary.**
   - `bad` = current HEAD (or the earliest commit you _know_ is broken).
   - `good` = the last known-green commit (check the tracking log or CI history).

2. **Start bisect:**
   ```bash
   git bisect start
   git bisect bad                          # mark HEAD as bad
   git bisect good <last-known-good-sha>   # mark the good commit
   ```
   Git checks out the midpoint automatically.

3. **Test each checkout.** Run your reproducer (unit test, `make test`, smoke
   script). Then:
   ```bash
   git bisect good   # this commit is fine
   # or
   git bisect bad    # this commit is broken
   ```
   Repeat until Git prints `<sha> is the first bad commit`.

4. **Automate** when the reproducer is a single command:
   ```bash
   git bisect run <command-that-exits-0-for-good-1-for-bad>
   ```
   Example: `git bisect run python -m pytest tests/test_foo.py -q`

5. **Inspect the culprit commit:**
   ```bash
   git show <bad-sha>
   git bisect reset   # return to HEAD
   ```

6. **Append a note row** before writing the fix:
   ```bash
   make track.add ACTION=note STATUS=completed SUMMARY="debug: bisect found regression in <sha> — <one line>"
   ```

7. Write a regression test that reproduces the bug, then fix it. The test and
   fix go in the same commit (see the TDD skill).

## Anti-patterns

- ❌ Starting bisect without a reliable reproducer — you'll mismark commits.
- ❌ Forgetting `git bisect reset` — leaves you on a detached HEAD.
- ❌ Using `git bisect bad/good` on commits that are unrelated to the bug (noisy
  test environment).
- ❌ Writing the fix _before_ writing the regression test.
