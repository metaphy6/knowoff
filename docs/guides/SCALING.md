# Scaling concurrent connections

The [Blueprint](../../BLUEPRINT.md) targets five text-only modes. This guide
separates current configured limits from the measurements required before
raising them. Text removes playable image transfers after migration, but adds
mode-specific boards, action history, trade responses and partitioned queues.
Lower asset bandwidth alone does not establish higher match capacity.

## Current configuration baseline

Values below come from checked-in config, not a benchmark or a claim that a
particular host can sustain that load. Verify effective values on the selected
host before making a capacity decision.

| Layer | Knob / source | Configured value |
|---|---|---|
| Server WebSockets | `server.max_connections`, [base.yaml](../../configs/base.yaml) | 20,000; admission ceiling, not proved capacity |
| Container descriptors | `ulimits.nofile`, server/nginx in [Compose](../../infra/compose/docker-compose.yaml) | 65,536 soft/hard |
| nginx descriptors | `worker_rlimit_nofile`, [nginx.conf](../../nginx/nginx.conf) | 65,536 |
| nginx worker connections | `worker_connections` / `worker_processes`, same file | 8,192 / auto |
| Per-IP guard | `limit_conn knowoff_conn`, [API vhost](../../nginx/conf.d/api.knowoff.local.conf) | 100; abuse guard, not total service capacity |
| Container resources | Server / nginx limits in Compose | 8 CPU + 8 GiB / 2 CPU + 1 GiB |
| PostgreSQL | Server `max_open_conns` / Compose `max_connections` | 50 / 200; leave room for jobs/migrations/admin |
| WebSocket frame | `websocket.max_message_bytes` / rate-limit frame limit | 65,536 bytes; validate new history snapshots against this bound |

The 2026-08-25 descriptor changes are historical work recorded in
[tracking.csv](../tracking/tracking.csv), runs `run-20260825103548-878377` and
`run-20260825103635-880084`. Host-local sysctl values and hardware descriptions
from that session are not current portable defaults.

## Verify the selected environment

Use Compose service names instead of generated container names. These are
read-only checks for the currently selected local Compose project:

```bash
pwd
docker compose -f infra/compose/docker-compose.yaml --profile core ps
docker compose -f infra/compose/docker-compose.yaml --profile core exec -T server sh -c 'ulimit -n'
docker compose -f infra/compose/docker-compose.yaml --profile core exec -T nginx sh -c 'ulimit -n'
docker compose -f infra/compose/docker-compose.yaml --profile core exec -T nginx nginx -T
sysctl net.core.somaxconn net.ipv4.ip_local_port_range
```

Resolve production host/project/overlays explicitly before using the same
procedure there. Inspect configuration output locally; do not publish secrets
or private deployment details. A command/configuration result does not replace
a traffic test. No host sysctl, package or daemon change is part of this guide's
documentation update; such changes follow [AGENTS.md](../../AGENTS.md) §4.

## Text-mode capacity proof

The current `tools/gamebot` speaks the older protocol. Update its mode-aware
legal actions and role-scoped observations before using it as a text load
generator. Use a dedicated test environment and accounts with live rewards
and leaderboard credit disabled. Do not flood production or let an outdated
bot's rejected intents masquerade as completed matches.

Define the workload, expected peak, headroom, duration and pass/fail budgets
before the run, then preserve tool/config/content versions and replay inputs.
Exercise all five modes at four and six seats, including:

- Connection/authentication bursts, steady matches, mode/size/language queue
  partitions, explicit queue changes and compatible Local Room starts.
- Maximum legal history, draws, timeout discards, repeated refusals, trade
  response/disconnect races, result transitions and rematch settings/Ready.
- Mobile/PWA reconnect and snapshot bursts, large localized text, slow clients,
  sequence-gap resync and malformed/oversized request rejection.
- PostgreSQL settlement, daily-counter contention, account/profile traffic and
  moderation/Portal work at the same time as play. External screening uses a
  controlled fake or explicitly planned provider test, not accidental paid calls.
- Dependency loss, admission drain and process restart. Verify no fake resumed
  matches, duplicate grants or lingering room/routing state after recovery.

Record p50/p95/p99 action-to-event latency, join/queue latency by compatible
partition, throughput and error counts; server CPU/RSS/GC/goroutines/descriptors;
DB pool waits/query/settlement lag; Redis behavior; snapshot/frame bytes and
retained history per room. Measure client frame time and memory on target
low-end native/PWA devices. Use bounded, secret-free metrics under
[LOGGING.md](LOGGING.md#text-transition-observability-contract).

A single sequential bot process can bottleneck on account creation rather than
the match server. Use enough controlled generators to separate client limits
from server limits and verify they actually complete legal matches. Increase
load in bounded steps; terminate and record the cause at an agreed failure
threshold. Repeat only to test a specific correction or unresolved measurement.

## Tune only the measured bottleneck

A proxied WebSocket normally uses two nginx connections/descriptors, plus
headroom for listeners, upstreams and ordinary requests. Sustainable connections
are bounded by worker connection limits, per-worker descriptor limits, container
limits, host limits and CPU/memory; multiplying all configured numbers is not a
capacity estimate. Keep app admission below the proved safe limit.

Raise container/nginx/app limits together only where measurement supports it.
Account for all running services and host workloads when sizing memory; never
turn an admission limit into an OOM policy. Size PostgreSQL pools for aggregate
query load and leave maintenance headroom. Validate configuration, rerun the
relevant workload and record before/after evidence.

Retire MinIO-prefetch load tests when gameplay storage delivery is removed;
replace them with text snapshot/history/activation workloads. Keep tests for
retained avatar assets, web application delivery and any justified archive
service. Readiness must check required dependencies only, as specified in the
[transition design](../design/DESIGN-text-transition.md).

## Horizontal scaling remains future work

One process owns each live match. Higher configured ceilings do not make that
state portable or provide multi-node recovery. Before introducing replicas,
prove room-to-node affinity for initial join/reconnect, shared compatible queue
coordination, node failure cleanup, per-replica admission/drain and aggregate
PostgreSQL/Redis pool budgets. Redis mapping methods alone are not complete
sticky routing or durable live-game storage.

Keep Kubernetes/Terraform deferred under the [Roadmap](../planning/ROADMAP.md).
Use measured demand to open a separate architecture scope. The current text
transition is complete only with its own single-node capacity and
[recovery rehearsal](../launch/VPS_MIGRATION_RUNBOOK.md) evidence; it does not
claim a verified tens-of-thousands or million-player capacity.
