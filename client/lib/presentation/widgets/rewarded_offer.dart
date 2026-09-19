import 'dart:async';
import 'package:flutter/material.dart';
import '../../data/rewarded_session.dart';
import '../../l10n/app_localizations.dart';
import 'ko_ui.dart';

/// Readiness includes a server-authorized exact claim. A match hint alone never
/// renders a watch offer, and tapping still performs the final server check.
class RewardedOffer extends StatelessWidget {
  const RewardedOffer({
    required this.session,
    required this.candidate,
    super.key,
  });
  final RewardedSessionController session;
  final RewardedMatchCandidate candidate;
  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: session,
    builder: (context, _) {
      if (!session.ready ||
          session.candidate != candidate ||
          session.identity !=
              (
                accountId: candidate.accountId,
                generation: candidate.generation,
              )) {
        return const SizedBox.shrink();
      }
      final l = AppLocalizations.of(context);
      return KoPanel(
        color: KoColors.aqua,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            KoButton(
              key: const Key('reward-watch-ad'),
              label: l.rewardWatchAd,
              onPressed: session.busy ? null : () => unawaited(session.show()),
            ),
            const SizedBox(height: 12),
            Text(l.rewardAdDetails),
          ],
        ),
      );
    },
  );
}

/// Consent choices remain accessible without a match, purchase, or login.
class RewardedPrivacyButton extends StatelessWidget {
  const RewardedPrivacyButton({required this.session, super.key});
  final RewardedSessionController session;
  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: session,
    builder: (context, _) => session.privacyOptionsRequired
        ? IconButton(
            key: const Key('reward-privacy-options'),
            tooltip: AppLocalizations.of(context).rewardPrivacyChoices,
            constraints: const BoxConstraints(minWidth: 48, minHeight: 48),
            icon: const Icon(Icons.privacy_tip_outlined),
            onPressed: session.busy
                ? null
                : () => unawaited(session.showPrivacyOptions()),
          )
        : const SizedBox.shrink(),
  );
}
