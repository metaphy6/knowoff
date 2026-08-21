import 'package:flutter/foundation.dart';

/// Immutable DTOs mirroring the server wire format for the realtime game loop.

@immutable
class PlayerDto {
  const PlayerDto({
    required this.seat,
    required this.name,
    required this.connected,
    required this.eliminated,
    this.role,
  });

  final int seat;
  final String name;
  final bool connected;
  final bool eliminated;
  final String? role;

  factory PlayerDto.fromJson(Map<String, dynamic> json) {
    return PlayerDto(
      seat: json['seat'] as int? ?? 0,
      name: json['name'] as String? ?? '',
      connected: json['connected'] as bool? ?? true,
      eliminated: json['eliminated'] as bool? ?? false,
      role: json['role'] as String?,
    );
  }
}

@immutable
class CardDto {
  const CardDto({required this.id, required this.type});

  final String id;
  final String type;

  factory CardDto.fromJson(Map<String, dynamic> json) {
    return CardDto(
      id: json['id'] as String? ?? '',
      type: json['type'] as String? ?? 'text',
    );
  }
}

@immutable
class NownRefDto {
  const NownRefDto({
    required this.id,
    required this.type,
    this.signedUrl,
    this.content,
  });

  final String id;
  final String type;
  final String? signedUrl;
  final String? content;

  factory NownRefDto.fromJson(Map<String, dynamic> json) {
    return NownRefDto(
      id: json['id'] as String? ?? '',
      type: json['type'] as String? ?? 'text',
      signedUrl: json['signed_url'] as String?,
      content: json['content'] as String?,
    );
  }
}

@immutable
class HandDto {
  const HandDto({
    required this.cards,
    required this.drawPile,
    required this.specialty,
  });

  final List<CardDto> cards;
  final List<CardDto> drawPile;
  final String? specialty;

  factory HandDto.fromJson(Map<String, dynamic> json) {
    return HandDto(
      cards: cardList(json['cards']),
      drawPile: cardList(json['draw_pile']),
      specialty: json['specialty'] as String?,
    );
  }
}

List<CardDto> cardList(dynamic value) {
  if (value is! List<dynamic>) return const [];
  return value.map((item) {
    if (item is String) {
      return CardDto(id: item, type: 'text');
    }
    if (item is Map<String, dynamic>) {
      return CardDto.fromJson(item);
    }
    return const CardDto(id: '', type: 'text');
  }).toList();
}

@immutable
class GameStateDto {
  const GameStateDto({
    this.phase = 'waiting',
    this.round = 0,
    this.seat = -1,
    this.remainingVotes = 0,
    this.players = const [],
    this.hand = const HandDto(cards: [], drawPile: [], specialty: null),
    this.nown,
    this.decoy = false,
    this.turnSeat = -1,
    this.plays = const {},
    this.discussionReady = false,
    this.voteTarget = -1,
    this.result,
    this.winner,
    this.nowns = const [],
    this.log = const [],
    this.matchPoints = 0,
    this.roomCode = '',
  });

  final String phase;
  final int round;
  final int seat;
  final int remainingVotes;
  final List<PlayerDto> players;
  final HandDto hand;
  final NownRefDto? nown;
  final bool decoy;
  final int turnSeat;
  final Map<String, String> plays;
  final bool discussionReady;
  final int voteTarget;
  final VoteResultDto? result;
  final String? winner;
  final List<NownRefDto> nowns;
  final List<String> log;
  final int matchPoints;
  final String roomCode;

  factory GameStateDto.fromJson(Map<String, dynamic> json) {
    return GameStateDto(
      phase: json['phase'] as String? ?? 'waiting',
      round: json['round'] as int? ?? 0,
      seat: json['seat'] as int? ?? -1,
      remainingVotes: json['remaining_votes'] as int? ?? 0,
      players: playerList(json['players']),
      hand:
          HandDto.fromJson((json['hand'] as Map<String, dynamic>?) ?? const {}),
      nown: json['nown'] == null
          ? null
          : NownRefDto.fromJson(json['nown'] as Map<String, dynamic>),
      decoy: json['decoy'] as bool? ?? false,
      turnSeat: json['turn_seat'] as int? ?? -1,
      plays: stringMap(json['plays']),
      discussionReady: json['discussion_ready'] as bool? ?? false,
      voteTarget: json['vote_target'] as int? ?? -1,
      result: json['result'] == null
          ? null
          : VoteResultDto.fromJson(json['result'] as Map<String, dynamic>),
      winner: json['winner'] as String?,
      nowns: nownList(json['nowns']),
      log: stringList(json['log']),
      matchPoints: json['match_points'] as int? ?? 0,
      roomCode: json['room_code'] as String? ?? '',
    );
  }

  GameStateDto copyWith({
    String? phase,
    int? round,
    int? seat,
    int? remainingVotes,
    List<PlayerDto>? players,
    HandDto? hand,
    NownRefDto? nown,
    bool? decoy,
    int? turnSeat,
    Map<String, String>? plays,
    bool? discussionReady,
    int? voteTarget,
    VoteResultDto? result,
    String? winner,
    List<NownRefDto>? nowns,
    List<String>? log,
    int? matchPoints,
    String? roomCode,
  }) {
    return GameStateDto(
      phase: phase ?? this.phase,
      round: round ?? this.round,
      seat: seat ?? this.seat,
      remainingVotes: remainingVotes ?? this.remainingVotes,
      players: players ?? this.players,
      hand: hand ?? this.hand,
      nown: nown ?? this.nown,
      decoy: decoy ?? this.decoy,
      turnSeat: turnSeat ?? this.turnSeat,
      plays: plays ?? this.plays,
      discussionReady: discussionReady ?? this.discussionReady,
      voteTarget: voteTarget ?? this.voteTarget,
      result: result ?? this.result,
      winner: winner ?? this.winner,
      nowns: nowns ?? this.nowns,
      log: log ?? this.log,
      matchPoints: matchPoints ?? this.matchPoints,
      roomCode: roomCode ?? this.roomCode,
    );
  }
}

@immutable
class VoteResultDto {
  const VoteResultDto({
    required this.eliminatedSeat,
    required this.role,
    required this.tally,
  });

  final int eliminatedSeat;
  final String? role;
  final Map<String, int> tally;

  factory VoteResultDto.fromJson(Map<String, dynamic> json) {
    return VoteResultDto(
      eliminatedSeat: json['eliminated_seat'] as int? ?? -1,
      role: json['role'] as String?,
      tally: (json['tally'] as Map<String, dynamic>? ?? const {})
          .map((k, v) => MapEntry(k, v as int)),
    );
  }
}

List<PlayerDto> playerList(dynamic value) {
  if (value is! List<dynamic>) return const [];
  return value
      .whereType<Map<String, dynamic>>()
      .map(PlayerDto.fromJson)
      .toList();
}

Map<String, String> stringMap(dynamic value) {
  if (value is! Map<String, dynamic>) return const {};
  return value.map((k, v) => MapEntry(k, v.toString()));
}

List<NownRefDto> nownList(dynamic value) {
  if (value is! List<dynamic>) return const [];
  return value
      .whereType<Map<String, dynamic>>()
      .map(NownRefDto.fromJson)
      .toList();
}

List<String> stringList(dynamic value) {
  if (value is! List<dynamic>) return const [];
  return value.whereType<String>().toList();
}
