# Curator Guide

This guide is the rulebook for Knowoff **Curators**: players who create Nowns and the cards that relate to them, test hands in the deal simulator, and screen Weekly Nown Challenge entries before they go public.

It describes the target curation and pack-release workflow. The current
Contributor Studio supports text submission and review; Nown/deck authoring,
editorial planning fields and pack publishing are not implemented there.
Accepted text is an input to curation, not a certified or deployed game pack.
See [Community operations](../docs/guides/COMMUNITY_OPERATIONS.md) for the
available operations.

## What a Curator does

- Create media submissions that become **Nowns** (the round's secret item).
- Author **cards** — static images and plain text — against candidate Nowns
  in the shared pool.
- Use the **deal simulator** to verify that every Nown has full band coverage at both 4- and 6-player table sizes.
- Screen Weekly Nown Challenge entries before they become publicly visible and votable.

## The relevance mesh

Every Nown and card carries an embedding in one shared multimodal space. Similarity, not tag intersection, is the balance mechanism.

A card's band is relative to a particular Nown, so it can be high for one Nown
and distant or chaos for another. Writing cards "against" a Nown is an
authoring method, not exclusive ownership of those cards by that Nown. Tone
buckets, editorial dimensions and recurring-character callbacks do not replace
these relationships; the `chaos` tone bucket is not the chaos relevance band.

Relevance bands (cosine similarity):

- **High** ≥ `dealing.band_high`
- **Distant** in [`dealing.band_low`, `dealing.band_high`)
- **Chaos** < `dealing.band_low`

A certifiable pack must support the Blueprint's constraint deal against every
scheduled Nown: at least `dealing.min_high_per_nown` high cards and
`dealing.min_distant_per_nown` distant cards per player across the initial
5-card hand and 3-card personal draw pile. Certification must establish
feasibility at 6 players as well as test the 4-player deal.

## Read the dealing path before authoring

For every Nown/card creation, review or integration task, automatically read
the following sources before labeling **High, Distant or Chaos** relationships.
Do not wait for the user to ask about game logic. Reuse a reading within a task
only while the sources remain unchanged; record consulted revisions and open
checks in the editorial record. Use CodeGraph first for indexed source files.

| Source | What to inspect |
|---|---|
| [Blueprint Game Rules §§2–5 and Media Engine §2](../BLUEPRINT.md) | Target 5-card hand plus 3-card reserve, all-scheduled-Nown coverage, draws, Shuffle and role secrecy. |
| [Gameplay tuning](../configs/gameplay/tuning.yaml) | Current `dealing.band_high`, `band_low`, `min_high_per_nown`, `min_distant_per_nown`, `hand` and relevant game/point settings. Read values here rather than copying remembered constants. |
| [Server mesh and dealer](../server/pkg/media/dealing.go) | `Cosine`, `BandFor`, `BuildCandidates`, `Dealer.Deal` and `DealFeasible`: vector comparison, band lists, sampling, retained hands/reserves and what simulation actually proves. |
| [Pack loader](../server/pkg/media/loader.go) and [certifier](../server/pkg/media/certify.go) | `LoadPack` and `Certify`: valid image/text data, compatible embedding declarations, candidate construction and certification coverage. |
| [Server match](../server/internal/game/match.go) | `buildNownSchedule`, `dealHands`, `handleDrawCards`, `useShuffle`, `sendHandDealt`, `viewFor` and `playedNowns`: secret schedule, actual mutations, recipient scopes and final reveals. |
| [Server payload renderer](../server/internal/game/payload.go) | `NownPayload`, `CardPayload` and `mediaItemPayload`: decoy versus Nown payloads, inline text, signed image URLs and absence of relevance scores from player payloads. |
| [Client DTOs](../client/lib/data/models/game_state_dto.dart) and [session state](../client/lib/presentation/state/game_session_provider.dart) | `CardDto`, `NownRefDto`, `HandDto` and `GameSessionNotifier._onMessage`/`_mergeState`: consume the server's hand/round events; no client band assignment or local dealing. |
| [Client role view](../client/lib/domain/entities/game_session.dart), [game screen](../client/lib/presentation/screens/game_screen.dart) and [media surfaces](../client/lib/presentation/widgets/game_surfaces.dart) | `showNown`/`showDecoy`, `GameScreen`, `GameMediaWell` and `GameCardTile`: image/text rendering and placeholders. Display guards supplement server secrecy; they cannot establish it alone. |
| [Client media engine](../client/lib/media/media_engine.dart) | `MediaEngine.syncPack` and `prefetchNown`: metadata sync, server-issued asset URLs and cache behavior, not a second relevance engine. Check live call sites before claiming the separate prefetch service is wired into a screen. |
| [Current mechanics and gaps](../docs/code/MODULE-media-engine.md) and [roadmap](../docs/planning/ROADMAP.md#content-readiness-audit--2026-09-11) | Reconcile the target with the verified implementation, including retained-hand coverage, Shuffle, draw privacy and pack isolation. Confirm source/tests before calling an audited gap closed. |

The server constructs many-to-many Nown/card bands using cosine similarity;
the client receives display data and sends player intents. Neither player
payloads nor card faces should disclose a High/Distant/Chaos answer label.
One card may be High for one Nown and Chaos for another. Tone, shared tags,
format, callbacks and editorial counts do not set the band's value, and an
editor's proposed band remains a hypothesis until compatible real embeddings
measure it. Do not manufacture vectors or tune thresholds to make a batch fit.

The 2026-09-11 audit found that the dealer retains its first Nown's selected
hand, then fills the reserve from later selections; its filler draws from all
bands, and it does not verify the final eight cards against every scheduled
Nown. Its per-player uniqueness check does not prevent the same card appearing
in different players' hands. Successful sampling alone therefore does not prove
the Blueprint's complete guarantee. Recheck this behavior for a new task rather
than treating this dated description as permanent implementation truth.

During rounds, the server sends the real Nown only to active Nowers; Donowers
and eliminated players receive a decoy. The final reveal includes only Nowns
from rounds actually begun. Initial hands/reserves go to their owner. Inspect
the existing draw-privacy gap separately instead of claiming all current card
delivery is private. Preserve these rules and report gaps while creating content;
content work does not implicitly authorize changing the algorithm.

## Coverage checklist

Before submitting a Nown for certification:

1. Run the deal simulator at both table sizes.
2. Confirm every player hand has strong, stretchy, and garbage options.
3. Confirm every band has enough candidates so the RNG does not fall back to duplicates.
4. Give every Nown and card exactly one tag from the
   [four-bucket tone rubric](tone-matrix.md).
5. Playtest real 4- and 6-player hands with target-language players. Record
   recognition, laughter, plausible alternative explanations and references
   that needed explanation. Check that a joke neither exposes Nown by itself
   nor removes the ambiguity needed for Donowers to bluff.

Simulation proves deal feasibility; human playtests judge whether that deal
is recognizable, funny and ambiguous enough to play. Neither replaces the other.
The current dealer and simulator do not yet prove the full requirement across
every scheduled Nown. Treat a successful current simulation as a partial
technical check, not proof of that guarantee; the remaining work is tracked in
the [roadmap](../docs/planning/ROADMAP.md).

## Visual review and media mix

Apply the Blueprint's [playable media direction](../BLUEPRINT.md#playable-media-direction)
and the [historical context and briefs](humor-development.md#visual-direction-and-history).
Use static images and plain text only, choosing the format that carries the
joke. Record completed image/text counts for Nown and playable-card pools
separately from concepts. There is no required format ratio or hand quota;
format counts do not change draw weights.

Review the **actual compressed static image at card size**: does the everyday
situation read, does the awkward gesture or crop supply an abrupt visual joke,
and does it leave room to argue? Preserve rough framing,
modest detail and compression while keeping the premise recognizable. Rework
polished studio/cinematic/illustrated output even when it fits the pixel cap;
do not prescribe upscaling, smoothing or beautification as the default fix.
The UI palette, doodles and chrome effects are not a pack-media template.

Record source/output dimensions, encoding, bytes, single-frame validation and
visual review against the Blueprint limits. GIFs, animated WebP and video are
unsupported game formats. Missing or unviewed media remains an unresolved
check; metadata and an AI description do not prove visual fit. Captions and
planning prose are not extra text cards.

## Editorial planning and freshness

Use [Humor development](humor-development.md) for the writing loop. Begin the
pilot with three themes and two target cultures/languages, then draft image/text
mechanisms per theme for a human editor to select, trim or rewrite. The pilot
is planned; the guide's example lines are not created or published assets.

Record human situation, comic mechanism, cultural reach, shelf life and
accessibility alongside each candidate and its human decisions in planning
records. These are editorial dimensions, not additional tone buckets or
runtime/dealing fields. Aim initially for a **70% evergreen / 20% seasonal or
cultural / 10% topical** release mix, independently of tone balance. It is a
playtest hypothesis to revise with feedback and reuse data, not a quota or a
sampling weight.

For every topical candidate, retain its source, observation date, intended
regions/languages, one-sentence context, review date and expiry date. Verify
factual premises through original sources or reliable reporting; trend
popularity is not verification. A human editor reviews these candidates
weekly and decides whether to retain, rewrite or retire them, including at
expiry. No automated scheduling, expiry or deployed-pack removal is implied.

Ask regional contributors to recreate jokes for their audience and repeat
playtests in each culture/language; literal translation is not enough. Use
recurring characters and callbacks sparingly, and preserve clear language for
rules, consent, errors and moderation decisions.

## Tone and content standard

- Humor may be suggestive/erotic within app-store rules — cartoon, drawn, abstract.
- Never pornographic or explicit.
- Erotic-leaning media ships only in adult-rated, age-gated packs.
- Record license and attribution for every asset.
- A human checks sources, rights, originality, age suitability and local
  meaning before release; write original material rather than copying a trend.
- Require both automated screening and human review. A safety provider does
  not determine funniness, ownership, factual accuracy or cultural fit.
- Satire may target institutions, powerful figures and everyday frustrations;
  do not make victims of a current tragedy the punchline.

## Submission rules

- A submission is **immutable once submitted**.
- Withdraw and resubmit costs the queue slot.
- Every upload requires explicit acceptance of the contribution terms; the accepted version and timestamp are stored with the submission.

## Challenge screening

- Screen entries before they become visible or votable.
- Rejected entries never appear and reopen a slot.
- One active vote per player per week; self-votes are forbidden.

## Certification

A pack version is publishable only when:

- It declares its BCP 47 language tag.
- Every Nown has full band coverage for a 6-player deal.
- Every card is reachable in some band.
- Monte Carlo `simulate` confirms deal feasibility at both table sizes.
- Human review and real-hand playtests satisfy the editorial checks above.

Complete the media pipeline's certification, bundling, publication and runtime
activation for the reviewed version before calling it available in the app.
Submission approval and contribution credit alone do not establish any of
those release states. Revise or retire weak cards using player feedback; a
planning decision to retire a card is not evidence that a deployed pack changed.
