# VPS Migration Runbook

This runbook moves the home-server Docker Compose stack to a small public VPS
with verified data parity. The same container images and config model run on
both hosts; only the host, DNS, and tunnel token change.

## Prerequisites

- Target VPS with Docker and Docker Compose installed.
- SSH access and sudo on the target.
- Cloudflare DNS ready for `play.<domain>` and `cdn.<domain>`.
- New `cloudflared` tunnel token for the VPS.
- `${ENV_FILE}` containing production secrets on the source host.

## 1. Stop writes

1. Put the source server in maintenance drain:
   - Schedule a maintenance notice in the Admin Console (🎮 §4).
   - Wait for running matches to end; matchmaking already stops admitting new
     rooms when the notice is active.
2. Verify no active rooms:
   ```bash
   docker exec -i knowoff-redis redis-cli PUBLISH knowoff:drain_check ping
   ```
3. Stop the public `knowoff-server` container but keep Postgres, Redis, and
   MinIO running.

## 2. Snapshot PostgreSQL

On the source host:
```bash
mkdir -p /tmp/knowoff-migrate
docker exec -i knowoff-postgres pg_dump -U knowoff -Fc knowoff \
  > /tmp/knowoff-migrate/knowoff.pgdump
```

Record the row count checksum:
```bash
docker exec -i knowoff-postgres psql -U knowoff -t -c \
  "SELECT count(*) FROM accounts;" > /tmp/knowoff-migrate/accounts.count
```
Repeat for `profiles`, `noin_wallets`, `noin_ledger`, `entitlements`,
`store_purchases`, `leaderboard_entries`, `media_packs`, `system_notices`,
`reports`, `feedback`, `admin_accounts`, `admin_audit`.

## 3. Snapshot Redis

```bash
docker exec -i knowoff-redis redis-cli BGSAVE
# Wait for the RDB to settle.
docker cp knowoff-redis:/data/dump.rdb /tmp/knowoff-migrate/redis.rdb
```

## 4. Mirror MinIO assets

```bash
docker run --rm --network knowoff_default -v /tmp/knowoff-migrate:/out \
  minio/mc alias set local http://knowoff-minio:9000 ${MINIO_USER} ${MINIO_PASS} && \
  mc mirror local/knowoff-assets /out/assets
```

Generate checksums:
```bash
find /tmp/knowoff-migrate/assets -type f | sort | xargs sha256sum \
  > /tmp/knowoff-migrate/assets.sha256
```

## 5. Transfer to target

```bash
rsync -avz --progress /tmp/knowoff-migrate/ vps:/tmp/knowoff-migrate/
```

## 6. Restore on target

1. Start Postgres, Redis, and MinIO on the target using the Compose file with
   the `core` profile.
2. Restore PostgreSQL:
   ```bash
   docker exec -i knowoff-postgres pg_restore -U knowoff -d knowoff --clean \
     < /tmp/knowoff-migrate/knowoff.pgdump
   ```
3. Restore Redis RDB by copying it into the Redis data volume before startup
   or using `redis-cli --rdb` after startup.
4. Mirror assets into MinIO:
   ```bash
   docker run --rm --network knowoff_default -v /tmp/knowoff-migrate:/src \
     minio/mc alias set local http://knowoff-minio:9000 ${MINIO_USER} ${MINIO_PASS} && \
     mc mirror /src/assets local/knowoff-assets
   ```

## 7. Verify parity

Run the same row-count queries on the target and diff against the source
`.count` files. Diff `assets.sha256` against the target asset tree.

Example parity script:
```bash
#!/bin/bash
set -e
for table in accounts profiles noin_wallets noin_ledger entitlements \
             store_purchases leaderboard_entries media_packs system_notices \
             reports feedback admin_accounts admin_audit; do
  src=$(cat /tmp/knowoff-migrate/${table}.count)
  dst=$(docker exec -i knowoff-postgres psql -U knowoff -t -c "SELECT count(*) FROM ${table};")
  if [ "$src" != "$dst" ]; then
    echo "MISMATCH: $table src=$src dst=$dst"
    exit 1
  fi
  echo "OK: $table = $src"
done
cd /tmp/knowoff-migrate && sha256sum -c assets.sha256
```

## 8. Cut over

1. Update Cloudflare DNS / tunnel to point at the VPS.
2. Start the full Compose stack on the target, including `cloudflared` under
   the `edge` profile.
3. Verify `/healthz` and `/readyz` return 200 through the public hostname.
4. Smoke test: one Quick Play 4-player match with one native client and three
   gamebot seats.
5. Bring the source stack down only after the smoke test passes and DNS TTLs
   have expired.

## Rollback

If anything fails during cutover:
1. Point DNS back to the source host.
2. Start the source stack.
3. The source database is still consistent because writes were stopped before
   snapshot.

## Cloudflare R2 offloading

If the home MinIO host retires completely, migrate assets to Cloudflare R2:
```bash
rclone sync /tmp/knowoff-migrate/assets r2:knowoff-assets
```
Update `configs/prod.yaml` → `storage.assets_url` to the R2 public/custom
 domain and redeploy.
