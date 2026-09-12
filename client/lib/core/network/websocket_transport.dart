import 'dart:async';
import 'dart:math';

import 'package:web_socket_channel/web_socket_channel.dart';

import '../logging/app_logger.dart';
import 'game_transport.dart';

/// Production WebSocket transport with exponential backoff reconnect.
class WebSocketTransport implements GameTransport {
  WebSocketTransport({
    required this.url,
    this.reconnectDelay = const Duration(milliseconds: 500),
    this.maxReconnectDelay = const Duration(seconds: 5),
    this.reconnectJitter = 0.2,
    this.decodeMessage = TransportCodec.decode,
    this.channelFactory = WebSocketChannel.connect,
  });

  final String url;
  final Duration reconnectDelay;
  final Duration maxReconnectDelay;
  final double reconnectJitter;
  final Map<String, dynamic> Function(String) decodeMessage;
  final WebSocketChannel Function(Uri) channelFactory;

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
  Future<void>? _connectAttempt;
  int _attemptGeneration = -1;

  @override
  Stream<Map<String, dynamic>> get messages => _messageController.stream;

  @override
  Stream<ConnectionState> get state => _stateController.stream;

  @override
  bool get isConnected => _connected;

  @override
  Future<void> connect() async {
    if (_disposed || _connected) return;
    AppLogger.info(LogTopic.network, 'Opening game connection');
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
    AppLogger.warning(LogTopic.network, 'Reconnecting game connection');
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
  }) {
    if (_disposed || _connected || generation != _generation) {
      return Future.value();
    }
    if (_connectAttempt != null && _attemptGeneration == generation) {
      return _connectAttempt!;
    }
    final pending = _openWithBackoff(
        initialDelay: initialDelay, attempt: attempt, generation: generation);
    _connectAttempt = pending;
    _attemptGeneration = generation;
    return pending.whenComplete(() {
      if (identical(_connectAttempt, pending)) _connectAttempt = null;
    });
  }

  Future<void> _openWithBackoff({
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
      final channel = channelFactory(Uri.parse(url));
      _channel = channel;
      await channel.ready;
      if (_disposed || generation != _generation) {
        await channel.sink.close();
        return;
      }
      _connected = true;
      _setState(ConnectionState.connected);
      AppLogger.info(LogTopic.network, 'Game connection established');

      channel.stream.listen(
        (dynamic data) {
          if (data is String) {
            try {
              if (!_disposed && generation == _generation) {
                _messageController.add(decodeMessage(data));
              }
            } on Exception catch (e) {
              AppLogger.warning(
                  LogTopic.network, 'Discarded malformed server message');
              _messageController.addError(e);
            }
          }
        },
        onDone: () {
          if (_disposed || generation != _generation) return;
          _connected = false;
          AppLogger.warning(LogTopic.network, 'Game connection closed');
          if (!_disposed && generation == _generation) {
            _setState(ConnectionState.disconnected);
            _scheduleReconnect(attempt + 1, generation);
          }
        },
        onError: (Object error) {
          if (_disposed || generation != _generation) return;
          AppLogger.error(LogTopic.network, 'Game connection failed', fields: {
            'error_type': error.runtimeType,
          });
          _messageController.addError(error);
        },
      );
    } on Exception catch (e) {
      if (_disposed || generation != _generation) return;
      _connected = false;
      _setState(ConnectionState.disconnected);
      AppLogger.error(LogTopic.network, 'Could not open game connection',
          fields: {
            'error_type': e.runtimeType,
          });
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
    AppLogger.info(LogTopic.network, 'Scheduled game reconnection', fields: {
      'attempt': attempt,
      'delay_ms': delay.inMilliseconds,
    });
    // Wait outside the pending-attempt gate: a failed handshake schedules this
    // while its own future is still completing. Coalescing that same future
    // would silently lose the retry.
    unawaited(Future<void>.delayed(delay, () {
      return _connectWithBackoff(
          initialDelay: Duration.zero,
          attempt: attempt,
          generation: generation);
    }));
  }

  @override
  Future<void> send(Map<String, dynamic> message) async {
    if (!_connected || _channel == null) {
      AppLogger.warning(LogTopic.network, 'Blocked send while disconnected');
      throw StateError('transport not connected');
    }
    AppLogger.debug(LogTopic.network, 'Sent game message', fields: {
      'message_type': message['type'],
    });
    _channel!.sink.add(TransportCodec.encode(message));
  }

  @override
  Future<void> close() async {
    AppLogger.info(LogTopic.network, 'Closing game connection');
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
