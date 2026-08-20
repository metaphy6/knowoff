import 'package:flutter/material.dart';

import 'knowoff_tokens.dart';

/// House typography.
///
/// Headings use the heaviest weights available on the default font stack to
/// approximate the chunky rounded display faces in the design matrix (Baloo 2 /
/// Fredoka). Body text is a plain geometric sans. No extra font package is
/// added.
TextTheme knowoffTextTheme(TextTheme base) {
  const ink = KoColors.ink;
  final display = base.displayLarge?.copyWith(
    fontWeight: FontWeight.bold,
    color: ink,
    letterSpacing: -0.5,
  );
  return base.copyWith(
    displayLarge: display,
    displayMedium: base.displayMedium?.copyWith(
      fontWeight: FontWeight.bold,
      color: ink,
    ),
    displaySmall: base.displaySmall?.copyWith(
      fontWeight: FontWeight.bold,
      color: ink,
    ),
    headlineLarge: base.headlineLarge?.copyWith(
      fontWeight: FontWeight.bold,
      color: ink,
    ),
    headlineMedium: base.headlineMedium?.copyWith(
      fontWeight: FontWeight.bold,
      color: ink,
    ),
    headlineSmall: base.headlineSmall?.copyWith(
      fontWeight: FontWeight.w700,
      color: ink,
    ),
    titleLarge: base.titleLarge?.copyWith(
      fontWeight: FontWeight.w700,
      color: ink,
    ),
    titleMedium: base.titleMedium?.copyWith(
      fontWeight: FontWeight.w600,
      color: ink,
    ),
    bodyLarge: base.bodyLarge?.copyWith(color: ink),
    bodyMedium: base.bodyMedium?.copyWith(color: ink),
    bodySmall: base.bodySmall?.copyWith(color: ink),
    labelLarge: base.labelLarge?.copyWith(
      fontWeight: FontWeight.w600,
      color: ink,
    ),
  );
}
