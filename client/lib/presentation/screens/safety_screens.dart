import 'package:flutter/material.dart';
import 'package:url_launcher/url_launcher.dart';
import '../../data/api_client.dart';
import '../../data/auth_service.dart';
import '../../l10n/app_localizations.dart';
import '../widgets/ko_ui.dart';
import '../widgets/service_components.dart';

/// Accepted only from the authenticated server response, never a local checkbox.
bool safetyTermsAccepted(Map<String, dynamic> data) {
  final terms = data['terms'];
  return terms is Map<String, dynamic> &&
      terms['available'] == true &&
      terms['accepted'] == true &&
      terms['version'] is String &&
      (terms['version'] as String).isNotEmpty &&
      terms['body'] is String &&
      (terms['body'] as String).isNotEmpty;
}

Uri? _safetyLink(Object? value) {
  if (value is! String || value.isEmpty || value.length > 2048) return null;
  final uri = Uri.tryParse(value);
  return uri != null &&
          uri.scheme == 'https' &&
          uri.host.isNotEmpty &&
          uri.userInfo.isEmpty
      ? uri
      : null;
}

String safetyAccountID(Object? value) {
  if (value is! String ||
      !RegExp(
        r'^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$',
      ).hasMatch(value) ||
      value == '00000000-0000-0000-0000-000000000000') {
    throw const FormatException('Invalid public account identity');
  }
  return value;
}

/// Inline confirmation owns only a reason, never authored text or match state.
/// Its parent removes it whenever the corresponding visible source disappears.
class TextContentReport extends StatefulWidget {
  const TextContentReport({required this.onReport, super.key});
  final Future<void> Function(String reason) onReport;
  @override
  State<TextContentReport> createState() => _TextContentReportState();
}

class _TextContentReportState extends State<TextContentReport> {
  bool _open = false, _pending = false, _sent = false, _failed = false;
  String? _reason, _attemptedReason;

  Future<void> _send() async {
    final reason = _attemptedReason ?? _reason;
    if (_pending || _sent || reason == null) return;
    setState(() {
      _pending = true;
      _failed = false;
      _attemptedReason = reason;
    });
    try {
      await widget.onReport(reason);
      if (mounted) setState(() => _sent = true);
    } catch (_) {
      if (mounted) setState(() => _failed = true);
    } finally {
      if (mounted) setState(() => _pending = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    if (_sent) {
      return Text(l.textReportSent, key: const Key('text-report-success'));
    }
    if (!_open) {
      return KoButton(
        key: const Key('text-report-open'),
        label: l.textReport,
        color: KoColors.surface,
        onPressed: () => setState(() => _open = true),
      );
    }
    return KoPanel(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(l.textReportConfirm),
          const SizedBox(height: 8),
          if (_attemptedReason == null)
            Wrap(
              spacing: 8,
              runSpacing: 8,
              children: [
                for (final reason in const [
                  'content.inappropriate',
                  'content.incorrect',
                  'content.other',
                ])
                  Semantics(
                    selected: _reason == reason,
                    child: KoButton(
                      key: Key('text-report-reason-$reason'),
                      label: switch (reason) {
                        'content.inappropriate' => l.textReportInappropriate,
                        'content.incorrect' => l.textReportIncorrect,
                        _ => l.textReportOther,
                      },
                      color: _reason == reason
                          ? KoColors.lime
                          : KoColors.surface,
                      onPressed: () => setState(() => _reason = reason),
                    ),
                  ),
              ],
            ),
          if (_failed)
            Text(l.textReportFailed, key: const Key('text-report-error')),
          const SizedBox(height: 8),
          Wrap(
            spacing: 8,
            runSpacing: 8,
            children: [
              KoButton(
                key: const Key('text-report-send'),
                label: _pending
                    ? l.loadingLabel
                    : _failed
                    ? l.textRetry
                    : l.textReportSend,
                onPressed:
                    _pending || (_reason == null && _attemptedReason == null)
                    ? null
                    : _send,
              ),
              if (!_pending && _attemptedReason == null)
                KoButton(
                  key: const Key('text-report-cancel'),
                  label: l.cancel,
                  color: KoColors.surface,
                  onPressed: () => setState(() {
                    _open = false;
                    _reason = null;
                  }),
                ),
            ],
          ),
        ],
      ),
    );
  }
}

class SafetyScreen extends StatefulWidget {
  const SafetyScreen({this.api, this.openLink, super.key});
  final ApiClient? api;
  final Future<bool> Function(Uri)? openLink;
  @override
  State<SafetyScreen> createState() => _SafetyScreenState();
}

class _SafetyScreenState extends State<SafetyScreen> {
  late final _api = serviceApi(widget.api);
  Map<String, dynamic>? _data;
  Map<String, dynamic>? _help;
  List<String> _blocks = [];
  String _after = '', _next = '';
  bool _busy = false, _blocksReady = false;
  Object? _error;
  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() => _run(() async {
    _data = null;
    _blocksReady = false;
    _blocks = [];
    _after = _next = '';
    Map<String, dynamic> data;
    try {
      data = await _api.getSafety();
    } catch (_) {
      try {
        final help = await _api.getSafetyHelp();
        if (mounted) _help = help;
      } catch (_) {
        // Preserve the authenticated failure; public help cannot restore it.
      }
      rethrow;
    }
    if (!mounted) return;
    _data = data;
    _help = data;
    // Read separate bounded pages: a block list never expands without user input.
    final page = await _api.getBlocks();
    if (!mounted) return;
    _setPage(page, '');
  });
  void _setPage(Map<String, dynamic> page, String after) {
    final ids = (page['account_ids'] as List)
        .map(safetyAccountID)
        .toList(growable: false);
    final next = page['next_cursor'] as String;
    if (ids.length > 100 ||
        ids.toSet().length != ids.length ||
        (next.isNotEmpty &&
            (safetyAccountID(next) != ids.lastOrNull ||
                next.compareTo(after) <= 0))) {
      throw const FormatException('Invalid block page');
    }
    _blocksReady = true;
    _blocks = ids;
    _after = after;
    _next = next;
  }

  Future<void> _run(
    Future<void> Function() action, {
    bool clearError = true,
  }) async {
    if (_busy) return;
    setState(() {
      _busy = true;
      if (clearError) _error = null;
    });
    try {
      await action();
    } catch (e) {
      if (mounted) _error = e;
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _accept(String version) => _run(() async {
    await _api.acceptUserTerms(version);
    final fresh = await _api.getSafety();
    if (mounted) _data = fresh;
  });
  Future<void> _page(String after) => _run(() async {
    final page = await _api.getBlocks(after: after);
    if (mounted) _setPage(page, after);
  });
  Future<void> _unblock(String id) => _run(() async {
    await _api.unblockAccount(id);
    final page = await _api.getBlocks(after: _after);
    if (mounted) _setPage(page, _after);
  });
  Future<void> _open(Uri uri) => _run(() async {
    final success =
        await (widget.openLink?.call(uri) ??
            launchUrl(uri, mode: LaunchMode.externalApplication));
    if (!success) throw const ApiException(503);
  }, clearError: false);
  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final terms = _data?['terms'];
    final available =
        terms is Map &&
        terms['available'] == true &&
        terms['version'] is String &&
        (terms['version'] as String).isNotEmpty &&
        terms['body'] is String &&
        (terms['body'] as String).isNotEmpty;
    final support = _safetyLink(_help?['support_url']);
    final privacy = _safetyLink(_help?['privacy_url']);
    return KoPage(
      title: l.safetyTitle,
      actions: [
        IconButton(
          key: const Key('safety-refresh'),
          tooltip: l.serviceRefresh,
          onPressed: _busy ? null : _load,
          icon: const Icon(Icons.refresh),
        ),
      ],
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Text(l.safetyIntro),
          const SizedBox(height: 20),
          if (_error != null)
            Padding(
              padding: const EdgeInsets.only(bottom: 16),
              child: KoPanel(
                key: const Key('safety-error'),
                color: KoColors.pink,
                child: Text(
                  _error is AuthSessionException
                      ? l.safetyRestore
                      : l.genericError,
                ),
              ),
            ),
          if (_data == null)
            ServiceStatus(error: _error != null, onRetry: _busy ? null : _load)
          else
            ServiceWorkspace(
              primary: KoPanel(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    KoHeading(title: l.safetyTerms),
                    if (available) ...[
                      Text(terms['version'], textDirection: TextDirection.ltr),
                      const SizedBox(height: 12),
                      SelectableText(terms['body']),
                      const SizedBox(height: 16),
                      if (safetyTermsAccepted(_data!))
                        Text(
                          l.safetyAccepted,
                          key: const Key('safety-accepted'),
                        )
                      else
                        KoButton(
                          key: const Key('safety-accept'),
                          label: l.safetyAccept,
                          icon: const DoodleIcon(Doodle.check),
                          onPressed: _busy
                              ? null
                              : () => _accept(terms['version']),
                        ),
                    ] else
                      Text(l.safetyTermsUnavailable),
                  ],
                ),
              ),
              secondary: KoPanel(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    KoHeading(title: l.safetyBlocks),
                    Text(l.safetyBlockHint),
                    const SizedBox(height: 16),
                    if (!_blocksReady)
                      ServiceStatus(
                        error: _error != null,
                        onRetry: _busy ? null : _load,
                      )
                    else if (_blocks.isEmpty)
                      Text(l.safetyBlocksEmpty),
                    for (final id in _blocks)
                      Padding(
                        padding: const EdgeInsets.only(bottom: 16),
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.stretch,
                          children: [
                            Text(id, textDirection: TextDirection.ltr),
                            KoButton(
                              key: Key('safety-unblock-$id'),
                              label: l.safetyUnblock,
                              onPressed: _busy ? null : () => _unblock(id),
                            ),
                          ],
                        ),
                      ),
                    if (_next.isNotEmpty)
                      KoButton(
                        key: const Key('safety-blocks-next'),
                        label: l.safetyNext,
                        onPressed: _busy ? null : () => _page(_next),
                      ),
                  ],
                ),
              ),
            ),
          if (support != null || privacy != null)
            Padding(
              padding: const EdgeInsets.only(top: 20),
              child: KoPanel(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    if (support != null)
                      KoButton(
                        key: const Key('safety-support'),
                        label: l.safetySupport,
                        onPressed: _busy ? null : () => _open(support),
                      ),
                    if (privacy != null)
                      Padding(
                        padding: const EdgeInsets.only(top: 12),
                        child: KoButton(
                          key: const Key('safety-privacy'),
                          label: l.safetyPrivacy,
                          onPressed: _busy ? null : () => _open(privacy),
                        ),
                      ),
                  ],
                ),
              ),
            ),
        ],
      ),
    );
  }
}

/// Match targets resolve from immutable server admissions. A known profile may
/// pass its public account ID; neither path contains roles, hands or evidence.
class PlayerSafetyScreen extends StatefulWidget {
  const PlayerSafetyScreen({
    required this.api,
    this.matchID,
    this.roomID,
    this.seat,
    this.accountID,
    this.nickname,
    super.key,
  }) : assert(
         ((matchID != null) != (roomID != null)) &&
                 seat != null &&
                 accountID == null ||
             (matchID == null &&
                 roomID == null &&
                 seat == null &&
                 accountID != null),
       );
  final ApiClient api;
  final String? matchID, roomID, accountID, nickname;
  final int? seat;
  @override
  State<PlayerSafetyScreen> createState() => _PlayerSafetyScreenState();
}

class _PlayerSafetyScreenState extends State<PlayerSafetyScreen> {
  late final Future<Map<String, dynamic>> _identity = _load();
  bool _busy = false, _blocked = false, _blockAttempted = false;
  Object? _error;
  Future<Map<String, dynamic>> _load() async {
    final data = widget.accountID == null
        ? widget.roomID != null
              ? await widget.api.getRoomSeatIdentity(
                  widget.roomID!,
                  widget.seat!,
                )
              : await widget.api.getMatchSeatIdentity(
                  widget.matchID!,
                  widget.seat!,
                )
        : {'account_id': widget.accountID, 'nickname': widget.nickname ?? ''};
    safetyAccountID(data['account_id']);
    if (data['nickname'] is! String) {
      throw const FormatException('Invalid public profile');
    }
    return data;
  }

  Future<void> _block(String id) async {
    if (_busy) return;
    setState(() {
      _busy = true;
      // The server may commit even if its response is lost. Refresh visibility
      // on return without presenting an uncertain result as confirmed success.
      _blockAttempted = true;
      _error = null;
    });
    try {
      await widget.api.blockAccount(id);
      if (mounted) setState(() => _blocked = true);
    } catch (e) {
      if (mounted) setState(() => _error = e);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    return PopScope(
      canPop: false,
      onPopInvokedWithResult: (didPop, _) {
        if (!didPop && !_busy) Navigator.of(context).pop(_blockAttempted);
      },
      child: KoPage(
        title: l.safetyTitle,
        showBack: !_busy,
        child: FutureBuilder<Map<String, dynamic>>(
          future: _identity,
          builder: (context, snapshot) {
            if (!snapshot.hasData) {
              return ServiceStatus(error: snapshot.hasError);
            }
            final id = snapshot.data!['account_id'] as String;
            return KoPanel(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  Text(
                    snapshot.data!['nickname'],
                    style: koDisplayStyle(size: 30),
                  ),
                  Text(id, textDirection: TextDirection.ltr),
                  const SizedBox(height: 20),
                  Text(l.safetyBlockHint),
                  const SizedBox(height: 20),
                  if (_error != null)
                    Text(l.genericError, key: const Key('safety-error')),
                  if (_blocked)
                    Text(l.safetyBlocked, key: const Key('safety-blocked'))
                  else
                    KoButton(
                      key: const Key('safety-block'),
                      label: _busy ? l.loadingLabel : l.safetyBlock,
                      color: KoColors.pink,
                      onPressed: _busy ? null : () => _block(id),
                    ),
                  const SizedBox(height: 16),
                  KoButton(
                    key: const Key('safety-report'),
                    label: l.reportTitle,
                    onPressed: _busy
                        ? null
                        : () async {
                            await showDialog<bool>(
                              context: context,
                              builder: (_) => ServiceReportDialog(
                                api: widget.api,
                                reportType: 'conduct',
                                accountId: id,
                              ),
                            );
                          },
                  ),
                ],
              ),
            );
          },
        ),
      ),
    );
  }
}
