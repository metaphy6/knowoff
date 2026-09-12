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
| **Nown** | The secret text prompt for a round: a situation, plan or criterion for the selected mode. The text-only target supersedes the former static-image/text offering; see the [Blueprint](../../BLUEPRINT.md) and [text-mode design](../design/DESIGN-text-game-modes.md). Existing identifiers such as `NownRef` and `NownStage` are implementation names, not an image-content requirement. |
| **Knowoff** | The vote at the end of every round. Most-voted player is eliminated, role revealed. |
| **Noin** | The game currency — earned by playing, sold in bulks. Identifiers: `NoinBadge`; event `noin_granted`. |
| **Round** | One mode action per active seat, discussion and one Knowoff; a fresh Nown and public starting state, with earlier public evidence retained. |
| **Match** | Up to 2 votings at 4 players, 3 at 6 — until a team wins. |
| **Session** | Matches played in one room; keeps a running scoreboard. |
| **Gameplay mode** | One of Missed the Briefing, Secret Scale, Make Room, Bad Bargains or Top That; a rule set selected before a match, separate from room type, size, language and theme pack. |
| **Missed the Briefing** | Play and consume one short response card against a hidden situation; the response pool is shared across situations. |
| **Secret Scale** | Place an item card at a rating from 1 to 5 against a hidden criterion. |
| **Make Room** | Replace one item in a three-item public bag against a hidden plan. |
| **Bad Bargains** | Offer a hand card for another player's displayed item; they accept or refuse. Trades move match card copies only, never currency or paid entitlements. |
| **Top That** | Play an item claimed to exceed the current comparison card against a hidden criterion; votes, not an automatic judge, resolve suspicion. |
| **Card instance** | One owned copy of a playable text card. Identical text can appear on distinct copies; public history preserves a copy's action and ownership transitions. |
| **Public evidence** | Ordered, attributed action history retained across rounds; never includes unrevealed Nown, future prompts, private reserves or unrevealed hand cards. |
| **Quick Play** | Online matchmaking with strangers — the main product. |
| **Local Room** | Private room for physically-present players (QR / 6-char code); never capped. |
| **Play Pass** | Noin-priced unlimited-Quick-Play pass (1/3/7-day). Never removes ads. |
| **Premium** | The one cash subscription (monthly/yearly) — sole ad-removal path; includes unlimited Quick Play. |
| **Week Winner** | Most-voted entrant of the Weekly Nown Challenge; holds the title until the next close. |
| **Overall Points** | Lifetime match points — public, never decreases. |
| **Non-Converted Points** | Convertible balance (100 → 1 Noin, one-way); owner-visible only. |
| **Shuffle / Revote** | Legacy specialty names, retained for historical references. All specialties are disabled in the first text release; any later return requires an explicit rule decision and per-mode proofs. |
| **Backfill bot** | A visibly labeled server-controlled seat, earning nothing. Production backfill is disabled in the first text release; a queue timeout offers explicit waiting, mode-change or leave choices. Development test drivers are separate. |
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
