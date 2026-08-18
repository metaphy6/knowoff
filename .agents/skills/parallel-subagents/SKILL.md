---
name: parallel-subagents
description: "Parallel subagents. You need to read or search several **independent** things — multiple files"
---

# Parallel subagents

## When to use

You need to read or search several **independent** things — multiple files
you haven't seen, multiple symbol lookups, multiple "does X exist?" probes.

Don't spawn a subagent for work you can do directly in a single response
(reading one file, editing one function you already see).

## Procedure

1. **Identify independent items.** "Independent" = the result of one
   doesn't change the question you'd ask of another.
2. **Fan out in a single tool batch** when the tool supports it. Each
   subagent / search runs in parallel.
3. **Wait for all results** before continuing.
4. **Do not duplicate the work** a subagent is doing. If you delegated a
   search, don't run it yourself; if a subagent is reading file X, don't
   open file X.
5. **Verify subagent output.** A subagent's reply describes intent, not
   necessarily what landed on disk. Read the actual file changes if it
   edited code; inspect the actual command output if it ran a command.

## Good fan-out shapes

- Read 5 module files in parallel.
- Search for a symbol across 3 codebases in parallel.
- Independent "what's the convention for X?" questions across docs.

## Anti-patterns

- ❌ Spawning a subagent for one search you could do directly.
- ❌ Spawning subagents for sequential work (B depends on A's output).
- ❌ Re-running the same search the subagent just ran.
- ❌ Trusting a subagent's "I edited the file" without reading the diff.
- ❌ Spawning 20 subagents for the same query type — batch them into one with a list.
