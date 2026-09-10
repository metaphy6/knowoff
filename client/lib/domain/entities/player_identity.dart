import '../../data/models/game_state_dto.dart';

/// Compatibility with server payloads predating the explicit bot flag.
const String kBotNicknamePrefix = 'Bot_';

bool isBotSeat(PlayerDto player) =>
    player.bot || player.name.startsWith(kBotNicknamePrefix);

String seatDisplayName(PlayerDto player) {
  if (isBotSeat(player)) return 'Bot ${player.seat}';
  if (player.name.isEmpty) return 'P${player.seat}';
  return player.name;
}
