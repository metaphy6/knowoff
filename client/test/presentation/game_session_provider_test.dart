import 'dart:async';

import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/core/network/game_transport.dart' as gt;
import 'package:knowoff_client/data/models/game_state_dto.dart';
import 'package:knowoff_client/presentation/state/game_session_provider.dart';

class _FakeTransport implements gt.GameTransport {
  final StreamController<Map<String, dynamic>> _controller =
      StreamController<Map<String, dynamic>>.broadcast();
  final List<Map<String, dynamic>> sent = <Map<String, dynamic>>[];

  void emit(String kind, Map<String, dynamic> payload) {
    _controller.add(<String, dynamic>{'kind': kind, 'payload': payload});
  }

  @override
  Stream<Map<String, dynamic>> get messages => _controller.stream;

  @override
  Stream<gt.ConnectionState> get state =>
      Stream<gt.ConnectionState>.value(gt.ConnectionState.connected);

  @override
  bool get isConnected => true;

  int reconnectCount = 0;

  @override
  Future<void> close() async => _controller.close();

  @override
  Future<void> connect() async {}

  @override
  Future<void> reconnect() async => reconnectCount++;

  @override
  Future<void> send(Map<String, dynamic> message) async => sent.add(message);
}

Future<void> _settle() => Future<void>.delayed(Duration.zero);

void main() {
  late _FakeTransport transport;
  late GameSessionNotifier notifier;

  setUp(() {
    transport = _FakeTransport();
    notifier = GameSessionNotifier(transport: transport);
  });

  tearDown(() {
    notifier.dispose();
    transport.close();
  });

  test('freezing buffers incoming events instead of applying them', () async {
    notifier.setFrozen(true);
    transport.emit('phase_started', <String, dynamic>{
      'phase': 'discussion',
      'window_seconds': 40,
    });
    await _settle();

    expect(notifier.state.frozen, isTrue);
    expect(notifier.state.dto.phase, equals('waiting'));
  });

  test('unfreezing replays buffered events in order', () async {
    notifier.setFrozen(true);
    transport.emit('phase_started', <String, dynamic>{'phase': 'discussion'});
    transport.emit('phase_started', <String, dynamic>{'phase': 'knowoff'});
    await _settle();
    expect(notifier.state.dto.phase, equals('waiting'));

    notifier.setFrozen(false);
    await _settle();

    expect(notifier.state.frozen, isFalse);
    expect(notifier.state.dto.phase, equals('knowoff'));
  });

  test('restarting resets state to initial and drops buffered events',
      () async {
    transport.emit('phase_started', <String, dynamic>{
      'phase': 'play',
      'round': 3,
    });
    await _settle();
    expect(notifier.state.dto.phase, equals('play'));

    notifier.setFrozen(true);
    transport.emit('phase_started', <String, dynamic>{'phase': 'discussion'});
    await _settle();

    notifier.restart();

    expect(notifier.state.frozen, isFalse);
    expect(notifier.state.dto.phase, equals('waiting'));
    expect(notifier.state.dto.round, equals(0));

    // The server only accepts a queue/join intent as a connection's first
    // message, so the next queue attempt needs a fresh handshake.
    await _settle();
    expect(transport.reconnectCount, equals(1));

    // The buffered 'discussion' event must not resurface after a restart.
    notifier.setFrozen(true);
    notifier.setFrozen(false);
    await _settle();
    expect(notifier.state.dto.phase, equals('waiting'));
  });

  test('a fresh ballot clears the previous result and vote', () async {
    transport.emit('phase_started', <String, dynamic>{
      'phase': 'result',
      'result': <String, dynamic>{
        'eliminated_seat': 1,
        'role': 'nower',
        'tally': <String, dynamic>{'1': 2},
      },
    });
    transport.emit('knowoff_resolved', <String, dynamic>{'vote_target': 1});
    await _settle();

    expect(notifier.state.dto.result, isNotNull);
    expect(notifier.state.dto.voteTarget, equals(1));

    transport.emit('phase_started', <String, dynamic>{
      'phase': 'knowoff',
      'window_seconds': 20,
    });
    await _settle();

    // Regression: a stale result made every later ballot render as an
    // already-resolved result window and refuse votes.
    expect(notifier.state.dto.result, isNull);
    expect(notifier.state.dto.voteTarget, equals(-1));
  });

  test('a phase window starts a display-only countdown', () async {
    transport.emit('phase_started', <String, dynamic>{
      'phase': 'knowoff',
      'window_seconds': 20,
    });
    await _settle();

    final dto = notifier.state.dto;
    expect(dto.phaseWindow, equals(20));
    expect(dto.turnDeadline, isNotNull);
    expect(
      dto.turnDeadline!.difference(DateTime.now()).inSeconds,
      inInclusiveRange(18, 20),
    );
  });

  test('a phase with no window clears the countdown', () async {
    transport.emit('phase_started', <String, dynamic>{
      'phase': 'knowoff',
      'window_seconds': 20,
    });
    await _settle();
    expect(notifier.state.dto.turnDeadline, isNotNull);

    transport.emit('phase_started', <String, dynamic>{'phase': 'verdict'});
    await _settle();

    expect(notifier.state.dto.turnDeadline, isNull);
    expect(notifier.state.dto.phaseWindow, equals(0));
  });

  test('merging a phase payload keeps the chat feed and the countdown',
      () async {
    transport.emit('phase_started', <String, dynamic>{
      'phase': 'discussion',
      'window_seconds': 40,
    });
    transport.emit('quick_chat', <String, dynamic>{
      'from_seat': 2,
      'kind': 'chat',
      'phrase_id': 'suspect',
    });
    await _settle();
    expect(notifier.state.dto.chatEvents, hasLength(1));

    transport.emit('round_resolved', <String, dynamic>{'round': 2});
    await _settle();

    // Regression: _mergeState rebuilt the DTO from scratch and dropped every
    // locally-owned field.
    expect(notifier.state.dto.chatEvents, hasLength(1));
    expect(notifier.state.dto.turnDeadline, isNotNull);
    expect(notifier.state.dto.phaseWindow, equals(40));
  });

  test('the vote budget survives phases that omit it', () async {
    transport.emit('phase_started', <String, dynamic>{
      'phase': 'knowoff',
      'remaining_votes': 2,
    });
    await _settle();
    expect(notifier.state.dto.remainingVotes, equals(2));

    transport.emit('phase_started', <String, dynamic>{'phase': 'result'});
    await _settle();
    expect(notifier.state.dto.remainingVotes, equals(2));
  });

  test('copyWith can explicitly clear the result and the deadline', () {
    final dto = GameStateDto(
      result: const VoteResultDto(
        eliminatedSeat: 1,
        role: 'nower',
        tally: <String, int>{},
      ),
      turnDeadline: DateTime.now(),
    );

    expect(dto.copyWith().result, isNotNull);
    expect(dto.copyWith(clearResult: true).result, isNull);
    expect(dto.copyWith(clearTurnDeadline: true).turnDeadline, isNull);
  });

  test('casting a vote locks the ballot locally and sends target_seat',
      () async {
    transport.emit('phase_started', <String, dynamic>{
      'phase': 'knowoff',
      'window_seconds': 20,
    });
    await _settle();

    await notifier.castVote(2);

    expect(notifier.state.dto.voteTarget, equals(2));
    expect(transport.sent.single['kind'], equals('cast_vote'));
    expect(
      (transport.sent.single['payload'] as Map<String, dynamic>)['target_seat'],
      equals(2),
    );
  });

  test('a rejected ballot releases the local lock', () async {
    transport.emit('phase_started', <String, dynamic>{
      'phase': 'knowoff',
      'window_seconds': 20,
    });
    await _settle();
    await notifier.castVote(2);
    expect(notifier.state.dto.voteTarget, equals(2));

    transport.emit('error', <String, dynamic>{'code': 'already_voted'});
    await _settle();

    expect(notifier.state.dto.voteTarget, equals(-1));
    expect(notifier.state.lastError, equals('already_voted'));
  });

  test('an error outside the voting phases leaves the ballot alone', () async {
    transport.emit('phase_started', <String, dynamic>{
      'phase': 'knowoff',
      'window_seconds': 20,
    });
    await _settle();
    await notifier.castVote(2);

    transport.emit('phase_started', <String, dynamic>{'phase': 'result'});
    await _settle();
    transport.emit('error', <String, dynamic>{'code': 'poke_cap'});
    await _settle();

    expect(notifier.state.dto.voteTarget, equals(2));
  });
}
