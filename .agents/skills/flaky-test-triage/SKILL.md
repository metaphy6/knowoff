---
name: flaky-test-triage
description: "Flaky Test Triage. A test passes locally but fails in CI (or vice versa)."
---

# Flaky Test Triage

## When to use

- A test passes locally but fails in CI (or vice versa).
- A test sometimes passes, sometimes fails, with the same code.
- You are tempted to re-run the test and hope it passes.

**Never** quarantine, skip, or delete a flaky test without first triaging it.

## Procedure

1. **Reproduce the flakiness.** Run the test 10–20 times in a loop:
   ```bash
   # bash
   for i in $(seq 1 20); do pytest tests/test_foo.py::test_bar -q && echo "PASS $i" || echo "FAIL $i"; done
   ```
   If it _never_ fails locally, the flakiness is environment-dependent
   (proceed to step 3).

2. **Classify the root cause.** Most flaky tests fall into one of these buckets:
   - **Timing / async race** — test doesn't wait for an async operation.
   - **Shared mutable state** — test depends on order; a previous test leaves
     side-effects.
   - **External service** — network call, real database, real filesystem.
   - **Random input** — test uses `random`, `uuid`, or current time without
     a fixed seed.
   - **Resource exhaustion** — port conflict, file descriptor limit.

3. **Fix the root cause, not the symptom.** Do _not_ just add `time.sleep(1)`.
   Correct fixes:
   - Async race → use proper `await`, `waitForElement`, condition variables.
   - Shared state → add `setUp`/`tearDown`, use fixtures, mock globals.
   - External service → mock or use a test double.
   - Random → fix the seed: `random.seed(42)`.
   - Resource → use ephemeral ports, temp directories, test containers.

4. **Verify the fix** by running the test ≥ 10 times in a loop and confirming
   100% pass rate.

5. **Write a comment in the test** explaining what was flaky and why the fix
   works, if non-obvious.

6. **Never quarantine silently.** If you cannot fix the flakiness now, open a
   tracking note:
   ```bash
   make track.add ACTION=note STATUS=completed \
     SUMMARY="debug(tests): <test_name> is flaky due to <cause> — quarantined pending <ticket>"
   ```
   Then mark with `@pytest.mark.xfail(strict=False, reason="…")` or equivalent
   — _with a comment_ that it must be resolved before the next release.

## Anti-patterns

- ❌ `time.sleep(N)` as a fix for async races.
- ❌ Deleting the flaky test ("it was testing something unimportant").
- ❌ Re-running CI until it goes green — this trains the team to ignore red bars.
- ❌ Marking `@skip` without a tracking row + deadline.
- ❌ Merging a PR that introduced the flaky test — revert and fix first.
