---
name: anti-the-silent-skip
description: "The Silent Skip. A test was failing after a refactor. The agent marked it `@pytest.mark.skip`"
user-invocable: false
---

# The Silent Skip

> **Failure story → corrective skill link**

## The failure story

A test was failing after a refactor. The agent marked it `@pytest.mark.skip`
with the note "temporary — needs updating" and committed. Three sprints later
it was still skipped. The code path it guarded had a latent null-pointer
defect that shipped to production.

## Why it's tempting

"I'll just skip it for now and come back later." The bar goes green, the PR
merges, the agent moves on.

## Why it's wrong

- Skipped tests do not catch regressions.
- "Temporary" skips have a near-100% survival rate unless there is a deadline.
- A skip without a linked issue or tracking row is invisible to future engineers.
- AGENTS.md §3 explicitly forbids silencing tests to make a gate pass.

## The corrective behaviour

Load the [`test-driven-development`](../coding/test-driven-development.prompt.md)
skill. Fix the test — it must fail for the right reason before the fix and
pass after. If you genuinely cannot fix it now, open a tracking row with a
deadline and mark `xfail(strict=False, reason="…<tracking-id>")`.

## Recognition pattern

> "I'll skip this test for now."
> "The test was testing something that doesn't exist anymore." (without deleting the feature too)
> "It's just a unit test, the integration tests cover it."
