import 'dart:async';
import 'package:flutter/material.dart';
import '../../data/bonus_session.dart';
import '../../l10n/app_localizations.dart';
import 'ko_ui.dart';
import 'service_components.dart';

/// Private, server-confirmed receipts. This view never credits a wallet locally.
class BonusReceipts extends StatelessWidget {
  const BonusReceipts({required this.session, super.key});
  final BonusSessionController session;

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: Listenable.merge([session, session.deliveries]),
    builder: (context, _) {
      final l = AppLocalizations.of(context);
      return Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        mainAxisSize: MainAxisSize.min,
        children: [
          for (final receipt in session.deliveries.receipts)
            Padding(
              padding: const EdgeInsets.only(bottom: 20),
              child: KoPanel(
                key: const Key('bonus-receipt'),
                color: KoColors.lime,
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    const DoodleIcon(Doodle.coin, size: 40),
                    Text(
                      receipt.payload.source == 'premium'
                          ? l.bonusPremium
                          : l.bonusRewardedAd,
                      style: koDisplayStyle(size: 26),
                    ),
                    const SizedBox(height: 12),
                    Text(
                      l.bonusCredited(
                        serviceNumber(context, receipt.payload.credited),
                      ),
                      style: koDisplayStyle(size: 32),
                    ),
                    Text(
                      l.bonusRequested(
                        serviceNumber(context, receipt.payload.requested),
                      ),
                    ),
                    if (receipt.payload.credited < receipt.payload.requested)
                      Text(l.bonusCapped),
                    const SizedBox(height: 16),
                    KoButton(
                      key: const Key('bonus-dismiss'),
                      label: l.close,
                      onPressed: () => unawaited(session.dismiss(receipt.id)),
                    ),
                  ],
                ),
              ),
            ),
          if (session.failed)
            Semantics(
              liveRegion: true,
              child: KoPanel(
                color: KoColors.pink,
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    const DoodleIcon(Doodle.clock, size: 32),
                    Text(l.bonusSyncRetry),
                    const SizedBox(height: 12),
                    KoButton(
                      key: const Key('bonus-retry'),
                      label: l.retry,
                      onPressed: () => unawaited(session.refresh()),
                    ),
                  ],
                ),
              ),
            ),
        ],
      );
    },
  );
}
