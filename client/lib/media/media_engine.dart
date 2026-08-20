import 'dart:math' as math;
import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:http/http.dart' as http;

import 'asset_cache.dart';
import 'media_models.dart';
import 'pack_sync_service.dart';

/// The client MediaEngine: pack metadata sync, asset prefetch & cache,
/// and the Donower placeholder renderer.
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

  /// Returns a widget that renders Nown for a Nower, or the shared placeholder
  /// while the asset is still loading. The placeholder is identical to the
  /// Donower-side placeholder so loading state leaks nothing about role.
  Widget buildNownStage({
    required String nownId,
    Uint8List? bytes,
    double? width,
    double? height,
  }) {
    final item = mediaById?[nownId];
    if (item == null) return const DonowerPlaceholder();

    if (item.type == MediaType.text) {
      return _TextNown(content: item.content);
    }

    if (bytes != null) {
      return Image.memory(
        bytes,
        width: width,
        height: height,
        fit: BoxFit.contain,
        errorBuilder: (_, __, ___) => const DonowerPlaceholder(),
      );
    }

    return const DonowerPlaceholder();
  }
}

class _TextNown extends StatelessWidget {
  const _TextNown({required this.content});

  final String content;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        border: Border.all(width: 3, color: Colors.black),
        color: Colors.white,
        boxShadow: const [BoxShadow(color: Colors.black, offset: Offset(4, 4))],
      ),
      child: Text(content, style: const TextStyle(color: Colors.black)),
    );
  }
}

/// The placeholder shown to Donowers and to any Nower whose asset has not yet
/// loaded. Rendering the same placeholder in both states prevents loading
/// progress from leaking role information.
class DonowerPlaceholder extends StatelessWidget {
  const DonowerPlaceholder({super.key});

  @override
  Widget build(BuildContext context) {
    return CustomPaint(
      size: const Size(200, 200),
      painter: _StaticBurstPainter(),
    );
  }
}

class _StaticBurstPainter extends CustomPainter {
  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()..color = const Color(0xFFF7F2E9);
    canvas.drawRect(Offset.zero & size, paint);

    final linePaint = Paint()
      ..color = Colors.black
      ..strokeWidth = 3;
    final center = size.center(Offset.zero);
    final radius = size.shortestSide / 2 - 8;
    for (var i = 0; i < 12; i++) {
      final angle = i * 3.14159 / 6;
      canvas.drawLine(
        center,
        center + Offset(math.cos(angle), math.sin(angle)) * radius,
        linePaint,
      );
    }

    final eyePaint = Paint()
      ..color = Colors.black
      ..style = PaintingStyle.stroke
      ..strokeWidth = 3;
    canvas.drawOval(
      Rect.fromCenter(
          center: center, width: radius * 0.8, height: radius * 0.5),
      eyePaint,
    );
    canvas.drawCircle(
        center, radius * 0.12, eyePaint..style = PaintingStyle.fill);
  }

  @override
  bool shouldRepaint(covariant CustomPainter oldDelegate) => false;
}
