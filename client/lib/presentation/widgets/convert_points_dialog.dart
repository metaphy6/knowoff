import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../core/config/app_config.dart';
import '../../data/api_client.dart';
import '../../l10n/app_localizations.dart';

/// Dialog that converts Non-Converted Points into Noin at the configured rate.
class ConvertPointsDialog extends StatefulWidget {
  const ConvertPointsDialog({
    required this.pointsToNoin,
    this.api,
    super.key,
  });

  final int pointsToNoin;
  final ApiClient? api;

  @override
  State<ConvertPointsDialog> createState() => _ConvertPointsDialogState();
}

class _ConvertPointsDialogState extends State<ConvertPointsDialog> {
  final _controller = TextEditingController();
  late final ApiClient _api = widget.api ??
      ApiClient(
        baseUrl: AppConfig.instance.serverUrl,
        auth: AppConfig.instance.authService,
      );
  bool _converting = false;
  String? _error;
  int? _resultNoin;

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  Future<void> _submit(AppLocalizations l10n) async {
    final text = _controller.text.trim();
    final points = int.tryParse(text);
    if (points == null || points <= 0 || points % widget.pointsToNoin != 0) {
      setState(() => _error = l10n.convertPointsMultiple(widget.pointsToNoin));
      return;
    }
    setState(() {
      _converting = true;
      _error = null;
      _resultNoin = null;
    });
    try {
      final res = await _api.convertPoints(points);
      final noin = (res['noin_granted'] as num?)?.toInt() ??
          (points ~/ widget.pointsToNoin);
      setState(() {
        _resultNoin = noin;
        _converting = false;
      });
    } catch (e) {
      setState(() {
        _error = l10n.genericError;
        _converting = false;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    return AlertDialog(
      title: Text(l10n.convertPointsTitle),
      content: SingleChildScrollView(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            if (_resultNoin == null)
              TextField(
                controller: _controller,
                keyboardType: TextInputType.number,
                inputFormatters: [FilteringTextInputFormatter.digitsOnly],
                decoration: InputDecoration(
                  hintText: l10n.convertPointsHint,
                  errorText: _error,
                ),
                enabled: !_converting,
              )
            else
              Text(l10n.convertPointsResult(_resultNoin!)),
          ],
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.of(context).pop(),
          child: Text(_resultNoin != null ? l10n.close : l10n.cancel),
        ),
        if (_resultNoin == null)
          TextButton(
            onPressed: _converting ? null : () => _submit(l10n),
            child: _converting
                ? const SizedBox(
                    width: 16,
                    height: 16,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  )
                : Text(l10n.convertPointsConvert),
          ),
      ],
    );
  }
}
