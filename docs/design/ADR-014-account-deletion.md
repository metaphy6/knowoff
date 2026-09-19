# ADR-014: Account deletion with explicit privacy transformations

- **Status**: proposed
- **Date**: 2026-09-19
- **Deciders**: Knowoff engineering; independent architecture review pending
- **Policy**: [Blueprint Profile §1a](../../BLUEPRINT.md#1a-account-deletion-and-retention)
- **Implementation sequence**: [ROADMAP](../planning/ROADMAP.md), D0–D6

The sections below preserve successive design proposals, including prerequisites
that later landed. The [finalization boundary](#finalization-boundary--2026-09-19)
identifies the implemented migration 41 scope and the excluded future work;
the overall deletion design remains proposed and its public journey disabled.

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

## 8. Schema 36 inventory supplement and proposed D4A boundary

This supplement extends, rather than replaces, the schema31 table manifest.
Applying migrations1–36 to isolated PostgreSQL16 produced **95 public tables**;
all original ordered column lists remain unchanged except `text_settlements`.
Evidence: `/tmp/agent-runs/deletion-schema36-plan--20260919T131220Z-1325707.log`.
The following are the eight additional tables and the one changed column list.
This is an inventory/design update, not a claim that these rows are erased.

| Table | Exact schema36 columns | Disposition |
|---|---|---|
| `account_deletion_fences` | `account_id`, `request_id`, `fenced_at` | X: retain exact request/subject admission fence until final subject removal and independent restore coverage; then detach after all disposition references are transformed. Never remove to resume login. |
| `privacy_deletion_capabilities` | `account_id`, `capability_id`, `secret_sha256`, `security_epoch`, `security_revoked_at`, `updated_at` | E: all subject capability IDs/hashes/security metadata after confirmation replay has a separate immutable receipt; never retain authentication secrets as evidence. |
| `privacy_deletion_intents` | `id`, `kind`, `account_id`, `secret_sha256`, `created_at`, `expires_at`, `state`, `session_epoch`, `initiating_token_id`, `credential_until`, `installation`, `capability_id`, `capability_sha256`, `security_epoch`, `provider`, `state_sha256`, `code_verifier`, `nonce_hash`, `provider_subject_sha256`, `consumed_request`, `consumed_capability`, `consumed_at` | E: every subject-bound enrollment/capability/OAuth intent, including PKCE, installation and provider-subject hashes. Preserve only the exact confirmation replay digest described below. Unbound expired OAuth attempts use the existing bounded expiry cleaner; requester/device-only associations are outside the account-bound D4A completion claim and require exact D4B ownership rules, never a shared-hash sweep. |
| `privacy_requests` | `id`, `account_id`, `policy_version`, `manifest_sha256`, `proof_sha256`, `status_token_sha256`, `verified_at`, `active_due_at`, `evidence_until`, `phase`, `suppression_sequence`, `suppression_sha256`, `active_removed_at` | X: private status/deadline/suppression facts remain only through their actual obligations; subject linkage/proof digests expire after exact references and last affected backup coverage. This control row is not an indefinite personal-data exception. |
| `privacy_step_receipts` | `request_id`, `step`, `object_key`, `original_sha256`, `replacement_sha256`, `result_code`, `completed_at` | X: private typed step identity/digests/result/time; no source values. Purge or unlink with completed request obligations; never copy credentials into object_key. |
| `text_reward_claims` | `claim_hash`, `match_id`, `account_id`, `ad_unit`, `issued_at`, `expires_at` | E/R: subject match/account claim linkage and ad-unit metadata removed after accepted bonus disposition. Only necessary reconciliation claim digest and event/expiry times may enter private finite evidence; no reward eligibility restoration. |
| `text_reward_ssv_receipts` | `provider_transaction_id`, `fingerprint`, `claim_hash`, `occurred_at`, `received_at` | R/X: resolve through claim_hash to subject claim; retain necessary provider transaction identity only as a private purpose-keyed selector plus fingerprint/event/receipt time. Terminal late-callback ownership checks must exist before removing source bindings. |
| `text_value_erasure_dispositions` | `id`, `request_id`, `admission_id`, `account_id`, `operation`, `ordinal`, `body_sha256`, `contract_sha256`, `policy_sha256`, `outcome_sha256`, `occurred_at`, `recorded_at` | R/X: subject accepted-work refusal identity and pinned source digests; preserve exact survivor facts, then transform/detach admission, request and account references explicitly. Immutable originals are not deleted by D4A. |
| `text_settlements` | `match_id`, `account_id`, `outcome_hash`, `state`, `applied_at`, `effects`, `erasure_disposition_id`, `erased_at` | R/X: schema31 disposition remains; erased state has no applied_at/effects and references a matching erasure disposition. Preserve original outcome_hash as replay anchor; no proof rewriting in D4A. |

Migration36 also adds exact subject uniqueness to `privacy_requests(id,account_id)`
and `text_admissions(id,account_id)`, and binds each fence to its same-subject
request. Dispositions reference the same-subject fence and admission; settlements
reference matching terminal dispositions. These constraints prohibit deleting a
request, fence or admission out of dependency order. The new terminal `erased`
settlement state and immutable/source-validation triggers are preserved.
Migration35 replaces only confirmation behavior: first confirmation serializes
release publication, withdraws exact authored lineage, and removes active
pointers. It preserves source/bundle bytes and the first withdrawal timestamp.
Exact replay of a pre35 consumed request does not backfill withdrawal; current
source admission still refuses a deleted creator. Any deployed migration of such
requests requires an explicit backfill proof before claiming immediate suppression.

### 8.1 D4A scope and API proposal

The next bounded implementation is credential erasure only, after D3 source and
survivor gates. The coordinator has authorized D4A under the all-phase request, reserving
migration37 after the migration36 review and shared gate finish. Tests may be
prepared meanwhile; no cleanup source lands ahead of that dependency. Source
ownership is privacy migration/store and auth readers;
the coordinator owns cutover catalogs, source ACLs, provisioning and tracking.
A single new typed function,
`public.privacy_erase_credentials_batch(p_request uuid,p_limit integer) RETURNS jsonb`,
uses the existing PrivacyOwner/PrivacyExecutor roles. The executor receives only
EXECUTE; ordinary runtime/capture/control receive no mutator rights. There is no
new LOGIN role, caller-selected table/column, retention hold or arbitrary payload.
Fixed `search_path=pg_catalog`, qualified names and exact prosrc/ACL pins remain
mandatory. New source grants are only reviewed SELECT/key-row-lock/DELETE rights
for the listed credential tables, and named admin credential UPDATE columns.

Each call accepts limit1–128, validates the exact request/fence/deleted subject,
policy and bound independent suppression receipt, then locks account → request →
selected credential rows. It processes a fixed family order and deterministic
primary-key order, at most the supplied number of credential rows per call.
It returns this call's processed count and whether the credential postcondition
is complete. Row removal and its terminal receipt are transactional/idempotent;
no permanent progress table or cumulative-count contract is needed. Unknown
source shape, RLS, unexpected trigger/rule/dependency or unsupported authority
state fails closed before any erasure/receipt. A narrowly compiled credential
manifest is required; the D1 profile manifest does not certify these sources.

### 8.2 Preserve exact confirmation retries before deleting proof rows

Add a private nullable `privacy_requests.confirmation_sha256 BYTEA` with a
32-byte check and immutable-once-set protection, paired with a private
`confirmation_kind` constrained to the original capability/OAuth kind. Both
fields are NULL together for unconfirmed foundation fixtures and become
immutable together. Trusted SQL computes the digest using
a versioned, domain-separated JSON tuple with explicit JSON nulls:
`["privacy-confirmation-v1", policy, intent_id, account_id, intent_kind,
secret_sha256_hex, capability_id_or_null, capability_sha256_hex_or_null,
request_id, status_token_sha256_hex]`. Text encoding and field order are fixed;
NULL, empty and present values are distinct. Callers never provide this digest.
First confirmation writes it atomically with the existing request, consumes the
intent and retains the original `proof_sha256` unchanged.

Migration backfill must find exactly one consumed intent with the exact original
request/account/status proof, recompute the existing proof hash for equality,
then derive the new tuple. Missing, ambiguous or mismatching retained histories
refuse migration; no guessed proof is accepted. Synthetic requests created later
through the owner-only foundation API may have NULL confirmation digest, but the
credential executor refuses them until a legitimate confirmation exists.

After intent/capability removal, confirmation may locate only the supplied
request and expected subject, lock the same account/request, and recompute the
tuple against the immutable digest. The minimal receipt therefore also retains
the fixed confirmed intent kind, with no provider, installation or secret bytes.
Successful fallback returns only the original request/account/verified-at/policy
metadata. It creates no new capability or proof lease, changes no epochs, never
runs withdrawal again and never rewrites phase/deadlines. A wrong tuple is an
ordinary authority refusal. The independent status digest remains separate.
This fallback is private executor authority; possessing a request UUID alone is
insufficient. Receipt metadata expires with its actual request obligations.

### 8.3 Exact credential postconditions

| Family and selected rows | Required terminal state |
|---|---|
| `oauth_links.account_id=subject` | No provider subject/email/link row remains. |
| `oauth_flows.account_id=subject` | No state/completion/nonce/PKCE/requester/installation/issuance fields remain; callbacks and result replay cannot mint tokens. |
| `portal_browser_sessions.account_id=subject`; `portal_login_requests.account_id=subject` | No browser, CSRF or pairing credential remains. Confirmation already revokes these; cleanup proves absence after waits. |
| `admin_sessions.admin_id` belonging to subject | No session/CSRF/expiry row remains. |
| `admin_accounts.account_id=subject` | Keep actor ID/FKs for later shared-history transformation, but erase email/password/TOTP, empty backup codes, set explicit non-authorizing erased role and `credentials_erased_at`. Nullable credentials plus the erased-role shape constraint and every admin auth reader must land together. No fabricated email or usable placeholder secret. |
| `privacy_deletion_intents.account_id=subject`; `privacy_deletion_capabilities.account_id=subject` | No purpose capability, enrollment or OAuth proof remains after immutable confirmation receipt verification. |
| `auth_revocations` with exact token IDs from selected flows/rotations/intents | Remove only after those exact credentials cannot authenticate, and only when no surviving owner references the ID. Unknown revocations retain their ordinary expiry; never sweep by inferred identity. |

Exact revocation references must be collected while the original flow/intent
rows still exist. Process their eligible revocation rows before removing that
source row; if a batch ends first, keep the source for the next invocation. All
removed revocations and source rows count toward the same total limit, including
limit1. Never discard the only ownership witness then sweep token IDs later.

The final `credentials_removed` step receipt is written only after a fresh
same-transaction check of every selected family's terminal state. Receipt fields
contain only request/step identity, typed manifest digest, outcome and time.
It does not set `active_removed`, delete the profile, claim 30-day completion or
remove the account/fence. Local caches, provider copies and backups remain later
explicit obligations. A scoped evidence hold cannot preserve credentials.

The existing separate autocommit expired-intent cleaner may keep its intent-only
locks; it must never acquire an account after an intent. In-flight OAuth claim,
provider callback and result issuance retain account-first post-wait authority
checks. Erasure cannot broaden account recovery or accept an expired bearer.

### 8.4 Proofs and remaining dependencies

D4A RED/GREEN cases seed unique markers in every credential column, batch across
all families with more than one page, crash/retry after each write, and compare
all survivor rows byte-for-byte. Change each confirmation tuple field separately
(including NULL/present capability shape) after proof deletion and require refusal;
exact replay must preserve the original receipt, epochs, phase and deadlines.
Test valid backfill, missing/ambiguous/mismatching history refusal, immutable
receipt mutation, and empty-only down/up parity. Test both lock orders for an
in-flight provider callback and result issuance versus cleanup, plus revocation,
expired intent cleanup, malformed limit, wrong request/account/phase and missing
suppression. Actual executor roles must pass while runtime/capture direct DML,
EXECUTE, SET ROLE and wrong-body/ACL attempts fail. Admin login/password/TOTP/
backup/session flows must all refuse the erased actor, preserving survivor audit
FKs. Review then independent cold verification precede the next slice.

D4A deliberately leaves shared installation rows and raw rotation receipts to
D4B. Before removing a canonical bootstrap mapping, migrate each unambiguous
legacy installation's original binding and preserve/refuse ambiguous mappings;
then remove `bootstrapAccount`'s sole-device-token fallback. Deleting account A
must never make anonymous bootstrap choose surviving account B on the same
installation. Only then may exact subject bindings/rotations and unused registry
rows be removed, with finite purpose-keyed sanction/erased-bootstrap selectors
and the previously specified expiry. Survivor login bindings remain exact.

D4C handles billing source locks and delayed proof/provider task receipts before
raw ownership evidence disappears; unknown legacy ownership refuses reassignment.
D4D transforms shared outcome/content/archive/consent/actor representations only
after original survivor effects are terminal and independently witnessed. D4E
adds typed minimum evidence and exact scoped holds/purge, followed by final
account/request/fence detachment and independently fresh restore suppression.
The offline migrator's existing trusted role can manage holds; no new online
arbitrary evidence writer is implied. None of these later boundaries is completed
by credential cleanup, and deletion routes remain closed until D6.

## 9. Proposed D4B: installation identity and finite security evidence

### Schema38 supplement

Fresh isolated PostgreSQL16 applying migrations1–38 contains **97 public tables**
including `schema_migrations`. Evidence:
`/tmp/agent-runs/privacy39-plan-catalog--20260919T135502Z-1506467.log`.
Relative to §8's schema36 inventory, only the following two existing column lists
change and two tables are added; the other93 ordered lists remain exact.

| Table | Exact schema38 columns | Disposition |
|---|---|---|
| `privacy_requests` | `id`, `account_id`, `policy_version`, `manifest_sha256`, `proof_sha256`, `status_token_sha256`, `verified_at`, `active_due_at`, `evidence_until`, `phase`, `suppression_sequence`, `suppression_sha256`, `active_removed_at`, `confirmation_sha256`, `confirmation_kind` | X: preserve the exact versioned confirmation replay anchor through credential erasure; no new proof lease or credential authority. Subject/status linkage is still subject to final obligation and backup expiry. |
| `admin_accounts` | `id`, `account_id`, `email`, `password_hash`, `totp_secret`, `backup_codes`, `role`, `created_at`, `updated_at`, `credentials_erased_at` | X: migration37's email/password/TOTP are nullable only in the constrained erased state with empty backup codes and non-authorizing role. Retained actor ID/FKs require later shared-history minimization. |
| `text_bonus_payments` | `match_id`, `account_id`, `source`, `provider_transaction_id`, `contract_sha256`, `outcome_sha256`, `policy_sha256`, `eligibility_sha256`, `settlement_sha256`, `awards_sha256`, `source_sha256`, `requested`, `credited`, `occurred_at`, `applied_at` | R/X: exact accepted financial effect and provenance anchors; later typed financial/shared-proof transformation retains only necessary minimized evidence until its scoped expiry. No delayed payment may recreate erased spendable value. |
| `text_bonus_payment_items` | `match_id`, `account_id`, `kind`, `ordinal`, `body_sha256`, `server_day`, `base_credited`, `credited`, `ledger_id` | R/X: exact bonus-to-original-award/ledger linkage, including capped zero-credit facts. Preserve survivor parity during later typed financial transformation; this is not an indefinite account or content retention exception. |

D4A is credential-only; it deliberately leaves device associations, canonical
bootstrap/rotation receipts and raw installation sanction captures. Migration39
is reserved for D4B planning after the independently accepted 37/38 boundaries.
This section does not authorize implementation before its independent review.
No new migration lands during the coordinator's head38 broad gate.

### 9.1 Preserve canonical identity before removing fallback

`bootstrapAccount` currently falls back to the sole distinct `device_tokens`
account when `auth_installation_bootstrap` is absent. Removing A's canonical row
while B survives on the same installation can therefore log A's requester into
B. Removing the fallback without migrating legacy identity also loses valid
single-account bootstrap bindings. Both changes belong in one boundary.

Extend the canonical table with a fixed `state` (`bound` or `ambiguous`) and
allow NULL account only for `ambiguous`. Under offline migration locks, preserve
every existing bound mapping exactly. For missing canonical mappings, backfill
the sole distinct legacy account; write explicit ambiguous state when multiple
accounts exist. Never choose min(account_id) as an ownership decision. Subsequent
bootstrap reads only canonical state: ambiguous refuses with recovery-required,
absent means a candidate new account, and bound means the exact old account.
The existing account-before-installation transaction and changed-mapping retry
remain intact. A deleted old account still refuses admission.

D4B removes a deleted account's bound canonical row only in the same transaction
that installs its finite erased-bootstrap selector. Bootstrap checks that selector
inside its authoritative transaction before committing any candidate new account.
A survivor's bound canonical mapping never changes. An ambiguous mapping remains
ambiguous while any survivor needs the installation; deleting one association
never resolves it into the survivor implicitly. A truly orphaned ambiguous row
can be removed after exact reference checks; it does not establish that any one
of the former accounts owned bootstrap authority.

### 9.2 Keyed matching without runtime-forgeable grants

HMAC keys live outside the application database, ordinary backups and logs.
Use a small trusted Go keyring interface injected into the existing authentication
and privacy adapters. It derives separate SHA-256 HMACs over versioned,
length-delimited `(installation identity, purpose, device_hash)` messages for
`sanction` and `erased-bootstrap`; an identifier or digest from an HTTP request
is never accepted as a precomputed selector. Key IDs are non-secret metadata.
A fixture implementation uses generated keys only; no production key provider,
credential path, deployment or recovery evidence is invented by this plan.

Reuse the existing PrivacyExecutor connection for an exact typed registration
call. Ordinary Runtime cannot insert/update selector mappings or call the
registration mutator. Trusted registration binds the actual raw installation to
key ID and both derived purpose tags; retries must match exactly. This avoids
using a runtime-writable mapping, session GUC or temp row as an authorization
substitute. The adapter's derivation is a trusted service boundary, like D2's
verified provider proof; PostgreSQL never receives the secret HMAC key.

Proposed private relations, with exact final SQL/ACLs reviewed before migration:

| Object | Required fields and responsibility |
|---|---|
| `privacy_installation_keys` | `(device_hash,key_id)` PK; `sanction_sha256`, `bootstrap_sha256`, `registered_at`. Private active-installation derivation mapping, no deleted account/request link. Runtime has no direct read/write. Remove subject-only orphan mappings by active-data completion; retain mappings required by surviving active installations. |
| `privacy_installation_evidence` | `(request_id,kind,source_id,key_id,selector_sha256)` PK; `kind` is exactly sanction or erased_bootstrap; `source_sha256`, `occurred_at`, `match_until`, `purge_after`. Including the selector permits multiple captured installations per sanction/request without collapsing evidence. Sanction source_id is the exact original operation UUID; erased-bootstrap source_id is the deletion request UUID. No raw device hash, token ID, email or free text. |
| `privacy_installation_erasure_authorizations` | Typed temporary-in-transaction source grants: request/account, source kind, exact device_hash plus old_refresh_id or sanction operation_id as appropriate, original row digest, transaction ID/backend PID. Only the private definer writes/reads them; every successful invocation removes them before returning. They never become durable source copies or a general SQL operation framework. |

The existing `installation_sanction_active(text)` becomes an exact private
SECURITY DEFINER boolean reader that combines remaining raw captures with live
finite sanction selectors and current lift state. Add a separate private boolean
reader for erased-bootstrap admission; it must not block a surviving account's
explicit OAuth restore merely because anonymous bootstrap for A was erased.
Expose only those boolean results to Runtime, with fixed `pg_catalog` search_path,
exact body/owner/EXECUTE grants and no capture/control execution.

A boolean reader derives the globally required live matching key set from private
evidence itself, separately for each purpose, using wall time and actual lift
state. For every such key it requires a registered tag for the supplied raw hash;
missing registration raises an error before either a clear or blocked result.
This invariant applies to direct Runtime SQL calls, old callers and configuration
downgrades as well as configured adapters. No adapter-only activation flag can
bypass it. Before finite evidence exists for that purpose, the reader preserves
the raw predicate's behavior. Trusted adapters register all required versions
before authoritative checks. Missing key, wrong purpose, mismatching registration
or unavailable executor fails the affected operation closed. Every existing SQL
predicate caller and the inline raw-capture
checks in D2 enrollment must participate; an old unguarded branch is a D6 blocker.

### 9.3 Default compatibility and activation

Before any finite evidence exists, ordinary authentication without a configured
production keyring/executor continues using existing canonical/raw-sanction
semantics; migration alone cannot globally require an absent service. After
erasure creates finite evidence, a default manager or configuration downgrade
cannot bypass the SQL reader's required-key checks. Installation erasure is
unavailable in the unconfigured mode and
must refuse before removing any raw mapping or capture. The typed Go erasure
adapter requires its keyring and dedicated executor, the reviewed source manifest
and trusted registration of every selected installation. No routine worker or
public deletion route is enabled in this boundary.

Before eventual D6 activation, the coordinator must prove all auth/install entry
paths use the same trusted registration contract, existing active installations
have required key coverage, and the pinned runtime configuration supplies the
keyring/executor. Missing or partial deployment remains explicitly closed. A local
SQL/fixture proof is not a production configuration attestation. Registration may
run in a separate completed transaction before account authority; it must never
hold an installation lock while later acquiring an account lock.

Key rotation retains every key needed by live finite evidence and affected
backups. New registration includes the current key and required older keys;
missing an older key refuses rather than forgetting an active sanction. Rotation
never extends `match_until` or evidence deadlines. Matching stops at the original
deadline even while a verification key remains necessary for an unexpired backup.
Final key removal follows actual last-affected-backup acknowledgements from §6.
At most four distinct keys may be globally required for live matching across both
purposes. A rotation that would exceed this bound refuses before publishing new
evidence. Retired verification-only keys do not enter the live key set. Registration
atomically replaces an installation's mapping set with exactly the required keys
plus the selected current key, within the same four-key bound; it removes retired
mapping rows. Repeated calls cannot accumulate arbitrarily many mapping rows.

### 9.4 Typed cleanup and exact ownership

Proposed executor functions are a bounded source reader, exact registration,
installation erasure batch and expired-evidence purge. Their final signatures
must be pinned with implementation; a concrete candidate is:

- `privacy_installation_sources(request_id uuid,limit_rows integer) RETURNS jsonb`:
  exposes only exact selected raw installation/source keys to the trusted adapter.
- `privacy_register_installation_keys(device_hash text,key_ids text[],
  sanction_sha256 bytea[],bootstrap_sha256 bytea[]) RETURNS void`: bounded, equal
  length, distinct nonempty key IDs and exactly 32-byte tags; no arbitrary JSON.
- `privacy_erase_installations_batch(request_id uuid,limit_rows integer,
  key_ids text[]) RETURNS jsonb`: locks and derives the global live key set,
  verifies the bounded caller set equals that set plus its trusted registered
  current key, and consumes exactly those derivations. Every transformed source
  receives evidence for the same complete set atomically; caller tags cannot
  authorize erasure. Concurrent rotations serialize with this set validation.
- `privacy_purge_installation_evidence(limit_rows integer) RETURNS jsonb`:
  removes only expired private evidence with actual deletion receipts.

Each request batch locks account → request → global keyset serialization lock →
sorted installation rows → exact source binding/capture rows. Standalone trusted
registration takes global keyset → sorted installations and commits before any
later account-authority transaction. A joined source-reader/registration/erasure
transaction also takes the global lock before its first installation lock; no
privacy writer may take installation → global. The exact lock is pinned in the
implementation, with both-order PostgreSQL wait proofs. Ordinary boolean readers
use one consistent statement snapshot for required-key coverage and matching;
they do not acquire this writer lock after an existing admission installation
lock. It requires D4A's credential receipt, matching fence,
independent suppression and correct manifest. Source read, trusted derivation,
registration and mutation can share the same short transaction without network
calls under locks. The public batch bound counts logical source units, not SQL
row writes: one unit may atomically insert its bounded key-version evidence,
authorize and remove one original row, then remove its authorization. Limit1
must make progress. Registration accepts at most four simultaneously required
key versions; each selected unit has an explicit fixed maximum write count,
tested against that count. Exact revocation removals consume their own units; a
source witness survives if those prerequisites exhaust the budget. Conditional
orphan key/registry cleanup remains atomic within its original source unit, so
removing that source cannot discard the only ownership witness. The typed erase
function permits at most 12 row mutations per logical unit plus one terminal
receipt, and reports its actual mutation count. The Go adapter's preceding
registration is separate: for each discovered hash it can ensure one registry
row, insert at most four mappings and remove at most four retired mappings.
`Mutations` describes the typed erase function, not every write in the joined Go
transaction, and is not limited to `limit_rows`. No source is removed before its required selector receipt is
committed. All catalog assumptions remain locked against concurrent DDL.

| Source | Exact selection and allowed result |
|---|---|
| `device_tokens` | Remove only rows with account_id=subject. A shared raw hash does not authorize deletion of B's association. |
| `auth_installation_bootstrap` | Bound mapping owned by subject is replaced by private finite erased-bootstrap evidence, then removed; unrelated bound or still-needed ambiguous mappings remain exact. |
| `auth_installation_rotations` | Remove subject account's original refresh and issuance receipts, after their exact revocations are safe to remove. Preserve every survivor receipt, including the same installation. |
| `account_sanction_installations` | Select only operations targeting subject. Derive finite sanction selector before removing each raw capture; preserve actual imposed/end/lift facts and operation linkage. No generic consent/security hold extends matching. |
| `oauth_flows`, `portal_login_requests`, `portal_browser_sessions` | Account-bound subject rows belong to D4A. Account-NULL/device/requester-only rows require a separately provable subject ownership witness; shared hash alone is insufficient. Otherwise retain only their existing bounded expiry and expose that outstanding deadline honestly. |
| `auth_installations` and private key mappings | Remove only after no surviving device, canonical/ambiguous, rotation or unexpired legitimately shared flow/session reference needs the installation. Recheck after row-lock waits. |

New source-specific immutable guards recognize only exact transaction-local typed
authorizations created by the private erasure function. Runtime cannot fabricate
those rows, SET ROLE, widen source selection, reuse a completed grant or call a
generic trigger bypass. Original unrelated rows and source digests remain exact.
Each successful invocation verifies that no temporary authorization row remains.
The source guards are narrowly scoped trigger-only SECURITY DEFINER functions
owned by PrivacyOwner, with fixed `pg_catalog` search_path, no PUBLIC/Runtime/
Capture/Control EXECUTE and exact body/owner pins. They alone read the private
authorization rows when ordinary source writers invoke a trigger. A permit must
match `pg_current_xact_id()`, backend PID, source kind, exact OLD primary key and
OLD canonical row digest; no user-supplied context changes those checks. Ordinary
invoker guards are not granted private SELECT merely to implement this exception.

For sanction evidence, `match_until` and `purge_after` are no later than the
earlier of the actual timed sanction end and deletion verified_at+180 days; lifted
sanctions cease matching immediately. Erased-bootstrap matching lasts no longer
than verified_at+180 days. Expiry does not permit the old account UUID to return:
bootstrap after expiry can create a new account, never infer a survivor through
the removed legacy fallback. Holds cannot retain authentication material or
extend sanction/bootstrap matching; later retention of necessary decision facts
uses a separate typed evidence class, not these live matching rows.

Failed or refused first-time authentication can leave a registration and registry
row without a subject request. D4B does not infer ownership or invent a retention
period for those operational rows. D4E must add bounded orphan registration
cleanup, with an explicitly adopted short operational TTL, request cancellation
and both-order registration/admission race proofs. It must also dispose of an
ambiguous canonical marker only after exact surviving references no longer need
it. Until that reviewed cleanup exists, these rows are an explicit outstanding
active-data obligation; installation-source completion is not all-data completion.

### 9.4a Proposed D4E orphan registration cleanup

**Planning only, no migration reserved.** Adopt a proposed **15-minute idle TTL**
for unbound operational registration rows, with a bounded cleanup pass at least
once per minute when enabled. This is an engineering recovery allowance, not a
new financial/security retention policy. Fifteen minutes exceeds the existing
five-second registration/anonymous/binding work and ten-minute OAuth flow window;
legitimate longer-lived flow references are protected explicitly, not by hoping
that this TTL is long enough. A stopped/failed request leaves no indefinitely
renewing timer. Rows idle beyond the TTL are eligible for removal as soon as the
bounded worker reaches them; queue age/backlog must be observable and activation
must demonstrate clearance within the existing 30-day active-data ceiling.
Repeated attempts may refresh the idle clock only after successful trusted
registration, subject to existing request budgets. They must not indefinitely
retain a permanently unbound registry incarnation: add a **24-hour hard maximum**
for an incarnation with no protecting reference. This is a separate operational
resource bound. It never extends deletion evidence `match_until`/`purge_after`,
resurrects a deleted account, or resets a deletion job's deadline.

**Registration contract.** Today migration39 inserts up to four
`privacy_installation_keys` rows with `registered_at`, but `ON CONFLICT DO NOTHING`
does not refresh that timestamp. A GC based on its age alone would remove mappings
under an active reused installation. The trusted registration function must update
`registered_at` for the exact validated key set at one database instant while
retaining the existing digest-conflict check, required global key coverage and
four-key bound. Return that database instant and `not_after=instant+15 minutes`
as an in-process registration receipt. It contains no secret or public client
capability and is never logged. Re-registration of identical mappings can advance
the timestamp; an older operation is not invalidated by a newer valid registration.
The current `auth_installations` schema has no timestamp. Add immutable database
`created_at` solely for registry-incarnation age, with an indexed cleanup
candidate path; existing rows receive migration observation time, explicitly not
a claim about their historical creation. An unbound registration receipt expires
at `LEAST(registered_at+15 minutes, created_at+24 hours)`. A further failed or
refused authentication cannot advance `created_at`, including conflict retries.
At the hard maximum, registration of an unprotected incarnation refuses before
refreshing mappings; GC may remove it after the short final authority interval
has expired. There is no synchronous delete/reinsert shortcut in registration.
A later fresh request after GC may create a new registry incarnation, but cannot
recover an account from removed mappings. Preventing an adversary from submitting
the identical raw input again is not an indefinite retention guarantee; existing
request limits still apply. Bound/shared/security references make the row
ineligible for this orphan maximum and are never shortened to 24 hours. For a
registry with no private mappings, `created_at` supplies the initial idle age;
do not invent an owner from an unbound hash.

Configured auth operations must bound the interval from registration through their
final database authority check/commit to at most 30 seconds, independently of a
caller that forgot a timeout. OAuth provider HTTP occurs outside this interval;
callback/result re-register before their own short authority transaction. After
all row waits and final writes, check database time against the original returned
`not_after`, current mapping coverage and current sanction/bootstrap predicates.
Use a narrow Runtime-callable boolean that reads private mappings under the
existing fixed definer contract; Runtime receives no private table SELECT. Its
caller-supplied receipt can only narrow the database-derived interval, never
extend it or assert a digest/key exists. Global required key coverage and exact
unchanged registered digest identities remain independent requirements.
If expired/missing, rollback and return a retryable authority refusal; any retry
re-registers outside account/installation locks. Never re-register or acquire the
global writer lock from `ValidateAccessTokenTx`. No stale receipt may authorize a
new mapping, and time checks never rely on workstation clock skew.

This receipt alone is insufficient: every transaction creating a durable hash
reference must hold the registry row through predicate, insert/update and commit.
In particular, `BeginOAuth` currently checks installation predicates and inserts
an unbound restore flow without an installation row lock. Add the same explicit
installation serialization used by bootstrap/binding before that flow can become
a GC protector. For link intent retain account-first locking; for account-NULL
restore lock only installation and then the existing OAuth budget/flow objects,
never discover and acquire a different account afterward. Callback/result already
know the account before linking and retain their existing account/flow/install
order. GC reads flow references without locking flow rows, so it must not introduce
an installation→flow-row wait opposite that order. All other creators of device,
rotation, sanction, pairing/browser or deletion-intent installation references
must be audited and receive equivalent serialization where absent. Read-only
access checks perform the freshness/coverage/predicate query in a single statement
snapshot; later mutating admission always revalidates under its normal locks.

**Exact eligibility and locks.** The executor discovers at most 100 candidate
hashes with an indexed age query, but processes **one installation per transaction**.
It locks the same global keyset serialization object as registration, then the
registry row and its at-most-four mapping rows. It rechecks their current maximum
registration timestamp after waits. If any mapping is fresh and the unprotected incarnation is below its hard
maximum, skip the entire installation; at the hard maximum require the returned
authority interval expired. Do not delete individual retired mappings independently of the
existing bounded registration/key-rotation contract. The candidate query is only
an optimization, never deletion authority. The GC acquires no account row and no
privacy request FK after the global keyset lock. If any account-associated
reference appears, skip it; account-owned removal remains the existing
account→request→global-keyset→sorted-installations eraser.

Before deletion, require absence of all `device_tokens` associations,
`auth_installation_rotations`, raw `account_sanction_installations`, any **bound**
`auth_installation_bootstrap`, and any unexpired `oauth_flows`,
`portal_login_requests`, `portal_browser_sessions` or `privacy_deletion_intents`
whose exact installation column matches. Preserve a shared association even when
its account is banned/deleted; only its own reviewed cleanup can remove that
identity. Preserve raw sanction/rotation rows irrespective of their apparent age
or lift, because their erasure/evidence obligations belong to other typed steps.
A private finite evidence selector match alone does not require keeping a raw
registration: fresh trusted HMAC derivation on the next attempt recovers the
matching check while the original finite evidence remains unchanged.

An **ambiguous**, account-NULL canonical marker can be removed only after the same
reference-absence checks, idle TTL and row lock prove it no longer protects any
surviving or retained association. Bound markers are never orphan-GC candidates.
Expired unbound flows/intents are handled by their own bounded expiration routines;
this operation does not delete those credentials or claim their cleanup. Reference
checks use their actual existing expiry/state authority, with immutable old OAuth
issuance receipts protected until their own valid-replay interval ends. If an
existing receipt's deadline cannot be established from the schema, preserve it
and report the remaining obligation instead of guessing a new TTL.

GC first removes the at-most-four mappings, then an eligible ambiguous marker,
then the registry row in one transaction, checking exact affected-row counts and
absence afterward. No keyring/HMAC material or raw hash is copied to a durable
orphan audit log. Counters record only selected/skipped/removed totals and oldest
eligible age. Registry creation races serialize on its unique row; if a candidate
vanished/reappeared, re-evaluate the current timestamp and references rather than
using the stale scan result. Interrupted or uncertain cleanup is safely retried
from actual remaining rows, without a fabricated deletion request.

**Authority and rollout.** One executor-only typed cleanup function with integer
limit 1..100; no caller hash, arbitrary source table or SQL. Add narrowly scoped
private transaction-local orphan authorization only if the existing immutable
bootstrap/registry guards require it. This authority has no fake account/request:
it binds transaction ID/PID, fixed source kind, exact OLD key/digest, and is
consumed/cleared within the same transaction. Do not relax the existing
request-backed erasure authorization to accept NULL subjects. The trigger-only
PrivacyOwner guard may recognize this separate exact permit; Runtime/Capture/
Control have neither permit rights nor cleanup EXECUTE. Fixed `pg_catalog`
search_path, literal source ACLs, source/private catalog checks with DDL locks and
cutover function body/property pins accompany the migration. The new registry timestamp also updates every dependent39 catalog manifest
and cutover source grant explicitly. Registration return
shape and all its callers change together; a compatibility default cannot start
GC while an old binary still admits without the required serialization.

No production GC is enabled until registration, all reference creators, predicate
readers and both old/new cutover inventories pass together. A runtime with no
configured privacy adapter preserves existing pre-erasure behavior; if globally
required finite evidence exists it still fails closed on missing mappings. GC
must not turn missing key coverage into an allowed bootstrap. In-flight read-only
old authority is not a substitute for final new-admission checks.

**Required red/green and acceptance proofs.**

- Registration paused before account acquisition versus GC in both orders;
  fresh attempt survives, expired attempt rolls back without account/profile/token,
  and retry re-registers without an account/global-lock inversion.
- Real PostgreSQL waits for OAuth restore flow creation versus GC, and callback,
  result, refresh, bind, pairing and protected admission versus GC; no live flow
  loses derivation between registration/predicate/commit and no stale access mints
  a credential. Include cancellation, process death and exact TTL boundary.
- Old mapping refreshed by registration survives within its unbound maximum;
  failed/refused first auth ages out, and repeated failures cannot reset the
  immutable 24-hour incarnation deadline. Bound/sanction/shared rows survive
  beyond that boundary exactly. Repeated key rotation remains at most four mappings. Mixed fresh/old key
  rows and fifth-key/missing-required-key attempts cannot cause partial erasure.
- Shared A/B installation, bound/ambiguous bootstrap, raw sanction/lift, rotation,
  unexpired/expired OAuth issuance and deletion-intent fixtures preserve exact
  survivor rows. Eligible ambiguous marker and limit-one orphan converge; no old
  deleted UUID or surviving account becomes anonymous bootstrap fallback.
- Direct Runtime/Executor permit forgery, extra/missing ACL, private RLS/rule/FK/
  trigger drift, concurrent DDL and injected rollback refuse atomically. Actual
  separate executor LOGIN proves cleanup; no raw hash appears in logs/metrics.
- Restart/backlog test proves bounded batches and age reporting; joined auth,
  privacy, portal, gameplay admission and cutover gates pass before enablement.

### 9.5 Required tests and ownership gates

The privacy lane owns the migration39 proposal, typed privacy/auth integration
and tests; the coordinator owns cutover catalogs/provisioning/configuration and
tracks D4B completion. Shared source readers are coordinated before edits. Review
then independent verification precede implementation of a later boundary.

Proofs must include canonical legacy single-account backfill, ambiguous refusal,
existing canonical parity, A/B shared installation in both canonical directions,
last-association cleanup, unbound flow refusal/expiry and restart. Run anonymous
bootstrap, bound refresh, legacy binding, OAuth callback/result, portal pairing,
admin and gameplay admission concurrently with deletion in both lock orders.
Observe real PostgreSQL waits and preserve all survivor rows/value. Verify that
no candidate profile/account commits on an erased bootstrap and no old UUID
returns after the finite selector expires.

Cryptographic tests cover wrong purpose/key/installation, malformed registration,
exact and uncertain retries, runtime forged rows/GUC/SET ROLE/EXECUTE denial,
missing executor/keyring, old-key rotation coverage, exact boundary expiry and
lift, and no secret material in DB/snapshot/log fixtures. Exercise both boolean
readers with direct Runtime SQL and a default manager after finite evidence exists,
missing each required key independently, concurrent key rotation, a fifth live
key refusal and repeated registration's bounded mapping count. Test failure at every
source/evidence/grant write and resumable bounded batches with limit1. A partial
batch must retain its ownership witness and cannot emit a completed receipt.
Include two installations captured by one sanction and two canonical installations
owned by one deleting account; limit1 retries preserve both distinct selectors.
Expired purge must not delete active survivor mappings or extend evidence based
on a backup's newer metadata. Down migration refuses retained transformed data;
empty up/down preserves original rows and exact function bodies.

This boundary cannot claim financial/bonus source erasure, shared outcome/content
transformation, all active-data completion or production restore readiness. Those
remain D4C–D4E/D5/D6 obligations.

## 10. Proposed D4C C1: fence and reconcile billing provider work

### 10.1 Goal and specification

Required reading: Blueprint **Profiles & Community §1a**, **Monetization §§1–3
and §5**, and **Product Baseline**, together with this ADR's disposition manifest.
Deletion ends access and spendable value; it neither refunds cash nor cancels a
platform subscription. C1 makes pending provider work converge without recreating
an erased account or losing the truth about an uncertain external ACK. This is a
proposal for migration41, after39/40 and their independent gates; the coordinator
reviewed this plan with the amendments below. Implementation remains gated on
39/40 independent acceptance; this section records a reviewed plan, not delivered
behavior.

C1 does not erase raw receipts, minimize financial identities, detach billing
FKs, cancel subscriptions, issue refunds, register a deletion worker, mount a
route, or claim active-data completion. C2 will erase raw payloads using the
resulting drain evidence; C3 will transform and expire retained financial identity.
Existing normal billing registration remains unchanged except for the new work
fences and attempt coordination required by this boundary.

### 10.2 Concrete schema and authority

Add one ordinary runtime-owned `billing_provider_work` relation. Its identity is
`(purchase_id uuid, operation text)`; purchase_id references store_purchases and
operation is exactly `observe` or `ack`. Fields are `work_kind` (purchase or
subscription), `generation bigint`, nullable `attempt_id uuid`, `state`
(ready/in_flight/uncertain/stopped), nullable `deadline_at timestamptz`, nullable
`request_sha256 bytea`, nullable `proof_sha256 bytea`, `last_outcome` (a fixed safe
code), nullable `privacy_request_id uuid`, and `updated_at timestamptz`. Hashes,
when present, are 32 bytes. No receipt, provider token, email or provider response
body is copied into this table. This is current coordination state, not an
immutable ledger or a generic task framework.

Subscription work uses its existing initial_purchase_id. Migration41 first
refuses ambiguous repeated initial_purchase_id mappings, then adds the redundant
UNIQUE constraint to billing_subscription_sources that makes this work identity
explicit. Existing imported rows and paid value are otherwise unchanged. A typed
guard validates work_kind against the retained purchase/source and product kind;
the same receipt cannot claim both source kinds. No legacy receipt without a
verified source becomes work merely because raw JSON is present.

The guard preserves identity, monotonically advances generation, and requires a
new unpredictable attempt_id for each claim. Only an exact current generation,
attempt, request/proof digest and state may finalize. In-flight fields are either
all present or all absent as appropriate. A deletion binding is write-once and
must match the existing request/fence/account. Runtime cannot establish or clear
that binding: use explicit INSERT and UPDATE column allowlists, both excluding
privacy_request_id, and an executor-only typed definer sets it. The cutover
verifier needs a new exact column-grant class for this table; the ordinary
table-level DML baseline must not accidentally grant those excluded writes. No arbitrary SQL or session-GUC grant
is introduced. Once deletion-bound, ordinary runtime cannot claim fresh work;
only the closed drain path may claim an ACK/reconciliation attempt. Ordinary
finalization of an already claimed attempt remains exact and cannot grant value.

Initial VerifyReceipt dispatch precedes any verified purchase row. Add a separate
bounded `billing_verification_slots` relation keyed `(account_id uuid, slot int)`:
`generation bigint`, `attempt_id uuid`, `request_sha256 bytea`,
`deadline_at timestamptz`, `state` (in_flight/uncertain/stopped), and
`updated_at timestamptz`. Account is an FK; request hash is exactly 32 bytes.
Slots range 1..32, matching the current enabled-provider validation in
server/internal/config/billing.go: MaxConcurrentRequests is already 1..32. Live
admissions still honor that configured limit and the existing global semaphore;
this introduces no lower configured ceiling. Test both boundary values and reject
future config/schema bound drift. Claim reuses only a stopped/expired slot
under the account lock, advancing generation; it never creates a purchase,
provider ownership or a raw-receipt copy. Ordinary verification claims this slot
before its first HTTP call. Deletion checks/stops these slots as well as retained
source work. A stale slot response cannot apply value: source discovery locks
precede a fresh account lock, then exact slot/attempt checks and the existing
provider-authenticated account-binding validation. Slot rows have no private
binding and cannot bypass the account deletion fence. Test a new receipt with no
store_purchases row, including deletion during HTTP and a paused expired sender.

Use the following executor-only typed contracts, each with literal signature/body
pins, fixed pg_catalog search_path and exact source-column allowlists:

- `privacy_begin_billing_drain(request_id uuid) RETURNS jsonb`
  binds/stops exactly one discovered source unit, or one verification slot. Its
  result contains only counts and the selected purchase/operation identity. Each
  invocation is its own transaction: the Go 1..100 budget loops transactions,
  never source units inside one transaction while an earlier account lock remains
  held. The selected source unit may include its observe and ACK work rows.
- `privacy_claim_billing_attempt(request_id uuid, purchase_id uuid,
  operation text, attempt_id uuid, timeout_seconds integer) RETURNS jsonb` derives the current exact
  request/proof hashes, increments generation and persists an absolute bounded
  deadline using the trusted adapter's validated Billing.HTTPTimeoutS (SQL1..30),
  capped at active_due_at. An exact attempt replay returns its original deadline,
  never extends it. It returns generation/deadline/hashes plus the exact selected raw
  request/proof only to the dedicated executor for that external call. It cannot
  accept arbitrary provider URLs or caller-substituted request bodies.
- `privacy_finish_billing_attempt(request_id uuid, purchase_id uuid,
  operation text, generation bigint, attempt_id uuid, request_sha256 bytea,
  proof_sha256 bytea, outcome text) RETURNS jsonb` accepts only a fixed adapter
  outcome enum. It locks and compares every field, handles exact retained replay,
  and rejects stale/conflicting finalization. It cannot grant value. Hashes prove
  request identity, not provider success; the trusted concrete adapter is the
  authority for the outcome, as in the existing verifier architecture.
- `privacy_finish_billing_drain(request_id uuid) RETURNS jsonb` performs the
  final fresh subject-wide postcondition query and reports pending/uncertain
  counts. It emits the existing private `billing_drained` step receipt only
  when no fresh ordinary work, live attempt or retryable ACK remains. A receipt
  explicitly records confirmed/no-op versus deadline-abandoned outcomes; it is
  evidence that application work stopped, not processor erasure attestation.

No PUBLIC/Runtime/Capture execution is granted. Functions derive ownership from
the request and retained purchase/source, rather than accepting account/provider
keys. Extend privacy_step_receipts' exact step/result CHECK; never set
privacy_requests.phase to active_removed. Runtime finalizers use their own
exact attempt comparison under the same guards; they cannot write private
binding columns. Do not rely on random attempt IDs alone to authorize a caller.

### 10.3 Locking, attempts, uncertainty and restart

Use the established billing order, not the account-first credential eraser:
discover source identity without locks; lock billing_account_sources keys in
sorted `(platform,key)` order, then the canonical account, privacy request if
needed, purchase/source/projection/task/work rows. Subscription replacement
checks lock both predecessor and successor keys before the account. Discovery
is rechecked after waits; a changed/missing source set rolls back and restarts
before acquiring additional source keys. Each transaction handles one bounded
source unit rather than locking an account's unbounded history.

C1 first aligns source-less legacy Refund with account-before-purchase: discover
the purchase owner without locking, lock that canonical account, lock the exact
purchase and recheck owner/product/refund state. Existing source-bound refusal
remains. Prove concurrent Refund/Refund and Refund/deletion in both orders before
using this common account-first tail in the eraser. No legacy unit needs a source
lock, and no purchase-to-account exception remains in the final implementation.
No SQL lock
survives an external HTTP call. The drain does not take D4B's installation/keyset
locks. Parent cutover inventory includes the work table, guards and executor APIs.

All ordinary verification and ACK entry points claim before dispatch, using a
verification slot before source discovery and source work for retained retries, including
the inline ACK after a verified purchase and both retry paths. A claim persists
an absolute database deadline bounded by the existing billing HTTP timeout and
the exact request/proof hashes before releasing locks. The provider call uses
that absolute deadline, an owned cancelable request, existing verification-slot
limits, and bounded response parsing. Recheck expiry immediately before dispatch;
a paused/resumed process cannot obtain a fresh timeout from an expired attempt.
Finalization uses a fresh transaction and exact attempt comparison, including
after a deletion fence or source replacement. It never recreates a removed task,
projection, entitlement or wallet, and a zero-row update is not success.

Timeout, transport loss, process death, or a successful provider ACK whose DB
finalization fails yields `uncertain`, not done. Expired in-flight claims become
uncertain on recovery; expiry alone is not evidence that the provider did nothing.
For ACK, use the existing concrete adapter's re-read-before-write semantics:
Google consumables GET purchaseState/consumptionState and regard consumed=1 as
complete; otherwise they POST consume. Subscriptions GET acknowledgementState
and regard ACKNOWLEDGED as complete; otherwise they POST acknowledge. A successful
POST is positive evidence. Neither a 404/410, an arbitrary error nor an unqueryable
consumed purchase is evidence of success. The current adapter exposes only error
or success, so extend the closed adapter result to distinguish observed complete,
POST succeeded, unavailable/unknown and no-server-operation. Apple Acknowledge is
a local no-op because StoreKit completion is client-side; it must not be labeled
server-confirmed consumption. Do not assert that every repeated provider POST is
idempotent: retain the current GET-before-write contract, and require provider
contract verification before any new retry interpretation. Do not call the existing value-applying VerifyReceipt path for a
deleted account. Retained `observe` work is read-only provider activity admitted
before deletion: the drain waits for its finite lease, then stops it and rejects
late value application. It does not launch a new observation GET for a deleted
account. Private claim/finish APIs accept only `ack`; the concrete ACK adapter
performs its existing GET-before-write reconciliation without executing
applyProof/applySubscription or syncBillingPremium.

The closed Go drain adapter accepts a deletion request ID, executor connection,
existing real verifier interfaces and a 1..100 source-unit budget. It calls the
typed begin operation, claims a selected attempt, performs ACK/reconciliation
HTTP outside transactions, finalizes that exact attempt, then calls the aggregate
finish operation. `billing_drained` means known application provider work stopped;
it is not ownership, verification, ACK or processor-erasure evidence for legacy
or unknown-source receipt rows. Such rows remain unverified and byte-identical
in C1, with their own explicit raw-input/deadline disposition required in C2.
The typed functions check the released source columns, constraints, incoming FKs,
trigger bodies/definitions/enabled state, RLS and relation shape under catalog
locks. The cutover verifier separately pins function owners, security mode,
configuration and execution ACLs; the runtime catalog check does not replace it.
It is executable in
tests but has no main registration. Disabled/missing providers produce an honest
pending obligation before the active-data deadline. Proposed bounded disposition
at the retained privacy_requests.active_due_at deadline: stop further dispatch, revoke the current attempt
generation, record `abandoned_at_privacy_deadline` with a safe unresolved outcome,
and permit C2 raw-input cleanup after its own gates. Late responses cannot mutate
value or reopen work. This never reports provider ACK success or processor erasure;
only minimal non-raw uncertainty evidence enters the existing finite financial
retention class (bounded by the retained privacy_requests.evidence_until). No raw token/receipt is kept
indefinitely merely because a provider no longer answers. This deadline outcome
is part of the plan for coordinator review, not a claim that C1 alone completes
active removal; C2 and processor obligations remain separately visible.

### 10.4 Files, implementation checklist and proof gates

Expected scope: migration41 pair; a focused economy billing-work source/test pair;
narrow hooks in purchases_verified.go, purchases_subscription_store.go, concrete
Google/Apple outcome adapters and
purchases_worker.go; source-less Refund compatibility tests in purchases_test.go;
typed privacy drain source/tests; and coordinator-owned cutover inventory, ACL,
provisioning and runtime-boundary tests. Confirm file absence before creating
these files. No edits to the ledger, bonus payout/outbox or client are planned.

- [ ] Add work identity/shape/transition constraints and empty migration up/down
  proof. Refuse ambiguous legacy source identity and retained drain rollback.
- [ ] Claim initial verification slots before a purchase exists and retained
  observation work with exact attempt identity;
  prove current behavior for active accounts and no duplicate source/value writes.
- [ ] Claim/finalize inline and retry ACK work, including subscription replacement;
  prove lost provider response and lost DB finalization recover the same operation.
- [ ] Bind one source unit to a verified deletion request under the documented
  locks; prove all fresh ordinary work stops without falsely marking ACK success.
- [ ] Add the closed ACK drain adapter, read-only observation lease stop and exact finish receipt;
  prove limit1 progress, restart, cancellation and unavailable-provider status.
- [ ] Exercise deletion versus verification/ACK in both orders with observed
  PostgreSQL waits, including a request copied before deletion, late HTTP success,
  expired attempt, stale generation and source-replacement races. Test consumed
  but unqueryable purchases, every provider error class, Apple no-op, exact retained active_due_at
  abandonment, late response after abandonment and bounded slot reuse.
- [ ] Prove exact survivor receipt/ledger/entitlement parity; deleted account never
  gains a profile, wallet, entitlement or grant, and unknown legacy proof stays
  unverified. Inject failure at every work/fence/task/receipt write.
- [ ] Independently review then cold-verify the focused matrix, real restricted
  executor/ACL denials, all touched billing tests, and the unified tests/lints
  runner before coordinator tracking. Keep the drain unregistered.

### 10.5 Remaining risks and later dependency order

Do not treat a lease as processor erasure attestation: restart/deadline/transport
proof and exact external outcomes are required. Source key removal is deferred;
future proof still must contain the provider-authenticated original account
binding, so old paid sources cannot be adopted by a newly registered account.
Unsupported legacy ownership fails closed rather than guessing an owner.

C2/C3 must respect migrations19/21/22: subscription tasks/current/replacement
edges/imports precede their referenced sources; current precedes observations;
imports and provider tasks precede billing_transactions; transactions and
subscription_sources precede store_purchases. Retained identity/projection/task
triggers require typed, row-bound erasure exceptions. billing_legacy_premium,
named_entitlement_items and active entitlements are separate dispositions.
Original financial/source hash anchors must not be mislabeled as hashes of
sanitized rows. Broader ledger/bonus links, finite evidence/holds and backup
suppression remain C2/C3 and D4E/D5/D6 gates.

### 10.6 Proposed C2: remove raw billing data after the C1 drain

**Planning only; no migration number reserved.** This continuation preserves
§10.1–10.5 and starts only after the accepted C1 boundary. Its goal is to remove
subject-owned provider secrets and application billing copies by
`privacy_requests.active_due_at`, while copying only necessary, typed financial
facts into private evidence expiring at that same request's `evidence_until`.
Neither clock restarts on retry, provider response, migration or restoration.
C2 does not refund money, cancel a platform subscription, erase another account's
source, rewrite a surviving player's ledger, or enable deletion routes. C3/D4E
still own final account/reference detachment and joined evidence/backup completion.

**Disposition and exact source surface.** Freeze a fresh post-42 catalog before
implementation; migrations3/19/21/22/41 and this list are the starting inventory,
not permission to ignore later foreign keys. The private copy is a versioned
allowlist; never copy `to_jsonb(row)` into an evidence payload.

| Source | Fields removed from ordinary tables | Minimal candidate evidence, only when necessary |
| --- | --- | --- |
| `store_purchases` | Entire subject row, including `raw_receipt`, `transaction_id`, `account_id`; delete only after dependencies below | Platform/product, nullable amount with its original unknown meaning, verification/refund/creation times, verified/legacy disposition; an opaque private evidence identity |
| `billing_provider_tasks`, `billing_subscription_tasks` | Entire task, especially `request` and `proof` (both contain additional provider/account copies) | Final C1 operation/outcome, attempts and completion/uncertainty time; no request/proof JSON or tokens |
| `billing_subscription_observations` | Entire row, including `evidence`, `source_key`, `transaction_key` | Provenance/state/product kind and product, purchased/observed/signed/coverage times; keyed necessary transaction correlation; original evidence digest explicitly labelled an original anchor |
| `billing_subscription_current` | Entire subject projection, including source/observation link and verified/checked times | No separate projection copy; relevant final observation facts already retained |
| `billing_subscription_imports` | Entire row, including purchase/source linkage | Necessary import provenance and original `row_sha256` anchor, never relabelled as a digest of a replacement row |
| `billing_subscription_replacements` | Exact subject-owned edge, including both raw keys and account | Only necessary predecessor/successor relationship between private keyed evidence identities, application/environment and original time |
| `billing_transactions` | Entire subject row, including raw `provider_key`, `original_key`, account and purchase link | Platform/application/environment/product/kind, quantity/Noin amount, state/refund state, original purchase/observation/provider/expiry/revocation/grant times; no raw provider identity |
| `billing_subscription_sources`, `billing_account_sources` | Entire exact subject binding after child rows disappear | Purpose-keyed source selector only when reconciliation requires it; original ownership classification and registration time, private and finite |
| `billing_provider_work` | Exact stopped subject work row, including purchase/request binding and attempt metadata | C1 generation, operation, terminal outcome and deadline only if needed to distinguish acknowledged from abandoned; source anchors optional and purpose justified |
| `billing_verification_slots` | Stopped subject slot, including account, attempt and request digest | Normally none; never preserve an initial receipt as verified evidence merely because a slot existed |
| `billing_legacy_premium`, `named_entitlement_items`, `entitlements` | Exact subject benefits/projections, including value and source association | Acquisition type/time and original coverage/unknown expiry only for a necessary financial case; no active benefit remains |

`noin_ledger.reason`/`payload`, refund/admin audit copies, receipt-related archives
and processor logs can also contain purchase IDs or provider material. Inventory
and classify their exact codecs before declaring *all billing raw data* removed.
Shared ledger/audit rows remain under the typed D4D/D4E transformations; C2 cannot
mark `active_removed` while those copies or unrelated deletion obligations remain.
Neither provider access credentials in server configuration nor another account's
billing records belong to a subject cleanup operation.

**Private representation and finite authority.** Propose one private
`privacy_billing_evidence` relation, owned by PrivacyOwner, with opaque `id`,
`request_id`, checked `kind`, version, keyed source selector plus key ID where
necessary, explicit original source digest, strict typed facts, `recorded_at` and
`expires_at`. There is no raw account/provider ID, receipt, signed token, email,
free-form provider error or arbitrary JSON field. A request FK is temporary
private lineage; no runtime grant, receipt retry or source-ownership decision may
depend on this table surviving expiry. Exact typed columns/JSON CHECKs and the
per-kind facts schema must be approved before its migration is written. Unknown
facts stay NULL with an explicit unknown classification; zero is not a substitute.

Use a distinct externally supplied financial-selector HMAC purpose, not an
installation/suppression digest or a naked hash of low-entropy provider keys.
The trusted executor derives selectors from the locked source; the public client
never submits them. Bound key versions (at most four), include the key ID in the
identity, and atomically retain every still-required version during rotation.
No perpetual key-to-account lookup is added. Evidence and correlation mappings
expire no later than `evidence_until`; normal runtime cannot read either. For this
C2 boundary, holds can select only exact minimized evidence and can shorten or
preserve the existing expiry, never extend it beyond the adopted 180-day window,
retain raw credentials, or renew automatically. Any different hold policy needs
an explicit later policy decision; it is not an implicit implementation exception.

**Drain prerequisite and legacy truth.** Require a matching deleted account,
request/fence, independently durable suppression receipt and retained C1
`billing_drained` or `billing_drained_unresolved` receipt. Recheck every selected
work tuple is stopped, and that no initial verification slot can still commit.
Unresolved provider writes retain their honest outcome; cleanup must not turn
`abandoned_at_privacy_deadline` into successful ACK. No fresh provider HTTP occurs
inside or because of a C2 erasure transaction. At `active_due_at`, C1 terminates
known application work using its existing deadline semantics; unavailability
cannot preserve raw copies indefinitely.

A `store_purchases.account_id` proves application row ownership, not provider
verification. A legacy row with no verified transaction/subscription source may
be removed for that exact account after fence/slot checks without inventing a
provider source or dispatching a provider request. Keep a minimal
`legacy_unverified_removed` disposition only if financial evidence is actually
needed (for example an existing non-NULL granted amount); do not retain its raw
receipt as evidence of ownership. Cross-account or inconsistent verified source
joins refuse and produce a diagnosable incomplete step, never choose an owner
from JSON, transaction text or a shared product. This does not silently exempt a
corrupt source from the 30-day deadline: it requires repair/escalation before
activation and explicit incomplete status if encountered operationally.

**Reader and delayed-work contract.** Before deleting the first source, update
and test `RecordReceipt`, `Refund`, `VerifyReceipt`/`applyProof`,
`verifySubscriptionReceipt`/`applySubscription`, `syncBillingPremium`,
`ProcessProviderTasks`, both ACK retry enumerators, management/account purchase
readers and C1 drain/replay APIs. `RecordReceipt` currently inserts directly;
it must acquire/recheck live account authority so a late legacy call cannot
recreate rows for a tombstone. Enumerators must tolerate a row erased after their
read and refuse before HTTP/value writes, without fabricating provider success.
Every C1 begin/claim/finish API checks its exact retained completion receipt
before source discovery, including between C2 batches when provider work has
been erased but the purchase/verified source still exists. Replay cannot recreate
work or recompute an original unresolved count as zero; preserve the original
terminal outcome through partial erasure, full erasure and evidence purge. Ordinary receipt replay after erasure returns an explicit
unavailable/deleted result, never reconstructs an original receipt from evidence.

Verified new claims continue to require the provider-authenticated original
`AccountID` to equal the current live account. An old purchase cannot be adopted
by a new identity after raw source keys and finite selector evidence expire.
Missing provider account binding fails closed, including legacy receipts; raw
transaction equality alone is insufficient. Prove this after complete evidence
purge, not only while a private deny selector exists. Financial callbacks may
record a narrowly scoped reconciliation outcome while evidence remains, but
must not reopen application work, recreate a benefit/wallet/account or refresh
evidence expiry. No new callback surface is authorized by this plan.

**Bounded transaction and dependency order.** One closed Go driver takes
`request_id` and a budget of 1..100 source rows; it invokes one typed SQL unit per
transaction. A unit chooses one indexed row from a fixed stage order, discovers
its complete bounded source key set, locks sorted `billing_account_sources`
keys, then account → request → selected source/dependent row. Recheck selection
and source ownership after waits. Replacement edges require both endpoints
locked before the account. Source-less legacy rows and stopped slots use
account → request → purchase/slot and must recheck that no verified source has
appeared; a newly discovered source forces rollback/restart, never a late source
lock. Do not process a second source after retaining the first unit's account
lock. No recursive unbounded replacement-chain lock or delete is allowed. Pin
migration41 UNIQUE(subscription_sources.initial_purchase_id), the replacement
predecessor primary key and successor UNIQUE: one selected source has at most
two adjacent edges/three source keys. Refuse catalog drift that invalidates this
bound before attempting source discovery; a whole chain is never one unit.

Process terminal provider-work/tasks and current projections before their
parents; remove current before observations; imports before transactions;
replacement edges before either source; observations/tasks/imports before
subscription sources; transactions and subscription sources before purchases;
remove `billing_account_sources` only after all matching verified source rows
are gone. Stopped verification slots and exact benefit rows are independent
account-scoped units. Keep a source key row until all its children are removed,
so it remains the serialization point across batches. Fresh indexed NOT EXISTS
checks, not a stale cached count, establish each stage's postcondition.

Each source-row unit copies zero or one strict evidence fact (up to four bounded
key-selector children if required), obtains an exact transaction-local erase
permit, deletes that one source row and records its disposition atomically.
A large subscription's many observations each consume a unit; a limit-one run
must always make progress without deleting an unbounded history. Set explicit
SQL/context statement budgets and supporting selection indexes. No generic
SQL/table argument, caller JSON patch, client digest authority, session GUC,
trigger disable, `session_replication_role` or cascading account delete is used.
Original and sanitized hashes remain distinct. On failure, no partial copy or
source deletion commits; restart discovers remaining rows and returns the same
retained result for a completed step without duplicating evidence.

**Privileges, catalog and completion.** Extend only the existing private executor
role. Typed read/erase/finish operations (exact signatures frozen during the
implementation plan) receive request IDs, never arbitrary source SQL. Runtime
has no evidence table rights, no erase EXECUTE and no ability to manufacture
private row permits. Existing immutable guards from19/21/22/25/41 gain only a
trigger-internal PrivacyOwner path verifying exact request, transaction/PID,
relation/key and OLD digest; permits cannot survive transaction completion.
Checks cover columns/types/nullability, ordered keys/constraints and incoming
FKs, inheritance/partitioning, RLS, rules, triggers and function properties.
Acquire compatible source catalog locks before the authoritative check and
retain them through mutation to prevent concurrent DDL replacing that contract.
Update affected C1/credential/installation catalog fingerprints and cutover body
pins atomically when new private references change their dependency manifests.
Compile exact minimum source grants in Go and the independent Python provisioner;
no wildcard/table-wide UPDATE merely to accommodate a column grant failure.

A final bounded finish query emits a distinct `billing_raw_removed` receipt only
when every listed ordinary subject billing class and known raw-copy obligation
is gone, C1 terminal evidence is still consistent, and no active attempt remains.
Private evidence count/expiry and unresolved processor outcome are separate
status fields. It neither means financial evidence purged nor whole-account
active removal. Evidence purge uses a separate bounded executor operation at the
retained deadline, removes selector children before parents and proves absence;
backup coverage and restored suppression remain D5/D6 gates. A populated downgrade
refuses without modifying source, evidence, receipts or function bodies.

**Ordered implementation and proof gates (not started).**

- [ ] Freeze post-42 catalog/reader/copy inventory and per-kind evidence schema;
  demonstrate literal source-marker fixtures covering all raw copies and an
  untouched second account; parent reviews exact objects/ACLs before migration.
- [ ] Close late ordinary/legacy writer paths first: reproduce RecordReceipt
  after deletion and enumeration-before-erasure races RED, then verify no new
  row/provider dispatch/value and unchanged active-account behavior.
- [ ] Add private typed copy/erase for terminal tasks/work/slots and observation
  leaves with limit-one restart, fault-between-copy/delete rollback, expiry and
  exact replay; every denied direct runtime/executor/source rewrite stays denied.
- [ ] Add edge/source/purchase/benefit removal in FK order, including multiple
  observations/imports/replacements, unknown legacy receipts, a shared survivor
  fixture and both lock orders against verification, ACK finalization and refund.
- [ ] Add finish/evidence purge and delayed replay after selector expiry; prove
  an old authenticated provider account cannot become a newly registered account,
  no NULL/zero financial fiction, no indefinite hold and no recreated entitlement.
- [ ] Run actual separately connected executor LOGIN journey, missing/extra
  grants, RLS/rule/trigger/FK and concurrent DDL negatives, wrong-request/OLD-digest
  refusal, populated down atomic refusal and empty down/up. Review and cold verify
  each slice, then all touched economy/store tests and the unified repository gate.

Expected files are a later unreserved migration pair, focused economy privacy
cleanup source/tests, narrow ordinary reader/writer fixes, private evidence/guard
helpers, cutover roles/pins/inventory and Python provisioning. No provider SDK,
client purchase flow or deployment toggle change is necessary for this closed
boundary. The principal risks are hidden duplicate payloads, insufficient legacy
ownership, and permanent replay authority accidentally tied to expiring evidence;
the inventory, unknown classification and after-purge proof above are acceptance
gates, not optional follow-up work.

## 11. Proposed D4D: typed shared-source transformations

This is an implementation proposal for coordinator review, following the owner
policy in [Blueprint Profiles §1a](../../BLUEPRINT.md) and the source inventory
and survivor contract in §§3–5. It does not mark D4D or account deletion complete.
The next migration numbers are assigned by the coordinator after C1/migration41;
none are reserved by this proposal. Existing credential, installation, billing
and accepted-work slices remain independently owned.

**Goal:** remove the deleting person's shared personal/content copies while
preserving other players' accepted outcomes, payments and private delivery.
Implement the following boundaries in order, with review and cold verification
before the next boundary. Do not combine them into a generic JSON eraser.

### 11.1 Terminal match representation and survivor authority

The first boundary handles one `(request_id, match_id)` per transaction. It
requires a verified request, matching account fence, confirmed independent
suppression receipt, terminal original match, and terminal base settlement for
every admitted seat. Migration36 erased dispositions satisfy only the deleting
seat's terminal condition; an unfinished survivor settlement must be recovered
through the ordinary accepted-work path first. No privacy operation manufactures
a successful settlement, finishes a live match, or changes its winner/time.

Add an explicit representation version to retained match state, with separate
original contract/outcome source anchors, original database canonical digests,
and current transformed representation digest. Use a narrow, versioned terminal
projection that carries shared match/rule/release identity and surviving player
projections. Remove the subject's `SponsorAccountID`, admission association and
outcome-player statistics from active shared JSON. An erased seat marker carries
only the position needed to interpret survivors; it is not a substitute profile
or a permanent subject pseudonym. It must not contain the subject's account ID,
role, points or old display/attribution data. Minimal request-to-source proof is
private and subject to the retained `evidence_until` deadline.

The source hash formats are different and must remain distinct. Existing
`valueHash(TextMatchRecord/TextOutcome)` hashes the Go source representation;
PostgreSQL `jsonb::text` has a different canonical form. A concrete trusted Go
executor first validates original bytes with the existing typed codecs and
policy normalization. Its typed SQL transform receives only request/match IDs
and expected original source/canonical digests. SQL locks and rechecks those
exact originals, derives the allowed projection itself, and commits the new
digest and transition receipt atomically. It accepts no caller-supplied JSON
patch. This is an attestation by the restricted, reviewed executor of a checked
transition, not an independent SQL recomputation of the Go hash or a cryptographic
membership proof after the original has been erased. Review must fix the exact
codec/version and argument contract before the migration is written.

Under the match lock, freeze a typed before/after witness of each survivor's
account/seat, own outcome fields, award identity/body hash, original server day,
requested/credited amounts, ledger IDs, settlement hash/effects, base outbox
identity/payload/acknowledgement, Premium-at-start eligibility and existing bonus
payment/item/outbox identity. Compare the complete ordered sets, including zero
awards and empty sets. No digest of an incomplete subset is acceptable. The
transition preserves the shared terminal state, original source hash anchors,
policy hash/cap and outcome time. Existing surviving relational receipts and
payloads remain byte-equivalent; only explicit representation metadata changes.

Optional future rewarded claims cannot delay deletion indefinitely. Preserve
the survivor's own verified terminal bonus source without retaining the deleted
player's projection. The migration38 `text_bonus_source_v1` projection uses
original contract/outcome anchors, policy, start eligibility, own applied
settlement and ordered genuine own awards. Prove its canonical bytes remain
identical for every survivor before/after transformation. Adapt ApplyBonus,
reward claim/check/SSV eligibility and their deferred SQL validation to consume
the explicit verified terminal representation. Keep all existing amount,
original-day cap, source-set, Premium/SSV and ledger-linkage checks. A newly
earned survivor bonus after transformation must still produce the ordinary
atomic payment/outbox; the deleted subject remains ineligible. Do not invent an
ad-claim lifetime, force a bonus early, or forfeit another player's benefit.

Update original-source readers before enabling transformation. Prepare, Finish,
Award and Abandon may recognize a byte-exact old request against its retained
original anchor and return the documented already-terminal result; changed
identity/body refuses. They must never decode sanitized JSON as executable
original match state. Recovery and Reconcile distinguish original, transformed,
pending-erasure and invalid-proof records explicitly. Retained survivor payment
replay, private outbox claims/ACKs and newly received valid SSV keep their existing
authority checks. Deleted-person delivery remains suppressed, not acknowledged.

Proposed transaction order is match, required week row, sorted affected account
rows, request, then the exact admission/settlement/award/bonus/outbox rows needed
by the witness. Verify it against every affected current writer, including week
closure and delivery, before implementation. Do not take an account lock first
and then discover/lock a match. The Go budget loops separate one-match
transactions; a limit never silently omits a seat or witness.

#### 11.1a First-boundary schema and executor contract for review

The following is the concrete first-boundary proposal. It needs coordinator
acceptance before migration42 is reserved; migration41 must first be frozen.
It does not authorize the later source, ledger or account-detachment operations.

Extend `text_matches` with `representation_version text NOT NULL DEFAULT
'original_v1'`, `terminal_projection jsonb`, `terminal_projection_sha256 bytea`,
and `original_db_sha256 bytea`. Existing `contract_hash` and `outcome_hash` remain
the original Go source anchors. Original rows require the new nullable fields
to be NULL and retain their original body constraints. Transformed rows require
`representation_version='privacy_terminal_v1'`, SQL NULL `contract` and `outcome`,
non-NULL projection and both 32-byte digests, a supported terminal state, and
non-NULL original anchors. This deliberately changes the relevant NOT NULL/check
constraints instead of inserting an empty counterfeit original body. Existing
state/timestamps/owner/fence and original hashes cannot change in the transform.
Do not backfill historical unknown input as verified evidence.

Add private `privacy_match_transforms` with composite primary key
`(request_id,match_id)`, exact FKs to the retained request and match, and fields
`codec_version text`, `original_contract_sha256 bytea`,
`original_outcome_sha256 bytea`, `original_db_sha256 bytea`,
`prior_projection_sha256 bytea`, `new_projection_sha256 bytea`,
`survivor_witness_sha256 bytea`, `transformed_at timestamptz`. Every digest is
32 bytes; `codec_version='text_privacy_terminal_v1'`; the first transition's
prior digest is the original database digest. Later deletion requests append
their own receipt and chain from the previous projection; an exact same-request
retry returns its historical receipt without transforming a later projection
again. No raw body, deleted-player projection or caller-selected timestamp goes
in this table. Its subject/request linkage remains private finite evidence,
with later D4E detachment designed before its expiry. It is immutable to ordinary
writes and does not count as an `active_removed` receipt.

Use two executor-only routines in one bounded Go transaction:

1. `privacy_read_terminal_match(request_id uuid, match_id uuid) RETURNS jsonb`
   acquires the complete locks, validates request/fence/suppression/terminal
   preconditions and exact catalog, and returns only this match's original or
   transformed source, pinned policy, required anchors and complete typed
   survivor witness. This is a private in-process validation input, never an HTTP
   response, log or persisted export. It creates no side effects.
2. `privacy_transform_terminal_match(request_id uuid, match_id uuid,
   expected_version text, original_contract_sha256 bytea,
   original_outcome_sha256 bytea, original_db_sha256 bytea,
   current_representation_sha256 bytea, survivor_witness_sha256 bytea)
   RETURNS jsonb` independently obtains/rechecks the same locks and source tuple,
   then derives the replacement projection, recomputes the complete survivor
   witness and atomically inserts the receipt plus match transition. Return only
   version/match/digests/receipt timestamp and applied/already-applied status.
   UUIDs, version and digest lengths are exact; missing, changed, unknown or
   unsupported sources refuse. No subset selectors, field paths, content,
   replacement policy, amounts or credited outcomes are input parameters.

The Go adapter calls the read routine, rejects unknown original fields/types,
decodes the original `TextMatchRecord` and `TextOutcome`, checks their existing
`valueHash` anchors and `Policy.SHA256()`, and supplies the observed exact tuple
to the transform within that same transaction. The database digest is SHA-256
of UTF-8 PostgreSQL canonical JSONB for the two-element array `[contract,outcome]`;
the current transformed digest is SHA-256 of UTF-8 `terminal_projection::text`.
Witness canonicalization is also versioned PostgreSQL JSONB, with every set
explicitly ordered and timestamps represented as UTC epoch values. Pin the
source/query functions and test alternate DateStyle/TimeZone sessions. Do not
hash driver-rendered JSON text or locale-dependent timestamp/date strings.

For a second deletion, there are no original bodies left to recompute. Validate
the current immutable match representation, its digest, the unchanged original
anchors and the complete surviving relational witness; remove only the new
deleting seat. An available private receipt whose new projection digest equals
the current digest can corroborate this transition; older retained receipts
remain historical and are not required to equal the latest projection. Absence
of a current receipt after authorized retention expiry is not an error and
never requires reconstructing the deleted request.
The original was validated during the first privileged transition, not
reverified from unavailable bytes. The adapter and SQL reject conflicting
source facts without treating an absent finite audit record as a missing
permanent runtime credential.

Permanent survivor authority is the current `text_matches` representation plus
unchanged genuine survivor source rows, protected by exact schema/immutable
transition guards. It has no FK or read dependency on `privacy_match_transforms`,
`privacy_requests`, an erased account/fence, or a request-linked witness table.
The private transition table is finite audit evidence only. Its later expiry
does not change the current projection, original hash anchors or survivor bonus
source. Thus D4E can delete request-linked receipts/permits and detach the
deleted person's relational lineage without rewriting survivor authority or
resetting any retention deadline. Original digests retained solely for survivor
integrity are labeled historical anchors; this does not assert anonymization or
provide a membership proof from those digests alone. No deleted account, request
ID, old admission ID, receipt pointer or free-text identity is added to the
permanent authority metadata.

The first-boundary proof must simulate authorized removal of expired private
transition/request-linked audit records, then demonstrate survivor read,
original anchored retry, existing payment replay, late valid SSV/payment and a
second participant deletion still work. Runtime must not receive SELECT on the
private audit table to implement these paths. D4E's actual purge routine remains
a separate gate; an owner-only fixture may simulate its final state but must
not claim the purge implementation exists. A same-request privacy executor
retry can return a historical receipt only while that exact finite receipt is
retained; once purged, an unknown request refuses rather than recreating it.

The exact `privacy_terminal_v1` JSON object has `version`, `shared`, `survivors`
and `erased_seats`. `shared` carries protocol/match/room/original-size/mode/rules/
language, pack release/hash, tuning version/hash, entry path, rewards and
leaderboard eligibility booleans, complete non-personal pinned tuning, original
process owner/epoch, prototype flag, terminal kind/winner and UTC occurrence
time. Omit `Contract.Eligibility.AdmissionID` from this shared projection: it
can identify the deleting seat even when the outer `AdmissionIDs` was scrubbed.
Survivors are ordered by seat and carry their exact retained account/admission
IDs and own original result fields; erased seats carry only integer seat
positions. A still-surviving sponsor may be represented by a separate optional
shared sponsor account field, removed on that sponsor's later deletion. No
original authored prompt, release bundle, display name or deleted account ID is
copied. Exact JSON keys, null/omission rules and numeric bounds are frozen as a
golden codec fixture before source implementation.

Prepare replay substitutes only the retained non-personal pinned Policy as the
current `compareTextPreparation` does, hashes the complete incoming original
request and compares to the original contract anchor. Finish hashes the exact
incoming original normalized outcome. Award/Abandon exact old identities use
their genuine receipt or migration36 erasure body anchors; a new event against
transformed terminal state refuses. Retain those subject replay anchors only
within the separately authorized finite lineage window; later removal cannot
turn an unknown replay into success. Ordinary `lockTextMatch` must return an
explicit transformed-state discriminator rather than a zero-valued original
record. Terminal-only bonus readers use a separate verified projection view;
they never call the original game constructor or validate a sanitized contract
as a full original `MatchContract`.

The first boundary leaves relational subject admissions, financial receipts
and private delivery rows for their individually reviewed removal steps. Their
presence is explicitly partial deletion; this operation's postcondition is
removal of the subject projection from shared match JSON, not whole-account
cleanup. Existing survivor bonus source bytes are preserved by projecting the
same original anchors, policy hash and unchanged own start/settlement/award rows.
Keep the migration38/40 validators' full source/item/ledger/outbox checks; change
only their verified source representation access. Test both a preexisting paid
bonus and a previously unpaid survivor receiving valid SSV after transformation.

The exact association scope is therefore: remove subject IDs from the shared
match JSON, including outer sponsorship/admission arrays and nested shared
eligibility admission; preserve all original relational `text_admissions`
rows/FKs, subject settlement/award/disposition/bonus rows and private outbox rows
unchanged in this first boundary. Their account associations remain a disclosed
pending cleanup obligation. No FK is detached, no subject account is physically
deleted and no aggregate deletion status becomes complete here. A later typed
lineage-removal step must remove those associations before active removal; this
plan must not label retained subject rows as subject-free merely because the
new shared projection is subject-free.

Before allowing SQL NULL original bodies, inventory and test every reader.
This includes Prepare/compareTextPreparation/lockTextMatch, Start, Finish,
Award, Abandon, interrupt/cancel/recovery paths, Admin room operations,
SettlePending/settle, reward eligibility/check/SSV, ApplyBonus/RecoverBonuses,
Reconcile and SQL bonus/disposition validators. Week-close queries that filter
started original contracts must explicitly exclude transformed terminal rows
without suppressing accepted pending work. Reconcile must enumerate transformed
survivors through their versioned projection: a NULL `outcome->'Players'` must
not turn a malformed or missing settlement into a zero-row false success.
Unknown versions, original-with-NULL-body, transformed-with-original-body and
missing/mismatched current digest all refuse. Every newly discovered direct
JSON reader joins this acceptance inventory before the migration can land.

Freeze independent golden fixtures at the pre-transform schema: literal original
contract/outcome codec bytes and expected Go digests; literal PostgreSQL
canonical original-pair digest; original `text_bonus_source_v1(... )::text`
and expected SHA-256 captured directly from the existing migration38 function;
and the exact expected transformed JSON plus surviving row snapshots. Store
these as reviewed test constants/artifacts, not values recomputed by the new
projection helper on both sides of an assertion. Cases cover four/six seats,
erased nested admission and sponsor, nonzero/zero/empty awards, preexisting
payment and unpaid survivor, interrupted and low-population results. Compare
the new SQL/Go readers to those independent originals after transformation,
under changed SQL DateStyle/TimeZone, and after finite audit-chain removal.

The source inspection supports this candidate lock order: settlement locks
match then week then account; bonus issuance/payment locks match then account;
private delivery locks account/installation then outbox and does not take match;
week closing commits its week mutation before draining matches in separate
transactions. The transform therefore locks match, existing relevant week,
sorted admitted accounts, request, then exact witness rows. It does not mutate
or need to lock installations, daily buckets, wallets or profiles. It must not
call a delivery callback or take a lobby mutex. Once account locks serialize
claim/ACK and value writers, lock witness rows in fixed relation/primary-key
order and take/recheck catalog locks before relying on their shape. Actual
both-order tests must cover these paths, direct SQL constraint writers and
concurrent DDL before this candidate is accepted as proven deadlock-free.

If an immutable trigger needs an exception, add a dedicated private permit
relation keyed by `(txid,backend_pid,match_id)` with exact request and OLD/NEW
row digests. Only the transform routine may populate it; a trigger-only
PrivacyOwner function consumes it for that exact transition and a deferred
constraint rejects any leftover permit at commit. No Runtime/Executor table
grant or direct trigger EXECUTE is allowed. Enumerate the two callable routines,
trigger-only routine, private relations, exact match-column UPDATE and witness
SELECT/lock privileges in the coordinator's Go/Python cutover maps and real-role
fixture. Do not broaden source UPDATE merely to make LOCK TABLE convenient.

Before implementing, add golden original/terminal/source/witness fixtures and
the read-only reader support first; prove all existing original tests unchanged.
Then add the typed transition and real executor tests in the same gated slice.
This boundary is accepted only after original replay/tamper tests, two sequential
deletions, all survivor byte/source parity, future bonus, no subject value,
rollback, retained downgrade refusal, exact role/catalog checks and observed
both-order lock tests pass independently. Later §11.2/§11.3 work stays pending.

### 11.2 Authored source, release and archive removal

The next boundary consumes migration35 withdrawal and the terminal-consumer
proof above. It handles one discovered source/release component in a bounded
transaction, with an explicit pending result if its complete dependency set
exceeds the reviewed bound. Do not split a required atomic component or hold a
database transaction across filesystem/processor work.

- Erase owned `portal_submissions` and `challenge_entries` text, tags, blob,
  asset reference, personal reasons and copied consent/attribution after exact
  accepted-input/release consumers are withdrawn. Preserve unrelated approved
  contributions; deleting a reviewer removes that actor reference through a
  separate typed transformation, not the author's content.
- Remove owned `text_accepted_inputs.text_content/provenance` and authored
  copies in `text_legacy_archive.source_row`. Resolve retained source/submission/
  entry FKs explicitly. Keep only the necessary digest-only revoked source
  identity, with no content-bearing placeholder or reusable accepted revision.
- Affected `text_releases.bundle` includes Nowns, cards, provenance, manifest
  and replay/certification artifacts. Remove the whole affected artifact after
  no begun consumer still needs it. Preserve an explicit unavailable historical
  release identity/digest for survivor contracts. Never replace bytes under an
  old certified hash/ID, or validate an empty bundle as the old release.
- `text_content_revisions` retain only a revoked identity where required to
  prevent reuse. `text_archive_progress` original rolling/capture hashes remain
  labeled historical anchors; add a typed privacy overlay with exact removed
  source keys and replacement inventory digest. Ordinary archive verification
  must understand this version or refuse, never bless edited rows under the
  original expected hash.
- New playable content is a separately reviewed/certified release. Withdrawal
  and deletion do not auto-publish replacement content. Owned pack/export/cache/
  blob copies produce exact path/key/hash work receipts for D5 and configured
  processor handling; an SQL source receipt alone is not blob deletion proof.

Content transactions are separate from match transformations. Reuse the actual
confirmation/publication release and source serialization order, starting from
the fenced account/request before release/source locks; never acquire a match
lock afterward. Fresh consumer checks must be safe against Start, publication,
archive capture and a second deletion. Prove both lock orders with observed
PostgreSQL waits, not only sequential assertions. A retained original release
needed by a begun match remains pending until ordinary recovery drains it.

### 11.3 Community, consent and shared actor records

Handle each producer family in a separate reviewed typed operation after the
match/content boundary. The manifest must enumerate every released relational,
array and JSON producer, its subject selector, current shape and exact allowed
replacement; unknown variants are pending/error, never a recursive scrub.

- Challenge votes/entries/topics/winner records use explicit erased voter,
  entry or actor representation. Suppress the deleted winner's public title;
  preserve surviving totals and historical payout identities. Do not recrown,
  re-open a week, transfer a title or repeat approval/challenge rewards.
- Remove subject leaderboard/profile/counter rows only after accepted-work
  disposition. Closed survivors keep their original ranks/points/cutoff and
  payouts; open-week views omit the erased subject without rewriting raw
  surviving totals. Public statistics and contributor attribution are not
  retained security evidence.
- Remove `user_terms_acceptances` and contribution consent references by
  default. Retain only necessary version/time for a specifically selected
  financial/reconciliation or substantiated-security case; published policy
  text is unaffected. No blanket consent-history retention.
- Enumerate `admin_operation_decisions.affected_accounts`, operation results,
  leaderboard decisions/results, audits, Guard/report cases, portal role grants
  and applications, and notice actor fields by their actual producer schema.
  Preserve surviving scope, decision identity/order and effects using an erased
  actor reference. Strip subject contact/free text and ordinary reports/chat;
  only substantiated selected evidence enters a minimized private variant.
  Deleting an operator must not invalidate a survivor's legitimate grant or
  refund lineage, and deleting a target must not widen another sanction.

The private evidence writer takes deadlines from the existing request, not a
fresh clock-based retention reset. No original UGC, contact, credentials or
public statistics enter the 180-day class. D4E supplies exact scoped holds and
eventual identity/FK detachment; it cannot extend active content retention or
turn an entire source snapshot into evidence.

### 11.4 Authority, files and acceptance gates

Use a small fixed set of typed PrivacyOwner routines executable only by the
dedicated PrivacyExecutor, with pinned bodies, ordinary Runtime authority
unchanged except exact required proof reads. Enumerate exact table/column ACLs
in both cutover implementations and provisioners. Immutable source exceptions
must authorize only the selected primary key and original row digest within
the same txid/backend PID, consumed in the same transaction; no arbitrary SQL,
caller-controlled table/column name, reusable GUC, blanket trigger disable or
public executor role. Before relying on reads/writes, take appropriate table
locks in the established application order and recheck exact catalog shape,
PK/FK/deferred constraints, RLS, inheritance, rules and expected triggers. Prove
concurrent DDL cannot create a cascade, hide a survivor or falsify completion.

Expected source scope, split by the boundaries above: new typed privacy
transform source/tests in server/internal/store and server/internal/privacy;
narrow readers in text_value.go, text_privacy_value.go, text_settlement.go,
text_delivery.go, text_bonus_payments.go, text_reward_claims.go and
text_bonus_delivery.go; text_release.go/text_archive.go and contribution/
challenge/leaderboard/audit readers only in their own boundary; additive
migration pairs, real restricted-executor fixtures and coordinator-owned
cutover inventories/provisioners. Search before adding files. No client,
economy tuning, production route or generic job framework belongs to this plan.

- [ ] Freeze the original/transformed codecs, exact typed SQL contract and
  before/after witness fixture; prove Go source hash differs from DB canonical
  hash without accepting either as a substitute for the other.
- [ ] Add terminal representation/immutable transition constraints and empty
  down/up preservation; retained transforms refuse downgrade atomically.
- [ ] Teach original retry/recovery/reconciliation readers the explicit version;
  test unchanged retry, changed source, malformed projection and hash tamper.
- [ ] Implement one-match typed transformation with fault rollback and both-order
  settlement/bonus/claim/ACK/delete races. Assert all survivor bytes, exact award
  sets, zero awards, ledger IDs, caps and private payloads stay unchanged.
- [ ] Prove valid late SSV and Premium recovery after transformation, two
  sequential deleted participants, low-population/interrupted/local-sponsored
  matches, deleted sponsor, original owner loss and pending survivor refusal.
- [ ] Implement the bounded content component operation and original archive
  overlay readers; test both source kinds, multi-release reuse, in-flight Start,
  publication/capture races, exact unaffected bundle bytes and restart.
- [ ] Add each community/consent/actor variant with a marked identifying-data
  fixture and exact survivor effect parity; prove unknown variants refuse and
  no winner, grant, correction, refund or leaderboard reward is repeated.
- [ ] Prove restricted-role positives, every missing/extra ACL negative,
  immutable bypass refusal, catalog drift and both-order concurrent DDL. Run
  independent review then cold focused gates for each boundary and the unified
  tests/lints runner before coordinator tracking.

Rejected alternatives are editing JSON while leaving old hashes labeled as
current, retaining full raw snapshots as indefinite proof, and waiting for
every optional future bonus before deletion. They respectively lose integrity,
violate minimization, or defeat the active-data deadline. The typed transition
adds reader/constraint work and requires a precise executor trust boundary; its
benefit is explicit evidence of what was erased and what remained unchanged.
No individual boundary sets `active_removed`: the complete source/processor/
blob inventory, finite evidence/hold handling and restore/admission suppression
must pass D4E/D5/D6 independently before the overall deletion journey opens.

## Finalization boundary — 2026-09-19

The implementation handoff ends at migration 41. Credential and installation
cleanup, survivor settlement and bounded provider-work draining are implemented
as closed, reviewed operations. Full account erasure is not enabled.

The typed match transformation in section 11 and billing evidence/raw erasure
plans remain design only. The unaccepted migration 42, its codecs/readers/fixtures,
and unused C2 evidence preparation were removed from the handoff at the owner's
request to leave no partially implemented slice. Future work must implement and
verify those contracts afresh; historical local experiment logs do not certify
shipped behavior. No migration 43 is included.
