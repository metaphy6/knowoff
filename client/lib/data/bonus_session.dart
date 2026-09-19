import 'dart:async';
import 'package:flutter/foundation.dart';
import 'auth_service.dart';
import 'api_client.dart';
import 'bonus_delivery.dart';

/// App-owned recovery, independent of ad SDK availability. The application must
/// explicitly start it after the joined reward surface is enabled.
class BonusSessionController extends ChangeNotifier {
  BonusSessionController(this._auth, this.deliveries);
  factory BonusSessionController.connected(
    AuthService auth,
    ApiClient api,
    BonusDismissalStorage storage,
  ) {
    if (!identical(auth, api.authService)) {
      throw ArgumentError('Reward identity mismatch');
    }
    late final BonusSessionController session;
    final transport = ApiBonusDeliveryTransport(
      api,
      refreshBalance: (account, generation) async {
        await api.getExistingSessionWallet(account, generation);
        session.walletRefreshed(account, generation);
      },
    );
    session = BonusSessionController(
      auth,
      BonusDeliveryController(transport, storage),
    );
    return session;
  }

  final AuthService _auth;
  final BonusDeliveryController deliveries;
  final _terminalMatches = <String>{};
  Future<void>? _running;
  bool _started = false, _disposed = false, _claiming = false, _again = false;
  bool _failed = false;
  int _walletRevision = 0;
  int get walletRevision => _disposed ? 0 : _walletRevision;
  ({String? accountId, int generation}) get identity => _scope;
  ValueListenable<({String? accountId, int generation})> get identityChanges =>
      _auth.identityChanges;

  /// Called only after a validated server wallet read for this exact identity.
  void walletRefreshed(String account, int generation) {
    if (!_started ||
        _disposed ||
        _scope != (accountId: account, generation: generation)) {
      return;
    }
    _walletRevision++;
    notifyListeners();
  }

  ({String? accountId, int generation})? _failureScope;
  ({String? accountId, int generation}) get _scope =>
      (accountId: _auth.accountId, generation: _auth.sessionGeneration);
  bool get failed => !_disposed && _failureScope == _scope && _failed;

  void start() {
    if (_started || _disposed) return;
    _started = true;
    _auth.identityChanges.addListener(_sessionChanged);
    unawaited(refresh());
  }

  void _sessionChanged() {
    // Invalidate presentation before any asynchronous restoration or read.
    deliveries.sessionChanged();
    _terminalMatches.clear();
    _walletRevision = 0;
    _failed = false;
    _failureScope = null;
    if (_disposed) return;
    notifyListeners();
    if (_running != null) {
      if (_claiming) _again = true;
    } else if (_auth.accountId != null) {
      unawaited(refresh());
    }
  }

  Future<void> refresh() {
    if (!_started || _disposed) return Future<void>.value();
    if (_running case final current?) return current;
    late final Future<void> operation;
    operation = _read().whenComplete(() {
      if (identical(_running, operation)) _running = null;
    });
    return _running = operation;
  }

  Future<void> _read() async {
    do {
      _again = false;
      var scope = _scope;
      try {
        final restored = await _auth.restoreExistingSession();
        if (_disposed || !restored) return;
        scope = _scope;
        if (scope.accountId == null) return;
        _claiming = true;
        await deliveries.refresh();
        if (!_disposed && scope == _scope) {
          _failed = false;
          _failureScope = scope;
        }
      } catch (_) {
        // Only stable presentation state is exposed; transport/credential
        // details never become UI copy or background unhandled exceptions.
        if (!_disposed && scope == _scope && scope.accountId != null) {
          _failed = true;
          _failureScope = scope;
        } else if (!_disposed && _auth.accountId != null && scope != _scope) {
          _again = true;
        }
      } finally {
        _claiming = false;
        if (!_disposed) notifyListeners();
      }
    } while (_again && !_disposed);
  }

  Future<void> dismiss(String id) async {
    if (!_started || _disposed) return;
    final scope = _scope;
    try {
      await deliveries.dismiss(id);
      if (!_disposed && scope == _scope) {
        _failed = false;
        _failureScope = scope;
      }
    } catch (_) {
      if (!_disposed && scope == _scope && scope.accountId != null) {
        _failed = true;
        _failureScope = scope;
      }
    } finally {
      if (!_disposed) notifyListeners();
    }
  }

  /// Called once per accepted terminal match, not for each snapshot rebuild.
  void completedMatch(String matchId) {
    if (!_started ||
        _disposed ||
        matchId.isEmpty ||
        !_terminalMatches.add(matchId)) {
      return;
    }
    if (_terminalMatches.length > 20) {
      _terminalMatches.remove(_terminalMatches.first);
    }
    if (_running != null && _claiming) _again = true;
    unawaited(refresh());
  }

  @override
  void dispose() {
    if (_disposed) return;
    _disposed = true;
    if (_started) _auth.identityChanges.removeListener(_sessionChanged);
    _terminalMatches.clear();
    // The service container owns the delivery controller separately.
    deliveries.sessionChanged();
    super.dispose();
  }
}
