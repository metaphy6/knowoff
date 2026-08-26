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
    super.key,
  });

  final List<PlayerDto> players;
  final Map<String, CardDto> plays;

  /// Seat whose play should read as the most recent one.
  final int highlightSeat;

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
          for (var i = 0; i < entries.length; i++) ...[
            if (i > 0) const SizedBox(height: KoSpace.md),
            _PlayedEntry(
              player: _playerFor(
                int.tryParse(entries[i].key) ?? -1,
                entries[i].key,
              ),
              card: entries[i].value,
              latest: (int.tryParse(entries[i].key) ?? -1) == highlightSeat,
            ),
          ],
        ],
      ),
    );
  }
}

class _PlayedEntry extends StatelessWidget {
  const _PlayedEntry({
    required this.player,
    required this.card,
    required this.latest,
  });

  final PlayerDto player;
  final CardDto card;
  final bool latest;

  @override
  Widget build(BuildContext context) {
    return Container(
      key: ValueKey<String>('played-card-${player.seat}'),
      width: double.infinity,
      padding: const EdgeInsets.all(KoSpace.sm),
      decoration: BoxDecoration(
        color: KoColors.surface,
        border: Border.all(
          width: latest ? KoBorders.thick : KoBorders.regular,
          color: KoColors.ink,
        ),
        borderRadius: BorderRadius.circular(KoRadii.card),
        boxShadow: <BoxShadow>[latest ? KoShadows.lg : KoShadows.sm],
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: <Widget>[
          Container(
            width: double.infinity,
            height: 96,
            alignment: Alignment.center,
            padding: const EdgeInsets.all(KoSpace.sm),
            decoration: BoxDecoration(
              color: KoColors.whiteWell,
              border: Border.all(width: KoBorders.thin, color: KoColors.ink),
              borderRadius: BorderRadius.circular(KoRadii.well),
            ),
            child: CardFace(card: card, compact: true, maxLines: 3),
          ),
          const SizedBox(height: KoSpace.sm),
          Row(
            children: <Widget>[
              SeatAvatar(player: player, size: 36),
              const SizedBox(width: KoSpace.sm),
              Expanded(
                child: Text(
                  seatDisplayName(player),
                  style: Theme.of(context).textTheme.labelMedium,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                ),
              ),
            ],
          ),
        ],
      ),
    );
  }
}
