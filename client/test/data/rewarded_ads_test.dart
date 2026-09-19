import 'dart:async';
import 'dart:convert';
import 'package:flutter/foundation.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/data/rewarded_ads.dart';

const unit = 'ca-app-pub-3940256099942544/5224354917';
const match = '11111111-1111-4111-8111-111111111111';
final at = DateTime.utc(2026, 9, 19);
Map<String, dynamic> claimJson() => {
  'claim': base64Url.encode(List.filled(32, 7)).replaceAll('=', ''),
  'ad_unit': '5224354917',
  'expires_at': at.add(const Duration(minutes: 5)).toIso8601String(),
};

class Claims implements RewardClaimTransport {
  @override
  String? accountId = 'owner';
  @override
  int sessionGeneration = 1;
  int issues = 0, checks = 0, reads = 0;
  bool allowed = true;
  Completer<void>? checking;
  @override
  Future<Map<String, dynamic>> issue(String match, String unit) async {
    issues++;
    return claimJson();
  }

  @override
  Future<void> check(String match, RewardClaim claim) async {
    checks++;
    if (checking != null) await checking!.future;
    if (!allowed) throw StateError('refused');
  }

  @override
  Future<void> refreshBonus() async {
    reads++;
  }
}

class Ad implements LoadedRewardAd {
  int shows = 0, disposals = 0;
  void Function()? reward, closed;
  Completer<void>? showing;
  @override
  Future<void> show({
    required void Function() onReward,
    required void Function() onClosed,
  }) async {
    shows++;
    reward = onReward;
    closed = onClosed;
    if (showing != null) await showing!.future;
  }

  @override
  Future<void> dispose() async {
    disposals++;
  }
}

class Platform implements RewardAdPlatform {
  @override
  final ValueNotifier<bool> formActivity = ValueNotifier<bool>(false);
  @override
  bool supported = true;
  bool allowed = true;
  @override
  bool privacyOptionsRequired = false;
  int forms = 0, loads = 0;
  final ad = Ad();
  Completer<LoadedRewardAd>? loading;
  Completer<bool>? consent;
  bool Function()? consentAuthority;
  RewardClaim? loadedClaim;
  @override
  Future<bool> refreshConsent({bool Function()? stillCurrent}) async {
    forms++;
    consentAuthority = stillCurrent;
    return consent == null ? allowed : consent!.future;
  }

  @override
  Future<bool> canRequestAds() async => allowed;
  @override
  Future<void> showPrivacyOptions() async {
    allowed = false;
  }

  @override
  Future<LoadedRewardAd> load(
    String sdkUnit,
    RewardClaim claim, {
    required bool Function() stillCurrent,
  }) async {
    expectSync(sdkUnit, unit);
    loads++;
    loadedClaim = claim;
    return loading == null ? ad : loading!.future;
  }
}

void main() {
  test(
    'form and final server check continuations retain original generation',
    () async {
      for (final duringForm in [true, false]) {
        final t = Claims(), p = Platform();
        final c = RewardedAdController(t, p, sdkAdUnit: unit, now: () => at);
        Future<void> pending;
        if (duringForm) {
          p.consent = Completer<bool>();
          pending = c.prepare(match);
        } else {
          await c.prepare(match);
          t.checking = Completer<void>();
          pending = c.show();
        }
        await Future<void>.delayed(Duration.zero);
        t.sessionGeneration++;
        if (duringForm) {
          p.consent!.complete(true);
        } else {
          t.checking!.complete();
        }
        await pending;
        expect(p.ad.shows, 0);
        if (duringForm) expect(t.issues, 0);
        c.dispose();
      }
    },
  );
  test(
    'load timeout invalidates late ad and show timeout retains native overlay lock',
    () async {
      final t = Claims(), p = Platform()..loading = Completer<LoadedRewardAd>();
      final c = RewardedAdController(
        t,
        p,
        sdkAdUnit: unit,
        now: () => at,
        timeout: const Duration(milliseconds: 5),
      );
      await c.prepare(match);
      expect(c.failed, true);
      p.loading!.complete(p.ad);
      await Future<void>.delayed(Duration.zero);
      expect(p.ad.disposals, 1);
      expect(t.reads, 0);
      c.dispose();
      final t2 = Claims(), p2 = Platform();
      p2.ad.showing = Completer<void>();
      final c2 = RewardedAdController(
        t2,
        p2,
        sdkAdUnit: unit,
        now: () => at,
        timeout: const Duration(milliseconds: 5),
      );
      await c2.prepare(match);
      await c2.show();
      expect(c2.busy, true);
      await c2.prepare(match);
      expect(p2.loads, 1);
      p2.ad.reward!();
      expect(t2.reads, 0);
      p2.ad.closed!();
      p2.ad.showing!.complete();
      await Future<void>.delayed(Duration.zero);
      expect(c2.busy, false);
      expect(p2.ad.disposals, 1);
      c2.dispose();
    },
  );
  test('strict claim binds provider unit and expiry', () {
    expect(
      RewardClaim.parse(
        claimJson(),
        expectedUnit: '5224354917',
        now: at,
      ).adUnit,
      '5224354917',
    );
    for (final bad in [
      {...claimJson(), 'extra': 1},
      {...claimJson(), 'ad_unit': 'other'},
      {...claimJson(), 'claim': 'x'},
      {...claimJson(), 'expires_at': '2027-99-99T00:00:00Z'},
      {...claimJson(), 'expires_at': at.toIso8601String()},
    ]) {
      expect(
        () => RewardClaim.parse(bad, expectedUnit: '5224354917', now: at),
        throwsFormatException,
      );
    }
  });
  test('exact claim loaded once; callback only reads server', () async {
    final transport = Claims(), platform = Platform();
    final c = RewardedAdController(
      transport,
      platform,
      sdkAdUnit: unit,
      now: () => at,
    );
    await Future.wait([c.prepare(match), c.prepare(match)]);
    expect(platform.loads, 1);
    expect(transport.issues, 1);
    expect(c.ready, true);
    final bound = platform.loadedClaim;
    await Future.wait([c.show(), c.show()]);
    expect(platform.ad.shows, 1);
    expect(transport.checks, 1);
    expect(identical(bound, platform.loadedClaim), true);
    platform.ad.reward!();
    await Future<void>.delayed(Duration.zero);
    expect(transport.reads, 1);
    platform.ad.closed!();
    await Future<void>.delayed(Duration.zero);
    expect(platform.ad.disposals, 1);
    expect(transport.reads, 2);
    platform.ad.closed!();
    expect(transport.reads, 2);
    c.dispose();
  });
  for (final reason in ['consent', 'scope', 'expiry', 'server']) {
    test('show refuses changed $reason after loading', () async {
      final transport = Claims(), platform = Platform();
      var now = at;
      final c = RewardedAdController(
        transport,
        platform,
        sdkAdUnit: unit,
        now: () => now,
      );
      await c.prepare(match);
      if (reason == 'consent') platform.allowed = false;
      if (reason == 'scope') transport.sessionGeneration++;
      if (reason == 'expiry') now = at.add(const Duration(minutes: 5));
      if (reason == 'server') transport.allowed = false;
      await c.show();
      expect(platform.ad.shows, 0);
      expect(c.ready, false);
      expect(platform.ad.disposals, 1);
      c.dispose();
    });
  }
  test('late load after account change disposed and cannot display', () async {
    final t = Claims(), p = Platform()..loading = Completer<LoadedRewardAd>();
    final c = RewardedAdController(t, p, sdkAdUnit: unit, now: () => at);
    final pending = c.prepare(match);
    await Future<void>.delayed(Duration.zero);
    t.accountId = 'other';
    c.sessionChanged();
    p.loading!.complete(p.ad);
    await pending;
    expect(p.ad.disposals, 1);
    expect(c.ready, false);
    c.dispose();
  });
  test(
    'stale shown callbacks cannot read new account and no second overlay',
    () async {
      final t = Claims(), p = Platform();
      final c = RewardedAdController(t, p, sdkAdUnit: unit, now: () => at);
      await c.prepare(match);
      await c.show();
      t.sessionGeneration++;
      c.sessionChanged();
      await c.prepare(match);
      expect(p.loads, 1);
      p.ad.reward!();
      expect(t.reads, 0);
      p.ad.closed!();
      await Future<void>.delayed(Duration.zero);
      expect(p.ad.disposals, 1);
      expect(t.reads, 0);
      c.dispose();
    },
  );
  test(
    'denied consent and unsupported platforms do not issue claims or load',
    () async {
      for (final unsupported in [true, false]) {
        final t = Claims(),
            p = Platform()
              ..supported = !unsupported
              ..allowed = false;
        final c = RewardedAdController(t, p, sdkAdUnit: unit, now: () => at);
        await c.prepare(match);
        expect(t.issues, 0);
        expect(p.loads, 0);
        c.dispose();
      }
    },
  );
  test('privacy options invalidate loaded claim before form', () async {
    final t = Claims(), p = Platform();
    final c = RewardedAdController(t, p, sdkAdUnit: unit, now: () => at);
    await c.prepare(match);
    await c.showPrivacyOptions();
    expect(c.ready, false);
    expect(p.ad.disposals, 1);
    await c.show();
    expect(p.ad.shows, 0);
    c.dispose();
  });
}
