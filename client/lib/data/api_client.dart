import 'dart:convert';

import 'package:http/http.dart' as http;

import 'auth_service.dart';

/// HTTP API client for Phase 4–5 endpoints: profile, leaderboard, economy,
/// notices, reports, feedback, and avatar upload.
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

  Future<Map<String, dynamic>> _postJson(
      String path, Map<String, dynamic> body) async {
    await _auth.ensureSession();
    final res = await _client.post(
      Uri.parse('$_baseUrl$path'),
      headers: {
        'Authorization': 'Bearer ${_auth.accessToken}',
        'Content-Type': 'application/json',
      },
      body: jsonEncode(body),
    );
    if (res.statusCode == 401) {
      await _auth.refresh();
      return _postJson(path, body);
    }
    if (res.statusCode < 200 || res.statusCode >= 300) {
      throw Exception('api error ${res.statusCode}: ${res.body}');
    }
    return jsonDecode(res.body) as Map<String, dynamic>;
  }

  Future<void> _post(String path, Map<String, dynamic> body) async {
    await _postJson(path, body);
  }

  Future<Map<String, dynamic>> getProfile() => _get('/api/profile');

  Future<void> updateNickname(String nickname) =>
      _patch('/api/profile/nickname', {'nickname': nickname});

  Future<void> updateAvatar(String avatar) =>
      _patch('/api/profile/avatar', {'avatar': avatar});

  Future<Map<String, dynamic>> getLeaderboard() => _get('/api/leaderboard');

  // ---- Rooms ---------------------------------------------------------

  Future<Map<String, dynamic>> createRoom(int size) =>
      _postJson('/rooms/create', {'size': size});

  // ---- Economy -----------------------------------------------------------

  Future<Map<String, dynamic>> getWallet() => _get('/api/economy/wallet');

  Future<Map<String, dynamic>> getStoreCatalog() =>
      _get('/api/economy/catalog');

  Future<Map<String, dynamic>> convertPoints(int points) =>
      _postJson('/api/economy/convert', {'points': points});

  Future<void> purchasePlayPass(String type) =>
      _post('/api/economy/playpass', {'type': type});

  Future<void> purchaseUnlock(String type, {String value = ''}) =>
      _post('/api/economy/unlock', {'type': type, 'value': value});

  // ---- Notices -----------------------------------------------------------

  Future<List<dynamic>> getNotices() async {
    final data = await _get('/api/notices');
    return data['notices'] as List<dynamic>;
  }

  // ---- Reports & feedback ------------------------------------------------

  Future<void> createReport({
    required String reportType,
    String? targetAccountID,
    String? targetMediaID,
    required String reason,
    String? description,
  }) =>
      _post('/api/reports', {
        'type': reportType,
        if (targetAccountID != null) 'target_account_id': targetAccountID,
        if (targetMediaID != null) 'target_media_id': targetMediaID,
        'reason': reason,
        if (description != null) 'description': description,
      });

  Future<void> createFeedback({
    required String type,
    required String title,
    required String message,
    Map<String, dynamic>? contextSnapshot,
  }) =>
      _post('/api/feedback', {
        'type': type,
        'title': title,
        'message': message,
        if (contextSnapshot != null) 'context_snapshot': contextSnapshot,
      });

  // ---- Avatar upload -----------------------------------------------------

  Future<void> uploadAvatar(List<int> bytes, String filename) async {
    await _auth.ensureSession();
    final uri = Uri.parse('$_baseUrl/api/avatar');
    final request = http.MultipartRequest('POST', uri)
      ..headers['Authorization'] = 'Bearer ${_auth.accessToken}'
      ..files.add(
          http.MultipartFile.fromBytes('avatar', bytes, filename: filename));
    final streamed = await _client.send(request);
    final res = await http.Response.fromStream(streamed);
    if (res.statusCode == 401) {
      await _auth.refresh();
      return uploadAvatar(bytes, filename);
    }
    if (res.statusCode < 200 || res.statusCode >= 300) {
      throw Exception('api error ${res.statusCode}: ${res.body}');
    }
  }
}
