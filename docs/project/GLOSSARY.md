# 📖 Glossary — Knowoff

Domain terms used across this repo. The terminology is **normative**
(locked rev 2026-08-17, [`docs/planning/ROADMAP.md`](../planning/ROADMAP.md)): game terms
are written **without "the"** ("Nown appears", not "the Nown appears").
Deprecated names must not reappear in new code, docs, or identifiers.

## Game terms (normative)

| Term | Meaning |
|---|---|
| **Nower(s)** | Players who see Nown. |
| **Donower(s)** | Players who can't see Nown; nobody knows who they are. |
| **Nown** | The media item of a round — a static image or text. Identifiers: `NownRef`, `NownStage`. See [ADR-011](../design/ADR-011-static-image-and-text-content.md). |
| **Knowoff** | The vote at the end of every round. Most-voted player is eliminated, role revealed. |
| **Noin** | The game currency — earned by playing, sold in bulks. Identifiers: `NoinBadge`; event `noin_granted`. |
| **Round** | Card play (turn-based) + discussion + one Knowoff. |
| **Match** | Up to 2 votings at 4 players, 3 at 6 — until a team wins. |
| **Session** | Matches played in one room; keeps a running scoreboard. |
| **Quick Play** | Online matchmaking with strangers — the main product. |
| **Local Room** | Private room for physically-present players (QR / 6-char code); never capped. |
| **Play Pass** | Noin-priced unlimited-Quick-Play pass (1/3/7-day). Never removes ads. |
| **Premium** | The one cash subscription (monthly/yearly) — sole ad-removal path; includes unlimited Quick Play. |
| **Week Winner** | Most-voted entrant of the Weekly Nown Challenge; holds the title until the next close. |
| **Overall Points** | Lifetime match points — public, never decreases. |
| **Non-Converted Points** | Convertible balance (100 → 1 Noin, one-way); owner-visible only. |
| **Shuffle / Revote** | Unique once-per-match specialty cards — Shuffle usable by Donowers, Revote by Nowers. |
| **Backfill bot** | 🤖-labeled server bot that tops up a short Quick Play queue; earns nothing. |
| **Chat moderation** | Server-side free-text masking using English plus the sender's selected-language list from `moderation.word_lists`. |

## Deprecated names — do not use

| Deprecated | Replaced by | When |
|---|---|---|
| Known | **Nown** | 2026-08-17 |
| Knoin | **Noin** | 2026-08-17 |
| Off / Donow | **Donower** | 2026-08-14 / 2026-08-17 |
| "the Unknown" (named placeholder) | concept removed — plain placeholder prompt, no name | 2026-08-17 |
| Daily Nown | feature removed | 2026-08-18 |
| One More Round (card) | removed — replaced by **Revote** | 2026-08-14 |
| One More Card | **One More Free Card** | 2026-08-14 |

## Framework terms

| Term | Meaning |
|---|---|
| Agent | An AI coding assistant (Copilot, Claude, Gemini, Codex, Cursor, ...). |
| Run | A single agent invocation that may span many tool calls. Identified by `run_id`. |
| Scope | The named slice of work an agent is touching (phase id, module slug, area). |
| Tracking row | One line in `docs/tracking/tracking.csv` recording an agent action. |
