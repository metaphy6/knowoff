import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/core/text/v2_contract.dart';
import 'package:knowoff_client/core/text/v2_reducer.dart';

import '../core/network/text_reducer_test.dart' show fixture, limits;

// The retired image LRU's verified-byte and eviction cases now protect the
// actual playable cache: immutable, bounded, role-scoped text snapshots.
void main() {
  test('stores only immutable verified text and preserves copy identity', () {
    final wire = fixture('snapshot-nower');
    final r = V2Reducer(limits)..snapshot(wire);
    final hand = r.current!.json['private']['hand'];
    wire['private']['hand'][0]['content']['text'] = 'tampered after decode';
    expect(hand[0]['content']['text'], 'Spare key');
    expect(hand[0]['copy_id'], 'copy-1');
    expect(() => hand.clear(), throwsUnsupportedError);
    expect(
      () => hand[0]['content']['text'] = 'tampered',
      throwsUnsupportedError,
    );
    expect(
      () => r.current!.json['private']['role'] = 'donower',
      throwsUnsupportedError,
    );
  });

  test('rejects corrupted page bytes before installing any private state', () {
    final r = V2Reducer(limits)..snapshot(fixture('snapshot-paged-history'));
    expect(r.current, isNull);
    final page = fixture('public-history-page');
    page['events'][0]['count'] = 2;
    expect(() => r.page(page), throwsA(isA<V2Failure>()));
    expect(r.current, isNull);
    expect(r.pendingPageCount, 0);
    expect(r.needsResync, isTrue);
  });

  test(
    'enforces configured frame and history budgets without partial install',
    () {
      const tiny = V2Limits(
        maxFrameBytes: 128,
        maxHistoryEvents: 8192,
        maxHistoryPageEvents: 8,
        maxTextBytes: 512,
        maxRequestsPerSeat: 512,
      );
      final r = V2Reducer(tiny);
      expect(
        () => r.snapshot(fixture('snapshot-nower')),
        throwsA(isA<V2Failure>()),
      );
      expect(r.current, isNull);
      const one = V2Limits(
        maxFrameBytes: 65536,
        maxHistoryEvents: 1,
        maxHistoryPageEvents: 1,
        maxTextBytes: 512,
        maxRequestsPerSeat: 512,
      );
      final bounded = V2Reducer(one);
      final wire = fixture('snapshot-paged-history');
      wire['history_pages']['total_events'] = 2;
      wire['history_pages']['page_count'] = 2;
      wire['history_pages']['through_evidence_seq'] = 2;
      wire['cursor']['evidence_seq'] = 2;
      expect(() => bounded.snapshot(wire), throwsA(isA<V2Failure>()));
      expect(bounded.current, isNull);
      expect(bounded.pendingPageCount, 0);
    },
  );

  test(
    'verified page replay is stable and disconnect evicts all private data',
    () {
      final r = V2Reducer(limits)..snapshot(fixture('snapshot-paged-history'));
      r.page(fixture('public-history-page'));
      final before = jsonEncode(r.current!.json);
      expect(
        () => r.page(fixture('public-history-page')),
        throwsA(isA<V2Failure>()),
      );
      expect(jsonEncode(r.current!.json), before);
      r.confirm({'kind': 'draw', 'count': 1}, 'draw-request', 0);
      r.disconnect();
      expect(r.current, isNull);
      expect(r.pendingRequest, isNull);
      expect(r.pendingPageCount, 0);
      final fresh = fixture('snapshot-paged-history');
      fresh['cursor']['stream_epoch'] = 'fresh-role-stream';
      r.snapshot(fresh);
      final page = fixture('public-history-page');
      page['stream_epoch'] = 'fresh-role-stream';
      r.page(page);
      expect(r.current!.json['private']['hand'][0]['copy_id'], 'copy-1');
      expect(r.current!.json['history'][0]['count'], 1);
      expect(
        () => r.snapshot(fixture('snapshot-paged-history')),
        throwsA(isA<V2Failure>()),
      );
      expect(r.current!.json['cursor']['stream_epoch'], 'fresh-role-stream');
    },
  );
}
