import 'dart:convert';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:knowoff_client/data/api_client.dart';
import 'package:knowoff_client/data/auth_service.dart';
import 'package:knowoff_client/data/rewarded_ads.dart';

class Auth extends AuthService {
  Auth() : super(baseUrl: 'https://game.example');
  String? owner = 'owner';
  int generation = 1;
  @override
  String? get accountId => owner;
  @override
  int get sessionGeneration => generation;
  @override
  String? get accessToken => 'fixture';
  @override
  Future<bool> restoreExistingSession() async => owner != null;
  @override
  Future<void> ensureSession() async => throw StateError('anonymous forbidden');
}

void main() {
  test(
    'claim and check use exact retained opaque binding and current session',
    () async {
      final auth = Auth();
      var calls = 0, reads = 0;
      final raw = base64Url.encode(List.filled(32, 7)).replaceAll('=', '');
      final at = DateTime.utc(2026, 9, 19);
      final data = {
        'claim': raw,
        'ad_unit': '5224354917',
        'expires_at': at.add(const Duration(minutes: 5)).toIso8601String(),
      };
      final api = ApiClient(
        baseUrl: 'https://game.example',
        auth: auth,
        client: MockClient((r) async {
          calls++;
          expect(r.headers['Authorization'], 'Bearer fixture');
          final body = jsonDecode(r.body);
          if (r.url.path.endsWith('/check')) {
            expect(body, {
              'match_id': 'match',
              'ad_unit': '5224354917',
              'claim': raw,
            });
            return http.Response('{"version":1,"eligible":true}', 200);
          }
          expect(body, {'match_id': 'match', 'ad_unit': '5224354917'});
          return http.Response(jsonEncode(data), 200);
        }),
      );
      final t = ApiRewardClaimTransport(
        api,
        refreshBonus: (account, generation) async {
          expect(account, 'owner');
          expect(generation, 1);
          reads++;
        },
      );
      final response = await t.issue('match', '5224354917');
      await t.check(
        'match',
        RewardClaim.parse(response, expectedUnit: '5224354917', now: at),
      );
      await t.refreshBonus();
      expect(calls, 2);
      expect(reads, 1);
    },
  );
  test(
    'missing session and changed generation never accept ad authority',
    () async {
      final auth = Auth()..owner = null;
      var calls = 0;
      final api = ApiClient(
        baseUrl: 'https://game.example',
        auth: auth,
        client: MockClient((r) async {
          calls++;
          auth.generation++;
          return http.Response('{}', 200);
        }),
      );
      final t = ApiRewardClaimTransport(api, refreshBonus: (_, __) async {});
      await expectLater(t.issue('match', 'unit'), throwsA(isA<ApiException>()));
      expect(calls, 0);
      auth.owner = 'owner';
      await expectLater(t.issue('match', 'unit'), throwsA(isA<ApiException>()));
      expect(calls, 1);
    },
  );
}
