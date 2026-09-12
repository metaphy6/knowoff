# Knowoff server

Authoritative Go game server.

## Layout

```text
cmd/knowoffd/        # main.go
internal/
├── config/          # Layered YAML config loader
├── transport/       # WebSocket handling, codec, connection lifecycle
├── lobby/           # Quick Play queues, room lifecycle, QR/room codes, reconnect grace
├── game/            # Phase state machine, timers, roles, votes, forfeits, scoring
├── bots/            # Quick Play backfill bots (labeled)
├── media/           # Pack loader, relevance mesh, dealing, signed-URL issuing, role-scoped payloads
├── economy/         # Noin wallet, ledger, play passes, entitlements
├── portal/          # Contributor Portal + Media Workbench + Admin Console (server-rendered)
└── store/           # Postgres repositories, Redis queues/presence/routing, object-storage client
migrations/          # Versioned up/down pairs
```

## Commands

Run these commands from `server/`:

```bash
python3 ../xops/test/tests-lints.py --suite go
go run ./cmd/knowoffd
```

The unified runner creates disposable PostgreSQL and Redis services and rejects
skipped integration tests. It uses the project Go development container when
the host has no C compiler; avatar WebP still needs CGO.

## Text-transition contracts and preflight

`pkg/gamecontract` holds the five stable mode IDs and canonical content-language
validation; `internal/transport/v2` holds strict typed fixtures and bounded
sequence/history contracts. These contracts are not wired into the live v1
handlers. `configs/base.yaml` keeps every text mode unavailable until the mode,
content and client gates pass. `config.ValidateTextCutover` checks obsolete key
presence in merged YAML before secret interpolation; it is a preflight, not a
complete config validator or a consumer-retirement approval.

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
when a later additive migration changes table shape. The current interruption
and controlled-down proof uses a disposable design fixture, not production
migrations 000009 onward; generic all-migrations-down remains unsupported for
transition rollback.
