# Knowoff

> An online social deduction party game for exactly 4 or 6 players: one
> media item — **Nown** — appears on every phone except the Donowers', who
> must pretend they see it. Play a card that relates, argue, vote them out.

**Nowers** (who see Nown) win by voting out every **Donower**; Donowers win
— together — if the votes run out first. Matches run ~5–8 minutes in
**Quick Play** with strangers worldwide (the main product) or in **Local
Rooms** with the people around your table.

📖 The full normative spec — rules, economy, media engine, network
protocol, infrastructure — lives in [`BLUEPRINT.md`](BLUEPRINT.md).
The sequenced build plan lives in
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
scaffold — is next. See the
[status snapshot](docs/planning/ROADMAP.md#-status-snapshot).

## Quickstart

```bash
make doctor          # sanity-check the agent framework wiring
make roadmap.status  # roadmap checkbox progress
```

Build/test/run commands for the game itself land with Phase 1 and will be
documented here.

## Repository layout

| Path | Purpose |
|---|---|
| [`BLUEPRINT.md`](BLUEPRINT.md) | The normative product + technical spec |
| [`docs/`](docs/README.md) | Roadmap, tracking, design docs, guides |
| [`.agents/skills/`](.agents/skills/README.md) | Skill library for AI coding agents |
| [`xops/`](xops/README.md) | Ops scripts (tracking, safe-run, make dispatchers) |
| `client/`, `server/`, `deploy/`, `tools/`, `content/`, `configs/` | The game monorepo — created in Phase 1 per the [blueprint taxonomy](BLUEPRINT.md) |

## Documentation

- [`AGENTS.md`](AGENTS.md) — rules every AI coding assistant follows in this repo.
- [`docs/planning/ROADMAP.md`](docs/planning/ROADMAP.md) — the plan.
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
