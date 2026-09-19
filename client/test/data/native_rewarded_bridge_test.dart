import 'dart:async';
import 'dart:convert';
import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:google_mobile_ads/google_mobile_ads.dart';
// Pinned SDK event codec exercises actual load/bind/show channel calls.
// ignore: implementation_imports
import 'package:google_mobile_ads/src/ad_instance_manager.dart'
    show AdMessageCodec;
// Exact pinned plugin channel codecs exercise the real SDK adapter boundary.
// ignore: implementation_imports
import 'package:google_mobile_ads/src/ump/user_messaging_codec.dart';
import 'package:knowoff_client/data/native_rewarded_bridge.dart';
import 'package:knowoff_client/data/rewarded_ads.dart';
import 'rewarded_ads_test.dart' as fixture;

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  final messenger =
      TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
  final ump = MethodChannel(
    'plugins.flutter.io/google_mobile_ads/ump',
    StandardMethodCodec(UserMessagingCodec()),
  );
  final ads = MethodChannel(
    'plugins.flutter.io/google_mobile_ads',
    StandardMethodCodec(AdMessageCodec()),
  );
  setUp(() => debugDefaultTargetPlatformOverride = TargetPlatform.android);
  tearDown(() {
    debugDefaultTargetPlatformOverride = null;
    messenger.setMockMethodCallHandler(ump, null);
    messenger.setMockMethodCallHandler(ads, null);
  });
  for (final change in ['account', 'generation', 'expiry']) {
    test(
      'paused SDK initialize refuses changed $change before ad request',
      () async {
        final entered = Completer<void>(), release = Completer<void>();
        var loads = 0;
        messenger.setMockMethodCallHandler(ump, (call) async {
          if (call.method == 'ConsentInformation#canRequestAds') return true;
          if (call.method ==
              'ConsentInformation#getPrivacyOptionsRequirementStatus') {
            return 0;
          }
          return null;
        });
        messenger.setMockMethodCallHandler(ads, (call) async {
          if (call.method == 'MobileAds#initialize') {
            entered.complete();
            await release.future;
            return InitializationStatus(<String, AdapterStatus>{});
          }
          if (call.method == 'loadRewardedAd') loads++;
          return null;
        });
        var now = fixture.at;
        final transport = fixture.Claims();
        final controller = RewardedAdController(
          transport,
          NativeRewardedBridge(enabled: true),
          sdkAdUnit: fixture.unit,
          now: () => now,
          timeout: const Duration(milliseconds: 100),
        );
        final pending = controller.prepare(fixture.match);
        await entered.future;
        if (change == 'account') transport.accountId = 'other';
        if (change == 'generation') transport.sessionGeneration++;
        if (change == 'expiry') now = now.add(const Duration(minutes: 5));
        release.complete();
        await pending;
        expect(loads, 0);
        expect(controller.ready, false);
        controller.dispose();
      },
    );
  }
  test('stale consent update never dispatches a new form', () async {
    final entered = Completer<void>(), release = Completer<void>();
    var forms = 0;
    messenger.setMockMethodCallHandler(ump, (call) async {
      if (call.method == 'ConsentInformation#requestConsentInfoUpdate') {
        entered.complete();
        await release.future;
      }
      if (call.method ==
          'UserMessagingPlatform#loadAndShowConsentFormIfRequired') {
        forms++;
      }
      if (call.method == 'ConsentInformation#canRequestAds') return true;
      if (call.method ==
          'ConsentInformation#getPrivacyOptionsRequirementStatus') {
        return 0;
      }
      return null;
    });
    final transport = fixture.Claims();
    final controller = RewardedAdController(
      transport,
      NativeRewardedBridge(enabled: true),
      sdkAdUnit: fixture.unit,
      now: () => fixture.at,
    );
    final pending = controller.prepare(fixture.match);
    await entered.future;
    transport.sessionGeneration++;
    release.complete();
    await pending;
    expect(forms, 0);
    expect(transport.issues, 0);
    controller.dispose();
  });
  test('stalled native SSV binding disposes the exact loaded ad', () async {
    final entered = Completer<void>();
    final never = Completer<Object?>();
    int? loadedId;
    final disposed = <int>[];
    messenger.setMockMethodCallHandler(ump, (call) async {
      if (call.method == 'ConsentInformation#canRequestAds') return true;
      if (call.method ==
          'ConsentInformation#getPrivacyOptionsRequirementStatus') {
        return 0;
      }
      return null;
    });
    messenger.setMockMethodCallHandler(ads, (call) async {
      if (call.method == 'MobileAds#initialize') {
        return InitializationStatus(<String, AdapterStatus>{});
      }
      if (call.method == 'loadRewardedAd') {
        loadedId = call.arguments['adId'] as int;
        scheduleMicrotask(() {
          // ignore: deprecated_member_use
          messenger.handlePlatformMessage(
            ads.name,
            ads.codec.encodeMethodCall(
              MethodCall('onAdEvent', {
                'adId': loadedId,
                'eventName': 'onAdLoaded',
              }),
            ),
            (_) {},
          );
        });
      }
      if (call.method == 'setServerSideVerificationOptions') {
        entered.complete();
        return never.future;
      }
      if (call.method == 'disposeAd') {
        disposed.add(call.arguments['adId'] as int);
      }
      return null;
    });
    final bridge = NativeRewardedBridge(
      enabled: true,
      bindingTimeout: const Duration(milliseconds: 5),
    );
    await bridge.refreshConsent();
    final pending = bridge.load(
      fixture.unit,
      RewardClaim.parse(
        fixture.claimJson(),
        expectedUnit: '5224354917',
        now: fixture.at,
      ),
      stillCurrent: () => true,
    );
    final failure = expectLater(pending, throwsStateError);
    await entered.future;
    await failure.timeout(const Duration(seconds: 1));
    expect(disposed, [loadedId]);
    never.complete(null);
    await Future<void>.delayed(Duration.zero);
    expect(disposed, [loadedId]);
  });
  test(
    'real SDK binds exact custom data before ready then shows and disposes once',
    () async {
      messenger.setMockMethodCallHandler(ump, (call) async {
        if (call.method == 'ConsentInformation#canRequestAds') return true;
        if (call.method ==
            'ConsentInformation#getPrivacyOptionsRequirementStatus') {
          return 0;
        }
        return null;
      });
      final calls = <String>[];
      final bound = Completer<void>(), release = Completer<void>();
      int? id;
      String? custom;
      Future<void> event(String name) async {
        final done = Completer<void>();
        // The pinned plugin owns the platform-event handler; simulate its native peer.
        // ignore: deprecated_member_use
        await messenger.handlePlatformMessage(
          ads.name,
          ads.codec.encodeMethodCall(
            MethodCall('onAdEvent', {'adId': id, 'eventName': name}),
          ),
          (_) => done.complete(),
        );
        await done.future;
      }

      messenger.setMockMethodCallHandler(ads, (call) async {
        calls.add(call.method);
        if (call.method == 'MobileAds#initialize') {
          return InitializationStatus(<String, AdapterStatus>{});
        }
        if (call.method == 'loadRewardedAd') {
          expect(
            call.arguments['adUnitId'],
            'ca-app-pub-3940256099942544/5224354917',
          );
          id = call.arguments['adId'] as int;
          scheduleMicrotask(() => unawaited(event('onAdLoaded')));
        }
        if (call.method == 'setServerSideVerificationOptions') {
          final options =
              call.arguments['serverSideVerificationOptions']
                  as ServerSideVerificationOptions;
          expect(options.userId, isNull);
          custom = options.customData;
          bound.complete();
          await release.future;
        }
        return null;
      });
      final p = NativeRewardedBridge(enabled: true);
      await p.refreshConsent();
      expect(
        calls,
        isEmpty,
        reason: 'Consent refresh cannot initialize or request ads',
      );
      final raw = base64Url.encode(List.filled(32, 7)).replaceAll('=', '');
      final claim = RewardClaim.parse(
        {
          'claim': raw,
          'ad_unit': '5224354917',
          'expires_at': DateTime.now()
              .toUtc()
              .add(const Duration(minutes: 5))
              .toIso8601String(),
        },
        expectedUnit: '5224354917',
        now: DateTime.now(),
      );
      var ready = false;
      final loading = p
          .load(
            'ca-app-pub-3940256099942544/5224354917',
            claim,
            stillCurrent: () => true,
          )
          .then((ad) {
            ready = true;
            return ad;
          });
      await bound.future.timeout(const Duration(seconds: 2));
      expect(ready, false);
      expect(custom, raw);
      release.complete();
      final ad = await loading;
      var closed = 0;
      await ad.show(onReward: () {}, onClosed: () => closed++);
      await event('onAdDismissedFullScreenContent');
      expect(closed, 1);
      await ad.dispose();
      await ad.dispose();
      expect(calls.where((v) => v == 'disposeAd').length, 1);
      expect(
        calls.indexOf('MobileAds#initialize'),
        lessThan(calls.indexOf('loadRewardedAd')),
      );
      expect(
        calls.indexOf('setServerSideVerificationOptions'),
        lessThan(calls.indexOf('showAdWithoutView')),
      );
    },
  );
  test(
    'UMP refresh/form/live permission and privacy options use actual plugin',
    () async {
      final calls = <String>[];
      var allowed = true;
      messenger.setMockMethodCallHandler(ump, (call) async {
        calls.add(call.method);
        if (call.method == 'ConsentInformation#canRequestAds') return allowed;
        if (call.method ==
            'ConsentInformation#getPrivacyOptionsRequirementStatus') {
          return 1;
        }
        return null;
      });
      final p = NativeRewardedBridge(enabled: true);
      expect(await p.refreshConsent(), true);
      expect(p.privacyOptionsRequired, true);
      expect(calls.take(2), [
        'ConsentInformation#requestConsentInfoUpdate',
        'UserMessagingPlatform#loadAndShowConsentFormIfRequired',
      ]);
      allowed = false;
      expect(await p.canRequestAds(), false);
      await p.showPrivacyOptions();
      expect(calls, contains('UserMessagingPlatform#showPrivacyOptionsForm'));
    },
  );
  test(
    'pending native form prevents another form after caller timeout',
    () async {
      final native = Completer<Object?>();
      var forms = 0;
      messenger.setMockMethodCallHandler(ump, (call) async {
        if (call.method ==
            'UserMessagingPlatform#loadAndShowConsentFormIfRequired') {
          forms++;
          return native.future;
        }
        if (call.method == 'ConsentInformation#canRequestAds') return false;
        if (call.method ==
            'ConsentInformation#getPrivacyOptionsRequirementStatus') {
          return 0;
        }
        return null;
      });
      final p = NativeRewardedBridge(enabled: true);
      final pending = p.refreshConsent();
      await Future<void>.delayed(Duration.zero);
      await expectLater(
        pending.timeout(const Duration(milliseconds: 1)),
        throwsA(isA<TimeoutException>()),
      );
      expect(await p.refreshConsent(), false);
      await p.showPrivacyOptions();
      expect(forms, 1);
      native.complete(null);
      expect(await pending, false);
    },
  );
  test('disabled and unsupported do zero platform work', () async {
    var calls = 0;
    messenger.setMockMethodCallHandler(ump, (_) async {
      calls++;
      return false;
    });
    for (final enabled in [false, true]) {
      if (enabled) debugDefaultTargetPlatformOverride = TargetPlatform.linux;
      final p = NativeRewardedBridge(enabled: enabled);
      expect(p.supported, false);
      expect(await p.refreshConsent(), false);
      expect(await p.canRequestAds(), false);
      await p.showPrivacyOptions();
    }
    expect(calls, 0);
  });
}
