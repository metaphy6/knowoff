---
agent: agent
description: Self-review the staged diff before handing off to the human.
---

# Self-review staged diff

Before the human runs `make git`, audit your own staged change:

1. Run `git diff --cached` and read it linearly.
2. For each touched file, verify:
   - The change is in scope of the task.
   - Tests cover the behavior change (new test for new behavior, regression
     test for a fix).
   - No `@Skip` / `skip:` / `xit(` / weakened assertions were introduced.
   - No secrets, hardcoded URLs, or magic numbers leaked in.
3. Verify [`docs/tracking/tracking.csv`](../../docs/tracking/tracking.csv) has a `pending`
   row whose `summary` matches what the diff actually does (Conventional
   Commits format).
4. Run the project's local test gate one more time (whatever `make test`
   or equivalent looks like).

Report findings in three buckets: **🚨 must-fix**, **⚠️ should-fix**,
**💡 nits**. Stop and ask before fixing any 🚨 if the fix would push the
diff outside the original scope.

Load [`self-review`](../../.agents/skills/self-review/SKILL.md)
for the full checklist.
