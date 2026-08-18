---
name: test-driven-development
description: "Test-Driven Development. You are about to add new behaviour (feature, endpoint, branch in logic)."
---

# Test-Driven Development

## When to use

- You are about to add new behaviour (feature, endpoint, branch in logic).
- You are about to fix a bug.
- You are about to change a public contract.

Do NOT use for: docs-only changes, config-only changes, deleting clearly-dead code.

## Procedure

1. **Write the test first.** It should fail. If it passes, you misunderstood the bug or feature.
2. Run the test → confirm 🔴 red, with the error message you'd expect.
3. Write the **smallest** code change that turns it 🟢 green.
4. Run the **full** test suite (or the relevant slice) → all green.
5. Refactor if needed — keep tests green at every step.
6. Stage the test and the code together in the **same commit**.

## Tracking

- One `action=test, status=failed` row before the implementation (optional but useful).
- One `action=test, status=passed` row after.
- One `action=commit, status=completed` row with `summary='feat(...): ...'` or `'fix(...): ...'`.

## Anti-patterns

- ❌ Writing the code first, then back-filling a test that "matches the code". The test no longer proves correctness — it locks in whatever you wrote.
- ❌ `@Skip` / `xit` / `it.skip` to make the bar green. See [`AGENTS.md`](../../../AGENTS.md) §3.
- ❌ Loosening an assertion (`expect(x).toBe(5)` → `expect(x).toBeDefined()`) to "fix" a failing test.
- ❌ Deleting a failing test as part of a feature commit without an explicit note in `summary`.
- ❌ Committing the code and the test separately.
