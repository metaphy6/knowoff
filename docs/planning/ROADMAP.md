# 🗺 ROADMAP — Knowoff

> Single source of truth for sequenced work in this repository. Derived
> from — and kept in lockstep with — [`BLUEPRINT.md`](../../BLUEPRINT.md)
> §"Step-by-Step Implementation Lifecycle": **a feature change lands
> together with its roadmap update**, and a phase is done only when its
> test gate passes. Agents implement phase-by-phase, drain every `[ ]`
> bullet in scope, append tracking rows, then stage.
>
> Emoji section references (🏛️ ⚙️ 🎮 💰 🧑‍🎨 👤 📦 🌐 🛡️) point into
> [`BLUEPRINT.md`](../../BLUEPRINT.md) — the normative spec for every rule,
> value, and format named below.

## 📊 Status snapshot

Update with `make roadmap.status` (parses the `[ ]` / `[x]` boxes below).

| Phase | Items | Done | Status |
|---|---|---|---|
| 1 — Foundation | 9 | 0 | 🟡 next up |
| 2 — Media Engine & Pipeline | 8 | 0 | ⚪ planned |
| 3 — Realtime Game Loop | 17 | 0 | ⚪ planned |
| 4 — Accounts, Quick Play & Hardening | 10 | 0 | ⚪ planned |
| 5 — Noin Economy, Admin & Launch Polish | 11 | 0 | ⚪ planned |
| 6 — Contributor Portal & Community | 5 | 0 | ⚪ planned |

## 🧭 Guiding principles

- **Server-authoritative always.** The client renders state and sends
  intents; it never decides an outcome, never computes a balance, never
  receives data its role shouldn't see.
- **Role secrecy is the one unforgivable failure.** Nown must never reach a
  Donower's (or eliminated player's) device — enforced server-side, proven
  by protocol-level tests, re-proven on every change to payload rendering.
- **Every tunable number lives in `configs/gameplay/tuning.yaml`.**
  Structure is code; values are keys. Economy and balance tuning must never
  need a client release.
- Smallest change that turns the test green; tests move with code in the
  same commit.
- Reversible > impressive. One owner per phase; one tracking `run_id` per
  implementation pass.

## 🚫 Non-goals (v1) — locked by BLUEPRINT

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

## 🧰 Skills discipline — how agents work this roadmap

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

## 📑 Table of contents

- [Phase 1 — Foundation](#phase-1--foundation)
- [Phase 2 — Media Engine & Pipeline](#phase-2--media-engine--pipeline)
- [Phase 3 — Realtime Game Loop](#phase-3--realtime-game-loop)
- [Phase 4 — Accounts, Quick Play & Hardening](#phase-4--accounts-quick-play--hardening)
- [Phase 5 — Noin Economy, Admin & Launch Polish](#phase-5--noin-economy-admin--launch-polish)
- [Phase 6 — Contributor Portal & Community](#phase-6--contributor-portal--community)
- [Appendix A — Tracking conventions](#appendix-a--tracking-conventions)
- [Appendix B — Definition of done](#appendix-b--definition-of-done)

---

## Phase 1 — Foundation

**Scope id.** `phase-1`

**What.** The monorepo skeleton and the run-anywhere substrate: the
BLUEPRINT codebase taxonomy on disk, the layered YAML config loader with
fail-fast validation, a Go server skeleton with health endpoints, the full
local Docker Compose stack, and a Flutter client scaffold (`GameTransport`
+ WebSocket echo) building for **Android and Web from day one**.

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
`adr-writing` (any deviation from the blueprint stack gets an ADR).

**Test plan.** `docker compose up` from a fresh clone yields a healthy
stack; both client targets connect and echo; config errors are clear and
complete; `make doctor` exits 0.

- [ ] Monorepo tree per 🏛️ taxonomy: `client/`, `server/`, `deploy/`, `tools/`, `content/`, `configs/`; the blueprint's three founding ADRs (server-authoritative over P2P, Flutter everywhere, home-server-first behind Cloudflare) recorded in `docs/design/`.
- [ ] `configs/`: `base.yaml` + `local/staging/prod.yaml` overlays and `gameplay/tuning.yaml` seeded with the blueprint's v1 values.
- [ ] `server/internal/config`: layered-YAML loader → one typed struct, `${VAR}` secret interpolation, fail-fast validation listing **all** missing/invalid keys — unit-tested.
- [ ] `server/cmd/knowoffd` skeleton: config load, graceful shutdown on SIGTERM, `/healthz`, `/readyz` (Postgres/Redis/storage checks), Prometheus metrics on a separate port.
- [ ] `deploy/compose`: one-command stack — server (dev live-reload), Postgres + auto-migrations, Redis, MinIO (seeded), adminer; profiles `core`/`tools`/`test`/`edge`.
- [ ] Flutter scaffold: `core/network` `GameTransport` abstraction + WebSocket implementation with an echo round-trip; builds for Android **and** Web PWA.
- [ ] `make` targets for build / test / lint of both stacks, documented in `README.md`.
- [ ] CI from day one (Product Baseline): client builds for Android, iOS, and Web + server image per commit; app-size budget enforced in CI (packs stream, binaries stay lean).
- [ ] Gate: Phase 1 test plan passes on a clean tree.

---

## Phase 2 — Media Engine & Pipeline

**Scope id.** `phase-2`

**What.** Everything that decides what media exists and how hands are dealt:
the pack bundle format, the `tools/mediapack` CLI, the server-side pack
loader with precomputed relevance-band lists, the dev-only Media Workbench,
the first certified seed pack (**≥150 Nowns / ≥1,500 cards**), and the
client media module (OTA metadata sync, prefetch, LRU cache, Donower
placeholder renderer).

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

**Test plan.** `mediapack simulate` proves deal feasibility at both table
sizes; certification rejects a band-starved pack; a cold-start client syncs
the pack and prefetches a round's Nown inside the `prefetch_countdown`
budget on a mid-range phone.

- [ ] Pack bundle format (⚙️ §1): `manifest.json` (pack tag, checksums, license + credits, age rating), `media.jsonl`, `cards.jsonl`, content-hash-addressed assets in object storage.
- [ ] `tools/mediapack` stages: `ingest` (transcode, EXIF strip, perceptual-hash dedupe) → `screen` (provider-swappable automated moderation) → `tag` + `embed` → `certify` → `bundle` → `publish` → `simulate` (deal feasibility **and** offline balance questions: band-threshold sweeps, Donower-survival proxies, Shuffle/Revote impact, expected per-match Noin).
- [ ] `server/internal/media`: in-memory pack loader, checksum verification, between-matches hot-swap, precomputed per-media band candidate lists (zero embedding math in the hot path).
- [ ] Certification gate: full band coverage per Nown at 6 players, every card reachable in some band, Monte Carlo deal feasibility at both sizes — TDD'd against a band-starved fixture pack.
- [ ] Media Workbench (server-rendered, dev-only): ingest-folder watch (ComfyUI / Ollama output), bulk keep/kill grid with tone buckets + per-batch keep-rate, embedding nearest-neighbor sanity view, deal simulator.
- [ ] Seed pack on the local GPU: **≥150 certified Nowns, ≥1,500 cards** at ⚙️ §3 quality targets; license + attribution recorded for copyleft-sourced media; tone rubric landed in `content/tone-matrix.md`.
- [ ] `client/lib/media`: pack metadata OTA sync (app start + unrecognized tag), signed-URL prefetch (URLs short-lived, single-round), LRU asset cache, Donower placeholder renderer — also shown while a Nower's asset is still loading, so loading state leaks nothing (⚙️ §4).
- [ ] Gate: Phase 2 test plan passes on a clean tree.

---

## Phase 3 — Realtime Game Loop

**Scope id.** `phase-3`

**What.** The heart of the product: room lifecycle, the versioned
intent/event protocol, reconnect snapshots, **role-scoped payload
rendering**, the full phase state machine (turn-based play → discussion →
Knowoff → verdict) with all five specialties and every fairness rule,
`tools/gamebot`, the Soft Neo-Brutalism design system (🎨), and the Flutter
match screens on native + PWA.

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

**Test plan.** BLUEPRINT Phase 3 testing criteria — headline: **a
protocol-level assertion proves no Donower or eliminated connection ever
received a Nown id or URL in any phase**; scripted bot matches complete at
both table sizes with forced mid-round disconnects/reconnects and no state
corruption; out-of-turn and off-role intents are rejected and the turn
order re-randomizes every round; design gates — token/primitive/press-motion
goldens, the guardrail audit (zero blur, single permitted gradient, no
stacked translucency), the diacritics render check, and the low-end
frame-budget trace on Round and Knowoff.

- [ ] Role-scoped payload rendering: Nower → `{nown: {id, signed_url, type}}`, Donower/eliminated → `{decoy: true}` — built first, security-reviewed before dependents land.
- [ ] Room lifecycle: exactly 4/6 seats, 6-character codes + QR deep links (into the native app if installed, the PWA otherwise — a guest is never blocked), seat reservation, 20 s reconnect grace, session-token snapshot rejoin (role-scoped); session scoreboard across a room's matches; room→node affinity (Redis `room_id → node`, no cross-node game state).
- [ ] Versioned JSON protocol with sequence numbers — the full intent/event set from 🌐.
- [ ] `tools/gamebot`: N seeded policy bots over the real WebSocket protocol, no server backdoors; a 6-seat dev match crosses every phase in under a minute; external bot connections refused whenever the `bots:` config block is absent — and it never exists in `prod.yaml`.
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
- [ ] Flutter match flow on native + PWA against the live protocol: MainMenu, Queue, Lobby, Round, Discussion, Knowoff, Verdict — `RoleCard` (press-and-hold role check), `NownStage`, `HandFan`, `PlayTable`, `VoteBoard`, `QuickChatBar`, `ReadyButton`, `PokeNudge`.
- [ ] Gate: full BLUEPRINT Phase 3 testing criteria pass, including the no-leak protocol assertion, plus the 📦 §4 criterion: a full 6-player match — four `gamebot` seats + one native client + one PWA client — playable against the local stack with zero cloud dependencies.

---

## Phase 4 — Accounts, Quick Play & Hardening

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

**Test plan.** A deliberately modified client (forged plays, late votes,
replayed messages, Donower requesting Nown assets) alters nothing and
triggers audit trails; a short queue backfills with labeled bots after
`queue_timeout_s` and never starts below `min_humans`; a `gamebot` load
test sustains hundreds of concurrent rooms on one node; a sign-in restores
progress, Noin, and entitlements on a second device.

- [ ] Auth: anonymous device accounts → JWT sessions; Google Sign-In / Facebook Login one-tap registration + account linking (provider subject id + email stored privately, never shown).
- [ ] Profiles + public stats (👤 §1) derived nightly from the audit stream; Non-Converted Points visible to the owner only; nickname profanity filter; free preset avatar gallery (👤 §2) — the first curated **nano banana (Gemini image) raster batch** per the 🎨 asset strategy: prompts derived from the design matrix, candidates → human curation → consistency pass → committed like any asset; API keys under the 📦 §3 config discipline.
- [ ] XP progression (Product Baseline): one server-side track (matches completed, correct votes, Donower survivals); levels gate portal role applications and cosmetic unlocks — values in `tuning.yaml`.
- [ ] Quick Play FIFO queues per room size (core pack + rotating featured pack) with reconnect-safe seat reservation and escalating abandon cooldowns.
- [ ] Backfill bots (`server/internal/bots`): 🤖 badge + reserved nicknames, human seat priority, `min_humans` floor, per-match randomized personality parameters (no farmable tell), per-queue sunset by fill-time measurement, economy + leaderboard guardrails (🎮 §1 — bot seats earn nothing).
- [ ] Beta ingress (📦 §2): `cloudflared` publishes `play.<domain>` (WebSockets) + `cdn.<domain>` (assets) — no open ports, no exposed home IP, TLS at the edge; Cache-Everything + long-TTL rule on content-hashed assets so the edge absorbs media traffic.
- [ ] Intent-pipeline hardening: rate limiting, deadline enforcement, protocol-boundary validation, structured audit log to PostgreSQL as **the** analytics event stream.
- [ ] Weekly Leaderboard (🎮 §5): Quick Play only, `leaderboard_min_humans` + daily counted cap, top-100 + own rank (ties share a rank), immutable weekly history; nightly stats/KPI jobs.
- [ ] Client surfaces on native + PWA: Profile (public stats, owner-only Non-Converted Points) and Leaderboard screens, themed per the design system.
- [ ] Gate: full BLUEPRINT Phase 4 testing criteria pass (forged-client suite, backfill behavior, load test, OAuth second-device restore, leaderboard guards).

---

## Phase 5 — Noin Economy, Admin & Launch Polish

**Scope id.** `phase-5`

**What.** The business and the ops board: the full Noin economy (wallet,
append-only ledger, instant grants, points conversion, Play Passes, the
Premium subscription, Noin bulks, SSV ad doubler, theme packs, cosmetics),
the player-facing trust surfaces (custom avatars, reports, feedback, the
notice inbox), the Admin Console, System Notices with the maintenance
drain, the how-to-play clip, and the launch performance passes + VPS
migration.

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

**Test plan.** BLUEPRINT Phase 5 testing criteria — headline: points
conversion is atomic, one-way, and cap-counted; the 11th free Quick Play
match of the day is rejected while a Local Room still opens; ads disappear
only under Premium; a scheduled maintenance window notifies, drains, and
never kills a live match; takedowns propagate and invalidate at the edge.

- [ ] `server/internal/economy`: Noin wallet + append-only ledger, instant per-event grants with `daily_earn_cap`, discreet crediting (no public per-match Noin surface — Rules §6).
- [ ] Overall / Non-Converted Points accrual + points→Noin conversion: 100:1, multiples of 100, one-way, atomic, cap-counted, Overall Points untouched.
- [ ] Play Passes (1/3/7-day, Noin) gating the free daily Quick Play cap; **Premium** subscription via platform billing (monthly / yearly −20%) as the sole ad-removal path.
- [ ] Noin bulk IAP via platform billing; SSV rewarded post-match doubler (client callback grants nothing; Premium subscribers get the doubling automatically, ad-free); theme packs with Host Pass enforcement; Poke Styles + Custom Avatar unlocks.
- [ ] Custom Avatar upload pipeline (👤 §2): server-side crop to 256×256 WebP, EXIF strip, size cap, automated moderation screen before display, admin takedown reverting to presets without refund.
- [ ] Player reports (👤 §3) + feedback (👤 §4): one-tap conduct/media reports, rate-limited, repeat reports collapsing into one case, feeding the case queues; in-app feedback form with consented context snapshot into Postgres triage.
- [ ] Client surfaces on native + PWA: Store (`NoinBadge`, bulks, passes, packs, cosmetics), NoticeInbox + dismissible notice banners (`system_notice` live + HTTPS fetch on start, hard-maintenance countdown), post-match SSV doubler flow.
- [ ] Admin Console (🛡️) on the internal port: conduct + media case queues (bans hit live connections immediately), Guard-freeze reviews, pack dashboard, leaderboard ops, economy ledger, feedback triage, system-notice composer — compose, schedule, localize, withdraw — with automatic matchmaking drain (🎮 §4).
- [ ] How-to-play clip (≤45 s, 7 beats, captions on the lime highlighter sweep per 🎨) captured on final production UI — ship gate; player-facing help text + store copy derived from the blueprint's Game Rules (deliberately the only rulebook).
- [ ] Launch passes: low-end client paint budget, server allocation/GC under queue load, edge-cache hit rates on pack release, **VPS migration runbook executed** (📦 §2; Cloudflare R2 free tier as the asset-offload option), store review prep (age gate, per-pack age ratings, UMP consent, privacy notice at first launch, in-app delete-my-data); app icon + store art via the curated nano banana raster-batch pipeline (🎨 asset strategy: matrix-derived prompts → human curation → consistency pass).
- [ ] Gate: full BLUEPRINT Phase 5 testing criteria pass.

---

## Phase 6 — Contributor Portal & Community

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

**Test plan.** BLUEPRINT Phase 6 testing criteria: audited immutable
submission lifecycle; entry #101 rejected until a screening rejection
reopens a slot; unscreened entries never visible or votable; second votes
and vote changes rejected; Guard freezes suspend within seconds, auto-expire
at `guard_freeze_max_h`, and only an admin converts them to bans; the weekly
close is atomic (title + payout + transfer at next close).

- [ ] Promote Workbench → public Contributor Portal: role applications with admin grants (Contributor / Curator / Guard), every role action audited and reversible.
- [ ] Curator toolchain: Curator Guide (derived from ⚙️ §2–3), Nown + deck authoring with the deal simulator; submission pipeline with terms-consent capture (version + timestamp) and credits + Noin rewards on acceptance — submissions immutable once submitted; withdraw + resubmit is the only correction path and it costs the queue slot.
- [ ] Guard freeze flows wired to the Admin Console case queue: timeboxed ≤ `guard_freeze_max_h`, one active freeze per Guard per target, auto-expiry, admin-final dismiss / timed ban / permanent ban.
- [ ] Weekly Nown Challenge end-to-end (🎮 §3): Monday topic publication, in-app entries (first-100 cap, rejection-reopened slots, immutable once submitted, per-entry consent stored with terms version + timestamp), pre-vote screening queue, open live tallies with one immutable vote and no self-votes, atomic weekly close (Week Winner title + `challenge_winner` payout + optional community-pack inclusion); challenge scheduler + contribution-terms versioning land in the Admin Console (🛡️).
- [ ] Gate: full BLUEPRINT Phase 6 testing criteria pass.

---

## Appendix A — Tracking conventions

- One `run_id` per implementation pass through a phase.
- `scope` column on every row = the phase id (e.g. `phase-3`).
- One `action=commit, status=completed, commit_sha=pending` row per logical
  commit, with the `summary` in Conventional Commits format. See
  [`docs/tracking/tracking.schema.md`](../tracking/tracking.schema.md).
- A feature change to [`BLUEPRINT.md`](../../BLUEPRINT.md) and its roadmap
  update land in the **same commit** — the lockstep rule.

## Appendix B — Definition of done

A phase is **done** when:

1. Every `[ ]` bullet under its heading is `[x]`.
2. The phase's *Test plan* passes on a clean tree.
3. `make doctor` exits 0.
4. The status snapshot at the top of this file has been updated.
5. The phase's run produced one or more `commit` tracking rows whose
   `[run-id]` trailers all appear in `git log`.
6. The roadmap still matches BLUEPRINT.md — any drift discovered during the
   phase was resolved in the same pass (lockstep rule, Appendix A).
