import 'package:flutter/foundation.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/core/config/client_config.dart';

void main() {
  test(
    'new client defaults and bundled config use incompatible text generation',
    () async {
      expect(ClientConfig.defaultConfig().protocolVersion, 2);
      expect(ClientConfig.defaultConfig().websocketUrl, endsWith('/ws/v2'));
    },
  );

  tearDown(() {
    debugDefaultTargetPlatformOverride = null;
  });

  group('ClientConfig.fromJson host resolution', () {
    test('leaves localhost as-is on non-Android platforms', () {
      debugDefaultTargetPlatformOverride = TargetPlatform.linux;

      final config = ClientConfig.fromJson(const {
        'serverUrl': 'http://localhost:8080',
        'websocketUrl': 'ws://localhost:8080/ws',
      });

      expect(config.serverUrl, 'http://localhost:8080');
      expect(config.websocketUrl, 'ws://localhost:8080/ws');
    });

    test('rewrites localhost to 10.0.2.2 on Android (emulator loopback)', () {
      debugDefaultTargetPlatformOverride = TargetPlatform.android;

      final config = ClientConfig.fromJson(const {
        'serverUrl': 'http://localhost:8080',
        'websocketUrl': 'ws://127.0.0.1:8080/ws',
      });

      expect(config.serverUrl, 'http://10.0.2.2:8080');
      expect(config.websocketUrl, 'ws://10.0.2.2:8080/ws');
    });

    test('leaves non-loopback hosts untouched on Android', () {
      debugDefaultTargetPlatformOverride = TargetPlatform.android;

      final config = ClientConfig.fromJson(const {
        'serverUrl': 'http://api.knowoff.example:8080',
      });

      expect(config.serverUrl, 'http://api.knowoff.example:8080');
    });
  });
}
