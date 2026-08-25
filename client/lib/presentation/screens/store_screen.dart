import 'package:flutter/material.dart';

import '../../core/config/app_config.dart';
import '../../data/api_client.dart';
import '../../l10n/app_localizations.dart';
import '../icons/doodles.dart';
import '../theme/knowoff_tokens.dart';
import '../theme/knowoff_typography.dart';
import '../theme/ko_breakpoints.dart';
import '../widgets/convert_points_dialog.dart';
import '../widgets/ko_body.dart';
import '../widgets/ko_button.dart';
import '../widgets/ko_container.dart';
import '../widgets/ko_scaffold.dart';
import '../widgets/ko_stat_tile.dart';
import '../widgets/noin_badge.dart';

/// The player-facing store: Noin balance, Play Passes, Noin bulks, unlocks,
/// Premium subscription, and points-to-Noin conversion (💰).
class StoreScreen extends StatefulWidget {
  const StoreScreen({this.api, super.key});

  final ApiClient? api;

  @override
  State<StoreScreen> createState() => _StoreScreenState();
}

class _StoreScreenState extends State<StoreScreen> {
  Map<String, dynamic>? _wallet;
  Map<String, dynamic>? _catalog;
  Object? _error;
  bool _loading = true;
  late final ApiClient _api = widget.api ??
      ApiClient(
        baseUrl: AppConfig.instance.serverUrl,
        auth: AppConfig.instance.authService,
      );

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    try {
      final wallet = await _api.getWallet();
      final catalog = await _api.getStoreCatalog();
      if (!mounted) return;
      setState(() {
        _wallet = wallet;
        _catalog = catalog;
        _loading = false;
        _error = null;
      });
    } catch (e) {
      if (!mounted) return;
      setState(() {
        _error = e;
        _loading = false;
      });
    }
  }

  Future<void> _buyPlayPass(String type) =>
      _purchase(() => _api.purchasePlayPass(type));

  Future<void> _buyUnlock(String type, {String value = ''}) =>
      _purchase(() => _api.purchaseUnlock(type, value: value));

  Future<void> _purchase(Future<void> Function() call) async {
    final l10n = AppLocalizations.of(context);
    try {
      await call();
      await _load();
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text(l10n.storeBought)),
        );
      }
    } catch (_) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text(l10n.storeError)),
        );
      }
    }
  }

  void _showConvertDialog() {
    final pointsToNoin = (_catalog?['points_to_noin'] as num?)?.toInt() ?? 100;
    showDialog<void>(
      context: context,
      builder: (context) => ConvertPointsDialog(pointsToNoin: pointsToNoin),
    ).then((_) => _load());
  }

  int _noinBalance() => (_wallet?['noin'] as num?)?.toInt() ?? 0;

  Map<String, dynamic> _playPassPrices() =>
      (_catalog?['play_pass_prices'] as Map<String, dynamic>?) ?? const {};

  List<dynamic> _noinBundles() =>
      (_catalog?['noin_bundles'] as List<dynamic>?) ?? const [];

  Map<String, dynamic> _unlockPrices() =>
      (_catalog?['unlock_prices'] as Map<String, dynamic>?) ?? const {};

  int _yearlyDiscount() =>
      (_catalog?['premium_yearly_discount_pct'] as num?)?.toInt() ?? 20;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    return KoScaffold(
      title: l10n.storeTitle,
      accent: KoColors.pink,
      leadingGlyph: const DoodleIcon(Doodle.coin, size: 30),
      actions: _loading || _error != null
          ? const <Widget>[]
          : <Widget>[
              NoinBadge(
                balance: _noinBalance(),
                label: l10n.storeBalanceLabel,
                compact: true,
              ),
            ],
      body: _body(context, l10n),
    );
  }

  Widget _body(BuildContext context, AppLocalizations l10n) {
    if (_loading) {
      return KoBody.single(child: KoLoading(label: l10n.loadingLabel));
    }
    if (_error != null) {
      return KoBody.single(
        child: KoEmptyState(
          doodle: Doodle.cross,
          message: l10n.genericError,
          accent: KoColors.pink,
          action: KoButton(label: l10n.retry, onTap: _load),
        ),
      );
    }

    return KoBody(
      children: <Widget>[
        KoButton(
          label: l10n.convertPointsTitle,
          subLabel: l10n.convertPointsHint,
          size: KoButtonSize.large,
          expand: true,
          backgroundColor: KoColors.lime,
          shadow: KoShadows.lg,
          icon: const DoodleIcon(Doodle.sparkle, size: 26),
          trailing: const Icon(Icons.arrow_forward, size: 24),
          onTap: _showConvertDialog,
        ),
        const SizedBox(height: KoSpace.xl),
        KoSectionHeader(
          label: l10n.storePlayPasses,
          glyph: const DoodleIcon(Doodle.clock, size: 20),
          accent: KoColors.violet,
        ),
        _OfferRow(
          label: l10n.storePlayPassDay1,
          price: (_playPassPrices()['day_1'] as num?)?.toInt(),
          accent: KoColors.surface,
          glyph: Doodle.clock,
          onBuy: () => _buyPlayPass('day_1'),
        ),
        _OfferRow(
          label: l10n.storePlayPassDay3,
          price: (_playPassPrices()['day_3'] as num?)?.toInt(),
          accent: KoColors.surface,
          glyph: Doodle.clock,
          onBuy: () => _buyPlayPass('day_3'),
        ),
        _OfferRow(
          label: l10n.storePlayPassDay7,
          price: (_playPassPrices()['day_7'] as num?)?.toInt(),
          accent: KoColors.aqua,
          glyph: Doodle.clock,
          onBuy: () => _buyPlayPass('day_7'),
        ),
        const SizedBox(height: KoSpace.lg),
        KoSectionHeader(
          label: l10n.storeNoinBulks,
          glyph: const DoodleIcon(Doodle.coin, size: 20),
          accent: KoColors.tangerine,
        ),
        GridView.count(
          crossAxisCount: KoLayout.of(context).columns(compact: 3, medium: 4),
          shrinkWrap: true,
          primary: false,
          physics: const NeverScrollableScrollPhysics(),
          mainAxisSpacing: KoSpace.md,
          crossAxisSpacing: KoSpace.md,
          childAspectRatio: 1.05,
          children: <Widget>[
            for (var i = 0; i < _noinBundles().length; i++)
              KoStatTile(
                value: '${(_noinBundles()[i] as num).toInt()}',
                label: l10n.storeBalanceLabel,
                accent: KoColors.tangerine,
                glyph: Doodle.coin,
                numeralSize: 24,
                rotation: KoTilt.alternating(i),
              ),
          ],
        ),
        const SizedBox(height: KoSpace.lg),
        KoSectionHeader(
          label: l10n.storeUnlocks,
          glyph: const DoodleIcon(Doodle.sparkle, size: 20),
          accent: KoColors.lime,
        ),
        _OfferRow(
          label: l10n.storeUnlockCustomAvatar,
          price: (_unlockPrices()['custom_avatar'] as num?)?.toInt(),
          accent: KoColors.surface,
          glyph: Doodle.eye,
          onBuy: () => _buyUnlock('custom_avatar'),
        ),
        _OfferRow(
          label: l10n.storeUnlockPokeStyle,
          price: (_unlockPrices()['poke_style'] as num?)?.toInt(),
          accent: KoColors.surface,
          glyph: Doodle.poke,
          onBuy: () => _buyUnlock('poke_style'),
        ),
        _OfferRow(
          label: l10n.storeUnlockThemePack,
          price: (_unlockPrices()['theme_pack'] as num?)?.toInt(),
          accent: KoColors.surface,
          glyph: Doodle.cards,
          onBuy: () => _buyUnlock('theme_pack'),
        ),
        const SizedBox(height: KoSpace.lg),
        KoSectionHeader(
          label: l10n.storePremium,
          glyph: const DoodleIcon(Doodle.crown, size: 20),
          accent: KoColors.pink,
        ),
        _PremiumCard(
          monthly: l10n.storePremiumMonthly,
          yearly: l10n.storePremiumYearly,
          discount: l10n.storePremiumYearlyDiscount(_yearlyDiscount()),
          blurb: l10n.storePremiumBlurb,
        ),
      ],
    );
  }
}

class _OfferRow extends StatelessWidget {
  const _OfferRow({
    required this.label,
    required this.price,
    required this.accent,
    required this.glyph,
    required this.onBuy,
  });

  final String label;
  final int? price;
  final Color accent;
  final Doodle glyph;
  final VoidCallback onBuy;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);

    return Padding(
      padding: const EdgeInsets.only(bottom: KoSpace.sm),
      child: KoContainer(
        backgroundColor: accent,
        padding: const EdgeInsets.all(KoSpace.md),
        child: Row(
          children: <Widget>[
            DoodleIcon(glyph, size: 24),
            const SizedBox(width: KoSpace.md),
            Expanded(
              child: Text(
                label,
                style: Theme.of(context).textTheme.titleLarge,
              ),
            ),
            if (price != null)
              KoButton(
                label: l10n.storeBuyPrice(price!),
                size: KoButtonSize.small,
                backgroundColor: KoColors.tangerine,
                icon: const DoodleIcon(Doodle.coin, size: 16),
                onTap: onBuy,
              ),
          ],
        ),
      ),
    );
  }
}

class _PremiumCard extends StatelessWidget {
  const _PremiumCard({
    required this.monthly,
    required this.yearly,
    required this.discount,
    required this.blurb,
  });

  final String monthly;
  final String yearly;
  final String discount;
  final String blurb;

  @override
  Widget build(BuildContext context) {
    final text = Theme.of(context).textTheme;

    return KoContainer(
      backgroundColor: KoColors.violet,
      borderWidth: KoBorders.thick,
      shadow: KoShadows.lg,
      padding: const EdgeInsets.all(KoSpace.lg),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: <Widget>[
          Row(
            children: <Widget>[
              Transform.rotate(
                angle: KoTilt.loud,
                child: const DoodleIcon(Doodle.crown, size: 30),
              ),
              const SizedBox(width: KoSpace.md),
              Expanded(
                child: Text(blurb, style: text.headlineSmall),
              ),
            ],
          ),
          const SizedBox(height: KoSpace.lg),
          Row(
            children: <Widget>[
              Expanded(
                child: _PlanTile(name: monthly, note: null),
              ),
              const SizedBox(width: KoSpace.md),
              Expanded(
                child: _PlanTile(name: yearly, note: discount),
              ),
            ],
          ),
        ],
      ),
    );
  }
}

class _PlanTile extends StatelessWidget {
  const _PlanTile({required this.name, required this.note});

  final String name;
  final String? note;

  @override
  Widget build(BuildContext context) {
    final text = Theme.of(context).textTheme;

    return Container(
      padding: const EdgeInsets.all(KoSpace.md),
      decoration: BoxDecoration(
        color: note == null ? KoColors.whiteWell : KoColors.lime,
        border: Border.all(width: KoBorders.regular, color: KoColors.ink),
        borderRadius: BorderRadius.circular(KoRadii.card),
        boxShadow: const <BoxShadow>[KoShadows.sm],
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: <Widget>[
          Text(name, style: koDisplayStyle(size: 22, height: 1.0)),
          if (note != null) ...<Widget>[
            const SizedBox(height: KoSpace.xs),
            Text(note!, style: text.labelMedium),
          ],
        ],
      ),
    );
  }
}
