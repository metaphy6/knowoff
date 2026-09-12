import 'package:flutter/material.dart';
import 'package:intl/intl.dart';

import '../../core/config/app_config.dart';
import '../../data/api_client.dart';
import '../../l10n/app_localizations.dart';
import 'ko_ui.dart';
import 'device_layout.dart';

/// Phones complete one task at a time; larger windows retain its context beside it.
class ServiceWorkspace extends StatefulWidget {
  const ServiceWorkspace({
    required this.primary,
    required this.secondary,
    super.key,
  });
  final Widget primary;
  final Widget secondary;

  @override
  State<ServiceWorkspace> createState() => _ServiceWorkspaceState();
}

class _ServiceWorkspaceState extends State<ServiceWorkspace> {
  final _primaryKey = GlobalKey();
  final _secondaryKey = GlobalKey();

  @override
  Widget build(BuildContext context) => LayoutBuilder(
    builder: (context, c) {
      final device = KoDeviceLayout.of(context);
      final scale = MediaQuery.textScalerOf(context).scale(14) / 14;
      final primary = KeyedSubtree(key: _primaryKey, child: widget.primary);
      final secondary = KeyedSubtree(
        key: _secondaryKey,
        child: widget.secondary,
      );
      if (device.isPhone || c.maxWidth < 620 * scale.clamp(1, 1.5)) {
        return Column(
          key: const Key('service-workspace-stacked'),
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [primary, const SizedBox(height: 28), secondary],
        );
      }
      return Row(
        key: const Key('service-workspace-split'),
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Expanded(flex: device.isDesktop ? 4 : 5, child: primary),
          const SizedBox(width: 28),
          Expanded(flex: 6, child: secondary),
        ],
      );
    },
  );
}

ApiClient serviceApi(ApiClient? api) =>
    api ??
    ApiClient(
      baseUrl: AppConfig.instance.serverUrl,
      auth: AppConfig.instance.authService,
    );

String serviceNumber(BuildContext context, Object? value) => value is num
    ? NumberFormat.decimalPattern(
        Localizations.localeOf(context).toLanguageTag(),
      ).format(value)
    : '0';

String servicePercent(
  BuildContext context,
  Object? numerator,
  Object? denominator,
) {
  final total = denominator is num ? denominator : 0;
  final part = numerator is num ? numerator : 0;
  return NumberFormat.percentPattern(
    Localizations.localeOf(context).toLanguageTag(),
  ).format(total > 0 ? part / total : 0);
}

void serviceMessage(BuildContext context, String message) =>
    ScaffoldMessenger.of(
      context,
    ).showSnackBar(SnackBar(content: Text(message)));

class ServiceStatus extends StatelessWidget {
  const ServiceStatus({this.error = false, this.onRetry, super.key});
  final bool error;
  final VoidCallback? onRetry;
  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    return KoPanel(
      color: error ? KoColors.pink : KoColors.aqua,
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          DoodleIcon(error ? Doodle.cross : Doodle.clock, size: 64),
          const SizedBox(height: 16),
          Text(
            error ? l10n.genericError : l10n.loadingLabel,
            style: koDisplayStyle(size: 25),
            textAlign: TextAlign.center,
          ),
          if (onRetry != null) ...[
            const SizedBox(height: 18),
            KoButton(label: l10n.retry, onPressed: onRetry),
          ],
        ],
      ),
    );
  }
}

class ServiceGrid extends StatelessWidget {
  const ServiceGrid({required this.children, this.minWidth = 250, super.key});
  final List<Widget> children;
  final double minWidth;
  @override
  Widget build(BuildContext context) => LayoutBuilder(
    builder: (context, constraints) {
      final columns = (constraints.maxWidth / (minWidth + 18)).floor().clamp(
        1,
        4,
      );
      final width = (constraints.maxWidth - (columns - 1) * 18) / columns;
      return Wrap(
        spacing: 18,
        runSpacing: 18,
        children: [
          for (final child in children) SizedBox(width: width, child: child),
        ],
      );
    },
  );
}

class ServiceMetric extends StatelessWidget {
  const ServiceMetric({
    required this.label,
    required this.value,
    this.color = KoColors.surface,
    super.key,
  });
  final String label;
  final String value;
  final Color color;
  @override
  Widget build(BuildContext context) => KoPanel(
    color: color,
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(value, style: koDisplayStyle(size: 44)),
        const SizedBox(height: 6),
        Text(label),
      ],
    ),
  );
}

class ServiceReportDialog extends StatefulWidget {
  const ServiceReportDialog({
    required this.api,
    required this.reportType,
    this.accountId,
    this.mediaId,
    super.key,
  }) : assert(accountId != null || mediaId != null);
  final ApiClient api;
  final String reportType;
  final String? accountId;
  final String? mediaId;
  @override
  State<ServiceReportDialog> createState() => _ServiceReportDialogState();
}

class _ServiceReportDialogState extends State<ServiceReportDialog> {
  final _form = GlobalKey<FormState>();
  final _reason = TextEditingController();
  final _description = TextEditingController();
  bool _busy = false;
  bool _failed = false;
  @override
  void dispose() {
    _reason.dispose();
    _description.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    if (!_form.currentState!.validate() || _busy) return;
    setState(() {
      _busy = true;
      _failed = false;
    });
    try {
      await widget.api.createReport(
        reportType: widget.reportType,
        targetAccountID: widget.accountId,
        targetMediaID: widget.mediaId,
        reason: _reason.text.trim(),
        description: _description.text.trim().isEmpty
            ? null
            : _description.text.trim(),
      );
      if (!mounted) return;
      Navigator.of(context).pop(true);
    } catch (_) {
      if (mounted) {
        setState(() {
          _busy = false;
          _failed = true;
        });
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    return Dialog(
      child: SingleChildScrollView(
        child: Padding(
          padding: const EdgeInsets.all(22),
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 440),
            child: Form(
              key: _form,
              child: Column(
                mainAxisSize: MainAxisSize.min,
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  KoHeading(
                    title: l10n.reportTitle,
                    trailing: const DoodleIcon(Doodle.hailer, size: 40),
                  ),
                  TextFormField(
                    key: const Key('report-reason'),
                    controller: _reason,
                    enabled: !_busy,
                    maxLength: 500,
                    decoration: InputDecoration(labelText: l10n.reportReason),
                    validator: (v) => v == null || v.trim().isEmpty
                        ? l10n.serviceRequired
                        : null,
                  ),
                  const SizedBox(height: 12),
                  TextFormField(
                    controller: _description,
                    enabled: !_busy,
                    minLines: 2,
                    maxLines: 4,
                    maxLength: 2000,
                    decoration: InputDecoration(
                      labelText: l10n.reportDescription,
                    ),
                  ),
                  if (_failed)
                    Padding(
                      padding: const EdgeInsets.symmetric(vertical: 12),
                      child: Text(l10n.genericError),
                    ),
                  const SizedBox(height: 16),
                  KoButton(
                    key: const Key('report-submit'),
                    label: _busy ? l10n.loadingLabel : l10n.reportSubmit,
                    color: KoColors.pink,
                    onPressed: _busy ? null : _submit,
                    expand: true,
                  ),
                  TextButton(
                    onPressed: _busy ? null : () => Navigator.pop(context),
                    child: Text(l10n.cancel),
                  ),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}
