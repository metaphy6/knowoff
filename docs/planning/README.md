# 🗺 `docs/planning/`

The **roadmap** lives here. It is the single source of truth for sequenced
work, and the only place where "phases" and `[ ]` bullets are authoritative.

- [`ROADMAP.md`](ROADMAP.md) — the roadmap chapter only ("🗺 Roadmap —
  Step-by-Step Implementation Lifecycle"): the six phases and their
  checkboxes, Proof tests, and gates. The normative spec chapters live in
  [`BLUEPRINT.md`](../../BLUEPRINT.md), not here.

Agents reading this folder: when implementing a phase, drain every `[ ]`
bullet in that phase before handing back. First read the phase's **Spec
(required reading)** chapters in [`BLUEPRINT.md`](../../BLUEPRINT.md) —
those are normative, while `ROADMAP.md` controls sequence, checkboxes,
Proof tests, and gates. See
[`phase-persistence`](../../.agents/skills/phase-persistence/SKILL.md).
