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

  static GameStateDto _initialDto() => const GameStateDto();

  void _onMessage(Map<String, dynamic> message) {
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
      case 'round_started':
      case 'turn_started':
        _mergeState(payload);
        break;
      case 'play_revealed':
        _mergePlayRevealed(payload);
        break;
      case 'round_resolved':
      case 'shuffle_occurred':
      case 'vote_result_pending':
      case 'vote_nullified':
      case 'knowoff_resolved':
      case 'quick_chat':
        _mergeState(payload);
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
        state = state.copyWith(lastError: code);
        break;
      case 'system_notice':
      default:
        break;
    }
  }

  void _mergePlayRevealed(Map<String, dynamic> payload) {
    final seat = payload['seat'] as int?;
    final cardId = payload['card_id'] as String?;
    if (seat == null || cardId == null) return;
    final current = state.dto;
    final plays = Map<String, String>.from(current.plays);
    plays[seat.toString()] = cardId;
    state = state.copyWith(
      dto: current.copyWith(plays: plays),
      lastError: null,
    );
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
          ? stringMap(payload['plays'])
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

  Future<void> useSpecialty(String specialty, {String? discardCardId}) =>
      _send('use_specialty', {
        'specialty': specialty,
        if (discardCardId != null) 'discard_card_id': discardCardId,
      });

  Future<void> drawCards(int count) => _send('draw_cards', {'count': count});

  Future<void> castVote(int targetSeat) =>
      _send('cast_vote', {'target_seat': targetSeat});

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
