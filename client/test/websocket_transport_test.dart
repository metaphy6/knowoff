import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/core/network/game_transport.dart';
import 'package:knowoff_client/core/network/websocket_transport.dart';
import 'package:web_socket_channel/web_socket_channel.dart';

class _DelayedChannel implements WebSocketChannel {
  final incoming = StreamController<dynamic>();
  final handshake = Completer<void>();
  @override
  Stream<dynamic> get stream => incoming.stream;
  @override
  Future<void> get ready => handshake.future;
  @override
  final WebSocketSink sink = _DetachedSink();
  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}

class _DetachedSink implements WebSocketSink {
  @override
  Future<void> close([int? code, String? reason]) async {}
  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}

void main() {
  test('a failed opening handshake still schedules a later attempt', () async {
    final first = _DelayedChannel();
    final next = _DelayedChannel()..handshake.complete();
    var opened = 0;
    final transport = WebSocketTransport(
        url: 'ws://example.invalid/ws/v2',
        reconnectDelay: const Duration(milliseconds: 5),
        maxReconnectDelay: const Duration(milliseconds: 10),
        reconnectJitter: 0,
        channelFactory: (_) => opened++ == 0 ? first : next);
    final subscription = transport.messages.listen((_) {}, onError: (_) {});
    final connecting = transport.connect();
    first.handshake.completeError(const FormatException('handshake failed'));
    await connecting;
    await Future<void>.delayed(const Duration(milliseconds: 30));
    expect(opened, 2);
    expect(transport.isConnected, isTrue);
    await transport.close();
    await next.incoming.close();
    await subscription.cancel();
  });

  test('concurrent connect requests share one pending handshake', () async {
    final channel = _DelayedChannel();
    var opened = 0;
    final transport = WebSocketTransport(
        url: 'ws://example.invalid/ws/v2',
        channelFactory: (_) {
          opened++;
          return channel;
        });
    final first = transport.connect(), second = transport.connect();
    expect(opened, 1);
    channel.handshake.complete();
    await Future.wait([first, second]);
    await transport.connect();
    expect(opened, 1);
    await transport.close();
    await channel.incoming.close();
  });

  test(
      'stale socket callbacks cannot disconnect or expose errors on a fresh connection',
      () async {
    final first = _DelayedChannel()..handshake.complete();
    final next = _DelayedChannel()..handshake.complete();
    var attempt = 0;
    final transport = WebSocketTransport(
        url: 'ws://example.invalid/ws/v2',
        channelFactory: (_) => attempt++ == 0 ? first : next);
    final errors = <Object>[];
    final subscription = transport.messages.listen((_) {}, onError: errors.add);
    await transport.connect();
    await transport.reconnect();
    first.incoming.addError(StateError('discarded socket'));
    await first.incoming.close();
    expect(errors, isEmpty);
    expect(transport.isConnected, isTrue);
    await transport.close();
    next.incoming.addError(StateError('disposed socket'));
    await next.incoming.close();
    await subscription.cancel();
  });

  test('connected is announced only after the socket handshake completes',
      () async {
    final channel = _DelayedChannel();
    final transport = WebSocketTransport(
        url: 'ws://example.invalid/ws/v2', channelFactory: (_) => channel);
    final connecting = transport.connect();
    await Future<void>.delayed(Duration.zero);
    expect(transport.isConnected, isFalse);
    channel.handshake.complete();
    await connecting;
    expect(transport.isConnected, isTrue);
    await transport.close();
    await channel.incoming.close();
  });

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
