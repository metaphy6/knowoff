# 📖 Knowoff — Project Architecture & Implementation Roadmap

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

* Palette tokens (Flutter constants; light theme only at v1):
  * `canvas` `#DCC8F7` — lavender field with a faint low-contrast grid tile; `surface` `#F7F2E9` warm cream for cards and sheets; `#FFFFFF` content wells inside them.
  * `ink` `#141414` — every border and every glyph; text is never gray-on-gray.
  * `violet` `#B49AF5` — the neutral interactive: buttons, selected tiles, timers, progress fills.
  * `lime` `#D4F04C` — the truth/reward signal: Nower catches, match points, Noin grants.
  * `pink` `#FF9ED2` — the risk/accusation signal: votes, the Knowoff board, Donower reveals. The palette's single permitted gradient (`#FFD9EC → #FF9ED2`) is reserved for the Knowoff reveal header.
* Structure: every container carries `Border.all(width: 3, color: ink)` and a hard shadow `BoxShadow(color: ink, offset: Offset(4, 4), blurRadius: 0)`. Corners rounded — radius 16 for cards and sheets, 12 for buttons, full pill for stat chips. Pressing a control collapses its shadow to zero offset while the control translates onto its own shadow footprint: the signature brutalist click.
* Typography: a chunky rounded display face for headings, timers, and Noin numbers (Baloo 2 / Fredoka class — both OFL; lock one after a diacritics render check), a plain geometric sans for body. **Highlighter emphasis is the house style:** the revealed role, a Noin delta, the clip caption — key phrases sit on a lime marker sweep, not bold-only.
* Illustration policy — deliberately sparse: no mascot, no scene art in the match flow. One tiny single-weight doodle glyph set (~12 glyphs: sparkle, static-burst, eye, cloud) reserved for empty states, win moments, and the Donower-side placeholder.
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

---

## 🗺 Roadmap — Step-by-Step Implementation Lifecycle

This chapter is the executable roadmap — the single source of truth for
sequenced work, living **in the same document as the spec it implements**.
The spec chapters above are the requirements; this chapter is the sequence
and the checklist. Every bullet is a checklist entry whose full
requirements live in the chapters its phase names under **Spec (required
reading)** — read them *before* implementing the phase's first bullet,
not after a gate fails. If a bullet and a spec chapter disagree, the spec
chapter wins; fix the drift in the same commit (Appendix A). A feature
change to any spec chapter lands together with its update to this chapter
— one document, one commit, no drift.

Agents implement phase-by-phase, drain every `[ ]` bullet in scope, append
tracking rows, then stage. Emoji references (🏛️ ⚙️ 🎮 💰 🧑‍🎨 👤 📦 🌐
🛡️ 🎨 🎬) point at the spec chapters above.

### 📊 Status snapshot

Update with `make roadmap.status` (parses the `[ ]` / `[x]` boxes in this
chapter).

| Phase | Items | Done | Status |
|---|---|---|---|
| 0 — Agent framework & project docs | — | — | ✅ landed (pre-roadmap) |
| 1 — Foundation | 12 | 12 | ✅ done |
| 2 — Media Engine & Pipeline | 9 | 9 | ✅ done |
| 3 — Realtime Game Loop | 18 | 0 | ⚪ planned |
| 4 — Accounts, Quick Play & Hardening | 11 | 0 | ⚪ planned |
| 5 — Noin Economy, Admin & Launch Polish | 12 | 0 | ⚪ planned |
| 6 — Contributor Portal & Community | 5 | 0 | ⚪ planned |

Phase 0 — the agent operating framework and this document — carries no
checkboxes; it landed before phase work began.

### 🧭 Guiding principles

- **The spec chapters are the requirements; this chapter is the sequence.**
  Implement from the chapters above, never from bullet text alone — each
  phase's **Spec (required reading)** line names the chapters that must be
  read before its first bullet is touched.
- **Server-authoritative always.** The client renders state and sends
  intents; it never decides an outcome, never computes a balance, never
  receives data its role shouldn't see.
- **Role secrecy is the one unforgivable failure.** Nown must never reach a
  Donower's (or eliminated player's) device — enforced server-side, proven
  by protocol-level tests, re-proven on every change to payload rendering.
- **Every tunable number lives in `configs/gameplay/tuning.yaml`.**
  Structure is code; values are keys. Economy and balance tuning must never
  need a client release.
- **Language-neutral from day one.** No user-facing string is hardcoded;
  the server sends stable ids/codes + parameters, never display text;
  adding a language is translation catalogs + localized store assets,
  never a code change (Product Baseline).
- Smallest change that turns the test green; tests move with code in the
  same commit.
- Reversible > impressive. One owner per phase; one tracking `run_id` per
  implementation pass.
- **No proof, no tick.** A `[ ]` bullet is done only when its named proof
  exists and passes — automated where automatable; manual drills (device
  traces, migration rehearsals) leave evidence in a tracking row.

### 🚫 Non-goals (v1) — locked by the spec chapters

Deliberate exclusions. Do not build these, and do not "improve" toward them:

- **No audio or video Nowns** — image, GIF, text only (sound leaks to
  Donowers in local rooms; assets stay tiny; v2 may revisit).
- **No free-text chat, no voice** — canned Quick Chat only; free text is a
  v2 candidate behind mute/report moderation infrastructure.
- **No public room browser, no skill rating.**
- **No pay-to-win, ever** — nothing purchasable affects dealing, roles,
  votes, or scoring.
- **No in-match economy** — no chips, no stakes; Noin and points are earned
  by the match, never wagered in it.
- **No disguised bots** — every bot is 🤖-labeled; no bot ever takes over a
  human's mid-match seat.
- **No third-party analytics SDK in the client** — KPIs derive from the
  server audit stream only.
- **No dark theme at v1** — light theme only, per the design matrix (🎨).
- **iOS is a fast-follow, not a launch target** — CI still builds it from
  day one.
- **Kubernetes/Terraform are scaffolded, not required** — and nothing in
  app code may block them: no local file writes, no in-container state, no
  hardcoded hostnames.

### 🧰 Skills discipline — how agents work this roadmap

Load from [`.agents/skills/`](../../.agents/skills/README.md) **before** the
matching work — at phase start, not mid-crisis:

- **Every phase, always:** `phase-persistence` (drain the scope before
  handing back), `minimal-change`, `test-driven-development`,
  `self-review` + `verification-before-completion` (before staging),
  `safe-run-wrapper` + `non-zero-exit-recovery` (every risky command).
- **Per phase:** each phase below carries a **Skills** line — the extra
  skills that phase's work specifically needs.
- **Whenever an architectural decision is made** (any phase):
  `adr-writing` → record it in [`docs/design/`](../design/README.md).

---

### Phase 1 — Foundation

**Scope id.** `phase-1`

**What.** The monorepo skeleton and the run-anywhere substrate: the 🏛️
codebase taxonomy on disk, the layered YAML config loader with
fail-fast validation, a Go server skeleton with health endpoints and
observability, the migration discipline, the full local Docker Compose
stack with a rehearsed volume snapshot/restore drill, and a Flutter client
scaffold (`GameTransport` + WebSocket echo + the localization scaffold)
building for **Android and Web from day one**.

**Why.** Every later phase assumes "fresh clone → one command → healthy
stack". Config discipline (env vars only select a config file and inject
secrets) and the stateless-server rule are cheap to establish now and
nearly unretrofittable later; building the Web PWA target from day one
surfaces browser limitations before code accretes around native-only
assumptions.

**How.** Tree per 🏛️ Codebase Taxonomy; config per 📦 §3 (one merged typed
struct at boot, failing fast with **every** missing/invalid key listed);
Compose per 📦 §4 with profiles `core`/`tools`/`test`/`edge`; multi-arch
distroless static Go image; named volumes `pg_data`/`redis_data`/`minio_data`
so data is migration-portable from day one.

**Skills.** `documentation-first` (README + make targets land with the
code), `security-by-default` (secrets never in YAML files or images),
`adr-writing` (any deviation from the 🧱 stack gets an ADR).

**Spec (required reading).** 🧱 Tech Stack + the three ADRs; 🏛️
Codebase Taxonomy; 📦 §1–5 (containers, hosting, config, Compose, k8s
readiness); Product Baseline (platforms & release, CI, localization).

**Proof tests.** Fresh clone → `docker compose up` → healthy stack; both
client targets connect and echo. Config negatives each fail fast listing
**every** error in one pass: missing key, wrong type, missing secret env
(named), unknown/typo key. SIGTERM with live connections: `/readyz` flips
first, sockets close cleanly, exit 0; stopping Postgres or Redis flips
`/readyz` while the process stays up and recovers without restart.
`compose down && up` preserves all three volumes; the snapshot/restore
drill restores onto fresh volumes with verified parity. A fresh database
migrates to head and a re-run is a no-op. The pseudo-locale build renders
every user-facing string transformed — one hardcoded string fails the
gate. `make doctor` exits 0; CI is green including lint and the app-size
budget.

- [x] Monorepo tree per 🏛️ taxonomy: `client/`, `server/`, `deploy/`, `tools/`, `content/`, `configs/`; the three founding ADRs (server-authoritative over P2P, Flutter everywhere, home-server-first behind Cloudflare — 🧱) recorded in `docs/design/`.
- [x] `configs/`: `base.yaml` + `local/staging/prod.yaml` overlays and `gameplay/tuning.yaml` seeded with the spec's v1 values (⚙️ Tuning); every key documented in-file; secrets only ever as `${VAR}` references, never literals.
- [x] `server/internal/config`: layered-YAML loader → one typed struct, `${VAR}` secret interpolation, fail-fast validation listing **all** missing/invalid keys in one pass **and rejecting unknown keys** (an overlay typo must fail loudly, never silently default) — table-driven unit tests covering every negative path.
- [x] `server/cmd/knowoffd` skeleton: config load; structured JSON logging with per-connection ids; panic-recovery middleware (a handler panic never kills the process); graceful shutdown on SIGTERM (`/readyz` flips first, connections close cleanly); `/healthz`, `/readyz` (Postgres/Redis/storage checks — dependency loss degrades to not-ready, never a crash loop); Prometheus metrics (build info, connection + goroutine gauges) on a separate port.
- [x] Migration discipline in `server/migrations`: versioned up/down pairs run by an auto-migrations runner; a fresh database migrates to head and a re-run is a no-op — enforced in CI from the first migration onward.
- [x] `deploy/compose`: one-command stack — server (dev live-reload), Postgres + migrations runner, Redis, MinIO (bucket bootstrap only; the dev pack seed arrives with Phase 2's fixture packs), adminer, `cloudflared` under the `edge` profile; healthcheck-gated startup order; profiles `core`/`tools`/`test`/`edge`; named volumes `pg_data`/`redis_data`/`minio_data`.
- [x] Volume snapshot/restore drill, scripted next to the compose files: `pg_dump`, Redis RDB snapshot, `mc mirror` → restore onto fresh volumes with verified parity — the 📦 §2 migration runbook rehearsed before any data matters.
- [x] Flutter scaffold: `core/config` client config loader (server URL, feature flags) + `core/network` `GameTransport` abstraction and WebSocket implementation — connect / backoff-reconnect / clean-close contract tests plus an echo round-trip; builds for Android **and** Web PWA.
- [x] Localization foundation (Product Baseline): Flutter ARB/`intl` catalogs with locale negotiation and English-root fallback, ICU plurals + locale-aware number/date formatting, text-expansion-tolerant layout rules, supported-locale list in config (`configs/base.yaml → localization:`); the day-one wire rule — the server never sends display text, only stable ids/codes + parameters the client localizes; pseudo-locale CI gate failing on any hardcoded user-facing string.
- [x] `make` targets for build / test / lint of both stacks — `golangci-lint` + `gofmt` and `dart analyze` + `dart format` as gates — documented in `README.md`.
- [x] CI from day one (Product Baseline): lint + unit tests + client builds for Android, iOS, and Web + multi-arch server image per commit; app-size budget enforced in CI (packs stream, binaries stay lean).
- [x] Gate: Phase 1 proof tests pass on a clean tree.

---

### Phase 2 — Media Engine & Pipeline

**Scope id.** `phase-2`

**What.** Everything that decides what media exists and how hands are dealt:
the pack bundle format, the `tools/mediapack` CLI, the server-side pack
loader with precomputed relevance-band lists and the signed-URL issuer,
the dev-only Media Workbench, the first certified seed pack (**≥150 Nowns
/ ≥1,500 cards**), versioned CI fixture packs, and the client media module
(OTA metadata sync, prefetch, LRU cache, Donower placeholder renderer).

**Why.** Content is the game's fuel and the relevance mesh is the balance
mechanism — dealing must be provably feasible *before* the game loop
exists, or Phase 3 gets tested against toy data that lies. The Workbench
ships this early because the owner curates the AI generation pipeline alone,
before any community exists; certification is the anti-dead-content gate
that keeps unplayable media out of packs forever.

**How.** Embeddings in one shared multimodal space (local SigLIP/CLIP at
build time; Gemini Embedding 2 as the API lane); relevance bands over
cosine similarity with thresholds from `tuning.yaml → dealing:`; per-media
band candidate lists precomputed at pack build so runtime dealing is array
sampling (⚙️ §2); quality targets deliberately lo-fi (⚙️ §3); copyleft-first
sourcing with license + attribution stored in the pack manifest.

**Skills.** `test-driven-development` (write the certification gate against
a deliberately band-starved fixture pack first), `ai-output-stability`
(seeded, reproducible generation and dealing), `cost-aware-tool-use`
(local GPU first; API lane only for style-critical or overflow work).

**Spec (required reading).** ⚙️ §1–4 in full — bundle format,
relevance mesh, pipeline & content production (quality targets; the
**content standard: the humor line** — suggestive/erotic allowed as
cartoon/drawn/abstract, never pornographic, erotic-leaning media only in
age-gated packs; copyleft sourcing wells; the four-bucket tone rubric and
keep-rate expectations), secrecy & sync; 🧑‍🎨 §3 (Workbench); ⚙️ Tuning →
`dealing:`; 🎨 asset strategy (design system and pack content never mix).

**Proof tests.** `mediapack simulate` proves deal feasibility at both table
sizes and, re-run with the same seed, reproduces its report byte-for-byte;
certification rejects the band-starved fixture **and** a manifest-incomplete
pack; a tampered bundle is refused while the current pack keeps serving —
and a hot-swap fired during a running match leaves that match untouched; an
expired or cross-round signed URL is refused; a cold-start client syncs the
pack and prefetches a round's Nown inside the `prefetch_countdown` budget
on a mid-range phone, and still converges (retry/backoff behind the
placeholder) on a throttled, lossy network profile; rebuilding the seed
pack from its logged inputs reproduces identical bundle hashes.

- [x] Pack bundle format (⚙️ §1): `manifest.json` (pack tag, **format version**, checksums, license + attribution per asset, age rating, **BCP 47 language tag**, **pinned embedding model + version**), `media.jsonl`, `cards.jsonl`, content-hash-addressed assets in object storage — packs are language-scoped, so new languages ship as new packs, never format changes.
- [x] `tools/mediapack` stages: `ingest` (transcode, EXIF strip, perceptual-hash dedupe) → `screen` (provider-swappable automated moderation) → `tag` + `embed` → `certify` → `bundle` → `publish` → `simulate` (deal feasibility **and** offline balance questions: band-threshold sweeps, Donower-survival proxies, Shuffle/Revote impact, expected per-match Noin); every stage seeded and deterministic — same inputs + seed reproduce byte-identical bundles — with meaningful non-zero exits for CI use.
- [x] `server/internal/media`: in-memory pack loader with checksum verification (a tampered bundle is refused and the current pack keeps serving), between-matches hot-swap that never blocks a live room, precomputed per-media band candidate lists (zero embedding math in the hot path), and the signed-URL issuer — short-lived, single-round, expiry enforced server-side.
- [x] Certification gate: full band coverage per Nown at 6 players, every card reachable in some band, Monte Carlo deal feasibility at both sizes, **manifest completeness** (license, attribution, age rating, language tag) and one consistent embedding model per pack — TDD'd against a band-starved fixture pack.
- [x] Versioned fixture packs committed for CI (a tiny golden pack + the band-starved pack): the test fuel every later phase reuses — gamebot matches, load tests, client cache tests, compose dev seeding.
- [x] Media Workbench (server-rendered, dev-only): ingest-folder watch (ComfyUI / Ollama output), bulk keep/kill grid with tone buckets + per-batch keep-rate, embedding nearest-neighbor sanity view, deal simulator.
- [x] Seed pack on the local GPU: **≥150 certified Nowns, ≥1,500 cards** at ⚙️ §3 quality targets, curated to the ⚙️ §3 content standard (suggestive/erotic allowed — cartoon, drawn, abstract — **never pornographic**; erotic-leaning media only in age-gated packs) with every asset tagged into the four-bucket tone rubric for pack-mix balancing; every generation run logged with model, seed, and params (reproducible curation input); license + attribution recorded for copyleft-sourced media; tone rubric landed in `content/tone-matrix.md`.
- [x] `client/lib/media`: pack metadata OTA sync (app start + unrecognized tag), signed-URL prefetch with retry/backoff on flaky networks (URLs short-lived, single-round), hash-verified LRU asset cache under an explicit size budget (corrupt entries evicted, never rendered), Donower placeholder renderer — also shown while a Nower's asset is still loading, so loading state leaks nothing (⚙️ §4).
- [x] Gate: Phase 2 proof tests pass on a clean tree.

---

### Phase 3 — Realtime Game Loop

**Scope id.** `phase-3`

**What.** The heart of the product: room lifecycle, the versioned
intent/event protocol, reconnect snapshots, **role-scoped payload
rendering**, the full phase state machine (turn-based play → discussion →
Knowoff → verdict) with all five specialties and every fairness rule,
`tools/gamebot`, the seeded match-replay harness, the Soft Neo-Brutalism
design system (🎨), and the Flutter match screens on native + PWA.

**Why.** Role secrecy is enforced here, and it is built and reviewed
**first**: if Nown ever crosses the wire to the wrong device, nothing
downstream can fix it. `gamebot` comes immediately after because every
Phase 3 test depends on scripted, seeded matches — solo development against
five bots is the daily loop.

**How.** Intents/events per 🌐 (versioned JSON, sequence numbers, ordered
replay); server-owned phase clock with display-only client timers and
rejected late intents; blind-simultaneous ballots; per-round randomized
role-blind turn order with immediate attributed reveals; every rules
constant read from `tuning.yaml`; failing matches replay exactly from their
seed; the 🎨 kit (tokens, structure primitives, painters) lands before any
match screen consumes it.

**Skills.** `security-by-default` (role-scoped rendering is the critical
surface), `code-review` (secrecy code reviewed before anything builds on
it), `systematic-debugging` (seeded replay of failing matches),
`flaky-test-triage` (timers + concurrency are flake bait).

**Spec (required reading).** 🕹️ Game Rules §1–8 in full — the
normative behavior spec this phase implements; 🌐 (protocol + wire
discipline); ⚙️ §4 (secrecy & anti-cheat); 🤖 (dev/test bots); 🎨 (the
full design matrix); ⚙️ Tuning → `game/timers/hand/points`.

**Proof tests.** *Secrecy:* a protocol-level scanner over every event
stream — both table sizes, every phase, reconnect snapshots, eliminated
spectators, verdict — proves **no Donower or eliminated connection ever
received a Nown id or URL**; a still-loading Nower renders the identical
placeholder (loading parity). *Rules:* out-of-turn, off-role,
duplicate-unique, and late intents are rejected without state change and
the turn order re-randomizes every round; a missed vote eliminates the
Nower it names, whose later intents are rejected while stay-to-the-end
points still pay; a 6-player match with two missed votes auto-ends after
the second Knowoff as a Donower win; a Revote nullifies the shown result
and its survival credit; a still-tied runoff counts as one survived voting
for Donowers; pile draws deduct `points.draw_penalty` each, a One More
Free Card draw deducts nothing, and a match's net floors at 0; a fully
absent Donower team forfeits after exactly the grace period.
*Reliability:* scripted bot matches complete at both sizes with forced
mid-round disconnects/reconnects and no state corruption; the same seed +
intent script replays a byte-identical event stream; a 100-match soak on
one node returns memory to baseline (rooms reclaimed); `kill -9` mid-match
loses only the live match — durable stores stay consistent and clients
fail clean to the menu. *Design gates:* token/primitive/press-motion
goldens, the guardrail audit (zero blur, single permitted gradient, no
stacked translucency), the diacritics render check, the pseudo-locale +
text-expansion sweep of the match flow, and the low-end frame-budget trace
on Round and Knowoff.

- [ ] Role-scoped payload rendering through one choke point — a single payload-renderer module every outbound event must pass through: Nower → `{nown: {id, signed_url, type}}`, Donower/eliminated → `{decoy: true}` — built first, security-reviewed before dependents land, with the event-stream leak scanner in CI from day one.
- [ ] Room lifecycle: exactly 4/6 seats, 6-character codes + QR deep links (into the native app if installed, the PWA otherwise — a guest is never blocked), seat reservation, 20 s reconnect grace, session-token snapshot rejoin (role-scoped); session scoreboard across a room's matches; room→node affinity (Redis `room_id → node`, no cross-node game state); finished rooms torn down and memory reclaimed — no leaked room state (soak-proven).
- [ ] Versioned JSON protocol with sequence numbers — the full intent/event set from 🌐 defined once, so the wire format stays stable across phases (later-phase intents like `convert_points` and `report_media` parse and reject cleanly as unavailable until their backends land); version handshake rejecting unknown protocol versions with an explicit error; unknown, malformed, or oversized frames rejected without state change; client-detected sequence gaps trigger a snapshot resync.
- [ ] `tools/gamebot`: N seeded policy bots over the real WebSocket protocol, no server backdoors; a 6-seat dev match crosses every phase in under a minute; external bot connections refused whenever the `bots:` config block is absent — and it never exists in `prod.yaml`; doubles as the Phase 4 load-test engine.
- [ ] Seeded match-replay harness: every match logs its seed + intent script; a replay run reproduces the byte-identical event stream — failing matches replay exactly, and failing seeds are committed as regression fixtures.
- [ ] Phase state machine: role assignment, role-blind constraint dealing, randomized per-round turn order, 15 s turns with immediate attributed reveals (played cards stay on the table all match — the evidence votes are argued over), table-announced penalized pile draws (who, how many), timeout auto-pass + random card loss.
- [ ] Specialties (Rules §5): Pass / full-hand Reveal / penalty-free One More Free Card / anonymous once-per-match Shuffle (Donower-use-only, round-start only, draw piles untouched) / attributed once-per-match Revote (Nower-use-only, result window) — off-role and duplicate uses rejected (dead cards remain usable as discard fodder); Type A plays cost one extra discard (except Pass); Shuffle re-deals against the current Nown schedule so the dealing guarantee survives.
- [ ] Discussion window (`10 s × players`, Ready fast-forward) with the localizable canned Quick Chat catalog; Poke once per target per round (buzz on native, screen shake on the PWA — no vibration API; pokes show who poked whom, no score effect), cap enforced server-side.
- [ ] Knowoff: 20 s blind ballot — one vote each, never for yourself → tie runoff → 15 s result window → elimination + role reveal (these windows always run full time — Ready never shortens them); early-end rule (votes remaining < uncaught Donowers); still-tied runoff = survived voting for Donowers; eliminated-spectator scoping (no Nown, no actions); verdict screen reveals all Nowns to everyone.
- [ ] Disconnect handling per Rules §7: auto-played seats (turns pass instantly, abstain from votes, count Ready, stay votable), 20 s grace, team forfeits + scored low-population ending; match points per Rules §6 with the zero floor (absent at match end = 0 points; already-earned Noin stays).
- [ ] Design tokens (🎨): the palette as Flutter constants — `canvas #DCC8F7` lavender field with its faint low-contrast grid tile (`CustomPainter`, no raster), `surface #F7F2E9` warm cream for cards and sheets, `#FFFFFF` content wells inside them, `ink #141414` for every border and every glyph (text is never gray-on-gray), `violet #B49AF5` the neutral interactive (buttons, selected tiles, timers, progress fills), `lime #D4F04C` the truth/reward signal (Nower catches, match points, Noin grants), `pink #FF9ED2` the risk/accusation signal (votes, the Knowoff board, Donower reveals); the palette's single permitted gradient `#FFD9EC → #FF9ED2` reserved for the Knowoff reveal header; light theme only at v1 — token values snapshot-tested so silent drift fails CI.
- [ ] Brutalist structure primitives: shared container/button/chip widgets — every container carries `Border.all(width: 3, color: ink)` + the hard shadow `BoxShadow(color: ink, offset: Offset(4, 4), blurRadius: 0)`; corner radius 16 for cards and sheets, 12 for buttons, full pill for stat chips; the signature brutalist click — pressing collapses the shadow to zero offset while the control translates onto its own shadow footprint — golden tests per primitive, widget test on the press motion.
- [ ] Typography + highlighter emphasis: chunky rounded display face for headings, timers, and Noin numbers — Baloo 2 vs Fredoka (both OFL), locked by a diacritics render check across launch locales and recorded as an ADR; plain geometric sans for body; **the lime marker sweep is the house emphasis** (`CustomPainter`) for the revealed role, Noin deltas, and the clip caption — never bold-only.
- [ ] Fixed color semantics, enforced at the API level: violet = interact, lime = truth/reward, pink = accuse/risk, ink = information; no verdict leans on hue alone — verdict-bearing widgets require icon + label parameters so a color-only state cannot compile (colorblind-safe by construction).
- [ ] Illustration policy — deliberately sparse: no mascot, no scene art in the match flow; the ~12-glyph single-weight doodle set (sparkle, static-burst, eye, cloud, the Donower placeholder glyphs) shipped as hand-authored SVG paths, reserved for empty states, win moments, and the Donower-side placeholder.
- [ ] Performance guardrails + asset discipline: flat fills (the reveal-header gradient is the one exception), zero blur radii, no stacked translucency — enforced by a widget-tree guardrail audit test that fails on any blur, second gradient, or translucency stack, plus a frame-budget trace on a low-end device profile for the busiest screens (Round, Knowoff); UI chrome 100 % widgets/`CustomPainter` — code first, raster last (true raster arrives only via the curated nano banana batches in Phases 4–5); the design system and media-pack content never mix.
- [ ] Flutter match flow on native + PWA against the live protocol: MainMenu, Queue, Lobby, Round, Discussion, Knowoff, Verdict — `RoleCard` (press-and-hold role check), `NownStage`, `HandFan`, `PlayTable`, `VoteBoard`, `QuickChatBar`, `ReadyButton`, `PokeNudge`; every string through the localization catalogs (the wire stays ids/codes only), with a pseudo-locale sweep + text-expansion check across the match flow.
- [ ] Gate: full Phase 3 proof tests pass, including the no-leak protocol assertion, plus the 📦 §4 criterion: a full 6-player match — four `gamebot` seats + one native client + one PWA client — playable against the local stack with zero cloud dependencies.

---

### Phase 4 — Accounts, Quick Play & Hardening

**Scope id.** `phase-4`

**What.** Turning a game loop into a service: accounts (anonymous-first with
Google / Facebook one-tap linking), public profiles, stats and the XP
progression track, Quick Play queues with labeled backfill bots, a hardened
intent pipeline feeding the append-only audit stream, the Weekly
Leaderboard, and the public beta ingress behind Cloudflare.

**Why.** This is the trust phase — real strangers, real load, real abuse.
The audit stream doubles as the analytics source (no third-party client
SDK), so hardening and observability are the same work done once. Backfill
bots make queues viable before critical mass without ever becoming a
disguised opponent or a Noin farm.

**How.** JWT sessions binding anonymous device tokens to accounts; OAuth
per the Product Baseline (provider subject id + email stored privately
only); `server/internal/bots` reusing the gamebot policy engine in-process
— 🤖-labeled, human seat priority, sunset per queue on sustained healthy
fill times; leaderboard as pure PostgreSQL aggregation over the audit
stream.

**Skills.** `security-by-default` (auth, forged intents, rate limits — the
OWASP phase), `flaky-test-triage` (load and concurrency tests),
`dependency-upgrade` (OAuth/JWT libraries vetted and pinned).

**Spec (required reading).** 🎮 §1 + §5 (Quick Play, leaderboard);
👤 §1–2 (stats, avatars); 🤖 (backfill bots); 📦 §2 (ingress + edge
cache); Product Baseline (compliance, auth, analytics); ⚙️ §4 (audit
stream); ⚙️ Tuning → `liquidity/liveops`.

**Proof tests.** A deliberately modified client (forged plays, late votes,
replayed messages, out-of-protocol frames, a Donower requesting Nown
assets) alters nothing and leaves audit trails; an expired or replayed JWT
is refused, and banning an account drops its live connection within
seconds; a short queue backfills with labeled bots after `queue_timeout_s`,
never starts below `min_humans`, and bot seats earn no Noin, XP, or
leaderboard entries; a `gamebot` load test sustains hundreds of concurrent
rooms on one node — with real MinIO prefetch traffic — inside a stated p95
intent→event latency budget and with zero corrupted matches; a sign-in
restores progress, Noin, and entitlements on a second device; a re-run
nightly job reproduces identical stats and standings (idempotency); killing
and restoring Redis mid-queue leaves a healthy, leak-free process; an idle
match survives 10+ minutes through the tunnel on heartbeats.

- [ ] Auth: anonymous device accounts → JWT sessions with expiry + refresh and server-side revocation (a ban invalidates tokens and drops live connections within seconds); Google Sign-In / Facebook Login one-tap registration + account linking via OAuth 2.0 / OIDC with PKCE and state validation (provider subject id + email stored privately, never shown; a subject already linked elsewhere fails with a clear, safe error).
- [ ] Profiles + public stats (👤 §1) derived nightly from the audit stream — pseudonymous, no PII on any public surface; Non-Converted Points visible to the owner only; locale-aware nickname profanity filter; free preset avatar gallery (👤 §2) — the first curated **nano banana (Gemini image) raster batch** per the 🎨 asset strategy: prompts derived from the design matrix, candidates → human curation → consistency pass → committed like any asset; API keys under the 📦 §3 config discipline.
- [ ] XP progression (Product Baseline): one server-side track (matches completed, correct votes, Donower survivals); levels gate portal role applications and cosmetic unlocks — values in `tuning.yaml`.
- [ ] Quick Play FIFO queues per room size (core pack + rotating featured pack) with reconnect-safe seat reservation and escalating abandon cooldowns.
- [ ] Backfill bots (`server/internal/bots`): 🤖 badge + reserved nicknames, human seat priority, `min_humans` floor, per-match randomized personality parameters (no farmable tell), per-queue sunset by fill-time measurement, economy + leaderboard guardrails (🎮 §1 — bot seats earn nothing).
- [ ] Beta ingress (📦 §2): `cloudflared` publishes `play.<domain>` (WebSockets) + `cdn.<domain>` (assets) — no open ports, no exposed home IP, TLS at the edge; WebSocket heartbeat interval below the edge idle timeout, verified end-to-end through the tunnel (no silent mid-match drops); Cache-Everything + long-TTL rule on content-hashed assets so the edge absorbs media traffic.
- [ ] Intent-pipeline hardening: per-connection + per-account rate limiting (Redis), deadline enforcement, protocol-boundary validation, frame-size caps, slow-consumer disconnect policy (one stalled client never blocks a room), structured audit log to PostgreSQL as **the** analytics event stream — schema-versioned from the first event.
- [ ] Weekly Leaderboard (🎮 §5): Quick Play only, Monday–Sunday on the server clock, `leaderboard_min_humans` + daily counted cap, top-100 + own rank (ties share a rank), immutable weekly history; nightly stats/KPI jobs idempotent and re-runnable — a crashed or repeated job never double-counts.
- [ ] Dependency-degradation drills: losing Redis or Postgres flips `/readyz` and pauses matchmaking with a clear client message while the process stays healthy; service resumes without restart when the store returns; no goroutine or connection leak across the outage (metrics-proven).
- [ ] Client surfaces on native + PWA: Profile (public stats, owner-only Non-Converted Points) and Leaderboard screens, themed per the design system.
- [ ] Gate: full Phase 4 proof tests pass (forged-client suite, backfill behavior, quantified load test, OAuth second-device restore, leaderboard guards).

---

### Phase 5 — Noin Economy, Admin & Launch Polish

**Scope id.** `phase-5`

**What.** The business and the ops board: the full Noin economy (wallet,
append-only ledger, instant grants, points conversion, Play Passes, the
Premium subscription, Noin bulks, SSV ad doubler, theme packs, cosmetics),
the player-facing trust surfaces (custom avatars, reports, feedback, the
notice inbox), the Admin Console, System Notices with the maintenance
drain, the how-to-play clip, the economy balance pass, and the launch
performance passes + VPS migration.

**Why.** Money and moderation must be boring and correct before strangers
pay: every Noin movement is a server-ledger event, ad rewards grant only
via server-side verification, and nothing purchasable touches gameplay.
The Admin Console must exist *before* public launch — moderation debt
can't be paid retroactively. The clip is ship-gated on final UI by design.

**How.** `server/internal/economy` per 💰: append-only ledger, debits
atomic with entitlement writes, balances server-side only, all prices in
`tuning.yaml`; Admin Console server-rendered on the internal admin port
(2FA, RBAC, CSRF, append-only audit trail — 🛡️); economy tuned against the
balance-protocol table (targets first, numbers second — ⚙️ Tuning).

**Skills.** `security-by-default` (payments, ledger, admin auth),
`release-checklist` + `changelog-discipline` (store submission),
`incident-postmortem` (rehearse maintenance + migration runbooks before
they're needed in anger).

**Spec (required reading).** 💰 in full; Rules §6 (points & Noin);
⚙️ Tuning (economy keys + the balance-protocol table); 🎮 §4 (notices +
maintenance drain); 👤 §2–4 (avatars, reports, feedback); 🛡️ (Admin
Console); 🎬 (clip); 📦 §2 (VPS migration).

**Proof tests.** The Phase 5 proof gate — headline: Noin
grants land instantly and survive a mid-match disconnect; points
conversion is atomic, one-way, and cap-counted, rejected below 100 points,
never touches Overall Points, and parallel conversions of the same balance
credit exactly once; the ledger admits no update or delete, every debit is
atomic with its entitlement write, and after a fuzzed storm of grants,
spends, and conversions every balance equals its ledger sum; a replayed
SSV callback or store receipt grants exactly once; a bot-heavy match under
`noin_min_humans` grants no team-win Noin; the 4th free Quick Play match
of the day is rejected at queue time while a Local Room still opens — and
the cap never rejects a Play Pass holder or Premium subscriber; a
Play Pass uncaps Quick Play, expires on schedule, re-gates correctly —
and never removes ads; ads disappear only under an active Premium
subscription (which also uncaps Quick Play), with the yearly price
carrying the 20% discount; a scheduled maintenance window notifies at
T−24 h / T−1 h / T−10 min, drains, and never kills a live match; a notice
authored in several locales renders in each client's locale with English
fallback; takedowns propagate in the next pack version and invalidate at
the edge; the Admin Console refuses sessions without 2FA, rejects requests
without a CSRF token, and is unreachable from the public ingress; the
migration rehearsal restores onto a fresh host with verified parity.

- [ ] `server/internal/economy`: Noin wallet + append-only ledger — append-only enforced at the database level (no update/delete path on ledger rows), every balance always the replayable sum of its ledger with a nightly reconciliation job proving it and alerting on drift; instant per-event grants with `daily_earn_cap` and the team-win guard — no team-win Noin below `liquidity.noin_min_humans` humans (🎮 §1); property tests — no sequence of grants, spends, and conversions can go negative or double-credit; discreet crediting (no public per-match Noin surface — Rules §6).
- [ ] Overall / Non-Converted Points accrual + points→Noin conversion: 100:1, multiples of 100, one-way, atomic under concurrency (parallel conversions can never double-credit), cap-counted, Overall Points untouched.
- [ ] Play Passes (1/3/7-day, priced in Noin — `economy.play_pass_prices`): **unlimited Quick Play while active**, lifting the free daily cap (`economy.free_daily_quickplay_matches`, 3 at launch — a daily taster; Local Rooms never capped) — **Play Passes never remove ads**; **Premium** — the one cash subscription, monthly / yearly (yearly −20%, `premium_yearly_discount_pct`) via platform billing — is the **sole ad-removal path and includes unlimited Quick Play**, so a subscriber never needs passes and **never hits the daily cap**.
- [ ] Noin bulks (`economy.noin_bundles`) via platform billing — **the only place money buys Noin: everything money can get, play can also get, slower** — with server-side receipt verification and idempotent grants keyed by platform transaction id (a replayed receipt grants exactly once; refunds/chargebacks revoke via an explicit audited admin action); SSV rewarded post-match doubler — callbacks signature-verified and replay-proof, the client callback grants nothing; Premium subscribers get the doubling automatically, ad-free; theme packs (`economy.unlock_prices.theme_pack`) with Host Pass enforcement — only the room creator needs the pack in private/local rooms, Quick Play runs core + free rotating featured pack; Poke Styles + Custom Avatar unlocks priced in Noin (`economy.unlock_prices`).
- [ ] Economy balance pass (⚙️ Tuning): every price and earn value read from `tuning.yaml` only — economy tuning never needs a client release (proof: a config price change reflects in the Store with no rebuild); numbers tuned against the balance-protocol table, targets first — expected per-match Noin from `mediapack simulate` vs the real numbers from the nightly KPI jobs, one lever at a time.
- [ ] Custom Avatar upload pipeline (👤 §2): server-side crop to 256×256 WebP, EXIF strip, size cap, automated moderation screen before display, admin takedown reverting to presets without refund.
- [ ] Player reports (👤 §3) + feedback (👤 §4): one-tap conduct/media reports, rate-limited, repeat reports collapsing into one case, feeding the case queues; in-app feedback form with consented context snapshot into Postgres triage.
- [ ] Client surfaces on native + PWA: Store (`NoinBadge`, bulks, passes, packs, cosmetics), NoticeInbox + dismissible notice banners (`system_notice` live + HTTPS fetch on start, hard-maintenance countdown), post-match SSV doubler flow.
- [ ] Admin Console (🛡️) on the internal port — 2FA-gated, RBAC-scoped, CSRF-protected, unreachable through the public ingress, every action writing an append-only audit row: conduct + media case queues (bans hit live connections immediately), Guard-freeze reviews, pack dashboard, leaderboard ops, economy ledger, feedback triage, system-notice composer — compose, schedule, localize, withdraw — with automatic matchmaking drain (🎮 §4).
- [ ] How-to-play clip (≤45 s, 7 beats, captions on the lime highlighter sweep per 🎨) captured on final production UI — ship gate; player-facing help text + store copy derived from the 🕹️ Game Rules chapter (deliberately the only rulebook).
- [ ] Launch passes: low-end client paint budget, server allocation/GC under queue load, edge-cache hit rates on pack release, **backup/restore + VPS migration runbook executed with verified data parity** (row counts + checksums across Postgres/Redis/MinIO — 📦 §2; Cloudflare R2 free tier as the asset-offload option), store review prep (age gate, per-pack age ratings, UMP consent, privacy notice at first launch, in-app delete-my-data, **store listings + clip captions localized for every launch locale**); app icon + store art via the curated nano banana raster-batch pipeline (🎨 asset strategy: matrix-derived prompts → human curation → consistency pass).
- [ ] Gate: full Phase 5 proof tests pass on a clean tree.

---

### Phase 6 — Contributor Portal & Community

**Scope id.** `phase-6`

**What.** Opening the content machine to the community: the Contributor
Portal (promoted from the Workbench codebase) with role applications
(Contributor / Curator / Guard), the Curator Guide + deal simulator, the
audited submission pipeline with consent capture, and the Weekly Nown
Challenge.

**Why.** The content pipeline is the game's long-term moat and the
community is its scale plan — but only behind screening-before-visibility,
immutability, full auditing, and admin-final enforcement. This ships last
because every rail it needs (economy rewards, Admin Console, moderation
queues) lands in Phases 4–5.

**How.** The same Go-server-rendered web app as the Workbench, now public
and role-gated (🧑‍🎨 §4); workflow states
`draft → submitted → in_review → approved | rejected → published(pack-tag)`
with every transition audited; challenge per 🎮 §3 (first-100 intake with
rejection-reopened slots, screening before visibility, one immutable vote,
atomic weekly close).

**Skills.** `security-by-default` (public UGC upload is the maximum attack
surface), `code-review` (permission boundaries per role),
`documentation-first` (the Curator Guide is itself a deliverable, derived
from ⚙️ §2–3).

**Spec (required reading).** 🧑‍🎨 in full (roles, submission
pipeline, portal architecture); 🎮 §3 (challenge); 🛡️ (portal
administration); ⚙️ §2–3 (the Curator Guide's source material); ⚙️
Tuning → `portal/liveops`.

**Proof tests.** The Phase 6 proof gate: audited immutable
submission lifecycle; two entries racing for slot #100 admit exactly one,
and entry #101 is rejected until a screening rejection reopens a slot;
unscreened entries are never visible or votable; second votes, vote
changes, and concurrent double-votes are rejected; oversized, mis-typed, or
decompression-bomb uploads are rejected safely and the per-day submission
cap holds under parallel load; Guard freezes suspend within seconds,
auto-expire at `guard_freeze_max_h` even across a server restart, and only
an admin converts them to bans; the weekly close is atomic and idempotent —
crashed mid-run and re-run, it still yields exactly one Week Winner title,
one payout, and one transfer at next close; admin routes are unreachable
from the portal ingress.

- [ ] Promote Workbench → public Contributor Portal: role applications gated by `portal.min_account_level_to_apply` with admin grants (Contributor / Curator / Guard), every role action audited and reversible; player-account sessions + role claims on the public subdomain, strictly separated from the Admin Console's internal port (route separation proven by test).
- [ ] Curator toolchain: Curator Guide (derived from ⚙️ §2–3), Nown + deck authoring with the deal simulator; submission pipeline with terms-consent capture (version + timestamp), the `submissions_per_contributor_per_day` cap, and upload hardening (size/type caps, transcode-on-ingest, automated screen before any human review) — credits + Noin rewards on acceptance; submissions immutable once submitted; withdraw + resubmit is the only correction path and it costs the queue slot.
- [ ] Guard freeze flows wired to the Admin Console case queue: timeboxed ≤ `guard_freeze_max_h`, one active freeze per Guard per target, auto-expiry surviving a server restart, admin-final dismiss / timed ban / permanent ban.
- [ ] Weekly Nown Challenge end-to-end (🎮 §3): Monday topic publication, in-app entries (first-100 cap race-proof under concurrency, rejection-reopened slots, immutable once submitted, per-entry consent stored with terms version + timestamp), pre-vote screening queue, open live tallies with one immutable vote and no self-votes, atomic **and idempotent** weekly close (Week Winner title + `challenge_winner` payout + optional community-pack inclusion, transferred at next close); challenge scheduler + contribution-terms versioning land in the Admin Console (🛡️).
- [ ] Gate: full Phase 6 proof tests pass on a clean tree.

---

### Appendix A — Tracking conventions

- One `run_id` per implementation pass through a phase.
- `scope` column on every row = the phase id (e.g. `phase-3`).
- One `action=commit, status=completed, commit_sha=pending` row per logical
  commit, with the `summary` in Conventional Commits format. See
  [`docs/tracking/tracking.schema.md`](../tracking/tracking.schema.md).
- A spec-chapter change and its roadmap-chapter update land in the **same
  commit** — the lockstep rule, now enforced by construction: one document.

### Appendix B — Definition of done

A phase is **done** when:

1. Every `[ ]` bullet under its heading is `[x]`.
2. The phase's *Proof tests* pass on a clean tree.
3. `make doctor` exits 0.
4. The status snapshot at the top of this chapter has been updated.
5. The phase's run produced one or more `commit` tracking rows whose
   `[run-id]` trailers all appear in `git log`.
6. This chapter still matches the spec chapters above — any drift
   discovered during the phase was resolved in the same pass (lockstep
   rule, Appendix A).
7. Every proof in the phase's *Proof tests* exists as an automated test
   where automatable; manual drills (device traces, migration rehearsals,
   store submissions) are recorded as `action=note` tracking rows with
   their evidence.

