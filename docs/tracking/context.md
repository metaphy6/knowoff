# 🧠 docs/tracking/context.md — shared project context pack

> **This file is the single place for project-specific overrides.**
> All vendor entry points (`CLAUDE.md`, `GEMINI.md`, `CONVENTIONS.md`,
> `.github/copilot-instructions.md`, etc.) are invited to reference this file
> so context stays in sync without editing every vendor file.

---

## Project identity

- **Name**: Knowoff
- **One-liner**: Online social deduction party game — 4/6 players; Donowers infer a hidden text situation/criterion through five selectable modes.
- **Primary languages**: Go (authoritative game server), Dart/Flutter (client — Android, iOS, Web PWA)
- **Data**: PostgreSQL + Redis retained; MinIO removed from active Compose/proxy; owned historical restore fixtures retained; Compose/Cloudflare → public host

## Key paths

| Concern | Path |
|---|---|
| **Normative product + tech spec** | `BLUEPRINT.md` — game rules, tech stack, architecture, economy, product baseline |
| Master rulebook (agents) | `AGENTS.md` |
| Project plan | `docs/planning/ROADMAP.md` — seven text-transition phases; previous phases archived |
| Terminology | `docs/project/GLOSSARY.md` |
| Tracking log | `docs/tracking/tracking.csv` |
| Skills library | `.agents/skills/` |
| Ops scripts | `xops/` |

## Active context (update as the project evolves)

The server, Flutter client and text contribution workflow exist. Consult the
[roadmap](../planning/ROADMAP.md) for current proof and open work rather than
treating this context pack as a completion snapshot. For content work, read the
[server mechanics audit](../code/MODULE-media-engine.md) and the source-linked
[content skills](../../.agents/skills/README.md#knowoff-content). Drafting,
human acceptance, certification and activation are separate states.

The owner adopted **text-only Nowns/cards and five selectable modes** on
2026-09-12: [ADR-012](../design/ADR-012-text-only-selectable-modes.md),
[Blueprint](../../BLUEPRINT.md), [transition design](../design/DESIGN-text-transition.md),
[business plan](../product/BUSINESS_PLAN.md). Phase 1 contracts/config, inventory,
wallet transaction review and isolated migration/backfill design proofs are
verified complete. The owner states no deployment exists. Phases 2–5 now have
substantial saved implementation, including five-mode runtime/client, durable
value and content/trust infrastructure. The owner committed checkpoint
`bd5e13e` and resumed all remaining phases on 2026-09-12; the earlier pause is
superseded. Read the [resumption evidence](../reports/2026-09-12-text-transition-resumption.md)
and current checkpoint. Later integration has not passed the final unified gate
and is not yet a completed commit handoff.
Initial text specialties and production
backfill are off. Preserve no-paid-advantage, shared caps, roles/votes, identity,
community and the visual brand. Follow the current content source checklist;
High/Distant/Chaos evidence does not automatically certify stateful mode actions.

Hand privacy, history, snapshot/sequence, pack pinning, locking and settlement
have targeted regression evidence in the resumption report. Final combined
validation, all-writer backup quiescence and complete source retirement remain
open in the active Roadmap. Applied SQL,
ADRs and tracking remain historical records. The subsequent implementation
request supersedes the original planning-only boundary; deploy/data-retirement
gates remain evidence-dependent. Do not automatically apply older image/CGO
retirement advice: avatars still consume WebP.

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
- **Lockstep rule**: normative chapters live in `BLUEPRINT.md`; the implementation
  sequence lives in `docs/planning/ROADMAP.md`. Update both in the same change
  when a specification change affects the roadmap.
- Config discipline: env vars only select the config file
  (`KNOWOFF_CONFIG`) and inject secrets; secrets never in YAML or images.
- No third-party analytics SDK in the client; KPIs derive from the
  server's audit stream.

## Out-of-scope / do not touch

- `BLUEPRINT.md`'s game rules, economy values, and terminology change
  **only on explicit owner instruction** — agents propose, never
  unilaterally edit.
- No pay-to-win mechanics of any kind: nothing purchasable may affect
  dealing, roles, votes, or scoring.
- Content policy line (suggestive OK, never pornographic; adult media only
  in age-gated packs) is owner-locked.

## External service dependencies

| Service | Env var | Notes |
|---|---|---|
| Cloudflare Tunnel | `TUNNEL_TOKEN` | dev/beta ingress; injected via env, never committed |
| Google Sign-In / Facebook Login | (client ids/secrets via config `${VAR}` interpolation) | Phase 4 — OAuth registration/linking |
| GPT-6 Astra | (API key via `${VAR}`) | planned AI candidate-generation lane per Blueprint ⚙️ §3; human-authored contributions and rewrites use the same curation process; non-playable brand raster batches remain separate |
| Gemini Embedding 2 | (API key via `${VAR}`) | historical multimodal option; text evaluator/provider selection requires current evidence |
| Platform billing (Play Billing / StoreKit) | — | Noin bulks + Premium subscription, Phase 5 |
| Ad network with SSV | (keys via `${VAR}`) | rewarded post-match doubler; server-side verification only |

## Agent quick-reference

```bash
make help           # all targets
make git.dry        # preview pending commits (read-only)
make git            # commit + push (human runs this)
make track.add ACTION=note SUMMARY="..."   # append tracking row
```
