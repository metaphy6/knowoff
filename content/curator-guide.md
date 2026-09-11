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
- Author **cards** — text, image, or GIF prompts — against each Nown.
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

## Editorial planning and freshness

Use [Humor development](humor-development.md) for the writing loop. Begin the
pilot with three themes and two target cultures/languages, then draft several
comic mechanisms per theme for a human editor to select and rewrite. The pilot
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
