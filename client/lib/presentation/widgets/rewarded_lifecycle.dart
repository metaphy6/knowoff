import 'package:flutter/widgets.dart';
import '../../data/rewarded_session.dart';

/// Owns consent and loaded ads above navigation without creating an account.
class RewardedLifecycle extends StatefulWidget {
  const RewardedLifecycle({
    required this.session,
    required this.child,
    super.key,
  });
  final RewardedSessionController session;
  final Widget child;
  @override
  State<RewardedLifecycle> createState() => _RewardedLifecycleState();
}

class _RewardedLifecycleState extends State<RewardedLifecycle>
    with WidgetsBindingObserver {
  bool get _foreground => switch (WidgetsBinding.instance.lifecycleState) {
    AppLifecycleState.paused ||
    AppLifecycleState.hidden ||
    AppLifecycleState.detached => false,
    _ => true,
  };
  void _start() {
    widget.session.setForeground(_foreground);
    widget.session.start();
  }

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    _start();
  }

  @override
  void didUpdateWidget(RewardedLifecycle oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (!identical(oldWidget.session, widget.session)) {
      oldWidget.session.dispose();
      _start();
    }
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state == AppLifecycleState.resumed) {
      widget.session.setForeground(true);
    } else if (state == AppLifecycleState.paused ||
        state == AppLifecycleState.hidden ||
        state == AppLifecycleState.detached) {
      widget.session.setForeground(false);
    }
    // Native consent/ad overlays can make the app inactive while they still own
    // the platform surface. Only a real background transition invalidates work.
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    widget.session.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) =>
      RewardedScope(session: widget.session, child: widget.child);
}

class RewardedScope extends InheritedWidget {
  const RewardedScope({required this.session, required super.child, super.key});
  final RewardedSessionController session;
  static RewardedSessionController? maybeOf(BuildContext context) =>
      context.dependOnInheritedWidgetOfExactType<RewardedScope>()?.session;
  @override
  bool updateShouldNotify(RewardedScope oldWidget) =>
      !identical(session, oldWidget.session);
}
