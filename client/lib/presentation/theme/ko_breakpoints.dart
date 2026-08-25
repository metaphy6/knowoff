import 'dart:math' as math;

import 'package:flutter/widgets.dart';

import 'knowoff_tokens.dart';

/// Window-size classes, phone-first.
///
/// Knowoff is designed for [KoBreakpoint.compact]; the wider classes only
/// widen the content column and let tile grids gain a track. Nothing gets a
/// different information architecture, so there is exactly one layout to
/// reason about per screen.
enum KoBreakpoint { compact, medium, expanded }

/// Where one window-size class ends and the next begins, plus the two
/// secondary thresholds the design system reacts to.
abstract final class KoBreakpoints {
  /// Material 3 window-size class boundaries.
  static const double medium = 600;
  static const double expanded = 900;

  /// Below this, side gutters shrink and stacked headers replace inline rows.
  static const double tight = 380;

  /// Landscape phones and short browser windows: trim the vertical rhythm so
  /// the primary action still lands above the fold.
  static const double short = 560;

  static KoBreakpoint of(double width) {
    if (width < medium) return KoBreakpoint.compact;
    if (width < expanded) return KoBreakpoint.medium;
    return KoBreakpoint.expanded;
  }
}

/// The resolved layout facts for the current window.
///
/// Every responsive decision in the client reads from here rather than
/// inventing its own `MediaQuery.of(context).size.width < 600` check, so the
/// breakpoints stay in one place and stay testable without a widget tree.
@immutable
class KoLayout {
  const KoLayout({required this.size, required this.breakpoint});

  factory KoLayout.fromSize(Size size) =>
      KoLayout(size: size, breakpoint: KoBreakpoints.of(size.width));

  /// Reads the layout for [context]. Depends on the window size only, so it
  /// rebuilds on resize and on orientation change.
  static KoLayout of(BuildContext context) =>
      KoLayout.fromSize(MediaQuery.sizeOf(context));

  final Size size;
  final KoBreakpoint breakpoint;

  bool get isCompact => breakpoint == KoBreakpoint.compact;
  bool get isMedium => breakpoint == KoBreakpoint.medium;
  bool get isExpanded => breakpoint == KoBreakpoint.expanded;

  /// Narrow enough that inline label + chip rows have to stack.
  bool get isTight => size.width < KoBreakpoints.tight;

  /// Short enough that fixed-height decorations must shrink.
  bool get isShort => size.height < KoBreakpoints.short;

  /// The widest the content column is ever allowed to get.
  ///
  /// Compact is the window itself — the column already *is* the screen. The
  /// wider classes stop the page turning into a 2000 px line-length.
  double get contentMaxWidth {
    switch (breakpoint) {
      case KoBreakpoint.compact:
        return size.width;
      case KoBreakpoint.medium:
        return 680;
      case KoBreakpoint.expanded:
        return 860;
    }
  }

  /// Minimum breathing room between the content column and the window edge.
  double get gutter {
    if (isTight) return KoSpace.md;
    if (isCompact) return KoSpace.lg;
    return KoSpace.xl;
  }

  /// Padding a full-bleed page scroll view uses to centre its content column.
  ///
  /// The insets live *inside* the scroll view on purpose: that keeps the
  /// scrollable itself viewport-wide, so its scrollbar lane sits at the window
  /// edge instead of on top of the content column.
  EdgeInsets get contentPadding {
    final double side = math.max(gutter, (size.width - contentMaxWidth) / 2);
    return EdgeInsets.fromLTRB(
      side,
      isShort ? KoSpace.md : KoSpace.lg,
      side,
      isShort ? KoSpace.lg : KoSpace.xxl,
    );
  }

  /// Horizontal insets for chrome that is not inside the page scroll view —
  /// the header band and the bottom bar.
  EdgeInsets get chromePadding =>
      EdgeInsets.symmetric(horizontal: gutter, vertical: KoSpace.md);

  /// Track count for a tile grid. [medium] defaults to [compact] and
  /// [expanded] to [medium], so callers only name the steps they want.
  int columns({required int compact, int? medium, int? expanded}) {
    switch (breakpoint) {
      case KoBreakpoint.compact:
        return compact;
      case KoBreakpoint.medium:
        return medium ?? compact;
      case KoBreakpoint.expanded:
        return expanded ?? medium ?? compact;
    }
  }

  /// Size of one card in the hand fan — cards are square, so this is both
  /// width and height. Deliberately enlarged so image/text cards read well.
  double get handCardSize {
    if (isTight) return 136;
    if (isCompact) return 152;
    if (isMedium) return 168;
    return 184;
  }

  /// Height of the hand-fan rail, including the lift room a selected card
  /// needs, the hard shadow underneath it, and the now-much-larger role square.
  double get handFanHeight => isShort ? 300 : 380;

  /// Ceiling for the Nown stage's picture area.
  ///
  /// Nown is the round's reference, so it gets more real estate than before,
  /// but the hand and turn action still have to stay reachable without scroll.
  double get nownStageMaxHeight {
    if (isShort) return 220;
    if (isTight) return 260;
    if (isCompact) return 320;
    return 380;
  }

  /// Edge length of a seat avatar in the turn rail.
  double get seatAvatarSize {
    if (isTight) return 56;
    if (isCompact) return 64;
    return 72;
  }

  /// Edge length of the reveal-your-role square above the draw pile.
  ///
  /// Scaled back slightly from the oversized 208dp but still large; it shrinks
  /// on short windows so the draw pile underneath keeps room for its own face.
  double get roleSquareSize => isShort ? 136 : 168;

  @override
  bool operator ==(Object other) =>
      other is KoLayout && other.size == size && other.breakpoint == breakpoint;

  @override
  int get hashCode => Object.hash(size, breakpoint);
}
