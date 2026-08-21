import 'package:flutter/material.dart';

import 'knowoff_tokens.dart';
import 'knowoff_typography.dart';

/// The single house `ThemeData` — every screen, including the stock Material
/// widgets a screen hasn't yet migrated to `Ko*` primitives, renders through
/// this theme so the app never reverts to the default Material look.
ThemeData knowoffTheme() {
  final base = ThemeData(useMaterial3: true, brightness: Brightness.light);
  final colorScheme = ColorScheme.fromSeed(
    seedColor: KoColors.violet,
    brightness: Brightness.light,
  ).copyWith(
    primary: KoColors.violet,
    onPrimary: KoColors.ink,
    secondary: KoColors.lime,
    onSecondary: KoColors.ink,
    error: KoColors.pink,
    onError: KoColors.ink,
    surface: KoColors.surface,
    onSurface: KoColors.ink,
  );

  return base.copyWith(
    colorScheme: colorScheme,
    scaffoldBackgroundColor: KoColors.canvas,
    textTheme: knowoffTextTheme(base.textTheme),
    appBarTheme: const AppBarTheme(
      backgroundColor: KoColors.surface,
      foregroundColor: KoColors.ink,
      elevation: 0,
      centerTitle: false,
    ),
    dialogTheme: DialogThemeData(
      backgroundColor: KoColors.surface,
      shape: RoundedRectangleBorder(
        side: const BorderSide(width: 3, color: KoColors.ink),
        borderRadius: BorderRadius.circular(KoRadii.sheet),
      ),
    ),
    cardTheme: CardThemeData(
      color: KoColors.surface,
      shape: RoundedRectangleBorder(
        side: const BorderSide(width: 3, color: KoColors.ink),
        borderRadius: BorderRadius.circular(KoRadii.card),
      ),
    ),
    chipTheme: base.chipTheme.copyWith(
      backgroundColor: KoColors.surface,
      selectedColor: KoColors.violet,
      labelStyle: const TextStyle(color: KoColors.ink),
      shape: RoundedRectangleBorder(
        side: const BorderSide(width: 3, color: KoColors.ink),
        borderRadius: BorderRadius.circular(KoRadii.chip),
      ),
    ),
    progressIndicatorTheme: const ProgressIndicatorThemeData(
      color: KoColors.violet,
    ),
    elevatedButtonTheme: ElevatedButtonThemeData(
      style: ElevatedButton.styleFrom(
        backgroundColor: KoColors.violet,
        foregroundColor: KoColors.ink,
        elevation: 0,
        shape: RoundedRectangleBorder(
          side: const BorderSide(width: 3, color: KoColors.ink),
          borderRadius: BorderRadius.circular(KoRadii.button),
        ),
      ),
    ),
    listTileTheme: const ListTileThemeData(
      textColor: KoColors.ink,
      iconColor: KoColors.ink,
    ),
  );
}
