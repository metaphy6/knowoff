import 'package:flutter/foundation.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/core/config/rewarded_ads_config.dart';

void main() {
  test('ads configuration closed by default and on unsupported platforms', () {
    expect(const RewardedAdsConfig().unitFor(TargetPlatform.android), isNull);
    expect(
      const RewardedAdsConfig(enabled: true).unitFor(TargetPlatform.linux),
      isNull,
    );
  });
  test(
    'debug uses official units; release requires exact supplied real identities',
    () {
      expect(
        const RewardedAdsConfig(enabled: true).unitFor(TargetPlatform.android),
        'ca-app-pub-3940256099942544/5224354917',
      );
      expect(
        const RewardedAdsConfig(
          enabled: true,
          release: true,
        ).unitFor(TargetPlatform.android),
        isNull,
      );
      expect(
        const RewardedAdsConfig(
          enabled: true,
          release: true,
          androidAppId: 'ca-app-pub-1111111111111111~2222222222',
          androidUnit: 'ca-app-pub-1111111111111111/3333333333',
        ).unitFor(TargetPlatform.android),
        'ca-app-pub-1111111111111111/3333333333',
      );
      expect(
        const RewardedAdsConfig(
          enabled: true,
          release: true,
          androidAppId: 'ca-app-pub-1111111111111111~2222222222',
          androidUnit: 'ca-app-pub-9999999999999999/3333333333',
        ).unitFor(TargetPlatform.android),
        isNull,
      );
    },
  );
}
