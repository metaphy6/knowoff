# ADR-003: Home-server-first behind Cloudflare

## Status

Accepted

## Context

Pre-launch hosting strategy must balance cost, security, and migration path. Options:

1. Cloud VPS from day one.
2. Home server behind a tunnel/CDN, moving to a VPS at public launch.

## Decision

Development and beta run on the owner's home machine under Docker Compose, fronted by Cloudflare Tunnel (`cloudflared`) and Cloudflare CDN. Public launch moves the same container images to a small VPS; assets may offload to Cloudflare R2.

## Consequences

- **≈ $0 infrastructure cost pre-launch.** No open ports, no exposed home IP, TLS at the edge.
- **Media traffic is absorbed by Cloudflare's edge cache.** Assets are content-hashed and immutable; a Cache-Everything rule with a long TTL means the home uplink serves mostly API/WebSocket traffic after first fetch.
- **Low/medium media quality is non-negotiable.** Small assets are what make home-server caching and fast prefetch viable.
- **Home ISP uptime is accepted as service uptime for beta.** The public launch runbook lifts the same Compose stack to a VPS.
- **Volumes are migration-portable from day one.** `pg_data`, `redis_data`, and `minio_data` are named Docker volumes; migration is snapshot/restore, not a schema rewrite.
