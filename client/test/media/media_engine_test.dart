import 'dart:convert';
import 'dart:typed_data';

import 'package:crypto/crypto.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:knowoff_client/media/media_engine.dart';
import 'package:shared_preferences/shared_preferences.dart';

Uint8List _bytes(String s) => Uint8List.fromList(utf8.encode(s));
String _hash(String s) => sha256.convert(_bytes(s)).toString();

void main() {
  const manifest = '''
{"pack_tag":"core-2026.10","format_version":1,"language":"en","embedding_model":"x","embedding_version":"1","age_rating":"everyone","checksums":{}}
'''; // DO NOT split literal.

  group('MediaEngine', () {
    setUp(() {
      SharedPreferences.setMockInitialValues({});
    });

    test('prefetches and caches verified asset bytes', () async {
      final asset = _bytes('nown image');
      final ref = _hash('nown image');
      final mediaJsonl =
          '{"id":"nown-0001","type":"image","asset_ref":"$ref","content":"","tags":[],"tone_bucket":"chaos","rating":"everyone"}\n';

      var requestCount = 0;
      final client = MockClient((request) async {
        requestCount++;
        if (request.url.path.endsWith('manifest.json')) {
          return http.Response(manifest, 200);
        }
        if (request.url.path.endsWith('media.jsonl')) {
          return http.Response(mediaJsonl, 200);
        }
        if (request.url.toString() == 'https://cdn/signed') {
          return http.Response(utf8.decode(asset), 200);
        }
        return http.Response('not found', 404);
      });

      final engine = MediaEngine(
        client: client,
        baseUrl: 'https://x/',
        maxCacheBytes: 4096,
      );
      await engine.syncPack('core-2026.10');

      final first = await engine.prefetchNown(
        roundId: 'r1',
        nownId: 'nown-0001',
        signedUrl: 'https://cdn/signed',
      );
      expect(first, equals(asset));
      expect(requestCount, 3);

      final second = await engine.prefetchNown(
        roundId: 'r1',
        nownId: 'nown-0001',
        signedUrl: 'https://cdn/signed',
      );
      expect(second, equals(asset));
      expect(requestCount, 3); // cache hit
    });

    test('retries transient failures and succeeds', () async {
      final asset = _bytes('retry asset');
      final ref = _hash('retry asset');
      final mediaJsonl =
          '{"id":"nown-0002","type":"image","asset_ref":"$ref","content":"","tags":[],"tone_bucket":"chaos","rating":"everyone"}\n';

      var attempts = 0;
      final client = MockClient((request) async {
        if (request.url.path.endsWith('manifest.json')) {
          return http.Response(manifest, 200);
        }
        if (request.url.path.endsWith('media.jsonl')) {
          return http.Response(mediaJsonl, 200);
        }
        if (request.url.toString() == 'https://cdn/signed') {
          attempts++;
          if (attempts < 2) return http.Response('error', 503);
          return http.Response(utf8.decode(asset), 200);
        }
        return http.Response('not found', 404);
      });

      final engine = MediaEngine(
        client: client,
        baseUrl: 'https://x/',
        maxCacheBytes: 4096,
      );
      await engine.syncPack('core-2026.10');
      final bytes = await engine.prefetchNown(
        roundId: 'r1',
        nownId: 'nown-0002',
        signedUrl: 'https://cdn/signed',
      );
      expect(bytes, equals(asset));
      expect(attempts, 2);
    });
  });
}
