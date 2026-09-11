# Humor development

Editorial guidance proposed on 2026-09-10 and adopted at the owner's request
on 2026-09-11. The [Blueprint](../BLUEPRINT.md) carries the normative content
requirements; this guide supplies the writing and review workflow. The four
required tone buckets in [the tone matrix](tone-matrix.md) remain unchanged.
Adopting this guidance does not create, certify or publish the pilot or the
illustrative draft lines below.

## Visual direction and history

Knowoff's playable media should feel **lo-fi, candid and abruptly funny**.
The original contract in `BLUEPRINT.md` at commit
`33380a6c8fcd493238903245b10da19b9d63cc4d` (2026-08-10, Media Pipeline §3)
specified deliberately medium/low quality: stills ≤720 px longest side,
compressed WebP, because lo-fi is the meme aesthetic. Its Visual Identity
chapter also separated UI and content pipelines.
The 2026-09-10 generation change (`467ab5e1e3ecab646fba120307163fb2bcf586e5`)
retained those targets. The drift was in how briefs and reviews applied them.

The owner's final 2026-09-11 decision limits all Nowns and cards to **static
images and plain text**, while retaining lo-fi texture, recognizable situations
and natural abruptness. [ADR-011](../docs/design/ADR-011-static-image-and-text-content.md)
records this decision and supersedes the earlier motion-led format direction.
The normative
[playable media direction](../BLUEPRINT.md#playable-media-direction) governs:

- Choose a static image when a visual moment carries the joke, or plain text
  when wording carries it. Record the completed image/text mix for Nowns and
  cards separately; concepts, captions and planning prose are not extra assets.
  There is no required format ratio. The choice does not change dealing
  probabilities, tone balance or the freshness experiment below.
- Seek ordinary settings, accidental-looking framing, awkward gestures, rough
  crops and visible compression. A frozen awkward gesture, incongruous object
  or blunt short line can supply the abrupt punchline.
  Preserve recognizability and plausible alternative interpretations.
- Usually work below the still-image ceiling: 360–640 px longest side is a
  useful starting range, with 720 px a maximum rather than a delivery target.
  Deliver compressed static WebP and retain the source aspect ratio. Inspect
  the compressed result at card size; do not upscale, sharpen or beautify by
  default. A small glossy render is still the wrong style.
- Use found or contributed moments with recorded rights/provenance, or brief
  generated candidates for that same rough feel. Do not apply UI pastels,
  doodle grammar, uniform illustration or advertising polish to pack content.
  Do not claim a generated image is an authentic capture.

### Briefs that preserve the feel

| Format | Useful candidate brief | Drift to revise |
|---|---|---|
| Static image | An awkwardly cropped snapshot of an overprepared desk beside one tiny task; modest detail, ordinary light, visible compression. | A pristine editorial illustration or glossy 3D desk, even if exported small. |
| Plain text | A short line whose wording carries the joke, such as “This meeting has a sequel.” | Explaining the whole joke or naming its intended Nown so there is nothing left to defend. |

These are briefs, not produced or approved assets. Record actual source/output
dimensions, static format, bytes, edits and card-size visual review when an
image exists; use **not run** for unperformed checks. Validate a single frame;
GIFs, animated WebP and video are unsupported. Keep concept descriptions
distinct from finished images and playable text cards.

## Make the table funny

Create cards that give players something to defend. A recognizable situation
and an unexpected reaction, action, image or short line should start a
conversation without revealing Nown by itself. The interpretation players say
aloud matters; a static image does not need a written punchline to be funny.

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

1. Choose three themes and two target cultures/languages for an image/text pilot;
   plan the media mix separately for Nowns and playable cards.
2. Automatically read the [dealing path](curator-guide.md#read-the-dealing-path-before-authoring),
   then explore several visual and verbal mechanisms per theme;
   a human editor selects, trims or rewrites. Review compressed media at card
   size for recognizability and an abrupt visual joke.
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
| Accepted text and moderation history | The repository's Contributor Portal and Admin Console. They store the implemented text workflow's approval state; editorial planning fields, Nown/deck authoring and finished image packs remain outside this slice. Acceptance is for curation, not certification or deployment. |
| Optional shared editorial calendar | Airtable if a separate planning board becomes useful. Its [content calendar template](https://www.airtable.com/templates/content-calendar/exp3FNmOkdHZvprXB) supports that workflow; avoid duplicating approval state. |
| Promotional artwork and collaborative review | Canva's existing integration and [content planning tools](https://www.canva.com/solutions/content-planning-scheduling/). Keep game assets in the specified media pipeline. |
| Interface polish | Impeccable for the presentation and usability of the content tools. Humor still needs editorial judgment. |
| Text safety checks | The implemented server moderation adapter, followed by a human reviewer. It does not judge funniness, rights or factual accuracy. |

Image and text exercises both support the editorial pilot; neither is a release.
No additional plugin is required to start editorial planning. External services may
need account access or credentials when we choose to use them. No recurring
monitor, service purchase or content publication is configured by this guidance.
