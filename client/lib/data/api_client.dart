import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';

import 'package:http/http.dart' as http;

import 'auth_service.dart';
import 'bonus_delivery.dart';
import '../core/text/v2_contract.dart' show V2Codec, V2Failure;
import 'purchases.dart';
import 'rewarded_ads.dart';

class ApiException implements Exception {
  const ApiException(this.statusCode, {this.code});
  final int statusCode;
  final String? code;
  @override
  String toString() => 'API request failed ($statusCode)';
}

/// Authenticated HTTP client for player services and community participation.
class ApiClient {
  ApiClient({required this._baseUrl, required this._auth, http.Client? client})
    : _client = client ?? http.Client();

  final String _baseUrl;
  final AuthService _auth;

  AuthService get authService => _auth;
  final http.Client _client;

  Future<Map<String, dynamic>> _get(
    String path, {
    bool retryAuth = true,
  }) async {
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

  Future<void> _patch(
    String path,
    Map<String, dynamic> body, {
    bool retryAuth = true,
    String? expectedAccount,
  }) async {
    await _auth.ensureSession();
    if (expectedAccount != null &&
        expectedAccount.isNotEmpty &&
        expectedAccount != _auth.accountId) {
      throw const ApiException(401, code: 'auth.required');
    }
    final account = _auth.accountId;
    final res = await _client.patch(
      Uri.parse('$_baseUrl$path'),
      headers: {
        'Authorization': 'Bearer ${_auth.accessToken}',
        'Content-Type': 'application/json',
      },
      body: jsonEncode(body),
    );
    if (expectedAccount != null && account != _auth.accountId) {
      throw const ApiException(401, code: 'auth.required');
    }
    if (res.statusCode == 401 && retryAuth) {
      await _auth.refresh();
      return _patch(
        path,
        body,
        retryAuth: false,
        expectedAccount: expectedAccount == null ? null : account ?? '',
      );
    }
    if (res.statusCode < 200 || res.statusCode >= 300) {
      throw _error(res);
    }
  }

  Future<Map<String, dynamic>> _postJson(
    String path,
    Map<String, dynamic> body, {
    bool retryAuth = true,
    String? expectedAccount,
  }) async {
    await _auth.ensureSession();
    if (expectedAccount != null &&
        expectedAccount.isNotEmpty &&
        expectedAccount != _auth.accountId) {
      throw const ApiException(401, code: 'auth.required');
    }
    final account = _auth.accountId;
    final res = await _client.post(
      Uri.parse('$_baseUrl$path'),
      headers: {
        'Authorization': 'Bearer ${_auth.accessToken}',
        'Content-Type': 'application/json',
      },
      body: jsonEncode(body),
    );
    if (expectedAccount != null && account != _auth.accountId) {
      throw const ApiException(401, code: 'auth.required');
    }
    if (res.statusCode == 401 && retryAuth) {
      await _auth.refresh();
      return _postJson(
        path,
        body,
        retryAuth: false,
        expectedAccount: expectedAccount == null ? null : account ?? '',
      );
    }
    if (res.statusCode < 200 || res.statusCode >= 300) {
      throw _error(res);
    }
    if (res.statusCode == 204 || res.body.trim().isEmpty) return {};
    return jsonDecode(res.body) as Map<String, dynamic>;
  }

  Future<Map<String, dynamic>> _bonusRequest(
    String path,
    Map<String, dynamic>? body,
    String account,
    int generation, {
    String method = 'POST',
    bool retry = true,
  }) async {
    void requireCurrent() {
      if (_auth.accountId != account || _auth.sessionGeneration != generation) {
        throw const ApiException(401, code: 'auth.required');
      }
    }

    requireCurrent();
    if (!await _auth.restoreExistingSession()) {
      throw const ApiException(401, code: 'auth.required');
    }
    requireCurrent();
    final abort = Completer<void>();
    final request = http.AbortableRequest(
      method,
      Uri.parse('$_baseUrl$path'),
      abortTrigger: abort.future,
    )..headers['Authorization'] = 'Bearer ${_auth.accessToken}';
    if (body != null) {
      request.headers['Content-Type'] = 'application/json';
      request.body = jsonEncode(body);
    }
    var accepting = true;
    final sending = _client.send(request).then((response) async {
      // Custom clients may ignore Abortable. Dispose late headers explicitly.
      if (!accepting) {
        await response.stream
            .listen((_) {}, onError: (Object _, StackTrace __) {})
            .cancel()
            .timeout(const Duration(seconds: 1));
        throw TimeoutException('reward.bonus_delivery_invalid');
      }
      return response;
    });
    late http.StreamedResponse streamed;
    late Uint8List raw;
    try {
      streamed = await sending.timeout(const Duration(seconds: 15));
      raw = await _readBoundedResponse(
        streamed.stream,
        65536,
        'reward.bonus_delivery_invalid',
      );
    } finally {
      accepting = false;
      abort.complete();
    }
    requireCurrent();
    if (streamed.statusCode == 401 && retry) {
      await _auth.refresh();
      requireCurrent();
      return _bonusRequest(
        path,
        body,
        account,
        generation,
        method: method,
        retry: false,
      );
    }
    final response = http.Response.bytes(
      raw,
      streamed.statusCode,
      headers: streamed.headers,
    );
    if (response.statusCode != 200) throw _error(response);
    try {
      return V2Codec.object(utf8.decode(raw), maxBytes: 65536);
    } on V2Failure {
      throw const FormatException('Invalid bonus response');
    }
  }

  Future<Map<String, dynamic>> getExistingSessionWallet(
    String account,
    int generation,
  ) async {
    final data = await _bonusRequest(
      '/api/economy/wallet',
      null,
      account,
      generation,
      method: 'GET',
    );
    const fields = {
      'balance',
      'noin',
      'daily_earned',
      'daily_earn_cap',
      'points_to_noin',
      'free_daily_matches',
      'non_converted_points',
    };
    if (data.length != fields.length ||
        !fields.every(data.containsKey) ||
        data.values.any((v) => v is! int || v < 0 || v > 9007199254740991) ||
        data['balance'] != data['noin'] ||
        data['points_to_noin'] == 0) {
      throw const FormatException('Invalid wallet response');
    }
    return Map.unmodifiable(data);
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
  }) => _postJson('/api/challenge/entry', {
    'topic_id': topicId,
    'content': content,
    'terms_version': termsVersion,
    'terms_accepted': true,
  });

  Future<void> voteChallenge({
    required String topicId,
    required String entryId,
  }) =>
      _post('/api/challenge/vote', {'topic_id': topicId, 'entry_id': entryId});

  Future<Map<String, dynamic>> getProfile() => _get('/api/profile');

  /// Another player's public career profile. Unconverted points are omitted
  /// server-side for non-owners.
  Future<Map<String, dynamic>> getPublicProfile(String accountID) =>
      _get('/api/profile/$accountID');

  Future<void> updateNickname(String nickname) =>
      _patch('/api/profile/nickname', {'nickname': nickname});

  Future<Uint8List?> getAvatarImage(String accountID) =>
      _getAvatarImage(accountID, _auth.accountId);

  Future<Uint8List?> _getAvatarImage(
    String accountID,
    String? expectedAccount, {
    bool retryAuth = true,
  }) async {
    await _auth.ensureSession();
    if (expectedAccount != null && expectedAccount != _auth.accountId) {
      throw const ApiException(401, code: 'auth.required');
    }
    final account = _auth.accountId;
    final request = http.Request(
      'GET',
      Uri.parse('$_baseUrl/api/avatar/${Uri.encodeComponent(accountID)}'),
    )..headers['Authorization'] = 'Bearer ${_auth.accessToken}';
    final response = await _client
        .send(request)
        .timeout(const Duration(seconds: 15));
    final bytes = await _readBoundedResponse(
      response.stream,
      2 * 1024 * 1024,
      'avatar.invalid',
    );
    if (account != _auth.accountId) {
      throw const ApiException(401, code: 'auth.required');
    }
    if (response.statusCode == 401 && retryAuth) {
      await _auth.refresh();
      if (account != _auth.accountId) {
        throw const ApiException(401, code: 'auth.required');
      }
      return _getAvatarImage(accountID, account, retryAuth: false);
    }
    if (response.statusCode == 404) return null;
    if (response.statusCode != 200) throw ApiException(response.statusCode);
    if (response.headers['content-type']?.split(';').first != 'image/webp' ||
        bytes.isEmpty) {
      throw const ApiException(502, code: 'avatar.invalid');
    }
    return bytes;
  }

  Future<void> updateAvatar(String avatar) => _patch('/api/profile/avatar', {
    'avatar': avatar,
  }, expectedAccount: _auth.accountId ?? '');

  Future<Map<String, dynamic>> getLeaderboard() => _get('/api/leaderboard');

  // ---- Rooms ---------------------------------------------------------

  Future<Map<String, dynamic>> createRoom(int size) =>
      _postJson('/rooms/create', {'size': size});

  // ---- Economy -----------------------------------------------------------

  Future<Map<String, dynamic>> getWallet() => _get('/api/economy/wallet');

  Future<Map<String, dynamic>> getStoreCatalog() => _get('/api/economy/store');

  Future<PurchaseVerification> verifyPurchase(
    PurchaseReceipt receipt, {
    required String accountID,
  }) async {
    void sameAccount() {
      if (!purchaseAccountID(accountID) || _auth.accountId != accountID) {
        throw const AuthSessionException('billing.account_changed');
      }
    }

    sameAccount(); // Receipt replay must never create an anonymous identity.
    if (!await _auth.restoreExistingSession()) {
      throw const AuthSessionException('auth.restore_required');
    }
    sameAccount();
    final body = jsonEncode(receipt.toJson());
    for (var attempt = 0; attempt < 2; attempt++) {
      sameAccount();
      final request =
          http.Request(
              'POST',
              Uri.parse('$_baseUrl/api/economy/purchase/receipt'),
            )
            ..headers.addAll({
              'Authorization': 'Bearer ${_auth.accessToken}',
              'Content-Type': 'application/json',
            })
            ..body = body;
      final streamed = await _client
          .send(request)
          .timeout(const Duration(seconds: 40));
      final bytes = await _readBoundedResponse(
        streamed.stream,
        2048,
        'billing.response',
      );
      final response = http.Response.bytes(
        bytes,
        streamed.statusCode,
        headers: streamed.headers,
      );
      sameAccount();
      if (response.statusCode == 401 && attempt == 0) {
        await _auth.refresh();
        sameAccount();
        continue;
      }
      if (response.statusCode != 200) throw _error(response);
      return PurchaseVerification.decode(jsonDecode(response.body));
    }
    throw const AuthSessionException('auth.restore_required');
  }

  Future<Map<String, dynamic>> convertPoints(int points) =>
      _postJson('/api/economy/convert', {'points': points});

  Future<void> purchasePlayPass(String type) =>
      _post('/api/economy/purchase/playpass', {'type': type});

  Future<void> purchaseUnlock(String type, {String value = ''}) async {
    await _postJson('/api/economy/purchase/unlock', {
      'type': type,
      'value': value,
    }, expectedAccount: type == 'custom_avatar' ? _auth.accountId ?? '' : null);
  }

  // ---- Notices -----------------------------------------------------------

  Future<List<dynamic>> getNotices() async {
    final data = await _get('/api/notices');
    return data['notices'] as List<dynamic>;
  }

  // Safety responses are private; keep them out of shared caches and URLs.
  Future<Map<String, dynamic>> getSafety() => _get('/api/safety');

  /// Public configuration only, available even when a saved identity is locked.
  Future<Map<String, dynamic>> getSafetyHelp() async {
    final res = await _client.get(Uri.parse('$_baseUrl/api/safety/help'));
    if (res.statusCode != 200) throw _error(res);
    return jsonDecode(res.body) as Map<String, dynamic>;
  }

  Future<void> acceptUserTerms(String version) =>
      _post('/api/safety/terms', {'version': version});
  Future<Map<String, dynamic>> getBlocks({String after = ''}) => _get(
    '/api/safety/blocks${after.isEmpty ? '' : '?after=${Uri.encodeQueryComponent(after)}'}',
  );
  Future<void> blockAccount(String accountID) =>
      _post('/api/safety/blocks', {'account_id': accountID});
  Future<void> unblockAccount(String accountID) =>
      _delete('/api/safety/blocks/${Uri.encodeComponent(accountID)}');
  Future<Map<String, dynamic>> getRoomSeatIdentity(String roomID, int seat) =>
      _get('/api/safety/rooms/${Uri.encodeComponent(roomID)}/seats/$seat');
  Future<Map<String, dynamic>> getMatchSeatIdentity(String matchID, int seat) =>
      _get('/api/safety/matches/${Uri.encodeComponent(matchID)}/seats/$seat');

  Future<void> _delete(String path, {bool retryAuth = true}) async {
    await _auth.ensureSession();
    final res = await _client.delete(
      Uri.parse('$_baseUrl$path'),
      headers: {'Authorization': 'Bearer ${_auth.accessToken}'},
    );
    if (res.statusCode == 401 && retryAuth) {
      await _auth.refresh();
      return _delete(path, retryAuth: false);
    }
    if (res.statusCode != 204) throw _error(res);
  }

  // ---- Reports & feedback ------------------------------------------------

  Future<void> createTextReport({
    required String matchID,
    required String contentID,
    required int revision,
    required String reason,
  }) => _post('/api/reports', {
    'report_type': 'media',
    'target_text': {
      'match_id': matchID,
      'content_ref': {'content_id': contentID, 'revision': revision},
    },
    'reason': reason,
  });

  Future<void> createReport({
    required String reportType,
    String? targetAccountID,
    String? targetMediaID,
    required String reason,
    String? description,
  }) => _post('/api/reports', {
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
  }) => _post('/api/feedback', {
    'type': type,
    'title': title,
    'message': message,
    if (contextSnapshot != null) 'context_snapshot': contextSnapshot,
  });

  // A response owns one total deadline, including an active trickle. Every
  // completion releases its subscription and timer; callers never retain bytes
  // beyond the size cap or apply a result under a replacement account.
  Future<Uint8List> _readBoundedResponse(
    Stream<List<int>> stream,
    int maxBytes,
    String code,
  ) async {
    final result = Completer<Uint8List>();
    final bytes = BytesBuilder(copy: false);
    final subscription = stream.listen(
      (chunk) {
        if (result.isCompleted) return;
        if (bytes.length + chunk.length > maxBytes) {
          result.completeError(ApiException(502, code: code));
          return;
        }
        bytes.add(chunk);
      },
      onError: (Object error, StackTrace stack) {
        if (!result.isCompleted) result.completeError(error, stack);
      },
      onDone: () {
        if (!result.isCompleted) result.complete(bytes.takeBytes());
      },
    );
    final deadline = Timer(const Duration(seconds: 15), () {
      if (!result.isCompleted) {
        result.completeError(TimeoutException(code));
      }
    });
    try {
      return await result.future;
    } finally {
      deadline.cancel();
      await subscription.cancel().timeout(const Duration(seconds: 1));
    }
  }

  // ---- Avatar upload -----------------------------------------------------

  Future<void> uploadAvatar(List<int> bytes, String filename) =>
      _uploadAvatar(bytes, filename, expectedAccount: _auth.accountId);

  Future<void> _uploadAvatar(
    List<int> bytes,
    String filename, {
    bool retryAuth = true,
    String? expectedAccount,
  }) async {
    if (bytes.isEmpty || bytes.length > 2 * 1024 * 1024) {
      throw const ApiException(400, code: 'avatar.invalid');
    }
    await _auth.ensureSession();
    if (expectedAccount != null && expectedAccount != _auth.accountId) {
      throw const ApiException(401, code: 'auth.required');
    }
    final account = _auth.accountId;
    final uri = Uri.parse('$_baseUrl/api/avatar');
    final request = http.MultipartRequest('POST', uri)
      ..headers['Authorization'] = 'Bearer ${_auth.accessToken}'
      ..files.add(
        http.MultipartFile.fromBytes('avatar', bytes, filename: filename),
      );
    final streamed = await _client
        .send(request)
        .timeout(const Duration(seconds: 45));
    final responseBytes = await _readBoundedResponse(
      streamed.stream,
      65536,
      'avatar.unavailable',
    );
    final res = http.Response.bytes(
      responseBytes,
      streamed.statusCode,
      headers: streamed.headers,
    );
    if (account != _auth.accountId) {
      throw const ApiException(401, code: 'auth.required');
    }
    if (res.statusCode == 401 && retryAuth) {
      await _auth.refresh();
      return _uploadAvatar(
        bytes,
        filename,
        retryAuth: false,
        expectedAccount: account,
      );
    }
    if (res.statusCode < 200 || res.statusCode >= 300) {
      throw _error(res);
    }
  }
}

/// Explicitly constructed only after the joined reward activation gate.
class ApiBonusDeliveryTransport implements BonusDeliveryTransport {
  ApiBonusDeliveryTransport(this._api, {required this._refreshBalance});
  final ApiClient _api;
  final Future<void> Function(String account, int generation) _refreshBalance;
  @override
  String? get accountId => _api.authService.accountId;
  @override
  int get sessionGeneration => _api.authService.sessionGeneration;
  Future<Map<String, dynamic>> _post(
    String path,
    Map<String, dynamic> body,
  ) async {
    final account = accountId;
    if (account == null) throw const ApiException(401, code: 'auth.required');
    return _api._bonusRequest(path, body, account, sessionGeneration);
  }

  @override
  Future<Map<String, dynamic>> claim(int limit, List<String> pendingIds) =>
      _post('/v2/rewards/bonuses/claim', {
        'limit': limit,
        'pending_delivery_ids': pendingIds,
      });
  @override
  Future<void> acknowledge(String id, String lease) async {
    final result = await _post('/v2/rewards/bonuses/ack', {
      'delivery_id': id,
      'lease': lease,
    });
    if (result.length != 2 ||
        result['version'] is! int ||
        result['version'] != 1 ||
        result['acknowledged'] != true) {
      throw const FormatException('Invalid bonus acknowledgment');
    }
  }

  @override
  Future<void> refreshBalance() async {
    final account = accountId;
    if (account != null) await _refreshBalance(account, sessionGeneration);
  }
}

/// Uses the same bounded existing-session transport as private bonus delivery.
class ApiRewardClaimTransport implements RewardClaimTransport {
  ApiRewardClaimTransport(
    this._api, {
    required Future<void> Function(String account, int generation) refreshBonus,
  }) : _refresh = refreshBonus;
  final ApiClient _api;
  final Future<void> Function(String account, int generation) _refresh;
  @override
  String? get accountId => _api.authService.accountId;
  @override
  int get sessionGeneration => _api.authService.sessionGeneration;
  Future<Map<String, dynamic>> _post(
    String path,
    Map<String, dynamic> body,
  ) async {
    final account = accountId;
    if (account == null) throw const ApiException(401, code: 'auth.required');
    return _api._bonusRequest(path, body, account, sessionGeneration);
  }

  @override
  Future<Map<String, dynamic>> issue(String match, String providerUnit) =>
      _post('/v2/rewards/claim', {'match_id': match, 'ad_unit': providerUnit});
  @override
  Future<void> check(String match, RewardClaim claim) async {
    final data = await _post('/v2/rewards/claim/check', {
      'match_id': match,
      'ad_unit': claim.adUnit,
      'claim': claim.opaque,
    });
    if (data.length != 2 ||
        data['version'] is! int ||
        data['version'] != 1 ||
        data['eligible'] != true) {
      throw const FormatException('Invalid reward check');
    }
  }

  @override
  Future<void> refreshBonus() async {
    final account = accountId;
    if (account != null) await _refresh(account, sessionGeneration);
  }
}
