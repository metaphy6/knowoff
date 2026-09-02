import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../state/game_session_provider.dart';
import 'discussion_screen.dart';
import 'knowoff_screen.dart';
import 'lobby_screen.dart';
import 'round_screen.dart';
import 'verdict_screen.dart';

/// Routes the user through the live match screens based on the server-driven
/// phase. Replaces the navigation stack once a match is assigned.
class GameShell extends ConsumerWidget {
  const GameShell({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final session = ref.watch(gameSessionProvider);
    final phase = session.dto.phase;

    late final Widget screen;
    switch (phase) {
      case '':
      case 'waiting':
        screen = LobbyScreen(
          code: session.dto.roomCode,
          players: session.dto.players,
        );
        break;
      case 'role_reveal':
      case 'prefetch':
      case 'play':
        screen = const RoundScreen();
        break;
      case 'discussion':
        screen = const DiscussionScreen();
        break;
      case 'knowoff':
      case 'runoff':
      case 'result':
        screen = const KnowoffScreen();
        break;
      case 'verdict':
      case 'finished':
        screen = const VerdictScreen();
        break;
      default:
        screen = LobbyScreen(
          code: session.dto.roomCode,
          players: session.dto.players,
        );
    }
    return screen;
  }
}
