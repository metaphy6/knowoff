# 📖 Knowoff — Project Architecture & Implementation Roadmap

## 🎲 The Game at a Glance

Knowoff is a **4–8 player**, same-room social deduction party game of broken signals and chaotic bluffing. Each round a piece of media — **the Known** — appears on every player's screen… except for the **Offs**, who see a decoy (**the Unknown**) and must fake it. Everyone plays cards from their hand that supposedly relate to the Known; **Knowers** read the plays for tells while Offs blend in behind deliberately ambiguous hands. After three rounds the table votes in **the Knowoff** — the accusation event the game is named after. Catch every Off and the Knowers win; survive, or steal the win with a last-second guess at the real Known, and the Offs take it.

There is deliberately **no separate plain-language rulebook** to keep in sync: the **Game Rules & Mechanics Blueprint** below is the single normative source for how the game plays, and all player-facing help text, store copy, and the how-to-play clip are derived from it at production time.

### Terminology (normative)

| Term | Meaning |
|---|---|
| **Knower(s)** | Players who perceive the Known |
| **Off(s)** | Players who don't — their screen shows the Unknown; nobody knows who they are |
| **the Known** | The central media item of a round (image, GIF, silent video, or text — never audible, §2) |
| **the Unknown** | The decoy stage rendered on an Off's screen — same layout and luminance as the Known stage |
| **the Knowoff** | The endgame accusation vote |
| **the Steal** | A fully-caught Off's last-chance guess at the real Known |

---

## 🧱 Tech Stack

| Layer | Technology | Role |
|---|---|---|
| Client | Flutter — one codebase: native Android/iOS apps + Flutter Web **PWA** (desktop, and app-less guest fallback on any phone browser) | UI, WebSocket client, client `MediaEngine` (pack metadata sync, asset prefetch & cache, Unknown decoy renderer) |
| Backend | Go | Authoritative game server: rooms, timers, role assignment, media dealing, vote resolution, scoring — plus the Admin Console and Contributor Portal (server-rendered) |
| Database | PostgreSQL | Durable data: profiles, game results, entitlements, media metadata & contribution workflow |
| Cache / Pub-Sub | Redis | Lobby→node routing, session presence, cross-node pub-sub, rate limiting |
| Object Storage + CDN | S3-compatible (MinIO locally; R2/Hetzner in prod) behind a CDN | Media-pack assets — streamed to clients, never stored in Postgres, never baked into the app binary |
| Transport | WebSocket (JSON messages) | Single realtime channel between client and server |
| Infrastructure | Docker Compose (now) → Kubernetes + Terraform (later) | Everything containerized from day one |

Architecture decision records:

* **Server-authoritative over P2P/local-mesh** — a trusted authority owns role secrecy, the phase clock, and blind-window resolution; a same-room game still runs through the server.
* **Flutter everywhere over PWA-only (v1 of this project) or native-only** — native haptics for Poke, one codebase for three surfaces, and the web build preserves the zero-install guest path the party loop depends on.
* **Object storage + CDN as a first-class layer** — media packs are hundreds of MB; app binaries stay lean (size budget tracked in CI) and packs ship over the air.

## 🕹️ Game Rules & Mechanics Blueprint

### 1. Game Parameters

* Player Count: a room opens with **5 seats by default**; the host may set any size from the minimum **4** to the maximum **8** before starting. A started game continues as long as **at least 3 players remain connected** (§7).
* Off Count — public and deterministic: **4–5 players → 1 Off; 6–8 players → 2 Offs** (`offs_by_table_size` in tuning). The table always knows *how many* Offs exist — never *who*. Offs do not know each other at 2-Off tables.
* Round Count: **3 rounds** by default, extendable to at most **4** by the One More Round specialty (§5); `max_extra_rounds: 1`.
* Match Clock: each round's play window is `10 s × players` (40 s at 4 players, 80 s at 8). A default game runs 3 play windows + the discussion window (`15 s × players`) + the 20 s Knowoff ballot + a possible 20 s Steal. Worst case at 8 players ≈ **7 minutes**; Ready unanimity (§8) can only shorten a game, never extend it.
* Victory: team-based per game — Knowers win by ejecting **all** Offs in the Knowoff; Offs win by any Off surviving, or by a successful Steal. A light session score accumulates across games in the same room (§6).

### 2. Setup: Roles, the Known, and the Unknown

* Role assignment: the server secretly assigns Offs at game start. Every player receives an identical **hold-to-peek role card** — same screen, same gesture, same duration for everyone, so the reveal is shoulder-surf-proof by construction. An Off's card says they're Off; it never reveals the other Off.
* The Known: one media item per round, drawn from the room's active media pack. **Known types: image, GIF, silent video (≤ 4 s loop), text. The Known is never audible** — sound fills a shared room and would un-blind the Offs. Audio assets exist in packs only for public surfaces: the post-Knowoff recap, poke/system sounds, and victory stingers.
* The Unknown: during every round, Off clients render a decoy stage — identical layout, identical luminance, a neutral signal-shimmer placeholder where the media sits — indistinguishable from a Knower's screen at table distance. (A Knower whose prefetch is still loading briefly shows the same shimmer, which is free cover, not a bug.)
* Secrecy is server-side: **an Off client is never sent the Known's asset URL, id, or any derivative** — it receives only a decoy-render instruction. Knower clients prefetch the asset via short-lived signed URLs during the inter-round countdown; a synchronized `show` event flips every screen on the same server tick.
* Hands: every player is dealt a hand of **5 cards** plus a personal **3-card draw pile**. Cards are text/image prompt cards dealt by the Media Engine's relevance mesh (⚙️ §2) so that, against every Known scheduled for the game, each player is guaranteed a mix of strong, stretchy, and garbage options — the mechanical shield that lets Offs survive and makes Knowers doubt each other.
* Shoulder-surfing the Known is a stated design constraint: media renders at modest size with a tap-to-zoom that auto-shrinks, and hand-privacy is reinforced by onboarding copy — the same social norm as holding playing cards.

### 3. The Round Loop (3 Rounds, Blind Play)

* Play window (`10 s × players`, may end early on Ready unanimity): every player secretly locks **one action** — play a card from hand, or use an eligible specialty (§5). Nothing is visible while the window runs; all plays reveal **simultaneously and attributed** at window close. Blind-simultaneous resolution means no fastest-tap races and no copying the table's vibe mid-round.
* The table: revealed plays accumulate in attributed rows per round and stay visible for the rest of the game — the evidence record the Knowoff debate runs on.
* Draws: at any time during a round window, a player may draw from their 3-card personal pile — all at once or in parts. **Every draw is publicly announced** (who, how many): drawing is a costly signal that your hand doesn't fit.
* Timeout penalty: a player whose window expires with no locked action auto-passes and the system discards a random card from their hand. Stalling burns your options.
* Empty hands: a player with zero cards auto-passes with a public **Empty** marker and suffers no discard (there is nothing to discard). Reaching Empty is itself loud information.
* Poke (§8) targets players who haven't locked an action yet.

### 4. Discussion

* After the final round's reveal, a discussion window opens (`15 s × players`, Ready-skippable): no card actions, table talk only, the full play history on screen. This is where reads are argued and Offs talk their way out. The window is the game's social heart — the timer exists to bound it, not to rush it.

### 5. Card Specialties

The deck contains five specialties in two strict types. Specialty frequency per hand is tuned in `tuning.yaml`; role-restricted specialties are dealt after role assignment.

**Type A — Standard (played as your round action; costs one additional discard, except Pass):**

* **Pass** (occasional): skip playing a card this round. Only the Pass itself is discarded — the safe valve for a hand that fits nothing.
* **Reveal** (rare): forcibly expose another player's hand to the lobby. Revealed **media cards show face-up; specialty cards show as a count of face-down backs** — so Reveal can never leak a role through a role-restricted specialty (the fix for the role-leak flaw in the original sketch). Costs one additional discard.
* **One More Card** (occasional): usable at any moment during a round window, not consuming your play action: draw 1 card beyond your personal pile, from the mesh. Costs one additional discard.

**Type B — Unique (free, role-restricted):**

* **Shuffle** (rare — **Off only**): triggerable only at the very start of a round, before any action is locked. **Effect: every player's unplayed hand is returned and re-dealt fresh from the mesh against the same schedule of Knowns** (personal draw piles are untouched). The lobby is alerted that *a Shuffle occurred* — hands visibly change — but the actor stays anonymous, and since nobody knows who is Off, the alert reveals nothing about identity, only that an Off spent their bomb. Strategic purpose: it erases the plans Knowers built around saved cards, drowning the Off's improvisation in table-wide improvisation.
* **One More Round** (rare — **Knower only**): usable any time before the discussion window; adds a 4th round. The lobby is alerted **with attribution** — using it publicly semi-clears you (it's Knower-only), which is exactly its cost: you spend anonymity to buy more evidence. Capped by `max_extra_rounds`. The 4th round's Known is chosen by the inverse-mesh query (⚙️ §2) so remaining hands stay balanced.

### 6. The Knowoff, the Steal & Scoring

* Ballot (20 s, always runs full — blind to the end): every player secretly names **exactly as many suspects as the table's Off count** (1 or 2), never themselves. Votes resolve simultaneously at close; a timeout ballot counts as abstention and scores nothing.
* Ejection: the top-N voted players (N = Off count) are ejected and their roles revealed. A tie for the last ejection slot triggers one 15 s revote among the tied only; a persistent tie ends the game **as an Off win** — a table that can't coordinate deserves what it gets.
* Team verdict: Knowers win only if the ejected set is exactly the Off set. Any surviving Off = Off win.
* **The Steal** (full catch only): when the Knowers eject every Off, each caught Off gets one 20 s guess: a lineup of candidate Knowns from the final round (`steal_lineup_size`: 4 options at 1-Off tables, 6 at 2-Off), containing the real one among mesh-near decoys. **Any correct guess steals the game for the Offs.** Three rounds of table talk make this a skill shot, not a coin flip — it keeps near-miss Off play rewarding and blunts blowouts.
* Reveal recap: after the verdict, the game's Knowns are shown to everyone (Offs finally see what they survived) — the one surface where audio stingers play.
* Session scoring (light, persistent per room until the lobby closes; values in `tuning.yaml → scoring:`):

| Event | Points |
|---|---|
| Each Off you correctly named in your ballot | +1 |
| Knower team win bonus | +1 each Knower |
| Surviving as an Off (any survival) | +3 |
| Successful Steal (caught Off) | +3 (Knowers keep ballot points; win bonus canceled) |
| Abstained / timed-out ballot | 0 |

No chip economy, no stakes, no elimination during play — everyone plays every round of every game; the session scoreboard exists to crown a night's winner and feed profile stats, nothing more.

### 7. Disconnects & Dropouts

* Seats persist: a dropped player's seat is auto-played passively — no play locked (auto-pass, no discard penalty while disconnected), no votes, never poked, counts as always-Ready. Role assignments never change mid-game; **a disconnected Off's seat stays an Off**.
* Grace window: 20 s to reconnect before counting as dropped for the below-minimum check; the seat is auto-played from the moment of disconnect until its owner returns. Phone locks and app switches at a party table are constant — this path is a first-class citizen, not an edge case.
* Rejoin: a reconnecting client authenticates by session token and receives a full role-scoped snapshot of the current round — including, for a Knower, a fresh signed URL for the current Known.
* Below minimum: if connected players drop below 3, the current game auto-completes and is scored as-is; the room survives for the next game.
* No bot takeover, ever: a disguised stand-in is unthinkable in a deduction game. Dev bots exist only in dev/staging (§ Bots).

### 8. Pace Controls: Ready & Poke

* Ready: locking an action (or auto-pass) marks a seat Ready; unanimity ends play windows and the discussion window early. **The Knowoff ballot, revotes, and the Steal always run their full windows** — blind to the end.
* Poke: **once per target per round**, any player may poke a player who hasn't locked an action. The target's device gives a haptic buzz (native apps) plus a screen shake and an optional sound (all platforms — the PWA has no vibration API, so the visual effect is the baseline and haptics are the garnish). Pokes are **attributed** ("X poked Y") — in a same-room game the laughter is the feature. No score effect; cap enforced server-side.
* Transport: `ready` and `poke` are ordinary validated intents; Ready unanimity emits the same phase-advance event as timer expiry.

---

## 🎮 Game Modes & Live Ops

### 1. Local Room (the product)

Private rooms with 6-character codes and a QR join flow: host creates a room, the QR encodes the deep link — native app if installed, the web PWA otherwise, so **a guest without the app is never blocked from the table**. Same-room play is the designed context; the netcode (server-authoritative, role-scoped events) also happens to work fully remotely for free.

### 2. Online Quick Play — Deferred (not in v1)

**Status: deferred.** A public matchmade queue (and the labeled backfill bots it would need) is a post-retention feature for a party-first game. The architecture already supports it — rooms don't care where players sit — so this is a product decision recorded now to avoid scope creep, revisited when live data shows remote demand.

### 3. Weekly Known Challenge (Community Event)

A weekly live-ops loop powered by the Contributor Portal (🧑‍🎨): Monday the server announces a theme prompt (e.g., "worst possible group chat message"); contributors submit candidate media through the portal's standard pipeline all week; the following Monday, curation publishes the winners into the community pack with **credits**, alongside the winner announcement. Pure PostgreSQL + the existing scheduled-job machinery; no realtime channel.

### 4. Daily Known

One curated media item on app start — a taste of the game's humor, a dismissible card, never a gate. Scheduled through the Admin Console's editorial calendar; fetched once over HTTPS and cached. The lightweight analog of a word-of-the-day.

### 5. Session Scoreboard

The per-room score (§6) is the only competitive surface at v1. Global/weekly leaderboards are deferred with Quick Play — private-room scores are collusion-trivial and don't belong on a board.

---

## 👤 Profiles & Community

### 1. Public Player Statistics

* Pseudonymous profiles (nickname + avatar, no PII), tappable from lobby and scoreboard: games played/won, Knowoff accuracy (correct suspects named), Off survival rate, Steal conversions, pokes sent.
* Derived nightly from the server's audit-event stream — no client-reported numbers anywhere.
* Reading reputations is part of the metagame: a profile that survives 60% of its Off games *should* scare the table.
* Privacy: delete-my-data erases stats with the account; contributor credits (🧑‍🎨) are the one opt-in public identity surface.

### 2. Avatars (Free Presets, Paid Uploads)

* A curated preset gallery, free forever, on-brand by construction.
* A one-time **Custom Avatar** unlock (game currency, 💰 §5) opens personal uploads: server-side crop to 256×256 WebP, EXIF strip, size cap, automated moderation screen before display, reportable forever, admin takedown reverts to presets without refund. Stored as small blobs in PostgreSQL (packs live in object storage; avatars are tiny and this avoids CDN cache-invalidation churn on profile edits).

### 3. Player Reports

* One tap from any profile or scoreboard: inappropriate avatar/nickname, harassment, cheating/collusion — plus, distinctly, **media reports** on any Known or card (offensive/broken/mistagged), which route to the curation queue rather than the conduct queue.
* Server-authoritative design already kills technical cheating; reports exist for the human kind. No automated punishment at v1 — everything lands in the Admin Console case queue. Rate-limited; repeat reports collapse into one case.

### 4. Feedback & Ideas

* In-app form (bug / idea / other) with an auto-attached context snapshot (app version, pack tag, last game id) shown to the user before sending. Rate-limited endpoint into a Postgres table, triaged in the Admin Console. Media disputes stay in their own lane (§3).

---

## 🧑‍🎨 Contributor Portal & Media Workbench

One web application, two deployments, three roles (**contributor / curator / admin**). It is the human UI over the media pipeline the same way the Admin Console is the human UI over operations — nothing enters a pack through hand-run scripts.

### 1. Media Workbench (dev/staging — ships early, Phase 2)

The internal curation surface for the AI generation pipeline; the project owner is user #1.

* **Batch ingestion:** watches ingest folders/buckets for ComfyUI (images, loops) and Ollama (text card) output; every asset is auto-processed on arrival — transcode (WebP/WebM), perceptual-hash dedupe, auto-tag, embedding (⚙️ §2), automated content screen.
* **Bulk curation grid:** keep/kill at keyboard speed with tone-bucket and rating assignment; measured keep-rate per generation batch (the pipeline's core KPI — expect 10–30% for the humor bar this game sets).
* **Embedding sanity view:** nearest-neighbor browser for any asset — catches mis-embedded media before it corrupts dealing.
* **Deal simulator:** for any candidate Known, render the hands the mesh would actually deal at each table size — the "is this media playable?" check, one click.
* Accepted assets enter a draft pack version; publishing is a versioned pack release (⚙️ §1).

### 2. Contributor Portal (prod — post-launch, Phase 6)

The same application, public-facing, for community submissions.

* Flow: upload or write media (image, GIF, silent video, text; audio only for public-surface asset slots) → the same automated processing as the Workbench → **submit** → curator review (accept / reject with note) → accepted media enters the next pack version.
* Workflow states: `draft → submitted → in_review → approved | rejected → published(pack-tag)` — every transition audited.
* Submission terms: a perpetual, non-exclusive license grant captured at submit time; age-rating and content flags required; per-contributor rate limits and a minimum account level.
* **Rewards: credits only (v1 decision)** — contributor name in pack credits and a profile badge. No revenue share at v1 (no payout rails, no tax surface); the schema stores attribution so a future rev-share is an economy feature, not a migration.
* Moderation: everything passes the automated screen *and* human curation before any player sees it; published media stays reportable (👤 §3) and takedown-able, with removals shipping in the next pack version.

### 3. Architecture

Server-rendered from the Go binary (same pattern as the Admin Console) on a public subdomain — no separate SPA build chain. Metadata and workflow in PostgreSQL; binaries in object storage; the automated moderation screen is provider-swappable. The Workbench deployment is the same code pointed at dev/staging with curator-role defaults.

---

## 🛡️ Admin Console

One authenticated, browser-based operations board, served from the Go server on the internal admin port — never through the public ingress. Admin accounts in PostgreSQL with role-based access, 2FA, rate-limited sessions, CSRF protection, and an append-only audit trail (who, what, when, before/after). Every action is an audited server mutation through product code paths.

* **Reports & moderation:** the case queue — conduct cases (kick, ban account+device, close room, avatar takedown) and media cases (asset takedown → next pack version, tag fixes). Bans hit live connections immediately.
* **Curation & packs:** the contributor review queue (🧑‍🎨 §2), pack dashboard — active tag per pack, checksum verification, hot-swap trigger, certification stats (⚙️ §3), and the Weekly Known Challenge scheduler.
* **Daily Known:** calendar editor with exact-render preview and hot-replace.
* **Economy & accounts** (read-mostly at v1): wallet ledger and entitlement lookups; grants/refunds are explicit audited actions.
* **Feedback triage:** new / seen / done.
* Roadmap fit: endpoints ship with the features they manage; the console UI is a Phase 5 deliverable — in place before any public launch.

---

## 🎬 How-to-Play Clip (Ship-Gated)

A ≤45-second, watch-don't-read onboarding clip: a first-timer should follow their first game after one viewing.

* Format: real UI capture only; silent-autoplay friendly — big captions carry the story; one idea per beat; readable at phone size.
* Storyboard (7 beats, 4–6 s each):
  1. Hook — "One of you can't see this." A meme glitches into static on one phone among five.
  2. Roles — the identical hold-to-peek gesture; one card whispers *you're Off*.
  3. The Known — media appears on every screen at once; the Off's screen shows the Unknown, and nobody can tell.
  4. Play — five cards lock blind, flip attributed; caption "whose card doesn't get it?"
  5. Pressure — a draw announcement, a poke shake, a Shuffle alert.
  6. The Knowoff — the vote board fills; an Off is dragged into the light.
  7. The Steal — the caught Off faces the lineup… and takes the win. Logo out.
* Ship gate: produced on final production UI, released with prod — no clip work while gameplay, engine, and netcode remain open.

---

## 🎨 Visual Identity: Broken-Signal Design Matrix (v1 direction)

Direction: **dark-first, signal-glitch minimalism**. Dark UI is a *functional* choice here, not an aesthetic one — parties happen in dim rooms, and a dark canvas minimizes screen-glow variance between the Known and the Unknown, reinforcing the decoy at the hardware level. Personality comes from tiles, type, and one glitch motif; no mascots, no scene art in the match flow.

* Palette tokens (Flutter constants; dark theme only at v1): `canvas` `#131320` near-black indigo; `surface` `#1E1E2E` for cards and sheets; `ink` `#F2F2F7` for every glyph and border — text is never gray-on-gray. Semantic trio: `signal` `#4CE0D2` cyan = interact/reveal; `lime` `#D4F04C` = truth/score; `pink` `#FF5C9D` = accusation/risk. One permitted gradient (`#4CE0D2 → #B49AF5`) reserved for the Knowoff reveal header.
* The glitch motif: the Unknown's shimmer, the clip's static hook, and empty states share one procedural scanline/shimmer treatment (`CustomPainter`, no raster) — the brand *is* the broken signal.
* Structure: 2 px `ink` borders, hard zero-blur shadows, radius 16/12/pill; press = shadow-collapse click. Fixed semantics; no verdict leans on hue alone — color always pairs with icon + label (colorblind-safe by construction).
* Performance guardrails: flat fills, zero blur radii, no stacked translucency — low-end phones at a party are the norm, not the exception.
* Asset strategy — code first, raster last: UI chrome is 100% widgets/`CustomPainter`s. True raster (preset avatars, app icon, store art) is produced offline in curated batches via the **nano banana (Gemini image) API**, with the AI assistant driving prompts derived from this matrix; candidates → human curation → consistency pass → committed like any asset. API keys live under the config discipline (📦 §2), never in YAML or images. In-game *media content* comes exclusively from media packs (⚙️) — the design system and the content pipeline never mix.

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
│       │   └── usecases/        # JoinRoom, LockPlay, DrawCards, CastKnowoffBallot, StealGuess
│       ├── presentation/
│       │   ├── state/           # Riverpod state for the server-driven phases
│       │   ├── screens/         # MainMenu, Lobby, Round, Discussion, Knowoff, Recap, Profile, PackStore
│       │   └── widgets/         # KnownStage, UnknownShimmer, HandFan, PlayTable, VoteBoard, PokeNudge, ReadyButton, RoleCard
│       └── media/               # Client MediaEngine: pack metadata sync, signed-URL prefetch, LRU asset cache, decoy renderer
├── server/                      # Go authoritative game server
│   ├── cmd/knowoffd/            # main.go — wiring, config load, graceful shutdown
│   ├── internal/
│   │   ├── config/              # Single typed config struct from layered YAML
│   │   ├── transport/           # WebSocket handling, codec, connection lifecycle
│   │   ├── lobby/               # Room lifecycle, QR/room codes, reconnect grace
│   │   ├── game/                # Phase state machine, timers, role assignment, vote & Steal resolution, scoring
│   │   ├── media/               # Pack loader, relevance mesh, dealing, signed-URL issuing, role-scoped payloads
│   │   ├── portal/              # Contributor Portal + Media Workbench (server-rendered) + Admin Console
│   │   └── store/               # Postgres repositories, Redis presence/routing, object-storage client
│   └── migrations/
├── deploy/
│   ├── compose/                 # server + postgres + redis + minio (+ adminer) — one command up
│   ├── k8s/                     # Future — scaffolded, not required to run
│   └── terraform/               # Future — provider-agnostic modules
├── tools/
│   ├── mediapack/               # Go CLI: ingest → tag → embed → certify → bundle → simulate (⚙️ §3)
│   └── gamebot/                 # Go CLI: protocol-level dev/test bots
├── content/                     # Generation-side inputs: prompt templates, tone matrix, style presets, curation overlays
└── configs/                     # base.yaml + <env>.yaml overlays (incl. gameplay/tuning.yaml)
```

Separation rule: `server/internal/game` and `server/internal/media` contain **all** rules, dealing, and secrecy logic — the client renders state and sends intents; it never decides an outcome and never receives data its role shouldn't see.

---

## ⚙️ Media Engine Specification

The Media Engine is Knowoff's analog of a word engine: it owns what media exists, how hands are dealt against it, and who is allowed to see what. It lives **server-side in Go**; the client carries a thin mirror for pack sync, prefetch, and decoy rendering only.

### 1. Media-Pack Bundle Format

* A pack is a versioned bundle: `manifest.json` (pack tag e.g. `core-2026.10`, checksums, license & credits, age rating), `media.jsonl` (per Known: id, type, asset ref, embedding vector, tags, tone bucket, rating), `cards.jsonl` (per hand card: id, text/asset ref, embedding, tags), plus assets in object storage addressed by content hash.
* Version discipline: the server embeds the active pack tag in `phase_started`; clients sync **metadata** OTA on app start and on unknown tags (delta when available), verify checksums, and hot-swap between games — never mid-game. Assets are *not* pre-downloaded wholesale: they stream on demand via the prefetch protocol (§4) with an LRU cache.
* Theme packs are additional bundles in the same format, selected per room at creation (💰 §4); pack updates and takedowns ship as version bumps the server hot-swaps without redeploying.

### 2. The Relevance Mesh (replaces literal tag matching)

* Every Known and every card carries an **embedding** in one shared multimodal space (local SigLIP/CLIP at build time; Gemini Embedding 2 as the API alternative — both produce the same artifact: a vector in `media.jsonl`/`cards.jsonl`). Tags remain as human-facing metadata and theme filters — **similarity, not tag intersection, is the balance mechanism**, because tag vocabularies rot and noisy tags silently break dealing.
* Relevance bands over cosine similarity (all thresholds in `tuning.yaml → dealing:`): **high** ≥ `band_high`, **distant** in [`band_low`, `band_high`), **chaos** < `band_low`.
* Dealing guarantee: the server picks the game's 3 Knowns up front (secret, RNG seed logged) and deals each 5+3 hand as a constraint deal: against **every** scheduled Known, each player holds ≥ `min_high_per_known` high cards and ≥ `min_distant_per_known` distant cards, with the remainder chaos — every hand always has a good answer, a stretch, and garbage, for Knower and Off alike. That ambiguity is the Offs' shield and the game's engine.
* Inverse query: a One More Round Known, and Shuffle re-deals, are chosen/dealt *against the current hands* so the guarantee survives mid-game mutation.
* The Steal lineup: `steal_lineup_size − 1` decoys sampled from the real Known's near band — close enough to be plausible, far enough to be wrong.
* Per-media candidate lists for all bands are precomputed at pack build; runtime dealing is array sampling, zero embedding math in the hot path.

### 3. Media Pipeline (`tools/mediapack`) & Content Production

* Pipeline stages (CLI + Workbench UI over the same code): `ingest` (transcode WebP/WebM, EXIF strip, perceptual-hash dedupe) → `screen` (automated moderation) → `tag` + `embed` (local vision/text models or API) → human curation (🧑‍🎨) → `certify` → `bundle` → `publish`.
* Certification (the anti-dead-content gate): a pack version is publishable only if every Known has full band coverage for a maxed 8-player deal, every card is reachable in some band, and the Monte Carlo `simulate` confirms deal feasibility across table sizes. Uncertifiable media stays in draft — no dead Knowns by construction.
* `mediapack simulate` also answers balance questions offline: band-threshold sweeps, Off-survival proxy rates under bot policies, Shuffle impact — tune `tuning.yaml` until distributions look right, then spend scarce playtests on feel.
* Production stack (verified against the owner's RTX 4080 Mobile, 12 GB VRAM): images via SDXL or Flux.1-Schnell fp8 (Apache-2.0), loops via LTX-Video 2B distilled or Wan 2.1-1.3B (~8 GB), text cards via Ollama-served 7–14B models, public-surface audio via Stable Audio Open. API lane for style-critical or overflow work: nano banana (~$0.034–0.067/image), Veo 3.1 Lite (~$0.05/s), batch pricing at half rate. The binding constraint is human curation keep-rate, not compute — which is why the Workbench ships in Phase 2, not after launch.
* Tone rubric: the four-bucket humor matrix (millennial cope / Gen-Z absurdism / social awkwardness / chaos) lives in `content/tone-matrix.md` as the generation and curation rubric; every asset carries its bucket for pack-mix balancing.

### 4. Secrecy, Sync & Anti-Cheat

* Role-scoped payloads: every gameplay event is rendered per-recipient. During a round, Knower clients receive `{known: {id, signed_url, type}}`; Off clients receive `{decoy: true}` — **the Known's identity never crosses the wire to an Off**, so no client inspection, proxying, or web-devtools spelunking can recover it.
* Signed URLs are short-lived and single-round; Knowers prefetch during the inter-round countdown and every screen flips on one synchronized `show` tick. A still-loading Knower renders the same shimmer as the Unknown.
* The server owns the phase clock; client timers are display-only; late intents are rejected. All game-deciding events — plays, specialty uses, ballots, Steal guesses — are intents resolved exclusively server-side. A modified client can render anything it wants; it cannot see the Known as an Off or change a verdict.
* Every scoring and role event lands in the append-only audit stream — the same stream that feeds stats, KPIs, and report replays.

#### Designer Workbench & Tuning

Every number a playtest could question lives in one versioned file, `configs/gameplay/tuning.yaml`; structure is code, values are keys:

```yaml
seed: 42                            # reproducible dealing — same seed, same game

game:
  players: {min: 4, default: 5, max: 8}
  offs_by_table_size: {4: 1, 5: 1, 6: 2, 7: 2, 8: 2}   # public knowledge at the table
  rounds: 3
  max_extra_rounds: 1               # One More Round budget per game
  min_connected: 3
  reconnect_grace_s: 20
  pokes_per_target_per_round: 1

timers:                             # seconds; which windows may fast-forward is structure (§8)
  round_per_player: 10              # play window = value × players
  discussion_per_player: 15
  knowoff_ballot: 20                # always runs full
  knowoff_revote: 15                # always runs full
  steal: 20                         # always runs full
  prefetch_countdown: 5             # inter-round countdown = Knower prefetch budget

hand:
  size: 5
  draw_pile: 3
  specialty_weights: {pass: 0.10, reveal: 0.04, one_more_card: 0.08, shuffle: 0.05, one_more_round: 0.05}
  # shuffle deals only into Off hands, one_more_round only into Knower hands, post role-assignment

dealing:                            # relevance mesh (⚙️ §2) — v1 placeholders, tuned via simulate
  band_high: 0.55
  band_low: 0.30
  min_high_per_known: 2
  min_distant_per_known: 2
  steal_lineup_size: {1_off: 4, 2_off: 6}

scoring:
  correct_suspect: 1
  knower_win_bonus: 1
  off_survival: 3
  steal_success: 3

economy:                            # 💰 — v1 placeholders
  free_daily_hosted_games: 0        # 0 = unlimited; a config knob, not a redesign, if it ever tightens
  unlock_prices: {custom_avatar: 1000, poke_style: 400}

portal:
  min_account_level_to_submit: 2
  submissions_per_contributor_per_day: 10
```

---

## 🌐 Network Architecture: Server-Authoritative WebSocket

```text
[ Flutter app  (Android/iOS) ] ─┐                        ┌──────────────────┐
[ Flutter PWA  (any browser) ] ─┼─( WSS / JSON intents )►│  GO GAME SERVER  │◄──►[ Redis ]
[ Flutter app / PWA … seat N ] ─┘  ◄─(role-scoped events)│  - Rooms & auth  │      routing/presence
                                                         │  - Phase timers  │◄──►[ PostgreSQL ]
             [ CDN / Object Storage ]◄─(signed GETs,     │  - Role secrecy  │      profiles/results/
               media-pack assets      Knowers only)      │  - Media dealing │      media metadata
                                                         │  - Vote & Steal  │◄──►[ Object Storage ]
                                                         └──────────────────┘      pack assets
```

* Protocol: one persistent WebSocket per client. Intents: `join_room`, `lock_play`, `use_specialty`, `draw_cards`, `cast_ballot`, `steal_guess`, `ready`, `poke`, `report_media`. Events: `phase_started`, `role_assigned` (private), `round_started` (role-scoped Known/decoy payload), `show`, `round_resolved` (attributed plays), `shuffle_occurred` (anonymous), `extra_round_added` (attributed), `knowoff_resolved`, `steal_resolved`, `session_scored`. Versioned JSON with sequence numbers for ordered replay.
* Fair arbitration: play windows, ballots, and the Steal are blind-simultaneous — collected privately, resolved at window close. No game-deciding event is a fastest-tap race.
* Reconnect: session-token snapshot rejoin (Game Rules §7), role-scoped like everything else.
* Lobby→node affinity: every room lives on exactly one node (Redis maps `room_id → node`); no cross-node game state — the property that makes horizontal scaling trivial later.
* Live game state in server memory only; PostgreSQL for durable outcomes; Redis for routing/presence; object storage for assets.

---

## 🤖 Bots: Development & Testing

* `tools/gamebot` (Go CLI) spawns N bot players over the real WebSocket protocol — same intents, same timers, no server backdoors. Card policy reuses `server/internal/media` as a library: play by noisy embedding preference as a Knower; play plausible-band cards blind as an Off; draw when the hand scores badly; ballot by a noisy suspicion heuristic; Steal-guess by lineup similarity. Seeded — a failing game replays exactly.
* Bots act early and Ready immediately: a full 8-seat dev game crosses every phase in well under a minute of wall clock. Solo development against 7 bots is the daily loop; scripted integration tests and load tests ride the same harness.
* Configuration: a `bots:` block exists only in `local.yaml`/`staging.yaml`; the key is absent from `prod.yaml` and the server refuses bot connections when unset — production isolation by configuration shape. **No public-facing bots at v1** (Quick Play backfill is deferred with the mode itself, 🎮 §2).

---

## 📦 Infrastructure & Deployment

### 1. Containerization Principles

* Every runnable ships as a container from day one: `server` (distroless static Go binary), `postgres`, `redis`, `minio`, dev tooling (migrations runner, adminer). Multi-arch images, environment-agnostic — the same image under Compose now and Kubernetes later; behavior differs only by mounted config.
* Stateless-by-design server: durable state in PostgreSQL, coordination in Redis, assets in object storage, live games in memory with room→node affinity.

### 2. Centralized Configuration (No Scattered Env Vars)

* All config in `configs/` as layered YAML: `base.yaml` holds every key with sane defaults; `local/staging/prod.yaml` are thin overlays. The server loads one merged typed struct at boot and fails fast listing missing/invalid keys.
* Environment variables do exactly two jobs: selecting the config file (`KNOWOFF_CONFIG=…`) and injecting secrets (DB password, JWT key, object-storage credentials, moderation/embedding API keys) via `${VAR}` interpolation. Secrets never live in YAML files or images.
* Maps 1:1 onto Compose volumes now, ConfigMaps + Secrets later, Terraform templating after that.

### 3. Local Development — Docker Compose

* `deploy/compose/docker-compose.yaml` brings up the full stack in one command: server (live-reload in dev profile), PostgreSQL with auto-migrations, Redis, MinIO with seeded dev pack, adminer. Profiles: `core`, `tools`, `test`.
* A full 8-player game — six `gamebot` seats plus two real clients (one native, one PWA) — must be playable against the local stack with zero cloud dependencies, including media prefetch from MinIO.

### 4. Kubernetes & Terraform Readiness (Future, Designed-For Now)

* `/healthz`, `/readyz` (checks Postgres/Redis/object-storage), Prometheus metrics on a separate port; graceful shutdown drains in-flight games on SIGTERM; WebSocket routing later uses sticky sessions at the ingress — rooms never span nodes, so no mesh, no distributed state layer.
* `deploy/terraform/` as provider-agnostic modules; Hetzner-first hosting strategy, CDN in front of object storage from the first public release.
* Rule: no k8s/TF-blocking decisions in application code — no local file writes, no in-container state, no hardcoded hostnames.

---

## 💰 Monetization Systems

**The prime directive: the guest is never the customer.** Party games grow at the table; anything that stops a seated guest from playing is growth-negative. All monetization is host-side and content-side.

### 1. Joining Is Free, Forever

No caps, no gates, no ads between a guest and a game — joining any room, base or premium-themed, costs nothing and never counts against anything. This is the structural fix over the original sketch's daily cap, which punished the exact moment word-of-mouth happens.

### 2. Hosting the Base Pack Is Free (and Uncapped at Launch)

Hosting with the core pack is free and unlimited; `economy.free_daily_hosted_games: 0` (unlimited) is a config knob so any future tightening is a tuning decision, not a redesign. Content, not access, is the product.

### 3. Theme Packs — the Host Pass Loop

* Curated media packs (humor verticals, seasonal packs, community pack highlights) sold as one-time purchases via platform billing.
* **Host Pass: only the host needs to own a pack for the whole table to play it.** Guests never pay — the pack sells itself to future hosts by being played.
* **Pack Trial via rewarded ad:** a free host may watch one server-verified ad (SSV — the ad network's servers call our verification endpoint; the client callback grants nothing) to unlock one premium pack for a single session. The funnel from taste to purchase, ad-free for everyone who's paid.

### 4. Premium Membership (Monthly)

Exactly two benefits: **no ad surfaces anywhere** (trials auto-granted) and **all theme packs while subscribed**. Standalone pack purchases remain permanent and independent — two clean lanes, membership neither includes nor discounts them retroactively.

### 5. Game Currency & Cosmetics

One soft currency (bulk packs via platform billing; earned trickles can attach later) funds standalone unlocks: **Poke Styles** (visual + haptic + sound sets — the vibration-pattern idea, rebuilt to work on every platform including the vibration-less PWA) and the **Custom Avatar** unlock (👤 §2). PostgreSQL wallet with an append-only ledger; debits atomic with entitlement writes; prices in `tuning.yaml`.

---

## 🧾 Product Baseline (v1 Decisions)

* Matchmaking & discovery: private rooms with 6-character codes + QR deep links only. No public browser, no queue, no skill rating (🎮 §2).
* Progression: one server-side XP track (game completed, correct suspects, Off survival); levels gate portal submission rights and cosmetic unlocks. Values in `tuning.yaml`.
* Compliance: anonymous device accounts by default, optional linking later; privacy notice at first launch; **13+ age gate plus per-pack age ratings** (edgy meme content is rated content); Google UMP consent before any personalized ad; in-app delete-my-data backed by a server endpoint; purchases exclusively through platform billing (Play Billing / StoreKit); contributor license grants stored with submissions.
* Analytics: no third-party client SDK — the authoritative server witnesses every event; nightly jobs derive KPIs (retention, games per room per night, Off win rate by table size, Steal conversion, pack attach rate) from the audit stream.
* Moderation & admin: nickname profanity filter, conduct + media reports, Admin Console actions (kick, ban, close room, avatar/media takedown) — all live before public launch.
* Platforms & release: **Android native + Web PWA first** (the PWA guarantees no table is ever blocked by a guest's platform — including iPhones — from day one), **iOS native fast-follow** once retention is proven. CI builds all three targets from day one, so iOS remains a release decision, not a porting project. App-size budget enforced in CI: packs stream, binaries stay lean.

---

## 🛠️ Step-by-Step Implementation Lifecycle

### Phase 1: Foundation — Monorepo, Containers & Config

* Task 1: Establish the monorepo tree; implement the layered YAML config loader in `server/internal/config` with fail-fast validation.
* Task 2: Stand up Compose (server skeleton with `/healthz`–`/readyz`, Postgres + migrations, Redis, MinIO); scaffold the Flutter client with `GameTransport` and a WebSocket echo loop building for Android **and** Web from day one.
* Testing Criteria: `docker compose up` yields a healthy stack from a fresh clone; both client targets connect and echo; config errors are clear and complete.

### Phase 2: Media Engine & Pipeline

* Task 1: Implement the pack bundle format, the object-storage layout, `tools/mediapack` (ingest → screen → tag → embed → certify → bundle → simulate), and the in-memory pack loader + precomputed band lists in `server/internal/media`.
* Task 2: Ship the **Media Workbench** (server-rendered, dev-only at this phase) and produce the seed pack on the local GPU: target **≥150 certified Knowns and ≥1,500 cards** post-curation, with keep-rate measured.
* Task 3: Client `media/` module: pack metadata OTA sync, signed-URL prefetch, LRU cache, decoy renderer.
* Testing Criteria: `mediapack simulate` proves deal feasibility at all table sizes; certification rejects a deliberately band-starved pack; a client cold-starts, syncs the pack, and prefetches a round's Known inside the countdown budget on a mid-range phone.

### Phase 3: Realtime Game Loop

* Task 1: Room lifecycle, versioned intent/event protocol with sequence numbers, session-token reconnect snapshots, **role-scoped payload rendering** (the security-critical piece — built and reviewed first). `tools/gamebot` alongside — this phase's tests depend on it.
* Task 2: The phase state machine: role assignment, constraint dealing, 3 blind round windows with attributed reveal, draws, specialty resolution (Pass / Reveal with hidden specialty backs / One More Card / anonymous Shuffle re-deal / attributed One More Round with inverse-mesh Known), timeout auto-discard, Empty handling, discussion, Knowoff ballot + revote + tie rules, the Steal with mesh-near lineups, session scoring, Ready/Poke.
* Task 3: The Flutter screens and phase state against the live protocol, on native and PWA.
* Testing Criteria: a scripted 8-bot game completes with forced mid-round disconnects/reconnects and no state corruption; **a protocol-level assertion proves no Off connection ever received a Known id or URL in any phase**; a second Shuffle in one game is rejected; tie → revote → persistent-tie → Off-win resolves deterministically; a full-catch game always offers the Steal and a partial catch never does.

### Phase 4: Accounts, Persistence & Hardening

* Task 1: Auth (JWT sessions) binding anonymous tokens to accounts; profiles, game-result persistence, session scoreboards, report & feedback endpoints; Redis presence and room→node routing.
* Task 2: Harden the intent pipeline: rate limiting, deadline enforcement, protocol-boundary validation, structured audit logs to PostgreSQL as the analytics event stream; nightly KPI and stats jobs.
* Testing Criteria: a deliberately modified client (forged plays, late ballots, replayed messages, Off requesting Known assets) alters nothing and triggers audit trails; a `gamebot` load test sustains hundreds of concurrent rooms on one node with prefetch traffic against MinIO.

### Phase 5: Monetization, Admin & Launch Polish

* Task 1: Platform billing, the currency wallet + append-only ledger, theme packs with Host Pass enforcement at room creation, the SSV rewarded Pack Trial, Premium Membership entitlements, Poke Styles, Custom Avatar upload + moderation pipeline.
* Task 2: Admin Console over Phases 4–5 endpoints (case queues, pack dashboard, Daily Known calendar, economy lookups); Daily Known feed; the how-to-play clip once UI is final.
* Task 3: Performance passes: client paints on low-end devices (dark theme, flat fills), server allocation/GC under room load, CDN cache behavior on pack releases.
* Testing Criteria: a guest joins any premium room free; a non-owning host can't create one except via a verified trial; ad-completion grants only via SSV; debits are atomic with entitlements; takedowns propagate in the next pack version and hit the CDN correctly; console actions land in the audit trail immediately.

### Phase 6: Contributor Portal & Community

* Task 1: Promote the Workbench codebase to the public **Contributor Portal**: contributor accounts and levels, submission pipeline with license-grant capture, curator review queue in the Admin Console, credits rendering in pack manifests and profiles.
* Task 2: The Weekly Known Challenge (schema, scheduled jobs, announcement surfaces) as the portal's recurring engine.
* Testing Criteria: a submission traverses `draft → published` with every transition audited; rejected and taken-down media never reaches a pack; challenge windows open/close on schedule with one-submission enforcement; a published community pack version hot-swaps into live rooms between games.
