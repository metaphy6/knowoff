---
name: codegraph-management
description: "CodeGraph management. You need to access, query, manage, or re-index the CodeGraph MCP server"
---

# CodeGraph management

## When to use

You're about to use, troubleshoot, or refresh the CodeGraph MCP server —
the local symbol-graph index that powers `mcp_codegraph_*` tools (search,
node, explore, callers). Read this **before** the first CodeGraph call of
a session if `.codegraph/` exists, and **always** after a large refactor.

## Access — how the index gets wired

CodeGraph runs as a stdio MCP server. The repo wires it in three places so
every assistant sees it:

| File | Client | Path strategy |
|---|---|---|
| [`.mcp.json`](../../.mcp.json) | Claude Code, generic MCP clients | Absolute path baked in by `scaffold.sh` |
| [`.vscode/mcp.json`](../../.vscode/mcp.json) | VS Code Copilot | `${workspaceFolder}` (expanded at runtime) |
| [`.cursor/mcp.json`](../../.cursor/mcp.json) | Cursor | `${workspaceFolder}` |

The on-disk index lives under [`.codegraph/`](../../.codegraph/) (gitignored
except for `.gitignore` itself). If the directory doesn't exist, the server
has nothing to open and the `mcp_codegraph_*` tools will fail.

## Use — pick the right call

| Intent | Tool |
|---|---|
| "How does X work" / surveying an area | `mcp_codegraph_explore` (verbatim source grouped by file) |
| "Where is the symbol named X?" | `mcp_codegraph_search` |
| "What calls this?" / blast radius | `mcp_codegraph_callers` |
| "What is this symbol — give me source + caller/callee trail" | `mcp_codegraph_node` |
| Reading an indexed source file | `mcp_codegraph_node` with `file=...` (Read-equivalent, faster, with dependents note) |

Trust CodeGraph results — they come from a full AST parse. **Don't re-verify
with grep.** Reach for raw `Read` / `grep` only for files CodeGraph doesn't
index (configs, docs) or when the **staleness banner** flags a file.

## Manage — when to re-index

Incremental updates happen automatically via the file watcher (~1 s lag
after writes). You do **not** need to re-index after every edit.

You **do** need to re-index in these cases:

| Trigger | Action | Who |
|---|---|---|
| Large refactor — folder moved, many files renamed (≥ ~20) | Full `init` | Agent (workspace-local write, allowed under AGENTS.md §4) |
| New project added codegraph after first scaffold | Full `init` | Agent |
| Tool response shows a **staleness banner** ("Some files referenced below were edited since the last index sync…") for a file you need fresh | Read raw file directly, then trigger a full `init` if it persists | Agent |
| `mcp_codegraph_*` returns "no such symbol" for something you know exists | Full `init` | Agent |
| `.codegraph/` directory missing entirely | Full `init` | Agent |
| Cross-file resolution looks wrong after a rename | Full `init` | Agent |
| Bumping the codegraph package version | Full `init` after the upgrade | Agent |

**Command** (works in any scaffolded project):

```bash
# Prefer the global install if present, fall back to npx.
codegraph init . 2>/dev/null || npx -y @colbymchenry/codegraph init .
```

The command is read-only with respect to your source code — it only writes
under `.codegraph/`. Safe to wrap with [`xops/agent/safe-run.sh`](../../xops/agent/safe-run.sh):

```bash
xops/agent/safe-run.sh codegraph-reindex -- \
  bash -c 'codegraph init . 2>/dev/null || npx -y @colbymchenry/codegraph init .'
```

After re-indexing, give the file watcher ~1 s to settle, then resume
`mcp_codegraph_*` calls.

## Track — record what you did

Re-indexing is **agent work**, so it gets a tracking row like anything else:

```bash
xops/agent/tracking_append.sh \
  --action=note --status=completed \
  --agent=copilot --scope=codegraph \
  --summary="chore(codegraph): full re-index after phase-3 refactor" \
  --refs=".codegraph/"
```

Use `--action=note` (not `commit`) — `.codegraph/` is gitignored, so there
is no staged diff. The row makes the rebuild discoverable in `make track.list`
and explains any subsequent behaviour change to the next session.

## Anti-patterns

- ❌ Re-indexing after every edit. The watcher handles small changes.
- ❌ Ignoring the staleness banner and quoting CodeGraph as truth anyway.
- ❌ `rm -rf .codegraph/` instead of `codegraph init .` (the init command
  handles the reset internally and writes proper file permissions).
- ❌ Committing `.codegraph/` contents. Only `.codegraph/.gitignore` is tracked.
- ❌ Putting `codegraph` in `.vscode/mcp.json` *and* `.mcp.json` — VS Code
  starts both, the second one stays inactive, tools silently fail. The
  scaffolder writes a `.vscode/mcp.json` without `codegraph` for this reason.
- ❌ Running `codegraph init` without a tracking row — the next session
  can't tell whether stale results are from a bug or a missed re-index.
