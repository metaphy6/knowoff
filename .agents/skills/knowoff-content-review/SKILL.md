---
name: knowoff-content-review
description: Evaluate or rework Knowoff Nowns, cards and localizations for lo-fi GIF-first style, humor, ambiguity and content evidence. Use for editorial quality review and playtest-evidence assessment; recommendations do not approve submissions or release packs.
---

# Knowoff content review

Give item-specific editorial recommendations that preserve both humor and
social deduction. Continue useful draft review when release tooling or human
evidence is unavailable; identify the missing evidence without inventing it.

## Read the current contract

For each new batch or material revision, read these current repository sources
before judging it; an earlier conversation summary is not the contract:

- [Blueprint](../../../BLUEPRINT.md): terminology, Game Rules §§2–3,
  Contributor Portal §§1–3, Media Engine §§1–4 (including Humor development
  and editorial release), [playable media direction](../../../BLUEPRINT.md#playable-media-direction),
  and Product Baseline content/localization rules.
  This is the normative product and content specification.
- [Humor development](../../../content/humor-development.md),
  [Curator guide](../../../content/curator-guide.md), and
  [Tone matrix](../../../content/tone-matrix.md): writing/review method,
  certification expectations, and the existing tone vocabulary.
- [Server content and dealing](../../../docs/code/MODULE-media-engine.md)
  and [Community operations](../../../docs/guides/COMMUNITY_OPERATIONS.md):
  what the current dealer, simulator, and submission workflow actually support.
- [Roadmap content readiness audit](../../../docs/planning/ROADMAP.md#content-readiness-audit--2026-09-11):
  open proof gaps and sequencing; inspect newer updates if they supersede it.

Follow [AGENTS.md](../../../AGENTS.md) for cross-cutting rules. The Blueprint
wins product conflicts; the editorial guides apply it, and implementation
audits describe capability rather than weakening the target. Flag conflicting
or missing sources for the affected decision while continuing independent
review. Read release mix, quality targets, and dealing thresholds from their
current sources, including [tuning.yaml](../../../configs/gameplay/tuning.yaml)
when checking technical results; do not copy tunable values into this skill.

## Review the candidates

1. Identify the candidate IDs and exact revisions, Nown versus playable card,
   intended language/region and audience rating, related candidate Nowns, and
   available source, editorial, screening, simulation, and playtest records.
   Keep these evidence types separate. Missing context becomes an unresolved
   check, not a made-up audience, test result, or approval.
2. Check the recognizable human situation, unexpected interpretation,
   speakability, comic mechanism, reference burden, and repeated/callback
   fatigue. Explain what makes this particular item work or fail. Popularity
   or an AI score cannot establish comic quality.
3. Assess plausible alternative explanations against candidate Nowns for drafts
   and every scheduled Nown for gameplay review; do not invent a match schedule.
   Flag lines that identify Nown by themselves, imply a role, or leave Donowers
   no defensible bluff; also flag obscurity that leaves everyone guessing
   without a recognizable connection. Proposed explanations are reviewer
   hypotheses until real players supply evidence.
4. Check exactly one existing tone bucket per asset using the tone matrix.
   Review its current pack-mix guidance separately from the experimental
   freshness mix. Human situation, comic mechanism, cultural reach, shelf life,
   and accessibility belong in planning records, not extra buckets, runtime
   fields, demographic inferences, hand quotas, or draw weights.
5. Review originality, asset license/attribution, source accuracy, suitability
   for the declared rating, and local meaning. Verify topical factual premises
   separately from trend popularity; retain source, observation date, intended
   regions/languages, context, review date, and expiry date. Use the guide's
   human review cadence; a date is not an automated removal job. Do not make
   victims of a current tragedy the punchline. Apply the Blueprint's explicit
   content limits and adult-pack restrictions without inventing broader bans.
6. Evaluate each localization in its own culture: recreating the joke may
   require different wording or references. Record what changed and why;
   familiarity in one language cannot prove recognition or ambiguity elsewhere.
7. For gameplay-readiness review, inspect actual 4- and 6-player hands across
   every scheduled Nown, including retained hands/reserves and relevant redeals.
   If no hands exist yet, review relationship hypotheses and list that evidence
   as not run; continue draft review without claiming coverage. Cards share a many-to-many
   Nown relevance mesh; tone, tags, callbacks, and authoring intentions neither
   prove a band nor create exclusive decks or card-to-card dependencies.
   Read measured similarity as technical evidence, never a correct-answer or
   funniness score; the chaos relevance band is separate from the chaos tone.

## Review the delivered look and motion

Inspect the actual compressed asset at card size and watch each complete loop,
including its reset. Look for a readable everyday action, candid awkwardness,
rough crop, modest detail and natural abruptness. Flag glossy lighting, pristine
3D or illustration and polished brand scenes even if downsized under the limit.
UI palette, doodle and zero-blur rules do not govern imagery inside media packs.

Check actual dimensions, bytes and encoding against the Blueprint: stills are
usually below 720 px longest side (720 is the maximum); GIF loops are ≤480p,
≤2 MB and delivered as animated WebP. A static frame, storyboard or pan/zoom over
a still is not a completed reaction/action GIF. If the asset or playback cannot
be inspected, mark that visual/motion check **not run**, rather than treating a
prompt, filename or technical metadata as proof of style or action.

For full mixed batches/releases, check Nown and playable-card pools separately:
GIF loops form the majority, stills follow and text-only assets have a smaller
share. Count actual assets separately from concepts; captions, translations and
planning descriptions are not extra text cards. Respect small or explicitly
format-scoped briefs and record deliberate departures. Keep media priority
separate from tone/freshness mixes and runtime weights or hand quotas.

## Keep evidence honest

- Real target-language playtests must record recognition, laughter, plausible
  alternative explanations, and references needing explanation at both table
  sizes. Attribute observations to their actual records and tested revisions;
  missing or stale human evidence remains pending. Never fabricate participants,
  laughter, human selection, safety review, consent, or approval.
- Automated screening supplements human review; it does not establish rights,
  truth, cultural fit, or humor. Simulation addresses technical feasibility;
  assess its actual coverage against the current readiness audit. Synthetic
  fixture vectors do not prove semantic quality. A simulator establishes the
  all-Nown guarantee only if its current implementation checks the final hands
  against the full contract; recheck the source when an audit gap is closed.
- Report each item's recommendation: **keep** for further curation, **rewrite**
  with a concrete change, **hold** for a named uncertainty, or **reject** with
  its specific reason. Include item/revision, evidence and source references,
  suggested revision where useful, and unresolved human/technical checks.
  Keep editorial recommendations separate from release readiness; “keep” is
  neither official submission approval, certification, nor deployment.
- Preserve authoritative submission decisions in the existing workflow; an
  editorial record references that history rather than inventing approval state.
  Route requested revisions through [content creation](../knowoff-content-create/SKILL.md)
  and requested packaging or app integration through
  [content integration](../knowoff-content-integrate/SKILL.md).

## Tracking handoff

For a saved review or revision, use the
[commit candidate handoff](../../../docs/tracking/README.md#commit-candidate-handoff)
and [CSV schema](../../../docs/tracking/tracking.schema.md). A `review` or `note`
row alone never appears as a commit candidate. The coordinator registers the
completed repository change with `commit/completed/pending`, stages it and
verifies its summary/run ID through `make git.dry`. A read-only child returns
evidence to the coordinator without duplicate completion rows or staging;
a chat-only review with no repository change needs no commit candidate.
