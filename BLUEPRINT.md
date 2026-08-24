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

## 🎲 The Game at a Glance

Knowoff is an online social deduction party game for **exactly 4 or 6 players** — matched with people around the world in **Quick Play** (the main product), or gathered in person in a **Local Room**. Each round, one media item — **Nown** — appears on every phone except the Donowers'; Donowers must pretend they see it. In a randomly ordered sequence of timed turns, everyone plays one card that supposedly relates to Nown — each play revealed the moment it lands — the table argues, then votes. **The most-voted player is eliminated and their role is revealed** — eliminating a Nower wastes the vote. **Nowers win by voting out every Donower. Donowers win — together — if the votes run out first.**

There is deliberately no separate rulebook: the **Game Rules** section below is the single source of truth, and all player-facing help text, store copy, and the how-to-play clip are derived from it.

### Terminology (normative — written without "the")

| Term | Meaning |
|---|---|
| **Nower(s)** | Players who see Nown |
| **Donower(s)** | Players who can't see Nown; nobody knows who they are |
| **Nown** | The media item of a round — image, GIF, or text (no audio or video at v1) |
| **Knowoff** | The vote at the end of every round |
| **Round** | Card play + discussion + one Knowoff |
| **Match** | Up to 2 votings at 4 players, up to 3 at 6 — until a team wins |
| **Session** | Matches played in one room; keeps a running scoreboard |
| **Noin** | The game currency — earned by playing, sold in bulks (💰) |

---

## 🧱 Tech Stack

| Layer | Technology | Role |
|---|---|---|
| Client | Flutter — one codebase: native Android/iOS apps + Flutter Web **PWA** (desktop, and app-less guest fallback on any phone browser) | UI, WebSocket client, client `MediaEngine` (pack metadata sync, asset prefetch & cache, Donower placeholder renderer) |
| Backend | Go | Authoritative game server: matchmaking, rooms, timers, roles, media dealing, votes, scoring, economy — plus the Admin Console and Contributor Portal (server-rendered) |
| Database | PostgreSQL | Durable data: profiles, match results, Noin wallet & ledger, entitlements, leaderboards, media metadata & contribution workflow |
| Cache / Pub-Sub | Redis | Matchmaking queues, room→node routing, session presence, cross-node pub-sub, rate limiting |
| Assets | MinIO (S3-compatible) on the home server, fronted by **Cloudflare Tunnel + CDN cache** | Media-pack assets — content-hashed and immutable, so the edge cache absorbs nearly all traffic |
| Transport | WebSocket (JSON messages) | Single realtime channel between client and server |
| Infrastructure | Docker Compose on the home server (now) → small VPS at public launch, then Kubernetes + Terraform (later) | Everything containerized from day one |

Architecture decision records:

* **Server-authoritative over P2P** — a trusted authority owns role secrecy, the phase clock, blind-window resolution, and the currency ledger.
* **Flutter everywhere** — native haptics for Poke, one codebase for three surfaces, and the web build keeps a zero-install join path.
* **Home-server-first hosting behind Cloudflare (≈ $0 infra pre-launch)** — `cloudflared` tunnels the game server and asset host out with no open ports, no exposed home IP, TLS at the edge; content-hashed assets get "Cache Everything" with long TTLs, so the edge serves media after first fetch. Low/medium media quality (⚙️ §3) keeps assets tiny, which is what makes this viable. Accepted trade: home uptime is service uptime — fine for development and beta; **the same Compose stack lifts to a ~€5/mo VPS at public launch** (an online-first product can't ride a home ISP into the stores), with Cloudflare R2's free tier (10 GB, zero egress) as the asset offload if needed.

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
* One voting per match can be re-run by a Revote card (§5).
* Match length: roughly **5 minutes** at 4 players, **8 minutes** at 6. Acting early and Ready (§8) can only shorten a match.

### 2. Roles, Nown & Hands

* The server assigns roles randomly and secretly at match start. Players check their role privately: **press and hold to show it, release to hide it** — everyone performs the same check, so nothing about it stands out.
* **Nown**: one item per round from the room's media pack — **image, GIF, or text**. No audio and no video at v1 (assets stay tiny, rounds are silent-autoplay-safe, and in local rooms sound would leak to Donowers; v2 may revisit).
* Donowers simply can't see Nown — their screen shows a basic placeholder prompt instead (designed and implemented during development), and that's all.
* Secrecy is enforced server-side: a Donower's device is never sent Nown at all (⚙️ §4). Nobody knows who is Donower or Nower until votes reveal roles.
* **Hands**: every player gets **5 cards** plus a personal **3-card draw pile**. Cards are prompts — **text, image, or GIF** — dealt by the relevance mesh (⚙️ §2) so every hand always holds a mix of strong, stretchy, and garbage options against every Nown in the match. That guaranteed ambiguity is what lets Donowers blend in and makes Nowers doubt each other.
* A match can never use more than those 8 cards, so a player always has a card to play.

### 3. The Round: Turn-Based Play

* **Turns, not a blind window:** at round start the server randomly assigns a turn order (re-randomized every round, role-blind). On your turn you have **15 seconds** (`timers.play_turn`) to take **one action** — play a card, or use a specialty (§5) — and your play is **revealed to the whole table immediately, with your name attached**; then the next turn begins. Building on what's already on the table is the point: a Donower is expected to read the earlier plays and put down something that relates. The random start seat is part of the tension — whoever opens the round, Nower or Donower, gets no earlier plays to lean on.
* Played cards stay on the table for the whole match — the evidence the votes are argued over.
* **Draws**: during your turn, you may draw from your 3-card pile — all at once or in parts. **Every draw is announced to the table** (who, how many), and **every pile draw costs match points** (−5 each, `points.draw_penalty`) — drawing is sometimes right, but panic-drawing is priced. The One More Free Card specialty (§5) is the one exception: its draw costs nothing, so spending it as your first draw makes that draw free. Drawing tells everyone your hand doesn't fit.
* **Timeout**: a player whose turn expires with no action auto-passes and loses one random card. Stalling costs.

### 4. Discussion & Knowoff (every round)

* After the round's final turn, a discussion window opens (`10 s × players`; ends early when everyone is Ready).
  * **Local rooms:** talk happens out loud at the table.
  * **Online rooms:** players argue through **Quick Chat** — a set of canned phrases and reactions ("I suspect P3", "my card fits, trust me", "that play was weird", emotes), sent as taps, localized automatically. **No free-text chat at v1** (see Product Baseline); free-text and voice are v2 candidates behind proper moderation.
* Then **Knowoff**: a 20-second blind ballot. Everyone still in the match votes for one player (never themselves); votes stay hidden until the window closes, then all votes are shown. The most-voted player is eliminated and their role revealed (§1). A tie triggers one 15-second **runoff** among the tied players only; if the runoff is still tied, the vote is a miss: it counts as one survived voting for the Donowers, eliminates nobody, and reveals no role.
* **Result window:** every vote's outcome is displayed for 15 seconds before it becomes final — this is the window where a Revote card (§5) can land. Then it applies.
* An eliminated player — Nower or Donower — watches the rest of the match: no plays, no votes, no chat, no pokes. Their screen no longer shows Nown (a revealed Donower could otherwise feed it to a surviving partner). Staying connected to the end collects their points as normal (§6, §7).
* After the match, the verdict screen shows all Nowns to everyone; Donowers finally see what they survived.

### 5. Card Specialties

Five specialties in two types. **Dealing is role-blind: any specialty can land in any player's hand.** Shuffle and Revote are restricted only in *use* — a Nower dealt Shuffle, or a Donower dealt Revote, simply holds a dead card (still usable as discard fodder). Deal frequencies are tuned in `tuning.yaml`.

**Type A — Standard (your one action for the round; costs one extra discard, except Pass):**

* **Pass** (occasional): skip playing a card this round. Only the Pass card is spent.
* **Reveal** (rare): show another player's entire hand to the whole table — media and specialty cards alike, all face-up. Safe by design: since any specialty can sit in any hand (dealing is role-blind), seeing a Shuffle or a Revote proves nothing about its holder's role. Costs one extra discard.
* **One More Free Card** (occasional): usable during your turn without spending your action — draw 1 fresh card from the mesh, **free of the draw penalty** (§3). Used before touching your pile, it serves as a penalty-free first draw — that's its edge. Costs one extra discard.

**Type B — Unique (free, use restricted by role, once per match):**

* **Shuffle** (rare — **usable by Donowers only**): usable only at the very start of a round, before the first turn begins. Every player's unplayed hand is returned and re-dealt fresh (draw piles untouched). The table is told *a Shuffle happened* — hands visibly change — but not who did it; since nobody knows who is a Donower, the alert exposes no one. It wipes out the plans Nowers built around saved cards.
* **Revote** (rare — **usable by Nowers only**): playable during the 15-second result window of any Knowoff, before the result finalizes. The shown result is **canceled: it reveals nobody, eliminates nobody, and does not count as a survived voting for Donowers.** A fresh ballot runs immediately with the full time, and only its result counts. The table sees who played the card — only a Nower can use it, so playing it publicly half-clears you; that's the price.
* **Unique cards fire once per match, total.** The same card can be dealt to two players (rare, since these cards are rare); only the first use works — later copies are dead cards, still usable as discard fodder.

### 6. Match Points & Noin Earnings

Two separate rewards come out of every match:

**Match points** — the competitive score. They feed the session scoreboard (local rooms) and the Weekly Leaderboard (Quick Play only, 🎮 §5), and every match's net also accumulates on the profile twice: into **Overall Points** (the lifetime total — it only ever grows) and into **Non-Converted Points**, a balance convertible to Noin (💰 §1: 100 points → 1 Noin, one-way). A match's net floors at 0 — draw penalties can empty a match's gains, never dig debt. A player who is absent when the match ends scores 0 points for it. Eliminated players are not absent: staying in the lobby to the end collects everything.

| Event | Points |
|---|---|
| Your vote names a Donower | +10 |
| Nower team win | +10 each Nower |
| Donower team win | +30 each Donower — caught Donowers included (they win together) |
| Each card drawn from your pile | −5 (a One More Free Card draw is exempt — §5) |

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

* **Ready** — for every player, in every room type (4 or 6, local or online): a play turn ends the moment its player acts, and in discussion, marking Ready (or having nothing left to do) counts you in — when everyone is Ready the discussion window ends early. Fast tables play fast; the 15 s turn and `10 s × players` discussion timers are only ceilings. **Ballots, runoffs, and result windows always run their full time** — blind and cancelable to the end.
* **Poke**: once per target per round, you may poke a player who hasn't acted — the turn player sitting on the clock, or anyone not yet Ready in discussion. Their phone buzzes (native apps) and their screen shakes (everywhere — the web PWA has no vibration). Pokes show who poked whom. No score effect; the cap is enforced server-side.

---

## 🎮 Game Modes & Live Ops

### 1. Online Quick Play (the main product)

* One FIFO queue per room size (4 or 6), running on the core pack plus a rotating featured pack. Tap Play, get a table of strangers, argue in Quick Chat, vote.
* **Launch liquidity — labeled backfill bots:** when a queue can't fill a room within `liquidity.queue_timeout_s` (default 25 s), the server tops it up with bots — server-side (`server/internal/bots`, reusing the `gamebot` policy engine), every seat acting through the same validated intent pipeline as humans. Every bot seat carries a visible 🤖 badge and a reserved bot nickname — in a game about reading people, a disguised bot would be a scandal; a labeled one is a practice partner. Humans always outrank bots for seats, and a room never starts below `liquidity.min_humans`.
* Guardrails: bot seats earn nothing; matches count for the Weekly Leaderboard only with ≥ `liquidity.leaderboard_min_humans` humans, and grant team-win Noin only with ≥ `liquidity.noin_min_humans` humans — a bot table can never become a Noin farm. Backfill sunsets per queue automatically once fill times stay healthy.
* Free accounts play `economy.free_daily_quickplay_matches` per server day (**3 at launch** — a daily taster; regular play runs on earnable Play Passes or Premium); a Play Pass or Premium (💰 §2) removes the cap entirely — **a pass holder or Premium subscriber never hits it**. **Local Rooms are never capped.**

### 2. Local Rooms

Private rooms for people in the same physical place: the host shares a QR code (or 6-character code); the QR deep-links into the native app if installed, the web PWA otherwise — a guest without the app is never blocked. Discussion happens out loud; everything else plays identically to Quick Play. Uncapped, always.

### 3. Weekly Nown Challenge (Community Event)

The game itself, stretched into a week-long social event for the whole community:

* **The topic is a Nown:** every Monday the server publishes the week's topic — an image, GIF, or text, exactly like a round's Nown. Players respond the way they play cards in a match: upload the one entry (image, GIF, or text) that best matches the topic.
* **Open to all players, in-app** — no portal role needed. One entry per player, **immutable once submitted** — no edits, no replacements. **The system accepts the first 100 entries**, then intake auto-closes; a slot reopens each time screening rejects an earlier entry.
* **Screening before visibility:** every entry passes the automated screen plus a human check (curators or admin) before it becomes publicly visible and votable. Rejected entries never appear.
* **Voting:** open to all players — one vote each, never for your own entry, **immutable once cast**. Tallies are public and live.
* **Week Winner:** at the weekly close, the most-voted entry wins. Its owner holds the **Week Winner title — shown on their profile and in lobbies until the next winner is crowned** — and receives a large Noin award (`noin.challenge_winner`). Winning and standout entries may also enter the community pack, with credits.
* **Consent:** submitting requires explicit acceptance of the contribution terms — the entry may be used in the system, commercially, and in modified form (perpetual, non-exclusive license). Terms version + timestamp are stored with the entry. No consent, no upload.

### 4. System Notices & Announcements

Players are never surprised by downtime:

* **Notice types:** scheduled **maintenance** (start time + expected duration), **downtime/incident updates**, and **general announcements** — events, pack releases, rule changes.
* **Delivery:** connected clients receive `system_notice` events in real time over the WebSocket; on app start the client fetches all active notices over HTTPS. Notices render as dismissible banners on the main menu plus a persistent notice inbox — never a blocking gate, except a hard-maintenance countdown.
* **Maintenance flow:** a scheduled window announces itself at T−24 h, T−1 h, and T−10 min; when it opens, matchmaking stops admitting new matches and running matches drain to their natural end before the server goes down. Nobody loses a live match to planned work.
* **Ops:** notices are composed, scheduled, localized, and withdrawn in the Admin Console (🛡️), audited like every admin action; scheduling a maintenance window arms the matchmaking drain automatically.

### 5. Weekly Leaderboard

* Cycle: Monday–Sunday on the server clock; Monday announces last week's podium alongside the new Week Winner.
* Score: the sum of a player's match points for the week — **Quick Play matches only** (private and local rooms are collusion-trivial and stay off the board), with backfilled matches counting only at ≥ `liquidity.leaderboard_min_humans` humans, and a daily cap on counted matches to blunt pure grind.
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
| **Curator** | Everything a Contributor can, plus: **create Nowns and the decks that relate to them** — authoring cards against a Nown, testing hands in the deal simulator, checking band coverage; screen Weekly Challenge entries before they go public. Curators work from the **Curator Guide**, a clear rulebook derived from this blueprint (⚙️ §2–3): quality targets, tone rubric, band-coverage requirements, and how to test a Nown before submitting it for certification |
| **Guard** | Community safety: review flagged accounts and **freeze** them — a timeboxed suspension (up to 48 h) from matchmaking and the portal, pending admin review. **Final action is always the admin's**: dismiss, timed ban, or permanent ban. One active freeze per Guard per target; freezes auto-expire if no admin acts |
| **Admin** | Everything: role grants, bans, pack publishing, challenge scheduling, takedowns |

* **Rewards:** contributors and curators earn **credits (name in the pack manifest and on the profile) and Noin** for accepted work — `portal.noin_per_accepted_asset` per published asset; the Weekly Nown Challenge pays its Week Winner from the same rails (🎮 §3). Attribution is stored per asset, so richer reward schemes later are an economy change, not a migration.
* **Submission terms:** every upload requires explicit acceptance of the contribution terms — perpetual, non-exclusive license, **commercial use and modification permitted** — with the accepted terms version and timestamp stored per submission (🎮 §3).

### 2. Submission Pipeline

* Flow: upload or write media (**image, GIF, or text** only) → automated processing (transcode to quality targets ⚙️ §3, perceptual-hash dedupe, auto-tag, embedding, automated content screen) → **submit** → screening/curation → accepted media enters the next pack version.
* Workflow states: `draft → submitted → in_review → approved | rejected → published(pack-tag)` — every transition audited. **Submissions are immutable once submitted** — no edits; withdraw and resubmit is the only correction path, and it costs the queue slot.
* Moderation: everything passes the automated screen *and* human review before any player sees it; published media stays reportable (👤 §3) and takedown-able, with removals shipping in the next pack version.

### 3. Media Workbench (dev/staging — ships early, Phase 2)

The same application pointed at dev/staging, where the owner curates the AI generation pipeline before the community exists:

* **Batch ingestion:** watches ingest folders/buckets for ComfyUI (images, GIF loops) and Ollama (text card) output; every asset auto-processed on arrival.
* **Bulk curation grid:** keep/kill at keyboard speed with tone-bucket and rating assignment; keep-rate measured per batch (the pipeline's core KPI — expect 10–30% at this humor bar).
* **Embedding sanity view:** nearest-neighbor browser for any asset — catches mis-embedded media before it corrupts dealing.
* **Deal simulator:** for any candidate Nown, render the hands the mesh would actually deal at both table sizes — the same tool Curators later use, per the Curator Guide.

### 4. Architecture

Server-rendered from the Go binary (same pattern as the Admin Console) on a public subdomain — no separate SPA build chain. Role and workflow state in PostgreSQL; binaries in object storage; the automated moderation screen is provider-swappable.

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
  1. Hook — "One of you can't see this." A meme cuts to static on one phone among four.
  2. Roles — the press-and-hold role check; one card whispers *you're Donower*.
  3. Nown appears on every screen at once; a Donower's screen shows only a plain placeholder — and nobody can tell.
  4. Play — cards hit the table one turn at a time, each revealed instantly with a name; caption "whose card doesn't get it?"
  5. Pressure — a draw announcement, a poke shake, a Shuffle alert, Quick Chat accusations flying.
  6. Knowoff — votes land; the loser is out, role face-up… a Nower. The table groans; one vote left.
  7. Verdict — the Donower grins; "Donowers win together." Noin rains onto the scoreboard. Logo out.
* Ship gate: produced on final production UI, released with prod — no clip work while gameplay, engine, and netcode remain open.

---

## 🎨 Visual Identity: Soft Neo-Brutalism Design Matrix

Direction locked: **pastel neo-brutalism, illustration-light**. A brutalist skeleton — thick ink borders, hard zero-blur shadows, chunky type, flat fills — wearing a soft candy palette; personality comes from tiles, type, and color, not mascots or scene art.

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
  * **Support accents** (ADR-008) — categorical only, never a verdict signal: `canvasDeep` `#C4A8F0` for header bands and rails, `tangerine` `#FFB020` for currency, streaks and heat, `aqua` `#7FE7DC` for time, connection and neutral information. Ten colours total is the ceiling; a further hue is another ADR.
* Structure: every container carries `Border.all(width: 3, color: ink)` and a hard shadow `BoxShadow(color: ink, offset: Offset(4, 4), blurRadius: 0)`. Corners rounded — radius 16 for cards and sheets, 12 for buttons, full pill for stat chips. Pressing a control collapses its shadow to zero offset while the control translates onto its own shadow footprint: the signature brutalist click. Shadows come from a **named tier scale** — `sm` (3,3) for chips and badges, `md` (4,4) for cards and buttons, `lg` (8,8) for overlays and hero moments, `lift` (7,7) for pointer hover — plus tinted `limeGlow`/`pinkGlow`/`violetGlow` reserved for celebration. Blur is always `0`.
* **Structured disruption:** decorative and celebratory surfaces (the hand fan, the evidence table, empty-state stickers, verdict banners) sit at a small rotation drawn from a closed set (`KoTilt`: ~1.1°, ~2°, ~2.9°). Ballots, timers, forms, and anything a fairness rule depends on stay mechanically aligned — never rotated.
* Typography: **Baloo 2** (SIL OFL, bundled under `client/assets/fonts/` — ADR-007) is the locked display face for headings, timers, room codes, tallies, and Noin numbers; body copy stays on a plain geometric sans. **Highlighter emphasis is the house style:** the revealed role, a Noin delta, the clip caption — key phrases sit on a lime marker sweep, not bold-only.
* Illustration policy — deliberately sparse: no mascot, no scene art in the match flow. One tiny single-weight doodle glyph set (13 glyphs: sparkle, static-burst, eye, cloud, placeholder, crown, coin, cards, check, cross, poke, mask, clock) reserved for empty states, win moments, state badges, and the Donower-side placeholder.
* Fixed color semantics: violet = interact, lime = truth/reward, pink = accuse/risk, ink = information. No verdict leans on hue alone — color always pairs with icon + label (colorblind-safe by construction).
* Performance guardrails: flat fills (the one gradient exception above), zero blur radii, no stacked translucency — low-end devices are the norm.
* Asset strategy — code first, raster last: UI chrome is 100% widgets/`CustomPainter`s (borders, hard shadows, grid tile, highlighter sweep, press animation); doodles ship as hand-authored SVG paths. True raster — preset avatars, app icon, store art — is produced offline in curated batches via the **nano banana (Gemini image) API**, prompts derived from this matrix; candidates → human curation → consistency pass → committed like any asset. API keys live under the config discipline (📦 §3). In-game *media content* comes exclusively from media packs (⚙️) — the design system and the content pipeline never mix.

---

## 🏛️ Codebase Taxonomy & Separation of Concerns

```text
knowoff/
├── client/                      # Flutter — one codebase: Android, iOS, Web (PWA)
│   └── lib/
│       ├── core/
│       │   ├── config/          # Client config loader (server URL, feature flags)
│       │   └── network/         # GameTransport abstraction + WebSocket implementation
│       ├── data/
│       │   ├── models/          # GameState, Player, Card, NownRef DTOs (mirror server protocol)
│       │   └── repositories/
│       ├── domain/
│       │   ├── entities/
│       │   ├── repositories/
│       │   └── usecases/        # QuickPlay, JoinRoom, PlayCard, DrawCards, CastVote, SendQuickChat, ConvertPoints
│       ├── presentation/
│       │   ├── state/           # Riverpod state for the server-driven phases
│       │   ├── screens/         # MainMenu, Queue, Lobby, Round, Discussion, Knowoff, Verdict, Profile, Leaderboard, Store, NoticeInbox
│       │   └── widgets/         # NownStage, HandFan, PlayTable, VoteBoard, QuickChatBar, PokeNudge, ReadyButton, RoleCard, NoinBadge
│       └── media/               # Client MediaEngine: pack metadata sync, signed-URL prefetch, LRU asset cache, Donower placeholder renderer
├── server/                      # Go authoritative game server
│   ├── cmd/knowoffd/            # main.go — wiring, config load, graceful shutdown
│   ├── internal/
│   │   ├── config/              # Single typed config struct from layered YAML
│   │   ├── transport/           # WebSocket handling, codec, connection lifecycle
│   │   ├── lobby/               # Quick Play queues, room lifecycle, QR/room codes, reconnect grace
│   │   ├── game/                # Phase state machine, timers, roles, votes, forfeits, scoring
│   │   ├── bots/                # Quick Play backfill bots (labeled; reuses gamebot policy engine)
│   │   ├── media/               # Pack loader, relevance mesh, dealing, signed-URL issuing, role-scoped payloads
│   │   ├── economy/             # Noin wallet, ledger, play passes, entitlements
│   │   ├── portal/              # Contributor Portal + Media Workbench (server-rendered) + Admin Console
│   │   └── store/               # Postgres repositories, Redis queues/presence/routing, object-storage client
│   └── migrations/
├── deploy/
│   ├── compose/                 # server + postgres + redis + minio + cloudflared — one command up
│   ├── k8s/                     # Future — scaffolded, not required to run
│   └── terraform/               # Future — provider-agnostic modules
├── tools/
│   ├── mediapack/               # Go CLI: ingest → tag → embed → certify → bundle → simulate (⚙️ §3)
│   └── gamebot/                 # Go CLI: protocol-level dev/test bots
├── content/                     # Generation-side inputs: prompt templates, tone matrix, style presets, curation overlays
└── configs/                     # base.yaml + <env>.yaml overlays (incl. gameplay/tuning.yaml)
```

Separation rule: `server/internal/game`, `server/internal/media`, and `server/internal/economy` contain **all** rules, dealing, secrecy, and currency logic — the client renders state and sends intents; it never decides an outcome, never computes a balance, and never receives data its role shouldn't see.

---

## ⚙️ Media Engine Specification

The Media Engine owns what media exists, how hands are dealt against it, and who is allowed to see what. It lives **server-side in Go**; the client carries a thin mirror for pack sync, prefetch, and placeholder rendering only.

### 1. Media-Pack Bundle Format

* A pack is a versioned bundle: `manifest.json` (pack tag e.g. `core-2026.10`, checksums, license & credits, age rating, BCP 47 language tag — packs are language-scoped, so new languages ship as new packs, never format changes), `media.jsonl` (per Nown: id, type `image|gif|text`, asset ref, embedding vector, tags, tone bucket, rating), `cards.jsonl` (per hand card: id, type `text|image|gif`, asset ref, embedding, tags), plus assets in object storage addressed by content hash.
* Version discipline: the server embeds the active pack tag in `phase_started`; clients sync **metadata** OTA on app start and on unrecognized tags, verify checksums, and hot-swap between matches — never mid-match. Assets stream on demand via the prefetch protocol (§4) with an LRU cache.
* Theme packs are additional bundles in the same format; pack updates and takedowns ship as version bumps the server hot-swaps without redeploying.

### 2. The Relevance Mesh

* Every Nown and card carries an **embedding** in one shared multimodal space (local SigLIP/CLIP at build time; Gemini Embedding 2 as the API alternative). Tags remain human-facing metadata and theme filters — **similarity, not tag intersection, is the balance mechanism**, because tag vocabularies rot and noisy tags silently break dealing.
* Relevance bands over cosine similarity (thresholds in `tuning.yaml → dealing:`): **high** ≥ `band_high`, **distant** in [`band_low`, `band_high`), **chaos** < `band_low`.
* Dealing guarantee: the server picks the match's Nowns up front (secret, RNG seed logged) and deals each 5+3 hand as a constraint deal: against **every** scheduled Nown, each player holds ≥ `min_high_per_nown` high cards and ≥ `min_distant_per_nown` distant cards, with the remainder chaos. Every hand always has a good answer, a stretch, and garbage — for Nower and Donower alike.
* Shuffle re-deals are dealt *against the current Nown schedule* so the guarantee survives mid-match mutation.
* Per-media candidate lists for all bands are precomputed at pack build; runtime dealing is array sampling, zero embedding math in the hot path.

### 3. Media Pipeline (`tools/mediapack`) & Content Production

* Pipeline stages (CLI + Workbench/Portal UI over the same code): `ingest` (transcode, EXIF strip, perceptual-hash dedupe) → `screen` (automated moderation) → `tag` + `embed` → human curation (🧑‍🎨) → `certify` → `bundle` → `publish`.
* **Quality targets — deliberately medium/low:** images ≤ 720 px longest side, compressed WebP; GIF loops ≤ 480p, ≤ 2 MB, re-encoded as animated WebP; text plain. Two reasons: small assets keep prefetch instant and the edge-cache path cheap, and lo-fi *is* the meme aesthetic.
* **Content standard:** humor may include sexuality within the bounds of eroticism — suggestive, cartoon, drawn, abstract — but **never pornographic or explicit content**, and always within app-store content rules. Erotic-leaning media carries an adult rating and ships only in age-gated packs (Product Baseline); the automated screen and human curation both enforce the line.
* **Copyleft-first sourcing:** wherever licensing allows, Nowns and cards are drawn from openly licensed and public-domain sources — letting players enjoy media they already know and love is part of the project's mission. Primary wells: [Wikimedia Commons](https://commons.wikimedia.org), [Openverse](https://openverse.org), [Flickr Commons](https://www.flickr.com/commons), the [Internet Archive](https://archive.org) (including [GifCities](https://gifcities.org) for GIFs), and [Project Gutenberg](https://www.gutenberg.org) / [Wikiquote](https://www.wikiquote.org) for text. Each imported asset stores its license and attribution in the pack manifest (§1); share-alike (CC BY-SA) obligations are honored in pack credits; NC/ND-licensed media is excluded — the game is commercial (see the [Creative Commons license guide](https://creativecommons.org/licenses/)).
* Certification (the anti-dead-content gate): a pack version is publishable only if it declares its language tag (§1), every Nown has full band coverage for a 6-player deal, every card is reachable in some band, and Monte Carlo `simulate` confirms deal feasibility at both table sizes. Uncertifiable media stays in draft. The same checks back the Curator Guide's testing workflow (🧑‍🎨 §1).
* `mediapack simulate` also answers balance questions offline: band-threshold sweeps, Donower-survival proxy rates under bot policies, Shuffle and Revote impact — tune `tuning.yaml` until distributions look right, then spend scarce playtests on feel.
* Production stack (verified on the owner's RTX 4080 Mobile, 12 GB VRAM): images via SDXL or Flux.1-Schnell fp8 (Apache-2.0); GIF loops via LTX-Video 2B distilled or Wan 2.1-1.3B (~8 GB), rendered to animated WebP; text cards via Ollama-served 7–14B models. API lane for style-critical or overflow work: nano banana (~$0.034–0.067/image), batch pricing at half rate. The binding constraint is human curation keep-rate, not compute.
* Tone rubric: the four-bucket humor matrix (millennial cope / Gen-Z absurdism / social awkwardness / chaos) lives in `content/tone-matrix.md`; every asset carries its bucket for pack-mix balancing.

### 4. Secrecy, Sync & Anti-Cheat

* Role-scoped payloads: every gameplay event is rendered per-recipient. During a round, Nower clients receive `{nown: {id, signed_url, type}}`; Donower clients — and every eliminated player — receive `{decoy: true}`. **Nown never crosses the wire to a device that shouldn't have it.**
* Signed URLs are short-lived and single-round; Nowers prefetch during the inter-round countdown and every screen flips on one synchronized `show` tick. A still-loading Nower renders the same placeholder a Donower sees, so loading state leaks nothing either.
* The server owns the phase clock; client timers are display-only; late intents are rejected. All match-deciding events — plays, specialty uses, votes, Revotes — are intents resolved exclusively server-side. All Noin movement happens in the server ledger; the client only renders balances.
* Every scoring, role, and currency event lands in the append-only audit stream — the same stream that feeds stats, the leaderboard, KPIs, and report replays.

#### Designer Workbench & Tuning

Every number a playtest could question lives in one versioned file, `configs/gameplay/tuning.yaml`; structure is code, values are keys:

```yaml
seed: 42                            # reproducible dealing — same seed, same match

game:
  room_sizes: [4, 6]                # the only valid room sizes
  donowers_by_size: {4: 1, 6: 2}      # public knowledge at the table
  votes_by_size: {4: 2, 6: 3}       # one Knowoff per round; match ends early when votes can't catch remaining Donowers
  min_connected: 3                  # below this, the match waits for grace then ends scored (Rules §7)
  reconnect_grace_s: 20             # also the team-forfeit grace (Rules §7)
  pokes_per_target_per_round: 1
  abandon_cooldowns_s: [60, 300, 900]   # escalating Quick Play matchmaking cooldowns

timers:                             # seconds; which windows may fast-forward is structure (§8)
  play_turn: 15                     # each player's turn; the play reveals immediately, the turn ends on action
  discussion_per_player: 10         # Ready unanimity ends it early
  knowoff_ballot: 20                # always runs full
  knowoff_runoff: 15                # tie-break among tied players; always runs full
  vote_result_window: 15            # result display before finalizing — the Revote window
  prefetch_countdown: 5             # inter-round countdown = Nower prefetch budget

hand:
  size: 5
  draw_pile: 3
  specialty_weights: {pass: 0.10, reveal: 0.04, one_more_free_card: 0.08, shuffle: 0.05, revote: 0.05}
  # dealing is role-blind: any specialty can land in any hand; shuffle is usable by Donowers,
  # revote by Nowers — off-role copies are dead cards (Rules §5)
  # unique specialties (shuffle, revote) fire once per match — later copies are dead cards

dealing:                            # relevance mesh (⚙️ §2) — v1 placeholders, tuned via simulate
  band_high: 0.55
  band_low: 0.30
  min_high_per_nown: 2
  min_distant_per_nown: 2

points:                             # match points — leaderboard, session scoreboard, Overall/Non-Converted accrual (Rules §6)
  correct_vote: 10
  nower_win_bonus: 10
  donower_team_win: 30
  draw_penalty: 5                   # per pile card drawn; a One More Free Card draw is exempt (Rules §3, §5)
  # a match's net points floor at 0; the net adds to both Overall and Non-Converted Points

noin:                              # currency earnings — instant, kept on disconnect (Rules §6)
  match_completed: 5
  nower_win: 30
  donower_team_win: 50
  correct_vote: 5
  donower_vote_survived: 10           # credited discreetly — never shown on any public surface (Rules §6)
  daily_first_win: 25
  daily_earn_cap: 300               # anti-farm ceiling on play earnings
  challenge_winner: 1000            # the Week Winner award (🎮 §3)
  contributor_accepted_asset: 100

economy:
  free_daily_quickplay_matches: 3   # per free account per server day; never applies under a Play Pass or Premium; local rooms never capped
  points_to_noin: 100              # Non-Converted Points per 1 Noin — one-way, multiples of 100, counts toward daily_earn_cap
  play_pass_prices: {day_1: 250, day_3: 600, day_7: 1200}    # Noin; passes never remove ads
  premium_yearly_discount_pct: 20   # Premium subscription (sole ad-removal path); monthly/yearly store products mapped at launch
  unlock_prices: {custom_avatar: 1000, poke_style: 400, theme_pack: 1500}   # Noin
  noin_bundles: [500, 1200, 3000, 8000]   # bulk IAP sizes; store price tiers mapped at launch

liquidity:                          # Quick Play backfill bots (🎮 §1)
  backfill_enabled: true
  queue_timeout_s: 25
  min_humans: 1
  leaderboard_min_humans: 3
  noin_min_humans: 2               # team-win Noin requires this many humans

liveops:
  leaderboard_daily_counted_matches: 10
  challenge_max_entries: 100        # intake auto-closes; rejections reopen slots
  challenge_votes_per_player: 1     # immutable once cast

portal:
  min_account_level_to_apply: 2
  submissions_per_contributor_per_day: 10
  guard_freeze_max_h: 48
```

**Economy balance protocol** — targets first, numbers second (all placeholders above are tuned against these):

| Target | Healthy band | The one lever |
|---|---|---|
| Active free player affords a 1-day Play Pass | every ~2 days of play | `noin.*` earn values |
| 7-day Play Pass for a committed free player | every ~8–10 days | `play_pass_prices` |
| Earned vs purchased Noin in circulation | ≥ 70% earned | bundle sizes/prices |
| Point-conversion share of Noin income | ≤ ~25% | `points_to_noin` rate |
| Draws per player per match | ~1 (drawing is a choice, not a habit) | `points.draw_penalty` |
| Free daily cap as the pass/Premium nudge | felt by regular free players, while a 1-day pass stays ~2 play-days of earnings away; casual once-a-day players untouched | `free_daily_quickplay_matches` |

`mediapack simulate` reports expected per-match Noin under bot policies at both table sizes; the nightly KPI jobs report the real numbers, and the levers above move one at a time.

---

## 🌐 Network Architecture: Server-Authoritative WebSocket

```text
[ Flutter app  (Android/iOS) ] ─┐                        ┌──────────────────┐
[ Flutter PWA  (any browser) ] ─┼─( WSS / JSON intents )►│  GO GAME SERVER  │◄──►[ Redis ]
[ Flutter app / PWA … seat N ] ─┘  ◄─(role-scoped events)│  - Queues & auth │      queues/presence
              ▲                                          │  - Phase timers  │◄──►[ PostgreSQL ]
              │ signed GETs (Nowers only)               │  - Role secrecy  │      profiles/ledger/
[ Cloudflare edge cache ]◄──[ MinIO on home server ]     │  - Media dealing │      leaderboard/media
  (tunnel: cloudflared — no open ports, TLS at edge)     │  - Votes/economy │◄──►[ MinIO (assets) ]
                                                         └──────────────────┘
```

* Protocol: one persistent WebSocket per client. Intents: `queue_quickplay`, `join_room`, `play_card`, `use_specialty` (covers Shuffle and Revote), `draw_cards`, `cast_vote`, `quick_chat` (canned phrase id), `ready`, `poke`, `report_media`, `convert_points` (100:1, outside matches). Events: `phase_started`, `role_assigned` (private), `round_started` (role-scoped Nown/decoy payload + the round's randomized turn order), `show`, `turn_started` (seat + 15 s deadline), `play_revealed` (attributed, immediate), `round_resolved` (play-phase summary), `shuffle_occurred` (anonymous), `vote_result_pending` (opens the 15 s result window), `vote_nullified` (attributed Revote), `knowoff_resolved` (elimination + role reveal), `match_verdict`, `points_scored`, `points_converted` (private), `noin_granted` (private, per-recipient — Rules §6 discreet crediting), `quick_chat` (broadcast), `system_notice` (broadcast — 🎮 §4). Versioned JSON with sequence numbers for ordered replay. **No display strings on the wire:** events and rejections carry stable ids/codes plus parameters; the client localizes them (Product Baseline).
* Fair arbitration: ballots and runoffs are blind-simultaneous — collected privately, resolved at window close. Play is sequential by server-enforced turn order with immediate attributed reveals; turn deadlines are server-owned. No match-deciding event is a speed race.
* Reconnect: session-token snapshot rejoin (Rules §7), role-scoped like everything else.
* Room→node affinity: every room lives on exactly one node (Redis maps `room_id → node`); no cross-node game state — the property that makes horizontal scaling trivial later.
* Live match state in server memory only; PostgreSQL for durable outcomes and the Noin ledger; Redis for queues/presence/routing; object storage for assets.

---

## 🤖 Bots: Development, Testing & Launch Liquidity

Bots fill seats in two sharply separated roles — dev/test bots that never meet the public, and the labeled Quick Play backfill bots (🎮 §1). In neither role does a bot take over a human's mid-match seat, and no bot is ever disguised as a human.

* **Dev/test bots:** `tools/gamebot` (Go CLI) spawns N bot players over the real WebSocket protocol — same intents, same timers, no server backdoors. Policy reuses `server/internal/media` as a library: play by noisy embedding preference as a Nower; play plausible-band cards that relate to the table's earlier plays as a Donower; draw when the hand scores badly (accepting the point penalty); vote by a noisy suspicion heuristic; occasionally Shuffle/Revote when held. Seeded — a failing match replays exactly. Bots act early and Ready immediately: a 6-seat dev match crosses every phase in well under a minute. Solo development against 5 bots is the daily loop.
* **Backfill bots:** run inside the server (`server/internal/bots`), reusing the same policy engine with per-match randomized personality parameters so regulars can't farm a fixed tell. Labeled 🤖 always; humans outrank bots for seats; economy and leaderboard guardrails in 🎮 §1. Sunset by measurement: per queue, once p50 time-to-fill stays under the timeout for a sustained window, the scheduler stops adding bots there.
* Configuration: a `bots:` block exists only in `local.yaml`/`staging.yaml`; the key is absent from `prod.yaml` and the server refuses external bot connections when unset. Backfill bots are configured separately (`liquidity:` in `tuning.yaml`) and are in-process, so the connection-refusal rule is untouched.

---

## 📦 Infrastructure & Deployment

### 1. Containerization Principles

* Every runnable ships as a container from day one: `server` (distroless static Go binary), `postgres`, `redis`, `minio`, `cloudflared`, dev tooling (migrations runner, adminer). Multi-arch images, environment-agnostic — the same images run on the home server now, a VPS later, Kubernetes after that; behavior differs only by mounted config.
* **Every backend service is stateless except the data stores — PostgreSQL and Redis.** The Go server writes nothing to local disk: durable state in PostgreSQL, coordination in Redis, assets in object storage (MinIO holds only immutable, re-publishable pack assets), live matches in ephemeral memory with room→node affinity — any server instance can be replaced at will.

### 2. Hosting: Home Server + Cloudflare (dev/beta), VPS at Launch

* Dev and beta run entirely on the owner's home machine under Docker Compose. **Cloudflare Tunnel** (`cloudflared`, free) publishes `play.<domain>` (game server — WebSockets pass through) and `cdn.<domain>` (MinIO assets) with no open ports, no exposed home IP, TLS at the edge.
* **Edge caching does the heavy lifting:** assets are content-hashed and immutable, so `cdn.<domain>/*` gets a Cache-Everything rule with a long edge TTL; after first request Cloudflare serves the media and the home uplink sees near-zero asset traffic. Low/medium media quality (⚙️ §3) keeps objects small; a round's prefetch is a few hundred KB across a table. v1 media is images and text only (loops ship as animated WebP), comfortably inside Cloudflare's free-plan content rules.
* **Public launch moves the stack to a small VPS** (~€5/mo class): an online-first product can't ride a home ISP's uptime into the stores. It's a lift-and-shift — same images, same config model; assets can offload to Cloudflare R2's free tier (10 GB, zero egress) if the home box retires completely.
* **Volume-portable data from day one:** every stateful service pins a named Docker volume (`pg_data`, `redis_data`, `minio_data`); containers never write outside them. Cloud migration is a rehearsed runbook, not a project: stop writes, snapshot (`pg_dump`/base backup for PostgreSQL, RDB snapshot for Redis, `mc mirror` for MinIO), restore onto the target volumes, re-point the tunnel/DNS — no schema changes, no path changes.
* Secrets that leave the machine: the tunnel token and API keys — injected via env into the config layer below, never committed.

### 3. Centralized Configuration (No Scattered Env Vars)

* All config in `configs/` as layered YAML: `base.yaml` holds every key with sane defaults; `local/staging/prod.yaml` are thin overlays. The server loads one merged typed struct at boot and fails fast listing missing/invalid keys.
* Environment variables do exactly two jobs: selecting the config file (`KNOWOFF_CONFIG=…`) and injecting secrets (DB password, JWT key, storage credentials, tunnel token, moderation/embedding API keys) via `${VAR}` interpolation. Secrets never live in YAML files or images.
* Maps 1:1 onto Compose volumes now, ConfigMaps + Secrets later, Terraform templating after that.

### 4. Local Development — Docker Compose

* `deploy/compose/docker-compose.yaml` brings up the full stack in one command: server (live-reload in dev profile), PostgreSQL with auto-migrations, Redis, MinIO with seeded dev pack, adminer. Profiles: `core`, `tools`, `test`, `edge` (adds `cloudflared` — beta-at-home only; dev needs no tunnel).
* A full 6-player match — four `gamebot` seats plus two real clients (one native, one PWA) — must be playable against the local stack with zero cloud dependencies, including media prefetch from MinIO.

### 5. Kubernetes & Terraform Readiness (Future, Designed-For Now)

* `/healthz`, `/readyz` (checks Postgres/Redis/storage), Prometheus metrics on a separate port; graceful shutdown drains in-flight matches on SIGTERM; WebSocket routing later uses sticky sessions at the ingress — rooms never span nodes, so no mesh, no distributed state layer.
* `deploy/terraform/` as provider-agnostic modules; hosting strategy: home server → small VPS → orchestration, each step a config change, not a rewrite.
* Rule: no k8s/TF-blocking decisions in application code — no local file writes, no in-container state, no hardcoded hostnames.

---

## 💰 Monetization: The Noin Economy

One currency sits at the center of the business: **Noin**. Players earn it by playing well, buy it in bulks when they want more, and spend it on play passes, packs, and cosmetics. Design goals, in order: keep free players playing daily, make earned progress feel meaningful (a free player must be able to reach everything), and monetize impatience, identity, and commitment — never gameplay advantage. **No pay-to-win: nothing purchasable affects dealing, roles, votes, or scoring.**

### 1. Earning Noin

* Play rewards (Rules §6): completing matches, winning as either team, correct votes, surviving votes as a Donower (credited discreetly — Rules §6), first win of the day — credited instantly and kept even on disconnect. A daily earn cap (`noin.daily_earn_cap`) blunts farming, and team-win Noin requires ≥ `liquidity.noin_min_humans` humans in the match.
* Contribution rewards (🧑‍🎨 §1): Noin per accepted asset, and the Week Winner award of the Weekly Nown Challenge (🎮 §3).
* **Point conversion:** every match's net points land on the profile as **Overall Points** (lifetime, never decreases) and **Non-Converted Points** (a balance). The owner may convert Non-Converted Points to Noin at **100 points → 1 Noin** (`economy.points_to_noin`), in multiples of 100. Conversion is **one-way and irreversible**: converted points are subtracted from the Non-Converted balance forever, and Overall Points never change. Converted Noin counts toward `noin.daily_earn_cap`, so points can never bypass the anti-farm ceiling.
* Balance target: an active free player earns a 1-day Play Pass every ~2 days of play (protocol table in ⚙️ Tuning) — collecting is deliberately *not hard*; the sink structure below is what makes the economy work.

### 2. Play Passes (Noin) & the Premium Subscription

* **Play Passes are unlimited Quick Play**, sold as **1-day (250), 3-day (600), and 7-day (1,200) passes priced in Noin** — plain in-game finance, an earnable convenience and nothing more, so the play-more path always runs through the one currency. **Play Passes do not remove ads.**
* **Premium** is the one cash subscription — **monthly or yearly, with the yearly plan 20% off**, sold through platform billing — and it is the **only way to remove ads entirely**. It also includes unlimited Quick Play while active, so a subscriber never needs passes.
* Free accounts get `economy.free_daily_quickplay_matches` per server day (**3 at launch** — a deliberate taster: regular free play runs on earned Play Passes, ~2 play-days per 1-day pass, or on Premium; **the cap never touches a Play Pass holder or Premium subscriber**). **Local Rooms are never capped** — play with the people in your living room is always free and unlimited.

### 3. Noin Bulks (the cash lane)

* Bulk packs via platform billing (Play Billing / StoreKit): sizes in `economy.noin_bundles`, store price tiers mapped at launch. This is the only place money enters; everything money can get, play can also get — slower.
* Rewarded ads (SSV — the ad network's servers call our verification endpoint; the client callback grants nothing): an optional post-match ad **doubles that match's Noin**; Premium subscribers get the doubling automatically, ad-free. **Ad surfaces disappear only under the Premium subscription (💰 §2)** — Noin Play Passes never remove ads.

### 4. Theme Packs

* Curated media packs (humor verticals, seasonal, community highlights, age-gated adult-humor packs) priced in Noin (`economy.unlock_prices.theme_pack`).
* In private and local rooms, the **Host Pass** rule applies: only the room creator needs the pack; the whole table plays it, guests never pay. Quick Play runs the core pack plus a free rotating featured pack.

### 5. Cosmetics & Identity

* **Poke Styles** (visual + haptic effect sets) and the **Custom Avatar** unlock (👤 §2), priced in Noin. More identity items (card backs, reveal animations) ride the same entitlement rails later.
* Technical spine for all of it: a PostgreSQL wallet with an append-only ledger (earns, purchases, spends, refunds); debits atomic with entitlement writes; balances server-side only; all prices in `tuning.yaml` so economy tuning never needs a client release.

---

## 🧾 Product Baseline (v1 Decisions)

* Modes & discovery: Quick Play queues per room size (4/6) as the main surface; private/local rooms via 6-character codes + QR deep links. No public room browser, no skill rating at v1.
* **Communication: canned Quick Chat + reactions only.** No free-text, no voice, no video at v1. The transport is identical either way — a canned tap and a typed message ride the same WebSocket relay, so the *build* cost is the same; **the real cost of free text is operating it**: profanity filtering, harassment review, minors' safety, and store UGC requirements, all ongoing. Canned phrases eliminate that entire surface while staying localizable and sufficient to accuse and defend. Free text is a v2 candidate behind mute/report infrastructure. Local rooms talk out loud anyway.
* **Localization & language neutrality (day one):** the game ships in multiple languages, and the whole system is language-neutral even while the launch-locale list is short. No user-facing string is hardcoded anywhere: the client renders all text through Flutter localization catalogs (ARB/`intl` — ICU plurals, locale-aware numbers and dates), layouts tolerate text expansion, and the display faces carry a script-capable font fallback (the diacritics render check runs across launch locales — 🎨). **The server never sends display text:** every player-facing message crosses the wire as a stable id/code plus parameters — Quick Chat phrase ids, rejection/error codes, event fields — localized client-side (🌐). System notices are authored per locale with an English fallback chain (🎮 §4, 🛡️); media packs declare a BCP 47 language tag in the manifest (⚙️ §1) so content ships per language without a format change; the supported-locale list is config (`configs/base.yaml → localization:`), so adding a language is new translation catalogs plus localized store assets — never a code change. A pseudo-locale CI gate keeps hardcoded strings out from Phase 1 onward.
* Progression: one server-side XP track (matches completed, correct votes, Donower survivals); levels gate portal role applications and cosmetic unlocks. Values in `tuning.yaml`.
* **Content policy:** humor may be suggestive/erotic within store rules — cartoon, drawn, abstract — **never pornographic or explicit**. Erotic-leaning media ships only in adult-rated, age-gated packs; store age ratings set accordingly (17+/18+ where such packs are available), and the age obligation sits on the user's declared age at the gate. Enforced twice: automated screen + human curation (⚙️ §3).
* Compliance: anonymous device accounts by default, with **one-tap registration via Google Sign-In or Facebook Login** (OAuth 2.0 / OpenID Connect) to carry progress, Noin, and entitlements across devices — the linked identity stores only the provider subject id and email, never shown publicly; privacy notice at first launch; age gate + per-pack age ratings; Google UMP consent before any personalized ad; in-app delete-my-data backed by a server endpoint; purchases exclusively through platform billing; contributor license grants (commercial use + modification) stored with terms version and timestamp per submission.
* Analytics: no third-party client SDK — the authoritative server witnesses every event; nightly jobs derive KPIs (retention, queue fill times, matches/day, Donower win rate by table size, Noin earn/spend flows, premium conversion, pack attach rate) from the audit stream.
* Moderation & admin: locale-aware nickname profanity filter, conduct + media reports, Guard freezes with admin-final bans, Admin Console actions (kick, ban, close room, avatar/media takedown) — all live before public launch.
* Platforms & release: **Android native + Web PWA first**, **iOS native fast-follow** once retention is proven. CI builds all three targets from day one. App-size budget enforced in CI: packs stream, binaries stay lean.

Do not paste spec content back into this file. If this file has drifted
into a second monolith again, trim it back to this stub in the same change.

