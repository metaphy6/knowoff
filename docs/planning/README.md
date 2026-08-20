# 🗺 `docs/planning/`

The **roadmap** lives here. It is the single source of truth for sequenced
work, and the only place where "phases" and `[ ]` bullets are authoritative.

- [`ROADMAP.md`](ROADMAP.md) — **the spec + roadmap monolith**: the full
  normative Knowoff spec chapters, then the roadmap chapter ("🗺 Roadmap —
  Step-by-Step Implementation Lifecycle") with the six phases and their
  checkboxes. [`BLUEPRINT.md`](../../BLUEPRINT.md) is a pointer stub.

Agents reading this folder: when implementing a phase, drain every `[ ]`
bullet in that phase before handing back. First read the phase's **Spec
(required reading)** chapters in the monolith: those opening blueprint/spec
chapters are normative, while the final roadmap chapter controls sequence,
checkboxes, Proof tests, and gates. See
[`phase-persistence`](../../.agents/skills/phase-persistence/SKILL.md).
