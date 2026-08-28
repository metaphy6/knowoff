import 'package:flutter/material.dart';

import '../../core/config/app_config.dart';
import '../../data/models/game_state_dto.dart';
import '../../l10n/app_localizations.dart';
import '../icons/doodles.dart';
import '../theme/knowoff_tokens.dart';
import '../theme/knowoff_typography.dart';
import '../widgets/ko_body.dart';
import '../widgets/ko_container.dart';
import '../widgets/ko_scaffold.dart';
import '../widgets/ko_stat_tile.dart';
import '../widgets/seat_tile.dart';
import '../widgets/seat_sheet.dart';

/// Lobby screen: the room code as a hero object, the QR to scan it, and the
/// seats as they fill.
class LobbyScreen extends StatelessWidget {
  const LobbyScreen({
    required this.code,
    required this.players,
    this.roomSize = 4,
    super.key,
  });

  final String code;
  final List<PlayerDto> players;
  final int roomSize;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final total = players.length > 4 ? 6 : roomSize;

    return KoScaffold(
      title: l10n.lobbyTitle,
      subtitle: l10n.lobbySeats(players.length, total),
      accent: KoColors.lime,
      leadingGlyph: const DoodleIcon(Doodle.cards, size: 30),
      body: KoBody(
        children: <Widget>[
          if (code.isNotEmpty) _CodeCard(code: code, hint: l10n.lobbyQRHint),
          const SizedBox(height: KoSpace.xl),
          KoSectionHeader(
            label: l10n.playersCount(players.length),
            glyph: const DoodleIcon(Doodle.eye, size: 20),
            accent: KoColors.violet,
          ),
          if (players.isEmpty)
            KoEmptyState(
              doodle: Doodle.clock,
              message: l10n.lobbyWaitingForPlayers,
              accent: KoColors.aqua,
            )
          else
            for (final player in players)
              Padding(
                padding: const EdgeInsets.only(bottom: KoSpace.sm),
                child: SeatTile(
                  player: player,
                  onTap: () => showSeatSheet(context, player: player),
                ),
              ),
        ],
      ),
    );
  }
}

class _CodeCard extends StatelessWidget {
  const _CodeCard({required this.code, required this.hint});

  final String code;
  final String hint;

  @override
  Widget build(BuildContext context) {
    return KoContainer(
      backgroundColor: KoColors.surface,
      borderWidth: KoBorders.thick,
      shadow: KoShadows.lg,
      padding: const EdgeInsets.all(KoSpace.lg),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        mainAxisSize: MainAxisSize.min,
        children: <Widget>[
          Text(
            AppLocalizations.of(context).lobbyCodeLabel,
            style: Theme.of(context).textTheme.labelMedium,
          ),
          const SizedBox(height: KoSpace.sm),
          // The code is the whole point of this screen, so it gets the biggest
          // type in the app outside the verdict headline.
          FittedBox(
            fit: BoxFit.scaleDown,
            child: Text(
              code.toUpperCase(),
              style: koDisplayStyle(size: 60, letterSpacing: 6, height: 1.0),
            ),
          ),
          const SizedBox(height: KoSpace.md),
          Center(
            child: Container(
              padding: const EdgeInsets.all(KoSpace.sm),
              decoration: BoxDecoration(
                color: KoColors.whiteWell,
                border:
                    Border.all(width: KoBorders.regular, color: KoColors.ink),
                borderRadius: BorderRadius.circular(KoRadii.well),
                boxShadow: const <BoxShadow>[KoShadows.sm],
              ),
              child: Image.network(
                '${AppConfig.instance.serverUrl}/join/$code?format=qr',
                width: 160,
                height: 160,
                errorBuilder: (context, error, stack) =>
                    const DoodleIcon(Doodle.placeholder, size: 160),
              ),
            ),
          ),
          const SizedBox(height: KoSpace.md),
          Text(hint, style: Theme.of(context).textTheme.bodySmall),
        ],
      ),
    );
  }
}
