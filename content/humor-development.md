# Humor development

Editorial guidance proposed on 2026-09-10 and adopted at the owner's request
on 2026-09-11. The [Blueprint](../BLUEPRINT.md) carries the normative content
requirements; this guide supplies the writing and review workflow. The four
required tone buckets in [the tone matrix](tone-matrix.md) remain unchanged.
Adopting this guidance does not create, certify or publish the pilot or the
illustrative draft lines below. The 2026-09-12 text-only transition supersedes
the former static-image delivery advice; runtime and packs have not migrated
through this documentation update.

## Visual direction and history

Knowoff's text should feel **lo-fi, everyday and abruptly funny**: short,
recognizable situations, useful but awkward responses, and ordinary items whose
meaning changes with the secret context. The target is plain text only for
Nowns, cards and Weekly Nown Challenge entries, under
[ADR-012](../docs/design/ADR-012-text-only-selectable-modes.md) and the normative
[playable media direction](../BLUEPRINT.md#playable-media-direction).

The earlier visual direction remains historical evidence. `BLUEPRINT.md` at
`33380a6c8fcd493238903245b10da19b9d63cc4d` (2026-08-10, Media Pipeline §3)
specified medium/low-quality compressed WebP with a 720 px ceiling. The
2026-09-10 generation change (`467ab5e1e3ecab646fba120307163fb2bcf586e5`)
retained that direction; [ADR-011](../docs/design/ADR-011-static-image-and-text-content.md)
limited it to static image/text on 2026-09-11. The text decision on 2026-09-12
supersedes those playable-image requirements. Preserve old source/rights records;
do not reinterpret old image filenames, captions or alt-text as approved cards.
Avatars, interface icons and promotional art remain separate assets.

### Briefs that preserve the feel

- Keep prompts roughly 5–10 words and cards roughly 1–4 words as editorial
  starting budgets. Some languages need more; enforce actual Unicode/UTF-8
  limits from the versioned content contract, not a universal word counter.
- For Missed the Briefing, write reusable responses that fit several unrelated
  situations but also have awkward contexts. Exclude universal safe answers
  and wording that uniquely identifies the secret prompt.
- For the four item modes, look for several defensible ratings, meaningful
  remove/add choices, plausible exchanges and arguable comparison pairs.
  A good standalone joke does not prove a whole hand or changing board works.
- Preserve blunt wording and alternative readings. Do not add explanatory
  captions or require typed defenses to rescue an unclear turn action.
- Inspect actual text at card size, with expanded text and script-capable fonts.
  Record exact wording, mode/language suitability, source/rights and revision;
  mark missing screening, playtest and accessibility evidence **not run**.

## Make the table funny

Create cards that give players something to defend. A recognizable situation
and an unexpected response or item choice should start a conversation without
revealing Nown by itself. The interpretation players say aloud matters; a short
item can support a joke through context without containing its own punchline.

For example, a theme about waiting could inspire “Your parcel is enjoying a
gap year.” A theme about meetings could use “This meeting has a sequel.” A
food theme might get “I opened the fridge for emotional support.” These are
draft examples, not certified cards: curators still need to test ambiguity,
mode suitability and full-match hands at both table sizes. These older
longer-line examples illustrate voice; they are not automatically response/item
pool entries under the new contract.

Keep exactly one existing bucket on every asset. Record the following
orthogonal editorial dimensions in planning records, alongside the candidate
and its human review decisions. They are not additional tone buckets, runtime
schema fields or inputs to the relevance/dealing algorithm; the current text
studio does not implement these planning fields.

| Dimension | Starting choices |
|---|---|
| Human situation | Work, family chats, dating, friendship, food, money, travel, bureaucracy, gaming, fandom, technology |
| Comic mechanism | Exaggeration, reversal, literal interpretation, misplaced confidence, petty stakes treated as epic, awkward honesty |
| Cultural reach | Widely recognizable, regional, niche/fandom |
| Shelf life | Evergreen, seasonal, topical |
| Accessibility | Reference explained by the card, specialist knowledge needed, translation difficulty |

Recurring characters and callbacks can give Knowoff its own voice: a parcel
that refuses to arrive, an overconfident group-chat expert, a tiny inconvenience
treated like a world-ending crisis. Rotate these so familiarity does not become
repetition. Interface jokes belong in low-pressure moments; rules, consent,
errors and moderation decisions should stay clear.

## Stay current without becoming disposable

Start with an experimental release mix of **70% evergreen, 20% seasonal or
cultural, and 10% topical**. This is a playtest hypothesis, not a fixed quota;
it is separate from the roughly balanced tone-bucket mix. Change it using
player feedback and reuse data. These percentages describe a release-level
editorial experiment, not card drop chances, hand quotas or dealing weights.

Use trends to discover topics, then write original material. Google Trends
supports regional comparisons and time windows; TikTok Creative Center offers
regional trend discovery. Neither establishes that an event is true or that a
reference works globally. Verify factual premises through the original event,
announcement or reliable reporting before writing around them.
Sources: [Google Trends](https://support.google.com/trends/answer/3076011?hl=en-GB),
[TikTok Creative Center](https://ads.tiktok.com/help/article/creative-center?lang=en).

For each topical candidate record the source, observation date, intended
regions/languages, context in one sentence, review date and expiry date. A human
editor reviews topical material weekly and decides whether to rewrite, retain
or retire it, including at expiry. These are editorial records and decisions;
there is no automated review, expiry or pack-removal job supplied by this
guidance. Ask regional contributors to recreate the joke in their own voice;
literal translation is rarely enough. Recheck ambiguity and reference knowledge
in each target culture/language. Satire can target institutions, powerful
figures and everyday frustrations without turning victims of a current tragedy
into a punchline.

## A small editorial loop

1. Choose a mode/language pilot, three themes and two cultural/language test
   contexts. Keep response/item pools distinct and document intended mode
   suitability; do not split public queues until release/liquidity gates pass.
2. Read the [dealing path](curator-guide.md#read-the-dealing-path-before-authoring),
   then explore verbal mechanisms for a human editor to select, trim or rewrite.
   Inspect short text in actual hand/table layouts, including all five action
   controls and neutral system seeds where relevant.
3. Check sources, rights, originality, age suitability and local meaning.
4. Run automatic screening and human review; neither substitutes for the other.
5. Playtest complete 4- and 6-player schedules in each mode and target language.
   Record recognition, laughter, defensible alternatives and references needing
   explanation; test Donower opening turns, late-seat imitation, changed boards,
   draw pressure and refusal/acceptance history. Prototype matches earn no live
   rewards, progression or leaderboard credit.
6. Prepare an immutable language/mode-scoped bundle, technical/action certificate
   and human pilot record before approved activation. The current synthetic
   builder and directory-copy publisher do not yet implement that full workflow.
   Verify pinned wording in new matches, reconnect and verdict, and preserve the
   previous compatible certified text release for rollback. Revisions/retirement
   become real only when the intended version is activated.

Do not rate quality only by AI scores or raw popularity. A joke that gets a
laugh but makes every card obviously correct can still weaken Knowoff.

## Tools to use together

| Need | Recommendation |
|---|---|
| Research, writing variants, cultural alternatives, editing, test planning | Work with Codex here; retain source links and human decisions in the editorial record. |
| Trend discovery | Google Trends and TikTok Creative Center as inputs; verify each factual premise separately. |
| Accepted text and moderation history | The repository's Contributor Portal and Admin Console. They store the implemented text workflow's approval state; editorial planning fields, mode-aware Nown/pool authoring and production pack activation remain transition work. Acceptance is for curation, not certification or deployment. |
| Optional shared editorial calendar | Airtable if a separate planning board becomes useful. Its [content calendar template](https://www.airtable.com/templates/content-calendar/exp3FNmOkdHZvprXB) supports that workflow; avoid duplicating approval state. |
| Promotional artwork and collaborative review | Canva's existing integration and [content planning tools](https://www.canva.com/solutions/content-planning-scheduling/). Keep promotional artwork separate from playable text bundles. |
| Interface polish | Impeccable for the presentation and usability of the content tools. Humor still needs editorial judgment. |
| Text safety checks | The implemented server moderation adapter, followed by a human reviewer. It does not judge funniness, rights or factual accuracy. |

Text exercises support the editorial pilot; they do not constitute a release.
No additional plugin is required to start editorial planning. External services may
need account access or credentials when we choose to use them. No recurring
monitor, service purchase or content publication is configured by this guidance.
