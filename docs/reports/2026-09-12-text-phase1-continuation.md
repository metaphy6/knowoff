# Text Phase 1 continuation evidence — 2026-09-12

Run: `text-phase1-cont-20260912`, continuing the already staged foundation.
This report extends the [foundation validation](2026-09-12-text-phase1-validation.md).
It distinguishes owner-attested deployment status, observed development state,
isolated fixture proof and engineering review from implemented runtime behavior.

## Deployment inventory

The project owner explicitly stated **“No deployment exists”** in the
2026-09-12 implementation session. The coordinating agent relayed that statement
before this inventory was finalized at approximately **09:47 UTC**. Deployment
absence is owner-attested; it is not inferred from an empty local Docker list.
Accordingly, deployed schema/dirty version, app/client/protocol/pack generations,
active matches, retained accounts/purchases/entitlements and object consumers
are **not applicable to an existing deployment**. No live database count/hash
report is claimed, and no nonexistent deployment was started to manufacture one.

Read-only local corroboration:

| Surface | Observation |
|---|---|
| Docker daemon | Current `default` context uses local `unix:///var/run/docker.sock`. Zero running or stopped containers; zero Compose projects. |
| Docker storage/network | Only the validation-created `knowoff-tests-go-build` compiler cache volume and Docker's default networks exist. No instantiated project PostgreSQL, Redis or MinIO volume. |
| Host processes | No process-name match for Knowoff server, PostgreSQL, Redis, Nginx or cloudflared. Four Dart processes are SDK/home-directory Flutter daemon/DevTools/tooling; none is a repo-directory `flutter run` process. This is metadata corroboration, not an exhaustive machine/remote-deployment census. |
| Host listeners | None on the configured development service ports 5432, 6379, 8080, 9090, 9091, 9000, 9001, 8081, 80 or 443. No confirmed local database target exists for a read-only preflight invocation. |
| Configured Compose topology | `infra/compose/docker-compose.yaml` core services are PostgreSQL, Redis, MinIO, migrate, server, Adminer and Nginx; declared data volumes are `pg_data`, `redis_data`, `minio_data`. Client-web and cloudflared definitions are commented out. Configuration is a development topology, not evidence that it has run. |
| Local client artifact | `client/assets/config.json` declares protocol 1 and localhost HTTP/WebSocket port 8080. This is source configuration, not a deployed client generation. |
| Local content artifact | `content/packs` contains one format-1 English manifest and two declared JSONL members: 150 text media rows and 5,400 text card rows. There are no asset files in that bundle. `content/ingest` contains only an empty placeholder; `client/assets/packs` does not exist. None is a deployed/active object consumer. |

No container environment values, credentials, receipts, account identifiers or
content wording were printed. No service, migration, deployment, restore or
remote connection was invoked. Unrelated host listeners were not contacted.

The existing [VPS runbook](../launch/VPS_MIGRATION_RUNBOOK.md) remains a future
operator procedure: readiness/maintenance alone does not prove match drain,
Redis routing does not recover in-memory matches, and its legacy snapshot/restore
scripts are not safe evidence collection. Future deployments must record their
actual server/client/protocol/pack generations and live match/object consumers.
For an explicitly selected database, `server/cmd/transition-preflight` accepts
`KNOWOFF_PREFLIGHT_DSN`; `--timeout` bounds the database observation after local
SQL-file hashing. The version-2 artifact wraps deterministic database evidence
with a timestamp, local file hashes and explicit coverage. Its fixed-category
content/status/entitlement counts, exact wallet/ledger aggregate totals and
public-table fingerprints come from one read-only repeatable-read transaction.
It rejects ambiguous migration state and reads that would be row-policy filtered;
it cannot establish live match/client activity or deployed SQL bytes.

The already completed isolated proof remains **7 store tests passed, zero
failed/skipped**, including repeated read/up hash parity, equal-count changed-row
detection, redaction, dirty migration refusal, cancellation, connection release
and the disposable database identity guard. Its full-suite evidence is
`/tmp/agent-runs/text-phase1-final-verifier--20260912T092733Z-213368.log` and
`/tmp/agent-runs/text-phase1-final-verifier-results.json`. No second full baseline
was run for this inventory.

## Wallet engineering review

The independent wallet-domain review inspected the current quota, wallet,
conversion, profile, leaderboard, match callback and SQL definitions through
CodeGraph, then compared them with the adopted Blueprint and
[durable identity design](../design/DESIGN-text-transition.md#phase-1-durable-identity-and-transaction-design).
The traces below are **static/tabletop failure traces**, not newly executed
reproductions or claims that the legacy runtime was repaired. They explain why
the future constraints are necessary; Phase 5 must supply failure-injection
tests against its actual implementation.

| Priority / legacy trace and precondition | Source anchors and required boundary |
|---|---|
| P1 — Partial finish loses effects; callback replay duplicates the committed prefix. A profile write commits, then a later leaderboard/grant operation fails or the shared five-second context expires. Errors are logged and processing continues without durable pending work. Repeating the callback increments the already committed profile/points/XP again. | `server/internal/lobby/room.go:677`, `server/internal/profile/profile.go:111`, `server/internal/economy/wallet.go:78`. Stable match/outcome and per-account settlement identities must atomically capture local effects; remote delivery uses durable outbox identities. Reused room IDs cannot identify rematches. |
| P1 — Event rewards are deferred, collapsed or lost. A player earns two correct votes, or a Donower survives a vote then loses the team result; current state keeps one `CorrectVote` boolean, and grants are computed only at finish. A player absent at finish is skipped entirely. | `server/internal/game/match.go:1368`, `server/internal/economy/grants.go:28`, `server/internal/lobby/room.go:756`. Persist each accepted vote event separately at occurrence, retaining configured amounts and private role-linked presentation. Event survival must not depend on eventual team victory; already earned value survives absence/interruption. |
| P1 — First-win duplication can survive serial grant transactions. Two callers both perform the outer ledger lookup before either grants; caller A commits its grant, then caller B begins its grant with sufficient daily cap remaining. B never repeats the first-win predicate inside that transaction. | `server/internal/economy/grants.go:109`, `server/migrations/000003_phase5_economy_admin.up.sql:13`. A unique account/day first-win claim belongs in the grant transaction. Zero/capped receipts still consume their logical event identity; an after-midnight retry cannot mint the old award. |
| P1 — Quota eligibility and consumption are separate, and rematches bypass the queue check. Two queued starts can observe the last free slot; expiry/day changes between queue and start are not revalidated. Counter updates happen after engine start, lack a match key, log failures, and include paid participants. | `server/internal/economy/economy.go:46`, `server/internal/economy/economy.go:85`, `server/internal/lobby/manager.go:340`, `server/internal/lobby/room.go:831`. Reserve/start/cancel/compensate atomically by match/account and original day; every rematch revalidates access. Paid/local/prototype access does not consume free allowance. |
| P1 — Daily leaderboard cap is computed from a cumulative weekly row. If day one reaches its cap, day two's first match sees yesterday's `updated_at` and passes; its update moves the whole weekly counter into day two, so the second match is already blocked. Concurrent calls can also both read below-cap before their unlocked additive updates. | `server/internal/leaderboard/leaderboard.go:52`, `server/migrations/000002_phase4_accounts_audit.up.sql:88`. Use a real shared account/day counter or immutable eligibility events with transactional counting; retries must retain accepted day/week. |
| P1 — Weekly history can diverge or change after close. `CloseWeek` reads rankings while `RecordPoints` may still add effects; writers never check closed state. Repeating close updates existing history rows instead of returning an immutable result. | `server/internal/leaderboard/leaderboard.go:144`, `server/internal/leaderboard/leaderboard.go:52`. A shared database barrier drains accepted pre-cutoff work before snapshot/close and rejects later mutation of that closed generation. |
| P1 — Scored low-population ending delivers client points without invoking the persistence callback. A match with accrued points ends by that path; no profile/leaderboard settlement is initiated there. | `server/internal/game/match.go:399`. Route this outcome through the durable result path, preserving disconnected players' accrued points without fabricating a team winner or paying team/first-win awards. |
| P2 — First-row cap races are not a durable retry strategy. A missing daily row cannot be locked by the existing `SELECT ... FOR UPDATE`; `Grant` and conversion use repeatable-read transactions, so a competing write may cause serialization failure. The finish callback logs a failed grant without retry work. This review does **not** claim that this repeatable-read race necessarily commits an over-cap grant. | `server/internal/economy/wallet.go:78`, `server/internal/economy/wallet.go:105`, `server/internal/economy/convert.go:16`. Serialize missing-row creation, share the account/day→profile→wallet lock order across all money writers, and retry whole immutable-key transactions within bounded context for `40001`/`40P01`. |
| P2 — Arithmetic parity is insufficient recovery evidence. Replaying a grant twice can keep wallet balance equal to ledger sum, while losing both a grant and its ledger row also preserves equality. Audit writes swallow database errors and the finish callback supplies an empty match ID; summing those audit rows cannot establish exactly-once awards. | `server/internal/economy/wallet.go:230`, `server/internal/audit/audit.go:43`, `server/internal/audit/audit.go:71`, `server/internal/lobby/room.go:724`. Reconcile immutable outcome/award receipts against every local value effect and pending outbox work; preserve unknown historical identities. |

The accepted design corrections now specify the common account/day/profile/
wallet lock protocol, bounded whole-transaction retries, an immutable weekly
close barrier, and the event-versus-terminal reward matrix. Recovery additionally
requires a durable process-incarnation/fencing epoch checked by each new database
write: merely naming a lost process cannot prevent a paused former owner from
resuming after quota compensation. Existing committed immutable outcomes remain
settleable; interruption must win a serialized state transition before creating
its single compensation.

Existing tests inspected for coverage include sequential quota/Play Pass checks,
single-day sequential leaderboard cap, one close, profile result application,
conversion cap and wallet/ledger arithmetic. Their previously passing full-suite
result remains valid for that coverage; it does not execute the concurrent,
rollover, replay and crash traces above. No additional test total is claimed.

**Review disposition: PASS for the Phase 1 durable-identity/transaction design**
after incorporating these engineering corrections. The coordinating agent
accepted them in this session. This records technical review of the adopted
economy; it changes no reward amount, cap, interruption policy or paid benefit,
and does not approve an implemented Phase 5 settlement runtime.

## Inventory implementation proof

Four CLI test functions cover a real read-only database artifact, local migration
file byte/hash parity, duplicate/unpaired/symlink rejection, missing DSN, invalid
arguments/deadlines and error redaction. Help succeeds without a database.
Database additions exercise fixed-category aggregation, exact value deltas and
private-label redaction. An ambiguous migration table reproduced a false clean
version; the corrected query now refuses multiple migration-state rows.

Independent review additionally reproduced a restricted role with SELECT access
seeing zero of one stored row under row-level security. Preflight now sets
`row_security=off` inside its read-only transaction so PostgreSQL rejects an
inventory that would be filtered; this setting grants no bypass permission.
See the [PostgreSQL 16 inventory behavior](https://www.postgresql.org/docs/16/app-pgdump.html).

Targeted passing logs: `/tmp/agent-runs/text-preflight-targeted-green--20260912T095548Z-267919.log`
(four CLI functions/ten terminal records and eight selected store tests), then
`/tmp/agent-runs/text-preflight-rls-green--20260912T095911Z-275891.log`
(nine selected store tests including the new policy-filtering regression).
Selected-file runs kept the concurrently unfinished migration design helpers
outside this targeted scope; they are not substitutes for the final full gate.
The missing native C compiler was handled through the existing project Go image;
no host packages were installed. Temporary test-first compile failures and the
ambiguous/RLS regression reds were diagnosed before their corrected runs.

## Additive migration and backfill design proof

The fixture lives only under `server/internal/store/testdata/transition-design/`
with test helpers in `transition_design_helpers_test.go`. It does not create
production migrations 000009–000012 or change the sixteen applied SQL files.
Its realistic head-8 seed contains image/text submission and challenge history,
terms/consent, self-references, topic/entry/vote/winner links, contributor credits,
wallet/ledger/day counters, a paid theme and expiring Premium entitlement,
verified/refunded receipts, reports and feedback. All values are synthetic.

The additive archive uses a source-kind/UUID identity and unique destination
identity, preserving same-UUID records from different source tables. Mappings
retain source hashes and references; every row remains `legacy_unreviewed` with
null mode, language and content revision. Archiving grants no suitability,
review approval, publication or contribution reward.

Five PostgreSQL test functions with thirteen named subtests passed:

- Capture a bounded source manifest, copy and verify in explicitly limited
  primary-key batches, atomically commit each mapping/cursor/hash update, then
  reconnect and resume after a committed batch. An injected mid-batch failure
  rolls back both rows and progress; repeated start and identical mappings are
  idempotent.
- Reject duplicate source identities, conflicting source hashes and destination
  collisions without overwrite; preserve distinct source namespaces.
- Detect changed/deleted source rows and inserted keys both below and above the
  captured cursor boundary. Copy/verification counts and ordered hashes must
  agree before completion. Writers must remain frozen from capture through
  cutover; these two bounded passes are not an online-migration safety claim.
- Reject a genuine schema at migration 7 and dirty migration 8 before additive
  DDL. Fresh installation still reaches clean head 8 and repeated up is a no-op.
- Permit the exact fixture down only with empty archive/progress tables. Refuse
  down after retained mappings or job evidence exist, with no original table
  mutation. Preserve complete original-table counts/row hashes, references,
  blobs, consent and value across success and failure paths; generic historical
  all-migrations-down is not a rollback strategy.

Intentional absent-feature test-first red:
`/tmp/agent-runs/transition-design-red--20260912T095104Z-255906.log` failed to
compile because the fixture APIs did not yet exist. This is distinct from the
behavioral ambiguity/RLS red tests above. The actual design behavior passed on
isolated PostgreSQL 16 in
`/tmp/agent-runs/transition-design-postgres-docker--20260912T100059Z-283066.log`
(9.517 seconds), followed by independent read-only review of all helper/SQL files.
The final unified run below remains the full repository gate. Phase 5 must rerun
these proofs against its actual new migrations, backfill and runtime writers.

## Final independent verification and handoff

The independent reviewer approved the complete continuation after the RLS
completeness fix, explicit database-only timeout wording and final migration
identity/restart checks. The separate verifier then ran all default suites via
`python3 xops/test/tests-lints.py` with fresh disposable PostgreSQL 16/Redis 7
and the existing project CGO image. **18/18 checks passed; 648 unique top-level
tests passed with zero failures, skips or failed packages.**

| Scope | Unique tests passed | Other verification |
|---|---:|---|
| Server Go | 284 | gofmt, vet and build passed |
| Go mediapack / gamebot | 3 / 0 | package tests, gofmt, vet and build passed in both modules |
| Python media / ops / runner | 34 | all three retained suites executed |
| Flutter | 327 | analyze passed; format checked 59 files, zero changed |

Go's 462 test/subtest terminal records and the aggregate 823 language events are
not unique-test totals. The isolated services/network were confirmed removed.
All sixteen original migration files remain byte-identical, tracking remains
append-only and the documentation links/whitespace checks pass.

Full log: `/tmp/agent-runs/text-phase1-continuation-final-verifier--20260912T100258Z-288277.log`.
Machine report: `/tmp/agent-runs/text-phase1-cont-final-results.json`.
Unique-count/cleanup summary: `/tmp/agent-runs/text-phase1-cont-final-summary.json`
and `/tmp/agent-runs/text-phase1-continuation-counts--20260912T100526Z-298713.log`.

Phase 1 is **12/12 complete (12/91 overall)**. This proves contracts, baseline,
inventory, reviewed transaction design and the isolated migration design;
it does not implement the five playable mode engines, a production backfill,
settlement runtime or live v2 cutover. Next are Phase 2 catalog/dealing and
Phase 3 engine/locking/ownership implementation against synthetic fixtures.
Remote CI, native release artifacts, human content certification and release
playtests retain their later roadmap gates. Nothing was committed or pushed.
