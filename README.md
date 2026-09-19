# Knowoff

> A text-based social deduction party game for exactly 4 or 6 players.
> Nowers know the secret context; Donowers infer it from the table and bluff.
> Play, argue and vote before the Knowoff budget runs out.

The target app has five selectable modes: **Missed the Briefing**, **Secret
Scale**, **Make Room**, **Bad Bargains** and **Top That**. Quick Play and Local
Rooms assemble the table; they are separate from gameplay mode and content
language. Missed the Briefing is the initial default, with each mode exposed
only after its release checks pass.

**Current priority: private playtesting (2026-09-19).** The real server has a
private synthetic prototype for all five modes and both table sizes, with no
earned currency or account progression. Normal Flutter clients can join it.
Use the [private playtest guide](docs/guides/PLAYTEST.md) for isolated startup,
separate player sessions, resets and Web/Android builds, and the
[test checklist and findings](docs/reports/2026-09-19-private-playtest.md) for
observed journey evidence and remaining limitations.

Production mode availability stays closed. Billing/provider acceptance,
complete deletion, certified content and public deployment remain unfinished;
Phase 6 final cutover still requires its existing dependencies. Bounded cleanup
and Phase 4 playtest setup proceed under the [existing roadmap](docs/planning/ROADMAP.md#current-priority--private-playtesting-2026-09-19).
[ADR-012](docs/design/ADR-012-text-only-selectable-modes.md) records the text pivot;
the [transition design](docs/design/DESIGN-text-transition.md) preserves migration
and retirement contracts. Earlier technical validation remains in the
[resumption record](docs/reports/2026-09-19-text-transition-resumption.md).

📖 The normative spec lives in [`BLUEPRINT.md`](BLUEPRINT.md) — game rules,
tech stack, architecture, economy, and the product baseline. The sequenced
build plan — phases, checkboxes, proof tests — lives in
[`docs/planning/ROADMAP.md`](docs/planning/ROADMAP.md).

## Tech stack

| Layer | Technology |
|---|---|
| Client | Flutter — one codebase: native Android/iOS + Web PWA |
| Game server | Go — authoritative WebSocket server (rooms, roles, dealing, votes, economy) |
| Data | PostgreSQL and Redis; historical object archives have a separate restore path |
| Infra | Docker Compose on a home server behind Cloudflare Tunnel → VPS at launch |

## Status

The active runtime and client implement the five text modes. Prior audits and
captured traces are useful technical evidence, but do not replace actual UI
journeys or human content playtests. Phase 4 device/accessibility/performance
proofs and broader production gates remain open.

## Quickstart

For the private prototype, follow [PLAYTEST.md](docs/guides/PLAYTEST.md):

```bash
make playtest.up       # isolated private server and six independent Web origins
make playtest.down     # stop the private stack; preserve its data
```

For your usual interactive debugging, `make up` enables the same private
zero-value prototype in the development stack, with Go Air reload:

```bash
make server.build    # build the Go server
make server.rebuild  # stop, rebuild, and recreate the server container
python3 xops/test/tests-lints.py  # all tests and lint checks
make up              # start local server with all five private prototype modes
make web.run         # interactive Flutter debug client on http://localhost:8000
make bots ROOM=ABC123 COUNT=3  # after creating a room; use COUNT=5 for six seats
make down            # stop the local stack
make web.rebuild     # force-rebuild + recreate the client-web dev container, cache-bust the browser
make web.stop        # stop locally owned Flutter web servers on any port
```

For roadmap checkbox progress, run:

```bash
python3 xops/makefile/roadmap_ops.py status
```

## Local stack

`make up` brings up server + postgres + redis + adminer, enables synthetic
private matches through `infra/compose/manual.yaml`, and is fronted by a local
**nginx** reverse proxy that terminates TLS
and publishes friendly `*.knowoff.local` names (dev convenience only — the
planned public ingress is a Cloudflare Tunnel; it is currently disabled. See
[`nginx/README.md`](nginx/README.md)). The `client-web` Compose service is
commented out; start Flutter separately for this general development stack.
Run `make localhostfile.add` once to resolve those names, then:

| URL | What |
|---|---|
| `https://app.knowoff.local`, `http://0.0.0.0:8000/` (flutter CLI debug and integrated browser access) | Flutter web client |
| `https://api.knowoff.local` | Game server — REST + `/ws` WebSocket |
| `https://admin.knowoff.local` | Admin Console / Contributor Portal (dev-only, never expose this) |
| `https://adminer.knowoff.local` | Postgres browser |

Prefer to skip nginx? Every service also publishes straight to
`127.0.0.1`: server `8080`/`9090`/`9091`, postgres `5432`, redis `6379`,
adminer `8081`. See [`infra/README.md`](infra/README.md)
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
