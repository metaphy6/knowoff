/// Lightweight client-side mirrors of the server pack format.
class PackManifest {
  PackManifest({
    required this.packTag,
    required this.formatVersion,
    required this.language,
    required this.embeddingModel,
    required this.embeddingVersion,
    required this.ageRating,
    required this.checksums,
  });

  factory PackManifest.fromJson(Map<String, dynamic> json) {
    return PackManifest(
      packTag: json['pack_tag'] as String? ?? '',
      formatVersion: json['format_version'] as int? ?? 0,
      language: json['language'] as String? ?? 'en',
      embeddingModel: json['embedding_model'] as String? ?? '',
      embeddingVersion: json['embedding_version'] as String? ?? '',
      ageRating: json['age_rating'] as String? ?? 'everyone',
      checksums: (json['checksums'] as Map<String, dynamic>? ?? {})
          .cast<String, String>(),
    );
  }

  final String packTag;
  final int formatVersion;
  final String language;
  final String embeddingModel;
  final String embeddingVersion;
  final String ageRating;
  final Map<String, String> checksums;
}

enum MediaType { image, gif, text }

class MediaItem {
  MediaItem({
    required this.id,
    required this.type,
    required this.assetRef,
    required this.content,
    required this.tags,
    required this.toneBucket,
    required this.rating,
  });

  factory MediaItem.fromJson(Map<String, dynamic> json) {
    return MediaItem(
      id: json['id'] as String? ?? '',
      type: _parseType(json['type'] as String? ?? 'text'),
      assetRef: json['asset_ref'] as String? ?? '',
      content: json['content'] as String? ?? '',
      tags: (json['tags'] as List<dynamic>? ?? []).cast<String>(),
      toneBucket: json['tone_bucket'] as String? ?? '',
      rating: json['rating'] as String? ?? 'everyone',
    );
  }

  final String id;
  final MediaType type;
  final String assetRef;
  final String content;
  final List<String> tags;
  final String toneBucket;
  final String rating;

  static MediaType _parseType(String raw) {
    switch (raw) {
      case 'image':
        return MediaType.image;
      case 'gif':
        return MediaType.gif;
      default:
        return MediaType.text;
    }
  }
}
