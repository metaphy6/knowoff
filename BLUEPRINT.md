# 📖 Knowoff — Project Blueprint

> **The implementation plan** — phases, ordered checkboxes, proof tests,
> and gates — lives in [`docs/planning/ROADMAP.md`](docs/planning/ROADMAP.md).
> This file is the normative product + technical spec: game rules, tech
> stack, architecture, media engine, economy, admin console, contributor
> portal, and the product baseline. When a roadmap phase's **Spec
> (required reading)** line names a chapter, it names a chapter in *this*
> file.
>
> This file owns *what the product is*; `ROADMAP.md` owns *the sequence to
> build it*. Keeping them as two files (rather than one monolith) is
> deliberate — but they must never drift: a feature change to a chapter
> here lands in the same commit as any roadmap-chapter update it implies,
> per the lockstep rule in `ROADMAP.md`'s Appendix A and `AGENTS.md` §1.
> Need a fast, high-level orientation first? → [`README.md`](README.md).

> **Text transition adopted for planning — 2026-09-12.** The owner selected
> text-only Knowoff with five selectable gameplay modes. This document now
> specifies the target; the checked-in runtime still implements the older
> association game. No mode, migration, content release or cleanup is claimed
> implemented by this documentation change. [ADR-012](docs/design/ADR-012-text-only-selectable-modes.md)
> records the decision; the [transition design](docs/design/DESIGN-text-transition.md)
> maps source gaps, data preservation and retirement proofs. Previous roadmap
> checkmarks remain historical evidence only.

> **Implementation update — 2026-09-12.** Phase 1 is verified complete with
> executable v2 contracts, typed closed availability, read-only inventory,
> reviewed transaction identities and isolated migration/backfill design proofs.
> The active wire protocol remains v1; none of the five text mode engines or
> deployed data transitions is complete. Current proof and remaining gates
> are tracked in the Roadmap.

## 🎲 The Game at a Glance

Knowoff is an online social deduction party game for **exactly 4 or 6 players**.
Active Nowers privately see a short text **Nown**; Donowers infer its hidden
context from public choices and try to blend in. Players choose one of five
modes: **Missed the Briefing**, **Secret Scale**, **Make Room**, **Bad Bargains**,
or **Top That**. Each has one simple card action; all share discussion and
Knowoff. Nowers win by eliminating every Donower before the vote budget runs
out; Donowers win together if they survive. Quick Play assembles compatible
strangers; Local Rooms assemble invited players. These are table-entry paths,
not gameplay modes. Missed the Briefing is the initial default; each mode is
exposed only after its own engineering, content and playtest gates pass.

There is deliberately no separate rulebook: the **Game Rules** section below is the single source of truth, and all player-facing help text, store copy, and the how-to-play clip are derived from it.

### Terminology (normative — written without "the")

| Term | Meaning |
|---|---|
| **Nower(s)** | Players who see Nown |
| **Donower(s)** | Players who can't see Nown; nobody knows who they are |
| **Nown** | The private text situation, plan or criterion for a round |
| **Knowoff** | The vote at the end of every round |
| **Round** | Mode-specific turns + discussion + one Knowoff |
| **Match** | Up to 2 votings at 4 players, up to 3 at 6 — until a team wins |
| **Session** | Matches played in one room; keeps a running scoreboard |
| **Noin** | The game currency — earned by playing, sold in bulks (💰) |

---

## 🧱 Tech Stack

| Layer | Technology | Target responsibility |
|---|---|---|
| Client | Flutter Android/iOS + Web PWA | Accessible mode controls, public history, private owner state and localized interface |
| Backend | Go | Authoritative lobby/match/actions, secrecy, votes, rewards, Portal/Admin |
| Durable data | PostgreSQL | Accounts, ledger, entitlements, results, audit, contributions and immutable content-release metadata |
| Coordination | Redis | Presence/routing/rate limits; mode/size/content-language queues where durable coordination is implemented |
| Playable content | Versioned server-local text bundles | Validated, immutable prompt/response/item catalogs; no public prompt catalog or signed playable-image delivery |
| Non-playable assets | Existing application assets and PostgreSQL avatar blobs | Fonts, icons, store art and moderated avatars remain; text-only does not mean image-free UI |
| Transport | WebSocket JSON | Versioned intents and per-recipient events/snapshots |
| Infrastructure | Docker Compose; VPS for public launch | PostgreSQL/Redis retained; MinIO/CDN game-content wiring retires after dependency and retention proofs |

Retain server authority (ADR-001), Flutter (ADR-002) and the public shared Go
content-library boundary in `server/pkg/media` (ADR-004). The historical package
name may remain: a name is not dead code; unused image/specialty behavior is.
The current Compose stack still includes MinIO. Retire its mandatory readiness,
credentials, routes and volume only after the transition inventory proves no
retained consumer needs them. Keep authorized historical backups separately.
Hosting cost and availability require current operator quotes and measurements;
old zero-cost, VPS-price and CDN-free-tier examples are not business evidence.

---

## 🕹️ Game Rules

### 1. Rooms, Votes & Victory

* A room holds **exactly 4 players (1 Donower) or exactly 6 players (2 Donowers)** — no other sizes. A match may run short-handed only because someone disconnected or left (§7).
* A match is a series of rounds; **every round ends with a Knowoff vote**. The vote budget is fixed: **2 votings at 4 players, 3 votings at 6 players**.
* What a vote does: **the player with the most votes is eliminated**, and their role is revealed. An eliminated player takes no further part in the match — no cards, no votes, no chat, no pokes — but they should **stay in the lobby to the end to collect their points in full**: elimination itself carries no penalty; disconnecting or leaving does (§7). Eliminating a Donower is a catch; eliminating a Nower is a wasted vote.
* **Nowers win** the moment every Donower is caught.
* **Donowers win** the moment the remaining votes can't catch the remaining Donowers. Concretely:
  * 4 players: Donower survives both votings → Donowers win.
  * 6 players: **if neither of the first two votings catches a Donower, the match ends right after the second Knowoff — one vote can't catch two Donowers — and Donowers win.** Otherwise it runs to the third voting, and any Donower still uncaught after it wins.
* **Donowers win and lose together**: a caught Donower still wins if their partner survives. Donowers don't know each other at match start; quietly working out who your partner is, and covering for them, is intended strategy.
* No specialty can reset a ballot in the first text release (§5).
* Match duration is a playtest metric by mode and size, not a verified 5/8-minute promise. Acting early and Ready (§8) can shorten phase ceilings.

### 2. Roles, Nown & Hands

* Roles are assigned randomly and privately at match start; the press-and-hold
  check is identical for all seats. Roles never move between seats.
* Every round starts a fresh text Nown, independently randomized active-seat
  order and fresh mode board. Hands/reserves carry across rounds; earlier
  public evidence remains accessible under its original round.
* Send Nown only to active Nowers. Donowers receive a neutral placeholder;
  eliminated players receive no new prompt. Never send the future schedule,
  prompt lookup catalog or relevance annotations to any player device.
* Start each player with `hand.size` cards and `hand.draw_pile` reserve cards
  (currently 5+3), dealt identically for both roles. Use separate response and
  item pools with explicit mode/language suitability. Budget and viability
  across complete schedules and changing boards must be certified (⚙️).
* Give every physical copy a match-unique instance identity separate from its
  content ID. Equal wording may exist on different copies; ownership, transfer,
  reservation and discard operate on instances. System seeds are extra copies
  outside 5+3, chosen independently of Nown and roles.
* No automatic refill, discard recycling or per-round new hand. Bad Bargains
  accepted trades preserve hand size; refusal preserves the offered card.
  Other mode actions consume one hand copy. Depletion still has a defined pass
  path; certification cannot be replaced by claiming eight cards always suffice.

### 3. The Round: Turn-Based Play

The normal turn deadline is `timers.play_turn` (currently 20 seconds). Only the
current connected active seat can draw or submit its mode action. A valid
confirmed action is immutable, public and attributed; it ends the turn, except
that an offer enters Bad Bargains' bounded response phase. The next turn starts
only after resolution/cancellation. Invalid or stale actions change nothing.

| Mode (stable ID) | Private Nown / pool | Atomic action and public board |
|---|---|---|
| Missed the Briefing (`missed_the_briefing`) | Situation / reusable responses | Spend one hand response; append the attributed response to ordered evidence. |
| Secret Scale (`secret_scale`) | Criterion / items | Confirm one hand copy and integer rating 1–5 together; spend the copy and append placement. Several cards may share a rating; neutral public endpoints never name the secret criterion. |
| Make Room (`make_room`) | Plan / items | Start with three distinct-text system copies; confirm hand copy + occupied slot. Incoming leaves hand, removed copy enters public discard history. Bag stays size three. Restoring an idea requires another owned copy. |
| Bad Bargains (`bad_bargains`) | Plan / items | Each active seat gets one public system display. Propose an owned hand copy for another connected active seat's display. Reserve both copies until that recipient accepts/refuses or the server cancels/expires. |
| Top That (`top_that`) | Criterion / items | Start with one neutral system target. Confirm owned hand copy against current target revision; consume it into the chain as new target, retaining every prior attributed link. |

No semantic correctness check, AI judge, automatic rank, veto or reward decides
whether a response fits, a rating is right, a trade is profitable or a card tops
the last. Players discuss and vote. Previews may change before confirmation;
accepted actions cannot be undone. Touch, keyboard and screen-reader controls
must provide the same confirmed action without requiring drag or free typing.

**Draws.** Optional, current-turn-only; draw from the remaining personal reserve,
without ending the turn. Charge `points.draw_penalty` per card (currently 5).
Publish only player and count; deliver new instances privately to the owner.
No draw while an offer is pending; no free-draw specialty exception.

**Timeout.** No action before deadline produces attributed auto-pass and one
random hand-copy discard. Show that copy as penalty evidence, not intentional
play; no card means pass with no invented copy. The mode board stays unchanged.
Disconnected human turns use the same evidence/penalty path immediately.

**Bad Bargains resolution.** Only one offer can be pending. Its server-owned
10-second response window is a planned `timers.trade_response_s` addition;
normal turn time limits submission only. No further action/draw by the proposer.
The recipient's response does not consume their later scheduled turn.

| Resolution | Offered hand copy | Requested display copy |
|---|---|---|
| Accept | Becomes recipient's display | Moves to proposer's hand |
| Refuse / response deadline | Returns unreserved to proposer's hand | Stays displayed |
| Either participant leaves/disconnects; forced phase/match close | Unreserve to proposer; public cancellation | Stays displayed |

All resolutions end the proposer's turn. Proposer's own display and recipient's
private hand stay unchanged. Publicly exposed cards stay known in history even
when transferred or returned to a hand. Validate offer ID, participants, both
instances and board revision atomically; first server-ordered resolution wins.
If no connected eligible recipient exists before proposing, auto-pass without
card loss and evaluate disconnect endings. Ordinary discussion/voting waits;
forced transitions cancel first. No pending offer crosses a round/elimination.
At each new round retire displays/bag/chain into history, reseed, and keep hands.

### 4. Discussion & Knowoff (every round)

* After the round's final turn, a discussion window opens (`timers.discussion_per_player × table size`, currently 5 s × 4/6; ends early when everyone is Ready).
  * **Local rooms:** talk happens out loud at the table.
  * **Online rooms:** players argue through **Quick Chat** — canned phrases and reactions plus moderated free text. Every typed message carries the player's selected client language; the server masks configured English words for every message and additionally masks the configured list for that language before broadcasting it. The client never performs the authoritative moderation decision.
* Then **Knowoff**: a 20-second open ballot. Everyone still in the match votes for one player (never themselves); every cast lands live and attributed for the whole table to see, and a voter may change their target as many times as they like right up until the ballot resolves. The most-voted player is eliminated and their role revealed (§1). A tie triggers one 15-second **runoff** among the tied players only; if the runoff is still tied, the vote is a miss: it counts as one survived voting for the Donowers, eliminates nobody, and reveals no role.
* **Result window:** every vote's outcome is displayed for 8 seconds before it becomes final — a 4-second falling reveal followed by 4 seconds on the role-result poster. No specialty can undo an exposed result (§5). Ends early once every connected active player marks Ready. Then it applies.
* An eliminated player — Nower or Donower — watches the rest of the match: no plays, no votes, no chat, no pokes. Their screen no longer shows Nown (a revealed Donower could otherwise feed it to a surviving partner). Staying connected to the end collects their points as normal (§6, §7).
* After the match, the verdict screen shows only Nowns from rounds that actually began to everyone; Donowers finally see what they survived.

### 5. Card Specialties

All five old specialties — Pass, Reveal, One More Free Card, Shuffle and Revote —
are absent from the first text release for every role and mode. Show this rule
before Ready. Remove their dealing, handlers, wire variants, client controls,
debug grant paths, localization, sole-use assets/config and obsolete tests only
with replacement proof and the retirement process. Ordinary draws, timeout
passes and tied-ballot runoffs remain. Reintroduction is a new design decision
with per-mode interaction/secrecy proofs, not dormant enabled-by-config code.

### 6. Match Points & Noin Earnings

Two separate rewards come out of every match:

**Match points** — the competitive score. They feed the session scoreboard (local rooms) and the Weekly Leaderboard (Quick Play only, 🎮 §5), and every match's net also accumulates on the profile twice: into **Overall Points** (the lifetime total — it only ever grows) and into **Non-Converted Points**, a balance convertible to Noin (💰 §1: 100 points → 1 Noin, one-way). A match's net floors at 0 — draw penalties can empty a match's gains, never dig debt. A player who is absent when the match ends scores 0 points for it. Eliminated players are not absent: staying in the lobby to the end collects everything.

| Event | Points |
|---|---|
| Your vote names a Donower | +10 |
| Nower team win | +10 each Nower |
| Donower team win | +30 each Donower — caught Donowers included (they win together) |
| Each card drawn from your pile | −5 (`points.draw_penalty`) |

**Noin** — the currency (💰 §1). Noin grants are **per-event and credited to the profile instantly**, so they survive disconnects and abandons: what you earned is yours the moment you earned it. Values in `tuning.yaml → noin:`.

| Event | Noin (v1 placeholders) |
|---|---|
| Match completed | +5 |
| Nower team win | +30 each Nower |
| Donower team win | +50 each Donower |
| Your vote names a Donower | +5 |
| Surviving a voting as a Donower | +10 — credited discreetly (below) |
| First win of the day | +25 |

**Discreet crediting:** no public surface — scoreboard, lobby, or profile — ever shows per-match Noin amounts or live balance changes, and role-linked grants like Donower vote survival appear only in their owner's private match-end settlement. The ledger credit is still instant (it survives disconnects); it just isn't visible, so numbers can never out a Donower mid-match.

No chips, no stakes, no in-match economy — Noin and points are earned by the match, never wagered in it.

### 7. Disconnects, Abandons & Fairness

Nobody should profit from a dropped connection — theirs or anyone else's. Three rules cover every case:

1. **Grace & auto-play (20 s).** A disconnected seat is auto-played from the moment it drops: its turns pass instantly, it abstains from votes, counts as Ready, and can still be voted. The player has 20 seconds to reconnect before counting as absent; reconnecting later restores the seat mid-match (with a fresh role-scoped snapshot — a Nower gets Nown back, a Donower doesn't).
2. **A fully absent team forfeits.** If **every uncaught Donower** is absent past grace, the match ends immediately as a **Nower win**. If every un-eliminated Nower is absent past grace, Donowers win. Quitting is conceding, never escaping. These checks count only players still in the match — eliminated players sit outside them.
3. **Too few humans ends the match after grace, scored.** If fewer than 3 players remain connected, the match waits until every disconnected seat's grace period has expired (and rule 2 hasn't already decided it), then ends without a team result. Every player receives the match points they accrued, including disconnected players; no team-win Noin is granted.

Backstops: except for the scored low-population ending above, an absent-at-end player — eliminated or not — scores 0 match points (already-earned Noin stays — §6); roles never move between seats; no bot ever takes over a human's hand. Repeated abandoning in Quick Play earns escalating matchmaking cooldowns.

### 8. Pace Controls: Ready & Poke

* **Ready** — for every player, in every room type (4 or 6, local or online): a play turn ends when its mode action resolves (a submitted Bad Bargains offer waits for its response/cancellation), and in discussion, marking Ready (or having nothing left to do) counts you in — when everyone is Ready the discussion window ends early. The 8 s result window works the same way: Ready counts you in, and once every connected active player has, it finalizes early instead of waiting out the timer. Fast tables play fast; the configured turn, discussion, and result timers are only ceilings. **The Knowoff ballot and any runoff work the same way, and only the same way** — casting a vote does not by itself count you in; the ballot always runs its full window unless every active connected seat also marks Ready, which is what actually leaves a voter room to change their mind before it closes (see ADR-009).
* **Poke**: once per target per phase, you may poke a player — the turn player sitting on the clock during play, anyone not yet Ready during discussion, or anyone during voting. Their phone buzzes (native apps) and their screen shakes (everywhere — the web PWA has no vibration). Pokes show who poked whom. The three phases are independent: you can poke the same player once in play, once in discussion, and once in voting. No score effect; the cap is enforced server-side. During a pending trade, the recipient is the only Poke target; this shares the play-phase per-target budget, creates no extra Poke allowance, and cannot extend the response deadline.

---

## 🎮 Game Modes & Live Ops

### 1. Online Quick Play (the main product)

* One explicit mode preference per queue join. Match FIFO within the compatible
  tuple **mode + size + content language + rules/protocol compatibility** and
  pack eligibility. No multi-mode preference or silent substitution in v1.
* Default new players to Missed the Briefing; remember a returning player's last
  available mode. If it is withdrawn, explain before presenting the default.
* At `liquidity.queue_timeout_s` (currently 25 s), show Keep waiting, Change mode,
  Leave queue. Keep waiting retains FIFO position. Change atomically leaves the
  old queue before joining the new one at its tail. Never charge a second
  allowance or reserve two seats during retry/change/disconnect races.
* Text production backfill is **off**, even though current tuning enables it.
  Require full human tables. Release modes/languages in measured cohorts so
  selection does not fragment queues beyond viable fill times. All five remain
  the intended offering; unreleased modes never enter matchmaking.
* Free accounts share `economy.free_daily_quickplay_matches` across every mode
  (currently 3/server day); active Play Pass/Premium removes this cap. Reserve
  eligibility at join, consume once on actual match start, release on canceled
  queue/start. Existing implementation needs reconciliation before this proof.
  Noin/leaderboard daily caps and human eligibility also remain shared.

### 2. Local Rooms and rematches

Host shares the existing QR/six-character code. Host selects an available mode,
4/6 size, eligible pack and content language; all members see settings and mark
Ready. Start requires a full table of connected ready players; settings or
membership changes clear readiness. Host departure elects longest-present
connected member (seat order breaks a timestamp tie), then clears readiness.
Local Rooms remain uncapped and use spoken discussion if co-located.

A rematch returns to settings/Ready, never automatically restarts or backfills.
Local host persists when present; Quick Play rematch host is lowest original
seat among returners. Replacements enter as unready and see the full contract;
leaving is always possible. Quick Play rematches remain Quick Play: shared allowance/reward/leaderboard rules
and free core/featured packs apply. Returning host may choose only eligible
Quick Play settings. Replacement humans join through the matching FIFO tuple
and must Ready; no paid private pack, hidden bot or free-allowance bypass.
A Local Room rematch remains Local. Reducing size below current membership
is rejected until players explicitly leave; no automatic seat eviction. A
setting change cancels outstanding replacement reservations before the new
queue tuple is published.

A started match pins mode, rules, language, content
version and reward eligibility. No lobby change or app-default update mutates
it; new roles are assigned at the next start only.

### 3. Weekly Nown Challenge (Community Event)

A separate text contribution/community event, not a sixth gameplay mode or a source of match correctness:

* **The topic is a Nown:** every Monday the server publishes the week's topic — a short public text prompt. Players respond the way they play cards in a match: submit one short text entry inspired by the topic.
* **Open to all players, in-app** — no portal role needed. One entry per player, **immutable once submitted** — no edits, no replacements. **The system accepts the first 100 entries**, then intake auto-closes; a slot reopens each time screening rejects an earlier entry.
* **Screening before visibility:** every entry passes the automated screen plus a human check (curators or admin) before it becomes publicly visible and votable. Rejected entries never appear.
* **Voting:** open to all players — one vote each, never for your own entry, **immutable once cast**. Tallies are public and live.
* **Week Winner:** at the weekly close, the most-voted entry wins. Tied vote totals are resolved by earliest accepted submission; an identical acceptance timestamp uses the immutable entry ID for deterministic ordering (owner-adopted 2026-09-12). Its owner holds the **Week Winner title — shown on their profile and in lobbies until the next winner is crowned** — and receives a large Noin award (`noin.challenge_winner`). Winning and standout entries may also enter the community pack, with credits.
* **Consent:** submitting requires explicit acceptance of the contribution terms — the entry may be used in the system, commercially, and in modified form (perpetual, non-exclusive license). Terms version + timestamp are stored with the entry. No consent, no upload.

### 4. System Notices & Announcements

Players are never surprised by downtime:

* **Notice types:** scheduled **maintenance** (start time + expected duration), **downtime/incident updates**, and **general announcements** — events, pack releases, rule changes.
* **Delivery:** connected clients receive `system_notice` events in real time over the WebSocket; on app start the client fetches all active notices over HTTPS. Notices render as dismissible banners on the main menu plus a persistent notice inbox — never a blocking gate, except a hard-maintenance countdown.
* **Maintenance flow:** a scheduled window announces itself at T−24 h, T−1 h, and T−10 min; when it opens, matchmaking stops admitting new matches and running matches drain to their natural end before the server goes down. Nobody loses a live match to planned work.
* **Ops:** notices are composed, scheduled, localized, and withdrawn in the Admin Console (🛡️), audited like every admin action; scheduling a maintenance window arms the matchmaking drain automatically.

### 5. Weekly Leaderboard

* Cycle: Monday–Sunday on the server clock; Monday announces last week's podium alongside the new Week Winner.
* Score: the sum of a player's match points for the week — **Quick Play matches only** (private and local rooms are collusion-trivial and stay off the board), with the retained ≥ `liquidity.leaderboard_min_humans` human eligibility guard (text backfill is off), and a daily cap on counted matches to blunt pure grind.
* Display: top 100 plus the viewer's own rank; ties share a rank. Every weekly close snapshots into an immutable history table; podium finishes show on profiles.
* Ops: Admin Console — standings inspection, cheat exclusion/reinstatement with reasons, manual re-run of the weekly close, history browsing.
* Infrastructure: pure PostgreSQL aggregation over the audit stream; no new stores.

---

## 👤 Profiles & Community

### 1. Public Player Statistics

* **Every profile is visible to anyone** — tap a name in a lobby, scoreboard, or the leaderboard: **Overall Points** (lifetime match points — never decreasing, untouched by conversions), matches played and won per role, vote accuracy, Donower survival rate, Week Winner titles, weekly podium finishes, pokes sent, contributor credits. **Non-Converted Points** — the convertible balance (💰 §1) — shows only to the profile's owner.
* Derived nightly from the server's audit-event stream — no client-reported numbers anywhere. Reputations are part of the metagame: a profile that survives most of its Donower matches *should* scare the table.
* Privacy: profiles are pseudonymous (nickname + avatar, no PII); delete-my-data erases stats with the account.

### 2. Avatars (Free Presets, Paid Uploads)

* A curated preset gallery, free forever, on-brand by construction.
* A one-time **Custom Avatar** unlock (Noin, 💰 §5) opens personal uploads: server-side crop to 256×256 WebP, EXIF strip, size cap, automated moderation screen before display, reportable forever, admin takedown reverts to presets without refund. Stored as small blobs in PostgreSQL.

### 3. Player Reports

* One tap from any profile or scoreboard: inappropriate avatar/nickname, harassment, cheating/collusion — plus **media reports** on any Nown or card, which route to the curation queue instead of the conduct queue.
* Reports feed the moderation queues worked by **Guards and admins** (🧑‍🎨 §2): a Guard can freeze a heavily-flagged account pending review; only an admin bans. Rate-limited; repeat reports collapse into one case.

### 4. Feedback & Ideas

* In-app form (bug / idea / other) with an auto-attached context snapshot (app version, pack tag, last match id) shown to the user before sending. Rate-limited endpoint into a Postgres table, triaged in the Admin Console.

---

## 🧑‍🎨 Contributor Portal & Media Workbench

**The portal is a web-only interface** (no in-app editing surface) where community members work on the game's content and community safety — under roles they apply for and an admin grants. One web application, three audiences: applicants, role-holders, and admins.

### 1. Roles & Permissions

Any player may **apply for a role** from their portal profile; applications are reviewed and granted by an admin, and every role action is audited and reversible.

| Role | Permissions |
|---|---|
| **Contributor** (entry role) | Submit media to open pack calls; view own submission history and Noin rewards. (The Weekly Nown Challenge needs no role — it is open to every player, in-app, 🎮 §3) |
| **Curator** | Everything a Contributor can, plus: **create Nowns and candidate card pools that relate to them** — authoring cards against multiple Nowns, testing complete match schedules in the deal simulator, checking band coverage and human playtest results; screen Weekly Challenge entries before they go public. Cards belong to the pack's shared pool, not an exclusive deck for one Nown. Curators work from the [Curator Guide](content/curator-guide.md), derived from ⚙️ §2–3: quality targets, tone rubric, editorial records, band coverage, and how to test a Nown before certification |
| **Guard** | Community safety: review flagged accounts and **freeze** them — a timeboxed suspension (up to 48 h) from matchmaking and the portal, pending admin review. **Final action is always the admin's**: dismiss, timed ban, or permanent ban. One active freeze per Guard per target; freezes auto-expire if no admin acts |
| **Admin** | Everything: role grants, bans, pack publishing, challenge scheduling, takedowns |

* **Rewards:** contributors and curators earn **credits (name in the pack manifest and on the profile) and Noin** for accepted work — `noin.contributor_accepted_asset` once per approved contribution; the Weekly Nown Challenge pays its Week Winner from the same rails (🎮 §3). Attribution is stored per asset, so richer reward schemes later are an economy change, not a migration.
* **Submission terms:** every upload requires explicit acceptance of the contribution terms — perpetual, non-exclusive license, **commercial use and modification permitted** — with the accepted terms version and timestamp stored per submission (🎮 §3).

### 2. Submission Pipeline

* Flow: write plain text → bounded UTF-8 validation and normalization, exact/near-duplicate review, mode/language labeling and automated text screen → **submit** → screening/curation → accepted media enters the next pack version.
* Workflow states: `draft → submitted → in_review → approved | rejected → published(pack-tag)` — every transition audited. **Submissions are immutable once submitted** — no edits; withdraw and resubmit is the only correction path, and it costs the queue slot.
* Moderation: everything passes the automated screen *and* human review before any player sees it; published media stays reportable (👤 §3) and takedown-able, with removals shipping in the next pack version.
* **Acceptance and release are separate:** approval accepts a contribution for curation. Playable release requires a versioned pack, technical certification, editorial playtests and activation through the media pipeline. Editorial source/expiry records supplement the approval history; they do not replace it. Current text-only capabilities and remaining integration work are described in [Community operations](docs/guides/COMMUNITY_OPERATIONS.md).

### 3. Media Workbench (dev/staging — ships early, Phase 2)

The same application pointed at dev/staging, where the owner curates the AI generation pipeline before the community exists:

* **Batch drafting:** human authors and optional AI produce short text candidates. Persist exact input/output, model/version when used, language, rights and review history; no generation result is automatically accepted. External generation is not assumed deterministic or implemented.
* **Bulk curation grid:** keep/kill at keyboard speed with tone-bucket and rating assignment; keep-rate measured per batch (the pipeline's core KPI — treat the historical 10–30% expectation as an unvalidated staffing assumption).
* **Suitability review:** compare reusable responses/items across unrelated prompts and whole schedules. Optional versioned text embeddings assist retrieval, never certify humor or action viability alone.
* **Deal simulator:** for any candidate Nown, render the hands the mesh would actually deal at both table sizes — the same tool Curators later use, per the Curator Guide.
* **Editorial review:** use the dimensions and pilot process in ⚙️ §3 alongside the tone bucket. Retain source context, cultural adaptation, human decisions and real-hand playtest observations in the editorial record; keep-rate and automated scores alone do not establish comic quality or fair ambiguity.

### 4. Architecture

Server-rendered from the Go binary (same pattern as the Admin Console) on a public subdomain — no separate SPA build chain. Role, text, consent and workflow state remain in PostgreSQL; immutable text bundles deploy to servers. The automated moderation screen is provider-swappable. Publishing and approval have distinct durable states and idempotency keys; publishing never repeats the approval reward.

---

## 🛡️ Admin Console

One authenticated, browser-based operations board, served from the Go server on the internal admin port — never through the public ingress. Admin accounts in PostgreSQL with role-based access, 2FA, rate-limited sessions, CSRF protection, and an append-only audit trail (who, what, when, before/after). Every action is an audited server mutation through product code paths.

* **Reports & moderation:** the case queue — conduct cases (kick, ban account+device, close room, avatar takedown, Guard-freeze reviews) and media cases (asset takedown → next pack version, tag fixes). Bans hit live connections immediately.
* **Portal administration:** role applications (grant/revoke Contributor, Curator, Guard), the challenge scheduler and its screening queue, contribution-terms versioning.
* **Curation & packs:** pack dashboard — active tag per pack, checksum verification, hot-swap trigger, certification stats (⚙️ §3).
* **Leaderboard ops** (🎮 §5): exclusions, reinstatements, weekly close re-runs, history.
* **Economy & accounts:** Noin ledger and entitlement lookups; grants/refunds are explicit audited actions.
* **System notices** (🎮 §4): composer and scheduler for maintenance windows, downtime updates, and announcements; scheduling a maintenance window arms the matchmaking drain automatically.
* **Feedback triage:** new / seen / done.
* Roadmap fit: endpoints ship with the features they manage; the console UI is a Phase 5 deliverable — in place before any public launch.

---

## 🎬 How-to-Play Clip (Ship-Gated)

A ≤45-second, watch-don't-read onboarding clip: a first-timer should follow their first match after one viewing.

* Format: real UI capture only; silent-autoplay friendly — big captions carry the story; one idea per beat; readable at phone size.
* Storyboard (7 beats, 4–6 s each):
  1. Hook — "One of you can't see this." A short secret situation is missing on one phone among four.
  2. Roles — the press-and-hold role check; one card whispers *you're Donower*.
  3. Nown appears on every screen at once; a Donower's screen shows only a plain placeholder — and nobody can tell.
  4. Play — cards hit the table one turn at a time, each revealed instantly with a name; caption "whose card doesn't get it?"
  5. Pressure — a draw announcement, a poke shake, Quick Chat accusations flying.
  6. Knowoff — votes land; the loser is out, role face-up… a Nower. The table groans; one vote left.
  7. Verdict — the Donower grins; "Donowers win together." Public team verdict and points; private reward settlement stays private. Logo out.
* Ship gate: produced on final production UI, released with prod — no clip work while gameplay, engine, and netcode remain open.

---

## 🎨 Visual Identity: Soft Neo-Brutalism Design Matrix

Direction locked: **pastel neo-brutalism, illustration-light**. A brutalist skeleton — thick ink borders, hard zero-blur shadows, chunky type, flat fills — wearing a soft candy palette; personality comes from tiles, type, and color, not mascots or scene art.

This matrix styles the interface around short text Nowns and cards. Keep the
[lo-fi text direction](#playable-media-direction): everyday, concise and
speakable. Fonts, icons, avatars, illustration policy and accessible finite
interface motion remain; playable-image loaders do not.

> **Implementing or redesigning any client UI against this chapter?** Load
> [`.agents/skills/neo-brutalism-ui-design/SKILL.md`](.agents/skills/neo-brutalism-ui-design/SKILL.md)
> first — it's the execution playbook for this matrix: neo-brutalism
> background/research, a per-component redesign guide mapped to the actual
> `client/lib/presentation/` files, the token/shadow/typography upgrade path,
> and the accessibility/guardrail checks a redesign must keep passing.

* Palette tokens (Flutter constants; light theme only at v1):
  * `canvas` `#DCC8F7` — lavender field with a faint low-contrast grid tile; `surface` `#F7F2E9` warm cream for cards and sheets; `#FFFFFF` content wells inside them.
  * `ink` `#141414` — every border and every glyph; text is never gray-on-gray.
  * `violet` `#B49AF5` — the neutral interactive: buttons, selected tiles, timers, progress fills.
  * `lime` `#D4F04C` — the truth/reward signal: Nower catches, match points, Noin grants.
  * `pink` `#FF9ED2` — the risk/accusation signal: votes, the Knowoff board, Donower reveals. The palette's single permitted gradient (`#FFD9EC → #FF9ED2`) is reserved for the Knowoff reveal header.
  * **Support accents** (ADR-008) — categorical only, never a verdict signal: `canvasDeep` `#C4A8F0` for header bands and rails, `tangerine` `#FFB020` for currency, streaks and heat, `aqua` `#7FE7DC` for time, connection and neutral information, and historically `sky` `#8FCBF5` for Shuffle (ADR-010; retire sole-use wiring after consumer audit). Eleven colours total is the ceiling; a further hue is another ADR.
* Structure: every container carries `Border.all(width: 3, color: ink)` and a hard shadow `BoxShadow(color: ink, offset: Offset(4, 4), blurRadius: 0)`. Corners rounded — radius 16 for cards and sheets, 12 for buttons, full pill for stat chips. Pressing a control collapses its shadow to zero offset while the control translates onto its own shadow footprint: the signature brutalist click. Shadows come from a **named tier scale** — `sm` (3,3) for chips and badges, `md` (4,4) for cards and buttons, `lg` (8,8) for overlays and hero moments, `lift` (7,7) for pointer hover — plus tinted `limeGlow`/`pinkGlow`/`violetGlow` reserved for celebration. Blur is always `0`.
* **Structured disruption:** decorative and celebratory surfaces (the hand fan, the evidence table, empty-state stickers, verdict banners) sit at a small rotation drawn from a closed set (`KoTilt`: ~1.1°, ~2°, ~2.9°). Ballots, timers, forms, and anything a fairness rule depends on stay mechanically aligned — never rotated.
* Typography: **Baloo 2** (SIL OFL, bundled under `client/assets/fonts/` — ADR-007) is the locked display face for headings, timers, room codes, tallies, and Noin numbers; body copy stays on a plain geometric sans. **Highlighter emphasis is the house style:** the revealed role, a Noin delta, the clip caption — key phrases sit on a lime marker sweep, not bold-only.
* Illustration policy — deliberately sparse: no mascot, no scene art in the match flow. One tiny single-weight doodle glyph set (13 glyphs: sparkle, static-burst, eye, cloud, placeholder, crown, coin, cards, check, cross, poke, mask, clock) reserved for empty states, win moments, state badges, and the Donower-side placeholder.
* Fixed color semantics: violet = interact, lime = truth/reward, pink = accuse/risk, ink = information. No verdict leans on hue alone — color always pairs with icon + label (colorblind-safe by construction).
* Performance guardrails: flat fills (the one gradient exception above), zero blur radii, no stacked translucency — low-end devices are the norm.
* **Voice:** short, speakable jokes and rotating callbacks can appear in low-pressure moments. Rules, consent, errors and moderation decisions stay clear and literal; a joke must never obscure a required action or a fairness rule. Recurring editorial characters belong to pack content, within the illustration policy above.
* Asset strategy — code first, raster last: UI chrome is 100% widgets/`CustomPainter`s (borders, hard shadows, grid tile, highlighter sweep, press animation); doodles ship as hand-authored SVG paths. True raster — preset avatars, app icon, store art — is produced offline in curated batches via **GPT-6 Astra**, prompts derived from this matrix; candidates → human curation → consistency pass → committed like any asset. API keys live under the config discipline (📦 §3). In-game *media content* comes exclusively from media packs (⚙️) — the design system and the content pipeline never mix.

---

## 🏛️ Codebase Taxonomy & Separation of Concerns

Boundary map for the transition; comments distinguish target from current paths.
Actual source inventory in the transition design takes precedence over schematic
filenames here (some former blueprint paths were aspirational).

| Path / boundary | Responsibility and disposition |
|---|---|
| `client/lib/core/` | Config, transport, logging and shared services retained; v2 network handling added. |
| `client/lib/data/` | API/auth and DTOs retained/adapted to copy IDs, contract and sequence snapshots. |
| `client/lib/presentation/` | Shared theme/screens/state retained; five mode controls added, specialty/image gameplay views retired. |
| `client/lib/media/` | Existing catalog/cache/prefetch path audited for removal; do not download secret prompt catalogs. |
| `server/cmd/knowoffd/` | Wiring, health, graceful drain and config, with obsolete storage/bot entry points removed. |
| `server/internal/{game,lobby,handler,transport}/` | One authoritative shared engine, compatible queues/readiness, role-scoped v2 protocol. |
| `server/pkg/media/` | Shared versioned text catalog/dealing/certification library (ADR-004); no client dealer. |
| `server/internal/{economy,profiles,leaderboard,store}/` | Durable value, stable match/admission/settlement identity and shared caps. |
| `server/internal/{portal,admin,workbench}/` | Text contribution/review/activation and secure operations; retire obsolete image-only workbench code. |
| `server/internal/{avatar,reports,notices}/` | Retained non-playable imagery, conduct/content cases and truthful drain notices. |
| `server/migrations/` | Applied history preserved; additive migration/backfill before contract cleanup. |
| `tools/mediapack/`, `tools/gamebot/` | Existing command surfaces adapted to text releases and v2 scripted test matches. |
| `infra/`, `nginx/`, `configs/` | Current stack adapted with verified backups and consumer-specific dependency retirement. |
| `content/` | Text editorial inputs and versioned packs; synthetic fixtures distinct from production. |

Separation rule: `server/internal/game`, `server/pkg/media`, and `server/internal/economy` contain **all** rules, dealing, secrecy, and currency logic — the client renders state and sends intents; it never decides an outcome, never computes a balance, and never receives data its role shouldn't see.

---

## ⚙️ Media Engine Specification

The historical “Media Engine” name denotes the shared content/dealing library.
The text target has no playable image format, image prefetch, signed prompt URL
or public secret catalog. Server-deployed bundles contain plain text; clients
receive only authorized instances through role-scoped events.

### 1. Text bundle and release contract

Retain immutable manifest + `media.jsonl` (Nowns) + `cards.jsonl` conventions
unless a versioned migration requires a rename. The planned new format declares
schema version, release ID, mode suitability, canonical BCP 47 language, rules
compatibility, per-file hashes, license/attribution, age policy and certification
artifact hashes. Nowns distinguish situation/plan/criterion; cards distinguish
response/item pool. These are target fields, not current loader capabilities.
Reject unknown types, blank/oversized text, invalid Unicode, missing coverage,
duplicate IDs or incompatible versions before activation. Never reinterpret an
old image alt-text/filename as approved playable text.

Use a stable content ID and immutable revision for wording/provenance, plus a
separate match-unique copy ID for every dealt/system card. Pin the complete
validated bundle and rules for each match. A new release affects only new
matches. Reconnect and evidence use the pinned bytes; activation cannot reword
history. Archived records may preserve old types, with explicit legacy status;
new active content must be text. Build/publish/activate/takedown remain separate
reviewed steps with audit and rollback to a compatible certified text release.

### 2. Dealing and action viability

Choose the secret full 2/3-round schedule before dealing, role-blind and without
repeating a Nown merely to fill an undersized pack. Reject infeasible setup
before consuming access. Retain 5+3 provisionally, but certify the **actual
retained cards** across every scheduled prompt, both sizes, and reachable mode
board/trade states. No empty candidate fallback that reveals the prompt.

High/Distant/Chaos remain useful candidate evidence, not a correctness oracle.
When used, preserve multiple defensible relations per scheduled prompt and
version the evaluator/thresholds. Mandatory multimodal embeddings and synthetic
fixture geometry are retired as production requirements. Text embeddings may
assist editorial search; explicit reviewed suitability plus legal-action and
whole-hand simulation form release evidence. Role-blind dealing must not encode
which seat knows the secret. Public seeds are independent of prompt/role,
drawn from the mode/language item pool outside player 5+3.

Response pools must work across unrelated situations, with awkward contexts as
well as plausible ones; exclude universal safe answers and unique prompt clues.
Item pools need defensible ratings, bag substitutions, exchanges and chains.
Record opening-seat disadvantage, late-seat imitation, exhaustion, draw pressure
and public-card knowledge after trades. Monte Carlo alone cannot prove every
reachable state; combine exhaustive small fixtures/property tests with sampled
production schedules and human playtests. No pack may claim all-schedule proof
from testing only first-Nown band counts.

### 3. Text pipeline and content production

Current `tools/mediapack` has `build`, `certify`, `simulate`, `publish`;
`build` makes synthetic fixtures and `publish` copies directories. They do not
yet implement the production workflow below. New commands must be documented
only once actually implemented; the Roadmap orders the work.

Target workflow: draft → normalize/validate → automated text screen → human
editorial/rights/locale review → versioned bundle → technical/action certification
→ human full-match pilot → approved activation. Capture exact accepted text,
source/terms version, editor and consent provenance. Approving a submission is
not deploying a pack. A repeated publish cannot pay approval rewards twice.

#### Playable media direction

Plain text only for Nowns, cards and Weekly Nown Challenge content. Target short
prompts (roughly 5–10 words) and cards (roughly 1–4 words); these are editorial
budgets, not byte limits. Some languages need different lengths. Use a validated
configurable UTF-8 bound, grapheme-aware UI, plain escaped rendering, no HTML,
Markdown execution, external asset URL or bidi/control-character spoofing.
Retain lo-fi everyday humor, multiple readings and speakable defenses. Preserve
four tone buckets and the distinction between chaos humor and Chaos relevance.
Keep topical source/observation/region/language/review/expiry records, human
weekly review, and cultural rewriting. The 70/20/10 freshness mix is an experiment,
not runtime weights. Respect the existing no-explicit-content policy, provenance,
rights and automated-plus-human screening. Avatars/store art are separate assets.

Start with a bounded mode/language pilot; all five intended modes need their own
4/6-player proof. At least three themes and two cultural/language pilots test
transferability, without promising both languages at public launch. The old
150-Nown/1,500-card aggregate goal and current 150/5,400 synthetic fixture are
not release capacity proofs; publish sufficient certified content based on
full-schedule viability and repeat-exposure measurements. Record recognition,
laughter, defensible alternatives, comprehension and reference explanations.

### 4. Secrecy, history and anti-cheat

Render every outbound event/snapshot per recipient at a single audited boundary.
Active Nower receives current private Nown text; Donower/eliminated receives a
neutral placeholder. Public history contains match/round/action IDs, monotonic
sequence, seat, public card instances, before/after state and reason: system
seeds, draws by count, penalties, actions and offer outcomes. It never includes
future prompts, hidden hand/reserve identities or relevance annotations.

Reconnect restores the same board, owner hand, pending offer/deadline and complete
public history without re-executing actions or extending clocks. A previously
informed human cannot unlearn a prompt; elimination clears app state/semantics
and blocks future delivery. Verdict reveals only begun-round Nowns. Keep
secret schedules, RNG seeds and privileged replay scripts out of general logs,
client catalogs, analytics dashboards and public pack endpoints. Privileged
diagnostics require explicit authorization, retention and audit.

#### Designer Workbench & Tuning

`configs/gameplay/tuning.yaml` is the only runtime numeric source. This planning
change does not edit it. Resolve its comments and add typed validation in Phase 1:

| Concern | Current value / key | Text target |
|---|---|---|
| Turn | `timers.play_turn: 20` | Retain; measured by mode |
| Discussion | `timers.discussion_per_player: 5` | Target × original configured table size, Ready shortcut; current code uses active-connected count and must change explicitly |
| Ballot/runoff/result | 20 / 15 / 8 seconds | Retain; unanimous active-connected Ready may end early |
| Grace | `game.reconnect_grace_s: 20` | Retain; reconnect never resets it |
| Inter-round countdown | `timers.prefetch_countdown: 5` | Replace with neutral round-start countdown in versioned config; no asset prefetch dependency |
| Hand/reserve | `hand.size: 5`, `hand.draw_pile: 3` | Provisional, certify before release |
| Trade response | Absent | Planned `timers.trade_response_s: 10`, positive bounded validation |
| Specialties | Nonzero weights + timers | Remove all first-release dealing/use/debug/config paths |
| Backfill | `liquidity.backfill_enabled: true` | Off for text; remove production scheduler after cutover |
| Queue timeout | `liquidity.queue_timeout_s: 25` | Offer explicit waiting/change/leave; no silent expiry/substitution |
| Draw penalty | `points.draw_penalty: 5` | Every ordinary drawn card, no free-card exemption |
| Economy/progression | Existing `points/noin/economy/liveops/progression` | Preserve values and account-wide caps; measure, do not invent mode multipliers |

Economy hypotheses remain: active free player affords a one-day pass in roughly
2 play-days; seven-day pass in 8–10; ≥70% circulation earned; conversion ≤25% of
income; roughly one draw per player/match. These are unvalidated targets, not
claims from the synthetic simulator. [Business plan](docs/product/BUSINESS_PLAN.md)
defines measurement, costs, cohort and rollout decisions.

---

## 🌐 Network Architecture: Server-Authoritative WebSocket

Client intents go to one authoritative Go match owner; role-scoped events go
back to each seat. PostgreSQL stores durable results/ledger/audit; Redis handles
coordination where implemented. Live match state remains in one process.

**Planned protocol revision 2** is deliberately incompatible with legacy v1.
Negotiate before binding a seat or charging access. V1 clients receive an
explicit upgrade-required response after cutover; no silent conversion of old
`play_card`/specialty intents to new mode actions. Deployment drains v1 matches
before switching, rather than promising mixed-version live matches.

The match contract contains `match_id`, `mode_id`, `rules_version`,
`content_language`, `pack_release_id`, `protocol_version`, original table size
and server-owned reward eligibility. Queue/lobby settings carry a revision;
Ready acknowledges that revision. Intents carry action/request ID, expected
match/round/phase and board revision plus exact owned instance/target fields.
Proposed action discriminators are `respond`, `place`, `replace`, `offer`,
`resolve_offer`, `top`; their schema is frozen with fixtures in Phase 1 before
handlers are written. Common draw/vote/chat/Ready/Poke behavior remains.

Idempotency is per match/seat/request: the same ID and body returns the original
result without another mutation; conflicting reuse is rejected. Validate and
mutate under match serialization; timer/leave/accept races have one winner.
Sequence gaps trigger a role-scoped snapshot; requests to resync do not replay
mutations. Stale boards/actions/deadlines fail with stable localized error codes.
Limit frames, request frequency, action-history size and slow consumers; preserve
complete bounded match evidence without exposing a private raw event log.

System UI copy uses stable IDs/codes and parameters. Authorized authored card/
Nown text, moderated chat, localized operator notices and consented user content
are explicitly permitted display data; “no display text” must not prohibit
text gameplay. Content language is pinned separately from interface locale.

There is no durable live-match recovery claim. On process loss, fail cleanly to
menu; terminate pending offers and mark interrupted match. Already durable
Noin grants survive; no fabricated completion, result or replacement hand.
For a confirmed server/process interruption, persist `interrupted` once and
release/refund any consumed free-Quick-Play allowance once using a compensating
admission record. Do not extend a paid pass/Premium expiry automatically. Grant
no completion/team/first-win reward, points, XP or leaderboard result from lost
in-memory state; already committed grants/accruals remain and reconcile by
idempotent event key. A client disconnect alone cannot trigger this policy.
No interruption compensation is payable twice on worker restart/retry.
Use durable idempotent outbox/settlement records so partial persistence retries
cannot double-pay points, XP, leaderboard or currency. Planned drain closes
admission and waits for actual active rooms; multi-node routing is separate
future work, not implemented merely by retaining Redis.

---

## 🤖 Bots: Development and Testing

Production text backfill is disabled and retired from active wiring. No bot
replaces a disconnected human. Future production bots require a separate
approved per-mode rollout, visible labels, role-scoped observations and existing
human reward thresholds; do not retain untested dormant production code.

Adapt `tools/gamebot` to protocol v2 for reproducible dev/test scripts in all
five modes, including recipient replies and race schedules. Policies consume
only what that role receives over the real WebSocket; no global pack/schedule
or other hand as a hidden shortcut. Reproducibility means saved seed plus ordered
intents/clock inputs and pinned rules/content, not seed alone. Test matches
never earn live rewards or leaderboard credit. Production must reject dev
identities/overrides through authenticated environment policy, not a client flag.

---

## 📦 Infrastructure & Deployment

### 1. Containerization Principles

Keep Go/Flutter build boundaries, Docker Compose, PostgreSQL and Redis. Current
images and tooling still depend on the older content path; Phase 6 removes
playable-image-only native libraries, storage clients and readiness checks after
consumer proof. The avatar WebP encoder remains a legitimate native consumer;
text-only gameplay does not by itself remove CGO/compiler needs. Build each
supported artifact in a verified toolchain; do not claim static/distroless or
multi-arch success without build/runtime evidence.

### 2. Hosting, transition and rollback

Home-hosted development/beta can remain behind Cloudflare; public launch needs
an operator-selected reliable host and measured costs. Use the corrected
[VPS migration runbook](docs/launch/VPS_MIGRATION_RUNBOOK.md) and transition
schema/retirement design. Rehearse old-schema restore, forward migration and
new-schema restore separately. Match state is ephemeral; Redis backup does not
recover in-memory rooms. Quiesce admission, drain matches and settlement work,
then capture coherent durable data and verify restored contents/ledger sums.

Do not delete legacy data, object volumes or old binaries during expand/backfill.
Before contract cleanup, record retained data/rights, compatibility floor,
rollback window and last restorable snapshot. After target writes begin, DNS
rollback alone loses data: stop writes, reconcile/replay durable changes or
restore the agreed recovery point with an explicit loss decision. Never overwrite
new purchases or earned balances using an old snapshot as routine rollback.

### 3. Centralized Configuration

Retain layered `configs/base.yaml` + local/staging/prod overlays and typed,
fail-fast validation. Environment variables select the config and inject secrets.
Text-mode availability, rules and content-language eligibility are server-owned.
Reject unknown keys; old specialty/image keys receive an explicit migration
error at the version boundary, then leave examples/overlays/tests too.

### 4. Local Development

Current `make up` remains the existing full Compose stack until implementation.
Target fresh-clone proof starts PostgreSQL/Redis/server/client with a certified
synthetic text fixture and no cloud key or playable object store. Run real
4/6-seat scripted mode matches over WebSocket, native/PWA smoke tests and
reconnect checks. The unified runner plus explicit standalone-module and real-DB
checks must execute without silent skips. Do not install host OS packages as
part of this documentation task.

### 5. Readiness and future scaling

Readiness checks required dependencies only; liveness remains healthy during a
recoverable dependency outage. SIGTERM marks unready and drains before closing.
Use initial engineering acceptance budgets (test assumptions to verify, not
current measurements): client p95 frame total ≤16.7 ms at 60 Hz on the recorded
low-end device, server p95 accepted intent-to-event ≤200 ms/p99 ≤500 ms in a
100-concurrent-room 4/6-seat test with network latency reported separately;
zero corrupted matches or leaks. After a 100-match soak and completed cleanup/GC,
retained heap stays within 10% of a warmed baseline and room/socket/goroutine
counts return to baseline. Worst-case history stays within configured frame
bounds or the tested paging contract. Freeze host/network/build inputs before
measuring; report desktop-only evidence as such. Also measure per-mode queue fill. Redis/room affinity is a future multi-node contract
until proven in code. Kubernetes/Terraform remain deferred; do not expand this
transition into orchestration work.

---

## 💰 Monetization: The Noin Economy

One currency sits at the center of the business: **Noin**. Players earn it by playing well, buy it in bulks when they want more, and spend it on play passes, packs, and cosmetics. Design goals, in order: keep free players playing daily, make earned progress feel meaningful (a free player must be able to earn every Noin-priced item; Premium remains a cash subscription), and monetize impatience, identity, and commitment — never gameplay advantage. **No pay-to-win: nothing purchasable affects dealing, roles, votes, or scoring.**

### 1. Earning Noin

* Play rewards (Rules §6): completing matches, winning as either team, correct votes, surviving votes as a Donower (credited discreetly — Rules §6), first win of the day — credited instantly and kept even on disconnect. A daily earn cap (`noin.daily_earn_cap`) blunts farming, and team-win Noin requires ≥ `liquidity.noin_min_humans` humans in the match.
* Contribution rewards (🧑‍🎨 §1): Noin per accepted asset, and the Week Winner award of the Weekly Nown Challenge (🎮 §3).
* **Point conversion:** every match's net points land on the profile as **Overall Points** (lifetime, never decreases) and **Non-Converted Points** (a balance). The owner may convert Non-Converted Points to Noin at **100 points → 1 Noin** (`economy.points_to_noin`), in multiples of 100. Conversion is **one-way and irreversible**: converted points are subtracted from the Non-Converted balance forever, and Overall Points never change. Converted Noin counts toward `noin.daily_earn_cap`, so points can never bypass the anti-farm ceiling.
* Balance target: an active free player earns a 1-day Play Pass every ~2 days of play (protocol table in ⚙️ Tuning) — this is an unvalidated target; measure it against actual shared allowances, earnings and sinks before promising affordability.

### 2. Play Passes (Noin) & the Premium Subscription

* **Play Passes are unlimited Quick Play**, sold as **1-day (250), 3-day (600), and 7-day (1,200) passes priced in Noin** — plain in-game finance, an earnable convenience and nothing more, so the play-more path always runs through the one currency. **Play Passes do not remove ads.**
* **Premium** is the one cash subscription — **monthly or yearly, with the yearly plan 20% off**, sold through platform billing — and it is the **only way to remove ads entirely**. It also includes unlimited Quick Play while active, so a subscriber never needs passes.
* Free accounts get `economy.free_daily_quickplay_matches` per server day (**3 at launch** — a deliberate taster: regular free play runs on earned Play Passes, ~2 play-days per 1-day pass, or on Premium; **the cap never touches a Play Pass holder or Premium subscriber**). **Local Rooms are never capped** — play with the people in your living room is always free and unlimited.

### 3. Noin Bulks (the cash lane)

* Bulk packs via platform billing (Play Billing / StoreKit): sizes in `economy.noin_bundles`, store price tiers mapped at launch. Noin bundles and Premium are the two cash lanes. Every Noin-priced item is earnable; Premium's ad removal is a subscription benefit.
* Rewarded ads (SSV — the ad network's servers call our verification endpoint; the client callback grants nothing): an optional post-match ad **doubles that match's Noin**; Premium subscribers get the doubling automatically, ad-free. **Ad surfaces disappear only under the Premium subscription (💰 §2)** — Noin Play Passes never remove ads.

### 4. Theme Packs

* Curated text content packs (humor verticals, seasonal, community highlights, age-gated adult-humor packs) priced in Noin (`economy.unlock_prices.theme_pack`).
* In private and local rooms, the **Host Pass** rule applies: only the current host sponsoring the next match needs the pack; the whole table plays it, guests never pay. Host transfer preserves any already-started match contract; before the next start revalidate the new host's entitlement. If unavailable, clear Ready and require an explicit eligible pack choice, never silently substitute or charge guests. Quick Play runs the core pack plus a free rotating featured pack.

### 5. Cosmetics & Identity

* **Poke Styles** (visual + haptic effect sets) and the **Custom Avatar** unlock (👤 §2), priced in Noin. More identity items (card backs, reveal animations) ride the same entitlement rails later.
* Technical spine for all of it: a PostgreSQL wallet with an append-only ledger (earns, purchases, spends, refunds); debits atomic with entitlement writes; balances server-side only; all prices in `tuning.yaml` so economy tuning never needs a client release.

---

## 🧾 Product Baseline (v1 Decisions)

* Modes & discovery: five selectable gameplay modes, initially defaulting to Missed the Briefing; explicit mode/size/content-language Quick Play and settings/Ready Local Rooms via code/QR. Each mode is release-gated. No public browser or skill rating.
* **Communication: canned Quick Chat, reactions, and moderated free text.** Voice and video remain out of scope. Typed text rides the existing WebSocket relay with the active client locale; server-side moderation applies English by default and the configured matching language list before any broadcast. Word lists live in `configs/base.yaml → moderation.word_lists`, are data rather than code, and can add a language without a client release. Harassment reports and account enforcement remain the path for conduct that masking cannot address. Local rooms talk out loud anyway.
* **Localization and content language:** the architecture supports multiple languages from day one; public exposure may start with one certified language. Add others only after editorial, interface and queue gates; the two-culture pilot is not a public-language promise. No user-facing string is hardcoded anywhere: the client renders all text through Flutter localization catalogs (ARB/`intl` — ICU plurals, locale-aware numbers and dates), layouts tolerate text expansion, and the display faces carry a script-capable font fallback (the diacritics render check runs across launch locales — 🎨). **System UI messages use IDs/codes; authored text content, moderated chat and localized notices are display-data exceptions:** system messages cross the wire as a stable id/code plus parameters — Quick Chat phrase ids, rejection/error codes, event fields — localized client-side (🌐). System notices are authored per locale with an English fallback chain (🎮 §4, 🛡️); text content packs declare a BCP 47 language tag in the manifest (⚙️ §1) so content ships per language without a format change; the supported-locale list is config (`configs/base.yaml → localization:`), so adding a language is new translation catalogs plus localized store assets — never a code change. A pseudo-locale CI gate keeps hardcoded strings out from Phase 1 onward.
* Progression: one server-side XP track (matches completed, correct votes, Donower survivals); levels gate portal role applications and cosmetic unlocks. Values in `tuning.yaml`.
* **Content policy:** text humor may be suggestive within the existing content policy — **never pornographic or explicit**. Erotic-leaning media ships only in adult-rated, age-gated packs; store age ratings and age-assurance/consent behavior must be reviewed for each intended platform/market; self-declared age is not assumed sufficient evidence of compliance. Enforced twice: automated screen + human curation (⚙️ §3).
* Compliance: capture versioned user-terms acceptance before authored chat/UGC as well as contribution-specific consent; anonymous device accounts by default, with **one-tap registration via Google Sign-In or Facebook Login** (OAuth 2.0 / OpenID Connect) to carry progress, Noin, and entitlements across devices — the linked identity stores only the provider subject id and email, never shown publicly; privacy notice at first launch; age gate + per-pack age ratings; Google UMP consent before any personalized ad; in-app delete-my-data backed by a server endpoint; purchases exclusively through platform billing; contributor license grants (commercial use + modification) stored with terms version and timestamp per submission.
* Analytics: no third-party client SDK — the authoritative server witnesses every event; nightly jobs derive KPIs (retention, queue fill times, matches/day, Donower win rate by table size, Noin earn/spend flows, premium conversion, pack attach rate) from the audit stream.
* Moderation & admin: locale-aware nickname filtering, player-facing report/block capability, published support contact, Guard freezes with admin-final bans and audited admin actions (kick, ban, close room, avatar/content takedown) — tested before public launch. Blocking hides user-authored chat/UGC and prevents future co-matching/invites; it never hides required public card/vote evidence, reveals a private block relation, removes a player mid-match or changes scoring. Leaving/reporting remains available. Serialize block changes with future queue reservations and test symmetric exclusion without public disclosure; live safety intervention remains an admin action. See the business plan's sourced platform checkpoints; no store acceptance is implied.
* Platforms & release: **Android native + Web PWA first**, **iOS native fast-follow** once retention is proven. CI builds all three targets from day one. App-size budget enforced in CI: authorized text arrives through role-scoped state; binaries stay lean.

The technical transition design expands operational contracts and evidence; the
Roadmap alone orders implementation. The business plan records assumptions and
commercial gates. Neither historical completion rows nor this spec imply that
the text transition has shipped.
