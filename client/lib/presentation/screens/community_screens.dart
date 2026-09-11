import 'dart:async';
import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:url_launcher/url_launcher.dart';

import '../../data/api_client.dart';
import '../../l10n/app_localizations.dart';
import '../widgets/ko_ui.dart';
import '../widgets/service_components.dart';

/// Approves a short-lived browser request without exposing the player token.
class ContributorConnectScreen extends StatefulWidget {
  const ContributorConnectScreen({this.api, this.openBrowser, super.key});
  final ApiClient? api;
  final Future<bool> Function(Uri)? openBrowser;
  @override
  State<ContributorConnectScreen> createState() =>
      _ContributorConnectScreenState();
}

class _ContributorConnectScreenState extends State<ContributorConnectScreen> {
  late final _api = serviceApi(widget.api);
  final _form = GlobalKey<FormState>();
  final _code = TextEditingController();
  bool _confirmed = false;
  bool _busy = false;
  bool _connected = false;
  bool _showConfirmationError = false;
  String? _error;

  @override
  void dispose() {
    _code.dispose();
    super.dispose();
  }

  String get _normalized =>
      _code.text.trim().replaceAll(RegExp(r'[- ]'), '').toUpperCase();

  Future<void> _open() async {
    final l = AppLocalizations.of(context);
    try {
      final opened = await (widget.openBrowser?.call(_api.portalLoginUri) ??
          launchUrl(_api.portalLoginUri, mode: LaunchMode.externalApplication));
      if (!opened && mounted) setState(() => _error = l.portalOpenFailed);
    } catch (_) {
      if (mounted) setState(() => _error = l.portalOpenFailed);
    }
  }

  Future<void> _connect() async {
    if (_busy) return;
    final valid = _form.currentState!.validate();
    setState(() => _showConfirmationError = !_confirmed);
    if (!valid || !_confirmed) return;
    final l = AppLocalizations.of(context);
    final code = _normalized;
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await _api.connectPortal('${code.substring(0, 4)}-${code.substring(4)}');
      if (mounted) setState(() => _connected = true);
    } catch (error) {
      if (mounted) {
        setState(() => _error = switch (error) {
              ApiException(statusCode: 400) => l.portalExpired,
              ApiException(statusCode: 403) => l.portalForbidden,
              ApiException(statusCode: 429) => l.portalRateLimited,
              _ => l.genericError,
            });
      }
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    return KoPage(
      title: l.portalTitle,
      accent: KoColors.aqua,
      maxWidth: 1000,
      child: ServiceWorkspace(
        primary:
            Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
          const DoodleIcon(Doodle.sparkle, size: 64),
          const SizedBox(height: 18),
          Text(l.portalIntro, style: koDisplayStyle(size: 36)),
          const SizedBox(height: 16),
          Text(l.portalBrowserHint),
          const SizedBox(height: 18),
          SelectableText(_api.portalLoginUri.toString()),
          const SizedBox(height: 16),
          KoButton(
              key: const Key('portal-open'),
              label: l.portalOpen,
              icon: const Icon(Icons.open_in_new),
              onPressed: _open),
        ]),
        secondary: KoPanel(
          color: _connected ? KoColors.lime : KoColors.surface,
          child: _connected
              ? Semantics(
                  liveRegion: true,
                  child: Text(l.portalConnected,
                      key: const Key('portal-connected'),
                      style: koDisplayStyle(size: 26)))
              : Form(
                  key: _form,
                  child: Column(
                      crossAxisAlignment: CrossAxisAlignment.stretch,
                      children: [
                        TextFormField(
                          key: const Key('portal-code'),
                          controller: _code,
                          enabled: !_busy,
                          autocorrect: false,
                          enableSuggestions: false,
                          textCapitalization: TextCapitalization.characters,
                          decoration: InputDecoration(
                              labelText: l.portalCode, hintText: 'ABCD-EFGH'),
                          validator: (_) =>
                              RegExp(r'^[A-Z2-7]{8}$').hasMatch(_normalized)
                                  ? null
                                  : l.portalCodeInvalid,
                          onFieldSubmitted: (_) => _connect(),
                        ),
                        const SizedBox(height: 18),
                        Material(
                          type: MaterialType.transparency,
                          child: CheckboxListTile(
                            key: const Key('portal-confirm-browser'),
                            value: _confirmed,
                            contentPadding: EdgeInsets.zero,
                            controlAffinity: ListTileControlAffinity.leading,
                            title: Text(l.portalConfirmBrowser),
                            onChanged: _busy
                                ? null
                                : (value) => setState(() {
                                      _confirmed = value ?? false;
                                      _showConfirmationError = false;
                                    }),
                          ),
                        ),
                        if (_showConfirmationError)
                          Text(l.portalConfirmBrowser,
                              style:
                                  const TextStyle(fontWeight: FontWeight.bold)),
                        const SizedBox(height: 18),
                        KoButton(
                            key: const Key('portal-connect'),
                            label: _busy ? l.loadingLabel : l.portalConnect,
                            onPressed: _busy ? null : _connect),
                        if (_error != null) ...[
                          const SizedBox(height: 16),
                          Semantics(liveRegion: true, child: Text(_error!)),
                        ],
                      ])),
        ),
      ),
    );
  }
}

class WeeklyChallengeScreen extends StatefulWidget {
  const WeeklyChallengeScreen({this.api, super.key});
  final ApiClient? api;
  @override
  State<WeeklyChallengeScreen> createState() => _WeeklyChallengeScreenState();
}

class _WeeklyChallengeScreenState extends State<WeeklyChallengeScreen>
    with WidgetsBindingObserver {
  late final _api = serviceApi(widget.api);
  final _content = TextEditingController();
  final _form = GlobalKey<FormState>();
  Map<String, dynamic>? _data;
  Timer? _refresh;
  bool _loading = false;
  bool _busy = false;
  bool _accepted = false;
  bool _consentError = false;
  bool _active = true;
  bool _loadFailed = false;
  bool _loadForbidden = false;
  int _loadEpoch = 0;
  String? _actionError;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    _load();
    _refresh = Timer.periodic(const Duration(seconds: 15), (_) {
      if (mounted &&
          _active &&
          !_busy &&
          ModalRoute.of(context)?.isCurrent == true &&
          // The replacement valuesOf API is absent from supported Flutter 3.24.
          // ignore: deprecated_member_use
          TickerMode.of(context)) {
        _load();
      }
    });
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    _active = state == AppLifecycleState.resumed;
    if (_active && !_busy) _load();
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    _refresh?.cancel();
    _content.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    if (_loading || _busy) return;
    final epoch = ++_loadEpoch;
    setState(() => _loading = true);
    try {
      final data = await _api.getActiveChallenge();
      if (!mounted || epoch != _loadEpoch) return;
      setState(() {
        if (_data?['terms']?['version'] != data['terms']?['version']) {
          _accepted = false;
        }
        _data = data;
        _loadFailed = false;
        _loadForbidden = false;
      });
    } catch (error) {
      if (mounted && epoch == _loadEpoch) {
        setState(() {
          _loadFailed = true;
          if (error is ApiException && error.code == 'challenge_forbidden') {
            _loadForbidden = true;
          }
        });
      }
    } finally {
      if (mounted && epoch == _loadEpoch) setState(() => _loading = false);
    }
  }

  Future<bool> _confirm(
      {required String title,
      required String warning,
      required String action,
      String? content}) async {
    final l = AppLocalizations.of(context);
    return await showDialog<bool>(
            context: context,
            builder: (context) => Dialog(
                  child: SingleChildScrollView(
                      child: Padding(
                          padding: const EdgeInsets.all(22),
                          child: ConstrainedBox(
                            constraints: const BoxConstraints(maxWidth: 460),
                            child: Column(
                                mainAxisSize: MainAxisSize.min,
                                crossAxisAlignment: CrossAxisAlignment.stretch,
                                children: [
                                  Text(title, style: koDisplayStyle(size: 28)),
                                  const SizedBox(height: 16),
                                  Text(warning),
                                  if (content != null) ...[
                                    const SizedBox(height: 18),
                                    Text(content,
                                        style: const TextStyle(
                                            fontWeight: FontWeight.bold))
                                  ],
                                  const SizedBox(height: 24),
                                  KoButton(
                                      key: const Key('challenge-confirm'),
                                      label: action,
                                      onPressed: () =>
                                          Navigator.pop(context, true)),
                                  const SizedBox(height: 12),
                                  KoButton(
                                      label: l.cancel,
                                      color: KoColors.surface,
                                      onPressed: () =>
                                          Navigator.pop(context, false)),
                                ]),
                          ))),
                )) ??
        false;
  }

  Future<void> _submit() async {
    if (_busy || _data == null) return;
    final valid = _form.currentState!.validate();
    setState(() => _consentError = !_accepted);
    if (!valid || !_accepted) return;
    final l = AppLocalizations.of(context);
    final topic = _data!['topic']['id'] as String;
    final terms = _data!['terms']['version'] as String;
    final content = _content.text.trim();
    if (!await _confirm(
            title: l.challengeSubmitConfirm,
            warning: l.challengeSubmitWarning,
            action: l.challengeFinalSubmit,
            content: content) ||
        !mounted) {
      return;
    }
    await _mutate(() async {
      final result = await _api.submitChallengeEntry(
          topicId: topic, content: content, termsVersion: terms);
      if (mounted) {
        setState(() {
          _data!['own_entry'] = result['entry'];
          _data!['can_submit'] = false;
        });
      }
    });
  }

  Future<void> _vote(Map<String, dynamic> entry) async {
    if (_busy || _data == null) return;
    final l = AppLocalizations.of(context);
    final topic = _data!['topic']['id'] as String;
    final id = entry['id'] as String;
    if (!await _confirm(
            title: l.challengeVoteConfirm,
            warning: l.challengeVoteWarning,
            action: l.challengeFinalVote,
            content: entry['content'] as String?) ||
        !mounted) {
      return;
    }
    await _mutate(() async {
      await _api.voteChallenge(topicId: topic, entryId: id);
      if (mounted) {
        setState(() {
          _data!['voted_entry_id'] = id;
          _data!['can_vote'] = false;
        });
      }
    });
  }

  Future<void> _mutate(Future<void> Function() action) async {
    if (_busy) return;
    final l = AppLocalizations.of(context);
    setState(() {
      _busy = true;
      // A read started before this write must not restore obsolete permissions.
      _loadEpoch++;
      _loading = false;
      _actionError = null;
    });
    try {
      await action();
    } catch (error) {
      if (mounted) {
        setState(() {
          _actionError = switch (error) {
            ApiException(code: 'challenge_forbidden') => l.challengeForbidden,
            ApiException(code: 'terms_outdated') => l.challengeUpdatedTerms,
            ApiException(code: 'terms_required') => l.challengeConsentRequired,
            ApiException(code: 'challenge_already_submitted') =>
              l.challengeAlreadySubmitted,
            ApiException(code: 'challenge_already_voted') =>
              l.challengeAlreadyVoted,
            ApiException(
              code: 'challenge_full' ||
                  'challenge_not_open' ||
                  'challenge_entry_unavailable' ||
                  'challenge_self_vote'
            ) =>
              l.challengeUnavailable,
            _ => l.genericError,
          };
          if (error is ApiException && error.code == 'terms_outdated') {
            _accepted = false;
          }
        });
      }
    } finally {
      if (mounted) {
        setState(() => _busy = false);
        await _load();
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final data = _data;
    return KoPage(
      title: l.challengeTitle,
      accent: KoColors.tangerine,
      maxWidth: 1200,
      actions: [
        IconButton(
            key: const Key('challenge-refresh'),
            tooltip: l.serviceRefresh,
            constraints: const BoxConstraints(minWidth: 48, minHeight: 48),
            icon: const Icon(Icons.refresh),
            onPressed: _loading || _busy ? null : _load)
      ],
      child: _loadForbidden
          ? KoPanel(
              color: KoColors.pink,
              child: Semantics(
                  liveRegion: true,
                  child: Text(l.challengeForbidden,
                      style: koDisplayStyle(size: 25))))
          : data == null
              ? ServiceStatus(
                  error: _loadFailed, onRetry: _loadFailed ? _load : null)
              : data['topic'] == null
                  ? KoPanel(
                      key: const Key('challenge-empty'),
                      child: Text(l.challengeEmpty,
                          style: koDisplayStyle(size: 28)))
                  : Column(
                      crossAxisAlignment: CrossAxisAlignment.stretch,
                      children: [
                          Text(l.challengeIntro,
                              style: koDisplayStyle(size: 36)),
                          const SizedBox(height: 12),
                          Text(l.challengeRules),
                          if (data['topic']['closed_at'] != null) ...[
                            const SizedBox(height: 20),
                            KoPanel(
                                color: KoColors.lime,
                                child: Text(
                                    data['topic']['winner_entry_id'] != null
                                        ? l.challengeWinnerAnnounced
                                        : l.challengeNoWinner,
                                    style: koDisplayStyle(size: 26))),
                          ],
                          if (_loadFailed || _actionError != null) ...[
                            const SizedBox(height: 20),
                            Semantics(
                                liveRegion: true,
                                child: KoPanel(
                                    color: KoColors.pink,
                                    child: Text(_actionError ??
                                        l.challengeRefreshFailed))),
                          ],
                          const SizedBox(height: 28),
                          ServiceWorkspace(
                              primary: _participation(l, data),
                              secondary: _gallery(l, data)),
                        ]),
    );
  }

  Widget _participation(AppLocalizations l, Map<String, dynamic> data) {
    final topic = data['topic'] as Map<String, dynamic>;
    final nown = topic['nown'] as Map<String, dynamic>;
    final own = data['own_entry'] as Map<String, dynamic>?;
    final terms = data['terms'] as Map<String, dynamic>;
    final limit = (data['max_text_bytes'] as num?)?.toInt() ?? 2000;
    return Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      KoPanel(
          color: KoColors.aqua,
          child:
              Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
            KoHeading(title: l.challengeTopic),
            Text(
                nown['type'] == 'text'
                    ? nown['content']?.toString() ?? ''
                    : l.gameMediaUnavailable,
                style: koDisplayStyle(size: 30)),
          ])),
      const SizedBox(height: 28),
      if (own != null)
        KoPanel(
            key: const Key('challenge-own-entry'),
            child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  KoHeading(title: l.challengeOwnEntry),
                  Text(own['content']?.toString() ?? ''),
                  const SizedBox(height: 16),
                  Text(
                      switch (own['status']) {
                        'approved' => l.challengeApproved,
                        'rejected' => l.challengeRejected,
                        _ => l.challengePending
                      },
                      style: const TextStyle(fontWeight: FontWeight.bold)),
                  if ((own['rejection_reason'] as String? ?? '')
                      .isNotEmpty) ...[
                    const SizedBox(height: 12),
                    Text(own['rejection_reason'] as String)
                  ],
                ]))
      else if (data['can_submit'] != true)
        KoPanel(child: Text(l.challengeClosed))
      else
        KoPanel(
            child: Form(
                key: _form,
                child: Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      KoHeading(title: l.challengeEntryHeading),
                      Text(l.challengeSlots(
                          (data['intake_remaining'] as num?)?.toInt() ?? 0)),
                      const SizedBox(height: 12),
                      Text(l.challengeTextOnly),
                      const SizedBox(height: 20),
                      TextFormField(
                        key: const Key('challenge-content'),
                        controller: _content,
                        enabled: !_busy,
                        minLines: 3,
                        maxLines: 7,
                        decoration: InputDecoration(
                            labelText: l.challengeContent,
                            helperText: l.challengeByteLimit(limit),
                            helperMaxLines: 3),
                        validator: (value) => (value?.trim().isEmpty ?? true) ||
                                utf8.encode(value!.trim()).length > limit
                            ? l.challengeContentInvalid
                            : null,
                      ),
                      const SizedBox(height: 16),
                      Material(
                        type: MaterialType.transparency,
                        child: ExpansionTile(
                            tilePadding: EdgeInsets.zero,
                            title: Text(l.challengeTerms),
                            children: [
                              Align(
                                  alignment: AlignmentDirectional.centerStart,
                                  child: SelectableText(
                                      '${terms['title']}\n${terms['version']}\n\n${terms['body']}')),
                            ]),
                      ),
                      Material(
                        type: MaterialType.transparency,
                        child: CheckboxListTile(
                            key: const Key('challenge-terms'),
                            contentPadding: EdgeInsets.zero,
                            controlAffinity: ListTileControlAffinity.leading,
                            value: _accepted,
                            title: Text(l.challengeAcceptTerms),
                            onChanged: _busy
                                ? null
                                : (value) => setState(() {
                                      _accepted = value ?? false;
                                      _consentError = false;
                                    })),
                      ),
                      if (_consentError)
                        Text(l.challengeConsentRequired,
                            style:
                                const TextStyle(fontWeight: FontWeight.bold)),
                      const SizedBox(height: 16),
                      KoButton(
                          key: const Key('challenge-submit'),
                          label: _busy ? l.loadingLabel : l.challengeSubmit,
                          onPressed: _busy ? null : _submit),
                    ]))),
    ]);
  }

  Widget _gallery(AppLocalizations l, Map<String, dynamic> data) {
    final entries =
        (data['entries'] as List? ?? []).whereType<Map<String, dynamic>>();
    final ownId = data['own_entry']?['id'];
    final voted = data['voted_entry_id'];
    final closed = data['topic']['closed_at'] != null;
    final winnerId = closed ? data['topic']['winner_entry_id'] : null;
    return Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      KoHeading(title: l.challengeGallery),
      if (data['can_vote'] != true && voted == null) ...[
        Text(l.challengeVotesClosed),
        const SizedBox(height: 16)
      ],
      if (entries.isEmpty && !closed)
        KoPanel(child: Text(l.challengeNoEntries)),
      for (final entry in entries)
        Padding(
            padding: const EdgeInsets.only(bottom: 20),
            child: KoPanel(
                color: winnerId == entry['id']
                    ? KoColors.tangerine
                    : voted == entry['id']
                        ? KoColors.lime
                        : KoColors.surface,
                child: Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      if (winnerId == entry['id']) ...[
                        Text(l.challengeWinner,
                            key: Key('challenge-winner-${entry['id']}'),
                            style: koDisplayStyle(size: 28)),
                        const SizedBox(height: 16),
                      ],
                      Text(
                          (entry['nickname'] as String? ?? '').trim().isEmpty
                              ? l.challengeAnonymous
                              : entry['nickname'] as String,
                          style: koDisplayStyle(size: 22)),
                      const SizedBox(height: 16),
                      Text(entry['content']?.toString() ?? '',
                          style: Theme.of(context).textTheme.bodyLarge),
                      const SizedBox(height: 18),
                      Text(l.challengeVoteCount(
                          (entry['vote_count'] as num?)?.toInt() ?? 0)),
                      const SizedBox(height: 12),
                      KoButton(
                          key: Key('challenge-vote-${entry['id']}'),
                          label: voted == entry['id']
                              ? l.challengeVoted
                              : l.challengeVote,
                          color: voted == entry['id']
                              ? KoColors.lime
                              : KoColors.violet,
                          onPressed: !_busy &&
                                  data['can_vote'] == true &&
                                  voted == null &&
                                  ownId != entry['id']
                              ? () => _vote(entry)
                              : null),
                    ]))),
    ]);
  }
}
