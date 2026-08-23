import 'package:flutter/material.dart';

import '../../data/models/game_state_dto.dart';
import '../../l10n/app_localizations.dart';
import '../icons/doodles.dart';
import '../theme/knowoff_tokens.dart';

/// Server-reserved backfill-bot nickname prefix (`server/internal/bots`).
/// A bot seat is always labeled — a disguised bot would be a scandal in a game
/// about reading people.
const String kBotNicknamePrefix = 'Bot_';

bool isBotSeat(PlayerDto player) => player.name.startsWith(kBotNicknamePrefix);

String seatDisplayName(PlayerDto player) {
  if (player.name.isEmpty) return 'P${player.seat}';
  if (isBotSeat(player)) return 'Bot ${player.seat}';
  return player.name;
}

/// Deterministic accent per seat, so a player keeps one colour all match.
Color seatAccent(int seat) {
  const palette = <Color>[
    KoColors.violet,
    KoColors.lime,
    KoColors.pink,
    KoColors.aqua,
    KoColors.tangerine,
    KoColors.canvasDeep,
  ];
  return palette[seat.abs() % palette.length];
}

/// Square ink-bordered seat disc carrying the player's initial.
class SeatAvatar extends StatelessWidget {
  const SeatAvatar({
    required this.player,
    this.size = 44,
    this.dimmed = false,
    super.key,
  });

  final PlayerDto player;
  final double size;

  /// Eliminated seats keep their border and glyph but drop to cream — the
  /// crossed-out doodle, not the colour, carries the meaning.
  final bool dimmed;

  @override
  Widget build(BuildContext context) {
    final name = seatDisplayName(player);
    final initial = name.isEmpty ? '?' : name.substring(0, 1).toUpperCase();

    return Container(
      width: size,
      height: size,
      alignment: Alignment.center,
      decoration: BoxDecoration(
        color: dimmed ? KoColors.surface : seatAccent(player.seat),
        border: Border.all(width: KoBorders.regular, color: KoColors.ink),
        borderRadius: BorderRadius.circular(KoRadii.well),
        boxShadow: const <BoxShadow>[KoShadows.sm],
      ),
      child: dimmed
          ? DoodleIcon(Doodle.cross, size: size * 0.55)
          : Text(
              initial,
              style: Theme.of(context).textTheme.titleLarge,
            ),
    );
  }
}

/// One player's row of truth: avatar, name, bot label, and every state the
/// rules make visible — turn, ready, disconnected, eliminated, revealed role.
///
/// Every state pairs an icon with a label; colour is never the only signal.
class SeatTile extends StatelessWidget {
  const SeatTile({
    required this.player,
    this.isLocal = false,
    this.isTurn = false,
    this.isReady = false,
    this.trailing,
    this.onTap,
    super.key,
  });

  final PlayerDto player;
  final bool isLocal;
  final bool isTurn;
  final bool isReady;
  final Widget? trailing;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final text = Theme.of(context).textTheme;
    final eliminated = player.eliminated;

    final badges = <Widget>[
      if (isLocal)
        _MiniBadge(
            icon: Icons.person, label: l10n.youLabel, color: KoColors.lime),
      if (isBotSeat(player))
        _MiniBadge(
          icon: Icons.smart_toy_outlined,
          label: l10n.botSeatLabel,
          color: KoColors.aqua,
        ),
      if (isTurn && !eliminated)
        _MiniBadge(
          icon: Icons.play_arrow,
          label: l10n.seatOnTheClock,
          color: KoColors.tangerine,
        ),
      if (isReady && !eliminated)
        _MiniBadge(
          icon: Icons.check,
          label: l10n.readyLabel,
          color: KoColors.lime,
        ),
      if (!player.connected && !eliminated)
        _MiniBadge(
          icon: Icons.wifi_off,
          label: l10n.seatDisconnected,
          color: KoColors.surface,
        ),
      if (eliminated)
        _MiniBadge(
          icon: Icons.block,
          label: l10n.eliminatedLabel,
          color: KoColors.surface,
        ),
      if (player.role != null)
        _MiniBadge(
          icon: player.role == 'donower'
              ? Icons.visibility_off
              : Icons.visibility,
          label: player.role == 'donower' ? l10n.roleDonower : l10n.roleNower,
          color: player.role == 'donower' ? KoColors.pink : KoColors.lime,
        ),
    ];

    final tile = Container(
      padding: const EdgeInsets.all(KoSpace.md),
      decoration: BoxDecoration(
        color: eliminated ? KoColors.surface : KoColors.whiteWell,
        border: Border.all(
          width: isTurn ? KoBorders.thick : KoBorders.regular,
          color: KoColors.ink,
        ),
        borderRadius: BorderRadius.circular(KoRadii.card),
        boxShadow: <BoxShadow>[isTurn ? KoShadows.md : KoShadows.sm],
      ),
      child: Row(
        children: <Widget>[
          SeatAvatar(player: player, dimmed: eliminated),
          const SizedBox(width: KoSpace.md),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              mainAxisSize: MainAxisSize.min,
              children: <Widget>[
                Text(
                  seatDisplayName(player),
                  style: text.titleLarge,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                ),
                if (badges.isNotEmpty)
                  Padding(
                    padding: const EdgeInsets.only(top: KoSpace.xs),
                    child: Wrap(
                      spacing: KoSpace.xs,
                      runSpacing: KoSpace.xs,
                      children: badges,
                    ),
                  ),
              ],
            ),
          ),
          if (trailing != null) ...<Widget>[
            const SizedBox(width: KoSpace.sm),
            trailing!,
          ],
        ],
      ),
    );

    if (onTap == null) return tile;
    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: tile,
    );
  }
}

class _MiniBadge extends StatelessWidget {
  const _MiniBadge({
    required this.icon,
    required this.label,
    required this.color,
  });

  final IconData icon;
  final String label;
  final Color color;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: KoSpace.sm, vertical: 2),
      decoration: BoxDecoration(
        color: color,
        border: Border.all(width: KoBorders.thin, color: KoColors.ink),
        borderRadius: BorderRadius.circular(KoRadii.chip),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: <Widget>[
          Icon(icon, size: 13, color: KoColors.ink),
          const SizedBox(width: KoSpace.xs),
          Text(label, style: Theme.of(context).textTheme.labelSmall),
        ],
      ),
    );
  }
}
