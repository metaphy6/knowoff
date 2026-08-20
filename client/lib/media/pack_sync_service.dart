import 'dart:convert';

import 'package:http/http.dart' as http;
import 'package:shared_preferences/shared_preferences.dart';

import 'media_models.dart';

/// Syncs pack metadata over-the-air. The active tag is persisted so an
/// unrecognized tag triggers a full sync, while a recognized tag is a cheap
/// no-op.
class PackSyncService {
  PackSyncService({required http.Client client, required String baseUrl})
      : _client = client,
        _baseUrl = baseUrl.replaceAll(RegExp(r'/+$'), '');

  final http.Client _client;
  final String _baseUrl;

  PackManifest? _manifest;
  Map<String, MediaItem>? _mediaById;

  PackManifest? get manifest => _manifest;
  Map<String, MediaItem>? get mediaById => _mediaById;

  /// Syncs the pack with [tag]. Returns true if the active pack is now [tag]
  /// and metadata is loaded.
  Future<bool> sync(String tag) async {
    final prefs = await SharedPreferences.getInstance();
    final storedTag = prefs.getString(_tagKey);
    final storedManifest = prefs.getString(_manifestKey);
    final storedMedia = prefs.getString(_mediaKey);

    if (storedTag == tag && storedManifest != null && storedMedia != null) {
      _loadFromLocal(storedManifest, storedMedia);
      return true;
    }

    final manifest = await _fetchManifest(tag);
    if (manifest == null) return false;
    if (manifest.formatVersion != 1) return false;

    final mediaJsonl = await _fetchJsonl(tag, 'media.jsonl');
    if (mediaJsonl == null) return false;

    final mediaItems = _parseMediaJsonl(mediaJsonl);
    final manifestRaw = jsonEncode({
      'pack_tag': manifest.packTag,
      'format_version': manifest.formatVersion,
      'language': manifest.language,
      'embedding_model': manifest.embeddingModel,
      'embedding_version': manifest.embeddingVersion,
      'age_rating': manifest.ageRating,
      'checksums': manifest.checksums,
    });

    await prefs.setString(_tagKey, tag);
    await prefs.setString(_manifestKey, manifestRaw);
    await prefs.setString(_mediaKey, mediaJsonl);

    _manifest = manifest;
    _mediaById = {for (final m in mediaItems) m.id: m};
    return true;
  }

  void _loadFromLocal(String manifestRaw, String mediaJsonl) {
    _manifest = PackManifest.fromJson(
      jsonDecode(manifestRaw) as Map<String, dynamic>,
    );
    _mediaById = {for (final m in _parseMediaJsonl(mediaJsonl)) m.id: m};
  }

  Future<PackManifest?> _fetchManifest(String tag) async {
    try {
      final uri = Uri.parse('$_baseUrl/media/pack/$tag/manifest.json');
      final response = await _client.get(uri);
      if (response.statusCode != 200) return null;
      return PackManifest.fromJson(
        jsonDecode(response.body) as Map<String, dynamic>,
      );
    } on Exception {
      return null;
    }
  }

  Future<String?> _fetchJsonl(String tag, String name) async {
    try {
      final uri = Uri.parse('$_baseUrl/media/pack/$tag/$name');
      final response = await _client.get(uri);
      if (response.statusCode != 200) return null;
      return response.body;
    } on Exception {
      return null;
    }
  }

  List<MediaItem> _parseMediaJsonl(String body) {
    final items = <MediaItem>[];
    for (final line in LineSplitter.split(body)) {
      final trimmed = line.trim();
      if (trimmed.isEmpty) continue;
      items.add(
        MediaItem.fromJson(jsonDecode(trimmed) as Map<String, dynamic>),
      );
    }
    return items;
  }

  static const _tagKey = 'media.active_tag';
  static const _manifestKey = 'media.manifest';
  static const _mediaKey = 'media.media_jsonl';
}
