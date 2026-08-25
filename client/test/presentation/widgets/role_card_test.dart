import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/l10n/app_localizations.dart';
import 'package:knowoff_client/presentation/widgets/role_card.dart';

void main() {
  Widget wrap(Widget child) => MaterialApp(
        localizationsDelegates: AppLocalizations.localizationsDelegates,
        supportedLocales: AppLocalizations.supportedLocales,
        home: Scaffold(body: Center(child: child)),
      );

  group('RoleCard', () {
    testWidgets(
        'idle square is square, large enough, and shows the reveal prompt',
        (tester) async {
      await tester.pumpWidget(wrap(const RoleCard(role: null)));
      await tester.pumpAndSettle();

      final size = tester.getSize(find.byType(RoleCard));
      expect(size.width, equals(size.height),
          reason: 'RoleCard must stay square');
      expect(size.width, greaterThanOrEqualTo(48),
          reason: 'RoleCard should be clearly visible');

      expect(find.text('Reveal your role'), findsOneWidget);
      expect(find.text('Nower'), findsNothing);
      expect(find.text('Donower'), findsNothing);
    });

    testWidgets('pressing the square reveals the role label', (tester) async {
      await tester.pumpWidget(wrap(const RoleCard(role: 'nower')));
      await tester.pumpAndSettle();

      expect(find.text('Reveal your role'), findsOneWidget);

      final gesture =
          await tester.startGesture(tester.getCenter(find.byType(RoleCard)));
      await tester.pump(const Duration(milliseconds: 100));

      expect(find.text('Nower'), findsOneWidget);
      expect(find.text('Reveal your role'), findsNothing);

      await gesture.up();
      await tester.pumpAndSettle();

      expect(find.text('Reveal your role'), findsOneWidget);
      expect(find.text('Nower'), findsNothing);
    });
  });
}
