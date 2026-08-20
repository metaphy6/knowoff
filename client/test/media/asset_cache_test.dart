import 'dart:convert';
import 'dart:typed_data';

import 'package:crypto/crypto.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/media/asset_cache.dart';

Uint8List _bytes(String s) => Uint8List.fromList(utf8.encode(s));

String _hash(String s) => sha256.convert(_bytes(s)).toString();

void main() {
  group('AssetCache', () {
    test('stores and retrieves verified bytes', () {
      final cache = AssetCache(maxBytes: 1024);
      final data = _bytes('hello asset');
      final ref = _hash('hello asset');
      cache.put(ref, data);
      expect(cache.get(ref), equals(data));
    });

    test('rejects corrupted bytes', () {
      final cache = AssetCache(maxBytes: 1024);
      final data = _bytes('tampered');
      cache.put('wrong-ref', data);
      expect(cache.get('wrong-ref'), isNull);
      expect(cache.size, 0);
    });

    test('evicts least recently used entries when over budget', () {
      final cache = AssetCache(maxBytes: 50);
      final a = _bytes('a'); // 1 byte
      final refA = _hash('a');
      final b = _bytes('bbb'); // 3 bytes
      final refB = _hash('bbb');
      final c = _bytes('cccccccccccccccccccc'); // 20 bytes
      final refC = _hash('cccccccccccccccccccc');

      cache.put(refA, a);
      cache.put(refB, b);
      cache.put(refC, c);

      // A should have been evicted to stay under budget (1+3+20=24 <= 30, actually fits)
      // Use a larger c to force eviction.
      final big = _bytes('c' * 40); // 40 bytes
      final refBig = _hash('c' * 40);
      cache.put(refBig, big);

      expect(cache.get(refA), isNull);
      expect(cache.get(refBig), equals(big));
    });

    test('moves touched entries to most-recently-used position', () {
      final cache = AssetCache(maxBytes: 10);
      final a = _bytes('aaaa');
      final refA = _hash('aaaa');
      final b = _bytes('bbbb');
      final refB = _hash('bbbb');
      cache.put(refA, a);
      cache.put(refB, b);

      // Touch A so it becomes MRU.
      cache.get(refA);

      final c = _bytes('cccc');
      final refC = _hash('cccc');
      cache.put(refC, c);

      // B (LRU) should be evicted, A and C remain.
      expect(cache.get(refA), isNotNull);
      expect(cache.get(refB), isNull);
      expect(cache.get(refC), isNotNull);
    });
  });
}
