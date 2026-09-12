import 'dart:async';

import 'package:flutter/material.dart';

import '../../data/api_client.dart';
import '../../l10n/app_localizations.dart';
import 'ko_ui.dart';
import 'service_components.dart';

String serviceNoticeText(Object? value, Locale locale) {
  if (value is Map) {
    return (value[locale.toLanguageTag()] ??
            value[locale.languageCode] ??
            value['en'] ??
            '')
        .toString();
  }
  return value?.toString() ?? '';
}

/// Optional live-ops surface: a notice request never blocks the menu.
class ServiceNoticeBanner extends StatefulWidget {
  const ServiceNoticeBanner({this.api, this.changes, super.key});
  final ApiClient? api;
  final Listenable? changes;
  @override
  State<ServiceNoticeBanner> createState() => _ServiceNoticeBannerState();
}

class _ServiceNoticeBannerState extends State<ServiceNoticeBanner>
    with WidgetsBindingObserver {
  late Future<List<dynamic>> _notices = _fetch();
  final Set<String> _dismissed = {};
  Timer? _refresh;
  bool _foreground = true;
  final _requests = <Timer, Completer<List<dynamic>>>{};
  Future<List<dynamic>> _fetch() {
    final result = Completer<List<dynamic>>();
    late Timer timeout;
    void finish(List<dynamic> notices) {
      timeout.cancel();
      _requests.remove(timeout);
      if (!result.isCompleted) result.complete(notices);
    }

    timeout = Timer(const Duration(seconds: 10), () => finish([]));
    _requests[timeout] = result;
    serviceApi(
      widget.api,
    ).getNotices().then(finish, onError: (Object _) => finish([]));
    return result.future;
  }

  String _identity(Map<String, dynamic> notice) =>
      '${notice['id'] ?? notice['title']}:${notice['reminder_stage'] ?? 0}';
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    widget.changes?.addListener(_reload);
    _startRefresh();
  }

  void _startRefresh() {
    _refresh?.cancel();
    // Recover lost invalidation/fetches, including menus without a live socket.
    // This interval exceeds the request timeout and stops in the background.
    _refresh = Timer.periodic(const Duration(seconds: 30), (_) => _reload());
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    _foreground = state == AppLifecycleState.resumed;
    _refresh?.cancel();
    if (_foreground) {
      _reload();
      _startRefresh();
    }
  }

  void _reload() {
    if (mounted && _foreground) {
      final request = _fetch();
      setState(() {
        _notices = request;
      });
    }
  }

  @override
  void didUpdateWidget(ServiceNoticeBanner oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.changes != widget.changes) {
      oldWidget.changes?.removeListener(_reload);
      widget.changes?.addListener(_reload);
    }
    if (oldWidget.api != widget.api || oldWidget.changes != widget.changes) {
      _reload();
    }
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    _refresh?.cancel();
    widget.changes?.removeListener(_reload);
    for (final request in _requests.entries) {
      request.key.cancel();
      if (!request.value.isCompleted) request.value.complete([]);
    }
    _requests.clear();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => FutureBuilder<List<dynamic>>(
    future: _notices,
    builder: (context, snapshot) {
      if (!snapshot.hasData || snapshot.hasError) {
        return const SizedBox.shrink();
      }
      final notices = snapshot.data!
          .whereType<Map<String, dynamic>>()
          .where((notice) => !_dismissed.contains(_identity(notice)))
          .toList();
      return Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          for (final notice in notices)
            Padding(
              padding: const EdgeInsets.only(bottom: 16),
              child: ServiceNoticeTile(
                notice: notice,
                onDismiss: () =>
                    setState(() => _dismissed.add(_identity(notice))),
              ),
            ),
        ],
      );
    },
  );
}

class ServiceNoticeTile extends StatelessWidget {
  const ServiceNoticeTile({required this.notice, this.onDismiss, super.key});
  final Map<String, dynamic> notice;
  final VoidCallback? onDismiss;
  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final locale = Localizations.localeOf(context);
    final type = notice['type'];
    final startValue = notice['maintenance_start'] ?? notice['start_time'];
    final start = startValue is num
        ? DateTime.fromMillisecondsSinceEpoch(
            startValue.toInt() * 1000,
            isUtc: true,
          )
        : DateTime.tryParse(startValue?.toString() ?? '');
    return KoPanel(
      color: type == 'downtime'
          ? KoColors.pink
          : type == 'maintenance'
          ? KoColors.aqua
          : KoColors.surface,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Expanded(
                child: KoTag(
                  label: type == 'maintenance'
                      ? l10n.noticeTypeMaintenance
                      : type == 'downtime'
                      ? l10n.noticeTypeDowntime
                      : l10n.noticeTypeAnnouncement,
                  icon: DoodleIcon(
                    type == 'maintenance' ? Doodle.clock : Doodle.hailer,
                    size: 22,
                  ),
                ),
              ),
              if (onDismiss != null)
                IconButton(
                  tooltip: l10n.close,
                  onPressed: onDismiss,
                  icon: const Icon(Icons.close),
                ),
            ],
          ),
          const SizedBox(height: 20),
          Text(
            serviceNoticeText(notice['title'], locale),
            style: koDisplayStyle(size: 30),
          ),
          const SizedBox(height: 12),
          Text(serviceNoticeText(notice['body'], locale)),
          if (type == 'maintenance' && start != null) ...[
            const SizedBox(height: 16),
            _MaintenanceClock(start: start),
          ],
        ],
      ),
    );
  }
}

/// Only this small readout rebuilds; the surrounding menu stays still.
class _MaintenanceClock extends StatefulWidget {
  const _MaintenanceClock({required this.start});
  final DateTime start;
  @override
  State<_MaintenanceClock> createState() => _MaintenanceClockState();
}

class _MaintenanceClockState extends State<_MaintenanceClock> {
  Timer? _timer;
  @override
  void initState() {
    super.initState();
    if (widget.start.isAfter(DateTime.now())) {
      _timer = Timer.periodic(const Duration(seconds: 1), (_) {
        if (!mounted) return;
        setState(() {});
        if (!widget.start.isAfter(DateTime.now())) _timer?.cancel();
      });
    }
  }

  @override
  void dispose() {
    _timer?.cancel();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final seconds = widget.start
        .difference(DateTime.now())
        .inSeconds
        .clamp(0, 99999999);
    final duration =
        '${(seconds ~/ 3600).toString().padLeft(2, '0')}:${((seconds ~/ 60) % 60).toString().padLeft(2, '0')}:${(seconds % 60).toString().padLeft(2, '0')}';
    return Text(
      seconds == 0
          ? l10n.noticeTypeMaintenance
          : l10n.noticeMaintenanceIn(duration),
      style: koDisplayStyle(size: 22),
    );
  }
}
