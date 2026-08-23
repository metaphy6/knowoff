import 'package:flutter/material.dart';

import '../../data/models/game_state_dto.dart';
import '../../l10n/app_localizations.dart';
import '../icons/doodles.dart';
import '../theme/knowoff_tokens.dart';
import '../theme/knowoff_typography.dart';
import 'seat_tile.dart';

/// The Knowoff ballot.
///
/// Fairness-critical surface, so it stays mechanically aligned — no tilt, no
/// scatter, full-width rows, one tap target per candidate. The weight comes
/// from the `lg` shadow tier and the pink accusation accent instead.
///
/// Rules §4: the ballot is blind — [tally] is only non-null once the window has
/// closed, and only then do the counts render.
class VoteBoard extends StatelessWidget {
  const VoteBoard({
    required this.players,
    required this.localSeat,
    required this.votedSeat,
    this.onVote,
    this.tally,
    this.eliminatedSeat = -1,
    super.key,
  });

  final List<PlayerDto> players;
  final int localSeat;
  final int votedSeat;
  final ValueChanged<int>? onVote;

  /// Per-seat vote counts, revealed after the window closes.
  final Map<String, int>? tally;
  final int eliminatedSeat;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final candidates = players
        .where((p) => !p.eliminated && p.seat != localSeat)
        .toList()
      ..sort((a, b) => a.seat.compareTo(b.seat));

    if (candidates.isEmpty) {
      return const SizedBox.shrink();
    }

    final locked = votedSeat >= 0;

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      mainAxisSize: MainAxisSize.min,
      children: <Widget>[
        for (final player in candidates)
          Padding(
            padding: const EdgeInsets.only(bottom: KoSpace.md),
            child: _BallotRow(
              player: player,
              chosen: votedSeat == player.seat,
              locked: locked,
              votes: tally?[player.seat.toString()],
              eliminated: eliminatedSeat == player.seat,
              onVote:
                  onVote == null || locked ? null : () => onVote!(player.seat),
              voteLabel: l10n.voteFor(seatDisplayName(player)),
            ),
          ),
      ],
    );
  }
}

class _BallotRow extends StatefulWidget {
  const _BallotRow({
    required this.player,
    required this.chosen,
    required this.locked,
    required this.votes,
    required this.eliminated,
    required this.onVote,
    required this.voteLabel,
  });

  final PlayerDto player;
  final bool chosen;
  final bool locked;
  final int? votes;
  final bool eliminated;
  final VoidCallback? onVote;
  final String voteLabel;

  @override
  State<_BallotRow> createState() => _BallotRowState();
}

class _BallotRowState extends State<_BallotRow> {
  bool _pressed = false;
  bool _hovered = false;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final text = Theme.of(context).textTheme;
    final enabled = widget.onVote != null;

    final Color fill = widget.eliminated
        ? KoColors.pink
        : widget.chosen
            ? KoColors.lime
            : enabled
                ? KoColors.whiteWell
                : KoColors.surface;

    final BoxShadow shadow = _pressed
        ? KoShadows.pressed
        : widget.chosen || widget.eliminated
            ? KoShadows.lg
            : _hovered && enabled
                ? KoShadows.lift
                : KoShadows.md;

    final status = widget.eliminated
        ? (icon: Doodle.cross, label: l10n.eliminatedLabel)
        : widget.chosen
            ? (icon: Doodle.check, label: l10n.yourVoteLabel)
            : null;

    return Semantics(
      button: enabled,
      selected: widget.chosen,
      label: widget.voteLabel,
      child: MouseRegion(
        cursor: enabled ? SystemMouseCursors.click : SystemMouseCursors.basic,
        onEnter: (_) => setState(() => _hovered = true),
        onExit: (_) => setState(() => _hovered = false),
        child: GestureDetector(
          onTapDown: enabled ? (_) => setState(() => _pressed = true) : null,
          onTapCancel: () => setState(() => _pressed = false),
          onTapUp: enabled
              ? (_) {
                  setState(() => _pressed = false);
                  widget.onVote!();
                }
              : null,
          child: AnimatedContainer(
            duration: KoMotion.press,
            curve: Curves.easeOut,
            constraints: const BoxConstraints(minHeight: 72),
            decoration: BoxDecoration(
              color: fill,
              border: Border.all(
                width: widget.chosen || widget.eliminated
                    ? KoBorders.thick
                    : KoBorders.regular,
                color: KoColors.ink,
              ),
              borderRadius: BorderRadius.circular(KoRadii.card),
              boxShadow: <BoxShadow>[shadow],
            ),
            child: Transform.translate(
              offset: _pressed ? const Offset(4, 4) : Offset.zero,
              child: Padding(
                padding: const EdgeInsets.all(KoSpace.md),
                child: Row(
                  children: <Widget>[
                    SeatAvatar(player: widget.player, size: 48),
                    const SizedBox(width: KoSpace.md),
                    Expanded(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        mainAxisSize: MainAxisSize.min,
                        children: <Widget>[
                          Text(
                            seatDisplayName(widget.player),
                            style: text.titleLarge,
                            maxLines: 1,
                            overflow: TextOverflow.ellipsis,
                          ),
                          if (status != null)
                            Row(
                              mainAxisSize: MainAxisSize.min,
                              children: <Widget>[
                                DoodleIcon(status.icon, size: 15),
                                const SizedBox(width: KoSpace.xs),
                                Text(status.label, style: text.labelMedium),
                              ],
                            )
                          else if (isBotSeat(widget.player))
                            Text(l10n.botSeatLabel, style: text.labelSmall),
                        ],
                      ),
                    ),
                    if (widget.votes != null)
                      _TallyStamp(votes: widget.votes!)
                    else if (enabled)
                      _AccuseStamp(label: l10n.voteLabel),
                  ],
                ),
              ),
            ),
          ),
        ),
      ),
    );
  }
}

class _AccuseStamp extends StatelessWidget {
  const _AccuseStamp({required this.label});

  final String label;

  @override
  Widget build(BuildContext context) {
    return Transform.rotate(
      angle: KoTilt.soft,
      child: Container(
        padding: const EdgeInsets.symmetric(
          horizontal: KoSpace.md,
          vertical: KoSpace.sm,
        ),
        decoration: BoxDecoration(
          color: KoColors.pink,
          border: Border.all(width: KoBorders.regular, color: KoColors.ink),
          borderRadius: BorderRadius.circular(KoRadii.button),
          boxShadow: const <BoxShadow>[KoShadows.sm],
        ),
        child: Text(label, style: Theme.of(context).textTheme.labelLarge),
      ),
    );
  }
}

class _TallyStamp extends StatelessWidget {
  const _TallyStamp({required this.votes});

  final int votes;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: 52,
      height: 52,
      alignment: Alignment.center,
      decoration: BoxDecoration(
        color: votes > 0 ? KoColors.pink : KoColors.whiteWell,
        border: Border.all(width: KoBorders.regular, color: KoColors.ink),
        borderRadius: BorderRadius.circular(KoRadii.well),
        boxShadow: const <BoxShadow>[KoShadows.sm],
      ),
      child: Text('$votes', style: koDisplayStyle(size: 24, height: 1.0)),
    );
  }
}
