# 🔌 MCP setup

> Model Context Protocol servers extend an agent's capabilities (file-system
> indexes, code-graph lookups, search, browser, ...). This repo wires them
> in two places so every assistant sees the same set.

## Files

| Path | Read by |
|---|---|
| [`.mcp.json`](../../.mcp.json) | Claude Code, Cursor, generic MCP clients. |
| [`.vscode/mcp.json`](../../.vscode/mcp.json) | VS Code Copilot. |

Keep the CodeGraph server definition synchronized across both files so
project custom agents work in VS Code and other MCP clients.

## CodeGraph

[CodeGraph](https://github.com/colbymchenry/codegraph) gives agents a pre-indexed
symbol graph — ~58% fewer tool calls, ~16% lower token cost, 100% local. It is
included in the repo MCP configs with the correct invocation:

```json
"codegraph": {
  "type": "stdio",
  "command": "npx",
  "args": ["-y", "@colbymchenry/codegraph", "serve", "--mcp"]
}
```

In MCP mode CodeGraph uses the client's workspace root. Do not add a
machine-specific `--path`; the same checked-in config then works across
clones, usernames, and operating systems.

**Two config files, two strategies:**

| File | Used by | Path strategy |
|---|---|---|
| `.vscode/mcp.json` | VS Code Copilot | Project-scoped `codegraph` entry |
| `.mcp.json` | Claude Code, Cursor, generic clients | Client-provided workspace root |

### Works alongside a global install

If you have codegraph installed globally (`codegraph install --location=global`),
the workspace-scoped entry takes precedence for this project. No global
registration is required.

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
| MCP panel shows two `codegraph` entries, one inactive | Duplicate global and workspace registrations | Remove the duplicate global registration |
| `mcp_codegraph_*` tools missing from picker | Server is not registered in `.vscode/mcp.json`, failed to start, or no `.codegraph/` | Check the project entry, then run `codegraph init .` if the index is missing |
| Stale results after a rename | Index lag or partial update | Full re-init |
| `npx -y @colbymchenry/codegraph` prints help | Missing `serve --mcp` args | Check the `args` array in your MCP config |
| MCP entry shows the wrong project's symbols | Client opened the wrong workspace, or a stale index | Open the intended workspace and run `npx -y @colbymchenry/codegraph init .` if needed |

## Adding a project-scoped server

If you have an MCP server that is specific to this project (e.g. a custom
tool server living under `xops/`), update the two configuration files and their documentation:

1. Add the server block to `.mcp.json` and `.vscode/mcp.json`.
2. Document its purpose + when to use it in this file.
3. If it requires secrets, document the env vars (never commit the values).
4. Add an entry to [`.agents/skills/mcp-usage/SKILL.md`](../../.agents/skills/mcp-usage/SKILL.md).

### VS Code merge semantics

VS Code Copilot merges the workspace `.vscode/mcp.json` with the global
`~/.config/Code/User/mcp.json`. The workspace entry supplies this project's
server definition; remove a duplicate global `codegraph` registration if VS
Code shows two entries or reports a startup conflict.

## Anti-patterns

- Using `npx -y @pkg` without `serve --mcp` → launches the interactive installer instead of the server.
- Adding an MCP server only to `.mcp.json` → VS Code Copilot users won't see it.
- Committing API keys in the `env` block.
- Wiring a write-capable server without a skill file explaining the blast radius.

