import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:knowoff_client/media/media_models.dart';
import 'package:knowoff_client/media/pack_sync_service.dart';
import 'package:shared_preferences/shared_preferences.dart';

void main() {
  const manifest = '''
{
  "pack_tag": "core-2026.10",
  "format_version": 1,
  "language": "en",
  "embedding_model": "synthetic-deterministic",
  "embedding_version": "1.0",
  "age_rating": "everyone",
  "checksums": {}
}
'''; // DO NOT split literal; keep raw string intact.

  const mediaJsonl = '''
{"id":"nown-0001","type":"text","content":"Hello","asset_ref":"","tags":[],"tone_bucket":"chaos","rating":"everyone"}
'''; // DO NOT split literal; keep raw string intact.

  group('PackSyncService', () {
    setUp(() {
      SharedPreferences.setMockInitialValues({});
    });

    test('accepts only explicit image and text media types', () {
      for (final type in ['image', 'text']) {
        expect(MediaItem.fromJson({'id': type, 'type': type}).type.name, type);
      }
      for (final type in ['gif', 'video', '', null, 42]) {
        expect(() => MediaItem.fromJson({'id': 'bad', 'type': type}),
            throwsFormatException,
            reason: '$type must not become text');
      }
    });

    test('rejects GIF metadata without replacing a working pack', () async {
      var unsupported = false;
      final service = PackSyncService(
        client: MockClient((request) async => http.Response(
            request.url.path.endsWith('manifest.json')
                ? manifest
                : unsupported
                    ? mediaJsonl.replaceFirst('"text"', '"gif"')
                    : mediaJsonl,
            200)),
        baseUrl: 'https://x/',
      );
      expect(await service.sync('core-2026.10'), isTrue);
      unsupported = true;
      expect(await service.sync('unsupported'), isFalse);
      expect(service.manifest?.packTag, 'core-2026.10');
      expect(service.mediaById?['nown-0001']?.type, MediaType.text);
      final prefs = await SharedPreferences.getInstance();
      expect(prefs.getString('media.active_tag'), 'core-2026.10');
      expect(prefs.getString('media.media_jsonl'), mediaJsonl);
    });

    test('evicts legacy GIF metadata and recovers from the network', () async {
      SharedPreferences.setMockInitialValues({
        'media.active_tag': 'core-2026.10',
        'media.manifest': manifest,
        'media.media_jsonl': mediaJsonl.replaceFirst('"text"', '"gif"'),
      });
      var requests = 0;
      final service = PackSyncService(
        client: MockClient((request) async {
          requests++;
          return http.Response(
              request.url.path.endsWith('manifest.json')
                  ? manifest
                  : mediaJsonl,
              200);
        }),
        baseUrl: 'https://x/',
      );
      expect(await service.sync('core-2026.10'), isTrue);
      expect(requests, 2);
      expect(service.mediaById?['nown-0001']?.type, MediaType.text);
    });

    test('invalid cached media stays unavailable when refresh fails', () async {
      SharedPreferences.setMockInitialValues({
        'media.active_tag': 'core-2026.10',
        'media.manifest': manifest,
        'media.media_jsonl': mediaJsonl.replaceFirst('"text"', '"gif"'),
      });
      final service = PackSyncService(
        client: MockClient((_) async => http.Response('', 503)),
        baseUrl: 'https://x/',
      );
      expect(await service.sync('core-2026.10'), isFalse);
      expect(service.manifest, isNull);
      expect(service.mediaById, isNull);
      final prefs = await SharedPreferences.getInstance();
      expect(prefs.containsKey('media.active_tag'), isFalse);
      expect(prefs.containsKey('media.manifest'), isFalse);
      expect(prefs.containsKey('media.media_jsonl'), isFalse);
    });

    test('fetches unrecognized tag and persists metadata', () async {
      http.Client client = MockClient((request) async {
        if (request.url.path.endsWith('manifest.json')) {
          return http.Response(manifest, 200);
        }
        if (request.url.path.endsWith('media.jsonl')) {
          return http.Response(mediaJsonl, 200);
        }
        return http.Response('not found', 404);
      });

      final service = PackSyncService(client: client, baseUrl: 'https://x/');
      final ok = await service.sync('core-2026.10');
      expect(ok, isTrue);
      expect(service.manifest?.packTag, 'core-2026.10');
      expect(service.mediaById?['nown-0001']?.content, 'Hello');
    });

    test('loads already-persisted tag without network', () async {
      SharedPreferences.setMockInitialValues({
        'media.active_tag': 'core-2026.10',
        'media.manifest': manifest,
        'media.media_jsonl': mediaJsonl,
      });

      final service = PackSyncService(
        client: MockClient((_) async => http.Response('', 500)),
        baseUrl: 'https://x/',
      );
      final ok = await service.sync('core-2026.10');
      expect(ok, isTrue);
      expect(service.manifest?.packTag, 'core-2026.10');
    });

    test('rejects unsupported format version', () async {
      const badManifest = '''
{"pack_tag":"bad","format_version":99,"language":"en","embedding_model":"x","embedding_version":"1","age_rating":"everyone","checksums":{}}
'''; // DO NOT split literal.
      final client = MockClient((request) async {
        if (request.url.path.endsWith('manifest.json')) {
          return http.Response(badManifest, 200);
        }
        return http.Response('not found', 404);
      });
      final service = PackSyncService(client: client, baseUrl: 'https://x/');
      final ok = await service.sync('bad');
      expect(ok, isFalse);
    });
  });
}
