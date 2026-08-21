import 'package:flutter/material.dart';

import '../../core/config/app_config.dart';
import '../../data/api_client.dart';
import '../../l10n/app_localizations.dart';
import '../theme/knowoff_tokens.dart';
import '../widgets/convert_points_dialog.dart';
import '../widgets/ko_button.dart';
import '../widgets/ko_chip.dart';
import '../widgets/ko_container.dart';

/// The player-facing store: Noin balance, Play Passes, Noin bulks, unlocks,
/// Premium subscription, and points-to-Noin conversion.
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
      setState(() {
        _wallet = wallet;
        _catalog = catalog;
        _loading = false;
        _error = null;
      });
    } catch (e) {
      setState(() {
        _error = e;
        _loading = false;
      });
    }
  }

  Future<void> _buyPlayPass(String type) async {
    await _purchase(() => _api.purchasePlayPass(type));
  }

  Future<void> _buyUnlock(String type, {String value = ''}) async {
    await _purchase(() => _api.purchaseUnlock(type, value: value));
  }

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
    } catch (e) {
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
      builder: (context) => ConvertPointsDialog(
        pointsToNoin: pointsToNoin,
      ),
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
    return Scaffold(
      backgroundColor: KoColors.canvas,
      appBar: AppBar(title: Text(l10n.storeTitle)),
      body: _body(context, l10n),
    );
  }

  Widget _body(BuildContext context, AppLocalizations l10n) {
    if (_loading) return const Center(child: CircularProgressIndicator());
    if (_error != null) {
      return Center(
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Text(l10n.genericError),
            const SizedBox(height: 12),
            KoButton(label: l10n.retry, onTap: _load),
          ],
        ),
      );
    }
    return SingleChildScrollView(
      padding: const EdgeInsets.all(16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          _balanceChip(l10n),
          const SizedBox(height: 8),
          KoButton(
            label: l10n.convertPointsTitle,
            backgroundColor: KoColors.lime,
            onTap: _showConvertDialog,
          ),
          const SizedBox(height: 24),
          _sectionTitle(l10n.storePlayPasses),
          _playPassTile(l10n.storePlayPassDay1, 'day_1'),
          _playPassTile(l10n.storePlayPassDay3, 'day_3'),
          _playPassTile(l10n.storePlayPassDay7, 'day_7'),
          const SizedBox(height: 24),
          _sectionTitle(l10n.storeNoinBulks),
          ..._noinBundles().map((size) => _bulkTile((size as num).toInt())),
          const SizedBox(height: 24),
          _sectionTitle(l10n.storeUnlocks),
          _unlockTile(
            l10n.storeUnlockCustomAvatar,
            'custom_avatar',
          ),
          _unlockTile(
            l10n.storeUnlockPokeStyle,
            'poke_style',
          ),
          _unlockTile(
            l10n.storeUnlockThemePack,
            'theme_pack',
          ),
          const SizedBox(height: 24),
          _sectionTitle(l10n.storePremium),
          _premiumCard(l10n),
        ],
      ),
    );
  }

  Widget _balanceChip(AppLocalizations l10n) {
    return KoChip(
      icon: const Icon(Icons.monetization_on, size: 16),
      label: l10n.storeNoinBalance(_noinBalance()),
      color: KoColors.lime,
    );
  }

  Widget _sectionTitle(String title) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 8),
      child: Text(
        title,
        style: const TextStyle(fontSize: 18, fontWeight: FontWeight.bold),
      ),
    );
  }

  Widget _playPassTile(String label, String type) {
    final l10n = AppLocalizations.of(context);
    final price = (_playPassPrices()[type] as num?)?.toInt();
    return Padding(
      padding: const EdgeInsets.only(bottom: 8),
      child: KoContainer(
        padding: const EdgeInsets.all(12),
        child: Row(
          mainAxisAlignment: MainAxisAlignment.spaceBetween,
          children: [
            Text(label),
            if (price != null)
              KoButton(
                label: l10n.storeBuyPrice(price),
                onTap: () => _buyPlayPass(type),
              ),
          ],
        ),
      ),
    );
  }

  Widget _bulkTile(int size) {
    final l10n = AppLocalizations.of(context);
    return Padding(
      padding: const EdgeInsets.only(bottom: 8),
      child: KoContainer(
        padding: const EdgeInsets.all(12),
        child: Row(
          mainAxisAlignment: MainAxisAlignment.spaceBetween,
          children: [
            Text(l10n.storeBulkSize(size)),
            const Icon(Icons.shopping_cart_outlined),
          ],
        ),
      ),
    );
  }

  Widget _unlockTile(String label, String type) {
    final l10n = AppLocalizations.of(context);
    final price = (_unlockPrices()[type] as num?)?.toInt();
    return Padding(
      padding: const EdgeInsets.only(bottom: 8),
      child: KoContainer(
        padding: const EdgeInsets.all(12),
        child: Row(
          mainAxisAlignment: MainAxisAlignment.spaceBetween,
          children: [
            Text(label),
            if (price != null)
              KoButton(
                label: l10n.storeBuyPrice(price),
                onTap: () => _buyUnlock(type),
              ),
          ],
        ),
      ),
    );
  }

  Widget _premiumCard(AppLocalizations l10n) {
    final discount = _yearlyDiscount();
    return KoContainer(
      padding: const EdgeInsets.all(16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(l10n.storePremiumMonthly),
          Text(l10n.storePremiumYearly),
          Text(l10n.storePremiumYearlyDiscount(discount)),
        ],
      ),
    );
  }
}
