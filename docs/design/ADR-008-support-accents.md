# ADR-008: Three Support Accents Beside the Locked Verdict Palette

## Status
Accepted.

## Context

The [Soft Neo-Brutalism Design Matrix](../planning/ROADMAP.md#-visual-identity-soft-neo-brutalism-design-matrix)
locks seven colours and assigns fixed semantics to three of them: `violet` =
interact, `lime` = truth/reward, `pink` = accuse/risk, `ink` = information.
Those semantics are load-bearing — a player learns them in their first match and
must never see them mean something else.

In practice the v1 client had a different problem. Because `violet` was the only
"neutral interactive", it ended up carrying buttons, timers, progress fills,
headers, and selected states simultaneously; `lime` did double duty as reward
*and* as "any secondary button"; every screen used the same lavender canvas with
the same cream cards. The result read as monotonous — not because the palette
was inaccessible (every accent clears WCAG AAA against `ink`, verified in
[`neo-brutalism-ui-design`](../../.agents/skills/neo-brutalism-ui-design/SKILL.md) §4),
but because there was no colour left over to differentiate one screen from
another or to mark a category that isn't a verdict.

## Decision

Add **three support accents** to `KoColors`, alongside — never replacing — the
seven locked tokens:

| Token | Value | Role | Contrast with `ink` |
|---|---|---|---|
| `canvasDeep` | `#C4A8F0` | Header bands, rails, section stamps | ~9.0:1 (AAA) |
| `tangerine` | `#FFB020` | Currency, streaks, heat, "on the clock" | ~10.1:1 (AAA) |
| `aqua` | `#7FE7DC` | Time, connection, neutral information, bot labels | ~12.6:1 (AAA) |

Binding constraints:

* **The three verdict semantics are untouched.** No support accent may signal a
  role, a vote outcome, a win, or a risk. A Donower reveal is `pink`; a catch is
  `lime`; an interactive control is `violet`. Support accents are categorical
  only.
* **Colour is still never the sole signal.** Every state-bearing surface pairs
  an icon and a label with its fill, enforced at the type level by `KoChip`,
  `VerdictChip`, and `SeatTile`'s badges.
* **The gradient rule is unchanged.** `KoColors.revealGradient` remains the one
  permitted gradient, spent on the Knowoff result window and nowhere else —
  asserted by `screens_test.dart`.
* Every new pairing was contrast-checked against `ink` before shipping; the
  values above are AAA for normal text with headroom.

The token snapshot test (`test/presentation/token_snapshot.txt`) was extended to
cover the new colours, the named shadow tiers, the border widths, and the `KoTilt`
set, so any future drift is caught mechanically.

## Rationale

* Screens can now be told apart at a glance — Knowoff runs on `pink` over
  `canvasDeep`, Discussion on `aqua`, Store on `pink`, Leaderboard on
  `tangerine`, Round on `violet` (or `lime` while it is your turn).
* Currency finally has its own hue. Under the old palette a Noin balance was
  rendered in `lime` — the same colour that means "a Donower was caught" — which
  is exactly the kind of semantic collision the matrix's fixed-meaning rule
  exists to prevent.
* Seat identity gets a stable six-colour rotation (`seatAccent`), which is only
  possible with more than three usable accents.

## Consequences

* `KoColors` grows from 7 to 10 colours. The palette is still small enough to
  hold in your head, and the three additions are deliberately lower-energy than
  the verdict trio, so they recede when a verdict fires.
* The design matrix chapter in `ROADMAP.md` was updated in the same change so
  the spec and the code do not drift.
* Any *further* hue is another ADR. Ten is the ceiling for this style.

## Reversal Criteria

Revisit if playtesting shows players reading a support accent as a verdict
signal, or if a colourblind-simulation pass finds a support accent that is
indistinguishable from `lime` or `pink` in a state-bearing context.
