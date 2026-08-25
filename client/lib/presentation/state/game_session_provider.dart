import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/config/app_config.dart';
import '../../core/network/game_transport.dart';
import '../../data/models/game_state_dto.dart';
import '../../domain/entities/game_session.dart';

/// Provides the live [GameSession] state for the current match.
final gameSessionProvider =
    StateNotifierProvider<GameSessionNotifier, GameSession>((ref) {
  throw UnimplementedError('override gameSessionProvider in ProviderScope');
});

/// Notifier that owns the [GameTransport] subscription and maps server events
/// to a local [GameSession] state.
class GameSessionNotifier extends StateNotifier<GameSession> {
  GameSessionNotifier({
    required GameTransport transport,
    GameSession? initialState,
  })  : _transport = transport,
        super(initialState ?? GameSession(dto: _initialDto())) {
    _subscription = transport.messages.listen(_onMessage);
    transport.connect();
  }

  final GameTransport _transport;
  StreamSubscription<Map<String, dynamic>>? _subscription;
  final List<Map<String, dynamic>> _bufferedMessages = [];

  static GameStateDto _initialDto() => const GameStateDto();

  /// Developer-only freeze toggle: while frozen, incoming server events are
  /// buffered instead of applied, so the current screen stops advancing for
  /// UI/UX inspection. Unfreezing replays the buffered events in order.
  void setFrozen(bool value) {
    if (state.frozen == value) return;
    state = state.copyWith(frozen: value);
    if (!value) {
      final pending = List<Map<String, dynamic>>.of(_bufferedMessages);
      _bufferedMessages.clear();
      for (final message in pending) {
        _onMessage(message);
      }
    }
  }

  /// Developer-only: discards any buffered/frozen state and resets the local
  /// session back to its initial (pre-match) shape, mirroring what the
  /// screen looks like before a match is joined. This does not notify the
  /// server or close the transport — the next server event simply overwrites
  /// these defaults, same as after a real match ends.
  void restart() {
    _bufferedMessages.clear();
    state = GameSession(dto: _initialDto());
  }

  void _onMessage(Map<String, dynamic> message) {
    if (state.frozen) {
      _bufferedMessages.add(message);
      return;
    }
    final kind = message['kind'] as String?;
    final payload = (message['payload'] as Map<String, dynamic>?) ?? const {};

    switch (kind) {
      case 'joined':
      case 'joined_joined':
        _setDto(state.dto.copyWith(
          seat: payload['seat'] as int? ?? state.dto.seat,
          roomCode: payload['code'] as String? ?? state.dto.roomCode,
        ));
        break;
      case 'phase_started':
        _mergeState(payload);
        _applyPhaseWindow(payload);
        break;
      case 'turn_started':
        _mergeState(payload);
        _setTurnDeadline(payload);
        break;
      case 'round_started':
        _setDto(state.dto.copyWith(plays: const {}));
        _mergeState(payload);
        break;
      case 'play_revealed':
        _mergePlayRevealed(payload);
        break;
      case 'round_resolved':
      case 'shuffle_occurred':
      case 'vote_result_pending':
      case 'knowoff_resolved':
        _mergeState(payload);
        break;
      case 'vote_nullified':
        // A Revote cancels the shown result outright: it reveals nobody and
        // eliminates nobody (Rules §5).
        _mergeState(payload);
        _setDto(state.dto.copyWith(clearResult: true, voteTarget: -1));
        break;
      case 'quick_chat':
        _appendChatEvent(payload);
        break;
      case 'role_assigned':
        final role = payload['role'] as String?;
        state = state.copyWith(myRole: role);
        break;
      case 'hand_dealt':
        final hand = HandDto.fromJson(payload);
        _setDto(state.dto.copyWith(hand: hand));
        break;
      case 'match_verdict':
      case 'points_scored':
        _mergeState(payload);
        break;
      case 'error':
        final code = payload['code'] as String?;
        // A rejected ballot has to release the local lock, or the row stays
        // stamped for a vote the server never accepted. Scoped to the voting
        // phases so an unrelated error can't wipe a ballot that did land.
        final voting = state.phase == 'knowoff' || state.phase == 'runoff';
        state = state.copyWith(
          dto: voting ? state.dto.copyWith(voteTarget: -1) : state.dto,
          lastError: code,
        );
        break;
      case 'system_notice':
      default:
        break;
    }
  }

  void _mergePlayRevealed(Map<String, dynamic> payload) {
    final seat = payload['seat'] as int?;
    final cardId = payload['card_id'] as String?;
    if (seat != null &&
        payload.containsKey('specialty') &&
        seat == state.dto.seat) {
      _setDto(state.dto.copyWith(
        hand: HandDto(
          cards: state.dto.hand.cards,
          drawPile: state.dto.hand.drawPile,
          specialty: null,
        ),
      ));
    }
    if (seat == null || cardId == null) return;
    final cardPayload = payload['card'] as Map<String, dynamic>?;
    final card = cardPayload != null
        ? CardDto.fromJson(cardPayload)
        : CardDto(id: cardId, type: 'text');
    final current = state.dto;
    final plays = Map<String, CardDto>.from(current.plays);
    plays[seat.toString()] = card;
    state = state.copyWith(
      dto: current.copyWith(plays: plays),
      lastError: null,
    );
  }

  void _setTurnDeadline(Map<String, dynamic> payload) {
    final timeoutSeconds = payload['timeout'] as int?;
    if (timeoutSeconds == null) return;
    _setDto(state.dto.copyWith(
      turnDeadline: DateTime.now().add(Duration(seconds: timeoutSeconds)),
      phaseWindow: timeoutSeconds,
    ));
  }

  /// Starts the display-only clock for a server-driven phase window, and
  /// clears the previous ballot when a fresh one opens.
  ///
  /// Without the reset a finished Knowoff's result stayed in the DTO forever —
  /// every later ballot then rendered as an already-resolved result window and
  /// refused votes.
  void _applyPhaseWindow(Map<String, dynamic> payload) {
    final phase = payload['phase'] as String?;
    final window = payload['window_seconds'] as int?;
    final opensBallot = phase == 'knowoff' || phase == 'runoff';

    var dto = state.dto;
    if (opensBallot) {
      dto = dto.copyWith(voteTarget: -1, clearResult: true);
    }
    dto = window == null || window <= 0
        ? dto.copyWith(clearTurnDeadline: true, phaseWindow: 0)
        : dto.copyWith(
            turnDeadline: DateTime.now().add(Duration(seconds: window)),
            phaseWindow: window,
          );
    _setDto(dto);
  }

  void _appendChatEvent(Map<String, dynamic> payload) {
    final fromSeat = payload['from_seat'] as int?;
    if (fromSeat == null) return;
    final kind = payload['kind'] as String? ?? 'chat';
    final event = ChatEventDto(
      kind: kind,
      fromSeat: fromSeat,
      phraseId: payload['phrase_id'] as String?,
      targetSeat: payload['target_seat'] as int?,
    );
    final events = [...state.dto.chatEvents, event];
    final trimmed =
        events.length > 20 ? events.sublist(events.length - 20) : events;
    _setDto(state.dto.copyWith(chatEvents: trimmed));
  }

  void _mergeState(Map<String, dynamic> payload) {
    final current = state.dto;
    final updated = GameStateDto(
      phase: payload['phase'] as String? ?? current.phase,
      round: payload['round'] as int? ?? current.round,
      seat: payload['seat'] as int? ?? current.seat,
      remainingVotes:
          payload['remaining_votes'] as int? ?? current.remainingVotes,
      players: payload.containsKey('players')
          ? playerList(payload['players'])
          : current.players,
      hand: payload.containsKey('hand')
          ? HandDto.fromJson(payload['hand'] as Map<String, dynamic>)
          : current.hand,
      nown: payload.containsKey('nown')
          ? (payload['nown'] == null
              ? null
              : NownRefDto.fromJson(payload['nown'] as Map<String, dynamic>))
          : current.nown,
      decoy: payload['decoy'] as bool? ?? current.decoy,
      turnSeat: payload['turn_seat'] as int? ?? current.turnSeat,
      plays: payload.containsKey('plays')
          ? cardMap(payload['plays'])
          : current.plays,
      discussionReady:
          payload['discussion_ready'] as bool? ?? current.discussionReady,
      voteTarget: payload['vote_target'] as int? ?? current.voteTarget,
      result: payload.containsKey('result')
          ? (payload['result'] == null
              ? null
              : VoteResultDto.fromJson(
                  payload['result'] as Map<String, dynamic>))
          : current.result,
      winner: payload.containsKey('winner')
          ? payload['winner'] as String?
          : current.winner,
      nowns: payload.containsKey('nowns')
          ? nownList(payload['nowns'])
          : current.nowns,
      log:
          payload.containsKey('log') ? stringList(payload['log']) : current.log,
      matchPoints: payload['match_points'] as int? ?? current.matchPoints,
      roomCode: payload['room_code'] as String? ?? current.roomCode,
      // Locally-owned fields the wire never carries: a merge must not drop
      // them, or the countdown resets and the chat feed empties on every
      // phase change.
      turnDeadline: current.turnDeadline,
      phaseWindow: current.phaseWindow,
      chatEvents: current.chatEvents,
    );
    state = state.copyWith(dto: updated, lastError: null);
  }

  void _setDto(GameStateDto dto) {
    state = state.copyWith(dto: dto, lastError: null);
  }

  void selectCard(String cardId) {
    state = state.copyWith(selectedCardId: cardId);
  }

  static const _protocolVersion = 1;

  Future<void> _send(String kind, Map<String, dynamic> payload) async {
    await _transport.send(<String, dynamic>{
      'v': _protocolVersion,
      'kind': kind,
      'payload': payload,
    });
  }

  Future<void> queueQuickPlay(int size) => _send('queue_quickplay', {
        'size': size,
        'access_token': AppConfig.instance.authService.accessToken ?? '',
      });

  Future<void> joinRoom(String code) => _send('join_room', {
        'code': code,
        'access_token': AppConfig.instance.authService.accessToken ?? '',
      });

  Future<void> playCard(String cardId) =>
      _send('play_card', {'card_id': cardId});

  Future<void> useSpecialty(String specialty,
          {String? discardCardId, int? targetSeat}) =>
      _send('use_specialty', {
        'specialty': specialty,
        if (discardCardId != null) 'discard_card_id': discardCardId,
        if (targetSeat != null) 'target_seat': targetSeat,
      });

  Future<void> drawCards(int count) => _send('draw_cards', {'count': count});

  /// Sends the ballot and locks it locally.
  ///
  /// The vote stays blind to the rest of the table (Rules §4), so the server
  /// never echoes it back — without the local lock the voter gets no feedback
  /// at all and can tap every row in turn.
  Future<void> castVote(int targetSeat) async {
    _setDto(state.dto.copyWith(voteTarget: targetSeat));
    await _send('cast_vote', {'target_seat': targetSeat});
  }

  Future<void> ready() => _send('ready', const {});

  Future<void> poke(int targetSeat) =>
      _send('poke', {'target_seat': targetSeat});

  Future<void> quickChat(String phraseId) =>
      _send('quick_chat', {'phrase_id': phraseId});

  @override
  void dispose() {
    _subscription?.cancel();
    _transport.close();
    super.dispose();
  }
}
