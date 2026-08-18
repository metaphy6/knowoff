<!--
CLAUDE.md — entry point for Claude (Anthropic / Claude Code).
Claude reads this file by convention. To avoid drift between assistants, it
delegates to AGENTS.md and adds Claude-specific notes.
-->

# 🤖 CLAUDE.md — entry point for Claude

Hello, Claude. To stay consistent with the other AI coding assistants
configured in this repository, please read these documents in order:

1. [`AGENTS.md`](AGENTS.md) — non-negotiable cross-cutting rules: discoverability,
   mandatory tracking + staging, tests-move-with-code, system-change guardrails,
   session recovery, non-zero-exit protocol, security.
2. [`.agents/skills/README.md`](.agents/skills/README.md) — the curated skill library.
   Load the relevant skill before doing the matching kind of work.
3. [`.github/copilot-instructions.md`](.github/copilot-instructions.md) — if
   present, contains project-specific rules. Read it as supplementary context.
4. [`docs/planning/ROADMAP.md`](docs/planning/ROADMAP.md) — the project plan.

`AGENTS.md` wins on cross-cutting concerns; project-specific files win on
domain rules.

## ⚡ Short summary if you read nothing else

- **You never `git commit` or `git push`.** Append a row to
  [`docs/tracking/tracking.csv`](docs/tracking/tracking.csv) via
  [`xops/agent/tracking_append.sh`](xops/agent/tracking_append.sh), then
  `git add -A`, then stop. The human runs `make git`.
- **Every code change ships its test in the same commit.** Skipping or
  weakening a test to make a gate green is a hard violation.
- **Do not run system-level commands** (`apt`, `systemctl`, global git
  config, …) without explicit per-occurrence confirmation. Inside the
  workspace, act freely.
- **Sessions die.** Read `docs/tracking/state/checkpoint.json`,
  `docs/tracking/state/last_failure.json`, and tail `docs/tracking/state/log.jsonl` before
  starting work. Run [`xops/agent/session-bootstrap.sh`](xops/agent/session-bootstrap.sh)
  if available.

## 🛠 Claude Code specifics

- **Tool allow-list.** The repo does not pre-restrict your tools. The
  forbidden-action list lives in `AGENTS.md` §2 + §4, not in tool config.
- **MCP servers.** Defined in [`.mcp.json`](.mcp.json). Picked up
  automatically by Claude Code.
- **Plugin manifest.** [`.claude-plugin/plugin.json`](.claude-plugin/plugin.json)
  declares this repo as a workspace Claude can attach to.
- **Slash commands.** Project slash commands live under
  [`.github/prompts/`](.github/prompts/) — they are written for Copilot
  Chat but are readable Markdown procedures you can follow.
- **Memory.** If you keep session notes, write them under
  `docs/tracking/state/notes/` (gitignored).

## 🎯 Behavioural calibration for Claude

Across Sonnet, Opus, and Haiku the same biases tend to show up:

- **Over-explaining.** Keep replies tight — one short status line per loop
  step, file paths as markdown links, no recap sections.
- **Asking permission to continue.** When the user asked you to implement a
  named phase / sub-phase, drain *every* `[ ]` bullet in that scope before
  handing back. See the [`phase-persistence`](.agents/skills/phase-persistence/SKILL.md)
  skill for the explicit rule and the (narrow) list of real blockers.
- **Refactoring opportunistically.** Do not. Make exactly the change asked
  for. See the [`minimal-change`](.agents/skills/minimal-change/SKILL.md)
  skill.

If anything in this repo seems to invite shortcutting (skipping a test,
silencing a warning, force-pushing, installing a system package without
asking), assume the rule is intentional and ask before bypassing it.
