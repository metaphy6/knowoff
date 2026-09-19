import 'dart:async';
import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/core/config/app_config.dart';
import 'package:knowoff_client/core/config/client_config.dart';
import 'package:knowoff_client/core/text/v2_contract.dart';
import 'package:knowoff_client/core/text/v2_session.dart';
import 'package:knowoff_client/core/text/v2_reducer.dart';
import 'package:knowoff_client/data/auth_service.dart';

import '../core/network/text_reducer_test.dart' show fixture;
import '../core/network/text_session_test.dart'
    show FakeTextTransport, hello, admit;

const _modes = <String, Map<String, dynamic>>{
  'nower': {'kind': 'respond', 'copy_id': 'copy-1'},
  'secret_scale': {'kind': 'place', 'copy_id': 'copy-1', 'rating': 3},
  'make_room': {'kind': 'replace', 'copy_id': 'copy-1', 'slot': 0},
  'bad_bargains': {
    'kind': 'offer',
    'copy_id': 'copy-1',
    'target_copy_id': 'seed-1',
    'target_seat': 1,
  },
  'top_that': {'kind': 'top', 'copy_id': 'copy-1', 'target_copy_id': 'seed-0'},
};

Future<(TextSession, FakeTextTransport)> _started([
  String name = 'nower',
  DateTime Function()? now,
]) async {
  final transport = FakeTextTransport();
  final session = TextSession(
    transport: transport,
    tokenLoader: () async => 'fixture-token',
    now: now ?? () => DateTime.fromMillisecondsSinceEpoch(0),
  );
  addTearDown(session.dispose);
  await session.connect();
  await Future<void>.delayed(Duration.zero);
  transport.emit('hello', hello());
  await session.control('room_create', {});
  admit(transport);
  transport.emit('snapshot', fixture('snapshot-$name'));
  expect(session.snapshot, isNotNull);
  return (session, transport);
}

void _ack(FakeTextTransport t, String id) => t.frames.add({
  'v': 2,
  'type': 'action_ack',
  'request_id': id,
  'payload': {'request_id': id, 'duplicate': false},
});
void _error(
  TextSession s,
  FakeTextTransport t,
  String id,
  String code, {
  int seq = 2,
}) => t.frames.add({
  'v': 2,
  'type': 'error',
  'request_id': id,
  'payload': {
    'v': 2,
    'request_id': id,
    'code': code,
    'cursor': {...s.snapshot!.json['cursor'], 'recipient_seq': seq},
    'current_board_revision': s.snapshot!.boardRevision,
  },
});

// Original notifier cases transfer to the actual recipient-scoped session.
// Retired powers are rejection proofs; local selection lives in TextMatchView.
void main() {
  for (final entry in _modes.entries) {
    test(
      '${entry.key}: atomic move carries exact evidence and never spends optimistically',
      () async {
        final (s, t) = await _started(entry.key);
        final before = jsonEncode(s.snapshot!.json);
        await s.act(entry.value);
        final sent = t.sent.last;
        expect(sent['v'], 2);
        expect(sent['type'], 'action');
        final request = sent['payload'];
        expect(request['action'], entry.value);
        expect(request['match_id'], s.snapshot!.matchID);
        expect(request['mode_id'], s.snapshot!.mode);
        expect(request['phase_id'], s.snapshot!.phaseID);
        expect(request['expected_board_revision'], s.snapshot!.boardRevision);
        expect(jsonEncode(s.snapshot!.json), before);
        expect(() => s.act(entry.value), throwsA(isA<V2Failure>()));
        expect(t.sent.last, same(sent));
        await s.retry();
        expect(t.sent.last['payload'], same(request));
        expect(jsonEncode(s.snapshot!.json), before);
      },
    );
    test(
      '${entry.key}: deadline is server-owned and expiry never emits an auto-play',
      () async {
        var now = DateTime.fromMillisecondsSinceEpoch(0);
        final (s, t) = await _started(entry.key, () => now);
        final before = t.sent.length;
        final deadline = s.snapshot!.deadlineMS;
        now = now.add(const Duration(seconds: 25));
        expect(s.snapshot!.deadlineMS, deadline);
        expect(
          () => s.act(entry.value),
          throwsA(
            isA<V2Failure>().having(
              (e) => e.code,
              'code',
              'action.deadline_expired',
            ),
          ),
        );
        expect(t.sent.length, before);
        expect(s.snapshot!.hand.single.copyID, 'copy-1');
      },
    );
    test(
      '${entry.key}: background erases private state and resume reclaims same seat without retrying move',
      () async {
        final (s, t) = await _started(entry.key);
        await s.act(entry.value);
        final before = t.sent.where((f) => f['type'] == 'action').length;
        s.background();
        expect(s.snapshot, isNull);
        expect(s.reducer!.pendingRequest, isNull);
        expect(s.awards, isEmpty);
        expect(s.settlements, isEmpty);
        t.emit('snapshot', fixture('snapshot-${entry.key}'));
        expect(s.snapshot, isNull);
        await s.resume();
        await Future<void>.delayed(Duration.zero);
        expect(t.reconnects, 0);
        expect(t.sent.last['type'], 'resync');
        expect(t.sent.last['payload'], isEmpty);
        final next = fixture('snapshot-${entry.key}');
        next['cursor']['stream_epoch'] = 'resumed';
        t.emit('snapshot', next);
        expect(s.snapshot!.seat, 0);
        expect(s.snapshot!.hand.single.copyID, 'copy-1');
        expect(s.snapshot!.deadlineMS, 21000);
        expect(t.sent.where((f) => f['type'] == 'action').length, before);
      },
    );
  }

  for (final type in [
    'hand_reveal_viewed',
    'hand_reveal_available',
    'specialty_used',
    'shuffle',
    'hand_dealt',
    'free_draw',
    'draw_result',
    'phase_started',
    'turn_started',
    'dev_force_role',
    'dev_grant_specialty',
    'prefetch',
    'play_revealed',
    'vote_cast',
    'ready_ack',
    'rematch_state',
  ]) {
    test(
      'retired $type cannot install private fields, buffer a replay or grant authority',
      () async {
        final (s, t) = await _started();
        final count = t.sent.length;
        t.emit(type, {
          'target_seat': 2,
          'role': 'donower',
          'cards': ['OTHER SECRET'],
          'draw_pile': ['PRIVATE RESERVE'],
          'specialty': 'reveal',
          'free_draws': 1,
        });
        expect(s.snapshot, isNull);
        expect(s.ready, isFalse);
        expect(s.reducer!.pendingRequest, isNull);
        expect(t.sent.length, count);
        expect(
          () => s.act({'kind': 'draw', 'count': 1}),
          throwsA(isA<V2Failure>()),
        );
        expect(t.sent.length, count);
      },
    );
  }

  test(
    'legacy envelope is refused rather than becoming a new role baseline',
    () async {
      final (s, t) = await _started();
      t.frames.add({
        'kind': 'hand_dealt',
        'payload': {
          'cards': ['secret'],
        },
      });
      expect(s.ready, isFalse);
      expect(s.snapshot, isNull);
    },
  );

  test(
    'Ready acknowledgement alone never fabricates readiness or a phase change',
    () async {
      final (s, t) = await _started('knowoff');
      final before = jsonEncode(s.snapshot!.json);
      await s.act({'kind': 'ready'});
      final id = s.reducer!.pendingRequest!['request_id'] as String;
      _ack(t, id);
      expect(jsonEncode(s.snapshot!.json), before);
      expect(s.reducer!.pendingRequest, isNotNull);
      final next = fixture('snapshot-knowoff');
      next['cursor']['recipient_seq'] = 2;
      next['ready_seats'] = [0, 1, 2];
      t.emit('snapshot', next);
      expect(s.snapshot!.json['ready_seats'], [0, 1, 2]);
      expect(s.reducer!.pendingRequest, isNull);
      next['cursor']['recipient_seq'] = 3;
      next['ready_seats'] = [1, 2];
      t.emit('snapshot', next);
      expect(s.snapshot!.json['ready_seats'], [1, 2]);
    },
  );

  test(
    'new round atomically replaces hand, prior ballot, Ready and phase deadline',
    () async {
      final (s, t) = await _started('result');
      final next = fixture('snapshot-nower');
      next['round'] = 2;
      next['phase_id'] = 'round-two';
      next['cursor']['recipient_seq'] = 2;
      next['deadline_ms'] = 35000;
      next['private']['hand'][0]['copy_id'] = 'retained-copy';
      t.emit('snapshot', next);
      expect(s.snapshot!.round, 2);
      expect(s.snapshot!.json.containsKey('ballot'), isFalse);
      expect(s.snapshot!.json['ready_seats'], isEmpty);
      expect(s.snapshot!.hand.single.copyID, 'retained-copy');
      expect(s.snapshot!.deadlineMS, 35000);
      expect(t.sent.where((f) => f['type'] == 'action'), isEmpty);
    },
  );

  for (final kind in ['draw', 'respond', 'timeout']) {
    test(
      '$kind changes the hand only when the authoritative complete snapshot arrives',
      () async {
        final (s, t) = await _started();
        if (kind == 'draw') await s.act({'kind': 'draw', 'count': 1});
        if (kind == 'respond') {
          await s.act({'kind': 'respond', 'copy_id': 'copy-1'});
        }
        expect(s.snapshot!.hand.single.copyID, 'copy-1');
        final next = fixture('snapshot-nower');
        next['cursor']['recipient_seq'] = 2;
        if (kind == 'draw') {
          next['private']['hand'].add({
            'copy_id': 'drawn-copy',
            'content': {'content_id': 'new', 'revision': 1, 'text': 'New card'},
          });
          next['private']['reserve_count'] = 2;
          final event = fixture('public-history-page')['events'][0];
          next['history'] = [event];
          next['cursor']['evidence_seq'] = 1;
          next['board']['revision'] = 1;
        } else {
          next['private']['hand'] = [];
          next['turn'] = 2;
          next['current_seat'] = 1;
          next['private']['capabilities'] = ['poke', 'chat'];
        }
        t.emit('snapshot', next);
        if (kind == 'draw') {
          expect(s.snapshot!.hand.map((c) => c.copyID), [
            'copy-1',
            'drawn-copy',
          ]);
          expect(s.snapshot!.reserveCount, 2);
          expect(s.snapshot!.json['history'][0]['actor'], {
            'kind': 'seat',
            'seat': 0,
          });
          expect(s.snapshot!.json['history'][0]['count'], 1);
          expect(s.snapshot!.json['history'][0]['cards'], isEmpty);
        } else {
          expect(s.snapshot!.hand, isEmpty);
          expect(s.snapshot!.json['current_seat'], 1);
        }
      },
    );
  }

  for (final code in [
    'action.persistence_pending',
    'request.rate_limited',
    'action.unauthorized',
    'action.stale_revision',
  ]) {
    test(
      'correlated $code preserves or clears only its exact pending ballot',
      () async {
        final (s, t) = await _started('knowoff');
        await s.act({'kind': 'vote', 'target_seat': 2});
        final pending = s.reducer!.pendingRequest!;
        final id = pending['request_id'] as String;
        _error(s, t, 'unrelated-request', code);
        expect(s.reducer!.pendingRequest, same(pending));
        _error(s, t, id, code, seq: 3);
        if (code == 'action.persistence_pending' ||
            code == 'request.rate_limited') {
          expect(s.reducer!.pendingRequest, same(pending));
          await s.retry();
          expect(t.sent.last['payload'], same(pending));
          expect(s.snapshot!.json['ballot']['votes'], [
            {'seat': 0, 'target_seat': 1},
          ]);
        } else {
          expect(s.reducer!.pendingRequest, isNull);
          expect(s.snapshot == null, code == 'action.stale_revision');
        }
      },
    );
  }

  test(
    'resolved ballot and revealed role arrive only from the server result',
    () async {
      final (s, t) = await _started('knowoff');
      final runoff = fixture('snapshot-runoff');
      runoff['cursor']['recipient_seq'] = 2;
      t.emit('snapshot', runoff);
      expect(s.snapshot!.json['ballot']['candidates'], [1, 2]);
      expect(s.snapshot!.json['ballot'].containsKey('result'), isFalse);
      final result = fixture('snapshot-result');
      result['cursor']['recipient_seq'] = 3;
      t.emit('snapshot', result);
      expect(s.snapshot!.json['ballot']['result']['revealed_role'], 'donower');
      expect(s.snapshot!.json['ballot']['votes'], [
        {'seat': 0, 'target_seat': 1},
      ]);
    },
  );

  for (final phase in ['discussion', 'knowoff', 'result']) {
    test(
      '$phase retains original deadline and history through unrelated seat updates',
      () async {
        final (s, t) = await _started(phase == 'discussion' ? 'nower' : phase);
        final next = fixture(
          'snapshot-${phase == 'discussion' ? 'nower' : phase}',
        );
        next['phase'] = phase;
        if (phase == 'discussion') {
          next.remove('current_seat');
          next['private']['capabilities'] = ['ready', 'chat'];
        }
        next['cursor']['recipient_seq'] = 2;
        next['seats'][2]['connected'] = false;
        t.emit('snapshot', next);
        expect(s.snapshot!.deadlineMS, 21000);
        expect(s.snapshot!.json['seats'][2]['connected'], isFalse);
        expect(s.snapshot!.json['history'], isEmpty);
        expect(t.sent.where((f) => f['type'] == 'action'), isEmpty);
      },
    );
  }

  for (final mode in ['same_table', 'new_table']) {
    test(
      'verdict $mode choice is explicit and leave erases match before new admission',
      () async {
        final (s, t) = await _started('verdict-begun-only');
        final before = jsonEncode(s.snapshot!.json);
        if (mode == 'same_table') {
          await s.control('rematch', {});
          expect(t.sent.last['type'], 'rematch');
          expect(t.sent.last['payload'], isEmpty);
          expect(jsonEncode(s.snapshot!.json), before);
        }
        await s.leave();
        expect(s.snapshot, isNull);
        expect(s.roomCode, isNull);
        expect(s.seat, isNull);
        expect(s.reducer!.pendingRequest, isNull);
        t.emit('snapshot', fixture('snapshot-nower'));
        expect(s.snapshot, isNull);
        if (mode == 'new_table') {
          const tuple = {
            'mode_id': 'missed_the_briefing',
            'size': 4,
            'content_language': 'tr',
            'pack_release_id': 'release-1',
            'rules_version': 'text-1',
          };
          await s.control('queue_join', tuple);
          expect(t.sent.last['type'], 'queue_join');
          expect(t.sent.last['payload'], tuple);
          expect(s.snapshot, isNull);
        }
      },
    );
  }

  test(
    'handshake waits for credentials and sends no role override or queue implicitly',
    () async {
      final token = Completer<String>();
      final t = FakeTextTransport();
      final s = TextSession(transport: t, tokenLoader: () => token.future);
      addTearDown(s.dispose);
      await s.connect();
      expect(t.sent, isEmpty);
      token.complete('refreshed-token');
      await Future<void>.delayed(Duration.zero);
      expect(t.sent.single, {
        'v': 2,
        'type': 'hello',
        'payload': {'client_generation': 2, 'access_token': 'refreshed-token'},
      });
      t.emit('hello', hello());
      expect(t.sent, hasLength(1));
      expect(s.ready, isTrue);
      expect(s.seat, isNull);
    },
  );

  test(
    'rejected handshake requires explicit retry without creating or queuing an identity',
    () async {
      final t = FakeTextTransport();
      var calls = 0;
      final s = TextSession(
        transport: t,
        tokenLoader: () async {
          calls++;
          return 'token-$calls';
        },
      );
      addTearDown(s.dispose);
      await s.connect();
      await Future<void>.delayed(Duration.zero);
      t.emit('error', {'code': 'auth.required'});
      expect(s.ready, isFalse);
      await Future<void>.delayed(Duration.zero);
      expect(calls, 1);
      expect(t.sent, hasLength(1));
      await s.resume();
      await Future<void>.delayed(Duration.zero);
      expect(calls, 2);
      expect(t.sent.last['payload']['access_token'], 'token-2');
      t.emit('hello', hello());
      expect(s.ready, isTrue);
      expect(t.sent.where((f) => f['type'] == 'queue_join'), isEmpty);
    },
  );

  test(
    'unavailable credentials clear authority and public AppConfig still initializes',
    () async {
      final t = FakeTextTransport();
      final s = TextSession(
        transport: t,
        tokenLoader: () async => throw StateError('offline'),
      );
      addTearDown(s.dispose);
      await s.connect();
      await Future<void>.delayed(Duration.zero);
      expect(t.sent, isEmpty);
      expect(s.errorCode, 'auth.required');
      expect(s.ready, isFalse);
      final app = await AppConfig.initialize(
        ClientConfig.defaultConfig(),
        AuthService(baseUrl: 'https://offline.invalid'),
      );
      expect(app.clientConfig.protocolVersion, 2);
    },
  );

  test('negotiated mutation budget survives phase changes and reconnect', () {
    const bounded = V2Limits(
      maxFrameBytes: 65536,
      maxHistoryEvents: 8192,
      maxHistoryPageEvents: 8,
      maxTextBytes: 512,
      maxRequestsPerSeat: 1,
    );
    final r = V2Reducer(bounded)..snapshot(fixture('snapshot-nower'));
    r.confirm({'kind': 'draw', 'count': 1}, 'first', 0);
    r.acknowledge('first');
    final ballot = fixture('snapshot-knowoff');
    ballot['cursor']['recipient_seq'] = 2;
    r.snapshot(ballot);
    expect(r.pendingRequest, isNull);
    r.disconnect();
    ballot['cursor']['stream_epoch'] = 'new-budget-stream';
    ballot['cursor']['recipient_seq'] = 1;
    r.snapshot(ballot);
    expect(
      () => r.confirm({'kind': 'vote', 'target_seat': 2}, 'second', 0),
      throwsA(isA<V2Failure>().having((e) => e.code, 'code', 'request.limit')),
    );
    expect(r.pendingRequest, isNull);
    expect(r.current!.json['ballot']['votes'], [
      {'seat': 0, 'target_seat': 1},
    ]);
  });

  test(
    'a phase missing its mandatory deadline cannot preserve an old countdown',
    () async {
      final (s, t) = await _started();
      final missing = fixture('snapshot-nower');
      missing['cursor']['recipient_seq'] = 2;
      missing.remove('deadline_ms');
      t.emit('snapshot', missing);
      expect(s.snapshot, isNull);
      expect(s.ready, isFalse);
      expect(
        () => s.act({'kind': 'draw', 'count': 1}),
        throwsA(isA<V2Failure>()),
      );
    },
  );

  test(
    'late token after background cannot send hello or revive hidden private state',
    () async {
      final token = Completer<String>();
      final t = FakeTextTransport();
      final s = TextSession(transport: t, tokenLoader: () => token.future);
      addTearDown(s.dispose);
      await s.connect();
      s.background();
      token.complete('late-token');
      await Future<void>.delayed(Duration.zero);
      expect(t.sent, isEmpty);
      expect(s.snapshot, isNull);
      expect(s.ready, isFalse);
    },
  );
}
