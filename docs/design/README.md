# 🎨 `docs/design/` — design docs & ADRs

Design docs (forward-looking proposals) and ADRs (Architecture Decision
Records — accepted decisions with rationale).

## Files

- [`DESIGN.template.md`](DESIGN.template.md) — for proposing a non-trivial change before building it.
- [`ADR.template.md`](ADR.template.md) — for recording a code-level decision after it's made.

## When to write

- **Design doc**: before a change > a few days of work, before a public API, before a cross-module refactor. Reviewed by humans + agents, signed off before code lands.
- **ADR**: after a meaningful architectural decision, so future-you can ask "why is it this way?" and get an answer.

Both live alongside the code they shape — file name embeds the topic
(`DESIGN-auth-rework.md`, `ADR-0007-use-postgres-not-mongo.md`).
