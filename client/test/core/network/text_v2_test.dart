import 'dart:convert';
import 'dart:io';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/core/text/v2_contract.dart';

void main() {
  test(
    'interrupted private receipt has zero terminal effects and only committed vote awards',
    () {
      final receipt = <String, dynamic>{
        'id': 1,
        'match_id': 'm',
        'settlement': {
          'match_id': 'm',
          'points': 0,
          'xp': 0,
          'leaderboard_counted': false,
          'interrupted': true,
          'awards': [
            {
              'kind': 'correct_vote',
              'ordinal': 1,
              'requested': 5,
              'credited': 5,
            },
          ],
        },
      };
      expect(() => V2Codec.control('delivery', receipt), returnsNormally);
      for (final key in [
        'points',
        'xp',
        'leaderboard_counted',
        'interrupted',
      ]) {
        final changed = jsonDecode(jsonEncode(receipt)) as Map<String, dynamic>;
        changed['settlement'][key] = key == 'leaderboard_counted'
            ? true
            : key == 'interrupted'
            ? false
            : 1;
        expect(
          () => V2Codec.control('delivery', changed),
          throwsA(isA<V2Failure>()),
        );
      }
    },
  );

  test(
    'private receipts reject extra identities, duplicate awards and invalid credit',
    () {
      Map<String, dynamic> receipt() => {
        'id': 1,
        'match_id': 'm',
        'settlement': {
          'match_id': 'm',
          'points': 1,
          'xp': 1,
          'leaderboard_counted': false,
          'awards': [
            {
              'kind': 'correct_vote',
              'ordinal': 1,
              'requested': 5,
              'credited': 3,
            },
          ],
        },
      };
      for (final mutation in ['identity', 'mismatch', 'credit', 'duplicate']) {
        final r = receipt();
        switch (mutation) {
          case 'identity':
            r['account_id'] = 'someone';
            break;
          case 'mismatch':
            r['settlement']['match_id'] = 'other';
            break;
          case 'credit':
            r['settlement']['awards'][0]['credited'] = 6;
            break;
          case 'duplicate':
            r['settlement']['awards'].add(r['settlement']['awards'][0]);
            break;
        }
        expect(() => V2Codec.control('delivery', r), throwsA(isA<V2Failure>()));
      }
    },
  );

  test('eliminated snapshots cannot retain another private hand', () {
    final fixtures =
        (jsonDecode(
              File(
                '../server/internal/transport/v2/testdata/contracts.json',
              ).readAsStringSync(),
            )
            as List);
    final hidden = fixtures.singleWhere(
      (f) => f['name'] == 'snapshot-eliminated',
    )['wire'];
    hidden['private']['hand'] = fixtures.singleWhere(
      (f) => f['name'] == 'snapshot-nower',
    )['wire']['private']['hand'];
    const bounds = V2Limits(
      maxFrameBytes: 65536,
      maxHistoryEvents: 8192,
      maxHistoryPageEvents: 8,
      maxTextBytes: 512,
      maxRequestsPerSeat: 512,
    );
    expect(
      () => V2Snapshot.decode(jsonEncode(hidden), bounds),
      throwsA(isA<V2Failure>()),
    );
  });

  const limits = V2Limits(
    maxFrameBytes: 65536,
    maxHistoryEvents: 8192,
    maxHistoryPageEvents: 8,
    maxTextBytes: 512,
    maxRequestsPerSeat: 512,
  );
  final fixtures =
      (jsonDecode(
                File(
                  '../server/internal/transport/v2/testdata/contracts.json',
                ).readAsStringSync(),
              )
              as List)
          .cast<Map<String, dynamic>>();
  test(
    'production v2 decoder consumes all shared positive and negative goldens',
    () {
      for (final f in fixtures) {
        final raw = jsonEncode(f['wire']);
        if (f['code'] == '') {
          expect(
            () => V2Codec.decode(f['kind'] as String, raw, limits),
            returnsNormally,
            reason: f['name'] as String,
          );
        } else {
          expect(
            () => V2Codec.decode(f['kind'] as String, raw, limits),
            throwsA(isA<V2Failure>().having((e) => e.code, 'code', f['code'])),
            reason: f['name'] as String,
          );
        }
      }
    },
  );
  test(
    'strict v2 parser refuses duplicate keys, nulls, nested excess and unsafe values',
    () {
      final valid = jsonEncode(fixtures.first['wire']);
      for (final raw in [
        valid.replaceFirst('"v":2', '"v":2,"v":2'),
        valid.replaceFirst('"v":2', '"v":null'),
        valid.replaceFirst('"v":2', '"v":2.0'),
        valid.replaceFirst('copy-1', r'\ud800'),
      ]) {
        expect(
          () => V2Codec.decode('action', raw, limits),
          throwsA(isA<V2Failure>()),
        );
      }
      expect(
        () => V2Codec.decode('action', ' ' * 65537, limits),
        throwsA(
          isA<V2Failure>().having(
            (e) => e.code,
            'code',
            'protocol.frame_too_large',
          ),
        ),
      );
    },
  );
  test(
    'decoded snapshots detach and freeze all private and public collections',
    () {
      final wire = fixtures.singleWhere(
        (f) => f['name'] == 'snapshot-nower',
      )['wire'];
      final s = V2Snapshot.decode(jsonEncode(wire), limits);
      expect(s.hand.first.copyID, 'copy-1');
      expect(() => s.json['private']['hand'].clear(), throwsUnsupportedError);
      expect(
        () => s.json['contract']['mode_id'] = 'top_that',
        throwsUnsupportedError,
      );
    },
  );
}
