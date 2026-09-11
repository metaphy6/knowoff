# Potential project divergence: selectable text game modes

- **Status:** Draft — owner-selected concepts for future implementation.
- **Author:** Knowoff owner and Codex, from the concept exploration in this task.
- **Date:** 2026-09-11.
- **Supersedes:** None. This proposal does not change the current game rules.

## Product direction

The owner selected five concepts to keep for Knowoff: **Missed the Briefing**,
**Secret Scale**, **Make Room**, **Bad Bargains**, and **Top That**. The intended
direction is to implement all five in the app and let players choose which
Knowoff gameplay mode to play. A later decision could make one of them the
project's main identity; no single-mode pivot or default has been selected.

This document keeps the five concepts together as a potential project
divergence. It records their gameplay, examples, appeal and outstanding design
questions. It is a concept proposal, not an implemented feature, a certified
content pack or a second implementation roadmap. The examples are illustrative
drafts; player fun, comprehension and balance have not been playtested.

The current [Blueprint](../../BLUEPRINT.md) remains normative and the
[Roadmap](../planning/ROADMAP.md) remains the sequenced implementation plan.
When this direction becomes an implementation scope, put the adopted rules in
the Blueprint and their ordered delivery work and proof tests in the Roadmap.

## Goals and common principles

**One glance to understand, one simple choice to make, something worth arguing
about afterward.**

- Keep Knowoff's visual identity and familiar hand, table, people, discussion,
  voting and reveal surfaces. Individual modes need their own table controls;
  preserving the UI means preserving its design language and familiar layout.
- Use text-only playable content for these five modes. Aim for a secret prompt
  of roughly 5–10 words or fewer and cards of 1–4 words. These are writing
  targets, not validated limits or new runtime settings.
- Use choices from a hand rather than requiring players to write stories,
  invent sentences or perform a timed defense. Explanations happen through
  the normal discussion flow. Keep rules and instructions literal and brief.
- Preserve the proposed shared social structure: 4 players with 1 Donower or
  6 players with 2 Donowers; Nowers know the secret context and Donowers infer
  it from public play while trying to blend in.
- Retain sequential, attributed turns and the discussion/Knowoff loop. The
  existing team victory conditions, elimination and role reveal remain the
  starting point: Nowers identify every Donower within the vote budget;
  Donowers win together if they survive it. The current budget is 2 Knowoffs
  at 4 players and 3 at 6 players.
- Preserve server authority and role secrecy. Hidden context must not reach
  Donower or eliminated-player devices before the allowed match-verdict
  reveal. Mode-specific reconnects, histories and pending actions must obey
  the same boundary.
- Preserve ambiguity. A Nower with an awkward hand can look suspicious; a
  Donower can make a useful choice. Votes decide outcomes. Relevance bands,
  rankings, item values and explanations are not automatic correctness or
  funniness scores.
- Preserve the economy's fairness principles. No purchase changes roles,
  dealing, votes or scoring. Bad Bargains trades fictional item cards, not
  Noin, cash, account inventory or paid entitlements.
- Keep current localization, accessibility and communication principles.
  Short wording must remain understandable in each language. Mode controls
  need accessible labels and touch/keyboard alternatives to dragging.

The current game already supports static images and plain text under
[ADR-011](ADR-011-static-image-and-text-content.md). These concepts explore
text-only interactions; they do not restore animated content. Their design
scope also excludes a new theme, renamed roles or a replacement economy.

## The five modes at a glance

| Mode | What Nowers know | Main turn action | What creates suspicion |
|---|---|---|---|
| Missed the Briefing | A short situation | Play a short response | Whether the response and explanation fit the situation |
| Secret Scale | A ranking criterion | Play an item at a rating from 1 to 5 | What the player values relative to other items |
| Make Room | A shared plan | Replace one item in a three-item bag | What the player sacrifices as well as contributes |
| Bad Bargains | A shared plan | Offer an item-card swap; the recipient accepts or refuses | Strange valuations and willingness to trade |
| Top That | A comparison criterion | Play a card claimed to top the current card | Whether the proposed escalation follows the hidden criterion |

Names in this document are working mode titles. Established game terminology
continues to follow the [Glossary](../project/GLOSSARY.md).

## 1. Missed the Briefing

**Contribute to a conversation you might not understand.**

### What players see and do

Nowers privately receive a short situation. Donowers receive no situation and
must infer it from the conversation. Everyone has short response cards.
On each turn, play one response from your hand onto the table. After everyone
has contributed, discuss the choices and hold Knowoff.

The central panel carries the private situation, the hand contains responses,
and the attributed played cards become the conversation. The main action is
selecting a card; free text belongs in normal discussion.

### Example round

Nowers see:

> A children's magician at a serious board meeting.

A hand might contain:

**Let him finish · Can we expense this? · Pretend it's intentional ·
Reschedule? · Technically team building**

A Nower plays **Pretend it's intentional**. A Donower follows with
**Reschedule?** Another Nower plays **Let him finish**.

During discussion, the Donower explains:

> Clearly, we weren't ready for this meeting.

That is plausible. Did they understand the magician problem, or borrow the
implication from the first player? Later players can learn from earlier
responses, so the bluff develops through the round.

### Why it could be fun

The same tiny response changes meaning with the situation. **Let him finish**
could defend a terrible speech, an elaborate lie or someone eating your lunch.
Players turn a few words into explanations that sound thoughtful, ridiculous
or suspicious.

The shorter variant from the discussion uses **First date. Your ex is the
waiter.** with responses such as **Act normal**, **Extra tip** and
**Fake emergency**. The humor comes from how players interpret those choices.

Responses should work across several situations without fitting everything.
Universal lines such as **Whatever** provide little evidence; exclusive answer
cards reveal too much. The editorial task is to preserve specific implications
and several plausible explanations.

## 2. Secret Scale

**Everyone ranks things, but Donowers do not know the criterion.**

### What players see and do

Nowers privately receive a short ranking criterion. Donowers see the public
scale and placements without its meaning. On your turn, choose an item card
from your hand and place it at a rating from 1 to 5. Multiple cards can share a
rating; this is a scale, not five exclusive slots.

After everyone places a card, discuss the comparisons and hold Knowoff. The
table presents the scale with attributed placements; a tap-based rating
control can complement any drag interaction.

### Example round

Nowers see:

> Hardest to explain to the police

A hand might contain:

**Six passports · Wedding cake · Live chicken · One shoe · Shovel**

The table develops like this:

| Card | Rating |
|---|---:|
| Six passports | 5 |
| Live chicken | 2 |
| Wedding cake | 4 |

The Donower who rated **Wedding cake** at 4 might have guessed that the scale
concerned price. Someone challenges them:

> Why is cake almost as suspicious as six passports?

They reply:

> Depends whose wedding it came from.

That could rescue a mistaken placement or express an unusual but informed
interpretation.

### Why it could be fun

Suspicion comes from comparisons. One placement can seem reasonable; several
may suggest someone is answering an entirely different question. Nowers can
also disagree sincerely, leaving Donowers room to learn and bluff.

Other criteria include **Worst birthday gift**, **Hardest to hide** and
**Most likely to start drama**. There is no objectively correct numerical
rating and no automatic ranking score. The placements are social evidence.

## 3. Make Room

**Pack one shared bag for a plan Donowers do not know.**

### What players see and do

Nowers privately receive a short plan. Everyone sees a shared bag with three
item slots and has a personal hand of item cards. On your turn, replace one
item in the bag with something from your hand.

After everyone makes a swap, discuss the choices and hold Knowoff. The bag
always presents three current items. Previous swaps still need an attributed
evidence history so removing an item does not erase another player's play.

### Example round

Nowers see:

> Escape a wedding

The bag currently contains:

**Car keys · Phone · Fake moustache**

Someone removes **Car keys** and adds **Cake**.

> Why did you get rid of our transport?

They answer:

> We can call a cab. I'm not leaving without cake.

Did they understand the escape plan, or assume everyone was preparing for a
party? The exchange leaves room for a practical explanation and a selfish joke.

### Why it could be fun

Every choice changes the shared plan. You can remove something another player
carefully contributed, argue for restoring a discarded idea, or insist that
an apparently ridiculous item is essential.

Suspicion comes from what someone sacrifices as much as what they contribute.
A limited hand also gives Nowers legitimate reasons to make awkward choices.
Other plans include **Fake being rich**, **Survive the in-laws** and
**Hide a pet**.

The social hook is: **I could forgive what you brought. Explain what you threw
away.** There is no predefined winning bag or automatic usefulness score.

## 4. Bad Bargains

**Make suspicious trades for a plan Donowers do not know.**

### What players see and do

Nowers privately receive a shared plan. Everyone has short item cards and
puts one item face up as an offer. On your turn, offer a card from your hand
for another player's face-up item. That player accepts or refuses.

The public evidence includes the offered exchange, its owner and the response.
After everyone has offered a trade, discuss the decisions and hold Knowoff.
The existing hand and table provide the interaction surfaces; the recipient
needs a clear, short accept/refuse action.

### Example round

Nowers see:

> Stranded on an island

Cards in play could include:

**Gold watch · Can opener · Matches · Perfume · Cash**

Someone offers their **Gold watch** for a **Can opener**.

> You're giving up a gold watch for that?

They reply:

> Enjoy eating your watch.

That apparently terrible bargain could make perfect sense to someone who
knows the plan. Meanwhile, another player desperately wants **Cash**.

> Where exactly are you planning to spend it?

They answer:

> I intend to be rescued by someone negotiable.

Are they thinking ahead, or imagining an entirely different situation?

### Why it could be fun

Players discover what others value and test them with questionable offers.
An impressive item might be useless; a mundane object might be essential.
Refusing a seemingly generous deal can be as revealing as accepting it.

This combines Make Room's practical choices with Secret Scale's disputed
values, adding direct exchanges between players. These are one-for-one item
card trades, not negotiations over typed prices or wagers. Acquiring a more
valuable-looking collection does not itself win the match.

## 5. Top That

**Every player tries to outdo the previous card.**

### What players see and do

Nowers privately receive a comparison criterion. Donowers see the cards but
do not know what makes one choice beat another. On your turn, play one card
from your hand that you claim tops the current card under that criterion.

For a criterion about the worst gift, topping the current card means proposing
an even worse gift. The played card becomes the next comparison target;
disagreement supplies evidence rather than an automatic rejection of the play.
After everyone plays, discuss the chain of choices and hold Knowoff.

### Example round

Nowers see:

> Worst birthday gift

The chain begins:

**Vacuum cleaner → Live chicken → Unpaid bill**

A Donower plays **Gym membership**.

> That's worse than an unpaid bill?

They reply:

> It's a bill that also makes you exercise.

Did they understand the criterion or invent a convincing connection after
seeing the other cards?

### Why it could be fun

Players try to outdo an already ridiculous choice using whatever their hand
provides. A seemingly weak escalation can have an unexpectedly good argument;
a Nower with an awkward hand can also struggle.

Other criteria include **Hardest to hide**, **Biggest party killer** and
**Worst thing to borrow**. Each turn compares two short cards. The table can
feature the latest comparison while keeping earlier attributed plays
available as evidence.

This combines Secret Scale's comparisons with Make Room's shared table.
There is no automatic winner for the funniest card or best escalation:
Knowoff still concerns who appears to lack the hidden context.

## Player selection and the shared app

The intended product is **one Knowoff app with five selectable gameplay
modes**. A mode is a rule set, separate from Quick Play versus Local Rooms,
table size, language, theme packs or specialty cards.

The proposed selection behavior is:

- Choose a gameplay mode before a match and show it clearly to every player.
  Mode descriptions can use the one-line turn actions from the comparison
  table above.
- Keep Quick Play and Local Rooms as the existing ways to assemble a table.
  For Quick Play, match players with compatible mode choices and table sizes;
  the queue design and possible multi-mode preferences need separate design.
- In a Local Room, make the selected mode visible and agreed before the match
  starts. Host controls and any group-selection mechanism remain open.
- Keep the selected mode fixed during a match. A rematch can offer a mode
  change before the next match begins; exact rematch consent and readiness
  behavior still need specification.
- Reuse shared identity, roles, people/chat, ballots, verdicts and economy
  surfaces where their rules remain applicable. Give each mode its own legal
  actions and public table state rather than presenting every mode's controls
  during every match.

Whether the current image/text association game remains an additional option,
becomes the default or is eventually replaced is unresolved. The five selected
concepts are all intended candidates for implementation; this document does
not select a winner or authorize retiring the existing game.

## Design questions to resolve before implementation

These are open decisions, not extra rules players must learn now.

| Area | Decisions needed |
|---|---|
| Shared match rules | Confirm mode-specific turn completion, timeouts, draws, disconnects, elimination and specialty behavior against the existing match loop. Specify any deliberate departure from current rules in the Blueprint. |
| Missed the Briefing | Build reusable response pools with enough implication to provide evidence without making the situation obvious from one card. |
| Secret Scale | Specify placement confirmation and correction, compact display of shared ratings, and how earlier placements remain accessible. |
| Make Room | Specify how the first three items are seeded, where removed cards go, hand replenishment, and which recovery/restoration moves are legal. |
| Bad Bargains | Specify face-up offer seeding, ownership and card destinations after acceptance, refused-offer handling, response timeouts and pending trades when a player leaves. |
| Top That | Specify the first comparison card, replenishment and history; preserve the distinction between a legal card play and a disputed semantic escalation. |
| Evidence and secrecy | Keep an attributed history of placements, replacements, offers, refusals and comparisons through reconnects and later Knowoffs without leaking private prompts or hands. |
| Dealing and content | Reassess the current five-card hand plus three-card draw pile and High/Distant/Chaos model for each action type. Do not assume similarity to a prompt alone balances a trade, replacement or comparison. |
| Mode choice and queues | Decide the default, Local Room selection, Quick Play queue organization, optional mode preferences and clear behavior when a chosen mode has too few players. |
| Fairness and rollout | Assess whether current specialties, bots, scoring, rewards and access policies remain suitable across modes. Preserve no-paid-advantage principles and avoid selecting a mode merely to farm easier rewards. |

All five use shared-pool content with multiple defensible relationships.
Illustrative examples here are not measured High/Distant/Chaos assignments or
evidence that the existing dealer supports these rule sets. The
[Curator Guide](../../content/curator-guide.md) documents the current relevance
and review requirements; implementation planning must inspect the actual
server/client paths before claiming compatibility.

## Evaluation and possible project divergence

Prototype and playtest the modes with actual 4- and 6-player groups. Evaluate
the same questions for each mode:

- Can players understand the secret prompt and their turn without a long
  explanation or a large amount of reading?
- Do choices produce laughter, disagreement and several believable defenses?
- Can Nowers with awkward hands remain plausible, and can Donowers learn and
  bluff without surviving solely through generic choices?
- Does the mode retain enough useful evidence for a vote without exposing the
  secret too quickly or hiding earlier moves?
- Do mode-specific actions work on small screens, with assistive technology
  and in each target language?
- Can players choose and change modes between matches without confusion or
  unacceptable matchmaking delays?

No playtest, balance result, implemented mode selector or release readiness is
claimed here. The next planning step is to translate the chosen delivery scope
into the normative Blueprint and the existing Roadmap, with meaningful tests
for each new action and its secrecy boundary.

The owner can later use those results to retain all five as player-selectable
modes, emphasize a subset or make one mode the central Knowoff experience.
That decision remains open; the current intent is to build the five-mode
offering and let players choose.
