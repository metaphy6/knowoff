# 🗺 Knowoff — Text Transition Roadmap

**Status: Resumed after credit interruption — 2026-09-19. Phases 1 and 3 complete; remaining implementation and release proof in progress.**
See the [current recovery record](../reports/2026-09-19-text-transition-resumption.md) and [earlier resumption evidence](../reports/2026-09-12-text-transition-resumption.md) for validation and remaining work; the earlier pause is historical.
Read [Blueprint](../../BLUEPRINT.md) for normative requirements, then the
[technical transition design](../design/DESIGN-text-transition.md) for audited
source gaps, data contracts and retirement inventory, and the
[business plan](../product/BUSINESS_PLAN.md) for experiments and economics.
This is the only active sequence. Checkboxes below require fresh implementation evidence.
The [pre-text roadmap](ROADMAP-pre-text-20260912.md) preserves original wording,
checkboxes and evidence; old checks do not certify the new offering.

## Goal

Deliver one text-only Knowoff app with five selectable modes, preserved player
accounts/value, correct secrecy and settlement, viable content and queues, and
no unexplained executable remnants of the association/image architecture.

## Non-goals

The original planning pass changed no code or deployed data; the owner's
subsequent request authorizes implementation following this sequence.
Implementation excludes specialty powers,
production bot backfill, free-typed turn answers, semantic judges, new currencies,
paid gameplay advantage, renamed roles, a visual rebrand or orchestration rewrite.
All five modes remain intended; stagger exposure according to evidence.

## Status snapshot

| Phase | Items | Done | Status |
|---|---|---|---|
| 1 — Contract, baseline and migration preflight | 12 | 12 | Complete; independent review and full validation passed |
| 2 — Text catalog, dealing and content certification | 10 | 8 | Technical action certification and bounded exhaustive/replay fixtures independently verified; human editorial/release gates open |
| 3 — Shared match state and five mode engines | 14 | 14 | Complete; independent review, cold race/property/privacy checks and full unified gate passed |
| 4 — Lobbies, protocol and client integration | 12 | 10 | Technical integration independently verified; physical-device journeys/performance and final gate open |
| 5 — Durable value, community and trust | 20 | 11 | Durable settlement/recovery, Guard, weekly lifecycle, OAuth, avatar and curation/report integration verified; remaining trust/provider gates open |
| 6 — Retirement, compatibility and operations | 11 | 0 | Drain and isolated restore proofs in progress; full retirement gates open |
| 7 — Playtests, business validation and release | 12 | 0 | Planned |

Counts reflect actual task boxes, not inherited phase completion. Run
`python3 xops/makefile/roadmap_ops.py status` from the repository root after
updates and reconcile this table with the parser. Scope IDs are `text-phase-N`;
old `phase-N` tracking IDs refer to the archived roadmap.

## Source-backed readiness correction

The following findings describe the pre-transition baseline, not production inspection.
Engine, queue, replay, client and session repairs now have scoped evidence in
the [resumption record](../reports/2026-09-12-text-transition-resumption.md);
retirement and the final unified gate remain open. Original findings:

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
transition design. Preserve existing failing-test evidence until resolved and
record fixes with their regression proof. The unified-runner/compiler issues
above are addressed by the [Phase 1 foundation](../reports/2026-09-12-text-phase1-validation.md);
current checkbox status and the resumption record identify the remaining runtime proof.

## Execution discipline and dependency order

Phase 1 foundations precede mode implementation. Phase 2 content work and Phase 3 engine work
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

- [x] Capture current build/test baseline including tool modules, native compiler and actual DB execution; preserve failures with logs and required environment remedies.
- [x] Inventory deployed schema/dirty version, clients/protocol, content types, active matches, entitlements and object consumers; produce count/hash preflight without exposing secrets.
- [x] Freeze version-2 match/lobby/action/snapshot schemas and golden fixtures, including all five mode IDs, content vs copy IDs and language vs UI locale.
- [x] Define monotonic event sequencing, request dedupe/conflict errors, expected revision checks and maximum history/frame budgets with negative fixtures.
- [x] Add typed config contract for mode availability, compatibility and trade response; retain numeric economy/clock defaults; define obsolete-key migration errors.
- [x] Record isolated failing reproductions for out-of-turn/public draw, active-pack drift and incomplete sequence/reconnect; land executable regressions with their fixes in Phases 2–4, not as a red completed baseline.
- [x] Record a bounded real Room/Match deadlock reproduction and define lock/callback ordering; the executable regression lands with the Phase 3 fix.
- [x] Design stable durable match, admission, settlement/outbox and daily-count keys; transaction boundaries and replay states reviewed with wallet owners.
- [x] Allocate new migration numbers after current 000008; additive up/controlled down plans, legacy archival states, indexes/FKs/unique constraints and resumable backfill.
- [x] Add fresh/head/repeated-up/dirty/interrupted/legacy-fixture migration proof on isolated PostgreSQL; mark unsupported lossy rollback explicitly.
- [x] Extend unified validation to all retained Go/Python tools, real integration services and skip failure; separate macOS iOS build from Linux jobs.
- [x] Gate: contracts reviewed, baseline limitations explicit, migration preflight and schema/wire/config fixtures verified; no historical completion claim substitutes for evidence.

#### Phase 1 execution detail — 2026-09-12

**Initial foundation handoff.** Eight of twelve parent items were complete; see the
[foundation validation report](../reports/2026-09-12-text-phase1-validation.md)
for the independent review, 18 passing checks and 636 tests with zero skips.
The continuation below addresses the then-open inventory, wallet review and
transition backfill/legacy-conflict proofs. Live gameplay still uses v1.

**Goal.** Establish executable, reviewed text-mode contracts and a trustworthy
validation/migration baseline before implementing mode behavior. The owner's
subsequent implementation request authorizes local engineering; the earlier
documentation-only boundaries describe the completed planning pass. This detail
decomposes the twelve checkboxes above and does not add another phase sequence.

**Non-goals.** No live v2 cutover, enabled mode, production content approval,
wallet rewrite, applied-migration edit, asset/dependency retirement or public
release in this foundation slice. V2 contract fixtures can coexist with the
current v1 runtime until the Phase 4 compatibility implementation; they must not
advertise an available v2 game. Preserve current economy/clock values and avatar
WebP support. Owner/operator release evidence is never inferred from local tests.

**File ownership.** Give each parallel worker one boundary: validation owns
`xops/test/` and `.github/workflows/ci.yaml`; contracts own
`server/internal/transport/` and versioned wire fixtures; configuration owns
`server/internal/config/` and `configs/`; migration/preflight owns
`server/internal/store/` and newly allocated migration fixtures. Existing
`protocol_test.go`, config `loader_test.go`/`loader_config_test.go`, store
`migrate_test.go`, and client `websocket_transport_test.dart`/game-session tests
are the starting points. Search before adding fixtures; share the same JSON
bytes across server and client validation rather than maintain independent
examples. Reproduction work may inspect game/lobby/handler/media code but lands
behavior fixes with the corresponding later phase. The parent alone updates
roadmap completion, tracking and staging.

**Bounded tasks and proof.** Each row is a reviewable child task of the named
checkbox; complete both subparts where listed before checking the parent box.
Write behavior tests first and retain the expected failing output, then the
passing result with its implementation. Evidence-only reproductions stay outside
the normal green suite until their repair phase.

| Parent checkbox / child task | Delivery and acceptance evidence |
|---|---|
| Baseline — toolchain | Record exact retained modules/platform tools and inherited native WebP failure; prove a workspace/container toolchain builds avatars with CGO. Do not install host packages merely to make this record green. |
| Baseline — execution | Run the unified gate and retained standalone modules; record commands, counts, skips and service/tool versions. An unreachable PostgreSQL test remains unexecuted, never passed. |
| Deployed inventory | Produce a read-only, redacted preflight with schema version/dirty state, image/protocol/pack IDs, active-match counts, content/status counts, entitlement totals and object-consumer inventory. Include counts/hashes of retained rows and migration files; distinguish an unavailable deployment from a proved empty one. |
| Wire schemas — identities/actions | Freeze five mode IDs and `respond`, `place`, `replace`, `offer`, `resolve_offer`, `top`; encode separate room/match, content/revision/copy, round/turn/phase and request identities. Golden fixtures cover each action plus invalid discriminator, unknown/contradictory fields, fractional rating/count, rating outside 1–5, invalid slot, wrong version and malformed language. |
| Wire schemas — lobby/snapshots | Freeze original 4/6 size, settings/Ready revision, content language separate from UI locale, pinned rules/tuning/pack identity and server-owned reward eligibility. Round-trip the same fixtures for active Nower, Donower and eliminated views; public data excludes current secret for unauthorized seats, future prompts, other hands and all hidden reserve identities. Equal-text copies retain distinct IDs. |
| Sequencing — replay contract | Freeze public `evidence_seq`, per-recipient `recipient_seq`, reconnect `stream_epoch` and atomic snapshot cursors. Fixtures cover a private event to another seat without a local gap, duplicate/out-of-order events, stale epoch/board, identical request retry and conflicting reuse. Cached outcomes reauthorize private replies after elimination; absolute deadlines survive reconnect. |
| Sequencing — bounds | Serialize a six-seat/three-round maximum-action trace with draws, penalties, trades, ballots and permitted chat. Prove the configured effective frame/history limits; if it exceeds them, freeze and test bounded cursor/hash pages with complete assembly. Never truncate required evidence or claim a future runtime sequence assignment already exists. |
| Typed config — additions | Add strict typed mode/language availability and compatibility contracts with positive bounded `timers.trade_response_s: 10`. Positive/negative load fixtures prove missing/unknown/duplicate modes, unknown defaults, unsupported versions/languages and timer bounds. Availability starts closed until certification; a known future default can remain disabled, and loading config does not enable unimplemented handlers. |
| Typed config — transition | Define the v2 obsolete-key migration errors and neutral round-start countdown replacement. Preserve legacy config compatibility until the declared boundary; test old-key rejection there, unchanged economy/clock defaults and fail-fast unknown keys. Final specialty/image/backfill field removal remains Phase 6 consumer-verified work. |
| Regression records — draws/pack | Record bounded reproductions of out-of-turn/public draw and same-ID wording replacement during a match, with preconditions and expected target assertions. Keep the current contradictory draw test attributable; replace it with the adopted regression alongside the Phase 3 repair, and pinning proof with Phase 2. |
| Regression records — resync/locks | Record zero-sequence/incomplete reconnect and a real websocket Room→Match→Room current-turn disconnect deadlock under an explicit timeout. Specify callbacks and socket writes outside incompatible nested locks and one connection writer. Save the observed failure; fixes and executable green regressions belong to Phases 3/4. |
| Durable identity design | Review match/account admission identity, immutable award keys, settlement/outbox replay states and shared UTC daily counters with wallet ownership. Specify atomic reserve/start/cancel/interrupt compensation, committed-award preservation, no double first-win/points/XP/leaderboard grants, and no process-loss fabrication of results. Tabletop failure traces include crash after start, partial award, callback retry and first-row races. |
| Migration allocation/design | Recheck head `000008`; allocate unused subsequent numbers in the transition design before SQL lands. Specify additive tables/columns, unique keys, FKs/indexes, legacy/unknown states, bounded primary-key backfill cursors and controlled down/forward-fix boundaries. Review data/entitlement parity and old-server compatibility; never rewrite the eight applied migrations. |
| Migration proofs — harness | Require an explicitly disposable PostgreSQL 16 database; refuse accidental shared/default DSNs before any schema drop. Prove fresh→head, realistic 000008 fixture→head, repeated up, dirty/pre-000008 refusal and unchanged retained-data hashes. Test exact supported rollback; generic all-migrations-down is not a recovery gate. |
| Migration proofs — resume | In Phase 1, exercise interruption/resume and duplicate/conflicting legacy rows against the reviewed additive migration/backfill design fixture on isolated PostgreSQL. A harness-only pass cannot complete this parent checkbox. Phase 5 reruns these cases against its actual migrations/backfill; Phase 1 fixture proof never certifies that later implementation. Unsupported lossy rollback remains an explicit refusal with forward-fix evidence. |
| Unified validation — coverage | Include both retained Go tools and Python suites, real integration services and failing skip accounting. Prove missing service/test failures propagate; retain meaningful existing assertions and avoid environment skips as success. |
| Unified validation — platform | Keep Linux Android/Web and macOS/Xcode iOS jobs separate. Run platform-appropriate commands and state unavailable local iOS evidence; configuring a macOS job is not a successful iOS build. |

**Acceptance gates.** Independent review then verification inspect the complete
diff and shared fixtures. Run `python3 xops/test/tests-lints.py` through safe-run
from the repository root after targeted red→green checks; record every omitted
platform/integration proof explicitly. Existing migration checksums, local
fixture ledger/receipt/entitlement hashes and v1 runtime compatibility must remain
unchanged unless the reviewed slice explicitly extends them. Tick a main box
only when all its listed proof is present; twelve boxes and the phase gate stay
authoritative. The parent records the tested scope and verified commit preview.

**Dependencies and risks.** Local contracts/config, isolated reproductions,
runner coverage and migration design can proceed now. Actual deployed inventory
needs operator access/evidence; wallet transaction review needs the responsible
owner. Neither is a reason to stop independent local work, but both keep the
full Phase 1 gate open until supplied. Content certification, actual engine
ordering/resync, durable backfill and platform/provider/public-cohort evidence
remain in Phases 2–7; fixture agreement cannot substitute for them. Highest
risks are enabling v2 before consumers agree, losing private-state boundaries in
replay, destructive DB fixtures, and concurrent edits to shared configuration;
mitigate with closed availability, role-negative fixtures, isolated services
and explicit file ownership. No owner spending, data deletion or deployment
authorization is implied by a planning default.

#### Phase 1 continuation — remaining evidence, 2026-09-12

**Verified completion.** All twelve Phase 1 items are now complete. The
[continuation report](../reports/2026-09-12-text-phase1-continuation.md) records
the owner-attested deployment absence, wallet-domain design review, realistic
migration/backfill proof and final 18 checks/648 tests with zero failures/skips.
Phase 2 catalog/dealing and Phase 3 synthetic engine work can now proceed;
actual production migrations, content certification and live v2 remain later gates.

**Goal and boundary.** Finish the executable inventory and realistic migration
proofs for remaining items 2, 8, 10 and 12. The owner states **“No deployment
exists.”** Record live deployment categories as not applicable under that dated
attestation; local SQL hashes and disposable fixtures remain local evidence.
Retain all previously staged work. No applied SQL rewrite, live v2 activation,
content approval, new wallet behavior or data-retirement operation is in scope.

**Ownership and ordered proof tasks.** The parent owns the existing preflight
CLI/store report and review packet; the migration worker owns store test helpers
and discovered testdata; the inventory worker performs read-only discovery.
The planner edits this continuation only. These rows decompose existing items;
they do not add parent checkboxes.

| Item / task | Concrete delivery and required proof |
|---|---|
| 2 — Inventory/report | Extend `server/internal/store/preflight*.go` and `server/cmd/transition-preflight/`: fixed-vocabulary content/status/entitlement counts and wallet/ledger aggregates; reject ambiguous migration state. Wrap deterministic DB evidence with version, timestamp, local SQL-file hashes and explicit coverage. Test privacy for unknown labels, missing DSN, invalid timeout/manifest and database errors. Never label local files as deployed artifacts or unavailable categories as measured zero. |
| 8 — Wallet review | Review the existing design's match/admission/award/settlement/outbox/day identities against actual wallet and finish paths. Record duplicate start/finish, partial commit, first-row/day/week races, interruption compensation and private presentation outcomes. Existing Blueprint policy and numeric defaults remain adopted; request a decision only for a concrete unresolved departure, not another approval of unchanged rules. Record who reviewed what and any remaining owner-specific evidence. |
| 10 — Real legacy fixture | On guarded disposable PostgreSQL 16, apply the actual migration chain to 8 and seed text/static-image submissions, Nown self-references, terms/consent, challenge topic/entry/vote/winner links, contributor credits, wallet/ledger, theme entitlement and verified/refunded receipts. Fingerprint every original table before additive sidecar archival structures; copied rows stay `legacy_unreviewed` without invented mode/language suitability. |
| 10 — Bounded copy/resume | Namespace source kind plus UUID; persist source hash, bounded copy/verification cursors and ordered counts/hashes. Commit sidecar rows and cursor together. Interrupt after one committed batch and inside another, reconnect and resume; copied/verified batch sizes never exceed the explicit test limit. Two bounded passes compare actual source rows with mappings and detect changed/deleted/late keys. Input writers must be frozen throughout; a maximum UUID alone is not a stable snapshot or an online-migration guarantee. |
| 10 — Refusal/parity | Actual pre-8 schema and dirty-8 refuse before DDL; fresh installation to 8 remains valid. Repeated up and identical replay are no-ops; duplicate manifest/source mappings and conflicting hashes fail without overwrite, while identical UUIDs in different source kinds remain distinct. Empty additive structures may be removed by the exact fixture down; populated retained structures refuse. Prove unchanged original rows, references, blobs, terms, credit/receipt/entitlement values and totals after each success/failure; no publication or reward side effect. |
| 12 — Review/verification | Independent review inspects all new files and retained staged changes, then run `python3 xops/test/tests-lints.py` through safe-run with the runner's disposable services. Require zero failures/skips, unchanged 16 applied SQL files, red-to-green targeted evidence and a dated report distinguishing fixture proof from future production migration. Mark only fully proved parent items; unresolved required review keeps the gate open. |

**Dependencies and risks.** After these local continuation proofs, the existing
parallel-dependency rule permits Phase 2's versioned text schema, validation and
snapshot pinning, and Phase 3's lock/ownership work using synthetic fixtures and
the frozen wire contract. Those tasks do not consume deployed data or approve
wallet policy. Keep any outstanding Phase 1 review visible; settlement changes,
activation, retirement and public exposure still require their actual gates.
The principal risks are mistaking archival copy for reviewed playable content,
missing source drift between batches and treating a successful fixture as a
production migration. Preserve source hashes, explicit frozen-input assumptions
and the later Phase 5 rehearsal against its actual migrations.

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

- [x] Implement versioned text-only manifest/records with mode/pool/language/rules suitability, provenance, hashes and stable content revisions; reject nontext active data.
- [x] Implement bounded Unicode text validation/rendering contract, safe normalization and duplicate review; malicious markup/control characters and Turkish/RTL fixtures.
- [x] Pin complete validated text snapshots per match; failed activation leaves prior release active, and mid-match activation never changes wording or evidence.
- [x] Replace required multimodal assumptions with reviewed versioned text suitability; record optional evaluator/model data without leaking candidate bands to clients.
- [x] Deal role-blind 5+3 against every scheduled Nown and reject infeasible setup; test retained hand/reserve coverage instead of discarded intermediate selections.
- [x] Certify distinct neutral system seeds independent of roles/prompts, unique copy identities and mode-specific reachable action/depletion viability.
- [x] Build isolated synthetic fixtures for five modes, small exhaustive ownership/state tests and reproducible production-candidate schedule simulations.
- [x] Connect accepted immutable contributions to reviewed bundle/certify/publish/activate lifecycle; preserve consent/credits and forbid duplicate publication rewards.
- [ ] Run editorial pilots in three themes/two cultures at 4/6 sizes; retain comprehension/ambiguity/first-seat/draw/repetition evidence per mode and language.
- [ ] Gate: technical certification and human release decision both present for each candidate mode/language; old seed volume counts never constitute production readiness.

**Verified implementation evidence (2026-09-12).** Immutable text schema,
reviewed mode suitability and role-blind retained 5+3 dealing passed independent
source review and cold tests: 35 media top-level tests (123 terminal cases) and
9 CLI tests, zero failures/skips. Run `text-transition-rest-20260912` records
exact logs. Unicode/client rendering and accepted-source lifecycle now have independent
proof in the resumption record. Final pinning/action audit and actual
editorial/release evidence remain separately open; synthetic fixtures
never authorize production activation.

#### Phase 2 execution detail — 2026-09-12

**Goal/boundary.** Build the shared text release and dealing path, then collect
the distinct editorial evidence required for availability. Synthetic releases
exercise implementation with zero live value; they never satisfy human review.
These numbered children decompose the ten parent items without changing counts.

**Ownership/API.** Content owns `server/pkg/media/`, `tools/mediapack/` and their
fixtures; the parent coordinates `portal`/`workbench` adapters and config. Use the implemented
`TextSnapshot`, `TextDeal`, `LoadTextPack`, `CertifyText` and `TextReleaseStore`
boundaries; the original image/association counterparts are retired. Agree an immutable
validated release snapshot, complete secret schedule and role-blind deal result
with the engine owner before either changes the shared call boundary. The result
carries content revisions and extra system seeds; the engine allocates distinct
physical copy IDs. Only authorized v2 projections reach players. Discover exact
symbols through CodeGraph before changing consumers or adding files.
The agreed adapter is immutable `TextSnapshot` → `TextDeal`, with independent
schedule/hand/system randomness, pinned release/language/rules/hash, Nowns,
per-seat cards/reserve and per-round system seeds. Reviewed High/Distant pair
bands retain configured minimum coverage when used; no fabricated vectors or
omitted metadata may masquerade as whole-schedule suitability.

| Parent / child | Ordered implementation and proof |
|---|---|
| 1a — Records | Add strict format/release/rules/language/pool/Nown-kind and immutable revision records; negative tests reject unknown/nontext/duplicate/incompatible records without coercion. |
| 1b — Manifest integrity | Validate exact member hashes, provenance/license/age and certification references before constructing a snapshot; tamper, traversal and missing-member fixtures fail closed. |
| 2a — Text boundary | Freeze configurable UTF-8 byte bounds and normalization rules shared with client validation, with grapheme-aware layout; reject invalid Unicode, blanks, excess length and spoofing controls while preserving Turkish casing, accents and legitimate RTL. |
| 2b — Duplicate/render proof | Separate canonical duplicate candidates from human near-duplicate decisions; hostile markup remains inert plain text, with expansion/line-overflow fixtures and no URL fetch. |
| 3a — Immutable lookup | Add release-bound lookup that cannot expose mutable manager storage; attempted caller mutation and failed activation leave existing snapshots unchanged. |
| 3b — Match pin adapter | Replace global active lookups in match rendering with pinned release bytes; replay the Phase 1 same-ID wording-drift reproduction and prove history/reconnect/verdict retain original wording. Parent coordinates `game/text_snapshot.go` with engine work. |
| 4 — Suitability | Version reviewed mode/prompt/card relations and optional evaluator metadata; validate pool/rules/language coverage without mandatory synthetic vectors. Negative projections prove candidates/model/thresholds never enter player DTOs. |
| 5a — Schedule/retention | Select distinct full 2/3-round schedules and retain each seat's actual 5+3 against every scheduled prompt; deterministic infeasible fixtures reject before admission. |
| 5b — Fairness | Same schedule/RNG inputs produce identical deals independent of later role assignment; both-size tests inspect retained hand/reserve coverage, depletion and absence of fallback/refill. |
| 6a — Seeds/copies | Generate distinct-text neutral seeds independently of roles/prompts, outside player budgets; tests prove copy uniqueness and correct three-bag/per-seat-display/single-target counts. |
| 6b — Action viability | Add mode-specific reachable-state certification for consumption, replacement, transfer/refusal and target chains; exhaustive small fixtures expose stranded choices and preserve explicit no-card pass behavior. |
| 7 — Reproducibility | Keep five-mode synthetic fixtures isolated and labelled; save release/rules/tuning identity, seed, ordered actions/clock inputs and simulation denominators. Repeat runs agree; sampled production candidates are never labelled exhaustive. |
| 8a — Accepted input | Adapt approved immutable text revisions with original consent/credits into bundle input; withdrawn/unreviewed/mutated content and mismatched approval identities refuse. |
| 8b — Lifecycle | Implement separate prepare/certify/publish/activate/takedown operations with immutable artifacts, audit and compatible text rollback; repeated publication neither duplicates approvals nor grants value. |
| 9 — Editorial pilots | Prepare exact-revision scripts and evidence records for three themes/two cultures, all five modes and both sizes; conduct human sessions and record comprehension/ambiguity/opening-seat/draw/repetition outcomes. Unrun sessions remain pending. |
| 10 — Gate | Independent review and unified verification cover loader/dealer/tools/adapters; each released mode/language additionally requires its recorded human decision and actual compatible activated artifact. |

**Risks/dependencies.** Do not make a mutable `Pack` pointer an immutability
promise or replace all-schedule coverage with first-prompt counts. Optional
embeddings cannot become a semantic judge. Engine tests may consume synthetic
snapshots while pilots/review continue; availability stays closed until both
technical and human release gates pass. Parent alone records completion/staging.

#### Phase 2 continuation — executable action evidence, 2026-09-19

**Goal.** Bind technical action evidence to replay through the authoritative
five-mode engine, preserving separate human editorial and release decisions.
**Non-goals.** No gameplay/economy changes, fabricated human evidence, enabled
production cells or claim of exhaustive production-state enumeration.
**Ownership.** The certification implementer owns `server/pkg/media/` action
artifact types, `server/pkg/textcert/`, the mediapack adapter and the narrow
`server/internal/store/text_release.go` integration plus corresponding tests.
Existing gamebot replay machinery is the reference; share engine execution
without copying gameplay rules. The coordinator owns roadmap/tracking/docs.

| Child | Implementation and proof |
|---|---|
| A1 | Define bounded typed evidence binding exact content, rules, full tuning, algorithm, cells and replay inputs; reject missing coverage, tampering, unsupported scope and exhausted budgets. |
| A2 | Implement an acyclic `textcert → game → media` wrapper for deterministic full-schedule sampled action witnesses at both sizes across all modes; cover draws, timeout, replacement, chains and trade accept/refuse/expiry with conservation and privacy assertions. |
| A3 | Generate and verify private action artifacts through mediapack and publication/activation/restored-release paths; generic human action attestations alone cannot satisfy executable proof. Preserve immutable release lineage and no repeated approval rewards. |
| A4 | Retain separately labelled small exhaustive/depletion fixtures; run targeted red/green, independent review, cold verification and the unified gate. Close technical parent items only when their full proof is present. |

**Risks.** Avoid a media/game import cycle; bound replay work before allocation;
never expose privileged seeds/actions in player payloads or ordinary logs.
Sampled candidate replay and small exhaustive fixtures have distinct scope.

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

- [x] Repair Room/Match lock/callback ordering with real disconnect broadcasts and concurrent completion; no socket I/O under nested state locks.
- [x] Add stable match/round/action/copy identity and one-owner location registry for hand/reserve/board/discard/pending; copied wording never conflates copies.
- [x] Implement whole-match ordered public evidence and private per-seat state, round reset and bounded snapshots using the pinned pack/rules.
- [x] Fix draws to current-turn-only, private owner delivery/public count and exactly-once penalty; reject eliminated/disconnected/pending-offer draws.
- [x] Remove specialty dealing/use from text engine and reject legacy specialty/dev grants; preserve timeout passes and tied runoffs.
- [x] Implement Missed the Briefing response confirmation/consumption, preview revision and attributed evidence with idempotent retry.
- [x] Implement Secret Scale atomic card+1–5 placement, neutral labels, multiple copies per rating and immutable history.
- [x] Implement Make Room three-item seeding and atomic slot replacement/discard with original before/after evidence retained.
- [x] Implement Bad Bargains offer reservation, recipient-only accept/refuse and atomic ownership transfer with display/hand conservation.
- [x] Implement Bad Bargains response deadline, disconnect/forced-transition cancellation and no-recipient pass; accept/expiry/leave race has one result.
- [x] Implement Top That neutral seed/current-target revision and ordered chain, with no semantic superiority judge or veto.
- [x] Prove all-mode shared vote budgets, early team victory, tied runoff, eliminated permissions, timeout penalties, grace/forfeit/low-population and begun-round-only verdict.
- [x] Update dev/test gamebot policies and seeded ordered replay scripts for each mode/role, own observation only; test matches never earn live value.
- [x] Gate: engine/property/race/no-leak tests pass at 4/6 and multi-round schedules, with no pending trade crossing round/elimination/verdict and no duplicate finish.


#### Phase 3 execution detail — 2026-09-12

**Goal/boundary.** Extend one serialized match engine to the five adopted actions
with conserved physical copies and complete recipient-safe evidence. Engine
ownership is `server/internal/game/`, `lobby/room.go`, their tests and
`tools/gamebot`; the parent owns lobby-manager/transport/handler/client integration
and durable store work. Shared v2 schema
changes require the contract owner and regenerated shared fixtures. No local
prototype grants live value, and no legacy intent silently becomes a new action.

**First dependency.** Agree the release/deal adapter above and the mutation/output
boundary before parallel edits. Mutate state under one match serialization order;
return immutable event/callback work, release incompatible locks, then deliver
through one connection writer. Stable outcome identities must survive callback
retry; Phase 5 owns their durable persistence.

| Parent / child | Ordered implementation and proof |
|---|---|
| 1a — Lock regression | Land the actual Room→Match→Room disconnect/deadlock regression with the repair; bounded real WebSocket tests finish and preserve intended broadcasts. |
| 1b — Output ordering | Move finish callbacks/socket writes outside nested state locks and serialize each connection writer; race-test disconnect, last Ready, simultaneous completion and error/event output. |
| 2a — Identity | Establish new match/round/turn/phase IDs and a copy registry mapping each physical instance to exactly one legal zone/owner; duplicate text does not merge copies. |
| 2b — Conservation | Centralize move/reserve/transfer/discard validation and mutation; property tests prove the conserved initial-plus-system multiset after valid, rejected and duplicate actions. |
| 3a — History | Retain attributed ordered actions, public before/after state, penalties and system seeds by original round; board reset preserves hand/reserve and previous evidence. |
| 3b — Projection | Build v2 active-Nower/Donower/eliminated snapshots from pinned bytes, including ballot/Ready/pending deadlines; full serialized scans reject future prompts, other hands/reserves and role-linked value. Page assembly preserves complete history. |
| 4 — Draw repair | Add current-turn-only text-engine draw proof; private instances/public count, one penalty per copy and no mutation for disconnected/eliminated/pending/stale requests. Keep the historical v1 behavior test until its consumer is retired under Phase 6. |
| 5 — Legacy rejection | Text setup deals no specialties; specialty/dev grants fail explicitly at every text entry point. Keep ordinary timeout passes, random penalty discard, no-card pass and tied runoff proofs. |
| 6 — Respond | Validate phase/turn/copy/preview revision, consume once and append attributed response; same request/body replays, changed body/stale revision refuses unchanged. |
| 7 — Place | Commit copy plus integer 1–5 atomically; duplicate ratings are legal, neutral endpoints expose no criterion, malformed ratings never spend a copy. |
| 8 — Replace | Seed three distinct items, replace one occupied slot and archive its removed copy; before/after history remains exact and restoration requires another owned copy. |
| 9a — Offer reservation | Validate connected recipient, both instances and board revision; reserve exactly those copies and block proposer draw/action while pending. |
| 9b — Offer resolution | Only the named recipient accepts/refuses; accept swaps offered hand/requested display while conserving hand size, refusal restores reservation and preserves public knowledge. |
| 10a — Response clock | Start absolute trade deadline independently of expired submission clock; fake-clock accept/expiry races produce exactly one resolution and advance once. |
| 10b — Cancellation | Participant disconnect/leave and forced round/elimination/end cancel first; no-recipient path passes without loss. Recipient keeps their later turn; Poke uses existing play budget. |
| 11 — Top | Confirm owned copy against exact current-target revision, append an immutable attributed chain and advance; no ranking, veto or semantic evaluator decides legality. |
| 12a — Vote/clock matrix | Both sizes/all modes prove fixed 2/3 vote budgets, early team result, one runoff, final ties, result Ready and discussion based on original table size; Ready never implicitly follows a cast vote. |
| 12b — Absence/result matrix | Fake-clock grace/reconnect/forfeit/scored-low-population cases preserve role/points rules, eliminate permissions immediately and reveal only begun Nowns. Duplicate close emits one immutable outcome. |
| 13 — Real test clients | Adapt gamebot to v2 observations and deterministic ordered scripts for each mode/role, including recipient replies; prove no global secret access and authenticated prototype zero-value policy. |
| 14 — Gate | Review then unified validation plus focused race/property/no-leak suites cover 4/6 and multi-round schedules. Pending offers never survive a forced boundary; whole-match copy conservation and single finish hold. |

**Risks/dependencies.** Timer callbacks must carry the phase/offer identity they
were created for; reconnect never extends a deadline. Known public cards remain
historical knowledge after returning to private hands. Content certification and
financial persistence continue independently, but their actual adapters must be
verified before a production gameplay gate can pass.

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

- [x] Implement pre-admission v2 handshake/version rejection, monotonic sequence assignment, gap detection and authorized snapshot resync without replaying mutations.
- [x] Implement explicit mode/size/content-language FIFO queues with no silent substitution; waiting preserves place, change leaves/rejoins atomically.
- [x] Implement mode availability and default/last-choice discovery; disabled/unreleased modes cannot queue or silently become another mode.
- [x] Implement full connected Ready lobby, revision invalidation on settings/membership, owner-only settings and deterministic host transfer; revalidate next-match Host Pass sponsorship without charging guests or silently changing packs.
- [x] Implement settings/Ready rematches for Local/Quick Play, host rules, replacement members and leave; Quick Play retains its caps/core-featured eligibility and same-tuple FIFO replacements. Reject downsizing over current membership; cancel old reservations on settings change.
- [x] Render match contract and action previews; reducers operate on copy IDs and reconcile duplicate/out-of-order events without spending another card.
- [x] Build five mode controls and boards with attribution/chronology, including trade recipient response and public known-card evidence.
- [x] Build role-scoped reconnect with current board/history/private hand/pending deadline; clear secret views on elimination/logout/match change.
- [x] Remove gameplay catalog sync/prefetch and migrate only obsolete cache keys/service-worker generation; retain authentication/account preferences and non-playable assets.
- [x] Localize mode/rule/error/instruction copy; keep content language explicit, test long/RTL/diacritic text and touch/keyboard/screen-reader action equivalence.
- [ ] Exercise native/PWA 4/6-seat flows, resize/reduced motion, stale deep links, old client/new server and new client/old server refusal; profile action/history updates against Blueprint client p95 ≤16.7 ms on the stated low-end device.
- [ ] Gate: no text secret in downloaded catalog, persistent cache, logs, unauthorized/hidden/eliminated semantics or unauthorized snapshot; active-Nower reveal is screen-reader accessible, all five action and lobby journeys proven, no stale hidden controls.

#### Phase 4 execution detail — 2026-09-12

**Goal/ownership.** Integrate the frozen v2 contract through existing lobby,
handler, client DTO/session and game-screen paths. The parent owns manager/
handler changes; coordinate Room integration with its engine owner. Assign client
DTO/reducer and presentation/localization separately if parallel workers are
available. Use the Soft Neo-Brutalism skill before UI edits; retain existing
tokens, fonts and accessible role checks. No persistent secret catalog or
compatibility adapter may quietly translate legacy actions.

| Parent / child | Ordered implementation and proof |
|---|---|
| 1a — Admission handshake | Negotiate v2/rules before seat binding, quota reservation or mutation; real sockets reject old/unknown versions with stable codes and no reserved seat. |
| 1b — Streams/resync | Assign recipient/evidence sequence and stream epoch at the single output boundary; gaps request an authorized atomic snapshot/page set, duplicate events and resync requests never replay actions. |
| 2a — Queue identity | Key FIFO by explicit mode/size/content-language/compatibility/eligible pack, with one account reservation; concurrency tests prove no double seat and no cross-tuple matching. |
| 2b — Queue change | Keep waiting preserves position; change atomically cancels old tuple and joins new tail, leave/disconnect/retry releases exactly once. Timeout only offers the three explicit choices. |
| 3 — Discovery | Serve available mode/language/pack metadata without secret content; default/remembered-mode tests explain withdrawn choices and refuse disabled queues. |
| 4a — Settings/Ready | Enforce host-only settings revision, full connected membership and Ready acknowledgments; stale revision, membership change and oversized downsizing refuse/reset as specified. |
| 4b — Host sponsorship | Host departure elects longest-present connected member, with seat order breaking a timestamp tie; transfer clears Ready and revalidates next-match pack access. Started match stays pinned, guests are never charged and missing sponsorship requires an explicit new choice. |
| 5 — Rematch | Return both entry paths to settings/Ready; Local host persists when present, Quick Play host is the lowest original seat among returners. Preserve each entry path's caps/packs; same-tuple FIFO replacements enter unready and membership changes clear Ready. Setting changes cancel old replacement reservations before publishing the new tuple; an empty table is retired rather than creating a host policy. |
| 6a — Client state | Parse v2 into typed copy-ID state; apply atomic snapshots and ordered events with dedupe/epoch validation. Replay/out-of-order/error tests never remove or spend another equal-text copy. |
| 6b — Confirmation | Show pinned contract and revision-bound action preview; editing preview is local, confirmation emits one stable request identity and stale replies require refreshed state. |
| 7 — Mode boards | Add response, scale, three-slot bag, display/trade and chain controls individually with widget tests for legal targets, attribution and chronological history; keyboard/touch/semantics invoke equivalent confirmations. |
| 8 — Private reconnect | Restore only authorized prompt/hand/history/pending absolute deadline; elimination/logout/match change clears private state, semantics and stale buffered callbacks. Test hidden/background and later reconnect paths. |
| 9 — Cache boundary | Remove active gameplay catalog/prefetch use, version obsolete cache/service-worker keys selectively, preserve auth/preferences/avatar assets; seeded-v1-cache tests prove no stale secret or broken login. |
| 10 — Localization | Add stable system codes/ARB messages and explicit content-language labels; pseudo-locale, Turkish/RTL/large-text tests preserve authored text and readable action/error controls. |
| 11a — Journey matrix | Run real v2 4/6-seat native/PWA join/action/disconnect/rejoin/rematch plus stale link/cache and both version-mismatch directions; record device/build/platform evidence separately. |
| 11b — Accessibility/performance | Verify focus order, screen-reader role reveal, reduced motion, resizing and worst-history update profiles; record actual low-end device p95 frame total against 16.7 ms, not a desktop substitute. |
| 12 — Gate | Review and unified tests plus complete wire/cache/log/semantics no-leak checks and all five mode journeys. Manual device evidence stays unexecuted until observed; disabled modes remain unavailable. |

**Risks.** Queue changes and blocks share reservation serialization; Ready is a
revisioned server fact, not a local checkbox. Reconnect drains obsolete pending
requests rather than replaying mutations under a new match. Native iOS evidence
requires macOS/Xcode; absence does not stop Android/PWA or server implementation.

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

- [x] Apply additive schema and resumable bounded backfill to test copies; old rows remain legacy/unknown, not fabricated new-mode history.
- [x] Implement stable match/account admission and exactly-once start counter; share Quick Play caps across modes, release canceled reservations and preserve local uncapped access.
- [x] Implement idempotent durable result/points/XP/leaderboard/Noin settlement with transaction/outbox recovery and no duplicate value after callback/crash retry, including scored low-population endings.
- [x] Implement confirmed server-interruption closure and exactly-once free-allowance compensation; preserve committed awards, issue no fabricated completion/first-win/XP/points and prove crash-after-start/mid-award/retry.
- [x] Repair daily leaderboard counts, concurrent first-win/first-counter creation and conversion/earn caps at UTC/week boundaries across modes.
- [x] Preserve instant durable event Noin plus private settlement, absent/low-population rules and prototype zero-value; no rewards for item trades or ratings.
- [ ] Reconcile legacy theme entitlements using reviewed equivalent-benefit mapping; inventory unresolved paid promises and remedy before disabling access.
- [ ] Preserve consent/submission/challenge/report relationships and text-only new-write controls; approvals/payments never rerun from backfill/publication.
- [x] Complete text curation/activation/takedown and public report resolution with immutable revisions; automated screening plus human decision before visibility.
- [x] Complete challenge scheduler and exactly-once weekly title/payout transfer with restart and week-boundary tests.
- [x] Repair Guard identity, overlapping suspension isolation and expiry; prove admin-final enforcement and role separation.
- [ ] Implement versioned chat/UGC terms acceptance and private user block/report/contact journey: hide authored chat/UGC, preserve required game evidence, prevent future co-matching/invites and prove no role leak or mid-match score manipulation; review platform compatibility.
- [ ] Complete audited moderation/admin operations and text-provider contribution journey with failure/no-visibility proof.
- [x] Complete OAuth linking and second-device restoration with account-collision, revoked-token and lost-session proofs.
- [ ] Complete in-app account deletion with reviewed retention, relational/JSONB/blob cleanup and reauthentication tests.
- [x] Complete entitled avatar screening/activation/takedown with unchanged non-playable WebP behavior and rejected-provider proof.
- [ ] Complete platform receipt validation, restore/refund and idempotent entitlements using platform test evidence.
- [ ] Complete verified SSV/Premium doubler and consent flows with replay, refusal and private reward tests.
- [ ] Verify ledger append-only enforcement, wallet reconciliation and deletion/retention handling across rows/JSONB/blobs; no private role rewards in public stats/analytics.
- [ ] Gate: financial/data parity and duplicate/failure-recovery proofs pass; current user value preserved, live trust surfaces verified, unresolved launch dependencies explicit.

#### Phase 5 execution detail — 2026-09-12

**Goal/boundary.** Implement durable value and trust with actual PostgreSQL
constraints and recovery. The parent owns initial migrations/store/economy work;
assign later account/community/provider modules one owner each before editing.
Schema and synthetic outcome tests can proceed alongside Phases 2/3. Actual
engine/handler callbacks must use this path before integration is complete.
Preserve numeric policy and all original migration bytes; the no-deployment
attestation does not weaken fixture parity or future deployment preflight.

**Initial contract.** Recheck and use allocated `000009_text_match_admissions`
and `000010_idempotent_match_settlement`, then separately `000011` content and
`000012` trust. Pin Phase 1 historical tests to actual migrations 1–8 as new head
advances; add fresh→new-head and actual 8→new-head proofs. Match ID, immutable
body hash, account/event ordinal and original UTC day/week are shared identity,
never a reusable room ID or retry time. Follow the reviewed fence/week/account/
day/profile/wallet lock order and bounded whole-transaction retry protocol.

| Parent / child | Ordered implementation and proof |
|---|---|
| 1a — Admission schema | Add match/participant/admission states, FKs/unique keys and pending indexes in 9; real SQL tests reject duplicate seats/accounts and impossible states, preserve old reads, and refuse populated down. |
| 1b — Settlement schema | Add immutable outcomes/award receipts, settlement/outbox, first-win/day/leaderboard claims in 10; prove conflicting identity refusal, required indexes/FKs, append-only privileges and empty-only exact down. |
| 1c — Content/trust schema | Implement 11/12 separately with nullable legacy metadata, private block/terms keys and reviewed retention; execute the realistic Phase 1 corpus against actual bounded copy/verify/resume and conflicting-source behavior. |
| 2a — Reserve/release | Atomically reserve match/account eligibility against current entitlement and shared free-day allowance; duplicate reserve/cancel is a no-op, local/paid/prototype does not consume free quota, rematch gets a new identity. |
| 2b — Start | Revalidate and commit start/count once after feasible content/full Ready; failed setup releases reservations. Race last-slot starts, entitlement expiry, settings cancellation and UTC rollover on real PostgreSQL. |
| 3a — Outcome receipt | Establish one immutable terminal outcome and participant settlement intent; duplicate callbacks agree, conflicting bodies reject, including scored low-population and caught teammate wins. |
| 3b — Atomic effects | Apply profile points/XP/eligible leaderboard and Noin receipts atomically per account with durable retry/outbox; inject failure after each write and uncertain commit, then replay without missing or duplicate effects. |
| 3c — Runtime adapter | Replace legacy sequential finish writes with durable outcome delivery, including low-population callback; integrate actual engine finish/reconnect and prove retry after callback failure. |
| 4a — Fenced interruption | Persist owner incarnation/epoch and serialized terminal/interrupted transition; paused old writers lose the fence and cannot award after recovery. A client disconnect alone cannot claim interruption. |
| 4b — Compensation | Compensate consumed free admission once in its original day; crash after start/mid-award/worker restart preserves committed events, invents no terminal value and never extends paid expiry. |
| 4c — Process lease schema | Allocate `000013_text_process_ownership` after trust 12: persistent incarnation UUID/generation, singleton current owner, loss/recovery state and owner identity on reservations/matches. Preserve historical rows; fresh/upgrade/empty-down tests reject impossible ownership and populated down. |
| 4d — Session authority | Acquire one PostgreSQL session advisory lock on a dedicated pinned connection, then atomically advance the singleton generation and record predecessor loss. Prove two-process exclusion, failed acquisition cleanup, forced backend loss, healthy release and permanent refusal by a lost instance. A heartbeat expiry alone never authorizes takeover. |
| 4e — Transaction fence | Bind production `TextValueStore` to the acquired token; before reserve/prepare/start/award/finish/cancel/interrupt, hold the singleton `FOR SHARE` and validate current ownership through commit. Takeover uses `FOR UPDATE`. Prove a paused old transaction completes before takeover or fails after it; unbound writers refuse whenever a singleton exists. |
| 4f — Bounded lost-owner recovery | Process only confirmed-lost incarnations in stable bounded batches: release unbound reservations, cancel prepared matches, interrupt started matches and compensate actual consumed free quota once. Preserve committed outcomes/awards and drain pending terminal settlement independently. Crash between batches, retry after uncertain commit, midnight and simultaneous finish/recovery tests retain exact value parity. |
| 4g — Runtime readiness | Inject the owner token/check into `TextManager`; remove its independent random process label. Admission opens only after prior-owner cleanup; lease loss stops admission/actions/output and never silently reacquires the same incarnation. Real WebSocket/process-loss tests return clients cleanly without reconstructed hands, offers or completion rewards. |
| 5a — Shared caps | Serialize first-row creation and unique first-win/day/leaderboard claims with conversion/gameplay writers; race all modes at earn/conversion/Quick Play limits and midnight without moving old events into new buckets. |
| 5b — Week close | Persist closing cutoff, drain accepted work without holding worker-required locks, then atomically seal immutable rankings; delayed writers/repeated close cannot alter history. |
| 6 — Event timing/privacy | Credit each correct-vote/survival event once at occurrence, including capped-zero receipts; absence/interruption preserves it. Only authorized private settlement displays role-linked amounts; ratings/trades and prototype matches grant nothing. |
| 7 — Paid equivalence | Implement explicit reviewed legacy-theme mapping with unresolved-state reporting; tests preserve owned benefits and block disabling unknown promises. Current deployed-owner set is N/A, not a fabricated universal mapping. |
| 8 — Historical relationships | Preserve submission/challenge/report/terms/blob/credit links while validating text-only new writes; retry backfill/publication without approval or reward replay. |
| 9 — Curation operations | Connect screened-plus-human-approved immutable revisions to certification/activation/takedown and public report resolution; privilege, stale-revision and failed-screen tests expose no unapproved content. |
| 9a — Content administration | Add authenticated, CSRF-protected internal-console capture/export of exact accepted inputs, bounded certified-bundle publication, separate activation/withdrawal and restart-visible release metadata. Store operations validate the actor again and commit their audit atomically; browser tests cover failed screening, unauthorized access, inert text and no publication payout. |
| 9b — Archive operation | Expose explicit bounded archive start/copy/verify/status operations with a deadline and an audit in the same transaction. Interrupted requests resume existing progress; read-only status never starts a backfill. |
| 9c — Immutable source boundary | Protect actual historical asset fields and approved decision/consent bytes. Submitted text cannot change until an explicit withdrawal, and challenge entry content stays immutable; report resolution references exact release/revision and never masquerades as a status-only takedown. |
| 10 — Challenge close | Add deterministic scheduler/close identity, one current title and exactly-once winner payout; test restart, ties under the documented rule, slot reopening and boundary races. Any unspecified winner-tie policy needs a concrete decision before that branch. |
| 11 — Guard enforcement | Correct authenticated Guard identity, one freeze per Guard/target, overlap isolation and expiry; only admin final actions affect all relevant sessions, with audited reversible case decisions. |
| 12a — Terms/block store | Require versioned user terms before authored chat/UGC and immutable contribution consent separately; reject self/duplicate abuse and implement private symmetric future-match exclusion. |
| 12b — Safety journey | Wire block/report/contact UI and queue reservation serialization; hide authored chat/UGC while preserving card/vote evidence, current membership/scoring and private block identity. Test unauthorized access and no role leaks. |
| 13 — Admin/contributor UI | Complete existing audited product mutations, role separation, CSRF/session controls and text submission/review journey; failure paths cannot publish or silently bypass screening. |
| 14 — Identity restore | Complete OAuth provider validation/link/second-device restore with collision, revoked-token and lost-session tests; provider sandbox evidence remains distinct from deterministic contract fixtures. |
| 14a — Development identity | Migration 14 adds immutable account purpose, preserving existing players. A server-configured local/staging prototype may issue signed development credentials through a secret-authenticated route; production rejects both token and stored development identity before admission. Device/OAuth/refresh cannot convert its purpose; tests prove zero prototype value and no ordinary-account substitution. Guard identity uses migration 15. |
| 15 — Deletion | Add reauthenticated in-app deletion through reviewed relational/JSONB/blob retention rules; preserve legally required immutable value history with no orphan PII or public statistics. |
| 16 — Avatar | Preserve entitlement, 256×256 WebP/crop/EXIF/size behavior and provider-screen/takedown flow; rejected or failed moderation never activates an upload. |
| 17a — Billing implementation | Implement authenticated server receipt verification, unique transaction entitlement grant and restore/refund reconciliation; fixture tests cover replay, wrong account/product/platform and provider failure. |
| 17b — Billing evidence | Run Play/StoreKit test transactions, restore and refund on configured platform accounts/devices; save redacted provider evidence. No mock or absent credentials counts as this proof. |
| 18a — Doubler/consent | Verify SSV signatures/transaction identity and Premium eligibility server-side; cap/replay/private-settlement tests prove client callbacks grant nothing and denied consent uses the allowed flow. |
| 18a decision, 2026-09-19 | The owner fixed Premium eligibility at authoritative match start. Persist that decision per match/account; later purchase or expiry cannot change it. Interrupted-match bonus treatment remains pending. |
| 18b — Provider evidence | Exercise actual configured ad verification and UMP/platform consent paths with test accounts; record revocation/refusal and policy review for intended markets. |
| 19 — Reconciliation | Check immutable accepted outcomes against every value effect and outbox item, not wallet=sum alone; enforce ledger immutability at DB privilege/trigger boundary with a separately authorized retention path. |
| 20 — Gate | Review complete integration and run unified real-service concurrency/failure/privacy checks; retain exact original-row/file parity, provider/manual results and unresolved launch dependencies separately. |

**Ownership 13 handoff.** Own `server/migrations/000013_*` and new
`server/internal/store/text_owner*.go`; coordinate changes to existing value
methods with their owner and `TextDeps`/startup with the lobby owner. Proposed
API: `AcquireTextOwner(ctx, dedicatedDB)` returns an immutable
`Token{IncarnationID, Generation}`, `Done()`, `Check(ctx)` and `Release(ctx)`;
an owner-bound value-store constructor supplies that token to all new writes.
`RecoverLostOwners(ctx, token, limit)` returns aggregate released/cancelled/
interrupted/pending counts and completion. Migration 12 remains the trust lane;
14 contains immutable development identity; 15 is reserved for Guard enforcement. 16 is provisionally reserved for durable challenge lifecycle. No hidden match state is persisted.

The singleton guard precedes match, week, sorted accounts, day, profile and
wallet locks; takeover cannot pass an in-flight guarded transaction. Persist
the owning backend identity and verify its advisory authority, not just its
last heartbeat. Keep one lease acquisition per dedicated session: advisory
locks are reentrant, so repeated acquisition is not a health probe. Explicitly
unlock on healthy shutdown; on uncertain loss discard the dedicated physical
connection/pool rather than returning a session-held lock to the shared pool.
Loss permanently fences the local instance. Recovery uses the new token and
the recorded lost incarnation, never an exception allowing the old token to
write. Stable indexes and per-match transactions keep each batch bounded;
partial progress survives another owner loss. Ownerless historical active work
must be classified explicitly before readiness, while isolated unit helpers may
operate unbound only when the database has no ownership singleton. Test this
fallback's refusal after first acquisition, backend termination, paused writer
and takeover races, healthy-owner exclusion, prepared/unbound cleanup, crash
after each compensation write, terminal-outcome precedence and clean client
return. Test/production configuration cannot bypass the persisted fence.

**External evidence.** Provider credentials/test accounts, current platform
policy review, retention/legal decisions and any actual legacy paid remedy are
required for their named paths; implement/test unrelated local paths meanwhile.
Human approval is needed only for a concrete new policy or external action, not
for the already adopted reward rules. Financial correctness cannot be inferred
from a passing fixture helper or a successful enqueue.

#### Phase 5 continuation — immutable consent, 2026-09-19

**Goal.** Preserve the exact contribution and user-terms bytes referenced by
accepted consent at the database boundary. **Non-goals.** No retention bypass,
deletion-policy invention, new terms wording or changed reward rules.
**Ownership.** The trust implementer owns a newly allocated migration 28,
focused store/portal trust tests and required migration-test updates; existing
applied migrations remain byte-identical. The coordinator owns documentation.

| Child | Implementation and proof |
|---|---|
| T1 | Reproduce direct SQL mutation/deletion/truncation of referenced contribution terms and missing retained user-consent truncate guards on guarded disposable PostgreSQL. |
| T2 | Add immutable row and retained-truncation enforcement with an empty-only controlled down; verify actual role/privilege behavior and preserve historical references/rows. |
| T3 | Prove fresh/upgrade/repeated-up/down refusal, transactional rollback and ordinary append/accept/replay paths; independent review then real-service cold and unified gates. |

**Risks.** PostgreSQL `TRUNCATE CASCADE` and owner/nonowner paths must not bypass
retention. Existing test isolation must use its guarded fixture reset, never
weaken production immutability to clear a test.

#### Phase 5 continuation — audited Noin corrections, 2026-09-19

**Goal.** Let an authenticated Admin issue an explicit Noin grant or refund one
original Noin spend, with an immutable decision and exactly one wallet credit.
**Non-goals.** No cash/provider refund, entitlement change, earn-cap bypass via
gameplay, new policy, or new migration; migration 27 already defines these
operator decisions and source-refund uniqueness.
**Ownership.** The trust implementer owns `economy/admin_corrections*.go`,
the narrow `AdminOperationStore.DecideTx` extraction, internal Admin correction
forms/route and their tests. Existing Decide remains a compatible wrapper;
the coordinator owns documentation and final tracking/staging.

| Child | Implementation and proof |
|---|---|
| O1 | Add caller-transaction decision reuse and atomic correction API; exact initiating-session checks, sorted actor/target locks, source/operation/wallet ordering, decision/audit/ledger/result commit together. |
| O2 | Require positive bounded grants; full refunds derive from one same-account negative `spend` ledger row, reject other sources/partial amounts/overflow/reuse. Preserve caps, XP, entitlements and original rows. |
| O3 | Add bounded strict internal-console form and receipt navigation with role/CSRF/no-store and escaped output; label Noin corrections explicitly, never imply cash refunds. |
| O4 | Prove retries/conflicts, source-refund/spend concurrency, authority expiry after waits, ledger/audit rollback and all unrelated value parity on real disposable PostgreSQL; independent review then cold and unified gates. |

**Risks.** The immutable operation decision and economic effect must share one
transaction. A failure cannot leave an authorized-looking pending correction
or report a credit that did not commit. Admin corrections have distinct ledger
provenance and do not pretend to be gameplay awards.

#### Phase 5 continuation — account and installation sanctions, 2026-09-19

**Goal.** Admins can issue independent permanent/timed account and known-app-
installation sanctions and lift an exact decision, with retained audit and
immediate enforcement. **Non-goals.** No hardware attestation, account merging,
Guard-history rewrite, deletion policy or provider calls. Installation identity
is resettable app data, not proof of a physical device.
**Ownership.** The sanctions implementer owns additive migration 29, new store
sanction/installation helpers, narrow auth/handler/portal/Admin authorization,
lobby/runtime enforcement and persistent client installation binding with tests.
Coordinate shared Admin files with the Noin owner before editing. The parent
owns documentation and tracking; independent review precedes implementation.

| Child | Implementation and proof |
|---|---|
| S1 | Add decision-linked account sanctions, captured installation identities, exact lifts and delivery receipts; preserve Guard provenance and retained history on upgrade/down refusal. |
| S2 | Serialize accounts before sorted installations; separate canonical anonymous bootstrap from many-account installation links. Concurrent first bootstrap rolls back losing candidates; ambiguous legacy mappings refuse without guessing. |
| S3 | Commit exact-session decision/audit, captured known installations and one epoch revocation atomically. Same-ID retry never recaptures or revokes again; an exact lift cannot clear unrelated sanctions. |
| S4 | Persist secure random installation identity before network; preserve existing signed device hash. Authenticated legacy binding rotates credentials for the same account; immutable old OAuth result bytes remain identical. |
| S5 | Apply current account/installation sanction checks after locks to token issue/refresh/OAuth, protected admission and derived browser authority. Legacy unbound player credentials require explicit binding before protected admission. |
| S6 | Validate socket binding under lobby serialization; retry pending live enforcement against the current peer and exact effective sanction. Expired/lifted receipts never close newly allowed sessions or repeatedly revoke epochs. |
| S7 | Prove real PostgreSQL bootstrap/link/sanction races, OAuth replay, session authority, migration parity and actual HTTP/WebSocket enforcement; test client restart/logout/switching/storage failures, then independent review and cold/full gates. |

**Risks.** Lock order must never acquire an existing account while holding an
installation lock. OAuth account switching remains valid and must not rewrite
anonymous bootstrap identity. Admin password sessions have no app-installation
signal and enforce account sanctions only. Captured installation hashes stay
private. Existing unbound credentials cannot reveal an originating installation;
compatibility uses explicit same-account binding, never a replacement account.

**S5 review refinement.** Production Reserve/Start require trusted copies of
each current participant's verified account, installation, epoch and exact
credential expiry/revocation identity. Copy these under lobby serialization;
within the value transaction lock the match, sorted participant accounts and
sorted distinct installation registries, then recheck authority before economic
writes. Never acquire another account after installations. Missing proof refuses
production admission; only the existing explicitly verified zero-value development
path is exempt. Test both orders of a linked peer's installation sanction versus
Start, token expiry/revocation during waits, clean-installation admission, and
every Start caller. Accepted terminal settlement/recovery must remain possible.
Independent plan review accepted this refinement before the admission edit.

#### Phase 5 continuation — leaderboard operations, 2026-09-19

**Goal.** Admins can inspect standings/history, exclude or reinstate an account
for an open week with reasons, and rerun a durable weekly close.
**Non-goals.** No score deletion, earn/quota changes, retroactive rewriting of
closed history or new title/reward policy. Exclusion/reinstatement freezes once
`closing_at` is set; reinstatement restores eligibility to accumulated points.
**Ownership.** The trust implementer owns migration 30 (after sanctions 29),
dedicated store decision/result helpers, filtered leaderboard projections,
Admin controls and close recovery integration with tests. Coordinate runtime
wiring with the sanctions owner. The coordinator owns documentation.

| Child | Implementation and proof |
|---|---|
| L1 | Add dedicated immutable week-scoped decision/result identities, reason/hash/actor and exact prior exclusion. Refuse populated down and register least-privilege/restore inventories. |
| L2 | Filter active exclusions before ranking open weeks; closed views use immutable history. Strict bounded Admin forms enforce exact session, role and CSRF; week lock precedes sorted actor/target accounts and final authority recheck. |
| L3 | Commit accepted close decision/audit and cutoff together, then drain outside account locks. Pending work survives restart; bounded retries and ordinary close converge to the same history and completion receipts without reopening a week. |
| L4 | Prove exclusion/reinstatement versus award/close races, ties/own-rank filtering, retries/conflicts, expiry after waits, atomic rollback and historical/value parity on real PostgreSQL; independent review, cold checks and unified gate. |

**Risks.** Do not reuse the account-first general operator decision path before
a week lock. A close rerun returns or completes its original snapshot, never
recalculates historical scores. Profile podium totals have legacy provenance;
this slice must not overwrite them with a guessed baseline.

#### Phase 5 continuation — Premium start eligibility, 2026-09-19

**Goal.** Preserve the owner's selected Premium-bonus eligibility at the
server-authoritative match start, including Local Rooms. **Non-goals.** No
bonus payment, SSV claim or interrupted-match policy; those remain separate.
**Ownership.** The coordinator owns migration 31 after 29/30, a narrow Start
hook, private store helper/tests and privilege/restore inventories.

| Child | Implementation and proof |
|---|---|
| P1 | Add immutable match/account eligibility rows with `premium_bonus_eligible`, authoritative start instant and policy version. Leave historical absence explicitly unknown; controlled down refuses retained rows. |
| P2 | Capture Premium independently of quota access kind under existing owner/match/week/sorted-account locks and in the same start transaction. Prototype/reward-disabled eligibility is false; exact expiry follows existing entitlement semantics. |
| P3 | Prove Local/Quick Play, free/Premium, expiry/reserve/start transitions, unchanged replay, both purchase/revoke lock orders and complete insert/final-update rollback on real PostgreSQL. No public wire or financial effect changes. |

**Risk.** A missing historical row must never be interpreted as a negative
eligibility decision by future bonus readers. Later subscription changes do not
recapture a started match. Independent plan review accepted this boundary.

#### Phase 5 reward verification foundation — 2026-09-19

**Status.** B1–B3 independently reviewed and tested; public registration and
financial effects remain closed. Premium eligibility uses the immutable start
receipt. Interrupted matches return `policy_pending` until the owner chooses
their bonus treatment.

| Child | Implementation and proof |
|---|---|
| B1 | Issue opaque match/account claims only after an applied completed settlement, known start eligibility and current account authority; enforce bounded rolling issuance. Missing historical eligibility is not false. |
| B2 | Verify the actual AdMob signed query before SQL, derive the account from the opaque claim, bind transaction/fingerprint once and accept exact retries. Signed occurrence must fall within the claim lifetime even when delivery is later. |
| B3 | Keep handlers unregistered, bound body reads including early refusals, and preserve provider acknowledgment semantics. Prove invalid signatures have no SQL effects and immutable claim/receipt rows cannot be rewritten. |
| B4 | Apply one shared Premium/SSV bonus identity with original-day caps and the selected interrupted-match policy; no double bonus or reconstructed eligibility. |
| B5 | Add private bonus delivery/reconnect and reconciliation against authoritative accepted receipts; no public role disclosure or invented historical baselines. |
| B6 | Join actual consent, provider and client paths only after the preceding proof gates; record external platform evidence separately. |

**Evidence.** Independent PostgreSQL/store/signature/config matrix passed 15 top-level
and 21 nested cases; final HTTP verification passed three tests including six
real TCP stalled-body cases. Schema 34 retains only claims and verified provider
receipts; it cannot grant Noin or rewrite existing settlements.

#### Phase 5 deletion decision packet — 2026-09-19 (resolved policy)

**Status: owner delegated mainstream mobile-app policy on 2026-09-19; the selected defaults are now in Blueprint Profile §1a.** Blueprint Profile requires
statistics erased; the transition design assigns retention policy to the owner.
The following table preserves the evaluated choices; Blueprint §1a is the adopted
policy. The current database also protects financial, consent, moderation, source and
match evidence. A disabled account UUID is still linkable, not anonymized.

| Owner choice | Concrete affected data and implementation consequence |
|---|---|
| Retained evidence and period | Specify each retained financial/consent/moderation/settlement class, its purpose, restricted readers, expiry/start event and holds; or authorize a redacted replacement representation. Ledger/purchase/subscription ownership and consent triggers currently forbid deletion. |
| Content and attribution | Choose approved-content retention with public credit, retained content with suppressed identifying credit, or withdrawal from future releases. Decide drafts/rejections/original blobs separately. Source archives contain whole rows/blobs; accepted provenance and release bundles copy nicknames. |
| Sanction identifiers | Choose restricted retention of installation/provider linkage for existing sanctions, or removal with the resulting re-registration limitation. Working credentials must not be retained as a login path. |
| Paid value/recovery | Decide whether deletion permanently ends access or permits a defined later purchase recovery. Existing provider ownership cannot simply move to a replacement account. Deletion does not perform a cash refund or cancel a platform subscription. |
| Backups/archives | Specify finite retention and restore authorization, with deletion suppression before restored service admission or explicit copy replacement/purge; define completion wording while older copies remain. |

**Prepared engineering boundary.** A fresh same-account reauthentication creates
one-use intent and durable request; authority denial/revocation is atomic.
A versioned disposition manifest applies bounded resumable batches across
relational columns, JSONB, arrays, blobs, source archives and backups. Ordinary
runtime cannot bypass retained-value guards. Delayed settlement/provider work
must neither strand other players' value nor recreate deleted profiles/access.
Proof uses identifiable synthetic markers in every data class, crash/acknowledgment
failures, replay/account-switch races and actual restore suppression.

**Current source evidence.** Migrations 2/3 contain identity, public counters,
ledger JSON, raw receipts and Admin secrets; 10 contains retained match/settlement/
outbox/leaderboard evidence; 11 freezes source archives and attribution; 19/21
retain provider ownership; 20 contains OAuth receipts; 25/27/28 enforce retained
truncation. The execution inventory includes sanctions 29, leaderboard 30 and
Premium eligibility 31. No deletion API or cleanup is claimed by this packet.

#### Phase 5 continuation — account deletion, 2026-09-19

**Goal.** Execute Blueprint Profile §1a with fresh same-account confirmation,
immediate access/publication fences, bounded personal-data removal and honest
retention status. The owner's policy delegation resolves the earlier policy
block; implementation and proof remain required.

| Child | Implementation and proof |
|---|---|
| D0 | Review the exact schema/column/JSON/blob disposition manifest, surviving-player effect witnesses and privacy authority design in ADR-014. Unknown inventory refuses cleanup; original hashes remain historical anchors, never hashes of rewritten payloads. |
| D1 | Add isolated, narrowly scoped privacy authority and transaction-bound transformations. Ordinary runtime SQL, role switching, GUC spoofing, RLS and cascading truncation cannot bypass retained-data protections. |
| D2 | Add one-use reauthentication intents, atomic confirmation/revocation/public suppression and separate read-only status capabilities for guest, linked and sanctioned accounts. Join HTTP, app and external web paths only after executor coverage is complete. |
| D3 | Complete pending matches/settlements for surviving players without restoring deleted value or profiles. Fence authored release admission, withdraw affected content and erase copied attribution/source blobs; replacement packs require independent certification. |
| D4 | Resume bounded cleanup and purpose-scoped evidence expiry; enforce finite sanction digests and explicit scoped retention holds. Prove exact nondeleted-row/value parity, crash recovery and every writer/confirmation race. |
| D5 | Enforce backup expiry and processor acknowledgments; export suppression to authority independent of restored application data, then require fresh replay before restored admission. Stale/missing suppression authority keeps admission closed. |
| D6 | Run a complete joined synthetic deletion/expiry/restore journey, independent review and the unified suite before exposing deletion. Physical/provider and production operator evidence remain separately required where applicable. |

**Risks.** Setting `accounts.deleted_at` alone strands settlement because ordinary
value locks deny deleted accounts. Accepted terminal work needs a narrow separate
path with deletion-disposition receipts. Shared immutable JSON requires verified
survivor witnesses before original bytes are removed. No generic trigger bypass,
blanket evidence exemption, fabricated historical baseline or support-only
deletion path is permitted.

#### D5a — bounded backup restore lifetime

**Goal.** Refuse restoration of an expired captured copy under Blueprint's
90-day maximum. **Non-goals.** No production backup authorization, purge receipt,
processor acknowledgment or independent suppression/freshness claim.
**Files.** Existing `infra/compose/snapshot.py`, snapshot tests and runbook.

| Child | Implementation and proof |
|---|---|
| D5a1 | Pin capture-start UTC creation and expiry in the immutable manifest; require a positive lifetime no longer than 90 days. Long captures cannot publish an already expired manifest. |
| D5a2 | Validate retention before any restore target or child command, including CLI dry-run. Missing/malformed/future creation and exact expiry refuse; integrity verification alone is not restore admission. |
| D5a3 | Prove integer/UTC boundaries, invalid metadata, no target creation on refusal and actual isolated snapshot restore parity; independent review and cold verification. |

**Risks.** Backup timestamps cannot certify an independent deletion watermark or
rollback resistance. Actual expired-copy removal and external authority remain
D5 work; a valid lifetime never opens application admission.

#### D5b — independent suppression fixture journal

**Goal.** Give D2 a concrete durable, idempotent suppression publisher whose
records can be authenticated independently of the application backup.
**Non-goals.** No deployed service, retention purge, processor proof or restored
application admission. The fixture cannot certify production rollback resistance.
**Files.** New `server/internal/privacy` package and tests; D2 consumes its
`Publisher` interface without coupling this package to auth/store.

| Child | Implementation and proof |
|---|---|
| D5b1 | Canonical typed request/receipt, installation/key identity and purpose-HMAC account selectors; signed bounded hash-chain records never persist raw account UUIDs or keys. Test exact replay, conflicting identity, malformed/unknown JSON and privacy markers. |
| D5b2 | Serialize independent opens using a stable lockfile; atomically fsync state and directory, then durably advance an independent minimum. Uncertain replay re-fsyncs before acknowledgment. Test concurrency, cancellation, fault/reopen and rollback/truncation against the separate minimum. |
| D5b3 | Sign a verifier-supplied random challenge with fresh head/installation/key identity; verify every sequence/digest/signature and the independently trusted minimum. Test stale/future/replayed challenge and changed identity; integrate D2's append/bind retry. |
| D5b4 | Independent review and cold verification. Document private directory ownership, backup-root separation, exact fixture limits and the still-unconfigured production authority; connect actual restore only after executor coverage. |

**Risks.** Journal and minimum must stay outside restored data; signed old bytes
alone do not prove freshness. Keys are separately supplied, never captured in
application artifacts. A locally mocked minimum is test evidence only.

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

#### Phase 6 execution detail — 2026-09-12

**Goal/boundary.** Prove exact disposable restore/cutover behavior, then remove
obsolete executable consumers. Assign `infra/`/ops scripts and retirement modules
explicit owners; inspect callers before deleting dependencies. No deployment
exists, so live migration/drain/rollback-window observations are N/A today;
document that fact instead of inventing a host. Artifact rehearsals and retained
data tests still apply. Future deployment actions require their concrete target,
release manifest and operator authorization.

| Parent / child | Ordered implementation and proof |
|---|---|
| 1a — Snapshot | Repair existing snapshot tooling with finite waits, checked errors, safe destination/permissions and complete PG/retained-object manifests; forced failure leaves no falsely valid backup. |
| 1b — Restore | Restore only into uniquely named empty disposable stores with explicit identity guards; prove roles/sequences/FKs/rows/blob/checksum parity and refusal to overwrite an occupied target. |
| 1c — Drain | Implement authenticated admission-close/status based on real room/trade/settlement counts; timeout reports pending work, maintenance and Redis messages alone cannot report drained. |
| 2 — Upgrade/rollback | Rehearse old restore, actual 8→new-head interrupted backfill and new restore separately; exact empty-only down refuses retained data, compatible app/pack rollback preserves target writes, forward fix remains executable. |
| 3a — Release artifact | Build actual production image plus secret-free config/protocol/text release manifest and startup validation; wrong schema/dirty state/uncertified pack refuses readiness. |
| 3b — Version fence | Start old fixtures under original rules and drain before v2 admission; explicit v1 rejection never binds/charges. New client/old server also fails clearly. |
| 4a — Single writer | Coordinate app/admin/community/jobs/receipt writers and durable pending work with cutover watermark; disposable dual-writer attempts refuse, held callback replay applies once. |
| 4b — Selective invalidation | Namespace/expire stale room/queue/cache generations while retaining identity/security/entitlement state; fixture parity and real human smoke cover every enabled mode/size. |
| 5a — Outage recovery | Inject server, Redis and PostgreSQL loss, restore from actual artifacts and prove readiness recovery/clean interruption without reconstructed hands or duplicated value. |
| 5b — Budgets | Run recorded-host 100-room mixed-size latency (p95≤200 ms/p99≤500 ms), worst-frame and 100-match soak tests; after cleanup/GC heap within 10% of warm baseline and room/socket/goroutine counts recover. |
| 6 — Gameplay retirement | Remove active association/specialty/dev-grant/backfill scheduler/wire/UI/string consumers in bounded module patches after replacement rejection tests; update every retained caller before removing its API. |
| 7a — Content retirement | Remove playable-image loaders/URLs/prefetch/catalog/workbench ingest routes and stale packaged secrets; prove unsupported input rejection and absence from route/artifact inventories. |
| 7b — Dependency retirement | Remove only packages/config/assets with no retained caller using official module tooling; rebuild server/tools/Android/Web and macOS iOS, retaining avatar WebP/CGO and account/UI utilities. |
| 8 — Storage retirement | Inventory archive/artifact/avatar consumers, then remove unused MinIO/CDN wiring, secrets and readiness checks; preserve authorized archives/applied SQL and verify text fresh-clone startup without cloud keys. |
| 9 — Test migration | Replace obsolete assertions with substantive mode/privacy/unsupported-input/restore coverage; present an exact file-removal list and release-note entry for required approval before deleting test files. Continue other retirement while that narrow decision is pending. |
| 10 — Final inventory | Compare source/call/dependency/route/config/packaged-artifact inventories with every temporary compatibility entry; close evidenced entries and record owner/reason/expiry for justified historical retention. |
| 11 — Gate | Independent review and full build/load/security/parity/restore verification precede staging. A future live rollback window needs measured operator evidence; no-deployment N/A does not certify one. |

**2026-09-19 implementation, 4a.** The offline physical cutover controller,
bounded JSON command and mandatory separate-cluster capture/restore rehearsal
are implemented; see the [C1–C7 plan and current evidence](../reports/2026-09-19-text-transition-resumption.md#phase-64a-continuation--physical-writer-fence-and-cross-cluster-proof).
They cover actual writer-role/session fencing, pending durable work, immutable
handoff, source/target parity and synthetic once-only SQL replay. Live external
process drain, provider callback holding/replay, selective invalidation and
human smoke remain open, so the broader parent checkbox remains unchecked.

**Risks.** Never use `compose down -v`, broad Redis flush or generic all-history
down as recovery. Avatar processing is an active image consumer. After new
financial writes, an old database snapshot cannot be routine rollback; preserve
one authoritative database or reconcile the exact durable delta before reopening.

#### Phase 6 continuation — selective legacy routing cleanup, 2026-09-19

**Goal.** Remove only inventoried obsolete Redis routing from an owned restored
fixture while retaining identity/security state exactly. **Non-goals.** No new
Redis-backed v2 queues, production cleanup, broad flush, or implied human smoke.
**Ownership.** The operations implementer owns unused legacy APIs in
`server/internal/store/redis.go`, focused tests, and selective restore-fixture
support in `infra/compose/` and `xops/test/`. Coordinate fixture migration-head
changes with the consent implementer; the coordinator owns docs/tracking.

| Child | Implementation and proof |
|---|---|
| R1 | Verify caller inventory, then retire unused legacy room-routing helpers with retained Redis readiness/error tests. Preserve the live Admin `AllowIntent` limiter and all test files; compiler verification found its caller missing from the graph result. |
| R2 | Add bounded explicit-inventory cleanup for owned disposable restored Redis only; verify target identity and exact stale key values before deletion, with unknown/current keys preserved and no unrestricted key pattern deletion. |
| R3 | Restore an actual captured fixture, remove only authorized stale routing, and prove all unrelated/security key types, values and absolute expiries unchanged; retry, cancellation, changed-key and error paths must not claim completion. |
| R4 | Independent review and cold verification, then final artifact/outage/source-retirement inventory and unified gate. Record remaining external callback/process and human/platform gates separately. |

**Risks.** A source snapshot's full parity proof precedes intentional selective
invalidation. Avoid races that delete a key whose ownership/value changed after
inventory; fixture ownership and stale-key compare/delete must remain atomic.

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

#### Phase 7 execution detail — 2026-09-12

**Goal/boundary.** Prepare and execute evidence collection for the implemented
product without fabricating people, elapsed cohorts or commercial results.
Engineering can build privacy-safe measurement, local prototype scripts,
analysis/reporting and release checks while recruitment/provider/device work is
pending. External outreach, spending, publication and deployment require the
actual named scope/target; an experiment budget in a document is not approval.

| Parent / child | Ordered delivery and proof |
|---|---|
| 1a — Instrumented prototype | Implement authorized zero-value prototype cohorts with pinned build/rules/pack and privacy-safe authoritative events; tests exclude them from live wallet/progression/leaderboard and public secret telemetry. |
| 1b — Human sessions | Prepare consented recruitment and facilitator sheets, then run five modes×both sizes across the three-theme/two-culture pilot. Record participant/table counts and exact observations; AI/bot runs are engineering evidence only. |
| 2 — Comprehension/balance | Produce reproducible per-cell reports for first-turn success, alternatives/laughter, first-seat/role outcomes, draws, completion and repeats; test denominators with synthetic data, then collect the specified human samples and uncertainty. |
| 3 — Farming | Implement deliberate abuse scripts and shared-cap probes, then compare actual credited/pre-cap rewards per human-minute with standardized role/cap mix and repeated-pairing evidence; investigate >1.25× rate before enabling rewards. |
| 4 — Queue liquidity | Record exact FIFO join/start/leave reasons and p50/p95 waits; run two staffed windows with ≥100 joins per exposed cell and apply the business plan's 90% start-within-timeout/≤10% abandonment assumptions. |
| 5 — First-run/accessibility | Run new-player onboarding/default/change/rematch sessions on real target accessibility/low-end devices; retain failures and versioned fixes without silently changing a selected tuple. |
| 6a — Cohort computation | Implement authoritative activation, second-match, D1/D7 and invitation definitions with UTC boundaries, exclusions and deletion-safe pseudonymous analysis; synthetic boundary tests prove formulas only. |
| 6b — Elapsed evidence | Observe two weekly ≥100-human activation cohorts and the defined return windows; report numerator/denominator/group correlation and missing data, never infer D7 before day seven. |
| 7 — Economics | Replace illustrative values with dated host/provider quotes, actual net settlements and measured authoring/moderation/support hours; validate units/formulas/sensitivity, then obtain owner cash/reserve/burn/authority records. No unapproved spend. |
| 8 — Decisions | Record named content/locale/safety/operations owners, launch cells, staffed capacity, age/content/market review, real legal/support URLs, billing/consent approval and any paid legacy remedy. Adopted game/economy defaults need no redundant approval. |
| 9 — Shipping assets | After final UI/engine/netcode gates, capture actual enabled journeys and ≤45 s clip, localized rules/screenshots/disclosures; verify real build identities, privacy cuts, captions/diacritics, dimensions/duration/size and store requirements. |
| 10 — Operator drill | Prepare a target-specific host/backup/incident/takedown/support packet and rehearse on disposable artifacts first; actual public-host migration requires selected host/operator/access and explicit routing authorization. |
| 11 — Exposure | Apply an audited per-cell enable/hold matrix only after technical/content/business approval; observe the agreed real window, stop admission on integrity/value/privacy failures and halt acquisition on failed liquidity/economics. |
| 12 — Gate | Review signed evidence for every launched cell; unavailable intended cells retain their missing proof and next action. Tests of reports/config never stand in for human, platform, commercial or production observations. |

**Specific external inputs.** Real consenting 4/6-player groups and locale
editors; declared pilot languages/cohort dates; Android/PWA low-end devices and
macOS/Xcode/iOS access; OAuth/billing/ad/moderation test accounts and provider
results; owner-controlled legal/support URLs and intended-market review;
named operator/host with launch and spending limits; elapsed playtest/retention/
rollout observations. Ask only for the next concrete missing input when its
dependent work is ready; keep unrelated implementation progressing. No existing
deployment or paid-customer inventory is asserted beyond the owner's dated N/A.

#### Phase 7 continuation — economics arithmetic, 2026-09-19

**Goal.** Make Business Plan monthly, acquisition, observed-cohort and runway
formulas reproducible, with explicit units and missing denominators. This is an
offline arithmetic tool; it cannot certify quotes, human measurements, funding
or a launch decision. **Files/ownership.** Coordinator: `tools/economics_report.py`,
`xops/test/test_economics_report.py`, tool documentation and evidence report.

| Child | Implementation and proof |
|---|---|
| E1 | Strict bounded JSON with decimal-string accounting units, one currency, formula version and explicit observation horizon. Reject unknown fields, nonfinite/negative inputs, impossible shares and invalid counts. |
| E2 | Compute subscription/bulk/ad receipts, player-match runtime and itemized labor/provider/hosting costs, acquisition contribution, CAC, observed cohort LTV and runway. Return unavailable ratios for missing/zero denominators; distinguish cash burn from operating contribution. |
| E3 | Reproduce all three Business Plan examples exactly, then test player-match units, zero activity/ads/burn, missing cash/reserve, loss-making cohorts, horizon preservation, reorder determinism and strict CLI/private output behavior. Independent review and full Python suite are required; commercial parent remains open. |

**Risk.** Formula validation does not validate business assumptions. Inputs and
reports remain explicitly unverified arithmetic; yearly receipts must already
be recognized on a monthly basis. Never infer revenue from virtual currency or
infer cash burn from profit.

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


#### Finalization boundary — 2026-09-19

At the owner's request, finalize and stage the current verified implementation
for a human push. No D3–D6 completion is claimed: there is no migration35 or36,
no public deletion route, no survivor-value transformation and no deployed
suppression/processor service. D2 supplies only closed same-account proof and
status adapters. The source-reader deletion check is implemented; atomic authored
release withdrawal and shared-byte erasure remain pending. Reward B1–B3 is closed
verification only; payouts, consent/provider/client joins and the interrupted
bonus decision remain pending. Human, device, provider, cohort and live cutover
evidence stays unchecked. This handoff does not mark all roadmap phases complete.
