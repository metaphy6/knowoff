import 'package:flutter/foundation.dart';

/// Native identities are build inputs, never mutable downloaded configuration.
/// Production stays closed unless a release build supplies matching real IDs.
class RewardedAdsConfig {
  const RewardedAdsConfig({
    this.enabled = const bool.fromEnvironment('KNOWOFF_REWARDED_ADS'),
    this.release = kReleaseMode,
    this.androidAppId = const String.fromEnvironment(
      'KNOWOFF_ADMOB_ANDROID_APP_ID',
    ),
    this.androidUnit = const String.fromEnvironment(
      'KNOWOFF_ADMOB_ANDROID_AD_UNIT',
    ),
    this.iosAppId = const String.fromEnvironment('KNOWOFF_ADMOB_IOS_APP_ID'),
    this.iosUnit = const String.fromEnvironment('KNOWOFF_ADMOB_IOS_AD_UNIT'),
  });
  final bool enabled, release;
  final String androidAppId, androidUnit, iosAppId, iosUnit;
  String? unitFor(TargetPlatform platform) {
    if (!enabled || kIsWeb) return null;
    final String app, unit;
    if (platform == TargetPlatform.android) {
      app = androidAppId;
      unit = androidUnit;
    } else if (platform == TargetPlatform.iOS) {
      app = iosAppId;
      unit = iosUnit;
    } else {
      return null;
    }
    if (!release && app.isEmpty && unit.isEmpty) {
      return platform == TargetPlatform.android
          ? 'ca-app-pub-3940256099942544/5224354917'
          : 'ca-app-pub-3940256099942544/1712485313';
    }
    final appMatch = RegExp(
      r'^ca-app-pub-([0-9]{16})~[0-9]{10}$',
    ).firstMatch(app);
    final unitMatch = RegExp(
      r'^ca-app-pub-([0-9]{16})/[0-9]{10}$',
    ).firstMatch(unit);
    if (appMatch == null ||
        unitMatch == null ||
        appMatch.group(1) != unitMatch.group(1) ||
        (release && appMatch.group(1) == '3940256099942544')) {
      return null;
    }
    return unit;
  }
}
