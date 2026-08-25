import 'dart:async';
import 'dart:math';

import 'package:web_socket_channel/web_socket_channel.dart';

import 'game_transport.dart';

/// Production WebSocket transport with exponential backoff reconnect.
class WebSocketTransport implements GameTransport {
  WebSocketTransport({
    required this.url,
    this.reconnectDelay = const Duration(seconds: 1),
    this.maxReconnectDelay = const Duration(seconds: 30),
    this.reconnectJitter = 0.2,
  });

  final String url;
  final Duration reconnectDelay;
  final Duration maxReconnectDelay;
  final double reconnectJitter;

  WebSocketChannel? _channel;
  final _messageController = StreamController<Map<String, dynamic>>.broadcast();
  final _stateController = StreamController<ConnectionState>.broadcast();
  final _rand = Random();
  bool _connected = false;
  bool _disposed = false;

  // Bumped by reconnect() so a pending auto-reconnect timer scheduled by the
  // socket it just discarded can recognise it's stale and bail out instead
  // of clobbering the fresh connection with a third socket.
  int _generation = 0;

  @override
  Stream<Map<String, dynamic>> get messages => _messageController.stream;

  @override
  Stream<ConnectionState> get state => _stateController.stream;

  @override
  bool get isConnected => _connected;

  @override
  Future<void> connect() async {
    if (_disposed) return;
    await _connectWithBackoff(
      initialDelay: Duration.zero,
      attempt: 0,
      generation: _generation,
    );
  }

  @override
  Future<void> reconnect() async {
    if (_disposed) return;
    _generation++;
    await _channel?.sink.close();
    _channel = null;
    _connected = false;
    await _connectWithBackoff(
      initialDelay: Duration.zero,
      attempt: 0,
      generation: _generation,
    );
  }

  Future<void> _connectWithBackoff({
    required Duration initialDelay,
    required int attempt,
    required int generation,
  }) async {
    if (_disposed || generation != _generation) return;
    if (initialDelay > Duration.zero) {
      await Future<void>.delayed(initialDelay);
    }
    if (_disposed || generation != _generation) return;

    _setState(attempt == 0
        ? ConnectionState.connecting
        : ConnectionState.reconnecting);

    try {
      _channel = WebSocketChannel.connect(Uri.parse(url));
      _connected = true;
      _setState(ConnectionState.connected);

      _channel!.stream.listen(
        (dynamic data) {
          if (data is String) {
            try {
              _messageController.add(TransportCodec.decode(data));
            } on FormatException catch (e) {
              _messageController.addError(e);
            }
          }
        },
        onDone: () {
          _connected = false;
          if (!_disposed && generation == _generation) {
            _setState(ConnectionState.disconnected);
            _scheduleReconnect(attempt + 1, generation);
          }
        },
        onError: (Object error) {
          _messageController.addError(error);
        },
      );
    } on Exception catch (e) {
      _connected = false;
      _setState(ConnectionState.disconnected);
      _messageController.addError(e);
      _scheduleReconnect(attempt + 1, generation);
    }
  }

  void _scheduleReconnect(int attempt, int generation) {
    if (_disposed || generation != _generation) return;
    final base = reconnectDelay.inMilliseconds * pow(2, attempt).toInt();
    final capped = min(base, maxReconnectDelay.inMilliseconds);
    final jitter = (capped * reconnectJitter * _rand.nextDouble()).toInt();
    final delay = Duration(milliseconds: capped + jitter);
    _connectWithBackoff(
        initialDelay: delay, attempt: attempt, generation: generation);
  }

  @override
  Future<void> send(Map<String, dynamic> message) async {
    if (!_connected || _channel == null) {
      throw StateError('transport not connected');
    }
    _channel!.sink.add(TransportCodec.encode(message));
  }

  @override
  Future<void> close() async {
    _disposed = true;
    _connected = false;
    await _channel?.sink.close();
    _channel = null;
    await _messageController.close();
    await _stateController.close();
  }

  void _setState(ConnectionState value) {
    if (!_stateController.isClosed) {
      _stateController.add(value);
    }
  }
}
