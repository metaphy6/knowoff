import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/data/purchase_controller.dart';
import 'package:knowoff_client/presentation/widgets/purchase_lifecycle.dart';
import '../data/purchase_controller_test.dart'
    show TestPurchaseAuth, TestPurchaseAPI, TestBridge;

void main() {
  testWidgets(
    'application listener starts before child and resumes retained receipt without another purchase',
    (t) async {
      final auth = TestPurchaseAuth(), bridge = TestBridge();
      final api = TestPurchaseAPI(auth)
        ..answer = () async => throw StateError('offline');
      final service = PurchaseController(auth: auth, api: api, bridge: bridge);
      await t.pumpWidget(
        PurchaseLifecycle(
          purchases: service,
          child: Builder(
            builder: (c) {
              expect(bridge.stream.hasListener, isTrue);
              return const SizedBox();
            },
          ),
        ),
      );
      bridge.stream.add(bridge.paid());
      await t.pumpAndSettle();
      expect(api.calls, 1);
      expect(service.status, PurchaseFlowStatus.retry);
      t.binding.handleAppLifecycleStateChanged(AppLifecycleState.paused);
      api.answer = null;
      t.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
      await t.pumpAndSettle();
      expect(api.calls, 2);
      expect(bridge.finishes, 1);
      expect(bridge.buys, 0);
      await t.pumpWidget(const SizedBox());
      await t.pump();
      expect(bridge.stream.hasListener, isFalse);
      await bridge.stream.close();
    },
  );
}
