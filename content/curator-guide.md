# Curator Guide

This guide describes how Knowoff Curators prepare text Nowns, reusable response
and item pools, verify full matches, and screen Weekly Nown Challenge entries.
It follows the [Blueprint](../BLUEPRINT.md) and the text-only five-mode decision
adopted on 2026-09-12 in
[ADR-012](../docs/design/ADR-012-text-only-selectable-modes.md).

Contributor Studio supports text submission and review. The text CLI and
durable release store implement preparation, certification, publication, explicit
activation and takedown. Editorial planning fields remain outside the Studio;
real candidate approval and human pilots remain separate release gates.
Accepted text is curation input, not a certified or deployed game pack.
See [Community operations](../docs/guides/COMMUNITY_OPERATIONS.md) for available
operations and the [transition design](../docs/design/DESIGN-text-transition.md)
for implementation dependencies and current proof.

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

The text runtime consumes explicit reviewed pair bands keyed to immutable
Nown/card revisions and mode. [Tuning](../configs/gameplay/tuning.yaml) sets the
minimum retained High/Distant coverage across the whole scheduled match.
Mandatory embeddings, cosine thresholds and synthetic geometry have retired
from live configuration. Frozen historical policy bytes retain their old fields
only for replay and preservation. Optional text embeddings may assist search;
record compatible evaluator versions and thresholds as evidence. Never
manufacture vectors or alter coverage to make a weak batch appear playable.

Reviewed mode suitability, legal-action simulation and human full-match pilots
must establish that the actual retained 5+3 budget remains usable through every
scheduled Nown and public-state change. Roles use the same dealing procedure;
public seeds are selected independently of the secret prompt/role. No similarity
score judges whether an action is funny, correct or superior.

## Read the dealing path before authoring

For every Nown/card creation, review or integration task, automatically consult
this map before making High/Distant/Chaos or gameplay-readiness claims. Reuse a
reading only while sources remain unchanged; record revisions and open checks.
Use CodeGraph first for indexed source. These are **audited starting paths**;
inspect the current implementation and its recorded proof. A source path
alone does not establish that a mode, human-reviewed pack or deployment is ready.

| Source | What to inspect |
|---|---|
| [Blueprint Game Rules §§2–5 and Media Engine §2](../BLUEPRINT.md) | Five mode actions, provisional 5+3, full-schedule viability, copy ownership, ordinary draws and role secrecy; specialties absent from the first text release. |
| [Gameplay tuning](../configs/gameplay/tuning.yaml) | Current hand/dealing/timer/point values and disabled-until-ready availability; inspect the current typed validation boundary. |
| [Text records](../server/pkg/media/text_types.go), [validation](../server/pkg/media/text_validation.go) and [loader](../server/pkg/media/text_loader.go) | `TextSnapshot` pins exact schema/release/language/rules/wording/provenance, configured bounds and member hashes; reviewed suitability replaces mandatory vectors. |
| [Text dealer](../server/pkg/media/text_dealing.go), [certifier](../server/pkg/media/text_certify.go) and [manager](../server/pkg/media/text_manager.go) | Inspect complete schedule/actual retained coverage, separate system randomness, sampled certificate scope and failed activation. Confirm replay/action evidence and durable lifecycle separately; an in-memory lineage map is insufficient after restart. |
| [Text match](../server/internal/game/text_match.go) and [actions](../server/internal/game/text_actions.go) | Unique physical copies, full secret schedule, private draws, evolving board/trades and consumed-card history; no specialty or semantic judge. |
| [Snapshot projection](../server/internal/game/text_snapshot.go) and [v2 contract](../server/internal/transport/v2/) | Recipient-scoped current Nown/hand, public evidence and bounded checked pages; resolve pinned bytes for every view. |
| [Release store](../server/internal/store/text_release.go) | Exact accepted text/consent capture, immutable durable lineage, separate publish/activate/takedown and retry without contributor reward duplication. |
| [Client contract](../client/lib/core/text/v2_contract.dart), [reducer](../client/lib/core/text/v2_reducer.dart) and [session](../client/lib/core/text/v2_session.dart) | Copy-aware board/history, duplicate and stale stream handling, account-bound restore and private-state clearing. |
| [Match screen](../client/lib/presentation/screens/text_match_screen.dart) and [play screen](../client/lib/presentation/screens/text_play_screen.dart) | Authorized prompt visibility, plain-text readability, five confirmed actions and explicit lobby/mode choices; no downloaded prompt catalog. |
| [Current mechanics](../docs/code/MODULE-media-engine.md), [transition design](../docs/design/DESIGN-text-transition.md) and [Roadmap](../docs/planning/ROADMAP.md) | Reconcile target contracts with actual capability and required proofs before declaring any gap closed. |

The historical dealer, draw-privacy, pack-drift and reconnect findings are
retained in the transition design. Current replacement tests exercise actual
retained hands, private current-turn draws and role-scoped pinned snapshots.
Equal wording may appear in different hands on separate physical copies;
inspect those cases rather than assume global text uniqueness. A passing
engineering test never supplies missing human content evidence or authorizes
activation outside the requested scope.

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
The synthetic fixture builder and sampled retained-card certifier do not
establish this complete contract. Require distinct reachable-action,
human/editorial and activation evidence before calling a release ready.

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
run** or pending. The checked-in synthetic fixtures and successful technical CLI checks do not
complete the human release workflow. Prototype matches receive no
live rewards/progression/leaderboard credit. Release readiness follows the
[Roadmap](../docs/planning/ROADMAP.md), not an aggregate content-count target.

Before declaring a release available, verify the actual activated version,
new-match eligibility and role-scoped text rendering; existing matches retain
pinned wording through history/reconnect/verdict. Keep a compatible certified
text release for rollback. Approval/contribution credit alone proves none of
these states, and a planning retirement date does not alter a deployed pack.
