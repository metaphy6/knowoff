# 🤖 The agent operating model

> Why this framework exists and how to live inside it.

## The problem

AI coding assistants (Copilot, Claude, Gemini, Codex, Cursor, ...) are
useful but operate **statelessly across sessions**, **drift across vendors**,
and **silently lose work** when a terminal or chat dies mid-task. Left to
themselves they will:

- commit half-finished work,
- skip writing tests,
- overwrite files they didn't read,
- silently drop output when a command exits non-zero,
- ask "should I continue?" instead of finishing.

## The model

We treat the agent as a **fast, narrow-context junior engineer** with full
access to the repo. To survive that, the repo owns the operating policy:

1. **One rulebook** at the top — [`AGENTS.md`](../../AGENTS.md). Every
   vendor-specific entry point (`CLAUDE.md`, `GEMINI.md`, `.cursor/rules`,
   `.github/copilot-instructions.md`, ...) delegates to it.

2. **Tracking layer** — [`docs/tracking/tracking.csv`](../tracking/tracking.csv) is
   the agent's external memory. Every action that *would* be committed,
   reverted, or noted appears as a row first. Humans push via `make git`;
   agents never `git commit`.

3. **Crash-safe execution** — [`xops/agent/safe-run.sh`](../../xops/agent/safe-run.sh)
   wraps anything risky so the log + exit code survive a killed terminal.
   [`docs/tracking/state/last_failure.json`](../tracking/state/) is the breadcrumb
   the next session reads.

4. **Skills library** — [`.agents/skills/`](../../.agents/skills/) holds short,
   model-agnostic procedures. Agents load the relevant one *before* the
   matching work; nothing is reinvented per-task.

5. **Phase persistence** — when implementing a named scope, the agent
   drains every `[ ]` bullet before handing back. "Should I continue?" is
   a failure mode, not engineering.

## What this is NOT

- **Not a CI tool.** Local gates only; project-specific CI lives elsewhere.
- **Not a model.** The framework works with any agent — vendor-specific
  calibrations in [`MODEL_PROFILES.md`](MODEL_PROFILES.md).
- **Not magic.** Every behaviour is a script under [`xops/`](../../xops/)
  you can `cat` and audit.

## The five-line policy

1. Read [`AGENTS.md`](../../AGENTS.md) at session start.
2. Wrap risky commands in [`safe-run.sh`](../../xops/agent/safe-run.sh).
3. Append a tracking row whenever you'd want a commit / decision visible.
4. `git add -A` and stop; the human runs `make git`.
5. Tests move with code in the same commit, always.
