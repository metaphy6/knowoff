# 📚 `docs/`

This folder is the human-readable side of the project. Source of truth for
agents on **what the project is and where it's going**; source of truth for
humans on **the project's design and history**.

## Layout

| Path | Purpose | Audience |
|---|---|---|
| [`../BLUEPRINT.md`](../BLUEPRINT.md) | **The normative Knowoff spec** — game rules, tech stack, architecture, economy, media engine, protocol, infra, product baseline. | Everyone. Read first. |
| [`planning/ROADMAP.md`](planning/ROADMAP.md) | The sequenced implementation plan — phases, checkboxes, proof tests. | Agents implementing a phase. |
| [`code/`](code/) | Module-level documentation (architecture, modules, APIs). | Devs joining the codebase. |
| [`project/`](project/) | The project's charter, decision log, glossary — [`GLOSSARY.md`](project/GLOSSARY.md) holds the normative Knowoff terminology. | New contributors. |
| [`design/`](design/) | Design docs (DESIGN.md) and ADRs. | Reviewers + future-you. |
| [`design/DESIGN-text-game-modes.md`](design/DESIGN-text-game-modes.md) | Adopted five-mode planning rationale and illustrative examples; runtime pending. | Product planning + playtesting. |
| [`design/DESIGN-text-transition.md`](design/DESIGN-text-transition.md) | Source audit, protocol/data contracts, migration, retirement and verification. | Engineering + operations. |
| [`product/BUSINESS_PLAN.md`](product/BUSINESS_PLAN.md) | Customer strategy, content operations, economics assumptions and launch gates. | Product + business. |
| [`reports/2026-09-12-text-transition-doc-audit.md`](reports/2026-09-12-text-transition-doc-audit.md) | Complete documentation coverage and retained historical exceptions. | Reviewers. |
| [`launch/`](launch/) | Release-gated copy, clip brief and migration runbook. | Launch operators. |
| [`planning/`](planning/) | The **ROADMAP** — single source of truth for sequenced work. | Agents + humans. |
| [`tracking/`](tracking/) | How the `docs/tracking/tracking.csv` workflow is used; [`context.md`](tracking/context.md) is the project context pack. | Agents. |
| [`guides/`](guides/) | Cross-cutting how-tos: agent operating model, model profiles, MCP usage. | Agents + ops. |
| [`reports/`](reports/) | Generated reports (audit, status snapshots). | Reviewers. |
| [`.agents/skills/`](../.agents/skills/) | The **skill library** — load on demand. | Agents. |

## Discoverability rule

For content work, start with the Blueprint's Media Engine chapter, the
[humor development guide](../content/humor-development.md),
[Curator Guide](../content/curator-guide.md) and
[current server content mechanics](code/MODULE-media-engine.md).
The target uses plain text only under [ADR-012](design/ADR-012-text-only-selectable-modes.md).
Current implementation still has the old image/text paths; source audits label
that difference. Authoring/review/integration skills follow the current Blueprint
and content guides; old static-image guidance cannot re-enable retired gameplay.
The [active Roadmap](planning/ROADMAP.md) holds transition work. Historical proof
and the September 11 content audit remain in the
[pre-text snapshot](planning/ROADMAP-pre-text-20260912.md#content-readiness-audit--2026-09-11).

Before creating a new doc, search for an existing one. Templates live next
to their READMEs (e.g. [`code/MODULE.template.md`](code/MODULE.template.md)).
Use them.
