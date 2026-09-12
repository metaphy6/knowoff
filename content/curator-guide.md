# Curator Guide

This guide describes how Knowoff Curators prepare text Nowns, reusable response
and item pools, verify full matches, and screen Weekly Nown Challenge entries.
It follows the [Blueprint](../BLUEPRINT.md) and the text-only five-mode decision
adopted on 2026-09-12 in
[ADR-012](../docs/design/ADR-012-text-only-selectable-modes.md).

This is the target curation/release workflow. The current Contributor Studio
supports text submission and review; mode-aware Nown/pool authoring, editorial
planning fields and production pack activation remain transition work.
Accepted text is curation input, not a certified or deployed game pack.
See [Community operations](../docs/guides/COMMUNITY_OPERATIONS.md) for available
operations and the [transition design](../docs/design/DESIGN-text-transition.md)
for implementation dependencies. No pack or runtime changes in this guide edit.

## What a Curator does

- Write short text **Nowns**: situations for Missed the Briefing, criteria for
  Secret Scale/Top That, and plans for Make Room/Bad Bargains.
- Author reusable **response** or **item** cards in explicit language/mode
  pools; keep several defensible readings without uniquely exposing a Nown.
- Review legal actions and actual retained hands/reserves across complete
  2/3-round schedules at both 4- and 6-player sizes, including changed boards.
- Screen Weekly Nown Challenge text before it becomes public and votable.
- Preserve exact revisions, language, rights/terms, human decisions and evidence
  separately from automated screening, certification, publication and activation.

## The relevance mesh

High/Distant/Chaos are candidate relationship evidence, not correct answers,
role clues, rating targets, item prices or automatic rewards. A card can support
several Nowns; writing against one prompt does not give it exclusive ownership
of that card. Tone buckets, tags, callbacks and the `chaos` humor bucket remain
separate from relevance bands.

The **current association runtime** computes cosine bands from embeddings:

- **High** ≥ `dealing.band_high`.
- **Distant** in [`dealing.band_low`, `dealing.band_high`).
- **Chaos** < `dealing.band_low`.

Read actual values in [tuning](../configs/gameplay/tuning.yaml). Those definitions
explain the existing dealer; they are not a sufficient text-mode release gate.
The target retires mandatory multimodal embeddings and synthetic geometry as
production evidence. Optional text embeddings may assist search; when used,
record compatible model/evaluator versions and thresholds. Never manufacture
vectors or alter thresholds to make a weak batch appear playable.

Reviewed mode suitability, legal-action simulation and human full-match pilots
must establish that the actual retained 5+3 budget remains usable through every
scheduled Nown and public-state change. Roles use the same dealing procedure;
public seeds are selected independently of the secret prompt/role. No similarity
score judges whether an action is funny, correct or superior.

## Read the dealing path before authoring

For every Nown/card creation, review or integration task, automatically consult
this map before making High/Distant/Chaos or gameplay-readiness claims. Reuse a
reading only while sources remain unchanged; record revisions and open checks.
Use CodeGraph first for indexed source. These are **audited starting paths**,
not evidence that text-mode contracts already exist.

| Source | What to inspect |
|---|---|
| [Blueprint Game Rules §§2–5 and Media Engine §2](../BLUEPRINT.md) | Five mode actions, provisional 5+3, full-schedule viability, copy ownership, ordinary draws and role secrecy; specialties absent from the first text release. |
| [Gameplay tuning](../configs/gameplay/tuning.yaml) | Current hand/dealing/timer/point values and legacy specialty/backfill settings. This docs-only transition has not changed those values. |
| [Server mesh and dealer](../server/pkg/media/dealing.go) | `Cosine`, `BandFor`, `BuildCandidates`, `Dealer.Deal`, `DealFeasible`: existing vector bands, sampled versus retained cards, and actual simulation coverage. |
| [Loader](../server/pkg/media/loader.go) and [certifier](../server/pkg/media/certify.go) | `LoadPack`/`Certify` currently validate the old image/text schema and embeddings; planned text-only mode/language/action certificates are additional work. |
| [Server match](../server/internal/game/match.go) | `buildNownSchedule`, `dealHands`, `handleDrawCards`, `sendHandDealt`, `viewFor`, `playedNowns`: schedule, mutation, recipient scopes and verdict. `useShuffle` is a legacy retirement surface, not a required text action. |
| [Payload renderer](../server/internal/game/payload.go) and [active manager](../server/pkg/media/manager.go) | Nown/decoy and inline-text projections; current signed-image branch retires. Ensure existing matches resolve pinned bytes instead of looking up the newest active pack. |
| [Client DTOs](../client/lib/data/models/game_state_dto.dart) and [session state](../client/lib/presentation/state/game_session_provider.dart) | Instance-aware hand/board/history consumption, duplicate request/event handling and snapshot restore. Current content-ID plays maps cannot model all five modes. |
| [Role view](../client/lib/domain/entities/game_session.dart), [game screen](../client/lib/presentation/screens/game_screen.dart) and [surfaces](../client/lib/presentation/widgets/game_surfaces.dart) | Authorized prompt visibility, plain-text readability and five confirmed action controls. Visual hiding does not establish wire secrecy; elimination must clear stale private state. |
| [Client media engine](../client/lib/media/media_engine.dart) | Legacy metadata/prefetch/cache path to inventory and retire. Its constructor had only a test caller in the 2026-09-12 CodeGraph audit; do not claim it is the live text renderer. |
| [Current mechanics](../docs/code/MODULE-media-engine.md), [transition design](../docs/design/DESIGN-text-transition.md) and [Roadmap](../docs/planning/ROADMAP.md) | Reconcile target contracts with actual capability and required proofs before declaring any gap closed. |

The 2026-09-11 dealer audit found first-Nown hand retention and partial later-Nown
reserve retention, not verification of the final eight cards against the complete
schedule. Per-player content-ID uniqueness does not prevent the same content
appearing in different hands. The target explicitly permits equal wording on
separate copies; fairness/certification must inspect those cases rather than
assume global uniqueness or use duplicate text as duplicate ownership.

The 2026-09-12 runtime audit also found out-of-turn draws and public drawn-card
payloads, current-round-only plays, incomplete pack pinning, and no authoritative
snapshot path in the observed rejoin handler. Existing scope tests do show
Nower/decoy routing and started-Nowns-only verdict behavior; those partial proofs
do not close the other gaps. Recheck source/tests during implementation. Content
planning does not authorize repairing runtime, changing packs or activating data.

## Coverage checklist

Before certifying a candidate release:

1. Identify exact Nown/card revisions, response/item pools, intended modes,
   canonical content language and compatible rules version. Keep role and future
   prompt schedule out of player-facing catalogs, IDs and metadata.
2. Verify complete feasible schedules with sufficient distinct Nowns, both table
   sizes and the actual retained hand/reserve budget. Invalid setup rejects before
   access is consumed; no repeat-last-Nown or revealing fallback rescues a pack.
3. Test each mode's evidence-producing choices and neutral public seeds:

   | Mode | Content/action proof |
   |---|---|
   | Missed the Briefing | Responses work across unrelated situations and have awkward contexts; no universal safe line or uniquely identifying response; test Donower opening first. |
   | Secret Scale | Several defensible 1–5 placements from real hands; public endpoints never name the hidden criterion; no automatic correct-rating score. |
   | Make Room | Three distinct-text system seeds independent of Nown; meaningful remove/add options after each replacement; history preserves removed cards and later restoration needs a different owned copy. |
   | Bad Bargains | Plausible exchanges across evolving displays and hands; acceptance preserves proposer hand size; refusal leaves offered copy playable later but permanently public in evidence. |
   | Top That | Arguable comparison pairs through complete chains, including awkward hands; any owned card remains legal without an AI judge or veto. |

4. Test optional draws, depletion, timeout penalty evidence, no refill/recycling,
   fresh-board resets and full historical evidence. Specialties/backfill are off
   for every first-release mode; test drivers must not depend on them.
5. Give each Nown and card exactly one [tone bucket](tone-matrix.md), separately
   from mode suitability, optional relevance evidence and freshness mix.
6. Conduct actual 4/6-player target-language full-match pilots. Record recognition,
   laughter, defensible alternatives, explanations needed, comprehension, opening
   and late-seat effects, draw pressure, completion times and repeat exposure.
   Record tested revisions and participant observations; never invent them.
7. Verify readable small-screen/expanded-text/assistive-technology presentation,
   mode confirmation controls, role-scoped snapshot/history, immutable pack
   wording and started-Nowns-only verdict with implementation evidence.

Simulation addresses the cases it actually checks; exhaustive small fixtures and
property tests complement sampled production schedules. Monte Carlo cannot alone
prove every reachable state, and human playtests cannot prove wire secrecy.
The current synthetic builder/certifier does not establish this complete contract.
A green current simulation is partial evidence, not a text release certificate.

## Text review and evidence

Apply the [text direction](../BLUEPRINT.md#playable-media-direction) and
[humor briefs/history](humor-development.md#visual-direction-and-history).
Prompts roughly 5–10 words and cards roughly 1–4 words are editorial targets,
not universal limits. Use the versioned configurable UTF-8 bound, appropriate
Unicode normalization/control-character validation and grapheme-aware layout.
Render escaped plain text; no executable HTML/Markdown or external asset URL.

Inspect actual text in hand and evidence views: legibility, script coverage,
recognizable premise, concise wording and multiple plausible interpretations.
Record completed Nown/response/item counts per mode/language separately from
concepts, duplicate localizations and physical runtime copies. A caption,
image alt-text or planning description is not an extra approved card.

New playable releases contain no image/GIF/video assets. Preserve historical
image/source/license/moderation records according to the transition retention
plan; do not delete provenance or generate substitute text from filenames.
Avatars, store art and the how-to clip have separate non-playable requirements.

## Editorial planning and freshness

Use [Humor development](humor-development.md) for a bounded pilot with three
themes and two cultural/language contexts. Draft mode-suitable text mechanisms
for a human editor to select, trim or rewrite. This is a pilot plan, not evidence
of finished candidates or a commitment to multiple public launch queues.

Record human situation, comic mechanism, cultural reach, shelf life and
accessibility alongside each exact candidate revision and human decision.
These are editorial dimensions, not additional tone buckets or dealing weights.
The **70% evergreen / 20% seasonal or cultural / 10% topical** release mix is an
experiment, independent of tone balance; revise it from feedback/reuse data.

Retain each topical candidate's source, observation date, region/language,
one-sentence context, review date and expiry date. Verify factual premises
through original sources or reliable reporting; popularity is not verification.
A human editor reviews topical candidates weekly and decides whether to retain,
rewrite or retire them. No automatic schedule/expiry/removal job is implied.

Ask regional contributors to recreate jokes and repeat full-match pilots in their
culture/language; literal translation does not establish ambiguity. Rotate
callbacks and keep rules, consent, errors and moderation decisions clear.

## Tone and content standard

- Humor may be suggestive within the existing content policy; never pornographic
  or explicit. Adult-oriented text remains restricted to adult-rated, age-gated
  packs under the product baseline.
- Record rights, source, license/attribution and contribution terms for every
  accepted revision. Human review checks originality, age suitability and local
  meaning; write original material rather than copying a trend.
- Require automated screening plus human review. Screening does not determine
  funniness, ownership, factual accuracy or cultural fit.
- Satire may target institutions, powerful figures and everyday frustrations;
  do not make victims of a current tragedy the punchline.

## Submission rules

- A submission is immutable once submitted; edits create a separately reviewed
  revision through the supported workflow.
- Withdraw and resubmit costs the queue slot. Pack preparation cannot invent
  a new approval or contribution reward.
- Every contribution requires explicit terms acceptance, with accepted version
  and timestamp retained. Editorial planning supplements that durable history.

## Challenge screening

The Weekly Nown Challenge remains a separate text contribution event, not a
sixth gameplay mode or an action-correctness contest inside a match.

- Screen entries before they become visible or votable.
- Rejected entries never appear and reopen a slot.
- One active vote per player per week; self-votes are forbidden.
- Challenge approval/winner records remain distinct from reusable response/item
  certification; winning does not automatically put a card into a live pack.

## Certification

A new text release requires immutable schema/release/content revisions; explicit
mode/language/rules compatibility; source/rights/age evidence; hashes and valid
plain text; full-schedule/action viability at both sizes; human review and
full-match pilots; recipient-scoped delivery and match-isolation proofs.
Optional embedding checks carry evaluator versions but cannot replace these.

Record separate states: prepared, screened, human-reviewed, playtested,
technically certified, published and activated. Missing evidence remains **not
run** or pending. The checked-in synthetic fixture and directory-copy publish
command do not complete the production workflow. Prototype matches receive no
live rewards/progression/leaderboard credit. Release readiness follows the
[Roadmap](../docs/planning/ROADMAP.md), not an aggregate content-count target.

Before declaring a release available, verify the actual activated version,
new-match eligibility and role-scoped text rendering; existing matches retain
pinned wording through history/reconnect/verdict. Keep a compatible certified
text release for rollback. Approval/contribution credit alone proves none of
these states, and a planning retirement date does not alter a deployed pack.
