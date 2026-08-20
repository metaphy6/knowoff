import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/presentation/theme/knowoff_tokens.dart';
import 'package:knowoff_client/presentation/widgets/ko_container.dart';

void main() {
  testWidgets('KoContainer uses 3px ink border and hard (4,4) shadow',
      (tester) async {
    await tester.pumpWidget(
      const MaterialApp(
        home: Scaffold(body: KoContainer()),
      ),
    );

    final container = tester.widget<Container>(
      find.descendant(
          of: find.byType(KoContainer), matching: find.byType(Container)),
    );
    final decoration = container.decoration as BoxDecoration;

    expect(decoration.border, isA<Border>());
    final border = decoration.border as Border;
    expect(border.top.width, equals(3));
    expect(border.top.color, equals(KoColors.ink));

    expect(decoration.boxShadow, isNotNull);
    expect(decoration.boxShadow!.length, equals(1));
    final shadow = decoration.boxShadow!.single;
    expect(shadow.offset, equals(const Offset(4, 4)));
    expect(shadow.blurRadius, equals(0));
  });
}
