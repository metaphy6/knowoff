# ADR-007: Lock Baloo 2 as the Display Face and Bundle It Offline

## Status
Accepted — supersedes [ADR-006](ADR-006-typography.md).

## Context

[ADR-006](ADR-006-typography.md) deliberately deferred the display face for v1: headings rendered
in `FontWeight.bold` on the platform default font stack. It listed three
reversal criteria — diacritics coverage, OFL licensing, and app-size fit — and
noted that swapping in a real face later would be "a drop-in replacement".

That stand-in turned out to be the single largest reason the client read as
generic Material rather than as the [Soft Neo-Brutalism Design
Matrix](../planning/ROADMAP.md#-visual-identity-soft-neo-brutalism-design-matrix).
The matrix names type as one of the three carriers of personality ("personality
comes from tiles, type, and color"); with a system font at `w700` doing all the
heading work, only two of the three were actually shipping. Oversized,
expressive typography is not decoration in this style — it is the grammar.

## Decision

Lock **Baloo 2** (the matrix's first-named candidate) as the display face and
**bundle it in the repository** rather than fetching it at runtime:

* `client/assets/fonts/Baloo2-Variable.ttf` — the variable `wght` axis (400–800),
  declared in `pubspec.yaml` as family `Baloo2`.
* `client/assets/fonts/OFL-Baloo2.txt` — the SIL Open Font License 1.1 text,
  committed alongside it.
* `presentation/theme/knowoff_typography.dart` exposes `KoFonts.display` and
  `koDisplayStyle()`, and wires the face into `display*`, `headline*`,
  `titleLarge`, and `labelLarge`. Numerals — timers, Noin, tallies, room codes,
  match points — go through `koDisplayStyle()` directly.
* **Body copy stays on the platform geometric sans.** The loud/calm contract is
  the point: if every label shouts, nothing does.

Weight is requested on both `TextStyle.fontWeight` and
`TextStyle.fontVariations` (`FontVariation('wght', …)`), because a variable font
needs the axis set explicitly rather than synthesised.

## Rationale — the ADR-006 reversal criteria, met

1. **Diacritics.** Baloo 2 ships Latin, Latin Extended-A/B and Devanagari.
   `test/presentation/diacritics_render_test.dart` renders the launch-locale
   sweep plus the `en_XA` pseudo-locale in the display face at headline size and
   asserts no exception and no fallback.
2. **License.** SIL OFL 1.1 — permissive, commercially redistributable, and the
   license file is committed with the binary as the OFL requires.
3. **Size.** One 683 KB TTF for the whole family, compressing to roughly a third
   of that over the wire, cached once on the Web PWA. No CI app-size gate exists
   yet; when one lands, this is the number it should be measured against.

Bundling was chosen over the `google_fonts` package because that package fetches
from `fonts.gstatic.com` on first use: it adds a runtime network dependency and
a third-party request on a screen we control, and it makes offline first-launch
non-deterministic. A committed TTF has none of those properties.

## Consequences

* The client gains a ~683 KB asset and no new package dependency.
* `knowoffTextTheme` is no longer a thin `copyWith` over the Material defaults —
  sizes, tracking, and line heights are now house values, so heading metrics no
  longer drift with Flutter's Material type-scale updates.
* Any future face swap is a change to `KoFonts.display` plus the pubspec asset;
  call sites route through `koDisplayStyle()` and the `TextTheme` slots.
* ADR-006 is superseded, not deleted.

## Reversal Criteria

Revisit if a launch locale needs a script Baloo 2 does not cover, if a CI
app-size budget lands that this asset breaks, or if the OFL terms change.
