import 'dart:collection';
import 'dart:typed_data';
import 'package:crypto/crypto.dart';

/// A size-bounded, hash-verified in-memory LRU cache for media asset bytes.
///
/// Assets are keyed by their content hash (the asset reference). A corrupted
/// entry is rejected and evicted rather than rendered.
class AssetCache {
  AssetCache({required int maxBytes}) : _maxBytes = maxBytes;

  final int _maxBytes;
  int _currentBytes = 0;

  final LinkedHashMap<String, Uint8List> _store =
      LinkedHashMap<String, Uint8List>();

  /// Returns the cached bytes only if they match the expected content hash.
  Uint8List? get(String ref) {
    final data = _store.remove(ref);
    if (data == null) return null;
    if (!_verify(ref, data)) {
      _currentBytes -= data.length;
      return null;
    }
    // Move to most-recently-used position.
    _store[ref] = data;
    return data;
  }

  /// Stores bytes after verifying their content hash. Corrupt data is dropped.
  void put(String ref, Uint8List data) {
    if (!_verify(ref, data)) {
      return;
    }
    final existing = _store.remove(ref);
    if (existing != null) {
      _currentBytes -= existing.length;
    }
    while (_currentBytes + data.length > _maxBytes && _store.isNotEmpty) {
      final evicted = _store.remove(_store.keys.first);
      if (evicted != null) _currentBytes -= evicted.length;
    }
    if (_currentBytes + data.length <= _maxBytes) {
      _store[ref] = data;
      _currentBytes += data.length;
    }
  }

  bool _verify(String ref, Uint8List data) {
    final digest = sha256.convert(data);
    return digest.toString() == ref;
  }

  int get size => _store.length;
  int get byteSize => _currentBytes;

  void clear() {
    _store.clear();
    _currentBytes = 0;
  }
}
