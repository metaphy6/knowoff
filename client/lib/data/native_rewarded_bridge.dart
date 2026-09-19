import 'dart:async';
import 'package:flutter/foundation.dart';
import 'package:google_mobile_ads/google_mobile_ads.dart';
import 'rewarded_ads.dart';

/// Native-only implementation. Constructing this adapter does not initialize
/// Mobile Ads. Native build startup is separately gated by platform manifests.
class NativeRewardedBridge implements RewardAdPlatform {
  NativeRewardedBridge({
    this.enabled = false,
    this.bindingTimeout = const Duration(seconds: 30),
  });
  final Duration bindingTimeout;
  final bool enabled;
  static final _forms = ValueNotifier<bool>(false);
  static bool get _formActive => _forms.value;
  static set _formActive(bool value) => _forms.value = value;
  @override
  ValueListenable<bool> get formActivity => _forms;
  bool _privacyRequired = false, _updated = false, _initialized = false;
  Future<void>? _initializing;
  @override
  bool get supported =>
      enabled &&
      !kIsWeb &&
      (defaultTargetPlatform == TargetPlatform.android ||
          defaultTargetPlatform == TargetPlatform.iOS);
  @override
  bool get privacyOptionsRequired => supported && _privacyRequired;
  Future<void> _privacyStatus() async {
    _privacyRequired =
        await ConsentInformation.instance
            .getPrivacyOptionsRequirementStatus() ==
        PrivacyOptionsRequirementStatus.required;
  }

  @override
  Future<bool> refreshConsent({bool Function()? stillCurrent}) async {
    if (!supported || _formActive || !(stillCurrent?.call() ?? true)) {
      return false;
    }
    _formActive = true;
    try {
      final update = Completer<void>();
      ConsentInformation.instance.requestConsentInfoUpdate(
        ConsentRequestParameters(),
        () => update.complete(),
        (_) => update.complete(),
      );
      await update.future;
      _updated = true;
      if (!(stillCurrent?.call() ?? true)) return false;
      final form = Completer<void>();
      await ConsentForm.loadAndShowConsentFormIfRequired((_) {
        if (!form.isCompleted) form.complete();
      });
      await form.future;
      await _privacyStatus();
      return await ConsentInformation.instance.canRequestAds();
    } catch (_) {
      return false;
    } finally {
      _formActive = false;
    }
  }

  @override
  Future<bool> canRequestAds() async =>
      supported &&
      _updated &&
      !_formActive &&
      await ConsentInformation.instance.canRequestAds();
  @override
  Future<void> showPrivacyOptions() async {
    if (!supported || _formActive) return;
    _formActive = true;
    try {
      final form = Completer<void>();
      await ConsentForm.showPrivacyOptionsForm((_) {
        if (!form.isCompleted) form.complete();
      });
      await form.future;
      await _privacyStatus();
    } finally {
      _formActive = false;
    }
  }

  Future<void> _initialize() async {
    if (_initialized) return;
    if (_initializing case final pending?) {
      await pending;
      return;
    }
    final pending = MobileAds.instance.initialize().then((_) {
      _initialized = true;
    });
    _initializing = pending;
    try {
      await pending;
    } finally {
      if (identical(_initializing, pending)) _initializing = null;
    }
  }

  @override
  Future<LoadedRewardAd> load(
    String sdkUnit,
    RewardClaim claim, {
    required bool Function() stillCurrent,
  }) async {
    if (!supported ||
        !stillCurrent() ||
        rewardProviderUnit(sdkUnit) != claim.adUnit ||
        !await canRequestAds()) {
      throw StateError('Reward unavailable');
    }
    if (!stillCurrent()) throw StateError('Reward unavailable');
    await _initialize();
    if (!stillCurrent() || !await canRequestAds() || !stillCurrent()) {
      throw StateError('Reward unavailable');
    }
    final result = Completer<LoadedRewardAd>();
    await RewardedAd.load(
      adUnitId: sdkUnit,
      request: const AdRequest(),
      rewardedAdLoadCallback: RewardedAdLoadCallback(
        onAdLoaded: (ad) async {
          try {
            if (!stillCurrent()) throw StateError('Reward unavailable');
            // Only the opaque server claim enters provider data. Never send an account,
            // installation identifier, access token or refresh token to the ad network.
            await ad
                .setServerSideOptions(
                  ServerSideVerificationOptions(customData: claim.opaque),
                )
                .timeout(bindingTimeout);
            if (!stillCurrent()) throw StateError('Reward unavailable');
            result.complete(_NativeLoadedRewardAd(ad));
          } catch (_) {
            // Dispatch cleanup even if the platform binding never answers. Do
            // not make logical completion depend on another platform response.
            unawaited(ad.dispose().catchError((Object _) {}));
            result.completeError(StateError('Reward unavailable'));
          }
        },
        onAdFailedToLoad: (_) =>
            result.completeError(StateError('Reward unavailable')),
      ),
    );
    return result.future;
  }
}

class _NativeLoadedRewardAd implements LoadedRewardAd {
  _NativeLoadedRewardAd(this.ad);
  final RewardedAd ad;
  bool _disposed = false, _shown = false;
  @override
  Future<void> show({
    required void Function() onReward,
    required void Function() onClosed,
  }) async {
    if (_disposed || _shown) throw StateError('Reward unavailable');
    _shown = true;
    var closed = false;
    void close() {
      if (!closed) {
        closed = true;
        onClosed();
      }
    }

    ad.fullScreenContentCallback = FullScreenContentCallback(
      onAdDismissedFullScreenContent: (_) => close(),
      onAdFailedToShowFullScreenContent: (_, __) => close(),
    );
    try {
      await ad.show(onUserEarnedReward: (_, __) => onReward());
    } catch (_) {
      close();
      rethrow;
    }
  }

  @override
  Future<void> dispose() async {
    if (_disposed) return;
    _disposed = true;
    await ad.dispose();
  }
}
