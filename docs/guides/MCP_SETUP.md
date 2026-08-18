# 🔌 MCP setup

> Model Context Protocol servers extend an agent's capabilities (file-system
> indexes, code-graph lookups, search, browser, ...). This repo wires them
> in three places so every assistant sees the same set.

## Files

| Path | Read by |
|---|---|
| [`.mcp.json`](../../.mcp.json) | Claude Code, generic MCP clients. |
| [`.vscode/mcp.json`](../../.vscode/mcp.json) | VS Code Copilot. |
| [`.cursor/mcp.json`](../../.cursor/mcp.json) | Cursor. |

Keep the `mcpServers` block synchronised across the three for any
**project-scoped** servers you add.

## CodeGraph

[CodeGraph](https://github.com/colbymchenry/codegraph) gives agents a pre-indexed
symbol graph — ~58% fewer tool calls, ~16% lower token cost, 100% local. It is
included in the repo MCP configs with the correct invocation:

```json
"codegraph": {
  "type": "stdio",
  "command": "npx",
  "args": ["-y", "@colbymchenry/codegraph", "serve", "--mcp", "--path", "/absolute/path/to/project"]
}
```

**Two config files, two strategies:**

| File | Used by | Path strategy |
|---|---|---|
| `.vscode/mcp.json` | VS Code Copilot | `${workspaceFolder}` — VS Code expands it at runtime |
| `.mcp.json` | Claude Code, generic clients | Absolute path baked in by `scaffold.sh` |
| `.cursor/mcp.json` | Cursor | `${workspaceFolder}` — Cursor expands it |

`scaffold.sh` generates a project-specific `.mcp.json` with the absolute target
path embedded. **Never copy `.mcp.json` from this framework repo into a project
by hand** — run `scaffold.sh` so the path gets substituted correctly.

### Works alongside a global install

If you have codegraph installed globally (`codegraph install --location=global`),
VS Code and Cursor apply workspace-scope configs with higher precedence than
user-scope ones for the same server name. The workspace entry replaces the global
one in this project — no duplication, no conflict.

### First-time setup

`scaffold.sh` builds the index automatically during scaffolding (using `npx`
as a fallback when `codegraph` is not on your PATH). If you need to rebuild
it after a large refactor, or if you're adding codegraph to an existing
project that wasn't scaffolded, run:

```bash
npx @colbymchenry/codegraph init .
```

### Index management

The on-disk index lives under [`.codegraph/`](../../.codegraph/) (gitignored
except for `.gitignore`). Incremental updates happen automatically through
the file watcher with a ~1 s lag — you do **not** need to re-index after
every edit.

You **do** need a full re-init when:

- A folder moves or many files are renamed (≥ ~20).
- Tool responses show the **staleness banner** ("Some files referenced
  below were edited since the last index sync…") for files you need fresh.
- `mcp_codegraph_*` returns "no such symbol" for something you know exists.
- `.codegraph/` is missing entirely.
- You bumped the `@colbymchenry/codegraph` package version.

```bash
codegraph init . 2>/dev/null || npx -y @colbymchenry/codegraph init .
```

Agents follow the [`codegraph-management`](../../.agents/skills/codegraph-management/SKILL.md)
skill for the full procedure including when, who, and how to record a
re-index in the tracking log.

### Troubleshooting

| Symptom | Likely cause | Fix |
|---|---|---|
| MCP panel shows two `codegraph` entries, one inactive | `codegraph` defined in both `.vscode/mcp.json` *and* `.mcp.json` | Remove the duplicate from `.vscode/mcp.json` (scaffolder does this) |
| `mcp_codegraph_*` tools missing from picker | Server failed to start, no `.codegraph/` | Run `codegraph init .` |
| Stale results after a rename | Index lag or partial update | Full re-init |
| `npx -y @colbymchenry/codegraph` prints help | Missing `serve --mcp` args | Check the `args` array in your MCP config |
| MCP entry shows the wrong project's symbols | `--path` not absolute (non-VS Code clients don't expand `${workspaceFolder}`) | `scaffold.sh --force` to bake the right path into `.mcp.json` |

## Adding a project-scoped server

If you have an MCP server that is specific to this project (e.g. a custom
tool server living under `xops/`), add it to all three files:

1. Add the server block to `.mcp.json`, `.vscode/mcp.json`, `.cursor/mcp.json`.
2. Document its purpose + when to use it in this file.
3. If it requires secrets, document the env vars (never commit the values).
4. Add an entry to [`.agents/skills/mcp-usage/SKILL.md`](../../.agents/skills/mcp-usage/SKILL.md).

### VS Code merge semantics

VS Code Copilot **merges** the workspace `.vscode/mcp.json` with the global
`~/.config/Code/User/mcp.json`. A server name defined in both files
appears **twice** in the MCP panel, and whichever entry loads last wins (or
both fail, depending on the version). To avoid this:

- Do not put globally-installed servers (codegraph, filesystem, etc.) in `.vscode/mcp.json`.
- Only put servers that are genuinely unique to this project here.

## Anti-patterns

- Using `npx -y @pkg` without `serve --mcp` → launches the interactive installer instead of the server.
- Adding an MCP server only to `.mcp.json` → VS Code & Cursor users won't see it.
- Committing API keys in the `env` block.
- Wiring a write-capable server without a skill file explaining the blast radius.

