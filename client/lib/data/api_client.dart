import 'dart:convert';

import 'package:http/http.dart' as http;

import 'auth_service.dart';

/// HTTP API client for Phase 4 endpoints: profile and leaderboard.
class ApiClient {
  ApiClient(
      {required String baseUrl, required AuthService auth, http.Client? client})
      : _baseUrl = baseUrl,
        _auth = auth,
        _client = client ?? http.Client();

  final String _baseUrl;
  final AuthService _auth;
  final http.Client _client;

  Future<Map<String, dynamic>> _get(String path) async {
    await _auth.ensureSession();
    final res = await _client.get(
      Uri.parse('$_baseUrl$path'),
      headers: {'Authorization': 'Bearer ${_auth.accessToken}'},
    );
    if (res.statusCode == 401) {
      await _auth.refresh();
      return _get(path);
    }
    if (res.statusCode != 200) {
      throw Exception('api error ${res.statusCode}: ${res.body}');
    }
    return jsonDecode(res.body) as Map<String, dynamic>;
  }

  Future<void> _patch(String path, Map<String, dynamic> body) async {
    await _auth.ensureSession();
    final res = await _client.patch(
      Uri.parse('$_baseUrl$path'),
      headers: {
        'Authorization': 'Bearer ${_auth.accessToken}',
        'Content-Type': 'application/json',
      },
      body: jsonEncode(body),
    );
    if (res.statusCode == 401) {
      await _auth.refresh();
      return _patch(path, body);
    }
    if (res.statusCode < 200 || res.statusCode >= 300) {
      throw Exception('api error ${res.statusCode}: ${res.body}');
    }
  }

  Future<Map<String, dynamic>> getProfile() => _get('/api/profile');

  Future<void> updateNickname(String nickname) =>
      _patch('/api/profile/nickname', {'nickname': nickname});

  Future<void> updateAvatar(String avatar) =>
      _patch('/api/profile/avatar', {'avatar': avatar});

  Future<Map<String, dynamic>> getLeaderboard() => _get('/api/leaderboard');
}
