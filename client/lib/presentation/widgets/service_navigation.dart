import 'package:flutter/material.dart';
import '../../l10n/app_localizations.dart';
import 'device_layout.dart';
import 'ko_ui.dart';

/// Service destinations keep their state while the window changes shape.
class ServiceNavigation extends StatelessWidget {
  const ServiceNavigation({
    required this.selected,
    required this.onSelected,
    required this.child,
    super.key,
  });
  final int selected;
  final ValueChanged<int> onSelected;
  final Widget child;

  @override
  Widget build(BuildContext context) {
    final device = KoDeviceLayout.of(context);
    final l = AppLocalizations.of(context);
    final destinations = [
      ('play', l.navigationPlay, Doodle.play),
      ('leaderboard', l.mainMenuLeaderboard, Doodle.crown),
      ('store', l.mainMenuStore, Doodle.market),
      ('profile', l.mainMenuProfile, Doodle.person),
    ];
    if (device.isPhone) {
      return Column(children: [
        Expanded(child: child),
        DecoratedBox(
          key: const Key('service-navigation-phone'),
          decoration: const BoxDecoration(
            color: KoColors.surface,
            border: Border(top: BorderSide(width: 3, color: KoColors.ink)),
          ),
          child: NavigationBarTheme(
            data: NavigationBarThemeData(
              backgroundColor: KoColors.surface,
              indicatorColor: KoColors.violet,
              labelTextStyle: WidgetStatePropertyAll(
                  Theme.of(context).textTheme.labelMedium),
              indicatorShape: RoundedRectangleBorder(
                  borderRadius: BorderRadius.circular(12),
                  side: const BorderSide(width: 2, color: KoColors.ink)),
            ),
            child: NavigationBar(
              height: 76,
              animationDuration: Duration.zero,
              selectedIndex: selected,
              onDestinationSelected: onSelected,
              labelBehavior:
                  NavigationDestinationLabelBehavior.onlyShowSelected,
              destinations: [
                for (final destination in destinations)
                  NavigationDestination(
                    key: Key('service-nav-${destination.$1}'),
                    icon: DoodleIcon(destination.$3, size: 26),
                    label: destination.$2,
                    tooltip: destination.$2,
                  ),
              ],
            ),
          ),
        ),
      ]);
    }
    final extended =
        device.isDesktop && MediaQuery.textScalerOf(context).scale(14) <= 20;
    return Row(children: [
      Container(
        key:
            Key('service-navigation-${device.isTablet ? 'tablet' : 'desktop'}'),
        decoration: const BoxDecoration(
          color: KoColors.surface,
          border: Border(right: BorderSide(width: 3, color: KoColors.ink)),
        ),
        child: SafeArea(
          child: SingleChildScrollView(
            child: ConstrainedBox(
              constraints: BoxConstraints(
                  minHeight: MediaQuery.sizeOf(context).height -
                      MediaQuery.paddingOf(context).vertical),
              child: IntrinsicHeight(
                child: NavigationRail(
                  extended: extended,
                  minWidth: 80,
                  minExtendedWidth: 212,
                  backgroundColor: KoColors.surface,
                  indicatorColor: KoColors.violet,
                  indicatorShape: RoundedRectangleBorder(
                      borderRadius: BorderRadius.circular(12),
                      side: const BorderSide(width: 2, color: KoColors.ink)),
                  selectedIndex: selected,
                  onDestinationSelected: onSelected,
                  selectedLabelTextStyle: const TextStyle(
                      color: KoColors.ink, fontWeight: FontWeight.w900),
                  unselectedLabelTextStyle:
                      const TextStyle(color: KoColors.ink),
                  leading: Padding(
                    padding: const EdgeInsets.symmetric(vertical: 24),
                    child: extended
                        ? Text(l.appTitle, style: koDisplayStyle(size: 28))
                        : const DoodleIcon(Doodle.incognito, size: 36),
                  ),
                  destinations: [
                    for (final destination in destinations)
                      NavigationRailDestination(
                        icon: Tooltip(
                          message: destination.$2,
                          child: DoodleIcon(destination.$3,
                              key: Key('service-nav-${destination.$1}'),
                              size: 28),
                        ),
                        label: Text(destination.$2),
                      ),
                  ],
                ),
              ),
            ),
          ),
        ),
      ),
      Expanded(child: child),
    ]);
  }
}
