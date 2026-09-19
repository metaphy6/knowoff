import 'dart:async';
import 'package:flutter/foundation.dart';
import '../core/config/rewarded_ads_config.dart';
import 'api_client.dart';
import 'auth_service.dart';
import 'bonus_session.dart';
import 'native_rewarded_bridge.dart';
import 'rewarded_ads.dart';

/// An immutable hint from the accepted socket identity, never payout authority.
@immutable
class RewardedMatchCandidate {
  const RewardedMatchCandidate({
    required this.matchId,
    required this.accountId,
    required this.generation,
  });
  final String matchId, accountId;
  final int generation;
  @override
  bool operator ==(Object other) =>
      other is RewardedMatchCandidate &&
      other.matchId == matchId &&
      other.accountId == accountId &&
      other.generation == generation;
  @override
  int get hashCode => Object.hash(matchId, accountId, generation);
}

enum RewardedOfferStatus { idle, preparing, ready, unavailable }

/// Owns native reward work for one app lifetime. Authentication and API services
/// are borrowed. Explicit events enqueue at most one latest candidate; notifier
/// rebuilds never create a retry loop.
class RewardedSessionController extends ChangeNotifier {
  RewardedSessionController(
    this._transport,
    this._platform, {
    required this.identityChanges,
    required String sdkAdUnit,
    DateTime Function()? now,
    this.timeout = const Duration(seconds: 30),
  }) : _ads = RewardedAdController(
         _transport,
         _platform,
         sdkAdUnit: sdkAdUnit,
         now: now,
         timeout: timeout,
       );
  factory RewardedSessionController.connected(
    AuthService auth,
    ApiClient api,
    BonusSessionController bonuses, {
    RewardedAdsConfig config = const RewardedAdsConfig(),
    RewardAdPlatform? platform,
  }) {
    if (!identical(auth, api.authService) ||
        !identical(auth.identityChanges, bonuses.identityChanges)) {
      throw ArgumentError('Reward identity mismatch');
    }
    return RewardedSessionController(
      ApiRewardClaimTransport(
        api,
        refreshBonus: (account, generation) async {
          if (bonuses.identity ==
              (accountId: account, generation: generation)) {
            await bonuses.refresh();
          }
        },
      ),
      platform ?? NativeRewardedBridge(enabled: config.enabled),
      identityChanges: auth.identityChanges,
      sdkAdUnit: config.unitFor(defaultTargetPlatform) ?? '',
    );
  }
  final RewardClaimTransport _transport;
  final RewardAdPlatform _platform;
  final RewardedAdController _ads;
  final ValueListenable<({String? accountId, int generation})> identityChanges;
  final Duration timeout;
  bool _started = false,
      _disposed = false,
      _foreground = true,
      _pending = false,
      _launched = false;
  int _lifecycleEpoch = 0;
  Future<void>? _launch, _running;
  RewardedMatchCandidate? _candidate;
  ({String? accountId, int generation})? _observedIdentity;
  ({String? accountId, int generation}) get identity => (
    accountId: _transport.accountId,
    generation: _transport.sessionGeneration,
  );
  RewardedMatchCandidate? get candidate => _candidate;
  bool get supported => !_disposed && _ads.supported;
  bool get ready => _active && _matches(_candidate) && _ads.ready;
  bool get busy =>
      !_disposed && (_launch != null || _running != null || _ads.busy);
  bool get failed => !_disposed && _ads.failed;
  bool get privacyOptionsRequired => supported && _ads.privacyOptionsRequired;
  RewardedOfferStatus get status => ready
      ? RewardedOfferStatus.ready
      : busy
      ? RewardedOfferStatus.preparing
      : _candidate == null
      ? RewardedOfferStatus.idle
      : RewardedOfferStatus.unavailable;
  bool get _active => _started && !_disposed && _foreground && supported;
  bool _matches(RewardedMatchCandidate? value) =>
      value != null &&
      value.accountId.isNotEmpty &&
      value.generation >= 0 &&
      identity == (accountId: value.accountId, generation: value.generation) &&
      RegExp(
        r'^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$',
      ).hasMatch(value.matchId);
  void _notify() {
    if (!_disposed) notifyListeners();
  }

  void start() {
    if (_started || _disposed) return;
    _started = true;
    _observedIdentity = identity;
    identityChanges.addListener(_identityChanged);
    _ads.addListener(_changed);
    _platform.formActivity.addListener(_changed);
    _launchConsent();
  }

  void _launchConsent() {
    if (!_active ||
        _launched ||
        _launch != null ||
        _platform.formActivity.value) {
      return;
    }
    _launched = true;
    final epoch = _lifecycleEpoch;
    var live = true;
    late final Future<void> launch;
    launch =
        Future<void>(() async {
          try {
            if (!_active || epoch != _lifecycleEpoch) return;
            await _platform
                .refreshConsent(
                  stillCurrent: () =>
                      live &&
                      !_disposed &&
                      _foreground &&
                      epoch == _lifecycleEpoch,
                )
                .timeout(timeout);
          } catch (_) {
            // Consent failure never authorizes an ad. A later explicit attempt can
            // ask the SDK again after the outstanding native callback completes.
          } finally {
            live = false;
          }
        }).whenComplete(() {
          if (identical(_launch, launch)) _launch = null;
          _changed();
        });
    _launch = launch;
    _notify();
  }

  void _identityChanged() {
    if (_disposed || _observedIdentity == identity) return;
    _observedIdentity = identity;
    _candidate = null;
    _pending = false;
    _ads.sessionChanged();
    _notify();
  }

  void _changed() {
    _launchConsent();
    _notify();
    if (_active &&
        _pending &&
        _running == null &&
        _launch == null &&
        !_ads.busy) {
      unawaited(_work());
    }
  }

  Future<void> offer(RewardedMatchCandidate value) {
    if (!_started || !supported || !_matches(value) || value == _candidate) {
      return Future.value();
    }
    _candidate = value;
    _pending = true;
    _ads.sessionChanged();
    return _work();
  }

  Future<void> retry() {
    if (!_active || !_matches(_candidate)) return Future.value();
    if (_running != null) return _running!;
    if (ready) return Future.value();
    _pending = true;
    return _work();
  }

  Future<void> _work() {
    if (_running case final pending?) return pending;
    if (!_active || !_pending) return Future.value();
    late final Future<void> operation;
    operation =
        Future<void>(() async {
          await _launch;
          while (_active && _pending && !_ads.busy) {
            final selected = _candidate;
            if (!_matches(selected)) {
              _pending = false;
              break;
            }
            _pending = false;
            await _ads.prepare(selected!.matchId);
          }
        }).whenComplete(() {
          if (identical(_running, operation)) _running = null;
          _changed();
        });
    _running = operation;
    _notify();
    return operation;
  }

  void setForeground(bool foreground) {
    if (_disposed || foreground == _foreground) return;
    _foreground = foreground;
    if (!foreground) {
      _lifecycleEpoch++;
      if (_launch != null) _launched = false;
      _pending = _matches(_candidate);
      _ads.sessionChanged();
    } else {
      _launchConsent();
      if (_matches(_candidate)) {
        _pending = true;
        unawaited(_work());
      }
    }
    _notify();
  }

  Future<void> show() async {
    if (ready) await _ads.show();
  }

  Future<void> showPrivacyOptions() async {
    if (!_active || busy) return;
    _pending = false;
    await _ads.showPrivacyOptions();
  }

  @override
  void dispose() {
    if (_disposed) return;
    _disposed = true;
    _pending = false;
    _candidate = null;
    if (_started) {
      identityChanges.removeListener(_identityChanged);
      _ads.removeListener(_changed);
      _platform.formActivity.removeListener(_changed);
    }
    _ads.dispose();
    super.dispose();
  }
}
