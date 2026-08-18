# 📚 `docs/`

This folder is the human-readable side of the project. Source of truth for
agents on **what the project is and where it's going**; source of truth for
humans on **the project's design and history**.

## Layout

| Path | Purpose | Audience |
|---|---|---|
| [`planning/ROADMAP.md`](planning/ROADMAP.md) | **The normative Knowoff spec + roadmap monolith** — rules, economy, media engine, protocol, infra, then the phase checklist. | Everyone. Read first. |
| [`code/`](code/) | Module-level documentation (architecture, modules, APIs). | Devs joining the codebase. |
| [`project/`](project/) | The project's charter, decision log, glossary — [`GLOSSARY.md`](project/GLOSSARY.md) holds the normative Knowoff terminology. | New contributors. |
| [`design/`](design/) | Design docs (DESIGN.md) and ADRs. | Reviewers + future-you. |
| [`planning/`](planning/) | The **ROADMAP** — single source of truth for sequenced work. | Agents + humans. |
| [`tracking/`](tracking/) | How the `docs/tracking/tracking.csv` workflow is used; [`context.md`](tracking/context.md) is the project context pack. | Agents. |
| [`guides/`](guides/) | Cross-cutting how-tos: agent operating model, model profiles, MCP usage. | Agents + ops. |
| [`reports/`](reports/) | Generated reports (audit, status snapshots). | Reviewers. |
| [`.agents/skills/`](../.agents/skills/) | The **skill library** — load on demand. | Agents. |

## Discoverability rule

Before creating a new doc, search for an existing one. Templates live next
to their READMEs (e.g. [`code/MODULE.template.md`](code/MODULE.template.md)).
Use them.
