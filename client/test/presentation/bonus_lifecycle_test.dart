import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/data/bonus_delivery.dart';
import 'package:knowoff_client/data/bonus_session.dart';
import 'package:knowoff_client/presentation/widgets/bonus_lifecycle.dart';
import '../data/bonus_delivery_test.dart' show Transport, MemoryDismissals;
import '../data/bonus_session_test.dart' show SessionAuth;

void main() {
  testWidgets('app lifecycle restores on mount and resume, stops on unmount', (
    tester,
  ) async {
    final transport = Transport();
    final deliveries = BonusDeliveryController(transport, MemoryDismissals());
    final session = BonusSessionController(SessionAuth(transport), deliveries);
    await tester.pumpWidget(
      BonusLifecycle(session: session, child: const SizedBox()),
    );
    await tester.pumpAndSettle();
    expect(transport.claims, 1);
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.paused);
    await tester.pump();
    expect(transport.claims, 1);
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
    await tester.pumpAndSettle();
    expect(transport.claims, 2);
    await tester.pumpWidget(const SizedBox());
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.paused);
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
    await tester.pumpAndSettle();
    expect(transport.claims, 2);
  });
}
