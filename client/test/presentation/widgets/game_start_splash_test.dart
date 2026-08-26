import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/l10n/app_localizations.dart';
import 'package:knowoff_client/presentation/theme/knowoff_theme.dart';
import 'package:knowoff_client/presentation/widgets/game_start_splash.dart';
import 'package:knowoff_client/presentation/widgets/guardrail_audit.dart';
import 'package:knowoff_client/presentation/widgets/ko_meters.dart';

Widget _wrap(Widget child) => MaterialApp(
      theme: knowoffTheme(),
      localizationsDelegates: AppLocalizations.localizationsDelegates,
      supportedLocales: AppLocalizations.supportedLocales,
      home: Scaffold(body: child),
    );

/// Mirrors the RoundScreen usage: the real countdown bar sits under the
/// splash, and the splash removes itself through [GameStartSplash.onDone]
/// once the shrink has landed on the bar.
class _Harness extends StatefulWidget {
  const _Harness({required this.anchor, this.remaining = 5});

  final GlobalKey anchor;
  final int remaining;

  @override
  State<_Harness> createState() => _HarnessState();
}

class _HarnessState extends State<_Harness> {
  bool _done = false;

  @override
  Widget build(BuildContext context) {
    return Stack(
      children: <Widget>[
        Align(
          alignment: Alignment.topCenter,
          child: Padding(
            padding: const EdgeInsets.only(top: 120),
            child: SizedBox(
              width: 320,
              child: KoTimerBar(
                key: widget.anchor,
                remainingSeconds: widget.remaining,
                totalSeconds: 5,
                label: '${widget.remaining}s left',
              ),
            ),
          ),
        ),
        if (!_done)
          Positioned.fill(
            child: GameStartSplash(
              windowSeconds: 5,
              remainingSeconds: widget.remaining,
              anchor: widget.anchor,
              onDone: () => setState(() => _done = true),
            ),
          ),
      ],
    );
  }
}

double _cardScale(WidgetTester tester) {
  final transform = tester.widget<Transform>(
    find.byKey(const ValueKey<String>('game-start-scale')),
  );
  // Not getMaxScaleOnAxis(): the Z axis of a 2D scale is always 1.0, which
  // would hide the shrink — the X axis carries it.
  return transform.transform.entry(0, 0);
}

void main() {
  group('GameStartSplash', () {
    testWidgets('announces the start with headline, subtitle and numeral',
        (tester) async {
      final anchor = GlobalKey();
      await tester.pumpWidget(_wrap(_Harness(anchor: anchor)));
      await tester.pump();

      expect(find.text("It's Knowoff time!"), findsOneWidget);
      expect(
        find.text('Roles are dealt. The first Nown is on its way.'),
        findsOneWidget,
      );
      expect(find.text('5'), findsOneWidget);
      // The honest countdown bar is already running underneath.
      expect(find.byType(KoTimerBar), findsOneWidget);
    });

    testWidgets('numeral follows the remaining-seconds prop', (tester) async {
      final anchor = GlobalKey();
      await tester.pumpWidget(_wrap(_Harness(anchor: anchor)));
      await tester.pump();
      expect(find.text('5'), findsOneWidget);

      await tester.pumpWidget(_wrap(_Harness(anchor: anchor, remaining: 3)));
      await tester.pump();
      expect(find.text('3'), findsOneWidget);
      expect(find.text('5'), findsNothing);
    });

    testWidgets('holds centre stage, then dives into the countdown bar',
        (tester) async {
      final anchor = GlobalKey();
      await tester.pumpWidget(_wrap(_Harness(anchor: anchor)));
      await tester.pump();

      final barCenter = tester.getCenter(find.byKey(anchor));
      final startDistance = (tester.getCenter(
                  find.byKey(const ValueKey<String>('game-start-card'))) -
              barCenter)
          .distance;
      expect(startDistance, greaterThan(80));

      // Most of the window is the announcement beat: still full size.
      await tester.pump(const Duration(seconds: 2));
      expect(_cardScale(tester), greaterThan(0.95));

      // The final two seconds shrink it onto the bar, slow out of the gate…
      await tester.pump(const Duration(milliseconds: 1900));
      expect(_cardScale(tester), lessThan(0.95));

      // …then the dive accelerates and lands on the bar's centre.
      await tester.pump(const Duration(milliseconds: 1000));
      expect(_cardScale(tester), lessThan(0.25));
      final landDistance = (tester.getCenter(
                  find.byKey(const ValueKey<String>('game-start-card'))) -
              barCenter)
          .distance;
      expect(landDistance, lessThan(startDistance / 3));

      // Landed: the splash hands off to the bar and is gone.
      await tester.pump(const Duration(milliseconds: 200));
      await tester.pump();
      expect(find.byType(GameStartSplash), findsNothing);
      expect(find.byType(KoTimerBar), findsOneWidget);
    });

    testWidgets('does not leave early', (tester) async {
      final anchor = GlobalKey();
      await tester.pumpWidget(_wrap(_Harness(anchor: anchor)));
      await tester.pump(const Duration(seconds: 4));

      expect(find.byType(GameStartSplash), findsOneWidget);
    });

    testWidgets('holds the design guardrails while animated', (tester) async {
      final anchor = GlobalKey();
      await tester.pumpWidget(_wrap(_Harness(anchor: anchor)));
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 4200));

      final violations = <String>[];
      tester.binding.rootElement?.visitChildren((element) {
        violations.addAll(GuardrailAudit.auditElement(element));
      });
      expect(violations, isEmpty);
    });
  });
}
