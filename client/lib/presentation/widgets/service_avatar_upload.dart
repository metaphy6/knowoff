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
  const ServiceAvatarUpload({
    required this.api,
    this.onUploaded,
    this.pickImage,
    super.key,
  });
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
  bool _owned = false;
  bool _available = false;
  bool _confirmUnlock = false;
  String? _selectedAccount;
  int _generation = 0;
  @override
  void didUpdateWidget(covariant ServiceAvatarUpload oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.api != widget.api) {
      _generation++;
      _bytes = null;
      _filename = null;
      _busy = false;
      _confirmUnlock = false;
    }
  }

  @override
  void dispose() {
    _generation++;
    super.dispose();
  }

  bool _current(int generation, String? account) =>
      mounted &&
      generation == _generation &&
      account == widget.api.authService.accountId;
  String _failure(Object error) {
    final l = AppLocalizations.of(context);
    if (error is ApiException) {
      return switch (error.code) {
        'avatar.unavailable' ||
        'avatar.busy' ||
        'store.catalog_unavailable' => l.serviceAvatarUnavailable,
        'avatar.flagged' => l.serviceAvatarRejected,
        'avatar.invalid' => l.serviceAvatarInvalid,
        'avatar.stale' ||
        'avatar.account_unavailable' ||
        'auth.required' => l.serviceAvatarChanged,
        _ => l.genericError,
      };
    }
    return l.genericError;
  }

  bool _busy = false;
  String? _error;
  Future<void> _pick() async {
    if (_busy) return;
    final l10n = AppLocalizations.of(context);
    final generation = ++_generation;
    final api = widget.api;
    final initialAccount = api.authService.accountId;
    setState(() {
      _busy = true;
      _error = null;
      _bytes = null;
      _confirmUnlock = false;
    });
    try {
      final file =
          await (widget.pickImage?.call() ??
              openFile(
                acceptedTypeGroups: [
                  const XTypeGroup(
                    label: 'images',
                    extensions: ['jpg', 'jpeg', 'png'],
                    uniformTypeIdentifiers: ['public.jpeg', 'public.png'],
                  ),
                ],
              ));
      if (file == null || !mounted || generation != _generation) return;
      final extension = file.name.split('.').last.toLowerCase();
      if (!['jpg', 'jpeg', 'png'].contains(extension) ||
          await file.length() > _maxBytes) {
        throw const FormatException();
      }
      if (!mounted || generation != _generation) return;
      final bytes = await file.readAsBytes();
      if (!mounted || generation != _generation) return;
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
      if (!mounted || generation != _generation) return;
      if (initialAccount != null &&
          initialAccount != api.authService.accountId) {
        throw const ApiException(401, code: 'auth.required');
      }
      final catalog = await api.getStoreCatalog();
      final price = (catalog['unlock_prices'] as Map?)?['custom_avatar'];
      if (price is! int || price <= 0) throw const FormatException();
      if (!mounted || generation != _generation) return;
      if (initialAccount != null &&
          initialAccount != widget.api.authService.accountId) {
        throw const ApiException(401, code: 'auth.required');
      }
      setState(() {
        _selectedAccount = widget.api.authService.accountId;
        _owned = catalog['custom_avatar_owned'] == true;
        _available = catalog['custom_avatar_available'] == true;
        _bytes = bytes;
        _filename = file.name;
        _price = price.toInt();
      });
    } on FormatException {
      if (mounted && generation == _generation) {
        setState(() => _error = l10n.serviceAvatarInvalid);
      }
    } catch (error) {
      if (mounted && generation == _generation) {
        setState(() => _error = _failure(error));
      }
    } finally {
      if (mounted && generation == _generation) setState(() => _busy = false);
    }
  }

  Future<void> _upload() async {
    final bytes = _bytes;
    final filename = _filename;
    if (_busy || !_owned || !_available || bytes == null || filename == null) {
      return;
    }
    final generation = _generation;
    final account = _selectedAccount;
    if (!_current(generation, account)) {
      setState(
        () => _error = AppLocalizations.of(context).serviceAvatarChanged,
      );
      return;
    }
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await widget.api.uploadAvatar(bytes, filename);
      if (!mounted || !_current(generation, account)) return;
      setState(() {
        _bytes = null;
        _filename = null;
      });
      serviceMessage(
        context,
        AppLocalizations.of(context).serviceAvatarSubmitted,
      );
      widget.onUploaded?.call();
    } catch (error) {
      if (_current(generation, account)) {
        setState(() => _error = _failure(error));
      }
    } finally {
      if (mounted && generation == _generation) setState(() => _busy = false);
    }
  }

  Future<void> _purchase() async {
    if (_busy || _owned || !_available || !_confirmUnlock) return;
    final generation = _generation;
    final account = _selectedAccount;
    if (!_current(generation, account)) {
      setState(
        () => _error = AppLocalizations.of(context).serviceAvatarChanged,
      );
      return;
    }
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await widget.api.purchaseUnlock('custom_avatar');
      if (!_current(generation, account)) return;
      setState(() {
        _owned = true;
        _confirmUnlock = false;
      });
    } catch (error) {
      if (_current(generation, account)) {
        setState(() => _error = _failure(error));
      }
    } finally {
      if (mounted && generation == _generation) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    return KoPanel(
      color: KoColors.aqua,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
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
            onPressed: _busy ? null : _pick,
          ),
          if (_bytes != null) ...[
            const SizedBox(height: 16),
            Align(
              alignment: Alignment.centerLeft,
              child: Image.memory(
                _bytes!,
                width: 112,
                height: 112,
                fit: BoxFit.cover,
              ),
            ),
            const SizedBox(height: 12),
            Text(_filename ?? ''),
            Text(
              _owned
                  ? l10n.serviceAvatarOwned
                  : l10n.serviceAvatarCostPrice(_price!),
            ),
            if (!_available) Text(l10n.serviceAvatarUnavailable),
            if (_available && !_owned && !_confirmUnlock)
              KoButton(
                key: const Key('avatar-unlock'),
                label: l10n.serviceAvatarUnlock,
                onPressed: _busy
                    ? null
                    : () => setState(() => _confirmUnlock = true),
              ),
            if (_available && !_owned && _confirmUnlock) ...[
              KoButton(
                key: const Key('avatar-unlock-confirm'),
                label: l10n.serviceAvatarUnlockConfirm,
                onPressed: _busy ? null : _purchase,
              ),
              const SizedBox(height: 8),
              KoButton(
                key: const Key('avatar-unlock-cancel'),
                label: l10n.cancel,
                onPressed: _busy
                    ? null
                    : () => setState(() => _confirmUnlock = false),
              ),
            ],
            const SizedBox(height: 16),
            if (_owned && _available)
              KoButton(
                key: const Key('avatar-upload'),
                label: l10n.avatarUploadSubmit,
                color: KoColors.lime,
                onPressed: _busy ? null : _upload,
              ),
          ],
          if (_busy) ...[const SizedBox(height: 12), Text(l10n.loadingLabel)],
          if (_error != null) ...[const SizedBox(height: 12), Text(_error!)],
        ],
      ),
    );
  }
}

/// Approved image bytes belong to one viewer, target and revision. A removed
/// image or changed identity immediately falls back to the curated preset.
class ServiceProfileAvatar extends StatefulWidget {
  const ServiceProfileAvatar({
    required this.api,
    required this.accountId,
    required this.revision,
    super.key,
  });
  final ApiClient api;
  final String accountId;
  final int revision;
  @override
  State<ServiceProfileAvatar> createState() => _ServiceProfileAvatarState();
}

class _ServiceProfileAvatarState extends State<ServiceProfileAvatar> {
  Uint8List? _bytes;
  String? _viewer;
  int _generation = 0;
  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void didUpdateWidget(covariant ServiceProfileAvatar oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.api != widget.api ||
        oldWidget.accountId != widget.accountId ||
        oldWidget.revision != widget.revision ||
        _viewer != widget.api.authService.accountId) {
      _load();
    }
  }

  void _load() {
    final generation = ++_generation;
    _viewer = widget.api.authService.accountId;
    _bytes = null;
    widget.api
        .getAvatarImage(widget.accountId)
        .then(
          (bytes) {
            if (mounted &&
                generation == _generation &&
                _viewer == widget.api.authService.accountId) {
              setState(() => _bytes = bytes);
            }
          },
          onError: (Object error, StackTrace trace) {
            if (mounted && generation == _generation) {
              setState(() => _bytes = null);
            }
          },
        );
  }

  @override
  void dispose() {
    _generation++;
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    const fallback = DoodleIcon(
      Doodle.person,
      key: Key('profile-avatar'),
      size: 60,
    );
    final bytes = _bytes;
    if (bytes == null || _viewer != widget.api.authService.accountId) {
      return fallback;
    }
    return Image.memory(
      bytes,
      key: const Key('profile-custom-avatar'),
      width: 60,
      height: 60,
      fit: BoxFit.cover,
      gaplessPlayback: false,
      errorBuilder: (_, __, ___) => fallback,
    );
  }
}
