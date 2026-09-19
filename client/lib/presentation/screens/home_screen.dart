import '../../data/rewarded_session.dart';
import '../widgets/rewarded_lifecycle.dart';
import '../widgets/rewarded_offer.dart';
import 'package:flutter/material.dart';
import '../../core/config/app_config.dart';
import '../../data/api_client.dart';
import '../../l10n/app_localizations.dart';
import '../widgets/ko_ui.dart';
import '../widgets/service_notices.dart';
import '../widgets/device_layout.dart';
import '../widgets/service_navigation.dart';
import 'text_play_screen.dart';
import 'account_screens.dart';
import 'community_screens.dart';
import 'safety_screens.dart';

class HomeScreen extends StatefulWidget {
  const HomeScreen({this.api, this.rewarded, super.key});
  final ApiClient? api;
  final RewardedSessionController? rewarded;
  @override
  State<HomeScreen> createState() => _HomeScreenState();
}

class _HomeScreenState extends State<HomeScreen> {
  int _destination = 0;
  final _visited = <int>{0};
  var _pagesKey = GlobalKey();
  int _size = 4;
  bool _busy = false;
  bool _failed = false;
  void _accountChanged() {
    if (!mounted) return;
    setState(() {
      _pagesKey = GlobalKey();
      _visited.clear();
      _visited.add(0);
      _destination = 0;
    });
  }

  Future<void> _accountAccess() async {
    final changed = await koPush<bool>(
      context,
      AccountAccessScreen(auth: widget.api?.authService),
    );
    if (changed == true) _accountChanged();
  }

  Future<void> _play() async {
    if (_busy) return;
    if (AppConfig.instance.clientConfig.protocolVersion != 2) {
      setState(() => _failed = true);
      return;
    }
    setState(() => _busy = true);
    await koPush<void>(
      context,
      TextPlayScreen(
        api: widget.api,
        rewarded: widget.rewarded,
        initialSize: _size,
      ),
    );
    if (mounted) setState(() => _busy = false);
  }

  @override
  Widget build(BuildContext context) => PopScope(
    canPop: _destination == 0,
    onPopInvokedWithResult: (didPop, _) {
      if (!didPop && _destination != 0) {
        FocusManager.instance.primaryFocus?.unfocus();
        setState(() => _destination = 0);
      }
    },
    child: ServiceNavigation(
      selected: _destination,
      onSelected: (index) {
        if (index == _destination) return;
        FocusManager.instance.primaryFocus?.unfocus();
        setState(() {
          _destination = index;
          _visited.add(index);
        });
      },
      child: IndexedStack(
        key: _pagesKey,
        index: _destination,
        children: [
          for (var index = 0; index < 4; index++)
            TickerMode(
              enabled: _destination == index,
              child: ExcludeFocus(
                excluding: _destination != index,
                child: !_visited.contains(index)
                    ? const SizedBox.shrink()
                    : switch (index) {
                        0 => _playPage(context),
                        1 => LeaderboardScreen(api: widget.api),
                        2 => StoreScreen(
                          api: widget.api,
                          rewarded: widget.rewarded,
                        ),
                        _ => ProfileScreen(
                          api: widget.api,
                          onAccountChanged: _accountChanged,
                        ),
                      },
              ),
            ),
        ],
      ),
    ),
  );

  Widget _playPage(BuildContext context) {
    final l = AppLocalizations.of(context);
    final device = KoDeviceLayout.of(context);
    final rewarded =
        widget.rewarded ??
        (widget.api == null ? RewardedScope.maybeOf(context) : null);
    return KoPage(
      title: l.appTitle,
      showBack: false,
      compactHeader: device.isPhone,
      maxWidth: 1280,
      actions: [
        if (rewarded != null) RewardedPrivacyButton(session: rewarded),
        IconButton(
          key: const Key('home-account'),
          tooltip: l.oauthTitle,
          icon: const Icon(Icons.account_circle_outlined),
          onPressed: _accountAccess,
        ),
        IconButton(
          key: const Key('home-safety'),
          tooltip: l.safetyTitle,
          icon: const Icon(Icons.flag_outlined),
          onPressed: () => koPush<void>(context, SafetyScreen(api: widget.api)),
        ),
        IconButton(
          key: const Key('home-notices'),
          tooltip: l.mainMenuNotices,
          constraints: const BoxConstraints(minWidth: 48, minHeight: 48),
          icon: const DoodleIcon(Doodle.hailer, size: 24),
          onPressed: () =>
              koPush<void>(context, NoticeInboxScreen(api: widget.api)),
        ),
        IconButton(
          key: const Key('home-feedback'),
          tooltip: l.mainMenuFeedback,
          constraints: const BoxConstraints(minWidth: 48, minHeight: 48),
          icon: const DoodleIcon(Doodle.quietBubble, size: 24),
          onPressed: () =>
              koPush<void>(context, FeedbackScreen(api: widget.api)),
        ),
      ],
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          ServiceNoticeBanner(api: widget.api),
          if (device.isPhone) ...[
            if (!device.isSmallPhone) ...[
              Text(l.homeHeadline, style: koDisplayStyle(size: 36)),
              const SizedBox(height: 16),
            ],
            _quickPlay(context, compact: true),
            const SizedBox(height: 20),
            _localPlay(context, compact: true),
            const SizedBox(height: 24),
            ExpansionTile(
              tilePadding: EdgeInsets.zero,
              title: Text(l.homeHowTo, style: koDisplayStyle(size: 24)),
              children: [_rules(context, compact: true)],
            ),
          ] else if (device.isTablet) ...[
            Text(l.homeHeadline, style: koDisplayStyle(size: 44)),
            const SizedBox(height: 24),
            LayoutBuilder(
              builder: (context, c) {
                final secondary = Column(
                  children: [
                    _localPlay(context, compact: true),
                    const SizedBox(height: 28),
                    _rules(context, compact: true),
                  ],
                );
                if (c.maxWidth < 620 ||
                    MediaQuery.textScalerOf(context).scale(14) > 20) {
                  return Column(
                    children: [
                      _quickPlay(context),
                      const SizedBox(height: 28),
                      secondary,
                    ],
                  );
                }
                return Row(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Expanded(child: _quickPlay(context)),
                    const SizedBox(width: 28),
                    Expanded(child: secondary),
                  ],
                );
              },
            ),
          ] else ...[
            LayoutBuilder(
              builder: (context, c) {
                final play = Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(l.homeHeadline, style: koDisplayStyle(size: 66)),
                    const SizedBox(height: 14),
                    Text(
                      l.homeIntro,
                      style: Theme.of(context).textTheme.bodyLarge,
                    ),
                    const SizedBox(height: 24),
                    _quickPlay(context),
                  ],
                );
                if (c.maxWidth < 900 ||
                    MediaQuery.textScalerOf(context).scale(14) > 20) {
                  return play;
                }
                return Row(
                  crossAxisAlignment: CrossAxisAlignment.center,
                  children: [
                    Expanded(flex: 6, child: play),
                    const SizedBox(width: 40),
                    Expanded(flex: 5, child: _EvidencePoster(l: l)),
                  ],
                );
              },
            ),
            const SizedBox(height: 36),
            _localPlay(context),
            const SizedBox(height: 36),
            KoHeading(title: l.homeHowTo),
            _rules(context),
          ],
          const SizedBox(height: 28),
          KoPanel(
            color: KoColors.tangerine,
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Text(l.challengeHomeHint, style: koDisplayStyle(size: 25)),
                const SizedBox(height: 16),
                KoButton(
                  key: const Key('home-challenge'),
                  label: l.challengeTitle,
                  color: KoColors.surface,
                  onPressed: () => koPush<void>(
                    context,
                    WeeklyChallengeScreen(api: widget.api),
                  ),
                ),
              ],
            ),
          ),
          const SizedBox(height: 28),
          Text(
            l.homeFooter,
            textAlign: TextAlign.center,
            style: Theme.of(context).textTheme.bodySmall,
          ),
        ],
      ),
    );
  }

  Widget _quickPlay(BuildContext context, {bool compact = false}) {
    final l = AppLocalizations.of(context);
    return KoPanel(
      padding: EdgeInsets.all(compact ? 14 : 20),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Text(
            l.homePlayHeading,
            style: koDisplayStyle(size: compact ? 24 : 28),
          ),
          const SizedBox(height: 12),
          Wrap(
            spacing: 10,
            runSpacing: 10,
            children: [
              for (final n in [4, 6])
                KoButton(
                  key: ValueKey('size-$n'),
                  label: n == 4 ? l.roomSize4 : l.roomSize6,
                  color: _size == n ? KoColors.lime : KoColors.whiteWell,
                  icon: DoodleIcon(
                    _size == n ? Doodle.check : Doodle.person,
                    size: 22,
                  ),
                  onPressed: _busy ? null : () => setState(() => _size = n),
                ),
            ],
          ),
          const SizedBox(height: 14),
          if (_failed) ...[
            Text(
              l.textUpgrade,
              style: const TextStyle(fontWeight: FontWeight.w700),
            ),
            const SizedBox(height: 12),
          ],
          KoButton(
            key: const Key('quick-play'),
            label: _busy ? l.queueInitializing : l.queueTitle,
            icon: const DoodleIcon(Doodle.play, size: 28),
            expand: true,
            onPressed: _busy ? null : _play,
          ),
          const SizedBox(height: 8),
          Text(l.queueHint, style: Theme.of(context).textTheme.bodySmall),
        ],
      ),
    );
  }

  Widget _localPlay(BuildContext context, {bool compact = false}) {
    final l = AppLocalizations.of(context);
    return KoPanel(
      color: KoColors.pink,
      padding: EdgeInsets.all(compact ? 14 : 20),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Text(
            l.homeLocalHeading,
            style: koDisplayStyle(size: compact ? 24 : 28),
          ),
          if (!compact) ...[const SizedBox(height: 8), Text(l.homeLocalHint)],
          const SizedBox(height: 14),
          Wrap(
            spacing: 12,
            runSpacing: 10,
            children: [
              KoButton(
                key: const Key('home-host'),
                label: l.localRoomHostAction,
                color: KoColors.surface,
                onPressed: () => koPush<void>(
                  context,
                  LocalRoomScreen(host: true, api: widget.api),
                ),
              ),
              KoButton(
                key: const Key('home-join'),
                label: l.localRoomJoinAction,
                color: KoColors.surface,
                onPressed: () => koPush<void>(
                  context,
                  LocalRoomScreen(host: false, api: widget.api),
                ),
              ),
            ],
          ),
        ],
      ),
    );
  }

  Widget _rules(BuildContext context, {bool compact = false}) {
    final l = AppLocalizations.of(context);
    return LayoutBuilder(
      builder: (context, c) {
        final items = [
          (Doodle.eye, l.homeRuleOne, KoColors.aqua),
          (Doodle.cards, l.homeRuleTwo, KoColors.violet),
          (Doodle.ballotBox, l.homeRuleThree, KoColors.lime),
        ];
        return Wrap(
          spacing: 18,
          runSpacing: 18,
          children: [
            for (final item in items)
              SizedBox(
                width: !compact && c.maxWidth >= 900
                    ? (c.maxWidth - 36) / 3
                    : c.maxWidth,
                child: Padding(
                  padding: const EdgeInsets.symmetric(vertical: 8),
                  child: Row(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      DoodleIcon(item.$1, size: 28),
                      const SizedBox(width: 12),
                      Expanded(
                        child: Text(
                          item.$2,
                          style: compact ? null : koDisplayStyle(size: 23),
                        ),
                      ),
                    ],
                  ),
                ),
              ),
          ],
        );
      },
    );
  }
}

class _EvidencePoster extends StatelessWidget {
  const _EvidencePoster({required this.l});
  final AppLocalizations l;
  @override
  Widget build(BuildContext context) => ExcludeSemantics(
    child: KoEntrance(
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 12),
        child: Transform.rotate(
          angle: KoTilt.soft,
          child: KoPanel(
            color: KoColors.canvasDeep,
            shadow: KoShadows.lg,
            padding: const EdgeInsets.all(24),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Row(
                  children: [
                    Expanded(
                      child: Text(
                        l.homeExhibit,
                        style: const TextStyle(
                          fontWeight: FontWeight.w900,
                          letterSpacing: 3,
                        ),
                      ),
                    ),
                    const DoodleIcon(Doodle.sparkle, size: 32),
                  ],
                ),
                const SizedBox(height: 24),
                Transform.rotate(
                  angle: -KoTilt.soft,
                  child: KoPanel(
                    color: KoColors.whiteWell,
                    child: Column(
                      children: [
                        const DoodleIcon(Doodle.incognito, size: 128),
                        const SizedBox(height: 20),
                        Text(
                          l.homeExampleClue,
                          textAlign: TextAlign.center,
                          style: koDisplayStyle(size: 30),
                        ),
                      ],
                    ),
                  ),
                ),
                const SizedBox(height: 22),
                Transform.rotate(
                  angle: KoTilt.loud,
                  child: KoPanel(
                    color: KoColors.lime,
                    padding: const EdgeInsets.all(16),
                    child: Text(
                      l.homeExampleAlibi,
                      textAlign: TextAlign.center,
                      style: koDisplayStyle(size: 26),
                    ),
                  ),
                ),
                const SizedBox(height: 22),
                KoTag(
                  label: l.homeExampleVerdict,
                  icon: const DoodleIcon(Doodle.eye, size: 24),
                  color: KoColors.pink,
                ),
              ],
            ),
          ),
        ),
      ),
    ),
  );
}

class LocalRoomScreen extends StatefulWidget {
  const LocalRoomScreen({
    required this.host,
    this.api,
    this.initialCode = '',
    super.key,
  });
  final bool host;
  final ApiClient? api;
  final String initialCode;
  @override
  State<LocalRoomScreen> createState() => _LocalRoomScreenState();
}

class _LocalRoomScreenState extends State<LocalRoomScreen> {
  late final _code = TextEditingController(text: widget.initialCode);
  final _form = GlobalKey<FormState>();
  int _size = 4;
  bool _busy = false;
  bool _failed = false;
  @override
  void dispose() {
    _code.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    if (_busy || (!widget.host && !_form.currentState!.validate())) return;
    if (AppConfig.instance.clientConfig.protocolVersion != 2) {
      setState(() => _failed = true);
      return;
    }
    setState(() => _busy = true);
    await Navigator.of(context).pushReplacement<void, void>(
      PageRouteBuilder(
        pageBuilder: (_, __, ___) => TextPlayScreen(
          api: widget.api,
          local: true,
          initialSize: _size,
          initialCode: widget.host ? null : _code.text.trim().toUpperCase(),
        ),
        transitionDuration: Duration.zero,
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    return KoPage(
      title: l.lobbyTitle,
      eyebrow: l.homeEyebrow,
      maxWidth: 660,
      child: KoPanel(
        key: const Key('local-room-panel'),
        color: KoColors.surface,
        child: Form(
          key: _form,
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              const Align(
                alignment: Alignment.centerLeft,
                child: DoodleIcon(Doodle.pin, size: 64),
              ),
              const SizedBox(height: 22),
              KoHeading(
                title: widget.host ? l.roomHostHeading : l.roomJoinHeading,
                subtitle: l.roomCodeHint,
              ),
              if (widget.host)
                Wrap(
                  spacing: 12,
                  runSpacing: 12,
                  children: [
                    for (final n in [4, 6])
                      KoButton(
                        label: n == 4 ? l.roomSize4 : l.roomSize6,
                        color: _size == n ? KoColors.lime : KoColors.whiteWell,
                        onPressed: _busy
                            ? null
                            : () => setState(() => _size = n),
                      ),
                  ],
                )
              else
                TextFormField(
                  key: const Key('room-code'),
                  controller: _code,
                  enabled: !_busy,
                  autocorrect: false,
                  textCapitalization: TextCapitalization.characters,
                  maxLength: 6,
                  decoration: InputDecoration(labelText: l.lobbyCodeLabel),
                  validator: (v) =>
                      RegExp(r'^[A-Za-z0-9]{6}$').hasMatch(v?.trim() ?? '')
                      ? null
                      : l.roomCodeInvalid,
                  onFieldSubmitted: (_) => _submit(),
                ),
              const SizedBox(height: 24),
              if (_failed) ...[
                Text(l.textUpgrade, semanticsLabel: l.textUpgrade),
                const SizedBox(height: 16),
              ],
              KoButton(
                label: _busy
                    ? l.loadingLabel
                    : widget.host
                    ? l.localRoomHostAction
                    : l.localRoomJoinAction,
                expand: true,
                onPressed: _busy ? null : _submit,
                icon: const DoodleIcon(Doodle.cards, size: 24),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
