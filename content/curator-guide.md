# Curator Guide

This guide is the rulebook for Knowoff **Curators**: players who create Nowns and the cards that relate to them, test hands in the deal simulator, and screen Weekly Nown Challenge entries before they go public.

## What a Curator does

- Create media submissions that become **Nowns** (the round's secret item).
- Author **cards** — text, image, or GIF prompts — against each Nown.
- Use the **deal simulator** to verify that every Nown has full band coverage at both 4- and 6-player table sizes.
- Screen Weekly Nown Challenge entries before they become publicly visible and votable.

## The relevance mesh

Every Nown and card carries an embedding in one shared multimodal space. Similarity, not tag intersection, is the balance mechanism.

Relevance bands (cosine similarity):

- **High** ≥ `dealing.band_high`
- **Distant** in [`dealing.band_low`, `dealing.band_high`)
- **Chaos** < `dealing.band_low`

A certifiable pack must guarantee that, for every Nown, the mesh can deal hands containing at least `dealing.min_high_per_nown` high cards and `dealing.min_distant_per_nown` distant cards per player at 6 players.

## Coverage checklist

Before submitting a Nown for certification:

1. Run the deal simulator at both table sizes.
2. Confirm every player hand has strong, stretchy, and garbage options.
3. Confirm every band has enough candidates so the RNG does not fall back to duplicates.
4. Tag the Nown and cards into the four-bucket tone rubric (`content/tone-matrix.md`).

## Tone and content standard

- Humor may be suggestive/erotic within app-store rules — cartoon, drawn, abstract.
- Never pornographic or explicit.
- Erotic-leaning media ships only in adult-rated, age-gated packs.
- Prefer copyleft-first sources; record license and attribution for every asset.

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
