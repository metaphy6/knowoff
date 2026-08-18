---
name: ai-output-stability
description: "AI output stability. You want the same input to produce the same (or near-same) output across"
---

# AI output stability

## When to use

You want the same input to produce the same (or near-same) output across
runs, models, and sessions. This matters for generated docs, generated
code, generated test cases, and anything an agent will hand off to another
agent.

## Procedure

1. **Anchor the input.** Quote the requirements literally; don't paraphrase
   them in your prompt. The plan in `docs/planning/` is the anchor.
2. **Cite explicit invariants.** Reference `AGENTS.md`, the skill files,
   and the schema docs by path — not "as discussed earlier".
3. **Constrain output shape.** Specify file paths, sections, line limits.
   "Write a 30-line README" beats "write a README".
4. **Reject style drift.** If a model invents extra sections or "improves"
   the requested format, regenerate with tighter constraints.
5. **Pin examples.** Show one concrete example of the desired output in
   the prompt; models match shape better than they follow description.
6. **Lower temperature / use seed** when the runtime supports it, for
   anything that should be reproducible.
7. **Capture the actual prompt + output** in a tracking `action=note` row
   when reproducibility actually matters (security review, audit, replay).

## Anti-patterns

- ❌ "Make this nicer." → unrepeatable, undefined.
- ❌ Letting the model pick the section structure for a doc that should
  match an existing template.
- ❌ Re-prompting with extra context every turn — the output diverges run-to-run.
- ❌ Trusting that "Claude got it right last time" means Claude will get
  it right next time. Pin the prompt.
- ❌ Using high temperature for code generation.
