import 'package:flutter/gestures.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/presentation/theme/knowoff_tokens.dart';
import 'package:knowoff_client/presentation/widgets/ko_button.dart';

BoxDecoration _decorationOf(WidgetTester tester) {
  final animatedContainer = tester.widget<AnimatedContainer>(
    find.descendant(
      of: find.byType(KoButton),
      matching: find.byType(AnimatedContainer),
    ),
  );
  return animatedContainer.decoration as BoxDecoration;
}

void main() {
  testWidgets('KoButton press collapses shadow and translates by (4,4)',
      (tester) async {
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: Center(child: KoButton(label: 'Tap me', onTap: () {})),
        ),
      ),
    );

    final gesture =
        await tester.startGesture(tester.getCenter(find.byType(KoButton)));
    await tester.pump(const Duration(milliseconds: 100));

    final decoration = _decorationOf(tester);
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

  testWidgets('KoButton hover lifts the shadow on pointer devices',
      (tester) async {
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: Center(child: KoButton(label: 'Hover me', onTap: () {})),
        ),
      ),
    );

    final pointer = await tester.createGesture(kind: PointerDeviceKind.mouse);
    await pointer.addPointer(location: Offset.zero);
    addTearDown(pointer.removePointer);
    await pointer.moveTo(tester.getCenter(find.byType(KoButton)));
    await tester.pump(const Duration(milliseconds: 150));

    expect(_decorationOf(tester).boxShadow!.single, equals(KoShadows.lift));
  });

  testWidgets('KoButton without onTap renders a disabled, non-pressing state',
      (tester) async {
    await tester.pumpWidget(
      const MaterialApp(
        home: Scaffold(body: Center(child: KoButton(label: 'Nope'))),
      ),
    );

    expect(_decorationOf(tester).color, equals(KoColors.surface));
    expect(_decorationOf(tester).boxShadow!.single, equals(KoShadows.sm));

    final gesture =
        await tester.startGesture(tester.getCenter(find.byType(KoButton)));
    await tester.pump(const Duration(milliseconds: 100));

    expect(_decorationOf(tester).boxShadow!.single, equals(KoShadows.sm));

    await gesture.up();
    await tester.pumpAndSettle();
  });

  testWidgets('KoButton meets the 48dp minimum touch target', (tester) async {
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: Center(
            child: KoButton(
              label: 'x',
              size: KoButtonSize.small,
              onTap: () {},
            ),
          ),
        ),
      ),
    );

    expect(tester.getSize(find.byType(KoButton)).height,
        greaterThanOrEqualTo(48.0));
  });
}
