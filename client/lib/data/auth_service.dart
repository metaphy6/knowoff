import 'dart:async';
import 'dart:convert';
import 'dart:math';

import 'package:http/http.dart' as http;
import 'package:shared_preferences/shared_preferences.dart';

/// A saved identity could not be restored. Callers must offer a retry or account
/// recovery; creating a replacement account would lose the player's history.
class AuthSessionException implements Exception {
  const AuthSessionException(this.code);
  final String code;
  @override
  String toString() => 'Account session unavailable ($code)';
}

/// A lightweight device-authentication service. Anonymous device accounts are
/// created on first launch; tokens are persisted locally.
class AuthService {
  AuthService({
    required this._baseUrl,
    http.Client? client,
    DateTime Function()? now,
  }) : _client = client ?? http.Client(),
       _now = now ?? DateTime.now;

  final String _baseUrl;
  final http.Client _client;
  final DateTime Function() _now;
  Future<void> _storageTail = Future<void>.value();
  int _sessionGeneration = 0;

  static const _accessKey = 'knowoff_access_token';
  static const _refreshKey = 'knowoff_refresh_token';
  static const _accountKey = 'knowoff_account_id';
  static const _installationKey = 'knowoff_installation_id';
  static Future<void>? _installationTail;

  String? _accessToken;
  String? _refreshToken;
  String? _accountId;
  Future<void>? _ensuring;
  Future<void>? _refreshing;

  String? get accessToken => _accessToken;
  String? get accountId => _accountId;

  /// Background transaction recovery must never create a replacement identity.
  Future<bool> restoreExistingSession() async {
    final generation = _sessionGeneration;
    final prefs = await SharedPreferences.getInstance();
    if (!prefs.containsKey(_accountKey) &&
        !prefs.containsKey(_accessKey) &&
        !prefs.containsKey(_refreshKey)) {
      return false;
    }
    _load(prefs);
    final account = _accountId;
    if (account == null || _refreshToken == null) {
      throw const AuthSessionException('auth.restore_required');
    }
    await _installation();
    if (_accessToken == null ||
        _isTokenExpired(_accessToken!) ||
        _tokenInstallation(_refreshToken) == null) {
      await refresh();
    }
    if (generation != _sessionGeneration || _accountId != account) {
      throw const AuthSessionException('auth.restore_required');
    }
    return true;
  }

  /// Only a fresh installation may create an anonymous account. Restoration
  /// failures preserve stored identity and never fall back to device signup.
  Future<void> ensureSession() {
    if (_ensuring case final pending?) return pending;
    late final Future<void> operation;
    operation = _ensureSession().whenComplete(() {
      if (identical(_ensuring, operation)) _ensuring = null;
    });
    return _ensuring = operation;
  }

  Future<void> _ensureSession() async {
    final generation = _sessionGeneration;
    if (_refreshing case final pending?) return pending;
    final prefs = await SharedPreferences.getInstance();
    _load(prefs);
    await _installation();
    if (_accessToken != null &&
        _refreshToken != null &&
        _accountId != null &&
        !_isTokenExpired(_accessToken!) &&
        _tokenInstallation(_refreshToken) != null) {
      return;
    }
    if (prefs.containsKey(_accountKey) ||
        prefs.containsKey(_accessKey) ||
        prefs.containsKey(_refreshKey)) {
      return refresh();
    }

    final deviceHash = await _installation();
    final res = await _client
        .post(
          Uri.parse('$_baseUrl/api/auth/device'),
          headers: {'Content-Type': 'application/json'},
          body: jsonEncode({'device_hash': deviceHash}),
        )
        .timeout(const Duration(seconds: 15));
    if (res.statusCode != 200) {
      throw const AuthSessionException('auth.unavailable');
    }
    final data = jsonDecode(res.body) as Map<String, dynamic>;
    await _persist(data, expectedGeneration: generation);
  }

  bool _isTokenExpired(String token) {
    try {
      final parts = token.split('.');
      if (parts.length != 3) return true;
      final payload =
          jsonDecode(
                utf8.decode(base64Url.decode(base64Url.normalize(parts[1]))),
              )
              as Map<String, dynamic>;
      final exp = payload['exp'] as int?;
      if (exp == null) return true;
      return !_now().isBefore(DateTime.fromMillisecondsSinceEpoch(exp * 1000));
    } catch (_) {
      return true;
    }
  }

  /// Coalesce concurrent reads: refresh tokens may rotate after one use.
  Future<void> refresh() {
    if (_refreshing case final pending?) return pending;
    late final Future<void> operation;
    operation = _refresh().whenComplete(() {
      if (identical(_refreshing, operation)) _refreshing = null;
    });
    return _refreshing = operation;
  }

  Future<void> _refresh() async {
    final generation = _sessionGeneration;
    final prefs = await SharedPreferences.getInstance();
    _load(prefs);
    final expectedAccount = _accountId;
    final token = _refreshToken;
    if (expectedAccount == null || token == null || _isTokenExpired(token)) {
      throw const AuthSessionException('auth.restore_required');
    }
    final installation = await _installation();
    final binding = _tokenInstallation(token) == null;
    final res = await _client
        .post(
          Uri.parse(
            '$_baseUrl/api/auth/${binding ? "installation" : "refresh"}',
          ),
          headers: {'Content-Type': 'application/json'},
          body: jsonEncode({
            'refresh_token': token,
            if (binding) 'device_hash': installation,
          }),
        )
        .timeout(const Duration(seconds: 15));
    if (res.statusCode != 200) {
      throw const AuthSessionException('auth.restore_required');
    }
    await _persist(
      jsonDecode(res.body) as Map<String, dynamic>,
      expectedAccount: expectedAccount,
      expectedGeneration: generation,
    );
    if (binding && _isTokenExpired(_accessToken!)) await _refresh();
  }

  /// A server-rejected access token may be refreshed for the same account.
  /// Keep the account and refresh token intact if restoration is unavailable.
  Future<void> invalidateSession() => refresh();

  void _load(SharedPreferences prefs) {
    _accessToken = _nonempty(prefs.getString(_accessKey));
    _refreshToken = _nonempty(prefs.getString(_refreshKey));
    _accountId = _nonempty(prefs.getString(_accountKey));
  }

  String? _nonempty(Object? value) =>
      value is String && value.isNotEmpty ? value : null;

  Future<void> _store(Future<void> Function(SharedPreferences) write) {
    final operation = _storageTail.then((_) async {
      await write(await SharedPreferences.getInstance());
    });
    _storageTail = operation.then<void>(
      (_) {},
      onError: (Object _, StackTrace __) {},
    );
    return operation;
  }

  Future<void> _persist(
    Map<String, dynamic> data, {
    String? expectedAccount,
    int? expectedGeneration,
    bool Function(SharedPreferences)? authorized,
    bool advanceGeneration = false,
  }) async {
    final access = _nonempty(data['access_token']);
    final refresh = _nonempty(data['refresh_token']);
    final account = _nonempty(data['account_id']);
    if (access == null ||
        refresh == null ||
        account == null ||
        (expectedAccount != null && account != expectedAccount)) {
      throw const AuthSessionException('auth.invalid_response');
    }
    await _store((prefs) async {
      if ((authorized != null && !authorized(prefs)) ||
          (expectedGeneration != null &&
              expectedGeneration != _sessionGeneration)) {
        throw const AuthSessionException('oauth.cancelled');
      }
      if (expectedAccount != null &&
          prefs.getString(_accountKey) != expectedAccount) {
        throw const AuthSessionException('auth.invalid_response');
      }
      // The account marker survives interrupted storage. Account changes are
      // serialized with refresh writes, so an old response cannot undo a switch.
      await prefs.setString(_accountKey, account);
      await prefs.setString(_refreshKey, refresh);
      await prefs.setString(_accessKey, access);
      _accessToken = access;
      _refreshToken = refresh;
      _accountId = account;
      if (advanceGeneration) _sessionGeneration++;
    });
  }

  static const _oauthKey = 'knowoff_oauth_pending';
  OAuthAttempt? _oauth;
  Map<String, dynamic>? _oauthCandidate;
  int _oauthGeneration = 0;
  bool _startingOAuth = false;
  Future<OAuthPollState>? _pollingOAuth;

  OAuthAttempt? get pendingOAuth => _oauth;

  /// A local page uses this fence to avoid cancelling a newer page's flow.
  int get oauthGeneration => _oauthGeneration;

  /// The browser gets only the authorization URL. The completion secret remains
  /// local, with the same short expiry as the durable server flow.
  Future<OAuthAttempt> startOAuth(String provider, OAuthIntent intent) async {
    if (_startingOAuth) throw const AuthSessionException('oauth.busy');
    if (provider != 'google' && provider != 'facebook') {
      throw const AuthSessionException('oauth.invalid');
    }
    _startingOAuth = true;
    try {
      if (intent == OAuthIntent.restore) {
        // Restoration cannot wait for a stalled device/refresh request. Fence its
        // eventual storage write and let subsequent reads use the restored session.
        _sessionGeneration++;
        _ensuring = null;
        _refreshing = null;
      }
      await cancelOAuth();
      final generation = _oauthGeneration;
      if (intent == OAuthIntent.link) await ensureSession();
      final prefs = await SharedPreferences.getInstance();
      if (generation != _oauthGeneration) {
        throw const AuthSessionException('oauth.cancelled');
      }
      _load(prefs);
      final originalAccount = _accountId;
      final response = await _oauthPost('/start', {
        'provider': provider,
        'intent': intent.name,
        'device_hash': await _installation(),
      }, access: intent == OAuthIntent.link ? _accessToken : null);
      if (generation != _oauthGeneration) {
        throw const AuthSessionException('oauth.cancelled');
      }
      if (response.statusCode != 200) throw _oauthError(response);
      final attempt = OAuthAttempt._read(
        _oauthMap(response.body),
        provider,
        intent,
        originalAccount,
        _baseUrl,
        _now(),
      );
      await _store((prefs) async {
        if (generation != _oauthGeneration) {
          throw const AuthSessionException('oauth.cancelled');
        }
        await prefs.setString(_oauthKey, jsonEncode(attempt._storage()));
        _oauth = attempt;
      });
      return attempt;
    } finally {
      _startingOAuth = false;
    }
  }

  Future<OAuthAttempt?> resumeOAuth() async {
    final generation = _oauthGeneration;
    if (_oauth != null) {
      if (!_oauth!.expiresAt.isAfter(_now())) {
        await cancelOAuth();
        throw const AuthSessionException('oauth.expired');
      }
      return _oauth;
    }
    final prefs = await SharedPreferences.getInstance();
    if (generation != _oauthGeneration) {
      throw const AuthSessionException('oauth.cancelled');
    }
    _load(prefs);
    final saved = prefs.getString(_oauthKey);
    if (saved == null) return null;
    try {
      final data = _oauthMap(saved);
      if (data['server'] != _baseUrl ||
          data['original_account'] != _accountId) {
        throw const AuthSessionException('oauth.cancelled');
      }
      final provider = data['provider'];
      final intent = data['intent'];
      if (provider is! String || (intent != 'link' && intent != 'restore')) {
        throw const AuthSessionException('oauth.invalid_response');
      }
      final attempt = OAuthAttempt._read(
        data,
        provider,
        intent == 'link' ? OAuthIntent.link : OAuthIntent.restore,
        _accountId,
        _baseUrl,
        _now(),
      );
      _oauth = attempt;
      return attempt;
    } on AuthSessionException {
      await cancelOAuth();
      rethrow;
    }
  }

  Future<void> cancelOAuth({int? generation}) async {
    if (generation != null && generation != _oauthGeneration) return;
    final current = ++_oauthGeneration;
    _oauth = null;
    _oauthCandidate = null;
    _pollingOAuth = null;
    await _store((prefs) async {
      if (current == _oauthGeneration) await prefs.remove(_oauthKey);
    });
  }

  Future<OAuthPollState> pollOAuth({int? generation}) {
    if (generation != null && generation != _oauthGeneration) {
      return Future.error(const AuthSessionException('oauth.cancelled'));
    }
    final current = _oauthGeneration;
    return _pollingOAuth ??= _pollOAuth(current).whenComplete(() {
      if (current == _oauthGeneration) _pollingOAuth = null;
    });
  }

  Future<OAuthPollState> _pollOAuth(int generation) async {
    final attempt = _oauth;
    if (attempt == null || generation != _oauthGeneration) {
      throw const AuthSessionException('oauth.cancelled');
    }
    if (!attempt.expiresAt.isAfter(_now())) {
      await cancelOAuth();
      throw const AuthSessionException('oauth.expired');
    }
    if (_oauthCandidate == null) {
      final response = await _oauthPost('/result', {
        'flow_id': attempt._flowId,
        'completion_secret': attempt._secret,
      });
      if (generation != _oauthGeneration) {
        throw const AuthSessionException('oauth.cancelled');
      }
      if (response.statusCode == 202 &&
          _oauthMap(response.body)['code'] == 'oauth.pending') {
        return OAuthPollState.pending;
      }
      if (response.statusCode != 200) throw _oauthError(response);
      final data = _oauthMap(response.body);
      final access = _nonempty(data['access_token']);
      final refresh = _nonempty(data['refresh_token']);
      final account = _nonempty(data['account_id']);
      if (access == null ||
          refresh == null ||
          account == null ||
          access.length > 16384 ||
          refresh.length > 16384 ||
          account.length > 128 ||
          _isTokenExpired(access) ||
          _isTokenExpired(refresh)) {
        throw const AuthSessionException('oauth.invalid_response');
      }
      _oauthCandidate = data;
    }
    _load(await SharedPreferences.getInstance());
    if (generation != _oauthGeneration ||
        _accountId != attempt._originalAccount) {
      throw const AuthSessionException('oauth.cancelled');
    }
    final different =
        _accountId != null && _oauthCandidate!['account_id'] != _accountId;
    if (different && attempt.intent == OAuthIntent.link) {
      throw const AuthSessionException('oauth.account_mismatch');
    }
    if (different) return OAuthPollState.confirmSwitch;
    await _applyOAuth(generation, attempt);
    return OAuthPollState.completed;
  }

  /// Called only by the explicit account-switch confirmation control.
  Future<void> confirmOAuthSwitch({int? generation}) async {
    if (generation != null && generation != _oauthGeneration) {
      throw const AuthSessionException('oauth.cancelled');
    }
    final attempt = _oauth;
    if (attempt == null ||
        attempt.intent != OAuthIntent.restore ||
        _oauthCandidate == null) {
      throw const AuthSessionException('oauth.invalid');
    }
    await _applyOAuth(_oauthGeneration, attempt);
  }

  Future<void> _applyOAuth(int generation, OAuthAttempt attempt) async {
    final sessionGeneration = _sessionGeneration;
    var candidate = _oauthCandidate;
    if (candidate == null ||
        !attempt.expiresAt.isAfter(_now()) ||
        _isTokenExpired(candidate['access_token'] as String) ||
        _isTokenExpired(candidate['refresh_token'] as String)) {
      throw const AuthSessionException('oauth.expired');
    }
    _load(await SharedPreferences.getInstance());
    if (generation != _oauthGeneration ||
        _accountId != attempt._originalAccount) {
      throw const AuthSessionException('oauth.cancelled');
    }
    if (attempt.intent == OAuthIntent.link &&
        candidate['account_id'] != _accountId) {
      throw const AuthSessionException('oauth.account_mismatch');
    }
    if (_tokenInstallation(candidate['refresh_token'] as String) == null) {
      final installation = await _installation();
      final response = await _client
          .post(
            Uri.parse('$_baseUrl/api/auth/installation'),
            headers: {'Content-Type': 'application/json'},
            body: jsonEncode({
              'refresh_token': candidate['refresh_token'],
              'device_hash': installation,
            }),
          )
          .timeout(const Duration(seconds: 15));
      if (response.statusCode != 200) {
        throw const AuthSessionException('auth.restore_required');
      }
      final bound = _oauthMap(response.body);
      if (bound['account_id'] != candidate['account_id'] ||
          _tokenInstallation(bound['refresh_token'] as String?) !=
              installation) {
        throw const AuthSessionException('oauth.invalid_response');
      }
      candidate = bound;
    }
    await _persist(
      candidate,
      expectedGeneration: sessionGeneration,
      authorized: (prefs) {
        if (!attempt.expiresAt.isAfter(_now()) ||
            _isTokenExpired(candidate!['access_token'] as String) ||
            _isTokenExpired(candidate['refresh_token'] as String)) {
          throw const AuthSessionException('oauth.expired');
        }
        return generation == _oauthGeneration &&
            prefs.getString(_accountKey) == attempt._originalAccount;
      },
      advanceGeneration: true,
    );
    await cancelOAuth(generation: generation);
  }

  Future<http.Response> _oauthPost(
    String suffix,
    Map<String, dynamic> body, {
    String? access,
  }) async {
    try {
      return await _client
          .post(
            Uri.parse('$_baseUrl/api/auth/oauth$suffix'),
            headers: {
              'Content-Type': 'application/json',
              if (access != null) 'Authorization': 'Bearer $access',
            },
            body: jsonEncode(body),
          )
          .timeout(const Duration(seconds: 15));
    } on TimeoutException {
      throw const AuthSessionException('oauth.provider_unavailable');
    } on http.ClientException {
      throw const AuthSessionException('oauth.provider_unavailable');
    }
  }

  Map<String, dynamic> _oauthMap(String body) {
    if (body.length > 65536) {
      throw const AuthSessionException('oauth.invalid_response');
    }
    try {
      final data = jsonDecode(body);
      if (data is Map<String, dynamic>) return data;
    } on FormatException {
      // Never include upstream response text or credentials in exceptions.
    }
    throw const AuthSessionException('oauth.invalid_response');
  }

  AuthSessionException _oauthError(http.Response response) {
    try {
      final code = _oauthMap(response.body)['code'];
      if (const {
        'oauth.invalid',
        'oauth.conflict',
        'oauth.restore_unlinked',
        'oauth.provider_unavailable',
        'oauth.rate_limited',
      }.contains(code)) {
        return AuthSessionException(code as String);
      }
    } on AuthSessionException {
      // A gateway response has no public provider error contract.
    }
    return const AuthSessionException('oauth.provider_unavailable');
  }

  String? _tokenInstallation(String? token) {
    if (token == null) return null;
    try {
      final parts = token.split('.');
      if (parts.length != 3) return null;
      final claims =
          jsonDecode(
                utf8.decode(base64Url.decode(base64Url.normalize(parts[1]))),
              )
              as Map<String, dynamic>;
      return _nonempty(claims['dvh']);
    } catch (_) {
      return null;
    }
  }

  Future<String> _installation() {
    final result = Completer<String>();
    final operation = (_installationTail ?? Future<void>.value()).then((
      _,
    ) async {
      final prefs = await SharedPreferences.getInstance();
      // Failed preference writes still change the plugin's in-memory cache.
      // Only persisted identity may authorize a subsequent network request.
      await prefs.reload();
      final existing = _nonempty(prefs.getString(_installationKey));
      final signed =
          _tokenInstallation(prefs.getString(_refreshKey)) ??
          _tokenInstallation(prefs.getString(_accessKey));
      if (existing != null) {
        if (signed != null && signed != existing) {
          throw const AuthSessionException('auth.installation_mismatch');
        }
        return existing;
      }
      final random = Random.secure();
      final value =
          signed ??
          base64Url
              .encode(List<int>.generate(32, (_) => random.nextInt(256)))
              .replaceAll('=', '');
      if (!await prefs.setString(_installationKey, value)) {
        throw const AuthSessionException('auth.storage_unavailable');
      }
      return value;
    });
    // Keep only pending work. Retaining a completed future also retains its
    // scheduling zone, which may have ended before the next auth lifecycle.
    late final Future<void> completion;
    completion = operation.then<void>(
      (value) {
        if (identical(_installationTail, completion)) _installationTail = null;
        result.complete(value);
      },
      onError: (Object error, StackTrace stack) {
        if (identical(_installationTail, completion)) _installationTail = null;
        result.completeError(error, stack);
      },
    );
    _installationTail = completion;
    return result.future;
  }
}

enum OAuthIntent { link, restore }

enum OAuthPollState { pending, confirmSwitch, completed }

/// Browser metadata is public; completion credentials have no public accessor
/// or diagnostic representation and are scoped to the configured server.
class OAuthAttempt {
  OAuthAttempt._(
    this.provider,
    this.intent,
    this.authorizationUri,
    this.expiresAt,
    this._flowId,
    this._secret,
    this._originalAccount,
    this._server,
  );
  final String provider;
  final OAuthIntent intent;
  final Uri authorizationUri;
  final DateTime expiresAt;
  final String _flowId, _secret, _server;
  final String? _originalAccount;

  static OAuthAttempt _read(
    Map<String, dynamic> data,
    String provider,
    OAuthIntent intent,
    String? originalAccount,
    String server,
    DateTime now,
  ) {
    final url = data['url'],
        flow = data['flow_id'],
        secret = data['completion_secret'];
    final expires = data['expires_at'];
    if (url is! String ||
        url.length > 8192 ||
        flow is! String ||
        !RegExp(
          r'^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$',
        ).hasMatch(flow) ||
        secret is! String ||
        !RegExp(r'^[A-Za-z0-9_-]{43}$').hasMatch(secret) ||
        expires is! String) {
      throw const AuthSessionException('oauth.invalid_response');
    }
    final uri = Uri.tryParse(url), at = DateTime.tryParse(expires);
    if (at != null && !at.isAfter(now)) {
      throw const AuthSessionException('oauth.expired');
    }
    if (uri == null ||
        at == null ||
        at.isAfter(now.add(const Duration(minutes: 11))) ||
        uri.scheme != 'https' ||
        uri.userInfo.isNotEmpty ||
        uri.fragment.isNotEmpty ||
        uri.port != 443 ||
        url.contains(secret) ||
        uri.queryParametersAll.values
            .expand((values) => values)
            .any((value) => value.contains(secret)) ||
        !((provider == 'google' &&
                uri.host == 'accounts.google.com' &&
                uri.path == '/o/oauth2/v2/auth') ||
            (provider == 'facebook' &&
                uri.host == 'www.facebook.com' &&
                RegExp(
                  r'^/v[1-9][0-9]?\.[0-9]{1,2}/dialog/oauth$',
                ).hasMatch(uri.path)))) {
      throw const AuthSessionException('oauth.invalid_response');
    }
    return OAuthAttempt._(
      provider,
      intent,
      uri,
      at,
      flow,
      secret,
      originalAccount,
      server,
    );
  }

  Map<String, dynamic> _storage() => {
    'provider': provider,
    'intent': intent.name,
    'url': authorizationUri.toString(),
    'expires_at': expiresAt.toIso8601String(),
    'flow_id': _flowId,
    'completion_secret': _secret,
    'original_account': _originalAccount,
    'server': _server,
  };
}
