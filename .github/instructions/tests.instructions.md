---
name: 'Test authoring'
description: 'TDD rules for Go and Dart test files in Knowoff'
applyTo: '**/*_test.go,client/test/**/*.dart'
---

# Test authoring

- Write the test before the implementation, and run it to confirm it fails for the reason you expect. A test that passes before the change proves nothing.
- Make the smallest change that turns it green, then re-run the full relevant suite.
- Never use `t.Skip`, `skip:`, or `@Skip` to clear a red bar — a skipped test is a silent regression.
- Never weaken an assertion to make a failing test pass. Fix the code or fix the test's premise.
- Assert on observable behaviour and public contracts, not on unexported internals, so refactors don't break the suite.
- Every bug fix gets a regression test that reproduces the bug before the fix.
- Stage the test and the code it covers in the same commit.

Full procedure and tracking rows: [test-driven-development skill](../../.agents/skills/test-driven-development/SKILL.md).
