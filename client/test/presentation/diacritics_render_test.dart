import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/presentation/theme/knowoff_theme.dart';
import 'package:knowoff_client/presentation/theme/knowoff_typography.dart';

/// ADR-008 reversal criterion #1: the bundled display face must render
/// diacritics for every launch locale, including the pseudo-locale sweep.
const _strings = <String>[
  'Knowoff — ¡jugar!',
  '¿Quién?',
  'Knowoff — играть',
  'Knowoff — jouer',
  'Knowoff — spielen',
  'Ğüşöçİ ÅÆØ ẞ',
  '[Ķńöŵöƒƒ — ρĺåŷ]',
];

void main() {
  testWidgets('diacritics strings pump without overflow', (tester) async {
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: Column(
            children: _strings
                .map((text) => Text(text, style: const TextStyle(fontSize: 24)))
                .toList(),
          ),
        ),
      ),
    );

    for (final text in _strings) {
      expect(find.text(text), findsOneWidget);
    }
  });

  testWidgets('diacritics render in the display face at headline size',
      (tester) async {
    await tester.pumpWidget(
      MaterialApp(
        theme: knowoffTheme(),
        home: Scaffold(
          body: ListView(
            children: <Widget>[
              for (final text in _strings)
                Text(text, style: koDisplayStyle(size: 30)),
            ],
          ),
        ),
      ),
    );
    await tester.pump();

    expect(tester.takeException(), isNull);
    for (final text in _strings) {
      final widget = tester.widget<Text>(find.text(text));
      expect(widget.style?.fontFamily, equals(KoFonts.display));
    }
  });
}
