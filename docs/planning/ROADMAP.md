# 🗺 Knowoff — Text Transition Roadmap

**Status: planning adopted, implementation not started — 2026-09-12.**
Read [Blueprint](../../BLUEPRINT.md) for normative requirements, then the
[technical transition design](../design/DESIGN-text-transition.md) for audited
source gaps, data contracts and retirement inventory, and the
[business plan](../product/BUSINESS_PLAN.md) for experiments and economics.
This is the only active sequence. All implementation boxes below are unchecked.
The [pre-text roadmap](ROADMAP-pre-text-20260912.md) preserves original wording,
checkboxes and evidence; old checks do not certify the new offering.

## Goal

Deliver one text-only Knowoff app with five selectable modes, preserved player
accounts/value, correct secrecy and settlement, viable content and queues, and
no unexplained executable remnants of the association/image architecture.

## Non-goals

No code, migration execution, live content activation, deployment or asset/data
removal in this planning change. Implementation later excludes specialty powers,
production bot backfill, free-typed turn answers, semantic judges, new currencies,
paid gameplay advantage, renamed roles, a visual rebrand or orchestration rewrite.
All five modes remain intended; stagger exposure according to evidence.

## Status snapshot

| Phase | Items | Done | Status |
|---|---|---|---|
| 1 — Contract, baseline and migration preflight | 12 | 0 | Planned |
| 2 — Text catalog, dealing and content certification | 10 | 0 | Planned |
| 3 — Shared match state and five mode engines | 14 | 0 | Planned |
| 4 — Lobbies, protocol and client integration | 12 | 0 | Planned |
| 5 — Durable value, community and trust | 20 | 0 | Planned |
| 6 — Retirement, compatibility and operations | 11 | 0 | Planned |
| 7 — Playtests, business validation and release | 12 | 0 | Planned |

Counts reflect actual task boxes, not inherited phase completion. Run
`python3 xops/makefile/roadmap_ops.py status` from the repository root after
updates and reconcile this table with the parser. Scope IDs are `text-phase-N`;
old `phase-N` tracking IDs refer to the archived roadmap.

## Source-backed readiness correction

The audit is based on repository source, not production inspection. Open risks:

- Draw handler permits out-of-turn draw and publicly reveals new card identities.
- Per-round play/lost-card maps reset; content IDs conflate copy ownership.
- Protocol v1 emits default zero sequences; join does not restore a complete
  snapshot. Older checked reconnect/replay claims need fresh proof.
- Rendering resolves global active content while dealing pins a different pack.
- Room→match callbacks can broadcast back under room locks; disconnect/trade
  work needs a tested lock-order repair before new phases are added.
- Queues are size-only; rematches auto-start/backfill. New readiness/mode/language
  behavior and timeout choices require server and client work.
- Settlement writes several stores separately without stable match identity;
  daily caps, first-win races and leaderboard daily accounting need repair.
- Old backup instructions/scripts are unsafe as recovery proof; live state is
  not recovered from Redis. Migration/head/down behavior must be rehearsed.
- Unified runner omits standalone tools; DB tests can skip, iOS CI uses Linux,
  and current native WebP gate lacks a C compiler. Avatar WebP still needs CGO.

Exact symbols, table dispositions and acceptance evidence belong to the
transition design. Preserve existing failing-test evidence until resolved; do
not spend this documentation pass repairing code or disabling checks.

## Execution discipline and dependency order

Phase 1 precedes implementation. Phase 2 content work and Phase 3 engine work
may use the frozen contract in parallel; Phase 3 initially uses synthetic
fixtures, not a falsely certified production pack. Phase 4 integrates those
contracts. Phase 5 value/trust work can proceed after Phase 1 schema contracts
but must be verified with actual Phase 3/4 behavior. Phase 6 cutover requires
Phases 2–5 green. Phase 7 research starts early; exposure/monetization waits for
its dependencies. Each checkbox is a reviewable deliverable; if it exceeds one
working session, split it into proof-bearing child tasks **here before coding**.

For every phase: read its required spec, record failing regression/contract proof,
implement scoped changes, run independent reviewer then verifier, repair findings,
update only proven boxes and tracking, stage after applicable gates. Never commit
or push. No test is silenced; deleting obsolete test files needs the concrete
owner approval required by AGENTS.md, with release notes and replacement proofs.
A failed gate enters diagnosis/repair; preserve pre-existing human changes.

### Phase 1 — Contract, baseline and migration preflight

**What/why.** Freeze cross-module identities, wire/config and data boundaries;
make gaps measurable before new features rely on them.
**How.** One contract shared by server/client/tools; inventory deployed data and
schema separately from the local repository. No assumptions of an empty DB.
**Spec (required reading).** Blueprint 🕹️ §1–8, 🌐, ⚙️ §1–4, 📦;
transition design contracts, database and verification sections.
**Touched files.** `server/internal/{game,transport,handler,lobby,store,config}/`,
`server/migrations/`, `configs/`, `xops/test/tests-lints.py`,
`.github/workflows/ci.yaml`, protocol/client fixture files (locate before adding).
**Risks.** Shared-file conflicts, unproven baseline, destructive DB tests.
**Proof tests.** Dedicated disposable DB only; full baseline with skip accounting;
wire/config negative fixtures; unchanged pre-transition ledger/data snapshots.

- [ ] Capture current build/test baseline including tool modules, native compiler and actual DB execution; preserve failures with logs and required environment remedies.
- [ ] Inventory deployed schema/dirty version, clients/protocol, content types, active matches, entitlements and object consumers; produce count/hash preflight without exposing secrets.
- [ ] Freeze version-2 match/lobby/action/snapshot schemas and golden fixtures, including all five mode IDs, content vs copy IDs and language vs UI locale.
- [ ] Define monotonic event sequencing, request dedupe/conflict errors, expected revision checks and maximum history/frame budgets with negative fixtures.
- [ ] Add typed config contract for mode availability, compatibility and trade response; retain numeric economy/clock defaults; define obsolete-key migration errors.
- [ ] Record isolated failing reproductions for out-of-turn/public draw, active-pack drift and incomplete sequence/reconnect; land executable regressions with their fixes in Phases 2–4, not as a red completed baseline.
- [ ] Record a bounded real Room/Match deadlock reproduction and define lock/callback ordering; the executable regression lands with the Phase 3 fix.
- [ ] Design stable durable match, admission, settlement/outbox and daily-count keys; transaction boundaries and replay states reviewed with wallet owners.
- [ ] Allocate new migration numbers after current 000008; additive up/controlled down plans, legacy archival states, indexes/FKs/unique constraints and resumable backfill.
- [ ] Add fresh/head/repeated-up/dirty/interrupted/legacy-fixture migration proof on isolated PostgreSQL; mark unsupported lossy rollback explicitly.
- [ ] Extend unified validation to all retained Go/Python tools, real integration services and skip failure; separate macOS iOS build from Linux jobs.
- [ ] Gate: contracts reviewed, baseline limitations explicit, migration preflight and schema/wire/config fixtures verified; no historical completion claim substitutes for evidence.

### Phase 2 — Text catalog, dealing and content certification

**What/why.** Replace image delivery and synthetic-only production assumptions
with a private, immutable, mode-aware text supply that can sustain whole matches.
**How.** Adapt shared `server/pkg/media`, keep runtime and tooling validation
identical, certify retained hands and changing boards, then human-test content.
**Spec (required reading).** Blueprint ⚙️ §1–4, 🧑‍🎨, 🕹️ §2–3;
content/curator-guide.md and business content operating plan.
**Touched files.** `server/pkg/media/`, `tools/mediapack/`, `content/`,
`server/internal/{portal,workbench}/`, bundle fixtures.
**Risks.** Generic safe responses, repeat-prompt leaks, distribution drift,
normalization damaging language, treating a synthetic pass as humor proof.
**Proof tests.** Both sizes × all modes × certified languages; malformed/tampered
pack refusal, immutable activation and deterministic fixture replay.

- [ ] Implement versioned text-only manifest/records with mode/pool/language/rules suitability, provenance, hashes and stable content revisions; reject nontext active data.
- [ ] Implement bounded Unicode text validation/rendering contract, safe normalization and duplicate review; malicious markup/control characters and Turkish/RTL fixtures.
- [ ] Pin complete validated text snapshots per match; failed activation leaves prior release active, and mid-match activation never changes wording or evidence.
- [ ] Replace required multimodal assumptions with reviewed versioned text suitability; record optional evaluator/model data without leaking candidate bands to clients.
- [ ] Deal role-blind 5+3 against every scheduled Nown and reject infeasible setup; test retained hand/reserve coverage instead of discarded intermediate selections.
- [ ] Certify distinct neutral system seeds independent of roles/prompts, unique copy identities and mode-specific reachable action/depletion viability.
- [ ] Build isolated synthetic fixtures for five modes, small exhaustive ownership/state tests and reproducible production-candidate schedule simulations.
- [ ] Connect accepted immutable contributions to reviewed bundle/certify/publish/activate lifecycle; preserve consent/credits and forbid duplicate publication rewards.
- [ ] Run editorial pilots in three themes/two cultures at 4/6 sizes; retain comprehension/ambiguity/first-seat/draw/repetition evidence per mode and language.
- [ ] Gate: technical certification and human release decision both present for each candidate mode/language; old seed volume counts never constitute production readiness.

### Phase 3 — Shared match state and five mode engines

**What/why.** Establish conserved card ownership, full history and serialized
phase changes; implement simple mode actions on one shared game engine.
**How.** Shared clocks/roles/votes/economy hooks plus discriminated mode state,
not five copied match engines. Phase 1 failing tests lead each repair.
**Spec (required reading).** Blueprint 🕹️ §1–8, ⚙️ §4, 🌐, 🤖;
transition design ownership/event and test matrices.
**Touched files.** `server/internal/game/`, `server/internal/lobby/room.go`,
`server/internal/transport/`, `tools/gamebot/` and corresponding tests.
**Risks.** Lock reentry, duplicate copies/transfers, leaking private state,
response/timeout races, result callbacks firing twice.
**Proof tests.** Conserved instance multiset, exact one transition/action,
state-machine/property/race tests, full no-leak event scanning.

- [ ] Repair Room/Match lock/callback ordering with real disconnect broadcasts and concurrent completion; no socket I/O under nested state locks.
- [ ] Add stable match/round/action/copy identity and one-owner location registry for hand/reserve/board/discard/pending; copied wording never conflates copies.
- [ ] Implement whole-match ordered public evidence and private per-seat state, round reset and bounded snapshots using the pinned pack/rules.
- [ ] Fix draws to current-turn-only, private owner delivery/public count and exactly-once penalty; reject eliminated/disconnected/pending-offer draws.
- [ ] Remove specialty dealing/use from text engine and reject legacy specialty/dev grants; preserve timeout passes and tied runoffs.
- [ ] Implement Missed the Briefing response confirmation/consumption, preview revision and attributed evidence with idempotent retry.
- [ ] Implement Secret Scale atomic card+1–5 placement, neutral labels, multiple copies per rating and immutable history.
- [ ] Implement Make Room three-item seeding and atomic slot replacement/discard with original before/after evidence retained.
- [ ] Implement Bad Bargains offer reservation, recipient-only accept/refuse and atomic ownership transfer with display/hand conservation.
- [ ] Implement Bad Bargains response deadline, disconnect/forced-transition cancellation and no-recipient pass; accept/expiry/leave race has one result.
- [ ] Implement Top That neutral seed/current-target revision and ordered chain, with no semantic superiority judge or veto.
- [ ] Prove all-mode shared vote budgets, early team victory, tied runoff, eliminated permissions, timeout penalties, grace/forfeit/low-population and begun-round-only verdict.
- [ ] Update dev/test gamebot policies and seeded ordered replay scripts for each mode/role, own observation only; test matches never earn live value.
- [ ] Gate: engine/property/race/no-leak tests pass at 4/6 and multi-round schedules, with no pending trade crossing round/elimination/verdict and no duplicate finish.

### Phase 4 — Lobbies, protocol and client integration

**What/why.** Make selecting, joining, playing, reconnecting and rematching
honest and accessible across native/PWA surfaces.
**How.** Server settings/readiness and compatibility first; client reducers
consume v2 snapshots/events and render legal actions only.
**Spec (required reading).** Blueprint 🎮 §1–2, 🌐, 🎨, Product Baseline;
transition design compatibility and client retirement boundaries.
**Touched files.** `server/internal/{lobby,handler,transport}/`,
`client/lib/{data,domain,presentation,media,l10n,core}/`, client tests.
**Risks.** Queue fragmentation/double seats, stale Ready, cache leakage,
host churn, inaccessible time-limited controls and duplicate reducer removal.
**Proof tests.** Queue/lobby integration, protocol matrix, reducer/widget tests,
small-screen/large-text/keyboard/screen-reader/native/Web evidence.

- [ ] Implement pre-admission v2 handshake/version rejection, monotonic sequence assignment, gap detection and authorized snapshot resync without replaying mutations.
- [ ] Implement explicit mode/size/content-language FIFO queues with no silent substitution; waiting preserves place, change leaves/rejoins atomically.
- [ ] Implement mode availability and default/last-choice discovery; disabled/unreleased modes cannot queue or silently become another mode.
- [ ] Implement full connected Ready lobby, revision invalidation on settings/membership, owner-only settings and deterministic host transfer; revalidate next-match Host Pass sponsorship without charging guests or silently changing packs.
- [ ] Implement settings/Ready rematches for Local/Quick Play, host rules, replacement members and leave; Quick Play retains its caps/core-featured eligibility and same-tuple FIFO replacements. Reject downsizing over current membership; cancel old reservations on settings change.
- [ ] Render match contract and action previews; reducers operate on copy IDs and reconcile duplicate/out-of-order events without spending another card.
- [ ] Build five mode controls and boards with attribution/chronology, including trade recipient response and public known-card evidence.
- [ ] Build role-scoped reconnect with current board/history/private hand/pending deadline; clear secret views on elimination/logout/match change.
- [ ] Remove gameplay catalog sync/prefetch and migrate only obsolete cache keys/service-worker generation; retain authentication/account preferences and non-playable assets.
- [ ] Localize mode/rule/error/instruction copy; keep content language explicit, test long/RTL/diacritic text and touch/keyboard/screen-reader action equivalence.
- [ ] Exercise native/PWA 4/6-seat flows, resize/reduced motion, stale deep links, old client/new server and new client/old server refusal; profile action/history updates against Blueprint client p95 ≤16.7 ms on the stated low-end device.
- [ ] Gate: no text secret in downloaded catalog, persistent cache, logs, unauthorized/hidden/eliminated semantics or unauthorized snapshot; active-Nower reveal is screen-reader accessible, all five action and lobby journeys proven, no stale hidden controls.

### Phase 5 — Durable value, community and trust

**What/why.** Preserve paid/earned value and existing community capability;
repair old integration gaps rather than carrying them into five reward lanes.
**How.** Additive schema/backfill and transactional idempotency; preserve account
identity, historical consent/ledger and unknown legacy mode labels honestly.
**Spec (required reading).** Blueprint Rules §6–7, 💰, 👤, 🧑‍🎨, 🛡️,
🎮 §3–5, Product Baseline; transition design DB/settlement sections.
**Touched files.** `server/migrations/`, `server/internal/{store,economy,profiles,leaderboard,portal,admin,handler,reports,notices}/`, client account/community surfaces.
**Risks.** Duplicate/partial rewards, first-row races, entitlement loss,
legacy-reference loss, false billing/readiness claims.
**Proof tests.** Real PostgreSQL transactions/concurrency/failure injection,
pre/post balance+ledger+receipt+FK parity, privilege/privacy tests.

- [ ] Apply additive schema and resumable bounded backfill to test copies; old rows remain legacy/unknown, not fabricated new-mode history.
- [ ] Implement stable match/account admission and exactly-once start counter; share Quick Play caps across modes, release canceled reservations and preserve local uncapped access.
- [ ] Implement idempotent durable result/points/XP/leaderboard/Noin settlement with transaction/outbox recovery and no duplicate value after callback/crash retry, including scored low-population endings.
- [ ] Implement confirmed server-interruption closure and exactly-once free-allowance compensation; preserve committed awards, issue no fabricated completion/first-win/XP/points and prove crash-after-start/mid-award/retry.
- [ ] Repair daily leaderboard counts, concurrent first-win/first-counter creation and conversion/earn caps at UTC/week boundaries across modes.
- [ ] Preserve instant durable event Noin plus private settlement, absent/low-population rules and prototype zero-value; no rewards for item trades or ratings.
- [ ] Reconcile legacy theme entitlements using reviewed equivalent-benefit mapping; inventory unresolved paid promises and remedy before disabling access.
- [ ] Preserve consent/submission/challenge/report relationships and text-only new-write controls; approvals/payments never rerun from backfill/publication.
- [ ] Complete text curation/activation/takedown and public report resolution with immutable revisions; automated screening plus human decision before visibility.
- [ ] Complete challenge scheduler and exactly-once weekly title/payout transfer with restart and week-boundary tests.
- [ ] Repair Guard identity, overlapping suspension isolation and expiry; prove admin-final enforcement and role separation.
- [ ] Implement versioned chat/UGC terms acceptance and private user block/report/contact journey: hide authored chat/UGC, preserve required game evidence, prevent future co-matching/invites and prove no role leak or mid-match score manipulation; review platform compatibility.
- [ ] Complete audited moderation/admin operations and text-provider contribution journey with failure/no-visibility proof.
- [ ] Complete OAuth linking and second-device restoration with account-collision, revoked-token and lost-session proofs.
- [ ] Complete in-app account deletion with reviewed retention, relational/JSONB/blob cleanup and reauthentication tests.
- [ ] Complete entitled avatar screening/activation/takedown with unchanged non-playable WebP behavior and rejected-provider proof.
- [ ] Complete platform receipt validation, restore/refund and idempotent entitlements using platform test evidence.
- [ ] Complete verified SSV/Premium doubler and consent flows with replay, refusal and private reward tests.
- [ ] Verify ledger append-only enforcement, wallet reconciliation and deletion/retention handling across rows/JSONB/blobs; no private role rewards in public stats/analytics.
- [ ] Gate: financial/data parity and duplicate/failure-recovery proofs pass; current user value preserved, live trust surfaces verified, unresolved launch dependencies explicit.

### Phase 6 — Retirement, compatibility and operations

**What/why.** Remove the obsolete architecture safely and prove deployment,
backup and rollback from real artifacts rather than documentation assertions.
**How.** Expand/backfill → compatible staging → old-match drain → text cutover →
observed rollback window → contract removal. One writer throughout.
**Spec (required reading).** Blueprint 📦, 🌐, ADR-012, transition design
retirement matrix and corrected VPS runbook.
**Touched files.** Runtime retirement entries in transition matrix; `infra/`,
`nginx/`, Dockerfiles, module manifests, configs, tools/tests, docs and skills.
**Risks.** Data deletion, stale clients, dead readiness dependency, removing avatar
codecs, restoring stale purchases, using unsafe generic MigrateDown/snapshot.
**Proof tests.** Disposable-volume rehearsal, artifact/caller/route/dependency
inventory, rollback before/after writes, every module/platform build.

- [ ] Repair and test snapshot/restore/drain tooling on disposable volumes with bounded failures; verify actual schema inventory and durable settlement quiescence.
- [ ] Rehearse 000008→text upgrade, mid-backfill resume, schema constraints and exact supported rollback/forward-fix; never use unreviewed all-migrations-down.
- [ ] Build text-compatible deployment/config/pack manifest, explicitly fence v1 clients and drain old matches on their original rules/content.
- [ ] Implement selective stale room/queue/cache invalidation and single-writer cutover with account/ledger/entitlement parity and real enabled-mode human smoke.
- [ ] Exercise server crash, Redis/Postgres loss and restore; no fabricated live-match recovery or lost/duplicate durable value; readiness recovers correctly, with Blueprint latency/100-room/soak budgets and frame bounds proven.
- [ ] Retire association-game handlers, specialty states/dev grants, production backfill scheduler, obsolete wire branches/UI/strings after new rejection proofs.
- [ ] Retire playable-image loaders/signed URLs/prefetch/catalog caches, workbench/image candidate path and sole-use config/assets/dependencies; preserve avatar/UI consumers.
- [ ] Retire MinIO/game-content CDN/credentials/healthchecks only after consumer/backup inventory; preserve authorized archives and all applied migration history.
- [ ] Replace obsolete fixtures/assertions with mode/privacy/migration/unsupported-input tests; obtain concrete approval before deleting test files and record release notes.
- [ ] Close every temporary compatibility entry after measured rollback window; run no-leftovers source/call/dependency/route/packaged-artifact scans and align active docs/skills.
- [ ] Gate: rehearsal/parity/build/load/security tests green, rollback demonstrated, retirement exceptions have owner/reason/expiry, no dormant old gameplay architecture.

### Phase 7 — Playtests, business validation and release

**What/why.** Confirm that the text product is understandable, fun, fair and
financially supportable before expanding acquisition or monetization.
**How.** Use business-plan experiment records, cohort cutoffs and decision owners;
quality per mode/language precedes scale. Dates depend on staffed capacity.
**Spec (required reading).** Blueprint Product Baseline, 🎮, 💰, 🎬;
BUSINESS_PLAN.md, STORE_COPY.md, HOW_TO_PLAY_CLIP.md and transition gates.
**Touched files.** Versioned playtest/release evidence, certified content,
availability config, localization/help/store assets, operator records.
**Risks.** Sparse queues, novelty decay, dominant farming mode, unreviewed claims,
paid users losing legacy access, mistaking small samples for proof.
**Proof tests.** Recorded human 4/6-player cohorts and release checklist with
measured latency/completion/role outcomes/content quality/cost assumptions.

- [ ] Recruit and run instrumented local prototypes across five modes, both sizes and initial language/culture pilots; no live rewards/leaderboard.
- [ ] Measure comprehension/first-turn success, laughter/defensible alternatives, first-seat/role win rates, draw pressure, completion and repeat content.
- [ ] Compare earnings/minute, repeated pairings and collusion across modes with shared caps; fix material abuse before enabling reward eligibility.
- [ ] Measure actual per-mode/size/language FIFO wait/abandonment; launch bounded scheduled cohorts and expand only when liquidity meets recorded targets.
- [ ] Test onboarding/default/mode-change/rematch with real new players and accessibility/low-end devices; preserve no-silent-fallback behavior.
- [ ] Validate retention and invitation hypotheses using defined cohorts/windows; report denominators, uncertainty and missing data, not fabricated success.
- [ ] Replace illustrative business economics with current quotes, platform net proceeds and measured moderation/content/support costs; owner funds the approved runway.
- [ ] Complete owner decisions on launch locales/cohorts, age/content policy operations, legacy entitlement remedies, public legal URLs and purchase/consent review.
- [ ] Capture final production UI how-to clip, localized rules/store screenshots and tested disclosures for only currently enabled capabilities.
- [ ] Rehearse public host migration, backup/restore, incident/takedown and customer-support escalation with explicit rollback triggers and responsible operator.
- [ ] Enable each certified mode/language progressively; observe agreed window, rollback on privacy/value/integrity failure and halt acquisition if queues or economics fail.
- [ ] Gate: all launched cohorts have signed technical/content/business/operations evidence; unlaunched intended modes remain tracked and unavailable, never falsely advertised as playable.

## Appendix A — Tracking and truthful completion

One run ID per scoped pass; `action=commit,status=completed,commit_sha=pending`
only after applicable gates. Parent stages and verifies `make git.dry`; human
commits/pushes. Blueprint changes and implied roadmap changes land together.
Record code review, test counts/skips, artifact versions and manual proof links.
No historical log rewrite. A documentation candidate is not a release approval.

## Appendix B — Definition of done

A phase completes when every required box and its proof is complete, independent
review/verifier findings are fixed, no skip masks an integration requirement,
status counts and current documentation agree, and tracking/staging handoff is
verified. An implementation gate blocked by environment or owner evidence stays
open with a checkpoint. The overall transition additionally requires retirement
proof, preserved user value and the business/content/operations launch decision;
a green compile cannot substitute for any of these.
