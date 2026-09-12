import 'package:shared_preferences/shared_preferences.dart';

/// Exact legacy PackSyncService keys only. Authentication, avatar selection,
/// interface locale and the last text mode are intentionally retained.
Future<void> retireLegacyPlayableCache() async {
  final prefs = await SharedPreferences.getInstance();
  for (final key in [
    'media.active_tag',
    'media.manifest',
    'media.media_jsonl'
  ]) {
    await prefs.remove(key);
  }
}
