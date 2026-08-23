import 'package:flutter/material.dart';

import '../theme/knowoff_tokens.dart';
import '../theme/ko_canvas_grid.dart';

/// Knowoff is a phone-first game; on a desktop browser the PWA centres its
/// content in a phone-shaped column instead of stretching tiles across 2000 px.
const double kKoContentMaxWidth = 620;

/// Centres [child] inside the house content column.
///
/// [hugHeight] must be set wherever the column sits in a slot that hands down
/// a loose height — a header band, a bottom bar — otherwise the centring
/// [Align] expands to the full available height and starves its siblings.
class KoPageWidth extends StatelessWidget {
  const KoPageWidth({
    required this.child,
    this.hugHeight = false,
    super.key,
  });

  final Widget child;
  final bool hugHeight;

  @override
  Widget build(BuildContext context) {
    return Align(
      alignment: Alignment.center,
      heightFactor: hugHeight ? 1.0 : null,
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: kKoContentMaxWidth),
        child: child,
      ),
    );
  }
}

/// The house page frame.
///
/// Replaces `Scaffold` + `AppBar` everywhere: a full-bleed lavender field with
/// the faint grid tile, a banded brutalist header carrying an oversized
/// display title, and a slot for status chips. Every screen picks an [accent]
/// so the app stops reading as one repeated lavender rectangle.
class KoScaffold extends StatelessWidget {
  const KoScaffold({
    required this.title,
    required this.body,
    this.subtitle,
    this.accent = KoColors.violet,
    this.canvasColor = KoColors.canvas,
    this.leadingGlyph,
    this.actions = const <Widget>[],
    this.statusBar,
    this.bottomBar,
    this.showBack = true,
    this.padding = const EdgeInsets.fromLTRB(
      KoSpace.lg,
      KoSpace.lg,
      KoSpace.lg,
      KoSpace.xl,
    ),
    super.key,
  });

  final String title;
  final String? subtitle;
  final Widget body;
  final Color accent;
  final Color canvasColor;

  /// Small glyph stamped into the header's accent tile.
  final Widget? leadingGlyph;
  final List<Widget> actions;

  /// A row of chips pinned under the header — timers, budgets, balances.
  final Widget? statusBar;
  final Widget? bottomBar;
  final bool showBack;
  final EdgeInsetsGeometry padding;

  @override
  Widget build(BuildContext context) {
    final canPop = showBack && Navigator.of(context).canPop();

    return Scaffold(
      backgroundColor: canvasColor,
      body: Stack(
        children: <Widget>[
          const Positioned.fill(
            child: CustomPaint(painter: KoCanvasGridPainter()),
          ),
          SafeArea(
            bottom: false,
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: <Widget>[
                _Header(
                  title: title,
                  subtitle: subtitle,
                  accent: accent,
                  glyph: leadingGlyph,
                  actions: actions,
                  onBack: canPop ? () => Navigator.of(context).pop() : null,
                ),
                if (statusBar != null)
                  KoPageWidth(
                    hugHeight: true,
                    child: Padding(
                      padding: const EdgeInsets.fromLTRB(
                        KoSpace.lg,
                        KoSpace.md,
                        KoSpace.lg,
                        0,
                      ),
                      child: statusBar,
                    ),
                  ),
                Expanded(
                  child: KoPageWidth(
                    child: Padding(padding: padding, child: body),
                  ),
                ),
              ],
            ),
          ),
        ],
      ),
      bottomNavigationBar: bottomBar == null
          ? null
          : Container(
              decoration: const BoxDecoration(
                color: KoColors.surface,
                border: Border(
                  top: BorderSide(
                    width: KoBorders.regular,
                    color: KoColors.ink,
                  ),
                ),
              ),
              child: SafeArea(
                top: false,
                child: KoPageWidth(
                  hugHeight: true,
                  child: Padding(
                    padding: const EdgeInsets.all(KoSpace.md),
                    child: bottomBar,
                  ),
                ),
              ),
            ),
    );
  }
}

class _Header extends StatelessWidget {
  const _Header({
    required this.title,
    required this.subtitle,
    required this.accent,
    required this.glyph,
    required this.actions,
    required this.onBack,
  });

  final String title;
  final String? subtitle;
  final Color accent;
  final Widget? glyph;
  final List<Widget> actions;
  final VoidCallback? onBack;

  @override
  Widget build(BuildContext context) {
    final text = Theme.of(context).textTheme;

    return Container(
      padding: const EdgeInsets.fromLTRB(
        KoSpace.lg,
        KoSpace.md,
        KoSpace.lg,
        KoSpace.md,
      ),
      decoration: BoxDecoration(
        color: accent,
        border: const Border(
          bottom: BorderSide(width: KoBorders.thick, color: KoColors.ink),
        ),
      ),
      child: KoPageWidth(
        hugHeight: true,
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.center,
          children: <Widget>[
            if (onBack != null) ...<Widget>[
              _SquareIconButton(
                icon: Icons.arrow_back,
                onTap: onBack!,
                semanticLabel:
                    MaterialLocalizations.of(context).backButtonTooltip,
              ),
              const SizedBox(width: KoSpace.md),
            ] else if (glyph != null) ...<Widget>[
              Transform.rotate(angle: KoTilt.soft, child: glyph),
              const SizedBox(width: KoSpace.md),
            ],
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                mainAxisSize: MainAxisSize.min,
                children: <Widget>[
                  Text(
                    title,
                    style: text.headlineMedium,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                  ),
                  if (subtitle != null)
                    Text(
                      subtitle!,
                      style: text.bodyMedium,
                      maxLines: 2,
                      overflow: TextOverflow.ellipsis,
                    ),
                ],
              ),
            ),
            for (final action in actions) ...<Widget>[
              const SizedBox(width: KoSpace.sm),
              action,
            ],
          ],
        ),
      ),
    );
  }
}

class _SquareIconButton extends StatefulWidget {
  const _SquareIconButton({
    required this.icon,
    required this.onTap,
    required this.semanticLabel,
  });

  final IconData icon;
  final VoidCallback onTap;
  final String semanticLabel;

  @override
  State<_SquareIconButton> createState() => _SquareIconButtonState();
}

class _SquareIconButtonState extends State<_SquareIconButton> {
  bool _pressed = false;

  @override
  Widget build(BuildContext context) {
    return Semantics(
      button: true,
      label: widget.semanticLabel,
      child: GestureDetector(
        onTapDown: (_) => setState(() => _pressed = true),
        onTapCancel: () => setState(() => _pressed = false),
        onTapUp: (_) {
          setState(() => _pressed = false);
          widget.onTap();
        },
        child: AnimatedContainer(
          duration: KoMotion.press,
          width: 48,
          height: 48,
          decoration: BoxDecoration(
            color: KoColors.surface,
            border: Border.all(width: KoBorders.regular, color: KoColors.ink),
            borderRadius: BorderRadius.circular(KoRadii.button),
            boxShadow: <BoxShadow>[
              _pressed ? KoShadows.pressed : KoShadows.sm,
            ],
          ),
          child: Transform.translate(
            offset: _pressed ? const Offset(3, 3) : Offset.zero,
            child: Icon(widget.icon, size: 22, color: KoColors.ink),
          ),
        ),
      ),
    );
  }
}

/// Section label: a stamped accent tile plus an oversized title. The tile is
/// tilted; the text is not, so scanning stays mechanical.
class KoSectionHeader extends StatelessWidget {
  const KoSectionHeader({
    required this.label,
    this.glyph,
    this.accent = KoColors.lime,
    this.trailing,
    super.key,
  });

  final String label;
  final Widget? glyph;
  final Color accent;
  final Widget? trailing;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(bottom: KoSpace.md, top: KoSpace.sm),
      child: Row(
        children: <Widget>[
          if (glyph != null) ...<Widget>[
            Transform.rotate(
              angle: KoTilt.subtle,
              child: Container(
                padding: const EdgeInsets.all(KoSpace.xs),
                decoration: BoxDecoration(
                  color: accent,
                  border: Border.all(
                    width: KoBorders.regular,
                    color: KoColors.ink,
                  ),
                  borderRadius: BorderRadius.circular(KoRadii.well),
                  boxShadow: const <BoxShadow>[KoShadows.sm],
                ),
                child: glyph,
              ),
            ),
            const SizedBox(width: KoSpace.md),
          ],
          Expanded(
            child: Text(
              label,
              style: Theme.of(context).textTheme.headlineSmall,
            ),
          ),
          if (trailing != null) trailing!,
        ],
      ),
    );
  }
}
