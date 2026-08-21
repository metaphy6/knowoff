import 'package:flutter/material.dart';

import '../../core/config/app_config.dart';
import '../../data/api_client.dart';
import '../../l10n/app_localizations.dart';

/// Dialog to submit a player or media report.
class ReportDialog extends StatefulWidget {
  const ReportDialog({
    this.targetAccountID,
    this.targetMediaID,
    this.api,
    super.key,
  }) : assert(
          targetAccountID != null || targetMediaID != null,
          'A report must target an account or a media asset.',
        );

  final String? targetAccountID;
  final String? targetMediaID;
  final ApiClient? api;

  @override
  State<ReportDialog> createState() => _ReportDialogState();
}

class _ReportDialogState extends State<ReportDialog> {
  final _reasonController = TextEditingController();
  final _descriptionController = TextEditingController();
  late final ApiClient _api = widget.api ??
      ApiClient(
        baseUrl: AppConfig.instance.serverUrl,
        auth: AppConfig.instance.authService,
      );
  String _reportType = 'conduct';
  bool _submitting = false;

  @override
  void dispose() {
    _reasonController.dispose();
    _descriptionController.dispose();
    super.dispose();
  }

  Future<void> _submit(AppLocalizations l10n) async {
    setState(() => _submitting = true);
    try {
      await _api.createReport(
        reportType: _reportType,
        targetAccountID: widget.targetAccountID,
        targetMediaID: widget.targetMediaID,
        reason: _reasonController.text.trim(),
        description: _descriptionController.text.trim().isEmpty
            ? null
            : _descriptionController.text.trim(),
      );
      if (mounted) {
        Navigator.of(context).pop();
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text(l10n.reportSubmit)),
        );
      }
    } catch (e) {
      if (mounted) {
        setState(() => _submitting = false);
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text(l10n.genericError)),
        );
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    return AlertDialog(
      title: Text(l10n.reportTitle),
      content: SingleChildScrollView(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            DropdownButtonFormField<String>(
              initialValue: _reportType,
              decoration: InputDecoration(labelText: l10n.reportType),
              items: [
                DropdownMenuItem(
                  value: 'conduct',
                  child: Text(l10n.reportTypeConduct),
                ),
                DropdownMenuItem(
                  value: 'media',
                  child: Text(l10n.reportTypeMedia),
                ),
                DropdownMenuItem(
                  value: 'cheating',
                  child: Text(l10n.reportTypeCheating),
                ),
                DropdownMenuItem(
                  value: 'harassment',
                  child: Text(l10n.reportTypeHarassment),
                ),
              ],
              onChanged: _submitting
                  ? null
                  : (value) {
                      if (value != null) {
                        setState(() => _reportType = value);
                      }
                    },
            ),
            TextField(
              controller: _reasonController,
              decoration: InputDecoration(labelText: l10n.reportReason),
              enabled: !_submitting,
            ),
            TextField(
              controller: _descriptionController,
              decoration: InputDecoration(labelText: l10n.reportDescription),
              minLines: 2,
              maxLines: 4,
              enabled: !_submitting,
            ),
          ],
        ),
      ),
      actions: [
        TextButton(
          onPressed: _submitting ? null : () => Navigator.of(context).pop(),
          child: Text(l10n.cancel),
        ),
        TextButton(
          onPressed: _submitting ? null : () => _submit(l10n),
          child: _submitting
              ? const SizedBox(
                  width: 16,
                  height: 16,
                  child: CircularProgressIndicator(strokeWidth: 2),
                )
              : Text(l10n.reportSubmit),
        ),
      ],
    );
  }
}
