# Knowoff

> An online social deduction party game for exactly 4 or 6 players: one
> media item — **Nown** — appears on every phone except the Donowers', who
> must pretend they see it. Play a card that relates, argue, vote them out.

**Nowers** (who see Nown) win by voting out every **Donower**; Donowers win
— together — if the votes run out first. Matches run ~5–8 minutes in
**Quick Play** with strangers worldwide (the main product) or in **Local
Rooms** with the people around your table. Role secrecy is enforced
server-side: a Donower's device is never sent Nown at all, so there's
nothing to leak even if you inspect the traffic yourself.

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

All 6 roadmap phases are built and passing their proof tests —
foundation, media engine, realtime game loop, accounts & Quick Play, the
Noin economy & Admin Console, and the Contributor Portal. `make
up` gets you a full 6-player match with zero cloud dependencies.
See the phase-by-phase snapshot in
[`docs/planning/ROADMAP.md`](docs/planning/ROADMAP.md).

## Quickstart

```bash
make server.build    # build the Go server
make server.rebuild  # stop, rebuild, and recreate the server container
python3 xops/test/tests-lints.py  # all tests and lint checks
make up              # start the local Docker Compose stack
make down            # stop the local stack
make web.rebuild     # force-rebuild + recreate the client-web dev container, cache-bust the browser
make web.stop        # stop locally owned Flutter web servers on any port
```

For roadmap checkbox progress, run:

```bash
python3 xops/makefile/roadmap_ops.py status
```

## Local stack

`make up` brings up server + postgres + redis + minio + adminer +
client-web, fronted by a local **nginx** reverse proxy that terminates TLS
and publishes friendly `*.knowoff.local` names (dev convenience only — the
public ingress is a Cloudflare Tunnel, see [`nginx/README.md`](nginx/README.md)).
Run `make localhostfile.add` once to resolve those names, then:

| URL | What |
|---|---|
| `https://app.knowoff.local`, `http://0.0.0.0:8000/` (flutter CLI debug and integrated browser access) | Flutter web client |
| `https://api.knowoff.local` | Game server — REST + `/ws` WebSocket |
| `https://admin.knowoff.local` | Admin Console / Contributor Portal (dev-only, never expose this) |
| `https://adminer.knowoff.local` | Postgres browser |
| `https://minio.knowoff.local` | MinIO console |

Prefer to skip nginx? Every service also publishes straight to
`127.0.0.1`: server `8080`/`9090`/`9091`, postgres `5432`, redis `6379`,
minio `9000`/`9001`, adminer `8081`. See [`infra/README.md`](infra/README.md)
for Compose profiles, volume snapshots, and per-service config.

## Repository layout

| Path | Purpose |
|---|---|
| [`BLUEPRINT.md`](BLUEPRINT.md) | The normative spec — game rules, tech stack, architecture, economy, product baseline |
| [`docs/`](docs/README.md) | Roadmap, tracking, design docs, guides |
| [`.agents/skills/`](.agents/skills/README.md) | Skill library for AI coding agents |
| [`xops/`](xops/README.md) | Ops scripts (tracking, safe-run, make dispatchers) |
| [`nginx/`](nginx/README.md) | Local reverse proxy for `*.knowoff.local` (dev convenience, not the public ingress) |
| `client/`, `server/`, `infra/`, `tools/`, `content/`, `configs/` | The game monorepo — created in Phase 1 per the [spec taxonomy](BLUEPRINT.md) |

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
make web.stop        # stop locally owned Flutter web servers on any port
make git.dry         # preview pending commits (read-only)
make git             # commit pending tracking rows + push (human-run)
make track.add ACTION=note SUMMARY="..."
```

## License

See [`LICENSE`](LICENSE).
