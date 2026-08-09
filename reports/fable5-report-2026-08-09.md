# Knowoff — Honest Assessment Report

**Date:** 2026-08-09 · **Reviewer:** Claude Fable 5 · **Inputs reviewed:** [BLUEPRINT.md](../BLUEPRINT.md), [Monetization.md](../Monetization.md), [Infra-and-algo.md](../Infra-and-algo.md), [Example-blueprint.md](../Example-blueprint.md)

**Revision v2:** updated for the owner's stack decision — **Flutter for the mobile apps, PWA for desktop.**

---

## 1. Executive Summary

**Verdict: the core concept is genuinely good and worth building — but the current documents are a concept sketch, not a blueprint, and three specific decisions would actively hurt the game if left as-is.**

The idea — hidden-role social deduction where the hidden information is *access to a multimedia prompt*, with card play as the bluffing medium — sits in a proven genre (The Chameleon, A Fake Artist Goes to New York, Spyfall) and adds two real innovations: multimedia prompts with subjective humor as the deduction substrate, and the algorithmic tag-mesh hand dealing that mathematically shields the Offs. The QR-join flow and the local GPU asset pipeline are both smart, cost-aware calls. **Stack (v2):** the client is now Flutter mobile apps plus a PWA for desktop — sanely implemented as one Flutter codebase with a Flutter Web desktop build. This resolves v1's iOS haptics blocker and makes the Example blueprint's client architecture directly reusable, at the cost of reintroducing install friction for party guests (§6.2).

The three decisions that need deliberate correction or mitigation before they calcify:

1. **The 3-games/day free cap will kill the game's growth loop.** Party games spread through group sessions; a cap that locks a guest out mid-party punishes exactly the moment word-of-mouth happens. (§5)
2. **Audio media breaks the game's core secret in a same-room setting**, and the "completely dark screen" is a physical tell. The hidden-information design needs a same-room privacy pass. (§4)
3. **Native mobile apps reintroduce the install friction your own Infra doc calls fatal.** The Flutter decision is sound — it fixes haptics and unlocks blueprint reuse — but a party guest must now download an app before joining, and that needs an explicit onboarding mitigation plan. (§6.2)

Beyond those, the rules document has roughly a dozen unspecified mechanics (the Shuffle card literally has no defined effect) that must be resolved before the Uydurum-style blueprint can be written. The good news: the Example blueprint you plan to copy from is an excellent normative standard, and with the Flutter decision most of its *client and server architecture* now transfers directly — only the backend technology choice remains open (§8).

---

## 2. Scorecard

| Area | Grade | One-line summary |
|---|---|---|
| Core game concept | **B+** | Proven genre + real innovation; fun loop is plausible but unproven |
| Rules completeness | **D** | Sketch-level; ~12 critical mechanics unspecified |
| Same-room information design | **D+** | Dark screen, audio media, and shoulder-surfing all leak the secret |
| Monetization model | **C−** | Right instincts (host pass, theme packs), one destructive choice (daily cap) |
| Monetization projections | **F / absent** | No pricing, no conversion assumptions, no numbers at all |
| Tech stack direction | **B** | Flutter mobile + desktop PWA fixes haptics and aligns with the blueprint; guest install friction and the hosting error (Vercel+WS) are the open items |
| Content pipeline plan | **B−** | Hardware and tools are realistic; curation effort underestimated |
| Balancing algorithm (tag mesh) | **B+** | The right idea; implementation should probably be embeddings, not literal tags |
| Readiness to write the blueprint | **Not yet** | Resolve §9's open questions first |

---

## 3. What Is Genuinely Strong

1. **The genre bet is sound.** "One player can't see the prompt and must blend in" is the mechanically proven core of The Chameleon and Fake Artist. Replacing word-clues/drawings with *card plays against multimedia* is a real differentiator, and the subjective-humor angle (a Know with weird taste looks like an Off) creates the table-talk that makes these games work. That last insight — humor subjectivity as noise that protects Offs — is the best design thought in the documents.
2. **The tag-mesh dealing ratio (2 high / 2 distant / 1 chaos) is the load-bearing mechanic**, and you correctly identified it as such. It guarantees every hand is ambiguous, which is what makes the Off's blind play survivable and the Know's odd play suspicious. This is the "mechanical shield" done right.
3. **QR-code joining is the correct distribution model** for a party game — the Jackbox lesson absorbed. With the v2 stack (Flutter mobile apps + desktop PWA) you trade some of the original zero-install purity for native haptics, better media performance, and direct reuse of the Example blueprint's client architecture; the friction cost and its mitigations are covered in §6.2.
4. **Local generation on your own RTX 4080 (12 GB)** for the asset library is cost-smart and feasible: SDXL/Flux-Schnell for images, AnimateDiff for short loops, a local 7–8B LLM for text prompts all run fine in 12 GB. Your content marginal cost approaches zero, which is exactly what a theme-pack business wants.
5. **Public draw visibility** (everyone sees who drew how many cards) is good information design — drawing becomes a costly signal ("my hand doesn't fit"), which feeds the deduction loop for free.
6. **No mid-game eliminations** (single vote at the end) keeps everyone playing the whole game — better party pacing than Werewolf-style kill-offs.

---

## 4. Game Design — The Critical Gaps

### 4.1 The same-room information problem (highest design risk)

The game protects **two secrets at once**: **(a)** the Offs must not perceive the media — that is their handicap — and **(b)** nobody at the table may learn who the Offs are — that is the deduction. All players sit around one physical table, so anything a phone *emits* — sound or light — is shared with the whole room. Three concrete leaks follow:

- **Audio media leaks secret (a) — broken outright.** If the central media is a sound clip, it plays out loud from every Know's phone speaker. Sound is not private: the Off hears the clip along with everyone else, is no longer blind, and can pick matching cards like a Know. The only technical fix is all players wearing headphones, which kills a party. **Recommendation: cut audio, and video-with-sound, from v1 media types.** Images, GIFs, silent video, and text render privately on each screen; sound cannot.
- **The "completely dark screen" leaks secret (b) — it visually marks the Off.** Phones at a table sit in everyone's peripheral vision. During the media reveal, every Know's screen glows with content while the Off's is pitch black — one sideways glance identifies the Off with zero deduction. The screen state itself broadcasts the very role the game is built on hiding. **Recommendation: show the Off a *decoy screen*** — same layout, same brightness, a plausible placeholder where the media would be — so that from normal table distance an Off's phone is indistinguishable from a Know's. The Off still knows their own role (told privately at round start); onlookers learn nothing.
- **Shoulder-surfing the media leaks it to the Off.** The Chameleon keeps its secret on a tiny card; you're putting a full-screen GIF on up to 7 phones — an Off who glimpses any neighbor's screen is un-blinded. Partially mitigable by hand-privacy social norms (players know this from card games) plus deliberately small media rendering, but it should be a stated design constraint, not an accident.

### 4.2 Unspecified mechanics (blocking issues for the blueprint)

In rough priority order:

1. **The Shuffle card has no defined effect.** The doc specifies its rarity, holder restriction, trigger timing, and secrecy — but never says what it *does*. Shuffles whose hands? Rotates hands between players? Reshuffles played cards? This is the game's signature Off tool and it is currently a name.
2. **Are card plays sequential or simultaneous, attributed or anonymous?** This decides the whole game. If plays are visible as they happen, Offs can wait and copy the table's vibe (and the timer of 10s × players implies sequential turns); if simultaneous-blind-reveal, round 1 is nearly a coin flip for the Off. Voting "based on suspicious card choices" implies plays are attributed — say so. *(Recommendation: simultaneous lock-in, simultaneous attributed reveal — it's latency-fair and matches the Example blueprint's blind-window pattern.)*
3. **The voting system is three sentences.** Plurality or majority? Tie handling? With 2 Offs, must Knows catch both? One vote each, or vote-per-Off? What are the win/scoring outcomes for partial catches (1 of 2 Offs)?
4. **Off win condition & comeback mechanic.** Offs win by surviving — fine. Consider the genre's proven balance device: a voted-out Off gets a last-chance *media guess* (pick the real media from a 4-option lineup) to steal the win. It makes wrong-but-close Off play rewarding and softens the blowout feel.
5. **How is the 1-vs-2 Off count decided?** Random? Scaled by player count? Random-and-hidden is actually the stronger choice (uncertainty about Off count is free tension) — but it must be a stated rule.
6. **Specialty-card dealing leaks roles.** Shuffle is Off-only and One More Round is Know-only. If a Reveal card exposes a hand containing either, the holder's role is *proven*. Either exclude specialty cards from Reveal, render them as generic backs, or deal them outside the hand (role-attached abilities, not cards).
7. **Card exhaustion is possible and unhandled.** 5 hand + 3 draw = 8 max; 3–4 plays; Type-A specialties each burn an extra discard; auto-discard penalties drain further. A player can reach a round with zero cards and no legal action. Define the rule (forced pass? public "empty" marker?).
8. **Where does discussion happen?** The Poke section references "tense debates," but no phase hosts them. During play timers? A dedicated debate window before voting? Social deduction lives on structured talk time — the Example blueprint timeboxes every phase; do the same, and add an explicit pre-vote discussion window.
9. **Timer scaling rationale.** 10s × player count only makes sense for sequential turns. If plays are simultaneous, a fixed 30–45s round with Ready-unanimity fast-forward (copy Uydurum §7 wholesale) is strictly better.
10. **Reveal card's actual value is unclear.** Because the tag mesh gives *everyone* an ambiguous hand — including Offs — a revealed hand looks the same for both roles. That's either an elegant trap (Reveal is deliberately weak, a noob tax) or a broken card. Decide which, on purpose.
11. **Meta-scoring across games.** A party session is 5–10 games. Is there any cross-game score, streak, or session winner? (Uydurum's chip economy has no analog here yet — you don't need one as deep, but a session scoreboard is near-mandatory for party stickiness.)
12. **Disconnect/reconnect is entirely absent.** Native mobile apps soften this versus a browser, but phone locks and app switches still suspend clients constantly at a party table (and the desktop PWA keeps the tab-kill problem). What happens when an Off's phone locks mid-round? Copy Uydurum §6's model (grace window, passive auto-play, rejoin snapshot) — it transfers almost verbatim.

### 4.3 Fun-proof status

Nothing here has been playtested, and this design is ~90% testable **with zero code**: a physical deck of index cards, a laptop showing media, and a "close your eyes" Off selection. **A paper prototype is the single highest-value next action** — before the blueprint, before the stack decision. It will answer #2, #3, #8, and #10 above in one evening.

---

## 5. Monetization — Honest Teardown

### 5.1 What the documents actually contain

To be blunt: there are **no projections** here — no price points, no conversion assumptions, no volume estimates, no LTV/CAC thinking. This is a monetization *model sketch*. That's fine at this stage, but it should be labeled as such, and the blueprint stage should add at least placeholder numbers in a tuning config (the Example blueprint's `economy:` block is the pattern to steal).

### 5.2 The daily cap is the plan's most dangerous decision

Three games/day ≈ **20 minutes of play** (a game is ~3 rounds × ~60–80s + voting ≈ 5–7 min). The failure mode is concrete: six friends are at a table, it's game four of the night, and two phones say *"come back tomorrow."* The whole table's session dies — at the exact moment the game was selling itself to five potential new installs.

Structural problems with the cap, independent of its size:

- **Party games are occasion-driven, not habit-driven.** Usage is spiky (weekends, gatherings), so a *daily* cap doesn't even monetize the actual usage pattern — a session cap or host-based model would. Your own document claims the model is "friendly to large groups" while the cap does the opposite; the Host Pass paragraph even celebrates that joining a friend's premium room burns your daily allowance ("gently pushing" — it isn't gentle, it's a mid-party lockout).
- **It taxes guests, but the natural payer is the host.** Jackbox proved the shape: the organizer pays, guests are free forever, and the organizer persona (the friend who brings the games) converts willingly. Squeeze hosts, never guests.
- Even your Example blueprint — a game with *daily solo-queue* usage where caps make more sense — set the free cap at **10/day** and marked it "expected to tighten as the base grows." Starting a party game at 3 is starting at the end state.

**Recommended restructure:**

1. **Joining is always free, forever.** No cap of any kind on guests. This is the growth loop; do not touch it.
2. **Cap free *hosting*** (e.g., 2 free hosted games/day, or 1 free session/day) — or make base hosting free and gate premium theme packs + quality-of-life host features.
3. Keep **Theme Packs + Host Pass** exactly as designed — that part is right.
4. **Premium = host-side unlimited + all packs** (one-time "party box" purchase or a cheaper subscription; for a content-pack game with your generation pipeline, one-time base + à-la-carte packs + optional "all packs" subscription is the natural ladder).
5. Consider **rewarded ads as the free-tier relief valve** (watch one ad to host another game) — notably absent from your docs; decide deliberately whether that's a brand choice or an oversight. The Example blueprint's SSV (server-side verification) section is the correct implementation pattern if you do it.
6. Poke vibration patterns as IAP: viable again on native mobile (§6.2), but still garnish — cosmetic-tier revenue, not a pillar; desktop players can't feel them, so sell *poke styles* (visual + haptic) rather than vibration alone.

---

## 6. Tech Stack — What's Right and What Breaks

### 6.1 The revised stack: Flutter mobile + desktop PWA

**Decision (v2): native Flutter apps on iOS/Android, PWA for desktop** — sanely implemented as one Flutter codebase whose Web build serves as the desktop PWA. Consequences:

- The React/Vue + Tailwind plan in [Infra-and-algo.md](../Infra-and-algo.md) is superseded; update that document so the stack has one source of truth.
- **What this buys:** full native haptics on both platforms (resurrecting Poke as designed — §6.2), better media/GIF performance than a mobile browser, push notifications, and near-verbatim reuse of the Example blueprint's client architecture (§8).
- **What this costs:** party guests must install an app (§6.2), two store review pipelines, and a Flutter Web desktop build needing its own QA pass (canvas rendering quirks; no vibration on desktop, so desktop Poke is the screen-shake variant).
- **Server-authoritative mindset** in the original docs: correct instinct — and the stack change makes the Example blueprint's authoritative WebSocket server the natural companion (§6.3).

### 6.2 Haptics resolved; install friction is the new open item

**Poke is un-blocked.** The v1 report flagged Poke as broken on iPhones because the Web Vibration API has never worked in iOS Safari. Native Flutter apps have full haptic access on both platforms, so the Poke's vibration effect — and the monetized vibration patterns — are viable again on mobile. Two residual notes: pair haptics with a visual effect anyway (the Example blueprint's Poke is "light haptic buzz **and a brief screen shake**") so the desktop PWA and silenced phones still feel it, and treat pattern IAPs as garnish revenue (§5.2).

**The trade: install friction.** [Infra-and-algo.md](../Infra-and-algo.md)'s own opening argument was *"a group of friends sitting at a table will not wait for everyone to download a heavy 200MB app"* — and the new stack asks exactly that of guests. Manageable, but only deliberately:

1. **App-size budget from day one.** A lean Flutter release build lands in the tens of MB, not 200 — keep all media out of the binary (stream packs from storage/CDN; the Example blueprint's OTA bundle-sync pattern is the template) and track install size in CI.
2. **QR deep-link flow:** scan → store page → install → app opens directly into the host's lobby (deferred deep link). A one-time cost per guest, near-zero at every later party.
3. **iOS App Clips are worth a spike:** a size-capped instant experience launched straight from the host's QR code — the closest native equivalent to the original PWA vision. Flutter inside an App Clip is possible but size-constrained; prototype before promising it.
4. **Fallback web join — decide explicitly:** letting a guest without the app join from their phone's browser (a thin join page, or the Flutter Web build with a slow-load warning) preserves the never-block-the-party guarantee at the cost of a second QA surface. If you cut it, accept that an app-less guest sits out a game while downloading.

### 6.3 The hosting contradiction: Vercel cannot run your realtime server

"Supabase Realtime **or Socket.io**" + "Node.js hosted on **Vercel** or Render" contains a real error: **Vercel's serverless/edge functions do not host persistent WebSocket servers** — a Socket.io game server cannot run there. Pick a coherent pair:

- **Path A — Supabase-centric:** Supabase Realtime (channels/presence/broadcast) + Postgres + Edge Functions. Caveat: Edge Functions are stateless and short-lived — **nobody is running your authoritative 10-second round timer**. You end up encoding phase deadlines in DB rows with clients racing to trigger transitions, or scheduled-function hacks. Workable for a casual same-room game, but it's fighting the platform.
- **Path B — dedicated stateful server (recommended):** one Go process (per the Example blueprint — now the natural fit alongside your Flutter client; Node also works) on Render/Fly/Railway holding lobbies in memory, owning phase timers, and speaking WebSockets — exactly the architecture you already have fully specified. Supabase can remain as Postgres + auth + storage/CDN for media. This is the honest fit for server-owned timers, secret role assignment, and blind-window resolution.

Related, non-negotiable server rule regardless of path: **the media URL must never be sent to Off clients.** Role secrecy must be server-side; the desktop PWA's dev tools are open to anyone curious, and a native app's traffic can be proxied. Stakes are low (same-room casual), but this one is free to do right.

### 6.4 Platform lifecycle realities to design around now

- **Backgrounding still exists on native.** A locked phone or an app switch suspends the app and can drop the socket — gentler than a browser tab, but constant at a party table. Server-owned clocks + reconnect snapshot (Uydurum §6/network model) remain mandatory, not optional.
- **Keep screens awake during rounds:** trivial on native (wakelock plugin); Wake Lock API on the desktop PWA. Small thing, big party-UX win.
- **Desktop PWA specifics:** browsers block unmuted media autoplay (moot once audio media is cut — §4.1); tabs get killed and site data can be evicted, so client state stays disposable and sessions resume from a server snapshot.

### 6.5 Content pipeline — realistic, with two corrections

- Hardware/tooling claims check out (SDXL/Flux-Schnell, AnimateDiff/SVD short loops, Ollama-hosted 7–8B for text — all fit 12 GB with standard quantization). Short *video* is your weakest generator; lean GIF/image/text for v1.
- **Curation is the real cost, and it's underestimated.** AI humor keep-rates for the "not corny" bar you've set run maybe 10–30%; the tone-matrix is a good rubric but a human (you) still reviews every shipped asset. Budget: to launch with, say, 300 media + 3,000 cards, expect to generate 3–10× that volume and hand-review all of it. Plan a `mediapack` curation CLI mirroring Uydurum's `dictpack` (batch-generate → auto-tag → human accept/reject → bundle version). That tool is the actual heart of your content business.
- **Moderation/compliance gap:** AI-generated edgy meme content + 13-to-25 audience needs an age gate, a content-rating pass per pack, and a report mechanism (copy Uydurum's Profiles & Community §3 and compliance baseline).

### 6.6 The tag mesh: right idea, probably the wrong mechanism

Literal tag intersection ("3+ shared tags") only works with a **closed, controlled vocabulary** (~100–300 canonical tags) applied consistently across thousands of assets — a curation burden that grows with every pack, and noisy tags silently break the 2/2/1 hand guarantee, which is your core balance mechanic.

**Simpler and better: embeddings.** Embed every card and media once (any local sentence-embedding model; CLIP for images), and define the relevance spectrum as cosine-similarity bands (high ≥ *a*, distant *b*–*a*, chaos < *b*). You get:

- No tag vocabulary to govern; new packs are automatically compatible.
- Continuous tunable thresholds (`a`, `b` live in a tuning YAML — steal Uydurum's `configs/gameplay/tuning.yaml` discipline).
- Keep tags only as human-facing pack metadata and for themed filtering, not as the balance mechanism.

Precompute per-media candidate lists offline (Uydurum's branching-factor precompute is the exact analogous pattern) so dealing is an O(1) lookup at runtime.

---

## 7. Risk Register (ranked)

| # | Risk | Severity | Mitigation |
|---|---|---|---|
| 1 | Core loop isn't actually fun (unproven) | Critical | Paper prototype this month, before any code |
| 2 | Same-room secret leakage (dark screen, audio, shoulder-surf) | Critical | Decoy screens; cut audio media; small media rendering |
| 3 | 3-game daily cap kills word-of-mouth growth | Critical | Guests free forever; monetize hosts |
| 4 | Guest install friction (native app at a party table) | High | App-size budget, QR deferred deep links into the lobby, App Clip spike, explicit web-fallback decision |
| 5 | Rules gaps (Shuffle undefined, voting underspecified, role-leaking specialty cards) | High | Resolve §9 checklist before blueprint |
| 6 | Vercel/Socket.io architecture mismatch | High | Dedicated stateful server (Render/Fly) or all-in Supabase; decide once |
| 7 | Content curation workload underestimated | Medium | Build the `mediapack` curation CLI first; measure keep-rate early |
| 8 | Tag-quality noise breaks hand balance | Medium | Switch to embedding-similarity bands |
| 9 | App/tab lifecycle (locked phones, desktop tab kills) corrupting live games | Medium | Server-owned timers + reconnect snapshots from day one |
| 10 | No compliance/moderation plan for edgy AI content + young audience | Medium | Age gate, pack ratings, report flow — copy Uydurum's baseline |

---

## 8. Using the Uydurum Example Blueprint

The Example blueprint is an excellent normative standard — single-source rules, every edge case resolved, config-driven tuning, phased lifecycle with testing criteria. Aspire to that bar. But copy with judgment:

**Transfers almost verbatim (steal freely):**

- Server-authoritative architecture & the intents/events WebSocket protocol (§4 anti-cheat, network diagram)
- Blind-simultaneous windows resolved at close (their draft/flag pattern → your card plays and endgame vote)
- Disconnects & dropouts model (§6): grace window, passive auto-play, rejoin snapshot, no-bot-takeover
- Pace controls (§7): Ready unanimity + Poke (theirs already includes the screen-shake hedge you need)
- Config discipline: one `tuning.yaml` for every number a playtest could question
- Bots: dev/test bots + labeled Quick Play backfill (your cold-start problem is *worse* — you need 4+ simultaneous same-room… actually your game is same-room-first, so backfill matters less at launch; dev bots still matter for testing)
- Product baseline: compliance, moderation, reports, admin console, analytics-from-server-events
- Monetization plumbing: entitlements, wallet/ledger atomicity, SSV for rewarded ads
- The `dictpack` tool philosophy → your `mediapack` (generate/tag/curate/bundle/simulate)

**Does NOT transfer automatically — decide deliberately:**

- **The backend (the stack is otherwise now aligned).** With the v2 Flutter decision, Uydurum's client architecture transfers nearly verbatim: the `core/network` `GameTransport` abstraction, DTO mirroring of the server protocol, Riverpod phase state, the widget taxonomy, and the OTA content-bundle sync (their dict-pack discipline → your media packs). What remains genuinely open is the backend: Uydurum's Go + Postgres + Redis versus your original Node + Supabase sketch. Either works; the Go shape is now the lower-friction copy, and Supabase can persist as Postgres/auth/storage under either (§6.3). Note also that Uydurum has no desktop target — your Flutter Web build is new ground the blueprint won't cover.
- **The word engine and dictionary pipeline** — no analog. Your analog is the media/tag/embedding pipeline, which deserves the same §-level rigor in your blueprint.
- **Chip economy & progressive stakes** — Knowoff has no in-match economy; don't inherit one. A light session scoreboard is likely all you need (§4.2-11).
- **Their monetization values** (10/day cap, currency unlocks) — the *plumbing* transfers, the *model* must be rebuilt host-centric (§5.2).

---

## 9. Open Questions to Resolve Before Writing the Knowoff Blueprint

Answer all of these (one paper-prototype evening resolves half):

1. Shuffle card: exact effect?
2. Card plays: simultaneous or sequential? Attributed or anonymous? Revealed when?
3. Voting: ballot structure, majority rule, ties, partial catches with 2 Offs?
4. Off comeback mechanic (media-guess steal) — in or out?
5. Off count: fixed by player count, or random-and-hidden?
6. Specialty cards vs Reveal: how is role leakage prevented?
7. Zero-card hands: forced pass rule?
8. Discussion phase: when, how long, skippable?
9. Round timer: fixed vs player-scaled; Ready fast-forward?
10. Session meta-scoring across games?
11. Media types at v1 (recommend: image, GIF, text only)?
12. Backend decision (client settled: Flutter mobile + desktop PWA): Go + Postgres + Redis per the blueprint, or Node + Supabase? And does a mobile-browser join fallback exist for app-less guests?
13. Monetization: who pays (host vs guest), cap unit (day vs session vs hosted-game)?
14. Age gate & content rating stance for packs?

## 10. Recommended Next Steps (ordered)

1. **Paper prototype, 2+ sessions, 4–6 players.** Index cards + laptop media. Answers questions 1–10 empirically.
2. **Rewrite Monetization.md host-centric** with placeholder numbers in a `tuning.yaml`-style block.
3. **Spike the content pipeline for one week:** generate 200 media + 500 cards, measure your keep-rate, auto-tag + embed them, and hand-verify the 2/2/1 dealing feels right. This de-risks the whole content business before any app code.
4. **Decide the backend** (client is settled — Flutter mobile + desktop PWA; recommendation: one stateful Go or Node WebSocket server + Postgres, keeping Supabase for auth/storage if desired), set the app-size budget, and write an ADR paragraph like Uydurum's. Update [Infra-and-algo.md](../Infra-and-algo.md) to the new stack.
5. **Then** write the Knowoff blueprint to the Uydurum standard, porting the sections listed in §8.
6. Build vertical slice: lobby → QR join → one full 3-round game with placeholder assets → vote → result screen. No monetization, no accounts.

---

*Bottom line: strong concept, correct platform instincts, real innovation in the tag-mesh shield — undermined by an anti-growth free cap, an unfinished ruleset, a same-room privacy hole, and an install-friction question the new Flutter stack must answer deliberately. All four are fixable on paper before a single line of code, which is exactly where you are. Fix them there.*
