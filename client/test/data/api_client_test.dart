import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:knowoff_client/data/api_client.dart';
import 'package:knowoff_client/data/auth_service.dart';

class _Auth extends AuthService {
  _Auth() : super(baseUrl: 'http://test');
  @override
  Future<void> ensureSession() async {}
  @override
  String? get accessToken => 'test-token';
}

void main() {
  test('economy requests match server routes and accept empty 204 unlock',
      () async {
    final requests = <http.Request>[];
    final client = MockClient((request) async {
      requests.add(request);
      switch (request.url.path) {
        case '/api/economy/store':
          return http.Response('{"points_to_noin":100}', 200);
        case '/api/economy/purchase/playpass':
          return http.Response('{"active_until":"2030-01-01T00:00:00Z"}', 200);
        case '/api/economy/purchase/unlock':
          return http.Response('', 204);
        default:
          return http.Response('not found', 404);
      }
    });
    final api =
        ApiClient(baseUrl: 'http://test', auth: _Auth(), client: client);
    expect(await api.getStoreCatalog(), {'points_to_noin': 100});
    await api.purchasePlayPass('day_3');
    await api.purchaseUnlock('custom_avatar');
    expect(requests.map((request) => request.method), ['GET', 'POST', 'POST']);
    expect(jsonDecode(requests[1].body), {'type': 'day_3'});
    expect(
        jsonDecode(requests[2].body), {'type': 'custom_avatar', 'value': ''});
  });
}
