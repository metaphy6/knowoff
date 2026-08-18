---
name: tool-search-discipline
description: "Tool search discipline. You're about to reach for a tool. Pause and ask: do I know what I'm"
---

# Tool search discipline

## When to use

You're about to reach for a tool. Pause and ask: do I know what I'm
solving yet?

## Procedure

1. **Restate the task in one sentence.** If you can't, you don't know
   what tool you need.
2. **Pick the narrowest tool** that fits:
   - exact text → `grep`-class.
   - filename pattern → file search.
   - "where is this concept?" → semantic search.
   - "what calls this function?" → MCP code graph.
   - reading specific files → file read (large ranges, not many small reads).
3. **Don't run searches you already ran.** Re-running the same query is
   the most common time-waster.
4. **Don't call search before you know what to search for.**
5. **Read a few results fully** before searching more — overlapping
   results across queries are a signal you have enough context.
6. **Stop searching when you can act.** Over-exploration is a failure
   mode, not thoroughness.

## Anti-patterns

- ❌ "Let me search the codebase" with no specific symbol or pattern in mind.
- ❌ Three near-identical searches because the first didn't return what you hoped.
- ❌ Reading 20 files when 3 would tell you the answer.
- ❌ Picking the most powerful tool when the simplest would do.
- ❌ Skipping search and reading random files hoping to "stumble onto" the answer.
