# 📐 ADR-010: A Sky-Blue Identity Colour for the Shuffle Specialty

- **Status**: accepted
- **Date**: 2026-09-08
- **Deciders**: @product-owner (via chat request)

## Context

Every specialty card carries its own accent so the hand reads as a set of
distinct powers, but Shuffle shipped on `violet` — the theme's generic
"neutral interactive" colour that also paints buttons, timers, selected
tiles, and headers. The one card whose whole point is table-wide disruption
was camouflaged as ordinary chrome. The palette was already at the
ten-colour ceiling set by ADR-008 and the 🎨 matrix, and every other
existing accent was taken: `aqua` (Pass), `pink` (Reveal), `tangerine`
(Revote), `lime` (Free Card) — while `canvas`/`canvasDeep` sit in the same
purple family the change was meant to escape.

## Decision

Add one new support accent, `sky` `#8FCBF5`, as the Shuffle specialty's
identity colour, raising the palette ceiling to eleven colours.

## Consequences

- ➕ Shuffle is instantly recognisable in the hand, in its table-wide
  announcement, and in the exposed-hand panel — it no longer reads as
  generic violet chrome.
- ➕ The fixed verdict semantics stay intact: `sky` is categorical only,
  never a vote/role/win signal, and it clears WCAG AAA against `ink`
  (~10.5:1), matching the ADR-008 bar.
- ➖ The palette grows to eleven tokens; the "ten colours total is the
  ceiling" line in the 🎨 matrix is amended by this ADR.
- 🔁 Reversible — one token plus one `specialtyColor` case; the token
  snapshot test now covers `sky`, so silent drift fails mechanically either
  way.

## Considered options

- **Add `sky` `#8FCBF5`** (chosen) — the only free hue family left (blue)
  that stays flat-pastel on-brand, is clearly not purple, and is unused by
  any other specialty.
- **Reuse an existing token** (`canvasDeep`, `aqua`, …) — `canvasDeep` is
  still purple, the very thing being fixed; `aqua`, `tangerine`, `lime`,
  and `pink` already belong to Pass, Revote, Free Card, and Reveal.
- **Swap Shuffle with another specialty's accent** — solves Shuffle but
  just moves the camouflage problem onto a different card and breaks an
  identity players have already learned.
