import 'dart:math' as math;

import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';

import '../theme/ko_breakpoints.dart';

enum _KoBodyMode { list, single, fill }

/// The page body of a [KoScaffold].
///
/// It owns three things every screen used to get wrong on its own:
///
/// 1. **Scrolling.** Nothing is allowed to be un-scrollable, so a short
///    browser window or a landscape phone never clips content off the bottom.
/// 2. **The content column.** The centring insets live *inside* the scroll
///    view, which keeps the scrollable viewport-wide — so the scrollbar lane
///    sits at the window edge instead of on top of the cards.
/// 3. **A persistent scrollbar** on pointer platforms, because on the web a
///    page that scrolls with no visible affordance reads as broken.
class KoBody extends StatefulWidget {
  /// A vertically scrolling page built from [children].
  const KoBody({required this.children, super.key})
      : child = null,
        _mode = _KoBodyMode.list;

  /// A single block of content — a loading state, an empty state, a queue
  /// beacon. Centred while it fits, scrollable the moment it does not.
  const KoBody.single({required Widget this.child, super.key})
      : children = const <Widget>[],
        _mode = _KoBodyMode.single;

  /// Content that manages its own inner scrolling and wants the full body
  /// height. Gets the content column insets but no scroll view of its own.
  const KoBody.fill({required Widget this.child, super.key})
      : children = const <Widget>[],
        _mode = _KoBodyMode.fill;

  final List<Widget> children;
  final Widget? child;
  final _KoBodyMode _mode;

  @override
  State<KoBody> createState() => _KoBodyState();
}

class _KoBodyState extends State<KoBody> {
  final ScrollController _controller = ScrollController();

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final layout = KoLayout.of(context);
    final padding = layout.contentPadding;

    if (widget._mode == _KoBodyMode.fill) {
      return Padding(padding: padding, child: widget.child);
    }

    final Widget scrollView = widget._mode == _KoBodyMode.list
        ? ListView(
            controller: _controller,
            primary: false,
            padding: padding,
            children: widget.children,
          )
        : LayoutBuilder(
            builder: (context, constraints) {
              final available = constraints.maxHeight.isFinite
                  ? math.max(0.0, constraints.maxHeight - padding.vertical)
                  : 0.0;
              return SingleChildScrollView(
                controller: _controller,
                primary: false,
                padding: padding,
                child: ConstrainedBox(
                  constraints: BoxConstraints(minHeight: available),
                  child: Center(child: widget.child),
                ),
              );
            },
          );

    return Scrollbar(
      controller: _controller,
      thumbVisibility: _persistentScrollbar,
      child: ScrollConfiguration(
        // Our own Scrollbar is the page scrollbar; suppress the implicit one
        // the platform behaviour would stack on top of it.
        behavior: ScrollConfiguration.of(context).copyWith(scrollbars: false),
        child: scrollView,
      ),
    );
  }

  /// Touch platforms fade their scrollbar in on scroll; pointer platforms —
  /// and every browser — expect it to be there before you touch anything.
  bool get _persistentScrollbar {
    if (kIsWeb) return true;
    switch (defaultTargetPlatform) {
      case TargetPlatform.linux:
      case TargetPlatform.macOS:
      case TargetPlatform.windows:
        return true;
      case TargetPlatform.android:
      case TargetPlatform.iOS:
      case TargetPlatform.fuchsia:
        return false;
    }
  }
}
