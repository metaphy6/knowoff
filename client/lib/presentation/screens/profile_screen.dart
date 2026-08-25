import 'package:flutter/material.dart';
import 'package:knowoff_client/l10n/app_localizations.dart';

import '../../core/config/app_config.dart';
import '../../data/api_client.dart';
import '../icons/doodles.dart';
import '../theme/knowoff_tokens.dart';
import '../theme/knowoff_typography.dart';
import '../theme/ko_breakpoints.dart';
import '../widgets/avatar_upload_sheet.dart';
import '../widgets/ko_body.dart';
import '../widgets/ko_button.dart';
import '../widgets/ko_container.dart';
import '../widgets/ko_scaffold.dart';
import '../widgets/ko_stat_tile.dart';

/// The public profile screen. The viewer always sees their own profile here,
/// including the owner-only Non-Converted Points balance (👤 §1).
class ProfileScreen extends StatefulWidget {
  const ProfileScreen({this.api, super.key});

  final ApiClient? api;

  @override
  State<ProfileScreen> createState() => _ProfileScreenState();
}

class _ProfileScreenState extends State<ProfileScreen> {
  Map<String, dynamic>? _profile;
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
      final p = await _api.getProfile();
      if (!mounted) return;
      setState(() {
        _profile = p;
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

  void _openAvatarUpload() {
    showModalBottomSheet<void>(
      context: context,
      backgroundColor: KoColors.surface,
      builder: (context) => AvatarUploadSheet(api: _api),
    ).then((_) => _load());
  }

  String _value(String key) => '${_profile?[key] ?? 0}';

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    return KoScaffold(
      title: l10n.profileTitle,
      accent: KoColors.aqua,
      leadingGlyph: const DoodleIcon(Doodle.eye, size: 30),
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

    final nickname = _profile?['nickname']?.toString() ?? '';

    return KoBody(
      children: <Widget>[
        _IdentityCard(
          nickname: nickname,
          level: _value('level'),
          xp: _value('xp'),
          levelLabel: l10n.profileLevel,
          xpLabel: l10n.profileXP,
          onChangeAvatar: _openAvatarUpload,
          changeAvatarLabel: l10n.profileChangeAvatar,
        ),
        const SizedBox(height: KoSpace.xl),
        KoSectionHeader(
          label: l10n.profileStatsTitle,
          glyph: const DoodleIcon(Doodle.crown, size: 20),
          accent: KoColors.tangerine,
        ),
        GridView.count(
          crossAxisCount: KoLayout.of(context).columns(compact: 2, medium: 3),
          shrinkWrap: true,
          primary: false,
          physics: const NeverScrollableScrollPhysics(),
          mainAxisSpacing: KoSpace.md,
          crossAxisSpacing: KoSpace.md,
          childAspectRatio: 1.6,
          children: <Widget>[
            KoStatTile(
              value: _value('overall_points'),
              label: l10n.profileOverallPoints,
              accent: KoColors.lime,
              glyph: Doodle.sparkle,
            ),
            KoStatTile(
              value: _value('non_converted_points'),
              label: l10n.profileNonConvertedPoints,
              accent: KoColors.tangerine,
              glyph: Doodle.coin,
            ),
            KoStatTile(
              value: _value('matches_played'),
              label: l10n.profileMatchesPlayed,
              accent: KoColors.surface,
              glyph: Doodle.cards,
            ),
            KoStatTile(
              value: _value('correct_votes'),
              label: l10n.profileCorrectVotes,
              accent: KoColors.surface,
              glyph: Doodle.check,
            ),
            KoStatTile(
              value: _value('matches_won_nower'),
              label: l10n.profileMatchesWonNower,
              accent: KoColors.aqua,
              glyph: Doodle.eye,
            ),
            KoStatTile(
              value: _value('matches_won_donower'),
              label: l10n.profileMatchesWonDonower,
              accent: KoColors.pink,
              glyph: Doodle.mask,
            ),
          ],
        ),
      ],
    );
  }
}

class _IdentityCard extends StatelessWidget {
  const _IdentityCard({
    required this.nickname,
    required this.level,
    required this.xp,
    required this.levelLabel,
    required this.xpLabel,
    required this.onChangeAvatar,
    required this.changeAvatarLabel,
  });

  final String nickname;
  final String level;
  final String xp;
  final String levelLabel;
  final String xpLabel;
  final VoidCallback onChangeAvatar;
  final String changeAvatarLabel;

  @override
  Widget build(BuildContext context) {
    final text = Theme.of(context).textTheme;

    return KoContainer(
      backgroundColor: KoColors.surface,
      borderWidth: KoBorders.thick,
      shadow: KoShadows.lg,
      padding: const EdgeInsets.all(KoSpace.lg),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: <Widget>[
          Row(
            children: <Widget>[
              Transform.rotate(
                angle: KoTilt.subtle,
                child: Container(
                  width: 64,
                  height: 64,
                  alignment: Alignment.center,
                  decoration: BoxDecoration(
                    color: KoColors.violet,
                    border: Border.all(
                      width: KoBorders.regular,
                      color: KoColors.ink,
                    ),
                    borderRadius: BorderRadius.circular(KoRadii.card),
                    boxShadow: const <BoxShadow>[KoShadows.sm],
                  ),
                  child: Text(
                    nickname.isEmpty
                        ? '?'
                        : nickname.substring(0, 1).toUpperCase(),
                    style: koDisplayStyle(size: 30, height: 1.0),
                  ),
                ),
              ),
              const SizedBox(width: KoSpace.lg),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  mainAxisSize: MainAxisSize.min,
                  children: <Widget>[
                    Text(
                      nickname,
                      style: text.headlineSmall,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                    ),
                    Text('$levelLabel $level · $xpLabel $xp',
                        style: text.bodySmall),
                  ],
                ),
              ),
            ],
          ),
          const SizedBox(height: KoSpace.lg),
          KoButton(
            label: changeAvatarLabel,
            expand: true,
            backgroundColor: KoColors.lime,
            icon: const DoodleIcon(Doodle.sparkle, size: 20),
            onTap: onChangeAvatar,
          ),
        ],
      ),
    );
  }
}
