import 'dart:math' as math;
import 'dart:typed_data';

import 'package:http/http.dart' as http;

import 'asset_cache.dart';
import 'media_models.dart';
import 'pack_sync_service.dart';

/// The client MediaEngine: pack metadata sync, asset prefetch and cache.
class MediaEngine {
  MediaEngine({
    required http.Client client,
    required String baseUrl,
    required int maxCacheBytes,
  })  : _client = client,
        _cache = AssetCache(maxBytes: maxCacheBytes),
        _sync = PackSyncService(client: client, baseUrl: baseUrl);

  final http.Client _client;
  final AssetCache _cache;
  final PackSyncService _sync;

  PackManifest? get manifest => _sync.manifest;
  Map<String, MediaItem>? get mediaById => _sync.mediaById;

  /// Syncs pack metadata for [tag]. Returns true on success.
  Future<bool> syncPack(String tag) => _sync.sync(tag);

  /// Prefetches the asset for [nownId] for the given [roundId], using the
  /// server-issued [signedUrl]. Retries with exponential backoff on transient
  /// failures. The asset is verified against its content hash before entering
  /// the cache.
  Future<Uint8List?> prefetchNown({
    required String roundId,
    required String nownId,
    required String signedUrl,
  }) async {
    final item = mediaById?[nownId];
    final ref = item?.assetRef;

    if (ref != null && ref.isNotEmpty) {
      final cached = _cache.get(ref);
      if (cached != null) return cached;
    }

    final bytes = await _fetchWithRetry(signedUrl);
    if (bytes == null) return null;

    if (ref != null && ref.isNotEmpty) {
      _cache.put(ref, bytes);
    }
    return bytes;
  }

  Future<Uint8List?> _fetchWithRetry(String url, {int maxAttempts = 4}) async {
    var delay = const Duration(milliseconds: 250);
    for (var attempt = 0; attempt < maxAttempts; attempt++) {
      try {
        final response = await _client.get(Uri.parse(url));
        if (response.statusCode == 200) {
          return response.bodyBytes;
        }
        if (response.statusCode >= 400 && response.statusCode < 500) {
          return null;
        }
      } on Exception {
        // transient network failure; retry
      }
      if (attempt < maxAttempts - 1) {
        await Future<void>.delayed(delay);
        delay =
            Duration(milliseconds: math.min(delay.inMilliseconds * 2, 4000));
      }
    }
    return null;
  }
}
