import 'package:flutter/material.dart';

import '../../core/config/app_config.dart';
import '../../data/api_client.dart';
import '../../data/models/game_state_dto.dart';
import '../../l10n/app_localizations.dart';
import '../icons/doodles.dart';
import '../theme/knowoff_tokens.dart';
import 'ko_button.dart';
import 'ko_stat_tile.dart';
import 'report_dialog.dart';
import 'seat_tile.dart';

/// Opens the seat inspector for [player].
Future<void> showSeatSheet(
  BuildContext context, {
  required PlayerDto player,
  bool isLocal = false,
  bool isTurn = false,
  bool isReady = false,
  ApiClient? api,
}) {
  return showModalBottomSheet<void>(
    context: context,
    backgroundColor: Colors.transparent,
    isScrollControlled: true,
    builder: (context) => SeatSheet(
      player: player,
      isLocal: isLocal,
      isTurn: isTurn,
      isReady: isReady,
      api: api,
    ),
  );
}

/// Everything you are allowed to know about one seat, in one place: who they
/// are, what the rules have already made public about them this match, their
/// public career numbers, and the flag that starts a conduct report (👤 §3).
///
/// Career stats are fetched per open rather than pushed with the match state —
/// they are not gameplay information, and pushing them would put a database
/// read on every phase broadcast.
class SeatSheet extends StatefulWidget {
  const SeatSheet({
    required this.player,
    this.isLocal = false,
    this.isTurn = false,
    this.isReady = false,
    this.api,
    super.key,
  });

  final PlayerDto player;
  final bool isLocal;
  final bool isTurn;
  final bool isReady;
  final ApiClient? api;

  @override
  State<SeatSheet> createState() => _SeatSheetState();
}

class _SeatSheetState extends State<SeatSheet> {
  Map<String, dynamic>? _profile;
  bool _loading = false;
  bool _failed = false;

  bool get _hasProfile =>
      !isBotSeat(widget.player) && widget.player.accountId.isNotEmpty;

  @override
  void initState() {
    super.initState();
    if (_hasProfile) _load();
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _failed = false;
    });
    try {
      final api = widget.api ??
          ApiClient(
            baseUrl: AppConfig.instance.serverUrl,
            auth: AppConfig.instance.authService,
          );
      final data = await api.getPublicProfile(widget.player.accountId);
      if (mounted) setState(() => _profile = data);
    } catch (_) {
      if (mounted) setState(() => _failed = true);
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  String _stat(String key) => (_profile?[key] as num?)?.toString() ?? '—';

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final text = Theme.of(context).textTheme;
    final player = widget.player;
    final bot = isBotSeat(player);

    return SafeArea(
      top: false,
      child: Container(
        margin: const EdgeInsets.all(KoSpace.md),
        padding: const EdgeInsets.all(KoSpace.lg),
        decoration: BoxDecoration(
          color: KoColors.surface,
          border: Border.all(width: KoBorders.thick, color: KoColors.ink),
          borderRadius: BorderRadius.circular(KoRadii.sheet),
          boxShadow: const <BoxShadow>[KoShadows.lg],
        ),
        child: SingleChildScrollView(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            mainAxisSize: MainAxisSize.min,
            children: <Widget>[
              Row(
                children: <Widget>[
                  SeatAvatar(
                    player: player,
                    size: 72,
                    dimmed: player.eliminated,
                  ),
                  const SizedBox(width: KoSpace.lg),
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      mainAxisSize: MainAxisSize.min,
                      children: <Widget>[
                        Text(
                          seatDisplayName(player),
                          style: text.headlineSmall,
                          maxLines: 1,
                          overflow: TextOverflow.ellipsis,
                        ),
                        Text(
                          l10n.seatNumberLabel(player.seat),
                          style: text.labelMedium,
                        ),
                      ],
                    ),
                  ),
                ],
              ),
              const SizedBox(height: KoSpace.lg),
              _StateChips(
                player: player,
                isLocal: widget.isLocal,
                isTurn: widget.isTurn,
                isReady: widget.isReady,
              ),
              const SizedBox(height: KoSpace.lg),
              if (bot)
                _Note(message: l10n.seatSheetBotNote, accent: KoColors.aqua)
              else if (!_hasProfile)
                _Note(message: l10n.seatSheetNoStats, accent: KoColors.surface)
              else if (_loading)
                _Note(message: l10n.loadingLabel, accent: KoColors.surface)
              else if (_failed)
                _Note(message: l10n.genericError, accent: KoColors.pink)
              else
                _Stats(
                  label: l10n.profileStatsTitle,
                  tiles: <Widget>[
                    KoStatTile(
                      value: _stat('matches_played'),
                      label: l10n.profileMatchesPlayed,
                      accent: KoColors.whiteWell,
                      glyph: Doodle.cards,
                      numeralSize: 26,
                    ),
                    KoStatTile(
                      value: _stat('level'),
                      label: l10n.profileLevel,
                      accent: KoColors.lime,
                      glyph: Doodle.crown,
                      numeralSize: 26,
                    ),
                    KoStatTile(
                      value: _stat('matches_won_nower'),
                      label: l10n.profileMatchesWonNower,
                      accent: KoColors.whiteWell,
                      glyph: Doodle.eye,
                      numeralSize: 26,
                    ),
                    KoStatTile(
                      value: _stat('matches_won_donower'),
                      label: l10n.profileMatchesWonDonower,
                      accent: KoColors.whiteWell,
                      glyph: Doodle.mask,
                      numeralSize: 26,
                    ),
                    KoStatTile(
                      value: _stat('correct_votes'),
                      label: l10n.profileCorrectVotes,
                      accent: KoColors.whiteWell,
                      glyph: Doodle.check,
                      numeralSize: 26,
                    ),
                    KoStatTile(
                      value: _stat('overall_points'),
                      label: l10n.profileOverallPoints,
                      accent: KoColors.tangerine,
                      glyph: Doodle.coin,
                      numeralSize: 26,
                    ),
                  ],
                ),
              const SizedBox(height: KoSpace.lg),
              Wrap(
                spacing: KoSpace.md,
                runSpacing: KoSpace.md,
                children: <Widget>[
                  if (!bot && !widget.isLocal && _hasProfile)
                    KoButton(
                      label: l10n.reportPlayerAction,
                      backgroundColor: KoColors.pink,
                      icon: const Icon(Icons.flag_outlined,
                          size: 18, color: KoColors.ink),
                      onTap: () => showDialog<void>(
                        context: context,
                        builder: (_) => ReportDialog(
                          targetAccountID: player.accountId,
                          api: widget.api,
                        ),
                      ),
                    ),
                  KoButton(
                    label: l10n.close,
                    backgroundColor: KoColors.whiteWell,
                    onTap: () => Navigator.of(context).pop(),
                  ),
                ],
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _StateChips extends StatelessWidget {
  const _StateChips({
    required this.player,
    required this.isLocal,
    required this.isTurn,
    required this.isReady,
  });

  final PlayerDto player;
  final bool isLocal;
  final bool isTurn;
  final bool isReady;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final eliminated = player.eliminated;

    final chips = <Widget>[
      if (isLocal) _Chip(icon: Icons.person, label: l10n.youLabel),
      if (isBotSeat(player))
        _Chip(icon: Icons.smart_toy_outlined, label: l10n.botSeatLabel),
      if (isTurn && !eliminated)
        _Chip(icon: Icons.play_arrow, label: l10n.seatOnTheClock),
      if (isReady && !eliminated)
        _Chip(icon: Icons.check, label: l10n.readyLabel),
      if (!player.connected && !eliminated)
        _Chip(icon: Icons.wifi_off, label: l10n.seatDisconnected),
      if (eliminated) _Chip(icon: Icons.block, label: l10n.eliminatedLabel),
      if (player.role != null)
        _Chip(
          icon: player.role == 'donower'
              ? Icons.visibility_off
              : Icons.visibility,
          label: player.role == 'donower' ? l10n.roleDonower : l10n.roleNower,
        ),
    ];

    return Wrap(spacing: KoSpace.sm, runSpacing: KoSpace.sm, children: chips);
  }
}

class _Chip extends StatelessWidget {
  const _Chip({required this.icon, required this.label});

  final IconData icon;
  final String label;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: KoSpace.sm, vertical: 3),
      decoration: BoxDecoration(
        color: KoColors.whiteWell,
        border: Border.all(width: KoBorders.thin, color: KoColors.ink),
        borderRadius: BorderRadius.circular(KoRadii.chip),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: <Widget>[
          Icon(icon, size: 14, color: KoColors.ink),
          const SizedBox(width: KoSpace.xs),
          Text(label, style: Theme.of(context).textTheme.labelSmall),
        ],
      ),
    );
  }
}

class _Stats extends StatelessWidget {
  const _Stats({required this.label, required this.tiles});

  final String label;
  final List<Widget> tiles;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisSize: MainAxisSize.min,
      children: <Widget>[
        Text(label, style: Theme.of(context).textTheme.titleMedium),
        const SizedBox(height: KoSpace.sm),
        GridView.count(
          crossAxisCount: 3,
          shrinkWrap: true,
          primary: false,
          physics: const NeverScrollableScrollPhysics(),
          crossAxisSpacing: KoSpace.sm,
          mainAxisSpacing: KoSpace.sm,
          childAspectRatio: 1.15,
          children: tiles,
        ),
      ],
    );
  }
}

class _Note extends StatelessWidget {
  const _Note({required this.message, required this.accent});

  final String message;
  final Color accent;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(KoSpace.md),
      decoration: BoxDecoration(
        color: accent,
        border: Border.all(width: KoBorders.regular, color: KoColors.ink),
        borderRadius: BorderRadius.circular(KoRadii.card),
      ),
      child: Text(message, style: Theme.of(context).textTheme.bodyMedium),
    );
  }
}
