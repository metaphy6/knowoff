#!/usr/bin/env bash
# Volume snapshot/restore drill for the Knowoff Compose stack.
# Rehearses the migration runbook from Infrastructure §2 before any data matters.
# Usage:
#   ./snapshot.sh create <snapshot-dir>   # snapshot running volumes
#   ./snapshot.sh restore <snapshot-dir>  # restore onto fresh volumes
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMPOSE="docker compose -f ${SCRIPT_DIR}/docker-compose.yaml --profile core"

set -a
source "$SCRIPT_DIR/config/redis/environment.env"
source "$SCRIPT_DIR/config/minio/environment.env"
source "$SCRIPT_DIR/config/migrate/environment.env"
set +a

cmd="${1:-}"
dir="${2:-}"

if [[ -z "$cmd" || -z "$dir" ]]; then
  echo "Usage: $0 create|restore <snapshot-dir>"
  exit 1
fi

mkdir -p "$dir"

case "$cmd" in
  create)
    echo "=== Creating snapshots in $dir ==="

    # PostgreSQL: logical dump.
    $COMPOSE exec -T postgres pg_dump -U knowoff -d knowoff -Fc > "$dir/postgres.dump"
    echo "postgres: $(stat -c%s "$dir/postgres.dump") bytes"

    # Redis: background save and copy RDB.
    $COMPOSE exec -T redis redis-cli -a "$KNOWOFF_REDIS_PASSWORD" BGSAVE
    # Wait for BGSAVE to complete.
    while true; do
      status=$($COMPOSE exec -T redis redis-cli -a "$KNOWOFF_REDIS_PASSWORD" LASTSAVE || true)
      bgsave_status=$($COMPOSE exec -T redis redis-cli -a "$KNOWOFF_REDIS_PASSWORD" INFO Persistence | grep rdb_bgsave_in_progress || true)
      if echo "$bgsave_status" | grep -q ":0"; then
        break
      fi
      sleep 0.5
    done
    $COMPOSE cp redis:/data/dump.rdb "$dir/redis.rdb"
    echo "redis: $(stat -c%s "$dir/redis.rdb") bytes"

    # MinIO: mirror buckets (create the bucket first if this is a fresh stack).
    mkdir -p "$dir/minio"
    docker run --rm --network knowoff_default \
      -e MC_HOST_knowoff="http://${MINIO_ROOT_USER}:${MINIO_ROOT_PASSWORD}@minio:9000" \
      minio/mc:latest mb knowoff/knowoff >/dev/null 2>&1 || true
    docker run --rm --network knowoff_default \
      -e MC_HOST_knowoff="http://${MINIO_ROOT_USER}:${MINIO_ROOT_PASSWORD}@minio:9000" \
      minio/mc:latest mirror knowoff/knowoff "$dir/minio" >/dev/null
    echo "minio: $(du -sb "$dir/minio" | cut -f1) bytes"

    echo "=== Snapshot complete ==="
    ;;

  restore)
    echo "=== Restoring snapshots from $dir ==="

    if [[ ! -f "$dir/postgres.dump" || ! -f "$dir/redis.rdb" || ! -d "$dir/minio" ]]; then
      echo "Snapshot incomplete; expected postgres.dump, redis.rdb, and minio/ in $dir"
      exit 1
    fi

    # Stop consumers and remove old volumes.
    $COMPOSE down
    docker volume rm knowoff_pg_data knowoff_redis_data knowoff_minio_data 2>/dev/null || true

    # Start data stores only and wait for Postgres to accept connections.
    $COMPOSE up -d postgres redis minio
    echo "Waiting for Postgres to be ready..."
    for i in {1..30}; do
      if $COMPOSE exec -T postgres pg_isready -U knowoff -d knowoff >/dev/null 2>&1; then
        break
      fi
      sleep 0.5
    done

    # Restore PostgreSQL.
    $COMPOSE exec -T -e PGPASSWORD="$KNOWOFF_DB_PASSWORD" postgres pg_restore -h postgres -U knowoff -d knowoff --clean --if-exists < "$dir/postgres.dump"

    # Restore Redis.
    $COMPOSE stop redis
    docker run --rm -v knowoff_redis_data:/data -v "$(realpath "$dir"):/snapshot" alpine \
      sh -c 'cp /snapshot/redis.rdb /data/dump.rdb'
    $COMPOSE start redis

    # Restore MinIO.
    docker run --rm --network knowoff_default \
      -e MC_HOST_knowoff="http://${MINIO_ROOT_USER}:${MINIO_ROOT_PASSWORD}@minio:9000" \
      minio/mc:latest mb knowoff/knowoff >/dev/null 2>&1 || true
    docker run --rm --network knowoff_default \
      -e MC_HOST_knowoff="http://${MINIO_ROOT_USER}:${MINIO_ROOT_PASSWORD}@minio:9000" \
      -v "$(realpath "$dir"):/snapshot" \
      minio/mc:latest mirror --overwrite /snapshot/minio knowoff/knowoff >/dev/null

    # Bring the full stack back up.
    $COMPOSE up -d

    echo "=== Restore complete ==="
    ;;

  *)
    echo "Unknown command: $cmd"
    echo "Usage: $0 create|restore <snapshot-dir>"
    exit 1
    ;;
esac
