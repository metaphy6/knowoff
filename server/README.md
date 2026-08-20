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

```bash
go test ./...
go run ./cmd/knowoffd
```
