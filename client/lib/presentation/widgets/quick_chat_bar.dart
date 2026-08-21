import 'package:flutter/material.dart';

import '../../l10n/app_localizations.dart';
import '../theme/knowoff_tokens.dart';
import 'ko_chip.dart';

/// Canned Quick Chat phrase ids paired with their localized labels.
List<(String id, String label)> quickChatPhrases(AppLocalizations l10n) => [
      ('suspect', l10n.quickChatSuspect),
      ('fit', l10n.quickChatFit),
      ('weird', l10n.quickChatWeird),
      ('trust', l10n.quickChatTrust),
      ('not_me', l10n.quickChatNotMe),
      ('laugh', l10n.quickChatLaugh),
    ];

/// Resolves a Quick Chat phrase id to its localized label, for the received
/// chat feed as well as the send bar.
String quickChatPhraseLabel(AppLocalizations l10n, String? phraseId) {
  for (final phrase in quickChatPhrases(l10n)) {
    if (phrase.$1 == phraseId) return phrase.$2;
  }
  return phraseId ?? '';
}

/// Row of canned Quick Chat phrase chips.
class QuickChatBar extends StatelessWidget {
  const QuickChatBar({this.onPhrase, super.key});

  final ValueChanged<String>? onPhrase;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final phrases = quickChatPhrases(l10n);

    return SingleChildScrollView(
      scrollDirection: Axis.horizontal,
      child: Row(
        children: phrases.map((phrase) {
          return Padding(
            padding: const EdgeInsets.symmetric(horizontal: 4),
            child: KoChip(
              icon: const Icon(Icons.chat_bubble, size: 16),
              label: phrase.$2,
              color: KoColors.violet,
              onTap: onPhrase != null ? () => onPhrase!(phrase.$1) : null,
            ),
          );
        }).toList(),
      ),
    );
  }
}
