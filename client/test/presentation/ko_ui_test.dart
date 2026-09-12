import 'package:flutter/material.dart';
import 'package:flutter/rendering.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/l10n/app_localizations.dart';
import 'package:knowoff_client/presentation/widgets/ko_ui.dart';

void main() {
  testWidgets('compact header gives unused action width back to its title', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(360, 800);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    final semantics = tester.ensureSemantics();
    await tester.pumpWidget(
      MaterialApp(
        theme: knowoffTheme(),
        localizationsDelegates: AppLocalizations.localizationsDelegates,
        supportedLocales: AppLocalizations.supportedLocales,
        home: KoPage(
          title: 'Knowoff',
          compactHeader: true,
          actions: [
            IconButton(icon: const Icon(Icons.inbox), onPressed: () {}),
            IconButton(icon: const Icon(Icons.feedback), onPressed: () {}),
          ],
          child: const SizedBox.shrink(),
        ),
      ),
    );
    await tester.pumpAndSettle();
    final title = find.text('Knowoff');
    expect(
      tester.renderObject<RenderParagraph>(title).didExceedMaxLines,
      isFalse,
    );
    expect(tester.getSemantics(title).label, 'Knowoff');
    expect(tester.takeException(), isNull);
    semantics.dispose();
  });

  testWidgets(
    'button has one accessible label and an actionable semantic target',
    (tester) async {
      final handle = tester.ensureSemantics();

      var taps = 0;
      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(
            body: KoButton(label: 'Play', onPressed: () => taps++),
          ),
        ),
      );
      final node = tester.getSemantics(find.byType(KoButton));
      expect(node.label, 'Play');
      expect(node.getSemanticsData().hasAction(SemanticsAction.tap), isTrue);
      node.owner!.performAction(node.id, SemanticsAction.tap);
      expect(taps, 1);
      handle.dispose();
    },
  );

  testWidgets('button has a 48dp target and supports keyboard activation', (
    tester,
  ) async {
    var taps = 0;
    await tester.pumpWidget(
      MaterialApp(
        theme: knowoffTheme(),
        home: Scaffold(
          body: Center(
            child: KoButton(label: 'Play', onPressed: () => taps++),
          ),
        ),
      ),
    );
    final finder = find.byType(KoButton);
    expect(tester.getSize(finder).height, greaterThanOrEqualTo(48));
    await tester.tap(finder);
    await tester.pumpAndSettle();
    expect(taps, 1);
    await tester.sendKeyEvent(LogicalKeyboardKey.tab);
    await tester.pump();
    await tester.sendKeyEvent(LogicalKeyboardKey.enter);
    await tester.pumpAndSettle();
    expect(taps, 2);
    expect(tester.binding.transientCallbackCount, 0);
  });
  testWidgets(
    'reduced motion settles without leaving a ticker or moving targets',
    (tester) async {
      await tester.pumpWidget(
        MaterialApp(
          home: MediaQuery(
            data: const MediaQueryData(disableAnimations: true),
            child: Scaffold(
              body: KoEntrance(
                child: KoButton(label: 'Ready', onPressed: () {}),
              ),
            ),
          ),
        ),
      );
      final before = tester.getRect(find.byType(KoButton));
      await tester.pumpAndSettle();
      expect(tester.getRect(find.byType(KoButton)), before);
      expect(tester.binding.transientCallbackCount, 0);
      expect(find.byType(BackdropFilter), findsNothing);
      expect(find.byType(Opacity), findsNothing);
    },
  );
  test('all house shadows remain hard and palette stays legible', () {
    for (final shadow in [
      KoShadows.sm,
      KoShadows.md,
      KoShadows.lg,
      KoShadows.lift,
    ]) {
      expect(shadow.blurRadius, 0);
    }
    for (final fill in [
      KoColors.canvas,
      KoColors.surface,
      KoColors.violet,
      KoColors.lime,
      KoColors.pink,
      KoColors.aqua,
      KoColors.tangerine,
    ]) {
      final contrast =
          (fill.computeLuminance() + 0.05) /
          (KoColors.ink.computeLuminance() + 0.05);
      expect(contrast, greaterThanOrEqualTo(4.5));
    }
  });
}
