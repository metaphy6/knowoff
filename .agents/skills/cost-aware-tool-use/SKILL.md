---
name: cost-aware-tool-use
description: "Cost-Aware Tool Use. You are an AI agent that has access to tools with per-call costs (LLM API"
---

# Cost-Aware Tool Use

## When to use

- You are an AI agent that has access to tools with per-call costs (LLM API
  calls, web searches, database queries, cloud API calls).
- You notice you are making many similar tool calls in a loop.
- The task involves processing a large corpus of external data.

## Procedure

1. **Batch reads before acting.** Gather all the context you need in parallel
   (or in one batch) before writing anything. Reading 5 files at once costs
   the same as reading 1; writing 5 separate edits costs 5× the round-trips.

2. **Search before reading.** Use a targeted search (grep, semantic search,
   file glob) to narrow down which files to read. Do not read every file in a
   directory hoping the relevant one turns up.

3. **Cache repeated lookups.** If you call the same web URL or run the same
   query twice in a session, you are wasting budget. Keep a mental (or
   session-memory) note of what you already fetched.

4. **Prefer local tools over remote tools.** `grep_search` over a web search
   when the answer might be in the repo. `read_file` over `semantic_search`
   when you know the file path.

5. **Set a personal call budget for complex tasks.** Before starting a
   multi-step task, estimate the number of tool calls needed. If you exceed
   twice the estimate, stop and re-plan rather than continuing to burn through
   calls searching blind.

6. **Avoid speculative tool calls.** Do not call a tool "just to see what
   happens". Every call should have a clear, stated purpose.

7. **Short-circuit on sufficient evidence.** Once you have enough context to
   act with high confidence, act — do not keep reading for marginal certainty
   improvements.

## Signs you are spending too much

- You have searched the same keyword 3+ times.
- You have read the same file twice in the same session.
- You are making web fetches for facts that are in the repo.
- You are using semantic search when an exact grep would do.

## Anti-patterns

- ❌ Running `semantic_search` for every sub-question instead of using the
  context you already have.
- ❌ Reading an entire directory of files to find one function.
- ❌ Making a web search to confirm a fact you found in the codebase 30
  seconds ago.
- ❌ Calling a write tool (create file, run command) before you have read
  enough context to be confident in the output.
