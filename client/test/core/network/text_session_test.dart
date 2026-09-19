import 'dart:async';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/core/network/game_transport.dart';
import 'package:knowoff_client/core/text/v2_session.dart';
import 'text_reducer_test.dart' show fixture;

class FakeTextTransport implements GameTransport {
  final frames = StreamController<Map<String, dynamic>>.broadcast(sync: true);
  final states = StreamController<ConnectionState>.broadcast(sync: true);
  final sent = <Map<String, dynamic>>[];
  @override
  bool isConnected = false;
  @override
  Stream<Map<String, dynamic>> get messages => frames.stream;
  @override
  Stream<ConnectionState> get state => states.stream;
  @override
  Future<void> connect() async {
    isConnected = true;
    states.add(ConnectionState.connected);
  }

  int reconnects = 0;
  @override
  Future<void> reconnect() {
    reconnects++;
    return connect();
  }

  @override
  Future<void> send(Map<String, dynamic> m) async {
    sent.add(m);
  }

  @override
  Future<void> close() async {
    isConnected = false;
  }

  void emit(String type, Map<String, dynamic> p) =>
      frames.add({'v': 2, 'type': type, 'payload': p});
}

Map<String, dynamic> hello({bool prototype = false}) => {
  'prototype': prototype,
  'client_generation': 2,
  'account_id': 'test-account',
  'limits': {
    'max_frame_bytes': 65536,
    'max_history_events': 8192,
    'max_history_page_events': 8,
    'max_text_bytes': 512,
    'max_requests_per_seat': 512,
  },
};
void admit(FakeTextTransport t) => t.emit('lobby', {
  'seat': 0,
  'code': 'ABC123',
  'lobby': fixture('lobby-ready-revisions'),
});
void main() {
  test(
    'identity replacement clears frozen state and role acknowledgement',
    () async {
      final t = FakeTextTransport();
      final s = TextSession(transport: t, tokenLoader: () async => 'token');
      await s.connect();
      await Future<void>.delayed(Duration.zero);
      t.emit('hello', hello(prototype: true));
      await s.control('room_create', {});
      admit(t);
      t.emit('dev_role', {'role': 'donower'});
      t.emit('snapshot', fixture('snapshot-nower'));
      s.freeze();
      await t.reconnect();
      await Future<void>.delayed(Duration.zero);
      t.emit('hello', {
        ...hello(prototype: true),
        'account_id': 'other-account',
      });
      expect(s.frozen, isFalse);
      expect(s.snapshot, isNull);
      expect(s.roomCode, isNull);
      expect(s.devRole, 'random');
      s.dispose();
    },
  );

  test(
    'prototype freeze holds clock, blocks actions and replays snapshots',
    () async {
      var clock = 1000;
      final t = FakeTextTransport();
      final s = TextSession(
        transport: t,
        tokenLoader: () async => 'token',
        now: () => DateTime.fromMillisecondsSinceEpoch(clock),
      );
      await s.connect();
      await Future<void>.delayed(Duration.zero);
      t.emit('hello', hello(prototype: true));
      await s.control('room_create', {});
      admit(t);
      final first = fixture('snapshot-nower');
      t.emit('snapshot', first);
      s.freeze();
      final held = s.serverNowMS;
      clock += 10000;
      final next = fixture('snapshot-nower');
      next['cursor']['recipient_seq']++;
      next['deadline_ms'] += 20000;
      t.emit('snapshot', next);
      clock += 1000;
      expect(s.frozen, isTrue);
      expect(s.serverNowMS, held);
      expect(s.snapshot!.deadlineMS, first['deadline_ms']);
      expect(() => s.act({'kind': 'ready'}), throwsA(anything));
      await s.unfreeze();
      expect(s.frozen, isFalse);
      expect(s.snapshot!.deadlineMS, next['deadline_ms']);
      expect(s.serverNowMS, isNot(held));
      s.freeze();
      s.background();
      expect(s.frozen, isFalse);
      expect(s.snapshot, isNull);
      s.dispose();
    },
  );

  test('freeze overflow discards private buffer and requires resync', () async {
    final t = FakeTextTransport();
    final s = TextSession(transport: t, tokenLoader: () async => 'token');
    await s.connect();
    await Future<void>.delayed(Duration.zero);
    t.emit('hello', hello(prototype: true));
    await s.control('room_create', {});
    admit(t);
    t.emit('snapshot', fixture('snapshot-nower'));
    s.freeze();
    for (var i = 0; i < 130; i++) {
      t.emit('snapshot', fixture('snapshot-nower'));
    }
    expect(s.frozen, isFalse);
    expect(s.snapshot, isNull);
    expect(t.sent.last['type'], 'resync');
    s.dispose();
  });

  test('dev role is prototype only and server acknowledged', () async {
    final t = FakeTextTransport();
    final s = TextSession(transport: t, tokenLoader: () async => 'token');
    await s.connect();
    await Future<void>.delayed(Duration.zero);
    t.emit('hello', hello(prototype: true));
    await s.selectDevRole('donower');
    expect(t.sent.last['type'], 'dev_role');
    expect(t.sent.last['payload'], {'role': 'donower'});
    t.emit('dev_role', {'role': 'donower'});
    expect(s.devRole, 'donower');
    expect(s.ready, isTrue);
    s.dispose();
    final production = TextSession(
      transport: FakeTextTransport(),
      tokenLoader: () async => 'token',
    );
    expect(() => production.freeze(), throwsA(anything));
    expect(() => production.selectDevRole('nower'), throwsA(anything));
    production.dispose();
  });

  test('rejected resync stops retries and later focus recovers', () async {
    final t = FakeTextTransport();
    final s = TextSession(transport: t, tokenLoader: () async => 'token');
    await s.connect();
    await Future<void>.delayed(Duration.zero);
    t.emit('hello', hello());
    await s.control('room_create', {});
    admit(t);
    t.emit('snapshot', fixture('snapshot-nower'));
    s.background();
    await s.resume();
    final count = t.sent.length;
    await s.control('resync', {});
    expect(t.sent.length, count, reason: 'one outstanding resync');
    final id = t.sent.last['request_id'];
    t.emit('error', {'code': 'request.rate_limited', 'request_id': id});
    expect(t.sent.length, count, reason: 'rejection must not retry itself');
    expect(s.snapshot, isNull);
    expect(s.reducer!.needsResync, isTrue);
    s.background();
    await s.resume();
    expect(t.sent.length, count + 1);
    expect(t.sent.last['type'], 'resync');
    final next = fixture('snapshot-nower');
    next['cursor']['stream_epoch'] = 'recovered';
    t.emit('snapshot', next);
    expect(s.snapshot, isNotNull);
    expect(s.errorCode, isNull);
    expect(t.sent.where((m) => m['type'] == 'action'), isEmpty);
    s.dispose();
  });

  test(
    'focus resume resyncs the same authenticated socket without disconnecting',
    () async {
      final t = FakeTextTransport();
      final s = TextSession(transport: t, tokenLoader: () async => 'token');
      await s.connect();
      await Future<void>.delayed(Duration.zero);
      t.emit('hello', hello());
      await s.control('room_create', {});
      admit(t);
      final before = fixture('snapshot-nower');
      t.emit('snapshot', before);
      s.background();
      expect(s.snapshot, isNull);
      t.emit('snapshot', before);
      expect(s.snapshot, isNull, reason: 'hidden frames never restore secrets');
      await s.resume();
      expect(t.reconnects, 0);
      expect(t.sent.last['type'], 'resync');
      expect(s.snapshot, isNull, reason: 'wait for authoritative state');
      final restored = fixture('snapshot-nower');
      restored['cursor']['stream_epoch'] = 'resumed-epoch';
      t.emit('snapshot', restored);
      expect(s.snapshot!.deadlineMS, before['deadline_ms']);
      expect(
        s.snapshot!.hand.map((c) => c.copyID),
        (before['private']['hand'] as List).map((c) => c['copy_id']),
      );
      expect(t.sent.where((m) => m['type'] == 'action'), isEmpty);
      s.dispose();
    },
  );

  test(
    'resume never reuses a changed token or a socket replaced while hidden',
    () async {
      for (final reason in ['token', 'socket', 'rejected']) {
        final t = FakeTextTransport();
        var token = 'original-token';
        final s = TextSession(transport: t, tokenLoader: () async => token);
        await s.connect();
        await Future<void>.delayed(Duration.zero);
        t.emit('hello', hello());
        if (reason == 'rejected') t.emit('error', {'code': 'auth.required'});
        s.background();
        if (reason == 'token') {
          token = 'replacement-token';
        } else if (reason == 'socket') {
          t.states.add(ConnectionState.disconnected);
          t.states.add(ConnectionState.connected);
        }
        await s.resume();
        await Future<void>.delayed(Duration.zero);
        expect(t.reconnects, 1);
        expect(t.sent.last['type'], 'hello');
        expect(t.sent.last['payload']['access_token'], token);
        expect(s.ready, isFalse);
        expect(s.snapshot, isNull);
        s.dispose();
      }
    },
  );

  test(
    'a delayed resume identity cannot restore a newly backgrounded client',
    () async {
      final t = FakeTextTransport();
      Completer<String>? pending;
      final s = TextSession(
        transport: t,
        tokenLoader: () => pending?.future ?? Future.value('token'),
      );
      await s.connect();
      await Future<void>.delayed(Duration.zero);
      t.emit('hello', hello());
      s.background();
      pending = Completer<String>();
      final resume = s.resume();
      s.background();
      pending.complete('token');
      await resume;
      expect(t.reconnects, 0);
      expect(s.ready, isFalse);
      expect(s.snapshot, isNull);
      s.dispose();
    },
  );

  test(
    'a lobby started while hidden resumes through authorized match resync',
    () async {
      final t = FakeTextTransport();
      final s = TextSession(transport: t, tokenLoader: () async => 'token');
      await s.connect();
      await Future<void>.delayed(Duration.zero);
      t.emit('hello', hello());
      await s.control('room_create', {});
      admit(t);
      s.background();
      t.emit('snapshot', fixture('snapshot-nower'));
      expect(s.snapshot, isNull);
      await s.resume();
      expect(t.reconnects, 0);
      expect(t.sent.last['type'], 'resync');
      t.emit('snapshot', fixture('snapshot-nower'));
      expect(s.snapshot!.nown, isNotNull);
      expect(s.lobby, isNull);
      s.dispose();
    },
  );

  test('resume refreshes authoritative lobby revisions', () async {
    final t = FakeTextTransport();
    final s = TextSession(transport: t, tokenLoader: () async => 'token');
    await s.connect();
    await Future<void>.delayed(Duration.zero);
    t.emit('hello', hello());
    await s.control('room_create', {});
    admit(t);
    s.background();
    await s.resume();
    final newer = fixture('lobby-ready-revisions');
    newer['membership_revision'] = 9;
    for (final seat in newer['seats']) {
      seat.remove('ready');
    }
    t.emit('lobby', {'seat': 0, 'code': 'ABC123', 'lobby': newer});
    expect(s.lobby!['membership_revision'], 9);
    expect(t.reconnects, 0);
    expect(s.reducer!.needsResync, isFalse);
    s.dispose();
  });

  test(
    'an unbound queue admission cannot reuse a socket as if it had a room',
    () async {
      final t = FakeTextTransport();
      final s = TextSession(transport: t, tokenLoader: () async => 'token');
      await s.connect();
      await Future<void>.delayed(Duration.zero);
      t.emit('hello', hello());
      await s.control('queue_join', {});
      s.background();
      await s.resume();
      expect(t.reconnects, 1);
      expect(t.sent.where((m) => m['type'] == 'resync'), isEmpty);
      s.dispose();
    },
  );

  test(
    'public notice invalidation is strict and never changes match state',
    () async {
      final t = FakeTextTransport();
      final s = TextSession(transport: t, tokenLoader: () async => 'token');
      var notices = 0;
      s.noticeChanges.addListener(() {
        notices++;
      });
      await s.connect();
      await Future<void>.delayed(Duration.zero);
      t.emit('hello', hello());
      final initial = notices;
      t.emit('system_notice', {'refresh': true});
      expect(notices, initial + 1);
      expect(s.ready, isTrue);
      expect(s.errorCode, isNull);
      t.emit('system_notice', {'refresh': true, 'hidden_nown': 'forged'});
      expect(notices, initial + 1);
      expect(s.ready, isFalse);
      s.dispose();
    },
  );
  test('prototype metadata must match the authenticated hello', () async {
    final t = FakeTextTransport();
    final s = TextSession(transport: t, tokenLoader: () async => 'token');
    await s.connect();
    await Future<void>.delayed(Duration.zero);
    t.emit('hello', hello(prototype: true));
    t.emit('availability', {
      'protocol_version': 2,
      'client_generation': 2,
      'prototype': false,
      'limits': hello()['limits'],
      'modes': [
        for (final mode in [
          'missed_the_briefing',
          'secret_scale',
          'make_room',
          'bad_bargains',
          'top_that',
        ])
          {'mode_id': mode, 'available': false, 'languages': []},
      ],
    });
    expect(s.errorCode, 'protocol.upgrade_required');
    expect(s.ready, isFalse);
    expect(s.availability, isEmpty);
    s.dispose();
  });

  test(
    'sequenced action errors retain exact retry intent and advance only their cursor',
    () async {
      final t = FakeTextTransport();
      final s = TextSession(
        transport: t,
        tokenLoader: () async => 'token',
        now: () => DateTime.fromMillisecondsSinceEpoch(0),
      );
      await s.connect();
      await Future<void>.delayed(Duration.zero);
      t.emit('hello', hello());
      await s.control('room_create', {});
      admit(t);
      t.emit('snapshot', fixture('snapshot-nower'));
      await s.act({'kind': 'draw', 'count': 1});
      final request = t.sent.last['payload'];
      final error = <String, dynamic>{
        'v': 2,
        'cursor': {...s.snapshot!.json['cursor'], 'recipient_seq': 2},
        'request_id': request['request_id'],
        'code': 'action.persistence_pending',
        'current_board_revision': s.snapshot!.boardRevision,
      };
      t.frames.add({
        'v': 2,
        'type': 'error',
        'request_id': request['request_id'],
        'payload': error,
      });
      expect(s.ready, isTrue);
      expect(s.snapshot?.hand, isNotEmpty);
      expect(s.reducer!.pendingRequest, request);
      await s.retry();
      expect(t.sent.last['payload'], request);
      t.frames.add({
        'v': 2,
        'type': 'error',
        'request_id': request['request_id'],
        'payload': error,
      });
      final next = fixture('snapshot-nower');
      next['cursor']['recipient_seq'] = 3;
      t.emit('snapshot', next);
      expect(s.snapshot?.recipientSeq, 3);
      expect(s.ready, isTrue);
      expect(s.reducer!.pendingRequest, request);
      t.frames.add({
        'v': 2,
        'type': 'action_ack',
        'request_id': request['request_id'],
        'payload': {'request_id': request['request_id'], 'duplicate': true},
      });
      expect(s.reducer!.pendingRequest, isNull);
      s.dispose();
    },
  );

  test(
    'authoritative rematch reaches every member and stale lobby cannot roll back',
    () async {
      for (final initiates in [false, true]) {
        final t = FakeTextTransport();
        final s = TextSession(transport: t, tokenLoader: () async => 'token');
        await s.connect();
        await Future<void>.delayed(Duration.zero);
        t.emit('hello', hello());
        await s.control('room_create', {});
        admit(t);
        t.emit('snapshot', fixture('snapshot-verdict-begun-only'));
        expect(s.snapshot?.phase, 'verdict');
        if (initiates) {
          await s.control('rematch', {});
          expect(
            s.snapshot?.phase,
            'verdict',
            reason: 'retain result until server accepts',
          );
        }
        admit(t);
        expect(
          s.snapshot?.phase,
          'verdict',
          reason: 'old lobby cannot erase result',
        );
        final next = fixture('lobby-ready-revisions');
        next['settings_revision'] = 3;
        next['membership_revision'] = 4;
        for (final seat in next['seats']) {
          seat.remove('ready');
        }
        t.emit('lobby', {'seat': 0, 'code': 'ABC123', 'lobby': next});
        expect(s.snapshot, isNull);
        expect(s.lobby?['settings_revision'], 3);
        admit(t);
        expect(
          s.lobby?['settings_revision'],
          3,
          reason: 'late Ready belongs to old revisions',
        );
        expect(s.lobby?['seats'][0]['ready'], isNull);
        s.dispose();
      }
    },
  );

  test('role-linked Noin receipts never enter the live match view', () async {
    final t = FakeTextTransport();
    final s = TextSession(transport: t, tokenLoader: () async => 'token');
    await s.connect();
    await Future<void>.delayed(Duration.zero);
    t.emit('hello', hello());
    await s.control('room_create', {});
    admit(t);
    t.emit('snapshot', fixture('snapshot-nower'));
    t.emit('award', {
      'match_id': 'match-a',
      'kind': 'correct_vote',
      'ordinal': 1,
      'requested': 5,
      'credited': 5,
    });
    expect(s.awards, isEmpty);
    t.emit('settlement', {
      'id': 77,
      'match_id': 'match-a',
      'settlement': {
        'match_id': 'match-a',
        'points': 20,
        'xp': 10,
        'leaderboard_counted': false,
        'awards': [
          {'kind': 'correct_vote', 'ordinal': 1, 'requested': 5, 'credited': 5},
        ],
      },
    });
    expect(s.settlements, isEmpty);
    expect(t.sent.where((f) => f['type'] == 'settlement_ack'), isEmpty);
    s.dispose();
  });

  test(
    'private settlement is immutable, deduplicated and acknowledged only after presentation',
    () async {
      final t = FakeTextTransport();
      final s = TextSession(transport: t, tokenLoader: () async => 'token');
      await s.connect();
      await Future<void>.delayed(Duration.zero);
      t.emit('hello', hello());
      final delivery = <String, dynamic>{
        'id': 12,
        'match_id': 'settled-match',
        'settlement': {
          'match_id': 'settled-match',
          'points': 20,
          'xp': 10,
          'leaderboard_counted': true,
          'awards': [
            {
              'kind': 'match_completed',
              'ordinal': 0,
              'requested': 5,
              'credited': 3,
            },
          ],
        },
      };
      t.emit('settlement', delivery);
      t.emit('settlement', delivery);
      expect(s.settlements, hasLength(1));
      expect(t.sent.where((x) => x['type'] == 'settlement_ack'), isEmpty);
      await s.dismissDelivery(12);
      expect(t.sent.last['type'], 'settlement_ack');
      expect(t.sent.last['payload'], {'id': 12});
      expect(s.settlements, isEmpty);
      t.emit('settlement', delivery);
      expect(s.settlements, isEmpty);
      await Future<void>.delayed(Duration.zero);
      expect(
        t.sent.where((x) => x['type'] == 'settlement_ack'),
        hasLength(2),
        reason: 'a lost acknowledgement must be retried without redisplaying',
      );
      delivery['settlement']['xp'] = 999;
      t.emit('settlement', delivery);
      expect(s.errorCode, 'request.conflict');
      s.dispose();
    },
  );

  test('late old match cannot attach to a new room admission', () async {
    final t = FakeTextTransport();
    final s = TextSession(transport: t, tokenLoader: () async => 'token');
    await s.connect();
    await Future<void>.delayed(Duration.zero);
    t.emit('hello', hello());
    await s.control('room_create', {});
    admit(t);
    t.emit('snapshot', fixture('snapshot-nower'));
    await s.leave();
    await s.control('room_create', {});
    t.emit('snapshot', fixture('snapshot-nower'));
    expect(s.snapshot, isNull);
    final other = fixture('lobby-ready-revisions');
    other['room_id'] = 'new-room';
    t.emit('lobby', {'seat': 0, 'code': 'NEW123', 'lobby': other});
    t.emit('snapshot', fixture('snapshot-nower'));
    expect(s.snapshot, isNull);
    s.dispose();
  });
  test(
    'late snapshot or hello cannot resurrect a left or backgrounded session',
    () async {
      for (final lifecycle in ['leave', 'background']) {
        final t = FakeTextTransport();
        final s = TextSession(transport: t, tokenLoader: () async => 'token');
        await s.connect();
        await Future<void>.delayed(Duration.zero);
        t.emit('hello', hello());
        await s.control('room_create', {});
        admit(t);
        t.emit('snapshot', fixture('snapshot-nower'));
        if (lifecycle == 'leave') {
          await s.leave();
        } else {
          s.background();
        }
        t.emit('snapshot', fixture('snapshot-nower'));
        t.emit('hello', hello());
        t.emit('snapshot', fixture('snapshot-nower'));
        expect(s.snapshot, isNull);
        if (lifecycle == 'background') expect(s.ready, isFalse);
        s.dispose();
      }
    },
  );

  test(
    'malformed control wrappers fail closed without exceptions or fabricated availability',
    () async {
      for (final payload in [
        <String, dynamic>{},
        <String, dynamic>{
          'client_generation': 2,
          'account_id': 'account',
          'limits': null,
        },
      ]) {
        final session = TextSession(
          transport: FakeTextTransport(),
          tokenLoader: () async => 'token',
        );
        expect(
          () => session.receive({'v': 2, 'type': 'hello', 'payload': payload}),
          returnsNormally,
        );
        expect(session.ready, isFalse);
        expect(session.snapshot, isNull);
        session.dispose();
      }
    },
  );
  test(
    'acknowledgement cannot unlock another action before authoritative state',
    () async {
      final t = FakeTextTransport();
      final session = TextSession(
        transport: t,
        tokenLoader: () async => 'token',
        now: () => DateTime.fromMillisecondsSinceEpoch(0),
      );
      await session.connect();
      await Future<void>.delayed(Duration.zero);
      t.emit('hello', hello());
      await session.control('room_create', {});
      admit(t);
      t.emit('snapshot', fixture('snapshot-nower'));
      session.reducer!.confirm(
        {'kind': 'respond', 'copy_id': 'copy-1'},
        'req',
        0,
      );
      t.frames.add({
        'v': 2,
        'type': 'action_ack',
        'request_id': 'req',
        'payload': {'request_id': 'req', 'duplicate': false},
      });
      expect(session.errorCode, isNull);
      expect(session.reducer!.pendingRequest, isNotNull);
      session.dispose();
    },
  );

  test(
    'v2 session negotiates before sending admission and accepts server availability only',
    () async {
      final t = FakeTextTransport();
      final s = TextSession(
        transport: t,
        tokenLoader: () async => 'private-token',
      );
      await s.connect();
      await Future<void>.delayed(Duration.zero);
      expect(t.sent.single['type'], 'hello');
      expect(t.sent.single['payload']['client_generation'], 2);
      expect(() => s.control('room_create', {}), throwsA(anything));
      t.emit('hello', hello());
      expect(s.ready, isTrue);
      t.emit('availability', {
        'protocol_version': 2,
        'client_generation': 2,
        'prototype': false,
        'limits': hello()['limits'],
        'modes': [
          for (final mode in [
            'missed_the_briefing',
            'secret_scale',
            'make_room',
            'bad_bargains',
            'top_that',
          ])
            {'mode_id': mode, 'available': false, 'languages': []},
        ],
      });
      expect(s.availability.every((x) => !x.available), isTrue);
      await s.control('room_create', {});
      admit(t);
      t.emit('snapshot', fixture('snapshot-nower'));
      expect(s.snapshot!.nown, isNotNull);
      t.states.add(ConnectionState.disconnected);
      expect(s.snapshot, isNull);
      expect(s.ready, isFalse);
      s.dispose();
    },
  );
  test(
    'upgrade failure and late token resolution never bind or expose private state',
    () async {
      final t = FakeTextTransport();
      final token = Completer<String>();
      final s = TextSession(transport: t, tokenLoader: () => token.future);
      await s.connect();
      s.dispose();
      token.complete('secret');
      await Future<void>.delayed(Duration.zero);
      expect(t.sent, isEmpty);
      final second = TextSession(
        transport: FakeTextTransport(),
        tokenLoader: () async => 'token',
      );
      second.receive({
        'v': 1,
        'type': 'state',
        'payload': fixture('snapshot-nower'),
      });
      expect(second.errorCode, 'protocol.upgrade_required');
      expect(second.snapshot, isNull);
      second.dispose();
    },
  );
}
