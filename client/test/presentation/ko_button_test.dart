import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/presentation/theme/knowoff_tokens.dart';
import 'package:knowoff_client/presentation/widgets/ko_button.dart';

void main() {
  testWidgets('KoButton press collapses shadow and translates by (4,4)',
      (tester) async {
    await tester.pumpWidget(
      const MaterialApp(
        home: Scaffold(body: Center(child: KoButton(label: 'Tap me'))),
      ),
    );

    final gesture =
        await tester.startGesture(tester.getCenter(find.byType(KoButton)));
    await tester.pump(const Duration(milliseconds: 100));

    final animatedContainer = tester.widget<AnimatedContainer>(
      find.descendant(
        of: find.byType(KoButton),
        matching: find.byType(AnimatedContainer),
      ),
    );
    final decoration = animatedContainer.decoration as BoxDecoration;
    expect(decoration.boxShadow, isNotNull);
    expect(decoration.boxShadow!.single, equals(KoShadows.pressed));

    final transform = tester.widget<Transform>(
      find.descendant(
        of: find.byType(AnimatedContainer),
        matching: find.byType(Transform),
      ),
    );
    expect(transform.transform.getTranslation().x, equals(4.0));
    expect(transform.transform.getTranslation().y, equals(4.0));

    await gesture.up();
    await tester.pumpAndSettle();
  });
}
