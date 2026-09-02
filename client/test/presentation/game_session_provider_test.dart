import 'dart:async';

import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/core/config/app_config.dart';
import 'package:knowoff_client/core/config/client_config.dart';
import 'package:knowoff_client/core/network/game_transport.dart' as gt;
import 'package:knowoff_client/data/auth_service.dart';
import 'package:knowoff_client/data/models/game_state_dto.dart';
import 'package:knowoff_client/domain/entities/game_session.dart';
import 'package:knowoff_client/presentation/state/game_session_provider.dart';

class _StubAuthService extends AuthService {
  _StubAuthService() : super(baseUrl: 'http://test');

  int ensureCalls = 0;
  String? _token = 'expired-token';

  @override
  String? get accessToken => _token;

  @override
  Future<void> ensureSession() async {
    ensureCalls++;
    _token = 'fresh-token';
  }
}

class _FakeTransport implements gt.GameTransport {
  final StreamController<Map<String, dynamic>> _controller =
      StreamController<Map<String, dynamic>>.broadcast();
  final StreamController<gt.ConnectionState> _stateController =
      StreamController<gt.ConnectionState>.broadcast();
  final List<Map<String, dynamic>> sent = <Map<String, dynamic>>[];
  bool failNextSend = false;

  void emit(String kind, Map<String, dynamic> payload) {
    _controller.add(<String, dynamic>{'kind': kind, 'payload': payload});
  }

  void emitState(gt.ConnectionState state) => _stateController.add(state);

  @override
  Stream<Map<String, dynamic>> get messages => _controller.stream;

  @override
  Stream<gt.ConnectionState> get state => _stateController.stream;

  @override
  bool get isConnected => true;

  int reconnectCount = 0;

  @override
  Future<void> close() async {
    await _controller.close();
    await _stateController.close();
  }

  @override
  Future<void> connect() async {}

  @override
  Future<void> reconnect() async => reconnectCount++;

  @override
  Future<void> send(Map<String, dynamic> message) async {
    if (failNextSend) {
      failNextSend = false;
      throw StateError('send failed');
    }
    sent.add(message);
  }
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

  test('public specialty use is retained for announcement surfaces', () async {
    transport.emit('specialty_used', <String, dynamic>{
      'seat': 2,
      'specialty': 'reveal',
    });
    await _settle();

    expect(notifier.state.specialtyAnnouncementSeat, equals(2));
    expect(notifier.state.specialtyAnnouncement, equals('reveal'));
  });

  test('public Ready events retain every ready seat for the current phase',
      () async {
    transport.emit('phase_started', <String, dynamic>{'phase': 'discussion'});
    transport.emit('ready_state', <String, dynamic>{
      'phase': 'discussion',
      'seat': 2,
    });
    await _settle();

    expect(notifier.state.dto.readySeats, equals(<int>[2]));
  });

  test('draw events add cards to the local hand without recording a play',
      () async {
    final drawTransport = _FakeTransport();
    final drawNotifier = GameSessionNotifier(
      transport: drawTransport,
      initialState: const GameSession(
        dto: GameStateDto(
          seat: 0,
          hand: HandDto(
            cards: [CardDto(id: 'existing', type: 'text')],
            drawPile: [CardDto(id: 'drawn-card', type: 'text')],
            specialty: null,
          ),
        ),
      ),
    );
    drawTransport.emit('play_revealed', <String, dynamic>{
      'seat': 0,
      'draw': 1,
      'cards': <Map<String, dynamic>>[
        <String, dynamic>{
          'id': 'drawn-card',
          'type': 'text',
          'content': 'Drawn'
        },
      ],
    });
    await _settle();

    expect(
      drawNotifier.state.dto.hand.cards.any((card) => card.id == 'drawn-card'),
      isTrue,
    );
    expect(drawNotifier.state.dto.plays.containsKey('0'), isFalse);
    drawNotifier.dispose();
    await drawTransport.close();
  });

  test('draw events retain a public drawer and count announcement', () async {
    final drawTransport = _FakeTransport();
    final drawNotifier = GameSessionNotifier(
      transport: drawTransport,
      initialState: const GameSession(dto: GameStateDto(seat: 0)),
    );

    drawTransport.emit('play_revealed', <String, dynamic>{
      'seat': 2,
      'draw': 2,
      'cards': <Map<String, dynamic>>[],
    });
    await _settle();

    expect(drawNotifier.state.drawAnnouncementSeat, equals(2));
    expect(drawNotifier.state.drawAnnouncementCount, equals(2));
    expect(drawNotifier.state.drawAnnouncementId, equals(1));

    drawNotifier.dispose();
    await drawTransport.close();
  });

  test('drawing cancels a pending auto-play selection', () async {
    final drawTransport = _FakeTransport();
    final drawNotifier = GameSessionNotifier(
      transport: drawTransport,
      initialState: const GameSession(
        dto: GameStateDto(seat: 0),
      ),
    );
    drawNotifier.lockMove('stale-card');

    await drawNotifier.drawCards(1);

    expect(drawNotifier.state.selectedCardId, isNull);
    expect(drawNotifier.state.moveLocked, isFalse);
    expect(drawTransport.sent.single['kind'], equals('draw_cards'));

    drawNotifier.dispose();
    await drawTransport.close();
  });

  test('drawing removes a queued auto-play request', () async {
    final drawTransport = _FakeTransport()..failNextSend = true;
    final drawNotifier = GameSessionNotifier(
      transport: drawTransport,
      initialState: const GameSession(dto: GameStateDto(seat: 0)),
    );
    drawTransport.emit('phase_started', <String, dynamic>{'phase': 'play'});
    await _settle();
    drawNotifier.lockMove('stale-card');

    drawTransport.emit('turn_started', <String, dynamic>{
      'turn_seat': 0,
      'round': 1,
      'timeout': 20,
    });
    await _settle();
    await drawNotifier.drawCards(1);

    drawTransport.emitState(gt.ConnectionState.connected);
    await Future<void>.delayed(const Duration(milliseconds: 250));

    expect(
      drawTransport.sent.map((message) => message['kind']),
      equals(<String>['draw_cards']),
    );

    drawNotifier.dispose();
    await drawTransport.close();
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

  test('a next-round phase clears the finalized result snapshot', () async {
    transport.emit('knowoff_resolved', <String, dynamic>{
      'result': <String, dynamic>{
        'eliminated_seat': 1,
        'tally': <String, dynamic>{'1': 2},
      },
    });
    await _settle();
    expect(notifier.state.dto.result, isNotNull);

    transport.emit('phase_started', <String, dynamic>{'phase': 'play'});
    await _settle();

    expect(notifier.state.dto.phase, equals('play'));
    expect(notifier.state.dto.result, isNull);
  });

  test('a ready_ack flips the local resultReady flag', () async {
    transport.emit('knowoff_resolved', <String, dynamic>{
      'result': <String, dynamic>{
        'eliminated_seat': 1,
        'tally': <String, dynamic>{'1': 2},
      },
    });
    await _settle();
    expect(notifier.state.dto.resultReady, isFalse);

    await notifier.ready();
    transport.emit('ready_ack', <String, dynamic>{'result_ready': true});
    await _settle();

    expect(notifier.state.dto.resultReady, isTrue);
  });

  test('a resolved ballot retains each voter target', () async {
    transport.emit('knowoff_resolved', <String, dynamic>{
      'votes': <String, dynamic>{'0': 1, '1': 2, '2': -1},
      'result': <String, dynamic>{
        'eliminated_seat': 1,
        'tally': <String, dynamic>{'1': 1, '2': 1},
      },
    });
    await _settle();

    expect(
        notifier.state.dto.result?.votes,
        equals(<String, int>{
          '0': 1,
          '1': 2,
          '2': -1,
        }));
  });

  test('a fresh result window clears a stale resultReady', () async {
    transport.emit('knowoff_resolved', <String, dynamic>{
      'result': <String, dynamic>{
        'eliminated_seat': 1,
        'tally': <String, dynamic>{'1': 2},
      },
    });
    transport.emit('ready_ack', <String, dynamic>{'result_ready': true});
    await _settle();
    expect(notifier.state.dto.resultReady, isTrue);

    // Regression: a stale resultReady would let a single seat's earlier
    // Ready silently finalize the very next round's result window too.
    transport.emit('knowoff_resolved', <String, dynamic>{
      'result': <String, dynamic>{
        'eliminated_seat': 2,
        'tally': <String, dynamic>{'2': 2},
      },
    });
    await _settle();

    expect(notifier.state.dto.resultReady, isFalse);
  });

  test('a finalized elimination stores the revealed role for announcement',
      () async {
    transport.emit('elimination_finalized', <String, dynamic>{
      'eliminated_seat': 1,
      'role': 'donower',
    });
    await _settle();

    expect(notifier.state.finalEliminatedSeat, equals(1));
    expect(notifier.state.finalEliminatedRole, equals('donower'));

    transport.emit('phase_started', <String, dynamic>{'phase': 'play'});
    await _settle();

    expect(notifier.state.dto.phase, equals('play'));
    expect(notifier.state.finalEliminatedSeat, equals(1));
    expect(notifier.state.finalEliminatedRole, equals('donower'));
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

  test('a live vote_cast event updates the live ballots map', () async {
    transport.emit('phase_started', <String, dynamic>{
      'phase': 'knowoff',
      'window_seconds': 20,
    });
    await _settle();

    transport.emit('vote_cast', <String, dynamic>{'seat': 0, 'target_seat': 2});
    await _settle();
    expect(notifier.state.dto.liveBallots, equals(<String, int>{'0': 2}));

    // Changing a mind overwrites the seat's live target.
    transport.emit('vote_cast', <String, dynamic>{'seat': 0, 'target_seat': 3});
    await _settle();
    expect(notifier.state.dto.liveBallots, equals(<String, int>{'0': 3}));

    // A fresh ballot window wipes the live view clean.
    transport.emit('phase_started', <String, dynamic>{
      'phase': 'runoff',
      'window_seconds': 15,
    });
    await _settle();
    expect(notifier.state.dto.liveBallots, isEmpty);
  });

  test('a rejected cast_vote releases the local lock', () async {
    transport.emit('phase_started', <String, dynamic>{
      'phase': 'knowoff',
      'window_seconds': 20,
    });
    await _settle();
    await notifier.castVote(2);
    expect(notifier.state.dto.voteTarget, equals(2));

    transport.emit('error', <String, dynamic>{
      'code': 'rejected',
      'params': <String, dynamic>{'reply_to': 'cast_vote'},
    });
    await _settle();

    expect(notifier.state.dto.voteTarget, equals(-1));
    expect(notifier.state.lastError, equals('rejected'));
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

  test(
      'a rejection of an unrelated intent during the voting phase does not '
      'unlock an already-cast ballot', () async {
    // Regression test: a stale/queued request (e.g. a "ready" that only
    // reaches the server after Knowoff opens) can come back rejected while
    // the player is sitting on a ballot that was already accepted. Only a
    // rejection replying to cast_vote itself may release the local lock —
    // otherwise the row flips back to unlocked and the player has to tap
    // Vote a second time even though their first vote already landed.
    transport.emit('phase_started', <String, dynamic>{
      'phase': 'knowoff',
      'window_seconds': 20,
    });
    await _settle();
    await notifier.castVote(2);
    expect(notifier.state.dto.voteTarget, equals(2));

    transport.emit('error', <String, dynamic>{
      'code': 'rejected',
      'params': <String, dynamic>{'reply_to': 'ready'},
    });
    await _settle();

    expect(notifier.state.dto.voteTarget, equals(2));
  });

  test('the queue handshake refreshes the session before sending its token',
      () async {
    // Regression: the socket sent whatever token was in memory, so an idle
    // tab handshaked with an expired one. The server then dropped the
    // connection to an anonymous account and rejected the join as a
    // quickplay-limit failure.
    final auth = _StubAuthService();
    await AppConfig.initialize(ClientConfig.defaultConfig(), auth);
    final before = auth.ensureCalls;

    await notifier.queueQuickPlay(4);

    expect(auth.ensureCalls, equals(before + 1));
    expect(transport.sent.single['kind'], equals('queue_quickplay'));
    expect(
      (transport.sent.single['payload']
          as Map<String, dynamic>)['access_token'],
      equals('fresh-token'),
    );
  });

  test('reclaims the room after a dropped authenticated socket', () async {
    final auth = _StubAuthService();
    await AppConfig.initialize(ClientConfig.defaultConfig(), auth);
    transport.emit('joined', <String, dynamic>{
      'seat': 0,
      'code': 'ABC123',
      'session_token': 'reclaim-token',
    });
    await _settle();

    transport.emitState(gt.ConnectionState.disconnected);
    transport.emitState(gt.ConnectionState.connected);
    await _settle();
    await _settle();

    expect(transport.sent, hasLength(1));
    expect(transport.sent.single['kind'], 'join_room');
    expect(transport.sent.single['payload'], <String, dynamic>{
      'code': 'ABC123',
      'session_token': 'reclaim-token',
      'access_token': 'fresh-token',
    });
  });

  test('lockMove marks the pending card locked without sending anything',
      () async {
    transport.emit('joined', <String, dynamic>{'seat': 0});
    transport.emit('phase_started', <String, dynamic>{'phase': 'play'});
    await _settle();

    notifier.lockMove('card-1');

    expect(notifier.state.selectedCardId, equals('card-1'));
    expect(notifier.state.moveLocked, isTrue);
    expect(transport.sent, isEmpty);
  });

  test('clearSelection drops the pending card and its lock without sending',
      () async {
    transport.emit('joined', <String, dynamic>{'seat': 0});
    transport.emit('phase_started', <String, dynamic>{'phase': 'play'});
    await _settle();
    notifier.lockMove('card-1');

    notifier.clearSelection();

    expect(notifier.state.selectedCardId, isNull);
    expect(notifier.state.moveLocked, isFalse);
    expect(transport.sent, isEmpty);
  });

  test("a locked move auto-plays the instant this seat's turn starts",
      () async {
    // Regression: picking a card during someone else's turn used to require
    // sitting through the wait and re-confirming once your turn arrived.
    transport.emit('joined', <String, dynamic>{'seat': 0});
    transport.emit('phase_started', <String, dynamic>{'phase': 'play'});
    await _settle();
    notifier.lockMove('card-1');

    transport.emit('turn_started', <String, dynamic>{
      'turn_seat': 0,
      'round': 1,
      'timeout': 7,
    });
    await _settle();

    expect(notifier.state.moveLocked, isFalse);
    expect(transport.sent.single['kind'], equals('play_card'));
    expect(
      (transport.sent.single['payload'] as Map<String, dynamic>)['card_id'],
      equals('card-1'),
    );
  });

  test('turn_started for another seat does not fire a locked move', () async {
    transport.emit('joined', <String, dynamic>{'seat': 0});
    transport.emit('phase_started', <String, dynamic>{'phase': 'play'});
    await _settle();
    notifier.lockMove('card-1');

    transport.emit('turn_started', <String, dynamic>{
      'turn_seat': 1,
      'round': 1,
      'timeout': 7,
    });
    await _settle();

    expect(notifier.state.moveLocked, isTrue);
    expect(transport.sent, isEmpty);
  });

  test('a turn timeout removes the auto-discarded card from my own hand',
      () async {
    // Regression: play_revealed for a timeout carries no card_id (the seat
    // never played), so the old merge silently ignored it — the seat's play
    // never landed in `plays` and the auto-discarded card stayed visible in
    // the local hand even though the server had already removed it.
    transport.emit('joined', <String, dynamic>{'seat': 0});
    transport.emit('phase_started', <String, dynamic>{'phase': 'play'});
    transport.emit('hand_dealt', <String, dynamic>{
      'cards': <String>['card-1', 'card-2'],
      'draw_pile': <String>[],
      'specialty': null,
    });
    await _settle();

    transport.emit('play_revealed', <String, dynamic>{
      'seat': 0,
      'timeout': true,
      'lost': <String, dynamic>{'id': 'card-1', 'type': 'text'},
    });
    await _settle();

    expect(
      notifier.state.dto.hand.cards.map((c) => c.id),
      equals(<String>['card-2']),
    );
    final play = notifier.state.dto.plays['0'];
    expect(play, isNotNull);
    expect(play!.timedOut, isTrue);
    expect(play.id, equals('card-1'));
  });

  test('a turn timeout for another seat still marks that seat as played',
      () async {
    transport.emit('joined', <String, dynamic>{'seat': 0});
    transport.emit('phase_started', <String, dynamic>{'phase': 'play'});
    await _settle();

    transport.emit('play_revealed', <String, dynamic>{
      'seat': 1,
      'timeout': true,
    });
    await _settle();

    expect(notifier.state.dto.plays['1'], isNotNull);
  });

  test('a new round clears a stale pre-selection', () async {
    transport.emit('joined', <String, dynamic>{'seat': 0});
    transport.emit('phase_started', <String, dynamic>{'phase': 'play'});
    await _settle();
    notifier.lockMove('card-1');

    transport.emit('round_started', <String, dynamic>{
      'round': 2,
      'turn_order': <int>[1, 0],
    });
    await _settle();

    expect(notifier.state.moveLocked, isFalse);
    expect(notifier.state.selectedCardId, isNull);
  });
}
