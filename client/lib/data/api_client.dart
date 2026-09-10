import 'dart:convert';

import 'package:http/http.dart' as http;

import 'auth_service.dart';

class ApiException implements Exception {
  const ApiException(this.statusCode, {this.code});
  final int statusCode;
  final String? code;
  @override
  String toString() => 'API request failed ($statusCode)';
}

/// Authenticated HTTP client for player services and community participation.
class ApiClient {
  ApiClient(
      {required String baseUrl, required AuthService auth, http.Client? client})
      : _baseUrl = baseUrl,
        _auth = auth,
        _client = client ?? http.Client();

  final String _baseUrl;
  final AuthService _auth;
  final http.Client _client;

  Future<Map<String, dynamic>> _get(String path,
      {bool retryAuth = true}) async {
    await _auth.ensureSession();
    final res = await _client.get(
      Uri.parse('$_baseUrl$path'),
      headers: {'Authorization': 'Bearer ${_auth.accessToken}'},
    );
    if (res.statusCode == 401 && retryAuth) {
      await _auth.refresh();
      return _get(path, retryAuth: false);
    }
    if (res.statusCode == 204) return {};
    if (res.statusCode != 200) throw _error(res);
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
      throw _error(res);
    }
  }

  Future<Map<String, dynamic>> _postJson(String path, Map<String, dynamic> body,
      {bool retryAuth = true}) async {
    await _auth.ensureSession();
    final res = await _client.post(
      Uri.parse('$_baseUrl$path'),
      headers: {
        'Authorization': 'Bearer ${_auth.accessToken}',
        'Content-Type': 'application/json',
      },
      body: jsonEncode(body),
    );
    if (res.statusCode == 401 && retryAuth) {
      await _auth.refresh();
      return _postJson(path, body, retryAuth: false);
    }
    if (res.statusCode < 200 || res.statusCode >= 300) {
      throw _error(res);
    }
    if (res.statusCode == 204 || res.body.trim().isEmpty) return {};
    return jsonDecode(res.body) as Map<String, dynamic>;
  }

  Future<void> _post(String path, Map<String, dynamic> body) async {
    await _postJson(path, body);
  }

  ApiException _error(http.Response response) {
    String? code;
    try {
      final data = jsonDecode(response.body);
      if (data is Map<String, dynamic> && data['code'] is String) {
        code = data['code'] as String;
      }
    } on FormatException {
      // Non-JSON gateway errors still provide an actionable HTTP status.
    }
    return ApiException(response.statusCode, code: code);
  }

  Uri get portalLoginUri =>
      Uri.parse('${Uri.parse(_baseUrl).origin}/portal/login');

  Future<void> connectPortal(String code) =>
      _post('/api/portal/connect', {'code': code});

  Future<Map<String, dynamic>> getActiveChallenge() =>
      _get('/api/challenge/active');

  Future<Map<String, dynamic>> submitChallengeEntry({
    required String topicId,
    required String content,
    required String termsVersion,
  }) =>
      _postJson('/api/challenge/entry', {
        'topic_id': topicId,
        'content': content,
        'terms_version': termsVersion,
        'terms_accepted': true,
      });

  Future<void> voteChallenge(
          {required String topicId, required String entryId}) =>
      _post('/api/challenge/vote', {'topic_id': topicId, 'entry_id': entryId});

  Future<Map<String, dynamic>> getProfile() => _get('/api/profile');

  /// Another player's public career profile. Unconverted points are omitted
  /// server-side for non-owners.
  Future<Map<String, dynamic>> getPublicProfile(String accountID) =>
      _get('/api/profile/$accountID');

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

  Future<Map<String, dynamic>> getStoreCatalog() => _get('/api/economy/store');

  Future<Map<String, dynamic>> convertPoints(int points) =>
      _postJson('/api/economy/convert', {'points': points});

  Future<void> purchasePlayPass(String type) =>
      _post('/api/economy/purchase/playpass', {'type': type});

  Future<void> purchaseUnlock(String type, {String value = ''}) =>
      _post('/api/economy/purchase/unlock', {'type': type, 'value': value});

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
        'report_type': reportType,
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
