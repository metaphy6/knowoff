# ADR-006: v1 Typography — Heaviest Default-Font Weights as Display Stand-In

## Status
Superseded by [ADR-007](ADR-007-display-typeface.md) — all three reversal criteria below were met and Baloo 2 is now bundled.

## Context
The design matrix calls for a chunky rounded display face (Baloo 2 or Fredoka class) for headings, timers, and Noin numbers, plus a plain geometric sans for body.

Adding a third-party font package to the Flutter client increases binary size, introduces a licensing verification step, and requires a diacritics render check across every launch locale before we can commit to it.

## Decision
For v1, headings use the heaviest available weight (`FontWeight.bold`) of the default platform font stack. Body text uses the default geometric sans at normal weight. No extra font package is added.

## Rationale
* Keeps the v1 binary lean and the build simple.
* Avoids committing to a font whose license and glyph coverage we have not yet verified for all launch locales.
* The press-and-hold role check, lime highlighter sweep, and brutalist structure carry enough identity that the font is not the primary differentiator.

## Reversal Criteria
Before public launch we will evaluate and potentially lock a display face if:

1. **Diacritics render correctly** across all launch locales on both Android and Web PWA.
2. The font is **OFL-licensed** (or has an equivalent permissive license) and compatible with commercial redistribution.
3. The **file-size impact** fits within the CI app-size budget.

If any of the above is not met, we keep the default-font stand-in.

## Consequences
* The visual character is slightly less chunky than the design matrix target.
* The change is localized to `knowoffTextTheme` in `presentation/theme/knowoff_typography.dart`; swapping in a real display face later is a drop-in replacement.
