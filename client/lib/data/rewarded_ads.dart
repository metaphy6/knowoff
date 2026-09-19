import 'dart:async';
import 'dart:convert';
import 'package:flutter/foundation.dart';

final _sdkUnit = RegExp(r'^ca-app-pub-[0-9]{16}/([0-9]{10})$');
final _matchId = RegExp(
  r'^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$',
);

/// Provider callback identity differs from the full SDK load identifier.
String? rewardProviderUnit(String sdkUnit) =>
    _sdkUnit.firstMatch(sdkUnit)?.group(1);

class RewardClaim {
  const RewardClaim._(this.opaque, this.adUnit, this.expiresAt);
  final String opaque, adUnit;
  final DateTime expiresAt;
  static RewardClaim parse(
    Map<String, dynamic> data, {
    required String expectedUnit,
    required DateTime now,
  }) {
    const fields = {'claim', 'ad_unit', 'expires_at'};
    if (data.length != 3 ||
        !fields.every(data.containsKey) ||
        data.values.any((v) => v is! String)) {
      throw const FormatException('Invalid reward claim');
    }
    final opaque = data['claim'] as String,
        unit = data['ad_unit'] as String,
        expiry = data['expires_at'] as String;
    final parsed = DateTime.tryParse(expiry);
    if (!RegExp(r'^[A-Za-z0-9_-]{43}$').hasMatch(opaque) ||
        base64Url.encode(base64Url.decode('$opaque=')).replaceAll('=', '') !=
            opaque ||
        unit != expectedUnit ||
        !RegExp(
          r'^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d{1,9})?Z$',
        ).hasMatch(expiry) ||
        parsed == null ||
        parsed.toIso8601String().substring(0, 19) != expiry.substring(0, 19) ||
        !parsed.isAfter(now)) {
      throw const FormatException('Invalid reward claim');
    }
    return RewardClaim._(opaque, unit, parsed);
  }
}

abstract interface class RewardClaimTransport {
  String? get accountId;
  int get sessionGeneration;
  Future<Map<String, dynamic>> issue(String match, String providerUnit);
  Future<void> check(String match, RewardClaim claim);
  Future<void> refreshBonus();
}

abstract interface class LoadedRewardAd {
  Future<void> show({
    required void Function() onReward,
    required void Function() onClosed,
  });
  Future<void> dispose();
}

abstract interface class RewardAdPlatform {
  bool get supported;
  bool get privacyOptionsRequired;
  ValueListenable<bool> get formActivity;
  Future<bool> refreshConsent({bool Function()? stillCurrent});
  Future<bool> canRequestAds();
  Future<void> showPrivacyOptions();
  Future<LoadedRewardAd> load(
    String sdkUnit,
    RewardClaim claim, {
    required bool Function() stillCurrent,
  });
}

/// Closed, explicitly owned ad flow. Every native continuation is tied to the
/// same account generation and logical epoch. Reward callbacks only request a
/// server read; they never contain wallet authority.
class RewardedAdController extends ChangeNotifier {
  RewardedAdController(
    this._transport,
    this._platform, {
    required this.sdkAdUnit,
    DateTime Function()? now,
    this.timeout = const Duration(seconds: 30),
  }) : _now = now ?? DateTime.now;
  final RewardClaimTransport _transport;
  final RewardAdPlatform _platform;
  final String sdkAdUnit;
  final DateTime Function() _now;
  final Duration timeout;
  int _epoch = 0;
  bool _disposed = false, _failed = false, _overlay = false, _showing = false;
  Future<void>? _preparing;
  LoadedRewardAd? _ad;
  RewardClaim? _claim;
  String? _match;
  ({String? account, int generation})? _loadedScope;
  ({String? account, int generation}) get _scope =>
      (account: _transport.accountId, generation: _transport.sessionGeneration);
  bool get supported =>
      !_disposed &&
      _platform.supported &&
      rewardProviderUnit(sdkAdUnit) != null;
  bool get ready =>
      !_disposed &&
      _ad != null &&
      _loadedScope == _scope &&
      _claim!.expiresAt.isAfter(_now());
  bool get busy =>
      _preparing != null ||
      _showing ||
      _overlay ||
      _platform.formActivity.value;
  bool get failed => _failed;
  bool get privacyOptionsRequired =>
      supported && _platform.privacyOptionsRequired;
  void _notify() {
    if (!_disposed) notifyListeners();
  }

  bool _current(int epoch, ({String? account, int generation}) scope) =>
      !_disposed && epoch == _epoch && scope == _scope && scope.account != null;
  void _discard() {
    final ad = _ad;
    _ad = null;
    _claim = null;
    _match = null;
    _loadedScope = null;
    if (ad != null) unawaited(ad.dispose().catchError((Object _) {}));
  }

  void sessionChanged() {
    _epoch++;
    _failed = false;
    _discard();
    _notify();
  }

  Future<void> prepare(String match) {
    if (_preparing case final pending?) return pending;
    if (!supported ||
        _overlay ||
        _showing ||
        !_matchId.hasMatch(match) ||
        _scope.account == null) {
      return Future.value();
    }
    if (ready && _match == match) return Future.value();
    _discard();
    final epoch = ++_epoch, scope = _scope;
    late final Future<void> pending;
    pending = _prepare(match, epoch, scope).whenComplete(() {
      if (identical(_preparing, pending)) _preparing = null;
      _notify();
    });
    _preparing = pending;
    _notify();
    return pending;
  }

  Future<void> _prepare(
    String match,
    int epoch,
    ({String? account, int generation}) scope,
  ) async {
    try {
      final consent = await _platform
          .refreshConsent(stillCurrent: () => _current(epoch, scope))
          .timeout(timeout);
      if (!_current(epoch, scope) || !consent) return;
      final data = await _transport
          .issue(match, rewardProviderUnit(sdkAdUnit)!)
          .timeout(timeout);
      if (!_current(epoch, scope)) return;
      final claim = RewardClaim.parse(
        data,
        expectedUnit: rewardProviderUnit(sdkAdUnit)!,
        now: _now(),
      );
      if (!await _platform.canRequestAds().timeout(timeout) ||
          !_current(epoch, scope)) {
        return;
      }
      final loading = _platform
          .load(
            sdkAdUnit,
            claim,
            stillCurrent: () =>
                _current(epoch, scope) && claim.expiresAt.isAfter(_now()),
          )
          .then((ad) async {
            if (!_current(epoch, scope) || !claim.expiresAt.isAfter(_now())) {
              await ad.dispose();
              return null;
            }
            return ad;
          });
      final ad = await loading.timeout(timeout);
      if (ad == null) return;
      if (!_current(epoch, scope)) {
        await ad.dispose();
        return;
      }
      _ad = ad;
      _claim = claim;
      _match = match;
      _loadedScope = scope;
      _failed = false;
    } catch (_) {
      if (_current(epoch, scope)) {
        _epoch++;
        _failed = true;
        _discard();
      }
    }
  }

  /// Only call from an explicit player action. The server check is a fresh
  /// authorization point, not an atomic lock across the external SDK overlay.
  Future<void> show() async {
    if (_disposed || _showing || _overlay) return;
    if (!ready) {
      _discard();
      _notify();
      return;
    }
    _showing = true;
    final epoch = _epoch,
        scope = _scope,
        ad = _ad!,
        claim = _claim!,
        match = _match!;
    try {
      if (!await _platform.canRequestAds().timeout(timeout) ||
          !_current(epoch, scope)) {
        throw StateError('Unavailable');
      }
      await _transport.check(match, claim).timeout(timeout);
      if (!await _platform.canRequestAds().timeout(timeout) ||
          !_current(epoch, scope) ||
          !claim.expiresAt.isAfter(_now())) {
        throw StateError('Unavailable');
      }
      _ad = null;
      _claim = null;
      _match = null;
      _loadedScope = null;
      _overlay = true;
      var closed = false, rewarded = false;
      void close() {
        if (closed) return;
        closed = true;
        _overlay = false;
        if (_current(epoch, scope)) {
          unawaited(_transport.refreshBonus().catchError((Object _) {}));
        }
        unawaited(ad.dispose().catchError((Object _) {}));
        _notify();
      }

      try {
        await ad
            .show(
              onReward: () {
                if (!rewarded && _current(epoch, scope)) {
                  rewarded = true;
                  unawaited(
                    _transport.refreshBonus().catchError((Object _) {}),
                  );
                }
              },
              onClosed: close,
            )
            .timeout(timeout);
      } catch (_) {
        // A timed-out native show may still own an overlay. Its native callback
        // must release it; logical cancellation never authorizes a second show.
        if (_current(epoch, scope)) {
          _epoch++;
          _failed = true;
        }
      }
    } catch (_) {
      if (_current(epoch, scope)) _failed = true;
      _discard();
    } finally {
      _showing = false;
      _notify();
    }
  }

  Future<void> showPrivacyOptions() async {
    if (!supported || _overlay || _showing) return;
    sessionChanged();
    try {
      await _platform.showPrivacyOptions().timeout(timeout);
    } catch (_) {
      if (!_disposed) _failed = true;
    }
    _notify();
  }

  @override
  void dispose() {
    if (_disposed) return;
    _disposed = true;
    _epoch++;
    _discard();
    super.dispose();
  }
}
