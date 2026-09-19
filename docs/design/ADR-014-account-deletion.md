# ADR-014: Account deletion with explicit privacy transformations

- **Status**: proposed
- **Date**: 2026-09-19
- **Deciders**: Knowoff engineering; independent architecture review pending
- **Policy**: [Blueprint Profile §1a](../../BLUEPRINT.md#1a-account-deletion-and-retention)
- **Implementation sequence**: [ROADMAP](../planning/ROADMAP.md), D0–D6

## Context

The adopted deletion policy requires immediate access/publication suppression,
active-data removal within 30 days, minimized purpose-restricted evidence for
180 days and backups expiring within 90 days of creation. Schema 31 deliberately
protects consent, value, shared match outcomes, content provenance and admin
history. Removing an account through cascades loses other players' evidence;
changing hashed JSON in place breaks outcome replay and certification. A mere
`deleted_at` update is also insufficient: `LockValueAccount` refuses it, so
accepted match cleanup/settlement would otherwise strand surviving players.

## Decision

Use the existing account/domain locks and immutable receipt conventions to run
an explicitly inventoried, bounded privacy executor through a separate database
principal, settle surviving value before transforming shared proofs, and require
an independently durable suppression receipt before confirming deletion.

This is a proposed executable design, not evidence of implemented erasure,
configured production processors, or a deployed suppression service. D1 adds a
closed foundation only. Runtime deletion routes stay unavailable until the joined
D6 journey proves every applicable class below.

## Consequences

- Credentials, authored bytes and public statistics receive no blanket financial,
  archive, licensing or immutable-history exemption. A disabled UUID remains
  pseudonymous, not anonymous.
- Other players' committed amounts, timestamps, rankings and accepted effects
  remain exact. Privacy transformations identify changed representations explicitly.
- The deleted account never returns; no cash refund or subscription cancellation
  is implied. Finite security evidence intentionally limits later linkage.
- New migrations may add narrow retained-data exceptions only after their typed
  operation, proof and reader changes pass review. Historical migrations 1–31
  remain byte-identical. No generic `DELETE`, `TRUNCATE`, trigger disabling or
  configurable arbitrary SQL is exposed to runtime or executor callers.
- Active removals, evidence expiry, scoped holds, processor acknowledgements and
  backup expiry are distinct progress facts. A stalled task reports the actual
  pending obligation; elapsed time never manufactures successful cleanup.

## Considered options

- **Typed executor and explicit proof transition (proposed)**: supplies a narrow
  erasure authority while retaining the identity of surviving accepted effects.
- **Soft delete only**: rejected; retained credentials, source copies, public
  attribution and raw provider evidence would remain personal data indefinitely.
- **Hard delete with cascades**: rejected; shared votes, sponsorship, sanctions,
  billing chains and multi-player outcomes have different owners and obligations.
- **Rewrite JSON under its old digest**: rejected; the digest would no longer
  authenticate that representation and legitimate original retries would fail.
- **General SQL job engine or runtime-set bypass GUC**: rejected; unnecessary
  machinery and a broad, forgeable path around immutable-value protections.

## 1. Minimal control and authority model

### 1.1 Proposed objects and stages

Migration 32 is reserved for the D1 foundation. Do not pre-authorize a later
transformation merely because its storage exists. D1 releases only requests, step receipts and fences plus the three typed mutators below. Evidence, holds and row authorizations are D3/D4 additions, listed here to pin their eventual contract; D1 does not create unused generic authority tables. Proposed objects are:

| Object | Exact responsibility and proposed fields |
|---|---|
| `public.privacy_requests` | `id UUID PK`, `account_id UUID UNIQUE`, `policy_version`, `manifest_sha256`, `proof_sha256 BYTEA(32)`, `status_token_sha256 BYTEA(32) UNIQUE`, `verified_at`, `active_due_at`, `evidence_until`, `phase`, `suppression_sequence`, `suppression_sha256`, `active_removed_at`. Account linkage is private and removed at expiry; phase is a fixed enum, never caller SQL. |
| `public.privacy_step_receipts` | `(request_id, step, object_key) PK`, `original_sha256`, `replacement_sha256`, `result_code`, `completed_at`. Step/result are fixed enums. Object keys are typed tuples serialized canonically, not predicates. No original field values, credentials, free text or blobs in receipts. |
| `public.privacy_retained_evidence` | `(request_id, class, source_key) PK`, `schema_version`, `expires_at`, `payload`. Each class has an exact typed field allowlist in §3; JSON is validated against it, never a copy of a source row. This table is private, including to ordinary Admin. No new generic transformation language. |
| `public.privacy_holds` | `id`, `request_id`, exact evidence class/source keys, `basis_code`, `basis_reference`, `owner_id`, `expires_at`, `review_at`, `created_at`. No whole-account wildcard or automatic renewal. Separate reviewed retention authority creates/ends holds; runtime and ordinary Admin cannot. |
| `public.privacy_row_authorizations` | Private transaction-local grants persisted only within the executing transaction: request/step, relation OID, exact PK, operation, original/replacement hashes, transaction/backend identity. Not an externally callable API or permanent substitute for step receipts. |
| `public.account_deletion_fences` | `account_id UUID PK`, `request_id UUID UNIQUE`, `fenced_at`. Runtime has SELECT only. Existing admission/read paths check this durable fence; no status/contact/evidence fields are public. The fence is removed only after final account removal and authoritative restore coverage. |

Exact D1 source grants to the NOLOGIN owner are SELECT(id,deleted_at) and
UPDATE(id) on accounts; SELECT,DELETE and UPDATE(account_id) on profiles;
SELECT(account_id) on text_admissions and noin_ledger. Column UPDATE grants
permit PostgreSQL row locking; no released function changes either key.
Only the offline migrator can SET ROLE to this owner.

One bounded function invocation locks one existing request and executes one
hard-coded step against exact owned/dependent rows. Keyset cursors and limits
reuse existing batch conventions. No full-table locks during ordinary cleanup.
Unknown table/column/JSON variant, incomplete survivor settlement, stale original
digest, missing independent suppression receipt, or expired authority refuses
the affected step with no partial receipt.

D1 can create/test its three released relations and a bounded **profile-removal fixture** only:
`profiles.account_id=request.account_id`, all profile columns removed, same-tx
step receipt. Its current request/fence records do not revoke sessions or set
`deleted_at`; tests set that precondition explicitly. It refuses any admission
or Noin ledger evidence until D3 supplies the required survivor witness path.
The released manifest digest covers the exact canonical JSON profile column
names/types/nullability and reviewed constraint/dependency contract. The typed
executor validates that shape, ordinary non-inherited tables, no RLS on profiles
or either accepted-effect source, and no new profile triggers/rewrite rules/incoming foreign
keys before removal. A new column or dependent relation refuses the step; this
is a profile-only executable manifest, not a claim that D1 executes all87
inventory dispositions.
It cannot erase protected value/source rows, create a usable
browser deletion endpoint, or advertise complete active removal. D3/D4 add each
protected operation only alongside its proof-preserving reader changes. This
keeps the first boundary concrete rather than implementing an unused framework.

### 1.2 Exact role and function contract

Deployment supplies role names through a `PrivacyRoleSpec`; migrations do not
create LOGIN roles or embed passwords. Proposed roles:

| Principal | Allowed rights | Explicitly forbidden |
|---|---|---|
| Existing runtime | SELECT `account_deletion_fences`; EXECUTE public status function below; existing non-privacy rights remain subject to the fence | Private-table SELECT/DML; mutator EXECUTE; membership/SET ROLE to privacy principals; CREATE/ALTER/TRUNCATE; writing authorization rows |
| Privacy function owner | NOLOGIN, NOINHERIT, no membership granted to runtime; owns only reviewed privacy functions and private control relations; exact source-column grants needed by each released step | Superuser, BYPASSRLS, CREATEROLE, owning/changing source tables, arbitrary migration or deployment privileges |
| Privacy executor | Separate LOGIN credential and pool/worker, no role membership; USAGE public schema and EXECUTE the exact released mutators | Direct private/source DML, SELECT evidence payloads, source ownership, dynamic SQL/table arguments |
| Capture | Explicit SELECT on private relations only for authorized backup, with their original expiry metadata | Mutator/status EXECUTE, authorization writes, retention extension |
| Existing control/migrator | Existing offline provisioning/migration trust; audited setup only. Control SELECT on private tables for physical fingerprint/parity, no online evidence use. Migrator SET-only membership to privacy owner, never INHERIT/ADMIN; control ADMIN-only membership to executor for LOGIN fencing, never SET/INHERIT | Being the routine online privacy executor |

D1 public signature: `public.account_deletion_status(status_token_sha256 bytea)
RETURNS jsonb`, returning only phase/deadlines/outstanding classes. The capability
is random 256-bit material; only its digest is stored. It grants no login,
account lookup, cancellation or access to evidence.

Proposed released executor signatures:

- `public.privacy_prepare_verified_request(request_id uuid, account_id uuid,
  proof_sha256 bytea, status_token_sha256 bytea) RETURNS uuid`.
- `public.privacy_bind_suppression(request_id uuid, sequence bigint,
  receipt_sha256 bytea) RETURNS void`.
- `public.privacy_erase_profile_batch(request_id uuid, limit_rows integer) RETURNS jsonb`
  as the first narrow D1 fixture operation.

Policy version, manifest, server timestamps/deadlines and allowed step are
selected inside the implementation, not supplied by callers. Preparation is a
privileged service call, **not** PostgreSQL verification of user identity: the
D2 adapter must first verify the complete same-account proof and is the only
online component handed this separate connection. Ordinary runtime cannot
insert an apparently verified job. D1 tests use explicit synthetic privileged
preparation; they are not reauthentication evidence.

Functions are SECURITY DEFINER only where necessary, `search_path=pg_catalog`
with schema-qualified object names, and PUBLIC EXECUTE revoked before any grant.
A trigger exception requires an exact row authorization created by the definer
inside a locked validated request/phase transaction; its owner and transaction
identity cannot be forged by runtime, a GUC, a temp relation or a supplied JSON
payload. No session-wide bypass survives rollback or connection reuse. The
executor cannot create authorization rows directly.

`cutover_roles.go` currently rejects arbitrary SECURITY DEFINER functions and
requires exact ownership/membership inventories. The coordinator must extend
that verifier and its real provisioning counterpart with exact public-object, owner,
signature, fixed-search-path, function-body/version and ACL exceptions; never
relax the global rule. D1 is not complete until runtime/capture/control negative
role tests prove this contract on PostgreSQL, including role inheritance and
`SET ROLE`. Private tables must appear explicitly in snapshot/restore inventory. Use the
existing `public` schema with `privacy_` prefixes; namespace separation adds no
ACL protection and would leave state behind existing guarded fixture resets.
Default grants must explicitly exclude these objects before runtime/capture
allowlists are applied.

The executor is a physical writer. Cutover closing first stops accepting privacy
work and new journal appends, then drains every in-flight typed transaction and
suppression append/bind handshake. Its LOGIN is fenced and existing sessions
terminated alongside runtime/migrator before the capture boundary. A worker may
not continue source cleanup or append an external suppression acknowledgement
after its generation is fenced. Source/destination activation verifies the exact
executor role, no owner membership, closed old generation, no pending unbound
append and fresh independent suppression head. Failed/unknown drain keeps
capture/admission closed; SQL fencing alone does not quiesce the external store.

## 2. User authority, confirmation and immediate fence

The future API uses one-use, short-lived intents bound to account, authentication
epoch, challenge, confirmation request identity and deletion purpose. Linked
accounts perform provider reauthentication without linking/restoring a different
account or minting general access. Guests prove possession of a separately enrolled, random deletion-purpose
capability against a fresh one-use server challenge. Gameplay sanctions and
refresh-token rotation do not revoke that capability; explicit security
revocation does. Enrollment requires valid same-account credentials, stores only
a digest, and places the secret in protected client storage. It grants no general
access or account restoration. A public/replayed `device_hash`, expired gameplay
token or gameplay-epoch mismatch alone never supplies this authority. Legacy
guest recovery requires a separately reviewed verifiable same-account path;
D2/D6 remain closed until sanctioned and expired-credential legacy fixtures pass.
No claim is made that such authority exists in schema31. Current source has no
separate legacy recovery secret. For a legacy guest lacking valid credentials or
enrolled deletion authority, return `recovery_required`, never successful
verification. The owner has attested no deployment, not an inventory of real
legacy users. Do not claim D2/D6 covers a nonexistent recovery mechanism: an
applicable deployment needs a supported evidence-based recovery/migration path
before enabling deletion. This limitation does not block D1's closed storage and
privilege tests. Guest web deletion uses the
existing app-to-browser confirmation mechanism; linked web deletion may use the
same provider reauthentication. No support-only route or gameplay-sanction gate.

The dedicated authorizer checks signature/capability proof, deletion-purpose
expiry and security revocation, exact account and the purpose-specific authority
epoch, while deliberately ignoring gameplay suspension/ban for this action.
Ordinary gameplay epoch is not reused as deletion authority.
A deleted/pending account may replay its existing deletion request/status but
cannot recover general credentials. Missing or revoked identity proof cannot
choose an arbitrary account. Original provider/account binding still applies.

The D2 privileged preparation transaction consumes the intent, records its proof
digest/request, increments epoch, revokes player/admin/portal sessions, sets the
fence and `deleted_at`, clears public profile/attribution visibility and records
a durable live-disconnect/publication-suppression intent. It commits before
calling the lobby; no SQL account lock is held while taking lobby locks.

Independent durability is an explicit two-step contract: a prepared request
already fences the live database, then the publisher durably appends its
suppression record and binds the external receipt. Only then does the service
acknowledge **confirmed deletion**. Crash after external append but before DB
acknowledgement retries the same request/receipt. Failure before export reports
preparing/retryable status, never a false confirmed deletion; local access stays
revoked. Restore can apply the external receipt even if the request DB commit
was lost. Verified-at is fixed on the initial verified preparation, so retries
cannot extend 30/180-day limits.

Status survives access revocation using its independent read-only capability.
Explicit confirmation explains loss of balance/progression and offers platform
subscription-management controls without delaying deletion. Processor or backup
obligations remain visible after active removal. No second account can acquire
an old purchase: provider proof already requires `VerifiedPurchase.AccountID`
equal to the requesting account (`purchases_verified.go:149`); missing legacy
ownership after expiry must never become permission to transfer it.

## 3. Source inventory and disposition contract

Inventory below is the actual column set produced by applying migrations 1–31
to isolated PostgreSQL 16 on 2026-09-19: **87 public tables**. Evidence:
`/tmp/agent-runs/deletion-schema31--20260919T112012Z-840398.log`.
The temporary evidence path is local validation, not a deployment dependency.
D1 tests pin a compiled exact inventory and refuse unreviewed schema drift.

Disposition codes:

- **E**: remove every listed column of the selected row by active-data completion
  (30-day maximum). Brief processing storage is not a retention exception.
- **R**: copy only the following typed minimum to private evidence, then remove
  the original selected row/fields by active completion. Purge evidence at
  verified deletion +180 days unless an exact scoped hold applies.
- **X**: shared record; remove this subject's fields/associations after the
  required survivor work, preserving other subjects' exact independently owed
  facts. Hash-bearing representations use §4, never in-place fake validation.
- **K**: retain unrelated operational/policy data unchanged. A copied personal
  field discovered by the marker tests disqualifies K and must be inventoried.

R evidence allowlists (everything else is E; opaque source keys remain private):
financial = event/product/platform/application/environment, amount/quantity,
server day and transaction/verification/refund times/state, keyed provider
transaction/source identity, source receipt digest; consent = version and
acceptance time only when necessary for an explicitly identified retained
financial/reconciliation or substantiated security case/effect (otherwise E);
settlement = match/event/effect key, accepted source digest,
requested/credited amount and occurrence time, final disposition; substantiated
security = decision/action code, imposed/expiry/lift times and finite keyed
installation digest. No raw receipts, email, credentials, free-text reasons,
original UGC/blobs, profile counters or ordinary chat are in these allowlists.

Selections include ownership **and references** to the subject: admin actor IDs
are resolved from `admin_accounts.account_id`; contribution IDs through original
submissions/entries; billing sources through `billing_account_sources` and
`billing_subscription_sources`; operation IDs through decisions. Shared rows are
not deleted simply because they mention the subject. FK cycles are resolved by
explicit reviewed null/erased-reference representations or companion receipts;
never by disabling constraints or cascading through surviving subjects.

| Table | Exact schema-31 columns | Disposition and selection |
|---|---|---|
| `account_sanction_deliveries` | `operation_id`, `outcome`, `delivered_at` | R security: linked selected sanction; preserve only outcome/time. |
| `account_sanction_installations` | `operation_id`, `device_hash` | R security: derive purpose-keyed digest before removing raw device_hash; expires at min(timed sanction end, deletion+180d). Shared survivor auth bindings are untouched. |
| `account_sanction_lifts` | `operation_id`, `sanction_id`, `created_at` | R security: linked selected sanction/lift; preserve exact effect, minimize actor link. |
| `account_sanctions` | `operation_id`, `account_id`, `until_at`, `created_at` | R security: subject target; only substantiated outcome/time, no automatic indefinite hold. |
| `accounts` | `id`, `nickname`, `avatar`, `locale`, `created_at`, `updated_at`, `deleted_at`, `banned_at`, `auth_purpose`, `suspended_until`, `session_epoch`, `avatar_revision` | X: `id=subject`; fence now, clear nickname/avatar/locale by active completion. Keep only private minimal subject linkage through evidence expiry, then remove original account and detach shared FKs; IDs are never reused. |
| `admin_accounts` | `id`, `account_id`, `email`, `password_hash`, `totp_secret`, `backup_codes`, `role`, `created_at`, `updated_at` | X: subject account; erase email/password/TOTP/backup_codes immediately, remove role access. Resolve actor references to survivor facts before removing the original admin row. |
| `admin_audit_log` | `id`, `admin_id`, `action`, `target_type`, `target_id`, `before_state`, `after_state`, `created_at` | R/X: affected-subject/actor/target entries; strip subject identifiers and free text in before/after JSON. Preserve survivor operation facts with explicit transformed provenance. |
| `admin_operation_decisions` | `id`, `actor_admin_id`, `kind`, `target_account_id`, `room_id`, `owner_id`, `owner_generation`, `source_ledger_id`, `amount`, `prior_sanction_id`, `sanction_until`, `reason`, `affected_accounts`, `request_hash`, `created_at` | R/X: target, actor or affected_accounts includes subject; finite necessary operation facts only, reason erased; survivor recipients/effects preserved, request_hash remains original anchor. |
| `admin_operation_results` | `operation_id`, `outcome`, `result`, `completed_at` | R/X: parent decision selected; exact survivor delivery/effect facts retained; subject payload/linkage minimized. |
| `admin_sessions` | `id`, `admin_id`, `csrf_token`, `expires_at`, `last_activity` | E: admin belongs to subject; revoke at confirmation. |
| `audit_events` | `id`, `occurred_at`, `account_id`, `event_type`, `payload`, `room_id`, `match_id` | E/X: subject-origin payload/identifiers erased; only exact financial/security evidence allowlist may be copied; unrelated actor facts unchanged. |
| `auth_installation_bootstrap` | `device_hash`, `account_id` | E/X: delete subject mapping; preserve surviving bindings on same installation; re-registration receives a new account only. |
| `auth_installation_rotations` | `old_refresh_id`, `account_id`, `device_hash`, `session_epoch`, `issued_at`, `access_id`, `refresh_id`, `issuance_config_hash` | E: subject account, including all replay token identities/config hash; preserve survivor rotations. |
| `auth_installations` | `device_hash` | X: delete registry only when no surviving account/bootstrap/rotation needs it; raw deleted-sanction capture is handled separately. |
| `auth_revocations` | `token_id`, `revoked_at`, `expires_at` | E: revocation IDs derived from subject credentials after those credentials can no longer authenticate; preserve unrelated tokens. |
| `billing_account_sources` | `platform`, `original_key`, `account_id` | R financial: subject provider ownership keys, purpose-limited; no restore into new account. |
| `billing_legacy_premium` | `account_id`, `entitlement_type`, `active_until` | R financial: subject entitlement/expiry provenance only, no active entitlement. |
| `billing_provider_tasks` | `purchase_id`, `request`, `proof`, `state`, `attempts`, `updated_at` | R/E: resolve/cancel task for subject purchase, retain only final reconciliation outcome; request/proof raw payloads erased. |
| `billing_subscription_current` | `platform`, `source_key`, `observation_id`, `verified_at`, `checked_at` | E: subject-source projection; terminal worker disposition prevents recreation. |
| `billing_subscription_imports` | `purchase_id`, `platform`, `source_key`, `row_sha256` | R financial: subject-source import identity/hash only until expiry. |
| `billing_subscription_observations` | `id`, `platform`, `source_key`, `evidence_sha256`, `provenance`, `state`, `product_id`, `product_kind`, `transaction_key`, `purchased_at`, `observed_at`, `transaction_signed_at`, `renewal_signed_at`, `monthly_until`, `yearly_until`, `evidence` | R financial: observations joined to subject source; evidence JSON erased, digest is labelled source anchor. |
| `billing_subscription_replacements` | `platform`, `predecessor_key`, `successor_key`, `account_id`, `application`, `environment`, `created_at` | R/X: subject-owned source links; preserve other independently owned verified source chains; no ownership reassignment. |
| `billing_subscription_sources` | `platform`, `source_key`, `account_id`, `application`, `environment`, `initial_purchase_id`, `registered_at` | R financial: subject source binding and product environment metadata; raw ownership keys minimized. |
| `billing_subscription_tasks` | `platform`, `source_key`, `request`, `proof`, `state`, `attempts`, `updated_at` | R/E: subject-source task; finish external acknowledgement where required, then erase request/proof. |
| `billing_transactions` | `purchase_id`, `platform`, `provider_key`, `original_key`, `account_id`, `application`, `environment`, `product_id`, `product_kind`, `quantity`, `noin_amount`, `state`, `purchased_at`, `observed_at`, `provider_signed_at`, `checked_at`, `expires_at`, `revoked_at`, `granted_at`, `refund_state` | R financial: subject rows; minimize provider keys, preserve exact amounts and states, reject all delayed spendable grants. |
| `challenge_current_winner` | `singleton`, `topic_id`, `entry_id`, `account_id`, `week_start`, `crowned_at` | X: remove subject public owner/title using an empty current-winner state; do not transfer title or repeat payout. |
| `challenge_entries` | `id`, `account_id`, `topic_id`, `entry_type`, `content`, `asset_ref`, `asset_blob`, `terms_version`, `terms_accepted_at`, `status`, `screen_decided_at`, `screen_decided_by`, `rejection_reason`, `vote_count`, `slot_number`, `created_at`, `updated_at` | E/X: subject-authored content/blob and public stats removed; linked votes/topics/winner provenance handled explicitly; survivor entries unchanged except erased reviewer reference. |
| `challenge_topics` | `id`, `week_start`, `week_end`, `nown_media_id`, `published_at`, `closed_at`, `winner_entry_id`, `created_at`, `updated_at`, `activated_at`, `source_revision` | X: withdrawn authored Nown/entry reference becomes erased-source placeholder; immutable week/closure and survivor payout facts retained; no future intake on unavailable source. |
| `challenge_votes` | `id`, `account_id`, `topic_id`, `entry_id`, `cast_at` | E/X: subject voter association removed; votes on removed entry lose target-content linkage through explicit erased-entry representation. Do not alter a surviving entry vote total/reward silently. |
| `challenge_winners` | `id`, `topic_id`, `entry_id`, `account_id`, `title_granted_at`, `noin_payout_granted_at`, `created_at`, `payout_amount` | R/X: deleted winner public title/stats suppressed; exact historic payout identity minimized privately; never recrown or re-award a different player. |
| `custom_avatars` | `account_id`, `blob`, `content_type`, `moderated`, `created_at`, `updated_at`, `revision` | E: subject blob and metadata; revoke current revision and cached/in-flight access immediately. |
| `cutover_handoffs` | `id`, `instance_id`, `generation`, `request_id`, `watermark_id`, `target_instance_id`, `target_cluster_system_identifier`, `target_database_oid`, `target_database_name`, `created_at` | K: operational instance/watermark facts, no player identity. |
| `cutover_instances` | `id`, `cluster_system_identifier`, `database_oid`, `database_name`, `parent_instance_id`, `parent_watermark_id`, `phase`, `generation`, `current_request`, `created_at`, `changed_at` | K: operational database instance identity; do not drop restore gate. |
| `cutover_requests` | `id`, `instance_id`, `generation`, `predecessor_id`, `writer_roles`, `schema_sha256`, `image_sha256`, `config_sha256`, `content_sha256`, `lease_sha256`, `issued_at`, `expires_at` | K: reviewed schema/config/image/content digests and writer roles; never put credentials or player rows here. |
| `cutover_watermarks` | `id`, `instance_id`, `generation`, `request_id`, `wal_lsn`, `evidence`, `evidence_sha256`, `created_at` | K/X: aggregate metadata/digests only; explicit evidence JSON allowlist must prove no player rows/identifiers before K. |
| `daily_noin_earned` | `account_id`, `server_day`, `earned`, `updated_at` | E: subject counters after accepted-work disposition; never reset another account cap. |
| `daily_quickplay_counts` | `account_id`, `server_day`, `count` | E: subject counters after reserved/begun admissions are terminal. |
| `device_tokens` | `id`, `account_id`, `device_hash`, `created_at`, `last_seen_at` | E: subject association only; preserve a surviving account association on the same installation. |
| `entitlements` | `account_id`, `entitlement_type`, `value`, `active_until`, `created_at`, `updated_at` | E: subject unlocks/expiry/value; no transfer to replacement account. |
| `feedback` | `id`, `account_id`, `type`, `title`, `message`, `context_snapshot`, `status`, `created_at`, `updated_at` | E: subject account plus title/message/context_snapshot. |
| `guard_freezes` | `id`, `account_id`, `frozen_by`, `reason`, `frozen_at`, `expires_at`, `dismissed_at`, `dismissed_by`, `converted_to_ban_at`, `converted_to_ban_by`, `created_at`, `updated_at`, `guard_account_id`, `expired_at`, `decision_kind`, `decision_reason`, `decision_until`, `disconnect_delivered_at`, `disconnect_obsolete_at` | R/X: substantiated subject-target cases only; free text/reasons E. Subject acting as Guard becomes erased actor on survivor case; overlapping survivor freezes keep exact scope. |
| `leaderboard_admin_decisions` | `id`, `actor_admin_id`, `kind`, `week_id`, `target_account_id`, `revision`, `prior_decision_id`, `reason`, `request_hash`, `accepted_closing_at`, `created_at` | R/X: subject target/actor or prior chain; reason E, necessary security facts private; survivor eligibility chain/order remains valid through explicit erased actor reference. |
| `leaderboard_admin_results` | `operation_id`, `outcome`, `result`, `completed_at` | R/X: selected decision result; closed week result metadata survives without subject detail. |
| `leaderboard_daily_counts` | `account_id`, `server_day`, `count` | E: subject counters; no quota reclamation for others. |
| `leaderboard_entries` | `week_id`, `account_id`, `points`, `matches_counted`, `updated_at` | E: subject public points/matches removed after accepted-work disposition; survivor totals unchanged. |
| `leaderboard_history` | `week_id`, `account_id`, `rank`, `points` | E/X: subject row removed through authorized privacy transform; survivors keep original ranks/points, not retroactive reranking or reward adjustment. |
| `leaderboard_weeks` | `week_id`, `start_at`, `end_at`, `closed_at`, `closed`, `closing_at` | K: week identity/cutoff/closed state unchanged; no reopening for deletion. |
| `named_entitlement_items` | `account_id`, `entitlement_type`, `value`, `acquired_at`, `source_id` | R financial: only acquisition/source identity needed reconciliation; erase subject benefit projection. |
| `noin_ledger` | `id`, `account_id`, `event_type`, `amount`, `reason`, `payload`, `server_day`, `created_at` | R financial: subject rows; strip reason and untyped payload; retain surviving ledger rows verbatim. Resolve award/refund FKs first. |
| `noin_wallets` | `account_id`, `balance`, `updated_at` | E: subject balance inaccessible immediately; no spendable value recreated. |
| `oauth_flows` | `id`, `provider`, `intent`, `state_hash`, `completion_hash`, `nonce_hash`, `code_verifier`, `requester_hash`, `account_id`, `initiating_token_id`, `session_epoch`, `status`, `error_code`, `created_at`, `expires_at`, `issued_at`, `issuance_config_hash`, `access_id`, `refresh_id`, `device_hash` | E: subject account/initiating credentials; clear all proof/PKCE/issuance fields, no token-mint replay. |
| `oauth_links` | `id`, `account_id`, `provider`, `provider_subject`, `provider_email`, `created_at` | E: subject account; includes provider subject/email. |
| `player_blocks` | `actor_id`, `target_id`, `created_at` | E: either actor or target is subject; no private relationship retained publicly. |
| `portal_browser_sessions` | `token_hash`, `account_id`, `csrf_token`, `created_at`, `expires_at`, `device_hash` | E: subject account and exact associated installation/browser tokens. |
| `portal_login_limits` | `principal_hash`, `attempts`, `expires_at` | E: only proven subject-owned principals, otherwise expire existing short-lived bucket; never reverse/infer a hash to sweep unrelated users. |
| `portal_login_requests` | `browser_hash`, `pairing_code`, `csrf_token`, `account_id`, `created_at`, `expires_at`, `device_hash` | E: subject-bound pairing requests and credentials; never erase unrelated shared-device request. |
| `portal_role_applications` | `id`, `account_id`, `role`, `status`, `applied_at`, `decided_at`, `decided_by`, `reason`, `created_at`, `updated_at` | E/X: subject applications/reasons removed; survivor application decision remains with erased actor reference. |
| `portal_roles` | `account_id`, `role`, `granted_by`, `granted_at`, `revoked_at`, `revoked_by`, `created_at`, `updated_at` | E/X: subject roles removed; subject as grant/revoke actor on survivor role becomes a minimized erased-actor reference. |
| `portal_submission_counts` | `account_id`, `server_day`, `count`, `updated_at` | E: subject counters. |
| `portal_submissions` | `id`, `account_id`, `media_type`, `content`, `asset_ref`, `asset_blob`, `status`, `tags`, `tone_bucket`, `nown_id`, `pack_tag`, `terms_version`, `terms_accepted_at`, `submitted_at`, `decided_at`, `decided_by`, `rejection_reason`, `created_at`, `updated_at` | E/X: subject-authored content/tags/blob/references/rejection text removed after release withdrawal; consent version/time E unless necessary for an exact retained financial/reconciliation/security case, then typed R for that case only. Survivor contribution with subject reviewer retains approved text and erased reviewer reference. |
| `portal_terms` | `version`, `title`, `body`, `active_from`, `created_at` | K: published policy wording; no account-specific acceptance lives here. |
| `profiles` | `account_id`, `level`, `xp`, `overall_points`, `non_converted_points`, `matches_played`, `matches_won_nower`, `matches_won_donower`, `correct_votes`, `votes_cast`, `donower_survivals`, `donower_matches`, `pokes_sent`, `week_winner_titles`, `weekly_podiums`, `contributor_credits`, `updated_at` | E: `account_id=subject`, including all counters and `contributor_credits`; public suppression is immediate. |
| `queue_cooldowns` | `account_id`, `abandon_count`, `cooldown_until`, `updated_at` | E: subject counters; finite substantiated sanction evidence is separate. |
| `report_cases` | `id`, `case_key`, `kind`, `target_account_id`, `target_media_id`, `text_target`, `status`, `resolution`, `resolution_reason`, `resolved_by`, `resolved_at`, `created_at`, `updated_at` | R/X: substantiated selected cases only; resolution_reason/text_target personal bytes E; surviving subject decision remains exact. |
| `report_rate_limits` | `bucket`, `last_at` | E: proven subject bucket only; otherwise bounded original expiry, no unrelated principal sweep. |
| `reports` | `id`, `report_type`, `reporter_id`, `target_account_id`, `target_media_id`, `reason`, `description`, `status`, `created_at`, `updated_at`, `case_id`, `text_target`, `submission_sha256` | E/R/X: subject reporter/target or withdrawn authored target; ordinary descriptions E; substantiated-security fields only R. Surviving reporter facts are minimized independently, not blanket-cascaded. |
| `schema_migrations` | `version`, `dirty` | K: schema version/dirty flag. |
| `store_purchases` | `id`, `account_id`, `platform`, `product_id`, `transaction_id`, `amount`, `verified_at`, `refunded_at`, `raw_receipt`, `created_at` | R financial: subject purchase ownership/time/state; raw_receipt is E, transaction identifiers keyed privately. |
| `system_notices` | `id`, `type`, `title`, `body`, `published_at`, `withdrawn_at`, `maintenance_start`, `maintenance_duration_min`, `created_by`, `created_at`, `updated_at` | X/K: operational localized text remains unless it contains identified subject-owned content; erase creator linkage when creator is subject, without cancelling unrelated maintenance. |
| `text_abandons` | `match_id`, `account_id`, `seat`, `occurred_at`, `body_hash`, `prior_count`, `applied_count`, `duration_seconds`, `cooldown_until` | R settlement: subject event/body digest and required accepted-work disposition only, no public cooldown/statistics. |
| `text_accepted_inputs` | `id`, `source_kind`, `source_id`, `text_content`, `provenance`, `submission_id`, `entry_id` | E/X: subject-authored text/provenance erased after consumers withdrawn; survivor source stays exact except explicit erased reviewing-actor metadata if present. |
| `text_active_releases` | `language`, `rules_version`, `access_key`, `release_id`, `activated_at` | E/X: delete affected active mapping immediately; replacement uses separately certified immutable release ID. |
| `text_admissions` | `id`, `account_id`, `match_id`, `seat`, `entry_path`, `prototype`, `access_kind`, `quota_day`, `reserved_at`, `state`, `process_owner_id`, `process_generation` | R/X: subject admission final disposition only; other seats and Sponsor effects settled first; no active reservation retained. |
| `text_archive_progress` | `job_id`, `phase`, `upper_kind`, `upper_id`, `expected_count`, `expected_sha256`, `copy_kind`, `copy_id`, `copy_count`, `copy_sha256`, `verify_kind`, `verify_id`, `verify_count`, `verify_sha256` | X: retained original capture manifest remains historical anchor; new privacy overlay records removed source keys/new inventory digest; never mark original hash verified against edited rows. |
| `text_award_receipts` | `match_id`, `account_id`, `kind`, `ordinal`, `body_hash`, `occurred_at`, `server_day`, `requested`, `credited`, `ledger_id` | R/X: subject receipts minimized private; survivor receipts/ledger IDs/body_hash untouched. Preserve source anchor until authorized proof transition. |
| `text_bonus_eligibility` | `match_id`, `account_id`, `premium_bonus_eligible`, `started_at`, `policy_version` | R settlement: subject start-time eligibility/decision source only if needed to reconcile accepted work; no bonus/profile resurrection. |
| `text_content_revisions` | `language`, `content_id`, `revision`, `sha256` | X/K: withdrawn subject content revision becomes digest-only revoked identity, not reusable source; survivor revision identities untouched. |
| `text_first_win_claims` | `account_id`, `server_day`, `match_id` | R settlement: subject day/match identity only, then erase; survivor claims unchanged. |
| `text_legacy_archive` | `source_kind`, `source_id`, `source_row`, `source_sha256`, `content_id`, `archival_state`, `media_type`, `mode_id`, `content_language`, `content_revision`, `submission_id`, `entry_id` | E/X: subject-owned whole source_row/blob/attribution erased; other rows exact; source_sha256 becomes anchor of explicit transform, not current-row proof. |
| `text_matches` | `id`, `room_id`, `contract`, `contract_hash`, `owner_id`, `fence`, `state`, `prototype`, `created_at`, `started_at`, `ended_at`, `outcome`, `outcome_hash`, `process_generation` | X: SponsorAccountID and Players subject projection; all survivor settlements terminal first, then §4 witness/representation transition; other players outcomes/effects exact. |
| `text_outbox` | `id`, `match_id`, `account_id`, `effect_kind`, `payload`, `attempts`, `available_at`, `claimed_by`, `claim_until`, `acknowledged_at` | E/X: subject private delivery cancelled/erased, including payload and claims; survivor delivery payload/ack identity unchanged. |
| `text_process_current` | `singleton`, `incarnation_id`, `generation` | K: process fence, not account identity. |
| `text_process_owners` | `incarnation_id`, `generation`, `backend_pid`, `backend_started_at`, `acquired_at`, `lost_at`, `recovered_at` | K: process/backend ownership metadata, not account identity. |
| `text_releases` | `release_id`, `language`, `rules_version`, `manifest_sha256`, `snapshot_sha256`, `bundle`, `access_class`, `entitlement_key`, `published_by`, `published_at`, `withdrawn_at` | E/X: any bundle embedding withdrawn source bytes/attribution/artifacts removed after immediate withdrawal; preserve digest-only historical release identity for surviving match contracts, never recertify edited bytes under old ID. |
| `text_settlements` | `match_id`, `account_id`, `outcome_hash`, `state`, `applied_at`, `effects` | R/X: subject work gets explicit erased disposition; survivors settle original pinned outcome first and retain exact effects/hash; no sanitized outcome fed to ordinary settle. |
| `user_terms_acceptances` | `account_id`, `version`, `accepted_at` | E by default. Only version/time necessary for an exact retained financial/reconciliation or substantiated security case may be typed R, private180d then purge; no general consent-history exception. |
| `user_terms_versions` | `version`, `body`, `active_from`, `created_at` | K: published policy wording. |

### 3.1 Structured and unstructured copies

| Location | Exact paths/class | Required transformation |
|---|---|---|
| `text_matches.contract` (`TextMatchRecord`) | `SponsorAccountID`, `AdmissionIDs[]`; `Contract` contains typed match/pack identity and `Policy` pinned rules | Remove subject sponsorship/admission linkage after counterpart obligations are terminal. Preserve exact shared rule/pack identity in survivor witness. |
| `text_matches.outcome` (`TextOutcome`) | `Players[].AccountID`, `Seat`, `Role`, `Points`, `CorrectVotes`, `VotesCast`, `Survivals`, `Pokes`, `Absent`; top-level MatchID/Owner/Epoch/Kind/Winner/At | No public or indefinite deleted-player statistics; survivor projections retain exact own fields. Original replay hash remains a distinct anchor. |
| `text_settlements.effects`, `text_outbox.payload` | `match_id`, `interrupted`, `points`, `xp`, `leaderboard_counted`, `awards[].kind/ordinal/requested/credited` | Subject delivery/effects become private minimized disposition; survivor payload remains byte-equivalent. AccountID is relational routing, not a public recipient list. |
| `noin_ledger.payload`, audit payloads, admin before/after, operation results | Producer-specific typed variants, including match_id/account_id/source_ledger_id/operation_id and nested affected-account arrays | Decode each released producer schema; keep only class allowlist. Unknown variants refuse the step. Never recursive string replacement or unchecked JSON path search. |
| `admin_operation_decisions.affected_accounts` | Array of UUID strings | Remove deleted-subject linkage only through shared-operation representation/witness; preserve survivor operation scope and all their exact effects. |
| `text_legacy_archive.source_row` | Whole original SQL row, including account_id/content/asset_blob/reasons/reviewer fields | Erase owned snapshot bytes; do not assume archiving made them anonymous. Preserve only necessary source digest identity. |
| `text_accepted_inputs.provenance` | source_kind/source_id/source_revision/accepted_text_sha256/terms_version/consent_reference/consent_at_ms/approval_reference/editor_reference/reviewed_at_ms/license/attribution | Remove authored content/provenance copy; attribution can freeze an old nickname. Reviewer references on surviving contributions get an explicit erased-actor representation. |
| `text_releases.bundle` (`TextBundle`) | `nowns[]`, `cards[]`, `suitability[]`, `manifest`, `artifacts` byte map; each Nown/card provenance and artifact contents | Withdraw whole affected artifact; artifacts may embed accepted inputs/replay witnesses, so erase bundle/artifact bytes rather than only visible attribution. New certified replacement has new hashes/ID. |
| `profiles.contributor_credits`, tags, admin backup_codes | SQL arrays | Subject-owned profile and secrets removed entirely; credits copied into other subjects' records require provenance-directed removal of this subject's attribution. |
| raw_receipt, billing task request/proof, subscription evidence | Provider-specific raw JSON, identifiers and signed evidence | Finish required acknowledgement/cancellation with bounded task first; minimize to typed financial evidence, erase raw copies and free-form provider fields. |
| feedback context, reports text_target/descriptions, notice localized text | Authored/unstructured text and typed references | Erase owned content; preserve unrelated notices, with identified personal portions handled explicitly. No assertion that absence of an `account_id` key proves absence of PII. |

The executable manifest must enumerate every producer variant above before D6;
D0 does not invent unknown JSON schemas. For each variant the D1–D4 implementation
adds a typed decoder/allowlist and marked fixture. The gate fails closed on a new
variant until reviewed; a generic JSON scrubber is not an acceptable fallback.

Filesystem/object/processor inventory includes configured retained blobs, old
pack directories, candidate/exported source bundles, action-replay and certification
artifacts, archive exports, Redis account/profile/session keys, telemetry exports,
backups and configured content-screening processors. Use existing explicit path,
size/hash and owned-target checks from snapshot code. Shared content-addressed
bytes are removed only when every surviving reference has a valid independent
replacement, or the affected consumer is explicitly withdrawn. No broad path,
Redis-pattern or bucket deletion. Processor unavailability remains a pending
obligation; a test double is not provider deletion evidence.

## 4. Survivor effects and original hashes

First prevent new admission/publication and complete existing accepted work
under original pinned contract/policy/outcome. Do not build a second settlement
engine. Add a narrowly named accepted-work account lock that permits the fenced
subject solely to record abandonment/terminal/privacy dispositions; ordinary
`LockValueAccount` keeps refusing deleted accounts. Keep match/week → sorted
accounts → dependent effects ordering. No account-locked callback reenters lobby.

For the deleting participant, record an immutable per-event/per-settlement erasure
disposition referencing request, original accepted identity/hash and policy. It
suppresses future profile/wallet/entitlement/outbox creation, rather than claiming
unpaid value was paid. For surviving players use existing Award/Finish/SettlePending
and private delivery unchanged. Test interruption, sponsorship, low-population
endings, late provider/ad events and process recovery; RecoverPending must not
stop forever at a deleted recipient.

Only after all surviving settlements on a shared match are terminal may its
personal shared representation change. Proposed `privacy-transform-v1` receipt:

- original row key, contract/outcome source hashes, and original canonical row
  digest verified under lock before any erasure;
- survivor account/seat identities and their exact committed award keys, ledger
  IDs/amounts, settlement source hash, effects digest and delivery identity;
- typed sanitized representation version and new canonical digest;
- request/step identity, actor authority class, completion time and transform hash.

The same transaction compares every survivor witness before/after and refuses
any change. It records the receipt and removes the original personal bytes only
when the reader can distinguish a transformed terminal record. Do not feed the
sanitized body back to `Finish`/ordinary settlement expecting its old hash.
Original retry bytes are hashed against the retained original anchor and map to
already-terminal accepted effects; changed bytes still conflict. No new effects
are evaluated from a digest-only or sanitized record.

A flat SHA256 does not supply cryptographic membership proofs after its source
is erased. The transformation receipt attests a privileged, verified transition;
it must not be labelled an independently recomputable original-content proof.
Retain only the source anchors/witnesses necessary for surviving players' own
facts. Purge deleted-subject linkage, statistics and raw bytes by their deadlines,
including from transformation metadata; request-to-subject mapping expires too.
Reconcile reports original, privacy-transformed, pending or invalid provenance
explicitly; it never interprets a missing original body as automatically clean.

Closed leaderboard survivors keep their original rank/points and week cutoff.
Deleting a row does not rerank/reward other players or transfer a challenge title.
FK detachments and erased actor/source markers must be exact table-specific
migrations; foreign keys on surviving value are never globally disabled.

## 5. Source withdrawal and cache races

CaptureAccepted, Publish, Activate, catalog load, Prepare and Start must serialize
with the same source/release deletion fence. A cached verified bundle cannot
bypass a current withdrawal check. Confirmed deletion immediately prevents future
publication/admission involving the source and suppresses public attribution.
Already-started matches follow existing absence/end rules; do not replace cards
or scoring mid-match. After terminal survivor work, erase pinned/cache copies
within the active-data deadline.

A replacement uses existing accepted-input assembly and `textcert` certification.
If the remaining corpus cannot certify, leave that pack unavailable and expose
replacement-needed status. Do not invent new accepted text, retrofit certificates
or claim that a byte-edited release has its old identity. Duplicate shared blobs
and archive rows are handled by exact source references, not nickname matching.

## 6. Independent suppression, backup and expiry

Configuration must name an independent authoritative suppression store, its
verification key/key ID, a separately persisted monotonic minimum watermark,
allowed endpoint or dedicated path, and expected installation identity. None is
part of the application's restorable database/Redis/blob backup set. A local
fixture can use a separately owned fsynced directory with explicit test identity;
that proves mechanics, not production durability or rollback resistance.

Suppression records bind sequence, previous-record digest, deletion request ID,
purpose-keyed account selector, verified-at and policy version, with an
authenticated receipt. Appending is idempotent by request. Crash-safe local
writes use temp file, fsync, rename and directory fsync; production persistence
requires its actual deployed equivalent. Key rotation retains only keys needed
to match unexpired affected backups/evidence; keys are not copied into ordinary
restore artifacts. Record expiry cannot precede every covered backup's expiry. In particular, a
backup created immediately before evidence purge at day180 can retain evidence
until its own creation+90-day expiry. The independent suppression selector and
verification key must cover that exact last affected backup, not merely
deletion+90 days. Purging primary evidence stops later captures including it;
snapshot manifests record covered request selectors so metadata-only backups
cannot indefinitely extend retention. Expiry is computed from actual affected
backup inventory and successful purge acknowledgements, never a new arbitrary
retention period.

Restore admission stays closed until it verifies a fresh authoritative head,
rejects sequence rollback relative to the independent minimum, applies every
relevant post-snapshot suppression request, completes required privacy work and
records the resulting receipt/watermark. Missing, stale, unauthenticated or
unavailable authority refuses restore admission, including after app-DB rollback.
A snapshot's own copy of a suppression head cannot certify freshness.

Backups carry creation/expiry timestamps in pinned manifests and expire no later
than creation+90 days. Restore rejects expired copies. Bounded purge validates
owned inventory and exact pins before removal, then records acknowledgement;
a filesystem timestamp or missing path is not a fabricated provider receipt.
`infra/compose/snapshot.py` currently proves fixture capture/restore only; keep
production activation gated until deployment authority and storage are configured.

Private evidence expires at verified-at+180 days; sanction digest expiry is the
earlier of that date and its timed sanction expiry. A shared installation still
used by another account keeps that survivor's login mapping; only deleted raw
captures/linkage are removed. Its anonymous bootstrap state must explicitly mark
the deleted mapping as erased instead of falling back to a surviving account's
sole `device_tokens` row. Delete→anonymous-bootstrap must create no survivor
session and never revive the removed account. The minimal erased-bootstrap
selector is a purpose-keyed finite digest under the same retention limit; once
it expires, bootstrap may create a new anonymous account but may never select a
linked survivor implicitly. This requires removing the ambiguous fallback,
not retaining raw installation evidence indefinitely. Holds require concrete basis, exact object scope,
responsible owner, expiry and review. An expired/unreviewed hold never renews
itself. Ordinary chat/credentials/original authored bytes are outside the default
evidence exception and cannot silently enter it through generic audit JSON.

## 7. Ordered implementation, ownership and acceptance

No source implementation is claimed here. The coordinator owns roadmap,
report/changelog/tracking and combined staging. Every boundary runs independent
reviewer then verifier before the next one; source owners coordinate shared files.

| Boundary | Owner/files | Required proof before completion |
|---|---|---|
| D0 proposed design | Consent/privacy lane: this ADR only; reviewer validates inventory, shared-proof semantics and minimal scope | Real schema31 column inventory matches87tables; every class/structured copy has explicit disposition or closed unsupported-variant gate; no invented deploy evidence |
| D1 privacy foundation | Privacy lane: migration32 and new `store/account_deletion*` plus tests; coordinator owns cutover catalogs/provisioning/snapshot inventory | Exact role/definer/grant negatives; runtime cannot prepare request/execute erasure/forge authorization/SET ROLE; wrong job/account/phase/digest fails; bounded synthetic profile removal and receipt atomic; migration up/repeat/down parity, no live endpoint |
| D2 reauth and request/status | Auth owner after sanctions handoff: auth/handler, app account screen, web confirmation; privacy store handshake | Guest/linked/sanctioned flows, fresh proof, expiry/revocation, cross-account/epoch/replay race, app/web confirmation copy, external suppression acknowledgement before confirmed status, no account recreation |
| D3 survivor value and content | Store/economy/lobby owner plus content owner; narrow shared hooks | Live/restart/delete races across modes/sizes; survivor exact effects; erased recipient never recreated; old-hash replay and transformed reader proofs; capture/publish/activate/Prepare/Start/cache race, correct withdrawal and independently certified replacement |
| D4 complete active/retained executor | Privacy lane with explicit per-family reviews | Every selected relational/JSON/array/blob class, typed evidence validation, FK detach order, scoped holds/expiry, finite shared-installation HMAC, no retained forbidden bytes, failure at each write and resume |
| D5 independent restore/processors | Coordinator/ops owner | Fresh external watermark, pre-deletion backup restore suppression, rollback/missing authority fail closed,90-day expiry/purge, provider acknowledgements and honest unresolved status |
| D6 joined gate and enablement | Coordinator with reviewer then verifier | Identifiable synthetic markers in every applicable source/copy/processor, real in-app/web request through removal and restore, exact other-player rows/effects, delayed billing/SSV no resurrection, cold restart and uncertainty replay, unified tests/lints; only then enable runtime routes |

The fixture corpus must cover a deleting player, a surviving player sharing an
installation, a deleted contributor/reviewer/admin, multi-account source/billing
relationships, pending and applied match effects, referenced and unreferenced
consent, historical archive/release copies, and every expiry/hold boundary.
Seed unique recognizable strings in every erasure class, never real user data.
Compare surviving rows/bytes and cryptographic anchors before and after, and
search all configured active outputs for those markers. Passing a few clean SQL
queries does not establish that JSON, blobs, copied artifacts and backups are gone.
