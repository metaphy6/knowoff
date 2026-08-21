import 'package:flutter/material.dart';

import '../../core/config/app_config.dart';
import '../../data/api_client.dart';
import '../../l10n/app_localizations.dart';

/// Dialog to submit bug reports, ideas, or general feedback.
class FeedbackDialog extends StatefulWidget {
  const FeedbackDialog({this.api, super.key});

  final ApiClient? api;

  @override
  State<FeedbackDialog> createState() => _FeedbackDialogState();
}

class _FeedbackDialogState extends State<FeedbackDialog> {
  final _titleController = TextEditingController();
  final _messageController = TextEditingController();
  late final ApiClient _api = widget.api ??
      ApiClient(
        baseUrl: AppConfig.instance.serverUrl,
        auth: AppConfig.instance.authService,
      );
  String _type = 'bug';
  bool _submitting = false;

  @override
  void dispose() {
    _titleController.dispose();
    _messageController.dispose();
    super.dispose();
  }

  Future<void> _submit(AppLocalizations l10n) async {
    setState(() => _submitting = true);
    try {
      await _api.createFeedback(
        type: _type,
        title: _titleController.text.trim(),
        message: _messageController.text.trim(),
      );
      if (mounted) {
        Navigator.of(context).pop();
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text(l10n.feedbackSubmit)),
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
      title: Text(l10n.feedbackTitle),
      content: SingleChildScrollView(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            DropdownButtonFormField<String>(
              initialValue: _type,
              decoration: InputDecoration(labelText: l10n.feedbackType),
              items: [
                DropdownMenuItem(
                  value: 'bug',
                  child: Text(l10n.feedbackTypeBug),
                ),
                DropdownMenuItem(
                  value: 'idea',
                  child: Text(l10n.feedbackTypeIdea),
                ),
                DropdownMenuItem(
                  value: 'other',
                  child: Text(l10n.feedbackTypeOther),
                ),
              ],
              onChanged: _submitting
                  ? null
                  : (value) {
                      if (value != null) {
                        setState(() => _type = value);
                      }
                    },
            ),
            TextField(
              controller: _titleController,
              decoration: InputDecoration(hintText: l10n.feedbackTitleHint),
              enabled: !_submitting,
            ),
            TextField(
              controller: _messageController,
              decoration: InputDecoration(labelText: l10n.feedbackMessage),
              minLines: 3,
              maxLines: 6,
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
              : Text(l10n.feedbackSubmit),
        ),
      ],
    );
  }
}
