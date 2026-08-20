import 'package:flutter/material.dart';
import 'package:knowoff_client/l10n/app_localizations.dart';

import '../../data/api_client.dart';
import '../../core/config/app_config.dart';

/// The public profile screen. The viewer always sees their own profile here,
/// including the owner-only Non-Converted Points balance.
class ProfileScreen extends StatefulWidget {
  const ProfileScreen({super.key});

  @override
  State<ProfileScreen> createState() => _ProfileScreenState();
}

class _ProfileScreenState extends State<ProfileScreen> {
  Map<String, dynamic>? _profile;
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
      final p = await _api.getProfile();
      setState(() {
        _profile = p;
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
      appBar: AppBar(title: Text(l10n.profileTitle)),
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
    final p = _profile!;
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        _stat(l10n.profileNickname, p['nickname'] ?? ''),
        _stat(l10n.profileLevel, '${p['level']}'),
        _stat(l10n.profileXP, '${p['xp']}'),
        _stat(l10n.profileOverallPoints, '${p['overall_points']}'),
        _stat(l10n.profileNonConvertedPoints, '${p['non_converted_points']}'),
        _stat(l10n.profileMatchesPlayed, '${p['matches_played']}'),
        _stat(l10n.profileMatchesWonNower, '${p['matches_won_nower']}'),
        _stat(l10n.profileMatchesWonDonower, '${p['matches_won_donower']}'),
        _stat(l10n.profileCorrectVotes, '${p['correct_votes']}'),
      ],
    );
  }

  Widget _stat(String label, String value) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 4),
      child: Row(
        mainAxisAlignment: MainAxisAlignment.spaceBetween,
        children: [
          Text(label),
          Text(value, style: const TextStyle(fontWeight: FontWeight.bold)),
        ],
      ),
    );
  }
}
