# Knowoff server

Authoritative Go server for five text-only Knowoff modes. Public gameplay uses
WebSocket protocol2; legacy gameplay routes return an explicit upgrade response
before authentication, socket admission or spending.

## Layout

```text
cmd/knowoffd/        # Runtime, migrations, compiled release manifest, internal ops
cmd/cutover-controller/ # Offline physical writer fence and target bootstrap
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

## Offline cutover controller

`cmd/cutover-controller` is a separate operator executable. It has no listener,
runtime-config fallback or automatic role provisioning. The selected database
must satisfy the exact `CutoverControlRoleSpec` capability contract. A controller
holds a dedicated database session and serialization lock for each command.
Use `--help` without credentials to inspect the command syntax.

Requests are bounded JSON on stdin (64 KiB maximum, duplicate/unknown fields
refused; object field names must be ASCII). `--operation` selects `identity`,
`begin`, `seal`, `handoff` or `bootstrap`; `--timeout` defaults to 30 seconds
and must be positive and at most
five minutes. Set `KNOWOFF_CUTOVER_CONTROL_DSN` privately; bootstrap additionally
requires the independently selected `KNOWOFF_CUTOVER_SOURCE_CONTROL_DSN`.
Never place DSNs or lease material in arguments, report files or command logs.

Every request has `roles`, the exact control role specification. Supply only
the matching operation object (`begin`, `seal`, `handoff` or `bootstrap`);
identity needs none. Bootstrap also needs `source_roles`. The typed field
contract is in [`cutover_controller.go`](internal/store/cutover_controller.go),
and the executable fixture in
[`cutover_integration.py`](../xops/test/cutover_integration.py) demonstrates the
complete synthetic protocol. Generate a 32-byte lease in the caller's private
request state; JSON represents it as base64. Reuse the same IDs and lease after
an uncertain result. Receipts expose identity/generation/phase and evidence
hashes, never lease bytes or raw SQL errors.

`WritersEnabled` declares the expected observed role state. It is true for the
source before Begin, false once closing commits and for a target before
bootstrap, then true for an already activated target. After an uncertain Begin,
reconnect expecting false and replay its original request; do not infer that
failure left the source logins enabled.

Begin records closing and disables both declared writer logins before existing
writer sessions are terminated. A command failure or process loss never
automatically reenables source writers. Seal requires actual fenced activity
and durable work checks. Unclassified startup lock holders cause refusal until
their session identity becomes observable; they are never terminated by guesswork.
The physical preflight commits before the retained-data snapshot begins, so a
writer that just finished cannot disappear from both the census and the snapshot.
Initial target activation uses the same ordering. Handoff binds one separately
initialized target cluster;
capture must follow that binding so the restored history contains it. Bootstrap
checks actual target contents and fresh source authority before target activation.
An expired lease cannot authorize a new mutation, but the exact authenticated
committed handoff can still be used for restore/bootstrap recovery. Target
bootstrap replay preserves its ready child after legitimate target value writes.
Source closing is terminal under schema 26; a new target or same-source reopening
requires a separately reviewed additive recovery protocol.

The automated rehearsal uses only owned disposable resources. Production use
still requires a selected release, verified external process drain, durable
callback holding/replay and operator authorization under the
[migration runbook](../docs/launch/VPS_MIGRATION_RUNBOOK.md). Passing synthetic
replay does not supply a live provider queue or a release decision.
