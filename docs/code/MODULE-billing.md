# Module: verified platform billing

## Purpose

Change Noin and Premium only after authenticated platform proof, preserving
account ownership and exactly-once effects through restore, retry and refund.
This is implemented server code with synthetic tests; platform purchase evidence
remains a separate [Phase 5 gate](../planning/ROADMAP.md#phase-5--durable-value-community-and-trust).

## Public surface

| Entry point | Contract |
|---|---|
| `NewPlatformPurchases` | Validates server config and constructs fixed-origin Google/Apple adapters without a startup network call. Disabled platforms stay unavailable. |
| `VerifyReceipt` | Authenticated account plus one provider proof reference; returns immutable purchase `id` and current `status`. |
| `RunProviderTasks(ctx, report)` | Configured bounded polling and durable acknowledgement retries; callback receives stable errors without receipt/credential data. |
| `Entitlements.HasValue` | Exact named theme/style lookup, unioned with preserved legacy benefits. |

Authenticated `POST /api/economy/purchase/receipt` accepts exactly:

```json
{"platform":"google_play","product_id":"<configured-product-id>","raw_receipt":{"purchase_token":"<provider-token>"}}
```

Apple uses `platform: "app_store"` and `raw_receipt: {"transaction_id": "<provider-transaction-id>"}`.
No amount, account, expiry or client completion flag grants value. Unknown or
duplicate fields, ambiguous JSON, oversized requests and outer transaction IDs
are refused. Restore sends the same authenticated request again; a provider
source cannot move between game accounts. Provider receipts must already carry
the game's exact account UUID (`obfuscatedExternalAccountId` or `appAccountToken`).

Responses use `id` and one of `pending`, `granted`, `expired`, `paused`, `on_hold`,
`canceled`, `billing_retry`, `refunded`, `reconciliation_pending`. Stable errors include
`billing.invalid_proof` (400), `billing.conflict` (409), `billing.busy` (429),
`billing.unavailable` (503), and the request-size rejection (413). Responses are
private/no-store and contain no balance. `granted` means the server effect is
committed; acknowledgement may still be retried from durable work.

## Configuration and internals

`configs/base.yaml → billing` defaults both platforms off. A private overlay
provides exact application identities, credentials and provider product maps.
Each product maps to an existing configured Noin bundle or `premium_monthly` /
`premium_yearly`; expiry always comes from platform proof. Google uses an RSA
service-account key and Android Publisher API; Apple uses a P-256 API key and
validates ES256 transaction signatures, pinned Apple roots, certificate purpose
and fresh signed OCSP status. Production rejects sandbox/test-purchase settings.
Credentials remain private configuration, never checked-in fixtures.

Limits configure receipt/response bytes, per-operation HTTP/database timeout, concurrent requests,
poll interval, batch size and maximum subscription entries. Fixed provider origins refuse redirects. Pending
observations have no invented purchase timestamp. Rechecks resume by persisted
`checked_at`; failed observations move behind other eligible work.

Migration 19 retains original receipt rows and Premium benefits, adds immutable
provider/account source identities and named theme/style items, and persists
acknowledgement work. Source → account → receipt locks serialize concurrent
grants. Atomic value writes add one ledger effect. A provider reversal applies
once; if refunded Noin was spent, it records `reconciliation_pending` and keeps
the existing wallet intact. There is no automatic debt or later confiscation.
Legacy manual refund cannot bypass provider authority for new verified receipts.
Controlled down refuses retained new value/identity rows.

Migrations 21–22 preserve receipt anchors and import their existing access without
claiming new provider verification. Immutable source observations feed one current
projection per Apple original subscription or Google token. `verified_at` advances
on every successful ordered verification, including identical evidence; failed
polls only advance scheduling `checked_at`. Apple signed transaction/renewal clocks
also cannot regress. Current non-retired sources and retained legacy benefits
determine both Premium kinds atomically. Canonical observation evidence is capped
at 32 KiB before PostgreSQL JSON encoding; oversized responses never grant access.

Apple polls current subscription status for the exact known original ID and
validates both signed payloads, optional signed group identity and chronology.
Google accepts a configured current item or the documented deferred pair. A paid
replacement retires its already-known predecessor only under the same account,
application and environment; pending/canceled replacement preserves predecessor
access. Immutable edges prevent conflicting successors and retired-token revival.
Source acknowledgement tasks survive restart, use the current product and never
reopen after completion/cancellation. Old Premium tasks are excluded from the
consumable queue. Disabled platforms are excluded before bounded selection;
one provider failure does not stop later acknowledgement work. Separate provider
and persistence deadlines retain retry scheduling after a provider timeout;
parent cancellation stops work and leaves uncertain results retryable. Provider
calls occur outside database/account locks.

Client receipt validation requires its submitted product in the provider's
current response. Only the durable worker may discover a changed configured
product for the exact persisted source: it rechecks source, account, application,
environment and ordering under the transaction, and cannot create a new source.
This also handles a response containing only the new item after rollover without
rewriting the initial receipt. Disabled source polls are filtered before `LIMIT`.

## Tests and remaining gates

`server/internal/economy/purchases*_test.go` covers synthetic Google/Apple proof,
crypto/HTTP refusal, PostgreSQL concurrency, replay, pending/expiry transitions,
refund and worker recovery. `entitlements_items_test.go` proves independent
named benefits and idempotent spend. `handler/economy_billing_test.go` covers
authenticated bounded HTTP input; config tests prove disabled/invalid behavior.
Run the repository test runner with its disposable PostgreSQL/Redis services.

## Native purchase and restore client

The authenticated, no-store `/api/economy/store` response includes `billing`
platforms with exact configured `{product_id,kind,noin}` mappings, plus
`premium_active` and `premium_management_platforms`. Disabled or unconstructed
verifiers expose no offers. Permanent legacy Premium remains active without
inventing a store subscription; management links require a retained account-bound
subscription source. Avatar availability and owned-unlock flags remain separate.

The app-scoped `PurchaseController` listens before native queries and survives
Store route disposal. Its official Flutter adapter uses Play Billing or StoreKit2
only, displays native localized prices, and rejects missing, ambiguous, promotional
or mismatched offers. Canonical account UUIDs bind both native purchase parameters
and exact receipt verification. Google supplies its purchase token, Apple its
numeric transaction ID; neither client payload grants currency or entitlement.
The API caps streamed replies and enforces an absolute response-body deadline.

Google consumables set `autoConsume:false`; server tasks alone acknowledge and
consume. Apple completion occurs after server `granted`, with completion failure
retained for retry. Pending/error work is bounded and coalesced, account changes
never rebind receipts, and background recovery uses saved identity/refresh only.
Native retained transactions can be revalidated after process loss through native
delivery or explicit Restore. Consumed Noin relies on the durable game account,
not a fabricated store restore entry. SDK fakes test this contract; real device
recovery remains a platform gate. Store UI distinguishes payment pending,
verification, retry and durable success, even if a later catalog/wallet read fails.

Google new tokens still need authenticated purchase/restore intake; polling an old
token cannot discover a replacement token. Out-of-app re-signup and provider
notifications remain separate intake work. Apple's server cannot finish the
device's StoreKit transaction. Configured product prices,
real Play/StoreKit purchase/restore/refund evidence, spent-currency remedy policy,
and SSV/UMP consent remain separate work. A synthetic receipt is never launch
approval. [Google validation](https://developer.android.com/google/play/billing/security)
and [Apple verification](https://apple.github.io/app-store-server-library-python/_modules/appstoreserverlibrary/signed_data_verifier.html)
are the primary adapter references.

The standalone `AdMobVerifier` is now implemented with bounded P-256/SHA-256
DER verification, a fixed Google key origin and an expiring/rate-limited key
cache. `rewarded_ads` remains disabled; the verifier is not yet connected to
claims, HTTP reward acceptance or settlement. Its signed fields use only
unreserved ASCII, so Google's URI-decoded reference and original query bytes
agree exactly. Synthetic signatures do not establish real ad/consent readiness.

The obsolete HMAC `/api/economy/ssv` route is retired (404). Its unverified grant
helpers and compatibility methods are removed. The separate AdMob ECDSA verifier
authenticates a bounded proof only; it grants no earnings and has no live claim
endpoint while the durable claim and policy gates remain open. Historical
`store_purchases` receipt/refund records remain readable and retained.
