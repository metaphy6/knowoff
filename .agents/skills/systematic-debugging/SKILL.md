---
name: systematic-debugging
description: "Systematic Debugging. A test is failing in a way you don't immediately understand."
---

# Systematic Debugging

## When to use

- A test is failing in a way you don't immediately understand.
- A user reports a bug.
- Behaviour differs between dev and prod.
- A previously-green test went red after an unrelated change ("flake").

## Procedure

1. **Reproduce reliably.** A bug you can't reproduce isn't a bug yet — it's a rumour. Capture the minimum input that triggers it.
2. **State the expected vs actual behaviour** in one sentence each. Out loud or in chat. Vagueness here = wasted hours later.
3. **Bisect**: find the smallest delta (commit, input, config) that flips the behaviour. `git bisect` is your friend.
4. **Locate the root cause**, not the closest symptom. Ask "why?" three times.
5. **Write a regression test** that fails on the buggy state and passes on the fix.
6. **Apply the smallest possible fix.**
7. **Run the full suite**, not just the new test.
8. **Note the cause** in the commit message body or an ADR if it surfaces a design issue.

## Anti-patterns

- ❌ "It works now, I don't know why" → ship a regression test or don't ship.
- ❌ Catching and swallowing the error to make the symptom go away.
- ❌ Adding `try/except`/`recover` blocks until the test passes.
- ❌ Marking the test as flaky / retried without diagnosing.
- ❌ Reverting an unrelated commit because "it broke after that".
- ❌ Reading 1 line of stack trace and "fixing" the closest function.

## Related skills

- [`non-zero-exit-recovery`](../reliability/non-zero-exit-recovery.prompt.md) — when the failure is a command exit, not a test red.
- [`verification-before-completion`](verification-before-completion.prompt.md) — before calling the fix done.
