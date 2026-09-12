# Documentation transition audit — 2026-09-12

**Scope:** Blueprint, Roadmap, every tracked document under `docs/`, root project
entry points and the content guides that directly govern the transition.
**Method:** read the normative spec and previous roadmap in full; split runtime,
client, data/ops and business audits across independent reviewers; inspect source
with CodeGraph first; compare active guidance to the five-mode design. The table
below names every document rather than treating a directory search as proof of
alignment. Generic templates were checked for applicability, not filled with
invented project facts. Append-only tracking was checked structurally and recent
rows read; historical implementation claims were not rerun or revalidated.

**Result:** active target requirements now specify text-only play, five selectable
modes, shared rules/value, atomic copy ownership, complete private/public state,
additive migration, explicit compatibility and verified retirement. Current-code
guides retain dated findings rather than falsely claiming fixes. Historical ADRs
and the previous roadmap remain attributable evidence. No source code, SQL,
runtime config, content pack, test, service or deployed artifact changed.

## Main corrections and evidence boundaries

- Blueprint's timer examples now agree with current values (20-second turn,
  five seconds discussion multiplier), while explicitly adopting original table
  size for text discussion rather than current active-connected-count behavior.
- Static-image and specialty/backfill targets are superseded by ADR-012. Ordinary
  UI images, avatar WebP, fonts, QR and visual motion remain legitimate consumers.
- Current draw/privacy, copy identity, whole history, pack pinning, reconnect/
  sequence and lock-order defects are mapped to exact source and test work.
- New settlement/admission identity, shared day caps, interruption handling and
  legacy entitlement treatment prevent the pivot from silently losing value.
- Old backup/runbook claims and full-test scope are corrected; no stale table or
  unhandled Redis command remains an active suggested proof of recovery.
- Business scenarios are labeled assumptions with checked arithmetic. No market,
  legal, platform-price or playtest success is asserted from this audit. Current
  external facts and operator decisions are release evidence tasks, not invented
  values. Public Apple/Google platform-policy pages were checked and linked in the business
  plan; no connected business account was modified and no person was contacted.
- Authoring, certification, human playtests, activation and business release are
  separate gates. Examples and the synthetic fixture are never production proof.

## Complete documentation disposition

“Aligned” means documentation changed now. “Retained” means the file was read
and its purpose remains valid; it does not claim fresh implementation proof.
“Historical” means keep the original text as evidence, with supersession made
explicit by ADR-012, indexes or a banner. The archived roadmap is copied exactly
from the preceding version except for its leading historical-status banner.

| File | Disposition / reason |
|---|---|
| [docs/README.md](../README.md) | Aligned: target/current distinction and complete reading path. |
| [docs/code/API.template.md](../code/API.template.md) | Retained: generic template, no active product requirement or completion claim. |
| [docs/code/ARCHITECTURE.template.md](../code/ARCHITECTURE.template.md) | Retained: generic template, no active product requirement or completion claim. |
| [docs/code/MODULE-media-engine.md](../code/MODULE-media-engine.md) | Aligned: dated current-code audit retained; new target/gaps/retirement reference. |
| [docs/code/MODULE.template.md](../code/MODULE.template.md) | Retained: generic template, no active product requirement or completion claim. |
| [docs/code/README.md](../code/README.md) | Aligned: current mechanics versus transition contract. |
| [docs/design/ADR-001-server-authoritative-over-p2p.md](../design/ADR-001-server-authoritative-over-p2p.md) | Historical decision retained verbatim: ongoing relevance/supersession recorded in design index and ADR-012. |
| [docs/design/ADR-002-flutter-everywhere.md](../design/ADR-002-flutter-everywhere.md) | Historical decision retained verbatim: ongoing relevance/supersession recorded in design index and ADR-012. |
| [docs/design/ADR-003-home-server-behind-cloudflare.md](../design/ADR-003-home-server-behind-cloudflare.md) | Historical decision retained verbatim: ongoing relevance/supersession recorded in design index and ADR-012. |
| [docs/design/ADR-004-media-package-in-server-pkg.md](../design/ADR-004-media-package-in-server-pkg.md) | Historical decision retained verbatim: ongoing relevance/supersession recorded in design index and ADR-012. |
| [docs/design/ADR-005-synthetic-seed-pack.md](../design/ADR-005-synthetic-seed-pack.md) | Historical decision retained verbatim: ongoing relevance/supersession recorded in design index and ADR-012. |
| [docs/design/ADR-006-typography.md](../design/ADR-006-typography.md) | Historical decision retained verbatim: ongoing relevance/supersession recorded in design index and ADR-012. |
| [docs/design/ADR-007-display-typeface.md](../design/ADR-007-display-typeface.md) | Historical decision retained verbatim: ongoing relevance/supersession recorded in design index and ADR-012. |
| [docs/design/ADR-008-support-accents.md](../design/ADR-008-support-accents.md) | Historical decision retained verbatim: ongoing relevance/supersession recorded in design index and ADR-012. |
| [docs/design/ADR-009-open-live-knowoff-ballot.md](../design/ADR-009-open-live-knowoff-ballot.md) | Historical decision retained verbatim: ongoing relevance/supersession recorded in design index and ADR-012. |
| [docs/design/ADR-010-shuffle-sky-accent.md](../design/ADR-010-shuffle-sky-accent.md) | Historical decision retained verbatim: ongoing relevance/supersession recorded in design index and ADR-012. |
| [docs/design/ADR-011-static-image-and-text-content.md](../design/ADR-011-static-image-and-text-content.md) | Historical decision retained verbatim: ongoing relevance/supersession recorded in design index and ADR-012. |
| [docs/design/ADR-012-text-only-selectable-modes.md](../design/ADR-012-text-only-selectable-modes.md) | New: owner text-only pivot and planning contract, explicitly no implementation. |
| [docs/design/ADR.template.md](../design/ADR.template.md) | Retained: generic template, no active product requirement or completion claim. |
| [docs/design/DESIGN-text-game-modes.md](../design/DESIGN-text-game-modes.md) | Aligned: adopted planning defaults, explanatory examples and current runtime disclaimer. |
| [docs/design/DESIGN-text-transition.md](../design/DESIGN-text-transition.md) | New: audited contracts, schema/value retention, compatibility, retirement and test matrix. |
| [docs/design/DESIGN.template.md](../design/DESIGN.template.md) | Retained: generic template, no active product requirement or completion claim. |
| [docs/design/README.md](../design/README.md) | Aligned: new design/ADR and superseded image/specialty/cost assumptions. |
| [docs/guides/AGENT_OPERATING_MODEL.md](../guides/AGENT_OPERATING_MODEL.md) | Retained: framework/runtime guide or index; no text-mode product conflict requiring behavioral change. |
| [docs/guides/CHAT_MODERATION.md](../guides/CHAT_MODERATION.md) | Aligned: text pipeline safety and locale proofs; provider setup remains current implementation. |
| [docs/guides/CLIENT_DEV_TOOLS.md](../guides/CLIENT_DEV_TOOLS.md) | Aligned: current tools documented as legacy; specialty grant retirement and text-debug gates. |
| [docs/guides/CODEX_SETUP.md](../guides/CODEX_SETUP.md) | Retained: framework/runtime guide or index; no text-mode product conflict requiring behavioral change. |
| [docs/guides/COMMUNITY_OPERATIONS.md](../guides/COMMUNITY_OPERATIONS.md) | Aligned: text-only target, actual approval workflow, archived API handoff and remaining operations. |
| [docs/guides/DEV_CREDENTIALS.md](../guides/DEV_CREDENTIALS.md) | Aligned: local legacy MinIO scope and malformed URL formatting; no credential mutation. |
| [docs/guides/LOGGING.md](../guides/LOGGING.md) | Aligned: secret prompt/hand/replay/telemetry access boundaries. |
| [docs/guides/MCP_SETUP.md](../guides/MCP_SETUP.md) | Retained framework setup; corrected two-config-file wording only. |
| [docs/guides/MODEL_PROFILES.md](../guides/MODEL_PROFILES.md) | Retained framework guidance; repaired obsolete skill links only. |
| [docs/guides/README.md](../guides/README.md) | Retained: framework/runtime guide or index; no text-mode product conflict requiring behavioral change. |
| [docs/guides/SCALING.md](../guides/SCALING.md) | Aligned: measured capacity/queue/history load gates; no unsupported capacity promise. |
| [docs/guides/UI_REDESIGN_PLAYBOOK.md](../guides/UI_REDESIGN_PLAYBOOK.md) | Aligned: target text controls, retained dated UI measurements and required new evidence. |
| [docs/launch/HOW_TO_PLAY_CLIP.md](../launch/HOW_TO_PLAY_CLIP.md) | Aligned: text response, open ballots, private rewards, per-mode demo and capture gate. |
| [docs/launch/STORE_COPY.md](../launch/STORE_COPY.md) | Aligned: release-gated text-mode draft, no phantom privacy URL or unverified feature promise. |
| [docs/launch/VPS_MIGRATION_RUNBOOK.md](../launch/VPS_MIGRATION_RUNBOOK.md) | Aligned: source-backed schema inventory, safe rehearsal, one-writer and post-write rollback constraints. |
| [docs/planning/README.md](../planning/README.md) | Aligned: active roadmap and historical snapshot distinction. |
| [docs/planning/ROADMAP-pre-text-20260912.md](../planning/ROADMAP-pre-text-20260912.md) | Historical: original checkboxes/evidence preserved; no active implementation authority. |
| [docs/planning/ROADMAP.md](../planning/ROADMAP.md) | Aligned: seven active phases with source-backed proof gates, all implementation open. |
| [docs/product/BUSINESS_PLAN.md](../product/BUSINESS_PLAN.md) | New: customer/experiment/content/operations/value/economics/launch plan; assumptions explicit. |
| [docs/product/README.md](../product/README.md) | New: product/business reading path and authority. |
| [docs/project/CHARTER.template.md](../project/CHARTER.template.md) | Retained: generic template, no active product requirement or completion claim. |
| [docs/project/DECISION_LOG.template.md](../project/DECISION_LOG.template.md) | Retained: generic template, no active product requirement or completion claim. |
| [docs/project/GLOSSARY.md](../project/GLOSSARY.md) | Aligned: text Nown, five mode names, copy identity and preserved terminology. |
| [docs/project/GLOSSARY.template.md](../project/GLOSSARY.template.md) | Retained: generic template, no active product requirement or completion claim. |
| [docs/project/README.md](../project/README.md) | Aligned: glossary and business plan links; generic templates remain optional. |
| [docs/reports/README.md](../reports/README.md) | Retained: dated report convention; this report is a new planning audit. |
| [docs/tracking/README.md](../tracking/README.md) | Retained: tracking/staging contract and distinction between docs handoff and release readiness. |
| [docs/tracking/context.md](../tracking/context.md) | Aligned: text target/current runtime gaps and scope; old supplier assumptions historical. |
| [docs/tracking/state/.gitkeep](../tracking/state/.gitkeep) | Retained: placeholder; runtime checkpoints are ignored operational state, not product docs. |
| [docs/tracking/tracking.csv](../tracking/tracking.csv) | Retained append-only: prior dirty rows preserved; this task appends truthful plan/review/validation/handoff records. |
| [docs/tracking/tracking.schema.md](../tracking/tracking.schema.md) | Retained: schema/enums; current and appended rows structurally checked, no historical rewrite. |

| [This audit](2026-09-12-text-transition-doc-audit.md) | New: complete disposition and validation boundaries for this planning change. |

## Additional entry points and implementation-time sweep

| Surface | Disposition |
|---|---|
| `BLUEPRINT.md` | Adopted normative text rules, architecture, protocol, secrecy, economy and release targets. |
| `README.md`, `CHANGELOG.md` | Target/current distinction, navigation and documentation-only release note. |
| `content/README.md`, `curator-guide.md`, `humor-development.md`, `tone-matrix.md` | Aligned active text authoring, whole-mode/schedule proofs, cultural/rights/freshness discipline; no candidates generated. |
| `.agents/skills/`, `.github/agents/`, client/server/infra/tools READMEs | Read relevant instructions/source maps for audit; preserve current runtime usage. Roadmap Phase 6 requires final capability/command/skill sweep alongside actual removals. Existing static-image wording in runtime instructions cannot override the new Blueprint target. |
| `server/migrations/000001`–`000008`, old fixtures, historic tracking/ADRs | Retain as migration/test/provenance evidence; new forward migrations and replacement tests are future implementation. |
| Ignored `docs/tracking/state/` | Recovery state only. Preserve inherited compiler failure until its actual cause is resolved; documentation validation is separate. |

The audit does not certify every old framework assertion or every historical
tracking row as current. For example, old ADRs can mention earlier palettes,
blind windows or media cost assumptions; the active Blueprint/ADR-012 explicitly
controls the current target. This preserves history without keeping a second
active product spec.

## Validation and handoff

Required documentation checks: local links/anchors, UTF-8/LF/fences/whitespace,
actual phase counts, archive fidelity, complete current change-set review,
business scenario arithmetic and a no-implementation scope check. Independent
review raised trade/Ready timing, accessible secret semantics, host entitlements,
rematch classification, interrupted-match value, language exposure, executable
red-test sequencing and performance budgets; the integrated plan resolves them.
Final execution evidence is recorded in tracking and the recovery checkpoint.

The inherited unified runtime gate is blocked by disabled CGO/missing C compiler
for the retained avatar WebP encoder. No unchanged failing suite is rerun blindly,
no compiler is installed on the host, and no tests are skipped/deleted to obtain
a documentation handoff. A document-validation pass is not a runtime test pass.
Follow AGENTS.md's applicable-gate handoff and record any required owner decision
truthfully; do not report this plan as deployed or release-ready.
