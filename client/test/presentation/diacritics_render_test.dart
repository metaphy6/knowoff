import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

/// Keep launch-locale and pseudo-locale character coverage for the next UI.
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
}
