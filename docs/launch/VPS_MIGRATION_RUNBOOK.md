# VPS migration and text-transition runbook

**Status — 2026-09-12:** planned operator procedure, not an executed migration
or verified recovery drill. The [Blueprint](../../BLUEPRINT.md) owns the target;
the [transition design](../design/DESIGN-text-transition.md) owns compatibility,
data and retirement requirements; the [Roadmap](../planning/ROADMAP.md) orders
implementation. No production data, service, schema, DNS or storage change is
authorized or performed by this documentation update.

The target is one writable PostgreSQL database, Redis coordination and a Go
server loading an immutable certified text bundle. The current stack still
uses MinIO/readiness checks and the older game protocol. A host move and the
text schema/protocol cutover are separate changes: rehearse them separately
before combining them in an operator-approved release window.

## Current gaps that block an execution claim

- The checked-in [snapshot script](../../infra/compose/snapshot.sh) is not a
  safe production backup/restore procedure. Its MinIO export lacks a host
  mount, its Redis wait is unbounded and suppresses errors, and restore removes
  the existing named volumes. Repair it and prove an isolated restore before
  using it for valued data. Never use that restore path as routine rollback.
- Publishing `knowoff:drain_check` in Redis does not query active rooms. No
  verified subscriber/response contract exists. Readiness becoming false or a
  maintenance notice appearing also does not prove all matches and settlements
  have drained. Add and verify an authenticated drain/status surface before
  replacing a process with live matches.
- Current room state lives in process memory. Redis room-to-node records and
  an RDB snapshot do not recover hands, phases, timers or pending trades.
- Current startup reads a local bundle; a configured storage endpoint is not
  an implemented S3 catalog loader. Current Compose uses a development server
  image and has `cloudflared` commented out. Prepare and verify the actual
  production image, mounted text bundle and ingress configuration first.
- Current financial completion applies several separate writes. Prove stable
  match/account settlement identity and retry recovery before reopening live
  rewards on the target. Native purchase verification remains a separate
  launch gate; never use a test receipt flow against real purchases.

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

Use the future verified drain operation to stop new queues/rooms and report
actual running match count. Let in-flight old matches finish on their pinned
old content. Confirm zero pending trades, zero active matches and zero pending
settlements before stopping the old process. If the drain deadline expires,
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

Use the actual schema inventory, then record counts and deterministic row/value
checksums for every table. The current schema contains:

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

There is no current `media_packs` or `admin_audit` table. Inventory new transition
tables from the actual applied migrations when implemented. Preserve receipt
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
failure behavior for actual required dependencies. Text readiness must not
remain tied to an unused MinIO service; its removal is implementation work.
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
| Target has accepted no writes | Freeze/stop target, verify the source remains complete, restore source routing and compatible application. Reconcile any held callbacks before admission. |
| Target has accepted writes; expanded schema remains compatible | Freeze admission and drain; roll back the application/config/pack on the same authoritative database using the rehearsed compatible release. Retain target receipts/results. |
| Target writes exist and the older app/schema is incompatible | Keep maintenance active. Prefer a forward fix; otherwise reconcile/replay target writes into a reviewed recovery database. An older snapshot requires an explicit recovery-point/data-loss decision, never routine DNS reversal. |

Do not replay a settlement without its stable idempotency key. Reverify wallet,
receipt, entitlement, counter and contribution parity before restoring service.
Record failure cause, affected interval, exact artifacts and recovery evidence.

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
