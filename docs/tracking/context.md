# 🧠 docs/tracking/context.md — shared project context pack

> **This file is the single place for project-specific overrides.**
> All vendor entry points (`CLAUDE.md`, `GEMINI.md`, `CONVENTIONS.md`,
> `.github/copilot-instructions.md`, etc.) are invited to reference this file
> so context stays in sync without editing every vendor file.

---

## Project identity

- **Name**: Knowoff
- **One-liner**: Online social deduction party game — 4/6 players; Donowers bluff about media they can't see.
- **Primary languages**: Go (authoritative game server), Dart/Flutter (client — Android, iOS, Web PWA)
- **Data**: PostgreSQL + Redis + MinIO; Docker Compose; Cloudflare Tunnel (dev/beta) → VPS (launch)

## Key paths

| Concern | Path |
|---|---|
| **Normative product + tech spec** | `BLUEPRINT.md` (root) — rules, economy, protocol, infra |
| Master rulebook (agents) | `AGENTS.md` |
| Project plan | `docs/planning/ROADMAP.md` — 6 phases, lockstep with BLUEPRINT.md |
| Terminology | `docs/project/GLOSSARY.md` |
| Tracking log | `docs/tracking/tracking.csv` |
| Skills library | `.agents/skills/` |
| Ops scripts | `xops/` |

## Active context (update as the project evolves)

Blueprint + roadmap written; no game code exists yet. Next work:
**Phase 1 — Foundation** (`phase-1` in `docs/planning/ROADMAP.md`): monorepo
tree, layered YAML config loader, Compose stack, Flutter scaffold with
WebSocket echo on Android + Web.

## Project-specific conventions

- **Terminology is normative** (see `docs/project/GLOSSARY.md`): Nower,
  Donower, Nown, Knowoff (the vote), Noin — written **without "the"**.
  Deprecated names (Known, Knoin, Off/Donow, "the Unknown", Daily Nown,
  One More Round) must not reappear.
- **Server-authoritative**: the client never decides an outcome, computes a
  balance, or receives data its role shouldn't see. Role secrecy (Nown
  never sent to Donower/eliminated devices) is the critical invariant —
  changes near payload rendering need a protocol-level no-leak test.
- **Every tunable number** lives in `configs/gameplay/tuning.yaml` — never
  hardcode a timer, price, threshold, or reward value.
- **Lockstep rule**: a feature change to `BLUEPRINT.md` lands in the same
  commit as its `ROADMAP.md` update.
- Config discipline: env vars only select the config file
  (`KNOWOFF_CONFIG`) and inject secrets; secrets never in YAML or images.
- No third-party analytics SDK in the client; KPIs derive from the
  server's audit stream.

## Out-of-scope / do not touch

- `BLUEPRINT.md` game rules, economy values, and terminology change **only
  on explicit owner instruction** — agents propose, never unilaterally edit.
- No pay-to-win mechanics of any kind: nothing purchasable may affect
  dealing, roles, votes, or scoring.
- Content policy line (suggestive OK, never pornographic; adult media only
  in age-gated packs) is owner-locked.

## External service dependencies

| Service | Env var | Notes |
|---|---|---|
| Cloudflare Tunnel | `TUNNEL_TOKEN` | dev/beta ingress; injected via env, never committed |
| Google Sign-In / Facebook Login | (client ids/secrets via config `${VAR}` interpolation) | Phase 4 — OAuth registration/linking |
| Gemini image API ("nano banana") | (API key via `${VAR}`) | offline raster batches + API generation lane |
| Gemini Embedding 2 | (API key via `${VAR}`) | optional API lane; local SigLIP/CLIP is default |
| Platform billing (Play Billing / StoreKit) | — | Noin bulks + Premium subscription, Phase 5 |
| Ad network with SSV | (keys via `${VAR}`) | rewarded post-match doubler; server-side verification only |

## Agent quick-reference

```bash
make help           # all targets
make doctor         # sanity-check framework install
make git.dry        # preview pending commits (read-only)
make git            # commit + push (human runs this)
make track.add ACTION=note SUMMARY="..."   # append tracking row
make skills.find TAG=<tag>                 # search skill library
make skills.status                         # skill index with line counts
```
