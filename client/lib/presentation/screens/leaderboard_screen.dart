import 'package:flutter/material.dart';
import 'package:knowoff_client/l10n/app_localizations.dart';

import '../../core/config/app_config.dart';
import '../../data/api_client.dart';

/// The weekly Quick Play leaderboard screen.
class LeaderboardScreen extends StatefulWidget {
  const LeaderboardScreen({super.key});

  @override
  State<LeaderboardScreen> createState() => _LeaderboardScreenState();
}

class _LeaderboardScreenState extends State<LeaderboardScreen> {
  List<dynamic>? _top;
  Map<String, dynamic>? _own;
  Object? _error;
  bool _loading = true;
  final _api = ApiClient(
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
      setState(() {
        _top = data['top'] as List<dynamic>?;
        _own = data['own'] as Map<String, dynamic>?;
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

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    return Scaffold(
      appBar: AppBar(title: Text(l10n.leaderboardTitle)),
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
    return Column(
      children: [
        if (_own != null)
          ListTile(
            tileColor: Theme.of(context).colorScheme.primaryContainer,
            title: Text(l10n.leaderboardYourRank),
            trailing: Text('#${_own!['rank']} • ${_own!['points']}'),
          ),
        Expanded(
          child: ListView.builder(
            itemCount: _top?.length ?? 0,
            itemBuilder: (context, index) {
              final r = _top![index] as Map<String, dynamic>;
              return ListTile(
                leading: Text('#${r['rank']}'),
                title: Text('${r['account_id']}'.substring(0, 8)),
                trailing: Text('${r['points']}'),
              );
            },
          ),
        ),
      ],
    );
  }
}
