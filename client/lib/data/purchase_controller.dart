import 'dart:async';
import 'package:flutter/foundation.dart';
import 'api_client.dart';
import 'auth_service.dart';
import 'purchase_bridge.dart';
import 'purchases.dart';

enum PurchaseFlowStatus {
  idle,
  unavailable,
  loading,
  opening,
  pending,
  verifying,
  verified,
  canceled,
  failed,
  retry,
  accountRequired,
}

class _ReceiptWork {
  _ReceiptWork(this.event);
  final NativePurchaseEvent event;
  bool running = false, verified = false, done = false;
  Future<void>? task;
}

/// App-owned transaction processing. Route disposal only removes a UI listener.
/// The stores retain unfinished transactions across process death; this queue
/// retains exact proof across transient HTTP/UI failures without disk secrets.
class PurchaseController extends ChangeNotifier {
  PurchaseController({
    required this._auth,
    required this._api,
    required this._bridge,
  });
  final AuthService _auth;
  final ApiClient _api;
  final PurchaseBridge _bridge;
  StreamSubscription<NativePurchaseEvent>? _subscription;
  final _work = <String, _ReceiptWork>{};
  final _completed = <String>{};
  Future<void> _processing = Future<void>.value();
  Future<void>? _retrying;
  List<PurchaseOffer> _offers = const [];
  String? _offerAccount, _statusAccount;
  bool _disposed = false, _operation = false;
  bool? _premiumActive;
  bool get premiumOwned =>
      _offerAccount == _auth.accountId && _premiumActive == true;
  bool get canBuySubscription =>
      _offerAccount == _auth.accountId && _premiumActive == false;
  int _loadGeneration = 0, _verifiedRevision = 0;
  PurchaseFlowStatus _status = PurchaseFlowStatus.idle;
  List<PurchaseOffer> get offers =>
      _offerAccount == _auth.accountId ? _offers : const [];
  PurchaseFlowStatus get status =>
      _statusAccount != null && _statusAccount != _auth.accountId
      ? PurchaseFlowStatus.accountRequired
      : _status;
  int get verifiedRevision =>
      _statusAccount == _auth.accountId ? _verifiedRevision : 0;
  bool get busy => _operation || _work.values.any((w) => w.task != null);
  bool get canBuy =>
      !busy &&
      !{
        PurchaseFlowStatus.opening,
        PurchaseFlowStatus.pending,
        PurchaseFlowStatus.verifying,
      }.contains(status);
  bool get canRetry => _work.values.any(
    (w) => !w.done && !w.running && w.event.accountID == _auth.accountId,
  );
  bool get supported => _bridge.platform.isNotEmpty;

  void start() {
    if (_disposed || _subscription != null || !supported) return;
    _subscription = _bridge.events.listen(
      _receive,
      onError: (Object error) {
        _set(PurchaseFlowStatus.retry);
      },
    );
  }

  void _set(PurchaseFlowStatus value, {String? account}) {
    if (_disposed) return;
    _status = value;
    _statusAccount = account;
    notifyListeners();
  }

  void disableOffers() {
    _loadGeneration++;
    _offers = const [];
    _offerAccount = null;
    _premiumActive = null;
    if (!_disposed) notifyListeners();
  }

  Future<void> load(
    PurchaseCatalog catalog, {
    bool? premiumActive,
    String? accountID,
  }) async {
    start();
    final generation = ++_loadGeneration;
    final expectedAccount = accountID ?? _auth.accountId;
    _offers = const [];
    _offerAccount = null;
    _premiumActive = null;
    if (!supported) {
      _set(PurchaseFlowStatus.unavailable);
      return;
    }
    if ({
      PurchaseFlowStatus.idle,
      PurchaseFlowStatus.unavailable,
      PurchaseFlowStatus.loading,
    }.contains(_status)) {
      _set(PurchaseFlowStatus.loading);
    }
    try {
      if (expectedAccount == null || expectedAccount != _auth.accountId) {
        throw const AuthSessionException('billing.account_changed');
      }
      await _auth.ensureSession();
      final account = _auth.accountId;
      if (account != expectedAccount) {
        throw const AuthSessionException('billing.account_changed');
      }
      if (account == null || !purchaseAccountID(account)) {
        throw const AuthSessionException('billing.account_required');
      }
      final products = catalog.forPlatform(_bridge.platform);
      final found = products.isEmpty
          ? <PurchaseOffer>[]
          : await _bridge.query(products);
      if (_disposed || generation != _loadGeneration) return;
      if (_auth.accountId != account) {
        throw const AuthSessionException('billing.account_changed');
      }
      _offers = List.unmodifiable(found);
      _offerAccount = account;
      _premiumActive = premiumActive;
      if (_status == PurchaseFlowStatus.loading) {
        _set(
          found.isEmpty
              ? PurchaseFlowStatus.unavailable
              : PurchaseFlowStatus.idle,
          account: account,
        );
      } else {
        notifyListeners();
      }
    } catch (_) {
      if (generation == _loadGeneration && !_disposed) {
        if (_status == PurchaseFlowStatus.loading) {
          _set(PurchaseFlowStatus.unavailable);
        } else {
          notifyListeners();
        }
      }
    }
  }

  Future<void> buy(PurchaseOffer offer) async {
    if (_disposed || !canBuy || !offers.contains(offer)) return;
    if (!offer.product.consumable && _premiumActive != false) return;
    final account = _offerAccount;
    if (account == null || !purchaseAccountID(account)) return;
    _operation = true;
    _set(PurchaseFlowStatus.opening, account: account);
    try {
      await _auth.ensureSession();
      if (_auth.accountId != account) {
        throw const AuthSessionException('billing.account_changed');
      }
      final opened = await _bridge.purchase(offer, account);
      if (!opened) _set(PurchaseFlowStatus.failed, account: account);
    } catch (_) {
      _set(PurchaseFlowStatus.failed, account: account);
    } finally {
      _operation = false;
      if (!_disposed) notifyListeners();
    }
  }

  Future<void> restore() async {
    start();
    if (_disposed || busy || !supported) return;
    _operation = true;
    try {
      if (!await _auth.restoreExistingSession()) {
        throw const AuthSessionException('billing.account_required');
      }
      final account = _auth.accountId;
      if (account == null || !purchaseAccountID(account)) {
        throw const AuthSessionException('billing.account_required');
      }
      _set(PurchaseFlowStatus.loading, account: account);
      await _bridge.restore(account);
      if (_status == PurchaseFlowStatus.loading) {
        _set(PurchaseFlowStatus.idle, account: account);
      }
    } on AuthSessionException {
      _set(PurchaseFlowStatus.accountRequired);
    } catch (_) {
      _set(PurchaseFlowStatus.failed);
    } finally {
      _operation = false;
      if (!_disposed) notifyListeners();
    }
    await retry();
  }

  void _receive(NativePurchaseEvent event) {
    if (_disposed) return;
    final account = event.accountID;
    if (account == null || !purchaseAccountID(account)) {
      _set(PurchaseFlowStatus.accountRequired);
      return;
    }
    if (event.status != NativePurchaseStatus.purchased &&
        event.status != NativePurchaseStatus.restored) {
      if (account == _auth.accountId) {
        _set(switch (event.status) {
          NativePurchaseStatus.pending => PurchaseFlowStatus.pending,
          NativePurchaseStatus.canceled => PurchaseFlowStatus.canceled,
          _ => PurchaseFlowStatus.failed,
        }, account: account);
      }
      return;
    }
    final receipt = event.receipt;
    if (receipt == null || receipt.platform != _bridge.platform) {
      _set(PurchaseFlowStatus.failed);
      return;
    }
    final key = '$account:${receipt.key}';
    if (_completed.contains(key) || _work.containsKey(key)) return;
    // Overflow stays unfinished in the native store and can be restored later.
    if (_work.length >= 64) {
      _set(PurchaseFlowStatus.retry);
      return;
    }
    final work = _ReceiptWork(event);
    _work[key] = work;
    unawaited(_enqueue(key, work));
  }

  Future<void> _enqueue(String key, _ReceiptWork work) {
    if (work.done || _disposed) return Future<void>.value();
    if (work.task case final existing?) return existing;
    final task = _processing.then((_) => _process(key, work)).whenComplete(() {
      work.task = null;
      if (!_disposed) notifyListeners();
    });
    work.task = task;
    return _processing = task;
  }

  Future<void> _process(String key, _ReceiptWork work) async {
    if (_disposed || work.running || work.done) return;
    work.running = true;
    final account = work.event.accountID!;
    try {
      if (_auth.accountId == null && !await _auth.restoreExistingSession()) {
        _set(PurchaseFlowStatus.accountRequired);
        return;
      }
      if (_auth.accountId != account) {
        _set(PurchaseFlowStatus.accountRequired);
        return;
      }
      _set(PurchaseFlowStatus.verifying, account: account);
      if (!work.verified) {
        final result = await _api.verifyPurchase(
          work.event.receipt!,
          accountID: account,
        );
        if (result.status != 'granted') {
          _set(
            result.status == 'pending'
                ? PurchaseFlowStatus.pending
                : PurchaseFlowStatus.failed,
            account: account,
          );
          return;
        }
        work.verified = true;
      }
      if (_disposed || _auth.accountId != account) {
        _set(PurchaseFlowStatus.accountRequired);
        return;
      }
      // Google consumption and acknowledgement are durable server tasks only.
      if (work.event.receipt!.platform == 'app_store' &&
          work.event.needsFinish) {
        await _bridge.finish(work.event);
      }
      if (_disposed || _auth.accountId != account) {
        _set(PurchaseFlowStatus.accountRequired);
        return;
      }
      work.done = true;
      _work.remove(key);
      _completed.add(key);
      if (_completed.length > 256) _completed.remove(_completed.first);
      _verifiedRevision++;
      _set(PurchaseFlowStatus.verified, account: account);
    } catch (_) {
      _set(PurchaseFlowStatus.retry, account: account);
    } finally {
      work.running = false;
      if (!_disposed) notifyListeners();
    }
  }

  Future<void> retry() {
    if (_disposed) return Future<void>.value();
    if (_retrying case final existing?) return existing;
    final tasks = [
      for (final item in _work.entries.toList())
        if (item.value.event.accountID == _auth.accountId)
          _enqueue(item.key, item.value),
    ];
    return _retrying = Future.wait(
      tasks,
    ).then<void>((_) {}).whenComplete(() => _retrying = null);
  }

  @override
  void dispose() {
    _disposed = true;
    _loadGeneration++;
    unawaited(_subscription?.cancel());
    _work.clear();
    _completed.clear();
    _offers = const [];
    super.dispose();
  }
}
