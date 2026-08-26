import 'package:flutter/material.dart';

import '../../data/models/game_state_dto.dart';
import '../../l10n/app_localizations.dart';
import '../icons/doodles.dart';
import '../theme/knowoff_tokens.dart';
import 'card_face.dart';
import 'ko_container.dart';
import 'seat_tile.dart';

/// The evidence table: every card played this match, with the name attached.
///
/// Cards list vertically in play order, each with the owner's avatar boxed
/// underneath — "who played what" has to read in one glance while arguing.
class PlayTable extends StatelessWidget {
  const PlayTable({
    required this.players,
    required this.plays,
    this.highlightSeat = -1,
    this.localSeat = -1,
    this.onPoke,
    this.onTargetedChat,
    super.key,
  });

  final List<PlayerDto> players;
  final Map<String, CardDto> plays;

  /// Seat whose play should read as the most recent one.
  final int highlightSeat;

  /// The local player's seat — its own box gets no poke/quick-chat actions.
  final int localSeat;

  /// Pokes the tapped seat's doodle badge. Null hides the badge entirely.
  final ValueChanged<int>? onPoke;

  /// Opens the targeted Quick Chat picker for the tapped seat. Null disables
  /// the tap-to-accuse/trust affordance on every box.
  final void Function(int seat)? onTargetedChat;

  PlayerDto _playerFor(int seat, String fallback) {
    for (final p in players) {
      if (p.seat == seat) return p;
    }
    return PlayerDto(
      seat: seat,
      name: fallback,
      connected: false,
      eliminated: false,
    );
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);

    if (plays.isEmpty) {
      return KoContainer(
        backgroundColor: KoColors.whiteWell,
        padding: const EdgeInsets.all(KoSpace.lg),
        child: Row(
          children: <Widget>[
            Transform.rotate(
              angle: KoTilt.loud,
              child: const DoodleIcon(Doodle.cards, size: 34),
            ),
            const SizedBox(width: KoSpace.md),
            Expanded(
              child: Text(
                l10n.emptyTable,
                style: Theme.of(context).textTheme.titleMedium,
              ),
            ),
          ],
        ),
      );
    }

    final entries = plays.entries.toList();

    return KoContainer(
      backgroundColor: KoColors.whiteWell,
      padding: const EdgeInsets.fromLTRB(
        KoSpace.md,
        KoSpace.md,
        KoSpace.md,
        KoSpace.lg,
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: <Widget>[
          Row(
            children: <Widget>[
              const DoodleIcon(Doodle.eye, size: 20),
              const SizedBox(width: KoSpace.sm),
              Text(
                l10n.evidenceTableTitle,
                style: Theme.of(context).textTheme.titleLarge,
              ),
            ],
          ),
          const SizedBox(height: KoSpace.md),
          Wrap(
            spacing: KoSpace.md,
            runSpacing: KoSpace.md,
            children: <Widget>[
              for (final entry in entries)
                _PlayedEntry(
                  player: _playerFor(
                    int.tryParse(entry.key) ?? -1,
                    entry.key,
                  ),
                  card: entry.value,
                  latest: (int.tryParse(entry.key) ?? -1) == highlightSeat,
                  isLocal: (int.tryParse(entry.key) ?? -1) == localSeat,
                  onPoke: onPoke,
                  onTargetedChat: onTargetedChat,
                ),
            ],
          ),
        ],
      ),
    );
  }
}

/// A single played card, square, sized so several sit side by side and wrap
/// onto a new row once the table runs out of width. Doubles as the poke and
/// targeted Quick Chat affordance for that seat: a poke doodle badge sits in
/// its corner, and tapping the box (any seat but your own, or an eliminated
/// one) opens the "I suspect you" / "Trust me" picker.
class _PlayedEntry extends StatelessWidget {
  const _PlayedEntry({
    required this.player,
    required this.card,
    required this.latest,
    this.isLocal = false,
    this.onPoke,
    this.onTargetedChat,
  });

  static const double _tileSize = 184;

  final PlayerDto player;
  final CardDto card;
  final bool latest;
  final bool isLocal;
  final ValueChanged<int>? onPoke;
  final void Function(int seat)? onTargetedChat;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final accent = seatAccent(player.seat);
    final canAct = !isLocal && !player.eliminated;

    final tile = Container(
      key: ValueKey<String>('played-card-${player.seat}'),
      width: _tileSize,
      padding: const EdgeInsets.all(KoSpace.md),
      decoration: BoxDecoration(
        color: KoColors.surface,
        border: Border.all(
          width: latest ? KoBorders.thick : KoBorders.regular,
          color: KoColors.ink,
        ),
        borderRadius: BorderRadius.circular(KoRadii.card),
        boxShadow: <BoxShadow>[
          BoxShadow(
            color: accent,
            offset: Offset(latest ? 8 : 6, latest ? 8 : 6),
            blurRadius: 0,
          ),
        ],
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: <Widget>[
          AspectRatio(
            aspectRatio: 1,
            child: Container(
              width: double.infinity,
              alignment: Alignment.center,
              padding: const EdgeInsets.all(KoSpace.sm),
              decoration: BoxDecoration(
                color: Color.alphaBlend(
                  accent.withValues(alpha: 0.28),
                  KoColors.whiteWell,
                ),
                border: Border.all(width: KoBorders.thin, color: KoColors.ink),
                borderRadius: BorderRadius.circular(KoRadii.well),
              ),
              child: CardFace(card: card, compact: true, maxLines: 3),
            ),
          ),
          const SizedBox(height: KoSpace.sm),
          Row(
            children: <Widget>[
              SeatAvatar(player: player, size: 34),
              const SizedBox(width: KoSpace.sm),
              Expanded(
                child: Text(
                  seatDisplayName(player),
                  style: Theme.of(context).textTheme.labelMedium,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                ),
              ),
              if (canAct && onPoke != null)
                _PokeBadge(
                  key: ValueKey<String>('poke-badge-${player.seat}'),
                  semanticLabel: '${l10n.pokeLabel} ${seatDisplayName(player)}',
                  onTap: () => onPoke!(player.seat),
                ),
            ],
          ),
        ],
      ),
    );

    if (!canAct || onTargetedChat == null) return tile;

    return Semantics(
      button: true,
      label: l10n.quickChatTargetHint(seatDisplayName(player)),
      child: Material(
        key: ValueKey<String>('played-card-tap-${player.seat}'),
        type: MaterialType.transparency,
        child: InkWell(
          borderRadius: BorderRadius.circular(KoRadii.card),
          onTap: () => onTargetedChat!(player.seat),
          child: tile,
        ),
      ),
    );
  }
}

/// Small tappable poke doodle pinned to a played-card box's corner.
class _PokeBadge extends StatelessWidget {
  const _PokeBadge(
      {required this.semanticLabel, required this.onTap, super.key});

  final String semanticLabel;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Semantics(
      button: true,
      label: semanticLabel,
      child: Material(
        color: KoColors.pink,
        shape: const CircleBorder(side: BorderSide(color: KoColors.ink)),
        child: InkWell(
          customBorder: const CircleBorder(),
          onTap: onTap,
          child: const Padding(
            padding: EdgeInsets.all(KoSpace.xs),
            child: DoodleIcon(Doodle.poke, size: 16),
          ),
        ),
      ),
    );
  }
}
