import 'package:flutter/foundation.dart';

import '../../data/models/game_state_dto.dart';

/// Domain view of the current match session.
///
/// Mirrors [GameStateDto] but exposes helper getters for the UI.
@immutable
class GameSession {
  const GameSession({
    required this.dto,
    this.myRole,
    this.lastError,
    this.selectedCardId,
    this.frozen = false,
  });

  final GameStateDto dto;
  final String? myRole;
  final String? lastError;
  final String? selectedCardId;

  /// Developer-only: while true, incoming server events are buffered instead
  /// of applied, so the screen stops advancing for UI/UX inspection.
  final bool frozen;

  GameSession copyWith({
    GameStateDto? dto,
    String? myRole,
    String? lastError,
    String? selectedCardId,
    bool? frozen,
  }) {
    return GameSession(
      dto: dto ?? this.dto,
      myRole: myRole ?? this.myRole,
      lastError: lastError,
      selectedCardId: selectedCardId ?? this.selectedCardId,
      frozen: frozen ?? this.frozen,
    );
  }

  int get seat => dto.seat;
  String get phase => dto.phase;
  int get round => dto.round;

  PlayerDto? get me => playerBySeat(seat);

  bool get amEliminated => me?.eliminated ?? false;
  bool get amConnected => me?.connected ?? false;

  bool get isMyTurn => dto.turnSeat == seat && !amEliminated;

  bool get canAct => isMyTurn && (phase == 'play' || phase == 'role_reveal');

  bool canVoteFor(int targetSeat) {
    if (amEliminated) return false;
    if (targetSeat == seat) return false;
    final target = playerBySeat(targetSeat);
    if (target == null || target.eliminated) return false;
    return phase == 'knowoff' || phase == 'runoff';
  }

  bool get canReady =>
      !amEliminated &&
      (phase == 'discussion' || phase == 'role_reveal') &&
      !dto.discussionReady;

  PlayerDto? playerBySeat(int seat) {
    for (final p in dto.players) {
      if (p.seat == seat) return p;
    }
    return null;
  }

  List<PlayerDto> get activePlayers =>
      dto.players.where((p) => !p.eliminated).toList();

  List<PlayerDto> get nonReadyPlayers =>
      activePlayers.where((p) => p.seat != seat && !p.connected).toList();

  bool get isDonower => myRole == 'donower';
  bool get isNower => myRole == 'nower';

  bool get showNown => !amEliminated && (isNower || myRole == null);

  bool get showDecoy => amEliminated || isDonower;

  bool get hasResult => dto.result != null;

  bool get isOver => phase == 'verdict' || phase == 'finished';
}
