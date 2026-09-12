import 'package:flutter/material.dart';
import 'knowoff_tokens.dart';

TextStyle koDisplayStyle({
  double size = 32,
  FontWeight weight = FontWeight.w800,
  double height = 1.05,
  Color color = KoColors.ink,
}) => TextStyle(
  fontFamily: 'Baloo2',
  fontVariations: [FontVariation('wght', weight.value.toDouble())],
  fontWeight: weight,
  fontSize: size,
  height: height,
  color: color,
  letterSpacing: -0.5,
);

ThemeData knowoffTheme() {
  final base = ThemeData.light(useMaterial3: true);
  final shape = RoundedRectangleBorder(
    borderRadius: BorderRadius.circular(12),
    side: const BorderSide(color: KoColors.ink, width: 3),
  );
  return base.copyWith(
    scaffoldBackgroundColor: KoColors.canvas,
    colorScheme: const ColorScheme.light(
      primary: KoColors.ink,
      onPrimary: KoColors.surface,
      secondary: KoColors.violet,
      onSecondary: KoColors.ink,
      surface: KoColors.surface,
      onSurface: KoColors.ink,
      error: KoColors.ink,
      onError: KoColors.pink,
    ),
    textTheme: base.textTheme
        .apply(bodyColor: KoColors.ink, displayColor: KoColors.ink)
        .copyWith(
          displayLarge: koDisplayStyle(size: 80),
          displayMedium: koDisplayStyle(size: 56),
          displaySmall: koDisplayStyle(size: 40),
          headlineLarge: koDisplayStyle(size: 36),
          headlineMedium: koDisplayStyle(size: 28),
          headlineSmall: koDisplayStyle(size: 24),
          titleLarge: koDisplayStyle(size: 22),
          bodyLarge: const TextStyle(
            fontSize: 17,
            height: 1.45,
            color: KoColors.ink,
          ),
          bodyMedium: const TextStyle(
            fontSize: 15,
            height: 1.45,
            color: KoColors.ink,
          ),
          bodySmall: const TextStyle(
            fontSize: 13,
            height: 1.35,
            color: KoColors.ink,
          ),
          labelLarge: koDisplayStyle(size: 18),
        ),
    inputDecorationTheme: InputDecorationTheme(
      filled: true,
      fillColor: KoColors.whiteWell,
      contentPadding: const EdgeInsets.all(16),
      border: OutlineInputBorder(
        borderRadius: BorderRadius.circular(12),
        borderSide: const BorderSide(width: 2),
      ),
      enabledBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(12),
        borderSide: const BorderSide(width: 2),
      ),
      focusedBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(12),
        borderSide: const BorderSide(width: 4),
      ),
    ),
    dialogTheme: DialogThemeData(
      backgroundColor: KoColors.surface,
      elevation: 0,
      shape: shape,
    ),
    snackBarTheme: SnackBarThemeData(
      backgroundColor: KoColors.ink,
      elevation: 0,
      shape: shape,
      behavior: SnackBarBehavior.floating,
      contentTextStyle: const TextStyle(color: KoColors.surface),
    ),
    progressIndicatorTheme: const ProgressIndicatorThemeData(
      color: KoColors.ink,
    ),
    dividerTheme: const DividerThemeData(color: KoColors.ink, thickness: 2),
    textButtonTheme: TextButtonThemeData(
      style: TextButton.styleFrom(
        foregroundColor: KoColors.ink,
        minimumSize: const Size(48, 48),
      ),
    ),
  );
}
