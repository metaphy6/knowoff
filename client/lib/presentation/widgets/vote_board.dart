import 'package:flutter/material.dart';

import '../../data/models/game_state_dto.dart';
import '../../l10n/app_localizations.dart';
import '../icons/doodles.dart';
import '../theme/knowoff_tokens.dart';
import '../theme/knowoff_typography.dart';
import 'seat_sheet.dart';
import 'seat_tile.dart';

/// The Knowoff ballot.
///
/// Fairness-critical surface, so it stays mechanically aligned — no tilt, no
/// scatter, full-width rows, one tap target per candidate. The weight comes
/// from the `lg` shadow tier and the pink accusation accent instead.
///
/// Rules §4 (ADR-009): the ballot is open and live — [ballots] carries the
/// live seat->target map while the window is open (fed by `vote_cast`
/// events) and the final one once it closes. Each candidate's own row pops
/// an animated, named chip for every voter as their vote lands (and briefly
/// flashes pink), so "who votes whom" plays out right where it happens
/// instead of in a separate feed. [tally] stays `null` until the window
/// closes; the numeric count only renders once the ballot resolves.
class VoteBoard extends StatelessWidget {
  const VoteBoard({
    required this.players,
    required this.localSeat,
    required this.votedSeat,
    this.onVote,
    this.tally,
    this.ballots,
    this.eliminatedSeat = -1,
    super.key,
  });

  final List<PlayerDto> players;
  final int localSeat;
  final int votedSeat;
  final ValueChanged<int>? onVote;

  /// Per-seat vote counts, revealed after the window closes.
  final Map<String, int>? tally;

  /// Per-voter targets — live while the ballot is open (ADR-009), final once
  /// the window closes. A negative target is an abstention and has no
  /// corresponding target-row stamp.
  final Map<String, int>? ballots;
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
              votes: tally?[player.seat.toString()],
              voters: _votersFor(player.seat),
              localSeat: localSeat,
              eliminated: eliminatedSeat == player.seat,
              onVote: onVote == null || votedSeat == player.seat
                  ? null
                  : () => onVote!(player.seat),
              voteLabel: l10n.voteFor(seatDisplayName(player)),
            ),
          ),
      ],
    );
  }

  List<PlayerDto>? _votersFor(int targetSeat) {
    if (ballots == null) return null;
    return players
        .where((player) => ballots![player.seat.toString()] == targetSeat)
        .toList()
      ..sort((left, right) => left.seat.compareTo(right.seat));
  }
}

class _BallotRow extends StatefulWidget {
  const _BallotRow({
    required this.player,
    required this.chosen,
    required this.votes,
    required this.voters,
    required this.localSeat,
    required this.eliminated,
    required this.onVote,
    required this.voteLabel,
  });

  final PlayerDto player;
  final bool chosen;
  final int? votes;
  final List<PlayerDto>? voters;
  final int localSeat;
  final bool eliminated;
  final VoidCallback? onVote;
  final String voteLabel;

  @override
  State<_BallotRow> createState() => _BallotRowState();
}

class _BallotRowState extends State<_BallotRow> {
  bool _pressed = false;
  bool _hovered = false;
  bool _flashing = false;
  Set<int> _voterSeats = <int>{};

  @override
  void initState() {
    super.initState();
    _voterSeats = _seatsOf(widget.voters);
  }

  @override
  void didUpdateWidget(_BallotRow oldWidget) {
    super.didUpdateWidget(oldWidget);
    final seats = _seatsOf(widget.voters);
    // A new voter (not just a reorder of the same set) flashes the row so a
    // vote landing here is obvious even without watching the row directly.
    if (seats.difference(_voterSeats).isNotEmpty) {
      _flashing = true;
      Future.delayed(KoMotion.pop * 3, () {
        if (mounted) setState(() => _flashing = false);
      });
    }
    _voterSeats = seats;
  }

  Set<int> _seatsOf(List<PlayerDto>? voters) =>
      voters?.map((p) => p.seat).toSet() ?? <int>{};

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final text = Theme.of(context).textTheme;
    final enabled = widget.onVote != null;

    final Color fill = widget.eliminated
        ? KoColors.pink
        : widget.chosen
            ? KoColors.lime
            : _flashing
                ? KoColors.pink
                : enabled
                    ? KoColors.whiteWell
                    : KoColors.surface;

    final BoxShadow shadow = _pressed
        ? KoShadows.pressed
        : widget.chosen || widget.eliminated || _flashing
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
          // Voting is the row tap; the avatar itself opens the seat sheet.
          onLongPress: () => showSeatSheet(
            context,
            player: widget.player,
            isLocal: widget.player.seat == widget.localSeat,
          ),
          child: AnimatedContainer(
            duration: KoMotion.pop,
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
                    SeatAvatar(
                      key: ValueKey<String>(
                        'profile-avatar-${widget.player.seat}',
                      ),
                      player: widget.player,
                      size: 48,
                      onTap: () => showSeatSheet(
                        context,
                        player: widget.player,
                        isLocal: widget.player.seat == widget.localSeat,
                      ),
                    ),
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
                          if (widget.voters != null &&
                              widget.voters!.isNotEmpty) ...<Widget>[
                            const SizedBox(height: KoSpace.sm),
                            Wrap(
                              spacing: KoSpace.sm,
                              runSpacing: KoSpace.sm,
                              children: <Widget>[
                                for (final voter in widget.voters!)
                                  _VoteChip(
                                    key: ValueKey<String>(
                                        'vote-trail-${voter.seat}-${widget.player.seat}'),
                                    voter: voter,
                                    target: widget.player,
                                    localSeat: widget.localSeat,
                                  ),
                              ],
                            ),
                          ],
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

/// A single live vote, rendered as a small badge inside its target's own
/// row — the "who" (avatar + name) sitting right where the "whom" already
/// is. Pops in once per new arrival: the key is stable across rebuilds for
/// a voter who stays on this target, so it only (re)plays the entrance
/// animation when that pairing is actually new.
class _VoteChip extends StatelessWidget {
  const _VoteChip({
    required this.voter,
    required this.target,
    required this.localSeat,
    super.key,
  });

  final PlayerDto voter;
  final PlayerDto target;
  final int localSeat;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    return TweenAnimationBuilder<double>(
      tween: Tween(begin: 0, end: 1),
      duration: KoMotion.pop * 2,
      curve: Curves.elasticOut,
      builder: (context, t, child) => Opacity(
        opacity: t.clamp(0, 1),
        child: Transform.scale(scale: 0.4 + 0.6 * t.clamp(0, 1), child: child),
      ),
      child: Semantics(
        label: l10n.voteCastAnnouncement(
          seatDisplayName(voter),
          seatDisplayName(target),
        ),
        child: GestureDetector(
          onTap: () => showSeatSheet(
            context,
            player: voter,
            isLocal: voter.seat == localSeat,
          ),
          child: Container(
            padding: const EdgeInsets.only(
              left: KoSpace.xs,
              right: KoSpace.sm,
              top: KoSpace.xs,
              bottom: KoSpace.xs,
            ),
            decoration: BoxDecoration(
              color: KoColors.pink,
              border: Border.all(width: KoBorders.thin, color: KoColors.ink),
              borderRadius: BorderRadius.circular(KoRadii.chip),
              boxShadow: const <BoxShadow>[KoShadows.sm],
            ),
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: <Widget>[
                SeatAvatar(player: voter, size: 22),
                const SizedBox(width: KoSpace.xs),
                Text(
                  seatDisplayName(voter),
                  style: Theme.of(context)
                      .textTheme
                      .labelMedium
                      ?.copyWith(fontWeight: FontWeight.w800),
                ),
              ],
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
