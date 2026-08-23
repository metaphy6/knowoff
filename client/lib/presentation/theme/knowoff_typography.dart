import 'package:flutter/material.dart';

import 'knowoff_tokens.dart';

/// House typography (ADR-008, superseding ADR-006).
///
/// Baloo 2 — an OFL variable face, bundled under `assets/fonts/` — carries
/// every loud gesture: headings, timers, Noin numerals, the verdict headline.
/// Body copy stays on the platform geometric sans, because the calm body is
/// what makes the loud headline sustainable.
abstract final class KoFonts {
  static const String display = 'Baloo2';
}

/// Variable-axis helper: Baloo 2 exposes a single `wght` axis (400–800), so a
/// weight has to be requested on the axis as well as on [TextStyle.fontWeight].
List<FontVariation> _wght(double weight) =>
    <FontVariation>[FontVariation('wght', weight)];

/// A display-face style. Used directly by widgets that need a numeral or a
/// headline outside the standard [TextTheme] slots.
TextStyle koDisplayStyle({
  required double size,
  FontWeight weight = FontWeight.w800,
  Color color = KoColors.ink,
  double letterSpacing = -0.5,
  double? height,
}) {
  return TextStyle(
    fontFamily: KoFonts.display,
    fontVariations: _wght(weight.value.toDouble()),
    fontWeight: weight,
    fontSize: size,
    color: color,
    letterSpacing: letterSpacing,
    height: height,
  );
}

TextTheme knowoffTextTheme(TextTheme base) {
  const ink = KoColors.ink;

  TextStyle display(double size, {double tracking = -1.0, double? height}) =>
      koDisplayStyle(size: size, letterSpacing: tracking, height: height);

  return base.copyWith(
    displayLarge: display(56, tracking: -2.0, height: 1.0),
    displayMedium: display(44, tracking: -1.6, height: 1.0),
    displaySmall: display(34, tracking: -1.2, height: 1.05),
    headlineLarge: display(30, tracking: -1.0, height: 1.1),
    headlineMedium: display(25, tracking: -0.8, height: 1.15),
    headlineSmall: display(21, tracking: -0.5, height: 1.2),
    titleLarge: display(19, tracking: -0.3, height: 1.2),
    titleMedium: base.titleMedium?.copyWith(
      fontWeight: FontWeight.w700,
      color: ink,
      letterSpacing: 0.1,
    ),
    titleSmall: base.titleSmall?.copyWith(
      fontWeight: FontWeight.w700,
      color: ink,
      letterSpacing: 0.4,
    ),
    bodyLarge: base.bodyLarge?.copyWith(color: ink, height: 1.4),
    bodyMedium: base.bodyMedium?.copyWith(color: ink, height: 1.4),
    bodySmall: base.bodySmall?.copyWith(color: ink, height: 1.35),
    labelLarge: koDisplayStyle(
      size: 17,
      weight: FontWeight.w700,
      letterSpacing: 0.2,
    ),
    labelMedium: base.labelMedium?.copyWith(
      fontWeight: FontWeight.w700,
      color: ink,
      letterSpacing: 0.6,
    ),
    labelSmall: base.labelSmall?.copyWith(
      fontWeight: FontWeight.w700,
      color: ink,
      letterSpacing: 0.8,
    ),
  );
}
