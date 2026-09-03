import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../data/models/game_state_dto.dart';
import '../../l10n/app_localizations.dart';
import '../icons/doodles.dart';
import '../state/game_session_provider.dart';
import '../theme/knowoff_tokens.dart';
import '../theme/knowoff_typography.dart';
import '../widgets/highlighter.dart';
import '../widgets/ko_body.dart';
import '../widgets/ko_button.dart';
import '../widgets/ko_container.dart';
import '../widgets/ko_scaffold.dart';
import '../widgets/ko_stat_tile.dart';
import '../widgets/rematch_overlay.dart';
import '../widgets/seat_sheet.dart';
import '../widgets/seat_tile.dart';

/// Match verdict: who won, every Nown revealed to everyone, and the match's
/// points. The generous end of the motion and colour budget lives here — it is
/// a post-match screen with no timer to protect.
class VerdictScreen extends ConsumerStatefulWidget {
  const VerdictScreen({super.key});

  @override
  ConsumerState<VerdictScreen> createState() => _VerdictScreenState();
}

class _VerdictScreenState extends ConsumerState<VerdictScreen> {
  bool _showRematchOverlay = true;

  PlayerDto _withRevealedDonowerRole(PlayerDto player) {
    return PlayerDto(
      seat: player.seat,
      name: player.name,
      connected: player.connected,
      eliminated: player.eliminated,
      avatar: player.avatar,
      bot: player.bot,
      accountId: player.accountId,
      role: 'donower',
    );
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final session = ref.watch(gameSessionProvider);
    final dto = session.dto;
    final winner =
        dto.winner ?? (session.myRole == 'donower' ? 'donower' : 'nower');
    final nowerWin = winner == 'nower';
    final iWon = nowerWin ? !session.isDonower : session.isDonower;
    final showMinimizedRematch =
        dto.phase == 'finished' && !_showRematchOverlay;
    final verdictPlayers = dto.players.map((player) {
      if (!nowerWin && dto.donowerSeats.contains(player.seat)) {
        return _withRevealedDonowerRole(player);
      }
      return player;
    }).toList()
      ..sort((a, b) {
        final aLocal = a.seat == dto.seat ? 1 : 0;
        final bLocal = b.seat == dto.seat ? 1 : 0;
        if (aLocal != bLocal) return aLocal - bLocal;
        return a.seat.compareTo(b.seat);
      });

    return Stack(
      children: <Widget>[
        KoScaffold(
          title: l10n.verdictTitle,
          accent: nowerWin ? KoColors.lime : KoColors.pink,
          canvasColor: KoColors.canvas,
          showBack: false,
          leadingGlyph:
              DoodleIcon(nowerWin ? Doodle.check : Doodle.mask, size: 30),
          bottomBar: _VerdictBottomBar(
            showRematchWakeButton: showMinimizedRematch,
            onWakeRematch: () => setState(() => _showRematchOverlay = true),
            onBackToMenu: () {
              ref.read(gameSessionProvider.notifier).restart();
              Navigator.of(context).popUntil((route) => route.isFirst);
            },
          ),
          body: KoBody(
            children: <Widget>[
              _WinnerBanner(
                headline: nowerWin ? l10n.nowerWin : l10n.donowerWin,
                blurb: nowerWin
                    ? l10n.verdictNowerBlurb
                    : l10n.verdictDonowerBlurb,
                accent: nowerWin ? KoColors.lime : KoColors.pink,
                celebrate: iWon,
              ),
              const SizedBox(height: KoSpace.lg),
              Row(
                children: <Widget>[
                  Expanded(
                    child: KoStatTile(
                      value: '${dto.matchPoints}',
                      label: l10n.verdictPointsLabel,
                      accent: KoColors.tangerine,
                      glyph: Doodle.sparkle,
                      numeralSize: 40,
                      shadow: KoShadows.md,
                      rotation: KoTilt.subtle,
                    ),
                  ),
                  const SizedBox(width: KoSpace.md),
                  Expanded(
                    child: KoStatTile(
                      value: '${dto.round}',
                      label: l10n.roundTitle,
                      accent: KoColors.aqua,
                      glyph: Doodle.clock,
                      numeralSize: 40,
                      shadow: KoShadows.md,
                      rotation: KoTilt.soft,
                    ),
                  ),
                ],
              ),
              const SizedBox(height: KoSpace.xl),
              KoSectionHeader(
                label: l10n.knowoffTitle,
                glyph: const DoodleIcon(Doodle.eye, size: 20),
                accent: KoColors.violet,
              ),
              for (final player in verdictPlayers)
                Padding(
                  padding: const EdgeInsets.only(bottom: KoSpace.sm),
                  child: SeatTile(
                    player: player,
                    isLocal: player.seat == session.seat,
                    onTap: () => showSeatSheet(
                      context,
                      player: player,
                      isLocal: player.seat == session.seat,
                    ),
                  ),
                ),
              const SizedBox(height: KoSpace.xl),
              KoSectionHeader(
                label: l10n.verdictNownsTitle,
                glyph: const DoodleIcon(Doodle.staticBurst, size: 20),
                accent: KoColors.pink,
              ),
              for (var i = 0; i < dto.nowns.length; i++)
                Padding(
                  padding: const EdgeInsets.only(bottom: KoSpace.lg),
                  child: _NownTile(nown: dto.nowns[i], index: i),
                ),
            ],
          ),
        ),
        // Wire phase is always literally "finished" — "verdict" is a legacy
        // phase constant that's never actually sent (see server
        // game.PhaseFinished). Gating on it precisely also keeps this
        // overlay out of the way of tests that build a session directly
        // with phase: 'verdict' for layout-only assertions.
        if (dto.phase == 'finished' && _showRematchOverlay)
          RematchOverlay(
            onMinimize: () => setState(() => _showRematchOverlay = false),
          ),
      ],
    );
  }
}

class _VerdictBottomBar extends StatelessWidget {
  const _VerdictBottomBar({
    required this.showRematchWakeButton,
    required this.onWakeRematch,
    required this.onBackToMenu,
  });

  final bool showRematchWakeButton;
  final VoidCallback onWakeRematch;
  final VoidCallback onBackToMenu;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);

    return Stack(
      alignment: Alignment.centerRight,
      children: <Widget>[
        KoButton(
          label: l10n.verdictBackToMenu,
          size: KoButtonSize.large,
          expand: true,
          icon: const DoodleIcon(Doodle.cards, size: 24),
          onTap: onBackToMenu,
        ),
        if (showRematchWakeButton)
          Positioned(
            right: KoSpace.md,
            child: _RematchWakeButton(onTap: onWakeRematch),
          ),
      ],
    );
  }
}

class _RematchWakeButton extends StatelessWidget {
  const _RematchWakeButton({required this.onTap});

  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);

    return GestureDetector(
      onTap: onTap,
      child: KoContainer(
        key: const Key('rematch-minimized-card'),
        backgroundColor: KoColors.lime,
        borderWidth: KoBorders.regular,
        shadow: KoShadows.lg,
        padding: const EdgeInsets.symmetric(
          horizontal: KoSpace.md,
          vertical: KoSpace.sm,
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: <Widget>[
            Transform.rotate(
              angle: KoTilt.loud,
              child: const DoodleIcon(Doodle.cards, size: 24),
            ),
            const SizedBox(width: KoSpace.xs),
            Text(l10n.rematchTitle, style: koDisplayStyle(size: 16)),
          ],
        ),
      ),
    );
  }
}

class _WinnerBanner extends StatelessWidget {
  const _WinnerBanner({
    required this.headline,
    required this.blurb,
    required this.accent,
    required this.celebrate,
  });

  final String headline;
  final String blurb;
  final Color accent;
  final bool celebrate;

  @override
  Widget build(BuildContext context) {
    return Transform.rotate(
      angle: KoTilt.subtle,
      child: Container(
        width: double.infinity,
        padding: const EdgeInsets.all(KoSpace.xl),
        decoration: BoxDecoration(
          color: accent,
          border: Border.all(width: KoBorders.thick, color: KoColors.ink),
          borderRadius: BorderRadius.circular(KoRadii.card),
          boxShadow: <BoxShadow>[
            celebrate
                ? (accent == KoColors.lime
                    ? KoShadows.violetGlow
                    : KoShadows.limeGlow)
                : KoShadows.lg,
          ],
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          mainAxisSize: MainAxisSize.min,
          children: <Widget>[
            Row(
              children: <Widget>[
                Transform.rotate(
                  angle: KoTilt.loud,
                  child: const DoodleIcon(Doodle.crown, size: 34),
                ),
                const SizedBox(width: KoSpace.sm),
                Transform.rotate(
                  angle: KoTilt.soft,
                  child: const DoodleIcon(Doodle.sparkle, size: 26),
                ),
              ],
            ),
            const SizedBox(height: KoSpace.md),
            FittedBox(
              fit: BoxFit.scaleDown,
              alignment: Alignment.centerLeft,
              child: Text(
                headline,
                style: koDisplayStyle(size: 46, letterSpacing: -2, height: 1.0),
              ),
            ),
            const SizedBox(height: KoSpace.md),
            Highlighter(
              color: KoColors.whiteWell,
              child: Text(
                blurb,
                style: Theme.of(context).textTheme.titleMedium,
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _NownTile extends StatelessWidget {
  const _NownTile({required this.nown, required this.index});

  final NownRefDto nown;
  final int index;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);

    return KoContainer(
      backgroundColor: KoColors.whiteWell,
      rotation: KoTilt.alternating(index),
      padding: EdgeInsets.zero,
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: <Widget>[
          Container(
            width: double.infinity,
            padding: const EdgeInsets.symmetric(
              horizontal: KoSpace.md,
              vertical: KoSpace.sm,
            ),
            decoration: const BoxDecoration(
              color: KoColors.canvasDeep,
              border: Border(
                bottom:
                    BorderSide(width: KoBorders.regular, color: KoColors.ink),
              ),
              borderRadius: BorderRadius.vertical(
                top: Radius.circular(KoRadii.card - KoBorders.regular),
              ),
            ),
            child: Text(
              l10n.roundLabel(index + 1),
              style: Theme.of(context).textTheme.labelMedium,
            ),
          ),
          Padding(
            padding: const EdgeInsets.all(KoSpace.md),
            child: nown.type == 'text'
                ? Text(
                    nown.content ?? '',
                    style: Theme.of(context).textTheme.titleLarge,
                  )
                : Image.network(
                    nown.signedUrl ?? '',
                    fit: BoxFit.contain,
                    errorBuilder: (context, error, stack) =>
                        Text(l10n.donowerPlaceholder),
                  ),
          ),
        ],
      ),
    );
  }
}
