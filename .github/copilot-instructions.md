<!--
.github/copilot-instructions.md — GitHub Copilot Chat / agent-mode rules.
This file delegates cross-cutting policy to AGENTS.md; it owns Copilot-specific
discovery hints, slash-command pointers, and custom-agent conventions.
-->

# 🐙 Copilot repo instructions

> **Primary rulebook:** [`AGENTS.md`](../AGENTS.md). When this file disagrees
> with `AGENTS.md`, AGENTS.md wins. This file owns Copilot-specific guidance
> (discoverability index, slash commands, custom agents).

## 🔍 Discoverability index — read before searching or creating

| Concern | Path |
|---|---|
| Master rulebook | [`AGENTS.md`](../AGENTS.md) |
| Vendor entry points | [`CLAUDE.md`](../CLAUDE.md), [`GEMINI.md`](../GEMINI.md), [`CONVENTIONS.md`](../CONVENTIONS.md) |
| Copilot custom agents | [`.github/agents/`](agents/) |
| Copilot slash-command prompts | [`.github/prompts/`](prompts/) |
| Always-on instruction files | [`.github/instructions/`](instructions/) |
| Skill library (load on demand) | [`.agents/skills/README.md`](../.agents/skills/README.md) |
| Normative spec + roadmap (one monolith) | [`docs/planning/ROADMAP.md`](../docs/planning/ROADMAP.md) — roadmap chapter at the end; stub at [`BLUEPRINT.md`](../BLUEPRINT.md) |
| Tracking log + schema | [`docs/tracking/tracking.csv`](../docs/tracking/tracking.csv), [`docs/tracking/tracking.schema.md`](../docs/tracking/tracking.schema.md) |
| Tracking guide + state | [`docs/tracking/README.md`](../docs/tracking/README.md), [`docs/tracking/state/`](../docs/tracking/state/) |
| Ops scripts | [`xops/agent/`](../xops/agent/), [`xops/makefile/`](../xops/makefile/) |
| MCP config | [`.mcp.json`](../.mcp.json), [`.vscode/mcp.json`](../.vscode/mcp.json) |
| VS Code workspace | [`.vscode/settings.json`](../.vscode/settings.json), [`.vscode/tasks.json`](../.vscode/tasks.json) |

**Rule:** if a file in this index already exists, **read it; do not recreate it**.
If you believe an existing file is wrong, propose an edit — never shadow it
with a new file at a different path.

## ⚡ Hard rules (read AGENTS.md for the long form)

1. **Never `git commit` / `git push`.** Append a tracking row via
   [`xops/agent/tracking_append.sh`](../xops/agent/tracking_append.sh),
   then `git add -A`, then stop. Human runs `make git`.
2. **Tests move with code** in the same commit.
3. **No system-level changes** without explicit per-occurrence confirmation.
4. **Non-zero exit recovery:** wrap risky commands with
   [`xops/agent/safe-run.sh`](../xops/agent/safe-run.sh); read the log,
   diagnose, fix, resume. Never retry blindly.
5. **Session recovery:** run [`xops/agent/session-bootstrap.sh`](../xops/agent/session-bootstrap.sh)
   at session start; surface any unresolved `last_failure.json`.
6. **Phase persistence:** when asked to implement a phase / sub-phase, drain every
   `[ ]` bullet before handing back. See
   [`.agents/skills/phase-persistence/SKILL.md`](../.agents/skills/phase-persistence/SKILL.md).
7. **Spec-first:** [`docs/planning/ROADMAP.md`](../docs/planning/ROADMAP.md)
   is one monolith — the spec chapters plus the roadmap chapter. Roadmap
   bullets are checklist entries, not full requirements: before implementing
   a phase's first bullet, read the chapters named in that phase's **Spec
   (required reading)** line — and when bullet text and a spec chapter
   disagree, the spec chapter wins.

### 🧭 Roadmap execution contract

The monolith has two deliberately different layers:

1. The chapters before **🗺 Roadmap — Step-by-Step Implementation Lifecycle**
   are the normative blueprint: product rules, architecture, protocol,
   security, design, and operational requirements.
2. The roadmap chapter is the implementation sequence: phase scope, ordered
   checkboxes, proof tests, skills, and gates.

For `/implement`, resolve work in this order: target phase and bullet scope,
that phase's **Spec (required reading)** chapters, the phase's What/Why/How
and Proof tests, then the ordered checkboxes. A checkbox never overrides the
blueprint. If a checkbox is narrower than, or conflicts with, its required
reading, implement the blueprint requirement and update the roadmap/spec in
the same change rather than silently choosing the narrower interpretation.
Read the required chapters before the first edit in each phase, keep the
phase's proof tests as the acceptance contract, and tick a checkbox only after
its implementation and proof are complete. Do not infer requirements from a
bullet alone or invent per-bullet Goal/Test-plan sections that are not present
in the monolith.

## 🤖 Custom agents

| Agent | When | File |
|---|---|---|
| `knowoff` | **Default entry point.** Any Knowoff-related request — Q&A, running the stack, features, debugging, migrations, DevOps, testing, UI/UX, economy tuning, the local AI content pipeline. Full read/write authority over the repo, including all docs. | [`agents/knowoff.agent.md`](agents/knowoff.agent.md) |
| `planner` | Decomposing a request into a roadmap or implementation plan | [`agents/planner.agent.md`](agents/planner.agent.md) |
| `implementer` | Executing a plan / phase end-to-end with tracking + staging | [`agents/implementer.agent.md`](agents/implementer.agent.md) |
| `reviewer` | Reviewing a phase's staged diff (middle stage of the per-phase gate) | [`agents/reviewer.agent.md`](agents/reviewer.agent.md) |
| `verifier` | Final mechanical gate per phase: test and lint gates + invariant checks | [`agents/verifier.agent.md`](agents/verifier.agent.md) |

`knowoff` handles day-to-day work directly; for roadmap-phase-sized work it
delegates into the same **`implementer → reviewer → verifier`** gate below
rather than reinventing it — no exceptions, and no phase skips the gate.

## ⚡ Slash commands

| Command | Purpose |
|---|---|
| `/plan` | Produce a written plan (no code) for a request. |
| `/implement` | Execute a plan / phase end-to-end, ending in `staged` / `reverted` / `no-op` / `blocked`. |
| `/review` | Self-review or peer-review staged or recent changes against AGENTS.md rules. |
| `/verify` | Run the mechanical verification gate (tests, lint, and invariants) and return PASS / FAIL. |
| `/track` | Append a tracking row (used implicitly by `/implement`). |
| `/roadmap-status` | Summarize ROADMAP checkbox progress. |

Prompts live under [`.github/prompts/`](prompts/).

## 🧠 Load skills on demand

[`.agents/skills/`](../.agents/skills/) contains short, model-agnostic skill files.
Load the relevant one before the matching kind of work. Especially:

- [`test-driven-development`](../.agents/skills/test-driven-development/SKILL.md)
- [`systematic-debugging`](../.agents/skills/systematic-debugging/SKILL.md)
- [`verification-before-completion`](../.agents/skills/verification-before-completion/SKILL.md)
- [`self-review`](../.agents/skills/self-review/SKILL.md)
- [`phase-persistence`](../.agents/skills/phase-persistence/SKILL.md)
- [`parallel-subagents`](../.agents/skills/parallel-subagents/SKILL.md)

## 💬 Communication discipline

- One short status line per loop step.
- File paths as workspace-relative markdown links.
- After staging: report `run_id`, files staged, tests run / passed / failed. Four lines max.
- After a revert: report `run_id`, which gate failed, the corrective action.

## 🧠 Memory & notes

If you keep long-lived per-repo notes, store them under `docs/tracking/state/notes/`
(gitignored). The memory tool, when available, may keep its own scope under
`/memories/repo/` — do not duplicate large facts already in this file.
