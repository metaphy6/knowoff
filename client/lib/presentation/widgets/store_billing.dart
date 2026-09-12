import 'package:flutter/material.dart';
import 'package:url_launcher/url_launcher.dart';
import '../../data/purchase_controller.dart';
import '../../l10n/app_localizations.dart';
import 'ko_ui.dart';
import 'service_components.dart';

class StoreBilling extends StatelessWidget {
  const StoreBilling({
    required this.purchases,
    this.managementPlatforms = const [],
    super.key,
  });
  final PurchaseController purchases;
  final List<String> managementPlatforms;
  @override
  Widget build(BuildContext context) => ListenableBuilder(
    listenable: purchases,
    builder: (context, _) {
      final l = AppLocalizations.of(context), p = purchases;
      final label = switch (p.status) {
        PurchaseFlowStatus.opening => l.purchaseOpening,
        PurchaseFlowStatus.pending => l.purchasePending,
        PurchaseFlowStatus.verifying => l.purchaseVerifying,
        PurchaseFlowStatus.verified => l.purchaseVerified,
        PurchaseFlowStatus.canceled => l.purchaseCanceled,
        PurchaseFlowStatus.failed => l.purchaseFailed,
        PurchaseFlowStatus.retry => l.purchaseRetryMessage,
        PurchaseFlowStatus.accountRequired => l.purchaseAccountRequired,
        _ => null,
      };
      return KoPanel(
        key: const Key('store-native-billing'),
        color: KoColors.aqua,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            KoHeading(
              title: l.purchaseTitle,
              trailing: const DoodleIcon(Doodle.crown, size: 42),
            ),
            if (label != null)
              Semantics(
                liveRegion: true,
                child: Text(label, key: const Key('native-purchase-status')),
              ),
            if (p.offers.isEmpty) Text(l.serviceBillingUnavailable),
            if (p.premiumOwned) Text(l.purchaseOwned),
            for (final offer in p.offers) ...[
              const SizedBox(height: 16),
              Text(
                offer.product.consumable
                    ? l.storeBulkSize(offer.product.noin)
                    : l.storePremium,
                style: koDisplayStyle(size: 24),
              ),
              Text(offer.title),
              if (!offer.product.consumable)
                Text(
                  offer.product.kind == 'premium_monthly'
                      ? l.purchaseMonthlyTerms
                      : l.purchaseYearlyTerms,
                ),
              KoButton(
                key: Key('native-buy-${offer.product.id}'),
                label: l.purchaseBuyPrice(offer.price),
                onPressed:
                    p.canBuy &&
                        (offer.product.consumable || p.canBuySubscription)
                    ? () => p.buy(offer)
                    : null,
              ),
            ],
            const SizedBox(height: 16),
            Text(l.purchaseRestoreHint),
            const SizedBox(height: 12),
            Wrap(
              spacing: 12,
              runSpacing: 12,
              children: [
                KoButton(
                  key: const Key('native-restore'),
                  label: l.purchaseRestore,
                  onPressed: p.supported && !p.busy ? () => p.restore() : null,
                ),
                if (p.canRetry)
                  KoButton(
                    key: const Key('native-retry'),
                    label: l.purchaseRetry,
                    onPressed: p.busy ? null : () => p.retry(),
                  ),
                for (final platform in managementPlatforms)
                  KoButton(
                    key: Key('native-manage-$platform'),
                    label: platform == 'app_store'
                        ? l.purchaseManageApple
                        : l.purchaseManageGoogle,
                    onPressed: () async {
                      final uri = Uri.parse(
                        platform == 'app_store'
                            ? 'https://account.apple.com/account/manage/section/subscriptions'
                            : 'https://play.google.com/store/account/subscriptions',
                      );
                      try {
                        if (!await launchUrl(
                          uri,
                          mode: LaunchMode.externalApplication,
                        )) {
                          throw StateError('unavailable');
                        }
                      } catch (_) {
                        if (context.mounted) {
                          serviceMessage(context, l.purchaseFailed);
                        }
                      }
                    },
                  ),
              ],
            ),
          ],
        ),
      );
    },
  );
}
