# 🎨 `docs/design/` — design docs & ADRs

Design docs (forward-looking proposals) and ADRs (Architecture Decision
Records — accepted decisions with rationale).

## Files

- [`DESIGN.template.md`](DESIGN.template.md) — for proposing a non-trivial change before building it.
- [`ADR.template.md`](ADR.template.md) — for recording a code-level decision after it's made.
- [`DESIGN-text-game-modes.md`](DESIGN-text-game-modes.md) — adopted five-mode planning rationale and illustrative examples; implementation/playtesting remain pending.

- [`DESIGN-text-transition.md`](DESIGN-text-transition.md) — source-backed protocol, data migration, compatibility, retirement and proof contract.

- [`ADR-014`](ADR-014-account-deletion.md) — proposed complete deletion disposition and authority design; the closed profile foundation is implemented, with remaining executor coverage tracked in the roadmap.

## When to write

- **Design doc**: before a change > a few days of work, before a public API, before a cross-module refactor. Reviewed by humans + agents, signed off before code lands.
- **ADR**: after a meaningful architectural decision, so future-you can ask "why is it this way?" and get an answer.

Both live alongside the code they shape — file name embeds the topic
(`DESIGN-auth-rework.md`, `ADR-0007-use-postgres-not-mongo.md`).

## Accepted ADRs

| ADR | Title |
|---|---|
| [ADR-001](ADR-001-server-authoritative-over-p2p.md) | Server-authoritative over P2P |
| [ADR-002](ADR-002-flutter-everywhere.md) | Flutter everywhere |
| [ADR-003](ADR-003-home-server-behind-cloudflare.md) | Home-server-first behind Cloudflare; original media cost/topology assumptions superseded by ADR-012 |
| [ADR-004](ADR-004-media-package-in-server-pkg.md) | Media package in `server/pkg` |
| [ADR-005](ADR-005-synthetic-seed-pack.md) | Synthetic seed pack; production format scope superseded by ADR-012; synthetic test purpose retained |
| [ADR-006](ADR-006-typography.md) | v1 typography stand-in — **superseded by ADR-007** |
| [ADR-007](ADR-007-display-typeface.md) | Baloo 2 locked as the bundled display face |
| [ADR-008](ADR-008-support-accents.md) | Three support accents beside the locked verdict palette |
| [ADR-009](ADR-009-open-live-knowoff-ballot.md) | Open, live Knowoff ballot (replaces the blind-simultaneous ballot) |
| [ADR-010](ADR-010-shuffle-sky-accent.md) | Historical Shuffle sky accent; active specialty use retired by ADR-012 |
| [ADR-011](ADR-011-static-image-and-text-content.md) | Historical static images/text; playable-image allowance superseded by ADR-012 |
| [ADR-012](ADR-012-text-only-selectable-modes.md) | Text-only five-mode transition; planning adopted, runtime pending |
| [ADR-013](ADR-013-authoritative-action-certification.md) | Shared authoritative engine replay for technical action certification |
