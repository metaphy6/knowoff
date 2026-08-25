import 'package:flutter/material.dart';

import '../../core/config/app_config.dart';
import '../../data/api_client.dart';
import '../../l10n/app_localizations.dart';
import '../icons/doodles.dart';
import '../theme/knowoff_tokens.dart';
import '../widgets/ko_body.dart';
import '../widgets/ko_button.dart';
import '../widgets/ko_scaffold.dart';
import '../widgets/ko_stat_tile.dart';
import '../widgets/notice_banner.dart';

/// Screen listing all active system notices (🎮 §4). Never a blocking gate.
class NoticeInboxScreen extends StatefulWidget {
  const NoticeInboxScreen({this.api, super.key});

  final ApiClient? api;

  @override
  State<NoticeInboxScreen> createState() => _NoticeInboxScreenState();
}

class _NoticeInboxScreenState extends State<NoticeInboxScreen> {
  List<dynamic>? _notices;
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
      final notices = await _api.getNotices();
      if (!mounted) return;
      setState(() {
        _notices = notices;
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
      title: l10n.noticeInboxTitle,
      accent: KoColors.surface,
      leadingGlyph: const DoodleIcon(Doodle.cloud, size: 30),
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

    final notices =
        (_notices ?? const <dynamic>[]).whereType<Map<String, dynamic>>();
    if (notices.isEmpty) {
      return KoBody.single(
        child: KoEmptyState(
          doodle: Doodle.sparkle,
          message: l10n.noticesEmpty,
          accent: KoColors.lime,
        ),
      );
    }

    return KoBody(
      children: <Widget>[
        for (final notice in notices)
          Padding(
            padding: const EdgeInsets.only(bottom: KoSpace.md),
            child: NoticeBanner(notice: notice),
          ),
      ],
    );
  }
}
