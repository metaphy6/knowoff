import 'dart:async';
import 'dart:convert';

/// Abstract interface for the single persistent game channel.
/// Implementations hide whether the underlying transport is a native
/// WebSocket, a web WebSocket, or a test fake.
abstract class GameTransport {
  /// Emitted for every inbound message after connection.
  Stream<Map<String, dynamic>> get messages;

  /// Emitted when the connection state changes.
  Stream<ConnectionState> get state;

  /// True when the transport believes it is connected.
  bool get isConnected;

  /// Establish the connection.
  Future<void> connect();

  /// Open a new socket for a fresh authenticated hello and recipient stream.
  /// The text session reclaims its existing room only after hello succeeds.
  Future<void> reconnect();

  /// Send a JSON message. Completes with an error if not connected.
  Future<void> send(Map<String, dynamic> message);

  /// Close the connection cleanly.
  Future<void> close();
}

enum ConnectionState { disconnected, connecting, connected, reconnecting }

/// Base event emitted by all transports.
mixin GameTransportEvent {
  Map<String, dynamic> toJson();
}

/// Wraps encoding/decoding helpers shared by concrete transports.
class TransportCodec {
  static String encode(Map<String, dynamic> message) => jsonEncode(message);
  static Map<String, dynamic> decode(String payload) {
    final parsed = jsonDecode(payload);
    if (parsed is! Map<String, dynamic>) {
      throw FormatException('expected JSON object, got ${parsed.runtimeType}');
    }
    return parsed;
  }
}
