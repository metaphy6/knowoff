import 'dart:async';

import 'package:flutter/material.dart';

import '../../l10n/app_localizations.dart';
import '../theme/knowoff_tokens.dart';
import 'ko_container.dart';

/// Dismissible banner for active system notices.
///
/// For maintenance notices that include a [start_time], the banner shows a live
/// countdown. Other notice types render title + body only.
class NoticeBanner extends StatefulWidget {
  const NoticeBanner({required this.notice, this.onDismiss, super.key});

  final Map<String, dynamic> notice;
  final VoidCallback? onDismiss;

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

  String _title() => widget.notice['title']?.toString() ?? '';

  String _body() => widget.notice['body']?.toString() ?? '';

  String? _countdown(AppLocalizations l10n) {
    final start = _startTime;
    if (start == null) return null;
    final remaining = start.difference(DateTime.now());
    if (remaining.isNegative) return null;
    return l10n.noticeMaintenanceIn(_formatDuration(remaining));
  }

  ({Color color, IconData icon, String label}) _kind(AppLocalizations l10n) {
    switch (widget.notice['type']?.toString()) {
      case 'maintenance':
        return (
          color: KoColors.tangerine,
          icon: Icons.build,
          label: l10n.noticeTypeMaintenance,
        );
      case 'downtime':
        return (
          color: KoColors.pink,
          icon: Icons.warning_amber,
          label: l10n.noticeTypeDowntime,
        );
      default:
        return (
          color: KoColors.aqua,
          icon: Icons.campaign,
          label: l10n.noticeTypeAnnouncement,
        );
    }
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final text = Theme.of(context).textTheme;
    final kind = _kind(l10n);
    final countdown = _countdown(l10n);

    return KoContainer(
      backgroundColor: KoColors.surface,
      padding: EdgeInsets.zero,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: <Widget>[
          Container(
            width: double.infinity,
            padding: const EdgeInsets.symmetric(
              horizontal: KoSpace.md,
              vertical: KoSpace.sm,
            ),
            decoration: BoxDecoration(
              color: kind.color,
              border: const Border(
                bottom:
                    BorderSide(width: KoBorders.regular, color: KoColors.ink),
              ),
              borderRadius: const BorderRadius.vertical(
                top: Radius.circular(KoRadii.card - KoBorders.regular),
              ),
            ),
            child: Row(
              children: <Widget>[
                Icon(kind.icon, size: 18, color: KoColors.ink),
                const SizedBox(width: KoSpace.sm),
                Expanded(child: Text(kind.label, style: text.labelMedium)),
                if (widget.onDismiss != null)
                  GestureDetector(
                    behavior: HitTestBehavior.opaque,
                    onTap: widget.onDismiss,
                    child: Semantics(
                      button: true,
                      label: l10n.close,
                      child: const Icon(Icons.close, size: 20),
                    ),
                  ),
              ],
            ),
          ),
          Padding(
            padding: const EdgeInsets.all(KoSpace.md),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              mainAxisSize: MainAxisSize.min,
              children: <Widget>[
                Text(_title(), style: text.titleLarge),
                if (countdown != null) ...<Widget>[
                  const SizedBox(height: KoSpace.sm),
                  // Countdown digits get the display face at hero size — the
                  // one number in a notice anybody actually reads.
                  Text(countdown, style: text.headlineSmall),
                ],
                if (_body().isNotEmpty) ...<Widget>[
                  const SizedBox(height: KoSpace.sm),
                  Text(_body(), style: text.bodyMedium),
                ],
              ],
            ),
          ),
        ],
      ),
    );
  }
}
