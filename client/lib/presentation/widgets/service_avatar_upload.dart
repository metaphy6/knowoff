import 'dart:typed_data';
import 'dart:ui' as ui;

import 'package:file_selector/file_selector.dart';
import 'package:flutter/material.dart';

import '../../data/api_client.dart';
import '../../l10n/app_localizations.dart';
import 'ko_ui.dart';
import 'service_components.dart';

/// The native/web picker is replaceable in tests; all selected bytes are bounded
/// before reading and all image dimensions before decoding a preview.
class ServiceAvatarUpload extends StatefulWidget {
  const ServiceAvatarUpload(
      {required this.api, this.onUploaded, this.pickImage, super.key});
  final ApiClient api;
  final VoidCallback? onUploaded;
  final Future<XFile?> Function()? pickImage;
  @override
  State<ServiceAvatarUpload> createState() => _ServiceAvatarUploadState();
}

class _ServiceAvatarUploadState extends State<ServiceAvatarUpload> {
  static const _maxBytes = 2 * 1024 * 1024;
  Uint8List? _bytes;
  String? _filename;
  int? _price;
  bool _busy = false;
  String? _error;
  Future<void> _pick() async {
    if (_busy) return;
    final l10n = AppLocalizations.of(context);
    setState(() {
      _busy = true;
      _error = null;
      _bytes = null;
    });
    try {
      final file = await (widget.pickImage?.call() ??
          openFile(acceptedTypeGroups: [
            const XTypeGroup(
              label: 'images',
              extensions: ['jpg', 'jpeg', 'png'],
              uniformTypeIdentifiers: ['public.jpeg', 'public.png'],
            )
          ]));
      if (file == null || !mounted) return;
      final extension = file.name.split('.').last.toLowerCase();
      if (!['jpg', 'jpeg', 'png'].contains(extension) ||
          await file.length() > _maxBytes) {
        throw const FormatException();
      }
      final bytes = await file.readAsBytes();
      if (bytes.isEmpty || bytes.length > _maxBytes) {
        throw const FormatException();
      }
      final buffer = await ui.ImmutableBuffer.fromUint8List(bytes);
      try {
        final descriptor = await ui.ImageDescriptor.encoded(buffer);
        try {
          if (descriptor.width > 2048 || descriptor.height > 2048) {
            throw const FormatException();
          }
        } finally {
          descriptor.dispose();
        }
      } finally {
        buffer.dispose();
      }
      final catalog = await widget.api.getStoreCatalog();
      final price = (catalog['unlock_prices'] as Map?)?['custom_avatar'];
      if (price is! num || price < 0) throw const FormatException();
      if (!mounted) return;
      setState(() {
        _bytes = bytes;
        _filename = file.name;
        _price = price.toInt();
      });
    } on FormatException {
      if (mounted) setState(() => _error = l10n.serviceAvatarInvalid);
    } catch (_) {
      if (mounted) setState(() => _error = l10n.genericError);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _upload() async {
    final bytes = _bytes;
    final filename = _filename;
    if (_busy || bytes == null || filename == null) return;
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await widget.api.uploadAvatar(bytes, filename);
      if (!mounted) return;
      setState(() {
        _bytes = null;
        _filename = null;
      });
      serviceMessage(
          context, AppLocalizations.of(context).serviceAvatarSubmitted);
      widget.onUploaded?.call();
    } catch (_) {
      if (mounted) {
        setState(() => _error = AppLocalizations.of(context).genericError);
      }
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    return KoPanel(
        color: KoColors.aqua,
        child:
            Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
          KoHeading(title: l10n.avatarUploadTitle),
          Text(l10n.serviceAvatarLimits),
          const SizedBox(height: 10),
          Text(l10n.serviceAvatarCost),
          const SizedBox(height: 8),
          Text(l10n.serviceAvatarReview),
          const SizedBox(height: 16),
          KoButton(
              key: const Key('avatar-pick'),
              label: l10n.avatarUploadPick,
              onPressed: _busy ? null : _pick),
          if (_bytes != null) ...[
            const SizedBox(height: 16),
            Align(
                alignment: Alignment.centerLeft,
                child: Image.memory(_bytes!,
                    width: 112, height: 112, fit: BoxFit.cover)),
            const SizedBox(height: 12),
            Text(_filename ?? ''),
            Text(l10n.serviceAvatarCostPrice(_price!)),
            const SizedBox(height: 16),
            KoButton(
                key: const Key('avatar-upload'),
                label: l10n.avatarUploadSubmit,
                color: KoColors.lime,
                onPressed: _busy ? null : _upload),
          ],
          if (_busy) ...[const SizedBox(height: 12), Text(l10n.loadingLabel)],
          if (_error != null) ...[const SizedBox(height: 12), Text(_error!)],
        ]));
  }
}
