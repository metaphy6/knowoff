import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/l10n/app_localizations.dart';
import 'package:knowoff_client/presentation/theme/knowoff_theme.dart';
import 'package:knowoff_client/presentation/theme/knowoff_tokens.dart';
import 'package:knowoff_client/presentation/theme/ko_breakpoints.dart';
import 'package:knowoff_client/presentation/widgets/ko_body.dart';
import 'package:knowoff_client/presentation/widgets/ko_button.dart';
import 'package:knowoff_client/presentation/widgets/ko_scaffold.dart';

Widget _harness(Widget child) {
  return MaterialApp(
    theme: knowoffTheme(),
    localizationsDelegates: AppLocalizations.localizationsDelegates,
    supportedLocales: AppLocalizations.supportedLocales,
    home: child,
  );
}

void main() {
  testWidgets('KoScaffold renders its body when a bottom bar is present',
      (tester) async {
    await tester.pumpWidget(
      _harness(
        KoScaffold(
          title: 'Verdict',
          bottomBar: KoButton(label: 'Back', onTap: () {}),
          body: const KoBody(
            children: <Widget>[Text('body content')],
          ),
        ),
      ),
    );
    await tester.pump();

    // Regression: the content column used to expand vertically inside the
    // bottom-bar slot, starving the body of every pixel of height.
    expect(find.text('body content'), findsOneWidget);
    expect(tester.getSize(find.byType(ListView)).height, greaterThan(0));
  });

  testWidgets('KoScaffold caps its content column on wide viewports',
      (tester) async {
    tester.view.physicalSize = const Size(2400, 1400);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.reset);

    await tester.pumpWidget(
      _harness(
        const KoScaffold(
          title: 'Wide',
          body: KoBody(children: <Widget>[Text('wide body')]),
        ),
      ),
    );
    await tester.pump();

    // The scroll view stays viewport-wide — that is what keeps the scrollbar
    // lane off the content — while the content itself is capped.
    final layout = KoLayout.fromSize(const Size(2400, 1400));
    expect(tester.getSize(find.byType(ListView)).width, 2400);
    expect(
      tester.getSize(find.byType(ListView)).width -
          layout.contentPadding.horizontal,
      lessThanOrEqualTo(layout.contentMaxWidth),
    );
  });

  testWidgets('KoScaffold bands its header in the screen accent',
      (tester) async {
    await tester.pumpWidget(
      _harness(
        const KoScaffold(
          title: 'Knowoff',
          accent: KoColors.pink,
          body: SizedBox.shrink(),
        ),
      ),
    );

    final banded = tester.widgetList<Container>(find.byType(Container)).where(
          (c) => (c.decoration as BoxDecoration?)?.color == KoColors.pink,
        );
    expect(banded, isNotEmpty);
    expect(find.text('Knowoff'), findsOneWidget);
  });

  testWidgets('KoScaffold shows a back control only when a route can pop',
      (tester) async {
    await tester.pumpWidget(
      _harness(
        const KoScaffold(title: 'Root', body: SizedBox.shrink()),
      ),
    );
    expect(find.byIcon(Icons.arrow_back), findsNothing);

    final navigator = tester.state<NavigatorState>(find.byType(Navigator));
    navigator.push(
      MaterialPageRoute<void>(
        builder: (_) => const KoScaffold(
          title: 'Pushed',
          body: SizedBox.shrink(),
        ),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.byIcon(Icons.arrow_back), findsOneWidget);
  });
}
