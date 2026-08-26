import 'package:flutter/material.dart';
import 'package:knowoff_client/l10n/app_localizations.dart';

import '../../core/config/app_config.dart';
import '../../data/api_client.dart';
import '../icons/doodles.dart';
import '../theme/knowoff_tokens.dart';
import '../theme/knowoff_typography.dart';
import '../theme/ko_breakpoints.dart';
import '../theme/ko_canvas_grid.dart';
import '../widgets/feedback_dialog.dart';
import '../widgets/highlighter.dart';
import '../widgets/ko_body.dart';
import '../widgets/ko_button.dart';
import '../widgets/noin_badge.dart';
import '../widgets/notice_banner.dart';
import 'leaderboard_screen.dart';
import 'lobby_screen.dart';
import 'notice_inbox_screen.dart';
import 'profile_screen.dart';
import 'queue_screen.dart';
import 'store_screen.dart';

/// The main entry screen. Every user-facing string is localized; no hardcoded
/// display text is allowed.
class MainMenuScreen extends StatefulWidget {
  const MainMenuScreen({this.api, super.key});

  final ApiClient? api;

  @override
  State<MainMenuScreen> createState() => _MainMenuScreenState();
}

class _MainMenuScreenState extends State<MainMenuScreen> {
  late final ApiClient _api = widget.api ??
      ApiClient(
        baseUrl: AppConfig.instance.serverUrl,
        auth: AppConfig.instance.authService,
      );
  List<Map<String, dynamic>> _notices = const [];
  final Set<String> _dismissed = {};
  int? _noin;

  @override
  void initState() {
    super.initState();
    _loadNotices();
    _loadWallet();
  }

  Future<void> _loadNotices() async {
    try {
      final notices = await _api.getNotices();
      if (mounted) {
        setState(() {
          _notices = notices.whereType<Map<String, dynamic>>().toList();
        });
      }
    } catch (_) {
      // Notices are a courtesy surface; a failed fetch never blocks the menu.
    }
  }

  Future<void> _loadWallet() async {
    try {
      final wallet = await _api.getWallet();
      if (mounted) {
        setState(() => _noin = (wallet['noin'] as num?)?.toInt());
      }
    } catch (_) {
      // The balance badge is decoration on this screen; failure hides it.
    }
  }

  void _open(Widget screen) {
    Navigator.of(context)
        .push(MaterialPageRoute<void>(builder: (_) => screen))
        .then((_) => _loadWallet());
  }

  Future<void> _hostRoom(BuildContext context, AppLocalizations l10n) async {
    final size = await showDialog<int>(
      context: context,
      builder: (context) => SimpleDialog(
        title: Text(l10n.roomSizeChooserTitle),
        children: [
          SimpleDialogOption(
            onPressed: () => Navigator.of(context).pop(4),
            child: Text(l10n.roomSize4),
          ),
          SimpleDialogOption(
            onPressed: () => Navigator.of(context).pop(6),
            child: Text(l10n.roomSize6),
          ),
        ],
      ),
    );
    if (size == null || !context.mounted) return;
    try {
      final room = await _api.createRoom(size);
      if (!context.mounted) return;
      Navigator.of(context).push(
        MaterialPageRoute<void>(
          builder: (context) => LobbyScreen(
            code: room['code']?.toString() ?? '',
            players: const [],
          ),
        ),
      );
    } catch (_) {
      if (context.mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text(l10n.genericError)),
        );
      }
    }
  }

  void _showLocalRoomChooser(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    showDialog<void>(
      context: context,
      builder: (context) => SimpleDialog(
        title: Text(l10n.mainMenuLocalRoom),
        children: [
          SimpleDialogOption(
            onPressed: () {
              Navigator.of(context).pop();
              _hostRoom(context, l10n);
            },
            child: Text(l10n.localRoomHostAction),
          ),
          SimpleDialogOption(
            onPressed: () {
              Navigator.of(context).pop();
              _showJoinRoomDialog(context);
            },
            child: Text(l10n.localRoomJoinAction),
          ),
        ],
      ),
    );
  }

  void _showJoinRoomDialog(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final controller = TextEditingController();
    showDialog<void>(
      context: context,
      builder: (context) => AlertDialog(
        title: Text(l10n.lobbyCodeLabel),
        content: TextField(
          controller: controller,
          decoration: InputDecoration(hintText: l10n.lobbyCodeLabel),
          textCapitalization: TextCapitalization.characters,
          maxLength: 6,
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(),
            child: Text(l10n.cancel),
          ),
          IconButton(
            icon: const Icon(Icons.check),
            onPressed: () {
              final code = controller.text.trim();
              if (code.isNotEmpty) {
                Navigator.of(context).pop();
                Navigator.of(context).push(
                  MaterialPageRoute<void>(
                    builder: (context) => LobbyScreen(
                      code: code,
                      players: const [],
                    ),
                  ),
                );
              }
            },
          ),
        ],
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final activeNotices = _notices
        .where((n) => !_dismissed.contains(n['id']?.toString() ?? ''))
        .toList();

    return Scaffold(
      backgroundColor: KoColors.canvas,
      body: Stack(
        children: [
          const Positioned.fill(
            child: CustomPaint(painter: KoCanvasGridPainter()),
          ),
          SafeArea(
            child: KoBody(
              children: [
                _Wordmark(
                  title: l10n.appTitle,
                  tagline: l10n.mainMenuTagline,
                  noin: _noin,
                  noinLabel: l10n.storeBalanceLabel,
                  onNoinTap: () => _open(const StoreScreen()),
                ),
                const SizedBox(height: KoSpace.xl),
                for (final notice in activeNotices)
                  Padding(
                    padding: const EdgeInsets.only(bottom: KoSpace.md),
                    child: NoticeBanner(
                      notice: notice,
                      onDismiss: () => setState(
                        () => _dismissed.add(notice['id']?.toString() ?? ''),
                      ),
                    ),
                  ),
                KoButton(
                  label: l10n.mainMenuPlay,
                  subLabel: l10n.mainMenuPlaySub,
                  size: KoButtonSize.large,
                  expand: true,
                  icon: const DoodleIcon(Doodle.staticBurst, size: 34),
                  trailing: const Icon(Icons.arrow_forward, size: 28),
                  shadow: KoShadows.lg,
                  onTap: () => _open(const QueueScreen()),
                ),
                const SizedBox(height: KoSpace.md),
                KoButton(
                  label: l10n.mainMenuLocalRoom,
                  subLabel: l10n.mainMenuLocalRoomSub,
                  size: KoButtonSize.large,
                  expand: true,
                  backgroundColor: KoColors.lime,
                  icon: const DoodleIcon(Doodle.cards, size: 34),
                  trailing: const Icon(Icons.qr_code_2, size: 28),
                  onTap: () => _showLocalRoomChooser(context),
                ),
                const SizedBox(height: KoSpace.xl),
                GridView.count(
                  crossAxisCount:
                      KoLayout.of(context).columns(compact: 2, medium: 3),
                  shrinkWrap: true,
                  primary: false,
                  physics: const NeverScrollableScrollPhysics(),
                  mainAxisSpacing: KoSpace.md,
                  crossAxisSpacing: KoSpace.md,
                  childAspectRatio: 1.45,
                  children: [
                    _MenuTile(
                      label: l10n.mainMenuProfile,
                      doodle: Doodle.eye,
                      accent: KoColors.aqua,
                      tilt: KoTilt.subtle,
                      onTap: () => _open(const ProfileScreen()),
                    ),
                    _MenuTile(
                      label: l10n.mainMenuLeaderboard,
                      doodle: Doodle.crown,
                      accent: KoColors.tangerine,
                      tilt: KoTilt.soft,
                      onTap: () => _open(const LeaderboardScreen()),
                    ),
                    _MenuTile(
                      label: l10n.mainMenuStore,
                      doodle: Doodle.coin,
                      accent: KoColors.pink,
                      tilt: KoTilt.soft,
                      onTap: () => _open(const StoreScreen()),
                    ),
                    _MenuTile(
                      label: l10n.mainMenuNotices,
                      doodle: Doodle.cloud,
                      accent: KoColors.surface,
                      tilt: KoTilt.subtle,
                      badge: activeNotices.length,
                      onTap: () => _open(const NoticeInboxScreen()),
                    ),
                  ],
                ),
                const SizedBox(height: KoSpace.lg),
                Center(
                  child: KoButton(
                    label: l10n.mainMenuFeedback,
                    size: KoButtonSize.small,
                    backgroundColor: KoColors.surface,
                    icon: const DoodleIcon(Doodle.sparkle, size: 18),
                    onTap: () => showDialog<void>(
                      context: context,
                      builder: (context) => const FeedbackDialog(),
                    ),
                  ),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

/// The hero: an oversized stamped wordmark, a highlighter-swept tagline, and
/// the wallet badge. The whole point of the screen is that it looks *built*.
class _Wordmark extends StatefulWidget {
  const _Wordmark({
    required this.title,
    required this.tagline,
    required this.noin,
    required this.noinLabel,
    required this.onNoinTap,
  });

  final String title;
  final String tagline;
  final int? noin;
  final String noinLabel;
  final VoidCallback onNoinTap;

  @override
  State<_Wordmark> createState() => _WordmarkState();
}

class _WordmarkState extends State<_Wordmark>
    with SingleTickerProviderStateMixin {
  late final AnimationController _fallController = AnimationController(
    vsync: this,
    duration: const Duration(milliseconds: 2200),
  )..forward();

  late final Animation<double> _drop = TweenSequence<double>([
    TweenSequenceItem(tween: Tween<double>(begin: 0, end: 0), weight: 18),
    TweenSequenceItem(
      tween: Tween<double>(begin: 0, end: 28)
          .chain(CurveTween(curve: Curves.easeIn)),
      weight: 22,
    ),
    TweenSequenceItem(
      tween: Tween<double>(begin: 28, end: 15)
          .chain(CurveTween(curve: Curves.easeOut)),
      weight: 10,
    ),
    TweenSequenceItem(tween: Tween<double>(begin: 15, end: 15), weight: 20),
    TweenSequenceItem(tween: Tween<double>(begin: 15, end: 15), weight: 30),
  ]).animate(_fallController);

  late final Animation<double> _tilt = TweenSequence<double>([
    TweenSequenceItem(tween: Tween<double>(begin: 0, end: 0), weight: 18),
    TweenSequenceItem(tween: Tween<double>(begin: 0, end: -0.16), weight: 22),
    TweenSequenceItem(tween: Tween<double>(begin: -0.16, end: 0.1), weight: 10),
    TweenSequenceItem(tween: Tween<double>(begin: 0.1, end: 0.06), weight: 20),
    TweenSequenceItem(tween: Tween<double>(begin: 0.06, end: 0.06), weight: 30),
  ]).animate(_fallController);

  @override
  void dispose() {
    _fallController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final title = widget.title.toUpperCase();
    final firstLetter = title.isEmpty ? '' : title.substring(0, 1);
    final remainingLetters = title.length <= 1 ? '' : title.substring(1);

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        if (widget.noin != null)
          Align(
            alignment: Alignment.centerRight,
            child: NoinBadge(
              balance: widget.noin!,
              label: widget.noinLabel,
              compact: true,
              onTap: widget.onNoinTap,
            ),
          ),
        const SizedBox(height: KoSpace.sm),
        Transform.rotate(
          angle: KoTilt.subtle,
          child: Container(
            width: double.infinity,
            padding: const EdgeInsets.symmetric(
              horizontal: KoSpace.lg,
              vertical: KoSpace.md,
            ),
            decoration: BoxDecoration(
              color: KoColors.violet,
              border: Border.all(width: KoBorders.thick, color: KoColors.ink),
              borderRadius: BorderRadius.circular(KoRadii.card),
              boxShadow: const <BoxShadow>[KoShadows.lg],
            ),
            child: FittedBox(
              fit: BoxFit.scaleDown,
              alignment: Alignment.centerLeft,
              child: Row(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  AnimatedBuilder(
                    animation: _fallController,
                    builder: (context, child) => Transform.translate(
                      offset: Offset(7, _drop.value),
                      child: Transform.rotate(angle: _tilt.value, child: child),
                    ),
                    child: Text(
                      firstLetter,
                      key: const ValueKey<String>('wordmark-k'),
                      style: koDisplayStyle(
                        size: 64,
                        letterSpacing: -3,
                        height: 1.0,
                      ),
                    ),
                  ),
                  const SizedBox(width: 5),
                  Text(
                    remainingLetters,
                    key: const ValueKey<String>('wordmark-rest'),
                    style: koDisplayStyle(
                      size: 64,
                      letterSpacing: -3,
                      height: 1.0,
                    ),
                  ),
                ],
              ),
            ),
          ),
        ),
        const SizedBox(height: KoSpace.lg),
        Padding(
          padding: const EdgeInsets.only(left: KoSpace.sm),
          child: Highlighter(
            child: Text(
              widget.tagline,
              style: Theme.of(context).textTheme.titleLarge,
            ),
          ),
        ),
      ],
    );
  }
}

class _MenuTile extends StatefulWidget {
  const _MenuTile({
    required this.label,
    required this.doodle,
    required this.accent,
    required this.tilt,
    required this.onTap,
    this.badge = 0,
  });

  final String label;
  final Doodle doodle;
  final Color accent;
  final double tilt;
  final VoidCallback onTap;
  final int badge;

  @override
  State<_MenuTile> createState() => _MenuTileState();
}

class _MenuTileState extends State<_MenuTile> {
  bool _pressed = false;
  bool _hovered = false;

  @override
  Widget build(BuildContext context) {
    final BoxShadow shadow = _pressed
        ? KoShadows.pressed
        : _hovered
            ? KoShadows.lift
            : KoShadows.md;

    return MouseRegion(
      cursor: SystemMouseCursors.click,
      onEnter: (_) => setState(() => _hovered = true),
      onExit: (_) => setState(() => _hovered = false),
      child: GestureDetector(
        onTapDown: (_) => setState(() => _pressed = true),
        onTapCancel: () => setState(() => _pressed = false),
        onTapUp: (_) {
          setState(() => _pressed = false);
          widget.onTap();
        },
        child: AnimatedContainer(
          duration: KoMotion.press,
          curve: Curves.easeOut,
          transform: Matrix4.translationValues(
            _pressed ? 4 : (_hovered ? -2 : 0),
            _pressed ? 4 : (_hovered ? -2 : 0),
            0,
          ),
          padding: const EdgeInsets.all(KoSpace.md),
          decoration: BoxDecoration(
            color: widget.accent,
            border: Border.all(width: KoBorders.regular, color: KoColors.ink),
            borderRadius: BorderRadius.circular(KoRadii.card),
            boxShadow: <BoxShadow>[shadow],
          ),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            mainAxisAlignment: MainAxisAlignment.spaceBetween,
            children: [
              Row(
                mainAxisAlignment: MainAxisAlignment.spaceBetween,
                children: [
                  Transform.rotate(
                    angle: widget.tilt,
                    child: DoodleIcon(widget.doodle, size: 32),
                  ),
                  if (widget.badge > 0)
                    Container(
                      width: 24,
                      height: 24,
                      alignment: Alignment.center,
                      decoration: BoxDecoration(
                        color: KoColors.pink,
                        border: Border.all(
                          width: KoBorders.thin,
                          color: KoColors.ink,
                        ),
                        borderRadius: BorderRadius.circular(KoRadii.chip),
                      ),
                      child: Text(
                        '${widget.badge}',
                        style: Theme.of(context).textTheme.labelSmall,
                      ),
                    ),
                ],
              ),
              Text(
                widget.label,
                style: Theme.of(context).textTheme.titleLarge,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
              ),
            ],
          ),
        ),
      ),
    );
  }
}
