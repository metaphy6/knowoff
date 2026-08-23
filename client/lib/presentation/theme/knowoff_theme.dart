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
    tertiary: KoColors.tangerine,
    onTertiary: KoColors.ink,
    error: KoColors.pink,
    onError: KoColors.ink,
    surface: KoColors.surface,
    onSurface: KoColors.ink,
  );

  const inkBorder = BorderSide(width: KoBorders.regular, color: KoColors.ink);
  final sheetShape = RoundedRectangleBorder(
    side: inkBorder,
    borderRadius: BorderRadius.circular(KoRadii.sheet),
  );
  final textTheme = knowoffTextTheme(base.textTheme);

  OutlineInputBorder field(Color color, double width) => OutlineInputBorder(
        borderRadius: BorderRadius.circular(KoRadii.button),
        borderSide: BorderSide(width: width, color: color),
      );

  return base.copyWith(
    colorScheme: colorScheme,
    scaffoldBackgroundColor: KoColors.canvas,
    textTheme: textTheme,
    primaryTextTheme: textTheme,
    appBarTheme: AppBarTheme(
      backgroundColor: KoColors.canvasDeep,
      foregroundColor: KoColors.ink,
      elevation: 0,
      scrolledUnderElevation: 0,
      centerTitle: false,
      titleTextStyle: textTheme.headlineSmall,
    ),
    dialogTheme: DialogThemeData(
      backgroundColor: KoColors.surface,
      elevation: 0,
      shape: sheetShape,
      titleTextStyle: textTheme.headlineSmall,
      contentTextStyle: textTheme.bodyLarge,
    ),
    bottomSheetTheme: BottomSheetThemeData(
      backgroundColor: KoColors.surface,
      elevation: 0,
      shape: sheetShape,
    ),
    cardTheme: CardThemeData(
      color: KoColors.surface,
      elevation: 0,
      shape: RoundedRectangleBorder(
        side: inkBorder,
        borderRadius: BorderRadius.circular(KoRadii.card),
      ),
    ),
    chipTheme: base.chipTheme.copyWith(
      backgroundColor: KoColors.surface,
      selectedColor: KoColors.violet,
      labelStyle: textTheme.labelLarge,
      showCheckmark: false,
      shape: RoundedRectangleBorder(
        side: inkBorder,
        borderRadius: BorderRadius.circular(KoRadii.chip),
      ),
    ),
    progressIndicatorTheme: const ProgressIndicatorThemeData(
      color: KoColors.violet,
      linearTrackColor: KoColors.whiteWell,
      linearMinHeight: 10,
    ),
    dividerTheme: const DividerThemeData(
      color: KoColors.ink,
      thickness: KoBorders.thin,
      space: KoSpace.lg,
    ),
    snackBarTheme: SnackBarThemeData(
      backgroundColor: KoColors.ink,
      contentTextStyle: textTheme.titleMedium?.copyWith(
        color: KoColors.surface,
      ),
      behavior: SnackBarBehavior.floating,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(KoRadii.button),
      ),
    ),
    inputDecorationTheme: InputDecorationTheme(
      filled: true,
      fillColor: KoColors.whiteWell,
      enabledBorder: field(KoColors.ink, KoBorders.regular),
      border: field(KoColors.ink, KoBorders.regular),
      focusedBorder: field(KoColors.violet, KoBorders.thick),
      errorBorder: field(KoColors.pink, KoBorders.thick),
      focusedErrorBorder: field(KoColors.pink, KoBorders.thick),
      labelStyle: textTheme.titleMedium,
      hintStyle: textTheme.bodyMedium,
    ),
    elevatedButtonTheme: ElevatedButtonThemeData(
      style: ElevatedButton.styleFrom(
        backgroundColor: KoColors.violet,
        foregroundColor: KoColors.ink,
        textStyle: textTheme.labelLarge,
        elevation: 0,
        shape: RoundedRectangleBorder(
          side: inkBorder,
          borderRadius: BorderRadius.circular(KoRadii.button),
        ),
      ),
    ),
    textButtonTheme: TextButtonThemeData(
      style: TextButton.styleFrom(
        foregroundColor: KoColors.ink,
        textStyle: textTheme.labelLarge,
      ),
    ),
    iconButtonTheme: IconButtonThemeData(
      style: IconButton.styleFrom(foregroundColor: KoColors.ink),
    ),
    iconTheme: const IconThemeData(color: KoColors.ink),
    listTileTheme: const ListTileThemeData(
      textColor: KoColors.ink,
      iconColor: KoColors.ink,
    ),
  );
}
