import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/presentation/icons/doodles.dart';
import 'package:knowoff_client/presentation/theme/knowoff_theme.dart';
import 'package:knowoff_client/presentation/theme/knowoff_tokens.dart';
import 'package:knowoff_client/presentation/theme/knowoff_typography.dart';

void main() {
  group('typography', () {
    test('every display slot resolves to the bundled Baloo 2 face', () {
      final theme = knowoffTextTheme(ThemeData.light().textTheme);
      final displaySlots = <TextStyle?>[
        theme.displayLarge,
        theme.displayMedium,
        theme.displaySmall,
        theme.headlineLarge,
        theme.headlineMedium,
        theme.headlineSmall,
        theme.titleLarge,
        theme.labelLarge,
      ];
      for (final style in displaySlots) {
        expect(style?.fontFamily, equals(KoFonts.display));
        expect(style?.fontVariations, isNotEmpty);
      }
    });

    test('body copy stays on the plain sans so headlines can shout', () {
      final theme = knowoffTextTheme(ThemeData.light().textTheme);
      expect(theme.bodyLarge?.fontFamily, isNot(equals(KoFonts.display)));
      expect(theme.bodyMedium?.fontFamily, isNot(equals(KoFonts.display)));
    });

    test('headlines are big enough to read as a graphic element', () {
      final theme = knowoffTextTheme(ThemeData.light().textTheme);
      expect(theme.displayLarge!.fontSize, greaterThanOrEqualTo(48));
      expect(theme.headlineMedium!.fontSize, greaterThanOrEqualTo(24));
    });

    test('koDisplayStyle keeps the weight axis and the fontWeight in sync', () {
      final style = koDisplayStyle(size: 20, weight: FontWeight.w700);
      expect(style.fontWeight, equals(FontWeight.w700));
      expect(
        style.fontVariations!.single.value,
        equals(FontWeight.w700.value.toDouble()),
      );
    });
  });

  group('theme', () {
    test('stock Material surfaces inherit the ink border grammar', () {
      final theme = knowoffTheme();

      final card = theme.cardTheme.shape! as RoundedRectangleBorder;
      expect(card.side.width, equals(KoBorders.regular));
      expect(card.side.color, equals(KoColors.ink));

      final dialog = theme.dialogTheme.shape! as RoundedRectangleBorder;
      expect(dialog.side.color, equals(KoColors.ink));

      expect(theme.cardTheme.elevation, equals(0));
      expect(theme.appBarTheme.elevation, equals(0));
      expect(theme.scaffoldBackgroundColor, equals(KoColors.canvas));
    });

    test('the support accents are wired into the colour scheme', () {
      final theme = knowoffTheme();
      expect(theme.colorScheme.primary, equals(KoColors.violet));
      expect(theme.colorScheme.secondary, equals(KoColors.lime));
      expect(theme.colorScheme.tertiary, equals(KoColors.tangerine));
      expect(theme.colorScheme.error, equals(KoColors.pink));
    });
  });

  group('doodles', () {
    testWidgets('every glyph in the set paints', (tester) async {
      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(
            body: Wrap(
              children: <Widget>[
                for (final doodle in Doodle.values)
                  DoodleIcon(doodle, size: 32),
              ],
            ),
          ),
        ),
      );

      expect(tester.takeException(), isNull);
      expect(find.byType(DoodleIcon), findsNWidgets(Doodle.values.length));
    });

    test('the set covers the ~12 glyphs the design matrix calls for', () {
      expect(Doodle.values.length, greaterThanOrEqualTo(12));
    });
  });
}
