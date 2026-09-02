import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/core/network/game_transport.dart';
import 'package:knowoff_client/core/network/websocket_transport.dart';

void main() {
  group('WebSocketTransport contract', () {
    late HttpServer server;
    late String url;
    WebSocket? lastServerSocket;

    Future<void> startServer() async {
      server = await HttpServer.bind('localhost', 0);
      url = 'ws://localhost:${server.port}/ws';
      lastServerSocket = null;

      server.listen((request) async {
        if (request.uri.path != '/ws') {
          request.response.statusCode = 404;
          await request.response.close();
          return;
        }
        final socket = await WebSocketTransformer.upgrade(request);
        lastServerSocket = socket;
        socket.listen((dynamic data) {
          if (data is String) {
            final payload = jsonDecode(data) as Map<String, dynamic>;
            payload['echo'] = true;
            socket.add(jsonEncode(payload));
          }
        });
      }, onError: (_) {});
    }

    setUp(startServer);

    tearDown(() async {
      await lastServerSocket?.close();
      await server.close();
    });

    test('defaults to a short maximum reconnect delay', () {
      final transport = WebSocketTransport(url: url);

      expect(transport.maxReconnectDelay, const Duration(seconds: 5));
    });

    test('connects, sends, and receives echoed JSON', () async {
      final transport = WebSocketTransport(url: url);
      await transport.connect();

      expect(transport.isConnected, isTrue);

      final received = transport.messages.first;
      await transport.send({'hello': 'world'});

      final msg = await received;
      expect(msg['hello'], equals('world'));
      expect(msg['echo'], isTrue);

      await transport.close();
      expect(transport.isConnected, isFalse);
    });

    test('reconnects after server drops the socket', () async {
      final transport = WebSocketTransport(
        url: url,
        reconnectDelay: const Duration(milliseconds: 50),
        maxReconnectDelay: const Duration(milliseconds: 200),
      );

      final stateLog = <String>[];
      transport.state.listen((s) => stateLog.add(s.name));

      await transport.connect();
      await Future<void>.delayed(const Duration(milliseconds: 100));

      // Force a disconnect from the server side.
      await lastServerSocket?.close();
      await Future<void>.delayed(const Duration(milliseconds: 400));

      expect(stateLog, contains(ConnectionState.disconnected.name));
      expect(stateLog, contains(ConnectionState.reconnecting.name));
      expect(stateLog.last, equals(ConnectionState.connected.name));

      await transport.close();
    });

    test('reconnect() opens a brand-new socket for a fresh handshake',
        () async {
      final transport = WebSocketTransport(
        url: url,
        reconnectDelay: const Duration(milliseconds: 50),
        maxReconnectDelay: const Duration(milliseconds: 200),
      );

      await transport.connect();
      await Future<void>.delayed(const Duration(milliseconds: 50));
      final firstServerSocket = lastServerSocket;
      expect(firstServerSocket, isNotNull);

      await transport.reconnect();
      await Future<void>.delayed(const Duration(milliseconds: 100));

      expect(transport.isConnected, isTrue);
      expect(lastServerSocket, isNot(same(firstServerSocket)));

      // The first socket's own onDone handler would otherwise schedule an
      // auto-reconnect that later clobbers the fresh connection with a
      // third socket. Give that stale timer a chance to fire and confirm it
      // didn't.
      final settledSocket = lastServerSocket;
      await Future<void>.delayed(const Duration(milliseconds: 300));
      expect(lastServerSocket, same(settledSocket));

      await transport.close();
    });
  });
}
