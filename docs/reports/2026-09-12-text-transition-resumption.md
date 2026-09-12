# Five-mode transition: resumption evidence

Owner resumed the full roadmap from committed checkpoint `bd5e13e` on
2026-09-12. The earlier pause is superseded. No deployment exists; tied Weekly
Challenge votes use earliest accepted submission, then immutable entry ID.

This is an active work record, not a release certificate. The complete current
implementation has not yet passed the unified gate. Production modes remain
closed until their release evidence and activation checks pass.

## Verified slices

All PostgreSQL results below used uniquely named disposable real databases.
Run numbers identify `/tmp/agent-runs/` safe-run logs on this workstation.

| Slice | Evidence | Review/status |
|---|---|---|
| Node PWA cache tests included in unified runner; complete TAP accounting rejects missing totals, skips and cancellation | Python20 tests19642; actual Node2 tests19679; independent Python20 tests90919 | Independently approved |
| Migration14 actual upgrade/repeat/down preserves legacy rows and refuses retained development identity deletion; generated name fits public bounds; dev key/config validation | PostgreSQL race23782 | Independently approved |
| Twelve server trace fixtures in production Dart session; terminal timing and reduced-motion behavior | Flutter analysis,412 tests, web build37826 | Independently approved; physical-device evidence remains open |
| Five modes ×4/6 authenticated prototype network sessions and recipient-safe replay | Full module race48854; final full module tests/vet/build80039 | No live value; cross-recipient trace credential check requested during review |
| Real executable text assembly, ownership exclusion/loss/recovery, authenticated HTTP/WebSocket and expired drain | Runtime integration56894 | Prototype evidence; production enablement still requires certified persisted-release proof |
| One shared socket writer for handler replies and room broadcasts, preserving original connection after replacement | Reproduction68947;10× race75738; full handler race80026 | Independently reviewed |
| Immutable user-terms publication and explicit acceptance; private paged block lists, deleted-target unblock and same-match identity lookup | Store34182; HTTP41501; admin publication67417 | Final integration review pending |
| Admin suspension blocks auth/admission/admin actions; expiry preserves independent bans and earned value | PostgreSQL race76264 | Auth portion independently reviewed; broader safety review pending |
| Current Week Winner projection independent of historical win count; current-room identity authorization and immediate peer closure despite cleanup failure | PostgreSQL race96127 | Session-epoch callback wiring and final review pending |
| Guard namespace/overlap/expiry, normalized UGC with exact configured user terms, weekly activation and close | Full portal race73044:70 test/subtest events | Subsequent durable enforcement/retry additions still in progress |

## Continue from here

1. Finish durable session epoch17, preserving device/OAuth identity; wire
   post-commit Guard final-action callback through canonical sanction checking
   and current text-peer closure. Exercise late expiry/unban and retry races.
2. Finish client safety/auth/current-title journeys and independently review the
   complete community/Guard/weekly and safety changes. Update roadmap checkboxes
   only when their full proof is observed.
3. Continue remaining Phase5 identity restoration, deletion, avatars, verified
   billing/doubler, moderation/content revision and value reconciliation work.
   Then complete Phase6 retirement/ops proofs and Phase7 local release evidence.
4. Run `python3 xops/test/tests-lints.py`, review the full diff, record the
   completed candidate and stage for the owner's `make git`. Never commit/push
   from an agent. Human editorial, provider, platform/device and market evidence
   must remain explicitly unexecuted until actually observed.


## Billing correction plan — implementation worker, 2026-09-12

Goal: purchases change durable value only after authenticated platform proof,
with exactly-once grants, restore and refund reconciliation. Parent assigned
`economy/purchases*.go`, `entitlement*.go`, receipt/SSV paths in
`handler/economy*.go`, additive billing config/tests and migration19. Changes to
wallet or text settlement require coordination. Client SDKs, production product
mapping, real purchases and SSV/Premium doubling are outside this first slice.

Observed gaps: Premium currently trusts any client receipt in every environment;
Noin checks only the literal `prod` environment and otherwise accepts arbitrary
positive product suffixes. Provider methods grant without provider checks;
pending receipt replay cannot resume verification. Premium expiry is fabricated
and not tied atomically to the receipt. Legacy transaction IDs lack provider
namespacing, subscription source tracking and provider refund/restore handling.
Theme/style entitlements share a type-only key. SSV currently uses generic HMAC,
trusts supplied amount/account and is not bound to a terminal match or earn cap.

Ordered tests and work:

- [x] Reproduce then seal unverified Premium/Noin grants in local, staging, prod
  and production; malformed/unknown requests cannot change wallet/entitlements.
- [x] Define bounded provider receipt validation and exact app/product/account/
  environment bindings; missing credentials, failed verification and unsupported
  products refuse without value. Fixtures remain explicitly synthetic.
- [x] Add migration19 source/effect identities and multi-value entitlements,
  preserving original receipts/benefits. Verify transaction replay, conflict,
  concurrent grant, uncertain commit, independent subscription expiry and restore.
- [x] Add fixed-origin bounded Google/Apple verification adapters and durable
  retry/acknowledgement/refund state. Crypto/HTTP fixtures cover altered proof,
  provider error, stale notification and retry without repeated ledger effects.
- [ ] Run read-only reviewer followed by disposable PostgreSQL race tests,
  compiler/vet and parent unified gates. Record remaining platform sandbox proof.

Authority references checked 2026-09-12: [Google backend purchase validation](https://developer.android.com/google/play/billing/security),
[Apple App Store Server API](https://developer.apple.com/documentation/appstoreserverapi),
[Apple account binding](https://developer.apple.com/documentation/appstoreservernotifications/appaccounttoken),
[AdMob SSV](https://developers.google.com/admob/android/ssv) and
[UMP consent](https://developers.google.com/admob/flutter/privacy).
Google token identity, authoritative purchased state and account binding precede
grant; Apple signed transaction identity/expiry must replace guessed durations.
No mock counts as Play/StoreKit test-transaction evidence. Product IDs, store
credentials, ad units, platform consent review and spent-currency reversal policy
remain explicit launch inputs; absent inputs keep their path unavailable.


Bounded server acceptance: provider primitives were independently reviewed by
`baseline_runner`; the parent independently reviewed core/worker/schema/receipt
paths, including the database-deadline correction. Cold real-service verification
`/tmp/agent-runs/text-billing-reviewed-full--20260912T164029Z-390696.log` passed
317 test events (economy 41, config 82, handler 92, store 102), with no skips,
plus vet and build. Its only failure was existing formatting in `convert.go` and
`economy_test.go`; whitespace-only correction passed the scoped format gate in
`text-billing-reviewed-format--20260912T164353Z-404453.log`. Each package used a
separate disposable PostgreSQL/Redis instance. The parent unified gate remains
open. No real Play/StoreKit transaction or client billing SDK was exercised.
Google linked-token replacement is explicitly refused. Apple polling rechecks
submitted transaction IDs; renewal discovery still needs a newly submitted
verified transaction or a future notification/status integration. The exact
current API and configuration are in [MODULE-billing](../code/MODULE-billing.md).

### Reward verification continuation — billing worker

Migration 22 is reserved for the doubler; migration 20 belongs to OAuth and
migration 21 now implements subscription source continuity (no version gap).
Parent approved independent AdMob verification work and the following durable
identity/cap design. Premium eligibility timing and interrupted-match handling
remain under specification review before dependent store writes.

- [ ] Replace the retired HMAC acceptance path with AdMob ECDSA P-256/SHA-256
  DER verification of the unchanged signed query. Fetch only the fixed Google
  key endpoint; enforce bounded responses, key count, query length, timeouts,
  no redirects and a key cache of at most 24 hours. Reject duplicate/ambiguous
  fields, altered encoding/order, wrong key/signature and unknown ad-unit mapping.
- [ ] Bind an authenticated post-match opt-in to an opaque server-generated
  claim. Store its hash and exact account/match/ad-unit identity; claim expiry
  limits a token, not the player's economic benefit. A replacement claim never
  permits a second bonus. Client ad callbacks supply no value authority.
- [ ] Use one immutable match/account bonus identity shared by Premium and SSV.
  Base amounts come only from credited award receipts, grouped by their original
  server day. Lock the account then day buckets in order and enforce the pinned
  earn cap for each original day; late callbacks cannot move earnings to today.
- [ ] Keep terminal settlement immutable. Persist the bonus and its separate
  private delivery atomically; no bonus, amount or balance is public or visible
  during an active match. Replay, failure, midnight, absent/prototype and
  cross-account tests must preserve exact ledger/outbox parity.
- [ ] Add configured client consent/ad integration only after the server contract
  and required platform inputs exist. No SDK/test-ad callback, fixture signature
  or generic consent checkbox is evidence of actual UMP/provider readiness.

Cryptographic references: [AdMob SSV](https://developers.google.com/admob/android/ssv)
and Google's linked [Tink verifier](https://github.com/tink-crypto/tink-java-apps/blob/main/rewardedads/src/main/java/com/google/crypto/tink/apps/rewardedads/RewardedAdsVerifier.java).
The documented key URL redirects to `https://www.gstatic.com/admob/reward/verifier-keys.json`;
the adapter will pin that observed HTTPS origin directly and refuse redirects.

## Exact content-report and runtime admission plan

Root will authorize report targets through the live recipient projection without
advancing socket sequence or returning catalog data. The report stores only the
pinned match/mode/language/release/content revision and hash; admin case handling
resolves that immutable release. Tests cover every mode/size, wrong revision,
other roles, outsider/departed seats, begun verdict prompts and unchanged stream.
Baseline owns migration18 and bounded report/admin case integration. Root then
proves actual runtime admission through a persisted certified test release before
removing the obsolete configuration fence; simulated editorial certificates
remain test doubles and do not enable any production mode by default.

### Report/case implementation contract — baseline worker

Ownership: `reports/**`, the reports/feedback section of `handler/api.go`,
`admin/operations.go` and matching tests; additive migration18. The existing
release store gains only a transaction-scoped takedown helper so case resolution,
withdrawal and audit commit together. Other owners retain auth, billing19 and
the runtime's authorized visible-text resolver. Original migrations and legacy
report references remain unchanged.

Acceptance sequence:

- Reproduce unbounded/unknown-field report input and restart-lost rate limits;
  validate bounded UTF-8 and allowlisted feedback context without storing role,
  hand, schedule, seed or arbitrary client diagnostic payloads.
- Authorize text targets from the current recipient projection, then persist
  only immutable match/mode/language/rules/release/content revision and text hash.
  Client-supplied text, another seat's private prompt, wrong revision and unknown
  targets must not create a report or reveal availability details.
- Add durable report cases and duplicate identity with bounded keyset pages;
  repeated reports collapse into their case without discarding historical rows.
  Concurrent/restarted intake and pagination tests verify exact counts and no
  reporter identity on public or Guard views.
- Resolve the exact stored revision in the authenticated internal console.
  Takedown closes the case and withdraws that release atomically through the
  existing product service; failed audit, revoked admin, conflicting replay and
  SQLSTATE retry cannot produce partial resolution or repeat contributor value.
- Review the frozen diff, then run disposable PostgreSQL race tests, handler
  wire/privacy tests and the coordinator's complete unified gate. Synthetic
  cases are test evidence; no live report, publication or moderation is implied.

## Identity restoration implementation — owner lease lane

Session revocation17 now preserves device/OAuth links, moderation state, wallet
and profile rows. Real PostgreSQL race125758 proves legacy epoch-zero parity,
lossy-down refusal, rollback preservation and both refresh/revoke commit orders;
broad auth/game/lobby/handler/runtime race133833 passes. Derived-session auth
and admin issuance race151293 passes; vetted157693. Browser issuance is owned by
the portal lane. Cross-recipient trace credential regression119268 is fixed and
passes ten race repetitions127316 plus vet129662. Frozen attribution was
independently reviewed and its real database race153561 passes.

Next bounded identity scope reserves migration20 for durable OAuth flow state.
Linking requires a validated player JWT and its captured session epoch; explicit
restoration resolves an existing provider subject without merging or creating an
account. Callbacks consume state once and atomically commit provider linkage plus
completion. A separate initiating-app completion secret retrieves one token pair
through a POST body; callback pages never put player credentials in redirect URLs.
Provider exchanges use bounded HTTPS, verified identity claims, sanitized errors,
Google state/nonce/PKCE and Facebook application/user token introspection.

Tests will cover process restart, callback/result replay, both revocation orders,
subject collisions, unchanged wallets/links and simulated provider rejection.
Google's [OIDC reference](https://developers.google.com/identity/openid-connect/reference)
and [live discovery](https://accounts.google.com/.well-known/openid-configuration)
were checked on 2026-09-12. Meta documentation returned HTTP429; its
[official SDK token metadata contract](https://github.com/facebookarchive/php-graph-sdk/blob/5.x/src/Facebook/Authentication/AccessTokenMetadata.php)
and [OAuth client](https://github.com/facebookarchive/php-graph-sdk/blob/5.x/src/Facebook/Authentication/OAuth2Client.php)
are historical primary references, not current provider approval evidence.
Actual Google/Meta console configuration, supported Graph version, approved
redirects and native/browser provider sessions remain unexecuted launch gates.

## Integration acceptance update

- Client:439 Flutter tests,2 Node tests, analyzer/formatter and release web build
  passed143724; independent review approved the final lost-block-response repair.
- Guard/weekly/browser:84 top-level tests,104 passing events, no failures/skips
  in cold serialized real-PostgreSQL race172526. Review found and repaired
  challenge approval after admin suspension and an unbounded legacy projection.
- Session17/auth/admin:real PostgreSQL/Redis race125758 and151293 approved;
  revocation and issuance serialize in both commit orders. Actual runtime/socket
  final moderation149729 passed and independently approved. Guard-only freeze
  leaves current session intact; final ban closes it and invalidates old refresh.
- Frozen contributor attribution retry:RED111728→GREEN114799; independent
  race153561 passed and approved.
- Report reference boundary:ten mode/size cells168480 pass, including no stream
  sequence mutation. Persisted certified test-release admission177198 passes all
  ten actual runtime/WebSocket cells; simulated review evidence is not a release.
- Completed technical Phase4 items1–10 and Phase5 Guard/weekly items have been
  checked. Physical platform, low-end performance, provider and human release
  observations remain open and cannot be inferred from these automated tests.

### Planned active-runtime retirement slice

Replace the executable's v1 media/backfill/workbench wiring with its proven text
runtime. Separate reversible maintenance admission pauses from dependency health
and permanent owner-loss/shutdown draining. Readiness must use the runtime and
PostgreSQL/Redis, with no gameplay image pack or object-store requirement. Preserve
begun matches during reversible pauses; reject obsolete gameplay routes before
identity, admission or quota mutations. Tests first cover pause interleavings,
readiness without images, explicit stale-client rejection, and actual executable
assembly. Historical source/SQL/test fixtures remain inventoried until their own
retirement proofs and any required test-deletion decision; this slice alone does
not claim Phase 6 complete.

### Visible-text reporting controls — baseline worker

Add an inline confirmation beside each currently rendered Nown/card in the
private prompt, hand, public board/history and verdict. The report carries only
the current match ID, immutable content ID/revision and a selected reason. It
does not copy words, role state, the hand or account identity into its payload.
Use existing Ko controls and localized labels; reporting remains available
without accepting authored-content terms. Server visibility and immutable
release membership remain authoritative at submission.

Tests first exercise exact HTTP payloads, duplicate/uncertain-response retries,
generic refusal, visible-only controls and disposal on elimination, background,
resync or match replacement. An inline form owns no authored text and cannot
outlive its rendered source; asynchronous completion checks its mounted state.
Review the frozen delta before the cold client analyzer/test/build gate. These
automated fixtures do not claim physical device or human content approval.

Server report/case review approved the implementation before cold disposable
PostgreSQL/Redis race301617:130 top-level tests and222 passing parent/subtest
events across reports/admin/store/handler, zero failures/skips. Exact case
resolution, first withdrawal and its audit commit together; historical report
targets remain historical. The client visible-control regression failed323999
before implementation; integrated67 tests pass349893 and scoped analyzer352721
reports no issues. That integration also exposed and fixed missing synchronous
UI notification when resync clears private state. Client source review and the
combined final gate remain coordinator-owned at this checkpoint.

### Integration review and follow-up — 16:10 UTC

Exact persisted-content reporting now passes through real HTTP and all ten actual
runtime cells (280402: cmd race 13.136 s): own cards accepted, unseen but existing
Nowns/wrong revisions/outsiders have one generic refusal, withdrawn pinned retry
stores once, with no quota/value effects. Report18 source independently reviewed.
Readiness/route tests212154→218558 and runtime238617 passed their scoped packages;
238617 handler vet encountered a concurrent file write, subsequently handler
264038 passed. Independent review found future maintenance announcement pausing
admission too early and missing v2 live notices. Start/end timing fixed with real
PostgreSQL boundary and failure tests288954→292378; live notice invalidation and
reminder delivery are the current root follow-up. Existing applied SQL is retained.
OAuth20 review found shared proxy-address initiation budget; worker has repaired
account/client separation with explicit trusted-hop handling and tests290411.

### Root integration checkpoint — 16:31 UTC

Maintenance timing and v2 public invalidation now pass PostgreSQL race340356;
strict client signal341071 and notice recovery/TextPlay360087 pass. Each notice
reminder stage has an independent dismiss key; foreground polling recovers a
failed fetch without another server mutation. No private match state enters the
invalidation. Review also exposed a true resync UI gap, now synchronously clearing
source-bound reporting widgets. Client report source independently approved;
server report/case cold301617 passed222 events, zero failures/skips.

Wallet privacy adds a manager-serialized, bounded database-only operation around
balance/daily-earned reads and Noin spending, so a live correct-vote grant cannot
be inferred through a second device or insufficient-balance responses. Waiting
lobbies remain usable; begun seats remain fenced through leave/disconnect until
verdict. Tests352413RED to360981GREEN; exact executable HTTP/recovery proof and
independent review remain next. Profile responses contain no wallet fields and
career counters update at terminal settlement. This is not yet a final gate.

OAuth20 server/proxy-budget source approved with real HTTP and migration proofs;
client includes explicit account-switch confirmation and is finishing lifecycle
checks. Home public restore entry and cached-page/session reset are under test.
Billing provider primitives passed independent review; core/worker/migration19
is next. Required owner retention decision remains unanswered. Backup/restore
planning is active; no existing destructive snapshot script has been run.


## Phase 6 backup and restore execution plan — 2026-09-12

Planning scope only at this checkpoint. The owner states that no deployment
exists; no live database, object store or old restore command was accessed.
The following is the proposed local rehearsal implementation, not evidence of
successful backup, restore, cutover or a production rollback window.

Required reading: Blueprint 📦 and 🌐; ROADMAP Phase 6 execution 1a–4b;
ADR-012; transition-design migration/retirement contract; corrected
[VPS runbook](../launch/VPS_MIGRATION_RUNBOOK.md). Existing file discovery found
one snapshot implementation, `infra/compose/snapshot.sh`, and no separate
backup/restore test suite. Reuse that entry point. `infra/README.md` still tells
operators to run the unsafe replacement restore; correct that instruction in
the implementation slice.

### Observed storage and writer boundaries

- Compose is named `knowoff`, with PostgreSQL 16, Redis 7 and MinIO, ordinary
  `pg_data`, `redis_data`, `minio_data` volumes, and core/tools/test/edge
  profiles. Server uses `Dockerfile.dev`; migrate and seed-admin use the
  production Dockerfile. Server/migrate mount configs, `content/packs` and
  `content/ingest`; nginx retains the MinIO console proxy. Local client-web
  and cloudflared services are commented out. Current snapshot assumes the
  default project/network names rather than verifying resource identity.
- There are twenty up/down migration pairs, creating 64 distinct tables in
  their SQL source. This is a source inventory, not an observed database
  count. Retain migrations 1–20 and discover the actual restored catalog;
  version 19 billing and 20 OAuth are still undergoing their own review.
  Beyond the old runbook list, retain admissions/contracts, award receipts,
  settlements/outbox, leaderboard daily counts, archive/release lineage,
  trust/terms, process ownership, Guard decisions, the current challenge
  crown, report cases/rate identities, named entitlements, provider source/
  retry records and OAuth/session-epoch state. Do not filter unknown tables.
- `custom_avatars.blob`, portal/challenge `asset_blob` and text legacy
  `source_row` are retained inside PostgreSQL. The archive JSON includes the
  original row and its bytes/reference; it does not prove external referenced
  bytes were exported. Text release bundles, certification artifacts and
  provenance are persisted in `text_releases.bundle` JSONB. Legacy pack files,
  ingest inputs, asset references and any existing bucket keys still require
  an explicit path/key/size/SHA256 relationship inventory. No active Go S3
  client was found in the executable assembly; this alone does not authorize
  deleting MinIO data, credentials, mounts or historical pack artifacts.
- `ReadTransitionPreflight` already offers bounded read-only table row hashes
  and migration/column/constraint/index evidence. Its schema hash omits
  functions, triggers, policies/RLS, roles/ACLs, sequence state and non-public
  schemas. Reuse its aggregate evidence but extend the restore inventory
  before calling it full-schema parity. Preserve database-resident avatars
  and all immutable trigger definitions, not merely account/ledger sums.
- Admission drain alone leaves other writers: auth/refresh/OAuth, profile and
  avatar writes, portal/browser/Guard/challenge actions, reports/terms/blocks,
  billing/ad callbacks and retries, admin publication/withdrawal/notices,
  community maintenance, leaderboard closure, delivery claims/acks and text
  owner recovery. Current shutdown interrupts unfinished matches; it is not
  a wait-for-natural-completion cutover status API. PostgreSQL advisory locks
  and Redis routing cannot reconstruct private hands, timers or pending trades.

### Unsafe behavior to replace

Current create evaluates shell environment files, exposes passwords in child
arguments, overwrites destination files, suppresses Redis errors and can wait
forever. It does not prove a newly successful RDB save. The MinIO export lacks a
host mount, uses an unpinned client and even creates a source bucket while
backing up. No checksummed completion manifest, role export or restore parity
exists. Current restore stops the ordinary project, removes its known volumes,
uses `pg_restore --clean`, overwrites object keys and restarts all services.
Missing readiness can fall through after its loop. These paths must be replaced,
not invoked as a preliminary test.

### Proposed implementation ownership and contract

Baseline runner owns the repaired `infra/compose/snapshot.sh` entry point, a
small sibling `snapshot.py` for structured subprocess/path/manifest handling,
`xops/test/test_snapshot.py` and its disposable integration driver, plus the
snapshot instructions in `infra/README.md`. Discover each proposed new path
again immediately before creation. Parent owns runtime/admin drain and writer
coordination, main wiring and the full preflight extension, or explicitly hands
off that bounded extension. The existing unified runner should discover the
Python tests and run the real restore fixture explicitly with skip accounting.
No alternate general-purpose backup framework or host package installation.

1. Add `create`, `verify`, `restore` and read-only `--dry-run` behavior behind
   the existing entry point. Explicit source identity, backup location and
   mode are mandatory; no implicit default Compose target. Initial execution
   is limited to local, project-owned disposable services with validated
   resource labels and a random nonce. Reject remote endpoints, source/target
   identity equality, ambiguous labels, symlinks, traversal, occupied output
   directories and unsupported manifest versions. Dry-run validates and
   prints only a secret-free operation inventory; it starts/stops nothing.
2. Create a new 0700 directory exclusively; regular artifact files are 0600.
   Do not source shell files or interpolate a command string. Pass credentials
   through protected temporary client config/stdin or container environment,
   never command output or the manifest. Use finite per-command and overall
   deadlines, kill and reap owned child processes on timeout/cancellation,
   inspect each exit code and retain a clear incomplete receipt on failure.
   Publish the final manifest atomically only after every artifact is flushed,
   sized and hashed. No partial backup is restorable. Retention/deletion of
   real backups remains subject to the owner's retention policy; permission
   bits are not encryption. Production export requires an approved encrypted
   destination/key-recovery arrangement; do not invent cryptography or copy
   live secrets into a synthetic rehearsal artifact.
3. Capture PostgreSQL with the same version 16 toolchain, full custom dump and required
   role attributes/memberships/ownership/default privileges. Role passwords
   come from separately managed secret references, not report output. Include
   all user schemas, functions, triggers, policies, extensions, sequences
   including `last_value`/`is_called`, constraints and validated state, indexes,
   large objects and every table's deterministic row hash. Use a shared
   exported snapshot for dump and row evidence where supported; separately
   freeze sequence/DDL/role writers and verify unchanged before/after state.
   One transaction snapshot alone does not make external objects or sequence
   counters consistent. Missing catalog privileges/RLS completeness is an
   error. Include exact migration-file checksums and tool/image digest IDs.
4. Preserve required external bytes through explicitly mounted directories and
   an immutable object inventory. Pin a tested object client image; verify
   actual source object metadata and downloaded bytes, including zero-byte
   objects and nested keys. Never create or rewrite source buckets. Missing
   required assets is a failure. Unknown/versioned bucket layouts require a
   complete versions/metadata export or explicit refusal, not latest-only
   claims. Preserve local immutable packs/config templates and references to
   secret versions and app/client artifacts. Redis capture must prove a
   successful bounded save and unchanged frozen source; label its RDB as
   security/routing data, never live-game recovery evidence.
5. Restore allocates its own fresh randomly named container/network/volume
   set. It accepts no ordinary Compose volume or arbitrary writable database
   target. Check the nonce/resource labels and database identity, then prove
   absence of user schemas/tables, external files/keys and RDB content before
   any load. Verify the entire manifest and every file first. Restore roles
   before schema/data/ACLs using checked, bounded commands; do not use clean,
   overwrite, source volume removal or auto-start app/admin/migration jobs.
   Revalidate destination identity at each mutation. A failed destination
   remains isolated/unready; clean up only resources created by this operation.
6. Compare complete catalog definitions, role/ACL state, all table counts and
   hashes, sequence values, FK/unique constraints, blob bytes and object
   inventories before declaring success. Exercise retained profile/avatar,
   receipt/entitlement, historical report and archived-content resolution
   against the restored copy. The report contains aggregates/digests, not row
   values, credentials, private prompts or raw receipts. A valid dump header
   and a successful database connection are insufficient.

### Quiescence and rollback dependency

Parent's authenticated drain must first reject new admissions and report actual
active matches, pending trades, prepared/reserved work, unfinished settlements
and retry work under the same manager ownership fence. Let existing matches
finish under their pinned rules/content or perform an explicit interruption;
a timeout returns remaining counts and keeps admission closed. A committed
settlement with an unacknowledged outbox message is retained delivery work, not
an unpaid settlement; these must have separate counters. Offline players cannot
hold a successful backup hostage by never acknowledging an already committed
receipt.

After gameplay quiescence, stop/fence every listed writer and wait for in-flight
operations, persist a cutover watermark, then capture database/objects while
that fence remains held. New external callbacks must be rejected for retry or
queued durably to the sole authority. Verify source/target cannot both acquire
admission or mutate value. For this first local rehearsal, seed fixtures before
capture and keep all app/writer processes stopped; do not claim this substitutes
for the later authenticated live writer barrier. A mere maintenance flag,
Redis publish, empty `pg_stat_activity` instant or a command-line assertion is
not proof of ongoing exclusion. Snapshot must fail closed if its required
quiescence evidence expires or changes.

Rehearse 8→head on a restored copy, crash/resume bounded archive work and snapshot
that new state separately. Prove allowed empty-only down refusals with retained
new data, but never call all-migrations-down. Before target writes, a frozen
source can remain the rollback authority. After target writes, test a compatible
application/config rollback on the same expanded database, or an explicit
forward fix that preserves the captured new purchase/settlement. Restoring an
older snapshot over newer value is excluded. Restore never opens admission;
parent's recovery/validation and single-writer handoff must succeed first.

### Ordered tests and acceptance gates

- [ ] Write command/path/manifest regressions first: arbitrary/occupied target,
  source-equals-target, symlink/traversal, missing/truncated/tampered artifact,
  role export failure, unhealthy service, timeout, cancellation and disk/write
  failure. Assert no destructive/overwrite/full-stack-start command can run;
  failed creation has no valid completion marker; dry-run makes no mutation.
- [ ] Implement bounded safe create/verify behavior, then review that slice.
  Test a stale/failed Redis save and a missing object mount as real failures;
  test required object absence rather than silently skipping MinIO coverage.
- [ ] Implement isolated restore and cold disposable PostgreSQL 16 proof from
  realistic legacy 8 and current-head fixtures. Seed avatars/legacy image bytes,
  receipts, roles/ACLs, nondefault sequences and full retained relationships.
  Compare all catalog/data/blob evidence and exercise mutation guards after
  restore. Include a function/trigger change that old preflight would miss.
- [ ] Add external-object and Redis fixtures with real containers. Check nested
  and empty objects, bytes/metadata, tamper refusal and exact target emptiness.
  Restore twice into distinct targets; occupied repeat refuses without changing
  either source or the first verified target. Test interruption at each step.
- [ ] After parent writer barrier lands, hold an in-flight settlement and
  external writer, expire drain timeout, reject stale quiescence evidence,
  attempt dual-writer admission and retry a held callback exactly once.
- [ ] Independently review before final cold verifier. Run the unified gate with
  exact counts/skips and save manifests/logs under private `/tmp/agent-runs/`
  paths. Update runbook commands only to the verified interface; record artifact
  versions, before/after parity and failures. No live migration/retirement or
  human-enabled-mode smoke is inferred from these isolated rehearsals.

Report-client follow-up: independently approved source then analyzer plus 67
focused tests passed 370651. Selected report reason now exposes semantics and
keeps its exact reason across uncertain retries. The wider client gate caught
missing translated report metadata and formatting; the repairs passed all nine
metadata/interpolation tests and formatting check372693. The coordinator owns
the final combined client and repository gate.


## Identity restoration and owner cancellation verification — owner lease lane

The typed Google/Facebook OAuth flow is implemented with separate authenticated
link and explicit restoration intents. Migration 20 retains bounded, single-use
state and private completion proof, initiating session epoch/JTI, immutable
issuance receipt and replay/config binding. Provider callbacks validate the
provider identity before the account lock and never return player tokens in
browser URLs. An existing provider owner produces an explicit conflict; no
account or wallet merge is performed. Restoration can recover an existing
account from a second device, including when that device already has an anonymous
account. Provider credentials remain disabled by default. Trusted proxy ranges
are explicit and empty by default; link budgets use verified account identity,
while restore budgets use a validated client address. The coordinator reviewed
and approved this server lane after the real HTTP and migration proof.

Server evidence: auth/handler/config full race gate 290411 passed (3.093s,
18.032s and 2.142s respectively); real loopback auth routes and actual migration
19→20 upgrade, exact empty down and retained-receipt refusal passed 304652
(auth 1.510s, store 3.438s). Final server OAuth vet 312261 passed. Official Google
provider metadata/JWKS validation and fixed Facebook Graph endpoints are exercised
through controlled HTTP provider fixtures. Actual Google/Meta app approval,
credentials, provider devices and store identities remain unexecuted evidence;
these tests do not open a production release gate.

The client now provides localized Link/Restore controls, external browser launch,
private bounded local completion proof, resume/poll/cancel, explicit confirmation
before switching a different restored account, and public help independent of a
working saved session. Response URLs and token/result shape are bounded and
validated before use. Cancellation and local generations reject stale network or
storage completions. Device/refresh calls are bounded and cannot block public
restoration; obsolete requests cannot overwrite the restored identity. Home and
Profile invalidate retained account views after a successful switch, including
system/header Back and Back during an accepted storage commit. Widget disposal
cannot cancel a newer page's flow. Four locales, larger text, background/resume,
late responses, unavailable providers, expired credentials and account collisions
are covered.

Client evidence before the final combined gate: initial service RED320735→GREEN
330388; initial screen RED337680→GREEN344955. Lifecycle bugs were reproduced by
361698 and 364166, then fixed with 363238 and 367352 (58 combined tests). Review
regressions 399944/403992 reproduced Back-result loss, stalled old refresh and old
cleanup cancelling a newer flow; 404253 passed 88 related tests. The additional
commit-exit race was RED404750→GREEN404965 (39 auth/OAuth tests). The initial
combined gate 370954 reported 473 passing/10 failing tests: translated metadata,
notice request timers after widget disposal, and two new missing-brace lints were
diagnosed separately. The responsible lanes repaired these; the next combined
gate must be green before handoff. Frozen reviewed client hashes are recorded in
`/tmp/agent-runs/oauth-client-frozen-20260912.json`.

The wallet privacy review found that an HTTP client cancelling its request could
cancel a query on the process-wide pinned PostgreSQL owner connection, causing
all live peers to lose authority. Owner Check now uses an independent bounded
server probe and reports caller cancellation without discarding healthy physical
authority. The manager respects that distinction, while actual probe/connection
failure still closes authority and permits confirmed-loss recovery. The watch
uses the probe's own deadline. Exact PostgreSQL regressions reproduced pre-cancel
and in-flight loss in 373541, with manager loss reproduced in 375664. The final
owner race suite passed 384769 (7.229s); the full lobby race suite passed 384761
(8.065s), each using an independent disposable PostgreSQL/Redis instance. Owner
and lobby vet 400182 passed. A prior combined selected-package run 380011 left
retained ownership rows for a later unbound fixture; independent disposable gates
resolved that test-state collision without weakening production fences or physical
loss assertions. Frozen hashes are recorded in
`/tmp/agent-runs/owner-cancellation-frozen-20260912.json`.

## Phase 6 runtime drain implementation plan — 2026-09-12

Root owns new lobby/store `text_drain` source/tests, new admin `runtime_ops`
source/tests and the narrow registration/main wiring. The first boundary closes
admission permanently for the current process, releases queued/unstarted
reservations with retryable cleanup, and lets begun matches finish naturally.
Authenticated admin status returns aggregate waiting/active/trade/abort counts
and actual persisted prepared/started/reserved/pending-settlement/outbox counts.
Status reads do not advance game clocks, recipient sequences or ledger state.
A bounded wait reports timeout plus remaining work and never interrupts a match.
Committed but unacknowledged private outbox messages are delivery backlog, not
unapplied currency; report them separately. A zero-match result must not claim
that account/admin/provider/community writers are paused or that backup is safe.
The subsequent writer barrier/snapshot integration remains a separate gate.

Admin drain requires a live session, role and CSRF recheck plus a durable audited
request before changing the in-memory admission flag; failed authorization or
audit cannot close admission. Audit identifies the process owner and request,
not hidden match/account content. Tests first reproduce missing behavior, then
cover rejection, actual waiting/active work, retry after cleanup error, read-only
status, bounded timeout, natural completion and retained settlement delivery.
Independent review and disposable PostgreSQL/Redis race tests close this slice.

Root fixes now reviewed: no placeholder contribution agreement is manufactured
at startup (RED394346 → GREEN400261, portal PostgreSQL race suite); published
nonempty terms remain required for contribution intake. Wallet privacy's actual
HTTP/restart test388952 passes all five modes at both sizes and preserves the
pinned five-Noin instant award through confirmed interruption. Full client gate
405174 passes489 Flutter tests and2 Node checks, analysis and formatting after
notice timer disposal and OAuth lifecycle fixes. Provider/device evidence and
owner retention/doubler decisions remain outstanding.

Owner-lane final client verification: the complete official client runner
`oauth-client-reviewed-official-final--20260912T164620Z-405174` passed 489
Flutter tests, two Node browser-cache tests, analysis and formatting with no
failed or skipped checks. Release web compilation
`oauth-client-release-web-final--20260912T164721Z-408090` passed. The build
reported the existing unused Cupertino icon-family warning; the artifact was
produced successfully. Root independently approved the OAuth lifecycle repairs.
Actual Google/Meta provider approval and device sign-in remain unexecuted.

### Admin actor authorization repair plan — owner lease lane

The five scoped portal admin methods must authorize in the same transaction as
their mutation and audit. Initial browser middleware validation alone does not
survive logout, session-epoch revocation, suspension, ban or admin-role removal
before a write. CreateTermsVersion and RejectApplication currently omit the
transactional actor check; GrantRole/RevokeRole lock the account but not the
admin-role row or the initiating session. The actual browser grant route uses
ApproveApplication, which also lacked actor authorization and took application
locks before accounts; it is included in this same bounded repair. The repair binds the browser's exact
session and CSRF proof through a context-carried transaction authorizer. Sorted
actor/target account locks precede admin-role and exact-session locks, and clock
checks run after lock waits. Trusted internal calls still require a canonical
active admin. Tests cover stale middleware, no unauthorized effect/audit, valid
controls, actual row-lock concurrency and expiry after a waiting transaction
starts. Root approved this narrow scope; no legal wording or consent is generated.


## Subscription lifecycle continuation plan — billing worker, 2026-09-12

**Goal.** Discover Apple renewals for a known subscription and accept verified
Google plan replacements without duplicate access, account transfer or loss of
retained receipts. The coordinator approved this boundary; schema and provider
adapters, current source projection and workers are independently reviewed and
verified. Native purchase/restore integration is the next separate boundary.

**Original gaps and provider evidence.** `AppleReceiptVerifier.Verify` reads only
`Get Transaction Info` for the submitted transaction. Apple provides
`GET /inApps/v1/subscriptions/{anyTransactionId}` for current subscription status;
its latest entries contain separately signed transaction and renewal data.
Poll a known original subscription without a status filter, so expiry/revocation
cannot disappear from the response. Select its exact original ID, not another
subscription returned for the same store customer. The response's application,
bundle and environment also require validation. These are established provider
primitives, not inferred renewal dates.
[Apple endpoint](https://developer.apple.com/documentation/appstoreserverapi/get-all-subscription-statuses),
[latest entries](https://developer.apple.com/documentation/appstoreserverapi/lasttransactionsitem),
[response identity](https://apple.github.io/app-store-server-library-python/appstoreserverlibrary.models.StatusResponse.html).

Apple status 1 is active, 2 expired, 3 billing retry, 4 billing grace and 5
revoked. Active access ends at the signed transaction expiry; grace uses the
separately signed grace expiry. Retry alone grants no extension. Cancellation
of auto-renewal does not revoke an unexpired entitlement. `autoRenewProductId`
describes the next period, not currently purchased access. Validate both JWS
chains/OCSP and cross-bind original ID, environment and product; a renewal
account token, if present, must agree with the transaction's mandatory account.
[Status values](https://apple.github.io/app-store-server-library-node/enums/Status.html),
[renewal fields](https://apple.github.io/app-store-server-library-node/interfaces/JWSRenewalInfoDecodedPayload.html).

The original Google adapter refused every linked token and any response with two line
items. Modern deferred replacement transfers the remaining old entitlement to
the new token immediately; one line has the current expiry and another future
item initially has no expiry. Later that same token represents the new product.
Therefore simply allowing `linkedPurchaseToken` is insufficient: the existing
immutable token/product row would reject the later transition. Retire the old
token's access while deriving current access from the new response; acknowledge
the successfully purchased replacement through durable work. Pending/canceled
replacement does not retire a still-valid predecessor.
[Deferred replacement](https://developer.android.com/google/play/billing/subscriptions),
[item/resource fields](https://developers.google.com/android-publisher/api-ref/rest/v3/purchases.subscriptionsv2),
[old-token access retirement](https://developer.android.com/google/play/billing/security).

**Design.** Add a typed subscription observation alongside consumable proof:
provider source (Apple original ID / Google token), immutable account/app/env,
verified transaction/item identities, provider state, current product and exact
access expiry, optional replacement predecessor, transaction/renewal signed
times, observation time and canonical evidence hash. Keep the external receipt
request unchanged. Verify its referenced transaction/product first; a subsequent
server-discovered configured Premium renewal may change product. Never map a
future item to access or invent purchase/start/expiry timestamps.

Coordinator-reserved migration21 (`000021_subscription_sources`) retains a subscription
source identity, immutable observations and an atomically updated current
projection, plus immutable Google predecessor→successor edges. Migration19 and
its receipt bytes remain untouched; migration22 adds source acknowledgement tasks.
The coordinator reserves migration23 for avatar retirement; doubler work may use
migration24 or later after its owner policy decision.
Existing subscription rows receive a provenance-marked projection preserving
current access exactly, with fresh provider revalidation queued; migration does
not claim new verification, grant currency or fabricate renewal observations.
Premium aggregation uses current non-superseded source projections plus retained
legacy benefits, rather than every historical transaction. Product changes
recompute both monthly/yearly projections in the same transaction.

For the first Google replacement boundary, require the predecessor to be an
already verified, retained source for the same game account; an unknown source
stays explicitly refused until restored/verified. Preserve the stricter current
account UUID binding on the new provider response. Lock all involved source
keys in stable order, then the value account, then receipt/projection rows.
Under the account lock, refuse self-links, cycles, conflicting successors,
cross-account/app/env links and stale observations. A retired token can never
regain access through an old receipt or delayed worker. Do not delete its audit
history. Restrict this slice to configured auto-renewing monthly/yearly Premium;
prepaid, add-on and ambiguous multi-item schemes remain refused. Accept only a
single current item or the documented two-item deferred pair, validating every
item and its replacement relationship.

Poll once per known Apple source, not once per historical renewal. Persist fair
bounded source work and its checkpoint atomically with observations/projection;
a restart retries the same identity. Separate immutable historical transaction
status from current source authority, so refunding an old Apple period cannot
revoke a newer valid period and an old future expiry cannot override current
revocation. Each database/provider operation retains configured timeout,
response-size and concurrency limits; add explicit bounded status-entry/item
limits. An absent/ambiguous provider match reports unavailable proof and never
claims a successful renewal. Disabled credentials perform no startup network.

**Touched files.** Existing `server/internal/economy/purchases_{apple,google,
verified,worker}*.go`, focused new subscription source/tests after discovery,
`server/internal/config/billing.go` and tests/base config for engineering bounds,
migrations21–22 and real migration tests, narrow receipt
handler tests, and `docs/code/MODULE-billing.md`. No client SDK, wallet amount,
match policy, default platform activation or provider notification route is part
of this boundary.

**Ordered tests and checklist.** Each row starts with a meaningful failing test,
then implementation, review and its scoped gate before proceeding.

- [x] Add schema/current-projection identity and upgrade proof: migration19/head
  fixture preserves every receipt/entitlement/ledger hash; empty-only down and
  retained-data refusal, immutable edges/observations and required indexes.
- [x] Apple adapter: old ID discovers new transaction; exact source/account/app/
  product bindings; active/grace/retry/expired/revoked; future renewal product
  grants nothing; missing/duplicate sources, bad renewal JWS/OCSP, stale signed
  dates, malformed/oversized entries and unavailable provider fail closed.
- [x] Apple durable lifecycle: renewal exactly once across restart/concurrency,
  stale old receipt cannot roll back current access, old-period refund cannot
  cancel later coverage, current revocation removes only that source, and other
  store/legacy Premium benefits remain intact.
- [x] Google adapter: immediate monthly↔yearly replacement and the documented
  deferred two-item transition; pending cancellation preserves predecessor;
  unknown/mismatched prior account, prepaid/add-ons, invalid pair and missing
  expiry for the claimed current item refuse without acknowledgement or access.
- [x] Google durability: same edge replay, conflicting successor/cycle, concurrent
  old/new restore, failed transaction/uncertain commit and acknowledgement retry;
  predecessor access never revives and no currency/second Premium term appears.
- [x] Worker/HTTP regression: fair per-source polling, bounded stalled SQL/network,
  disabled startup, unchanged private response/no balance, and known-source
  restore after process restart. Independent review then cold isolated economy/
  config/handler/store tests, vet/build/format close implementation acceptance.

**Implementation evidence so far.** SQL21 and SQL22 are independently approved
and frozen; the latter imports exact existing acknowledgement work and refuses
invalid retained payloads before any partial DDL. Its up/down hashes are
`aa0103050f42b39c7d31b84dab940dd6ed8313737f0a9bf07708594a086afce0` /
`dc760aab4184f40bed015070475d59682252884277c491d26324f7f277bb2f4d`.
Apple optional signed group/chronology regression `927141`→`934098` and Google
deferred/current-item gate `877903` passed independent source review. Signed
group, purchase, expiry and signature fields follow Apple's
[transaction schema](https://apple.github.io/app-store-server-library-node/interfaces/JWSTransactionDecodedPayload.html).

The complete economy race gate `text-subscription-full-economy--20260912T174422Z-969994`
passed 115 test events without failures/skips, including a real PostgreSQL test
using the synthetic signed Apple adapter for old-anchor discovery, renewal,
old-period refund and current-source revocation. Coordinator review then found
acknowledgement starvation after a provider timeout. Regression `999216`
reproduced missing retry metadata and healthy-task starvation in both source and
consumable queues; `1001474` reproduced disabled-provider selection. Separate
bounded query/provider/write contexts, filtering before selection, safe aggregate
failures and parent cancellation corrected them. Scoped race gate
`text-subscription-ack-starvation-green--20260912T175233Z-1005938` passed 10 events,
including the prior stalled-SQL and concurrent grant/acknowledgement proofs.
Final independent review also reproduced disabled-platform polling and collapsed
Google item rollover in `1021040`. A known-source-only discovery path retains
strict client product matching, rechecks the exact existing source identity
inside the value transaction, and atomically updates current access and pending
acknowledgement product. Unknown product/account/app/environment never changes
access; unknown/mismatched source cannot create an identity or acknowledge.
`1023272` passed 28 race events with the previous Google negative cases; the
batch-one mixed Google/Apple proof confirms filtering before selection.
Google documents two items at the first deferred renewal, but does not promise
historical item retention forever; accepting a validated current-only known-source
response is a conservative lifecycle boundary, not a claim about observed live
provider behavior. [Deferred lifecycle](https://developer.android.com/google/play/billing/subscriptions).
Final current-source/worker review is approved. Independent cold race gate
`billing-source-independent-accepted--20260912T180844Z-1063711` passed 29 top-level
tests / 43 events, zero failures/skips. The first combined run exposed a synthetic
Apple fixture reusing an account ID and then a unique nickname; each fixture now
has its own UUID and parameterized nickname, retaining every signed binding and
assertion. Exact two-case gate `1055349` and complete economy rerun
`text-subscription-reviewed-economy-final-20260912T180841Z-1062589` passed
132 events, zero failures/skips. Retained passing isolated-package stages from
`text-subscription-reviewed-full--20260912T180434Z-1033115` are config101,
handler96 and store108, plus vet/build/format. Together the final passing package
runs contain 437 test events; each package used its own disposable PostgreSQL and
Redis. The intermediate combined run was not itself green and is not represented
as such. Later avatar-only entitlement/handler edits have separate owner gates.

**Remaining evidence and risks.** A Google new token cannot be discovered by
polling its predecessor: live client purchase/restore or authenticated RTDN
intake must deliver it. After-expiry out-of-app re-signup uses a new token and
separate expired-account context; it is distinct from linked replacement and
remains a later bounded intake task. Google documents token lookup validity
through 60 days after expiry; failures must not cause value mutation or endless
head-of-queue starvation.
[Google lifecycle](https://developer.android.com/google/play/billing/lifecycle/subscriptions).
Actual configured store products, subscription groups, sandbox purchases,
renewal/refund devices and notification identities remain platform evidence,
not simulated success. No new owner economy policy is needed for provider-exact
expiry/replacement; Premium doubler timing, interrupted bonuses and spent-Noin
refund remedy remain separate unresolved owner decisions.

Admin actor implementation evidence: actual stale-request RED483172 reproduced
unauthorized terms/rejection writes and role writes after session loss; the
actual browser approval path reproduced the same error in RED502650. All five
methods now lock the canonical actor/target account rows, admin role and exact
browser session before their mutation/audit. The reusable AuthorizeSessionTx
also protects the root-owned runtime-drain audit. Application approval/rejection
use account-first lock order and recheck the exact target. Full PostgreSQL race
suite571820 passed admin (8.516s) and portal (4.947s), including 40 stale-state
cases, six real authorization/revocation commit orders and session expiry after
transaction start. Vet574140 passed. Intermediate541366/568242 compile failures
were a missing errors import, corrected before that full gate. Frozen file hashes:
`/tmp/agent-runs/admin-actor-authority-frozen-20260912.json` (manifest SHA256
`7b0a52d211c2a5679cb8eca044858d13efd354ba91a36711a5bc14f8394272f0`).

A final direct Availability caller bypassed the earlier cancellation guard;
RED442584 reproduced it with a real PostgreSQL owner and active match. The
request now closes only its own discovery result on cancellation while healthy
authority remains live; actual lost authority still closes all peers. The full
lobby race suite448260 passed (7.802s), and root approved the exact repair.


Subscription schema first boundary is implemented in migration21, with no
provider/runtime cutover yet. RED576000 proved the absent source schema;
RED580287 reproduced a receipt-owner binding hole before its SQL guard. Scoped
PostgreSQL race verification582475 passed six tests with no failures/skips:
exact retained receipt/access/wallet/ledger parity, preserved per-source monthly
and yearly coverage, immutable imports/observations/edges, source-scoped
monotonic projection, concurrent cycle refusal, required indexes and empty-only
down. The migration19 test first removes empty21 inside its rollback-only
fixture; migrations1–20 are unchanged. Coordinator schema review precedes
provider integration.

The standalone AdMob primitive completed independent review followed by cold
verification524586: twelve test events passed, zero failed/skipped, with scoped
economy/config vet and formatting green. Claims, authenticated SSV acceptance,
Premium timing, durable bonus effects and client consent remain unimplemented
parts of their separate checklist; signature validity alone grants nothing.

Root runtime drain passed independent source review and actual persisted ten-cell
HTTP/WebSocket/PG proof586953 (10.645s, race): five modes at both table sizes,
idempotent audited drain, bounded timeout preserving all live matches, and
replacement-owner rejection. Compilation-only584985 selected no tests and is
not counted; missing-import574530 was corrected before the actual proof.

Next narrow store repair: priced categories currently accept arbitrary client
item names while the client correctly disables absent named catalogs. Add an
authenticated negative purchase regression proving no wallet/ledger/entitlement
change, then refuse named purchases until a reviewed selectable catalog exists.
Existing owned benefits remain valid. A later full catalog/release selection
implementation must authorize the exact item atomically with debit; this repair
does not certify that remaining feature.

Runtime/store follow-up606277 passed PostgreSQL race checks: strict drain body
bounds/duplicates/unknown fields, status lock deadline, natural finish with
cleanup retry, durable pending/missing work and unacknowledged output distinction.
Named-catalog RED590802 spent 1900 Noin on two nonexistent items; the public
handler now refuses those unavailable categories and GREEN606277 preserves
wallet/ledger/entitlements while existing avatar purchase/privacy tests pass.
The complete selectable catalog remains open; owned item grants are unchanged.

### Remaining admin mutation authority — inventory and proposed plan

**Goal.** An admin request that loses its initiating session, CSRF binding,
account status or required role before its mutation is authorized must produce
no durable effect, audit claiming success, contribution reward or runtime signal.
All effects and their successful-action audit must share one transaction.
This is a source inventory and implementation proposal, not an executed proof.
The previous five portal-operation repair is independently approved by root.

**Inventory of the active internal console.** `cmd/knowoffd/main.go` mounts
`/admin/`, `/admin/portal/` and `/admin/runtime/`. The following table includes
all mutation families exposed by those handlers; ordinary GET/export pages are
read-only and retain their existing request authentication.

| Active route / operation | Domain boundary and current evidence | Planned action |
| --- | --- | --- |
| `POST /admin/text/capture` | `TextReleaseStore.CaptureAccepted` screens outside the transaction, then calls `store.textAdmin` before locking its accepted source. Canonical account/role is checked, but the initiating session/CSRF is absent. | Common authorization after screening, before source locks; exact source recheck and capture audit remain atomic. |
| `POST /admin/text/publish`, `/admin/text/releases/{id}/activate`, `/withdraw` | `Publish`, `Activate`, `TakedownTx` use `textAdmin` then the release table lock and source/activation rows. `textAdmin` currently acquires joined account/admin SHARE locks and uses transaction-start `now()`. | Replace the shared helper with explicit account-first common authority; preserve release lock order, immutable replay and no publication reward. |
| `POST /admin/text/archive/{start,batch}` | `TextArchiveStore.StartAs` authorizes before source-table exclusion; `BatchAs` currently locks archive progress before `textAdmin`. Both audit in their transaction. | Authorize before progress/source locks in both branches; recheck time after long waits/scans before writing progress. Preserve trusted non-browser `Start`/`Batch` use. |
| `POST /admin/user-terms` | `TextTrustStore.PublishTerms` calls `textAdmin`; its insert and audit are atomic. | Inherit common exact-session authority; retain immutable wording and no manufactured consent. |
| `POST /admin/report-cases/{id}/resolve` | `reports.ResolveCase` calls `reports.LockAdmin`, then release withdrawal (for that decision), case and report locks. Joined account/admin SHARE check ignores session/CSRF. | Delegate to common account-first authority; nested `TakedownTx` must reuse the same bound context/transaction and already-held actor locks. Both case and release audits roll back together. |
| `POST /admin/{reports,feedback,cases}/{id}/status` | `admin.Triage` calls the same `reports.LockAdmin` before queue row changes/audit. Report aliases can recurse to a canonical case. | Preserve context across alias resolution; common check before every no-op/replay return or queue mutation. |
| `POST /admin/notices`, `/admin/notices/{id}/withdraw` | `CreateNotice`/`WithdrawNotice` audit transactionally but do not reauthorize the actor. A notice can trigger broadcasts/maintenance after commit. Actor is currently optional for trusted internal calls. | Authorize any named actor, and require the exact bound actor whenever a browser proof exists; bound requests cannot bypass by omitting/replacing CreatedBy/adminID. Check before notice row/replay handling; broadcast/refresh/maintenance only from committed state. |
| `POST /admin/avatars/{account_id}/takedown` | `TakedownAvatar` receives no actor, deletes avatar/entitlement rows without the canonical target account lock, then the handler separately calls and ignores `LogAction` failure. | Pass actor explicitly; sorted actor/target account locks before avatar/entitlement rows; put successful audit into that same transaction and propagate its error. Preserve current monetary semantics in this authorization slice. |
| `POST /admin/portal/submissions/{id}/{approve,reject}` | `portal.DecideSubmission` has no actor check, locks submission before its contributor account, and approval grants Noin/credit in the transaction. Screening can take place long after initial middleware validation. | Read target identity without locking, complete provider screening outside locks, then sorted actor/contributor account locks and common session authority before the exact submission row. Recheck target, revision and reviewable state, then grant/credit/audit atomically. |
| Portal role revoke, terms creation, application approve/reject; direct `GrantRole` | Five methods now use `lockPortalAdmin`, including exact session callback; 40 stale-state cases and six real lock interleavings passed571820. | Retain those proofs while making `lockPortalAdmin` a thin common-helper adapter; no second authorization implementation. |
| Portal Guard dismiss/timed/permanent decision | `decideFreeze` calls `lockPortalAdmin` after reading the target, before its freeze row, with sorted actor/target locks. Durable decision/audit precede the retryable disconnect callback. | Keep the existing exact-session protection; add coverage during common-helper transition. A committed final decision's system retry remains valid after its initiating browser session ends. |
| Portal challenge topic create; entry approve/reject; manual week close | All reach `lockPortalAdmin`. Entry decisions acquire topic before actor; week close acquires crown then topic then sorted actor/winner accounts. Scheduled closure/activation is a separate trusted worker path. | Preserve those established community lock prefixes and exact-session checks. Do not force actor-first ahead of crown/topic or attach a browser session to scheduled jobs. Add stale manual-operation tests. |
| `POST /admin/runtime/drain` | Root-owned `auditRuntimeDrain` already calls `Manager.AuthorizeSessionTx` in its audit transaction while the lobby admission mutex is held. | Preserve exact API and runtime implementation; include its tests after any common-helper change. No SQL transaction may call back into lobby. |
| `POST /admin/login`, `/logout` | Login uses pre-auth browser CSRF/TOTP and epoch-bound session issuance. Logout only removes that exact session and clears its cookie. | Keep distinct authentication lifecycle semantics; a stale logout may remove nothing but cannot authorize another mutation. Existing issuance/revocation tests remain required. |

The contribution `publish` route deliberately returns501; its dormant
`PublishSubmission` service method is not an active administrative write.
The workbench is not mounted by the current main. No active general-purpose
kick, close-one-room, arbitrary account ban/unban, refund or wallet-adjustment
route was found in these mounts. Those missing product capabilities remain
Phase5 work; this session repair must not expose them or mark them complete.
`seed-admin` is a local CLI path, not browser-session authorization.

**Non-goals.** No new moderation powers, role policy, legal wording, provider
approval, account merging or release approval. No changes to root's runtime
operations/main, migration21 subscription sources, or the other agents' backup lane.
No writer-barrier migration exists at this boundary.
No generic request wrapper may hold database locks across external screening or
HTTP responses. Previously committed worker actions and settlement deliveries
continue under their durable identities rather than an expired browser cookie.

**Cycle-free mechanism and touched files.** Introduce
`server/internal/store/admin_authorization.go` (no existing file/package was
found) with a private context key and immutable expected-actor binding. Proposed
`store.WithAdminAuthorization(ctx, actorID, authorizeTxCallback)` carries the
callback from `admin.requireRole`; the callback remains the already-approved
`Manager.AuthorizeSessionTx(ctx, tx, sessionID, csrfToken)`. The store package
imports no admin/portal/reports/notices package, so this direction has no import
cycle and no session secret is serialized or logged.

A shared `store.LockAdminTx(ctx, tx, actorID, allowedRoles, targetAccounts...)`
will reject a mismatched expected actor or nil bound callback before taking
locks; resolve the actor account; lock the unique actor/target accounts in UUID
order through `LockValueAccount`; lock the exact admin role row; validate current
status with `clock_timestamp`; then invoke the exact-session callback and require
its returned actor to match. The role list is a server-supplied constant:
portal/admin paths retain admin-only policy, and existing trusted text/report
helpers retain their current admin-or-superadmin policy. An absent browser
binding never replaces those canonical actor/role checks; explicit existing
system paths stay separate. Do not add automatic admin locking to
`WithValueTransaction`, because that would invert the community prefixes below.

`store.textAdmin`, `reports.LockAdmin` and `portal.lockPortalAdmin` become narrow
adapters. Move the context carrier out of `portal/authorization.go`; update only
the binding in `admin/handler.go`, preserving the session checker API for root.
Call the same bound callback again at the mutation/no-op boundary after domain
lock waits (already-held actor/role/session locks do not change order), so a
natural session expiry during a release/progress/application lock wait cannot
be accepted using an earlier clock check. This final clock recheck applies to
the five approved portal methods too; their lock/revocation behavior stays intact.
Other scoped files: `store/text_release.go`, `store/text_archive.go`,
`store/text_trust.go`, `reports/cases.go`, `notices/notices.go`,
`admin/{admin,handler,operations}.go` and the `DecideSubmission` body in
`portal/manager.go`, plus their tests. Preserve callback context through all
nested functions and idempotent retries.

**Lock order.** Start after any separately established writer barrier. Ordinary
admin writes use sorted actor/target accounts → admin role → exact session →
domain rows → value/profile/ledger rows → audit. Release paths use the same
authority prefix → release table lock → release/source rows; report resolution
adds case/reports after the release lock. Archive start/batch use authority →
source table exclusion/progress → source mappings; move BatchAs's current
progress-first check. Contribution decisions use sorted actor/contributor →
role/session → submission → wallet/profile/credit. Avatar takedown uses sorted
actor/target → role/session → avatar/entitlement. Existing challenge review uses
topic → authority → entry; manual challenge close uses crown → topic → sorted
actor/winner → role/session → winner/profile/wallet/audit, matching intake and
scheduled workers. Runtime drain remains lobby mutex → authority/audit transaction
→ commit → in-memory admission change. No callback into lobby under SQL locks.

**Implementation checklist (approved plan; concentrated cross-package tests).**

- [x] Add shared-helper RED/GREEN tests for bound actor/session/CSRF/status, valid controls, sorted account locking and both revocation commit orders; bind the common adapters to actual middleware.
- [x] Wire text release/capture, user terms and archive authority; preserve synthetic-only certified fixtures, reauthorize replay branches and reject browser-bound unnamed worker calls. The central mutation test proves capture/withdrawal/terms/progress expiry rollback; existing lineage/restart/activation proofs remain green.
- [x] Wire report resolution/triage and preserve context through case aliases and nested release withdrawal. Actual middleware case decisions and domain-wait expiry tests preserve domain/audit state; existing atomic nested withdrawal/retry tests pass.
- [x] Wire notices and atomic avatar takedown. Bound actor omission/mismatch/logout and audit failure preserve data and notice runtime signals; the authorized takedown waits for its target account lock and commits one audit.
- [x] Add contribution screening-boundary RED/GREEN cases: no locks during provider I/O, actor changes/audit failures preserve status, wallet, ledger and credit, and valid approval grants once.
- [x] Prove final DB-clock expiry checks after18 domain lock waits, retaining common sorted account checks and existing crown/topic/contribution concurrency tests in the full package race gates.
- [x] Freeze all14 files for independent review and pass full affected PostgreSQL/Redis race suites plus vet; reviewer approved the exact hashes. Root's runtime gate777819 passed after common-adapter integration.
- [ ] Root combines the final official repository gate after all parallel source/config/artifact work freezes. Full upload-versus-takedown serialization and durable paid-avatar policy are a separately assigned avatar slice, not a claim from the target-side authorization test.

**Risks and limits.** Joined SHARE-to-UPDATE lock upgrades must be removed before
calling the approved session checker, otherwise concurrent requests by the same
admin can deadlock. Account locks cannot be introduced after contribution
submission locks. Existing crown/topic prefixes are intentional. Optional trusted
notice actors are a compatibility boundary: a browser-bound context must never
be treated as a system call. A successful audit cannot be appended separately
from an avatar mutation. Provider calls remain outside locked transactions, and
all clock checks use DB wall time after the relevant waits. This approved plan adds no migration or privileged production action.
Implementation evidence and the independent review result are recorded below.


The coordinator approved schema21 after the separate successful-verification
clock correction. RED588609 → GREEN592285 adds repeated-evidence, delayed
changed response, failed-poll and later state recovery assertions. Final cold
reviewed gate617553 passed seven PostgreSQL race tests, economy vet and focused
formatting with zero failures/skips. `verified_at` records successful request
ordering; `checked_at` schedules attempts. Row guards do not claim TRUNCATE or
application-role privilege enforcement. Provider/projection continuation is now
authorized; account/store platform evidence remains unexecuted.

### Production artifact startup contract — implementation plan

Goal: the executable declares the migration files it was built with and refuses
an absent, dirty, ambiguous, older or newer database version before acquiring
process ownership, starting workers or exposing listeners. This does not apply
migrations, attest real deployed SQL bytes, publish content or perform a host
cutover. Exact forward compatibility is closed until separately rehearsed.

Root owns a discovered-new Go helper beside existing `server/migrations/*.sql`
that embeds those immutable files and emits their paired contiguous versions,
byte lengths and SHA256 values; `server/internal/store/runtime_schema.go` and
its tests consume its head. `newTextRuntime` calls a bounded read-only schema
check first. A secret-free `knowoffd release-manifest` command will emit this
build's migration/protocol identity before configuration/DB loading. Existing
preflight still describes an operator-selected directory and observed DB, not
compiled artifact provenance. No historical SQL is edited.

Ordered checks: malformed/unpaired/gapped local manifest tests; real PostgreSQL
startup negative tests (dirty/current-minus-one/future/empty) proving no owner
row was created; correct-head runtime integration; image build and isolated
startup smoke with no provider credentials. Pack/config hashes are supplied by
the separately reviewed release inputs, never secret-expanded config logging.
A schema version is a compatibility requirement, not proof against deliberate
DDL tampering; restore parity and controlled migration procedures remain required.
Independent review precedes the complete production artifact gate.


### Common admin transaction authority — first implementation boundary

The root-approved common-helper/adapters boundary is implemented and frozen for
independent review. `store.WithAdminAuthorization` carries an immutable expected
admin ID and SQL-only exact-session callback; `store.LockAdminTx` refuses invalid
bindings before touching authority rows, locks unique actor/target accounts in
UUID order, then the admin role and exact session, and checks canonical status
with the database wall clock. Existing trusted text/report superadmin allowance
remains explicit; portal/browser mutations retain admin-only policy. The old
portal-only context carrier is removed; all three adapters share the store
mechanism without an import cycle. Root runtime/session API remains unchanged.

Tests extend the actual middleware-to-domain boundary to user-terms publication
and real report-case dismissal alongside the five previous portal methods:
seven operations × nine status/session cases (63), with no unauthorized domain
or audit effect. Binding tests cover omitted/mismatched actor, empty binding,
nil or wrong/error callback, invalid target and roles, valid bound/unbound calls,
non-mutated caller slices, trusted superadmin policy, and PostgreSQL-observed
sorted account locks before actor/role/session. The existing real session
revocation test now exercises both the session checker and shared helper, for
logout, ban and role loss in both transaction commit orders (12 cases).

- Actual RED `admin-common-adapters-red--20260912T171933Z-721111` reproduced
  user-terms publication and success audit after logout, epoch invalidation,
  session expiry and CSRF rotation. Its report fixture had an unrelated UUID/text
  parameter inference error; that fixture was corrected before the report proof.
- Actual report RED `admin-common-report-red--20260912T172015Z-734864` then
  reproduced all four unauthorized report-case decisions and success audits.
- Shared API RED `admin-common-helper-red--20260912T172157Z-761399` failed only
  because `WithAdminAuthorization`/`LockAdminTx` had not been implemented yet.
- Real PostgreSQL/Redis race GREEN
  `admin-common-helper-green--20260912T172342Z-775977` passed full admin (9.995s),
  portal (6.171s), reports (1.511s), with `-p=1 -count=1` and no skips added.
- Full real PostgreSQL/Redis store race GREEN `admin-common-store-green--20260912T172422Z-779645` passed (103.956s); affected admin/portal/reports/store vet GREEN `admin-common-vet--20260912T172422Z-779653`. Root independently approved this exact frozen helper boundary and its runtime gate777819 passed all runtime tests (19.808s).

Frozen source/test manifest:
`/tmp/agent-runs/admin-common-authority-frozen-20260912.json`.
Domain-lock final expiry rechecks and remaining notices/avatar/contribution
families are subsequent approved-plan work, not claimed complete by this helper
boundary. No applied migration or root runtime/economy/main source was edited.

### Phase 6 isolated backup and restore proof — 2026-09-12

The unsafe ordinary-Compose replacement path in `infra/compose/snapshot.sh` is
replaced by a bounded fixture-only Python entrypoint. Production capture remains
explicitly refused: owner attestation that no deployment exists and the runtime's
`matches_drained` status do not constitute an all-writer barrier. No ordinary
Compose volume was restored, removed or replaced. A preliminary refusal test
accidentally attempted the old read-only `compose exec postgres pg_dump` without
its recording stub; it failed because that service was not running (402409).
The test was corrected immediately; no source database or service was changed.

Parent reviewed the ownership/streaming boundary, then the complete isolated
restore path. Findings were corrected with recorded regressions: nested manifest
names and streaming output bounds (410974→415039), exited-parent descendants
(448219→454036), retained root manifest data (600202→600251), foreign-volume
cleanup collision and per-role database read-only settings (720996→721067).
Twenty unit/entry tests pass (18 safety methods814641; two entry methods736097).
They cover symlinks/traversal, private exclusive writes, hash pins, incomplete and
boolean manifests, archive duplicates/truncation, output/disk/seal failures,
SIGTERM/deadline cleanup, source receipt rebinding and production refusal.

The final real rehearsal passed in
`/tmp/agent-runs/snapshot-reviewed-real-final-v2--20260912T172555Z-797770.log`.
It starts with migration8 and synthetic retained data, restores it, applies all
21 currently present paired migrations, captures that result, then performs two
independent current restores: **three restores, 70 tables, six sequences, zero
failures and zero skips**. Exact SQL files are captured and checked before later
application; Go files beside migrations are not SQL migration inputs. Source21
was independently frozen by its owner before this final gate.

Checks cover every user table's row hash, non-public schema data, large-object
bytes, schema/role/ACL/trigger/function/RLS definitions, sequence values and call
state, database owner/comment/locale/ACL/connection limits, database and per-role
settings, legacy wallets/ledger/receipts/entitlements, inline avatar/source bytes,
a legacy archive row, and nonempty synthetic terminal award/settlement/outbox and
named-entitlement rows. These SQL fixture terminal records prove retained storage;
they are not a claim of a completed production match. Redis values and absolute
expiry survive. Stopped raw MinIO volume parity plus authenticated S3 reads prove
current metadata and both historical version bodies. Retained checked-in config
templates, migrations, synthetic text pack and legacy pack bytes survive.

Database metadata exposed two real rehearsal issues before green: `--create`
restored the fixture's read-only capture setting before reconnecting (723169;
private synthetic diagnostic764723), and equivalent database ACL arrays appeared
in a different order (767774; diagnostic781943). The importer now overrides
read-only only for its bounded import connection, preserves DATABASE PROPERTIES,
then removes only the fixture's all-role capture setting. ACL comparison uses
sorted grantor/grantee/public/privilege/grant-option tuples; application and
per-role settings remain in the inventory and restore checks.

The final run also refuses a foreign network peer, an additional PostgreSQL
client and a partially detached network, retaining an invalid incomplete capture.
Cleanup succeeds with PostgreSQL stopped. Follow-up artifact gate
`/tmp/agent-runs/snapshot-artifact-check--20260912T173118Z-923668.log` verifies both
manifests through the CLI, repeats explicit database/per-role metadata assertions,
proves restore dry-run creates nothing, and confirms all five fixture resource
sets (containers, volumes and networks) are gone. Ordinary resources are outside
that cleanup scope.

Private evidence is retained at
`/tmp/agent-runs/knowoff-snapshot-reviewed-20260912-v2/`; `results.json` holds the
separate pins and aggregate result. Legacy manifest SHA256:
`3d919880c3fa41bb8d8d39fdc26f229df81506fdcce12bd6dfaf6b5d0f6497a2`.
Current manifest SHA256:
`ef4c1f7268aa780c6602027a1134992758b3e2db601af80fdc684571ed48a653`.
Files are private and output roots are0700. This is a fixture-only recovery
rehearsal. No production secrets, role passwords, live deployment, all-writer
quiescence, application reopening or MinIO retirement is claimed. Historical
MinIO/MC image digests are retained only to reproduce the old storage format.
The default unified Python gate includes both real integration tests without
skip markers; the coordinator owns the later combined full-repository gate.

Owned files: `infra/compose/snapshot.sh`, `infra/compose/snapshot.py`,
`xops/test/test_snapshot.py`, `xops/test/snapshot_integration.py`, and
`infra/README.md`. No tracking rows, staging, commit or push was performed by
this delegated worker.

### Production artifact startup contract — reviewed evidence

Independent review approved the compiled manifest and bounded read-only startup
schema guard. Actual PostgreSQL race gate777819 passed all runtime tests
(19.808s), including ten persisted mode/table cells, owner loss and six invalid
schema states with no ownership writes. Schema read-only/search-path/deadline
gate763650 passed. Manifest command regression767456 reproduced accidental config
loading before its implementation; the full runtime gate proves config-free CLI.

Production Docker build788806 passed using the retained native avatar WebP
runtime. Image ID `sha256:ec44baa4f88780e10b0c1928385a0676ddfbce79312d8aeacca2fe558bfb7338`
was executed without networking, with a read-only filesystem and UID/GID65534.
Gate931787 verified protocol2, five modes, head21 and all42 compiled SQL files'
exact source lengths/hashes. Private manifest:
`/tmp/agent-runs/text-artifact-manifest-20260912.json`.
Initial hardened Docker attempt923501/924548/926683 was refused before app
execution (`operation not permitted`, also for `/bin/sh`); no host security
settings were changed. Normal runtime restrictions above are proven; the extra
no-new-privileges/cap-drop combination remains unverified on this workstation.
Harness927160 used the wrong JSON field name; it was corrected from inspected
output before931787. This image predates subsequent schema22/provider/admin
changes and must be rebuilt at final gate. No service was deployed.

### Text runtime configuration retirement — implementation plan

Root will remove playable storage/media settings from active base/local/staging/
production examples and remove their mandatory secret checks. The only current
`config.Load` executable caller is knowoffd; active main no longer constructs any
playable-image manager or object store. Retained historical Go types/callers are
tracked for the separate source-retirement inventory, not silently exposed in
loaded config. Old storage/media keys receive an explicit text-v2 migration
error before secret interpolation; unknown config keys remain strict. The active
protocol becomes2 and explicit older protocol versions fail. Disabled OAuth
providers have empty default credentials rather than requiring unrelated cloud
keys. Required core secrets remain PostgreSQL, Redis and JWT. Avatar PostgreSQL
blobs/native codec and optional explicitly enabled provider credentials remain.

RED tests first load every checked-in overlay with only core secrets, reject
old storage/media/protocol keys without echoing secret values, and preserve
unknown-key/invalid-core-config failures. Update old config test inputs to the
text contract without deleting test files or weakening their original unknown/
validation assertions. Config race/vet, actual image startup against disposable
PG/Redis, and independent review precede the final gate. Old source/module and
Compose retirement remain separate reviewed work; this narrow change does not
claim all old architecture removed.


### Active object-store retirement — inventory and execution plan

**Goal.** Remove MinIO and playable-image delivery from the active local
infrastructure, with a runnable proxy proof and explicit retained-data boundary.
The owner states no deployment exists; there is no live old-client drain window
to infer. The independently reviewed head21 restore797770 remains the last
captured local recovery proof. Later schema22 requires a new final rehearsal.

**Observed consumers.** Source/CodeGraph, config and artifact inventory on
2026-09-12 distinguishes these paths:

| Path | Current consumer and disposition |
|---|---|
| `infra/compose/docker-compose.yaml` | MinIO service, server/nginx health dependencies, MinIO volume declaration, and server/migrate legacy pack/ingest mounts remain active startup requirements. Remove those declarations; never delete an existing Docker volume or run ordinary Compose down/restore. |
| `infra/compose/config/{server,migrate,seed-admin}/environment.env` | Old storage access/secret and media URL keys remain in three development templates. Remove only those assignments; preserve core credentials privately. `config/minio/environment.env` becomes unreferenced and is the exact proposed non-test file deletion. |
| `nginx/nginx.conf`, `nginx/conf.d/minio.knowoff.local.conf` | MinIO upstream and explicit console vhost include remain active. Remove both and delete that exact non-test vhost file. Preserve root's OAuth callback query/Referer log suppression. |
| `nginx/conf.d/app.knowoff.local.conf`, `00-security-headers.conf`, `xops/makefile/hosts_ops.py` | Remove only MinIO/9000 CSP origins and desired host entry/comment. Keep avatar blob/data images, Flutter/web/font origins and all other hostnames. Never invoke host-file mutation. |
| `server/cmd/knowoffd/main.go`, text runtime | Active assembly now constructs `newTextRuntime`, scoped text snapshots and persisted release resolver. No legacy media/workbench manager is constructed. Parent owns config/readiness/metrics retirement and actual server image proof. |
| `server/pkg/media/{manager,loader,static_image,urls}.go` | Retained v1 loader/image validator and URL signer; signer feeds legacy `game/payload.go`, which feeds old Match/tests. These are source-retirement candidates, not evidence of an active S3 client. `bundle.go` also defines ContentHash used by current text code, so package/file-wide deletion is unsafe. No S3 SDK is declared by current Go modules. |
| `server/internal/workbench/workbench.go` | Unmounted legacy image/dealing/ingest watcher and dev routes remain source. The current authenticated text catalog review/simulation is the replacement; later source retirement must remove the old executable after caller proof. |
| `tools/mediapack/cmd/mediapack/main.go` and old generator/writer | Old build/certify/simulate/publish commands still exist beside text commands; old synthetic packs are text records in format1, not valid new releases. `prepare_candidates.py` still performs Pillow image preparation with substantive provenance/hash/security tests. These require a separate source-retirement change. |
| `client/lib/media/{media_engine,pack_sync_service,asset_cache,media_models}.dart` | Legacy whole-catalog sync/prefetch has test consumers; active text rendering does not call it. HomeScreen still has explicit protocol1 fallback into GameScreen. Parent owns removing that fallback and legacy UI/source after equivalent tests. |
| `client/lib/presentation/widgets/game_surfaces.dart` | Old GameMediaWell image rendering shares a file with GameRoleSeal used by text match UI. Do not delete the whole file. Keep targeted poke feedback from dev_tools_panel as a real current caller. |
| `client/pubspec.yaml`, `client/assets`, `client/web` | Packaged Flutter assets are config.json and Baloo2 font, with license retained; web PNGs are favicon/app icons. No playable image pack is declared. Keep these and http/crypto/shared_preferences/file_selector/QR/url-launcher, which have account/security/avatar/UI consumers. |
| `server/internal/avatar/avatar.go`, `server/Dockerfile*` | Current upload decodes images, encodes WebP and writes custom_avatars PostgreSQL blobs. Retain chai2010/webp, x/image, CGO/compiler/runtime dependencies and all avatar tests. |
| `content/packs/core-2026.10`, media golden/band-starved fixtures, applied SQL, archive/consent/report/entitlement rows | Preserve historical bytes and references; the checked-in old pack currently has three JSON/JSONL files and no binary assets. No archive/image-data deletion is proposed. |
| `infra/compose/snapshot.py`, `xops/test/{snapshot_integration,test_snapshot}.py` | Retain digest-pinned MinIO/MC only for isolated raw-volume/version restore fixtures. This consumer has an explicit archive-rehearsal purpose, no runtime Compose dependency. |
| `xops/test/tests-lints.py`, CI | Legacy three media/storage environment keys can leave the runner after parent's config removal; Pillow remains while the image candidate tests remain. Parent owns coordinated runner cleanup/full gate. |

**Bounded touched files.** Infrastructure worker owns Compose, its three service
environment templates, `infra/README.md`; nginx main/API/app/security configs,
`nginx/README.md`; `xops/makefile/hosts_ops.py`; and new discovered-absent
`xops/test/test_infra.py` (with a sibling integration helper only if needed).
Root retains ownership of server/client/config/workflow changes. Exact file
deletions in this bounded change: `infra/compose/config/minio/environment.env`
and `nginx/conf.d/minio.knowoff.local.conf`. **Test-file deletion list: empty.**
No source package, archived data, applied migration, historical test or binary
asset is deleted. Any later legacy test removal needs its own exact list and
required user confirmation/release note; it is not authorized by this plan.

**Tests and checklist.**
- [x] Add failing infrastructure contract tests for rendered core services,
  dependencies/mounts, old environment-key absence and MinIO route/origin absence.
- [x] Remove the inventoried active declarations and update operator docs to
  state actual profiles (client-web/cloudflared currently commented out), without
  touching ordinary Docker resources or host files.
- [x] Build the current nginx Dockerfile; run actual `nginx -t` in a fresh owned
  isolated network with synthetic upstreams and private temporary certificates.
- [x] Send unique synthetic code/state/query/Referer sentinels to the real TLS
  callback success path, then stop only its owned upstream and exercise the
  upstream-error path. Assert status and absence from captured access/error logs;
  confirm ordinary path-level diagnostics remain useful. Prove unknown/retired
  MinIO hostname cannot open a MinIO backend. All deadlines/cleanup are bounded.
- [ ] Independent review then cold scoped verifier; parent combines actual fresh
  PostgreSQL/Redis/server with no MinIO/cloud keys and full repository gate after
  config/schema/artifacts freeze. Do not label a proxy stub as a real game proof.

**Risks and boundaries.** Removing a volume declaration never authorizes volume
removal. Existing retained backup evidence and source exceptions above remain
explicit until later source retirement. Empty content availability stays closed;
synthetic local fixtures never become production releases. Nginx tests use only
nonce-owned containers/networks and private workspace-allowed temporary data,
not current TLS keys, real OAuth codes, external providers or host installation.

### Runtime config and connection metrics — reviewed implementation

Config RED942490 reproduced mandatory retired cloud keys in all four checked-in
environments and silent acceptance of retired settings. GREEN947425 passed the
full config race suite (2.633s); independent review approved the loader, exact
retired-key rejection before interpolation and core-only secrets. Shared text
policy values remain unchanged; removal of old unused fields changes the full
policy hash, requiring genuine current-policy certification for new releases.
Historical typed snapshots are not rewritten. Server README and VPS runbook now
describe actual durable settlement/release/restore evidence and remaining gates.

Metrics plan and source independently approved. The existing production
connection gauge is now injected into the text handler, increments only after
successful WebSocket upgrade and decrements through every return. Capacity
rejections increment only when acquisition fails; no identity labels exist.
API RED955101 included a corrected test integer mismatch. Actual behavior
RED956841 showed zero active metric with one live socket; GREEN959403 passed
real Prometheus Gather, capacity refusal, failed HTTP upgrade reservation release,
protocol/auth refusal, valid handshake and close/reconnect cleanup (1.113s).
Formatting962906 passed; broader actual runtime integration965910 follows.

### Client active-route retirement — approved implementation plan

Remove Home/LocalRoom's active v1 GameScreen/notifier fallback. Existing text
selection receives the explicit4/6 size and injected API, currently lost on the
v2 local path; a joined room still takes authoritative settings from its server.
Unsupported configured client protocol shows the existing localized upgrade
message before connecting or admitting, including direct TextPlayScreen entry.
Keep leave/back/account/safety, current Ko widgets, code validation and layout.
Replace old Home journey assertions with substantive v2 size/code/explicit
selection/no-admission and stale-config refusal coverage; delete no test files.
Independent plan review approved. Root owns Home, TextPlayScreen and their tests;
legacy source class removal remains a separate inventory boundary.

The broader root integration gate965910 passed against fresh PostgreSQL/Redis:
knowoffd18.067s, config3.648s, game6.697s, lobby8.124s, handler18.668s and
pkg/media31.640s, all with race detection. Temporary services were removed.
This validates new config/tuning against runtime and existing engine consumers;
it does not replace the final all-repository runner on the final diff.

### Remaining admin mutation authority — implemented domain boundary

The approved common mechanism now protects the remaining active mutation
families. Named notice actors must match the initiating request; omission cannot
fall back to a trusted system path. Avatar takedown receives the actor explicitly,
locks sorted actor/target accounts before avatar/entitlement rows, and commits its
audit atomically. Contribution approval and rejection capture the contributor
identity before screening, then lock actor/contributor accounts and revalidate
the exact source inside the transaction before status, Noin, credit and audit.
External screening never holds account or session locks.

All scoped domain success and no-op branches recheck exact session authority
with the database wall clock after their later locks or audit writes. This covers
portal roles/applications/terms, user terms, accepted-input capture and replay,
release publish/activation/withdrawal, archive progress, report decisions/triage,
notices, Guard decisions and manual challenge review/closure. Archive batch
checks the actor before progress. Browser-bound contexts cannot enter unnamed
archive worker APIs. Existing community crown/topic prefixes are retained;
trusted scheduled closure and already-committed Guard delivery retries retain
their separate durable authority. No applied SQL migration changed in this slice.

The new `admin/mutation_authorization_test.go` concentrates the cross-package
proofs using the real session callback and disposable PostgreSQL/Redis, alongside
the first boundary's actual middleware tests. Its 47 cases cover 18 observed
PostgreSQL lock waits spanning session expiry (including accepted-input replay),
12 notice/avatar omitted/mismatched/revoked actor or failed-audit cases, 14
approval/rejection screening/actor/audit cases, two forbidden unnamed archive
calls, and an authorized avatar takedown that waits for the target account lock
and produces exactly one audit. Failed maintenance creation neither broadcasts
nor pauses admission. Failed contribution decisions preserve wallet balances,
ledger count, contributor credit and submission state. Valid approval grants
once; its repeat cannot grant again.

Exact evidence (logs under `/tmp/agent-runs/`):

- `admin-domain-expiry-red--20260912T172946Z-901914` reproduced expired-session
  domain/audit commits and missing notice/avatar authority. After initial fixes,
  full admin/notices race `admin-domain-authority-green--20260912T173247Z-927223`
  passed24.074s/1.196s.
- `admin-contribution-actor-red--20260912T173519Z-940421` reproduced stale approval
  granting7Noin plus credit/ledger/audit, and stale rejection. The corrected
  screening-boundary implementation passed14 cases in
  `admin-contribution-authority-green--20260912T173631Z-943078` (2.814s).
- Guard expiry RED951961 and challenge expiry RED959842 reproduced committed
  decisions/audits after domain waits. The challenge fixture first required its
  explicit configured portal text bound; that setup failure was diagnosed before
  the actual challenge RED. Combined new-domain GREEN964704 passed25.760s.
- `admin-worker-binding-red--20260912T174444Z-972345` reproduced both bound-browser
  unnamed archive bypasses. Its new positive avatar fixture used a nonexistent
  column; that fixture was corrected to the existing blob/value columns.
- `admin-capture-replay-expiry-red--20260912T174630Z-983248` reproduced a successful
  replay after session expiry; its final authority check is now covered by the
  complete admin gate below.
- Full real PostgreSQL/Redis admin race
  `admin-domain-authority-final-admin-race--20260912T174719Z-991530`: **PASS34.519s**.
  Full portal/reports/notices race from
  `admin-domain-authority-final-race--20260912T174551Z-979546`: **PASS8.565s/1.523s/1.236s**.
  That earlier combined run's only failure was an avatar test assertion comparing
  text audit IDs against a UUID parameter; the explicit text cast was corrected
  before991530, without weakening the assertion.
- Full real PostgreSQL/Redis store race
  `admin-domain-authority-store-race--20260912T174551Z-979554`: **PASS82.040s**.
  The final one-line capture replay recheck subsequently passed the complete
  accepted-lineage/restart/takedown test in
  `admin-authority-lineage-final-race--20260912T174806Z-994088`: **PASS13.317s**.
- All five affected package vet passed979570; final admin/store vet passed994089.
  No test skips or assertions were removed; all fixture failures and actual REDs
  were read and diagnosed before reruns.

Frozen source/test manifest:
`/tmp/agent-runs/admin-domain-authority-frozen-20260912.json`, SHA256
`7740b31d79024ae51f70a3381e0f3333aede910be42a4675bf28e54aa702920f`.
Baseline independently approved all14 file hashes against that manifest, with
no actionable finding. Root coordinates the final repository gate and
tracking/staging.

**Avatar boundary retained for the separate product slice.** This change keeps
the pre-existing takedown behavior of deleting the custom avatar and its
custom-avatar entitlement, with no refund. It does not decide future re-upload
entitlement policy. The active upload implementation still needs serialization
on the same account lock and atomic blob/unlock handling; the current upload
writes its blob separately and ignores the unlock result. The new target-lock
proof covers the takedown side only, not that outstanding upload race. Root owns
that explicitly separated full-avatar work. No avatar bytes, paid value,
historical contribution records or policies were rewritten by this auth slice.

### Active infrastructure retirement — independent review and verification

Independent review approved the bounded Compose/nginx/hosts/env-template change:
only active MinIO service/dependency/mount/vhost/CSP/desired-host declarations
were removed. Existing Docker volumes and historical content remain untouched;
avatar build dependencies and isolated archival MinIO restore fixtures remain.
No test file was removed and no host-file command was run.

Cold verifier `infra-retirement-independent-verify--20260912T174834Z-996574`
passed all6 tests in6.009s, including building the actual nginx Dockerfile and
running `nginx -t` plus TLS requests in a nonce-owned internal network. Callback
success204 and upstream-error502 both excluded synthetic code/state/Referer
sentinels from captured logs; ordinary path/error diagnostics remained. The
fixture had zero MinIO containers and zero published ports, and cleaned its own
containers/network/image. Evidence with source hashes:
`/tmp/agent-runs/knowoff-infra-80113ce2f532-_hsc1pu1/results.json`.
This is a proxy proof with synthetic upstreams, not a real game-server or
external-provider production proof.


### Active object-store retirement — reviewed evidence

The bounded infrastructure plan is implemented and independently approved. No
legacy test files were deleted. The two approved non-test deletions are the old
MinIO environment template and console vhost. Compose rendering now contains
exactly PostgreSQL, Redis, migrate, server, adminer and nginx in the core profile;
no old object credentials, pack/ingest mount or object-store dependency remains.
This does not erase any existing Docker volume or historical content. The
separate isolated snapshot fixture remains the explicit MinIO retention consumer.

The contract regression first failed on the actual old paths in
`/tmp/agent-runs/infra-retirement-red--20260912T174053Z-956710.log`, then all five
checks passed in957877. Real proxy fixture development uncovered two environment/
harness issues: this Docker daemon cannot bind the workstation's `/tmp` path
(961276/968535; replaced by streaming fixture bytes), and stopping the whole
upstream container removed its network interface, producing a timeout instead
of deterministic connection refusal (973787; now stop only the owned upstream
process). Curl negotiated HTTP/2 while the test expected HTTP/1.1 (977101); the
input now explicitly requests HTTP/1.1. Duplicate short/full cleanup IDs were
corrected after984825; full IDs are deduplicated and ownership checked.

Final implementation gate988740 passed all six tests in 6.667s. Independent
review and cold verification:
`/tmp/agent-runs/infra-retirement-independent-verify--20260912T174834Z-996574.log`:
**six passed, zero failed, zero skipped**, 6.009s. The real built nginx image
passes `nginx -t`; the TLS callback returns 204 before and 502 after upstream
shutdown. Unique synthetic code/state/Referer sentinels are absent from captured
access/error logs, while ordinary health-path status and connection errors remain
observable. The fixture publishes no ports, creates no MinIO container, and
cleans up its labelled proxy/upstream/network/image. No current certificate or
ordinary Compose service was used. This is a proxy integration proof, not a
real game-server or external OAuth-provider proof.

Final compile/whitespace gate1010842 passed. Resource audit1018348 found all six
owned container (including stopped), network and image inventories empty for the
implementation and independent verification fixtures.

Independent source-hash/image/status evidence is private at
`/tmp/agent-runs/knowoff-infra-80113ce2f532-_hsc1pu1/results.json`; companion
`proxy.log` contains synthetic diagnostics only. Actual `go list -deps
./cmd/knowoffd` inventory993270 reports366 imports, no MinIO/AWS/S3 package, and
retained `chai2010/webp` plus `x/image` avatar decoding/scaling packages.
Flutter pubspec declares configuration and font assets; favicon/app icons remain.
Root's upcoming actual fresh PostgreSQL/Redis/server proof and final full gate
remain separate. Schema22 is now frozen; backup797770 proves its capturedhead21,
so the later combined restore gate must capture the exact new1–22 files.

The same worker independently approved the final domain-authority source in
`/tmp/agent-runs/admin-domain-authority-frozen-20260912.json` (SHA256
7740b31d79024ae51f70a3381e0f3333aede910be42a4675bf28e54aa702920f), with all14
file hashes matching. Final mutation and successful replay paths reauthorize the
initiating actor/session after domain/audit lock waits; bound archive callers
cannot use unnamed worker APIs, notice signals follow commit, avatar audit is
atomic, and contribution screening precedes the sorted actor/contributor lock
transaction. The review read all18 domain-expiry cases and accompanying actor,
screener, rollback and target-serialization assertions. Existing avatar deletion
policy remains a separate coordinator-owned product correction.

### Full avatar lifecycle — inventory and proposed execution plan

**Required contract and policy.** ROADMAP Phase5 child16 and Blueprint Profiles
§2 / Economy §5 require a one-time paid upload entitlement, server-side centered
crop to256×256 WebP with EXIF stripped and size bounds, automated screening before
display, reportability and admin takedown to presets without refund. Root's new
avatar assignment explicitly selects retaining the paid upload unlock on asset
takedown, matching the roadmap's preserve-entitlement wording; account moderation
is separate. The previous authorization slice intentionally preserved the old
delete-entitlement behavior until this product slice. No applied migration or
historical blob is rewritten as reviewed/approved by this transition.

**Current source findings.** CodeGraph shows ProcessUpload decodes the full image
before checking dimensions, persists an unmoderated blob before a separate
ignored unlock-purchase failure, and neither screens nor selects a custom image.
The multipart handler limits form memory rather than total request bytes and
leaves temporary multipart cleanup implicit. Profile avatar PATCH accepts any
string; the UI renders only preset doodles, and no custom-avatar read route
exists. The picker already validates2MiB/2048px before preview and obtains the
server price, but its upload button currently implies purchase without an
explicit entitlement decision. The existing store unlock route already supports
custom_avatar at the unchanged configured price and uses the wallet admission
guard. Takedown now has correct actor/target locks and atomic audit, but requires
the newly authorized paid-entitlement preservation and selector reset.

**Proposed bounded implementation, awaiting root plan review.**

1. Keep purchasing explicit through the existing guarded one-time store unlock
   route. Upload requires a previously purchased permanent entitlement and never
   silently spends/refunds Noin; canonical entitlement, account status and JWT
   session are validated together with activation. The client presents an explicit
   unlock action and confirms success before offering upload; existing owners
   upload without another purchase, including after asset takedown.
2. Allocate additive migration23 (subject to root reservation) for an account
   avatar revision. Capture it with session/account authority before remote
   screening, release locks, then reject activation if a later upload, preset
   choice or takedown advanced it. This prevents a slow reviewed request from
   recreating a removed image. Successful normalized blob+moderated flag+custom
   selector+revision change share one account-locked transaction. Ban, deletion,
   suspension and session revocation are checked again after all waits. The
   existing raw blobs remain inactive unless newly screened; no invented receipt.
3. Add a separate disabled-by-default avatar-screening configuration and provider
   adapter. Use the official image moderation contract at the existing fixed
   OpenAI endpoint, with the normalized WebP as a base64 data URI. Bound timeout
   and response bytes, disable redirects, require one explicit nonflagged result,
   and map refusal/unavailable/malformed responses to stable errors without
   logging provider bodies, images or credentials. No request occurs in the
   constructor and no real provider credential or upload is used in proofs.
4. Enforce whole multipart-body/file bounds and cleanup without trusting declared
   MIME, dimensions before full decode, supported raster signatures and bounded
   dimensions/pixels. Encode a fresh256×256 WebP so EXIF/other input metadata do
   not survive. Screen exactly those prospective displayed bytes. Keep existing
   native WebP/CGO tooling and all substantive avatar tests.
5. Preserve the paid entitlement on takedown, reset to the default free preset,
   advance the avatar revision and audit in the existing admin transaction. Add
   curated-only preset selection under the same account/revision lock. Serve
   selected moderated custom blobs only through authenticated, no-store/nosniff
   HTTP, with deleted accounts and private blocked UGC refusing identically.
   Profile displays fetch through the authenticated API, never a token-bearing
   URL, and fall back to a preset on unavailability/removal.
6. Update the existing picker/profile with explicit ownership/purchase, bounded
   upload lifecycle and localized stable refusal states; preserve previous avatar
   and selected input on recoverable failure, and clear stale account drafts on
   identity switch. Keep report/block/back actions available.

**Ownership requested.** `server/internal/avatar/**`; narrow avatar selector/read
portions of `profile/profile.go`, `handler/api.go` and their tests; new avatar
screening config field/tests and unchanged-default YAML entries; migration23 if
reserved; the avatar takedown portion of `admin/admin.go` and its regression
expectations; client `service_avatar_upload.dart`, a narrowly discovered-absent
avatar display helper only if needed, avatar API methods, and avatar-specific
account screen/localization/tests. Root owns unrelated config/client/main paths;
other agents' billing/recovery edits remain unchanged. No new dependency is
expected, and no live provider activation is included.

**Proof gates.** Capture actual REDs before implementation: unpaid/disabled or
failed provider cannot mutate blob, selector, revision, entitlement, wallet or
ledger; exact input bounds/crop/WebP/EXIF; rollback at every activation write;
revocation/ban/deletion during provider I/O; real PostgreSQL upload→takedown and
takedown→stalled-upload orders; concurrent upload/preset decisions; exact admin
audit failure retains old state and paid unlock. Run actual multipart HTTP and
custom read authorization/block/caching tests; fixture provider refusal, timeout,
redirect, malformed/missing verdict and oversized response tests. Client proofs
cover explicit one-time purchase/cancel/already-owned upload, provider failure,
custom rendering/preset fallback and account-switch disposal. New migration must
pass actual fresh/upgrade/down parity without approving legacy blobs. Finish
with independent source review, realPG race/vet and official client tests/build;
external moderation-provider acceptance remains an unexecuted launch dependency.

Provider contract source (checked2026-09-12):
[OpenAI Moderations API](https://platform.openai.com/docs/api-reference/moderations)
accepts image_url objects carrying image URLs or base64 data URLs. This plan
uses only normalized in-memory data; it introduces no remote image-fetch URL.

### Client active-route retirement — reviewed implementation

The seven targeted regressions failed on the original behavior in1001326:
unsupported protocol1/3 still reached the legacy game, direct text entry exposed
Connect/resume, and Home/Local lost six-seat selection or the supplied API.
Home and Local now navigate only to explicit text selection; unsupported
configurations keep upgrade copy and account/back access with no legacy intent.
TextPlayScreen carries the requested size and refuses connect/resume/actions for
an unsupported configured protocol. Existing Local code validation and explicit
join are preserved. No test file was deleted: old journey assertions now verify
v2 selection, no premature admission, normalized code and offline navigation.
Two additional wire tests prove exact six-seat queue/create mode/language/release/
rules tuples. Scoped1011220 passed all40 cases; independent source review by
billing worker approved. Full updated Flutter gate follows the remaining avatar
client work; this is not a physical-device or all-retirement completion claim.

### Production service artifact and outage rehearsal — approved extension

Root owns new `xops/test/artifact_integration.py`, after confirming no existing
artifact-service harness. Independent baseline review approved this fixture-only
extension of the compiled-manifest plan. Build a frozen server source/config/SQL
snapshot with the actual production Dockerfile; record immutable image identity
and compare every embedded migration byte/hash. Start only nonce/UID-labelled,
unpublished internal-network PostgreSQL/Redis and a nonroot read-only runtime,
using synthetic core credentials and no MinIO. Stream fixtures rather than bind
host temporary paths. Verify actual migration CLI, readiness/liveness, obsolete
route refusal, authenticated unavailable-mode discovery and metric presence.
Stop/restart Redis and observe readiness recovery; PostgreSQL loss must close
the old process, then a new process acquires a later owner generation without
changing retained account/value state. Quiesce before incompatible schema/pack
negative tests and prove refusal without admission/value writes. All commands,
output, polls and cleanup are bounded, failures retain private diagnostics, and
cleanup verifies exact owned resources including uncertain-create discovery.
This harness does not claim human smoke, a live deployment, 100-room latency,
soak or physical-device evidence. Final frozen source review and actual execution
are required; migration23/avatar changes must be complete before the final build.

### Retained permanent Premium admission — reviewed correction

Final billing consumer review found NULL-duration Premium was recognized by the
wallet but treated as free access by text admission. Actual regression1016285
failed both reservation and start at the exhausted free cap. Only the Premium
query now accepts NULL; pass expiry stays unchanged, and provider epoch/exact
expiry remain inactive. Four-seat tests cover monthly/yearly permanent coverage,
expired cases, granting between reservation/start, start replay, unchanged free
counts and zero currency. Scoped real-PG race1018656 passed (3.504s); independent
baseline source review approved. Client route independent verification1013843
also passed all40 cases.

### Production artifact service rehearsal — executed evidence

Actual production build and six service proof stages passed in1066762 using a
frozen in-progress source snapshot at migration23. Image identity:
`sha256:2d221f9cf7f97f345d4c44502b5d9e6404ca828e2366faa432ca9bfd562eab1f`.
Private results/source/config/migration hashes:
`/tmp/agent-runs/knowoff-artifact-d897dee6e941-9hn7cqsp/results.json`.
The exact image ran UID/GID65534 with read-only root and configuration, only
three synthetic core credentials, no published ports and no MinIO. Actual
migration CLI reached head23; authenticated discovery kept all five modes
unavailable, three legacy routes returned426 and internal/obsolete surfaces
were absent from the public listener. Metrics were present at zero sockets.
Redis stop caused503 readiness with200 liveness, then recovered under the same
owner. PostgreSQL stop caused permanent owner-loss exit; the successor became
ready at generation2 after generation1. One fixture account and empty financial
table counts were preserved: this is not populated ledger/receipt parity (the
separate backup proof covers retained financial fixtures). Dirty/older schema and
a missing prototype pack refused startup without new owners/admissions/value.
All labelled containers, volumes, network and unique image were cleaned up.

First run1052631 built successfully but Docker refused copying into a read-only
root. The harness now streams to a separately populated owned config volume
and mounts it read-only; no host settings changed. Independent review approved
the safety boundary and requested the missing-pack proof assert its exact
initialization/lstat cause, now corrected using the observed failure log. A
final reviewed source snapshot/build is still required after ongoing avatar and
other changes; no human, load, device or live-deployment evidence is asserted.


## Native purchase and restore integration plan — billing worker, 2026-09-12

**Goal.** Connect the existing Store screen to native Play Billing / StoreKit
purchase and restore, using exact configured products and server-verified value.
The coordinator approved this bounded implementation; no store dialog, charge
or platform activation has been attempted. Blueprint 💰 §§2–3 and Product Baseline, and
ROADMAP Phase5.17a/17b separate implementation from genuine platform evidence.

**Current gaps and dependency evidence.** `StoreActions` handles only Noin spends
and conversion; `StoreScreen` has disabled Premium buttons and bundle tags.
There is no purchase plugin, receipt API call or app-scoped purchase listener.
`storeCatalog` returns generated legacy IDs and placeholder zero cash prices,
not the actual billing configuration. Android/iOS still use example application
identifiers and Android release signing uses debug keys. These are explicit
launch blockers, not identities to guess or silently replace.

Pin the related official Flutter packages to `in_app_purchase:3.3.0`,
`in_app_purchase_android:0.5.3` and `in_app_purchase_storekit:0.4.13`, with the
lockfile. Pub metadata and SHA256-verified archive inspection
`purchase-sdk-source-inspection--20260912T181345Z-1090650` confirm Android's
Flutter≥3.44/Dart≥3.12 requirement and Play Billing8; local Flutter3.47.3/Dart3.13.3
is compatible. Raise the misleading manifest minimum to the actual dependency
minimum; do not upgrade unrelated packages or OS tools. The provider maintains
these packages; its current public security page lists no purchase-plugin
advisory. This limited check is not a claim of vulnerability freedom.
[Flutter plugin](https://pub.dev/packages/in_app_purchase),
[Android changelog](https://pub.dev/packages/in_app_purchase_android/changelog),
[StoreKit changelog](https://pub.dev/packages/in_app_purchase_storekit/changelog),
[provider security policy](https://github.com/flutter/packages/security).

**Protocol and ownership.** Add an authenticated, no-store `billing` section to
the existing `/api/economy/store` response: a bounded sorted list of platforms,
availability and exact `{product_id,kind,noin}` products from configured and
constructed verifiers; disabled platforms expose no purchasable entries. No
credential, receipt, wallet balance, invented currency price or generated SKU
enters it. Preserve the avatar owner's `custom_avatar_available` and
`custom_avatar_owned`. The client intersects this catalog with native returned
products and their localized prices; missing/ambiguous/type-mismatched products
stay unavailable. For the first slice, subscriptions must have an unambiguous
regular monthly/yearly auto-renewing base plan with no trial/promotional phase;
multiple eligible choices require explicit subsequent configuration, never an
arbitrary first offer. Display actual store prices and terms; do not claim the
planned yearly discount where store pricing does not establish it.

The existing receipt shape is sufficient. Google sends
`{platform:"google_play",product_id,raw_receipt:{purchase_token}}`; Apple sends
`{platform:"app_store",product_id,raw_receipt:{transaction_id}}` from numeric
`purchaseID`, never a client-decoded JWT or fabricated verification flag.
Both omit outer `transaction_id`. Purchase parameters bind the exact authenticated
canonical UUID through `applicationUserName`: the pinned Android code maps it
to obfuscated account ID; StoreKit2 maps it to `appAccountToken`. Refuse StoreKit1
purchase fallback because it cannot provide this required account-token flow.
[parameter API](https://pub.dev/documentation/in_app_purchase_android/latest/in_app_purchase_android/GooglePlayPurchaseParam-class.html),
[StoreKit parameter API](https://pub.dev/documentation/in_app_purchase_storekit/latest/in_app_purchase_storekit/Sk2PurchaseParam-class.html).

One app-scoped listener subscribes before native queries, survives Store route
disposal and captures account plus server identity for each verification. No
receipt work creates a new anonymous account; failed refresh/account switch
keeps the purchase uncompleted and offers identity restoration. Bound concurrency,
coalesce exact duplicate callbacks and keep retryable pending work without
logging provider payloads. Native pending/canceled/error and server
pending/granted/inactive/refunded states are distinct; only server `granted`
permits success. A failed wallet refresh does not undo or mislabel a durable
purchase, and in-match privacy continues to refuse balance projection.

Android consumables use `autoConsume:false`; the existing server owns consume
and acknowledgement, so no second client consumption/acknowledgement competes
with its durable worker. StoreKit2 finishes only after durable server acceptance,
and retries finish separately if the response or completion is lost. Restore
revalidates each returned purchase through the same path. Consumed bundles are
recovered through the retained game account/wallet, not invented native restore
entries. Existing active subscriptions expose management rather than choosing
unrequested upgrade/proration rules; native store changes still feed the same
server verification path. Web/unsupported/unconfigured stores perform no purchase
launch and grant nothing.
[Flutter lifecycle and restore](https://pub.dev/documentation/in_app_purchase/latest/),
[Google verification and consumption](https://developer.android.com/google/play/billing/integrate).

**Touched files.** New discovered-absent `client/lib/data/` purchase adapter and
controller files plus focused data tests; `ApiClient` receipt method,
`StoreActions`/StoreScreen purchase sections, app-scoped service wiring, purchase
ARB keys, pubspec/lockfile and minimal native build compatibility. Narrow server
catalog method/projection tests and MODULE-billing documentation require
coordinator ownership allocation. Avatar Profile/upload source remains with its
owner. Preserve all old tests and shared source.

**Ordered checklist and proof.**

- [x] Catalog/typed API: red tests prove disabled and partial config never exposes
  a fake product, exact configured IDs are sorted and secret-free, malformed
  catalog/status/proof is refused, and authenticated receipt JSON matches Go.
- [x] Native bridge: fake-platform tests verify exact account/product/token,
  Android manual consumption, StoreKit2-only account binding, unavailable or
  ambiguous offer refusal, restored stream shape and no premature completion.
- [x] Lifecycle: callback-before-query, duplicate/concurrent updates, native
  pending→purchased, server outage/lost response, finish failure, stale account
  callback, explicit restore, restart recovery and route disposal tests prove
  one authoritative result and no client value mutation.
- [x] Store UI: localized real prices and pending/retry/restore/status/management
  controls, same-account refresh, unavailable Web and enlarged-width/RTL widget
  tests; preserve avatar availability and owned-unlock behavior.
- [x] Independent review, then cold analyzer/full Flutter tests, web build and
  Node cache checks; native Android SDK is absent in this workstation.
  Report native toolchain limitations directly, never install OS tools.

**Final local acceptance.** Parent independently approved native catalog,
controller/account fences, retry coalescing, receipt streaming and Store UI.
Combined client retirement and native billing gate1290662 passed504 Flutter tests
and2 actual Node cache tests, with0 failures/skips, analyzer0 issues and formatter
80 files/0 changes. Release web1295336 built successfully in38.7s. The first full
run1285036 exposed six missing ARB translator/placeholder metadata contracts and
34 files of Dart-version formatting drift; only metadata and mechanical format
were corrected. Follow-up1288725 exposed one formatted test conditional requiring
braces; final1290662 passed after that correction. No translated wording or
behavior assertions were weakened. Build output retained an existing Cupertino
font-family warning; no native platform, real store or device result is claimed.

**Non-goals and risks.** No live charge, provider setup, credentials, launch
application ID, store price or subscription proration is invented. No new ad,
consent/doubler policy, RTDN or cross-account purchase transfer enters this slice.
Actual configured Play/StoreKit purchase, restore, refund, pending payment and
renewal tests on platform accounts/devices remain Phase5.17b. Linux cannot supply
Xcode/iOS device evidence; an SDK mock or unsigned Android build does not close
that gate. Native APIs can return after timeout/disposal, so lifecycle fencing
and continued transaction ownership are required before rendering success.


**Implementation evidence (source frozen for review).** The app now uses the pinned
official SDKs with exact configured products, native prices, typed immutable receipt
proof and an app-owned bounded queue. Google never consumes or acknowledges in the
client; Apple finishes only after durable server acceptance. Store ownership comes
from the authenticated account, including permanent legacy Premium; provider
management requires a retained source. Custom Avatar is purchasable only with an
explicit available/unowned pair. No purchase or receipt path adds local value.

- Catalog/authenticated HTTP RED `1103384` → GREEN `1109467`; Premium projection
  RED `1130381` → GREEN `1139758`. New sorted/immutable/verifier-presence and real
  PostgreSQL account-source tests passed `1194300` (2 tests); that run's handler
  package hit a separate in-progress rematch-test typo. After its owner fixed it,
  the retained HTTP catalog/receipt test passed `1201913` (1 test, race, 1.558s).
- Initial DTO/controller/native adapter missing-symbol REDs `1109364`, `1123836`,
  `1126559` preceded implementation; targeted greens `1119491`, `1125512`,
  `1127132` establish actual typed HTTP/native wrapper/controller behavior.
- Concurrent dialog/receipt serialization RED `1127921` → GREEN `1134860`;
  retry-burst and wallet/catalog account-switch RED `1158092` → GREEN `1183499`
  (54 tests). Controller handoff/ensure/query switches are also covered.
- Streamed response total-deadline RED `1161952` → GREEN `1183499`; the same
  suite verifies immediate cancellation above 2048 bytes. Background restoration
  RED `1185569` → GREEN `1187435` (47 auth/OAuth/controller/native tests). A real
  AuthService storage-loss regression then reproduced unwanted device signup
  (`1210627`); receipt verification now calls only saved-session restoration,
  and all 58 data/auth/OAuth/native tests passed `1213855`.
- Minimal StoreKit pending/cancel callbacks have no account token: RED `1147119`
  → GREEN `1150554` verifies launch-only labeling without binding paid proof.
  Store route disposal, pending/verified/restore status, failed wallet refresh,
  Arabic/pseudo-localized 320px/2x layouts and owned Premium controls passed in
  the 94-test scoped suite `1199812`. Custom-avatar availability/owned behavior
  RED `1193853` → GREEN `1196980`.
- Final self-review found that queued-work completion did not notify the Retry
  control, and a later native catalog error erased the verified label. Both
  behavior failures reproduced in `1206197`; queue notifications and separate
  status preservation passed `1206907` (22 controller/lifecycle/Store tests).
  A controller recreation with native retained-transaction replay also passes;
  this is a synthetic lifecycle proof, not a real process/device store test.
- Mechanical analyzer findings were corrected with scoped Dart fixes. `1213855`
  left only two then-moving legacy media files; their owner subsequently retired
  them with transferred tests. Independent review and the final combined official
  client/analyzer/format/web/Node verifier remain pending this source boundary.

Flutter doctor `native-purchase-toolchain--20260912T190346Z-1214135.log` reports
Flutter3.47.3/Dart3.13.3 and no Android SDK; only Linux is connected. No OS toolchain
was installed. Android native build, Xcode/iOS build, configured store accounts,
real transactions and pending/restore/refund/renewal device evidence remain open.
Example application identifiers and debug release signing were preserved rather
than replaced with invented launch configuration.


### Legacy executable retirement — inventory and implementation plan

Scope: Phase 6 items 6, 7 and 9, followed by the item 10 inventory. This is an
implementation plan, not a claim that the remaining source has been retired.
The owner states no deployment exists; an observed live rollback window is N/A.
Historical source/data evidence and disposable upgrade/restore proof still have
to survive. Required reading: Blueprint protocol and infrastructure chapters,
ADR-012, the transition-design retirement matrix and corrected VPS runbook.
The coordinator owns tracking, staging and roadmap changes.

CodeGraph source/call maps were supplemented by exact Go imports, Dart relative
and package imports, and actual container `go list -deps -json` output. The
current server command compiles packages containing the old game, bots, lobby,
handler, wire and media files even though main no longer constructs them.
The read-only inventory artifact is
`/tmp/agent-runs/legacy-retirement-inventory-20260912.json`. Server graph: 362
packages; mediapack graph: 126 packages, both complete. Commands and evidence:
`/tmp/agent-runs/legacy-json-server--20260912T181848Z-1097405.log`,
`/tmp/agent-runs/legacy-json-mediapack--20260912T181848Z-1097400.log`.
Gamebot graph is explicitly incomplete (279 emitted packages): its module lacks
the `golang.org/x/crypto/ocsp` dependency/sum now required by the Apple verifier.
`/tmp/agent-runs/legacy-json-gamebot--20260912T181848Z-1097430.log` records that
error. Fix with official module tooling before its next gate, not hand-written
checksums. No ordinary services, migrations or source were changed by inventory.

#### Concrete source inventory and ordered ownership partitions

| Partition and owner to assign | Exact production change | Retained boundary / prerequisite |
|---|---|---|
| A — Server gameplay worker | Remove `server/internal/game/{match,payload,poke,types}.go`; remove `server/internal/lobby/{deps,manager,room}.go`; remove `server/internal/bots/manager.go`; remove `server/internal/handler/{handler,room}.go`. | Keep all `game/text*.go`, `lobby/text*.go`, handler text/auth/account/report/economy/community endpoints. Old `generateRoomCode` and `randomHex` in lobby have no text caller; text uses its own code/seed functions. Old bot nickname recognition has no retained production caller. No shared type needs to keep the old Match alive. Finish C's economy/wire bridges in the same compile boundary. |
| B — Catalog/CLI worker | Reduce `server/pkg/media/bundle.go` to the active `ContentHash` utility and current package comment. Remove `server/pkg/media/{loader,dealing,certify,manager,static_image,urls}.go`, `server/internal/workbench/workbench.go`, `tools/mediapack/internal/generator/generator.go` and `tools/mediapack/internal/bundlewriter/writer.go`. Rewrite `tools/mediapack/cmd/mediapack/main.go` to dispatch only current text commands, with explicit old-command refusals before reading/writing paths. | `ContentHash` is used by text bundles, stored release lineage and tools; keep it. `Cosine` has only old dealing/workbench callers; current text duplicates use their own reviewed evidence. Keep every `media/text*.go`, synthetic text generator, all current text CLI commands and `media.WriteTextBundle` as the sole writer. Remove the old watcher entirely, not just its route registration. |
| B — Candidate entry point | Rewrite `tools/mediapack/prepare_candidates.py` as a bounded retired-command refusal directing operators to existing text preparation; it must not import Pillow, open candidate files, normalize images, make network calls or create output. | The retained entry point is an explicit unsupported-input error, not an alternate content pipeline. Existing historical candidate/pack bytes remain evidence. No new parallel text-preparation Python implementation. |
| C — Coordinator/server bridge worker | Remove `server/internal/transport/{protocol,websocket}.go`; make `transport/health.go` depend on explicit `RuntimeReady` plus PG/Redis and remove `StoragePing`, `Media` and obsolete `Connections` fields. Remove notices' `Broadcaster`, `broadcastNotice`, legacy transport import and constructor type assertion from `server/internal/notices/notices.go`; retain `SetChangeNotifier` and authoritative refresh/poll delivery. | Move every audit/broadcast test probe to the current invalidation callback, preserving post-commit timing, failure/retry and no-early-notice assertions. Missing runtime authority must fail readiness closed. Keep health, middleware and all `transport/v2` code; change the old v2 test assertion that Phase 1 must retain `ProtocolVersion == 1` into actual v1 refusal. |
| C — Economy bridge worker | Remove `server/internal/economy/grants.go` and the old `CanQueueQuickPlay`, `RecordQuickPlayMatch`, `CheckCooldown`, `RecordAbandon`, `nextCooldownSeconds` methods in `economy/economy.go`. Their production callers are exclusively the old manager/room. | Keep `Manager`, constructor, `DB`, `serverDay`, wallet, ledger constants, billing/refund/named benefits and conversion. TextValueStore owns durable admission and settlement. Inventory follow-up found no active text cooldown writer/check: preserve this required behavior through a reviewed durable replacement before deleting the old cooldown methods. Rewrite only obsolete economy tests to use real transactions. Do not touch current subscription/acknowledgement or avatar work. |
| D — Client worker | Remove `client/lib/presentation/screens/game_screen.dart`, `presentation/state/{game_session_provider,game_actions}.dart`, `domain/entities/{game_session,player_identity}.dart`, `data/models/game_state_dto.dart`, and all four `client/lib/media/{asset_cache,media_engine,media_models,pack_sync_service}.dart` files. Remove old provider construction, transport and ProviderScope wiring from `client/lib/main.dart`. | Root already removed Home's v1 route fallback; preserve that current source. The old provider is still created by main and referenced from debug chrome, so this is an active dependency repair as well as dormant source removal. Keep TextSession/reducer, WebSocketTransport/GameTransport, auth/account/community/store actions, quick-chat identities, routing and cache-generation cleanup. |
| D — Shared UI extraction | Keep `GameCountdown`, `gameSecondsLeft` and `GameRoleSeal` in `game_surfaces.dart`. Move the unchanged finite `GamePokeFeedback` there, update its active text imports, then remove `dev_tools_panel.dart`. Remove only old `GameLobbyShare`, `GameMediaWell`, `GameResultReveal`, `GameCardTile` and their DTO/image imports. Remove DevToolsButton/header insertion and `showDevTools` plumbing in `ko_ui.dart` and its callers. | TextMatch actively uses the countdown, private hold-to-reveal seal and finite poke feedback; they must remain with existing semantics, lifecycle and reduced-motion tests. Remove specialty/dev-role/freeze/restart controls; explicit server-only authenticated prototype tools remain. QR/deep-link/clipboard functionality belongs to current TextPlay and shared route helpers. |
| E — Config/dependency coordinator | Remove unused top-level `StorageConfig`, `MediaConfig`, `BotsConfig` and historical Config fields only after A–C compile. Remove obsolete test runner cloud-key fixtures. Prune only verified unused module dependencies through official tools after all callers/tests have migrated. | Retain strict retired-key rejection before interpolation. Persisted tuning policy is a separate versioned compatibility boundary, described below; do not simply delete its JSON fields. Existing mandatory DB/Redis/JWT, optional configured provider secrets, avatar handling and explicit unavailable content remain unchanged. |
| F — Gamebot worker | Rewrite `tools/gamebot/main.go` to retain only text simulation, replay and authenticated prototype network execution; remove v1 envelope/constants/bot/deviceAuth/websocketToHTTP implementation and room/queue/count fallback. Default network URL must select `/ws/v2`; reject ambiguous/missing/obsolete command flags without opening a connection. | Preserve `text.go`, `text_network.go`, stable seed/replay/private output handling, production refusal before admission, authenticated development identities and all ten mode/size real network proofs. |

These partitions are assignments to coordinate before editing; they are not
permission for overlapping workers. A and C are one atomic build boundary
because economy/notices/health retain references to legacy game/wire types.
B can first prepare its tests while A/C remove the old consumers; delete media
APIs only after those imports are gone. D can run independently. F follows the
same server package boundary and the immediate module dependency repair.

#### Persisted policy identity must survive type retirement

`config.TuningConfig.SHA256` hashes typed JSON under `text-tuning-v1`.
`TextMatchRecord.Policy` is stored with the match and used for award/settlement
replay after configuration changes. Removing specialty/prefetch/reveal/shuffle,
legacy band or bot fields from the live struct changes its serialized identity
even when their values were zero. This is not ordinary YAML cleanup.

Before removal, freeze a read-only version 1 policy representation/decoder with
exact canonical bytes and old field order. Introduce an explicitly versioned
active-only policy format for new preparations, or preserve the version 1
serialization DTO separately from runtime configuration. The coordinator must
choose and document the smallest implementation before edits. The old decoder
must only read/replay existing durable value; it cannot create a legacy Match,
accept old settings or reactivate gameplay. Tests must store an original policy
with nonzero obsolete fields, restart under the new active configuration,
replay preparation/award/settlement and verify original contract/hash/amounts,
zero duplicate receipts and unchanged stored bytes. Do not regenerate old
certification evidence or silently relabel old contracts. New text pack
certifications retain their distinct dealing policy identity.

#### Test migration — exact files, no test-file deletions

**Requested test-file removal list: empty.** Preserve files and replace tests
whose old positive behavior is deliberately unsupported. Do not retain a second
engine only so its tests compile. Retain negative historical bytes as data.

| Existing test files | Substantive replacement obligations |
|---|---|
| `server/internal/game/{match_test,payload_test}.go` | Transfer role counts, turn ordering, draw penalties, timeout loss, 4/6 vote termination, live vote changes, Ready, reconnect/forfeit, complete begun-round evidence and synchronous callback/reentry proofs to TextMatch. Replace every specialty/out-of-turn-draw/hidden-hand positive case with rejection for all five modes, unchanged copy ledger/board/points and no hook calls. Include direct JSON old fields and malformed payloads. Preserve private/public projection boundaries. |
| `server/internal/lobby/{manager_test,room_lock_test}.go` | Exercise real TextManager FIFO tuple matching, capacity, explicit Ready revisions, no production backfill, current-socket generation CAS, stale grace timer, concurrent start, callback lock ordering and timer teardown. Preserve the exact race scenarios previously repaired, now on the surviving implementation. |
| `server/internal/bots/manager_test.go` | Keep this as a test-only package importing the active config/game/runtime boundaries; prove production cannot enable backfill or use development credentials, no synthesized lobby member/start after queue timeout, and authorized prototype recipients get only legal own observations. Do not add a no-op production bot manager to support tests. |
| `server/internal/handler/{handler_test,rematch_test}.go` | Preserve real socket serialization, idle ping, capacity, auth-before-admission, error correlation, rematch host/Ready and deep-link behavior on v2. Replace old HTTP room-create/join/QR API positive cases with obsolete-route refusal/no admission or debit. Reject old version/dev override/asset acknowledgment/specialty frames before mutation. |
| `server/internal/transport/{protocol_test,health_test}.go`, `transport/v2/contract_test.go` | Current strict v2 decode/encode bounds, unknown-field/version rejection and correlated errors; actual readiness requires owner/content policy plus dependencies and has no object-store branch. Remove the stale Phase 1 assertion that v1 must remain active. Preserve transport middleware tests. |
| `server/internal/notices/notices_test.go`, `server/internal/admin/mutation_authorization_test.go` | Replace legacy envelope probes with current change invalidation callbacks; maintain committed audit-before-signal ordering, failure recovery, expired actor refusal and scheduled/withdrawn notice behavior. |
| `server/internal/economy/economy_test.go` | Move first-win, completed/winner/correct-vote rewards, daily quota and cooldown cases to durable TextValueStore hooks/transactions, including UTC retry occurrence and idempotency. Retain all independent wallet/receipt/entitlement/convert tests and legacy row parity. |
| `server/pkg/media/{fixture_test,manager_test,media_test,media_test_helpers_test,static_image_test,urls_test}.go` | Test new text loader/catalog publication using existing immutable text fixtures. Preserve provenance, exact checksum, confinement/symlink, immutable snapshot and failed activation proof; reject legacy format1 packs and image/video/asset_ref/URL/embedding fields without serving/downloading payloads. Test returned copies and role-scoped content rather than signed URL success. Keep old fixture bytes unchanged for these refusals. |
| `tools/mediapack/internal/bundlewriter/writer_test.go` | Keep test-only package and call the existing canonical `media.WriteTextBundle`: immutable output/no overwrite, full manifest+member parity, wrong checksum/content type rejection and cleanup after failure. No replacement writer wrapper. |
| `tools/mediapack/test_prepare_candidates.py` | Keep recorded malicious/path/provenance cases, invoke the retired Python entry point and assert explicit refusal, no candidate file reads, no output mutation, no networking and no Pillow dependency. Existing text CLI tests cover successful normalized/provenance-bound text preparation and certification. |
| `tools/mediapack/cmd/mediapack/text_test.go`, `tools/gamebot/{text_test,text_network_test}.go` | Add old CLI command/flag refusal with no filesystem/network effects; retain all deterministic all-mode dealing/replay and authenticated prototype network evidence. CLI tests should execute the actual dispatcher, not only helper functions. |
| `client/test/presentation/{game_screen_test,game_session_provider_test,dev_tools_panel_test}.dart` | Rewrite old provider fixtures using production TextSession and TextMatchView. Preserve phone/tablet/desktop/pseudo/RTL layouts, semantics, keyboard selection, role-hold privacy, background cleanup, exact ballot/result timing, finite poke and rematch state. Assert no specialty/dev-role/hidden-hand UI or v1 frame on debug and normal entry paths. No assertion deletion merely because the old widget class is removed. |
| `client/test/domain/preserved_actions_test.dart` | Current mode-specific target selection/confirmation, only-current-turn draw, same-request retry, per-round poke and localized quick-chat identities; reject obsolete specialties/role overrides. Render actual authenticated seat labels rather than the retired bot nickname heuristic. |
| `client/test/media/{asset_cache_test,media_engine_test,pack_sync_service_test}.dart` | Preserve cache integrity and privacy intent through current exact snapshot/page hashes, bounded history, role cleanup and selective stale-generation cache invalidation. Capture fake HTTP traffic to prove no whole catalog/image/prefetch fetch after launch, reconnect or unsupported input; keep account/auth/locale preferences unchanged. |
| `client/test/{app_shell_test,websocket_transport_test}.dart`, `presentation/{home_screen_test,ko_ui_test,text_match_test,text_play_test}.dart` | Adapt removed provider imports without losing current navigation, cache cleanup, socket lifecycle, text legality and shared private/poke UI assertions. Keep root's latest no-v1-fallback tests intact. |

#### Dependencies, retained data and final evidence

After migrated tests have no legacy imports, official `go mod tidy` in all three
retained Go modules should remove `github.com/skip2/go-qrcode` (old HTTP room QR
only). Keep `github.com/boombuler/barcode` through admin OTP. No fsnotify
package was found: the retired workbench uses its own ticker, so do not invent
a dependency removal. `github.com/chai2010/webp`, `golang.org/x/image` and CGO
are still required by active avatar processing. `x/crypto` also remains for
bcrypt/OAuth/Apple OCSP. Keep http/crypto/shared_preferences/file_selector,
qr_flutter/url_launcher and font/icon assets on the client. Riverpod and
flutter_riverpod currently have only the legacy main/provider/debug/game
consumers; remove through Flutter tooling after the migrated test harness no
longer needs them. Billing native SDK changes belong to the billing worker.
Pillow can leave CI only when the retired Python path/tests no longer import it
and a full repository dependency search confirms no other retained consumer.

Preserve `content/packs/core-2026.10`, media golden/band-starved fixture JSON,
all applied migration SQL and historical reports/ADRs as explicitly classified
historical data. Preserve retained PostgreSQL blobs, source/approval/attribution,
purchase/entitlement/ledger identities, and isolated backup MinIO fixtures.
No blob, ordinary volume, receipt or migration history deletion is proposed.
UI font/license/app icons and active avatar bytes are not playable content.
Update current package comments, README/CLI usage, active localization strings
and transition matrix; never rewrite historical proof claims. Remove an ARB key
only after generated source and all retained tests have no consumer, preserving
all four locales and metadata. Add release notes documenting v1/old CLI/image
refusals and the replacement text-only paths.

Each partition begins with replacement tests and keeps its retained behavior
proofs. Behavioral refusals that previously performed work must show RED then
GREEN. A deliberate no-legacy-import/source architecture gate complements real
CLI/HTTP/WS tests; it does not replace them. Use independent source review then
cold verifier at each stable boundary. Run the unified runner after all owners
freeze, with all retained Go/Python/client suites and skip accounting. Rebuild
all Go modules, Web and Android; report the macOS/iOS gate separately if this
Linux host cannot execute it. Rebuild the actual production image and inspect
its dependency/route/packaged-file inventory; run fresh core-secrets-only PG/
Redis startup without MinIO. Rerun exact latest-head backup/restore parity after
migrations22/23 are frozen. Record actual test counts and source/image hashes;
no human playtest, provider production or load/soak claim follows from this
source retirement work.

Targeted gamebot module repair: official Go container `go mod tidy` added only
`x/crypto v0.55.0` to go.mod and the required module checksums (no upgrades or
removals). The first isolated attempt lacked network for uncached sumdb/test
dependencies; log1106179 identifies that environment boundary. Authorized
project-tool network retry1106573 exited0. Full offline dependency graph then
passed `/tmp/agent-runs/gamebot-graph-after-tidy--20260912T182459Z-1111739.log`;
raw graph is `/tmp/agent-runs/gamebot-graph-after-tidy-20260912.json`. The earlier
inventory remains an honest record of the pre-repair failure. Full gamebot
behavioral tests run at the stable retirement boundary.

### Stored tuning policy preservation — coordinator plan

**Goal.** Retire obsolete live configuration fields while preserving the exact
`text-tuning-v1` JSON identity and durable award/settlement replay of old matches.
**Boundary.** Keep version 1 as a frozen serialization DTO, separate from live
configuration. No new game rules, migration, stored-row rewrite or historical
certificate regeneration. The DTO can serialize/replay value policy only; it
cannot enable an old handler or read an obsolete YAML key.
**Files.** `server/internal/config/{config,text}.go`, discovered-absent
`policy_v1.go`/`policy_v1_test.go` and a config testdata policy fixture;
`server/internal/store/text_value_test.go`. Remove obsolete live fields only
after the server retirement owner removes their executable callers.
**Checklist/proofs.** Capture current typed JSON with nonzero obsolete values
before changing serialization, then assert fixed canonical bytes/hash through
JSONB-style key reordering and Clone. Freeze all version-1 field names/order/types
in the DTO, preserving omitted VoteResultFalling and nil/empty distinctions.
Custom JSON encoding maps active fields and retained historical-only values;
YAML uses active fields only and keeps existing retired-key refusal. Verify
immutable input/copy semantics and current default hash parity. Store the original
policy in a real prepared/started match, restart with changed active tuning,
replay preparation/award/finish/settlement and assert unchanged contract bytes,
original amounts and one receipt/grant. Retire active-only legacy fields, rerun
all affected config/game/lobby/store/handler/tool tests, then independent review
and cold verification.
**Risks.** Implicit field order, JSONB reordering, map aliasing, omitempty drift,
and accidentally reusing current nested structs could change a historical hash.
Copy the complete version-1 DTO and retain an immutable pre-change golden fixture.
Unknown historical fields must fail closed rather than silently disappear.

### Full avatar lifecycle — implemented server boundary and client handoff

The approved avatar slice now uses an explicit permanent unlock purchase, then a separate upload. Upload processing never grants ownership or debits Noin. `custom_avatar_available` is server-derived from configured image screening and `custom_avatar_owned` is returned only on the authenticated, non-cacheable catalog. Disabled screening refuses new purchases; an already-owned purchase replay remains free. The custom-avatar purchase transaction holds the account row, revalidates the initiating JWT before and after value writes, and preserves the existing named-item behavior.

Migration **23** adds monotonic `accounts.avatar_revision` and nullable `custom_avatars.revision`. Existing account values and historical blobs remain byte-preserved; NULL historical image revisions are never displayable. Successful uploads preflight dimensions before raster allocation, center-crop to 256×256, encode fresh WebP, screen those exact bytes, and atomically select/store a positive matching revision. Capture and activation check current ownership, account status, session epoch and revision around provider I/O. Presets and takedown advance the revision; takedown retains the paid entitlement and creates no refund. Failed screening, failed image writes, concurrent losing uploads, late revoked credentials and account transitions retain the previous image/value.

Root review identified and closed resource bounds: configured upload slots default to **2**, allow explicit **1–16**, and reject excess HTTP/trusted uploads before body reading or image allocation (`429 avatar.busy`). Slots release on errors/cancellation. Initial auth, capture and activation use independent three-second SQL limits; HTTP body reading has a ten-second deadline and the upload has a 45-second context. The provider has a bounded timeout and response size, fixed HTTPS moderation endpoint, no redirects, and requires one explicit `flagged:false` result. GET uses sorted viewer/target account locks, current revision/moderation/ownership checks, symmetric private block exclusion and uniform 404s; images use authenticated bytes, `no-store` and `nosniff`. Avatar PATCH now has a three-second context, strict single JSON body capped at 4 KiB, exact initiating JWT checks, preset allowlist and non-cacheable responses.

Concrete evidence:

- Actual disabled-upload RED **1011414** proved an unwanted blob, implicit entitlement and 1,000-Noin charge. Preset/takedown RED **1041878** proved arbitrary avatar selectors, missing revision fences and deleted ownership. Purchase RED **1083586** proved disabled-screening purchases still charged. These were repaired; six-package real PostgreSQL/Redis race run **1090675** passed avatar **7.094s**, profile **1.151s**, admin **33.735s**, handler **18.374s**, economy **15.674s**, config **2.644s**.
- Migration23 actual22-upgrade/legacy parity/idempotency/exact safe down/refused used down passed the store portion of **1085707** (**2.930s**). Earlier **1075586** incorrectly ran the destructive migration fixture concurrently with avatar tests; this was corrected to `-p=1`. The remaining read fixture error was a missing required block timestamp, corrected without weakening visibility checks.
- Resource-bound RED **1094384** proved excess requests read the body; GREEN **1100790** passed full avatar **7.837s** and config **2.688s**, including configured single-slot cancellation release and real capture/final account-lock timeout/no activation. Avatar PATCH/JWT/purchase gate **1106845** passed **4.352s**. Seven-package vet **1112575** passed.
- Client unlock RED **1093986** proved an unowned upload button; image fetch RED **1100603** and display RED **1106102** proved missing authenticated image support. The implemented controls explicitly confirm permanent purchase, never purchase while uploading, retain selected bytes on rejection, and use owned/no-extra-charge language. Approved images fall back to presets when missing and are scoped to viewer/target/revision. API fetch/upload responses are size-bounded, retry authentication once, and reject account changes. API+profile/widget gate **1109161** passed **40 tests**; API **1105667** passed **12 tests**. Added final stale-response/disposal tests are awaiting the billing lane's compile freeze: **1112188** caught its then-unqualified `_platforms` symbol and is a diagnosed dependent compile window, not an avatar product failure.

The seven-package server vet and source review cover local behavior only. Image-provider live credentials/acceptance, real provider/device evidence, and the combined official client/build gate remain unexecuted. No provider has been enabled, no player content was sent externally, and nothing is staged by this subagent. The parent retains final roadmap checkbox/staging responsibility after combined review and verification.

Stored-policy initial implementation evidence: pre-change typed JSON capture1113093
produced immutable golden SHA256 aeeb1bbfc2bc881635a04d504aa61a73168ea1ed36b00565dee44a5b6778fb7b
with nonzero retired fields. Strict unknown-field RED1114002 became config
GREEN1116582. Real PostgreSQL/race gate1119725 passed config1.023s/store2.745s
for historical JSONB replay, unchanged contract bytes, original award amounts
and receipt/value idempotency; the first DB gate1116957 only exposed a missing
JSON test import, now repaired. Source review and final live-field removal
remain pending until the retirement owner removes legacy consumers.


### Durable text abandonment and cooldown — bounded repair plan

Coordinator assigns baseline runner migration24 and the narrow store/engine/
lobby hook boundary. Blueprint Rules7 defines grace20s and escalating Quick Play
cooldowns; current tuning is60/300/900s. The legacy room invokes a best-effort
counter update at grace expiry, including Local Rooms and repeated callbacks.
Current text code has no equivalent check or writer. This is a behavior gap,
not permission to preserve an unused legacy manager indefinitely.

Implement an immutable `text_abandons` receipt keyed by match/account (seat and
first grace-expiry occurrence included), retaining prior count/applied count and
resulting expiry. Only an authenticated started non-prototype Quick Play match
may originate one. Lock match identity then canonical account then receipt/
queue_cooldowns; preserve prior legacy count and never shorten a later recorded
expiry. Compute configured escalation from the pinned match policy and the
frozen UTC occurrence, not retry time. Same receipt retry compares its immutable
identity and returns without another increment; changed seat/time/body refuses.
Migration24 adds receipt constraints/append-only protection and exact empty-only
down; versions1–23 stay unchanged.

Add a private engine abandonment hook collected when a disconnected player's
first grace deadline actually expires, before any resulting forfeit/low-pop
finish. Mark that first incident in the copied state so reconnect and another
disconnect cannot create a second identity for this match. Persist the hook
outside the game mutex through the existing pending-candidate mechanism; failed
writes keep the exact event and hold publication/acknowledgement until retry.
No public history/role-linked value field is added. The lobby closure binds
seat→account and the durable match owner/fence. Local/prototype behavior has no
cooldown write. A server Close or lost-owner recovery never invents an abandon
for its interrupted seats; already committed genuine grace-expiry receipts
remain valid. No forced reconnect or reward policy changes are included.

Check the canonical active cooldown during Quick Play Reserve and again during
Start under the existing account locks, rejecting `until > occurrence` while
allowing exact expiry. Local admission bypasses this matchmaking cooldown;
prototype admission remains zero effect. Reserving/leaving a queue, cancelling
an unstarted room, transient disconnect before grace and returning after a
server interruption do not count as abandonment.

Tests first: real-PG fresh+retained counts and escalating/saturated duration;
concurrent same receipt and changed identity; frozen UTC day/expiry on retries;
active boundary/exact expiry/Local and prototype exclusion; new cooldown between
Reserve and Start; rollback before receipt/counter commit; SQL immutability and
empty-only down. Engine fake clock tests cover before/exact grace, reconnect,
second disconnect, terminal forfeit ordering, callback reentry and failed-write
retry. Real engine→store integration proves one receipt, no value/ledger changes,
no blanket cooldown after infrastructure interruption and fence refusal after
owner loss. Remove old cooldown methods/tests only after these equivalent active
proofs are green. Independent coordinator review precedes cold full affected
package verification and the final unified gate.

### Concurrent network and soak budget — coordinator execution plan

**Goal.** Exercise one real authenticated text server with 100 simultaneously
started rooms (ten of each five-mode/4-or-6-seat cell), then prove completed
match cleanup and measure latency/frame/heap behavior on this recorded host.
**Boundary.** Extend the existing guarded gamebot real PostgreSQL/Redis fixture;
all identities/content remain explicitly synthetic prototype fixtures and value
ineligible. This is engineering load evidence, not live human/content economy
or low-end client proof. No production load endpoint, public telemetry label,
provider request or OS tuning is added.
**Files.** Discovered-absent `tools/gamebot/text_budget_test.go`; narrow reusable
fixture counters in `text_network_test.go`; existing gamebot networking/policy
helpers remain authoritative. Add pure budget calculation tests as needed.
**Checklist/proof.** Record Go/OS/architecture/CPU/GOMAXPROCS, source hashes,
config/pack hashes, limits and PostgreSQL/Redis versions. Warm the same fixture,
then hold all 100 rooms started before a barrier-driven concurrent action loop.
Each agent chooses solely from its authorized snapshot. Shared deterministic
clock advances only when all unfinished rooms have no legal action; every match
must reach an ordinary verdict without errors/forfeit/interruption. Measure full
client intent-to-ack round trips as conservative upper bounds on server handling,
and report separate idle loopback round trips without subtracting percentiles.
Keep configured frame/request/history bounds unchanged and record worst frame.
After all 100 completions, leave/close every socket, settle delivery queues,
advance only the existing expiry clock, force GC and verify room/socket/goroutine
counts recover and retained heap is within 10% of warmed baseline. Cap total
runtime and trace retention; failure must retain explicit measured evidence.
**Risks.** The existing global manager lock may exceed latency budgets; a red
measurement is a finding to fix, not grounds to loosen thresholds. In-process
client allocations affect heap measurements, so report that limitation explicitly
and retain a separate-process follow-up before calling server-only heap proven.
Independent review and a cold run precede any budget checkbox claim.

### Avatar final review repairs and frozen client boundary

Coordinator independent server review approved the repaired upload semaphore,
3-second capture/final SQL contexts, same-account PATCH fence, and immutable
head23 migration. Cold real PostgreSQL/Redis race1122280 passed avatar7.919s,
profile1.055s, handler4.075s and exact migration23 parity/down2.921s. Separate
EXIF/crop proof1114259 passed1.490s with a JPEG containing actual EXIF bytes;
the stored WebP is fresh256x256 centered pixels and contains no source metadata.

Client review reproduced three remaining defects in1125664: an old preset
mutation could retry under a switched account; image/upload response trickles
extended per-chunk timeouts indefinitely; old picker completion released a
replacement widget's busy state. Corrected catalog probe1125980 additionally
proved the old picker invoked the replacement API. Avatar-only expected-account
fences now run before send, after response, and across refresh (generic nickname
behavior is unchanged). Image/upload reads own one15s total timer and bounded
subscription cancellation on every completion; byte caps remain2MiB/64KiB.
Picker file/native/decode waits check captured generation and use captured API;
FormatException and completion cleanup cannot mutate a newer operation.

The first testWidgets clock variant1126724 and diagnostic1127052 reached all
assertions and stream cleanup but stalled in widget-test finalization; both were
stopped and recorded as incomplete, not passing. The data-only tests now use
deterministic FakeAsync, preserving the exact deadline/cancellation assertions
and additionally requiring zero pending timers. Existing resolved fake_async1.3.3
was promoted to a direct development dependency through Flutter pub tooling; no
version or OS package changed. Full focused API/account widget gate1129720 passed
all47 tests in5s, including the recent identity/disposal/rejection regressions.
The combined official client/build gate awaits the concurrently implemented
native purchase bridge compile freeze. No actual image-provider/device evidence
or release activation is claimed. Parent retains final review/tracking/staging.

Owner-lease independent review also approved the frozen policy-v1 current schema,
canonical golden hash, receiver-preserving strict decode and map/slice clones;
the live-to-frozen bridge was tightened by coordinator after the forward-field
maintenance suggestion. Concurrent100-room budget plan reviewed: preserve the
actual started-room barrier, all roundtrip samples including persistence waits,
resource-count cleanup and source/host evidence; process-wide heap evidence must
not be labeled server-only.

Avatar final source/style follow-up1141758 passed scoped analysis with zero issues, format and all47 API/account tests again (6s). The analysis1134949 findings were concrete brace/style and helper-mounted recognition diagnostics, fixed with scoped Dart tooling and an explicit post-await mounted check. Frozen client manifest: `/tmp/agent-runs/avatar-client-source-frozen-20260912.json`. Shared API/account files may subsequently change in the independently owned billing slice; review avatar hunks against this captured boundary.

### Client legacy retirement partition D — bounded inventory and execution plan

**Goal.** Ship a client whose only playable session/rendering path is the typed
five-mode text implementation, preserving private UI lifecycle, account services
and every retained test file. This implements ROADMAP6.6/7a/7b/9 and contributes
to6.10; it does not certify device performance, content release or the whole
transition. Required reading completed: Blueprint Network/Infrastructure and
Secrecy chapters, ADR-012 and transition retirement matrix. No deployment exists.

**Inventory.** CodeGraph was consulted before source/import inspection. The
current source/test import inventory and all discovered old test case names are
saved in `/tmp/agent-runs/client-legacy-partition-d-inventory-20260912.json`.
The eleven removal candidates total3865 source lines: GameScreen1509,
GameSessionNotifier961, GameActions95, GameSession248, player_identity13,
game_state_dto426, AssetCache62, MediaEngine78, MediaItem/PackManifest78,
PackSyncService128 and DevToolsPanel267. GameScreen has only test callers now.
Main still constructs the old transport/provider; KoPage debug chrome reaches
DevToolsPanel, which reaches the old provider. TextMatchView imports only
GamePokeFeedback from that debug module. The four media files have no surviving
production caller outside their own closed dependency chain. GameLobbyShare,
GameMediaWell, GameResultReveal and GameCardTile have only old GameScreen/internal
callers; text QR/deep-link sharing already lives in TextPlayScreen. Home production
code no longer imports Riverpod or the old provider; its tests retain a v1 spy
harness that must be replaced by actual v2 transport/no-admission observations.

**Boundaries.** Preserve GameTransport/WebSocketTransport, TextSession/V2Reducer,
private settled delivery state, identity/OAuth/avatar/purchases, shared QR/route
helpers, clocks, typography and the exact selective cache-key cleanup. Preserve
all historical SQL/fixture/report bytes. No server, economy policy, native billing
SDK or provider configuration changes belong here. No test file is proposed for
deletion. Old positive specialty/free-draw/dev-role assertions become explicit
unsupported-input/no-send tests; neither a no-op legacy engine nor aliases will
be retained just to compile tests.

**Exact source edits.** Remove `client/lib/presentation/screens/game_screen.dart`,
`presentation/state/{game_session_provider,game_actions}.dart`,
`domain/entities/{game_session,player_identity}.dart`,
`data/models/game_state_dto.dart`, all four `client/lib/media/*.dart` listed above
and `presentation/widgets/dev_tools_panel.dart`. In `game_surfaces.dart` retain
GameCountdown/gameSecondsLeft/GameRoleSeal and move the unchanged finite
GamePokeFeedback animation there, with only required imports/comment updates;
remove the four DTO/image/share/result legacy widgets. Remove DevToolsButton and
showDevTools constructor/header branches from `ko_ui.dart`, then the three
no-longer-needed argument sites in `safety_screens.dart` and `text_play_screen.dart`.
Change TextMatchView's one GamePokeFeedback import. Root owns Home/TextPlay; request
these exact shared import/argument hunks at approval, no other screen edits.

Billing worker owns main/AppConfig/Store during native integration. It confirmed
main now starts `services.purchases` and wraps KnowoffApp in PurchaseLifecycle.
After coordinating a stable file boundary, remove only legacy transport creation,
Riverpod import and ProviderScope/provider override from main, retaining services
initialization, cache upgrade and that purchase lifecycle exactly. AppConfig has
no legacy dependency to remove and is outside this implementation.

**Ordered checklist and proof mapping.**

- [ ] Capture actual RED for debug KoPage mounting obsolete controls/provider and
  no-session app mounting; add v2 frame/privacy assertions before source removal.
- [ ] Extract the three retained game surfaces and migrate
  `client/test/presentation/dev_tools_panel_test.dart`: debug/default headers have
  no freeze/grant/role/restart controls or provider requirement; negative legacy
  actions emit no frame; unchanged countdown accessible/frozen/resume behavior;
  finite900ms poke, stable child layout, reduced motion and disposal cleanup.
- [ ] Rewrite `client/test/presentation/game_session_provider_test.dart` using
  production TextSession and existing FakeTextTransport/hello/admit fixtures.
  Transfer auth-before-hello, rejection/no-loop, reconnect/same-seat/new epoch,
  board+hand authoritative updates, count-only draw, Ready/ballot/error correlation,
  server deadline retention, rematch and complete reset. Old hidden-hand, specialty,
  free-draw, anonymous shuffle, client freeze-buffer and dev-role positives become
  strict rejection and no private state/frame mutation across all five modes.
- [ ] Rewrite `client/test/domain/preserved_actions_test.dart` against V2Reducer
  and TextMatchView: exact copy/target/rating/slot/offer action per mode, no early
  automatic play, no out-of-turn draw, immutable pending retry ID/body, current
  round poke limits, locale-bound canned/free chat, legal ballot candidates and
  retired action rejection. Human seat nickname data never invokes bot heuristics.
- [ ] Rewrite `client/test/presentation/game_screen_test.dart` around actual
  TextSession→TextMatchView observations, retaining phone/tablet/desktop, all
  five boards, 4/6 seats, enlarged pseudo/RTL, semantics/keyboard confirmation,
  stable hand/selection through public updates/rotation, evidence ordering,
  role hold/background/elimination secrecy, server reveal boundary, ballot/runoff,
  trade recipient choices, final begun-only Nowns and rematch/Ready. Remove only
  assertions tied to deliberately retired powers by replacing them with absence
  and rejected-wire tests. Current TextMatch test suite stays intact.
- [ ] Rewrite `client/test/media/asset_cache_test.dart` to verify authenticated
  snapshot/page integrity: wrong hash/duplicate page/conflicting content, configured
  history bound and immutable owned snapshots, privacy clearing on gap/disconnect.
  Rewrite `media_engine_test.dart` to capture actual transport/HTTP boundaries on
  text launch/reconnect and malformed image/catalog/prefetch input: no catalog or
  image fetch and no asset acknowledgment; role-scoped snapshot still restores.
  Rewrite `pack_sync_service_test.dart` for exact selective legacy cache invalidation
  (normal/corrupt/unsupported cached values, repeat run, fresh install), retaining
  account/token/locale/avatar/last-mode preferences and proving no refresh fetch.
- [ ] Adapt `home_screen_test.dart` and `app_shell_test.dart` away from old provider
  harnesses while retaining all root route/selected-size/API/unsupported-version
  assertions and native/web deep links. Use current FakeTextTransport or real
  socket counts; do not replace the old send spy with a never-used dummy. Update
  `text_match_test.dart` moved poke import, preserve `ko_ui_test.dart` semantic
  targets and verify header now returns debug-control space to content. Retain
  generic WebSocket transport lifecycle tests with canonical v2 example envelopes.
- [ ] After all callers migrate, remove eleven source files and dead shared
  classes, then re-run all mapped tests. Record old-case→replacement-family mapping
  against the inventory's62 provider/40 screen/10 debug/9 action/13 media case
  declarations so none is silently abandoned. Loop expansion adds actual cases.
- [ ] Scan exact remaining imports/localization references. Remove riverpod and
  flutter_riverpod only through Flutter pub tooling after their last test/main
  caller disappears. Retain http/crypto/shared_preferences/qr_flutter/file_selector,
  fonts, WebP/avatar and native purchase dependencies.109 ARB keys are initial
  removed-file-only candidates, not an automatic deletion list: recompute after
  shared surface extraction and test migration, then remove only unreferenced
  obsolete gameplay/dev keys plus metadata from all four locales, preserving
  ongoing billing keys via coordinated fresh reads. Regenerate localizations.
- [ ] Independent source review, full official client runner (tests, analysis,
  format and Node cache tests), release web build, final no-legacy import/package/
  generated-artifact scan. Android/macOS iOS/device proof remains separate actual
  toolchain evidence; desktop test success does not stand in for it.

**Risks and review gates.** This is a large test transfer despite a simple product
change; maintain the inventory mapping at each boundary and never reduce privacy
assertions to mere source-string scans. A RED that reveals a real TextSession/UI
defect will be reported with the smallest dedicated regression before any fix.
Removal of Riverpod requires all shell/test wrappers to move in the same compile
boundary; native billing startup/disposal must remain unchanged. The existing
selective cache purge remains an explicit historical-key compatibility cleanup
with no catalog reader or old engine behind it. Parent source-plan approval is
required before this subagent mutates the shared implementation, then review and
verification at each bounded transfer boundary; final staging remains parent-owned.

### Coordinator verification and load execution — 2026-09-12 18:50 UTC

Frozen avatar client delta source review approved: preset account fences survive refresh, streamed image/upload responses own total deadlines and byte caps, and picker/upload/purchase completion uses API/generation/account guards. Independent cold1146758 passed all avatar/API checks (46 total passing events) but the combined file failed its Store placeholder expectation during native billing edits. The exact log was read; billing owner is replacing obsolete non-native bundle advertising with the real availability contract while preserving pass/conversion/wallet assertions. No combined-client green claim yet. Server23 independent1122280 remains approved.

Resource-count unit proofs1137145 passed with actual PostgreSQL/Redis and lobby race instrumentation; gamebot percentile proof also passed in the CGO-enabled project Docker runtime. Native CGO-disabled1131837 could not compile the preserved WebP avatar dependency; it was diagnosed from the log and the official project test image resolved this environmental mismatch. Engineering load1147966 reached 100 active rooms and500 real sockets after10 completed warm-up matches, with complete input hashes, runtime/host metadata and raw latency samples being recorded. Completion, memory and percentile results remain pending; this is prototype/no-value loopback evidence, not human/provider or low-end client rendering evidence.

Native billing source review requested per-work queued retry coalescing, captured-account catalog/entitlement handoff, and a bounded streamed receipt-verification response. These are implementation findings, not owner policy questions. All remaining executable roadmap work continues; no staging window opened.

### Durable value TRUNCATE boundary — coordinator bounded plan

Scope: Phase5 child19 DB enforcement gap, additive migration25 reserved for the coordinator after24. Existing10 rejects row UPDATE/DELETE but PostgreSQL TRUNCATE bypasses row triggers. Add a statement-level refusal to the retained value identity tables: noin_ledger, text_award_receipts, text_first_win_claims, text_settlements, text_outbox and leaderboard_history. A shared trigger checks actual retained rows with row_security=off; empty-table truncation is harmless and remains allowed. Nonempty direct or cascading truncation must fail atomically, including under an explicitly granted non-owner role. No correction/deletion/retention bypass is introduced. Runtime DB role provisioning and broader table policy remain separate work, not falsely closed by this patch.

Proof order: real PostgreSQL RED before migration25 for direct ledger loss and cascade receipt loss; apply exact24→25 without changing row bytes/sequences/FKs; fresh→25 parity, repeat migrate idempotence, non-owner INSERT/SELECT permitted while UPDATE/DELETE/TRUNCATE refused; RLS-filtered rows cannot be hidden from the retained-history guard. Simultaneous transaction append/truncate serialization must preserve committed history. Empty-only exact25 down is allowed only if all protected tables are empty; any retained value requires forward fix. Test fixtures that intentionally destroy retained history must use their existing uniquely guarded disposable schema reset, never weaken the production trigger. Preserve all old migrations and test files. Review before source edits; current100-room engineering run remains isolated.

Partition D first shell/extraction checkpoint: behavior RED1165882 found the
compact and regular KoPage still mounted dev-tools-open. GREEN1173209 passed66
app-shell/debug-transfer/current-text/KoUI/purchase-lifecycle tests in5s after
removing active debug controls and main's legacy transport/provider construction.
Native purchase start/lifecycle is preserved. GamePokeFeedback moved verbatim
into game_surfaces; current text callers now import it there. Existing app-shell
tests mount without Riverpod.10 original debug test declarations map to current
header48dp/absence checks, all5 mode retired-power no-frame/no-state-mutation
checks, small pseudo shell reachability, unchanged countdown labels/frozen resume
and finite reduced-motion poke checks. Old dev_tools_panel.dart temporarily holds
only the original devEchoPokes value used by old GameScreen test fixtures; it will
be removed with GameScreen in this same partition, not retained as a compatibility
API. Old card/media/result widgets also await their mapped tests. Frozen manifest:
`/tmp/agent-runs/client-retirement-shell-frozen-20260912.json`.

### Ledger truncation RED→GREEN and load failure diagnosis

Real PostgreSQL1172282 reproduced all seven retained-history loss paths: each of the six value tables and TRUNCATE accounts CASCADE. Migration25 adds retained-row statement triggers with row_security=off and an exact empty-only rollback. Initial realPG/race1177473 passes all seven refusals and preserves reconciliation; nonowner/RLS/concurrency/migration parity proofs remain in progress, so25 is not frozen.

Engineering load1147966 reached100rooms/500sockets, then failed on an action.persistence_pending vote at355.99s. Logs were read: later teardown attempted pending awards with canceled request contexts. This does not establish the cause of the first rejection. Production Action currently holds the global lobby mutex through owner checks, apply/persistence and all-recipient projection; the next run1179595 captures CPU and per-step refresh timings to test contention rather than widening timeouts. The load fixture also incorrectly stopped reading early-finished spectators while other rooms played, risking websocket ping expiry; this was corrected before the diagnostic run. No load/soak completion claim is made.


### Retained value history — reviewed migration 25

The six retained ledger/award/claim/settlement/outbox/weekly-history tables now refuse direct or cascading TRUNCATE when populated, including restricted callers whose RLS view hides rows. Empty fixtures remain valid. Exact down locks all six tables and refuses retained history. Root actual-PG race proof1190093 passed four tests/15 events; independent source review and cold race1207247 passed the same15 events in6.194s. Migration25 is frozen. Existing admin/economy/leaderboard setup now uses explicitly token-guarded disposable schema reset instead of truncating retained history; full package race1201446 passed72.426s/38.508s/2.939s. Runtime least-privilege role provisioning remains open.

### Snapshot serialization performance — bounded repair plan

**Goal.** Remove repeated history hashing/pagination from recipient delivery while retaining byte-identical v2 history evidence and all privacy/frame validation.

**Evidence.** Actual100-room/500-socket run1147966 failed355.99s with action.persistence_pending. Keeping completed spectator sockets actively reading did not resolve it: diagnostic CPU run1179595 failed385.59s; all-room refresh grew from972ms to5.03s, and canonical JSON writing alone accounted for89.17s cumulative/12.79% sampled CPU. Source inspection shows every candidate page recalculates its growing-prefix hash and the lobby discards/rebuilds the engine's already-paginated history. Profile includes Go clients and concurrent workspace activity, so it is diagnostic evidence, not a frozen acceptance measurement.

**Non-goals.** No threshold changes, omitted history, cached authorization/block decisions, relaxed contract validation, wire/hash version change, or lock-order redesign in this repair.

**Touched files.** server/internal/transport/v2/history.go and existing contract tests; server/internal/game/text_snapshot.go and existing text snapshot tests; server/internal/lobby/text_runtime.go; root gamebot budget harness only.

**Test plan.** Frozen reference pagination must produce exact equal page boundaries, hashes and manifest across tiny/large frames, Unicode and optional nested event fields; caller mutation must not alias returned pages. A detached engine projection must retain all history beyond a frame, remain role scoped, and resist caller mutation. Existing full transport/game/lobby/handler race and network secrecy/paging tests follow, then actual100-room budget rerun with unchanged thresholds.

**Checklist.**
- [x] Add reference/parity and detached-projection regressions and record pre-change failure.
- [x] Hash only finalized page candidates (fixed-width placeholder permits exact size checks); retain final deep copy.
- [x] Expose explicitly internal, detached complete projection; paginate once after recipient chat filtering at the lobby output boundary.
- [ ] Independent source review, scoped race/privacy tests, and fresh load/soak evidence.

**Risks.** Hash encoding/page boundaries or nested aliasing could break integrity or privacy. Exact parity, mutation tests and complete existing v2 tests remain gates; the new complete projection is not a directly sendable wire frame. Further global-lock work requires a separately reviewed plan if this repair is insufficient.

### Legacy retirement A+C — server compile boundary

The approved server partition now removes the legacy image/specialty match, lobby, production backfill actor, v1 handler/room routes and transport envelopes, plus the superseded free-quota/cooldown/grant methods. Thirteen production files are removed; **no test files are deleted**. The Unicode profanity masker is moved unchanged from the retired poke implementation into active `game/text_chat.go`, because text moderation still consumes it. Health requires `RuntimeReady`; notices retain their committed audit and retryable public invalidation poll.

Existing test paths now exercise actual five-mode v2 privacy/draw/timeout behavior, exact human FIFO and explicit Ready, current-socket generation and concurrent start/disconnect, complete WS rematch/replacement flow, v1/specialty refusal, offline simulation without human takeover, and durable PostgreSQL quota/cooldown/first-win/discreet settlement behavior. Shared v2 Phase 1 transition assertions now verify the active decoder rejects v1. The economy test helper also refuses any DSN without the existing disposable token/database identity contract.

Pre-removal replacement proof: handler race `retirement-handler-transfer-seed--20260912T190054Z-1206456.log` passed (6.100s); offline all ten cells `retirement-bots-transfer-bounds--20260912T190510Z-1217301.log` passed (1.496s); five durable economy tests `retirement-economy-transfer-corrected--20260912T190454Z-1214919.log` passed (2.028s); two notice atomicity tests `retirement-notice-transfer--20260912T190523Z-1217580.log` passed (1.306s). The early admin regex selected no tests and is not admin verification evidence. All logs are under `/tmp/agent-runs/`. Initial rewritten fixture failures were corrected from their logs (field names, required initial system seed, exact persisted award kind, frame limit); they are not claimed as production defects.

Post-removal all server packages compiled with `retirement-ac-compile-shared--20260912T190715Z-1221472.log`. Frozen review manifest: `/tmp/agent-runs/retirement-ac-frozen.json`. Structural and behavioral post-removal gates and independent reviewer → cold verifier are still pending; no completion claim for the whole retirement phase. Config historical serialization, media/tool partitions, and gamebot CLI retirement remain separate owned steps.

A+C post-removal local checks: `retirement-ac-behavior--20260912T190757Z-1222497.log` passed bot, structural/refusal transport, v2, handler and lobby packages; game compiled during the coordinator’s separate `SnapshotProjection` red test window. After that API landed, full game race passed `retirement-game-final--20260912T191017Z-1231377.log` (8.885s). Seven notice/economy cases passed in `retirement-ac-durable--20260912T190758Z-1222889.log`; its admin fixture needed to observe the newly polled positive invalidation before asserting unchanged count. Corrected probe passed all 13 actor events in `retirement-admin-probe-green--20260912T191016Z-1230713.log` (8.849s). Final frozen manifest SHA256 is `781e4f5e2c1b874b1a9f048fb35e3c45c6f81cf8f7e76749a11351a658ba8e84`. Reviewer/cold verifier gate remains pending.

### All-writer quiescence and cutover watermark — proposed bounded plan

**Goal.** Allow coherent ordinary snapshots only while a durable, independently
checked exclusion prevents every runtime/job writer from changing the source;
restore and handoff keep exactly one writable authority. Planning only: no new
migration, role, credential, container or live capture has been changed here.
Read Blueprint network/infrastructure chapters, ADR-012, ROADMAP Phase6.1/4 and
the corrected VPS runbook before this proposal.

**Observed writer inventory.** CodeGraph main/manager/transaction maps, followed
by direct SQL-site enumeration, show the following current paths. File families
are ownership boundaries; the final registry must be checked against the exact
post-retirement source before implementation.

| Writer family | Current entry points and durable effects |
|---|---|
| Text owner/runtime | `lobby/text*.go`, `store/text_{owner,value,settlement,cooldown,delivery,week}.go`: ownership/recovery, reservation/start/abandon, grants/settlements, weekly close, outbox lease/ack. Gameplay drain covers begun/prepared work, not all SQL writes. |
| Account/safety/profile | `auth/{auth,store,session,oauth,development}.go`, `profile/profile.go`, `store/text_trust.go`, `avatar/avatar.go`: account/session issuance and rotation, OAuth flow/receipt, sanctions, blocks/terms, nickname/preset and screened avatar bytes. A GET/auth/browser flow may also mutate session state; method names alone cannot classify writers. |
| Billing/spend | `economy/{wallet,convert,entitlements,purchases*}.go`: HTTP proof intake, provider observation/poll scheduling, acknowledgement tasks, refund reconciliation, spend/conversion and named grants. Provider calls can finish after a caller loses its reply. |
| Community/content/moderation | `portal/{manager,challenge,challenge_lifecycle,guard,browser_session}.go`, `reports/{intake,reports,cases}.go`, `store/text_{release,archive}.go`: proposals/reviews/rights/publication/takedown, challenge and Guard maintenance, rewards, sessions, reports/feedback and audit. External screening stays outside DB locks. |
| Admin/ops/jobs | `admin/{admin,operations,runtime_ops}.go` and transaction callbacks, notices, audit; `knowoffd` standalone `close-week`/`nightly`, `seed-admin`, and `migrate`. Main starts health/notices, provider, community, match and delivery goroutines; it currently cancels without joining every worker. |
| Non-PostgreSQL inputs | Redis coordination/security/notice state and absolute TTLs; immutable release inputs and retained archives/objects. Current avatars are PostgreSQL bytes. No current playable-object writer is assumed after retirement; any retained external writer must be declared and stopped. |

`WithValueTransaction` is a retry helper, not a global exclusion. `TextOwner`
protects gameplay authority, not other account/admin/job transactions. The current
runtime status deliberately leaves `writers_quiescent=false`. `snapshot.py`
requires its own labelled fixture, no other database clients, read-only defaults
and disconnected fixture network; ordinary capture is explicitly refused. These
are sound current boundaries, not evidence for a live all-writer barrier.

**Chosen boundary.** Prefer a bounded offline writer handoff after gameplay drain,
without adding a contended database lock to every ordinary game statement. A local
request counter alone is insufficient: the final fence must revoke runtime login/
connection authority and close its physical connections, while a separate capture
control identity retains only the authority needed for capture and handoff.
Database roles must be distinct: runtime/app/admin/job identities are non-owner,
non-superuser, cannot create/alter roles/schema or bypass the capture fence; schema
migration and cutover control credentials never enter the running application.
The current single configured database user does not establish that separation;
ordinary capture must refuse it until exact roles/ownership/grants are verified.
Privileged database administrators remain the trusted operational boundary.

**Protocol and ordering.**

1. Bind an authenticated cutover request to source database identity, exact clean
   schema/image/config/content manifests, current process owner/generation and a
   fresh request UUID. Close admission with the existing drain. Timeout keeps it
   closed and reports active matches/trades/reservations/settlement counts.
2. After matches are drained, atomically stop accepting *all* player/admin/portal
   request work (except bounded status/control/health) and all recurring job passes.
   Join registered in-flight request/worker operations with configured deadlines.
   Keep committed unacknowledged delivery and provider tasks distinct from unpaid
   settlement; offline clients cannot block capture forever. No forced gameplay
   completion or implicit interruption is introduced.
3. Under a separately authenticated control connection, record a durable closing
   generation; disable LOGIN/CONNECT for the exact declared writer roles, including
   PUBLIC/inherited paths, then stop the exact declared writer containers/processes.
   Wait for in-flight transactions and release/close their pools; terminate only
   explicitly identified source writer sessions when the authorized bounded drain
   has failed to close them. Refuse unknown roles, clients, prepared transactions,
   replication/subscription writers or undeclared services. Never treat one empty
   `pg_stat_activity` sample or read-only default as the lasting fence.
4. Verify the physical/login fence and stopped-writer identities again; persist an
   immutable watermark containing request/generation, source DB identity, schema
   and artifact pins, observed WAL position, stopped writer inventory digest and
   durable pending-work counts/digests. Use a fresh bounded capture lease bound to
   that watermark; lease expiry invalidates capture permission and never reopens
   writers. A lost controller leaves the source closed for explicit recovery.
5. Extend the existing snapshot tool with a distinct ordinary-source adapter. It
   must verify live control authority and the same watermark/fence before and after
   every capture stage and before publishing its completion manifest. Preserve
   existing bounded manifests, roles/ACLs/sequences/rows/blobs/Redis TTL/object
   parity. Capture sealed source role state; never reset writer restrictions as
   cleanup. The fixture adapter and its explicit labels remain intact.
6. Restore only to an empty identified target and keep its runtime roles disabled.
   Recheck source fence and artifact/watermark parity. An explicit audited handoff
   may enable only the verified target generation while the source stays sealed.
   Stale requests cannot unseal either copy. Before target writes, an explicit
   compatible rollback can return authority to the unchanged source; after target
   writes, require reconciliation/forward repair, never overwrite with old data.
7. Resume held receipt/provider work on the sole target using existing immutable
   account/source/request keys. An acknowledgement may have succeeded remotely
   while its local completion was lost; replay must remain idempotent. Capture is
   not a claim that external stores stop processing payments. A callback arriving
   during closure receives a retryable response, never a fabricated grant or ack.

**Touched files.** New discovered-absent store cutover state/helper/tests and one
additive migration only after coordinator allocation; `cmd/knowoffd` request/job
lifecycle wiring, internal authenticated runtime controls, explicit configuration
for role/control bounds, checked-in role provisioning/verification under existing
infra tooling, `infra/compose/snapshot.py`, its current tests/rehearsal and VPS
runbook. Billing algorithms stay frozen; only their enclosing worker lifecycle is
joined. No secrets in command arguments, URLs, manifests or logs.

**Test-first checklist.** Each implementation boundary requires independent review
before its cold verifier.

- [ ] Prove current ordinary capture refusal and enumerate exact writer/role/table
  coverage; missing writer/privilege/provisioning evidence must refuse before
  changing state. Provision only random guarded disposable roles for proofs.
- [ ] Add durable closing/watermark identity and retry/conflict/stale-generation
  tests; immutable terminal history/down refusal and crash recovery stay closed.
- [ ] Add joined request/job registry tests with a held SQL write, provider call,
  community/admin mutation and outbox ack. Timeout never reports quiescent.
- [ ] Prove physical DB exclusion with a second process, reused pooled session,
  inherited role/SET ROLE attempt, new login, direct INSERT and sequence mutation;
  after the fence every ordinary writer refuses, while capture reads succeed.
- [ ] Add ordinary-source adapter tests for expired/changed watermark, controller
  loss, foreign client, interrupted stage and missing object; no completed backup
  marker and no automatic writer reopening on any failure.
- [ ] Run two-copy handoff: source remains sealed, target opens once, stale source
  restart/old operator request refuses. Replay held purchase/ack exactly once and
  compare complete account/ledger/entitlement/receipt/outbox/sequence parity.
- [ ] Integrate existing snapshot restore/rehearsal and record the exact manifest,
  role/grant evidence and cleanup. No actual deployment target is selected today.

**Risks/non-goals.** Role provisioning is a prerequisite, not an optional deployment
note; indiscriminate session termination or role changes are forbidden. Unknown
external writers keep capture closed. Source and target cannot share a reusable
unseal claim. This plan does not implement multi-node match routing, Kubernetes,
remote credential setup, DNS changes, live data loss acceptance or deployment.
The control API/role privileges need coordinator review before implementation.

### Live configuration type retirement — coordinator partition E1

**Goal.** Remove retired storage/media/bot and specialty/timer/backfill fields from live typed configuration while retaining exact historical policy replay. **Scope.** config.go, text.go Clone, runtime_config_test.go, plus existing policy-v1/real-PG replay tests. BandHigh/BandLow remain until the media/portal partition retires their last callers; old HMAC-SSV constructor fields remain until the billing owner transfers their tests. No tuning numbers, wire version, frozen DTO or migration changes.

**Inventory.** CodeGraph and explicit remaining field references find Storage/Media/Bots only in config's historical zero-value assertion; old timers/backfill fields have no production caller after A/C. SpecialtyWeights has only Clone plus the independent frozen policy DTO. **Test plan.** A reflection regression must fail for old live fields and pass after removal. Replace the old zero-value assertion with actual type absence; preserve provider-disabled checks. Full config tests, all-module compilation, unchanged old JSON byte/hash and actual historical65-Noin policy replay gate the change.

**Checklist.**
- [ ] Record old-type-presence RED.
- [ ] Remove unused live fields/types and obsolete Clone branch.
- [ ] Independent source review and native/PG compatibility gates.

**Risks.** Accidental stored-policy hash change or undeclared compiler consumer. Frozen v1 DTO remains the sole historical decoder; source/module compilation and exact fixture SHA prohibit silently dropping persisted old values. E2 covers remaining media/SSV fields and dependency cleanup after their owners finish.

### Obsolete billing executable retirement — billing worker

**Approved scope.** Remove the still-mounted `/api/economy/ssv` HMAC route and its
unverified grant/signature helpers, old key/sender fields, and unused Google/Apple
compatibility methods. `NewPurchases` becomes a two-argument closed manager;
`NewManager` no longer consumes retired Security keys. Keep all historical
receipt/refund test files and explicit SQL fixtures, the verified platform
adapters and the authentic AdMob ECDSA primitive unchanged. There is no live
reward-claim endpoint until its independent policy and durable claim gates close.

**Tests first.** GET/POST to the retired route must return404 without changing
wallet, ledger or receipts (the current route returns400/405). Transfer the old
SSV signature case to prove the authentic verifier rejects both old HMAC bytes
and malformed signatures, while a synthetic correctly signed ECDSA control
passes. Transfer compatibility refusal to the actual unconfigured VerifyReceipt
boundary. Cold economy/handler gates and config compilation follow independent
review. No migration, payment algorithm or external platform operation changes.

**Retirement evidence.** HTTP1259718 reproduced400/405 before removal. Scoped
actual PostgreSQL race1270905 passed10 top-level tests/17 events, preserving
receipt/refund parity and authentic AdMob rejection tests. Parent independently
approved source. Independent cold1286127 passed handler42 top-level/89 events
(25.204s) and economy67 top-level/134 events (37.812s), with0 failures/skips,
including this frozen retirement. The first green attempt1266296 caught a
removed fixture format import; it was fixed before the passing run.

### Retirement B portal caller clarification

CodeGraph found one remaining legacy media consumer beyond the initial B file
list: `portal.Handler` still mounts `/portal/simulate`, whose `simulateRun`
constructs the old cosine dealer from `Manager.media` and live BandHigh/BandLow.
The production text runtime never injects that dependency. B will remove only
that dependency field/constructor assignment, old simulator functions/template
and navigation flags, leaving an explicit authenticated retired-route refusal
before form parsing. The existing `portal_test.go` simulator positive case and
its private old vector/pack helpers will become real HTTP refusal/no content
work assertions; all current browser/Guard/challenge/contribution tests stay.
No terms, submission, screening, value or role mutation is in this extra scope.
Current text certification/simulation remains the existing `mediapack text-*`
CLI and authenticated text-content administration, not a new portal simulator.

### Retirement F dispatcher acceptance

After the A/C verifier closes, gamebot's existing `main.go` will accept exactly
one of text simulation, recorded replay or authenticated prototype networking.
The default endpoint becomes literal-loopback `/ws/v2`; obsolete room/queue/count
flags, extra positional arguments and contradictory command/scope flags fail
before reading paths, creating artifacts or contacting auth. The existing
network policy, development-identity checks, engine algorithms and performance
harness stay unchanged. Existing `text_test.go` will execute the actual main
in bounded subprocesses, checking refusal without HTTP/output effects and a
successful private simulation artifact followed by full replay. No test file
is deleted. Root owns network/performance files and reviews this two-file
production/test boundary before its cold module gate.


### Client partition D — source retirement and explicit test transfer

The approved D implementation now removes all eleven legacy production files:
GameScreen/notifier/actions, old session/player/DTO entities, old dev panel and
four playable media cache/downloader/model files. GameCountdown, GameRoleSeal
and finite GamePokeFeedback remain current shared UI. DTO-independent sharing
is now TextRoomShare and is mounted in the real text lobby: actual missing-copy
RED1240651 was repaired while preserving rendered native/web QR-versus-clipboard
byte assertions. Main retains native purchase startup/lifecycle and constructs no
legacy provider or connection. KoPage has no debug authority. The generic socket
interface and all8 real transport lifecycle tests remain with canonical v2 frames.

All test files remain. The durable companion
`docs/reports/2026-09-12-client-retirement-test-map.json` accounts for192 original
declarations, including62 notifier and40 old-screen declarations, with exact
replacement test-search strings and semantic reasons. Removed powers/private
hand peeks are strict no-frame/no-state/no-grant proofs; current selection is
explicit and never auto-plays. The current scrollable page permits a disconnected
seat label to wrap: unlike the retired fixed workspace, that may shift content
vertically. Parent approved preserving exact position on selection alone plus
same card Element/dimensions/order/selected-copy across status/rotation and bounded
360px layout; board revisions deliberately invalidate stale action previews.

Media transfer GREEN1209388 passed14 tests using actual TextPlay/ApiClient HTTP
capture, strict role snapshot/page hashes, configured bounds and exact-key cache
retirement with prohibited network refresh. Initial1206612 diagnosed completed
page replay refusal and the plural safety rooms path, both fixture assumptions.
Session/Home transfer errors1217062/1219970 were missing required duplicate ack
flag and public notice auth using a non-injected API, respectively; fixtures now
use actual protocol shape and real token-loader call counts. Domain1225462
required eliminated-seat public role and correct discussion Poke target rules.
Source gate1244459 found seven fixture semantics handles disposed after Flutter's
body checks, plus the documented status wrapping. Corrected1250312 passed266
current/mapped shell, screen, session, domain, media and actual socket tests in21s.
Additional exact4/6seat public attribution, long authored text, round-zero/missing
deadline refusal and immutable request-budget tests were added for final mapping.

Fresh exact-reference inventory1259091 identified113 now-unused English legacy
play/dev keys (60 existed in pseudo; none in Arabic/Turkish). Only those keys and
matching metadata were removed from all four locales; active billing/avatar keys
were preserved. Flutter pub tooling removed flutter_riverpod, riverpod and now-
unused state_notifier; shared HTTP/crypto/avatar/WebP/QR/purchase dependencies stay.
Localization regeneration succeeded. Analyzer1270141 caught only new test import/
brace cleanup;1277417 observed the remaining brace during a formatted-script anchor
repair. These exact logs were read and the source corrected. Final scoped
1279021 and the separate official full client/Node/analyze/format/web gate remain
pending; this checkpoint is source-complete, not a full-client verification claim.
CodeGraph final reindex is coordinator-owned after concurrent server retirement.

D final scoped1279021 passed155 current/mapped tests in11s and the analyzer reported zero issues after dependency/localization cleanup. Frozen26-file/11-removal manifest and192-declaration mapping were sent for independent source review; v2 verifier now owns the full official client/Node/analyze/format/web build. No staging or all-phase completion claim.

### All-writer prerequisite role inventory — bounded implementation subplan

**Scope before migration26.** Add `server/internal/store/cutover_roles.go` and matching tests only: a bounded read-only privilege verifier, with no runtime wiring, role mutation, capture authority or watermark yet. Its disposable test fixture alone provisions exact random roles after the existing DSN/token and `current_database()` checks. The control-function privilege and26 transition schema remain a separate review boundary.

**Current physical writer inventory at head25.** All71 application tables are in `public`; five BIGSERIAL sequences belong to `audit_events`, `noin_ledger`, `admin_audit_log`, `text_outbox`, `billing_subscription_observations`. This list reconciles migrations and current non-test SQL statements. Modules are writer lifecycle owners, not separate SQL principals; the current runtime shares one database user. Migration-only imports and version state remain readable but cannot be rewritten by runtime.

| Table | Introduced | Current writer modules |
|---|---|---|
| `accounts` | 000002 | admin, auth, avatar, portal, profile |
| `admin_accounts` | 000003 | admin |
| `admin_audit_log` | 000003 | admin, notices, portal, reports, store |
| `admin_sessions` | 000003 | admin, auth |
| `audit_events` | 000002 | audit |
| `auth_revocations` | 000002 | auth |
| `billing_account_sources` | 000019 | economy |
| `billing_legacy_premium` | 000019 | migration only |
| `billing_provider_tasks` | 000019 | economy |
| `billing_subscription_current` | 000021 | economy |
| `billing_subscription_imports` | 000021 | migration only |
| `billing_subscription_observations` | 000021 | economy |
| `billing_subscription_replacements` | 000021 | economy |
| `billing_subscription_sources` | 000021 | economy |
| `billing_subscription_tasks` | 000022 | economy |
| `billing_transactions` | 000019 | economy |
| `challenge_current_winner` | 000016 | portal |
| `challenge_entries` | 000004 | portal |
| `challenge_topics` | 000004 | portal |
| `challenge_votes` | 000004 | portal |
| `challenge_winners` | 000004 | portal |
| `custom_avatars` | 000003 | admin, avatar |
| `daily_noin_earned` | 000003 | economy, store |
| `daily_quickplay_counts` | 000002 | store |
| `device_tokens` | 000002 | auth |
| `entitlements` | 000003 | economy |
| `feedback` | 000003 | admin, reports |
| `guard_freezes` | 000004 | portal |
| `leaderboard_daily_counts` | 000010 | leaderboard, store |
| `leaderboard_entries` | 000002 | leaderboard, store |
| `leaderboard_history` | 000002 | store |
| `leaderboard_weeks` | 000002 | leaderboard, store |
| `named_entitlement_items` | 000019 | economy |
| `noin_ledger` | 000003 | economy, store |
| `noin_wallets` | 000003 | economy, store |
| `oauth_flows` | 000020 | auth |
| `oauth_links` | 000002 | auth |
| `player_blocks` | 000012 | store |
| `portal_browser_sessions` | 000006 | auth, portal |
| `portal_login_limits` | 000006 | portal |
| `portal_login_requests` | 000006 | auth, portal |
| `portal_role_applications` | 000004 | portal |
| `portal_roles` | 000004 | portal |
| `portal_submission_counts` | 000004 | portal |
| `portal_submissions` | 000004 | portal |
| `portal_terms` | 000004 | portal |
| `profiles` | 000002 | auth, economy, profile, store |
| `queue_cooldowns` | 000002 | store |
| `report_cases` | 000018 | admin, reports |
| `report_rate_limits` | 000018 | reports |
| `reports` | 000003 | admin, reports |
| `schema_migrations` | 000001 | migration only |
| `store_purchases` | 000003 | economy |
| `system_notices` | 000003 | notices |
| `text_abandons` | 000024 | store |
| `text_accepted_inputs` | 000011 | store |
| `text_active_releases` | 000011 | store |
| `text_admissions` | 000009 | store |
| `text_archive_progress` | 000011 | store |
| `text_award_receipts` | 000010 | store |
| `text_content_revisions` | 000011 | store |
| `text_first_win_claims` | 000010 | store |
| `text_legacy_archive` | 000011 | store |
| `text_matches` | 000009 | store |
| `text_outbox` | 000010 | store |
| `text_process_current` | 000013 | store |
| `text_process_owners` | 000013 | store |
| `text_releases` | 000011 | store |
| `text_settlements` | 000010 | store |
| `user_terms_acceptances` | 000012 | store |
| `user_terms_versions` | 000012 | store |

**Role contract to verify.** Four distinct explicit identities: NOLOGIN schema/database owner; runtime LOGIN; capture LOGIN; migration LOGIN with only an explicit SET-owner membership (INHERIT false, SET true, ADMIN false). Runtime and capture have no role memberships, including reverse grants allowing another ordinary identity to assume them. All four are NOSUPERUSER, NOCREATEDB, NOCREATEROLE, NOREPLICATION and NOBYPASSRLS; runtime/capture own no object. PUBLIC has no database CONNECT/TEMP or schema CREATE. Runtime/capture have no schema CREATE/database CREATE/TEMP. Runtime gets SELECT on71 tables, DML on68 current-writer tables, sequence USAGE/SELECT, no TRIGGER/TRUNCATE/REFERENCES or sequence UPDATE. Capture gets SELECT on all tables/sequences and schema USAGE, with no DML or sequence advancement. Existing trigger functions remain invoker functions owned by the schema owner; unknown security-definer functions refuse. Unknown schema/relation, RLS or foreign/replication writer refuses this narrow contract. The trusted bootstrap superuser remains an explicit administrative boundary, not a runtime principal or a claim that superusers can be fenced by grants.

**Tests.** Prove the current single-owner fixture refuses and correct four-role provisioning passes. Change one fact per negative case: ownership; superuser/bypass/create-role/create-db/replication flags; INHERIT/SET membership; PUBLIC CONNECT/TEMP/CREATE; missing read/DML; capture DML/sequence UPDATE; runtime TRUNCATE/TRIGGER/grant option; extra table/sequence/security-definer; RLS. Separate actual runtime connections must append permitted data but refuse ALTER TABLE/DISABLE TRIGGER/SET ROLE; capture reads but refuses INSERT/nextval/setval. Context cancellation and unchanged rows/ACLs on refusal are required. No ordinary service/container is touched.

**Next schema review.** Migration26 will bind declared writer roles, source UUID, request/generation and transitions to retained audit/watermark records; its exact schema, revocation capability and recovery paths follow these proofs. PostgreSQL16 requires CREATEROLE plus ADMIN OPTION to change another non-superuser role LOGIN flag; that capability stays behind narrowly reviewed control functions or a separate trusted operator, never the application. INHERIT and SET permissions are separate. Primary references: [ALTER ROLE](https://www.postgresql.org/docs/16/sql-alterrole.html), [role membership](https://www.postgresql.org/docs/16/role-membership.html), [predefined roles](https://www.postgresql.org/docs/16/predefined-roles.html).

### Config E2 SSV type removal and load diagnostic checkpoint

E1 source independently approved by v2_contract, with native1257394 and actual-PG historical policy1258061 passing. After reviewed HMAC route/grant removal, SecurityConfig no longer carries SSVCallbackKey or SSVAllowedSenders. Reflection regression1288169 reproduced both obsolete fields; full config1288493 passed after removal. Frozen historical tuning serialization is unchanged. Dealing band fields still await portal/media retirement B.

The corrected single-pagination 100-room/500-socket run1239702 completed all matches in554.65s and returned room/peer/socket resources to zero with goroutines8→8, but FAILED its unchanged acceptance bounds: intent-to-ack p95=1.191039889s, p99=1.578198121s; combined server/client retained heap1,346,384→2,796,456 bytes. Worst wire frame8185 bytes. This is diagnostic evidence, not a passed performance gate. Next isolate retained test bookkeeping and cross-room global-lock contention without changing thresholds or excluding queue delay.

### Retirement A/C cold verifier closure

Independent source review approved the initial A/C manifest and the subsequent
fixture-only owner/isolation corrections. The first whole gate1235422 passed
all seven other packages but exposed five economy fixtures using unbound value
stores after an earlier runtime owner. Binding actual owners passed the five
focused tests1274005; the cross-package rerun1279657 then correctly refused
ownerless historical work retained by a handler fixture. The five new economy
fixtures now use the existing exact nonce/DSN/current_database guard before
isolating their schema, then acquire/bind/recover/release a real TextOwner.
They do not classify foreign fixture history or bypass immutable value fences.
No production value code or original assertions changed in this correction.

Final cold PostgreSQL/Redis race gate:
`/tmp/agent-runs/retirement-ac-isolated-economy-final--20260912T192331Z-1286127.log`
and `/tmp/agent-runs/retirement-ac-isolated-economy-final-results.json`:
handler42 top-level/89 total test results in25.204s; economy67/134 in37.812s,
zero failures/skips. This includes the separately reviewed HMAC-SSV retirement.
The unchanged packages from1235422 passed: game37/137, bots1/11, lobby39/69,
transport7/22, v2 contract19/126 and notices7/7. Post-review admin gate
`/tmp/agent-runs/retirement-admin-cold-verifier--20260912T191130Z-1236063.log`
passed1/13 in10.179s. Across these accepted disjoint package scopes: **220
unique top-level tests,608 total test results,zero failures/skips**. Vet passed
`/tmp/agent-runs/retirement-ac-vet--20260912T191131Z-1236727.log`; the last fixture
correction additionally compiled/vetted in its cold `go test` command.

Final A/C manifest including the corrected helper:
`/tmp/agent-runs/retirement-ac-owner-fixture-final.json`, SHA256
`dfb7e4596f9d948e2c37b0311ba3142535302a4717ca93c8b7737bcd4cbf7479`.
The original failure logs remain evidence; only the matching recovery breadcrumb
was marked resolved. This closes this server retirement boundary, not all Phase6
or the final repository-wide runner.

### Cross-room action concurrency — coordinator repair plan

Goal: independent active tables can apply and serialize recipient state concurrently while membership/rebind/admin controls remain mutually exclusive with every in-flight action. The unchanged 100-room200ms/500ms budget remains the acceptance target.

Non-goals: no per-room actor rewrite, no asynchronous stale snapshot queue, no weaker validation or owner checks, no timing boundary moved past queue wait. Start, Tick, membership, drain and admin controls retain exclusive manager locking in this first bounded repair.

Touched files: lobby/text_manager.go (RW membership guard, atomic terminal flags, rate-map guard), text_runtime.go (room serial section for Action/Resync, non-pruning frame broadcast), text_room.go/text_queue.go/text_drain.go only atomic-field access as required, existing lobby tests and new bounded concurrency tests in existing room_lock_test.go. Test fakes become concurrency-safe only where their shared accounting is now exercised in parallel.

Lock contract: acquire manager RLock before room runtime mutex for Action/Resync; retain RLock until room unlock so all existing manager writers exclude in-flight actions and peer replacement. Other rooms share the read lock; one room serializes its engine mutation, recipient sequence and complete frame groups. current peer pointers and maps stay stable. Peer closed and permanent lost/draining flags use atomic state because terminal closure can arise under parallel readers. loseAuthority closes peers through an atomic once-only transition, never changes maps. AllowRequest uses RLock plus a distinct request-rate mutex so its accounting does not serialize whole actions. Routine Send remains an exclusive control operation. Terminal pruning, when necessary, happens after releasing the read section under the manager write lock and revalidates the same room pointer.

Proof checklist:
- [ ] Block one room in a bounded HideChat hook; another room action and resync must complete before release (RED on current global lock). Same-room action/resync must wait, context cancellation must terminate waiting work, and authority loss emits no new private frames.
- [ ] Implement the above lock contract with exact existing membership/owner checks and frame ordering. A slow peer closes only itself; repeated concurrent owner loss cannot double-close a channel.
- [ ] Run full lobby/game/handler/store race slices, including disconnect/rebind, budget/replay/owner loss, wallet privacy, drain and durable abandon/settlement. Independent source review precedes corrected100-room profiling.
- [ ] Diagnose retained heap with actual heap profile and distinguish workload warm-up/pool/test-log retention from live server resources; preserve the specified10% criterion and report combined-process limitations.

Risks: RWMutex writer priority can still delay new actions behind administrative work; this partition deliberately preserves that authority barrier. A terminal read path must not mutate maps, and reentrant callbacks remain forbidden. Tests must prove no read-to-write lock upgrade and no stale-generation output. Further Tick/start concurrency needs a separately reviewed scope if measurements still fail.


### Remaining admin operator operations — read-only inventory and proposed boundaries

**Goal and source.** Complete Blueprint Admin Console lines386–397 and moderation line795 / ROADMAP Phase5 child13 with actual internal-browser kick, account/installation sanction, exact-room closure and audited Noin correction actions. This is a proposed implementation plan for coordinator review; no operator implementation has begun. CodeGraph was queried before source reads. Existing client partition D is source-approved; official combined client verification remains v2_contract-owned after metadata/format findings.

**Observed coverage, not assumed completion.**

| Required operation | Current implementation and proof | Remaining gap |
| --- | --- | --- |
| Kick | `TextManager.EnforceAccount` closes the current peer after canonical final-sanction recheck; `leave` retains a begun seat for existing absence rules. Real final-Guard-ban HTTP/WS proof exists in `cmd/knowoffd/text_runtime_integration_test.go`. | No named kick command, room-specific exclusion, browser form or audited retry identity. Disconnect alone permits rejoin and is not a completed kick. |
| Ban account + device | Portal `decideFreeze` converts an existing Guard freeze to permanent/timed account state; `guard_delivery.go` retries committed disconnect delivery. Auth17 epochs invalidate prior credentials without deleting identity/value. | No independent admin sanction without an existing freeze; no installation sanction table or token check. Client `_deviceHash()` generates a new random value for each anonymous creation; OAuthResult issues an empty DeviceHash; `device_tokens` only has uniqueness on `(account_id,device_hash)`. Legacy `RevokeAccount` deletes links before banning and must not be exposed as this operation. |
| Close room | Internal runtime drain stops admissions but intentionally leaves begun matches running. Process `TextManager.Close` closes all rooms; engine Close/owner-fenced store Interrupt already preserve instant awards, compensate consumed free quota once and publish private interrupted receipts. | No command for one immutable room incarnation; no audited close receipt/status or stale room-code rejection. |
| Grants/refunds | `/admin/economy` is a read-only ledger/entitlement lookup. Verified platform refund reconciliation exists, including spent-credit pending reconciliation; legacy Purchases.Refund rejects provider-managed sources. | No initiating-admin audited grant/refund mutation. A browser operation must not call a payment-provider refund or pretend a Noin credit is a cash refund. |
| Leaderboard administration | Current `leaderboard.Manager.CloseWeek` and executable `close-week`/`nightly` reuse durable week-close primitives; immutable close/replay tests exist. | No internal-admin exclusions/reinstatements or explicit audited close-rerun/history console was found. This is another Blueprint Admin gap, requiring a later separate week-lock/projection plan; it is not claimed by the four boundaries below. |
| Other operator families | Existing text release publish/activate/withdraw, exact report resolution, Guard decisions, terms/roles/applications, notices, avatar takedown and feedback statuses already use product paths. Common `LockAdminTx` + exact initiating-session callback and after-domain-wait expiry proofs were reviewed previously. | This slice extends that authority boundary; it does not replace these families or reopen their accepted policies. |

**Non-goals.** No live provider calls/credentials, money-refund claims, hardware attestation, account merging, entitlement revocation policy, deletion/retention policy, external deployment or writer-barrier implementation. Migration26 belongs to the separate all-writer lane; any new operator schema needs a later coordinator-reserved number and its own reviewed up/down parity proof. Existing 1–25 SQL remains immutable.

**Proposed behavior for review.** A kick targets an immutable room ID plus account and expected runtime owner/generation: remove a waiting member/release its reservation; for a begun seat close its connection and block that account from rejoining that same room, while existing absence rules govern the match. It does not ban the account from other rooms or fabricate scores. Close-room blocks that exact room immediately after the audited decision commits, releases waiting/prepared admissions, or interrupts a begun match using the existing owner-fenced compensation/private-delivery path; a committed terminal result is preserved. Reusing a code for another room cannot carry an old action forward. Direct account sanctions support explicit permanent/timed and explicit lifting receipts; existing Guard-final decisions remain independent provenance and cannot be accidentally lifted by an unrelated receipt. An account+installation action captures the current known installation hashes, preserves device/OAuth links and value, revokes sessions once with the decision, and enforces live connections after commit.

Installation identity must be persisted separately from the account/session in SharedPreferences, carried through OAuth start/result and signed JWTs, and rechecked at login, refresh, derived-session issuance and socket admission. It identifies a client installation and remains resettable by a hostile client; no stronger device-ban claim is made. Concurrent first authentication must select one canonical account or fail closed on an already ambiguous legacy hash instead of silently choosing an arbitrary account. Explicit migration/compatibility tests must preserve existing account credentials and reviewed old OAuth receipt behavior while requiring the new device binding for newly begun flows.

For economy, propose two explicit Noin operations: a reasoned administrative grant, and a full compensating credit referencing one immutable negative `spend` ledger row belonging to the target. Refund amount derives from that source, with one refund per source; positive awards, platform purchases/refunds and a foreign account's ledger cannot be used. Keep these separate from daily gameplay earning counters, premium doubling and match outcome. Entitlements remain unchanged; avatar takedown still receives no automatic refund. The browser must label this as a Noin correction, not a platform cash refund. Coordinator review is required for this exact product interpretation before implementing it.

**Authority and lock order.** Each new internal route uses bounded body/fields, no-store, admin role, CSRF, exact session binding and a required request UUID/reason. Receipt payload identity includes actor, action, immutable target, amount/source or sanction bounds; same-key mismatches fail. Browser retries still validate the initiating session before returning success. Already committed trusted delivery retries use the retained decision, never a freshly supplied actor or an expired browser credential. Audit failure prevents decision/effects; delivery failure leaves explicit pending status and cannot claim success.

Runtime selection uses a bounded manager lock before SQL, matching existing admission/enforcement order; no SQL transaction may call back into the manager. Owner-fenced value delivery locks current process singleton before match identity, then sorted affected accounts, then dependent admissions/wallet rows. Initial admin decision transactions use sorted actor+target accounts, admin role and exact session, then operation/sanction/source rows, followed by a final `clock_timestamp()` authority recheck after waits. Device writes must use one documented account-before-device ordering, validate the captured link set again under account locks and never introduce a device-before-account path into an existing transaction. A separate schema/API review must settle race-safe first-device claiming and outbox delivery before those bodies are written. Runtime decisions and delivery are distinct durable states: the receipt records authorization first, then existing value transactions deliver effects with a delivery audit and exactly-once state. Pending runtime decisions fence further actions in that room; replay/restart either delivers to the exact incarnation or records it obsolete after confirmed owner-loss recovery.

**Touched files and bounded checklist / tests.** Paths listed as new must be checked for existence immediately before creation.

- [ ] Boundary1: add the minimal immutable operator decision/delivery schema and store API (`server/internal/store/admin_operations*.go`, one later migration), internal handler forms/routes (`admin/operator*.go`, `pages.go`, narrow `handler.go` registration). RED tests for unauthorized role, missing/wrong CSRF, revoked/expired session after account and domain waits, changed request payload, forged actor, audit rollback and parallel same-ID claims. Migration prior-head→new-head/down→prior-head preserves all original rows; ledger/audit protection covers new receipts. Freeze for independent review before runtime/auth/value extensions.
- [ ] Boundary2: exact-room kick/close hooks (`lobby/text_operator*.go`, narrow runtime adapter binding in `cmd/knowoffd`, owner-fenced store delivery). Actual authenticated internal HTTP plus player WebSockets in every five-mode×4/6 cell: kick waiting/active/reconnect, close waiting/prepared/mid-award/verdict, unaffected other room, recycled code, stale owner, pending delivery denying actions, restart retry, request cancellation and closed socket cleanup. Assert one compensation/receipt, retained earned Noin, no invented points/XP/leaderboard/roles and unchanged terminal outcomes. Manager/store transaction lock order gets a real two-connection deadlock/cancellation test.
- [ ] Boundary3: direct account/installation sanction and lift (`auth` targeted API + tests; narrow OAuth device binding and client persistent-installation changes after billing/client freeze; operator handler). Actual PG race tests for ban versus device creation/token issue/refresh/OAuth result/admin+portal derived sessions in both orders; new anonymous login using the same banned installation refuses; unrelated installation/account succeeds. Repeated decision never bumps epoch twice; expired/lifted delayed delivery cannot close a fresh allowed session. Browser routes hit live sockets immediately; changed/unknown installation identifiers are recorded as the stated identification limitation, not tested as hardware prevention.
- [ ] Boundary4: explicit grant/source-refund (`economy/admin_corrections*.go`, narrow economy-lookup forms) using wallet+ledger+receipt+audit in one transaction. RED tests for actor/session expiry while waiting, grant overflow/nonpositive input, wrong source/type/account, partial or repeated refund attempts, concurrent spend/refund, ledger/audit insertion failure, immutable original rows, exact one wallet delta and no game cap/doubler/XP mutation. Preserve provider refund tests and avatar entitlement/no-automatic-refund behavior.
- [ ] Each boundary: independent source review, real disposable PostgreSQL16/Redis7 race gate with `-p=1`, scoped vet; end with the official repository runner and affected real runtime/browser tests. Keep existing test files and update docs/roadmap only for proven deliverables; platform/provider/device evidence stays distinct.

**Risks.** Stable installation binding expands the OAuth/client contract and must preserve old account identity; direct-sanction provenance can conflict with current Guard columns if flattened; late room delivery could affect a replacement incarnation; nested transactions under the manager could deadlock; refunds could double-credit or be mistaken for payment refunds. Separate provenance/receipt identities, explicit actor/owner fences, closed pending status and the above race tests are acceptance requirements, not optional follow-ups. This inventory does not mark Phase5 child13 complete.

Cross-room plan reviewed by owner_lease: add context-bound manager RLock and room-lock acquisition, recheck authority/current/context after waiting, per-peer enqueue/close mutex with a final owner check after serialization, and exact-pointer terminal pruning. TextOwner pinned-connection mutex wait is a separate known bounded-wait follow-up. Regression1295555 reproduces the unrelated-room stall (initial1295183 had a test Count-pointer compile typo, corrected without production edits).

### Retirement F reviewed implementation

`tools/gamebot/main.go` now contains only the current three text command paths.
Its default endpoint is literal-loopback `/ws/v2`; retired flags are retained as
refusal names only, with no legacy bot/envelope/auth code behind them. Unknown,
ambiguous, empty, positional, wrong mode/size and incompatible command flags
return before file/network work. Private exclusive simulation output and full
replay checks remain unchanged. Existing `tools/README.md` explains the current
network identity/prototype gate and the retired room/queue/count entry points.

The actual dispatcher subprocess RED
`/tmp/agent-runs/retirement-gamebot-cli-red--20260912T192641Z-1291505.log`
reproduced seven violations, including an authentication request for ambiguous
commands. The retained simulation/replay and new CLI cases passed race in
`/tmp/agent-runs/retirement-gamebot-cli-green--20260912T192748Z-1292171.log`
(7 top-level/14 total results,30.759s). Twelve additional scope/empty-input cases
and actual help-default proof passed
`/tmp/agent-runs/retirement-gamebot-scope-arguments--20260912T192930Z-1295805.log`
(1/13,2.215s). Parent source review approved the complete boundary. Frozen
manifest `/tmp/agent-runs/retirement-f-frozen.json` SHA256
`d99366d06614f23e5c7106e0b426d7243f09f2e74eda316ccf64eddbf469eb05`.
The final isolated PG/Redis cold verifier is pending the shared runtime's
compile-coherent boundary; it will run all existing network/simulation/replay/
CLI tests and the percentile unit test. The parent's separately owned100-room
performance test remains a distinct active repair/gate, with no skip added.

### Owner probe queued cancellation — bounded follow-up

Owner review identified TextOwner.Check using an uncancellable mutex wait before its already-bounded independent physical-session probe. Root owns text_owner.go and text_owner_test.go for this narrow fix. Add a regression holding the owner mutex while a short request context expires: Check must return context.DeadlineExceeded before the held mutex releases, without closing Done or changing readiness/authority. Replace only queued lock acquisition with a context/done-aware TryLock wait; recheck after acquiring. Keep the actual PostgreSQL probe on its independent three-second context and the exact advisory/backend SQL unchanged, so cancelling one player never cancels the process-wide connection. Existing request-cancel and real backend-loss tests are required with the new regression. Release/watch/fence transactions are unchanged. Review and actual-PG race proof precede acceptance.

Cross-room source was independently approved after enqueue/closure and cancellation safeguards. Actual PG/race1301135 passed full lobby14.062s, game9.665s and handler26.366s. Additional AllowRequest context API and handler timing boundary preserve the five-second budget including rate wait; regression1306700 (missingcontextAPI) turned green1309676 with both manager/rate cancellation cases. Owner probe queued cancellation1304060RED→1304329GREEN; all actual-PG owner tests1304587 passed9.816s, retaining the independent physical-session SQL probe. Final lobby/handler delta gate1310801 running.

Client partition D source review approved192 original declarations mapped to53 concrete test families; no test files removed. Full official client1290662 passed504 Flutter tests and2 Node cache tests with zero failures/skips, clean analysis, and80 formatted files unchanged. Web release1295336 succeeded38.7s. Native Android/iOS device/provider proofs remain open. Server A/C corrected cold verifier1286127 passed109 top-level223 events in handler/economy; accepted combined A/C gate evidence is220 top-level608 events including admin, with no failures/skips after guarded fixture isolation.

### Migration26 cutover authority — concrete schema and privilege review

**Bounded next step.** Migration26 adds durable authority/evidence only; it does
not grant roles, disable a login, stop a process or authorize ordinary capture.
The role prerequisite remains separate. New state is initially empty, preserving
all existing rows and current runtime behavior. No SECURITY DEFINER function is
introduced. All rehearsals still use uniquely guarded disposable targets.

**Physical identity.** A provisioned instance is bound immutably to its random
instance UUID, PostgreSQL cluster system identifier, database OID and database
name. Logical restore retains the source instance row. A target must have a
different physical tuple and a separately audited one-use handoff before a new
local authority row can become ready. Physical-clone activation is refused by
this first protocol. The physical tuple is observed through the control
connection, never accepted as client-provided identity. PostgreSQL's control-data
functions expose the cluster identifier. The offline control contract requires
an explicit EXECUTE grant as provisioning evidence; PostgreSQL16 already exposes
`pg_control_system()` through default PUBLIC execution, so reading that identifier
alone is not privileged cutover authority.

**Tables, all without sequences.**

- `cutover_instances`: UUID primary key; immutable physical tuple and optional
  parent instance/watermark; phase `ready|closing|sealed`, generation bigint
  `>=0`, current request UUID, created/changed timestamps. Fresh source bootstrap
  creates ready/generation0 only. `ready→closing` consumes generation1;
  `closing→sealed` keeps the generation and requires its exact persisted
  watermark. Recovery of closing/sealed creates a new request and advances the
  generation by exactly1 while remaining closed. No existing row can return to
  ready, and recovery cannot enable writer roles. Different-target bootstrap is
  a separate insertion, never rewriting the restored source identity.
- `cutover_requests`: immutable UUID request key, instance FK, positive generation,
  predecessor request when recovering, exact declared writer role names/OIDs,
  schema/image/config/content SHA256 pins, 32-byte lease-secret digest and bounded
  issue/expiry timestamps. Unique(instance,generation); duplicate request bytes
  are idempotent, conflicting bytes refuse. The secret stays outside SQL/logs.
  Expiry is checked against server time and never authorizes reopening.
- `cutover_watermarks`: immutable UUID key and unique request FK, matching
  instance/generation, WAL LSN, canonical evidence JSON with explicit byte bound,
  evidence SHA256 and creation time. Evidence contains stopped writer identity
  digest, durable pending-work counts/digests and exact artifact pins, without
  receipts, tokens or private content. Schema identity/FKs bind the records; the
  later control API must compute/verify evidence from the actual closed source.
  Merely inserting a row is never the physical-fence proof.
- `cutover_handoffs`: immutable UUID key, unique source instance and watermark,
  exact distinct target physical tuple, target instance UUID and creation time.
  This records the sole chosen target before enabling that target. The source
  stays sealed after controller loss; a second target or changed target refuses.
  Target activation later requires a fresh authenticated read of this binding
  through the source control connection plus verified restored watermark parity.

Row guards enforce immutable identity/evidence, state/generation chronology,
request/watermark binding and no DELETE. Statement TRUNCATE guards cover all four,
including cascade. Timestamp and JSON bounds are explicit; lease limits are
engineering safety limits, not gameplay tuning. Down takes exclusive locks and
refuses if any authority/request/watermark/handoff row exists; pristine empty
structures can be removed while exact head25 old-table data remains intact.

**Control privileges, provisioned outside migrations.** Add a fifth offline
control LOGIN, NOINHERIT/NOSUPERUSER/NOCREATEDB/NOBYPASSRLS/NOREPLICATION. Unlike
runtime it needs CREATEROLE plus ADMIN OPTION only on the explicitly declared
runtime and migration roles, with SET/INHERIT false, to disable/enable their
LOGIN flags. It receives no owner membership and cannot mutate ordinary game or
value tables. It gets SELECT on retained tables, SELECT/INSERT on immutable
cutover evidence, and SELECT/INSERT/UPDATE only on `cutover_instances`, constrained
by the guards. Database CONNECT revocation is redundant once verified NOLOGIN
and no alternate SET-role paths hold; a trusted provisioning owner handles
initial ACLs. Control's explicitly reviewed `pg_signal_backend` membership is
used only after matching database OID, role OID and process identity; unknown
sessions refuse, and all operations use bounded contexts. Its power is an
operator trust boundary, never an application credential or a public endpoint.

**Tests before SQL and source implementation.** Fresh/up25→26 old-table hash and
sequence parity; empty down; nonempty down leaves all bytes/rows unchanged;
request identity/conflict/replay; stale generation; invalid transition and lease
shape; mismatched instance/request/watermark; forged second target; direct row
rewrite/delete/truncate/cascade refusal. Then test actual provisioned control
capabilities and denials separately before any runtime integration. Finally
held-transaction/new-login/pooled-session tests must establish the physical fence;
these are later gates and are not implied by schema tests.

Primary semantics: [PostgreSQL16 ALTER ROLE](https://www.postgresql.org/docs/16/sql-alterrole.html),
[predefined roles](https://www.postgresql.org/docs/16/predefined-roles.html), and
[control-data and privilege functions](https://www.postgresql.org/docs/16/functions-info.html).

**Coordinator schema refinements, approved before26 edits.** Enforce UNIQUE on the
physical tuple, preventing an alternate fresh-ready row for one database. Use
composite instance/generation/request/watermark foreign keys, including
`current_request`; a handoff binds the current sealed generation and refuses a
watermark made stale by recovery. Generation is a positive bounded bigint for
requests; overflow refuses without a transition. A fresh source may bootstrap
only in an empty authority history. A restored target must match the actual new
physical tuple and an imported one-use handoff for the restored sealed source;
its parent instance/watermark fields are mandatory and distinct. Neither a
caller-supplied ready phase nor an arbitrary new UUID substitutes for this
bootstrap distinction. Control privilege proofs include actual denied
memberships/attributes and precondition inventories. Migration27 stays in its
owner's testdata until canonical26 exists, so no compiled migration gap is added.

**Prerequisite implementation evidence.** Missing helper/API RED1292657 preceded
implementation. Initial27-event role proof1296712 passed; actual runtime append
and capture/DDL/sequence refusals plus six extra ACL cases passed34 events in
1312548. Review then found PostgreSQL parameter grants/defaults and a `pgxevil`
schema-prefix bypass. Eight actual unsafe-provisioning cases reproduced in
1334374 (parameter subset1323282); corrected1363051 passed43 events/4 top-level
cases, 8.406s, real PostgreSQL16 with race checking and no skips. A fixture restore
statement used GRANT ... FROM in1306854; it was corrected to TO after reading the
log. Strict catalog checks first refused PostgreSQL's built-in pg_settings UPDATE
view and safe pg_subscription column SELECT; predicate diagnostics1350554/1356190
identified these exact initialized ACLs. The final checker retains ordinary SET
semantics but rejects trigger-bypass parameter authority; final actual SQL tests
exercise that view path as well. All temporary diagnostic prints were removed.

PUBLIC large-object writer functions are a separate retained-data write path,
even with table-only SELECT grants; isolated provisioning revokes their execution
and cleanup restores only the original initialized PUBLIC grants. The role
contract does not claim absence of all default PostgreSQL ephemeral functions
(e.g. notification/advisory lock effects). Unknown persistent functions/grants,
parameter ACLs, role defaults, ownership and extra schemas refuse. This remains
a bounded point-in-time prerequisite, not physical quiescence or capture approval.

### Accepted contribution to immutable release — coordinator integration proof

Goal: exercise real contributor draft/submit/screen/revision-bound approval and its one-time value before capture, certification, publication, activation and restart. Existing release fixtures inject approved SQL rows; this proof starts with the product workflow. Non-goals: no new editorial content, human pilot, live screening, public activation or changed reward policy.

Touched files: new `server/internal/portal/text_release_journey_test.go` (directory searched first), existing report and roadmap only unless the test exposes a production defect. Reuse guarded PostgreSQL fixtures and shared media validation/certification. Every external/human gate artifact is clearly labelled test-only and exists only in the disposable test.

Proof/checklist: (1) draft/submitted inputs and unavailable screening cannot capture/approve or grant value; exact approved bytes bind consent/editor/source identity and one credit/reward; (2) shared loader/dealer certifies the fixture, publish does not activate, missing evidence fails; (3) publish/activate retries and reconstructed release store preserve all profile/ledger/submission/consent rows byte-for-byte, and takedown leaves pinned bytes intact. Separate existing engine/network pinning and browser authorization proofs remain required. Risk: a mocked release artifact is only a workflow-contract fixture, never a production-readiness claim.

### Retirement B replacement-test preparation

Before production removal, the old executable-surface AST regression fails on
nine retained declarations (`retirement-media-surface-red--20260912T193805Z-1319410.log`),
the real mediapack dispatcher fails four old-command refusals
(`retirement-mediapack-cli-red--20260912T193806Z-1319652.log`), and the authenticated
portal regression reproduces GET/POST 200 plus the obsolete navigation entry
(`retirement-portal-simulator-red--20260912T193926Z-1330430.log`). The original Python
behavioral regression actually prepared an image instead of refusing it
(`retirement-image-python-red--20260912T193807Z-1319881.log`).

The retained media test files now exercise exact historical-byte preservation,
text-only closed schemas, immutable revision/provenance round trips, complete
scheduled prompt coverage, symlink/path refusal, missing/tampered member and
evidence detection, reviewed activation and refusal of synthetic activation.
Their current text paths passed before removal in
`retirement-media-replacement-tests--20260912T194656Z-1381773.log`; this establishes
replacement coverage, not the pending retirement proof. The existing writer
test file now directly tests the canonical writer, with no new writer wrapper.
Its initial argument-order compilation mistake was diagnosed and fixed;
`retirement-canonical-writer-tests-green--20260912T194851Z-1393260.log` passes.

The existing Python test file now invokes the actual retired entry point under
`python -I -S`: valid retained PNG/provenance, repeated input, malformed and
animated-format markers, path traversal, symlinks, a FIFO (no read may block),
corrupt/duplicate historical receipts and an audit hook forbidding input opens
and network calls. The broader red run
`retirement-python-refusal-matrix-red--20260912T194834Z-1391868.log` confirms the old
entry point still imports Pillow rather than returning the required refusal.
All original test files and historical fixture bytes remain on disk. Production
B removal is gated on the preceding reviewed F verifier; no B production change
has occurred at this checkpoint.

### Text projection copying — profile-directed repair plan

The profiled100-room run1319927 completed635.96s, all rooms/sockets cleaned and goroutines9→9, but failed unchanged budgets: p95=1.020155038s/p99=1.435838433s; first-GC combined heap4,601,912→30,494,032. The exit heap profile after another collection retained2,279.59KiB, mostly runtime goroutine/thread structures and JSON type metadata. This difference requires matched warmed/steady-state measurement, not a relaxed threshold. CPU used2319.05s with heavy JSON encode/decode/allocation paths.

Goal: preserve identical private/public values and wire bytes while avoiding JSON round trips solely used to detach already typed state. Non-goals: no removed validation, weaker secrecy, changed history/hash/page contract, action rules, clocks, budgets or owner fences. Touched files: new transport/v2 clone.go and clone_test.go; game text_snapshot.go and text_match.go plus scoped ownership tests. Paths searched first.

Ordered proof/checklist: (1) new full-shape clone tests fail for missing API; typed clones must preserve golden wire bytes, nil/empty behavior and every mutable nested field, with reflection-filled future-field coverage; (2) replace SnapshotProjection detachment and action-result copying only, then compare measured JSON-reference allocations/CPU and rerun all transport/game/lobby/handler race tests; (3) separately review private engine state copying before changing it; (4) source review then100-room proof with unchanged200ms/500ms and10% limits. Risk: missing a nested pointer reintroduces aliasing; shape-wide mutation tests and current projection privacy/rollback tests are mandatory.

### Retirement F final cold verifier

After source review approval, the final cold run
`/tmp/agent-runs/retirement-gamebot-cold-final--20260912T193832Z-1323960.log`
passed **16 top-level tests / 47 test events**, zero failures and zero skips,
with formatting, vet and build also passing. The authenticated five-mode/four-
and six-seat matrix passed all ten cells and its aggregate durable assertions
in 701.60 seconds; the full selected package run took 736.652 seconds. This
explicit selection retains the CLI, simulation, ordered replay, network and
percentile tests. The coordinator-owned 100-room performance test remains a
separate gate. No test skip annotation was added. Results are in
`/tmp/agent-runs/retirement-gamebot-cold-final-results.json`.

The earlier wrapper's four-minute timeout was too short for the existing
12-minute matrix context. Its failure log remains retained. The final run used
an explicit 13-minute package bound and fresh disposable services; it reran the
entire matrix, preserving the aggregate receipt/zero-value proof. The transient
`AllowRequest` signature compile failure in the earlier vet stage was already
fixed by its owner before this final run. These are recorded diagnostics,
not ignored failures or altered gameplay assertions.

### Retirement B frozen source boundary

The approved nine legacy executable files are removed; `bundle.go` retains only
`ContentHash`. `media/text*.go`, the synthetic text generator and the canonical
`WriteTextBundle` remain. Current command implementations are unchanged. The
mediapack dispatcher accepts only the six versioned text commands and refuses
all four former commands before parsing paths. The retained Python entry point
returns a constant retirement error without input reads, image imports, output
writes or network calls. Tool documentation reflects the supported commands.

Portal changes are limited to removing the obsolete media-manager dependency,
legacy simulator body/navigation and vector-based test fixture. The existing
GET/POST routes retain authentication, live curator role checks and browser
CSRF middleware, then return 410 without parsing simulation input. All other
portal/Guard/challenge/authority code is preserved. The six existing media test
files, writer test, Python test and portal test file remain; no test file was
deleted. Historical packs, candidate bytes, applied migrations and avatar
codecs were untouched. The frozen file manifest is
`/tmp/agent-runs/retirement-b-frozen.json` (27 paths, nine deleted production
files). Review and independent cold acceptance remain pending.

Local implementation checks passed: full media race **28 top-level / 136 test
events**, `retirement-b-media-green--20260912T195250Z-1403593.log`; whole mediapack
race **12 top-level / 23 test events**,
`retirement-b-mediapack-green--20260912T195250Z-1403624.log`; Python **8 tests**,
`retirement-python-refusal-matrix-green--20260912T195225Z-1402726.log`.
There were zero test failures/skips. The generator package has no test files;
its behavior is exercised by the canonical CLI/writer tests. Full server
compilation passed `retirement-b-server-compile-green--20260912T195333Z-1407968.log`
after removing the diagnosed unused import left by the old portal fixture.
The authenticated portal regression is running separately with disposable
PostgreSQL/Redis before final review acceptance.

The frozen portal regression subsequently passed **1 top-level / 5 events**
with actual disposable PostgreSQL and Redis:
`/tmp/agent-runs/retirement-b-portal-green--20260912T195442Z-1410894.log`, results
`/tmp/agent-runs/retirement-b-portal-results.json`. Both methods preserve the
non-curator 403 guard, return curator 410 without consuming form input, remove
the navigation link and leave submission/ledger counts unchanged. Frozen B
manifest SHA-256:
`bd7a98c34589b98645f3790e6667b1f90788aa6452474d2ae28be909a06046de`.


### Admin operator boundary 1 — immutable decisions and delivery receipts

The approved boundary is implemented in the six paths recorded by
`/tmp/agent-runs/admin-operator-boundary1-frozen.json`. Migration 27 landed only
after the contiguous migration 26 pair existed and its author confirmed actual
PostgreSQL upgrade tests. Temporary candidate SQL under admin testdata has been
removed. The canonical 26→27 test preserves every existing table fingerprint,
including historical audit, wallet, entitlement and ledger rows; replay creates
no decisions, empty down restores head 26, and a populated down refuses while
retaining decisions/results. Neither applied migrations 1–25 nor migration 26
were edited by this lane.

`AdminOperationStore.Decide` requires a bound initiating admin session, captures
only the product-supplied affected account list, sorts/deduplicates it, validates
the typed command and binds its canonical digest to actor/request identity.
Room commands check current physical owner authority before sorted account locks;
all commands then lock the actor role/exact session, acquire a transaction
advisory lock for request identity, and recheck wall-clock session/status after
that wait. Decisions and their audit are atomic. A replay may not change actor,
targets, amount or command, and cannot return success under a revoked session.
No decision grants value or changes a room by itself.

`CompleteTx` joins immutable delivery facts and a delivery audit to the caller's
domain transaction. Trusted workers can finish/replay a committed decision after
the original session is revoked; browser-bound completion/replay still requires
the exact live initiating session. A failed audit rolls back both the receipt
and a surrounding disposable domain marker. Concurrent completion creates one
result and one audit. The receipt identity lock uses an advisory transaction lock,
not SELECT FOR UPDATE on insert-only tables. A restricted-role test executes
Decide/replay/Get/Complete with only SELECT+INSERT receipt/audit privileges and
requires PostgreSQL 42501 for all nine table UPDATE/DELETE/TRUNCATE attempts.

The separate internal `OperatorHandler` exposes escaped, no-store history and
reviewable receipt details with request, actor, account, amount, source spend,
room/owner and reason fields. Unsafe forms have a 4 KiB limit, exact field names,
no duplicates, CSRF and five-second work context. Revoking session, rotating
CSRF, losing role, banning or suspending after middleware but before Decide
refuses with no decision, audit or ledger effect. An absent product hook returns
503 without recording a decision. It is deliberately not mounted by main yet:
room delivery, installation/account sanctions and Noin correction hooks remain
the separately reviewed boundaries 2–4; this boundary does not claim them done.

Evidence: actual missing-API RED `1317340`; nil-result scan fixture failure
`1347105` was diagnosed and fixed; actual typed-null CHECK RED `1363430` fixed
by requiring the complete kind expression `IS TRUE`; delivery/HTTP API RED
`1374063`; receipt-details RED `1413186` corrected before canonical GREEN
`1420368` (admin race 7.945s, actual migration race 3.526s). Insert-only API/ACL
proof `1424706` passed 1.365s. Final admin/store vet `1428381` passed. The full
admin race gate `1426815`, including 14 additional invalid SQL command shapes,
is running at this source-review boundary. Earlier accidental runner path and
missing host gofmt path were command setup errors, diagnosed without changing
assertions. Source review and final broader gate acceptance remain pending.

Alongside this lane, independent source review approved the coordinator's
accepted-contribution release journey. Cold PostgreSQL/Redis race `1422037`
passed 7.946s against head 27 after baseline's diagnosed temporary unused import
was corrected. This proves the service workflow with disposable screening/human
records, not a real provider, editorial approval or production pack release.

The frozen full admin PostgreSQL/Redis race gate `1426815` completed **PASS 75.558s**, including all existing authority/mutation/session tests and the new receipt, concurrent delivery, five post-middleware revocation races, insert-only ACL checks and fourteen invalid SQL shapes. All six frozen file hashes remain the source-review boundary; no further code edits followed the gate.

### Unified runner gamebot time budget — approved plan

The default unified runner still gives every Go package a 120-second timeout.
That is shorter than the existing authenticated ten-cell matrix, whose own
context allows 12 minutes, and the 100-room warm/load/soak test, whose context
allows 15 minutes. The completed F matrix took 701.60 seconds under race
instrumentation; its remaining selected tests took about 35 seconds.

After the B cold gate, add an exact `tools/gamebot` module override of **30m**:
12m matrix + 15m load/soak + 3m auxiliary setup/replay/cleanup allowance. Keep
120s for server, mediapack and newly discovered modules until specific measured
evidence warrants a separate change. Keep complete `./...`, `-count=1`, `-p=1`
selection and existing skip/failure accounting. Do not add `-run`, `-skip` or
`-short`; each test's own context and the performance assertions remain
unchanged. Native and Docker command-construction regressions must fail on the
old short bound, verify the exact module override and preserve complete
selection. Run the official Python runner suite red→green, then independent
source review and cold verification. The real combined gamebot package remains
part of the final unified gate after its owner's performance work is frozen.

The nine-check B cold run `retirement-b-cold-verifier--20260912T200247Z-1439961.log`
completed all checks and exposed two test-isolation premises in the full portal
suite: the retired simulator expected a globally empty ledger, and the new
coordinator publication journey counted earlier contribution receipts. Existing
retained ledger rows must not be truncated. The simulator regression now
captures exact table counts immediately before its requests and requires them
to remain unchanged; the coordinator owns the journey's immutable-ID scoping
fix. The independent reviewer approved this invariant correction. A preexisting
format issue in `portal/screen_test.go` was mechanically formatted with the
coordinator's authorization. No production change was needed. Full media,
mediapack, Python, vet and build checks passed in that run; final acceptance
awaits the corrected full portal rerun and format check.

### Cutover authority schema 26 — review boundary

Migration 26 is additive and empty: `cutover_instances`, `cutover_requests`,
`cutover_watermarks` and `cutover_handoffs` record explicit offline authority,
without granting roles, changing login state or activating runtime behavior.
The instance uses the actual PostgreSQL system identifier, database OID and
name; one physical tuple cannot acquire a second fresh identity. Composite keys
bind current request, generation, predecessor, watermark and handoff. A sealed
handoff is terminal for its source and names one target identity/physical tuple.
Restored target bootstrap requires that retained handoff and sealed source;
ordinary fresh bootstrap is allowed only in empty authority history.

Requests and the corresponding closing transition must commit atomically. The
new orphan-request regression demonstrated that an independently committed
request could otherwise occupy the next unique generation and make recovery
impossible after its lease expired (RED1430650). A deferred constraint now
rejects that commit, while preserving identical historical replay and subsequent
closed recovery (GREEN1433304, six tests). Watermarks hash PostgreSQL 16
`jsonb::text` UTF-8 bytes explicitly. Records are immutable, evidence and writer
inventory are bounded, stale/expired authority refuses, and populated down
refuses under exclusive table locks. Empty up/down retains every preexisting
head25 table fingerprint, including account, wallet, ledger, entitlement and
private purchase-receipt fixtures.

The separate target test creates a random sibling database only inside the
already token/name-guarded disposable cluster. It restores exact retained
schema rows with trusted owner triggers disabled only during that fixture
restore, reenables them, and proves wrong identity/watermark/fresh bootstrap
refuse, the declared actual target bootstraps once, and its own first closing
generation preserves the sealed source. This is a schema authority proof,
**not** an execution of the production backup tool or proof that all writers
are quiescent. The eight-test PostgreSQL race scope passed 12.324s in
`/tmp/agent-runs/cutover-schema-target-proof--20260912T200409Z-1446086.log`.
Final concurrent-generation and lease-expiry tests are running in 1456717.

The prerequisite inventory now covers head27 exactly: 77 application tables,
five sequences and 29 invoker trigger functions. Runtime receives SELECT only
on the four authority tables and existing migration/import tables; the two
admin operation receipts and historical admin audit table receive SELECT and
INSERT only. A correct insert-only fixture failed the old DML expectation
(RED1435481); updated exact privilege checks and real SQL denial tests passed
47 events, 0 failures/skips in 3.699s (1437774), independently source-approved.

The future offline controller is a **trusted operator boundary**. PostgreSQL
ADMIN membership capability can indirectly grant itself membership/SET access;
direct/default ACL denial is not a sandbox against a malicious controller.
Its credentials must stay outside runtime containers, application environment
and public routes. Physical login/connection fencing, joined writers,
controller lease checks and ordinary snapshot integration remain separate
implementation and verification gates. No role provisioning was performed
outside random disposable test containers.


Independent boundary-1 review found a destructive down-migration race: the old
empty check could run before an in-flight writer committed, after which DROP
waited for that writer and discarded its committed receipt. Actual RED
`1453235` reproduced that loss. The repair takes ACCESS EXCLUSIVE locks on
both receipt tables and the audit table before testing emptiness/removing
triggers, in the same PostgreSQL migration transaction. Test `1461818` passed
12.246s with real SQL writer-before-down and down-before-writer lock observation,
plus actual 26→27 retained-row parity, empty down and populated refusal. The
first attempt `1445400` only hit the existing fixture's one-connection pool;
the concurrency fixture now explicitly allows four connections for two
transactions and the pg_locks observer. The six-file freeze manifest has been
refreshed for this two-file repair; the handler/store API source is unchanged.

### Retirement B final cold acceptance

After independent source approval and the separately reviewed fixture fixes,
`/tmp/agent-runs/retirement-b-portal-cold-final--20260912T200557Z-1461748.log`
passed the complete portal package (**89 test events**, 26.467 seconds), including
the new accepted-contribution publication journey and retained-ledger retirement
invariant. The scoped portal/media formatter also passed. Results:
`/tmp/agent-runs/retirement-b-portal-cold-final-results.json`.

Together with the unchanged successful checks from the initial nine-check cold
run, accepted B evidence is full portal + full media (136 events) + full
mediapack (23 events) + Python (8 tests), formatting, vet and server/tool builds.
There are zero accepted test failures/skips; the earlier failed log and its
corrective evidence remain preserved. Disposable services were cleaned by their
own runner. The corrected fixture manifest is
`/tmp/agent-runs/retirement-b-fixture-final.json`, SHA-256
`bfb08ab6e75abffe18c13f76b41b704ad6bf4d9ce1b58d7b5228fe3c413e0da5`.

The final ten schema tests passed in 18.937s with PostgreSQL race checking
(`cutover-schema-final--20260912T200530Z-1456717.log`). They include competing
controllers choosing exactly one generation, transaction rollback preserving
recovery, and actual server-clock expiry refusing watermark/seal while allowing
new closed recovery. Coordinator source review approved both SQL files; the
independent combined 57-event schema/role gate follows before changing shared
privilege predicates. SQL26 SHA-256: up
`d10ccb87a13c9109d0728fc38bede41ba7b563d8cc5e246e4ce0eb144e54af45`, down
`fff7ec8a5571a4a4a455eb36bd7b5b80106dd600ff1ff4af7c1c61f7b25029fc`.

### Offline control privileges — approved next capability boundary

Keep `VerifyCutoverRoles` as the strict four-role prerequisite interface.
Add an explicit `CutoverControlRoleSpec` containing those roles, a distinct
`Control` name and the expected `WritersEnabled` state, with a bounded read-only
`VerifyCutoverControlRoles` entry point. It may run only from the exact capture
or control session identity. Share the existing exact object/catalog checks,
adding one explicitly named control identity and an exact membership allowlist;
no implicit control mode or credentials in runtime configuration.

The control LOGIN has CREATEROLE but no superuser, createdb, replication,
bypass-RLS or role-default inheritance. It holds ADMIN on exactly the runtime
and migrator roles with SET and INHERIT false; the existing migrator-to-owner
SET edge remains. It inherits only `pg_signal_backend` and
`pg_read_all_stats` with no SET/ADMIN on those memberships, and has explicit
EXECUTE on `pg_control_system()`. These are offline operator capabilities,
including cross-session statistics, never public application authority. Runtime
and migrator LOGIN values must exactly match the requested enabled/fenced state;
this checks state and does not change it.

Control gets SELECT on retained tables/sequences, INSERT on the three cutover
receipt tables, INSERT/UPDATE on the instance table, and no ordinary-table DML,
sequence advancement/reset, schema DDL, trigger disabling or parameter bypass.
Tests first exercise a separately connected real control role: bootstrap the
actual instance, commit a bound closing request, toggle only declared writer
LOGIN attributes, inspect an exact fixture backend and cancel it, and require
permission errors for forbidden direct table/sequence/role operations. Negative
provisioning tests cover extra/missing membership, SET/INHERIT/ADMIN options,
missing bounded stats/system capability, broad ordinary-data or function grants,
and wrong reader identity. No actual application is stopped and no snapshot is
certified by this capability boundary.

Primary PostgreSQL 16 references were checked for per-membership INHERIT/SET,
non-superuser CREATEROLE+ADMIN requirements and stats/signal scope:
[role membership](https://www.postgresql.org/docs/16/role-membership.html),
[ALTER ROLE](https://www.postgresql.org/docs/16/sql-alterrole.html), and
[predefined roles](https://www.postgresql.org/docs/16/predefined-roles.html).


### Admin operator boundary 2 — exact room delivery plan for review

Ownership proposed: new `lobby/text_operator.go` and tests, new
`store/admin_room_operations.go` and tests, narrow transaction adapters in
`text_value.go`, and operator-only fields/checks in `textRoom`, Join/Ready/Start/
Settings/runtime lock acquisition/Tick. Root retains final main mounting. No
new schema is planned. The existing typed decision stores the immutable room
UUID, process incarnation/generation and affected account set; a room code is
only an operator lookup/display aid and never a replay target.

1. Add a bounded manager method that holds the existing manager write lock,
   verifies live physical owner authority, locates that exact room UUID, captures
   its account/reservation/match identities, and calls `Decide` under those
   locks. Rejected authorization changes no room state. After a committed pending
   decision, retain a per-room pending-operation fence; Join, Ready, Settings,
   Start and player actions refuse while delivery needs retry. Tick retries the
   same ID before advancing that room. Other rooms remain usable. Keep an exact
   room-local exclusion set for kicked accounts, checked before reconnect/Join.

2. Add a transaction-scoped pending-receipt lock/read helper. Concrete delivery
   must acquire owner singleton authority first, then any match/week prefix,
   sorted affected accounts (plus initiating actor if browser-bound), then the
   operation advisory identity. Re-read immutable decision/result under that
   serialization and **skip all effects when already completed**, validating the
   expected command/room binding. `CompleteTx` remains the final receipt/audit
   writer; it must not be used as the first idempotency check after effects.

3. Waiting-room close/kick will use one value transaction to release the exact
   captured reservation IDs and append completion/audit. Prepared close will
   join the existing cancellation/compensation logic to that same transaction.
   Extract narrow transaction adapters from the existing value methods so the
   default paths retain all current owner, account, state and compensation
   checks. Audit failure leaves reservations/quota and receipt unchanged.
   After commit, update membership/host/revisions and emit the normal lobby
   projection; no HTTP or socket write occurs inside the SQL transaction.

4. Active close uses the existing engine Close/pending-candidate semantics. Its
   interrupted Finish hook dispatches an explicit operator value path that joins
   interrupt/compensation/private retained-award delivery and CompleteTx in one
   transaction. The engine commits its candidate only after this succeeds. A
   previously committed terminal result is preserved and the close becomes
   obsolete without another refund. Pending already-earned award work is retried
   through existing immutable occurrence identities before interruption; no award
   is inferred from the admin decision. Never invoke the engine while holding an
   outer value transaction: its normal hooks acquire their own transactions.

5. Active kick keeps the original seat and follows existing SetConnected(false)
   absence/offer/vote rules, closes only that account's current peer and bars
   reconnection to the same immutable room. Its **decision audit commits before
   this irreversible transport effect**; completion remains pending until the
   engine's existing persistence candidate and completion audit succeed. If the
   engine committed but receipt delivery failed, retry sees already-disconnected
   state and cannot reset grace, award again or rewrite a terminal outcome.
   Legitimate awards committed by ordinary engine advancement are retained;
   transport closure is not claimed to roll back on a later receipt failure.
   This distinction needs explicit review because memory/transport changes cannot
   be enlisted in the PostgreSQL transaction. No applied receipt or success page
   is returned before the actual kick and completion receipt both finish.

6. A process restart never delivers an old room decision into a replacement
   room/code. Confirmed-old-owner recovery first resolves its original reserved/
   prepared/started value work. Only then may a bounded trusted reconciler append
   an obsolete receipt for a pending old-owner room decision. It must prove no
   unresolved predecessor admissions/matches remain; no timeout pretends owner
   loss. Do not add a new independent scheduler until its writer/lease registration
   is coordinated with the all-writer control lane.

Tests before implementation: waiting kick/close releases exact admissions once;
prepared close audit failure rolls back cancellation/quota; all five modes and
4/6 seats active close preserve instant Noin and terminal outcome; exact target
kick changes no other account, preserves normal grace/offer handling and prevents
same-room reconnect; duplicate/concurrent request skips effects; receipt/audit
failure then retry preserves occurrence identities; replacement room UUID with
same display code refuses old decision; owner loss/recovery then trusted obsolete
receipt; revoked initiating session cannot submit or replay success; canceled
manager/database lock waits leave no new fence; pending room blocks admission and
player actions while unrelated rooms progress. Actual HTTP/WebSocket integration
will follow the same real PG/Redis runtime fixture, with no private role/earned
points in operator views and no synthetic production eligibility claim.


Boundary 1 (including the reviewed down-race repair) is independently approved.
The updated freeze-manifest SHA-256 is
`f7bc89ef34341650288a07db1d5c25a4ff03e0a82ae79e66ac704c70e5e09f63`.
The boundary-2 plan above is awaiting review; no room-action source has changed.
Independent combined frozen migration-26/head-27 role verification subsequently
passed: `cutover-head27-independent--20260912T200812Z-1479510.log`, real PG/Redis
race **39.132s**, and store vet `1479515`. Pre-run hashes matched approved
26-up `d10ccb87a13c9109d0728fc38bede41ba7b563d8cc5e246e4ce0eb144e54af45`
and 26-down `fff7ec8a5571a4a4a455eb36bd7b5b80106dd600ff1ff4af7c1c61f7b25029fc`.
That verification covers the ten schema cases plus the role suite with explicit
insert-only receipt/audit privileges; it does not establish operational all-writer
quiescence or authorize live backup/cutover.

### Coordinator verification and documentation reconciliation — 20:11 UTC

Public snapshot/history cloning and private match-state cloning are independently source-approved. Full actual PostgreSQL/Redis race1448400 passed v2 10.282s, config3.093s, game4.096s, lobby8.792s and handler25.283s after both clone edits. Whole-shape equality/alias mutation proofs and monotonic/fixed-zone occurrence clock identity passed. Benchmarks1419148 measured private typed40.6µs versus JSON2185µs on the same populated fixture; this is microbenchmark evidence, not the outstanding100-room acceptance. The unchanged 100-room workload1458218 is running. Previous1319927 performance/memory failures remain diagnostic evidence until that gate is repaired.

Contribution lifecycle source was independently approved and scoped1422037 passed. Full portal verification exposed a test-only global reward-count assumption: other retained fixture contributors also had legitimate rewards. The assertion now scopes to the exact newly created contributor accounts, retaining original per-contribution credit/balance and global before/after hash checks. Full portal1451736 passed16.569s. Baseline's independent full portal1461748 passed26.467s; the accepted B gate includes107Go top-level/248events and8Python tests, with no failed/skipped accepted result.

E3 removes the final live cosine band fields and YAML after their last runtime consumers were retired; frozen version1 policy schema/bytes remain untouched. New early-key/reflection regression1441887 failed on actual obsolete acceptance, then full config1445196 passed. E3 source independently approved. The sole Python Pillow CI installation is removed after the image tool became an explicit zero-I/O refusal. Avatar CGO/WebP remains intact.

Operator decision/receipt boundary1 is independently source-approved after a real down-migration race1453235 was repaired with exclusive table locks before the empty check. Root1495911 passed actual PostgreSQL/race admin22.686s and store13.914s, including both writer/down orders. No room/sanction/wallet product hooks are mounted yet. Migration26 schema source-approved and independently verified1479510 (ten schema tests plus47 role events), store vet1479515; the actual all-writer barrier is still being implemented.

The official runner's exact gamebot module timeout is30m for its12m mode matrix,15m load/soak and3m auxiliary allowance; other modules remain120s and no tests are excluded. Source review approved; independent1516196 passed21runner tests. Current Blueprint, curator source map, content module and community operations now describe implemented v2 boundaries and explicit remaining release gates, preserving historical audit references. Phase2 Unicode/lifecycle and Phase3 lock-parent boxes are checked on their independent evidence; human/content/provider/cutover/performance gates remain open.

### Server intent timing and warmed-soak measurement — plan for review

The existing100-room harness uses client intent-to-ack time as a conservative upper bound for the Blueprint's server accepted-intent-to-event budget. Profiles show substantial client decoding, canonical history hashing and trace validation in that combined process; they cannot isolate server latency. Add an optional in-process accepted-action duration observer to the actual WebSocket handler, timing from a complete received text frame through rate/auth/validation/room locks/persistence and all emitted recipient snapshots plus ack. Observe only successfully accepted actions and pass only public mode plus elapsed duration; no account, room, request, secret content or token labels. This observer is injected in the disposable network fixture and does not mount new telemetry routes or external analytics. Existing client RTT/raw samples remain recorded and its current upper-bound assertion remains unchanged while collecting comparative diagnostics. Test actual WebSocket success, duplicate, denied/stale/malformed actions and a blocked owner/room hook, proving exactly one accepted observation after ack enqueue and inclusion of time waiting on server dependencies.

Heap profiling1319927 showed final post-test GC retained about2.28MiB, mostly runtime goroutine stacks, thread allocation, JSON type metadata and timer pools; the in-test first-GC measurement was30.5MiB. Current warm-up uses10rooms before measuring100 and retains testing.Log buffers throughout. Before changing acceptance measurements, freeze a matched-load methodology: warm the same100room/500socket population and paths, fully finish/ack/leave, release fixture-owned traces/sample slices, use the same two explicit GC cycles at baseline/final (sync.Pool victim generations), and retain raw timing data in a bounded external test artifact rather than testing.Log heap. Exact10%threshold, all100measuredmatches, both-size/mode samples and zero resource/goroutine requirements remain. No process leak is excused by a changed warm-up, and initial failed runs remain recorded. A fresh final frozen-source run must pass; neither this plan nor microbenchmarks close the gate.


### Profile writer retirement — bounded implementation plan

CodeGraph plus the unresolved call-site scan shows the old profile match writer
and conversion writer have only retained test callers. Active text settlement
owns durable match/profile results; the active conversion endpoint calls
`economy.Manager.ConvertPoints`. This slice removes unused executable alternatives
without changing profile visibility, avatar decisions, deletion, scoring or XP
policy. No migration or test-file deletion is planned.

Production removals in `server/internal/profile/profile.go`:
`ApplyMatchResult`, `ConvertPoints`, `boolInt`, `xpForMatch`, `levelForXP`, the
non-transactional `AddContributorCredit` and `AddWeekWinnerTitle`, and the test-only
`EnsureProfile` account backfill. Remove the unused stored progression field and
constructor parameter. Keep `AddContributorCreditTx` and `AddWeekWinnerTitleTx`
exactly as the canonical portal approval/challenge transaction helpers; update
only their comments that currently refer to the deleted wrappers. Preserve
`Get`, nickname validation/update and both avatar update entry points.

Narrow constructor-only call-site updates: `cmd/knowoffd/main.go`,
`internal/handler/challenge_test.go`, `internal/handler/avatar_profile_test.go`,
`internal/portal/portal_test.go`, `internal/admin/mutation_authorization_test.go`
and `internal/admin/operations_workflow_test.go`. These do not change runtime
assembly, authorization, or the parent/peer-owned test scenarios. The existing
`profile_test.go` remains the test surface; test-only account creation moves into
that file with fresh UUIDs and unique nicknames. Its database setup must require
the existing disposable nonce/name/host/actual-current-database guard before any
migration or fixture writes; missing services fail instead of silently skipping.
No broad account/ledger truncation is used.

| Retained invariant | Replacement proof |
| --- | --- |
| Ensure/Get nickname and initial level | Explicit test-only account/profile creation, same public Get assertions; no production backfill API. |
| ApplyMatchResult matches/wins/votes/pokes, both point totals, positive XP | Real owner acquisition/recovery, Reserve/Prepare/Start, committed correct-vote receipts, Finish/SettlePending. Assert exact original 45 points, vote/poke counts and pinned XP; repeat Finish and settlement and assert no duplicate profile/ledger/outbox effects. |
| ConvertPoints 250 overall, 200 converted, 50 remaining, 2 Noin, insufficient balance refusal | Seed retained convertible points, call the actual economy conversion with loaded tuning, then assert wallet + ledger + daily cap accounting as well as unchanged Overall Points. Invalid multiples, insufficient balance and capped credit leave the profile/wallet/ledger unchanged. |
| No dormant unfenced profile writers | A source-declaration test first fails on the exact retired method names/helpers and constructor dependency, then passes after removal; runtime result/conversion assertions remain the behavioral proof. |
| Current Week Winner versus lifetime count | Preserve existing canonical singleton/title assertions and run the real portal challenge close tests. |
| Avatar preset/revision/privacy and historical blobs | Preserve existing positive/negative avatar and public/owner profile tests unchanged in meaning. |
| Contribution/crown transaction writers | Run existing approval/payout/weekly lifecycle and mutation-authorization regressions with the retained transaction helper calls. |

Reviewer approval precedes source removal. First capture the refusal regression
RED and replacement behavior baseline, implement only these removals/call sites,
then freeze for independent source review. Cold verification covers full profile
and focused active conversion, portal approval/weekly/current winner, avatar
profile HTTP and admin mutation regressions with real disposable PostgreSQL and
race detection, plus affected-package format/vet and the full server build. The
coordinator's final unified gate retains every package and test.

Control capability implementation reached source review with 66 passing events
(seven top-level tests), no failures/skips, actual PostgreSQL race 9.120s:
`/tmp/agent-runs/cutover-control-explicit-grant-green--20260912T201536Z-1601379.log`.
Missing typed API RED1531083 preceded implementation. The first full capability
run 1558708 passed the real control operations and old four-role tests, but the
missing explicit system-identity grant case found that effective function access
remained through PostgreSQL16's default PUBLIC EXECUTE. The corrected checker
requires an exact non-grantable ACL entry for the declared controller, in
addition to effective access; it does not claim that this read-only built-in is
otherwise restricted. All assertions stayed intact. The earlier physical-ID
plan text was corrected to distinguish default reads from actual authority.

The combined frozen schema26/head27 role boundary was independently verified
before this capability extension: `TestCutover` race1479510 passed 39.132s (57
events), store vet1479515 passed, and all five reviewed source hashes matched.
No SQL26 or27 changes were made during the control capability implementation.


### Unified runner budget — final acceptance

The exact-module 30-minute gamebot timeout was source-approved after the native
and Docker command-construction regression failed at the old 120-second value
(`runner-gamebot-budget-red--20260912T200718Z-1472017.log`). The independent
coordinator run `runner-timeout-independent--20260912T201037Z-1516196.log` passed
all 21 runner tests.
The official full xops Python suite then passed **49 tests, zero failures/skips**
in 560.208 seconds: `runner-gamebot-budget-green--20260912T200755Z-1475515.log`.
This includes actual nginx/TLS callback secrecy and fixture isolation, plus three
backup restores across legacy schema 8 and dynamically discovered current schema
27: 78 tables and 6 sequences with catalog/roles/ACL/rows/blob parity. Current
backup manifest SHA-256 is
`5d92f32f30449a8ffb0a14d7659208f268d144da9cb4ea332eab1336fb7f2db4`;
legacy manifest is
`cf670c29c4bf82a6275267e55a04dde6346ca227754a1542bc886f1eaee47133`.
No production writer-quiescence or live cutover claim follows from these isolated
fixture results. The exact gamebot full-package workload remains in the final
unified gate; this runner change excludes no tests.

**Physical implementation constraints discovered during mapping.** PostgreSQL
roles are cluster-wide. A distinct sibling database satisfies schema26's tuple
identity proof, but enabling the same writer roles there would also enable
source login. The first executable handoff should therefore require a different
cluster system identifier (as the existing separate-container restore rehearsal
does); same-cluster activation needs separate role/CONNECT isolation before it
can be supported. This is a physical activation requirement, not a defect in the
explicitly schema-only sibling-database fixture.

The original proposal's same-source rollback sentence also exceeds the approved
schema: a committed closing generation cannot return to ready and cannot create
another ready identity for the same physical tuple. Drain may be canceled before
durable closing; after closing, the implemented authority history stays closed.
Actual same-source reopening would require a separate reviewed additive protocol,
never a disabled trigger or rewritten authority row. No post-closing same-source
rollback acceptance is claimed by schema26 or the role capability tests.


### E3 independent compatibility verification

The live band/config/Pillow retirement is independently source-approved. Exact
retired-key and frozen-policy selection passed config **6 top-level/24 total test
events**, race 1.427s (`config-retirement-e3-independent--20260912T201801Z-1650923.log`).
The store part initially compiled during the independent admin-operation owner's
intentional missing-API RED window; no historical-policy assertion ran in that
attempt. Once that API was implemented, only the interrupted store test was
rerun: `config-retirement-historical-independent-20260912T202201Z-1663441.log`,
**1 top-level/1 event**, actual PostgreSQL race 1.443s, PASS. Existing serialized
version1 policy bytes/hash and durable replay therefore retain their accepted
compatibility evidence. Disposable services were removed; accepted tests have no
failures or skips.

### Profile retirement frozen review boundary

The original whole profile suite passed **7 top-level/11 total test events** on
real PostgreSQL/race before changes (`profile-retirement-original-baseline--20260912T201536Z-1601268.log`, 1.935s).
The retained match/conversion tests were then moved to the active durable paths
and passed before production removal (`profile-retirement-replacement-baseline--20260912T201844Z-1654265.log`, 1.550s).
An earlier replacement fixture compile failure1650271 was an int/int64 argument
error and was read/fixed; it is not behavior-change RED evidence. The actual
retirement declaration regression1658402 failed on all eight old functions and
the obsolete progression constructor dependency before removal.

The approved production removal and exact constructor call sites are frozen in
`/tmp/agent-runs/profile-retirement-frozen.json`, SHA-256
`69b167636a1c7baf99a487895b3782a08f0eb7b7e14c7e65c312f0c4e513351c`.
Whole profile race now passes **8 top-level/12 total events** (1.545s,
`profile-retirement-green--20260912T202019Z-1660518.log`). No test file was removed;
all avatar/current-title/read invariants remain, and the fixture no longer accepts
an arbitrary/default database or silently skips missing PostgreSQL. Independent
source review and affected-package cold acceptance are the remaining boundary.

### Room operator prerequisite — bounded manager waits

The approved boundary-2 delivery design keeps slow room work under manager read protection plus its room mutex. Actual regressions exposed Go writer preference: a queued Tick blocked unrelated readers and a canceled Create waited behind unrelated work (RED1569808). Context-bearing writer entry points now poll TryLock with the existing cancellable 1ms waiter, preserving their original mutation order. Contextless readiness/drain setters retain their existing background completion contract through the same waiter; Send and ActiveMatches use read protection. RejectAction now takes the still-live request context, locks only its recipient room, rechecks authority, and preserves the existing no-room error shape; its handler cancels work after response handling (missing-API RED1665530). The initial concurrency/availability/drain families passed real PG/Redis race1606018. Full lobby/handler gate1669188 is running. Source checkpoint: `/tmp/agent-runs/room-lock-prerequisite-frozen.json`. Room operator product plumbing has not been added to this checkpoint.

### Root load diagnosis and matched measurement implementation — 20:26 UTC

Run1458218 completed all100 measured matches in538.64s and returned room/peer/socket counts to zero and goroutines8→8. It **failed** the unchanged200/500ms limits: RTTp95=803.904566ms,p99=1.066484538s. Worst frame8185bytes remains within8192. Heap1,245,488→2,121,072 also exceeded10%; no budget checkbox is closed.

The reviewed observer now measures full text-frame receipt through accepted `Lobby.Action` persistence, recipient projections and acknowledgement enqueue, including rate/auth/room waits. Only public mode and duration leave the handler; denied/stale/malformed actions are excluded and accepted retries count as requests. RED1633582 failed for the missing typed dependency; actual PostgreSQL/Redis raceGREEN1667961 passed1.219s, including delayed authorization, successful retry and refusal controls.

The separately approved matched method is now implemented for source review:100rooms/500sockets for both warm-up and measurement; complete match/settlement acknowledgement/leave before two GC cycles at each boundary;15minutes per population inside33minutes total. Raw bounded numeric JSON timings go directly to captured process output, avoiding retained testing.Log sample buffers. A50,000-entry server observation bound reports overflow and exact server/client accepted-count mismatch; per-mode server timings are diagnostic only and existing RTT200/500ms assertions remain unchanged. A fresh frozen-source run remains required.

Profile legacy-writer removal received root source approval against its exact8-file pre-removal manifest; old-value tests now exercise owner-bound durable settlement/replay and active wallet conversion with cap/rollback assertions. Independent cold integration is pending. ConfigE3 independently passed6top/24events in1650923 plus historical-policy real PostgreSQL race1663441; frozen policyv1 serialization and durable replay remain unchanged. Full official xops Python1475515 passed49 tests including actual old/current restore to migration27 (78tables/6sequences/3restores).


### Profile and control cold acceptance; matched-load runner update

Profile retirement is independently source-approved. Cold verifier
`profile-retirement-cold-verifier--20260912T202357Z-1673709.log` passed all eight
checks and **86 unique Go top-level/149 total test events**, zero failures/skips:
portal67/89, profile8/12, handler5/10, admin3/35 and economy3/3, followed by clean
format/vet/full-server-build checks. The reviewed eight-file manifest stayed
unchanged. Machine report: `/tmp/agent-runs/profile-retirement-cold-results.json`.
The full portal suite ran before profile in the same disposable database, so this
proof also covers retained community records without a profile-specific reset.

The offline control capability extension received independent full-source review
and a fresh real PostgreSQL/race verification:
`cutover-control-capability-independent--20260912T202525Z-1681173.log`, all 66
test events passed in 5.940s with zero failures/skips. Exact role membership,
runtime/capture ACL constraints, narrow control-table writes and live backend
inspection/termination are covered; this is not a physical capture fence.

The coordinator subsequently approved a matched 100-room warm-up and 100-room
measurement workload with separate 15-minute inner deadlines. The unified
runner's exact gamebot budget is therefore updated from30m to **45m** (12-minute
matrix +15-minute warm-up +15-minute measured run +3-minute auxiliary allowance).
All other modules remain120s; no tests or inner deadlines are excluded/relaxed.
Native/Docker expectation RED1683274 was read, then all21 runner tests passed
`runner-matched-budget-green--20260912T202617Z-1683389.log`. Frozen two-file
manifest `/tmp/agent-runs/runner-matched-timeout-frozen.json` SHA-256
`7d6dd332bb0a0a4be3c6ff44938557f1c0d71b3ec0aa56ad7bd3f9485dc94f01` awaits
coordinator final source/independent check. Earlier 49-test Python/restore evidence
remains unchanged; long gamebot acceptance belongs to the matched workload gate.

Lock prerequisite full actual PG/Redis race1669188 passed (lobby8.823s, handler24.575s); vet1683505 passed all lobby/handler/store. Root source review approved the bounded acquisition changes before room product edits. Independently verified the reviewed cutover control-role boundary with real PG/Redis race1670042 (6.062s, exact requested Control/Roles/Runtime/Capture families) and store vet1683505; source hashes matched its frozen manifest. No live cutover/writer-quiescence claim.

### Admin room value adapter — review checkpoint

New ApplyRoomOperation requires the immutable live owner, exact room/match/reservation capture, and affected-account equality. Its transaction takes the owner prefix, match prefix if any, sorted account locks, and operation advisory identity; LockPendingTx returns an already completed receipt before any effect. Waiting close/kick release only captured reservations. Prepared close cancels; started close uses existing interruption compensation plus private retained-award receipts. Completion receipt and operator_applied audit are in the same transaction. Active kick completion only acknowledges already committed ordinary engine disconnect/transport work and grants no value. No fresh session proof is required for a committed trusted delivery retry; a foreground decision/replay still uses Decide's exact-session validation. RED1648151 missing API → actual PG/Redis race1656298 PASS6.142s including waiting audit rollback/replay, prepared/started compensation and retained awards. Review snapshot `/tmp/agent-runs/admin-room-value-frozen.json`. Runtime effects, terminal-preservation extensions, and confirmed-owner-loss receipt reconciliation remain to implement/test in this boundary.

### Local work gate — approved primitive boundary

The next all-writer substep is an in-process `transport.WorkGate`, separately
reviewed before any main wiring. It counts the full wrapped HTTP handler lifetime
without replacing ResponseWriter (including hijacked WebSockets), rejects new
work after Close, cancels registered workers, and joins their actual completion.
Wait remains bounded by its caller and cannot mistake an open idle gate or a
worker cancellation request for completed quiescence. The zero value works and
Close is irreversible/idempotent. Real HTTP/hijack, cooperative and held-worker,
canceled-wait, panic and concurrent-start tests precede implementation. This is
only local lifecycle accounting: it does not fence database logins, stop other
processes, certify a snapshot, or reopen a durable closing instance. Main/service
integration stays outside this primitive review boundary.

### Work gate next integration — bounded plan for review

The primitive's initial six request/worker tests passed race1701356 after
missing-API RED1692566. Its final review set also covers parent cancellation
without closing the gate and races 100 registered workers plus 200 entrants
against eight closers. No running service uses the primitive yet.

Proposed executable integration uses separate request and worker gates. Public
and admin application muxes are wholly behind the request gate, including GET,
OAuth, portal and receipt routes; exact health/readiness handlers remain outside.
There is no cutover controller HTTP exception. Every one of the six current main
background loops is registered with the worker gate. During orderly shutdown,
readiness/admission close first, the request gate refuses fresh application work,
and existing gameplay requests plus clock/delivery workers retain the existing
bounded graceful-drain opportunity. After text closure, worker cancellation and
joining precede successful shutdown; HTTP/hijacked request completion is joined
as well. A failed join returns failure and cannot certify capture. Listener
failure and owner-loss exits use the same cleanup accounting, with no fabricated
successful drain. The application owner heartbeat remains owned by TextOwner and
must actually Release/finish before the process is considered stopped.

Implementation must first add real bounded integration tests for a held HTTP
handler, an active hijacked request, and a worker whose cancellation is delayed;
readiness cannot recover after close, new auth/receipt/admin work cannot enter,
and a join timeout cannot report success. Existing runtime owner-loss/grace,
physical socket/outbox, and startup refusal tests must remain unchanged in
meaning. This is a main lifecycle step only: standalone commands and independent
processes remain covered by the later durable/physical source fence. No ordinary
capture permission is implied by a local gate count.

#### Local lifecycle integration — concrete approved mapping

Three listeners are tracked through their actual ListenAndServe return: public,
admin, and metrics. Public/admin application handlers share the request gate;
only exact health/readiness remain outside it. Metrics serves process collectors
and has no application writer routes; its listener is still shut down/joined.
The worker gate owns all six loops: dependency health, notice refresh, provider
reconciliation, community maintenance, text Tick, and delivery retries. The
WebSocket handler already joins its writer goroutine and durable Disconnect in
its defer, so the outer request count spans those effects as well.

Shutdown closes readiness and request admission, preserves the existing bounded
text grace while Tick remains alive, then closes text peers/owner, cancels workers,
closes listeners and joins both gates. Listener errors, owner loss and signals
use the same accounting and return joined failures. Health/notices' contextless
SetReady/SetDependencyReady calls remain inside their registered worker/request
lifetimes; a callback still holding a manager wait keeps that task outstanding.
Wait timeout reports failure, never a successful capture barrier. The new
shutdown path itself uses cancellable DrainContext/ActiveMatchesContext methods,
including per-room locks before reading game clocks, so it does not block behind
contextless Drain/ActiveMatches. TextOwner.Release gains a bounded queue wait;
if cancellation prevents acquiring its mutex, it reports failure and leaves
physical authority intact for an explicit later Release/process termination.
The owner watch gets an actual completion join, distinct from its Done/loss
signal. No callback is detached and called completed because its context expired.

The later startup-state slice is separately reviewed. It will be read-only and
must precede owner acquisition/listeners: no cutover rows means initial
unprovisioned bootstrap, with no claim of capture eligibility; existing history
requires the exact observed physical database tuple's ready instance, and a
restored or closing/sealed tuple must refuse. It never inserts a ready instance,
grants a role, enables LOGIN or treats read-only defaults as a fence.


### Additional Phase 5 closures — 2026-09-12 20:41 UTC

Independent baseline audit closes parent items 1 (additive schema/backfill) and 4
(confirmed owner-loss interruption). `text_value_test.go:275` exercises actual
8→10/repeated-up/empty-down/populated-down behavior. `text_archive_test.go`
exercises bounded 8→11 copy, verification, restart and source drift, preserving
NULL legacy mode/language/revision; cold runner1475515 restores actual head27
(78 tables, six sequences) three times with parity. No legacy row becomes
reviewed playable content.

Owner-loss tests in `text_owner_test.go` prove actual session loss, preserved
committed awards/terminal outcomes, rollback after participant effects and
successor recovery. Full store race1233681 passed in76.539s; these proofs and the
independent audit substantiate interruption compensation only, while the broader
settlement and shared-cap failure matrices remain under additional verification.

Profile API retirement cold1673709 passed86 top-level/149 terminal Go tests
across portal/profile/handler/admin/economy, plus format, vet and complete server
build. Root source approval and frozen manifest69b167636a1c7baf99a487895b3782a08f0eb7b7e14c7e65c312f0c4e513351c
are recorded above; this supersedes the prior cold-verification-pending note.

The new all-mode/size engine-boundary pinning proof passed race1711280(14.542s)
and independent source review. A final fixture-only change gives the replacement
match its own UUID and describes its release as a replacement (not certified).
Final verification remains pending for these exact bytes.


### Small reachable action proof — plan for source review, 20:46 UTC

**Goal.** Add exhaustive finite action-ownership checks that complement full
5+3 sampled matches without certifying human suitability or all reachable states.
**Boundary.** Test-only, zero-value engine fixtures; no rule/tuning default,
production availability or content approval changes.
**Files.** New `server/internal/game/text_exhaustive_test.go` (inventory confirmed
no existing file), existing game fixture helpers only.
**Checklist/tests.** Enumerate all legal mode actions for two consecutive turns
of a reduced one-hand/one-reserve fixture at both4/6 sizes: every hand instance,
optional draw, each1–5 rating/occupied slot/other eligible display/current chain
target, accept/refuse/timeout and ordinary timeout. Every enumerated edge runs
through actual Apply/Advance, checks conservation and expected hand/board/history
effects, refuses foreign/stale copies without changing committed state, and
replays accepted requests without mutation. Enumerate exact node/edge/leaf
denominators and bound the traversal. Separate full clock paths with one hand
and no reserve prove next-round no-refill and no-card pass, retaining history.
**Risks.** Reduced tuning must receive its correct pin and remain explicitly
synthetic. Explore prefixes through fresh engines instead of copying live mutexes
or injecting impossible private state. This is exhaustive only for the declared
two-turn finite fixture; production5+3 schedules and human choices are separate
proofs. Source review precedes cold race verification.


### Final pinning proof and load finding — 20:47 UTC

Final fixture bytes pass cold race1774910: game16.119s, media31.493s,
store36.885s. Independent source approval plus real media activation/refusal,
durable release/restart/takedown, and all-ten-cell engine reconnect/history/
verdict tests close Phase2 item3. These separate boundaries are explicitly
combined evidence; the engine test is not itself a catalog publication test.

Matched100-room warm+measurement run1688115 failed after843.04s with an actual
`top_that/4` request.unavailable during measured step110–120. Warm100 matches
completed; measured100 did not, so no final latency/heap/cleanup denominator is
claimed. The run overlapped other cold/race tests and source changes; preserve
its exact input hashes and failure. Read full tail/log; investigate the bounded
request refusal before a fresh frozen and uncontended measurement. No retry or
latency/heap budget change was made.

Baseline independently approved the small reachable-action proof plan, requiring
committed-state comparisons separate from recipient cursor increments and an
independent ownership/score/history effect oracle. Implementation follows.

### Admin operator boundary 2 — implemented room delivery and recovery review

The revised approved implementation uses manager read protection plus the exact
room runtime mutex during decision/persistence, followed by a bounded manager
write acquisition only for membership cleanup. It never upgrades an RWMutex or
holds a queued blocking writer while another room persists. The earlier plan's
manager-wide write-lock description is superseded by this reviewed scope.
Existing Tick retries pending operations; all player mutation/reconnect paths
respect the same room fence and kicked-account exclusion. Active kick retains
the original seat and the first disconnect/grace occurrence. Failed completion
audit leaves a durable pending receipt after the legitimate engine/transport
change; it does not invent a rollback of socket closure.

Store review extensions now prove whole-row value equality after injected audit
failure for waiting, prepared and started close; terminal-result precedence;
concurrent kick acknowledgement with no value writes; captured-account, owner
and generation refusal; and restart receipts only after confirmed predecessor
loss and completed compensation. Recovery appends an obsolete audit/receipt and
never rewrites compensation or retained awards. The runtime loop API is
`RecoverRoomOperations(ctx, limit)` after `RecoverLostOwners` reports Done;
continue until the returned candidate count is less than limit. Runtime assembly
is owned and independently tested by the lifecycle lane.

A lost decision response needs an additional tentative room fence. The trusted
`ResolveRoomDecision` waits on the original sorted accounts and operation
advisory identity, then compares the exact actor/command/account digest. It can
confirm only an existing decision. A missing account, RLS restriction, timeout
or unavailable database cannot masquerade as proven decision absence. A
serialized absent/conflicting decision clears only the same still-tentative
fence; confirmed work is retained for delivery. The probe never calls Decide,
creates a decision, or upgrades a failed foreground response into authorized
success. Every browser replay still uses the initiating exact-session check.

Evidence before final combined verification:

- Value/audit/terminal/refusal/concurrency extensions: real PG/Redis race
  `1716568` PASS 13.210s; confirmed-loss recovery extensions `1733704` PASS
  14.369s, including injected recovery-audit failure and retained instant award.
- Waiting operations, all five modes × 4/6 active close, and unrelated-room
  responsiveness under withheld persistence: `1721373` PASS 25.612s.
- All ten active-kick cells with completion-audit failure, transport closure,
  unchanged first grace and replay/exclusion: `1750251` PASS 20.207s. The prior
  `1742042` failure was a fixture policy hash mismatch after changing grace;
  rebuilding the bound value store from that same policy corrected the fixture.
- Actual authenticated admin HTTP → player WebSocket kick/close/reconnect/private
  settlement across all ten cells: `1770474` PASS 10.868s. The earlier `1761287`
  test expected the wrong wire name; actual private type is `settlement`.
  These use explicit prototype fixtures; they do not certify content or providers.
- Lost-response fence RED `1782334` and serialized original SQL commit/rollback
  RED `1787071` → focused GREEN `1799589` (store 2.358s, lobby 2.686s).
- Self-review additionally reproduced a failed foreground response being
  upgraded by a trusted probe: actual RED `1804407`. The repaired path preserves
  that error and leaves confirmed work for Tick. Full lobby/handler race gate
  `1809307` is running, alongside broader selected store/admin gate `1802561`.

Source review snapshot: `/tmp/agent-runs/admin-room-boundary2-frozen.json`,
SHA-256 `a3e28ab14526cfceafcd9305529e5921a21cddb623c1d86fb2ff217bbb4ad339`.
It records the new value/runtime/network files and narrow shared entry/fence
adapters; migration 27 is unchanged from its independently approved boundary.
Admin room discovery/UI mounting, account/installation sanctions, Noin correction
and leaderboard operator controls remain the subsequent reviewed scopes.

### Local lifecycle integration — source freeze for review, 20:52 UTC

The executable now registers all three listener loops and six application
workers in the worker gate. Public/admin application requests retain their full
HTTP or WebSocket-handler lifetime in the request gate; only exact health and
readiness endpoints bypass admission. Shutdown closes fresh request admission,
preserves the game clock through the configured grace, performs independently
bounded interruption/compensation, then cancels workers, closes listeners and
joins actual request/worker returns. An incomplete join stays an error even if
work finishes later. Confirmed physical owner loss skips gameplay grace and
fails closed; it does not publish a successful quiescence receipt.

Context-aware lobby drain/count APIs bound manager and per-room lock waits and
count pending preparation/operator cleanup as unfinished rooms. Owner Release
now bounds its mutex queue wait without discarding healthy authority merely
because a waiting caller canceled; Owner.Wait separately joins the heartbeat.
Startup also injects the actual operator store and drains bounded obsolete-room
receipt recovery only after confirmed previous-owner value recovery, before
opening listeners. No controller credential, control route, SQL26 change or
physical capture fence is introduced here.

Test evidence: queued owner Release regression RED1739455 → GREEN1746701;
missing shutdown APIs RED1757847 → real PostgreSQL race1765085 (15 passing events,
store22.381s/lobby1.322s); lifecycle API RED1776301 → GREEN1787162; bounded room
recovery API RED1795579 → final runtime assembly race1806458 (22 passing events,
31.195s, no failures/skips). Final assembly coverage includes four real authenticated
WebSockets with private interruption outboxes, three actual listener returns,
held handler/worker refusal, health exceptions, prototype zero-value parity,
previous-owner loss/recovery and all-ten-cell persisted release admission.
Frozen file hashes: `/tmp/agent-runs/runtime-lifecycle-frozen-20260912.json`.
Independent source review and post-review cold verification follow. This proves
local process completion only; startup cutover-state guard and external physical
writer fencing/watermark/restore handoff remain separate unfinished work.


### Value failure matrix verified — 20:56 UTC

Root reviewed both frozen files; independent cold1812869 passed45 top-level/94
terminal tests with zero failures/skips: store63.110s, economy9.732s, plus format,
vet and full server build. Manifest8b2ff09fb7558be132b0fbcce20155ed6d787616d5a4b4b2d8a1a1ce1834200a
remained unchanged. The only review correction selected a possible Nower for
the new four-seat correct-vote cap fixture.

Twenty real PostgreSQL AFTER-trigger failures prove exact15-business-table
rollback and retry at award, outcome and per-account settlement writes; prior
committed awards survive. Five SQLSTATE schedules distinguish bounded
serialization/deadlock retries from nonretryable failure and preserve identity/
UTC occurrence. All-five-mode reservation/preparation/start races, shared final
allowance, original-day replay, leaderboard N→N+1/day reset and actual Award vs
ConvertPoints cap concurrency complete the missing tests for P5 items2/3/5.

Independent audit of existing actual vote-grant, private-delivery, low-population,
absence and zero-value prototype tests closes item6. Rating/trade actions have
no instant award branch; committed role-linked receipts remain private until
authoritative terminal delivery. No production value change was required by
this additional proof slice.

### Runtime cutover startup guard — bounded next plan for review

Add one read-only `store.CheckRuntimeCutover(ctx, db)` guard, invoked by
`newTextRuntime` immediately after the compiled schema check and before owner
acquisition or recovery writes. Use a bounded transaction with row security
explicitly disabled so a hidden registry row cannot look like an empty registry.
An empty registry permits initial ordinary startup only; it creates no instance,
role grant, capture eligibility or handoff. If any registry history exists,
require exactly the actual database's `(system_identifier, oid, name)` ready
instance. Closing, sealed, restored-but-unbootstrapped and identity-mismatched
registries refuse with a stable error. No caller-supplied physical identity.

The actual physical identifier comes from PostgreSQL16 `pg_control_system()`,
whose current control-data implementation reads the control file. Prove it with
the existing provisioned runtime role before proposing any ACL changes. A denied
read fails closed; do not add a privileged wrapper, pg_monitor membership or
controller credentials. Primary references: [PostgreSQL16 control data functions](https://www.postgresql.org/docs/16/functions-info.html#FUNCTIONS-CONTROLDATA)
and [PostgreSQL16 implementation](https://raw.githubusercontent.com/postgres/postgres/REL_16_STABLE/src/backend/utils/misc/pg_controldata.c).

Tests first in a new `store/cutover_runtime_test.go` (discovery required before
creation), and existing executable integration tests: initial empty read leaves
all rows unchanged; actual provisioned runtime reads physical tuple and accepts
ready; closing/sealed refuse; altered identity/restore fixture refuses; denied
SELECT or RLS cannot fake absence; blocked query/cancellation is bounded; actual
`newTextRuntime` refusal occurs before `text_process_current` or recovery changes.
Any fixture-only identity alteration must be explicit isolated restore simulation,
not a claimed executable cross-cluster handoff. Existing migrations26/27 and
strict role checker remain frozen. Scope is normal service startup, not an
online writer fence or permission to run standalone migration/nightly/admin
commands during capture. External process stopping and physical role/connection
fencing remain mandatory for every writer before a watermark can be issued.

### Phase 7 offline cohort computation — proposed bounded plan, 20:56 UTC

**Goal.** Build reproducible local calculations for Phase 7.6a and the related
activation/completion/queue denominators; synthetic evidence verifies formulas
without claiming recruited people, elapsed retention, approved collection or a
launch decision. Definitions come from BUSINESS_PLAN acquisition experiments and
KPI definitions (lines 113–131 and 296–339), plus Blueprint Product Baseline.

**Inventory and boundary.** CodeGraph/source discovery found no cohort calculator
or analytical export. Existing `server/internal/reports` is the conduct/content
case service; it must not be repurposed. The active `/metrics` surface provides
aggregate connection/action timing. Durable `text_matches`/`text_admissions` and
terminal outcomes preserve match/value facts, but do not establish first-session
arrivals, recruitment cohorts, voluntary intent, invitation attribution, complete
queue waits or observation completeness. Never infer these missing facts from
account creation, sockets, receipts or client opens. No collector, SQL migration,
nightly job, live database query, SDK, external service, spend or publication is
in this first slice. Access/deletion/retention policy remains a prerequisite to
collecting real people’s data.

**Proposed files.** New `tools/cohort_report.py` (stdlib offline calculator/CLI),
`xops/test/test_cohort_report.py` (discovered absent), narrow `tools/README.md`
usage and this evidence record. Reuse unified Python discovery; no new module,
dependency or public/server endpoint. Output is an exclusive 0600 local artifact
with no individual identifiers; stdout contains only report status/hash. Inputs
are bounded regular synthetic fixture files, strict versioned JSON, unknown keys
and duplicate/conflicting identities refused. No live-input switch is supplied.

**Definition contract.** A manifest freezes formula version, experiment/cohort
window, UTC observation-complete watermark, rules/pack/build identity and exposure
cells. Explicit opaque fixture subject/session/group IDs join events; neither
raw account/device IDs nor names, chat, prompts, hands, roles or currency amounts
are accepted. New-eligible-human status, session start/end, voluntary versus
forced repeat and hosted status are input facts, never guessed defaults.

- Activation: a new eligible human starts and completes a valid normal match
  inside the same explicit first session. Report against all new arrivals and
  separately queue entrants; abandonment remains in denominators.
- Completion: normal completed matches divided by all started human matches in
  each exposed cell; low-population, forfeit and infrastructure outcomes remain
  failures, while still-active or unknown outcomes are separately incomplete.
- Second match: unique activated humans completing a distinct voluntary valid
  match within seven days of activation; exact cutoff is frozen in the manifest.
  Forced tutorial repeats do not qualify; hosted cohorts remain separate.
- D1/D7: a distinct valid completion on UTC calendar day 1/7 after activation.
  Include only subjects whose entire target day is covered by the observation
  watermark; separately show immature/excluded/missing counts and coverage.
  Do not report an immature cohort as having zero returns or meeting its gate.
- Durable participation: distinct eligible humans with at least two valid
  completions on different UTC days in a declared week, segmented without
  pooling weaker mode/language or access groups. Queue joins retain their own
  identity across retries; start-within-configured-timeout and prestart leave
  use all joins, plus deterministic p50/p95 waits and unresolved joins.
- Invitation/opt-in intent remains separate descriptive counts for explicitly
  supplied stages. The business plan defines no invitation conversion threshold
  or attribution window; absent definitions produce `unavailable`, not a new
  invented success gate. Laughter, role balance, financial rate comparison and
  human comprehension observations are later separately scoped reports.

**Ordered implementation and tests.**

1. Freeze the strict fixture event/manifest schema and write malformed, duplicate,
   conflict, oversize, nonregular-input and forbidden-private-field tests.
2. Implement canonical identity joins and exact outcome/session classification;
   fixtures prove reconnect/receipt duplicates do not create extra humans,
   matches, queue joins or activations, and missing facts stay explicit.
3. Implement UTC activation/second/D1/D7 calculations; test midnight, leap day,
   exact cutoffs, D7 not elapsed, forced repeats, hosted separation and whole-day
   maturity. Test per-cell denominators and prevent aggregate-only pass claims.
4. Add weekly participation and queue calculations with tied waits, timeout
   boundary, missing terminal events and separate leave reasons. Provide raw
   numerators/denominators, cohort dates, exclusions and sample/group counts;
   any independence-based interval is labelled screening-only and correlated
   samples never automatically pass a balance or commercial gate.
5. Implement deterministic private output and a deletion/recomputation fixture: a
   removed subject contributes no rows or output IDs and cannot be restored by
   replayed stale events. No retained financial history is touched.
6. Review source then run cold fixture tests plus CLI/format and unified Python
   checks. Record synthetic-only results. Later authoritative export/nightly
   integration needs a separately reviewed minimum-event, access, pseudonym
   key rotation, retention/deletion and complete-watermark contract.

**Risks.** First-session boundaries, voluntary intent, invitation attribution and
missing queue data are unavailable in current durable sources; refusal and
explicit availability fields prevent invented denominators. Pseudonyms remain
sensitive, so this slice accepts synthetic fixtures only and produces private
aggregate artifacts. Passing calculations do not close Phase 7 human/cohort
parent items or authorize launch, analytics collection or retention policy.
