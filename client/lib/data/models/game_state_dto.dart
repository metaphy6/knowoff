import 'package:flutter/foundation.dart';

/// Immutable DTOs mirroring the server wire format for the realtime game loop.

@immutable
class PlayerDto {
  const PlayerDto({
    required this.seat,
    required this.name,
    required this.connected,
    required this.eliminated,
    this.avatar = '',
    this.bot = false,
    this.accountId = '',
    this.role,
  });

  final int seat;
  final String name;
  final bool connected;
  final bool eliminated;

  /// Preset avatar id from the server's curated gallery, or empty.
  final String avatar;

  /// Server-declared backfill bot. Rules §1: a bot seat is never disguised.
  final bool bot;

  /// Pseudonymous account id, used to open the seat's public profile and to
  /// target a conduct report. Empty for bots.
  final String accountId;
  final String? role;

  factory PlayerDto.fromJson(Map<String, dynamic> json) {
    return PlayerDto(
      seat: json['seat'] as int? ?? 0,
      name: json['name'] as String? ?? '',
      connected: json['connected'] as bool? ?? true,
      eliminated: json['eliminated'] as bool? ?? false,
      avatar: json['avatar'] as String? ?? '',
      bot: json['bot'] as bool? ?? false,
      accountId: json['account_id'] as String? ?? '',
      role: json['role'] as String?,
    );
  }
}

@immutable
class CardDto {
  const CardDto({
    required this.id,
    required this.type,
    this.content,
    this.signedUrl,
    this.timedOut = false,
  });

  final String id;
  final String type;
  final String? content;
  final String? signedUrl;

  /// True when this play is a turn-timeout auto-pass showing the randomly
  /// discarded card (Rules §3), not an actual play.
  final bool timedOut;

  factory CardDto.fromJson(Map<String, dynamic> json) {
    return CardDto(
      id: json['id'] as String? ?? '',
      type: json['type'] as String? ?? 'text',
      content: json['content'] as String?,
      signedUrl: json['signed_url'] as String?,
      timedOut: json['timed_out'] as bool? ?? false,
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
class ChatEventDto {
  const ChatEventDto({
    required this.kind,
    required this.fromSeat,
    this.phraseId,
    this.text,
    this.targetSeat,
  });

  final String kind;
  final int fromSeat;
  final String? phraseId;
  final String? text;
  final int? targetSeat;
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
    this.resultReady = false,
    this.ballotReady = false,
    this.voteTarget = -1,
    this.result,
    this.winner,
    this.donowerSeats = const [],
    this.nowns = const [],
    this.log = const [],
    this.matchPoints = 0,
    this.roomCode = '',
    this.turnDeadline,
    this.phaseWindow = 0,
    this.chatEvents = const [],
    this.liveBallots = const {},
    this.readySeats = const [],
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
  final Map<String, CardDto> plays;
  final bool discussionReady;

  /// True once this seat has marked Ready during the post-ballot Revote
  /// window (Rules §4) — skips the wait once everyone agrees to finalize.
  final bool resultReady;

  /// True once this seat has marked the current ballot ready to resolve.
  final bool ballotReady;
  final int voteTarget;
  final VoteResultDto? result;
  final String? winner;
  final List<int> donowerSeats;
  final List<NownRefDto> nowns;
  final List<String> log;
  final int matchPoints;
  final String roomCode;
  final DateTime? turnDeadline;

  /// Wall-clock length of the current server-driven window, in seconds.
  /// Display-only: the server owns the phase clock.
  final int phaseWindow;
  final List<ChatEventDto> chatEvents;

  /// Live seat->target ballot, filled in from `vote_cast` events while the
  /// Knowoff/runoff window is open (Rules §4: the open ballot). Cleared the
  /// moment a fresh ballot starts.
  final Map<String, int> liveBallots;
  final List<int> readySeats;

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
      plays: cardMap(json['plays']),
      discussionReady: json['discussion_ready'] as bool? ?? false,
      resultReady: json['result_ready'] as bool? ?? false,
      ballotReady: json['ballot_ready'] as bool? ?? false,
      voteTarget: json['vote_target'] as int? ?? -1,
      result: json['result'] == null
          ? null
          : VoteResultDto.fromJson(json['result'] as Map<String, dynamic>),
      winner: json['winner'] as String?,
      donowerSeats: intList(json['donower_seats']),
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
    Map<String, CardDto>? plays,
    bool? discussionReady,
    bool? resultReady,
    bool? ballotReady,
    int? voteTarget,
    VoteResultDto? result,
    String? winner,
    List<int>? donowerSeats,
    List<NownRefDto>? nowns,
    List<String>? log,
    int? matchPoints,
    String? roomCode,
    DateTime? turnDeadline,
    int? phaseWindow,
    List<ChatEventDto>? chatEvents,
    Map<String, int>? liveBallots,
    List<int>? readySeats,
    bool clearResult = false,
    bool clearTurnDeadline = false,
    bool clearLiveBallots = false,
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
      resultReady: resultReady ?? this.resultReady,
      ballotReady: ballotReady ?? this.ballotReady,
      voteTarget: voteTarget ?? this.voteTarget,
      result: clearResult ? null : (result ?? this.result),
      winner: winner ?? this.winner,
      donowerSeats: donowerSeats ?? this.donowerSeats,
      nowns: nowns ?? this.nowns,
      log: log ?? this.log,
      matchPoints: matchPoints ?? this.matchPoints,
      roomCode: roomCode ?? this.roomCode,
      turnDeadline:
          clearTurnDeadline ? null : (turnDeadline ?? this.turnDeadline),
      phaseWindow: phaseWindow ?? this.phaseWindow,
      chatEvents: chatEvents ?? this.chatEvents,
      liveBallots:
          clearLiveBallots ? const {} : (liveBallots ?? this.liveBallots),
      readySeats: readySeats ?? this.readySeats,
    );
  }
}

@immutable
class VoteResultDto {
  const VoteResultDto({
    required this.eliminatedSeat,
    required this.role,
    required this.tally,
    this.votes = const {},
  });

  final int eliminatedSeat;
  final String? role;
  final Map<String, int> tally;

  /// Per-voter targets, revealed only after the server closes the ballot.
  /// A negative target represents an abstention.
  final Map<String, int> votes;

  factory VoteResultDto.fromJson(Map<String, dynamic> json) {
    return VoteResultDto(
      eliminatedSeat: json['eliminated_seat'] as int? ?? -1,
      role: json['role'] as String?,
      tally: (json['tally'] as Map<String, dynamic>? ?? const {})
          .map((k, v) => MapEntry(k, v as int)),
      votes: (json['votes'] as Map<String, dynamic>? ?? const {})
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

Map<String, CardDto> cardMap(dynamic value) {
  if (value is! Map<String, dynamic>) return const {};
  return value.map((k, v) {
    if (v is Map<String, dynamic>) {
      return MapEntry(k, CardDto.fromJson(v));
    }
    return MapEntry(k, CardDto(id: v.toString(), type: 'text'));
  });
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

List<int> intList(dynamic value) {
  if (value is! List<dynamic>) return const [];
  return value.whereType<num>().map((item) => item.toInt()).toList();
}
