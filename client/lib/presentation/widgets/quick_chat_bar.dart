import 'package:flutter/material.dart';

import '../../l10n/app_localizations.dart';
import '../theme/knowoff_tokens.dart';
import 'ko_chip.dart';

/// Row of canned Quick Chat phrase chips.
class QuickChatBar extends StatelessWidget {
  const QuickChatBar({this.onPhrase, super.key});

  final ValueChanged<String>? onPhrase;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final phrases = <(String id, String label)>[
      ('suspect', l10n.quickChatSuspect),
      ('fit', l10n.quickChatFit),
      ('weird', l10n.quickChatWeird),
      ('trust', l10n.quickChatTrust),
      ('not_me', l10n.quickChatNotMe),
      ('laugh', l10n.quickChatLaugh),
    ];

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
