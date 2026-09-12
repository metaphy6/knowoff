import 'dart:async';
import 'package:flutter/widgets.dart';
import '../../data/purchase_controller.dart';

/// Lives above the navigator so leaving Store never cancels verification.
class PurchaseLifecycle extends StatefulWidget {
  const PurchaseLifecycle({
    required this.purchases,
    required this.child,
    super.key,
  });
  final PurchaseController purchases;
  final Widget child;
  @override
  State<PurchaseLifecycle> createState() => _PurchaseLifecycleState();
}

class _PurchaseLifecycleState extends State<PurchaseLifecycle>
    with WidgetsBindingObserver {
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    widget.purchases.start();
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state == AppLifecycleState.resumed) unawaited(widget.purchases.retry());
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    widget.purchases.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => widget.child;
}
