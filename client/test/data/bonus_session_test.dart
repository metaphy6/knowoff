import 'dart:convert';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:knowoff_client/data/api_client.dart';
import 'dart:async';
import 'package:flutter/foundation.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/data/auth_service.dart';
import 'package:knowoff_client/data/bonus_delivery.dart';
import 'package:knowoff_client/data/bonus_session.dart';
import 'bonus_delivery_test.dart'
    show
        BonusAuth,
        Transport,
        MemoryDismissals,
        account,
        otherAccount,
        delivery,
        page,
        matchId;

class SessionAuth extends AuthService {
  SessionAuth(this.transport) : super(baseUrl: 'https://game.example');
  final Transport transport;
  final identity = ValueNotifier<({String? accountId, int generation})>((
    accountId: account,
    generation: 0,
  ));
  int restores = 0;
  @override
  String? get accountId => transport.accountId;
  @override
  int get sessionGeneration => transport.sessionGeneration;
  @override
  ValueListenable<({String? accountId, int generation})> get identityChanges =>
      identity;
  @override
  Future<bool> restoreExistingSession() async {
    restores++;
    return accountId != null;
  }

  @override
  Future<void> ensureSession() async =>
      throw StateError('must not create an identity');
  void switchAccount(String? id) {
    transport.accountId = id;
    transport.sessionGeneration++;
    identity.value = (accountId: id, generation: transport.sessionGeneration);
  }
}

void main() {
  test(
    'connected session reads authoritative wallet and emits a scoped revision',
    () async {
      final auth = BonusAuth();
      final paths = <String>[];
      final api = ApiClient(
        baseUrl: 'https://game.example',
        auth: auth,
        client: MockClient((r) async {
          paths.add(r.url.path);
          if (r.url.path.endsWith('/claim')) {
            return http.Response(
              jsonEncode(page(deliveries: [delivery()])),
              200,
            );
          }
          return http.Response(
            jsonEncode({
              'balance': 12,
              'noin': 12,
              'daily_earned': 5,
              'daily_earn_cap': 50,
              'points_to_noin': 100,
              'free_daily_matches': 3,
              'non_converted_points': 0,
            }),
            200,
          );
        }),
      );
      final session = BonusSessionController.connected(
        auth,
        api,
        MemoryDismissals(),
      );
      expect(paths, isEmpty);
      session.start();
      await session.refresh();
      expect(paths, ['/v2/rewards/bonuses/claim', '/api/economy/wallet']);
      expect(session.walletRevision, 1);
      expect(session.deliveries.receipts.single.payload.credited, 5);
      session.dispose();
      session.deliveries.dispose();
    },
  );

  test('wallet revision accepts only the captured current identity', () async {
    final transport = Transport();
    final auth = SessionAuth(transport);
    final deliveries = BonusDeliveryController(transport, MemoryDismissals());
    final session = BonusSessionController(auth, deliveries)..start();
    await session.refresh();
    session.walletRefreshed(account, 0);
    expect(session.walletRevision, 1);
    auth.switchAccount(account);
    expect(session.walletRevision, 0);
    session.walletRefreshed(account, 0);
    expect(session.walletRevision, 0);
    session.walletRefreshed(account, 1);
    expect(session.walletRevision, 1);
    session.dispose();
    deliveries.dispose();
  });

  test(
    'lifecycle coalesces reads and deduplicates terminal match snapshots',
    () async {
      final transport = Transport();
      final auth = SessionAuth(transport);
      final deliveries = BonusDeliveryController(transport, MemoryDismissals());
      final session = BonusSessionController(auth, deliveries);
      final pending = Completer<Map<String, dynamic>>();
      transport.onClaim = () => pending.future;
      session.start();
      session.start();
      await Future<void>.delayed(Duration.zero);
      final refresh = session.refresh();
      session.completedMatch(matchId);
      session.completedMatch(matchId);
      expect(transport.claims, 1);
      pending.complete(page());
      await refresh;
      transport.onClaim = null;
      session.completedMatch(matchId);
      await Future<void>.delayed(Duration.zero);
      expect(transport.claims, 2);
      session.completedMatch('55555555-5555-4555-8555-555555555555');
      await session.refresh();
      expect(transport.claims, 3);
      session.dispose();
      deliveries.dispose();
    },
  );

  test(
    'account switch hides immediately and rejects delayed old receipt',
    () async {
      final transport = Transport()..answer = page(deliveries: [delivery()]);
      final auth = SessionAuth(transport);
      final deliveries = BonusDeliveryController(transport, MemoryDismissals());
      final session = BonusSessionController(auth, deliveries)..start();
      await session.refresh();
      expect(deliveries.receipts, hasLength(1));
      final pending = Completer<Map<String, dynamic>>();
      transport.onClaim = () => pending.future;
      final refresh = session.refresh();
      await Future<void>.delayed(Duration.zero);
      auth.switchAccount(otherAccount);
      expect(deliveries.receipts, isEmpty);
      transport.onClaim = null;
      transport.answer = page();
      pending.complete(page(deliveries: [delivery()]));
      await refresh;
      expect(deliveries.receipts, isEmpty);
      expect(transport.balances, 1);
      session.dispose();
      deliveries.dispose();
    },
  );

  test(
    'background refusal retries without bootstrap and disposal stops future work',
    () async {
      final transport = Transport()..accountId = null;
      final auth = SessionAuth(transport);
      final deliveries = BonusDeliveryController(transport, MemoryDismissals());
      final session = BonusSessionController(auth, deliveries)..start();
      await session.refresh();
      expect(transport.claims, 0);
      auth.switchAccount(account);
      transport.onClaim = () async =>
          throw StateError('private network detail');
      await session.refresh();
      expect(session.failed, isTrue);
      transport.onClaim = null;
      await session.refresh();
      expect(session.failed, isFalse);
      final count = transport.claims;
      session.dispose();
      auth.switchAccount(otherAccount);
      await session.refresh();
      expect(transport.claims, count);
      deliveries.dispose();
    },
  );
  test(
    'disposal during a pending claim rejects its receipt and queued refresh',
    () async {
      final transport = Transport();
      final auth = SessionAuth(transport);
      final deliveries = BonusDeliveryController(transport, MemoryDismissals());
      final session = BonusSessionController(auth, deliveries);
      final pending = Completer<Map<String, dynamic>>();
      transport.onClaim = () => pending.future;
      session.start();
      await Future<void>.delayed(Duration.zero);
      final refresh = session.refresh();
      session.completedMatch(matchId);
      session.dispose();
      pending.complete(page(deliveries: [delivery()]));
      await refresh;
      expect(deliveries.receipts, isEmpty);
      expect(transport.claims, 1);
      expect(transport.balances, 0);
      deliveries.dispose();
    },
  );
  test(
    'dismissal failure stays retryable without redisplaying an acknowledged value',
    () async {
      final transport = Transport()..answer = page(deliveries: [delivery()]);
      final auth = SessionAuth(transport);
      final deliveries = BonusDeliveryController(transport, MemoryDismissals());
      final session = BonusSessionController(auth, deliveries)..start();
      await session.refresh();
      transport.failAck = true;
      await session.dismiss(deliveries.receipts.single.id);
      expect(session.failed, isTrue);
      expect(deliveries.receipts, isEmpty);
      transport.failAck = false;
      await session.refresh();
      expect(session.failed, isFalse);
      expect(deliveries.receipts, isEmpty);
      session.dispose();
      deliveries.dispose();
    },
  );
}
