# Text transition resumption — 2026-09-19

## Recovery

The owner requested continuation after credits interrupted development. Work
resumes from clean commit `ec8b7ab4faed8c837570eb6f5af9f26786e80bfb` on `main`.
Session bootstrap found no checkpoint and no unresolved failure. The
[previous resumption report](2026-09-12-text-transition-resumption.md) preserves
the interrupted scope, but its historical `/tmp/agent-runs` logs and frozen
manifests are no longer available. Historical passes below remain attributed
to that report until fresh verification is recorded.

The last recorded 100-room measurement failed with `top_that/4
request.unavailable`; an earlier measurement failed latency and retained-heap
budgets. Neither run establishes load acceptance. Admin room operations,
runtime shutdown integration and small exhaustive action tests were saved with
final independent checks still unfinished. The runtime cutover startup guard
and synthetic cohort calculator were planned but absent.

## Resumed engineering scope

**Goal.** Complete the interrupted implementation and verification slices while
keeping release claims tied to actual evidence.

**Non-goals.** This pass does not supply human playtests, physical-device or
provider sandbox results, legal/retention decisions, production enablement,
external outreach, spending or deployment.

**Files.** Existing gamebot/load and implicated runtime code; store cutover
startup guard and executable integration; offline cohort tool and Python tests;
roadmap, current context, discoverability documents and changelog. Each delegated
implementation has separate file ownership. Existing migrations remain frozen.

**Test plan.** Run the unified repository gate through safe-run; preserve every
failure and repair its cause. Use test-first targeted proofs for new behavior,
independent source review then cold verification, and frozen uncontended load
measurement with the existing 100-room, latency, heap and cleanup budgets.
Synthetic cohort tests prove calculations only.

**Checklist.**

- [x] Recover saved scope and distinguish completed code from unfinished proof.
- [x] Investigate the recorded refusal and add diagnostics; fresh load checks pass. Historical root cause remains unconfirmed, so no speculative runtime repair is applied.
- [x] Complete the saved runtime cutover startup guard plan and its proofs.
- [x] Complete the saved synthetic-only offline cohort calculation plan.
- [x] Review and verify saved admin/lifecycle/exhaustive-action implementations.
- [x] Finish unified validation, reconcile documentation and stage the reviewed changes.

**Risks.** Concurrent tests invalidate performance comparisons; isolate final
measurement. Cutover identity reads must fail closed without expanding runtime
privileges. Synthetic data cannot establish content quality, user retention or
release readiness. No historical evidence or failed check is silently removed.

## Current evidence

### Notice fixture isolation

The initial unified run failed `TestLocalizedFallback` and
`TestNoticeAuditFailureRollsBackMutation`: notice rows from earlier tests
survived because setup ignored a failed `TRUNCATE`. Migration 27 deliberately
protects retained admin audit history. Production immutability is unchanged.

New `TestNoticeFixturesResetRetainedAudit` reproduced one retained notice and
audit row in `notices-isolation-red--20260919T085027Z-121248.log`. The setup now
uses the existing admin suite's disposable-database identity checks and bounded
schema rebuild, rejects unavailable/unverified databases instead of skipping,
and checks cleanup errors. Full notice race tests passed in 4.600 s:
`notices-isolation-green--20260919T085116Z-129683.log`. Independent source review
approved the complete test diff without findings.

### Saved-work source review

Independent review found no actionable issues in the saved runtime lifecycle,
request/worker gates, executable assembly, exact-room admin operation decisions
and delivery/recovery, small finite ownership/depletion tests, or new gamebot
failure diagnostics. The diagnostics preserve error causes and add operation,
request and load-step context without printing private state or changing budgets.
Their test-first proof is RED `load-control-diagnostics-red--20260919T084720Z-104622.log`
and GREEN `load-control-diagnostics-green--20260919T084743Z-105855.log`
(six targeted race tests).

Cold verification of the assigned server scope subsequently passed 948
tests/subtests across eight packages under `-race -count=1 -p=1 -timeout=180s`,
with zero failures/skips, plus format, vet, build and diff hygiene:
`text-cold-verifier--20260919T085517Z-150740.log`. This covers executable,
store, admin, lobby, handler, game, notices and transport. Server source hashes
remained unchanged during the independent run.

### Runtime cutover guard

The existing provisioned runtime role read the actual PostgreSQL system/database
identity without any privilege change (`cutover-runtime-prerequisite--20260919T084628Z-98089.log`).
The new startup check uses a bounded read-only transaction, refuses hidden or
unreadable registry data and mismatched/non-ready databases, and runs before
owner acquisition or recovery writes. An empty registry still permits ordinary
initial startup without creating cutover authority. External writer fencing and
capture/handoff remain separate work.

Missing API and unsafe-startup regressions failed in runs `113557` and `117482`;
targeted existing/new cutover checks passed in
`cutover-runtime-green--20260919T084947Z-118927.log` (store 17.424 s, executable
integration 1.646 s). An independent reviewer approved all four changed files
with no findings. The independent 948-test cold server gate includes this guard;
migrations and role checks are unchanged.

### Synthetic cohort calculator

The saved offline plan is implemented in `tools/cohort_report.py`, with a strict
synthetic fixture schema documented in `tools/README.md`. It reports per-cell
activation, completion, voluntary second match, UTC D1/D7, weekly participation
and queue denominators. Missing facts, immature windows and uncertain activation
times remain explicit; known successes survive unrelated unknown outcome/intent facts.
Deleted fixture subjects cannot reappear through stale event replay. Private,
exclusive aggregate output contains no individual IDs; the tool has no live
collection or launch-approval path.

Independent review identified missing-fact classifications, cross-cell sample
counts and activation-time ambiguity; each was repaired with a regression.
Final targeted evidence is 25 passing tests in
`cohort-activation-green--20260919T090212Z-179609.log`, followed by source
reapproval without findings. The final unified run also passes these tests.

### Initial combined run and environment repair

`text-resume-20260919-baseline--20260919T084351Z-34480.log` records 1,857
passing test events, two notice failures and zero skips. Other failed stages
were a two-line Go indentation issue and the host Flutter SDK: Dart 3.10.4
cannot satisfy the client's minimum 3.12.0. No client source was reformatted
with that incompatible formatter. The migration test indentation is corrected.

The already-built `knowoff-client-web:latest` image provides Flutter 3.47.1 /
Dart 3.13.1. Its SDK was copied into
`/tmp/agent-runs/flutter-sdk-20260919/flutter` without changing the host SDK.
The first compatible client run exposed stale ignored localization output;
the existing `flutter gen-l10n` preparation step regenerated it. Then
`text-resume-client-generated--20260919T085729Z-172265.log` passed all 504
Flutter tests, two web-cache tests, analysis and formatting, with zero skips.
The release web build also passed in
`text-resume-web-build--20260919T090057Z-178201.log`. Its CupertinoIcons font
warning is recorded as a visual-release follow-up; native/device evidence is
not implied by this web build.

The initial 100-room test passed in 195.21 s with both warm and measured
100-match workloads. On Intel Core Ultra 9 285H, 16 CPUs, Go 1.27.1,
PostgreSQL 16.13 and loopback networking: action RTT p95 125.619 ms /
p99 142.661 ms; accepted server action p95 125.839 ms / p99 150.113 ms
(5,923 samples); worst frame 8,185 bytes against 8,192. Retained heap was
2,473,256 → 2,541,920 bytes (+2.78%); goroutines returned 8 → 8.
No earlier request refusal recurred. This is a fresh passing observation,
not a proved diagnosis or code fix for the lost historical failure. Final
frozen combined verification is recorded below.

### Restore fixture startup race

A later Python slice failed the real legacy-restore rehearsal: the first
target catalog query exited 2 after readiness had succeeded. The fixture used
Unix-socket queries, and the inspected PostgreSQL image entrypoint starts a
temporary socket-only initialization server before restarting the final server.
The deterministic new test reproduces acceptance of that temporary server
(`snapshot-bootstrap-red--20260919T085917Z-175549.log`).

Fixture queries now use authenticated loopback TCP, which the initialization
server does not offer. The regression passes
(`snapshot-bootstrap-green--20260919T085936Z-176019.log`). Independent review
approved the change, including sealed/network-detached fixture behavior; the
following unified attempt passed real old/current restore parity. No new wait, retry budget or production
credential is introduced.

### Lifecycle test connection cleanup

The first frozen unified attempt (`text-resume-final--20260919T090342Z-181702.log`)
passed all 91 Python tests, including real old/current restore parity, then
failed the one-second lifecycle shutdown test. The exact runner was stopped
before load with SIGINT; its subprocesses and disposable services were removed.
All 625 frozen inputs remained unchanged. The failed run remains recorded.

Temporary connection-state instrumentation reproduced seven failures in 25
runs: each failure retained never-used `StateNew` client connections. The
installed Go HTTP server waits five seconds before treating these as idle.
The fixture closed unread response bodies, allowing transport dial races.
It now consumes responses and closes client idle connections before releasing
the grace barrier. Diagnostic instrumentation was removed; the production
lifecycle, one-second deadline and all shutdown assertions remain unchanged.

Evidence: `lifecycle-shutdown-diagnostic--20260919T090751Z-242967.log` RED;
all four lifecycle tests repeated 25 times under the race detector PASS in
2.186 s (`lifecycle-shutdown-green--20260919T090847Z-244892.log`). Independent
source review approved the repair. The new frozen unified run passes below.

### Final frozen unified gate

`text-resume-final-fixed--20260919T090946Z-246082.log` and machine-readable
`/tmp/agent-runs/text-resume-final-fixed.json` record exit **0**, **2,404 passed**,
**0 failed**, **0 skipped**, with all 19 stages passing. Command:

```bash
env PATH=/tmp/agent-runs/flutter-sdk-20260919/flutter/bin:$PATH \
  python3 xops/test/tests-lints.py \
  --report-json /tmp/agent-runs/text-resume-final-fixed.json
```

| Scope | Passing tests/subtests | Other passing checks |
|---|---:|---|
| Python tools, make helpers and ops | 91 | Real legacy/current restore parity |
| Server | 1,703 | Format, vet, build |
| Gamebot | 81 | Format, vet, build; authenticated five-mode/4/6 matrix, replay and load |
| Mediapack Go module | 23 | Format, vet, build |
| Flutter and web cache | 504 + 2 | Analyze, format |

The independently reviewed cold 948-test race run and the repaired lifecycle
25-repeat race run complement this full gate. Test-event counts include named
subtests and are not a count of independent scenarios.

All **625 source/test/config inputs remained unchanged** during the run;
before/after manifest SHA-256:
`cb4e2eda0a3ec7e2c4c66158d0a853538ab9a55b4caf0958b12e08b3fdbc6450`.
Markdown and tracking updates are outside this source freeze. The verifier
retained `/tmp/agent-runs/text-resume-final-fixed-evidence.json` and confirmed
removal of the disposable PostgreSQL, Redis and test network. Applied migrations
are unchanged. These durable summaries remain useful if temporary logs expire.

The final uncontended `TestTextNetworkHundredRoomsAndMatchSoak` passed in
186.78 s, with 100 warm matches plus 100 measured matches and 500 simultaneous
sockets on the host described above. Action RTT p95 **125.372 ms** / p99
**147.729 ms** meet the 200/500 ms limits. Server accepted-action diagnostics
were p95 125.647 ms / p99 152.679 ms (5,938 samples). Worst frame **8,185 bytes**
meets 8,192; retained heap **2,461,416 → 2,554,120 bytes (+3.77%)** meets the
10% limit; goroutines return **8 → 8**. Room/socket cleanup checks pass.

Independent source review plus these engine/property/race/privacy/network
proofs close only Phase 3's remaining technical gate. Roadmap completion is
**53/91** items: Phase 1 12/12, Phase 2 6/10, Phase 3 14/14, Phase 4 10/12,
Phase 5 11/20, Phase 6 0/11 and Phase 7 0/12. The earlier summary's Phase 2/5
counts were stale; correcting them does not invent new completed deliverables.

## Remaining roadmap work at the recovery handoff

This handoff completes the interrupted startup-guard, synthetic-calculation and
verification slices. It does not finish the seven-phase roadmap. Remaining
engineering includes production-candidate action/depletion certification,
complete moderation/admin and private block/report journeys, deletion/retention
integration, provider receipt/consent flows, and retirement/cutover operations.
Follow the ordered unchecked items in [ROADMAP.md](../planning/ROADMAP.md).

In particular, the runtime startup guard is only one cutover prerequisite.
Every external writer still needs physical role/connection fencing before a
watermark, with executable capture/handoff and cross-cluster parity evidence.
The next bounded engineering slice is Phase 6.4a: controller capture/fencing,
watermark and restore bootstrap on a different PostgreSQL cluster system ID.
Roles are cluster-wide, so a sibling database cannot justify reopening source
writer logins. Schema 26 closing is terminal; cancel before closing, and never
rewrite authority rows or reopen the same source without an additive protocol.
The synthetic calculator likewise needs a separately reviewed minimum-event,
access, deletion and complete-watermark contract before any real collection.

Human editorial pilots, real-player cohorts, native/PWA device performance,
platform sandbox observations, entitlement remedies and owner launch decisions
remain unexecuted where the roadmap says so. Public mode availability remains
closed. A passing repository test gate supplies engineering evidence only.

## Recovery commit handoff

Run ID: `text-resume-20260919`. The reviewed 23-file change set is staged with
the candidate summary `feat(transition): complete interrupted runtime guard and cohort tooling`.
The human runs `make git` after the `make git.dry` preview; agents do not commit
or push. Resume further engineering with the Phase 6.4a boundary above.

## Phase 6.4a continuation — physical writer fence and cross-cluster proof

### Goal

Implement the offline controller's physical single-writer boundary and prove a
complete synthetic capture/restore/handoff between independently initialized
PostgreSQL 16 clusters, preserving durable value and all existing authority.

### Non-goals

- No production operation, ordinary Compose snapshot access, deployment,
  routing change, role provisioning outside guarded disposable fixtures, or
  public/Admin controller endpoint. Controller credentials remain offline.
- No edits to applied migrations 26/27, same-source reopening after closing,
  same-cluster activation, physical-clone activation, automatic recovery to
  LOGIN, or relaxed immutable-table/trigger/role contracts.
- No claim that synthetic held-event replay implements provider callbacks,
  external durable queues, human smoke, selective Redis invalidation or the
  complete Phase 6 parent gate. Existing unfinished product work stays open.

### Touched files

The controller implementer owns new `server/internal/store/cutover_controller.go`
and `cutover_controller_test.go`, with narrow changes to `cutover_roles.go` only
if needed to reuse its exact capability checks. The pre-edit inventory confirmed these new
controller files did not exist. The coordinator owns the bounded offline command and documentation; the
rehearsal worker owns the distinct-cluster fixture and its tests, plus
`infra/compose/snapshot.py` only if a reusable fixture helper is necessary;
its current nonce/resource guards remain intact. Existing
`cutover_runtime.go`, `cutover_migration_test.go`, `cutover_control_roles_test.go`
and `text_drain.go` supply compatibility and regression coverage. Documentation
updates belong in this report, `VPS_MIGRATION_RUNBOOK.md`, the transition design,
Roadmap evidence, changelog and parent-owned tracking. Preserve the already
staged 23-file recovery handoff and its evidence above.

The implementation boundary is a typed `CutoverController` with explicit
`Begin`, `Seal`, `Handoff` and target `Bootstrap` operations, typed physical
identity/artifact pins/request/receipt values, and bounded context-aware close.
Exact signatures are frozen with the implementer before command integration.
Arguments never accept a caller's `writers_quiescent` or `parity_verified`
boolean. Receipts expose IDs, generations, bounded aggregate counts and hashes;
raw DSNs, lease secrets, SQL diagnostics and retained account/content rows do
not enter command output. Controller operations use the authenticated control
connection; artifact capture uses a separately authenticated capture connection,
never runtime or privileged fixture-owner connections.

### Test plan and acceptance gates

| Checklist proof | Required observable evidence |
|---|---|
| C1 — Request authority | Real controller-role tests reject wrong database/role/pins, malformed or conflicting requests, expired leases, lock cancellation and stale generation; identical authorized replay retains original IDs. A competing controller cannot interleave effects. |
| C2 — Physical fence | Start real runtime and migrator connections, including a pooled idle session, an open transaction and the migrator after `SET ROLE` owner. Closing atomically records its request and disables both LOGIN roles; old connections cannot commit and newly authenticated connections refuse. Unknown sessions or prepared transactions refuse success. No failure path reenables source writers. |
| C3 — Watermark | Pending prepared/started matches, reserved admissions, pending/missing settlements and undelivered operator decisions prevent a watermark. Already committed unacknowledged outbox value is counted and retained, not misclassified as unpaid. Evidence derives from current fenced DB reads, exact role names/OIDs, artifact pins, catalog inventory, table fingerprints and sequence states; a failed read or altered role/identity cannot certify empty work. |
| C4 — Source handoff | The exact source request/watermark selects one target with a different actual system identifier. Wrong token/target, sibling database, second target and expired pre-handoff request fail. Retried matching handoff preserves one immutable receipt and the source stays sealed. |
| C5 — Restore/bootstrap | Two uniquely owned containers have unequal observed system IDs. Capture through the capture role after handoff; restore only into the selected empty disposable target with writer logins disabled. Compare all retained domain tables, sequences, constraints/ACL shape and immutable cutover ancestry. Wrong artifact, target identity, source binding, target value or source LOGIN state refuses activation with target still closed. |
| C6 — Single writer and replay | With source still NOLOGIN and old connections unusable, target bootstrap creates its own ready child and enables only target writers. A held synthetic event with a stable existing value idempotency key applies once after activation and remains once after retry; account/ledger/entitlement checks include its exact delta. Restored source history is untouched. |
| C7 — Operational bounds | Tests assert command failure/cancellation is finite, no secret/raw-row echo, no shell interpolation of inputs, no valid completion artifact after failure, and exact resource cleanup. Source fence survives controller loss before/after seal and after handoff; recovery only advances a closed generation where schema 26 permits it. |

Run each new behavior test RED before implementation and retain failure evidence.
At each implementation boundary run independent source review, then cold
PostgreSQL race verification of the entire affected cutover family and runtime
startup integration; keep existing migration and ACL tests intact. The final
acceptance is the official `python3 xops/test/tests-lints.py` with every module,
zero silent skips, an uncontended load stage and frozen source/config inputs.
The new executable rehearsal must run in that official gate, not merely as an
optional manually reported script. Parent reviews the combined diff, appends
tracking and verifies staging with `make git.dry`; nobody commits or pushes.

### Checklist

- [x] C1: Add the typed offline controller/session authority and request tests;
  use the existing schema 26 advisory serialization key on a dedicated physical
  connection with bounded acquisition, server-clock expiry and secret-digest
  comparison. Request insertion and closing-generation update commit together.
- [x] C2: Commit physical NOLOGIN for the complete declared runtime/migrator
  writer set and terminate only matching source database/role/backend identities.
  Inspect session-user role OIDs, not an application-supplied process list or
  current SET role. Recheck fresh activity and prepared transactions before
  success; a canceled or uncertain close remains unavailable for capture.
- [x] C3: Compute a bounded watermark only after physical fencing and durable
  pending-work checks. Keep domain fingerprints separate from append-only
  cutover metadata so adding watermark/seal/handoff does not manufacture a
  financial parity mismatch. Persist PostgreSQL `jsonb::text` UTF-8 evidence
  digest, WAL position and exact current generation; seal only that watermark.
- [x] C4: Observe a preallocated empty target's identity through its authenticated
  connection, require a different system identifier and bind it once on the
  sealed source. Capture must include this committed handoff; a dump taken
  before it cannot bootstrap the target.
- [x] C5: Execute capture/restore in guarded fixtures, compare the target's
  actual restored data with the retained source evidence, and authenticate a
  fresh live read of the source binding/fence. Insert only the bound target
  ready child, then enable target writers. Never rewrite restored source rows.
- [x] C6: Add dual-writer denial, actual durable replay/parity and interrupted
  controller/restore/retry proofs on independent clusters.
- [x] C7: Complete bounded command, full runner integration, independent review
  and cold verification; append measured evidence and reconcile the runbook's
  pre-write rollback sentence with schema 26's terminal closing transition.

### Risks and schema feasibility

Schema 26 supports this first distinct-cluster protocol without an additive
migration: root bootstrap requires empty authority history; closing/request
commit atomically; seal binds a watermark; handoff names one target; the restored
target inserts a new ready child referencing its sealed source and watermark.
Capture ordering is essential: handoff must be included in the restored bytes.
Target roles are provisioned on their own cluster and remain NOLOGIN until
bootstrap; their new PostgreSQL OIDs are not substituted into historical source
writer inventories. Compare role privileges by the approved contract, while
preserving historical OIDs as source evidence.

A committed handoff cannot be retargeted, and source authority never returns to
ready. Failed target restore is retried only against the same chosen identity;
another target or same-source reopening needs a separately reviewed additive
protocol. Closing can advance a recovery generation only before handoff and
without reopening writer logins. Raw SQL controller privileges remain a trusted
operator boundary rather than protection against a malicious controller or
superuser; no guarantee is made while another privileged operator alters the
cluster. Unknown observable sessions, role drift and prepared transactions are
failures, not reasons to broaden termination or permission scope.

The existing snapshot fixture's read-only default and network detach are not a
physical writer fence. Reuse its disposable resource ownership and restore
checks without presenting them as ordinary database backup authority. A real
deployment additionally needs verified stopped/joined external processes and a
durable callback holding/replay contract; synthetic rehearsal proves the bounded
controller and restore behavior only. Keep Phase 6 parent checkboxes open until
their complete ordered proofs, including human and operational evidence, exist.

### Implementation and review evidence

The controller uses its dedicated authenticated PostgreSQL control session for
request/closing, physical role fencing, sealing, immutable target binding and
target bootstrap. Applied migrations 26/27 are unchanged. Source and target
hold independent controller lanes during activation. The command accepts a
bounded typed JSON request on stdin and reports redacted receipts/errors;
its timeout includes incomplete input, not only database work.

Initial CLI refusal tests failed before the command existed
(`cutover-cli-red--20260919T092942Z-353833.log`). A stalled-pipe regression then
failed at its one-second outer deadline
(`cutover-cli-input-red--20260919T093603Z-365710.log`); the input read now obeys
the operation context and the complete CLI race suite passed
(`cutover-cli-green--20260919T093742Z-369918.log`).

The first real separate-cluster proof passed four tests in 9.857 seconds
(`cutover-rehearsal-binary--20260919T094038Z-380686.log`). It captured through
the capture role, restored both pre-handoff and authorized archives, rejected
missing ancestry, changed target value and source LOGIN drift, and activated
only the selected target. The synthetic SQL replay preserved source wallet 456
and applied exactly one target delta of 17, with matching ledger, purchase and
entitlement effects. This is a proof of SQL transaction-key idempotency, not an
implemented external provider holding queue. A subsequent five-test proof
observed actual server-clock lease expiry after committed handoff and still
restored/bootstrapped the exact target
(`cutover-rehearsal-expiry--20260919T094149Z-388471.log`).

Independent review reproduced a missing `pg_rewrite` definition in schema
evidence: an `ON INSERT ... DO INSTEAD NOTHING` rule changed write behavior
while sealed row hashes remained unchanged
(`cutover-review-rules--20260919T094205Z-395050.log`). The controller now
rejects unsupported rewrite rules, inheritance and row policies, and schema
evidence also binds sequence ownership. Targeted regressions pass; independent
review and the full gate remain required below.

Review also reproduced a physical fence bypass with an ordinary runtime
connection using PostgreSQL's `post_auth_delay`: it had already passed the LOGIN
check but was not yet published in `pg_stat_activity`. Begin and Seal returned
success, then the delayed connection committed one account after sealing
(`cutover-review-startup-option--20260919T094500Z-414990.log`). Its database
startup lock was observable in `pg_locks`. A safe fence must refuse unclassified
startup lock holders, refresh statistics before signaling and recheck after
termination; changing LOGIN alone is insufficient. The fixed implementation
and independent regression results are recorded after the final checks below.

Review found one CLI parser ambiguity as well: Go's JSON decoder treats the
Unicode Kelvin sign as a case-folded `K`, while the duplicate detector used
uppercase conversion. The new regression reached credential lookup before the
fix (`cutover-cli-unicode-red--20260919T094714Z-432080.log`). Object names now
require ASCII, matching every typed field; string values remain Unicode.
Invalid-input tests require the exact refusal code and prove that credential
lookup never occurs. The complete command race suite passes after this fix
(`cutover-cli-reviewed--20260919T094732Z-433201.log`).

The complete targeted controller race run passed **21 tests/subtests** in
20.256 seconds, including idle/active/SET ROLE writer termination, delayed
authentication, request/lease/identity/pin rejection, stale-generation refusal,
all requested durable-pending categories, schema/rule/sequence drift and
committed-handoff expiry recovery. The existing cutover family also passed
82 test/subtest events before the final added cases. Writer name/OID evidence
is retained in the immutable request, reauthenticated on the source, and
preserved through full authority-table parity; target cluster-local role OIDs
are deliberately not substituted into source history.

The final targeted separate-cluster run passed **7 tests** in 19.331 seconds
(`cutover-rehearsal-prepared-fixed--20260919T095315Z-449583.log`), including
one retained unacknowledged outbox record and prepared-transaction refusal.
The initial prepared test expected closing before refusal and failed
(`cutover-rehearsal-prepared--20260919T095051Z-439534.log`). Inspection showed
that the existing capability verifier already rejects prepared transactions
before creating authority. The corrected proof asserts zero authority and
watermarks, unchanged initial writer state, then successful Begin only after
the fixture rolls back its own abandoned transaction. No production guard
was loosened.

The fixture compiler's Docker fallback was separately exercised through the
existing server development Dockerfile
(`cutover-builder-fallback--20260919T094411Z-412868.log`). Its temporary builder
has no network, read-only source and exact ownership checks. The database
containers have independent system IDs, tmpfs data, an owned internal network
and no published ports. Completion evidence is written only after successful
cleanup; fixture-boundary/failure tests cover refusal and missing completion.

A fourth review proof exposed snapshot ordering: Seal's repeatable-read
snapshot could predate a writer that committed and disconnected before the
census. The deterministic locked-table probe observed one committed account
but zero in the sealed fingerprint
(`cutover-review-snapshot--20260919T095837Z-468989.log`). A later Handoff
correctly rejected the mismatch, so this demonstrated stale certification and
an unavailable generation, not source data loss through activation. The fix
must establish a verified physical fence before opening the retained-data
snapshot, including the corresponding initial target bootstrap boundary.

The minimal fix now commits a verified physical preflight before each source
retained-data transaction and before initial target parity. Exact already-ready
target replay still permits legitimate target writes and rechecks binding and
ancestry. The normal repository regression failed before the fix
(`cutover-controller-snapshot-red--20260919T100030Z-472725.log`) and passed after
it (`cutover-controller-snapshot-green--20260919T100141Z-475519.log`), with
actual and captured account counts both one and a successful fresh Handoff.
The final controller race suite passed **23 tests/subtests** in **22.873 s**
(`cutover-controller-preflight-green--20260919T100218Z-476989.log`); store vet
also passed (`cutover-controller-preflight-vet--20260919T100300Z-478981.log`).

Independent review approved all four corrections and the final source/target
transaction ordering. The first three corrections had independent runtime
rechecks (`cutover-review-regressions-green--20260919T095415Z-458735.log`);
the final ordering correction received read-only review plus normal repository
regressions. An additional independent target timing probe was stopped by an
automated safety check before execution and was not retried; no result is
claimed for that probe. The ordinary two-cluster and full repository checks
below supply the final execution evidence.

Before the ordering correction, a broader independent cold race run passed
**361 tests/subtests**, zero failures/skips, covering the entire store,
runtime command and controller command packages. All 631 inputs were unchanged
(`cutover-control-cold--20260919T095654Z-464696.log`). That diagnostic pass is
retained separately; the corrected bytes receive the final checks below.

### Final independent verification for Phase 6.4a

After review approval, the corrected controller and command passed **43 cold
race tests/subtests** (23 controller, 20 command), zero failures/skips
(`cutover-control-cold-fixed--20260919T100335Z-479965.log`). All **631 inputs**
remained unchanged; manifest SHA-256:
`37d51b84dec10a845215308a6ebd18b52ecbb1e6c1694061600f4c208c5a9b7d`.
The owned PostgreSQL container and network were removed before the full gate.

The subsequent official gate includes the seven-test rehearsal automatically
through Python discovery. Its real capture/restore portion passed on the
corrected controller, including prepared refusal, 73 retained domain tables,
two restores, one preserved unacknowledged outbox record and exactly one
synthetic replay. No reviewer probes or other tests ran alongside this full
gate. All 631 source/test/config inputs remained unchanged; Markdown/tracking
updates are outside that source freeze.

The full gate's uncontended `TestTextNetworkHundredRoomsAndMatchSoak` passed in
**190.75 s**, with 100 warm matches and 100 measured matches, each phase at
500 simultaneous sockets. Action RTT p95 **126.045 ms** and p99 **146.737 ms**
meet the 200/500 ms limits. Server accepted-action diagnostics were p95
126.228 ms / p99 153.138 ms across 6,082 samples. Worst frame **8,185 bytes**
meets 8,192; retained heap **2,470,296 → 2,533,176 bytes (+2.55%)** meets 10%;
goroutines return **8 → 8**. Room/socket cleanup checks pass. This uses the
same recorded host as the recovery run above and is not physical-device evidence.

The final official gate exited **0** with **2,454 passed, 0 failed, 0 skipped**
and **all 19 stages green**. Log:
`/tmp/agent-runs/cutover-control-final--20260919T100408Z-482075.log`;
machine reports: `/tmp/agent-runs/cutover-control-final.json` and
`/tmp/agent-runs/cutover-control-final-evidence.json`. Before/after input
manifests have the identical SHA-256 recorded above. Command:

```bash
env PATH=/tmp/agent-runs/flutter-sdk-20260919/flutter/bin:$PATH \
  python3 xops/test/tests-lints.py \
  --report-json /tmp/agent-runs/cutover-control-final.json
```

| Scope | Passing tests/subtests | Other passing checks |
|---|---:|---|
| Python tools, make helpers and ops | 98 | Two-cluster cutover and historical restore proofs |
| Server | 1,746 | Format, vet, build |
| Gamebot | 81 | Format, vet, build; authenticated matrix, load and soak |
| Mediapack Go module | 23 | Format, vet, build |
| Flutter and web cache | 504 + 2 | Analyze, format |

### Phase 6.4a handoff and remaining work

Run ID: `cutover-control-20260919`. The completed C1–C7 controller slice joins
the earlier recovery staging window; the combined handoff contains **33 files**,
including the prior 23. Candidate summary:
`feat(cutover): add physical writer fence and verified target bootstrap`.
The agent stages and checks `make git.dry`; the human runs `make git`.

The broader roadmap remains **53/91**, with Phase 6 parent gates open. Next
engineering is live external-writer drain and durable provider callback holding/
replay, selective stale-generation invalidation, and the remaining retirement
inventory. Human enabled-mode smoke, platform/provider observations and release
decisions still require their own evidence. This slice supplies an offline
controller and guarded synthetic rehearsal, not a production cutover or release.

## All-phase continuation — 2026-09-19, from e840852

The owner requested all remaining phases. Bootstrap confirmed clean `main` at
`e840852`; the older staged checkpoint was already committed. The current
roadmap continuation tables record implementation ownership and proof before
edits. Run ID: `roadmap-completion-20260919`. No deployment, outreach, spending,
human study or provider transaction is inferred from this request.

### Action certification

The new public `server/pkg/textcert` boundary replays deterministic full-schedule
action/timeout witnesses through the authoritative engine. It binds exact
content, full tuning, rules, algorithm, samples, ordered requests/clocks and
history; `action-replay.json` is privileged technical evidence. Human
`actions.json`, screening, editorial and release evidence remain separate.
Store publication/activation/restored loading and mediapack use the same gate.
Request context cancels new replay work per witness/step; the pre-existing
retained-card gate remains synchronous. ADR-013 records the import boundary.

Independent source review passed after direct Turkish/Arabic coverage and
cancellation proofs. Cold final certifier evidence: **21 tests/subtests**,
9.802 s, zero failures/skips, `action-review-final--20260919T104309Z-760480.log`.
Independent media/game suites passed in 2.458/6.308 s, including existing small
exhaustive/depletion fixtures (`action-review-core--20260919T104037Z-753167.log`);
the entire mediapack CLI passed in 6.329 s
(`action-review-cli--20260919T104038Z-753373.log`). Two real PostgreSQL durable
release integration tests passed in 13.440 s
(`action-review-store--20260919T104119Z-755665.log`). Final combined verification
is still required; samples never claim exhaustive production-state or human proof.

### Immutable consent

Migration 28 protects contribution terms against UPDATE/DELETE and all three
retained contribution/user-consent tables against TRUNCATE, including cascading
and RLS-hidden attempts. Runtime privilege inventory now permits only INSERT
for these tables. Existing migrations remain unchanged; controlled down refuses
retained consent. Portal fixtures rebuild only their independently verified
disposable database instead of weakening production guards.

Independent source review and **21 cold tests/subtests** passed in 3.109 s:
`consent-independent--20260919T103836Z-747579.log`. Proof includes original
full-table parity, 27→28/repeated-up/empty-down/retained-down refusal, exact
wording/timestamps, nonowner/RLS/CASCADE and ordinary acceptance replay.
An earlier parallel portal compile encountered the new package mid-edit;
stable full portal rerun passed in 28.384 s
(`action-portal-full--20260919T103900Z-749072.log`). That transient failure is
retained, not counted as passing evidence.

### Selective routing retirement

The restored-fixture operation accepts at most 100 exact legacy room key/node/
absolute-expiry entries. It verifies restore pin and target ownership, atomically
checks all entries before deleting any, and compares every retained Redis value
and expiry afterward. Changed type/value/expiry refuses the entire batch;
absent keys support uncertain-response retry. Production Redis and unrestricted
key-pattern cleanup are not accepted targets.

The first integration passed with head 27, three actual restores and exactly two
stale routes removed; four security/current/unknown keys remained unchanged
(`routing-restored-integration--20260919T103357Z-610111.log`, 101.761 s).
Independent final snapshot tests repeated the complete rehearsal at **head 28**:
**25 tests**, 111.152 s, zero failures
(`routing-review-unit--20260919T103639Z-679931.log`). Two added CLI dispatch/error
tests independently passed (`routing-cli-independent--20260919T103848Z-748750.log`).
All owned fixture resources were cleaned up.

Compiler verification found that CodeGraph's empty caller result for
`AllowIntent` missed the Admin limiter. The method was restored unchanged;
only the three unused legacy room-routing methods were removed. All server
packages compiled afterward (`routing-go-compile-fixed--20260919T103506Z-654164.log`).
Compilation selected zero tests and is not a runtime test claim.

### Platform checks and open size gate

Web build passed (`roadmap-web-build--20260919T102706Z-596718.log`), with
`main.dart.js` 3,539,442 bytes. Android debug compilation passed
(`roadmap-android-build--20260919T102753Z-598213.log`), but its universal APK is
**157,506,946 bytes**, failing the existing **52,428,800-byte** CI size gate.
The archive contains three debug engine binaries, kernel/isolate snapshots and
a Vulkan validation layer. Release-artifact measurement is underway; this
failure cannot be labelled a complete Android acceptance pass. Flutter inserted
the two Android Gradle compatibility flags used by this build.

Only the Linux desktop device is connected. Physical native/PWA accessibility,
low-end frame timing and macOS/Xcode evidence remain unavailable.

### Owner decisions and further work

The owner selected **Premium eligibility at authoritative match start** during
this continuation. Blueprint and roadmap now record that decision.
Interrupted-match bonus treatment remains pending. Reviewed retention/deletion,
provider/device checks, human editorial pilots, elapsed cohorts and commercial/
operator release evidence remain distinct prerequisites.

Local work continues on audited Noin corrections under existing migration 27,
followed by the remaining Admin, reconciliation and operational boundaries.
No all-phase completion or final staging is claimed by these partial gates.

### Package-size decision and release build evidence

The owner explicitly removed package size as a failing gate on 2026-09-19.
CI still builds Android and Web, and retains its macOS iOS build job; the
existing APK measurement now reports size without a threshold. Blueprint
Product Baseline is aligned with this decision.

Local split release APKs also built successfully in
`/tmp/agent-runs/roadmap-android-release-size--20260919T104417Z-763353.log`:
arm64 20,176,110 bytes; armeabi-v7a 17,830,488 bytes; x86_64 21,741,761 bytes.
These local artifacts use the existing signing configuration and do not prove
store signing, physical-device behavior or store acceptance. The earlier
150.2 MiB debug size is no longer an unresolved gate under the owner's choice.

### Audited Noin corrections

Implemented Phase 5 O1–O4 under existing migration 27. The internal wallet view
now offers reasoned grants and full refunds derived from one immutable
same-account Noin spend. Exact initiating-session authority is rechecked after
locks, and decision/audit/wallet/ledger/result are one transaction. No cash
refund, entitlement, points, XP or gameplay-cap behavior changes.

Independent source review and cold PostgreSQL/Redis `TestNoinCorrection|TestOperator`
passed in 10.450 s, exit 0:
`/tmp/agent-runs/noin-review-cold--20260919T105347Z-775957.log`.
Tests include replay/conflict, source uniqueness, spend races, authority expiry
after account/operation/wallet waits, balance overflow and ledger/audit/result
rollback. Full economy passed in 26.969 s and vet passed for Admin/economy/store.
The broader Admin run exposed an old content-publish fixture lacking the new
action certification artifact; its failure is being repaired in that fixture
and is not represented as an all-Admin pass.

Further independent plans cover account/installation sanctions (migration 29)
and leaderboard operations (migration 30), with implementations underway.

### Leaderboard controls and Premium capture

Migration 30 adds immutable weekly operator decisions/results. Exclusion is
applied before open-week ranking; closed reads use frozen history. Accepted close
work drains in bounded batches and resumes independently of the initiating
session. Final whole-package tests passed: leaderboard 1.335 s and Admin 68.000 s
in `/tmp/agent-runs/leaderboard-final-broad--20260919T111228Z-819294.log`.
This also resolves the earlier Admin content-fixture failure. Independent cold
review covered close/exclusion races and a 33-match bounded recovery case.
Main runtime recovery wiring is being joined with sanctions work.

Migration 31 and the authoritative Start transaction now capture one immutable
Premium eligibility decision per admitted participant, including Local Rooms.
The capture uses existing sorted account locks and rejects partial writes.
Historical absence is unknown, and this change does not pay a bonus.
Seven top-level tests (17 reported events) passed in 5.043 s, independently
repeated in 5.301 s:
`/tmp/agent-runs/premium-start-complete--20260919T110623Z-799879.log` and
`/tmp/agent-runs/premium-review-cold--20260919T110733Z-803341.log`.
An additional six-seat/PlayPass negative test passed in 0.38 s.

The privilege manifest and disposable cutover provisioning now include schema
29–31, insert-only evidence and exactly two invoker SQL sanction predicates.
Unknown routines and SECURITY DEFINER drift still refuse. Independent cold
`^TestCutover` passed in 33.061 s, exit 0:
`/tmp/agent-runs/cutover-all-independent--20260919T111342Z-822462.log`.
Earlier predicate-signature and return-type fixture failures are resolved;
the complete two-cluster integration will be repeated after the schema freezes.

### Delegated deletion policy

The owner delegated selection of mainstream mobile-app deletion policy.
Blueprint Profile §1a records the chosen product defaults and checked Apple/
Google guidance: immediate access/public suppression, active-data removal within
30 days, minimized purpose-bound evidence for 180 days, and backups expiring
within 90 days of creation. These durations are product defaults, not asserted
universal legal retention periods. The policy question is resolved.

Implementation is planned as D0–D6 in the roadmap. The source audit found that
ordinary deleted-account denial would strand surviving players' settlement, and
that shared immutable contract/outcome hashes cannot be reused after privacy
transformation. These require explicit terminal-work dispositions and verified
survivor witnesses before removing source payloads. No deletion endpoint or
successful cleanup is claimed by the policy or architectural plan.

### Head 31 two-cluster rehearsal

The actual separate-cluster cutover fixture passed all seven tests in 20.479 s:
`/tmp/agent-runs/cutover-head31-integration--20260919T111947Z-830463.log`.
The two restores used distinct PostgreSQL system identities, preserved 83 domain
tables plus the cutover authority inventory, retained an unacknowledged outbox
row and proved committed handoff recovery after lease expiry. Prepared
transactions, source-login drift, changed target data and wrong target identity
refused. Source writers stayed fenced after target activation.

The synthetic held transaction was applied once despite two deliveries; it is
isolated SQL replay, not a verified provider holding queue. The capture SHA-256
was `b5acaca3f43e3f5daa6603cb65fd227ad2a9b0fca3d44a4b74fa2052e27b59f0`.
Fixture resources were cleaned up. Later schema changes require a new rehearsal.

### Offline economics formulas

Phase 7 E1–E3 adds `tools/economics_report.py` with bounded strict JSON,
decimal-string accounting units and explicit missing/zero-denominator results.
All three illustrative Business Plan examples reproduce exactly. Cash burn is
independent of accounting contribution; cohort LTV preserves its observation
horizon. Every output labels itself unverified arithmetic with no launch
approval, and reports contain no inferred quotes or financial authority.

Expected preimplementation RED was the missing calculator module. Thirteen
focused tests now pass in 0.133 s:
`/tmp/agent-runs/economics-boundaries--20260919T112520Z-848562.log`.
These include maximum magnitude/rounding, zero ads/activity/burn, reserve
shortfall, private errors, new-only 0600 output and duplicate/symlink/FIFO/size
refusal. Independent review and full Python verification remain pending.

Independent source review subsequently found no economics issues, and the same
13 tests passed cold in 0.135 s, exit 0:
`/tmp/agent-runs/economics-review-cold--20260919T112653Z-851530.log`.
The mandatory runner's full Python slice is in progress.

That full Python slice subsequently passed all **115 tests across three stages**,
zero failures/skips, via `python3 xops/test/tests-lints.py --suite python`:
`/tmp/agent-runs/roadmap-python-head31--20260919T112726Z-852231.log` and
`/tmp/agent-runs/roadmap-python-head31.json`. Its 99-test xops stage took 131.500 s
and included fresh head-31 actual snapshots (three restores, 88 tables including
the fixture sentinel, six sequences) and the separate-cluster cutover. Selective
routing cleanup again removed exactly two obsolete keys and retained four others.
These results close the local E1–E3 calculator proof; the commercial parent
requires actual dated evidence and remains open.

### Sanctions and client recovery acceptance

Independent sanctions review and cold PostgreSQL verification passed 52 top-level
checks plus 131 subtests across seven packages:
`/tmp/agent-runs/sanctions-final-independent--20260919T114602Z-971853.log`.
Current credential expiry is checked after admission and OAuth lock waits, and
actual Admin HTTP decisions drive live WebSocket enforcement. Exact lifts and
expired decisions preserve unrelated authority and retained history.

Client proofs cover persistent installation identity, same-account upgrade,
OAuth recovery and storage failure. Regression tests exposed failed preference
writes leaving a misleading cache, stale asynchronous OAuth binding overwriting
a switched account, and expired replacement access. Full-suite verification
then found a completed initializer Future retained across Flutter test zones;
clearing only the identical completed queue tail preserves serialized writes.
All 515 client tests passed, with clean analysis:
`/tmp/agent-runs/sanctions-client-all-green--20260919T114753Z-978300.log` and
`/tmp/agent-runs/sanctions-client-final-analyze--20260919T114754Z-978562.log`.
The independent combined 60-test regression group and analysis also passed:
`/tmp/agent-runs/sanctions-client-queue-independent--20260919T115054Z-983934.log`.

### Closed deletion foundation

Migration 32 creates a separately owned privacy request/fence/receipt boundary
and four narrowly granted typed routines. Its sole erasure step removes a
profile only when its complete shape matches the reviewed manifest and no
accepted match or ledger effects need survivor handling. Unknown columns,
constraints, inheritance, RLS, user triggers and rewrite rules refuse;
transaction locks prevent concurrent schema drift. Faults roll back all step
receipts. This is a closed foundation, not a complete deletion endpoint.

Independent source review and all eight top-level cold tests passed in 4.850 s:
`/tmp/agent-runs/deletion-foundation-independent--20260919T115253Z-987370.log`.
Full D2–D6 reauthentication, source/content/evidence cleanup, survivor value and
independent restore suppression remain in progress.

The actual head-32 two-cluster rehearsal passed all seven tests in 20.906 s,
including two restores and 86 domain tables:
`/tmp/agent-runs/cutover-head32-integration--20260919T115508Z-991516.log`.
It additionally fenced the privacy executor's existing and future connections.
Its held event remains an isolated SQL uniqueness proof, not provider delivery.

### Backup restore lifetime

D5a now pins creation at capture start and expiry no later than 90 days later.
Restore requires strict integer UTC timestamps and rechecks the deadline before
target creation and before successful completion. Late capture retains its
INCOMPLETE marker; late restore closes its owned target without a success receipt.
Hash verification remains available for inspecting an expired artifact.

Independent source review and 30 cold safety/CLI tests passed in 0.665 s:
`/tmp/agent-runs/backup-retention-independent--20260919T120220Z-1032175.log`.
The complete snapshot suite then passed all32tests in109.332s, including actual
legacy8→head32 migration and three restores across91tables (fixture sentinel
included),6sequences and selective routing parity:
`/tmp/agent-runs/backup-retention-complete--20260919T120142Z-1011517.log`.
This does not implement expired-copy purge or external suppression; D5 remains open.

Cutover32 independent final source review and all controller/role tests also
passed: store38.136s, CLI0.016s, exit0,
`/tmp/agent-runs/cutover32-final-independent--20260919T115813Z-1005152.log`.

### Independent suppression fixture journal

`server/internal/privacy` supplies D2's Publisher contract and a concrete closed
fixture journal. Records contain purpose-HMAC selectors instead of account UUIDs,
canonical signed request identities and a verified hash chain. Ed25519 keys and
selector keys are supplied separately and never written to the journal. A
separate signed private minimum file advances durably before Publish succeeds.
Both directories must be disjoint from declared application backup roots and
each other; reopening the minimum object refuses older journal contents.

Cross-handle file locking, bounded reads, strict canonical decoding, uncertain
rename/fsync replay, minimum failures, cancellation and key/installation drift
have regression proofs. A fresh verifier challenge is signed with the complete
head; stale/future/challenge-replayed heads refuse. A reviewer-found lossy
sub-microsecond timestamp identity was reproduced and now refuses explicitly.

Independent source review, all11top-level race tests (plus6subtests), and vet
passed, race runtime1.270s:
`/tmp/agent-runs/privacy-journal-independent--20260919T121740Z-1103467.log` and
`/tmp/agent-runs/privacy-journal-vet-independent--20260919T121756Z-1103900.log`.
This is a filesystem mechanics/replay proof. No production authority, processor,
retention purge or restored application admission is claimed; D2 binding and
complete executor/restore integration remain required.

Latest client artifacts also rebuilt after the installation/OAuth fixes:
Android debug5.6s (`roadmap-android-final--20260919T120250Z-1054839.log`) and
Web18.6s (`roadmap-web-final--20260919T120608Z-1083125.log`), both exit0.
Physical-device performance and iOS still require their stated environments.


### Reward claim verification and cutover schema 34

Closed reward foundation B1–B3 passed independent store/config/signature coverage
(15 top-level, 21 nested cases) and the final HTTP deadline gate (3 tests,
4.779 seconds). Immutable opaque claims and provider receipts remain separate
from payouts. No reward route is publicly mounted. Premium eligibility is fixed
at start; interrupted-match policy and payout/delivery/consent joins remain open.

Schema 34 cutover integration passed 8 tests in 21.618 seconds, including two
actual independent-cluster restores and exact parity of 90 domain tables.
A real privacy-executor login completed enrollment, challenge, confirmation,
suppression binding and profile fixture removal while direct preparation was
denied. Exact source ACLs are compared in both directions, including omitted
grants; reward tables have runtime SELECT/INSERT only. The later D2 cleanup
lock-order review requires refreshed function pins and another cold gate before
final acceptance.


### Final handoff scope requested by owner

The owner requested finalization for a human remote push. Source is frozen at
schema34, with no migration35/36 and no D3 survivor-value implementation. Closed
D2 authority uses ten narrowly granted functions, including independently
committed bounded expiry cleanup to avoid account/intent lock inversion.
Source readers reject deleted/fenced authors and acquire release rows before
active mappings; atomic withdrawal and shared-data erasure remain pending.

Final independent review accepted the current boundary with no outstanding
blocker: 48 top-level tests and 133 subtests passed (store60.518s, auth2.054s,
handler0.552s), plus the nil-UUID regression0.003s; diff whitespace checks passed.
Evidence: `final-boundary-independent--20260919T123652Z-1163931.log` and
`deletion-id-independent--20260919T123815Z-1219763.log` under `/tmp/agent-runs`.
The full mandatory repository runner and final immutable image proof are the
remaining finalization gates; their completed results are recorded below.


Final immutable production-image proof passed all six stages: embedded manifest,
actual migration CLI, runtime without object storage, Redis outage/recovery,
PostgreSQL ownership-loss/restart and incompatible-startup refusal. The fixture
source inventory exactly matches the final source (zero drift), schema34,
manifest `37e282269b8dd4fd79f0b81a6546f29ec475b1e1cbcf45f6ad1f0751bad4e6e3`,
image `sha256:3c420092d214e1c3a53c2ccc5b9072019084040c3104f8506aa4cb7adce055fc`.
Evidence: `/tmp/agent-runs/knowoff-artifact-312a6ae22edd-7s39bnbx/results.json`.


The mandatory full run completed with all Python121, gamebot81,
mediapack-Go24, web-cache2 and client515 tests passing, plus client analysis and
formatting and Go vet/build checks. Its server test stage exposed two head31
rollback fixtures that ignored a later FK, a schema24 fixture that invoked a
newer runtime, and the enlarged store package exceeding its120-second watchdog.
A Go formatting issue was also found and corrected. These were repaired with
dependency-ordered real migrations, explicit historical-schema seeding with
stronger receipt parity, and a bounded300-second package watchdog; inner SQL,
network, performance and load assertions remain unchanged. Independent repair
verification passed all3 migration tests +2 subtests and21 runner tests.
The full store package is rerun below; the initial failed report is retained
rather than relabeled green.

The100-room matched warm/measurement and100-match soak passed197.530s:
action RTT p95=125.965ms,p99=151.533ms; worst frame8185bytes; warm/final heap
2473248/2556984bytes; warm/final goroutines8/8. No runtime source changed during
the fixture/watchdog repairs. Source hashes in the production-image evidence
still match every runtime and migration file; subsequent edits affect tests
and the test runner only.


Final repaired gates are green. The complete store package passed496 tests/subtests
in186.683 seconds, followed by vet and build, with zero failures/skips. Server
format verification is clean. Combining the full runner's unaffected packages
with this complete cold store replacement yields 2207 passing Go tests/subtests
across31 packages containing tests, with zero remaining failures or test skips.
Python121, Flutter515 and web-cache2 passed; the changed runner's21 tests also
passed independently. Android debug and Web artifact builds passed earlier on
the same client source. iOS/device/provider/human release evidence remains pending.

Reports: `/tmp/agent-runs/roadmap-final-unified.json` (initial failure retained)
and `/tmp/agent-runs/roadmap-final-store.json` (complete repaired package),
plus `roadmap-runner-regressions--20260919T124907Z-1283985.log`.
