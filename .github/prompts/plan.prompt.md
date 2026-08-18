---
agent: agent
description: Produce a written implementation plan for a feature, fix, or roadmap phase. Read-only — no code changes in this prompt.
---

# Plan a piece of work

Switch to the [`planner` custom agent](../agents/planner.agent.md) for
this prompt. Output is a Markdown plan only — no `editFiles`, no `git`.

## Steps

1. Restate the user's request in one sentence.
2. Read the relevant slice of:
   - [`docs/planning/ROADMAP.md`](../../docs/planning/ROADMAP.md)
   - [`docs/code/`](../../docs/code/) / [`docs/design/`](../../docs/design/)
   - any project-specific rules in [`.github/copilot-instructions.md`](../copilot-instructions.md)
3. Load [`.agents/skills/writing-plans/SKILL.md`](../../.agents/skills/writing-plans/SKILL.md).
4. Produce the plan in the shape the planner mode prescribes
   (Goal, Non-goals, Touched files, Test plan, Checklist, Risks).
5. If the request conflicts with the ROADMAP, **say so and stop**.

Do not edit files, run commands, or stage anything in this prompt.
