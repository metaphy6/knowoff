---
name: 'CodeGraph-first code exploration'
description: 'Use CodeGraph as the first tool for code questions in this repo'
applyTo: '**/*.{go,dart,py,sh}'
---

# CodeGraph-first exploration

This repository is indexed by CodeGraph. For any question about source code —
how a symbol works, where it is defined, what calls it, or what it affects —
use the appropriate `mcp_codegraph_*` tool first:

| Intent | Tool |
|---|---|
| "How does X work?" / surveying an area | `mcp_codegraph_explore` |
| "Where is the symbol named X?" | `mcp_codegraph_search` |
| "What calls this?" / blast radius | `mcp_codegraph_callers` |
| "What is this symbol — source + caller/callee trail?" | `mcp_codegraph_node` |
| Reading an indexed source file | `mcp_codegraph_node` with `file=...` |

Do **not** start with `read_file` or `grep_search` for symbol lookup,
call-graph questions, or understanding how code works. Fall back to raw
reads/searches only for files CodeGraph does not index (configs, docs, build
scripts), or when a tool response flags the file as stale.

Trust CodeGraph results — they come from a full AST parse. Re-index via
`make codeg` when the index is stale or missing; record the re-index as a
tracking note. Full procedure: [codegraph-management skill](../../.agents/skills/codegraph-management/SKILL.md).
