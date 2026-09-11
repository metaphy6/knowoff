# 🗺 Knowoff — Implementation Roadmap

> **Read [`BLUEPRINT.md`](../../BLUEPRINT.md) first.** It is the normative
> product + technical spec: game rules, tech stack, architecture, media
> engine, economy, admin console, contributor portal, and the product
> baseline. This file is the sequenced implementation plan only — phases,
> ordered checkboxes, proof tests, and gates. Emoji references (🏛️ ⚙️ 🎮 💰
> 🧑‍🎨 👤 📦 🌐 🛡️ 🎨 🎬) point at chapters in `BLUEPRINT.md`. When a phase's
> **Spec (required reading)** line names a chapter, it names a chapter in
> `BLUEPRINT.md` — read it before that phase's first bullet, not after a
> gate fails. If a bullet and a spec chapter disagree, the spec chapter
> wins; fix the drift in the same commit (Appendix A).

---

## 🗺 Roadmap — Step-by-Step Implementation Lifecycle

This chapter is the executable roadmap — the single source of truth for
sequenced work. The spec it implements lives in `BLUEPRINT.md`; this
chapter is the sequence and the checklist. Every bullet is a checklist
entry whose full
requirements live in the chapters its phase names under **Spec (required
reading)** — read them *before* implementing the phase's first bullet,
not after a gate fails. If a bullet and a spec chapter disagree, the spec
chapter wins; fix the drift in the same commit (Appendix A). A feature
change to any spec chapter in `BLUEPRINT.md` lands together with its update
to this chapter — one commit, no drift.

Agents implement phase-by-phase, drain every `[ ]` bullet in scope, append
tracking rows, then stage. Emoji references (🏛️ ⚙️ 🎮 💰 🧑‍🎨 👤 📦 🌐
🛡️ 🎨 🎬) point at chapters in `BLUEPRINT.md`.

### 📊 Status snapshot

#### Content readiness audit — 2026-09-11

The owner adopted [humor-development.md](../../content/humor-development.md)
as editorial guidance and requested an explanation of current server content
logic before creating content. The Blueprint's ⚙️ §3 now owns the standard;
the [Curator Guide](../../content/curator-guide.md) applies it. No content was
generated, certified or published in this documentation slice.

The [source audit](../code/MODULE-media-engine.md) supersedes earlier completion
claims for the affected Phase 2/3 items below. It found:

| Area | Current state | Required proof before real-content readiness |
|---|---|---|
| Production pipeline | CLI `build` generates synthetic text/vectors; `publish` copies a directory. The current fixture has 150 Nowns and 5,400 cards. | Real candidates, compatible semantic embeddings, screened/curated bundles and verified activation; the fixture is not production humor. |
| Final hand coverage | First Nown fills five hand slots; second fills three reserve slots; later selections are discarded. The fifth card can come from any band. | Check each player's retained cards against every scheduled Nown at both table sizes; reject invalid deals and make certification inspect final coverage. |
| Shuffle | Replaces hands, reserves and specialties. | Preserve reserves and intended specialty state; prove the redeal's schedule coverage and repeated-use rules. |
| Draw handling | Allows out-of-turn draws during play and broadcasts drawn identities. A test explicitly expects out-of-turn drawing. | Align handler and regression tests with 🕹️ §3: owner's turn only; public who/count announcement and private card payloads. |
| Pack hot swap | Dealing retains the old pack; payload lookup uses the global active pack. | A running match renders its original pack correctly after another pack activates. |

Current simulator success does not prove all-Nown coverage or editorial
quality. Existing passing tests do not close these gaps. The checked-in pack
is the development fixture described by ADR-005; its synthetic embedding
geometry must not be reused as semantic proof for rewritten jokes.

**Next content sequence.** Select three themes and two target cultures/languages;
apply the restored [lo-fi, GIF-first direction](../../BLUEPRINT.md#playable-media-direction),
record the media mix separately for Nowns and cards, the editorial dimensions
and experimental 70/20/10 freshness mix; draft and human-edit GIF-led candidates,
watch actual loops and inspect compressed assets at card size; check sources,
rights, originality, local meaning and age suitability; apply automated screening and human review;
playtest actual 4- and 6-player schedules; certify and activate a small pack
per language after the engineering gaps above are closed. Record recognition,
laughter, alternative explanations and references needing explanation. Review
topical records weekly, including their review/expiry dates, and revise or
retire weak content through pack versions. This pilot precedes the full
production seed-pack target; drafting does not require claiming release readiness.

**Frontend reset (2026-09-10):** the owner requested removal of the existing
Flutter UI. Historical UI completions below are superseded by this reset:
screens, widgets, design-system implementation, media rendering and their
UI-only tests were removed. The subsequent reconstruction below restores the
player interface around the retained services. Localization,
state, API/authentication, transport, media services and extracted nonvisual
action rules remain; backend/admin/portal implementations remain intact.
The owner subsequently requested a complete Flutter UI recreation using the
existing blueprint, roadmap and design docs: creative, funny, dramatic and
vibrant, with particular care to avoid animation jank. The design matrix is
the reconstruction contract. Prior UI proof gates are not evidence that the
current client is playable; the replacement checklist below tracks fresh proof.

#### Community and operations: source audit and first working slice — 2026-09-10

**Goal.** Make the existing Go-rendered Contributor Portal and internal Admin
Console usable in an ordinary browser, connect a working Weekly Nown Challenge
to Flutter, and correct completion claims that currently exceed the source.
This is the owner's request to start these interfaces and their missing backend,
not a claim that every Phase 5/6 launch deliverable can be completed in this slice.
Read the Blueprint's 🎮, 👤, 🧑‍🎨 and 🛡️ chapters as the behavior contract.

**Evidence standard.** The audit below describes the source before this slice.
Existing service methods and database tests are reusable work, but do not prove
a reachable browser or in-app journey. The broad Phase 4–6 checkboxes reopened
below retain their original acceptance wording. Historical tracking rows remain
append-only; the earlier "Phase 6 complete" row is not current completion proof.
The root coordinator updates this checklist with actual tests and runtime evidence.

| Surface | Present and reusable | Concrete gap at audit time |
|---|---|---|
| Quick Play, local rooms, profiles, leaderboard, reports, feedback | Flutter pages, API methods and matching Go services exist. Current device layouts and developer controls have separate proof above/below. | OAuth linking and account deletion have no Flutter integration; server OAuth helpers alone do not prove second-device restoration. Public profile statistics exist, but this audit does not certify every historical nightly-job claim. |
| Portal browser access | `/portal/` Go pages and role/application/submission manager methods. | `portal/handler.go` accepts only a Bearer header; normal page navigation and form posts cannot carry it. No usable browser session or form CSRF flow. |
| Admin browser access | PostgreSQL accounts/sessions, password and TOTP validation, login throttling, separate internal listener, RBAC middleware. | `admin.ValidateSession` requires a CSRF token even for GET; login sends it only in a response header, so an ordinary cookie-only redirect back to `/admin/` fails. Cookies also need an explicit secure-transport policy. |
| Contributor/Curator workflow | Role application/grant/reject/revoke, text drafts, submit/withdraw, decide/publish methods, real deal simulator, `content/curator-guide.md`, acceptance reward/profile-credit transaction. | Draft HTML has no submit/withdraw controls or explicit terms consent; consent is currently stamped automatically. No draft editing, open pack calls, Nown/deck authoring, image/GIF upload processing or automatic screening integration. `PublishSubmission` only changes a database status/pack tag; it does not build or activate a pack. Several audits are ignored after mutations commit. |
| Admin operations | Existing `/admin/portal/` routes for terms, applications, submissions, freeze review and challenge; notice create/withdraw and avatar takedown routes. `reports.ListReports`/`ListFeedback`, wallet balance and entitlement readers are reusable. | Main dashboard links only Notices. Conduct/media case triage, feedback transitions, economy lookup UI, pack operations and leaderboard operations are absent. Terms creation does not change the config-selected active terms. Existing challenge admin lists approved entries, hiding the pending screening queue. |
| Weekly Nown Challenge | Public active/entry/vote endpoints; approved-only public listing; unique entry/vote rows; approval slot locking; rejection; winner/title/payout transaction. Topic IDs point to approved portal submissions. | No Flutter API or screen. Active response hardcodes `voted: false` and lacks renderable topic content. The first-100 limit is enforced on approval instead of intake; no explicit consent; votes do not check whether the topic is still open. Week-end date handling excludes most of Sunday. No automatic publish/close scheduler or current-title transfer proof. |
| Guard enforcement | Freeze/dismiss/permanent-ban/expiry service methods and admin review routes. | No public Guard action UI/route. `frozen_by` references admin accounts although Guards are player accounts. Dismissal can clear another active suspension/ban; expiry is not continuously scheduled in the server loop. Do not enable these unsafe actions merely to fill a navigation item. |
| Notices and avatars | Notice storage/localization/broadcast-on-create, maintenance helper, Flutter banners/inbox; avatar size/crop/WebP storage and preset picker. | Notice administration is English-only; scheduled broadcast/reminder/drain behavior is not proven by the helper alone. Avatar upload stores `moderated=false`, ignores unlock failure and has no review-to-activation delivery. The full avatar pipeline remains incomplete. |
| Launch evidence | Clip storyboard and migration runbook exist. | A storyboard is not a captured clip; a runbook is not an executed parity rehearsal. The clip storyboard also still describes a blind ballot, superseded by ADR-009. Physical low-end/native proof, consent/account deletion and localized launch assets are not established by the current UI gates. |

Source references: `server/internal/{portal,admin}/`,
`server/internal/handler/challenge.go`, `server/internal/reports/reports.go`,
`server/internal/notices/notices.go`, `server/internal/avatar/avatar.go`,
`server/cmd/knowoffd/main.go`, migrations `000003`–`000005`,
`client/lib/data/{api_client,auth_service}.dart`, and the Flutter screens.
Portal database tests currently call `t.Skipf` when PostgreSQL is unavailable;
the new proof run must use an isolated migrated test database and report skips.
`TestChallenge_RaceToSlot100` currently submits 150 entries before racing
approval, so it proves the wrong boundary for the Blueprint's intake limit.

**Delivery checklist, in order.** These tasks reuse product services; they do
not replace the Go server with a SPA or duplicate game state in portal pages.

- [x] Repair internal admin browser authentication: cookie-only GET after
  password+TOTP login, server-loaded form token, CSRF on mutations, logout,
  expiry, rate limiting and separate public/admin route tests. A cookie-jar
  test must follow redirects without injecting an `X-CSRF-Token` header.
- [x] Add a bounded player-session handoff into the public portal using the
  existing authenticated account, followed by HttpOnly browser sessions and
  CSRF-protected forms. Prove expiry/revocation, invalid/replayed handoff denial,
  no token in URLs/logs, and no admin access through a portal session.
- [x] Render a shared on-brand Go page shell with truthful navigation to
  available portal/admin workspaces, accessible labels, clear errors and
  keyboard focus. Inspect ordinary browser navigation at phone, tablet and
  desktop widths; this portal remains web-only as the Blueprint requires.
- [x] Complete application and role management pages: show eligibility and
  own status; admin can review, grant, reject and revoke supported roles.
  Test server-side level/role checks and audit persistence. Curator includes
  Contributor access; Guard permissions remain independent as the Blueprint requires.
- [x] Complete the text contribution journey: draft creation/editing,
  displayed versioned terms plus explicit acceptance, submit, immutable
  submitted content, withdraw/resubmit and own history. Test missing/stale
  consent, another owner's draft, parallel daily-cap submissions and replay.
- [x] Complete internal submission review: pending queue, safe content preview,
  reject reason and approval using the existing reward/credit transaction.
  Prove one decision/reward under concurrency and durable audit behavior;
  do not present a status-only publish operation as a deployed media pack.
- [x] Connect real internal operations: existing terms, application,
  submission, challenge and notice pages; report/feedback lists and safe
  supported triage transitions; account wallet/entitlement lookup. Prove
  role/CSRF boundaries and persistent mutations. Unimplemented enforcement,
  grants/refunds, pack deployment and leaderboard tools stay explicitly pending.
- [x] Repair challenge backend admission and view state: approved topic media,
  actual own-entry/own-vote status, explicit versioned consent, bounded text,
  first-100 intake under a topic lock, rejection-reopened capacity, complete
  Monday–Sunday activity window and stable error codes. Parallel entry #100/#101
  and absence of pending content from public responses are required tests.
- [x] Repair challenge screening/voting/close: pending entries are available
  only to authorized reviewers; vote requires an approved non-self entry in an
  open topic; repeat/change/concurrent votes fail; close serializes with voting
  and admission and credits the winner once. Use controlled clocks and a real
  database for boundary/concurrency proof, not only manually seeded rows.
- [x] Add the in-app Weekly Nown Challenge and its API methods: topic, approved
  entries/tallies, own submission status, consented text submission, immutable
  vote confirmation, empty/loading/error/full/closed states and refresh.
  Follow phone/tablet/web navigation and localization conventions; hidden
  screens must not keep polling. Test API payloads, anonymous-account access,
  own-entry privacy, 204/no-topic behavior, text expansion and reduced motion.
- [x] Repair missing ARB metadata for the source and pseudo locale, require
  descriptions during localization generation and add metadata regressions.
- [x] Run the complete repository gate; prove the applicant, contribution,
  review and challenge contracts through real HTTP/database integration tests
  and Flutter interaction tests. Inspect portal/admin browser flows and player
  navigation; obtain independent review before root staging.
- [x] Build the release Web client after final changes.
- [ ] Execute the complete live-provider journey from applicant through Flutter
  entry to screened publication and payout, plus native-device checks. Local
  tests use a controlled screening provider; production credentials and real
  contribution terms are operator setup, not claimed live proof.

**Explicit follow-on scope.** Full image/GIF processing, dedupe/embeddings,
image moderation/generation, pack calls and Nown/deck authoring, real pack
publication/attribution, scheduled challenge rollover/current-title transfer,
Guard identity and suspension isolation/runtime expiry, avatar moderation and
activation, complete moderation/economy/leaderboard/pack admin actions, launch
OAuth/deletion/billing/consent integration, capture assets and external drills
remain the original Phase 4–6 deliverables. A working text/community slice does
not close these broader checkboxes. External providers need verified credentials;
local service and UI work should continue independently.

**Fresh proof (2026-09-10).** The required `python3 xops/test/tests-lints.py`
passes against the isolated, migrated `knowoff_community_gate` PostgreSQL database:
8 Python tests, all Go packages and lint, 317 Flutter tests, Flutter analysis,
and Dart formatting. Evidence:
`/tmp/agent-runs/community-full-gate-fixed--20260910T125756Z-999661.log`.
Explicit execution evidence for the five changed database packages records
130 passed test/subtest events with zero skips or failures:
`/tmp/agent-runs/community-db-execution--20260910T125858Z-1004553.log`.
The final release Web build passes (21.2 seconds):
`/tmp/agent-runs/community-release-web--20260910T125858Z-1004509.log`;
it emits a CupertinoIcons font warning, with MaterialIcons included.
The first run found an obsolete simulator heading assertion; it now requires
the exact new heading and still verifies dealt cards. Review found and fixed
stale human approval, independent Guard permissions, draft privacy, notice
audit transactions, live account eligibility and authoritative winner display.

Browser checks used a separate synthetic `knowoff_browser_qa` database and the
actual Go handlers at widths 360, 820 and 1366: editor/history, operations review,
blocked approval without a screener, rejection reaching contributor history,
immutable voting, admin closure and visible authoritative winner. The rebuilt
Flutter app at port 7357 opens the new challenge and correctly renders no topic.
Widget tests additionally cover all four device sizes, enlarged pseudo-localized
text, reduced motion, hidden-route polling and stale refresh races. The local
Docker server rebuild and migrations passed. No live provider request, real
content publication or physical native-device test was performed.

**Challenge API handoff for this slice.** Keep the three existing route paths.
`GET /api/challenge/active` returns 204 when there is no current-week topic,
otherwise an object with `topic`, approved `entries`, nullable `own_entry`,
nullable `voted_entry_id`, `terms`, `intake_remaining`, `can_submit` and
`can_vote`, plus `max_text_bytes` from the configured portal text limit.
Both server and client validate trimmed UTF-8 byte length against that value.
`topic` contains `id`, ISO dates `week_start`/`week_end`, nullable
`closed_at` and `nown: {type, content}`. `week_end` denotes Sunday; server
activity checks include the whole day. A closed current-week topic remains
viewable with both capabilities false. Topic content comes from the approved
portal submission UUID stored in `nown_media_id`, not an active-pack media ID.
Entries contain `id`, `account_id`, public `nickname`, `entry_type`, `content`, `vote_count`;
the owner's additional private entry record contains `status` and
`rejection_reason`. Terms contain `version`, `title`, `body`.

`POST /api/challenge/entry` accepts `topic_id`, `content`, `terms_version`,
`terms_accepted: true` and returns 201 with `{entry: {id, entry_type, content,
status}}`. `POST /api/challenge/vote` accepts `topic_id`, `entry_id` and returns
204. Both use the existing bearer-authenticated player account. The first
working submission type is text; image/GIF upload controls require their real
processing pipeline before appearing. Return stable `{code: ...}` errors:
`unauthorized`, `invalid_request`, `terms_required`, `terms_outdated`,
`challenge_not_open`, `challenge_full`, `challenge_already_submitted`,
`challenge_already_voted`, `challenge_self_vote`, `challenge_entry_unavailable`,
`challenge_forbidden` (403 for deleted or actively frozen accounts).
The Flutter client localizes these and treats unknown codes as a generic error.

**Ownership and risks.** Root owns Go HTML/admin operations, integration in
`main.go`, roadmap completion evidence and final tracking/staging. The auth
worker owns session/CSRF changes in the admin and portal packages. The challenge
worker owns challenge manager methods, `handler/challenge.go`, its migration and
database tests; agree on shared `portal/manager.go` boundaries before editing.
Migration `000006` is reserved for portal sessions; challenge changes use `000007`.
The Flutter worker owns challenge API/UI/tests and ARBs. Shared migrations and
manager interfaces require explicit coordination. Preserve the current game's
secrecy and private reward boundaries; portal/admin access is not a game-role
override. No live Guard ban control ships on the currently unsafe identity model.

#### Device experiences and developer tools — 2026-09-10

**Goal.** Restore all five historical debug controls and give phones, tablets
and desktop windows distinct navigation and match compositions in one Flutter
codebase. The existing brand and server-authoritative gameplay remain fixed.

**Scope and non-goals.** Presentation widgets/screens, localization, UI tests,
and narrow developer-role state/join fixes with regressions. No new game rules,
economy changes, OS installs, or backend feature invention. Historical CLI seed
administration and the future Media Workbench are separate tools.

**Files.** Shared device policy/page shell; Home/account screens; GameScreen;
debug tools and countdown widgets; GameSession/notifier dev role handling;
matching tests; CLIENT_DEV_TOOLS.md, UI_REDESIGN_PLAYBOOK.md and CHANGELOG.md.

**Checklist and proof.**
- [x] Restore debug-only freeze/resume, restart, poke echo, grant-only specialty
  picker and persistent next-match role picker. Test countdown pause, finite
  reduced-motion echo, exact grants, release gate and Random clearing.
- [x] Carry preselected roles through valid first join messages for Quick Play
  and local rooms; test no pre-join dev intent and preserved server prod gates.
- [x] Add one window-size policy and device navigation: compact phone play hub
  and bottom destinations; tablet rail/two-part hub; desktop persistent menu.
  Test sizes, navigation, forms and server-backed account actions.
- [x] Compose phone Hand/Table/People workspaces, tablet split desk and desktop
  three-pane match. Test card selection/rectangles through bot updates and
  discussion, retained workspace/draft on resize, table attribution, privacy,
  phase actions, safe areas, large text and reduced motion.
- [x] Run full repository gate, release build, independent review and batched
  browser inspection across small/large phone, tablet and desktop. Record actual
  runtime/performance evidence and native-device limits, then track and stage.

**Risks.** Shared state must stay outside device compositions; hidden panes must
not expose private semantics or run animation tickers. Keep stable card IDs and
reserved hint/action dimensions. Use bounded independent scrolling for dense
boards and allow readable scrolling at large text sizes. ADR-002's single
Flutter UI codebase now shares behavior across distinct visual compositions.

**Verification.** All 288 Flutter tests, the Python/Go checks, static analysis
and the release build passed after review fixes. Logs:
`/tmp/agent-runs/device-delivery-gate--20260910T110202Z-561609.log`
and `/tmp/agent-runs/device-delivery-build--20260910T110203Z-561761.log`.
Independent review findings were fixed with regressions: semantic workspace
activation, narrow-tablet card bounds, scrollable phone dialogs, timed-phase
workspace selection and hidden account focus. The device playbook records
browser smoke evidence and the inconclusive throttled profile trace; physical
native/low-end performance remains an explicit validation limit.

#### Flutter UI reconstruction — 2026-09-10

**Goal.** Restore the complete player-facing Flutter experience around the
preserved services, with a theatrical card-table identity and stable controls.

**Scope.** `client/lib/presentation/{theme,icons,widgets,screens}/`,
`client/lib/main.dart`, thin media rendering widgets, bundled display-font
registration, localization catalog additions, and matching UI tests. Preserve
the existing localization infrastructure, state/action rules, transport,
authentication and media services with their tests. Narrow integration fixes
carry existing runoff candidates and correct obsolete store client routes; no
gameplay or economy rules change. Avatar file selection and QR rendering are
UI dependencies, and join-route registration is client navigation.
Backend/Admin/Contributor Portal changes, new gameplay/economy rules, paid-provider integrations, and
generated raster batches are outside this reconstruction.

**Design direction.** An oversized Baloo 2 wordmark, asymmetric cream/lavender
panels, ink outlines and hard shadows, highlighter phrases, sparse doodles,
and localized comic suspense. Quick Play is the primary call to action.
Personality belongs in headings and decorative cards; forms, Nown, timers,
ballots and their hit targets remain aligned and readable. Use the eleven
existing palette tokens and their locked semantics (ADRs 007–010).

**Checklist and acceptance tests.** Each item is one independently reviewable
slice; check it only when its named assertions pass.

- [x] Restore shared tokens, typography, canvas, surfaces, buttons, status chips,
  doodles and page layout. Tests pin palette/font/shadows, 48 dp touch targets,
  keyboard focus/activation, press/hover states, and icon-plus-label status.
- [x] Restore MainMenu, Queue and Lobby: 4/6-player Quick Play, local room
  creation and 6-character join, shareable room code/deep link, visible bot and
  connection labels, cancel/leave, and navigation to account/community pages.
  Tests assert the existing API/notifier calls and server-driven transitions.
- [x] Restore private role hold/release, role-scoped Nown/identical loading
  placeholder, hand selection/lock/play, draw pile with the blueprint draw penalty
  or FREE, and attributed persistent evidence. Tests cover pointer cancellation,
  stale-media replacement, no Nown for Donower/eliminated seats, and actions
  staying disabled when prohibited by preserved state rules.
- [x] Restore Pass, Reveal target/view expiry, One More Free Card, anonymous
  Shuffle announcement, and open-ballot Revote. Tests prove correct intents,
  lockout/role/phase restrictions, free draw before normal play, and private
  revealed-hand closure using the preserved expiry state.
- [x] Restore Discussion, live Knowoff/runoff and result: server-masked/canned
  Quick Chat, Ready, Poke, live attributed vote changes, result poster, and
  eliminated spectator state. Tests prove no self/eliminated vote, mutable
  targets until resolution, Ready independent of voting, no result-window
  Revote, and no spectator chat/play/vote/poke controls.
- [x] Restore Verdict: server team outcome, retained owner match-points field,
  all Nowns after the match, and same/new-table replay. Tests prove no public
  per-match Noin or live private balance and verify both replay intents without
  inventing rewards or winning outcomes. The preserved DTO has no public
  per-seat/session scoreboard or private Noin settlement fields; those broader
  roadmap integrations remain explicitly unavailable.
- [x] Restore Profile/public profiles, Leaderboard, nickname/avatar controls,
  reports and consented-context Feedback using `ApiClient`. Tests cover
  owner-only Non-Converted Points, top/own-rank rendering, mutation success,
  loading/empty/retry states, and safe localized errors.
- [x] Restore Store and NoticeInbox/menu banners using `StoreActions` and
  `ApiClient`: server-priced passes/unlocks/conversion, catalog availability,
  maintenance timing and locale fallback. Tests prove server values
  drive prices/balances, irreversible conversion is explained before submit,
  passes never claim ad removal, and unavailable billing never claims success.
- [x] Restore all new copy through existing localization catalogs and configured
  locale selection. Sweep every screen at 360×800 and 1440×900, `en`/`en_XA`, and
  2× text scaling; assert no overflow, clipped actions, zero-height bodies,
  inaccessible labels, or unreachable primary actions (scrolling is allowed
  at large text; desktop timed play keeps controls visible); visually inspect
  release PWA screenshots and exercise launch-locale diacritics.
- [x] Prove bounded motion: press/hover and decorative entrances settle after
  one event, repeat rebuilds do not restart effects, disposed/offscreen widgets
  leave no active tickers, and reduced-motion/disableAnimations produces the
  static equivalent with identical information. Keep timers in small isolated
  subtrees, reuse static animation children, and paint expensive media/grid
  independently. No looping ambient controllers, blur, stacked opacity or
  full-screen per-frame rebuilds; the single gradient stays on the result
  reveal. Test these constraints and record profile-mode Round/Knowoff frame
  timings on the available device, with a 60 Hz target of p95 ≤16.7 ms
  and all slower samples disclosed;
  label desktop/emulator evidence honestly if no low-end device is available.
- [x] Run `python3 xops/test/tests-lints.py`, release Web build, the protocol
  no-leak tests, and a real local-stack match and restart-recovery smoke. Review the full
  combined diff and record exact available runtime/performance evidence before
  the parent appends tracking and stages. New widget tests must first fail
  against the blank shell; previous removal tests are not replacement proof.

**Fresh verification (2026-09-10).** The final repository gate passed:
193 Flutter tests, 8 Python tests, all 20 Go test packages, Go formatting/vet,
Flutter analysis and Dart formatting. It includes retained transport reconnect
and protocol/privacy tests, plus new interaction, semantics, large-text,
pseudo-locale and reduced-motion coverage. Local log:
`/tmp/agent-runs/ui-rebuild-final-gate--20260910T094730Z-274315.log`.
The release Web build passed and is served by the final preview run:
`/tmp/agent-runs/ui-delivery-preview--20260910T094044Z-253603.log`.

Live browser proof covered Quick Play, card play, live ballots, both team
verdicts, replay, a fresh queue after a local server restart, a prefilled app
join link, and the server-priced store. Phone (360×800) and desktop layouts
were inspected; final browser error/warning logs were empty. Restart recovery
was checked from a finished match, not during a live match. Native deep-link
delivery and native device builds remain unverified. Independent review found
no remaining P1/P2 issue. The [measured motion report](../guides/UI_REDESIGN_PLAYBOOK.md#2026-09-10-reconstruction-and-measured-motion)
records desktop p95 totals of 13.2/11.5/2.1 ms for Round/Knowoff/Result,
with four isolated slower state updates and explicit low-end/mobile limits.

**Interface boundaries and risks.** The existing `GameSessionNotifier`,
`GameActions`, `GameSession`, `StoreActions`, `ApiClient`, `AuthService`, and
`MediaEngine` remain authoritative integration points. At planning time,
`ApiClient` has no Weekly Challenge, OAuth-linking, delete-my-data, or native
billing methods; `AuthService` supplies anonymous session/refresh only. Those
blueprint capabilities must not be represented as working client actions
without an existing verified service. Preserve any established links or
availability messaging, and record the missing client integrations as follow-up
gaps rather than inventing endpoints or expanding backend scope. The historical
playbook's blind-vote advice is superseded by Blueprint Rules §4 and ADR-009.

Update with `python3 xops/makefile/roadmap_ops.py status` (parses the `[ ]` / `[x]` boxes in this
chapter).

| Phase | Items | Done | Status |
|---|---|---|---|
| 0 — Agent framework & project docs | — | — | ✅ landed (pre-roadmap) |
| 1 — Foundation | 12 | 12 | ✅ done |
| 2 — Media Engine & Pipeline | 9 | 3 | 🚧 incomplete — content audit reopened pipeline, certification and hot-swap proof |
| 3 — Realtime Game Loop | 18 | 14 | 🚧 incomplete — content audit reopened draw privacy, dealing, Shuffle and gate |
| 4 — Accounts, Quick Play & Hardening | 12 | 9 | 🚧 incomplete — source audit reopened unproven end-to-end claims |
| 5 — Noin Economy, Admin & Launch Polish | 13 | 6 | 🚧 incomplete — source audit reopened unproven end-to-end claims |
| 6 — Contributor Portal & Community | 5 | 0 | 🚧 incomplete — source audit reopened unproven end-to-end claims |

Phase 0 — the agent operating framework and this document — carries no
checkboxes; it landed before phase work began.

### 🧭 Guiding principles

- **The spec is normative; this chapter is the sequence.**
  Implement from `BLUEPRINT.md`'s chapters, never from bullet text alone —
  each phase's **Spec (required reading)** line names the chapters that
  must be
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
- **No voice or video chat** — free text is limited to server-masked messages
  under the configured multilingual moderation policy.
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
gate. CI is green including lint and the app-size
budget.

- [x] Monorepo tree per 🏛️ taxonomy: `client/`, `server/`, `infra/`, `tools/`, `content/`, `configs/`; the three founding ADRs (server-authoritative over P2P, Flutter everywhere, home-server-first behind Cloudflare — 🧱) recorded in `docs/design/`.
- [x] `configs/`: `base.yaml` + `local/staging/prod.yaml` overlays and `gameplay/tuning.yaml` seeded with the spec's v1 values (⚙️ Tuning); every key documented in-file; secrets only ever as `${VAR}` references, never literals.
- [x] `server/internal/config`: layered-YAML loader → one typed struct, `${VAR}` secret interpolation, fail-fast validation listing **all** missing/invalid keys in one pass **and rejecting unknown keys** (an overlay typo must fail loudly, never silently default) — table-driven unit tests covering every negative path.
- [x] `server/cmd/knowoffd` skeleton: config load; structured JSON logging with per-connection ids; panic-recovery middleware (a handler panic never kills the process); graceful shutdown on SIGTERM (`/readyz` flips first, connections close cleanly); `/healthz`, `/readyz` (Postgres/Redis/storage checks — dependency loss degrades to not-ready, never a crash loop); Prometheus metrics (build info, connection + goroutine gauges) on a separate port.
- [x] Migration discipline in `server/migrations`: versioned up/down pairs run by an auto-migrations runner; a fresh database migrates to head and a re-run is a no-op — enforced in CI from the first migration onward.
- [x] `infra/compose`: one-command stack — server (dev live-reload), Postgres + migrations runner, Redis, MinIO (bucket bootstrap only; the dev pack seed arrives with Phase 2's fixture packs), adminer, `cloudflared` under the `edge` profile; healthcheck-gated startup order; profiles `core`/`tools`/`test`/`edge`; named volumes `pg_data`/`redis_data`/`minio_data`.
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
sampling (⚙️ §2); quality targets deliberately lo-fi (⚙️ §3); license +
attribution stored in the pack manifest for every generated asset.

**Skills.** `test-driven-development` (write the certification gate against
a deliberately band-starved fixture pack first), `ai-output-stability`
(seeded, reproducible generation and dealing), `cost-aware-tool-use`
(batch and cache GPT-6 Astra generations; avoid redundant API calls —
local generation is out of scope for v1).

**Spec (required reading).** ⚙️ §1–4 in full — bundle format,
relevance mesh, pipeline & content production (quality targets; the
**content standard: the humor line** — suggestive/erotic allowed as
cartoon/drawn/abstract, never pornographic, erotic-leaning media only in
age-gated packs; the four-bucket tone rubric and keep-rate expectations),
secrecy & sync; 🧑‍🎨 §3 (Workbench); ⚙️ Tuning →
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

Real-content acceptance also requires the ⚙️ §3 editorial record and human
playtests at both sizes across complete schedules, including recognition,
laughter and plausible alternatives. A technical pass or AI score alone
cannot complete the production-content deliverable. For topical material,
retain source/observation/context/region/language/review/expiry records and
the human weekly review decision; the freshness mix is an experiment.
Review actual compressed media for the lo-fi direction and loop timing, with
GIFs leading each full Nown/card pool per ⚙️ §3. Record dimensions, encoding,
bytes, actual motion and editorial mix; a downsized polished image or a static
frame labeled as a GIF does not meet that proof. Format priority does not
alter runtime dealing weights or certify the existing synthetic text fixture.

- [x] Pack bundle format (⚙️ §1): `manifest.json` (pack tag, **format version**, checksums, license + attribution per asset, age rating, **BCP 47 language tag**, **pinned embedding model + version**), `media.jsonl`, `cards.jsonl`, content-hash-addressed assets in object storage — packs are language-scoped, so new languages ship as new packs, never format changes.
- [ ] `tools/mediapack` stages: `ingest` (transcode, EXIF strip, perceptual-hash dedupe) → `screen` (provider-swappable automated moderation) → `tag` + `embed` → `certify` → `bundle` → `publish` → `simulate` (deal feasibility **and** offline balance questions: band-threshold sweeps, Donower-survival proxies, Shuffle/Revote impact, expected per-match Noin); every stage seeded and deterministic — same inputs + seed reproduce byte-identical bundles — with meaningful non-zero exits for CI use.
- [ ] `server/pkg/media`: in-memory pack loader with checksum verification (a tampered bundle is refused and the current pack keeps serving), between-matches hot-swap that never blocks a live room, precomputed per-media band candidate lists (zero embedding math in the hot path), and the signed-URL issuer — short-lived, single-round, expiry enforced server-side.
- [ ] Certification gate: full band coverage per Nown at 6 players, every card reachable in some band, Monte Carlo deal feasibility at both sizes, **manifest completeness** (license, attribution, age rating, language tag) and one consistent embedding model per pack — TDD'd against a band-starved fixture pack.
- [x] Versioned fixture packs committed for CI (a tiny golden pack + the band-starved pack): the test fuel every later phase reuses — gamebot matches, load tests, client cache tests, compose dev seeding.
- [ ] Media Workbench (server-rendered, dev-only): issues generation batches through **GPT-6 Astra**, prioritizing lo-fi GIF loops, then still images and supporting text per ⚙️ §3; actual motion capability and delivered media must be verified. Bulk keep/kill grid with tone buckets + per-batch keep-rate, embedding nearest-neighbor sanity view, deal simulator.
- [ ] Seed pack generated via **GPT-6 Astra**: **≥150 certified Nowns, ≥1,500 cards** at ⚙️ §3 quality targets, curated to the ⚙️ §3 content standard (suggestive/erotic allowed — cartoon, drawn, abstract — **never pornographic**; erotic-leaning media only in age-gated packs) with every asset tagged into the four-bucket tone rubric for pack-mix balancing; every generation run logged with model, seed, and params (reproducible curation input); license + attribution recorded per asset; tone rubric landed in `content/tone-matrix.md`; adopt the humor-development editorial record, experimental freshness mix and actual 4/6-player pilot evidence from ⚙️ §3 before release.
- [x] `client/lib/media`: pack metadata OTA sync (app start + unrecognized tag), signed-URL prefetch with retry/backoff on flaky networks (URLs short-lived, single-round), hash-verified LRU asset cache under an explicit size budget (corrupt entries evicted, never rendered), Donower placeholder renderer — also shown while a Nower's asset is still loading, so loading state leaks nothing (⚙️ §4).
- [ ] Gate: Phase 2 proof tests pass on a clean tree.

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
rejected late intents; open live attributed ballots (ADR-009); per-round randomized
role-blind turn order with immediate attributed reveals; every rules
constant read from `tuning.yaml`; failing matches replay exactly from their
seed; the 🎨 kit (tokens, structure primitives, painters) lands before any
match screen consumes it.

**Skills.** `security-by-default` (role-scoped rendering is the critical
surface), `code-review` (secrecy code reviewed before anything builds on
it), `systematic-debugging` (seeded replay of failing matches),
`flaky-test-triage` (timers + concurrency are flake bait),
`neo-brutalism-ui-design` (the 🎨 design-system bullets below and any later
visual redesign work — background, per-component guide, guardrails).

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
the second Knowoff as a Donower win; a Revote resets an open ballot without
consuming a vote and is rejected once the result window has exposed the
eliminated player's role; a still-tied runoff counts as one survived voting
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
- [x] Room lifecycle: exactly 4/6 seats, 6-character codes + QR deep links (into the native app if installed, the PWA otherwise — a guest is never blocked), seat reservation, 20 s reconnect grace, session-token snapshot rejoin (role-scoped); session scoreboard across a room's matches; room→node affinity (Redis `room_id → node`, no cross-node game state); finished rooms torn down and memory reclaimed — no leaked room state (soak-proven).
- [x] Versioned JSON protocol with sequence numbers — the full intent/event set from 🌐 defined once, so the wire format stays stable across phases (later-phase intents like `convert_points` and `report_media` parse and reject cleanly as unavailable until their backends land); version handshake rejecting unknown protocol versions with an explicit error; unknown, malformed, or oversized frames rejected without state change; client-detected sequence gaps trigger a snapshot resync.
- [x] `tools/gamebot`: N seeded policy bots over the real WebSocket protocol, no server backdoors; a 6-seat dev match crosses every phase in under a minute; external bot connections refused whenever the `bots:` config block is absent — and it never exists in `prod.yaml`; doubles as the Phase 4 load-test engine.
- [x] Seeded match-replay harness: every match logs its seed + intent script; a replay run reproduces the byte-identical event stream — failing matches replay exactly, and failing seeds are committed as regression fixtures.
- [ ] Phase state machine: role assignment, role-blind constraint dealing, randomized per-round turn order, `timers.play_turn` turns with immediate attributed reveals (played cards stay on the table all match — the evidence votes are argued over), table-announced penalized pile draws (who, how many), timeout auto-pass + random card loss.
- [ ] Specialties (Rules §5): Pass / full-hand Reveal / penalty-free One More Free Card / anonymous once-per-match Shuffle (Donower-use-only, usable at any point in the round, sweeps the table's plays and restarts the turn order with a time bonus, draw piles untouched) / attributed once-per-match Revote (Nower-use-only, open Knowoff ballot or runoff only, never the result window) — off-role and duplicate uses rejected (dead cards remain usable as discard fodder); Reveal and One More Free Card leave the normal hand-card action open, with the free draw spent before that action; Shuffle re-deals against the current Nown schedule so the dealing guarantee survives.
- [x] Discussion window (`timers.discussion_per_player × players`, Ready fast-forward) with localizable canned Quick Chat plus server-masked free text; English is applied to every message and configured language lists apply from the active client locale. Poke once per target per phase (buzz on native, screen shake on the PWA — no vibration API; pokes show who poked whom, no score effect), cap enforced server-side.
- [x] Knowoff: open live attributed ballot — never vote for yourself, targets may change until resolution → tied-player runoff → `timers.vote_result_window` result display with elimination + role reveal. Ballot, runoff and result end early only when every connected active seat marks Ready; casting alone does not count as Ready (ADR-009). Early-end rule (votes remaining < uncaught Donowers); still-tied runoff = survived voting for Donowers; eliminated-spectator scoping (no Nown, no actions); verdict screen reveals all Nowns to everyone.
- [x] Disconnect handling per Rules §7: auto-played seats (turns pass instantly, abstain from votes, count Ready, stay votable), 20 s grace, team forfeits + scored low-population ending; match points per Rules §6 with the zero floor (absent at match end = 0 points; already-earned Noin stays).
- [x] Design tokens (🎨): the palette as Flutter constants — `canvas #DCC8F7` lavender field with its faint low-contrast grid tile (`CustomPainter`, no raster), `surface #F7F2E9` warm cream for cards and sheets, `#FFFFFF` content wells inside them, `ink #141414` for every border and every glyph (text is never gray-on-gray), `violet #B49AF5` the neutral interactive (buttons, selected tiles, timers, progress fills), `lime #D4F04C` the truth/reward signal (Nower catches, match points, Noin grants), `pink #FF9ED2` the risk/accusation signal (votes, the Knowoff board, Donower reveals); the palette's single permitted gradient `#FFD9EC → #FF9ED2` reserved for the Knowoff reveal header; light theme only at v1 — token values snapshot-tested so silent drift fails CI.
- [x] Brutalist structure primitives: shared container/button/chip widgets — every container carries `Border.all(width: 3, color: ink)` + the hard shadow `BoxShadow(color: ink, offset: Offset(4, 4), blurRadius: 0)`; corner radius 16 for cards and sheets, 12 for buttons, full pill for stat chips; the signature brutalist click — pressing collapses the shadow to zero offset while the control translates onto its own shadow footprint — golden tests per primitive, widget test on the press motion.
- [x] Typography + highlighter emphasis: chunky rounded display face for headings, timers, and Noin numbers — Baloo 2 vs Fredoka (both OFL), locked by a diacritics render check across launch locales and recorded as an ADR; plain geometric sans for body; **the lime marker sweep is the house emphasis** (`CustomPainter`) for the revealed role, Noin deltas, and the clip caption — never bold-only.
- [x] Fixed color semantics, enforced at the API level: violet = interact, lime = truth/reward, pink = accuse/risk, ink = information; no verdict leans on hue alone — verdict-bearing widgets require icon + label parameters so a color-only state cannot compile (colorblind-safe by construction).
- [x] Illustration policy — deliberately sparse: no mascot, no scene art in the match flow; the ~12-glyph single-weight doodle set (sparkle, static-burst, eye, cloud, the Donower placeholder glyphs) shipped as hand-authored SVG paths, reserved for empty states, win moments, and the Donower-side placeholder.
- [x] Performance guardrails + asset discipline: flat fills (the reveal-header gradient is the one exception), zero blur radii, no stacked translucency — enforced by a widget-tree guardrail audit test that fails on any blur, second gradient, or translucency stack, plus a frame-budget trace on a low-end device profile for the busiest screens (Round, Knowoff); UI chrome 100 % widgets/`CustomPainter` — code first, raster last (true raster arrives only via the curated GPT-6 Astra batches in Phases 4–5); the design system and media-pack content never mix.
- [x] Flutter match flow on native + PWA against the live protocol: MainMenu, Queue, Lobby, Round, Discussion, Knowoff, Verdict — `RoleCard` (press-and-hold role check), `NownStage`, `HandFan`, `PlayTable`, `VoteBoard`, `QuickChatBar`, `ReadyButton`, `PokeNudge`; every string through the localization catalogs (the wire stays ids/codes only), with a pseudo-locale sweep + text-expansion check across the match flow.
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
- [x] Profiles + public stats (👤 §1) derived nightly from the audit stream — pseudonymous, no PII on any public surface; Non-Converted Points visible to the owner only; locale-aware nickname profanity filter.
- [ ] Free preset avatar gallery (👤 §2) — the first curated **GPT-6 Astra raster batch** per the 🎨 asset strategy: prompts derived from the design matrix, candidates → human curation → consistency pass → committed like any asset; API keys under the 📦 §3 config discipline.
- [x] XP progression (Product Baseline): one server-side track (matches completed, correct votes, Donower survivals); levels gate portal role applications and cosmetic unlocks — values in `tuning.yaml`.
- [x] Quick Play FIFO queues per room size (core pack + rotating featured pack) with reconnect-safe seat reservation and escalating abandon cooldowns.
- [x] Backfill bots (`server/internal/bots`): 🤖 badge + reserved nicknames, human seat priority, `min_humans` floor, per-match randomized personality parameters (no farmable tell), per-queue sunset by fill-time measurement, economy + leaderboard guardrails (🎮 §1 — bot seats earn nothing).
- [x] Beta ingress (📦 §2): `cloudflared` publishes `play.<domain>` (WebSockets) + `cdn.<domain>` (assets) — no open ports, no exposed home IP, TLS at the edge; WebSocket heartbeat interval below the edge idle timeout, verified end-to-end through the tunnel (no silent mid-match drops); Cache-Everything + long-TTL rule on content-hashed assets so the edge absorbs media traffic.
- [x] Intent-pipeline hardening: per-connection + per-account rate limiting (Redis), deadline enforcement, protocol-boundary validation, frame-size caps, slow-consumer disconnect policy (one stalled client never blocks a room), structured audit log to PostgreSQL as **the** analytics event stream — schema-versioned from the first event.
- [x] Weekly Leaderboard (🎮 §5): Quick Play only, Monday–Sunday on the server clock, `leaderboard_min_humans` + daily counted cap, top-100 + own rank (ties share a rank), immutable weekly history; nightly stats/KPI jobs idempotent and re-runnable — a crashed or repeated job never double-counts.
- [x] Dependency-degradation drills: losing Redis or Postgres flips `/readyz` and pauses matchmaking with a clear client message while the process stays healthy; service resumes without restart when the store returns; no goroutine or connection leak across the outage (metrics-proven).
- [x] Client surfaces on native + PWA: Profile (public stats, owner-only Non-Converted Points) and Leaderboard screens, themed per the design system.
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

- [x] `server/internal/economy`: Noin wallet + append-only ledger — append-only enforced at the database level (no update/delete path on ledger rows), every balance always the replayable sum of its ledger with a nightly reconciliation job proving it and alerting on drift; instant per-event grants with `daily_earn_cap` and the team-win guard — no team-win Noin below `liquidity.noin_min_humans` humans (🎮 §1); property tests — no sequence of grants, spends, and conversions can go negative or double-credit; discreet crediting (no public per-match Noin surface — Rules §6).
- [x] Overall / Non-Converted Points accrual + points→Noin conversion: 100:1, multiples of 100, one-way, atomic under concurrency (parallel conversions can never double-credit), cap-counted, Overall Points untouched.
- [x] Play Passes (1/3/7-day, priced in Noin — `economy.play_pass_prices`): **unlimited Quick Play while active**, lifting the free daily cap (`economy.free_daily_quickplay_matches`, 3 at launch — a daily taster; Local Rooms never capped) — **Play Passes never remove ads**; **Premium** — the one cash subscription, monthly / yearly (yearly −20%, `premium_yearly_discount_pct`) via platform billing — is the **sole ad-removal path and includes unlimited Quick Play**, so a subscriber never needs passes and **never hits the daily cap**.
- [x] Noin bulks (`economy.noin_bundles`) via platform billing — **the only place money buys Noin: everything money can get, play can also get, slower** — with server-side receipt verification and idempotent grants keyed by platform transaction id (a replayed receipt grants exactly once; refunds/chargebacks revoke via an explicit audited admin action); SSV rewarded post-match doubler — callbacks signature-verified and replay-proof, the client callback grants nothing; Premium subscribers get the doubling automatically, ad-free; theme packs (`economy.unlock_prices.theme_pack`) with Host Pass enforcement — only the room creator needs the pack in private/local rooms, Quick Play runs core + free rotating featured pack; Poke Styles + Custom Avatar unlocks priced in Noin (`economy.unlock_prices`).
- [x] Economy balance pass (⚙️ Tuning): every price and earn value read from `tuning.yaml` only — economy tuning never needs a client release (proof: a config price change reflects in the Store with no rebuild); numbers tuned against the balance-protocol table, targets first — expected per-match Noin from `mediapack simulate` vs the real numbers from the nightly KPI jobs, one lever at a time.
- [ ] Custom Avatar upload pipeline (👤 §2): server-side crop to 256×256 WebP, EXIF strip, size cap, automated moderation screen before display, admin takedown reverting to presets without refund.
- [x] Player reports (👤 §3) + feedback (👤 §4): one-tap conduct/media reports, rate-limited, repeat reports collapsing into one case, feeding the case queues; in-app feedback form with consented context snapshot into Postgres triage.
- [ ] Client surfaces on native + PWA: Store (`NoinBadge`, bulks, passes, packs, cosmetics), NoticeInbox + dismissible notice banners (`system_notice` live + HTTPS fetch on start, hard-maintenance countdown), post-match SSV doubler flow.
- [ ] Admin Console (🛡️) on the internal port — 2FA-gated, RBAC-scoped, CSRF-protected, unreachable through the public ingress, every action writing an append-only audit row: conduct + media case queues (bans hit live connections immediately), Guard-freeze reviews, pack dashboard, leaderboard ops, economy ledger, feedback triage, system-notice composer — compose, schedule, localize, withdraw — with automatic matchmaking drain (🎮 §4).
- [ ] How-to-play clip (≤45 s, 7 beats, captions on the lime highlighter sweep per 🎨) captured on final production UI — ship gate; player-facing help text + store copy derived from the 🕹️ Game Rules chapter (deliberately the only rulebook).
- [ ] Launch passes: low-end client paint budget, server allocation/GC under queue load, edge-cache hit rates on pack release, **backup/restore + VPS migration runbook executed with verified data parity** (row counts + checksums across Postgres/Redis/MinIO — 📦 §2; Cloudflare R2 free tier as the asset-offload option), store review prep (age gate, per-pack age ratings, UMP consent, privacy notice at first launch, in-app delete-my-data, **store listings + clip captions localized for every launch locale**).
- [ ] App icon + store art via the curated **GPT-6 Astra** raster-batch pipeline (🎨 asset strategy: matrix-derived prompts → human curation → consistency pass).
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
- [ ] Curator toolchain: Curator Guide (derived from ⚙️ §2–3), shared Nown/card-pool authoring with the deal simulator and human ambiguity playtests; retain editorial dimensions, topical source/review/expiry records and cultural rewrites separately from the tone bucket; submission pipeline with terms-consent capture (version + timestamp), the `submissions_per_contributor_per_day` cap, and upload hardening (size/type caps, transcode-on-ingest, automated screen before any human review) — credits + Noin rewards on acceptance; submissions immutable once submitted; withdraw + resubmit is the only correction path and it costs the queue slot.
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
- A spec-chapter change in `BLUEPRINT.md` and its roadmap-chapter update in
  `ROADMAP.md` land in the **same commit** — the lockstep rule, across the
  two files.

### Appendix B — Definition of done

A phase is **done** when:

1. Every `[ ]` bullet under its heading is `[x]`.
2. The phase's *Proof tests* pass on a clean tree.
3. Project-specific verification checks exit 0.
4. The status snapshot at the top of this chapter has been updated.
5. The phase's run produced one or more `commit` tracking rows whose
   `[run-id]` trailers all appear in `git log`.
6. This chapter still matches the spec chapters in `BLUEPRINT.md` — any
   drift
   discovered during the phase was resolved in the same pass (lockstep
   rule, Appendix A).
7. Every proof in the phase's *Proof tests* exists as an automated test
   where automatable; manual drills (device traces, migration rehearsals,
   store submissions) are recorded as `action=note` tracking rows with
   their evidence.
