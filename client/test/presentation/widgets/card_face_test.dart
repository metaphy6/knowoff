import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/data/models/game_state_dto.dart';
import 'package:knowoff_client/l10n/app_localizations.dart';
import 'package:knowoff_client/presentation/widgets/card_face.dart';

void main() {
  Widget wrap(Widget child) => MaterialApp(
        localizationsDelegates: AppLocalizations.localizationsDelegates,
        supportedLocales: AppLocalizations.supportedLocales,
        home: Scaffold(body: Center(child: child)),
      );

  testWidgets('a text card renders its content', (tester) async {
    await tester.pumpWidget(wrap(const CardFace(
      card: CardDto(id: 'card-1', type: 'text', content: 'Hello'),
    )));

    expect(find.text('Hello'), findsOneWidget);
  });

  testWidgets(
      'a turn-timeout card with nothing lost shows a label instead of a broken-image placeholder',
      (tester) async {
    // Regression: a timed-out seat with an empty hand played and lost no
    // card at all, but the evidence table still needs a slot for it — a bare
    // image placeholder read as an empty/broken box, not as "this seat
    // auto-passed".
    await tester.pumpWidget(wrap(const CardFace(
      card: CardDto(id: '', type: 'text', timedOut: true),
    )));

    expect(find.text('Timed out'), findsOneWidget);
  });

  testWidgets('a turn-timeout card shows the card it lost, tagged as timed out',
      (tester) async {
    // Regression: a timed-out seat still lost a real card to the stalling
    // penalty (Rules §3) — the table should show what was auto-discarded,
    // not just a bare "timed out" marker.
    await tester.pumpWidget(wrap(const CardFace(
      card: CardDto(
        id: 'card-1',
        type: 'text',
        content: 'Distant match for topic 44',
        timedOut: true,
      ),
    )));

    expect(find.text('Distant match for topic 44'), findsOneWidget);
    expect(find.text('Timed out'), findsOneWidget);
  });
}
