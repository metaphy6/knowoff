import 'dart:async';
import 'dart:convert';
import 'dart:math';
import 'package:flutter/foundation.dart';
import '../network/game_transport.dart';
import 'v2_contract.dart';
import 'v2_reducer.dart';

class TextModeAvailability {
  TextModeAvailability(
      {required this.mode, required this.available, required this.languages});
  final String mode;
  final bool available;
  final List<Map<String, dynamic>> languages;
}

/// One foreground text connection. The server supplies availability and all
/// gameplay limits; authentication and stale async callbacks are generation fenced.
class TextSession extends ChangeNotifier {
  TextSession(
      {required this.transport,
      required this.tokenLoader,
      DateTime Function()? now})
      : now = now ?? DateTime.now {
    _messages = transport.messages
        .listen(receive, onError: (_) => _failure('protocol.malformed'));
    _connection = transport.state.listen((state) {
      if (_disposed || !_foreground) return;
      if (state == ConnectionState.connected) {
        unawaited(_hello());
      } else {
        ready = false;
        _helloPending = false;
        _controls.clear();
        reducer?.disconnect();
        notifyListeners();
      }
    });
  }
  final GameTransport transport;
  final Future<String> Function() tokenLoader;
  final DateTime Function() now;
  late final StreamSubscription<Map<String, dynamic>> _messages;
  late final StreamSubscription<ConnectionState> _connection;
  final _random = Random.secure();
  bool _disposed = false, ready = false;
  bool _foreground = true, _helloPending = false, _admitted = false;
  final _controls = <String, String>{};
  String? _limitsHash;
  bool? _prototype;
  bool get prototype => _prototype == true;
  String? _roomID;
  int _settingsRevision = 0, _membershipRevision = 0;
  final _deliveryHashes = <int, String>{};
  final _deliveries = <int, Map<String, dynamic>>{};
  final _awards = <String, Map<String, dynamic>>{};
  List<Map<String, dynamic>> get settlements =>
      List.unmodifiable(_deliveries.values);
  List<Map<String, dynamic>> get awards => List.unmodifiable(_awards.values);
  int _generation = 0;
  V2Reducer? reducer;
  V2Limits? limits;
  V2Snapshot? get snapshot => reducer?.current;
  String? errorCode, roomCode;
  int? seat;
  Map<String, dynamic>? lobby, queue;
  List<TextModeAvailability> availability = const [];
  int _serverOffset = 0;
  int get serverNowMS => now().millisecondsSinceEpoch + _serverOffset;
  String nextRequestID() => List.generate(
      16, (_) => _random.nextInt(256).toRadixString(16).padLeft(2, '0')).join();
  Future<void> connect() {
    _foreground = true;
    return transport.connect();
  }

  Future<void> _hello() async {
    final generation = ++_generation;
    _helloPending = true;
    try {
      final token = await tokenLoader();
      if (_disposed || !_foreground || generation != _generation) return;
      await transport.send({
        'v': 2,
        'type': 'hello',
        'payload': {'client_generation': 2, 'access_token': token}
      });
    } catch (_) {
      if (!_disposed && generation == _generation) _failure('auth.required');
    }
  }

  void _failure(String code) {
    if (_disposed || !_foreground) return;
    errorCode = code;
    ready = false;
    _clearValueViews();
    reducer?.disconnect();
    notifyListeners();
  }

  Map<String, dynamic> decode(String raw) =>
      V2Codec.envelope(raw, maxBytes: limits?.maxFrameBytes ?? 1048576);
  void receive(Map<String, dynamic> message) {
    if (_disposed || !_foreground) return;
    try {
      final envelope = decode(jsonEncode(message));
      final type = envelope['type'];
      final p = envelope['payload'] as Map<String, dynamic>;
      if (type == 'error') {
        if (p.containsKey('cursor')) {
          if (!ready || !_admitted || reducer == null) return;
          if (envelope['request_id'] != p['request_id']) {
            throw const V2Failure('protocol.malformed');
          }
          reducer!.errorEvent(p);
          errorCode = p['code'];
          if (reducer!.needsResync) unawaited(control('resync', {}));
          notifyListeners();
          return;
        }
        V2Codec.control('controlError', p);
        final code = p['code'];
        if (code is! String) throw const V2Failure('protocol.malformed');
        final id = p['request_id'] as String?;
        if (envelope.containsKey('request_id') &&
            envelope['request_id'] != id) {
          throw const V2Failure('protocol.malformed');
        }
        final fatal =
            code == 'protocol.upgrade_required' || code == 'auth.required';
        final controlType = _controls.remove(id);
        if (!fatal &&
            controlType == null &&
            reducer?.pendingRequest?['request_id'] != id) {
          return;
        }
        errorCode = code;
        reducer?.error(code, id);
        if (['room_create', 'room_join', 'queue_join'].contains(controlType) &&
            snapshot == null &&
            lobby == null) {
          _admitted = false;
        }
        if (fatal) {
          ready = false;
          _clearValueViews();
          _helloPending = false;
          reducer?.disconnect();
        }
        if (reducer?.needsResync == true && ready) {
          unawaited(control('resync', {}));
        }
        notifyListeners();
        return;
      }
      if (type == 'hello') {
        if (!_helloPending) return;
        V2Codec.control('hello', p);
        if (p['client_generation'] != 2 || p['account_id'] is! String) {
          throw const V2Failure('protocol.upgrade_required');
        }
        final limitsHash = v2Hash(p['limits']);
        if (_limitsHash != null && limitsHash != _limitsHash) {
          throw const V2Failure('protocol.upgrade_required');
        }
        if (_prototype != null && _prototype != p['prototype']) {
          throw const V2Failure('protocol.upgrade_required');
        }
        _prototype = p['prototype'];
        _limitsHash = limitsHash;
        _helloPending = false;
        limits = V2Limits.fromJson(p['limits']);
        reducer ??= V2Reducer(limits!);
        ready = true;
        errorCode = null;
        if (roomCode != null) {
          unawaited(control('room_join', {'code': roomCode}));
        }
      } else {
        if (!ready) throw const V2Failure('protocol.upgrade_required');
        switch (type) {
          case 'availability':
            V2Codec.control('availability', p);
            if (v2Hash(p['limits']) != _limitsHash ||
                p['prototype'] != _prototype) {
              throw const V2Failure('protocol.upgrade_required');
            }
            _availability(p);
            break;
          case 'settlement':
            V2Codec.control('delivery', p);
            if (snapshot?.matchID == p['match_id'] &&
                snapshot?.phase != 'verdict') {
              return;
            }
            final id = p['id'] as int, hash = v2Hash(p);
            final old = _deliveryHashes[id];
            if (old != null && old != hash) {
              throw const V2Failure('request.conflict');
            }
            if (old == null) {
              if (_deliveries.length >= limits!.maxRequestsPerSeat) {
                throw const V2Failure('request.limit');
              }
              _deliveryHashes[id] = hash;
              _deliveries[id] = p;
              while (_deliveryHashes.length > limits!.maxRequestsPerSeat) {
                final removable = _deliveryHashes.keys
                    .where((key) => !_deliveries.containsKey(key));
                if (removable.isEmpty) break;
                _deliveryHashes.remove(removable.first);
              }
            } else if (!_deliveries.containsKey(id)) {
              // Acknowledgements can be lost. Explicit dismissal authorizes
              // another ack when the durable server lease replays this receipt.
              unawaited(_ackDismissedDelivery(id));
            }
            break;
          case 'award':
            V2Codec.control('instantAward', p);
            // Crediting is immediate on the server; role-linked Noin evidence
            // belongs only in the owner's match-end settlement surface.
            if (snapshot?.phase != 'verdict' ||
                snapshot?.matchID != p['match_id']) {
              return;
            }
            final key = '${p['match_id']}:${p['kind']}:${p['ordinal']}';
            final previous = _awards[key];
            if (previous != null && v2Hash(previous) != v2Hash(p)) {
              throw const V2Failure('request.conflict');
            }
            if (previous == null) {
              if (_awards.length >= limits!.maxRequestsPerSeat) {
                throw const V2Failure('request.limit');
              }
              _awards[key] = p;
            }
            break;
          case 'lobby':
            if (!_admitted) return;
            V2Codec.control('lobbyEnvelope', p);
            final next =
                V2Codec.decode('lobby', jsonEncode(p['lobby']), limits!);
            if (p['seat'] is! int ||
                !(next['seats'] as List).any((s) => s['seat'] == p['seat']) ||
                p['code'] is! String ||
                !RegExp(r'^[A-Z0-9]{6}$').hasMatch(p['code'])) {
              throw const V2Failure('protocol.malformed');
            }
            if (_roomID != null && _roomID != next['room_id']) return;
            final settingsRevision = next['settings_revision'] as int;
            final membershipRevision = next['membership_revision'] as int;
            if (settingsRevision < _settingsRevision ||
                membershipRevision < _membershipRevision) {
              return;
            }
            if (reducer!.hasPendingSnapshot) return;
            if (reducer!.current != null &&
                (reducer!.current!.phase != 'verdict' ||
                    settingsRevision <= _settingsRevision ||
                    membershipRevision <= _membershipRevision)) {
              return;
            }
            reducer!.reset();
            lobby = next;
            seat = p['seat'];
            roomCode = p['code'];
            _roomID = next['room_id'];
            _settingsRevision = settingsRevision;
            _membershipRevision = membershipRevision;
            queue = null;
            break;
          case 'queue':
            if (!_admitted) return;
            V2Codec.control('queue', p);
            if (!['waiting', 'choice_required', 'assigned', 'left']
                    .contains(p['status']) ||
                p['joined_at_ms'] is! int ||
                p['decision_at_ms'] is! int) {
              throw const V2Failure('protocol.malformed');
            }
            queue = p['status'] == 'left' ? null : p;
            if (p['status'] == 'left') _admitted = false;
            break;
          case 'snapshot':
            if (!_admitted) return;
            if (_roomID == null || p['contract']?['room_id'] != _roomID) return;
            reducer!.snapshot(p);
            if (reducer!.current != null) errorCode = null;
            lobby = null;
            queue = null;
            _serverOffset =
                (p['server_time_ms'] as int) - now().millisecondsSinceEpoch;
            break;
          case 'action_ack':
          case 'control_ack':
            V2Codec.control('action_ack', p);
            if (envelope['request_id'] != p['request_id']) {
              throw const V2Failure('protocol.malformed');
            }
            if (type == 'action_ack') {
              reducer!.acknowledge(p['request_id']);
            } else {
              _controls.remove(p['request_id']);
            }
            break;
          case 'history_page':
            if (!_admitted) return;
            reducer!.page(p);
            if (reducer!.current != null) errorCode = null;
            break;
          default:
            throw const V2Failure('protocol.malformed');
        }
      }
      notifyListeners();
    } on V2Failure catch (e) {
      errorCode = e.code;
      if (e.code == 'stream.duplicate' || e.code == 'stream.stale_epoch') {
        notifyListeners();
        return;
      }
      reducer?.disconnect();
      if (ready &&
          ['stream.gap', 'stream.stale_evidence', 'history.integrity']
              .contains(e.code)) {
        unawaited(control('resync', {}));
      } else {
        ready = false;
        _clearValueViews();
      }
      notifyListeners();
    }
  }

  void _availability(Map<String, dynamic> p) {
    if (p['protocol_version'] != 2 ||
        p['client_generation'] != 2 ||
        p['modes'] is! List) {
      throw const V2Failure('protocol.upgrade_required');
    }
    final result = <TextModeAvailability>[];
    final seen = <String>{};
    for (final m in p['modes']) {
      if (m is! Map ||
          !textModes.contains(m['mode_id']) ||
          !seen.add(m['mode_id']) ||
          m['available'] is! bool ||
          m['languages'] is! List) {
        throw const V2Failure('protocol.malformed');
      }
      final languages = <Map<String, dynamic>>[];
      final tuples = <String>{};
      for (final language in m['languages']) {
        if (language is! Map<String, dynamic> ||
            language.keys.length != 3 ||
            language['content_language'] is! String ||
            language['pack_release_id'] is! String ||
            language['rules_version'] is! String ||
            !tuples.add(jsonEncode(language))) {
          throw const V2Failure('protocol.malformed');
        }
        languages.add(language);
      }
      if (m['available'] != languages.isNotEmpty) {
        throw const V2Failure('protocol.malformed');
      }
      result.add(TextModeAvailability(
          mode: m['mode_id'],
          available: m['available'],
          languages: List.unmodifiable(languages)));
    }
    if (seen.length != textModes.length) {
      throw const V2Failure('protocol.malformed');
    }
    availability = List.unmodifiable(result);
  }

  Future<void> control(String type, Map<String, dynamic> payload) {
    if (!ready || _disposed || !_foreground) {
      throw const V2Failure('protocol.upgrade_required');
    }
    if (_controls.length >= limits!.maxRequestsPerSeat) {
      throw const V2Failure('request.limit');
    }
    if (type == 'resync' && reducer?.current != null) reducer!.disconnect();
    if (['queue_join', 'room_create', 'room_join', 'rematch'].contains(type)) {
      _admitted = true;
    }
    final requestID = nextRequestID();
    _controls[requestID] = type;
    return transport.send(
        {'v': 2, 'type': type, 'request_id': requestID, 'payload': payload});
  }

  Future<void> act(Map<String, dynamic> action) {
    if (!ready || reducer == null) throw const V2Failure('action.unauthorized');
    final request = reducer!.confirm(action, nextRequestID(), serverNowMS);
    notifyListeners();
    return transport.send({'v': 2, 'type': 'action', 'payload': request});
  }

  Future<void> retry() =>
      transport.send({'v': 2, 'type': 'action', 'payload': reducer!.retry()});
  Future<void> dismissDelivery(int id) async {
    if (!_deliveries.containsKey(id)) return;
    await control('settlement_ack', {'id': id});
    _deliveries.remove(id);
    if (!_disposed) notifyListeners();
  }

  Future<void> _ackDismissedDelivery(int id) async {
    try {
      await control('settlement_ack', {'id': id});
    } catch (_) {
      _failure('request.failed');
    }
  }

  Future<void> leave() async {
    final type = queue != null ? 'queue_leave' : 'room_leave';
    roomCode = null;
    _roomID = null;
    _settingsRevision = 0;
    _membershipRevision = 0;
    _admitted = false;
    _controls.clear();
    seat = null;
    lobby = null;
    queue = null;
    reducer?.reset();
    notifyListeners();
    if (ready) await control(type, {});
  }

  void background() {
    _generation++;
    _foreground = false;
    _helloPending = false;
    _controls.clear();
    _clearValueViews();
    reducer?.disconnect();
    ready = false;
    notifyListeners();
  }

  void _clearValueViews() {
    // Unacknowledged deliveries may be replayed after the server lease expires.
    for (final id in _deliveries.keys) {
      _deliveryHashes.remove(id);
    }
    _deliveries.clear();
    _awards.clear();
  }

  Future<void> resume() {
    _foreground = true;
    return transport.reconnect();
  }

  @override
  void dispose() {
    _disposed = true;
    _generation++;
    reducer?.reset();
    _deliveries.clear();
    _deliveryHashes.clear();
    _awards.clear();
    availability = const [];
    lobby = null;
    queue = null;
    unawaited(_messages.cancel());
    unawaited(_connection.cancel());
    unawaited(transport.close());
    super.dispose();
  }
}
