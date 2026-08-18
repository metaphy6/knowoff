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
| 0 — Agent framework & project docs | — | — | ✅ landed (pre-roadmap) |
| 1 — Foundation | 12 | 0 | 🟡 next up |
| 2 — Media Engine & Pipeline | 9 | 0 | ⚪ planned |
| 3 — Realtime Game Loop | 18 | 0 | ⚪ planned |
| 4 — Accounts, Quick Play & Hardening | 11 | 0 | ⚪ planned |
| 5 — Noin Economy, Admin & Launch Polish | 12 | 0 | ⚪ planned |
| 6 — Contributor Portal & Community | 5 | 0 | ⚪ planned |

Phase 0 — the agent operating framework, [`BLUEPRINT.md`](../../BLUEPRINT.md),
and this roadmap — carries no checkboxes; it landed before phase work began.

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
`adr-writing` (any deviation from the blueprint stack gets an ADR).

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

- [ ] Monorepo tree per 🏛️ taxonomy: `client/`, `server/`, `deploy/`, `tools/`, `content/`, `configs/`; the blueprint's three founding ADRs (server-authoritative over P2P, Flutter everywhere, home-server-first behind Cloudflare) recorded in `docs/design/`.
- [ ] `configs/`: `base.yaml` + `local/staging/prod.yaml` overlays and `gameplay/tuning.yaml` seeded with the blueprint's v1 values; every key documented in-file; secrets only ever as `${VAR}` references, never literals.
- [ ] `server/internal/config`: layered-YAML loader → one typed struct, `${VAR}` secret interpolation, fail-fast validation listing **all** missing/invalid keys in one pass **and rejecting unknown keys** (an overlay typo must fail loudly, never silently default) — table-driven unit tests covering every negative path.
- [ ] `server/cmd/knowoffd` skeleton: config load; structured JSON logging with per-connection ids; panic-recovery middleware (a handler panic never kills the process); graceful shutdown on SIGTERM (`/readyz` flips first, connections close cleanly); `/healthz`, `/readyz` (Postgres/Redis/storage checks — dependency loss degrades to not-ready, never a crash loop); Prometheus metrics (build info, connection + goroutine gauges) on a separate port.
- [ ] Migration discipline in `server/migrations`: versioned up/down pairs run by an auto-migrations runner; a fresh database migrates to head and a re-run is a no-op — enforced in CI from the first migration onward.
- [ ] `deploy/compose`: one-command stack — server (dev live-reload), Postgres + migrations runner, Redis, MinIO (bucket bootstrap only; the dev pack seed arrives with Phase 2's fixture packs), adminer, `cloudflared` under the `edge` profile; healthcheck-gated startup order; profiles `core`/`tools`/`test`/`edge`; named volumes `pg_data`/`redis_data`/`minio_data`.
- [ ] Volume snapshot/restore drill, scripted next to the compose files: `pg_dump`, Redis RDB snapshot, `mc mirror` → restore onto fresh volumes with verified parity — the 📦 §2 migration runbook rehearsed before any data matters.
- [ ] Flutter scaffold: `core/config` client config loader (server URL, feature flags) + `core/network` `GameTransport` abstraction and WebSocket implementation — connect / backoff-reconnect / clean-close contract tests plus an echo round-trip; builds for Android **and** Web PWA.
- [ ] Localization foundation (Product Baseline): Flutter ARB/`intl` catalogs with locale negotiation and English-root fallback, ICU plurals + locale-aware number/date formatting, text-expansion-tolerant layout rules, supported-locale list in config (`configs/base.yaml → localization:`); the day-one wire rule — the server never sends display text, only stable ids/codes + parameters the client localizes; pseudo-locale CI gate failing on any hardcoded user-facing string.
- [ ] `make` targets for build / test / lint of both stacks — `golangci-lint` + `gofmt` and `dart analyze` + `dart format` as gates — documented in `README.md`.
- [ ] CI from day one (Product Baseline): lint + unit tests + client builds for Android, iOS, and Web + multi-arch server image per commit; app-size budget enforced in CI (packs stream, binaries stay lean).
- [ ] Gate: Phase 1 proof tests pass on a clean tree.

---

## Phase 2 — Media Engine & Pipeline

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

- [ ] Pack bundle format (⚙️ §1): `manifest.json` (pack tag, **format version**, checksums, license + attribution per asset, age rating, **BCP 47 language tag**, **pinned embedding model + version**), `media.jsonl`, `cards.jsonl`, content-hash-addressed assets in object storage — packs are language-scoped, so new languages ship as new packs, never format changes.
- [ ] `tools/mediapack` stages: `ingest` (transcode, EXIF strip, perceptual-hash dedupe) → `screen` (provider-swappable automated moderation) → `tag` + `embed` → `certify` → `bundle` → `publish` → `simulate` (deal feasibility **and** offline balance questions: band-threshold sweeps, Donower-survival proxies, Shuffle/Revote impact, expected per-match Noin); every stage seeded and deterministic — same inputs + seed reproduce byte-identical bundles — with meaningful non-zero exits for CI use.
- [ ] `server/internal/media`: in-memory pack loader with checksum verification (a tampered bundle is refused and the current pack keeps serving), between-matches hot-swap that never blocks a live room, precomputed per-media band candidate lists (zero embedding math in the hot path), and the signed-URL issuer — short-lived, single-round, expiry enforced server-side.
- [ ] Certification gate: full band coverage per Nown at 6 players, every card reachable in some band, Monte Carlo deal feasibility at both sizes, **manifest completeness** (license, attribution, age rating, language tag) and one consistent embedding model per pack — TDD'd against a band-starved fixture pack.
- [ ] Versioned fixture packs committed for CI (a tiny golden pack + the band-starved pack): the test fuel every later phase reuses — gamebot matches, load tests, client cache tests, compose dev seeding.
- [ ] Media Workbench (server-rendered, dev-only): ingest-folder watch (ComfyUI / Ollama output), bulk keep/kill grid with tone buckets + per-batch keep-rate, embedding nearest-neighbor sanity view, deal simulator.
- [ ] Seed pack on the local GPU: **≥150 certified Nowns, ≥1,500 cards** at ⚙️ §3 quality targets; every generation run logged with model, seed, and params (reproducible curation input); license + attribution recorded for copyleft-sourced media; tone rubric landed in `content/tone-matrix.md`.
- [ ] `client/lib/media`: pack metadata OTA sync (app start + unrecognized tag), signed-URL prefetch with retry/backoff on flaky networks (URLs short-lived, single-round), hash-verified LRU asset cache under an explicit size budget (corrupt entries evicted, never rendered), Donower placeholder renderer — also shown while a Nower's asset is still loading, so loading state leaks nothing (⚙️ §4).
- [ ] Gate: Phase 2 proof tests pass on a clean tree.

---

## Phase 3 — Realtime Game Loop

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
- [ ] Gate: full BLUEPRINT Phase 4 testing criteria pass (forged-client suite, backfill behavior, quantified load test, OAuth second-device restore, leaderboard guards).

---

## Phase 5 — Noin Economy, Admin & Launch Polish

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

**Proof tests.** BLUEPRINT Phase 5 testing criteria — headline: Noin
grants land instantly and survive a mid-match disconnect; points
conversion is atomic, one-way, and cap-counted, rejected below 100 points,
never touches Overall Points, and parallel conversions of the same balance
credit exactly once; the ledger admits no update or delete, every debit is
atomic with its entitlement write, and after a fuzzed storm of grants,
spends, and conversions every balance equals its ledger sum; a replayed
SSV callback or store receipt grants exactly once; a bot-heavy match under
`noin_min_humans` grants no team-win Noin; the 11th free Quick Play match
of the day is rejected at queue time while a Local Room still opens; a
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
- [ ] Play Passes (1/3/7-day, priced in Noin — `economy.play_pass_prices`): **unlimited Quick Play while active**, lifting the free daily cap (`economy.free_daily_quickplay_matches`, 10 at launch; Local Rooms never capped) — **Play Passes never remove ads**; **Premium** — the one cash subscription, monthly / yearly (yearly −20%, `premium_yearly_discount_pct`) via platform billing — is the **sole ad-removal path and includes unlimited Quick Play**, so a subscriber never needs passes.
- [ ] Noin bulks (`economy.noin_bundles`) via platform billing — **the only place money buys Noin: everything money can get, play can also get, slower** — with server-side receipt verification and idempotent grants keyed by platform transaction id (a replayed receipt grants exactly once; refunds/chargebacks revoke via an explicit audited admin action); SSV rewarded post-match doubler — callbacks signature-verified and replay-proof, the client callback grants nothing; Premium subscribers get the doubling automatically, ad-free; theme packs (`economy.unlock_prices.theme_pack`) with Host Pass enforcement — only the room creator needs the pack in private/local rooms, Quick Play runs core + free rotating featured pack; Poke Styles + Custom Avatar unlocks priced in Noin (`economy.unlock_prices`).
- [ ] Economy balance pass (⚙️ Tuning): every price and earn value read from `tuning.yaml` only — economy tuning never needs a client release (proof: a config price change reflects in the Store with no rebuild); numbers tuned against the balance-protocol table, targets first — expected per-match Noin from `mediapack simulate` vs the real numbers from the nightly KPI jobs, one lever at a time.
- [ ] Custom Avatar upload pipeline (👤 §2): server-side crop to 256×256 WebP, EXIF strip, size cap, automated moderation screen before display, admin takedown reverting to presets without refund.
- [ ] Player reports (👤 §3) + feedback (👤 §4): one-tap conduct/media reports, rate-limited, repeat reports collapsing into one case, feeding the case queues; in-app feedback form with consented context snapshot into Postgres triage.
- [ ] Client surfaces on native + PWA: Store (`NoinBadge`, bulks, passes, packs, cosmetics), NoticeInbox + dismissible notice banners (`system_notice` live + HTTPS fetch on start, hard-maintenance countdown), post-match SSV doubler flow.
- [ ] Admin Console (🛡️) on the internal port — 2FA-gated, RBAC-scoped, CSRF-protected, unreachable through the public ingress, every action writing an append-only audit row: conduct + media case queues (bans hit live connections immediately), Guard-freeze reviews, pack dashboard, leaderboard ops, economy ledger, feedback triage, system-notice composer — compose, schedule, localize, withdraw — with automatic matchmaking drain (🎮 §4).
- [ ] How-to-play clip (≤45 s, 7 beats, captions on the lime highlighter sweep per 🎨) captured on final production UI — ship gate; player-facing help text + store copy derived from the blueprint's Game Rules (deliberately the only rulebook).
- [ ] Launch passes: low-end client paint budget, server allocation/GC under queue load, edge-cache hit rates on pack release, **backup/restore + VPS migration runbook executed with verified data parity** (row counts + checksums across Postgres/Redis/MinIO — 📦 §2; Cloudflare R2 free tier as the asset-offload option), store review prep (age gate, per-pack age ratings, UMP consent, privacy notice at first launch, in-app delete-my-data, **store listings + clip captions localized for every launch locale**); app icon + store art via the curated nano banana raster-batch pipeline (🎨 asset strategy: matrix-derived prompts → human curation → consistency pass).
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

**Proof tests.** BLUEPRINT Phase 6 testing criteria: audited immutable
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
2. The phase's *Proof tests* pass on a clean tree.
3. `make doctor` exits 0.
4. The status snapshot at the top of this file has been updated.
5. The phase's run produced one or more `commit` tracking rows whose
   `[run-id]` trailers all appear in `git log`.
6. The roadmap still matches BLUEPRINT.md — any drift discovered during the
   phase was resolved in the same pass (lockstep rule, Appendix A).
7. Every proof in the phase's *Proof tests* exists as an automated test
   where automatable; manual drills (device traces, migration rehearsals,
   store submissions) are recorded as `action=note` tracking rows with
   their evidence.
