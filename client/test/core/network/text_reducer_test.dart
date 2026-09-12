import 'dart:convert';
import 'dart:io';
import 'package:flutter_test/flutter_test.dart';
import 'package:crypto/crypto.dart';
import 'package:knowoff_client/core/text/v2_contract.dart';
import 'package:knowoff_client/core/text/v2_reducer.dart';

const limits = V2Limits(
    maxFrameBytes: 65536,
    maxHistoryEvents: 8192,
    maxHistoryPageEvents: 8,
    maxTextBytes: 512,
    maxRequestsPerSeat: 512);
Map<String, dynamic> fixture(String name) =>
    (jsonDecode(File('../server/internal/transport/v2/testdata/contracts.json')
            .readAsStringSync()) as List)
        .singleWhere((f) => f['name'] == name)['wire'] as Map<String, dynamic>;
void main() {
  test(
      'same-round terminal turn zero is accepted and terminal rollback is refused',
      () {
    final r = V2Reducer(limits);
    r.snapshot(fixture('snapshot-nower'));
    final end = fixture('snapshot-verdict-begun-only');
    end['turn'] = 0;
    end['cursor']['recipient_seq'] = 2;
    r.snapshot(end);
    expect(r.current?.phase, 'verdict');
    r.disconnect();
    end['cursor']['stream_epoch'] = 'new-terminal';
    end['cursor']['recipient_seq'] = 1;
    r.snapshot(end);
    final old = fixture('snapshot-nower');
    old['cursor']['stream_epoch'] = 'new-terminal';
    old['cursor']['recipient_seq'] = 2;
    expect(() => r.snapshot(old), throwsA(isA<V2Failure>()));
    expect(r.current, isNull);
  });

  test(
      'chat visibility changes preserve known text and every gameplay fingerprint',
      () {
    for (final initiallyHidden in [false, true]) {
      final r = V2Reducer(limits);
      var seq = 0;
      Map<String, dynamic> state(
          {bool hidden = false, String text = 'original'}) {
        final s = fixture('snapshot-nower');
        final e =
            fixture('public-history-page')['events'][0] as Map<String, dynamic>;
        e.remove('count');
        e['kind'] = 'chat';
        e['ui_locale'] = 'en';
        e['after_revision'] = 0;
        if (hidden) {
          e['phrase_id'] = 'chat.hidden';
        } else {
          e['text'] = text;
        }
        s['history'] = [e];
        s['cursor']['recipient_seq'] = ++seq;
        s['cursor']['evidence_seq'] = 1;
        return s;
      }

      r.snapshot(state(hidden: initiallyHidden));
      r.snapshot(state());
      r.snapshot(state(hidden: true));
      r.snapshot(state());
      r.snapshot(state(hidden: true));
      expect(
          () => r.snapshot(state(text: 'changed')), throwsA(isA<V2Failure>()));
      expect(r.current, isNull);
    }
  });

  test(
      'paged chat projection cannot erase observed text or rewrite actor/gameplay',
      () {
    for (final mutation in ['text', 'actor', 'gameplay', 'revision']) {
      final r = V2Reducer(limits);
      final initial = fixture('snapshot-nower');
      final chat =
          fixture('public-history-page')['events'][0] as Map<String, dynamic>;
      chat.remove('count');
      chat['kind'] = 'chat';
      chat['after_revision'] = 0;
      chat['ui_locale'] = 'en';
      chat['text'] = 'known';
      final draw =
          fixture('public-history-page')['events'][0] as Map<String, dynamic>;
      draw['event_id'] = 'draw-two';
      draw['evidence_seq'] = 2;
      initial['history'] = [chat, draw];
      initial['cursor']['evidence_seq'] = 2;
      initial['board']['revision'] = 2;
      r.snapshot(initial);
      r.disconnect();
      final hidden = jsonDecode(jsonEncode(initial)) as Map<String, dynamic>;
      hidden['history'][0].remove('text');
      hidden['history'][0]['phrase_id'] = 'chat.hidden';
      hidden['cursor']['stream_epoch'] = 'reconnected';
      final events = hidden['history'];
      final page = fixture('public-history-page');
      page['stream_epoch'] = 'reconnected';
      page['events'] = events;
      page['through_evidence_seq'] = 2;
      page['sha256'] = v2Hash(events);
      final digest = page['sha256'] as String;
      hidden['history_pages'] = {
        'page_count': 1,
        'total_events': 2,
        'through_evidence_seq': 2,
        'root_sha256': sha256.convert([
          for (var i = 0; i < digest.length; i += 2)
            int.parse(digest.substring(i, i + 2), radix: 16)
        ]).toString()
      };
      hidden['history'] = [];
      r.snapshot(hidden);
      r.page(page);
      expect(r.current!.json['history'][0]['phrase_id'], 'chat.hidden');
      final tampered = jsonDecode(jsonEncode(initial)) as Map<String, dynamic>;
      tampered['cursor']['stream_epoch'] = 'reconnected';
      tampered['cursor']['recipient_seq'] = 2;
      if (mutation == 'text') tampered['history'][0]['text'] = 'rewritten';
      if (mutation == 'actor') tampered['history'][0]['actor']['seat'] = 1;
      if (mutation == 'gameplay') tampered['history'][1]['count'] = 2;
      if (mutation == 'revision') tampered['history'][1]['after_revision'] = 2;
      expect(
          () => r.snapshot(tampered),
          throwsA(isA<V2Failure>()
              .having((e) => e.code, 'code', 'history.integrity')));
      expect(r.current, isNull);
    }
  });

  test('action error cursors reject gaps, changed evidence and stale epochs',
      () {
    for (final change in ['gap', 'evidence', 'board', 'epoch']) {
      final r = V2Reducer(limits);
      r.snapshot(fixture('snapshot-nower'));
      r.confirm({'kind': 'draw', 'count': 1}, 'request', 0);
      final event = <String, dynamic>{
        'v': 2,
        'cursor': {...r.current!.json['cursor'], 'recipient_seq': 2},
        'request_id': 'request',
        'code': 'request.rate_limited',
        'current_board_revision': r.current!.boardRevision
      };
      if (change == 'gap') event['cursor']['recipient_seq'] = 3;
      if (change == 'evidence') event['cursor']['evidence_seq'] += 1;
      if (change == 'board') event['current_board_revision'] += 1;
      if (change == 'epoch') event['cursor']['stream_epoch'] = 'old';
      expect(() => r.errorEvent(event), throwsA(isA<V2Failure>()));
      if (change != 'epoch') {
        expect(r.current, isNull);
      } else {
        expect(r.current, isNotNull);
        expect(r.pendingRequest?['request_id'], 'request');
      }
    }
  });

  test('sequenced old-request error cannot clear a newer intent', () {
    final r = V2Reducer(limits);
    r.snapshot(fixture('snapshot-nower'));
    r.confirm({'kind': 'draw', 'count': 1}, 'new', 0);
    r.errorEvent({
      'v': 2,
      'cursor': {...r.current!.json['cursor'], 'recipient_seq': 2},
      'request_id': 'old',
      'code': 'action.stale_revision',
      'current_board_revision': r.current!.boardRevision
    });
    expect(r.pendingRequest?['request_id'], 'new');
    expect(r.current, isNotNull);
    r.errorEvent({
      'v': 2,
      'cursor': {...r.current!.json['cursor'], 'recipient_seq': 3},
      'request_id': 'new',
      'code': 'request.rate_limited',
      'current_board_revision': r.current!.boardRevision
    });
    expect(r.retry()['request_id'], 'new');
  });

  test(
      'only a matching ack and newer complete state release the matching intent',
      () {
    final r = V2Reducer(limits);
    r.snapshot(fixture('snapshot-nower'));
    r.confirm({'kind': 'draw', 'count': 1}, 'one', 0);
    final next = fixture('snapshot-nower');
    next['cursor']['recipient_seq'] = 2;
    r.snapshot(next);
    expect(r.pendingRequest, isNotNull);
    r.acknowledge('older');
    expect(r.pendingRequest, isNotNull);
    r.acknowledge('one');
    expect(r.pendingRequest, isNull);
    r.confirm({'kind': 'draw', 'count': 1}, 'two', 0);
    r.acknowledge('one');
    expect(r.pendingRequest!['request_id'], 'two');
  });

  test('fresh epoch cannot roll back evidence, board or pinned role', () {
    for (final mutation in ['board', 'role']) {
      final r = V2Reducer(limits);
      final initial = fixture('snapshot-nower');
      initial['board']['revision'] = 3;
      r.snapshot(initial);
      r.disconnect();
      final stale = fixture('snapshot-nower');
      stale['cursor']['stream_epoch'] = 'fresh';
      stale['board']['revision'] = mutation == 'board' ? 2 : 3;
      if (mutation == 'role') {
        stale['private']['role'] = 'donower';
        stale['private'].remove('nown');
      }
      expect(() => r.snapshot(stale), throwsA(isA<V2Failure>()));
      expect(r.current, isNull);
    }
  });
  test('late previous-request error cannot clear current pending intent', () {
    final r = V2Reducer(limits);
    r.snapshot(fixture('snapshot-nower'));
    r.confirm({'kind': 'respond', 'copy_id': 'copy-1'}, 'new-request', 0);
    r.error('action.stale_revision', 'old-request');
    expect(r.pendingRequest!['request_id'], 'new-request');
    expect(r.current, isNotNull);
  });

  test('snapshot gaps freeze actions and resync replaces role state atomically',
      () {
    final r = V2Reducer(limits);
    final s = fixture('snapshot-nower');
    r.snapshot(s);
    expect(r.current!.nown, isNotNull);
    final newer = fixture('snapshot-nower');
    newer['cursor']['recipient_seq'] = 3;
    expect(() => r.snapshot(newer),
        throwsA(isA<V2Failure>().having((x) => x.code, 'code', 'stream.gap')));
    expect(r.current, isNull);
    expect(r.needsResync, isTrue);
    final hidden = fixture('snapshot-eliminated');
    hidden['cursor']['stream_epoch'] = 'new-stream';
    hidden['cursor']['recipient_seq'] = 1;
    r.snapshot(hidden);
    expect(r.current!.nown, isNull);
    expect(r.needsResync, isFalse);
    expect(() => r.snapshot(s), throwsA(isA<V2Failure>()));
    expect(r.current!.nown, isNull);
  });
  test(
      'paged history waits for every bound hash then rejects changed page context',
      () {
    final r = V2Reducer(limits);
    r.snapshot(fixture('snapshot-paged-history'));
    expect(r.current, isNull);
    r.page(fixture('public-history-page'));
    expect(r.current!.json['history'], hasLength(1));
    final other = V2Reducer(limits);
    other.snapshot(fixture('snapshot-paged-history'));
    final page = fixture('public-history-page');
    page['snapshot_id'] = 'wrong';
    expect(() => other.page(page), throwsA(isA<V2Failure>()));
    expect(other.current, isNull);
  });
  test('same cursor duplicate is harmless; conflicting payload is not applied',
      () {
    final r = V2Reducer(limits);
    final s = fixture('snapshot-nower');
    r.snapshot(s);
    r.snapshot(s);
    expect(r.current!.recipientSeq, 1);
    s['private']['hand'][0]['content']['text'] = 'changed';
    expect(() => r.snapshot(s), throwsA(isA<V2Failure>()));
    expect(r.current!.hand.first.text, isNot('changed'));
  });
  test('confirm pins atomic preview and request ID; retry preserves exact body',
      () {
    final r = V2Reducer(limits);
    r.snapshot(fixture('snapshot-nower'));
    final request =
        r.confirm({'kind': 'respond', 'copy_id': 'copy-1'}, 'intent-1', 0);
    expect(r.retry(), same(request));
    expect(request['phase_id'], r.current!.phaseID);
    expect(
        () =>
            r.confirm({'kind': 'respond', 'copy_id': 'copy-1'}, 'intent-2', 0),
        throwsA(isA<V2Failure>()));
    r.error('action.stale_revision', 'intent-1');
    expect(r.current, isNull);
    expect(r.pendingRequest, isNull);
  });
  test(
      'disconnect logout and match reset erase all private buffers and intents',
      () {
    final r = V2Reducer(limits);
    r.snapshot(fixture('snapshot-nower'));
    r.confirm({'kind': 'respond', 'copy_id': 'copy-1'}, 'req', 0);
    r.reset();
    expect(r.current, isNull);
    expect(r.pendingRequest, isNull);
    expect(r.pendingPageCount, 0);
    r.snapshot(fixture('snapshot-paged-history'));
    r.reset();
    expect(r.pendingPageCount, 0);
    expect(() => r.page(fixture('public-history-page')),
        throwsA(isA<V2Failure>()));
  });
}
