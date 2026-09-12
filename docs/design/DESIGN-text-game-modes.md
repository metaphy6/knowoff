# Text-based Knowoff: selectable game modes

- **Status:** Text-based product direction and planning defaults adopted in
  Blueprint/Roadmap on 2026-09-12; implementation and playtest validation pending.
- **Author:** Knowoff owner and Codex, from the concept exploration in this task.
- **Date:** 2026-09-11; design answers added 2026-09-12.
- **Supersedes:** The undecided text-versus-image product direction in this
  document. ADR-012 supersedes the playable-image product plan; current runtime remains unchanged until implementation/cutover.

## Product direction

The owner selected five concepts to keep for Knowoff: **Missed the Briefing**,
**Secret Scale**, **Make Room**, **Bad Bargains**, and **Top That**. The intended
direction is to implement all five in the app and let players choose which
Knowoff gameplay mode to play. On 2026-09-12 the owner confirmed the switch to
text-based play. This carries forward the five-mode selection; it does not
choose a single-mode pivot.

The adopted planning default is **Missed the Briefing**, because its single response
play requires the fewest new controls. The target offering uses text Nowns and
text cards throughout; the current image/text association game leaves the
player-facing selector when the text migration is ready. Existing image assets,
loaders and historical records are not deleted by this design change.

This document records gameplay, examples and concrete answers to the design
questions. The owner's text-based direction is confirmed; the detailed defaults
are now the Blueprint implementation-planning contract. This is not a claim of
playtest validation or separate owner sign-off on every technical detail. The examples remain illustrative drafts; player
fun, comprehension and balance have not been playtested.

The current [Blueprint](../../BLUEPRINT.md) remains normative and the
[Roadmap](../planning/ROADMAP.md) remains the sequenced implementation plan.
The adopted rules now live in the Blueprint and their ordered delivery work
and proof tests in the Roadmap; where this explanatory design differs, follow
the Blueprint. The [transition design](DESIGN-text-transition.md) records the
source audit, compatibility, data retention and retirement contracts. This document is not a second
roadmap and does not claim that the current app already supports these modes.

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

The current runtime supports static images and plain text under historical
[ADR-011](ADR-011-static-image-and-text-content.md). The target in
[ADR-012](ADR-012-text-only-selectable-modes.md) uses text-only interactions. Their design
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
starts with one server-seeded face-up offer. On your turn, offer a card from your hand
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

- Start new players on **Missed the Briefing**; remember a returning player's
  last available choice. Show each mode's one-line action before joining.
- Keep Quick Play and Local Rooms as the ways to assemble a table. Quick Play
  uses one explicit mode preference, with FIFO matching among compatible mode,
  table-size and content-language choices. No multi-mode preference in v1.
- After `liquidity.queue_timeout_s`, show that the selected queue is still
  waiting and offer **Keep waiting**, **Change mode** or **Leave queue**. A mode
  change leaves the old queue and joins the new one; never silently substitute
  another mode, table size, language or bot. Text-mode bot backfill starts off.
- In a Local Room, the host selects an available mode, size and content
  language. Everyone sees the same settings and explicitly marks Ready. Any
  setting or membership change clears readiness; start requires a full table
  of connected, ready players. If the host leaves, the longest-present
  connected member becomes host.
- A rematch returns to a settings/Ready lobby. Local Rooms retain their host;
  a Quick Play rematch uses the lowest original seat among returning players
  as host. Leaving the rematch is always available. The same readiness and
  full-table checks apply; new roles are assigned only at the next start.
- Lock mode, content pack/language and rule version for the whole match.
  Reconnect restores that contract; a new app default cannot change it.
- Reuse identity, people/chat, ballots, verdicts and applicable economy
  surfaces. Show only the current mode's legal actions and public table state.

All five remain in the intended offering. Expose a mode only after its release
checks pass; keep unfinished modes out of matchmaking rather than suggesting
that selecting them starts a playable match.

## Design questions adopted for planning

These concrete defaults have been adopted in the Blueprint for implementation
planning. They replace the former open-question list; playtesting must validate
them before release.

| Area | Proposed answer |
|---|---|
| Shared match rules | Retain one turn per active seat, fresh Nown each round, existing Knowoff/victory/disconnect rules, and 5-card hand plus 3-card reserve; use the action completion rules below. |
| Missed the Briefing | Consume one response card; use reusable, situation-tested response pools without universal safe answers. |
| Secret Scale | Confirm card and 1–5 rating together; allow edits before confirmation, then keep the placement immutable. |
| Make Room | Seed three neutral items; replace one with a hand card, discard the removed copy, and retain the complete swap history. |
| Bad Bargains | Seed one public offer per seat; an accepted trade moves the requested card into the proposer’s hand and the offered card onto the recipient’s display. A refusal returns the offered card to the proposer’s hand. |
| Top That | Seed one neutral comparison card; any owned card is a legal next play. Players dispute meaning through discussion and voting. |
| Evidence and secrecy | Keep ordered public action history for the whole match; reconnect receives only public evidence plus the recipient’s authorized private state. |
| Dealing and content | Retain 5+3 as the prototype budget, without automatic refill; certify viable actions across complete schedules and each mode’s changing public state. |
| Mode choice and queues | Default to Missed the Briefing; host settings plus unanimous readiness locally; explicit single-mode Quick Play queues with no silent fallback. |
| Fairness and rollout | No specialties or production bot backfill in the first text release; preserve existing access/reward rules and gate each mode on balance and abuse checks. |

### Shared match rules and card budget

- Keep 4/6-player roles, the 2/3-Knowoff budget, early victory when remaining
  votes cannot catch surviving Donowers, open ballots/runoffs, elimination,
  Ready and match-verdict reveal. A tied runoff consumes a vote without an
  elimination; it does not create a new kind of mode-specific tie.
- Assign a fresh secret text Nown and randomized active-seat order every
  round. Reset that round's board and system seeds; preserve earlier evidence
  under its original round. Hands and reserves carry across rounds.
- Retain `hand.size` and `hand.draw_pile` (currently 5 and 3) for each mode.
  Drawing is optional, current-turn-only and does not end the turn; announce
  player/count publicly and send new card identities only to the owner. Keep
  `points.draw_penalty`. There is no automatic replenishment or discard
  recycling, including between rounds.
- Completing a response, placement, replacement or comparison ends the turn.
  In Bad Bargains, submitting an offer locks the action; its response or
  cancellation finishes the turn before the next seat starts.
- Use the existing tuning keys for turn/discussion/vote/grace clocks. A turn
  with no submitted action expires into an attributed auto-pass and one
  random hand-card discard, without changing the bag, scale or comparison
  target. Record the discarded card as penalty evidence, not an intentional
  play. If there is no card, record the pass without inventing one.
- A disconnected human keeps their seat/role and uses the existing auto-pass,
  abstention, grace, forfeiture and low-population endings. No bot takes over
  that hand. Reconnect never restarts a deadline. Eliminated players retain
  public evidence but cannot act, receive new secret prompts, chat or vote.

### Missed the Briefing: reusable responses

Play one response from the hand, then consume that copy. The public table
keeps the response and its player in turn order. No typed answer is required.

Build response pools by language and mode, shared across situations. Review
each response against multiple unrelated situations: it should support
different defensible readings while still having contexts where it is an
awkward choice. Exclude lines that quote the secret, identify it uniquely, or
work as a safe answer to almost everything. Test whole hands, including a
Donower opening the round; a good standalone joke is insufficient evidence
that a pool is playable. These are editorial acceptance rules, not claims
that a production response pool already exists.

### Secret Scale: confirmation and readable history

The player selects a card and a labeled rating button, previews the choice,
then taps **Place**. Card and rating commit atomically. Both can change before
confirmation; after server acceptance neither can be edited or moved. A late
or invalid submission leaves the current state intact and explains the error.
Duplicate submissions must not spend another card or produce another turn.

Ratings run from low (1) to high (5) along the private criterion. Use neutral
public endpoint labels; do not reveal the criterion to label the scale.
Several cards may share a rating. On a phone, show the five rating groups
with short cards and player attribution, plus a chronological evidence view.
Earlier rounds remain selectable. Dragging is optional; touch, keyboard and
screen-reader controls perform the same confirmed action. Ratings never
produce correctness points.

### Make Room: seed, replacement and restoration

At each round start, the server places three distinct item texts from the
mode/language pool into the bag as separate system-owned card copies. Seed
selection is independent of Nown and roles, uses neither hands nor reserves,
and is labeled **Starting bag**. It must not encode a suggested solution.

Select one hand card and one occupied bag slot, preview the before/after
pair, then confirm one atomic replacement. The incoming copy leaves the
hand; the outgoing copy enters a public discard history. It never enters
another hand or the reserve. The bag always contains three cards. Record
player, slot, removed card and added card even after later replacements.

There is no undo, free retrieval or restoration from history. A later player
may restore the same *idea* by spending a different copy of that item already
in their hand. For example, replacing Car keys with Cake consumes Cake and
discards that Car keys copy; restoring Car keys requires another playable
copy. At a new round, retire the old bag into history and seed a fresh bag.

### Bad Bargains: ownership and a bounded response

At each round start, give every active seat one face-up offer, drawn as a
separate system seed from the shared item pool independently of Nown/roles.
These are additional public copies, not cards taken from the player's 5+3.
Players own their display for that round; they cannot refresh it freely.
Copies need distinct instance identities even when their text is identical.

On the proposer's turn, choose one owned hand card and another connected
active player's displayed card, preview the swap, then submit. Only one
trade can be pending. Reserve the offered hand copy and the requested display
copy; publish both cards and participants. No further draw or action is legal
for the proposer while the offer awaits its response. Only that recipient
may accept or refuse, and responding does not consume their own later turn.

| Resolution | Offered hand card | Requested display card | Turn result |
|---|---|---|---|
| Accepted | Becomes the recipient's new face-up offer | Moves into the proposer's hand | Proposer's turn ends |
| Refused or response expired | Unreserved in the proposer's hand | Stays on the recipient's display | Proposer's turn ends |
| Either participant disconnects/leaves before resolution | Unreserved in the proposer's hand | Stays on the recipient's display | Cancel publicly; proposer's turn ends |

For example, offering Gold watch for Can opener puts Gold watch on the
recipient's display and Can opener in the proposer's hand on acceptance.
The proposer's own display and recipient's private hand are unchanged.
Transferred cards remain known from public history even when now in a hand.
Refused cards also stay public in the offer history; refusal cannot erase
evidence. A refused card may be offered again on a later turn, not this turn.

Propose a new, configurable **10-second response window**, starting when the
server publishes the offer; this is a proposed tuning addition, not an
existing key. The normal turn clock governs submission only. Response expiry
counts as refusal, with no hand discard for either participant. Before an
offer exists, ordinary turn expiry still auto-passes/discards. If no connected
eligible recipient exists, auto-pass without a card penalty and run the
shared disconnect checks; never wait forever or allow a self-trade.

Acceptance must validate the pending offer, participants, exact copies and
board revision, then transfer both copies atomically. The first server-ordered
resolution wins; late/repeated responses cannot transfer twice. End-of-match
or administrative phase transitions cancel an unresolved offer before the
transition; normal discussion/Knowoff waits for resolution. No pending trade
crosses a round or elimination. Retire displays to history and reseed at the
next round; acquired hand cards carry over. There is no automatic refill.

### Top That: a legal play is not an agreed comparison

Seed one system comparison card independently of Nown/roles, outside the
5+3 budget, at each round start. Mark it **Starting card** with no player
attribution. A player previews the current target and a hand card, then
confirms. The hand copy becomes the new target; retain the old target and
the attributed link in the chronological chain. No refill or chain rollback.

Validate turn, ownership and current target revision, not semantic superiority.
An absurd or unconvincing escalation still commits and ends the turn. No veto,
AI judge, automatic rank or mandatory explanation. Discussion and Knowoff
handle disagreement. A timeout leaves the target unchanged; a fresh round
starts a new labeled chain while older chains remain readable.

### Evidence, secrecy and dealing

Keep a server-ordered history with match/round/action identity, seat, public
card copies, before/after board state and resolution reason. Include system
seeds, draws by count, timeout discards, placements, replacements, offers,
acceptances, refusals and cancellations. Reconnect restores the current board,
pending action/deadline and the same complete public history without replaying
actions as new ones. Pin content versions so past cards cannot change wording.

Public history never contains private Nown, the future prompt schedule, reserve
identities, unrevealed hand cards or relevance-band annotations. Send each
hand privately and Nown only to active Nowers. The client must not receive
secrets merely to hide them visually; elimination clears private prompt views.
At match verdict, reveal only Nowns from rounds that actually began.

Use separate response and item pools, with explicit mode/language suitability.
Retain High/Distant/Chaos as candidate relevance evidence where useful; those
labels must not become a required rating, winning bag, trade price or verdict
on an escalation. Deal using the same procedure for both roles. Public seeds
must be chosen independently of the secret, and failed content validation
must reject the pack/match setup rather than fall back to revealing clues.

Certify the full 2/3-round schedule and public-state transitions: useful but
non-obvious responses, several defensible rating placements, meaningful
remove/add choices, plausible exchanges and arguable comparison pairs. Retain
5+3 provisionally because one action per round fits that budget; explicitly
measure draw pressure, especially since accepted trades preserve hand size
while the other modes consume cards. Do not claim prompt cosine similarity
alone balances these actions. The existing synthetic pack and the illustrative
examples are not production content evidence; follow the
[Curator Guide](../../content/curator-guide.md) for review and certification.

### Fairness and release policy

Disable all five specialties for the first text release, for every role and
mode, and show that rule before Ready. This deliberately simplifies the shared
rules: no specialty dealing, Reveal, free draw, Pass card, Shuffle or Revote.
Ordinary draws, timeout passes and tied-ballot runoffs still work. Reintroducing
a specialty later requires its own per-mode interaction and secrecy tests;
particularly, Shuffle cannot safely inherit undefined trade/board rollback.

Keep the existing match-point, Noin, access and no-paid-advantage rules. Modes
have no individual unlock fee; daily Quick Play allowances, earning caps and
leaderboard limits are shared across modes, not reset on switching. Local
Rooms remain uncapped. Add no rewards for ratings, trades, accepted offers,
item collections or allegedly correct responses. Preserve private role-linked
reward settlement and the existing human-count eligibility checks.

Prototype/playtest matches earn no live rewards or leaderboard credit.
Production bot backfill remains disabled until policies are tested for each
mode's legal actions and role-scoped observations; bots never receive extra
secrets, replace a disconnected human or earn rewards. If enabled later, keep
visible bot labels and human-count reward gates. Before enabling rewards for a
mode, check win rates by role and first seat, draws, completion time, repeat
pairings and earnings per minute. Fix material balance or farming problems
before rollout rather than adding a lucrative mode-specific multiplier.

### Implementation handoff and known compatibility gaps

The [current dealing audit](../code/MODULE-media-engine.md),
[match engine](../../server/internal/game/match.go),
[payload renderer](../../server/internal/game/payload.go) and
[tuning](../../configs/gameplay/tuning.yaml) are the implementation starting
points. The current engine's per-round seat-to-card map is insufficient for
whole-match action history. The existing draw path also needs turn-ownership
and private-card delivery fixes; describing desired secrecy is not evidence
that the current handler enforces it.

The selected contract is adopted in Blueprint game rules, content/dealing,
protocol, bots and product baseline. The active Roadmap now orders the transition;
its predecessor is a labeled historical snapshot. Deliberate departures include
text-only play, mode-aware queues, disabled specialties/backfill and the trade
response phase. Timer baselines are reconciled against current tuning (20-second
turns, five seconds of discussion per original table seat). This planning change
does not edit tuning, runtime, content activation or historical proof records.
The [technical transition design](DESIGN-text-transition.md) also covers copy
identity, actual sequence/snapshot gaps, durable settlement, schema migration,
compatibility and complete retirement. [Business planning](../product/BUSINESS_PLAN.md)
covers adoption, liquidity, content labor, value preservation and launch evidence.

Required proofs include atomic action/copy ownership, idempotent confirmation,
timeouts and trade/disconnect races, fresh-round resets with retained history,
role-scoped reconnect/elimination/verdict payloads, correct Knowoff budgets,
mode/locale/readiness isolation, shared reward caps, and mobile/accessibility
behavior. Recheck actual server and client paths during implementation.

## Evaluation of the text-based direction

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
claimed here. The text-based direction is confirmed and the former design
questions have proposed answers. The contract is now adopted in the normative Blueprint and active Roadmap.
Implementation starts only under a subsequent implementation request; changing
the five-mode offering or planning defaults requires a recorded decision
supported by the results above.
