<!--
GEMINI.md — entry point for Gemini / Gemini CLI.
Delegates to AGENTS.md and adds Gemini-specific behavioural notes.
-->

# 🤖 GEMINI.md — entry point for Gemini

Hello, Gemini. Please read these documents in order before doing any real
work:

1. [`AGENTS.md`](AGENTS.md) — the master rulebook (commit/push policy,
   tracking, tests, system safety, session recovery, security).
2. [`.agents/skills/README.md`](.agents/skills/README.md) — load the relevant
   skill before the matching kind of work.
3. [`.github/copilot-instructions.md`](.github/copilot-instructions.md) —
   project-specific rules (if present).
4. [`docs/planning/ROADMAP.md`](docs/planning/ROADMAP.md) — the plan.

## ⚡ Short summary

- **Never `git commit` / `git push`.** Append a row to
  [`docs/tracking/tracking.csv`](docs/tracking/tracking.csv) via
  [`xops/agent/tracking_append.sh`](xops/agent/tracking_append.sh), then
  `git add -A`. Human runs `make git`.
- **Tests ship with code in the same commit.** No skipping, no weakening.
- **No system-level changes** without explicit confirmation.
- **Read session state** before starting:
  [`xops/agent/session-bootstrap.sh`](xops/agent/session-bootstrap.sh).

## 🛠 Gemini CLI specifics

- **Extension manifest.** [`gemini-extension.json`](gemini-extension.json)
  declares this repo to the Gemini CLI.
- **MCP servers.** Read from [`.mcp.json`](.mcp.json) (same file other
  assistants use). Gemini CLI picks them up by default.

## 🎯 Behavioural calibration for Gemini

Across the Gemini family the recurring biases are:

- **Returning partial work and asking "should I continue?"** When the user
  asks you to implement a named phase, sub-phase, or slice, the work is
  the **entire named scope**. Loop internally over every `[ ]` bullet
  until they are all `[x]` or a *real* blocker is hit. See
  [`phase-persistence`](.agents/skills/phase-persistence/SKILL.md)
  for the full rule and the narrow list of legitimate blockers.
- **Re-planning the project mid-task.** If the user's request conflicts
  with [`docs/planning/ROADMAP.md`](docs/planning/ROADMAP.md), **ask
  before deviating**. Do not silently re-plan.
- **Inventing helpers.** Make exactly the change asked for. See
  [`minimal-change`](.agents/skills/minimal-change/SKILL.md).

When in genuine doubt, prefer one short clarifying question over a wrong
implementation. "Genuine doubt" is narrow — see AGENTS.md §6.
