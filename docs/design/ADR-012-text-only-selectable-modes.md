# ADR-012: Text-only Knowoff with five selectable modes

- **Status:** Accepted product direction and planning contract; implementation pending.
- **Date:** 2026-09-12.
- **Decider:** Project owner (text pivot and comprehensive planning request).
- **Supersedes:** [ADR-011](ADR-011-static-image-and-text-content.md)'s playable-image allowance; the association-game/specialty/backfill first-release plan. [ADR-010](ADR-010-shuffle-sky-accent.md)'s active Shuffle use becomes historical at cutover.

## Context

The owner chose text-based Knowoff and retained five selectable concepts from
[the mode design](DESIGN-text-game-modes.md). Existing code still implements an
image/text association game; source audit exposed secrecy, history, reconnect,
settlement and migration gaps that a format-only switch would carry forward.
The owner requests plans and documentation before any code implementation.

## Decision

Adopt [Blueprint](../../BLUEPRINT.md)'s text-only five-mode target, and execute
the [Roadmap](../planning/ROADMAP.md) through additive data migration, explicit
protocol cutover, per-mode evidence gates and verified retirement of old paths.

## Consequences

- Missed the Briefing, Secret Scale, Make Room, Bad Bargains and Top That remain
  the intended offering; default to Missed the Briefing and expose each only
  after its engineering/content/business gates. Quick Play/Local Room remain
  entry paths, distinct from gameplay modes and content language.
- No first-release specialty powers or production bot backfill. Shared roles,
  voting, economy, identity and visual language remain, with discovered defects
  repaired against their intended contracts.
- Text Nowns/cards/challenge submissions use reviewed mode/language suitability;
  private prompts arrive only via authorized server payloads. Whole public
  catalogs and image prefetch cannot be retained as a secret-delivery mechanism.
- Add unique card-copy identity, complete ordered history, atomic mode actions,
  trade response deadlines, real reconnect and durable idempotent settlement.
- Preserve accounts, ledgers, entitlements, provenance, consent, reports and
  applied migrations. Non-playable avatars/icons/fonts/QR/store imagery remain;
  avatar WebP still requires a valid native build toolchain.
- Retire executable old behavior after drain/rollback gates, including config,
  routes, dependencies, caches, fixtures, localization and dev controls. Historical
  ADRs/tracking/roadmap are labeled records, not a second active architecture.
- Five modes increase editorial, QA and queue-liquidity demands. Human pilots,
  per-cell metrics and measured costs must validate release. No flawless outcome,
  market demand, profitable business or implemented capability is claimed here.
- Reversing the pivot later requires another explicit decision; it is not a
  feature-flag fallback. The technical defaults are adopted for implementation
  planning and may be revised with a documented, versioned playtest decision.

## Considered options

- **Text-only, five shared-engine modes (chosen):** matches the owner's direction,
  preserves Knowoff's identity and supports distinct simple actions.
- **Only change card formats:** leaves gameplay/action/history and integrity gaps
  unresolved and does not deliver the selected offering.
- **Retain the old image game alongside text:** adds permanent compatibility,
  content and testing costs and conflicts with removal of obsolete architecture.
- **Pick only one permanent mode:** lowers initial scope but discards four
  selected concepts. Staged exposure addresses capacity without that product change.

## Validation and reversibility

[Transition design](DESIGN-text-transition.md) records source-backed risks,
retention and proof requirements. [Business plan](../product/BUSINESS_PLAN.md)
records hypotheses, ownership and launch decisions. Planning changes no runtime,
DB or assets. Deployment rollback uses a compatible text artifact and the current
durable database; it never silently restores a stale financial snapshot.
