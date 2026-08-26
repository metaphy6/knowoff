import 'dart:convert';
import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';

/// Client-side configuration: server URL, feature flags, and localization
/// settings. Loaded from an asset bundle JSON file so the same build can be
/// pointed at different environments without recompilation.
class ClientConfig {
  const ClientConfig({
    required this.serverUrl,
    required this.websocketUrl,
    required this.protocolVersion,
    required this.featureFlags,
    required this.supportedLocales,
    required this.defaultLocale,
  });

  final String serverUrl;
  final String websocketUrl;
  final int protocolVersion;
  final Map<String, dynamic> featureFlags;
  final List<String> supportedLocales;
  final String defaultLocale;

  /// Loads config from `assets/config.json` with a fallback to sensible
  /// development defaults.
  static Future<ClientConfig> load() async {
    try {
      final raw = await rootBundle.loadString('assets/config.json');
      final json = jsonDecode(raw) as Map<String, dynamic>;
      return ClientConfig.fromJson(json);
    } on Exception {
      return defaultConfig();
    }
  }

  factory ClientConfig.fromJson(Map<String, dynamic> json) {
    return ClientConfig(
      serverUrl: _resolveHostForEmulator(
        json['serverUrl'] as String? ?? 'http://localhost:8080',
      ),
      websocketUrl: _resolveHostForEmulator(
        json['websocketUrl'] as String? ?? 'ws://localhost:8080/ws',
      ),
      protocolVersion: json['protocolVersion'] as int? ?? 1,
      featureFlags: (json['featureFlags'] as Map<String, dynamic>?) ?? const {},
      supportedLocales:
          (json['supportedLocales'] as List<dynamic>?)?.cast<String>() ??
              const ['en'],
      defaultLocale: json['defaultLocale'] as String? ?? 'en',
    );
  }

  /// The Android emulator has its own loopback network, so `localhost`/
  /// `127.0.0.1` in config.json (meant for the host machine) must be
  /// rewritten to the emulator's host-loopback alias `10.0.2.2`.
  static String _resolveHostForEmulator(String url) {
    if (kIsWeb || defaultTargetPlatform != TargetPlatform.android) return url;
    return url
        .replaceFirst('localhost', '10.0.2.2')
        .replaceFirst('127.0.0.1', '10.0.2.2');
  }

  static ClientConfig defaultConfig() => ClientConfig(
        serverUrl: _resolveHostForEmulator('http://localhost:8080'),
        websocketUrl: _resolveHostForEmulator('ws://localhost:8080/ws'),
        protocolVersion: 1,
        featureFlags: const {},
        supportedLocales: const ['en'],
        defaultLocale: 'en',
      );

  bool isEnabled(String flag) => featureFlags[flag] == true;
}
