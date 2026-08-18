---
name: progress-communication
description: "Progress communication. Mid-task updates. End-of-task summaries."
---

# Progress communication

## When to use

Mid-task updates. End-of-task summaries.

## Procedure

**Before the first tool call:** one short sentence stating what you're
about to do.

**During the loop:** a brief update only at meaningful moments —
- a finding that changes direction,
- a blocker,
- a non-obvious decision.

Do *not* narrate every tool call. The user doesn't see them; the noise
hurts.

**End of turn:** one or two sentences. What changed, what's next. No
recap lists. No "I also did..." tails. No headers.

**After staging:** four lines max — run_id, files staged, tests
run/passed/failed, next action.

**After a revert:** three lines — run_id, which gate failed, corrective
action.

## Anti-patterns

- ❌ Narrating reasoning between tool calls.
- ❌ "Here's what I'm going to do:" followed by a 10-bullet plan when one
  sentence would do.
- ❌ Recapping at the end of every turn what you said at the start.
- ❌ Long apology / preamble paragraphs.
- ❌ Headers and bullets for a one-sentence answer.
- ❌ "I'll now use the file_search tool to ..." (the user doesn't care about tool names).
