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
/// This is a read-only surface, so it takes the loudest layout treatment in
/// the app: plays are scattered at alternating tilts with the player's seat
/// colour stamped on each, instead of stacking as a tidy upright list.
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
          Wrap(
            spacing: KoSpace.lg,
            runSpacing: KoSpace.lg,
            children: <Widget>[
              for (var i = 0; i < entries.length; i++)
                _PlayedCard(
                  player: _playerFor(
                    int.tryParse(entries[i].key) ?? -1,
                    entries[i].key,
                  ),
                  card: entries[i].value,
                  tilt: KoTilt.alternating(i),
                  latest: (int.tryParse(entries[i].key) ?? -1) == highlightSeat,
                ),
            ],
          ),
        ],
      ),
    );
  }
}

class _PlayedCard extends StatelessWidget {
  const _PlayedCard({
    required this.player,
    required this.card,
    required this.tilt,
    required this.latest,
  });

  final PlayerDto player;
  final CardDto card;
  final double tilt;
  final bool latest;

  @override
  Widget build(BuildContext context) {
    return Transform.rotate(
      angle: tilt,
      child: Container(
        width: 132,
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
              padding: const EdgeInsets.symmetric(
                horizontal: KoSpace.sm,
                vertical: 3,
              ),
              decoration: BoxDecoration(
                color: seatAccent(player.seat),
                border: Border.all(width: KoBorders.thin, color: KoColors.ink),
                borderRadius: BorderRadius.circular(KoRadii.chip),
              ),
              child: Text(
                seatDisplayName(player),
                style: Theme.of(context).textTheme.labelSmall,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
              ),
            ),
            const SizedBox(height: KoSpace.sm),
            Container(
              height: 92,
              width: double.infinity,
              alignment: Alignment.center,
              padding: const EdgeInsets.all(KoSpace.sm),
              decoration: BoxDecoration(
                color: KoColors.whiteWell,
                border: Border.all(width: KoBorders.thin, color: KoColors.ink),
                borderRadius: BorderRadius.circular(KoRadii.well),
              ),
              child: CardFace(card: card, compact: true, maxLines: 3),
            ),
          ],
        ),
      ),
    );
  }
}
