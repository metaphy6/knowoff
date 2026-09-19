import 'dart:async';
import 'package:flutter/foundation.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/data/rewarded_session.dart';
import 'package:knowoff_client/data/api_client.dart';
import 'package:knowoff_client/data/auth_service.dart';
import 'package:knowoff_client/data/bonus_session.dart';
import 'package:knowoff_client/data/bonus_delivery.dart';
import 'rewarded_ads_test.dart' as fixture;

const candidate = RewardedMatchCandidate(
  matchId: fixture.match,
  accountId: 'owner',
  generation: 1,
);

class Harness {
  final claims = fixture.Claims();
  final platform = fixture.Platform();
  final identity = ValueNotifier<({String? accountId, int generation})>((
    accountId: 'owner',
    generation: 1,
  ));
  late final session = RewardedSessionController(
    claims,
    platform,
    identityChanges: identity,
    sdkAdUnit: fixture.unit,
    now: () => fixture.at,
    timeout: const Duration(milliseconds: 20),
  );
  void change(String? account, int generation) {
    claims.accountId = account;
    claims.sessionGeneration = generation;
    identity.value = (accountId: account, generation: generation);
  }

  void dispose() {
    session.dispose();
    identity.dispose();
  }
}

class RefusedClaims extends fixture.Claims {
  @override
  Future<Map<String, dynamic>> issue(String match, String unit) async {
    issues++;
    throw StateError('server refused current Premium');
  }
}

void main() {
  test('resume cannot revive a pre-background launch continuation', () async {
    final h = Harness();
    h.platform.consent = Completer<bool>();
    h.session.start();
    await Future<void>.delayed(Duration.zero);
    final oldAuthority = h.platform.consentAuthority!;
    expect(oldAuthority(), true);
    h.session.setForeground(false);
    h.session.setForeground(true);
    expect(oldAuthority(), false);
    final pending = h.platform.consent!;
    h.platform.consent = null;
    pending.complete(true);
    await Future<void>.delayed(Duration.zero);
    await Future<void>.delayed(Duration.zero);
    expect(h.platform.forms, 2);
    expect(h.claims.issues, 0);
    h.dispose();
  });

  test(
    'background mount defers the single launch consent until resume',
    () async {
      final h = Harness();
      h.session.setForeground(false);
      h.session.start();
      await Future<void>.delayed(Duration.zero);
      expect(h.platform.forms, 0);
      expect(h.claims.issues, 0);
      h.session.setForeground(true);
      h.session.setForeground(true);
      await Future<void>.delayed(Duration.zero);
      expect(h.platform.forms, 1);
      expect(h.claims.issues, 0);
      h.dispose();
    },
  );

  test(
    'authoritative claim refusal renders no watch offer and loads no ad',
    () async {
      final t = RefusedClaims(), p = fixture.Platform();
      final identity = ValueNotifier<({String? accountId, int generation})>((
        accountId: 'owner',
        generation: 1,
      ));
      final c = RewardedSessionController(
        t,
        p,
        identityChanges: identity,
        sdkAdUnit: fixture.unit,
        now: () => fixture.at,
      );
      c.start();
      await c.offer(candidate);
      expect(t.issues, 1);
      expect(p.loads, 0);
      expect(c.ready, false);
      expect(c.status, RewardedOfferStatus.unavailable);
      c.dispose();
      identity.dispose();
    },
  );
  test('connected factory rejects mismatched borrowed account services', () {
    final a = AuthService(baseUrl: 'http://localhost'),
        b = AuthService(baseUrl: 'http://localhost');
    final api = ApiClient(baseUrl: 'http://localhost', auth: a);
    final bonuses = BonusSessionController.connected(
      b,
      ApiClient(baseUrl: 'http://localhost', auth: b),
      PreferencesBonusDismissals('http://localhost'),
    );
    expect(
      () => RewardedSessionController.connected(a, api, bonuses),
      throwsArgumentError,
    );
    bonuses.dispose();
  });
  test(
    'background retains native overlay ownership and suppresses late reward reads',
    () async {
      final h = Harness();
      h.session.start();
      await h.session.offer(candidate);
      await h.session.show();
      h.session.setForeground(false);
      expect(h.session.ready, false);
      expect(h.session.busy, true);
      h.session.setForeground(true);
      await h.session.retry();
      expect(h.platform.loads, 1);
      h.platform.ad.reward!();
      h.platform.ad.closed!();
      await Future<void>.delayed(Duration.zero);
      expect(h.claims.reads, 0);
      // One queued resume attempt may now load, but the old native overlay has closed.
      await h.session.retry();
      expect(h.platform.loads, 2);
      h.dispose();
    },
  );
  test(
    'privacy remains discoverable without a candidate and dispose removes observers',
    () async {
      final h = Harness();
      h.platform.privacyOptionsRequired = true;
      h.session.start();
      await Future<void>.delayed(Duration.zero);
      expect(h.session.privacyOptionsRequired, true);
      await h.session.showPrivacyOptions();
      expect(h.claims.issues, 0);
      expect(h.platform.allowed, false);
      h.session.dispose();
      h.change('other', 2);
      h.platform.formActivity.value = true;
      expect(h.session.supported, false);
      expect(h.session.ready, false);
      h.identity.dispose();
    },
  );
  test(
    'launch consent is coalesced and cannot bootstrap or issue a claim',
    () async {
      final h = Harness();
      h.change(null, 1);
      h.session.start();
      h.session.start();
      await Future<void>.delayed(Duration.zero);
      expect(h.platform.forms, 1);
      expect(h.claims.issues, 0);
      expect(h.platform.loads, 0);
      h.dispose();
    },
  );
  test(
    'only exact candidate loads once and explicit show refreshes server',
    () async {
      final h = Harness();
      h.session.start();
      await h.session.offer(
        const RewardedMatchCandidate(
          matchId: fixture.match,
          accountId: 'other',
          generation: 1,
        ),
      );
      expect(h.claims.issues, 0);
      await h.session.offer(candidate);
      await h.session.offer(candidate);
      expect(h.claims.issues, 1);
      expect(h.session.ready, true);
      expect(h.platform.ad.shows, 0);
      await h.session.show();
      h.platform.ad.reward!();
      h.platform.ad.closed!();
      expect(h.claims.reads, 2);
      expect(h.platform.ad.shows, 1);
      h.dispose();
    },
  );
  test(
    'background invalidates pending consent and resume retries once',
    () async {
      final h = Harness();
      h.platform.consent = Completer<bool>();
      h.session.start();
      final pending = h.session.offer(candidate);
      h.session.setForeground(false);
      h.platform.consent!.complete(true);
      await pending;
      expect(h.claims.issues, 0);
      expect(h.session.ready, false);
      h.platform.consent = null;
      h.session.setForeground(true);
      h.session.setForeground(true);
      await h.session.retry();
      expect(h.claims.issues, 1);
      expect(h.session.ready, true);
      h.dispose();
    },
  );
  test('same-account generation invalidates immutable old candidate', () async {
    final h = Harness();
    h.session.start();
    await h.session.offer(candidate);
    h.change('owner', 2);
    expect(h.session.ready, false);
    expect(h.session.candidate, isNull);
    await h.session.offer(candidate);
    expect(h.claims.issues, 1);
    h.dispose();
  });
  test(
    'latest candidate replaces paused prior attempt without lost work',
    () async {
      final h = Harness();
      h.session.start();
      await Future<void>.delayed(Duration.zero);
      h.platform.loading = Completer<fixture.Ad>();
      final first = h.session.offer(candidate);
      await Future<void>.delayed(Duration.zero);
      const next = RewardedMatchCandidate(
        matchId: '22222222-2222-4222-8222-222222222222',
        accountId: 'owner',
        generation: 1,
      );
      final replacement = h.session.offer(next);
      final old = h.platform.loading!;
      h.platform.loading = null;
      old.complete(fixture.Ad());
      await Future.wait([first, replacement]);
      expect(h.session.candidate, next);
      expect(h.session.ready, true);
      expect(h.claims.issues, 2);
      h.dispose();
    },
  );
  test(
    'pending native form stays busy after timeout and no notification retry',
    () async {
      final h = Harness();
      h.platform.consent = Completer<bool>();
      h.session.start();
      await Future<void>.delayed(Duration.zero);
      h.platform.formActivity.value = true;
      final pending = h.session.offer(candidate);
      await Future<void>.delayed(const Duration(milliseconds: 30));
      expect(h.session.busy, true);
      expect(h.claims.issues, 0);
      h.platform.consent!.complete(true);
      h.platform.consent = null;
      h.platform.formActivity.value = false;
      await pending;
      await Future<void>.delayed(Duration.zero);
      expect(h.claims.issues, 1);
      h.platform.formActivity.value = true;
      h.platform.formActivity.value = false;
      await Future<void>.delayed(Duration.zero);
      expect(h.claims.issues, 1);
      h.dispose();
    },
  );
  test(
    'unsupported and invalid candidates perform no platform or claim work',
    () async {
      final h = Harness();
      h.platform.supported = false;
      h.session.start();
      await h.session.offer(candidate);
      expect(h.platform.forms, 0);
      expect(h.claims.issues, 0);
      h.dispose();
      final b = Harness();
      b.session.start();
      await b.session.offer(
        const RewardedMatchCandidate(
          matchId: 'bad',
          accountId: 'owner',
          generation: 1,
        ),
      );
      expect(b.claims.issues, 0);
      expect(b.session.candidate, isNull);
      b.dispose();
    },
  );
}
