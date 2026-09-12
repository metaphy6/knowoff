import 'dart:async';
import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import '../../data/api_client.dart';
import '../../data/auth_service.dart';
import '../../data/purchase_controller.dart';
import '../../core/config/app_config.dart';
import 'package:url_launcher/url_launcher.dart';
import '../../l10n/app_localizations.dart';
import '../state/store_actions.dart';
import '../widgets/ko_ui.dart';
import '../widgets/device_layout.dart';
import '../widgets/service_components.dart';
import '../widgets/service_notices.dart';
import '../widgets/service_avatar_upload.dart';
import '../widgets/store_billing.dart';
import 'community_screens.dart';
import 'safety_screens.dart';

class ProfileScreen extends StatefulWidget {
  const ProfileScreen({
    this.api,
    this.accountId,
    this.onAccountChanged,
    super.key,
  });
  final ApiClient? api;
  final String? accountId;
  final VoidCallback? onAccountChanged;
  @override
  State<ProfileScreen> createState() => _ProfileScreenState();
}

class _ProfileScreenState extends State<ProfileScreen> {
  late final _api = serviceApi(widget.api);
  Future<Map<String, dynamic>>? _future;
  bool _preserveNextDraft = false;
  final _nickname = TextEditingController();
  final _form = GlobalKey<FormState>();
  bool _busy = false;
  String? _loadedNickname;
  Future<Map<String, dynamic>> _load({bool preserveDraft = false}) async {
    final profile = widget.accountId == null
        ? await _api.getProfile()
        : await _api.getPublicProfile(widget.accountId!);
    if (mounted) {
      final hasDraft =
          _loadedNickname != null && _nickname.text != _loadedNickname;
      final name = profile['nickname']?.toString() ?? '';
      if (!preserveDraft || !hasDraft) _nickname.text = name;
      _loadedNickname = name;
    }
    return profile;
  }

  void _reload({bool preserveDraft = true}) => setState(() {
    _preserveNextDraft = preserveDraft;
    _future = null;
  });
  @override
  void dispose() {
    _nickname.dispose();
    super.dispose();
  }

  Future<void> _save(Future<void> Function() call) async {
    if (_busy) return;
    setState(() => _busy = true);
    try {
      await call();
      if (!mounted) return;
      _reload(preserveDraft: false);
      serviceMessage(context, AppLocalizations.of(context).serviceSaved);
    } catch (_) {
      if (mounted) {
        serviceMessage(context, AppLocalizations.of(context).genericError);
      }
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    return KoPage(
      title: l10n.profileTitle,
      eyebrow: l10n.serviceProfileEyebrow,
      accent: KoColors.aqua,
      actions: [
        if (widget.accountId == null)
          IconButton(
            key: const Key('profile-account-access'),
            tooltip: l10n.oauthTitle,
            icon: const Icon(Icons.manage_accounts_outlined),
            onPressed: () async {
              final changed = await koPush<bool>(
                context,
                AccountAccessScreen(auth: _api.authService),
              );
              if (changed == true && mounted) {
                _reload(preserveDraft: false);
                widget.onAccountChanged?.call();
              }
            },
          ),
        IconButton(
          key: const Key('profile-safety'),
          tooltip: l10n.safetyTitle,
          icon: const Icon(Icons.flag_outlined),
          onPressed: () => koPush<void>(
            context,
            widget.accountId == null
                ? SafetyScreen(api: _api)
                : PlayerSafetyScreen(
                    api: _api,
                    accountID: widget.accountId,
                    nickname: _loadedNickname,
                  ),
          ),
        ),
        IconButton(
          key: const Key('profile-refresh'),
          tooltip: l10n.serviceRefresh,
          constraints: const BoxConstraints(minWidth: 48, minHeight: 48),
          icon: const Icon(Icons.refresh),
          onPressed: _busy ? null : () => _reload(preserveDraft: true),
        ),
      ],
      child: FutureBuilder<Map<String, dynamic>>(
        future: _future ??= _load(preserveDraft: _preserveNextDraft),
        builder: (context, snapshot) {
          if (snapshot.hasError) {
            return ServiceStatus(error: true, onRetry: _reload);
          }
          if (!snapshot.hasData) return const ServiceStatus();
          final p = snapshot.data!;
          final own = widget.accountId == null;
          return KoEntrance(
            child: ServiceWorkspace(
              primary: Column(
                key: const Key('profile-editor'),
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  KoPanel(
                    color: KoColors.aqua,
                    shadow: KoShadows.lg,
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Row(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Expanded(
                              child: Text(
                                p['nickname']?.toString() ??
                                    l10n.profileNickname,
                                style: koDisplayStyle(size: 42),
                              ),
                            ),
                            const SizedBox(width: 12),
                            Transform.rotate(
                              angle: KoTilt.soft,
                              child: p['avatar'] == 'custom'
                                  ? ServiceProfileAvatar(
                                      api: _api,
                                      accountId:
                                          p['account_id']?.toString() ??
                                          widget.accountId ??
                                          '',
                                      revision:
                                          (p['avatar_revision'] as num?)
                                              ?.toInt() ??
                                          0,
                                    )
                                  : DoodleIcon(
                                      switch (p['avatar']) {
                                        'nower' => Doodle.eye,
                                        'donower' => Doodle.mask,
                                        'detective' => Doodle.incognito,
                                        'party' => Doodle.sparkle,
                                        _ => Doodle.person,
                                      },
                                      key: const Key('profile-avatar'),
                                      size: 60,
                                    ),
                            ),
                          ],
                        ),
                        const SizedBox(height: 12),
                        if (p['current_week_winner'] == true)
                          KoTag(
                            key: const Key('profile-current-week-winner'),
                            label: l10n.challengeWinner,
                            color: KoColors.lime,
                            icon: const DoodleIcon(Doodle.crown, size: 22),
                          ),
                        Text(l10n.serviceProfileBlurb),
                        const SizedBox(height: 18),
                        Wrap(
                          spacing: 10,
                          runSpacing: 10,
                          children: [
                            KoTag(
                              label:
                                  '${l10n.profileLevel} ${serviceNumber(context, p['level'])}',
                              icon: const DoodleIcon(Doodle.sparkle, size: 20),
                              color: KoColors.lime,
                            ),
                            KoTag(
                              label:
                                  '${serviceNumber(context, p['xp'])} ${l10n.profileXP}',
                              icon: const DoodleIcon(Doodle.crown, size: 20),
                            ),
                          ],
                        ),
                      ],
                    ),
                  ),
                  const SizedBox(height: 28),
                  if (own) ...[
                    KoButton(
                      key: const Key('profile-contributor'),
                      label: l10n.portalTitle,
                      color: KoColors.aqua,
                      icon: const Icon(Icons.open_in_browser),
                      onPressed: () => koPush<void>(
                        context,
                        ContributorConnectScreen(api: _api),
                      ),
                    ),
                    const SizedBox(height: 28),
                    KoPanel(
                      child: Form(
                        key: _form,
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.stretch,
                          children: [
                            TextFormField(
                              key: const Key('profile-nickname'),
                              controller: _nickname,
                              enabled: !_busy,
                              decoration: InputDecoration(
                                labelText: l10n.profileNickname,
                              ),
                              validator: (text) {
                                final length = utf8
                                    .encode(text?.trim() ?? '')
                                    .length;
                                return length < 2 || length > 20
                                    ? l10n.serviceNicknameInvalid
                                    : null;
                              },
                            ),
                            const SizedBox(height: 14),
                            KoButton(
                              key: const Key('profile-save'),
                              label: _busy
                                  ? l10n.loadingLabel
                                  : l10n.serviceSave,
                              expand: true,
                              onPressed: _busy
                                  ? null
                                  : () {
                                      if (_form.currentState!.validate()) {
                                        _save(
                                          () => _api.updateNickname(
                                            _nickname.text.trim(),
                                          ),
                                        );
                                      }
                                    },
                            ),
                            const SizedBox(height: 22),
                            KoHeading(title: l10n.profileChangeAvatar),
                            Wrap(
                              spacing: 10,
                              runSpacing: 10,
                              children: [
                                for (final preset in <(String, String, Doodle)>[
                                  (
                                    'default',
                                    l10n.serviceAvatarDefault,
                                    Doodle.person,
                                  ),
                                  ('nower', l10n.roleNower, Doodle.eye),
                                  ('donower', l10n.roleDonower, Doodle.mask),
                                  (
                                    'detective',
                                    l10n.serviceAvatarDetective,
                                    Doodle.incognito,
                                  ),
                                  (
                                    'party',
                                    l10n.serviceAvatarParty,
                                    Doodle.sparkle,
                                  ),
                                ])
                                  KoButton(
                                    key: Key('avatar-${preset.$1}'),
                                    label: preset.$2,
                                    icon: DoodleIcon(
                                      p['avatar'] == preset.$1
                                          ? Doodle.check
                                          : preset.$3,
                                      size: 25,
                                    ),
                                    color: p['avatar'] == preset.$1
                                        ? KoColors.lime
                                        : KoColors.surface,
                                    onPressed: _busy || p['avatar'] == preset.$1
                                        ? null
                                        : () => _save(
                                            () => _api.updateAvatar(preset.$1),
                                          ),
                                  ),
                              ],
                            ),
                          ],
                        ),
                      ),
                    ),
                    const SizedBox(height: 28),
                  ],
                  if (own) ...[
                    ServiceAvatarUpload(api: _api, onUploaded: _reload),
                    const SizedBox(height: 28),
                  ],
                ],
              ),
              secondary: Column(
                key: const Key('profile-statistics'),
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  KoHeading(title: l10n.profileStatsTitle),
                  ServiceGrid(
                    minWidth: 190,
                    children: [
                      ServiceMetric(
                        label: l10n.profileOverallPoints,
                        value: serviceNumber(context, p['overall_points']),
                        color: KoColors.lime,
                      ),
                      if (own)
                        ServiceMetric(
                          label: l10n.profileNonConvertedPoints,
                          value: serviceNumber(
                            context,
                            p['non_converted_points'],
                          ),
                          color: KoColors.tangerine,
                        ),
                      ServiceMetric(
                        label: l10n.profileMatchesPlayed,
                        value: serviceNumber(context, p['matches_played']),
                      ),
                      ServiceMetric(
                        label: l10n.profileMatchesWonNower,
                        value: serviceNumber(context, p['matches_won_nower']),
                        color: KoColors.aqua,
                      ),
                      ServiceMetric(
                        label: l10n.profileMatchesWonDonower,
                        value: serviceNumber(context, p['matches_won_donower']),
                        color: KoColors.pink,
                      ),
                      ServiceMetric(
                        label: l10n.profileCorrectVotes,
                        value: serviceNumber(context, p['correct_votes']),
                      ),
                    ],
                  ),
                  const SizedBox(height: 18),
                  ServiceGrid(
                    minWidth: 190,
                    children: [
                      ServiceMetric(
                        label: l10n.serviceVoteAccuracy,
                        value: servicePercent(
                          context,
                          p['correct_votes'],
                          p['votes_cast'],
                        ),
                      ),
                      ServiceMetric(
                        label: l10n.serviceSurvivalRate,
                        value: servicePercent(
                          context,
                          p['donower_survivals'],
                          p['donower_matches'],
                        ),
                        color: KoColors.pink,
                      ),
                      ServiceMetric(
                        label: l10n.serviceWeekWins,
                        value: serviceNumber(context, p['week_winner_titles']),
                        color: KoColors.lime,
                      ),
                      ServiceMetric(
                        label: l10n.servicePodiums,
                        value: serviceNumber(context, p['weekly_podiums']),
                      ),
                      ServiceMetric(
                        label: l10n.servicePokesSent,
                        value: serviceNumber(context, p['pokes_sent']),
                      ),
                    ],
                  ),
                  if ((p['contributor_credits'] as List? ?? []).isNotEmpty) ...[
                    const SizedBox(height: 24),
                    KoPanel(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          KoHeading(title: l10n.serviceCredits),
                          for (final credit
                              in (p['contributor_credits'] as List))
                            Text(credit.toString()),
                        ],
                      ),
                    ),
                  ],
                  if (!own) ...[
                    const SizedBox(height: 24),
                    KoButton(
                      label: l10n.reportPlayerAction,
                      color: KoColors.pink,
                      icon: const Icon(Icons.flag_outlined),
                      onPressed: () async {
                        final sent = await showDialog<bool>(
                          context: context,
                          builder: (_) => ServiceReportDialog(
                            api: _api,
                            reportType: 'conduct',
                            accountId: widget.accountId,
                          ),
                        );
                        if (sent == true && context.mounted) {
                          serviceMessage(context, l10n.serviceReportSent);
                        }
                      },
                    ),
                  ],
                ],
              ),
            ),
          );
        },
      ),
    );
  }
}

class LeaderboardScreen extends StatefulWidget {
  const LeaderboardScreen({this.api, super.key});
  final ApiClient? api;
  @override
  State<LeaderboardScreen> createState() => _LeaderboardScreenState();
}

class _LeaderboardScreenState extends State<LeaderboardScreen> {
  late final _api = serviceApi(widget.api);
  Future<Map<String, dynamic>>? _future;
  void _reload() => setState(() {
    _future = null;
  });
  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    return KoPage(
      title: l10n.leaderboardTitle,
      eyebrow: l10n.serviceLeaderboardEyebrow,
      accent: KoColors.tangerine,
      actions: [
        IconButton(
          key: const Key('leaderboard-refresh'),
          tooltip: l10n.serviceRefresh,
          constraints: const BoxConstraints(minWidth: 48, minHeight: 48),
          icon: const Icon(Icons.refresh),
          onPressed: _reload,
        ),
      ],
      child: FutureBuilder<Map<String, dynamic>>(
        future: _future ??= _api.getLeaderboard(),
        builder: (context, snapshot) {
          if (snapshot.hasError) {
            return ServiceStatus(error: true, onRetry: _reload);
          }
          if (!snapshot.hasData) return const ServiceStatus();
          final data = snapshot.data!;
          final own = data['own'] as Map<String, dynamic>?;
          final rows = (data['top'] as List? ?? [])
              .whereType<Map<String, dynamic>>()
              .toList();
          return KoEntrance(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                KoPanel(
                  color: KoColors.lime,
                  shadow: KoShadows.lg,
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      const DoodleIcon(Doodle.crown, size: 70),
                      const SizedBox(height: 14),
                      Text(
                        l10n.leaderboardYourRank,
                        style: koDisplayStyle(size: 24),
                      ),
                      Text(
                        own == null
                            ? '—'
                            : '#${serviceNumber(context, own['rank'])}',
                        style: koDisplayStyle(size: 80),
                      ),
                      if (own != null)
                        Text(
                          '${serviceNumber(context, own['points'])} ${l10n.profileOverallPoints}',
                        ),
                    ],
                  ),
                ),
                const SizedBox(height: 28),
                if (rows.isEmpty)
                  KoPanel(
                    child: Text(
                      l10n.leaderboardEmpty,
                      style: koDisplayStyle(size: 28),
                    ),
                  ),
                for (var i = 0; i < rows.length; i++)
                  Padding(
                    padding: const EdgeInsets.only(bottom: 16),
                    child: _rankRow(context, rows[i], i),
                  ),
              ],
            ),
          );
        },
      ),
    );
  }

  Widget _rankRow(BuildContext context, Map<String, dynamic> row, int index) {
    final id = row['account_id']?.toString() ?? '';
    final name =
        row['nickname']?.toString() ??
        (id.length > 8 ? id.substring(0, 8) : id);
    return KoPanel(
      color: index == 0 ? KoColors.tangerine : KoColors.surface,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Wrap(
            spacing: 18,
            runSpacing: 12,
            crossAxisAlignment: WrapCrossAlignment.center,
            children: [
              Text(
                '#${serviceNumber(context, row['rank'])}',
                style: koDisplayStyle(size: 36),
              ),
              Text(
                serviceNumber(context, row['points']),
                style: koDisplayStyle(size: 32),
              ),
            ],
          ),
          const SizedBox(height: 12),
          KoButton(
            label: name,
            color: KoColors.surface,
            icon: const DoodleIcon(Doodle.person, size: 24),
            onPressed: id.isEmpty
                ? null
                : () =>
                      koPush(context, ProfileScreen(api: _api, accountId: id)),
          ),
        ],
      ),
    );
  }
}

class StoreScreen extends StatefulWidget {
  const StoreScreen({this.api, this.purchases, super.key});
  final ApiClient? api;
  final PurchaseController? purchases;
  @override
  State<StoreScreen> createState() => _StoreScreenState();
}

class _StoreScreenState extends State<StoreScreen> {
  late final _store = StoreActions(serviceApi(widget.api));
  late final _purchases = widget.purchases ?? AppConfig.instance.purchases;
  int _verifiedRevision = 0;
  @override
  void initState() {
    super.initState();
    _verifiedRevision = _purchases.verifiedRevision;
    _purchases.addListener(_purchaseChanged);
    _purchases.start();
  }

  @override
  void dispose() {
    _purchases.removeListener(_purchaseChanged);
    super.dispose();
  }

  void _purchaseChanged() {
    if (!mounted) return;
    final revision = _purchases.verifiedRevision;
    if (revision > _verifiedRevision) {
      _verifiedRevision = revision;
      _reload();
    }
  }

  Future<void> _load() async {
    await _store.load();
    final catalog = _store.purchaseCatalog;
    if (catalog == null) {
      _purchases.disableOffers();
    } else {
      await _purchases.load(
        catalog,
        premiumActive: _store.premiumActive,
        accountID: _store.accountID,
      );
    }
  }

  Future<void>? _future;
  bool _busy = false;
  void _reload() => setState(() {
    _future = null;
  });
  Future<void> _purchase(Future<void> Function() call) async {
    if (_busy) return;
    setState(() => _busy = true);
    try {
      await call();
      if (mounted) {
        serviceMessage(context, AppLocalizations.of(context).storeBought);
      }
    } catch (_) {
      if (mounted) {
        serviceMessage(context, AppLocalizations.of(context).storeError);
      }
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    return KoPage(
      title: l10n.storeTitle,
      eyebrow: l10n.serviceStoreEyebrow,
      accent: KoColors.tangerine,
      actions: [
        IconButton(
          key: const Key('store-refresh'),
          tooltip: l10n.serviceRefresh,
          constraints: const BoxConstraints(minWidth: 48, minHeight: 48),
          icon: const Icon(Icons.refresh),
          onPressed: _busy ? null : _reload,
        ),
      ],
      child: FutureBuilder<void>(
        future: _future ??= _load(),
        builder: (context, snapshot) {
          if (snapshot.hasError) {
            return Column(
              children: [
                ServiceStatus(error: true, onRetry: _reload),
                StoreBilling(purchases: _purchases),
              ],
            );
          }
          if (snapshot.connectionState != ConnectionState.done) {
            return const ServiceStatus();
          }
          final catalog = _store.catalog ?? {};
          final prices =
              catalog['play_pass_prices'] as Map<String, dynamic>? ?? {};
          final unlocks =
              catalog['unlock_prices'] as Map<String, dynamic>? ?? {};
          final rate = (catalog['points_to_noin'] as num?)?.toInt() ?? 100;
          return KoEntrance(
            child: ServiceWorkspace(
              primary: Column(
                key: const Key('store-wallet'),
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  KoPanel(
                    color: KoColors.tangerine,
                    shadow: KoShadows.lg,
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        DoodleIcon(
                          Doodle.coin,
                          size: KoDeviceLayout.of(context).isPhone ? 32 : 68,
                        ),
                        Text(
                          serviceNumber(context, _store.wallet?['noin']),
                          style: koDisplayStyle(
                            size: KoDeviceLayout.of(context).isPhone ? 42 : 70,
                          ),
                        ),
                        Text(
                          l10n.storeBalanceLabel,
                          style: koDisplayStyle(size: 26),
                        ),
                        const SizedBox(height: 12),
                        Text(l10n.serviceStoreBlurb),
                      ],
                    ),
                  ),
                  const SizedBox(height: 24),
                  KoButton(
                    key: const Key('convert-open'),
                    label: l10n.convertPointsTitle,
                    expand: true,
                    color: KoColors.lime,
                    icon: const DoodleIcon(Doodle.coin, size: 25),
                    onPressed: _busy || rate <= 0
                        ? null
                        : () async {
                            await showDialog<bool>(
                              barrierDismissible: false,
                              context: context,
                              builder: (_) =>
                                  _ConversionDialog(store: _store, rate: rate),
                            );
                            if (mounted) _reload();
                          },
                  ),
                  const SizedBox(height: 28),
                ],
              ),
              secondary: Column(
                key: const Key('store-catalog'),
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  KoHeading(title: l10n.storePlayPasses),
                  ServiceGrid(
                    children: [
                      for (final pass in <(String, String)>[
                        ('day_1', l10n.storePlayPassDay1),
                        ('day_3', l10n.storePlayPassDay3),
                        ('day_7', l10n.storePlayPassDay7),
                      ])
                        if (prices[pass.$1] is num)
                          KoPanel(
                            color: KoColors.surface,
                            child: Column(
                              crossAxisAlignment: CrossAxisAlignment.stretch,
                              children: [
                                const DoodleIcon(Doodle.pass, size: 48),
                                const SizedBox(height: 14),
                                Text(pass.$2, style: koDisplayStyle(size: 36)),
                                const SizedBox(height: 16),
                                KoButton(
                                  key: Key('pass-${pass.$1}'),
                                  label: l10n.storeBuyPrice(
                                    (prices[pass.$1] as num).toInt(),
                                  ),
                                  onPressed: _busy
                                      ? null
                                      : () => _purchase(
                                          () =>
                                              _store.purchasePlayPass(pass.$1),
                                        ),
                                ),
                              ],
                            ),
                          ),
                    ],
                  ),
                  const SizedBox(height: 28),
                  KoHeading(title: l10n.storeUnlocks),
                  ServiceGrid(
                    children: [
                      for (final unlock in <(String, String, Doodle)>[
                        (
                          'custom_avatar',
                          l10n.storeUnlockCustomAvatar,
                          Doodle.person,
                        ),
                        ('poke_style', l10n.storeUnlockPokeStyle, Doodle.poke),
                        ('theme_pack', l10n.storeUnlockThemePack, Doodle.cards),
                      ])
                        if (unlocks[unlock.$1] is num)
                          KoPanel(
                            child: Column(
                              crossAxisAlignment: CrossAxisAlignment.stretch,
                              children: [
                                DoodleIcon(unlock.$3, size: 48),
                                const SizedBox(height: 14),
                                Text(
                                  unlock.$2,
                                  style: koDisplayStyle(size: 25),
                                ),
                                if (unlock.$1 != 'custom_avatar') ...[
                                  const SizedBox(height: 10),
                                  Text(l10n.serviceCatalogUnavailable),
                                ],
                                if (unlock.$1 == 'custom_avatar' &&
                                    catalog['custom_avatar_owned'] == true)
                                  Text(l10n.serviceAvatarOwned),
                                if (unlock.$1 == 'custom_avatar' &&
                                    catalog['custom_avatar_owned'] != true &&
                                    catalog['custom_avatar_available'] != true)
                                  Text(l10n.serviceCatalogUnavailable),
                                const SizedBox(height: 16),
                                KoButton(
                                  label: l10n.storeBuyPrice(
                                    (unlocks[unlock.$1] as num).toInt(),
                                  ),
                                  onPressed:
                                      _busy ||
                                          unlock.$1 != 'custom_avatar' ||
                                          catalog['custom_avatar_available'] !=
                                              true ||
                                          catalog['custom_avatar_owned'] !=
                                              false
                                      ? null
                                      : () => _purchase(
                                          () => _store.purchaseUnlock(
                                            'custom_avatar',
                                          ),
                                        ),
                                ),
                              ],
                            ),
                          ),
                    ],
                  ),
                  const SizedBox(height: 28),
                  StoreBilling(
                    purchases: _purchases,
                    managementPlatforms: _store.managementPlatforms,
                  ),
                ],
              ),
            ),
          );
        },
      ),
    );
  }
}

class _ConversionDialog extends StatefulWidget {
  const _ConversionDialog({required this.store, required this.rate});
  final StoreActions store;
  final int rate;
  @override
  State<_ConversionDialog> createState() => _ConversionDialogState();
}

class _ConversionDialogState extends State<_ConversionDialog> {
  final _input = TextEditingController();
  bool _busy = false;
  String? _error;
  int? _result;
  @override
  void dispose() {
    _input.dispose();
    super.dispose();
  }

  Future<void> _convert() async {
    if (_busy) return;
    final l10n = AppLocalizations.of(context);
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      final result = await widget.store.convertPoints(
        _input.text,
        pointsToNoin: widget.rate,
      );
      if (mounted) setState(() => _result = result);
    } on FormatException {
      if (mounted) {
        setState(() => _error = l10n.convertPointsMultiple(widget.rate));
      }
    } catch (_) {
      if (mounted) setState(() => _error = l10n.genericError);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    return PopScope(
      canPop: !_busy,
      child: Dialog(
        child: SingleChildScrollView(
          child: Padding(
            padding: const EdgeInsets.all(22),
            child: ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 420),
              child: Column(
                mainAxisSize: MainAxisSize.min,
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  KoHeading(title: l10n.convertPointsTitle),
                  if (_result == null) ...[
                    Text(l10n.serviceConvertWarning),
                    const SizedBox(height: 18),
                    TextField(
                      key: const Key('convert-input'),
                      controller: _input,
                      enabled: !_busy,
                      keyboardType: TextInputType.number,
                      inputFormatters: [FilteringTextInputFormatter.digitsOnly],
                      decoration: InputDecoration(
                        labelText: l10n.convertPointsHint,
                        errorText: _error,
                      ),
                    ),
                    const SizedBox(height: 18),
                    KoButton(
                      key: const Key('convert-submit'),
                      label: _busy
                          ? l10n.loadingLabel
                          : l10n.convertPointsConvert,
                      color: KoColors.lime,
                      onPressed: _busy ? null : _convert,
                      expand: true,
                    ),
                  ] else ...[
                    const DoodleIcon(Doodle.coin, size: 70),
                    const SizedBox(height: 18),
                    Text(
                      l10n.convertPointsResult(_result!),
                      style: koDisplayStyle(size: 30),
                    ),
                  ],
                  TextButton(
                    onPressed: _busy
                        ? null
                        : () => Navigator.pop(context, _result != null),
                    child: Text(_result == null ? l10n.cancel : l10n.close),
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

class NoticeInboxScreen extends StatefulWidget {
  const NoticeInboxScreen({this.api, super.key});
  final ApiClient? api;
  @override
  State<NoticeInboxScreen> createState() => _NoticeInboxScreenState();
}

class _NoticeInboxScreenState extends State<NoticeInboxScreen> {
  late final _api = serviceApi(widget.api);
  late Future<List<dynamic>> _future = _api.getNotices();
  void _reload() => setState(() {
    _future = _api.getNotices();
  });
  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    return KoPage(
      title: l10n.noticeInboxTitle,
      eyebrow: l10n.serviceNoticesEyebrow,
      accent: KoColors.aqua,
      child: FutureBuilder<List<dynamic>>(
        future: _future,
        builder: (context, snapshot) {
          if (snapshot.hasError) {
            return ServiceStatus(error: true, onRetry: _reload);
          }
          if (!snapshot.hasData) return const ServiceStatus();
          final notices = snapshot.data!
              .whereType<Map<String, dynamic>>()
              .toList();
          return KoEntrance(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                if (notices.isEmpty)
                  KoPanel(
                    color: KoColors.aqua,
                    child: Column(
                      children: [
                        const DoodleIcon(Doodle.quietBubble, size: 80),
                        const SizedBox(height: 18),
                        Text(
                          l10n.noticesEmpty,
                          style: koDisplayStyle(size: 32),
                        ),
                      ],
                    ),
                  ),
                for (final notice in notices)
                  Padding(
                    padding: const EdgeInsets.only(bottom: 22),
                    child: ServiceNoticeTile(notice: notice),
                  ),
              ],
            ),
          );
        },
      ),
    );
  }
}

class FeedbackScreen extends StatefulWidget {
  const FeedbackScreen({this.api, this.contextSnapshot, super.key});
  final ApiClient? api;
  final Map<String, dynamic>? contextSnapshot;
  @override
  State<FeedbackScreen> createState() => _FeedbackScreenState();
}

class _FeedbackScreenState extends State<FeedbackScreen> {
  late final _api = serviceApi(widget.api);
  final _form = GlobalKey<FormState>();
  final _title = TextEditingController();
  final _message = TextEditingController();
  String _type = 'bug';
  bool _busy = false;
  bool _sent = false;
  bool _failed = false;
  @override
  void dispose() {
    _title.dispose();
    _message.dispose();
    super.dispose();
  }

  Future<void> _send() async {
    if (_busy || !_form.currentState!.validate()) return;
    setState(() {
      _busy = true;
      _failed = false;
    });
    try {
      await _api.createFeedback(
        type: _type,
        title: _title.text.trim(),
        message: _message.text.trim(),
        contextSnapshot: widget.contextSnapshot,
      );
      if (mounted) setState(() => _sent = true);
    } catch (_) {
      if (mounted) setState(() => _failed = true);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    return KoPage(
      title: l10n.feedbackTitle,
      eyebrow: l10n.serviceFeedbackEyebrow,
      maxWidth: 820,
      accent: KoColors.pink,
      child: _sent
          ? KoPanel(
              color: KoColors.lime,
              child: Column(
                children: [
                  const DoodleIcon(Doodle.check, size: 90),
                  const SizedBox(height: 20),
                  Text(
                    l10n.serviceFeedbackSent,
                    style: koDisplayStyle(size: 32),
                  ),
                ],
              ),
            )
          : KoPanel(
              child: Form(
                key: _form,
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    const Align(
                      alignment: Alignment.centerLeft,
                      child: DoodleIcon(Doodle.hailer, size: 64),
                    ),
                    const SizedBox(height: 20),
                    Wrap(
                      spacing: 10,
                      runSpacing: 10,
                      children: [
                        for (final type in [
                          ('bug', l10n.feedbackTypeBug),
                          ('idea', l10n.feedbackTypeIdea),
                          ('other', l10n.feedbackTypeOther),
                        ])
                          KoButton(
                            label: type.$2,
                            color: _type == type.$1
                                ? KoColors.lime
                                : KoColors.surface,
                            icon: DoodleIcon(
                              _type == type.$1
                                  ? Doodle.check
                                  : Doodle.quietBubble,
                              size: 22,
                            ),
                            onPressed: _busy
                                ? null
                                : () => setState(() => _type = type.$1),
                          ),
                      ],
                    ),
                    const SizedBox(height: 24),
                    TextFormField(
                      key: const Key('feedback-title'),
                      controller: _title,
                      enabled: !_busy,
                      maxLength: 200,
                      decoration: InputDecoration(
                        labelText: l10n.feedbackTitleHint,
                      ),
                      validator: (v) => v == null || v.trim().isEmpty
                          ? l10n.serviceRequired
                          : null,
                    ),
                    const SizedBox(height: 14),
                    TextFormField(
                      key: const Key('feedback-message'),
                      controller: _message,
                      enabled: !_busy,
                      minLines: 4,
                      maxLines: 8,
                      maxLength: 4000,
                      decoration: InputDecoration(
                        labelText: l10n.feedbackMessage,
                      ),
                      validator: (v) => v == null || v.trim().isEmpty
                          ? l10n.serviceRequired
                          : null,
                    ),
                    const SizedBox(height: 16),
                    Text(l10n.serviceContext, style: koDisplayStyle(size: 20)),
                    Text(
                      widget.contextSnapshot == null
                          ? l10n.serviceNoContext
                          : const JsonEncoder.withIndent(
                              '  ',
                            ).convert(widget.contextSnapshot),
                    ),
                    if (_failed) ...[
                      const SizedBox(height: 14),
                      Text(l10n.genericError),
                    ],
                    const SizedBox(height: 22),
                    KoButton(
                      key: const Key('feedback-submit'),
                      label: _busy ? l10n.loadingLabel : l10n.feedbackSubmit,
                      expand: true,
                      onPressed: _busy ? null : _send,
                    ),
                  ],
                ),
              ),
            ),
    );
  }
}

/// Account access remains available independently of authenticated profile reads.
/// Leaving this page cancels its polling; process restarts may resume its bounded
/// local proof, while browser callbacks never receive player credentials.
class AccountAccessScreen extends StatefulWidget {
  const AccountAccessScreen({this.auth, this.openBrowser, super.key});
  final AuthService? auth;
  final Future<bool> Function(Uri)? openBrowser;
  @override
  State<AccountAccessScreen> createState() => _AccountAccessScreenState();
}

class _AccountAccessScreenState extends State<AccountAccessScreen>
    with WidgetsBindingObserver {
  late final _auth = widget.auth ?? AppConfig.instance.authService;
  OAuthIntent _intent = OAuthIntent.link;
  OAuthAttempt? _attempt;
  OAuthPollState? _state;
  Timer? _timer;
  bool _busy = true, _foreground = true;
  String? _error;
  int _operation = 0;
  late int _flowGeneration;
  String? _initialAccount;
  bool _exiting = false;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    _resume();
  }

  Future<void> _resume() async {
    _flowGeneration = _auth.oauthGeneration;
    try {
      final attempt = await _auth.resumeOAuth();
      if (!mounted) return;
      _initialAccount = _auth.accountId;
      _attempt = attempt;
      if (attempt != null) _intent = attempt.intent;
    } catch (error) {
      if (mounted) _error = _safeCode(error);
    } finally {
      if (mounted) {
        setState(() => _busy = false);
        _schedule();
      }
    }
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    _foreground = state == AppLifecycleState.resumed;
    _timer?.cancel();
    if (_foreground) _schedule(delay: Duration.zero);
  }

  void _schedule({Duration delay = const Duration(seconds: 2)}) {
    _timer?.cancel();
    if (!_foreground ||
        _attempt == null ||
        _busy ||
        _state == OAuthPollState.confirmSwitch ||
        _state == OAuthPollState.completed ||
        _error != null) {
      return;
    }
    _timer = Timer(delay, _poll);
  }

  String _safeCode(Object error) =>
      error is AuthSessionException ? error.code : 'oauth.provider_unavailable';

  Future<void> _start(String provider) async {
    if (_busy) return;
    final operation = ++_operation;
    setState(() {
      _busy = true;
      _error = null;
      _state = null;
    });
    try {
      final starting = _auth.startOAuth(provider, _intent);
      _flowGeneration = _auth.oauthGeneration;
      final attempt = await starting;
      if (!mounted || operation != _operation) return;
      _attempt = attempt;
      await _launch(attempt);
    } catch (error) {
      if (mounted && operation == _operation) _error = _safeCode(error);
    } finally {
      if (mounted && operation == _operation) {
        setState(() => _busy = false);
        _schedule();
      }
    }
  }

  Future<void> _launch(OAuthAttempt attempt) async {
    bool opened;
    try {
      opened =
          await (widget.openBrowser?.call(attempt.authorizationUri) ??
              launchUrl(
                attempt.authorizationUri,
                mode: LaunchMode.externalApplication,
              ));
    } catch (_) {
      opened = false;
    }
    if (!opened) throw const AuthSessionException('oauth.browser_failed');
  }

  Future<void> _reopen() async {
    final attempt = _attempt;
    if (_busy || attempt == null) return;
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await _launch(attempt);
    } catch (error) {
      if (mounted) _error = _safeCode(error);
    } finally {
      if (mounted) {
        setState(() => _busy = false);
        _schedule();
      }
    }
  }

  Future<void> _poll() async {
    if (_busy || _attempt == null || !_foreground) return;
    final operation = ++_operation;
    _timer?.cancel();
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      final state = await _auth.pollOAuth(generation: _flowGeneration);
      if (!mounted || operation != _operation) return;
      _state = state;
      if (state == OAuthPollState.completed) _attempt = null;
    } catch (error) {
      if (mounted && operation == _operation) {
        _error = _safeCode(error);
        if (_error == 'oauth.expired') _attempt = null;
      }
    } finally {
      if (mounted && operation == _operation) {
        setState(() => _busy = false);
        _schedule();
      }
    }
  }

  Future<void> _confirmSwitch() async {
    if (_busy) return;
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await _auth.confirmOAuthSwitch(generation: _flowGeneration);
      if (mounted) {
        _state = OAuthPollState.completed;
        _attempt = null;
      }
    } catch (error) {
      if (mounted) _error = _safeCode(error);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _cancel() async {
    ++_operation;
    _timer?.cancel();
    await _auth.cancelOAuth(generation: _flowGeneration);
    if (mounted) {
      setState(() {
        _attempt = null;
        _state = null;
        _busy = false;
        _error = null;
      });
    }
  }

  Future<void> _exit() async {
    if (_exiting) return;
    _exiting = true;
    ++_operation;
    _timer?.cancel();
    try {
      // Cancellation queues after any accepted storage commit. Report the actual
      // identity change even if Back arrived before the success panel rendered.
      await _auth.cancelOAuth(generation: _flowGeneration);
      if (mounted) {
        Navigator.of(context).pop(
          _state == OAuthPollState.completed ||
              _auth.accountId != _initialAccount,
        );
      }
    } catch (error) {
      if (mounted) {
        setState(() {
          _exiting = false;
          _busy = false;
          _error = _safeCode(error);
        });
      }
    }
  }

  @override
  void dispose() {
    ++_operation;
    _timer?.cancel();
    WidgetsBinding.instance.removeObserver(this);
    unawaited(
      _auth.cancelOAuth(generation: _flowGeneration).catchError((Object _) {}),
    );
    super.dispose();
  }

  String _message(AppLocalizations l) => switch (_error) {
    'oauth.conflict' => l.oauthConflict,
    'oauth.restore_unlinked' => l.oauthUnlinked,
    'oauth.expired' => l.oauthExpired,
    'oauth.rate_limited' => l.oauthRateLimited,
    'oauth.account_mismatch' => l.oauthMismatch,
    'oauth.browser_failed' => l.oauthBrowserFailed,
    'auth.restore_required' ||
    'auth.invalid_response' ||
    'auth.unavailable' => l.oauthSessionRequired,
    'oauth.invalid' ||
    'oauth.invalid_response' ||
    'oauth.cancelled' => l.oauthInvalid,
    _ => l.oauthUnavailable,
  };

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    return PopScope<bool>(
      canPop: false,
      onPopInvokedWithResult: (didPop, result) {
        if (!didPop) unawaited(_exit());
      },
      child: KoPage(
        title: l.oauthTitle,
        accent: KoColors.aqua,
        maxWidth: 700,
        actions: [
          IconButton(
            key: const Key('oauth-help'),
            tooltip: l.safetyTitle,
            icon: const Icon(Icons.help_outline),
            onPressed: () => koPush<void>(context, const SafetyScreen()),
          ),
        ],
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            if (_state == OAuthPollState.completed) ...[
              KoPanel(
                key: const Key('oauth-success'),
                color: KoColors.lime,
                child: Column(
                  children: [
                    const DoodleIcon(Doodle.check, size: 56),
                    const SizedBox(height: 16),
                    Text(l.oauthSuccess, style: koDisplayStyle(size: 27)),
                    const SizedBox(height: 20),
                    KoButton(
                      label: l.close,
                      onPressed: () => Navigator.of(context).pop(true),
                    ),
                  ],
                ),
              ),
            ] else ...[
              KoPanel(color: KoColors.aqua, child: Text(l.oauthIntro)),
              if (_error != null) ...[
                const SizedBox(height: 20),
                Semantics(
                  liveRegion: true,
                  child: KoPanel(
                    color: KoColors.pink,
                    child: Text(_message(l)),
                  ),
                ),
              ],
              const SizedBox(height: 20),
              if (_state == OAuthPollState.confirmSwitch) ...[
                KoPanel(
                  color: KoColors.pink,
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      Text(l.oauthSwitchTitle, style: koDisplayStyle(size: 28)),
                      const SizedBox(height: 12),
                      Text(l.oauthSwitchBody),
                      const SizedBox(height: 20),
                      KoButton(
                        key: const Key('oauth-switch-confirm'),
                        label: l.oauthSwitch,
                        onPressed: _busy ? null : _confirmSwitch,
                      ),
                      const SizedBox(height: 12),
                      KoButton(
                        key: const Key('oauth-cancel'),
                        label: l.oauthKeepCurrent,
                        color: KoColors.surface,
                        onPressed: _busy ? null : _cancel,
                      ),
                    ],
                  ),
                ),
              ] else if (_attempt != null) ...[
                KoPanel(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      const DoodleIcon(Doodle.clock, size: 48),
                      const SizedBox(height: 12),
                      Text(l.oauthPending),
                      const SizedBox(height: 20),
                      KoButton(
                        key: const Key('oauth-check'),
                        label: l.oauthCheck,
                        onPressed: _busy ? null : _poll,
                      ),
                      const SizedBox(height: 12),
                      KoButton(
                        key: const Key('oauth-open-browser'),
                        label: l.oauthOpenBrowser,
                        color: KoColors.surface,
                        onPressed: _busy ? null : _reopen,
                      ),
                      const SizedBox(height: 12),
                      KoButton(
                        key: const Key('oauth-cancel'),
                        label: l.oauthCancel,
                        color: KoColors.surface,
                        onPressed: _cancel,
                      ),
                    ],
                  ),
                ),
              ] else ...[
                KoPanel(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      KoButton(
                        key: const Key('oauth-intent-link'),
                        label: l.oauthLink,
                        icon: DoodleIcon(
                          _intent == OAuthIntent.link
                              ? Doodle.check
                              : Doodle.person,
                          size: 22,
                        ),
                        color: _intent == OAuthIntent.link
                            ? KoColors.lime
                            : KoColors.surface,
                        onPressed: _busy
                            ? null
                            : () => setState(() => _intent = OAuthIntent.link),
                      ),
                      const SizedBox(height: 12),
                      KoButton(
                        key: const Key('oauth-intent-restore'),
                        label: l.oauthRestore,
                        icon: DoodleIcon(
                          _intent == OAuthIntent.restore
                              ? Doodle.check
                              : Doodle.person,
                          size: 22,
                        ),
                        color: _intent == OAuthIntent.restore
                            ? KoColors.lime
                            : KoColors.surface,
                        onPressed: _busy
                            ? null
                            : () =>
                                  setState(() => _intent = OAuthIntent.restore),
                      ),
                      const SizedBox(height: 24),
                      KoButton(
                        key: const Key('oauth-google'),
                        label: l.oauthGoogle,
                        onPressed: _busy ? null : () => _start('google'),
                      ),
                      const SizedBox(height: 12),
                      KoButton(
                        key: const Key('oauth-facebook'),
                        label: l.oauthFacebook,
                        color: KoColors.surface,
                        onPressed: _busy ? null : () => _start('facebook'),
                      ),
                    ],
                  ),
                ),
              ],
            ],
          ],
        ),
      ),
    );
  }
}
