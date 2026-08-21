import 'package:flutter/material.dart';

import '../../core/config/app_config.dart';
import '../../data/api_client.dart';
import '../../l10n/app_localizations.dart';

/// Bottom sheet that accepts a picked image and uploads it as a custom avatar.
///
/// [onPickImage] is test-only hook that returns raw image bytes and a filename.
/// In production this would be wired to an image picker plugin; the file picker
/// itself is intentionally not imported here so the widget stays testable and
/// dart:io-free.
class AvatarUploadSheet extends StatefulWidget {
  const AvatarUploadSheet({this.onPickImage, this.api, super.key});

  final Future<(List<int>, String)?> Function()? onPickImage;
  final ApiClient? api;

  @override
  State<AvatarUploadSheet> createState() => _AvatarUploadSheetState();
}

class _AvatarUploadSheetState extends State<AvatarUploadSheet> {
  late final ApiClient _api = widget.api ??
      ApiClient(
        baseUrl: AppConfig.instance.serverUrl,
        auth: AppConfig.instance.authService,
      );
  List<int>? _bytes;
  String? _filename;
  bool _uploading = false;

  Future<void> _pick(AppLocalizations l10n) async {
    final picked = await widget.onPickImage?.call();
    if (!mounted) return;
    if (picked != null) {
      setState(() {
        _bytes = picked.$1;
        _filename = picked.$2;
      });
    } else {
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(l10n.genericError)),
      );
    }
  }

  Future<void> _upload(AppLocalizations l10n) async {
    final bytes = _bytes;
    final filename = _filename;
    if (bytes == null || filename == null) return;
    setState(() => _uploading = true);
    try {
      await _api.uploadAvatar(bytes, filename);
      if (mounted) {
        Navigator.of(context).pop();
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text(l10n.avatarUploadSubmit)),
        );
      }
    } catch (e) {
      if (mounted) {
        setState(() => _uploading = false);
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text(l10n.genericError)),
        );
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    return Padding(
      padding: const EdgeInsets.all(24),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(
            l10n.avatarUploadTitle,
            style: Theme.of(context).textTheme.titleLarge,
          ),
          const SizedBox(height: 16),
          ElevatedButton(
            onPressed: _uploading ? null : () => _pick(l10n),
            child: Text(l10n.avatarUploadPick),
          ),
          if (_filename != null) ...[
            const SizedBox(height: 8),
            Text(_filename!),
          ],
          const SizedBox(height: 16),
          ElevatedButton(
            onPressed:
                (_bytes == null || _uploading) ? null : () => _upload(l10n),
            child: _uploading
                ? const SizedBox(
                    width: 16,
                    height: 16,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  )
                : Text(l10n.avatarUploadSubmit),
          ),
        ],
      ),
    );
  }
}
