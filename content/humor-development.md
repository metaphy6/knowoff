# Humor development

Editorial guidance proposed on 2026-09-10 and adopted at the owner's request
on 2026-09-11. The [Blueprint](../BLUEPRINT.md) carries the normative content
requirements; this guide supplies the writing and review workflow. The four
required tone buckets in [the tone matrix](tone-matrix.md) remain unchanged.
Adopting this guidance does not create, certify or publish the pilot or the
illustrative draft lines below.

## Make the table funny

Write cards that give players something to defend. A recognizable situation,
an unexpected interpretation and a short, speakable line make better game
material than a topical reference that only one person understands. The joke
should start a conversation without revealing the Nown by itself.

For example, a theme about waiting could inspire “Your parcel is enjoying a
gap year.” A theme about meetings could use “This meeting has a sequel.” A
food theme might get “I opened the fridge for emotional support.” These are
draft examples, not certified cards: curators still need to test ambiguity,
relevance bands and hands at both table sizes.

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

1. Choose three themes and two target cultures/languages for a pilot.
2. Generate several mechanisms per theme; a human editor selects and rewrites.
3. Check sources, rights, originality, age suitability and local meaning.
4. Run automatic screening and human review; neither substitutes for the other.
5. Playtest real 4- and 6-player hands. Record recognition, laughter, plausible
   alternative explanations and references players needed explained. Check
   whether the humor reveals Nown too directly or leaves Donowers no plausible
   bluff; a successful joke must preserve the game's ambiguity.
6. Build and certify a small language-scoped pack through the media pipeline,
   publish and activate its verified version, then inspect feedback and revise
   or retire weak cards. Text approval alone does not complete these steps.

Do not rate quality only by AI scores or raw popularity. A joke that gets a
laugh but makes every card obviously correct can still weaken Knowoff.

## Tools to use together

| Need | Recommendation |
|---|---|
| Research, writing variants, cultural alternatives, editing, test planning | Work with Codex here; retain source links and human decisions in the editorial record. |
| Trend discovery | Google Trends and TikTok Creative Center as inputs; verify each factual premise separately. |
| Accepted text and moderation history | The repository's Contributor Portal and Admin Console. They store the implemented text workflow's approval state; editorial planning fields, Nown/deck authoring and finished image/GIF packs remain outside this slice. Acceptance is for curation, not certification or deployment. |
| Optional shared editorial calendar | Airtable if a separate planning board becomes useful. Its [content calendar template](https://www.airtable.com/templates/content-calendar/exp3FNmOkdHZvprXB) supports that workflow; avoid duplicating approval state. |
| Promotional artwork and collaborative review | Canva's existing integration and [content planning tools](https://www.canva.com/solutions/content-planning-scheduling/). Keep game assets in the specified media pipeline. |
| Interface polish | Impeccable for the presentation and usability of the content tools. Humor still needs editorial judgment. |
| Text safety checks | The implemented server moderation adapter, followed by a human reviewer. It does not judge funniness, rights or factual accuracy. |

No additional plugin is required to start the text pilot. External services may
need account access or credentials when we choose to use them. No recurring
monitor, service purchase or content publication is configured by this guidance.
