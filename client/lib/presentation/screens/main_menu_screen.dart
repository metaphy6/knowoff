import 'package:flutter/material.dart';
import 'package:knowoff_client/l10n/app_localizations.dart';

import '../../core/config/app_config.dart';
import '../../data/api_client.dart';
import '../theme/knowoff_tokens.dart';
import '../widgets/feedback_dialog.dart';
import '../widgets/ko_button.dart';
import '../widgets/ko_container.dart';
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

  @override
  void initState() {
    super.initState();
    _loadNotices();
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
      appBar: AppBar(title: Text(l10n.appTitle)),
      body: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          children: [
            for (final notice in activeNotices)
              Padding(
                padding: const EdgeInsets.only(bottom: 12),
                child: NoticeBanner(
                  notice: notice,
                  onDismiss: () => setState(
                    () => _dismissed.add(notice['id']?.toString() ?? ''),
                  ),
                ),
              ),
            Expanded(
              child: Center(
                child: KoContainer(
                  padding: const EdgeInsets.all(24),
                  child: Column(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      KoButton(
                        label: l10n.mainMenuPlay,
                        onTap: () {
                          Navigator.of(context).push(
                            MaterialPageRoute<void>(
                              builder: (context) => const QueueScreen(),
                            ),
                          );
                        },
                      ),
                      const SizedBox(height: 16),
                      KoButton(
                        label: l10n.mainMenuLocalRoom,
                        backgroundColor: KoColors.lime,
                        onTap: () => _showLocalRoomChooser(context),
                      ),
                      const SizedBox(height: 16),
                      KoButton(
                        label: l10n.mainMenuProfile,
                        backgroundColor: KoColors.surface,
                        onTap: () {
                          Navigator.of(context).push(
                            MaterialPageRoute<void>(
                              builder: (context) => const ProfileScreen(),
                            ),
                          );
                        },
                      ),
                      const SizedBox(height: 16),
                      KoButton(
                        label: l10n.mainMenuLeaderboard,
                        backgroundColor: KoColors.surface,
                        onTap: () {
                          Navigator.of(context).push(
                            MaterialPageRoute<void>(
                              builder: (context) => const LeaderboardScreen(),
                            ),
                          );
                        },
                      ),
                      const SizedBox(height: 16),
                      KoButton(
                        label: l10n.mainMenuStore,
                        backgroundColor: KoColors.surface,
                        onTap: () {
                          Navigator.of(context).push(
                            MaterialPageRoute<void>(
                              builder: (context) => const StoreScreen(),
                            ),
                          );
                        },
                      ),
                      const SizedBox(height: 16),
                      KoButton(
                        label: l10n.mainMenuNotices,
                        backgroundColor: KoColors.surface,
                        onTap: () {
                          Navigator.of(context).push(
                            MaterialPageRoute<void>(
                              builder: (context) => const NoticeInboxScreen(),
                            ),
                          );
                        },
                      ),
                      const SizedBox(height: 16),
                      KoButton(
                        label: l10n.mainMenuFeedback,
                        backgroundColor: KoColors.surface,
                        onTap: () {
                          showDialog<void>(
                            context: context,
                            builder: (context) => const FeedbackDialog(),
                          );
                        },
                      ),
                    ],
                  ),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}
