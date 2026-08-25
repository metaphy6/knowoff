import 'package:flutter/material.dart';
import 'package:knowoff_client/l10n/app_localizations.dart';

import '../../core/config/app_config.dart';
import '../../data/api_client.dart';
import '../icons/doodles.dart';
import '../theme/knowoff_tokens.dart';
import '../theme/knowoff_typography.dart';
import '../widgets/ko_body.dart';
import '../widgets/ko_button.dart';
import '../widgets/ko_container.dart';
import '../widgets/ko_scaffold.dart';
import '../widgets/ko_stat_tile.dart';

/// The weekly Quick Play leaderboard screen (🎮 §5): top 100 plus the viewer's
/// own rank; ties share a rank.
class LeaderboardScreen extends StatefulWidget {
  const LeaderboardScreen({this.api, super.key});

  final ApiClient? api;

  @override
  State<LeaderboardScreen> createState() => _LeaderboardScreenState();
}

class _LeaderboardScreenState extends State<LeaderboardScreen> {
  List<dynamic>? _top;
  Map<String, dynamic>? _own;
  Object? _error;
  bool _loading = true;
  late final ApiClient _api = widget.api ??
      ApiClient(
        baseUrl: AppConfig.instance.serverUrl,
        auth: AppConfig.instance.authService,
      );

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    try {
      final data = await _api.getLeaderboard();
      if (!mounted) return;
      setState(() {
        _top = data['top'] as List<dynamic>?;
        _own = data['own'] as Map<String, dynamic>?;
        _loading = false;
        _error = null;
      });
    } catch (e) {
      if (!mounted) return;
      setState(() {
        _error = e;
        _loading = false;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    return KoScaffold(
      title: l10n.leaderboardTitle,
      accent: KoColors.tangerine,
      leadingGlyph: const DoodleIcon(Doodle.crown, size: 30),
      body: _body(context, l10n),
    );
  }

  Widget _body(BuildContext context, AppLocalizations l10n) {
    if (_loading) {
      return KoBody.single(child: KoLoading(label: l10n.loadingLabel));
    }
    if (_error != null) {
      return KoBody.single(
        child: KoEmptyState(
          doodle: Doodle.cross,
          message: l10n.genericError,
          accent: KoColors.pink,
          action: KoButton(label: l10n.retry, onTap: _load),
        ),
      );
    }

    final rows = _top ?? const <dynamic>[];

    return KoBody(
      children: <Widget>[
        if (_own != null)
          Padding(
            padding: const EdgeInsets.only(bottom: KoSpace.lg),
            child: KoContainer(
              backgroundColor: KoColors.lime,
              borderWidth: KoBorders.thick,
              shadow: KoShadows.lg,
              padding: const EdgeInsets.all(KoSpace.lg),
              child: Row(
                children: <Widget>[
                  Transform.rotate(
                    angle: KoTilt.soft,
                    child: const DoodleIcon(Doodle.crown, size: 32),
                  ),
                  const SizedBox(width: KoSpace.md),
                  Expanded(
                    child: Text(
                      l10n.leaderboardYourRank,
                      style: Theme.of(context).textTheme.titleLarge,
                    ),
                  ),
                  Text(
                    '#${_own!['rank']}',
                    style: koDisplayStyle(size: 32, height: 1.0),
                  ),
                  const SizedBox(width: KoSpace.md),
                  Text(
                    '${_own!['points']}',
                    style: Theme.of(context).textTheme.titleLarge,
                  ),
                ],
              ),
            ),
          ),
        if (rows.isEmpty)
          KoEmptyState(
            doodle: Doodle.clock,
            message: l10n.leaderboardEmpty,
            accent: KoColors.aqua,
          )
        else
          for (var index = 0; index < rows.length; index++)
            Padding(
              padding: const EdgeInsets.only(bottom: KoSpace.sm),
              child: _RankRow(
                rank: '${(rows[index] as Map<String, dynamic>)['rank']}',
                name: _shortId(
                  (rows[index] as Map<String, dynamic>)['account_id'],
                ),
                points: '${(rows[index] as Map<String, dynamic>)['points']}',
                podium: index < 3,
              ),
            ),
      ],
    );
  }

  String _shortId(Object? id) {
    final value = '$id';
    return value.length <= 8 ? value : value.substring(0, 8);
  }
}

class _RankRow extends StatelessWidget {
  const _RankRow({
    required this.rank,
    required this.name,
    required this.points,
    required this.podium,
  });

  final String rank;
  final String name;
  final String points;
  final bool podium;

  @override
  Widget build(BuildContext context) {
    return KoContainer(
      backgroundColor: podium ? KoColors.surface : KoColors.whiteWell,
      shadow: podium ? KoShadows.md : KoShadows.sm,
      padding: const EdgeInsets.all(KoSpace.md),
      child: Row(
        children: <Widget>[
          Container(
            width: 46,
            height: 46,
            alignment: Alignment.center,
            decoration: BoxDecoration(
              color: podium ? KoColors.tangerine : KoColors.canvasDeep,
              border: Border.all(width: KoBorders.regular, color: KoColors.ink),
              borderRadius: BorderRadius.circular(KoRadii.well),
            ),
            child: Text(rank, style: koDisplayStyle(size: 20, height: 1.0)),
          ),
          const SizedBox(width: KoSpace.md),
          if (podium) ...<Widget>[
            const DoodleIcon(Doodle.crown, size: 20),
            const SizedBox(width: KoSpace.sm),
          ],
          Expanded(
            child: Text(
              name,
              style: Theme.of(context).textTheme.titleMedium,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
            ),
          ),
          Text(points, style: koDisplayStyle(size: 22, height: 1.0)),
        ],
      ),
    );
  }
}
