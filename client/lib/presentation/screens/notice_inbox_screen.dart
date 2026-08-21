import 'package:flutter/material.dart';

import '../../core/config/app_config.dart';
import '../../data/api_client.dart';
import '../../l10n/app_localizations.dart';

/// Screen listing all active system notices.
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
      setState(() {
        _notices = notices;
        _loading = false;
        _error = null;
      });
    } catch (e) {
      setState(() {
        _error = e;
        _loading = false;
      });
    }
  }

  String _typeLabel(AppLocalizations l10n, String? type) {
    switch (type) {
      case 'maintenance':
        return l10n.noticeTypeMaintenance;
      case 'downtime':
        return l10n.noticeTypeDowntime;
      case 'announcement':
      default:
        return l10n.noticeTypeAnnouncement;
    }
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    return Scaffold(
      appBar: AppBar(title: Text(l10n.noticeInboxTitle)),
      body: _body(context, l10n),
    );
  }

  Widget _body(BuildContext context, AppLocalizations l10n) {
    if (_loading) return const Center(child: CircularProgressIndicator());
    if (_error != null) {
      return Center(
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Text(l10n.genericError),
            TextButton(onPressed: _load, child: Text(l10n.retry)),
          ],
        ),
      );
    }
    final notices = _notices ?? [];
    if (notices.isEmpty) {
      return Center(child: Text(l10n.noticeTypeAnnouncement));
    }
    return ListView.builder(
      itemCount: notices.length,
      itemBuilder: (context, index) {
        final n = notices[index] as Map<String, dynamic>;
        return ListTile(
          title: Text(n['title']?.toString() ?? ''),
          subtitle: Text(n['body']?.toString() ?? ''),
          trailing: Text(_typeLabel(l10n, n['type']?.toString())),
        );
      },
    );
  }
}
