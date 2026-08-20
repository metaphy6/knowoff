import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/presentation/theme/knowoff_tokens.dart';
import 'package:knowoff_client/presentation/widgets/highlighter.dart';

void main() {
  testWidgets('Highlighter contains a CustomPaint with the marker painter',
      (tester) async {
    await tester.pumpWidget(
      const MaterialApp(
        home: Scaffold(
          body: Highlighter(
            color: KoColors.lime,
            child: Text('highlighted'),
          ),
        ),
      ),
    );

    final customPaint = tester.widget<CustomPaint>(
      find.descendant(
          of: find.byType(Highlighter), matching: find.byType(CustomPaint)),
    );
    expect(customPaint.painter, isNotNull);
  });
}
