import 'package:flutter/material.dart';

import '../../data/models/game_state_dto.dart';
import '../../l10n/app_localizations.dart';

/// Renders a card's actual content — text, image, or GIF — never its raw id.
class CardFace extends StatelessWidget {
  const CardFace({required this.card, this.compact = false, super.key});

  final CardDto card;
  final bool compact;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    if (card.type == 'text') {
      return Text(
        card.content ?? card.id,
        maxLines: compact ? 2 : null,
        overflow: compact ? TextOverflow.ellipsis : TextOverflow.clip,
      );
    }
    final url = card.signedUrl;
    if (url == null || url.isEmpty) {
      return Text(l10n.genericError);
    }
    return Image.network(
      url,
      fit: BoxFit.cover,
      height: compact ? 48 : null,
      errorBuilder: (context, error, stack) => Text(l10n.genericError),
    );
  }
}
