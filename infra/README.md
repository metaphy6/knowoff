# Deployment artifacts

```text
infra/
├── compose/    # Docker Compose local stack
├── k8s/        # Future Kubernetes manifests (scaffolded, not required to run)
└── terraform/  # Future Terraform modules (provider-agnostic)
```

The local reverse proxy lives at [`nginx/`](../nginx/README.md) (repo root,
alongside `client/` and `server/`), not under `infra/` — it's built and
versioned like the other service images, not a deployment artifact.

## Local stack

```bash
cd infra/compose
docker compose --profile core up --build
```

Opening this workspace in VS Code also runs this automatically (the
"Compose: Up (core)" task has `runOn: folderOpen` — see
[`.vscode/tasks.json`](../.vscode/tasks.json); port 443 is configured to
auto-open in VS Code's browser preview once nginx is reachable (see
[`.vscode/settings.json`](../.vscode/settings.json)).

Then resolve the local domains once (see [`nginx/README.md`](../nginx/README.md)):

```bash
make localhostfile.add   # adds *.knowoff.local -> 127.0.0.1 to your hosts file
```

The checked-in `client-web` service is currently commented out; start a client
separately or deliberately enable that service before using the app proxy.
The browser will warn about the self-signed certificate on first visit — see
[`nginx/README.md`](../nginx/README.md#tls-certificates).

Seed a dev-only Admin Console login (`https://admin.knowoff.local`):

```bash
cd infra/compose
docker compose --profile tools run --rm --build seed-admin
```

Local service configuration lives in `compose/config/<service>/environment.env`.
The files contain the complete development environment, including the visible
dev secrets used by the stack — see
[`docs/guides/DEV_CREDENTIALS.md`](../../docs/guides/DEV_CREDENTIALS.md) for
the full list. The Cloudflared service is currently commented out; the `edge` profile does not
start a tunnel. Production secrets must come from a deployment secret store.

Profiles:

- `core` — server, postgres, redis, migrate, adminer, nginx
- `tools` — server/stores/migrate plus explicitly invoked seed-admin
- `test` — server/stores/migrate; test commands run through the unified runner
- `edge` — current server/stores/migrate/adminer/nginx; tunnel remains disabled

Active startup has no MinIO service, storage/CDN credentials, old playable pack
or ingest mounts, object-store readiness edge, or console vhost. Existing object
volumes are not deleted by this configuration change. Preserve any retained
archive until its explicit retention decision; do not run `down -v` as cleanup.
The separate restore fixture below deliberately retains historical MinIO tooling.
Avatars remain PostgreSQL-backed WebP; the native image codec/build dependencies
remain required. Text releases are private authenticated server payloads.

Infrastructure verification (from the repository root):

```bash
python3 -m unittest discover -s xops/test -p test_infra.py -v
```

This renders the actual core Compose model without printing expanded secrets,
checks retired paths, builds the actual nginx image, runs `nginx -t`, and tests
TLS OAuth callback success/upstream failure with synthetic query/Referer
sentinels. It creates only labelled temporary proxy/upstream containers on an
unpublished internal network, then removes them. It does not start ordinary
Compose services or modify host files. The unified Python gate also runs it.

## Isolated backup and restore rehearsal

`snapshot.sh` now delegates to the guarded Python tool. Production capture is
closed until an authenticated all-writer barrier exists. An operator's assertion
that writers stopped, or `matches_drained=true` from the runtime, is insufficient.
The tool accepts only receipts for nonce-labelled, tool-created local fixtures;
it accepts no remote DSN, ordinary Compose volume, or occupied restore target.
The owner has confirmed that no deployment currently exists.

Run the complete rehearsal from the repository root:

```bash
python3 xops/test/snapshot_integration.py --output /tmp/agent-runs/knowoff-restore-rehearsal
```

The output must be new. The driver prepares missing digest-pinned Docker images,
creates an internal unpublished network and separate PostgreSQL, Redis and MinIO
volumes, applies the exact historical migration 8, and seeds synthetic retained
data. It restores that backup, applies every currently present paired migration
in order, captures the current schema, and proves two independent current restores.
It also proves refusal of a foreign network peer, another PostgreSQL client and
a partial network seal. It removes only its own containers, volumes and networks
in bounded cleanup; private artifacts and `results.json` remain for inspection.
The default `python3 xops/test/tests-lints.py` includes the same real rehearsal
and never silently skips it if Docker or an image is unavailable.

The driver preserves accounts, wallets/ledger, legacy receipts and entitlements,
inline custom-avatar/source bytes, a legacy archive row, synthetic terminal
receipt/outbox rows, named entitlement rows, PostgreSQL large objects, non-public
schemas, database owner/comment/ACL/settings, role membership and schema ACLs,
functions/triggers/RLS, sequences, all table
row hashes, Redis values and absolute expiries, MinIO current object metadata
and previous version bytes, and retained files. Terminal SQL fixture rows exercise
storage preservation only; they do not claim a completed production match.
Exact checked-in migration files, config templates and the synthetic text pack
are included. Production `.env` values and role passwords are not captured.

A capture first checks immutable Docker resource IDs, nonce/owner labels, image
digests, volume ownership and mount/network isolation. It rejects another SQL
client, makes the source database read-only and disconnects all fixture network
interfaces. It captures PostgreSQL and global roles, saves Redis synchronously,
stops Redis and MinIO, and copies their retained data. Before/after database and
file hashes must agree. This is a fixture-only writer fence; no app is started.
The stopped MinIO volume archive retains object versions and metadata that a
simple bucket mirror would omit. The old MinIO image matches the historical
format only and is not approved for production deployment.

Every artifact is private (files `0600`, output root `0700`). The manifest is
written last, and its SHA256 must be supplied separately when verifying or
restoring. Keep that pin outside the backup directory. Bounded commands enforce
a live 64 MiB combined stdout/stderr ceiling, 30-second default command limit
and 20-minute fixture budget; image preparation and cleanup have separate bounded
budgets. File and archive validation rejects symlinks, traversal, duplicates,
oversized entries and more than 10,000 entries. Tablespaces outside the standard
cluster, foreign servers and active subscriptions are explicitly unsupported.
Cancellation kills and reaps the command process group; it cannot contain a
malicious descendant that creates a new operating-system session.

After the rehearsal, `results.json` contains the separate pins. A retained backup
can be checked and restored again into a newly allocated isolated fixture:

```bash
./infra/compose/snapshot.sh verify /tmp/agent-runs/knowoff-restore-rehearsal/current-backup --sha256 <PIN>
./infra/compose/snapshot.sh restore /tmp/agent-runs/knowoff-restore-rehearsal/current-backup --sha256 <PIN> --output /tmp/agent-runs/another-restore --dry-run
./infra/compose/snapshot.sh restore /tmp/agent-runs/knowoff-restore-rehearsal/current-backup --sha256 <PIN> --output /tmp/agent-runs/another-restore
./infra/compose/snapshot.sh create /tmp/agent-runs/recaptured-fixture --source /tmp/agent-runs/another-restore/fixture.json --dry-run
./infra/compose/snapshot.sh create /tmp/agent-runs/recaptured-fixture --source /tmp/agent-runs/another-restore/fixture.json
./infra/compose/snapshot.sh cleanup /tmp/agent-runs/another-restore/fixture.json --dry-run
./infra/compose/snapshot.sh cleanup /tmp/agent-runs/another-restore/fixture.json
```

Restore creates the absent database inside a newly allocated cluster, preserving
database metadata without `--clean`, dropping an existing database, or reusing a
target. Only the source read-only capture setting is removed from the restored
fixture. A successful restore verifies catalog/roles/ACL/data/sequence parity before
writing `restore.json`; `admission_open` remains false. It starts only the three
isolated stores. Cleanup revalidates exact resource ownership even if PostgreSQL
has stopped. It does not remove evidence directories or ordinary Compose data.
If cleanup fails, retain the private receipt and inspect those exact resources;
never substitute broad volume deletion or the former restore procedure.

A failed capture retains `INCOMPLETE` and cannot be verified or restored. A failed
restore leaves no valid `restore.json` and attempts bounded owned-resource cleanup.
Do not overwrite partial evidence, turn a dirty migration clean, or resume a
partially sealed source. Fix the cause and create a fresh fixture/rehearsal.
The reversible route is another isolated restore plus a forward migration fix;
production cutover, legacy consumer retirement and reopening admission remain
separate reviewed operations.
