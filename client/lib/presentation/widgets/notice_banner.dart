import 'dart:async';

import 'package:flutter/material.dart';

import '../../l10n/app_localizations.dart';

/// Dismissible banner for active system notices.
///
/// For maintenance notices that include a [start_time], the banner shows a live
/// countdown. Other notice types render title + body only.
class NoticeBanner extends StatefulWidget {
  const NoticeBanner({required this.notice, super.key});

  final Map<String, dynamic> notice;

  @override
  State<NoticeBanner> createState() => _NoticeBannerState();
}

class _NoticeBannerState extends State<NoticeBanner> {
  Timer? _timer;

  @override
  void initState() {
    super.initState();
    if (_startTime != null) {
      _timer = Timer.periodic(const Duration(seconds: 1), (_) {
        if (mounted) setState(() {});
      });
    }
  }

  @override
  void dispose() {
    _timer?.cancel();
    super.dispose();
  }

  DateTime? get _startTime {
    final raw = widget.notice['start_time'];
    if (raw == null) return null;
    if (raw is int) return DateTime.fromMillisecondsSinceEpoch(raw * 1000);
    final parsed = DateTime.tryParse(raw.toString());
    return parsed?.toLocal();
  }

  String _formatDuration(Duration d) {
    final hours = d.inHours;
    final minutes = d.inMinutes % 60;
    final seconds = d.inSeconds % 60;
    final parts = <String>[];
    if (hours > 0) parts.add('${hours}h');
    if (minutes > 0 || hours > 0) parts.add('${minutes}m');
    parts.add('${seconds}s');
    return parts.join(' ');
  }

  String _subtitle(AppLocalizations l10n) {
    final start = _startTime;
    if (start == null) return widget.notice['body']?.toString() ?? '';
    final remaining = start.difference(DateTime.now());
    if (remaining.isNegative) {
      return widget.notice['body']?.toString() ?? '';
    }
    final duration = _formatDuration(remaining);
    return '${l10n.noticeMaintenanceIn(duration)}\n${widget.notice['body']?.toString() ?? ''}';
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    return MaterialBanner(
      content: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            widget.notice['title']?.toString() ?? '',
            style: const TextStyle(fontWeight: FontWeight.bold),
          ),
          Text(_subtitle(l10n)),
        ],
      ),
      actions: [
        TextButton(
          onPressed: () {
            ScaffoldMessenger.of(context).hideCurrentMaterialBanner();
          },
          child: Text(l10n.close),
        ),
      ],
    );
  }
}
