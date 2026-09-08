import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/l10n/app_localizations.dart';
import 'package:knowoff_client/presentation/theme/knowoff_tokens.dart';
import 'package:knowoff_client/presentation/widgets/revote_card.dart';

Widget _wrap(Widget child) {
  return MaterialApp(
    localizationsDelegates: AppLocalizations.localizationsDelegates,
    supportedLocales: AppLocalizations.supportedLocales,
    home: Scaffold(body: Center(child: child)),
  );
}

void main() {
  group('RevoteCard', () {
    testWidgets('renders in the specialty tangerine, not the ballot violet',
        (tester) async {
      await tester.pumpWidget(
        _wrap(RevoteCard(remainingSeconds: 15, onTap: () {})),
      );
      await tester.pump();

      expect(find.text('Revote'), findsOneWidget);
      expect(find.text('Only a Nower can reset this ballot.'), findsOneWidget);
      final container = tester.widget<AnimatedContainer>(
        find.descendant(
          of: find.byType(RevoteCard),
          matching: find.byType(AnimatedContainer),
        ),
      );
      final decoration = container.decoration as BoxDecoration;
      expect(decoration.color, KoColors.tangerine);
    });

    testWidgets('tap fires the callback', (tester) async {
      var taps = 0;
      await tester.pumpWidget(
        _wrap(RevoteCard(remainingSeconds: 15, onTap: () => taps++)),
      );
      await tester.pump();

      await tester.tap(find.byType(RevoteCard));
      expect(taps, 1);
    });

    testWidgets('the last five seconds switch to rush mode', (tester) async {
      await tester.pumpWidget(
        _wrap(RevoteCard(remainingSeconds: 4, onTap: () {})),
      );
      await tester.pump();

      // Urgent copy + a live countdown numeral replace the calm hint.
      expect(
        find.text('Last seconds — slam it before the ballot closes!'),
        findsOneWidget,
      );
      expect(find.byKey(const Key('revote-rush-countdown')), findsOneWidget);
      expect(find.text('4'), findsOneWidget);

      // The heartbeat animation keeps running (pumping frames must not
      // throw or overflow), and settles without the calm hint.
      await tester.pump(const Duration(milliseconds: 350));
      await tester.pump(const Duration(milliseconds: 350));
      expect(
        find.text('Only a Nower can reset this ballot.'),
        findsNothing,
      );
    });

    testWidgets('leaving rush mode restores the calm hint', (tester) async {
      await tester.pumpWidget(
        _wrap(RevoteCard(remainingSeconds: 4, onTap: () {})),
      );
      await tester.pump();
      expect(find.byKey(const Key('revote-rush-countdown')), findsOneWidget);

      await tester.pumpWidget(
        _wrap(RevoteCard(remainingSeconds: 12, onTap: () {})),
      );
      await tester.pump();
      expect(find.byKey(const Key('revote-rush-countdown')), findsNothing);
      expect(find.text('Only a Nower can reset this ballot.'), findsOneWidget);
    });
  });
}
