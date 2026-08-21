import 'dart:convert';
import 'dart:math';

import 'package:crypto/crypto.dart';
import 'package:http/http.dart' as http;
import 'package:shared_preferences/shared_preferences.dart';

/// A lightweight device-authentication service. Anonymous device accounts are
/// created on first launch; tokens are persisted locally.
class AuthService {
  AuthService({required String baseUrl, http.Client? client})
      : _baseUrl = baseUrl,
        _client = client ?? http.Client();

  final String _baseUrl;
  final http.Client _client;

  static const _accessKey = 'knowoff_access_token';
  static const _refreshKey = 'knowoff_refresh_token';
  static const _accountKey = 'knowoff_account_id';

  String? _accessToken;
  String? _refreshToken;
  String? _accountId;

  String? get accessToken => _accessToken;
  String? get accountId => _accountId;

  /// Ensures the device has an account and valid tokens.
  Future<void> ensureSession() async {
    final prefs = await SharedPreferences.getInstance();
    _accessToken = prefs.getString(_accessKey);
    _refreshToken = prefs.getString(_refreshKey);
    _accountId = prefs.getString(_accountKey);

    if (_accessToken != null &&
        _refreshToken != null &&
        _accountId != null &&
        !_isTokenExpired(_accessToken!)) {
      return;
    }

    // Access token missing or expired: try to refresh with the stored
    // refresh token before falling back to a brand-new device session.
    if (_refreshToken != null && !_isTokenExpired(_refreshToken!)) {
      try {
        await refresh();
        return;
      } catch (_) {
        // Fall through to new device session.
      }
    }

    final deviceHash = _deviceHash();
    final res = await _client.post(
      Uri.parse('$_baseUrl/api/auth/device'),
      headers: {'Content-Type': 'application/json'},
      body: jsonEncode({'device_hash': deviceHash}),
    );
    if (res.statusCode != 200) {
      throw Exception('device auth failed: ${res.statusCode}');
    }
    final data = jsonDecode(res.body) as Map<String, dynamic>;
    await _persist(data);
  }

  bool _isTokenExpired(String token) {
    try {
      final parts = token.split('.');
      if (parts.length != 3) return true;
      final payload = jsonDecode(
        utf8.decode(base64Url.decode(base64Url.normalize(parts[1]))),
      ) as Map<String, dynamic>;
      final exp = payload['exp'] as int?;
      if (exp == null) return true;
      return DateTime.now().isAfter(
        DateTime.fromMillisecondsSinceEpoch(exp * 1000),
      );
    } catch (_) {
      return true;
    }
  }

  /// Refreshes the access token using the stored refresh token.
  Future<void> refresh() async {
    final prefs = await SharedPreferences.getInstance();
    final refresh = prefs.getString(_refreshKey);
    if (refresh == null) return ensureSession();

    final res = await _client.post(
      Uri.parse('$_baseUrl/api/auth/refresh'),
      headers: {'Content-Type': 'application/json'},
      body: jsonEncode({'refresh_token': refresh}),
    );
    if (res.statusCode != 200) {
      return ensureSession();
    }
    final data = jsonDecode(res.body) as Map<String, dynamic>;
    await _persist(data);
  }

  Future<void> _persist(Map<String, dynamic> data) async {
    final prefs = await SharedPreferences.getInstance();
    _accessToken = data['access_token'] as String?;
    _refreshToken = data['refresh_token'] as String?;
    _accountId = data['account_id'] as String?;
    await prefs.setString(_accessKey, _accessToken ?? '');
    await prefs.setString(_refreshKey, _refreshToken ?? '');
    await prefs.setString(_accountKey, _accountId ?? '');
  }

  String _deviceHash() {
    final bytes = utf8.encode(
        'knowoff-device-${DateTime.now().millisecondsSinceEpoch}-${Random().nextInt(1 << 30)}');
    return base64Encode(sha256.convert(bytes).bytes);
  }
}
