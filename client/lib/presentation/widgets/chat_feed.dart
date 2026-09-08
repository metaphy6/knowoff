import 'package:flutter/material.dart';

import '../../data/models/game_state_dto.dart';
import '../../l10n/app_localizations.dart';
import '../icons/doodles.dart';
import '../theme/knowoff_tokens.dart';
import 'ko_container.dart';
import 'quick_chat_bar.dart';
import 'seat_sheet.dart';
import 'seat_tile.dart';

/// Scrollable feed of recent chat messages and pokes.
class ChatFeed extends StatelessWidget {
  const ChatFeed({
    required this.events,
    required this.players,
    required this.localSeat,
    super.key,
  });

  final List<ChatEventDto> events;
  final List<PlayerDto> players;
  final int localSeat;

  PlayerDto _playerFor(int seat) {
    for (final p in players) {
      if (p.seat == seat) return p;
    }
    return PlayerDto(
      seat: seat,
      name: '',
      connected: false,
      eliminated: false,
    );
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final text = Theme.of(context).textTheme;

    if (events.isEmpty) {
      return KoContainer(
        backgroundColor: KoColors.whiteWell,
        shadow: KoShadows.sm,
        padding: const EdgeInsets.all(KoSpace.lg),
        child: Row(
          children: <Widget>[
            Transform.rotate(
              angle: KoTilt.soft,
              child: const DoodleIcon(Doodle.quietBubble, size: 26),
            ),
            const SizedBox(width: KoSpace.md),
            Expanded(
              child: Text(l10n.chatLogEmpty, style: text.titleMedium),
            ),
          ],
        ),
      );
    }

    return KoContainer(
      backgroundColor: KoColors.whiteWell,
      shadow: KoShadows.sm,
      padding: const EdgeInsets.all(KoSpace.md),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: events.reversed.take(6).map((event) {
          final from = _playerFor(event.fromSeat);
          final isPoke = event.kind == 'poke';
          final label = isPoke
              ? (event.targetSeat == localSeat
                  ? l10n.pokedByLabel(seatDisplayName(from))
                  : l10n.pokedTargetLog(
                      seatDisplayName(from),
                      seatDisplayName(_playerFor(event.targetSeat ?? -1)),
                    ))
              : (event.text ?? quickChatPhraseLabel(l10n, event.phraseId));

          return Padding(
            padding: const EdgeInsets.symmetric(vertical: KoSpace.xs),
            child: Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: <Widget>[
                SeatAvatar(
                  key: ValueKey<String>('profile-avatar-${from.seat}'),
                  player: from,
                  size: 30,
                  onTap: () => showSeatSheet(
                    context,
                    player: from,
                    isLocal: from.seat == localSeat,
                  ),
                ),
                const SizedBox(width: KoSpace.sm),
                Expanded(
                  child: Container(
                    padding: const EdgeInsets.symmetric(
                      horizontal: KoSpace.md,
                      vertical: KoSpace.sm,
                    ),
                    decoration: BoxDecoration(
                      color: isPoke ? KoColors.pink : KoColors.surface,
                      border: Border.all(
                        width: KoBorders.thin,
                        color: KoColors.ink,
                      ),
                      borderRadius: BorderRadius.circular(KoRadii.well),
                    ),
                    child: Row(
                      children: <Widget>[
                        DoodleIcon(
                          isPoke ? Doodle.poke : Doodle.cloud,
                          size: 14,
                        ),
                        const SizedBox(width: KoSpace.sm),
                        Expanded(
                          child: Text(
                            isPoke ? label : '${seatDisplayName(from)}: $label',
                            style: text.bodyMedium,
                          ),
                        ),
                      ],
                    ),
                  ),
                ),
              ],
            ),
          );
        }).toList(),
      ),
    );
  }
}
