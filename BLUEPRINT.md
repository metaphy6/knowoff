# 📖 Knowoff — Project Architecture & Implementation Roadmap

## 🎲 The Game at a Glance

Knowoff is an online social deduction party game for **exactly 4 or 6 players** — matched with people around the world in **Quick Play** (the main product), or gathered in person in a **Local Room**. Each round, one media item — **Known** — appears on every phone except the Offs'; Offs must pretend they see it. Everyone plays one card that supposedly relates to Known, the table argues, then votes. A vote reveals the chosen player's role: an Off is caught and out; a Knower stays, and the vote is wasted. **Knowers win by catching every Off. Offs win — together — if the votes run out first.**

There is deliberately no separate rulebook: the **Game Rules** section below is the single source of truth, and all player-facing help text, store copy, and the how-to-play clip are derived from it.

### Terminology (normative — written without "the")

| Term | Meaning |
|---|---|
| **Knower(s)** | Players who see Known |
| **Off(s)** | Players who can't see Known; nobody knows who they are |
| **Known** | The media item of a round — image, GIF, or text (no audio or video at v1) |
| **Unknown** | The neutral placeholder an Off's screen shows instead of Known |
| **Knowoff** | The vote at the end of every round |
| **Round** | Card play + discussion + one Knowoff |
| **Match** | Up to 2 votings at 4 players, up to 3 at 6 — until a team wins |
| **Session** | Matches played in one room; keeps a running scoreboard |
| **Knoin** | The game currency — earned by playing, sold in bulks (💰) |

---

## 🧱 Tech Stack

| Layer | Technology | Role |
|---|---|---|
| Client | Flutter — one codebase: native Android/iOS apps + Flutter Web **PWA** (desktop, and app-less guest fallback on any phone browser) | UI, WebSocket client, client `MediaEngine` (pack metadata sync, asset prefetch & cache, Unknown renderer) |
| Backend | Go | Authoritative game server: matchmaking, rooms, timers, roles, media dealing, votes, scoring, economy — plus the Admin Console and Contributor Portal (server-rendered) |
| Database | PostgreSQL | Durable data: profiles, match results, Knoin wallet & ledger, entitlements, leaderboards, media metadata & contribution workflow |
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

* A room holds **exactly 4 players (1 Off) or exactly 6 players (2 Offs)** — no other sizes. A match may run short-handed only because someone disconnected or left (§7).
* A match is a series of rounds; **every round ends with a Knowoff vote**. The vote budget is fixed: **2 votings at 4 players, 3 votings at 6 players**.
* What a vote does: the player with the most votes has their role revealed. **If they're an Off, they're caught** — out of play for the rest of the match. **If they're a Knower, they stay seated and keep playing** — the vote was wasted, and that player is now publicly cleared.
* **Knowers win** the moment every Off is caught.
* **Offs win** the moment the remaining votes can't catch the remaining Offs. Concretely:
  * 4 players: Off survives both votings → Offs win.
  * 6 players: **if neither of the first two votings catches an Off, the match ends right after the second Knowoff — one vote can't catch two Offs — and Offs win.** Otherwise it runs to the third voting, and any Off still uncaught after it wins.
* **Offs win and lose together**: a caught Off still wins if their partner survives. Offs don't know each other at match start; quietly working out who your partner is, and covering for them, is intended strategy.
* One voting per match can be re-run by a Revote card (§5).
* Match length: roughly **4 minutes** at 4 players, **7 minutes** at 6. Ready (§8) can only shorten a match.

### 2. Roles, Known & Hands

* The server assigns roles randomly and secretly at match start. Players check their role privately: **press and hold to show it, release to hide it** — everyone performs the same check, so nothing about it stands out.
* **Known**: one item per round from the room's media pack — **image, GIF, or text**. No audio and no video at v1 (assets stay tiny, rounds are silent-autoplay-safe, and in local rooms sound would leak to Offs; v2 may revisit).
* **Unknown**: Offs simply don't see Known — their screen shows a neutral placeholder in the same layout. In a local room this also means a glance at a neighbor's phone reveals nothing.
* Secrecy is enforced server-side: an Off's device is never sent Known at all (⚙️ §4). Nobody knows who is Off or Knower until votes reveal roles.
* **Hands**: every player gets **5 cards** plus a personal **3-card draw pile**. Cards are prompts — **text, image, or GIF** — dealt by the relevance mesh (⚙️ §2) so every hand always holds a mix of strong, stretchy, and garbage options against every Known in the match. That guaranteed ambiguity is what lets Offs blend in and makes Knowers doubt each other.
* A match can never use more than those 8 cards, so a player always has a card to play.

### 3. The Round: Blind Play

* Play window (`10 s × players`; ends early when everyone is Ready): each player secretly locks **one action** — play a card, or use a specialty (§5). Nothing shows while the window runs; when it closes, **all plays flip face-up at once, with names attached**. No speed races, no copying the table mid-round.
* Played cards stay on the table for the whole match — the evidence the votes are argued over.
* **Draws**: during a play window, a player may draw from their 3-card pile — all at once or in parts. **Every draw is announced to the table** (who, how many). Drawing tells everyone your hand doesn't fit.
* **Timeout**: a player who locks nothing auto-passes and loses one random card. Stalling costs.

### 4. Discussion & Knowoff (every round)

* After the reveal, a discussion window opens (`10 s × players`; ends early when everyone is Ready).
  * **Local rooms:** talk happens out loud at the table.
  * **Online rooms:** players argue through **Quick Chat** — a set of canned phrases and reactions ("I suspect P3", "my card fits, trust me", "that play was weird", emotes), sent as taps, localized automatically. **No free-text chat at v1** (see Product Baseline); free-text and voice are v2 candidates behind proper moderation.
* Then **Knowoff**: a 20-second blind ballot. Everyone votes for one player (never themselves); votes stay hidden until the window closes, then all votes are shown. The most-voted player's role is revealed (§1). A tie triggers one 15-second **runoff** among the tied players only; still tied → the vote is a miss (it counts as survived, reveals nobody).
* **Result window:** every vote's outcome is displayed for 15 seconds before it becomes final — this is the window where a Revote card (§5) can land. Then it applies.
* A caught Off watches the rest of the match; their screen no longer shows Known — a revealed Off can't feed it to a surviving partner — and they don't vote.
* After the match, the verdict screen shows all Knowns to everyone; Offs finally see what they survived.

### 5. Card Specialties

Five specialties in two types. Deal frequencies are tuned in `tuning.yaml`; role-restricted specialties are dealt after roles are assigned.

**Type A — Standard (your one action for the round; costs one extra discard, except Pass):**

* **Pass** (occasional): skip playing a card this round. Only the Pass card is spent.
* **Reveal** (rare): show another player's hand to the whole table. Their media cards show face-up; **their specialty cards show only as face-down backs** — so Reveal can never expose a role through a role-restricted card. Costs one extra discard.
* **One More Card** (occasional): usable any moment during a play window without spending your action — draw 1 fresh card from the mesh. Costs one extra discard.

**Type B — Unique (free, role-restricted, once per match):**

* **Shuffle** (rare — **Off only**): usable only at the very start of a round, before anyone locks an action. Every player's unplayed hand is returned and re-dealt fresh (draw piles untouched). The table is told *a Shuffle happened* — hands visibly change — but not who did it; since nobody knows who is Off, the alert exposes no one. It wipes out the plans Knowers built around saved cards.
* **Revote** (rare — **Knower only**): playable during the 15-second result window of any Knowoff, before the result finalizes. The shown result is **canceled: it reveals nobody, removes nobody, and does not count as a survived voting for Offs.** A fresh ballot runs immediately with the full time, and only its result counts. The table sees who played the card — it's Knower-only, so using it publicly half-clears you; that's the price. (This replaces the earlier One More Round card, which fed Knowers extra votes and broke balance.)
* **Unique cards fire once per match, total.** The same card can be dealt to two players (rare, since these cards are rare); only the first use works — later copies are dead cards, still usable as discard fodder.

### 6. Match Points & Knoin Earnings

Two separate rewards come out of every match:

**Match points** — the competitive score. They feed the session scoreboard (local rooms) and the Weekly Leaderboard (Quick Play only, 🎮 §5). A player who is absent when the match ends scores 0 points for it.

| Event | Points |
|---|---|
| Your vote names an Off | +1 |
| Knower team win | +1 each Knower |
| Off team win | +3 each Off — caught Offs included (they win together) |

**Knoin** — the currency (💰 §1). Knoin grants are **per-event and credited to the profile instantly**, so they survive disconnects and abandons: what you earned is yours the moment you earned it. Values in `tuning.yaml → knoin:`.

| Event | Knoin (v1 placeholders) |
|---|---|
| Match completed | +5 |
| Knower team win | +30 each Knower |
| Off team win | +50 each Off |
| Your vote names an Off | +5 |
| Surviving a voting as an Off | +10 |
| First win of the day | +25 |

No chips, no stakes, no in-match economy — Knoin and points are earned by the match, never wagered in it.

### 7. Disconnects, Abandons & Fairness

Nobody should profit from a dropped connection — theirs or anyone else's. Three rules cover every case:

1. **Grace & auto-play (20 s).** A disconnected seat is auto-played from the moment it drops: it passes, abstains from votes, counts as Ready, and can still be voted. The player has 20 seconds to reconnect before counting as absent; reconnecting later restores the seat mid-match (with a fresh role-scoped snapshot — a Knower gets Known back, an Off doesn't).
2. **A fully absent team forfeits.** If **all Offs** are absent past grace, the match ends immediately as a **Knower win**. If all Knowers are absent past grace, Offs win. Quitting is conceding, never escaping.
3. **Too few humans ends the match unscored.** If fewer than 3 players remain connected (and rule 2 hasn't already decided it), the match ends with no points and no team-win Knoin.

Backstops: an absent-at-end player scores 0 match points (already-earned Knoin stays — §6); roles never move between seats; no bot ever takes over a human's hand. Repeated abandoning in Quick Play earns escalating matchmaking cooldowns.

### 8. Pace Controls: Ready & Poke

* **Ready** — for every player, in every room type (4 or 6, local or online): locking an action (or having nothing left to do) marks you Ready, and when everyone is Ready the play and discussion windows end early. Fast tables play fast; the `10 s × players` timers are only the ceiling. **Ballots, runoffs, and result windows always run their full time** — blind and cancelable to the end.
* **Poke**: once per target per round, you may poke a player who hasn't locked an action. Their phone buzzes (native apps) and their screen shakes (everywhere — the web PWA has no vibration). Pokes show who poked whom. No score effect; the cap is enforced server-side.

---

## 🎮 Game Modes & Live Ops

### 1. Online Quick Play (the main product)

* One FIFO queue per room size (4 or 6), running on the core pack plus a rotating featured pack. Tap Play, get a table of strangers, argue in Quick Chat, vote.
* **Launch liquidity — labeled backfill bots:** when a queue can't fill a room within `liquidity.queue_timeout_s` (default 25 s), the server tops it up with bots — server-side (`server/internal/bots`, reusing the `gamebot` policy engine), every seat acting through the same validated intent pipeline as humans. Every bot seat carries a visible 🤖 badge and a reserved bot nickname — in a game about reading people, a disguised bot would be a scandal; a labeled one is a practice partner. Humans always outrank bots for seats, and a room never starts below `liquidity.min_humans`.
* Guardrails: bot seats earn nothing; matches count for the Weekly Leaderboard only with ≥ `liquidity.leaderboard_min_humans` humans, and grant team-win Knoin only with ≥ `liquidity.knoin_min_humans` humans — a bot table can never become a Knoin farm. Backfill sunsets per queue automatically once fill times stay healthy.
* Free accounts play `economy.free_daily_quickplay_matches` per server day (**10 at launch** — generous on purpose); Premium (💰 §2) removes the cap. **Local Rooms are never capped.**

### 2. Local Rooms

Private rooms for people in the same physical place: the host shares a QR code (or 6-character code); the QR deep-links into the native app if installed, the web PWA otherwise — a guest without the app is never blocked. Discussion happens out loud; everything else plays identically to Quick Play. Uncapped, always.

### 3. Weekly Known Challenge (Community Event)

A weekly community content contest, run through the Contributor Portal (🧑‍🎨):

* **Submissions:** the week's theme opens Monday. Anyone with portal access may submit **one entry** (image, GIF, or text). **Uploads are immutable — no edits, no replacements.** Submission capacity is **100 entries**: intake auto-closes at 100, and a slot reopens each time screening rejects an earlier entry.
* **Screening before visibility:** every entry passes the automated screen plus a human check (curators or admin) **before** it becomes publicly visible and votable. Rejected entries never appear.
* **Open voting:** all players may vote, one vote each, never for your own entry, and **votes are immutable** once cast. Tallies are public and live — open voting is the point.
* **Terms:** submitting requires explicit acceptance of the contribution terms — a perpetual, non-exclusive license permitting **commercial use and modification** of the entry. Consent (terms version + timestamp) is stored with the submission. No consent, no upload.
* **Monday close:** the winner (and curation picks) enter the community pack with **credits and Knoin rewards** (💰 §1), alongside the new week's theme.

### 4. Daily Known

One curated media item on app start — a taste of the game's humor, a dismissible card, never a gate. Scheduled via the Admin Console's editorial calendar; fetched once over HTTPS and cached.

### 5. Weekly Leaderboard

* Cycle: Monday–Sunday on the server clock; Monday announces last week's podium alongside the challenge winner.
* Score: the sum of a player's match points for the week — **Quick Play matches only** (private and local rooms are collusion-trivial and stay off the board), with backfilled matches counting only at ≥ `liquidity.leaderboard_min_humans` humans, and a daily cap on counted matches to blunt pure grind.
* Display: top 100 plus the viewer's own rank; ties share a rank. Every weekly close snapshots into an immutable history table; podium finishes show on profiles.
* Ops: Admin Console — standings inspection, cheat exclusion/reinstatement with reasons, manual re-run of the weekly close, history browsing.
* Infrastructure: pure PostgreSQL aggregation over the audit stream; no new stores.

---

## 👤 Profiles & Community

### 1. Public Player Statistics

* **Every profile is visible to anyone** — tap a name in a lobby, scoreboard, or the leaderboard: matches played and won per role, vote accuracy, Off survival rate, weekly podium finishes, pokes sent, contributor credits.
* Derived nightly from the server's audit-event stream — no client-reported numbers anywhere. Reputations are part of the metagame: a profile that survives most of its Off matches *should* scare the table.
* Privacy: profiles are pseudonymous (nickname + avatar, no PII); delete-my-data erases stats with the account.

### 2. Avatars (Free Presets, Paid Uploads)

* A curated preset gallery, free forever, on-brand by construction.
* A one-time **Custom Avatar** unlock (Knoin, 💰 §5) opens personal uploads: server-side crop to 256×256 WebP, EXIF strip, size cap, automated moderation screen before display, reportable forever, admin takedown reverts to presets without refund. Stored as small blobs in PostgreSQL.

### 3. Player Reports

* One tap from any profile or scoreboard: inappropriate avatar/nickname, harassment, cheating/collusion — plus **media reports** on any Known or card, which route to the curation queue instead of the conduct queue.
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
| **Contributor** (entry role) | Submit media to the Weekly Known Challenge and to open pack calls; view own submission history and Knoin rewards |
| **Curator** | Everything a Contributor can, plus: **create Knowns and the decks that relate to them** — authoring cards against a Known, testing hands in the deal simulator, checking band coverage; screen Weekly Challenge entries before they go public. Curators work from the **Curator Guide**, a clear rulebook derived from this blueprint (⚙️ §2–3): quality targets, tone rubric, band-coverage requirements, and how to test a Known before submitting it for certification |
| **Guard** | Community safety: review flagged accounts and **freeze** them — a timeboxed suspension (up to 48 h) from matchmaking and the portal, pending admin review. **Final action is always the admin's**: dismiss, timed ban, or permanent ban. One active freeze per Guard per target; freezes auto-expire if no admin acts |
| **Admin** | Everything: role grants, bans, pack publishing, challenge scheduling, takedowns |

* **Rewards:** contributors and curators earn **credits (name in the pack manifest and on the profile) and Knoin** for accepted work — `portal.knoin_per_accepted_asset` per published asset, a larger bonus for a Weekly Challenge win. Attribution is stored per asset, so richer reward schemes later are an economy change, not a migration.
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
* **Deal simulator:** for any candidate Known, render the hands the mesh would actually deal at both table sizes — the same tool Curators later use, per the Curator Guide.

### 4. Architecture

Server-rendered from the Go binary (same pattern as the Admin Console) on a public subdomain — no separate SPA build chain. Role and workflow state in PostgreSQL; binaries in object storage; the automated moderation screen is provider-swappable.

---

## 🛡️ Admin Console

One authenticated, browser-based operations board, served from the Go server on the internal admin port — never through the public ingress. Admin accounts in PostgreSQL with role-based access, 2FA, rate-limited sessions, CSRF protection, and an append-only audit trail (who, what, when, before/after). Every action is an audited server mutation through product code paths.

* **Reports & moderation:** the case queue — conduct cases (kick, ban account+device, close room, avatar takedown, Guard-freeze reviews) and media cases (asset takedown → next pack version, tag fixes). Bans hit live connections immediately.
* **Portal administration:** role applications (grant/revoke Contributor, Curator, Guard), the challenge scheduler and its screening queue, contribution-terms versioning.
* **Curation & packs:** pack dashboard — active tag per pack, checksum verification, hot-swap trigger, certification stats (⚙️ §3).
* **Leaderboard ops** (🎮 §5): exclusions, reinstatements, weekly close re-runs, history.
* **Economy & accounts:** Knoin ledger and entitlement lookups; grants/refunds are explicit audited actions.
* **Daily Known:** calendar editor with exact-render preview and hot-replace. **Feedback triage:** new / seen / done.
* Roadmap fit: endpoints ship with the features they manage; the console UI is a Phase 5 deliverable — in place before any public launch.

---

## 🎬 How-to-Play Clip (Ship-Gated)

A ≤45-second, watch-don't-read onboarding clip: a first-timer should follow their first match after one viewing.

* Format: real UI capture only; silent-autoplay friendly — big captions carry the story; one idea per beat; readable at phone size.
* Storyboard (7 beats, 4–6 s each):
  1. Hook — "One of you can't see this." A meme cuts to static on one phone among four.
  2. Roles — the press-and-hold role check; one card whispers *you're Off*.
  3. Known appears on every screen at once; an Off's screen shows Unknown — and nobody can tell.
  4. Play — four cards lock blind, flip face-up with names; caption "whose card doesn't get it?"
  5. Pressure — a draw announcement, a poke shake, a Shuffle alert, Quick Chat accusations flying.
  6. Knowoff — votes land; a role flips face-up… it's a Knower. The table groans; one vote left.
  7. Verdict — the Off grins; "Offs win together." Knoin rains onto the scoreboard. Logo out.
* Ship gate: produced on final production UI, released with prod — no clip work while gameplay, engine, and netcode remain open.

---

## 🎨 Visual Identity: Soft Neo-Brutalism Design Matrix

Direction locked: **pastel neo-brutalism, illustration-light** — the same design language as the example blueprint. A brutalist skeleton — thick ink borders, hard zero-blur shadows, chunky type, flat fills — wearing a soft candy palette; personality comes from tiles, type, and color, not mascots or scene art.

* Palette tokens (Flutter constants; light theme only at v1):
  * `canvas` `#DCC8F7` — lavender field with a faint low-contrast grid tile; `surface` `#F7F2E9` warm cream for cards and sheets; `#FFFFFF` content wells inside them.
  * `ink` `#141414` — every border and every glyph; text is never gray-on-gray.
  * `violet` `#B49AF5` — the neutral interactive: buttons, selected tiles, timers, progress fills.
  * `lime` `#D4F04C` — the truth/reward signal: Knower catches, match points, Knoin grants.
  * `pink` `#FF9ED2` — the risk/accusation signal: votes, the Knowoff board, Off reveals. The palette's single permitted gradient (`#FFD9EC → #FF9ED2`) is reserved for the Knowoff reveal header.
* Structure: every container carries `Border.all(width: 3, color: ink)` and a hard shadow `BoxShadow(color: ink, offset: Offset(4, 4), blurRadius: 0)`. Corners rounded — radius 16 for cards and sheets, 12 for buttons, full pill for stat chips. Pressing a control collapses its shadow to zero offset while the control translates onto its own shadow footprint: the signature brutalist click.
* Typography: a chunky rounded display face for headings, timers, and Knoin numbers (Baloo 2 / Fredoka class — both OFL; lock one after a diacritics render check), a plain geometric sans for body. **Highlighter emphasis is the house style:** the revealed role, a Knoin delta, the clip caption — key phrases sit on a lime marker sweep, not bold-only.
* Illustration policy — deliberately sparse: no mascot, no scene art in the match flow. One tiny single-weight doodle glyph set (~12 glyphs: sparkle, static-burst, eye, cloud) reserved for empty states, win moments, Unknown's placeholder, and the Daily Known card.
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
│       │   ├── models/          # GameState, Player, Card, KnownRef DTOs (mirror server protocol)
│       │   └── repositories/
│       ├── domain/
│       │   ├── entities/
│       │   ├── repositories/
│       │   └── usecases/        # QuickPlay, JoinRoom, LockPlay, DrawCards, CastVote, SendQuickChat
│       ├── presentation/
│       │   ├── state/           # Riverpod state for the server-driven phases
│       │   ├── screens/         # MainMenu, Queue, Lobby, Round, Discussion, Knowoff, Verdict, Profile, Leaderboard, Store
│       │   └── widgets/         # KnownStage, UnknownStage, HandFan, PlayTable, VoteBoard, QuickChatBar, PokeNudge, ReadyButton, RoleCard, KnoinBadge
│       └── media/               # Client MediaEngine: pack metadata sync, signed-URL prefetch, LRU asset cache, Unknown renderer
├── server/                      # Go authoritative game server
│   ├── cmd/knowoffd/            # main.go — wiring, config load, graceful shutdown
│   ├── internal/
│   │   ├── config/              # Single typed config struct from layered YAML
│   │   ├── transport/           # WebSocket handling, codec, connection lifecycle
│   │   ├── lobby/               # Quick Play queues, room lifecycle, QR/room codes, reconnect grace
│   │   ├── game/                # Phase state machine, timers, roles, votes, forfeits, scoring
│   │   ├── bots/                # Quick Play backfill bots (labeled; reuses gamebot policy engine)
│   │   ├── media/               # Pack loader, relevance mesh, dealing, signed-URL issuing, role-scoped payloads
│   │   ├── economy/             # Knoin wallet, ledger, premium passes, entitlements
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

The Media Engine owns what media exists, how hands are dealt against it, and who is allowed to see what. It lives **server-side in Go**; the client carries a thin mirror for pack sync, prefetch, and Unknown rendering only.

### 1. Media-Pack Bundle Format

* A pack is a versioned bundle: `manifest.json` (pack tag e.g. `core-2026.10`, checksums, license & credits, age rating), `media.jsonl` (per Known: id, type `image|gif|text`, asset ref, embedding vector, tags, tone bucket, rating), `cards.jsonl` (per hand card: id, type `text|image|gif`, asset ref, embedding, tags), plus assets in object storage addressed by content hash.
* Version discipline: the server embeds the active pack tag in `phase_started`; clients sync **metadata** OTA on app start and on unknown tags, verify checksums, and hot-swap between matches — never mid-match. Assets stream on demand via the prefetch protocol (§4) with an LRU cache.
* Theme packs are additional bundles in the same format; pack updates and takedowns ship as version bumps the server hot-swaps without redeploying.

### 2. The Relevance Mesh

* Every Known and card carries an **embedding** in one shared multimodal space (local SigLIP/CLIP at build time; Gemini Embedding 2 as the API alternative). Tags remain human-facing metadata and theme filters — **similarity, not tag intersection, is the balance mechanism**, because tag vocabularies rot and noisy tags silently break dealing.
* Relevance bands over cosine similarity (thresholds in `tuning.yaml → dealing:`): **high** ≥ `band_high`, **distant** in [`band_low`, `band_high`), **chaos** < `band_low`.
* Dealing guarantee: the server picks the match's Knowns up front (secret, RNG seed logged) and deals each 5+3 hand as a constraint deal: against **every** scheduled Known, each player holds ≥ `min_high_per_known` high cards and ≥ `min_distant_per_known` distant cards, with the remainder chaos. Every hand always has a good answer, a stretch, and garbage — for Knower and Off alike.
* Shuffle re-deals are dealt *against the current Known schedule* so the guarantee survives mid-match mutation.
* Per-media candidate lists for all bands are precomputed at pack build; runtime dealing is array sampling, zero embedding math in the hot path.

### 3. Media Pipeline (`tools/mediapack`) & Content Production

* Pipeline stages (CLI + Workbench/Portal UI over the same code): `ingest` (transcode, EXIF strip, perceptual-hash dedupe) → `screen` (automated moderation) → `tag` + `embed` → human curation (🧑‍🎨) → `certify` → `bundle` → `publish`.
* **Quality targets — deliberately medium/low:** images ≤ 720 px longest side, compressed WebP; GIF loops ≤ 480p, ≤ 2 MB, re-encoded as animated WebP; text plain. Two reasons: small assets keep prefetch instant and the edge-cache path cheap, and lo-fi *is* the meme aesthetic.
* **Content standard:** humor may include sexuality within the bounds of eroticism — suggestive, cartoon, drawn, abstract — but **never pornographic or explicit content**, and always within app-store content rules. Erotic-leaning media carries an adult rating and ships only in age-gated packs (Product Baseline); the automated screen and human curation both enforce the line.
* Certification (the anti-dead-content gate): a pack version is publishable only if every Known has full band coverage for a 6-player deal, every card is reachable in some band, and Monte Carlo `simulate` confirms deal feasibility at both table sizes. Uncertifiable media stays in draft. The same checks back the Curator Guide's testing workflow (🧑‍🎨 §1).
* `mediapack simulate` also answers balance questions offline: band-threshold sweeps, Off-survival proxy rates under bot policies, Shuffle and Revote impact — tune `tuning.yaml` until distributions look right, then spend scarce playtests on feel.
* Production stack (verified on the owner's RTX 4080 Mobile, 12 GB VRAM): images via SDXL or Flux.1-Schnell fp8 (Apache-2.0); GIF loops via LTX-Video 2B distilled or Wan 2.1-1.3B (~8 GB), rendered to animated WebP; text cards via Ollama-served 7–14B models. API lane for style-critical or overflow work: nano banana (~$0.034–0.067/image), batch pricing at half rate. The binding constraint is human curation keep-rate, not compute.
* Tone rubric: the four-bucket humor matrix (millennial cope / Gen-Z absurdism / social awkwardness / chaos) lives in `content/tone-matrix.md`; every asset carries its bucket for pack-mix balancing.

### 4. Secrecy, Sync & Anti-Cheat

* Role-scoped payloads: every gameplay event is rendered per-recipient. During a round, Knower clients receive `{known: {id, signed_url, type}}`; Off clients — and caught-Off spectators — receive `{decoy: true}`. **Known never crosses the wire to a device that shouldn't have it.**
* Signed URLs are short-lived and single-round; Knowers prefetch during the inter-round countdown and every screen flips on one synchronized `show` tick. A still-loading Knower renders the same placeholder as Unknown, so loading state leaks nothing either.
* The server owns the phase clock; client timers are display-only; late intents are rejected. All match-deciding events — plays, specialty uses, votes, Revotes — are intents resolved exclusively server-side. All Knoin movement happens in the server ledger; the client only renders balances.
* Every scoring, role, and currency event lands in the append-only audit stream — the same stream that feeds stats, the leaderboard, KPIs, and report replays.

#### Designer Workbench & Tuning

Every number a playtest could question lives in one versioned file, `configs/gameplay/tuning.yaml`; structure is code, values are keys:

```yaml
seed: 42                            # reproducible dealing — same seed, same match

game:
  room_sizes: [4, 6]                # the only valid room sizes
  offs_by_size: {4: 1, 6: 2}        # public knowledge at the table
  votes_by_size: {4: 2, 6: 3}       # one Knowoff per round; match ends early when votes can't catch remaining Offs
  min_connected: 3                  # below this the match ends unscored (Rules §7)
  reconnect_grace_s: 20             # also the team-forfeit grace (Rules §7)
  pokes_per_target_per_round: 1
  abandon_cooldowns_s: [60, 300, 900]   # escalating Quick Play matchmaking cooldowns

timers:                             # seconds; which windows may fast-forward is structure (§8)
  round_per_player: 10              # play window = value × players; Ready unanimity ends it early
  discussion_per_player: 10         # Ready unanimity ends it early
  knowoff_ballot: 20                # always runs full
  knowoff_runoff: 15                # tie-break among tied players; always runs full
  vote_result_window: 15            # result display before finalizing — the Revote window
  prefetch_countdown: 5             # inter-round countdown = Knower prefetch budget

hand:
  size: 5
  draw_pile: 3
  specialty_weights: {pass: 0.10, reveal: 0.04, one_more_card: 0.08, shuffle: 0.05, revote: 0.05}
  # shuffle deals only into Off hands, revote only into Knower hands, post role-assignment
  # unique specialties (shuffle, revote) fire once per match — later copies are dead cards

dealing:                            # relevance mesh (⚙️ §2) — v1 placeholders, tuned via simulate
  band_high: 0.55
  band_low: 0.30
  min_high_per_known: 2
  min_distant_per_known: 2

points:                             # match points — leaderboard & session scoreboard
  correct_vote: 1
  knower_win_bonus: 1
  off_team_win: 3

knoin:                              # currency earnings — instant, kept on disconnect (Rules §6)
  match_completed: 5
  knower_win: 30
  off_team_win: 50
  correct_vote: 5
  off_vote_survived: 10
  daily_first_win: 25
  daily_earn_cap: 300               # anti-farm ceiling on play earnings
  challenge_winner: 500
  contributor_accepted_asset: 100

economy:
  free_daily_quickplay_matches: 10  # per free account per server day; local rooms never capped
  premium_prices: {day_1: 250, day_3: 600, day_7: 1200}      # Knoin
  unlock_prices: {custom_avatar: 1000, poke_style: 400, theme_pack: 1500}   # Knoin
  knoin_bundles: [500, 1200, 3000, 8000]   # bulk IAP sizes; store price tiers mapped at launch

liquidity:                          # Quick Play backfill bots (🎮 §1)
  backfill_enabled: true
  queue_timeout_s: 25
  min_humans: 1
  leaderboard_min_humans: 3
  knoin_min_humans: 2               # team-win Knoin requires this many humans

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
| Active free player affords a 1-day premium | every ~2 days of play | `knoin.*` earn values |
| 7-day premium for a committed free player | every ~8–10 days | `premium_prices` |
| Earned vs purchased Knoin in circulation | ≥ 70% earned | bundle sizes/prices |
| Free daily cap actually felt | by the top ~20% of free players only | `free_daily_quickplay_matches` |

`mediapack simulate` reports expected per-match Knoin under bot policies at both table sizes; the nightly KPI jobs report the real numbers, and the levers above move one at a time.

---

## 🌐 Network Architecture: Server-Authoritative WebSocket

```text
[ Flutter app  (Android/iOS) ] ─┐                        ┌──────────────────┐
[ Flutter PWA  (any browser) ] ─┼─( WSS / JSON intents )►│  GO GAME SERVER  │◄──►[ Redis ]
[ Flutter app / PWA … seat N ] ─┘  ◄─(role-scoped events)│  - Queues & auth │      queues/presence
              ▲                                          │  - Phase timers  │◄──►[ PostgreSQL ]
              │ signed GETs (Knowers only)               │  - Role secrecy  │      profiles/ledger/
[ Cloudflare edge cache ]◄──[ MinIO on home server ]     │  - Media dealing │      leaderboard/media
  (tunnel: cloudflared — no open ports, TLS at edge)     │  - Votes/economy │◄──►[ MinIO (assets) ]
                                                         └──────────────────┘
```

* Protocol: one persistent WebSocket per client. Intents: `queue_quickplay`, `join_room`, `lock_play`, `use_specialty` (covers Shuffle and Revote), `draw_cards`, `cast_vote`, `quick_chat` (canned phrase id), `ready`, `poke`, `report_media`. Events: `phase_started`, `role_assigned` (private), `round_started` (role-scoped Known/decoy payload), `show`, `round_resolved` (attributed plays), `shuffle_occurred` (anonymous), `vote_result_pending` (opens the 15 s result window), `vote_nullified` (attributed Revote), `knowoff_resolved` (caught / missed + role reveal), `match_verdict`, `points_scored`, `knoin_granted`, `quick_chat` (broadcast). Versioned JSON with sequence numbers for ordered replay.
* Fair arbitration: play windows, ballots, and runoffs are blind-simultaneous — collected privately, resolved at window close. No match-deciding event is a speed race.
* Reconnect: session-token snapshot rejoin (Rules §7), role-scoped like everything else.
* Room→node affinity: every room lives on exactly one node (Redis maps `room_id → node`); no cross-node game state — the property that makes horizontal scaling trivial later.
* Live match state in server memory only; PostgreSQL for durable outcomes and the Knoin ledger; Redis for queues/presence/routing; object storage for assets.

---

## 🤖 Bots: Development, Testing & Launch Liquidity

Bots fill seats in two sharply separated roles — dev/test bots that never meet the public, and the labeled Quick Play backfill bots (🎮 §1). In neither role does a bot take over a human's mid-match seat, and no bot is ever disguised as a human.

* **Dev/test bots:** `tools/gamebot` (Go CLI) spawns N bot players over the real WebSocket protocol — same intents, same timers, no server backdoors. Policy reuses `server/internal/media` as a library: play by noisy embedding preference as a Knower; play plausible-band cards blind as an Off; draw when the hand scores badly; vote by a noisy suspicion heuristic; occasionally Shuffle/Revote when held. Seeded — a failing match replays exactly. Bots act early and Ready immediately: a 6-seat dev match crosses every phase in well under a minute. Solo development against 5 bots is the daily loop.
* **Backfill bots:** run inside the server (`server/internal/bots`), reusing the same policy engine with per-match randomized personality parameters so regulars can't farm a fixed tell. Labeled 🤖 always; humans outrank bots for seats; economy and leaderboard guardrails in 🎮 §1. Sunset by measurement: per queue, once p50 time-to-fill stays under the timeout for a sustained window, the scheduler stops adding bots there.
* Configuration: a `bots:` block exists only in `local.yaml`/`staging.yaml`; the key is absent from `prod.yaml` and the server refuses external bot connections when unset. Backfill bots are configured separately (`liquidity:` in `tuning.yaml`) and are in-process, so the connection-refusal rule is untouched.

---

## 📦 Infrastructure & Deployment

### 1. Containerization Principles

* Every runnable ships as a container from day one: `server` (distroless static Go binary), `postgres`, `redis`, `minio`, `cloudflared`, dev tooling (migrations runner, adminer). Multi-arch images, environment-agnostic — the same images run on the home server now, a VPS later, Kubernetes after that; behavior differs only by mounted config.
* Stateless-by-design server: durable state in PostgreSQL, coordination in Redis, assets in object storage, live matches in memory with room→node affinity.

### 2. Hosting: Home Server + Cloudflare (dev/beta), VPS at Launch

* Dev and beta run entirely on the owner's home machine under Docker Compose. **Cloudflare Tunnel** (`cloudflared`, free) publishes `play.<domain>` (game server — WebSockets pass through) and `cdn.<domain>` (MinIO assets) with no open ports, no exposed home IP, TLS at the edge.
* **Edge caching does the heavy lifting:** assets are content-hashed and immutable, so `cdn.<domain>/*` gets a Cache-Everything rule with a long edge TTL; after first request Cloudflare serves the media and the home uplink sees near-zero asset traffic. Low/medium media quality (⚙️ §3) keeps objects small; a round's prefetch is a few hundred KB across a table. v1 media is images and text only (loops ship as animated WebP), comfortably inside Cloudflare's free-plan content rules.
* **Public launch moves the stack to a small VPS** (~€5/mo class): an online-first product can't ride a home ISP's uptime into the stores. It's a lift-and-shift — same images, same config model; assets can offload to Cloudflare R2's free tier (10 GB, zero egress) if the home box retires completely.
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

## 💰 Monetization: The Knoin Economy

One currency sits at the center of the business: **Knoin**. Players earn it by playing well, buy it in bulks when they want more, and spend it on premium time, packs, and cosmetics. Design goals, in order: keep free players playing daily, make earned progress feel meaningful (a free player must be able to reach everything), and monetize impatience and identity — never gameplay advantage. **No pay-to-win: nothing purchasable affects dealing, roles, votes, or scoring.**

### 1. Earning Knoin

* Play rewards (Rules §6): completing matches, winning as either team, correct votes, surviving votes as an Off, first win of the day — credited instantly and kept even on disconnect. A daily earn cap (`knoin.daily_earn_cap`) blunts farming, and team-win Knoin requires ≥ `liquidity.knoin_min_humans` humans in the match.
* Contribution rewards (🧑‍🎨 §1): Knoin per accepted asset, a large bonus for winning the Weekly Known Challenge.
* Balance target: an active free player earns a 1-day premium every ~2 days of play (protocol table in ⚙️ Tuning) — collecting is deliberately *not hard*; the sink structure below is what makes the economy work.

### 2. Premium — Time Passes Bought With Knoin

* **Premium is unlimited Quick Play + no ads**, sold as **1-day (250), 3-day (600), and 7-day (1,200) passes priced in Knoin** — never as a separate cash subscription, so every path to premium runs through the one currency.
* Free accounts get `economy.free_daily_quickplay_matches` per server day (**10 at launch** — tuned so only the most engaged fifth of free players ever feel it). **Local Rooms are never capped** — play with the people in your living room is always free and unlimited.

### 3. Knoin Bulks (the cash lane)

* Bulk packs via platform billing (Play Billing / StoreKit): sizes in `economy.knoin_bundles`, store price tiers mapped at launch. This is the only place money enters; everything money can get, play can also get — slower.
* Rewarded ads (SSV — the ad network's servers call our verification endpoint; the client callback grants nothing): an optional post-match ad **doubles that match's Knoin**; premium players get the doubling automatically, ad-free. All ad surfaces disappear under premium.

### 4. Theme Packs

* Curated media packs (humor verticals, seasonal, community highlights, age-gated adult-humor packs) priced in Knoin (`economy.unlock_prices.theme_pack`).
* In private and local rooms, the **Host Pass** rule applies: only the room creator needs the pack; the whole table plays it, guests never pay. Quick Play runs the core pack plus a free rotating featured pack.

### 5. Cosmetics & Identity

* **Poke Styles** (visual + haptic effect sets) and the **Custom Avatar** unlock (👤 §2), priced in Knoin. More identity items (card backs, reveal animations) ride the same entitlement rails later.
* Technical spine for all of it: a PostgreSQL wallet with an append-only ledger (earns, purchases, spends, refunds); debits atomic with entitlement writes; balances server-side only; all prices in `tuning.yaml` so economy tuning never needs a client release.

---

## 🧾 Product Baseline (v1 Decisions)

* Modes & discovery: Quick Play queues per room size (4/6) as the main surface; private/local rooms via 6-character codes + QR deep links. No public room browser, no skill rating at v1.
* **Communication: canned Quick Chat + reactions only.** No free-text, no voice, no video at v1 — canned phrases are localizable, moderation-safe, and enough to accuse and defend; free-text is a v2 candidate behind mute/report infrastructure. Local rooms talk out loud anyway.
* Progression: one server-side XP track (matches completed, correct votes, Off survivals); levels gate portal role applications and cosmetic unlocks. Values in `tuning.yaml`.
* **Content policy:** humor may be suggestive/erotic within store rules — cartoon, drawn, abstract — **never pornographic or explicit**. Erotic-leaning media ships only in adult-rated, age-gated packs; store age ratings set accordingly (17+/18+ where such packs are available), and the age obligation sits on the user's declared age at the gate. Enforced twice: automated screen + human curation (⚙️ §3).
* Compliance: anonymous device accounts by default, optional linking later; privacy notice at first launch; age gate + per-pack age ratings; Google UMP consent before any personalized ad; in-app delete-my-data backed by a server endpoint; purchases exclusively through platform billing; contributor license grants (commercial use + modification) stored with terms version and timestamp per submission.
* Analytics: no third-party client SDK — the authoritative server witnesses every event; nightly jobs derive KPIs (retention, queue fill times, matches/day, Off win rate by table size, Knoin earn/spend flows, premium conversion, pack attach rate) from the audit stream.
* Moderation & admin: nickname profanity filter, conduct + media reports, Guard freezes with admin-final bans, Admin Console actions (kick, ban, close room, avatar/media takedown) — all live before public launch.
* Platforms & release: **Android native + Web PWA first**, **iOS native fast-follow** once retention is proven. CI builds all three targets from day one. App-size budget enforced in CI: packs stream, binaries stay lean.

---

## 🛠️ Step-by-Step Implementation Lifecycle

### Phase 1: Foundation — Monorepo, Containers & Config

* Task 1: Establish the monorepo tree; implement the layered YAML config loader in `server/internal/config` with fail-fast validation.
* Task 2: Stand up Compose (server skeleton with `/healthz`–`/readyz`, Postgres + migrations, Redis, MinIO); scaffold the Flutter client with `GameTransport` and a WebSocket echo loop building for Android **and** Web from day one.
* Testing Criteria: `docker compose up` yields a healthy stack from a fresh clone; both client targets connect and echo; config errors are clear and complete.

### Phase 2: Media Engine & Pipeline

* Task 1: Implement the pack bundle format, the object-storage layout, `tools/mediapack` (ingest → screen → tag → embed → certify → bundle → simulate), and the in-memory pack loader + precomputed band lists in `server/internal/media`.
* Task 2: Ship the **Media Workbench** (server-rendered, dev-only at this phase) and produce the seed pack on the local GPU: target **≥150 certified Knowns and ≥1,500 cards** post-curation at the §3 quality targets, with keep-rate measured.
* Task 3: Client `media/` module: pack metadata OTA sync, signed-URL prefetch, LRU cache, Unknown renderer.
* Testing Criteria: `mediapack simulate` proves deal feasibility at both table sizes; certification rejects a deliberately band-starved pack; a client cold-starts, syncs the pack, and prefetches a round's Known inside the countdown budget on a mid-range phone.

### Phase 3: Realtime Game Loop

* Task 1: Room lifecycle (4/6 seats only), versioned intent/event protocol with sequence numbers, session-token reconnect snapshots, **role-scoped payload rendering** (the security-critical piece — built and reviewed first). `tools/gamebot` alongside — this phase's tests depend on it.
* Task 2: The phase state machine: role assignment, constraint dealing, per-round blind play + attributed reveal, draws, specialty resolution (Pass / Reveal with hidden specialty backs / One More Card / anonymous once-per-match Shuffle / attributed once-per-match Revote in the result window), timeout auto-discard, discussion with Quick Chat, Knowoff ballot + runoff + result window, catch/miss resolution with the early-end rule (votes remaining < Offs remaining), forfeit and unscored-match rules, match points, Ready/Poke.
* Task 3: The Flutter screens and phase state against the live protocol, on native and PWA, including the Quick Chat bar.
* Testing Criteria: scripted bot matches complete at both table sizes with forced mid-round disconnects/reconnects and no state corruption; **a protocol-level assertion proves no Off or caught-Off connection ever received a Known id or URL in any phase**; a missed vote reveals and removes nobody; a 6-player match with two missed votes auto-ends after the second Knowoff as an Off win; a Revote in the result window nullifies the shown result and its survival credit, and a second Shuffle or Revote in the same match is rejected; disconnecting every Off ends the match as a Knower forfeit win after exactly the grace period.

### Phase 4: Accounts, Quick Play & Hardening

* Task 1: Auth (JWT sessions) binding anonymous tokens to accounts; profiles and public stats; Quick Play FIFO queues per room size with reconnect-safe seat reservation; **backfill bots** (`server/internal/bots`) with labeling, seat-priority, and sunset scheduling; abandon cooldowns.
* Task 2: Harden the intent pipeline: rate limiting, deadline enforcement, protocol-boundary validation, structured audit logs to PostgreSQL as the analytics event stream; nightly jobs for public stats and the **Weekly Leaderboard** (Quick Play only, min-humans and daily-count guards, immutable weekly history).
* Testing Criteria: a deliberately modified client (forged plays, late votes, replayed messages, Off requesting Known assets) alters nothing and triggers audit trails; a queue short of humans backfills with labeled bots after the timeout and never starts below `min_humans`; a `gamebot` load test sustains hundreds of concurrent rooms on one node with prefetch traffic against MinIO; leaderboard excludes sub-threshold backfilled matches and respects the daily counted cap.

### Phase 5: Knoin Economy, Admin & Launch Polish

* Task 1: The `economy` module: Knoin wallet + append-only ledger, instant per-event play grants with the daily earn cap, premium time passes (1/3/7-day) with the Quick Play cap gate, Knoin bulk IAP via platform billing, SSV rewarded post-match doubler, theme packs with Host Pass enforcement, Poke Styles and Custom Avatar unlocks.
* Task 2: Admin Console over Phases 4–5 endpoints (case queues, Guard-freeze reviews, pack dashboard, leaderboard ops, Daily Known calendar, economy ledger); Daily Known feed; the how-to-play clip once UI is final.
* Task 3: Performance and launch passes: client paints on low-end devices, server allocation/GC under queue load, edge-cache hit rates on pack releases, **VPS migration runbook executed** (Infra §2), store review prep for the content policy (age-gated adult packs, UMP, age gate).
* Testing Criteria: Knoin grants land instantly and survive a mid-match disconnect; a free account's 11th Quick Play match of the day is rejected at queue time while a local room still opens; premium passes expire on schedule and re-gate correctly; ad grants only via SSV; debits atomic with entitlements; a bot-heavy match under `knoin_min_humans` grants no team-win Knoin; takedowns propagate in the next pack version and invalidate correctly at the edge.

### Phase 6: Contributor Portal & Community

* Task 1: Promote the Workbench codebase to the public **Contributor Portal**: role applications with admin grants (Contributor / Curator / Guard), the Curator Guide and deal-simulator access, Guard freeze flows wired to the Admin Console case queue, submission pipeline with terms-consent capture (version + timestamp), credits + Knoin rewards on acceptance.
* Task 2: The **Weekly Known Challenge**: 100-entry capacity with rejection-reopened slots, pre-vote screening queue, open live-tally voting with one immutable vote per player and no self-votes, Monday close with winner publication, credits, and Knoin payout.
* Testing Criteria: a submission traverses `draft → published` with every transition audited and immutable after submit; entry #101 is rejected until a screening rejection reopens a slot; an unscreened entry is never publicly visible or votable; a second vote or a vote change is rejected; a Guard freeze suspends matchmaking within seconds, auto-expires at `guard_freeze_max_h`, and only an admin can convert it to a ban; challenge winners receive credits and Knoin atomically with pack publication.
