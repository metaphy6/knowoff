import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets('diacritics strings pump without overflow', (tester) async {
    final strings = [
      'Knowoff — ¡jugar!',
      '¿Quién?',
      'Knowoff — играть',
      'Knowoff — jouer',
      'Knowoff — spielen',
    ];

    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: Column(
            children: strings
                .map((text) => Text(text, style: const TextStyle(fontSize: 24)))
                .toList(),
          ),
        ),
      ),
    );

    for (final text in strings) {
      expect(find.text(text), findsOneWidget);
    }
  });
}
