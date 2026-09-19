import 'dart:async';
import 'package:flutter/widgets.dart';
import '../../data/bonus_session.dart';

/// Owns the reward session above navigation; starting it never creates an account.
class BonusLifecycle extends StatefulWidget {
  const BonusLifecycle({required this.session, required this.child, super.key});
  final BonusSessionController session;
  final Widget child;
  @override
  State<BonusLifecycle> createState() => _BonusLifecycleState();
}

class _BonusLifecycleState extends State<BonusLifecycle>
    with WidgetsBindingObserver {
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    widget.session.start();
  }

  @override
  void didUpdateWidget(BonusLifecycle oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (!identical(oldWidget.session, widget.session)) {
      oldWidget.session.dispose();
      oldWidget.session.deliveries.dispose();
      widget.session.start();
    }
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state == AppLifecycleState.resumed) unawaited(widget.session.refresh());
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    widget.session.dispose();
    widget.session.deliveries.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) =>
      BonusScope(session: widget.session, child: widget.child);
}

class BonusScope extends InheritedWidget {
  const BonusScope({required this.session, required super.child, super.key});
  final BonusSessionController session;
  static BonusSessionController? maybeOf(BuildContext context) =>
      context.dependOnInheritedWidgetOfExactType<BonusScope>()?.session;
  @override
  bool updateShouldNotify(BonusScope oldWidget) =>
      !identical(session, oldWidget.session);
}
