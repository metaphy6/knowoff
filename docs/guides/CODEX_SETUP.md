# Codex Setup

Use a current local Codex client: Codex in the ChatGPT desktop app, the CLI,
or the IDE extension. Ordinary ChatGPT chats do not automatically load a local
repository. Open the scaffolded folder in Codex and approve project trust after
reviewing its configuration. The installer never changes global settings or
grants trust, and it does not install ChatGPT or Codex.

## Install

From a local framework clone:

```bash
./install.sh --target /path/to/project --agents codex --no-vscode
```

The default agent selection also includes Codex alongside Copilot, Claude and
local clients. Requires Bash, Git and Python 3.9+; CodeGraph requires either its
installed CLI or Node.js/npx. The existing index setup may download CodeGraph
through npx. No system packages are installed. If index setup fails, the
installer reports it; resolve that prerequisite before relying on graph tools.

## Installed Surfaces

| Surface | Codex use |
| --- | --- |
| `AGENTS.md` | Native repository instruction discovery |
| `CONVENTIONS.md`, `docs/tracking/context.md` | Explicitly loaded by project instructions |
| `.github/instructions/*.instructions.md` | Read and apply their `applyTo` scopes; Codex does not use Copilot's automatic glob loader |
| `.agents/instructions/` | Shared roadmap discipline |
| `.agents/skills/` | Native skills, including supporting scripts and references |
| `.codex/agents/*.toml` | Native planner, implementer, reviewer and verifier roles |
| `.agents/skills/avb-*/` | Explicitly invoked adapters for every Copilot prompt |
| `.codex/config.toml` | Runtime guidance, approval/sandbox defaults, project CodeGraph MCP |

The original `.github/agents/` and `.github/prompts/` documents are also installed
as the authoritative workflow bodies. Native adapters reference them instead of
duplicating their prose. Models and reasoning effort inherit the user's choices.

## Runtime Translation

Read the master rulebook, conventions, context pack and relevant scoped
instructions before work. These adaptations change client mechanics only:

- Ignore Copilot YAML `tools`, `agent`, and handoff metadata. Use native Codex
  file-editing, shell, search and delegation tools with equivalent capabilities.
- A request to switch to an agent means use the matching native Codex role.
  The coordinating parent owns tracking and staging; children return evidence
  and never append duplicate tracking rows or stage one another's work.
  Use `--agent=codex` in tracking commands even when source examples say copilot.
  Before final staging, reviewers and verifiers inspect the complete current
  diff, including unstaged changes and new files supplied by the parent.
  References to a staged diff in source workflows also accept this review set.
  Children verify tests and scope first; the parent then appends the completion
  row, stages, and checks the final staged diff and tracking metadata.
- `/plan`, `/implement`, `/review`, `/verify`, `/track`, `/self-review`,
  `/session-bootstrap` and `/roadmap-status` refer to the corresponding
  `$avb-*` skills in Codex. These are not installed as native slash commands.
  Use the client's skill picker where its invocation syntax differs.
- Replace references to VS Code edit tools with the available patch tool;
  replace VS Code task runners with the documented shell commands. If a memory
  tool is unavailable, use `docs/tracking/state/` for repository-local recovery.
- Use only CodeGraph tools actually advertised by the connected server.
  The generated configuration preserves its `env` settings, including tool
  selection; do not assume a particular tool set is available. If the server is
  unavailable or an index is missing, report it and use local reads/searches.
  Never silently claim graph-backed results or bypass a required check.
- Respect `--no-mcp` and `--no-skills`: do not automatically undo these opt-outs.
  Source workflow documents remain available when native skills are disabled.
- Never discard human changes on a failed gate. Repair only the authorized
  changes, and ask when ownership is ambiguous. Review the complete staging
  set before applying the repository's staging policy.
- Sandbox permissions are enforced by the client, not these documents. Do not
  elevate permissions to avoid a failure. Ask for required approval, including
  protected Git writes. Verifier test artifacts/logs are allowed, source edits
  are not; the reviewer uses a read-only sandbox.

## Verify In The Client

After opening and trusting the folder, start a new session and ask:

> Without changing files, list the loaded instructions, available avb skills,
> custom agents and CodeGraph tools. Report any unavailable dependency.

Try `$avb-plan` with a small task. Ask Codex to delegate a read-only review to
`reviewer`. Inspect the MCP panel (or `/mcp` in the CLI) and request a CodeGraph
query against an actual symbol in your project. Configuration parsing alone
does not prove the client loaded it or that the server is healthy.

## Repeat Installs And Portability

The framework repository itself contains the shared rules and skills; native
`.codex/` roles and `avb-*` adapters are generated in the target by the installer.
Cloning this framework alone is not the same as scaffolding a project.

Shared anti-skills retain Copilot's `user-invocable: false` metadata. Codex
discovered these six skills in the audit session, but its generic skill-creator
validator rejects that extra field. This is a cross-client metadata difference;
it does not hide the skills in Codex. Preserve the Copilot setting when sharing
the same skill tree, and verify discovery in your actual client.

Existing files are preserved unless `--force` is supplied. A kept configuration
is not automatically merged: review it against a fresh scaffold before updating.
`--no-mcp`, `--no-skills` and agent exclusions skip new files; they do not remove
previously installed integrations. Back up customizations before using `--force`.

MCP paths are bound to the absolute target directory so nested working
directories cannot select a different index. After moving/cloning the repository
to another location, update the MCP paths (and executable path if applicable).
Do not commit credentials or change user-level trust settings in a scaffold.

## References

- [Instruction discovery](https://learn.chatgpt.com/docs/agent-configuration/agents-md)
- [Native custom agents](https://learn.chatgpt.com/docs/agent-configuration/subagents)
- [Skills and discovery](https://learn.chatgpt.com/docs/build-skills)
- [MCP configuration](https://learn.chatgpt.com/docs/extend/mcp)
- [Project trust and configuration](https://learn.chatgpt.com/docs/config-file/config-basic)
