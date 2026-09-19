# VPS migration and text-transition runbook

**Status — 2026-09-12:** operator procedure with locally verified text-runtime
drain; no deployment exists and no live migration has occurred. Isolated
backup/restore evidence is tracked in the resumption report. The [Blueprint](../../BLUEPRINT.md) owns the target;
the [transition design](../design/DESIGN-text-transition.md) owns compatibility,
data and retirement requirements; the [Roadmap](../planning/ROADMAP.md) orders
implementation. No production data, service, schema, DNS or storage change is
authorized or performed by this documentation update.

The target is one writable PostgreSQL database, Redis coordination and a Go
server loading an immutable certified text bundle. The active server uses protocol v2 and checks PostgreSQL/Redis readiness;
Compose storage and legacy source retirement remain separate gates. A host move and the
text schema/protocol cutover are separate changes: rehearse them separately
before combining them in an operator-approved release window.

## Implemented local proofs and remaining execution gates

- The [snapshot tool](../../infra/compose/snapshot.sh) now accepts only its own
  nonce-labelled disposable fixture resources. It never restores into existing
  databases/volumes or operates ordinary Compose resources. The reviewed
  head21 rehearsal preserved PostgreSQL schema/data/roles/ACLs/settings,
  sequences, Redis absolute expiries, two retained object versions and immutable
  release inputs across three restores. Commands, files and cleanup are bounded.
  See the [dated evidence](../reports/2026-09-12-text-transition-resumption.md).
  This is an executable rehearsal, not production backup authorization; a real
  deployment still needs an implemented all-writer barrier and selected target.
- The internal authenticated runtime drain/status surface is verified against
  real persisted matches. It reports match/admission/settlement work and rejects
  stale process generations. It does not yet coordinate every SQL/background
  writer; `writers_quiescent` remains false. A maintenance notice or Redis
  `knowoff:drain_check` message does not establish either condition.
- Room state lives in process memory. Redis records and an RDB snapshot do not
  recover hands, phases, timers or pending trades. Confirmed owner loss preserves
  committed awards and records interruption/allowance compensation exactly once.
- Startup resolves persisted immutable certified text releases and checks exact
  clean schema compatibility before process ownership. A local synthetic pack
  is available only through explicit non-production prototype configuration.
  The compiled `release-manifest` command reports paired migration bytes/hashes,
  supported modes and protocol before reading configuration or credentials.
  Image build788806 and manifest931787 prove the head21 artifact; rebuild and
  reverify after subsequent migrations. Actual target ingress/device evidence
  remains required; no service has been deployed.
- Durable account settlement, outbox recovery, shared caps and provider receipt
  processing have real PostgreSQL tests. Current subscription discovery/retry
  integration is still being completed. Platform purchase/ad test accounts,
  reviewed retention/consent policy and actual device results remain distinct
  release gates; synthetic receipts cannot certify live purchases.

The latest head-34 cutover fixture uses separate schema and privacy
NOLOGIN owners, runtime, migrator, privacy executor, capture and offline control.
Control fences all three LOGIN writers and their existing sessions. Exact
private-table/function ownership, signatures, bodies and exact source ACLs are verified;
unbound deletion-suppression publication prevents sealing. The two-cluster
rehearsal preserves 90 domain tables plus cutover authority. Source SQL fencing
still requires a separately proven external publisher/provider drain.
See the [current evidence](../reports/2026-09-19-text-transition-resumption.md).

## 1. Prepare a concrete release manifest

Record source and target host, operator, window, rollback owner, database
identity, migration version/dirty state, image digest, config digest without
secrets, protocol/client minimum, enabled modes/languages, text release ID and
checksums. Include the last compatible application/schema/pack combination,
rollback deadline, recovery-time objective and agreed recovery point.

Inventory deployed client generations and outstanding purchases, contributor
reviews, challenge closures and jobs. Preserve JWT/account identity settings
needed for continuity. Provision target access through the deployment secret
store; do not print secrets in commands, logs, reports or manifests.

Use service names from the checked-in Compose file rather than generated
container names. Read-only inventory from the repository root is:

```bash
pwd
docker compose -f infra/compose/docker-compose.yaml --profile core config --services
docker compose -f infra/compose/docker-compose.yaml --profile core ps
```

This checks the selected Compose project only. Resolve the production project,
overlays and host explicitly before any later command; do not assume a local
project name or bucket is the production one. Do not paste a full interpolated
Compose configuration into a report because it can contain secrets.

## 2. Rehearse on isolated data stores

Restore a source snapshot into a separately named disposable target, never
into production volumes. Use a PostgreSQL-compatible dump/restore toolchain,
validate command exits and perform these three distinct rehearsals:

1. Restore the old schema/data and start its compatible old application.
2. Apply the additive migration/backfill, interrupt and resume it, then run
   the new application against the expanded schema.
3. Snapshot and restore the new schema/data, including new settlements and
   content-release metadata, then exercise the exact allowed rollback or
   forward-fix procedure.

Migrations `000001`–`000008` are historical inputs, not files to rewrite. Check
legacy content formats before upgrades: `000008` already refuses formats other
than text/image. New text constraints must coexist with preserved historical
records or a verified archive/reference map. Test valid and invalid old rows,
foreign keys, repeated up migrations, dirty-state refusal and backfill cursors.
Never change image records to text just to satisfy a constraint.

Do not invoke generic `MigrateDown`: it rolls back all migrations, and historical
down paths are not a lossless rollback guarantee. In particular, `000005` can
collapse roles and `000007` refuses a downgrade that loses application history.
Schema contraction belongs after compatibility and retention gates, not this
initial host move.

## 3. Freeze admission and drain all writers

Use the internal Admin listener with an authenticated Admin browser session.
`GET /admin/runtime/status` returns the current `owner_id`, `generation`,
`observed_at`, process counts and `durable_all_owners` counts. Submit
`POST /admin/runtime/drain` with `Content-Type: application/json`, the session's
`X-CSRF-Token`, and exactly `owner_id`, `generation`, and a fresh UUID
`request_id`. Reuse those exact values after an uncertain response. The durable
audit records that request; a replacement process rejects the old generation
with `409 runtime.owner_changed`. Never place session or CSRF secrets in URLs,
shell history or reports.

The operation stops new admission, releases unused reservations and lets begun
matches finish on their pinned content. Repeated status reads do not advance
game clocks. `GET /admin/runtime/wait?timeout_ms=5000` waits at most five seconds;
`202` reports the most recent successful timestamped counts with `timed_out`.
A failed initial read returns `503`, never a fabricated empty state. Repeat a
bounded wait while the same process remains authoritative.

`matches_drained` requires closed admission and zero active matches, pending
trades/aborts, queued players, durable prepared/started matches, reserved
admissions and pending/missing settlements. Unacknowledged output is reported
separately: its value is already committed. Empty waiting rooms do not prevent
match drain. Confirm these conditions before stopping the old process. If the drain deadline expires,
keep the release paused or invoke the separately reviewed interrupted-match
policy; do not invent completed outcomes or silently drop earned rewards.

Quiesce every writer, including Admin/Portal mutations, challenge/leaderboard
jobs, receipt/ad callbacks and contribution processing. Route retryable
callbacks to a durable queue or return the documented retry response. Count
and later reconcile held events. An active maintenance banner alone is not
write quiescence. Record the last durable settlement/receipt/audit watermark.

After process loss, clients receive a clean interrupted-match/rejoin flow.
Already durable grants survive; unfinished games do not become completed games
when Redis routing is restored. Reconnect must not fabricate an old hand or
restart an old deadline.

## 4. Snapshot durable data and retained artifacts

Capture PostgreSQL while writers are quiescent. Encrypt and restrict access to
the backup; include schema, data, sequences, constraints and required roles/
privileges, documenting any exclusions. Verify the dump is readable and record
its checksum/size/tool versions. A nonempty backup file alone is insufficient.

Fixture backup manifests now pin UTC capture-start creation and expiry, with a
maximum 90-day lifetime. Restore and restore dry-run refuse missing or malformed
windows, future creation and the exact expiry instant. Restore rechecks expiry
before creating its isolated target and before certifying success. Hash-only
`verify` remains an integrity inspection; it does not authorize restoring an
expired copy. This enforces lifetime admission, not deletion of expired copies
or fresh independent deletion suppression; those D5 obligations remain open.

Use the actual schema inventory, then record counts and deterministic row/value
checksums for every table. The retained legacy schema includes:

| Group | Tables |
|---|---|
| Migration and identity | `schema_migrations`, `accounts`, `device_tokens`, `oauth_links`, `auth_revocations`, `profiles` |
| Gameplay audit/access | `audit_events`, `queue_cooldowns`, `daily_quickplay_counts` |
| Economy | `noin_wallets`, `noin_ledger`, `daily_noin_earned`, `entitlements`, `store_purchases` |
| Leaderboard | `leaderboard_weeks`, `leaderboard_entries`, `leaderboard_history` |
| Administration | `admin_accounts`, `admin_sessions`, `admin_audit_log`, `system_notices`, `reports`, `feedback`, `custom_avatars` |
| Contribution workflow | `portal_roles`, `portal_role_applications`, `portal_terms`, `portal_submissions`, `portal_submission_counts`, `guard_freezes` |
| Browser access | `portal_browser_sessions`, `portal_login_requests`, `portal_login_limits` |
| Challenge | `challenge_topics`, `challenge_entries`, `challenge_votes`, `challenge_winners` |

There is no `media_packs` or `admin_audit` table. The table above is not a
complete current inventory: migrations9 onward add admissions, outcomes, owner
fences, releases, trust, challenge lifecycle, reports, OAuth and provider source
authority. Use the actual database catalog plus the compiled migration manifest
for every rehearsal; never use a hardcoded table list as backup coverage. Preserve receipt
transaction IDs, amounts/status, entitlement values/expiry, terms/version,
creator credits, report/challenge references and avatar blobs. Prove wallet
balances equal the ledger sum and compare per-day counters and profile totals;
matching row counts alone cannot prove parity.

Inventory every stored object prefix and consumer before retiring MinIO. For
retained objects, export through an authenticated tool with a verified host
output mount; record relative key, byte count and checksum. Preserve legacy
provenance and attribution under the agreed retention policy. Record immutable
text bundles and their certification artifacts separately. No public catalog
or CDN may expose private Nown data as a shortcut for client sync.

If Redis state is retained, wait for a successful bounded snapshot operation,
verify persistence status and checksum its output. Classify keys: expired
room/node references to dead processes must be cleared/rebuilt selectively;
auth/rate/security state must not be erased blindly. Do not call an RDB backup
live-match recovery evidence.

## 5. Restore and verify before routing users

Transfer checksummed encrypted artifacts over authenticated transport. Restore
into fresh target stores with the target application and writer jobs stopped.
Restore required database roles/privileges, verify sequences and run all parity
checks. Apply only the rehearsed additive migration/backfill for this release.
Validate every deployed text bundle before selecting it as active.

Start the intended production image with admission still closed. Confirm the
known `/healthz` and `/readyz` surfaces through the selected ingress, including
failure behavior for actual required dependencies. Text readiness checks PostgreSQL, Redis, process authority and admission
policy; it has no MinIO dependency. Verify the selected Compose/ingress
artifact also has no mandatory playable-object-store dependency.
Verify the new process cannot accidentally load the old fixture or expose
prompt catalogs through old media routes.

Run isolated test-account smoke matches with no live rewards: every enabled
mode, both table sizes, chosen languages, native and PWA clients, readiness,
turn actions, reconnect, pending-offer expiry, discussion/ballots, verdict and
rematch. Confirm role secrecy on the wire. Test old-client rejection, stale
service-worker/cache state and account/entitlement continuity. Dev/test bots
may support a dedicated test environment after their protocol update; production
bot backfill stays disabled and cannot substitute for human release proof.

## 6. Cut over with one authoritative writer

Switch the verified ingress/DNS to the target while source application and jobs
remain read-only/stopped. Confirm no client is admitted to an old-protocol
match; provide an explicit update-required response before binding or charging
access. Reopen writers in a controlled order, release held callbacks exactly
once, then open the certified mode queues/rooms.

Observe error rates, queue fill, action latency, reconnects, settlement lag,
ledger parity and content-release identity. Use bounded metrics and redacted
records under the [logging contract](../guides/LOGGING.md). Record the first
target write watermark: it changes which rollback is safe. Keep the source
snapshot and immutable release artifacts intact through the declared rollback
window. DNS TTL expiry alone is not permission to destroy them.

## 7. Roll back according to write state

| State | Permitted recovery |
|---|---|
| Target has accepted no writes and source has not committed durable cutover closing | Freeze/stop target, verify the source remains complete, cancel the pre-closing drain and restore compatible source routing/application. Reconcile held callbacks before admission. |
| Source has committed durable cutover closing, regardless of target writes | Keep source writer logins fenced. Schema 26 cannot reopen that source identity; proceed with the verified separate-cluster handoff or a separately reviewed additive recovery protocol. Never disable guards or rewrite authority history to reopen it. |
| Target has accepted writes; expanded schema remains compatible | Freeze admission and drain; roll back the application/config/pack on the same authoritative database using the rehearsed compatible release. Retain target receipts/results. |
| Target writes exist and the older app/schema is incompatible | Keep maintenance active. Prefer a forward fix; otherwise reconcile/replay target writes into a reviewed recovery database. An older snapshot requires an explicit recovery-point/data-loss decision, never routine DNS reversal. |

Do not replay a settlement without its stable idempotency key. Reverify wallet,
receipt, entitlement, counter and contribution parity before restoring service.
Record failure cause, affected interval, exact artifacts and recovery evidence.

The first physical handoff requires distinct PostgreSQL cluster system IDs.
Roles are cluster-wide: enabling the same writer roles in a sibling database
would also reopen source logins. A schema-only sibling-database fixture proves
identity constraints, not safe physical writer activation.

## 8. Contract and retire only after the rollback window

After old clients/matches and retained consumers are accounted for, remove
obsolete protocol/image/specialty/backfill paths, config/env references, storage
routes and unused volumes through the reviewed retirement inventory. Preserve
applied migrations, historical ADR/proof records and justified data archives.
No production wipe, `compose down -v`, or snapshot-script volume removal is part
of routine transition cleanup. Retained archives need access, owner, retention
and deletion rules; a permanent unused compatibility branch is not an archive.

The run is complete only when its record contains release identity, all parity
results, smoke evidence, writer watermarks, measured recovery time, rollback
outcome and remaining retention obligations. Until a rehearsal supplies that
evidence, this remains a plan.
