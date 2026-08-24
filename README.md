# Knowoff

> An online social deduction party game for exactly 4 or 6 players: one
> media item — **Nown** — appears on every phone except the Donowers', who
> must pretend they see it. Play a card that relates, argue, vote them out.

**Nowers** (who see Nown) win by voting out every **Donower**; Donowers win
— together — if the votes run out first. Matches run ~5–8 minutes in
**Quick Play** with strangers worldwide (the main product) or in **Local
Rooms** with the people around your table.

📖 The normative spec lives in [`BLUEPRINT.md`](BLUEPRINT.md) — game rules,
tech stack, architecture, economy, and the product baseline. The sequenced
build plan — phases, checkboxes, proof tests — lives in
[`docs/planning/ROADMAP.md`](docs/planning/ROADMAP.md).

## Tech stack

| Layer | Technology |
|---|---|
| Client | Flutter — one codebase: native Android/iOS + Web PWA |
| Game server | Go — authoritative WebSocket server (rooms, roles, dealing, votes, economy) |
| Data | PostgreSQL (durable), Redis (queues/presence), MinIO (media assets) |
| Infra | Docker Compose on a home server behind Cloudflare Tunnel → VPS at launch |

## Status

Pre-implementation. The blueprint and roadmap are written; **Phase 1
(Foundation)** — monorepo tree, config loader, Compose stack, Flutter
scaffold — is next. See the status snapshot in the roadmap chapter of
[`docs/planning/ROADMAP.md`](docs/planning/ROADMAP.md).

## Quickstart

```bash
make roadmap.status  # roadmap checkbox progress
make server.build    # build the Go server
make server.test     # run Go unit tests
make server.lint     # gofmt + go vet
make client.build    # Flutter Android + Web builds
make client.test     # Flutter unit/widget tests
make client.lint     # flutter analyze + dart format check
make compose.up      # start the local Docker Compose stack
make compose.down    # stop the local stack
```

## Repository layout

| Path | Purpose |
|---|---|
| [`BLUEPRINT.md`](BLUEPRINT.md) | The normative spec — game rules, tech stack, architecture, economy, product baseline |
| [`docs/`](docs/README.md) | Roadmap, tracking, design docs, guides |
| [`.agents/skills/`](.agents/skills/README.md) | Skill library for AI coding agents |
| [`xops/`](xops/README.md) | Ops scripts (tracking, safe-run, make dispatchers) |
| [`nginx/`](nginx/README.md) | Local reverse proxy for `*.knowoff.local` (dev convenience, not the public ingress) |
| `client/`, `server/`, `deploy/`, `tools/`, `content/`, `configs/` | The game monorepo — created in Phase 1 per the [spec taxonomy](BLUEPRINT.md) |

## Documentation

- [`AGENTS.md`](AGENTS.md) — rules every AI coding assistant follows in this repo.
- [`BLUEPRINT.md`](BLUEPRINT.md) — the normative spec (game rules, tech stack, architecture, economy, product baseline).
- [`docs/planning/ROADMAP.md`](docs/planning/ROADMAP.md) — the sequenced implementation plan (phases, checkboxes, proof tests).
- [`docs/tracking/README.md`](docs/tracking/README.md) — how the tracking log works.
- [`docs/tracking/context.md`](docs/tracking/context.md) — project context pack.
- [`docs/project/GLOSSARY.md`](docs/project/GLOSSARY.md) — Knowoff terminology (normative).
- [`.agents/skills/README.md`](.agents/skills/README.md) — curated skill library.

## Common commands

```bash
make help            # list available targets
make git.dry         # preview pending commits (read-only)
make git             # commit pending tracking rows + push (human-run)
make track.add ACTION=note SUMMARY="..."
```

## License

See [`LICENSE`](LICENSE).
