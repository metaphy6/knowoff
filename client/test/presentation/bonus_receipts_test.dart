import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/data/bonus_delivery.dart';
import 'package:knowoff_client/data/bonus_session.dart';
import 'package:knowoff_client/l10n/app_localizations.dart';
import 'package:knowoff_client/presentation/widgets/bonus_receipts.dart';
import '../data/bonus_delivery_test.dart'
    show Transport, MemoryDismissals, delivery, page, otherAccount;
import 'package:knowoff_client/presentation/widgets/service_components.dart';
import '../data/bonus_session_test.dart' show SessionAuth;

void main() {
  for (final locale in const [
    Locale('en'),
    Locale('tr'),
    Locale('ar'),
    Locale('en', 'XA'),
  ]) {
    testWidgets(
      'private receipt dismiss and large text ${locale.toLanguageTag()}',
      (tester) async {
        tester.view.physicalSize = const Size(360, 800);
        tester.view.devicePixelRatio = 1;
        addTearDown(tester.view.resetPhysicalSize);
        addTearDown(tester.view.resetDevicePixelRatio);
        final transport = Transport()
          ..answer = page(deliveries: [delivery(credited: 0)]);
        final auth = SessionAuth(transport);
        final deliveries = BonusDeliveryController(
          transport,
          MemoryDismissals(),
        );
        final session = BonusSessionController(auth, deliveries)..start();
        await session.refresh();
        await tester.pumpWidget(
          MaterialApp(
            locale: locale,
            localizationsDelegates: AppLocalizations.localizationsDelegates,
            supportedLocales: AppLocalizations.supportedLocales,
            builder: (context, child) => MediaQuery(
              data: MediaQuery.of(
                context,
              ).copyWith(textScaler: const TextScaler.linear(2)),
              child: child!,
            ),
            home: Scaffold(
              body: SingleChildScrollView(
                child: BonusReceipts(session: session),
              ),
            ),
          ),
        );
        await tester.pumpAndSettle();
        expect(find.byKey(const Key('bonus-receipt')), findsOneWidget);
        final context = tester.element(find.byType(BonusReceipts));
        final l = AppLocalizations.of(context);
        expect(
          find.text(l.bonusCredited(serviceNumber(context, 0))),
          findsOneWidget,
        );
        expect(
          find.text(l.bonusRequested(serviceNumber(context, 8))),
          findsOneWidget,
        );
        expect(tester.takeException(), isNull);
        transport.failAck = true;
        await tester.ensureVisible(find.byKey(const Key('bonus-dismiss')));
        await tester.tap(find.byKey(const Key('bonus-dismiss')));
        await tester.pumpAndSettle();
        expect(find.byKey(const Key('bonus-receipt')), findsNothing);
        expect(find.byKey(const Key('bonus-retry')), findsOneWidget);
        expect(tester.takeException(), isNull);
        auth.switchAccount(otherAccount);
        transport.answer = page();
        await session.refresh();
        await tester.pumpAndSettle();
        expect(find.byKey(const Key('bonus-retry')), findsNothing);
        await tester.pumpWidget(const SizedBox());
        session.dispose();
        deliveries.dispose();
      },
    );
  }
}
