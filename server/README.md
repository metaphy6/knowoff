# Knowoff server

Authoritative Go server for five text-only Knowoff modes. Public gameplay uses
WebSocket protocol2; legacy gameplay routes return an explicit upgrade response
before authentication, socket admission or spending.

## Layout

```text
cmd/knowoffd/        # Runtime, migrations, compiled release manifest, internal ops
internal/config/    # Strict layered text config and retired-key rejection
internal/handler/   # Authenticated HTTP/WS boundaries and private delivery
internal/transport/ # Readiness, middleware and strict v2 wire contracts
internal/lobby/     # Mode/size/language queues, rooms, ownership and drain
internal/game/      # Authoritative five-mode actions, roles, history and scoring
internal/economy/   # Wallet, passes, named entitlements and provider verification
internal/portal/    # Text contributions, screening and weekly challenge
internal/admin/     # Session/CSRF authority, atomic audit and operations
internal/store/     # PostgreSQL persistence, releases, settlement and recovery
internal/avatar/    # Retained non-playable PostgreSQL blobs and WebP codec
pkg/media/          # Shared text bundle validation/dealing boundary
migrations/         # Immutable paired SQL and compiled identity manifest
```

Old association/image implementation files still present in these packages are
tracked retirement work; they are not an alternate playable runtime.

## Verification and configuration

From the repository root:

```bash
python3 xops/test/tests-lints.py
```

The runner creates disposable PostgreSQL/Redis services, runs integration tests
and rejects silent skips. Its Go development container supplies the native
compiler required by retained avatar WebP. No host OS installation is needed.

Runtime uses `configs/base.yaml`, `configs/gameplay/tuning.yaml` and the overlay
selected by `KNOWOFF_CONFIG` (default `configs/local.yaml`). Core secrets are
`KNOWOFF_DB_PASSWORD`, `KNOWOFF_REDIS_PASSWORD` and `KNOWOFF_JWT_KEY`; default
startup requires no object store or cloud provider key. Optional OAuth, billing
and screening providers require an explicit private overlay and verified setup.
Do not print secret-expanded configuration. All intended gameplay cells default
to unavailable until their immutable release and product gates are satisfied.

The production image is built from `server/Dockerfile`; mount configuration at
`/app/configs` and provide the selected PostgreSQL/Redis access privately. Run
`/knowoffd release-manifest` inside the built image to inspect protocol/migration
identity without configuration, credentials or network access. Normal startup
requires the exact clean compiled schema head; it never migrates implicitly.
Run the separate `migrate` command only against the explicitly selected database
under the rehearsed migration procedure.

## Text-transition contracts and preflight

`pkg/gamecontract` holds the five stable mode IDs and canonical content-language
validation; `internal/transport/v2` holds strict typed fixtures and bounded
sequence/history contracts. The runtime wires these contracts through `/ws/v2`. `configs/base.yaml` keeps every text mode unavailable until the mode,
content and client gates pass. `config.ValidateTextCutover` checks obsolete key
presence in merged YAML before secret interpolation and is invoked by `Load`.
Retired storage/media/bots/HMAC-ad and specialty/backfill settings are rejected
by presence, including empty values. Other unknown keys and invalid core values
remain errors; a validator does not authorize data deletion.

To fingerprint an explicitly selected database with SELECT privileges, set
`KNOWOFF_PREFLIGHT_DSN` through the operator's private environment and run from
`server/`:

```bash
go run ./cmd/transition-preflight --timeout=30s --migrations-dir=migrations
```

`--timeout` bounds database observation; local SQL-file hashing runs first.
The version-2 report wraps the deterministic database snapshot with an observation
timestamp, local migration-file hashes and explicit coverage labels. Its bounded,
read-only repeatable-read transaction includes fixed-category content/status and
entitlement counts, exact aggregate wallet/ledger totals and per-table fingerprints.
Unknown stored labels count as `other`; missing legacy schema is `unavailable_schema`.
The command rejects ambiguous migration state or policy-filtered rows and never
migrates, repairs dirty state or bypasses row-level permissions.
Keep this aggregate metadata as private operational evidence.

Local SQL hashes do not prove which bytes were previously deployed. Client/image
versions, content releases, active in-memory matches, object consumers and paid
benefit equivalence stay `not_observed`; collect those separately when a deployment
exists. The owner stated that no deployment exists on 2026-09-12; that attestation
belongs to the continuation report, not a hardcoded assumption in this command.
Fingerprints include all columns, so compare retained projections deliberately
when a later additive migration changes table shape. Actual additive migrations and interrupted owner/settlement recovery now have
disposable PostgreSQL proofs. The backup rehearsal records its exact captured
head and retained-row parity separately; generic all-migrations-down remains
unsupported for transition rollback.
