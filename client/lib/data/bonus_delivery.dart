import 'dart:async';
import 'dart:convert';
import 'package:crypto/crypto.dart';
import 'package:flutter/foundation.dart';
import 'package:shared_preferences/shared_preferences.dart';

const _maxReceipts = 20;
final _uuid = RegExp(
  r'^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$',
);
final _digest = RegExp(r'^[0-9a-f]{64}$');
Never _invalid() => throw const FormatException('Invalid bonus delivery');
bool _id(Object? value) =>
    value is String &&
    _uuid.hasMatch(value) &&
    value != '00000000-0000-0000-0000-000000000000';
Map<String, dynamic> _object(Object? value, Set<String> keys) {
  if (value is! Map<String, dynamic> ||
      value.length != keys.length ||
      !value.keys.every(keys.contains)) {
    _invalid();
  }
  return value;
}

int _amount(Object? value) {
  if (value is! int || value < 0 || value > 2147483647) _invalid();
  return value;
}

class BonusDay {
  const BonusDay._(this.day, this.requested, this.credited);
  final String day;
  final int requested, credited;
}

class BonusPayload {
  const BonusPayload._(
    this.matchId,
    this.source,
    this.requested,
    this.credited,
    this.days,
    this.sha256,
  );
  final String matchId, source, sha256;
  final int requested, credited;
  final List<BonusDay> days;
  static BonusPayload decode(Object? raw) {
    final p = _object(raw, {
      'version',
      'match_id',
      'source',
      'requested',
      'credited',
      'days',
    });
    if (p['version'] != 1 ||
        p['version'] is! int ||
        !_id(p['match_id']) ||
        !{'premium', 'rewarded_ad'}.contains(p['source'])) {
      _invalid();
    }
    final requested = _amount(p['requested']),
        credited = _amount(p['credited']);
    if (credited > requested ||
        p['days'] is! List ||
        (p['days'] as List).length > 9) {
      _invalid();
    }
    final days = <BonusDay>[];
    var totalRequested = 0, totalCredited = 0, previous = '';
    for (final rawDay in p['days'] as List) {
      final d = _object(rawDay, {'server_day', 'requested', 'credited'});
      final day = d['server_day'];
      if (day is! String ||
          !RegExp(r'^\d{4}-\d{2}-\d{2}$').hasMatch(day) ||
          day.compareTo(previous) <= 0) {
        _invalid();
      }
      final date = DateTime.tryParse('${day}T00:00:00Z');
      if (date == null || date.toIso8601String().substring(0, 10) != day) {
        _invalid();
      }
      final dr = _amount(d['requested']), dc = _amount(d['credited']);
      if (dc > dr) _invalid();
      totalRequested += dr;
      totalCredited += dc;
      days.add(BonusDay._(day, dr, dc));
      previous = day;
    }
    if (totalRequested != requested || totalCredited != credited) _invalid();
    final vector = [
      1,
      p['match_id'],
      p['source'],
      requested,
      credited,
      [
        for (final d in days) [d.day, d.requested, d.credited],
      ],
    ];
    final hash = _bonusHash(vector);
    return BonusPayload._(
      p['match_id'] as String,
      p['source'] as String,
      requested,
      credited,
      List.unmodifiable(days),
      hash,
    );
  }
}

String _bonusHash(Object value) =>
    sha256.convert(utf8.encode(jsonEncode(value))).toString();

class BonusDelivery {
  const BonusDelivery._(this.id, this.lease, this.leaseExpiresAt, this.payload);
  final String id, lease;
  final DateTime leaseExpiresAt;
  final BonusPayload payload;
  static BonusDelivery decode(Object? raw) {
    final d = _object(raw, {
      'delivery_id',
      'lease',
      'lease_expires_at',
      'payload_sha256',
      'payload',
    });
    if (!_id(d['delivery_id'])) _invalid();
    final lease = d['lease'];
    if (lease is! String || !RegExp(r'^[A-Za-z0-9_-]{43}$').hasMatch(lease)) {
      _invalid();
    }
    final bytes = base64Url.decode('$lease=');
    if (bytes.length != 32 ||
        base64Url.encode(bytes).replaceAll('=', '') != lease) {
      _invalid();
    }
    final expires = d['lease_expires_at'];
    if (expires is! String ||
        !RegExp(
          r'^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d{1,9})?Z$',
        ).hasMatch(expires)) {
      _invalid();
    }
    final time = DateTime.tryParse(expires);
    if (time == null ||
        time.toIso8601String().substring(0, 19) != expires.substring(0, 19)) {
      _invalid();
    }
    final payload = BonusPayload.decode(d['payload']);
    if (d['payload_sha256'] != payload.sha256) _invalid();
    return BonusDelivery._(d['delivery_id'] as String, lease, time, payload);
  }
}

abstract interface class BonusDeliveryTransport {
  String? get accountId;
  int get sessionGeneration;
  Future<Map<String, dynamic>> claim(int limit, List<String> pendingIds);
  Future<void> acknowledge(String id, String lease);
  Future<void> refreshBalance();
}

abstract interface class BonusDismissalStorage {
  Future<Map<String, String>> read(String account);
  Future<void> write(String account, Map<String, String> pending);
}

/// Stores dismissal identity only. A lease or receipt never becomes wallet data.
/// Namespace must identify the server, keeping separate installations isolated.
class PreferencesBonusDismissals implements BonusDismissalStorage {
  PreferencesBonusDismissals(String server) : _namespace = _bonusHash(server);
  final String _namespace;
  String _key(String account) {
    if (!_id(account)) _invalid();
    return 'knowoff_bonus_dismissals_${_namespace}_$account';
  }

  @override
  Future<Map<String, String>> read(String account) async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.reload();
    final raw = prefs.getString(_key(account));
    if (raw == null) return {};
    if (raw.length > 4096) _invalid();
    final decoded = jsonDecode(raw);
    if (decoded is! Map<String, dynamic>) _invalid();
    final result = <String, String>{};
    for (final entry in decoded.entries) {
      if (entry.value is! String) _invalid();
      result[entry.key] = entry.value as String;
    }
    _validatePending(result);
    return result;
  }

  @override
  Future<void> write(String account, Map<String, String> pending) async {
    _validatePending(pending);
    final prefs = await SharedPreferences.getInstance();
    final succeeded = pending.isEmpty
        ? await prefs.remove(_key(account))
        : await prefs.setString(_key(account), jsonEncode(pending));
    if (!succeeded) {
      await prefs.reload();
      throw StateError('Bonus dismissal persistence failed');
    }
  }
}

void _validatePending(Map<String, String> pending) {
  if (pending.length > _maxReceipts ||
      pending.entries.any((e) => !_id(e.key) || !_digest.hasMatch(e.value))) {
    _invalid();
  }
}

/// Closed delivery state: explicitly driven by an authenticated lifecycle owner.
/// All operations serialize, including dismissal, so pre-ACK claims cannot later
/// redisplay a receipt. Session changes immediately hide all private getters.
class BonusDeliveryController extends ChangeNotifier {
  BonusDeliveryController(this._transport, this._storage);
  final BonusDeliveryTransport _transport;
  final BonusDismissalStorage _storage;
  Future<void> _tail = Future<void>.value();
  final _visible = <String, BonusDelivery>{};
  Map<String, String> _pending = {};
  String? _account;
  int? _generation;
  bool _loaded = false, _disposed = false, _walletPending = false;
  bool get _current =>
      !_disposed &&
      _account != null &&
      _account == _transport.accountId &&
      _generation == _transport.sessionGeneration;
  List<BonusDelivery> get receipts =>
      _current ? List.unmodifiable(_visible.values) : const [];
  Future<void> _serial(Future<void> Function() work) {
    final operation = _tail.then((_) async {
      if (!_disposed) await work();
    });
    _tail = operation.then<void>((_) {}, onError: (Object _, StackTrace __) {});
    return operation;
  }

  void sessionChanged() {
    _account = null;
    _generation = null;
    _loaded = false;
    _walletPending = false;
    _visible.clear();
    _pending = {};
    if (!_disposed) notifyListeners();
  }

  Future<bool> _load() async {
    if (!_current) {
      sessionChanged();
      _account = _transport.accountId;
      _generation = _transport.sessionGeneration;
    }
    if (!_current) return false;
    if (!_loaded) {
      final pending = await _storage.read(_account!);
      if (!_current) return false;
      _validatePending(pending);
      _pending = Map.of(pending);
      _loaded = true;
    }
    return true;
  }

  Future<void> refresh() => _serial(() async {
    if (!await _load()) return;
    final requested = _pending.keys.toList()..sort();
    final raw = await _transport.claim(_maxReceipts, requested);
    if (!_current) return;
    final p = _object(raw, {'version', 'deliveries', 'acknowledged_ids'});
    if (p['version'] != 1 ||
        p['version'] is! int ||
        p['deliveries'] is! List ||
        (p['deliveries'] as List).length > _maxReceipts ||
        p['acknowledged_ids'] is! List ||
        (p['acknowledged_ids'] as List).length > _maxReceipts) {
      _invalid();
    }
    final acknowledged = <String>{};
    var previous = '';
    for (final id in p['acknowledged_ids'] as List) {
      if (id is! String ||
          !requested.contains(id) ||
          id.compareTo(previous) <= 0 ||
          !acknowledged.add(id)) {
        _invalid();
      }
      previous = id;
    }
    final deliveries = <String, BonusDelivery>{};
    for (final rawDelivery in p['deliveries'] as List) {
      final d = BonusDelivery.decode(rawDelivery);
      if (deliveries.containsKey(d.id) || acknowledged.contains(d.id)) {
        _invalid();
      }
      final oldHash = _pending[d.id] ?? _visible[d.id]?.payload.sha256;
      if (oldHash != null && oldHash != d.payload.sha256) _invalid();
      deliveries[d.id] = d;
    }
    final nextPending = Map<String, String>.of(_pending)
      ..removeWhere((id, _) => acknowledged.contains(id));
    final allIds = {..._visible.keys, ...nextPending.keys, ...deliveries.keys};
    if (acknowledged.isNotEmpty) {
      await _storage.write(_account!, nextPending);
      if (!_current) return;
      _pending = nextPending;
    }
    // Confirmed ACK status must make progress even under presentation pressure.
    if (allIds.length > _maxReceipts) {
      throw StateError('Bonus delivery capacity reached');
    }
    if (deliveries.isNotEmpty) _walletPending = true;
    for (final d in deliveries.values) {
      if (!_pending.containsKey(d.id)) _visible[d.id] = d;
    }
    if (!_disposed) notifyListeners();
    for (final d in deliveries.values) {
      if (_pending.containsKey(d.id)) {
        await _ack(d);
        if (!_current) return;
      }
    }
    if (_walletPending && _current) {
      await _transport.refreshBalance();
      if (_current) _walletPending = false;
    }
  });
  Future<void> dismiss(String id) => _serial(() async {
    if (!await _load()) return;
    final d = _visible[id];
    if (d == null) return;
    final pending = {..._pending, id: d.payload.sha256};
    await _storage.write(_account!, pending);
    if (!_current) return;
    _pending = pending;
    _visible.remove(id);
    notifyListeners();
    await _ack(d);
  });
  Future<void> _ack(BonusDelivery d) async {
    if (!_current) return;
    await _transport.acknowledge(d.id, d.lease);
    if (!_current) return;
    final pending = Map<String, String>.of(_pending)..remove(d.id);
    await _storage.write(_account!, pending);
    if (_current) _pending = pending;
  }

  @override
  void dispose() {
    _disposed = true;
    _visible.clear();
    _pending.clear();
    super.dispose();
  }
}
