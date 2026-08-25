# 📈 Scaling concurrent connections

How to raise Knowoff's concurrent-connection ceiling as the user base grows,
and when tuning one box stops being enough. Read this before touching any
of the knobs below — they interact, and raising one without the others is a
no-op (e.g. raising a container's `ulimit -n` does nothing if nginx's
`worker_rlimit_nofile` still caps it lower).

## The knobs, in the order the fix has to happen

Each layer's ceiling is only real if every layer below it is raised too.
From the wire inward:

| Layer | Knob | File | Current value |
|---|---|---|---|
| Host kernel | `net.core.somaxconn` (listen backlog) | `/etc/sysctl.d/99-knowoff.conf` (not in this repo — host-local) | `16384` |
| Host kernel | `net.ipv4.ip_local_port_range` (ephemeral ports for outbound legs) | same | `1024-65535` |
| Container | `ulimits.nofile` (open-file/fd ceiling per container) | [`infra/compose/docker-compose.yaml`](../../infra/compose/docker-compose.yaml) — `server` and `nginx` services | `65536` soft/hard |
| nginx process | `worker_rlimit_nofile` (must match or be ≤ the container ulimit) | [`nginx/nginx.conf`](../../nginx/nginx.conf) | `65536` |
| nginx worker | `worker_connections` (per worker; a proxied WebSocket costs 2 fds — client leg + upstream leg) | same, inside `events {}` | `8192` |
| nginx worker | `worker_processes` | same | `auto` (one per host core) |
| nginx vhost | `limit_conn` / `limit_req` (per-IP abuse guard, **not** a global cap) | [`nginx/conf.d/api.knowoff.local.conf`](../../nginx/conf.d/api.knowoff.local.conf) | `limit_conn knowoff_conn 100;` |
| App | `server.max_connections` (global WS cap; upgrade rejected with 503 above it) | [`configs/base.yaml`](../../configs/base.yaml) | `20000` |
| App | Docker resource limits (`deploy.resources.limits.cpus/memory`) | `docker-compose.yaml`, `server`/`nginx` services | server: 8 CPU/8GB, nginx: 2 CPU/1GB |

Container `ulimits` and nginx's `worker_rlimit_nofile`/`worker_connections`
were raised together on 2026-08-25 after finding Docker's default 1024-fd
container limit — not the app-level `max_connections`, which was already
set generously — was the real bottleneck. See
[`docs/tracking/tracking.csv`](../tracking/tracking.csv) run ids
`run-20260825103548-878377` and `run-20260825103635-880084` for that change.

## Raising the ceiling further on this box

1. **Container fds**: bump `ulimits.nofile.soft`/`.hard` in `docker-compose.yaml`
   for `server` and `nginx`. Keep both numbers equal; Docker won't raise the
   hard limit above the host's own `ulimit -n` (check with `ulimit -Hn` on
   the host first).
2. **nginx**: raise `worker_rlimit_nofile` to match, and raise
   `worker_connections` so `worker_processes × worker_connections × (fds per
   connection)` doesn't self-limit below the container ceiling. Leave
   `worker_processes auto` — more event-driven workers cost little idle CPU
   and use the container's CPU quota more evenly than pinning a low count.
3. **App**: raise `server.max_connections` in `configs/base.yaml`, leaving
   headroom under the container fd ceiling for the DB pool, Redis pool, and
   nginx's own upstream-leg fds (don't set it equal to `ulimits.nofile`).
4. **Docker resource limits**: give `server`/`nginx` more CPU/memory in
   `docker-compose.yaml` if `docker stats` shows either approaching its
   limit under load — memory scales roughly linearly with steady open
   connections (a few KB each) plus active-match state.
5. **Host kernel**: only needed once you're pushing thousands of
   simultaneous new connections per second — raise
   `net.core.somaxconn` and widen `net.ipv4.ip_local_port_range` via a file
   under `/etc/sysctl.d/`, then `sudo sysctl --system`. This is a host-level
   change outside the repo — per [`AGENTS.md`](../../AGENTS.md) §4, always
   get explicit user confirmation before applying it, and record it as a
   tracking `note` row (the host file itself isn't version-controlled).

### Verifying a change actually took effect

```bash
# container fd ceiling
docker exec knowoff-server-1 sh -c 'ulimit -n'
docker exec knowoff-nginx-1 sh -c 'ulimit -n'

# nginx picked up the new directives
docker exec knowoff-nginx-1 nginx -T 2>&1 | grep -E 'worker_rlimit_nofile|worker_connections'

# host kernel values
sysctl net.core.somaxconn net.ipv4.ip_local_port_range
```

Then load-test past the *old* ceiling to prove the fix, not just past the
new one. `tools/gamebot` connects sequentially within one process (each bot
does a real device-auth round trip, which is intentionally
compute/latency-costly — see 🛡️ auth in `BLUEPRINT.md`), so a single
gamebot process is a poor concurrency generator. Launch several in
parallel instead:

```bash
for i in $(seq 1 12); do
  (timeout -k 3 45 /tmp/gamebot -server ws://localhost:8080/ws -queue 6 -count 150 > /tmp/gb_$i.log 2>&1 &)
done
sleep 25
PID=$(docker exec knowoff-server-1 sh -c "ps aux | grep '[k]nowoffd' | awk '{print \$1}'")
docker exec knowoff-server-1 sh -c "ls /proc/$PID/fd | wc -l"   # should exceed the old ceiling
docker logs --since 60s knowoff-server-1 | grep -ciE 'too many open files|panic'  # must be 0
```

## When tuning this box stops being enough

Every knob above is **vertical** — it makes one box handle more. That has a
hard ceiling: one process, one set of CPU cores, one NIC. Rough guidance:

- **Tens of thousands of concurrent connections on capable hardware
  (16 cores / 30GB, this box's class)**: vertical tuning as above is
  sufficient. `server.max_connections` in the tens of thousands is realistic.
- **Hundreds of thousands to millions of concurrent connections**: requires
  **horizontal scaling** — this is an architecture change, not a config
  change, and is not solved by any knob in this file.

## What horizontal scaling requires (not yet built)

Per [`BLUEPRINT.md` § Infrastructure & Deployment](../../BLUEPRINT.md) item
5 and [ADR-001](../design/ADR-001-server-authoritative-over-p2p.md), the
design already anticipates this:

- **Multiple `server` replicas** behind a load balancer/ingress instead of
  one nginx → one server container.
- **Room→node affinity with sticky routing**: a live match's authoritative
  state stays in one node's memory (ADR-001) — the ingress must route every
  WebSocket for a given room to the same replica (consistent hashing on
  room id, or a lookup table in Redis), not round-robin per connection.
- **Shared coordination state in Redis** (already the plan for
  matchmaking/session coordination) so any replica can look up which node
  owns a room, instead of every replica needing full in-memory state.
- **Postgres/Redis connection pool sizing per replica**: pools bound query
  *throughput*, not WebSocket connection count — multiply per-replica pool
  size by replica count when sizing Postgres's own `max_connections`.
- **Kubernetes readiness already designed for**: `/healthz`/`/readyz`
  (Postgres/Redis/storage checks), Prometheus metrics on a separate port,
  graceful SIGTERM draining of in-flight matches — see `BLUEPRINT.md` item
  5 for the full list.

This is roadmap-phase-sized work, not a config tweak — plan it with
`/plan` or the [`planner`](../../.github/agents/planner.agent.md) agent
rather than improvising knob changes when the numbers above are actually
being approached.
