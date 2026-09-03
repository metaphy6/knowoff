import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/data/api_client.dart';
import 'package:knowoff_client/data/auth_service.dart';
import 'package:knowoff_client/data/models/game_state_dto.dart';
import 'package:knowoff_client/l10n/app_localizations.dart';
import 'package:knowoff_client/presentation/icons/doodles.dart';
import 'package:knowoff_client/presentation/theme/knowoff_tokens.dart';
import 'package:knowoff_client/presentation/widgets/card_pile.dart';
import 'package:knowoff_client/presentation/widgets/hand_fan.dart';
import 'package:knowoff_client/presentation/widgets/ko_meters.dart';
import 'package:knowoff_client/presentation/widgets/role_card.dart';
import 'package:knowoff_client/presentation/widgets/seat_sheet.dart';
import 'package:knowoff_client/presentation/widgets/seat_tile.dart';
import 'package:knowoff_client/presentation/widgets/vote_board.dart';

class _StubAuthService extends AuthService {
  _StubAuthService() : super(baseUrl: 'http://test');

  @override
  String? get accessToken => 'fake-token';

  @override
  Future<void> ensureSession() async {}

  @override
  Future<void> refresh() async {}
}

class _StubApiClient extends ApiClient {
  _StubApiClient() : super(baseUrl: 'http://test', auth: _StubAuthService());

  @override
  Future<Map<String, dynamic>> getPublicProfile(String accountID) async => {
        'account_id': accountID,
        'nickname': 'Beta',
        'level': 7,
        'matches_played': 42,
        'matches_won_nower': 20,
        'matches_won_donower': 6,
        'correct_votes': 31,
        'overall_points': 900,
      };
}

const _players = <PlayerDto>[
  PlayerDto(seat: 0, name: 'Alpha', connected: true, eliminated: false),
  PlayerDto(seat: 1, name: 'Beta', connected: true, eliminated: false),
  PlayerDto(seat: 2, name: 'Bot_abc_2', connected: true, eliminated: false),
  PlayerDto(seat: 3, name: 'Delta', connected: true, eliminated: true),
];

Widget _wrap(Widget child) {
  return MaterialApp(
    localizationsDelegates: AppLocalizations.localizationsDelegates,
    supportedLocales: AppLocalizations.supportedLocales,
    home: Scaffold(body: SingleChildScrollView(child: child)),
  );
}

void main() {
  group('VoteBoard', () {
    testWidgets('lists every living seat and disables the local vote row',
        (tester) async {
      var votes = 0;
      await tester.pumpWidget(
        _wrap(
          VoteBoard(
            players: _players,
            localSeat: 0,
            votedSeat: -1,
            onVote: (_) => votes++,
          ),
        ),
      );

      expect(find.text('Alpha'), findsWidgets);
      expect(find.text('Beta'), findsOneWidget);
      expect(find.text('Delta'), findsNothing);
      await tester.tap(find.text('Alpha'));
      expect(votes, equals(0));

      await tester.pumpWidget(
        _wrap(
          VoteBoard(
            players: _players,
            localSeat: 0,
            votedSeat: -1,
            ballots: const {'2': 0},
            onVote: (_) => votes++,
          ),
        ),
      );
      await tester.pump(const Duration(milliseconds: 500));
      await tester.tap(find.text('Alpha'));
      expect(votes, equals(0));
      // Blind ballot: no counts anywhere until the window closes.
      expect(find.text('0'), findsNothing);
    });

    testWidgets('labels bot seats', (tester) async {
      await tester.pumpWidget(
        _wrap(
          VoteBoard(
            players: _players,
            localSeat: 0,
            votedSeat: -1,
            onVote: (_) {},
          ),
        ),
      );

      expect(find.text('Bot 2'), findsOneWidget);
      expect(find.text('Bot'), findsOneWidget);
    });

    testWidgets('re-taps the already-chosen row as a no-op', (tester) async {
      var votes = 0;
      await tester.pumpWidget(
        _wrap(
          VoteBoard(
            players: _players,
            localSeat: 0,
            votedSeat: 1,
            onVote: (_) => votes++,
          ),
        ),
      );

      expect(find.text('Your vote'), findsOneWidget);
      await tester.tap(find.text('Beta'));
      await tester.pump();
      expect(votes, equals(0));
    });

    testWidgets('a different row stays tappable so a vote can be changed',
        (tester) async {
      var lastVote = -1;
      await tester.pumpWidget(
        _wrap(
          VoteBoard(
            players: _players,
            localSeat: 0,
            votedSeat: 1,
            onVote: (seat) => lastVote = seat,
          ),
        ),
      );

      await tester.tap(find.text('Bot 2'));
      await tester.pump();
      expect(lastVote, equals(2));
    });

    testWidgets('long-pressing a row opens the seat sheet instead of voting',
        (tester) async {
      var votes = 0;
      await tester.pumpWidget(
        _wrap(
          VoteBoard(
            players: _players,
            localSeat: 0,
            votedSeat: -1,
            onVote: (_) => votes++,
          ),
        ),
      );

      await tester.longPress(find.text('Beta'));
      await tester.pumpAndSettle();

      expect(find.byType(SeatSheet), findsOneWidget);
      expect(find.text('Report player'), findsOneWidget);
      expect(votes, equals(0));
    });

    testWidgets('reveals the tally after the window closes', (tester) async {
      await tester.pumpWidget(
        _wrap(
          const VoteBoard(
            players: _players,
            localSeat: 0,
            votedSeat: 1,
            tally: {'1': 2, '2': 0},
            ballots: {'0': 1, '2': 1},
            eliminatedSeat: 1,
          ),
        ),
      );
      await tester.pump(const Duration(milliseconds: 400));

      expect(find.text('2'), findsOneWidget);
      expect(find.text('0'), findsOneWidget);
      expect(find.byKey(const Key('vote-trail-0-1')), findsOneWidget);
      expect(find.byKey(const Key('vote-trail-2-1')), findsOneWidget);
    });

    testWidgets(
        'announces each voter as a named chip inside the target\'s own row',
        (tester) async {
      await tester.pumpWidget(
        _wrap(
          const VoteBoard(
            players: _players,
            localSeat: 0,
            votedSeat: 1,
            ballots: {'0': 1},
          ),
        ),
      );
      await tester.pump(const Duration(milliseconds: 400));

      expect(find.byKey(const Key('vote-trail-0-1')), findsOneWidget);
      // The chip carries the voter's name, not just a bare avatar, so the
      // table can read "who" without a separate feed.
      expect(find.text('Alpha'), findsWidgets);
    });
  });

  group('HandFan', () {
    const cards = <CardDto>[
      CardDto(id: 'c1', type: 'text', content: 'first'),
      CardDto(id: 'c2', type: 'text', content: 'second'),
    ];

    testWidgets('uses no stock Material chip', (tester) async {
      await tester.pumpWidget(
        _wrap(
          HandFan(
            cards: cards,
            drawPile: const [CardDto(id: 'd1', type: 'text')],
            onSelect: (_) {},
          ),
        ),
      );

      expect(find.byType(ChoiceChip), findsNothing);
      expect(find.byType(Chip), findsNothing);
    });

    testWidgets('marks the selected card and prices the draw pile',
        (tester) async {
      await tester.pumpWidget(
        _wrap(
          HandFan(
            cards: cards,
            drawPile: const [CardDto(id: 'd1', type: 'text')],
            selectedCardId: 'c2',
            onSelect: (_) {},
          ),
        ),
      );

      expect(find.text('Selected'), findsOneWidget);
      expect(find.byType(CardPile), findsOneWidget);
      expect(find.text('Draw pile'), findsOneWidget);
      expect(find.text('-5'), findsOneWidget);
      expect(
        tester.getSize(find.byKey(const ValueKey<String>('hand-card-c2'))),
        tester.getSize(find.byType(RoleCard)),
      );
      final firstCard = tester.getRect(
        find.byKey(const ValueKey<String>('hand-card-c1')),
      );
      final secondCard = tester.getRect(
        find.byKey(const ValueKey<String>('hand-card-c2')),
      );
      expect(secondCard.left - firstCard.right, lessThan(20));
      expect(secondCard.center.dy, closeTo(firstCard.center.dy, 2));
      expect(secondCard.center.dx, greaterThan(firstCard.center.dx));
    });

    testWidgets('reports the tapped card id', (tester) async {
      String? tapped;
      await tester.pumpWidget(
        _wrap(
          HandFan(
            cards: cards,
            drawPile: const [],
            onSelect: (id) => tapped = id,
          ),
        ),
      );

      await tester.tap(find.text('second'));
      await tester.pump();
      expect(tapped, equals('c2'));
    });

    testWidgets('shows a tap-to-play banner once a card is selected on my turn',
        (tester) async {
      await tester.pumpWidget(
        _wrap(
          HandFan(
            cards: cards,
            drawPile: const [],
            selectedCardId: 'c2',
            isMyTurn: true,
            onSelect: (_) {},
            onConfirm: (_) {},
          ),
        ),
      );

      expect(
        find.byKey(const ValueKey<String>('hand-move-banner')),
        findsOneWidget,
      );
      expect(find.text('Tap it again to play'), findsOneWidget);
    });

    testWidgets(
        'shows a play-early banner once a card is selected before my turn',
        (tester) async {
      await tester.pumpWidget(
        _wrap(
          HandFan(
            cards: cards,
            drawPile: const [],
            selectedCardId: 'c2',
            isMyTurn: false,
            onSelect: (_) {},
            onConfirm: (_) {},
          ),
        ),
      );

      expect(
        find.text('Tap it again to play early — it fires the instant your '
            'turn starts'),
        findsOneWidget,
      );
    });

    testWidgets('a locked early move shows the locked-in banner',
        (tester) async {
      await tester.pumpWidget(
        _wrap(
          HandFan(
            cards: cards,
            drawPile: const [],
            selectedCardId: 'c2',
            isMyTurn: false,
            moveLocked: true,
            onSelect: (_) {},
            onConfirm: (_) {},
          ),
        ),
      );

      expect(find.text('Locked in — plays automatically on your turn'),
          findsOneWidget);
    });

    testWidgets('a second tap on the selected card confirms it, not selects',
        (tester) async {
      String? selected;
      String? confirmed;
      await tester.pumpWidget(
        StatefulBuilder(
          builder: (context, setState) => _wrap(
            HandFan(
              cards: cards,
              drawPile: const [],
              selectedCardId: selected,
              onSelect: (id) => setState(() => selected = id),
              onConfirm: (id) => confirmed = id,
            ),
          ),
        ),
      );

      await tester.tap(find.text('second'));
      await tester.pump();
      expect(selected, equals('c2'));
      expect(confirmed, isNull);

      await tester.tap(find.text('second'));
      await tester.pump();
      expect(confirmed, equals('c2'));
    });

    testWidgets('the banner cancel affordance calls onCancelSelection',
        (tester) async {
      var cancelled = false;
      await tester.pumpWidget(
        _wrap(
          HandFan(
            cards: cards,
            drawPile: const [],
            selectedCardId: 'c2',
            onSelect: (_) {},
            onConfirm: (_) {},
            onCancelSelection: () => cancelled = true,
          ),
        ),
      );

      await tester
          .tap(find.byKey(const ValueKey<String>('hand-move-banner-cancel')));
      await tester.pump();
      expect(cancelled, isTrue);
    });

    testWidgets(
        'fits a tuned starting hand (5 cards + specialty) without scrolling',
        (tester) async {
      await tester.pumpWidget(
        _wrap(
          const HandFan(
            cards: <CardDto>[
              CardDto(id: 'c1', type: 'text', content: 'one'),
              CardDto(id: 'c2', type: 'text', content: 'two'),
              CardDto(id: 'c3', type: 'text', content: 'three'),
              CardDto(id: 'c4', type: 'text', content: 'four'),
              CardDto(id: 'c5', type: 'text', content: 'five'),
            ],
            drawPile: <CardDto>[
              CardDto(id: 'd1', type: 'text'),
              CardDto(id: 'd2', type: 'text'),
              CardDto(id: 'd3', type: 'text'),
            ],
            specialty: 'shuffle',
          ),
        ),
      );
      await tester.pump();

      expect(tester.takeException(), isNull);
      expect(find.byType(Scrollbar), findsNothing);

      // The pile rides at the end of the rail, not the start.
      final pileX = tester.getTopLeft(find.byType(CardPile)).dx;
      final lastCardX = tester.getTopLeft(find.text('five')).dx;
      expect(pileX, greaterThan(lastCardX));

      // The role chip now rides above the pile instead of its own row.
      final roleY = tester.getTopLeft(find.byType(RoleCard)).dy;
      final pileY = tester.getTopLeft(find.byType(CardPile)).dy;
      expect(roleY, lessThan(pileY));
    });

    testWidgets(
        'fits the largest reachable hand (base + full draw pile + '
        'specialty) on the narrowest supported phone without scrolling',
        (tester) async {
      tester.view.physicalSize = const Size(320, 700);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);

      await tester.pumpWidget(
        _wrap(
          const HandFan(
            cards: <CardDto>[
              CardDto(id: 'c1', type: 'text', content: 'one'),
              CardDto(id: 'c2', type: 'text', content: 'two'),
              CardDto(id: 'c3', type: 'text', content: 'three'),
              CardDto(id: 'c4', type: 'text', content: 'four'),
              CardDto(id: 'c5', type: 'text', content: 'five'),
              CardDto(id: 'c6', type: 'text', content: 'six'),
              CardDto(id: 'c7', type: 'text', content: 'seven'),
              CardDto(id: 'c8', type: 'text', content: 'eight'),
            ],
            drawPile: <CardDto>[],
            specialty: 'shuffle',
          ),
        ),
      );
      await tester.pump();

      expect(tester.takeException(), isNull);
      expect(find.byType(Scrollbar), findsNothing);
    });

    testWidgets(
        'renders the specialty ability in the same card shape as '
        'regular cards', (tester) async {
      await tester.pumpWidget(
        _wrap(
          const HandFan(
            cards: cards,
            drawPile: [CardDto(id: 'd1', type: 'text')],
            specialty: 'shuffle',
          ),
        ),
      );

      // Same container primitives as _HandCard: no stock Material chip/pill,
      // and the ability's footer is icon-only (no "Specialty" caption) plus
      // its Donower/Nower-only note.
      expect(find.byType(ChoiceChip), findsNothing);
      expect(find.byType(Chip), findsNothing);
      expect(find.text('Shuffle'), findsOneWidget);
      expect(find.text('Donowers only.'), findsOneWidget);
    });

    testWidgets('tapping the specialty card invokes its in-deck action',
        (tester) async {
      String? used;
      await tester.pumpWidget(
        _wrap(
          HandFan(
            cards: cards,
            drawPile: const [],
            specialty: 'reveal',
            onUseSpecialty: (specialty) => used = specialty,
          ),
        ),
      );

      await tester.tap(
        find.byKey(const ValueKey<String>('hand-specialty-reveal')),
      );

      expect(used, 'reveal');
    });
  });

  group('KoVoteBudget', () {
    testWidgets('draws one pip per voting and crosses out spent ones',
        (tester) async {
      await tester.pumpWidget(
        _wrap(
          const KoVoteBudget(remaining: 1, total: 3, label: 'Votes left'),
        ),
      );

      bool isDoodle(Widget w, Doodle doodle) =>
          w is DoodleIcon && w.doodle == doodle;
      expect(
        find.byWidgetPredicate((w) => isDoodle(w, Doodle.check)),
        findsOneWidget,
      );
      expect(
        find.byWidgetPredicate((w) => isDoodle(w, Doodle.cross)),
        findsNWidgets(2),
      );
      expect(
        find.byKey(const ValueKey<String>('vote-budget-critical')),
        findsOneWidget,
      );
    });

    testWidgets('draws nothing until the server reports a budget',
        (tester) async {
      await tester.pumpWidget(
        _wrap(
          const KoVoteBudget(remaining: 0, total: 2, label: 'Votes left'),
        ),
      );

      expect(find.text('Votes left'), findsNothing);
      expect(find.byType(DoodleIcon), findsNothing);
    });

    testWidgets('pulses once when the remaining budget changes',
        (tester) async {
      await tester.pumpWidget(
        _wrap(
          const KoVoteBudget(remaining: 2, total: 3, label: 'Votes left'),
        ),
      );
      await tester.pump();

      await tester.pumpWidget(
        _wrap(
          const KoVoteBudget(remaining: 1, total: 3, label: 'Votes left'),
        ),
      );
      await tester.pump(const Duration(milliseconds: 100));

      final transition = tester.widget<ScaleTransition>(
        find.byType(ScaleTransition).first,
      );
      expect(transition.scale.value, greaterThan(1));
      await tester.pump(const Duration(milliseconds: 300));
      expect(transition.scale.value, closeTo(1, 0.01));
    });
  });

  group('SeatTile', () {
    testWidgets('pairs every state with an icon and a label', (tester) async {
      await tester.pumpWidget(
        _wrap(
          const SeatTile(
            player: PlayerDto(
              seat: 1,
              name: 'Beta',
              connected: false,
              eliminated: false,
              role: 'donower',
            ),
            isTurn: true,
          ),
        ),
      );

      expect(find.text('On the clock'), findsOneWidget);
      expect(find.text('Offline'), findsOneWidget);
      expect(find.text('Donower'), findsOneWidget);
      expect(find.byIcon(Icons.wifi_off), findsOneWidget);
      expect(find.byIcon(Icons.visibility_off), findsOneWidget);
    });
  });

  group('seat identity helpers', () {
    test('recognizes the server-reserved bot nickname prefix', () {
      const bot = PlayerDto(
          seat: 2, name: 'Bot_abc_2', connected: true, eliminated: false);
      const human =
          PlayerDto(seat: 1, name: 'Beta', connected: true, eliminated: false);
      expect(isBotSeat(bot), isTrue);
      expect(isBotSeat(human), isFalse);
      expect(seatDisplayName(bot), equals('Bot 2'));
      expect(seatDisplayName(human), equals('Beta'));
    });

    test('trusts the server bot flag even without a bot nickname', () {
      const bot = PlayerDto(
          seat: 3, name: '', connected: true, eliminated: false, bot: true);
      expect(isBotSeat(bot), isTrue);
      expect(seatDisplayName(bot), equals('Bot 3'));
    });

    test('gives every seat a stable accent from the house palette', () {
      const palette = <Color>[
        KoColors.violet,
        KoColors.lime,
        KoColors.pink,
        KoColors.aqua,
        KoColors.tangerine,
        KoColors.canvasDeep,
      ];
      for (var seat = 0; seat < 6; seat++) {
        expect(palette, contains(seatAccent(seat)));
        expect(seatAccent(seat), equals(seatAccent(seat)));
      }
    });

    test('maps every server avatar preset to a doodle', () {
      for (final preset in <String>[
        'default',
        'nower',
        'donower',
        'detective',
        'party',
      ]) {
        expect(avatarDoodle(preset), isNotNull);
      }
      expect(avatarDoodle(''), isNull);
    });
  });

  group('SeatAvatar', () {
    testWidgets('marks a bot seat with the robot doodle, not an initial',
        (tester) async {
      await tester.pumpWidget(
        _wrap(
          const SeatAvatar(
            player: PlayerDto(
              seat: 2,
              name: '',
              connected: true,
              eliminated: false,
              bot: true,
            ),
          ),
        ),
      );

      expect(find.byType(DoodleIcon), findsOneWidget);
      final icon = tester.widget<DoodleIcon>(find.byType(DoodleIcon));
      expect(icon.doodle, equals(Doodle.robot));
      expect(find.text('B'), findsNothing);
    });

    testWidgets('renders the seat avatar preset when one is set',
        (tester) async {
      await tester.pumpWidget(
        _wrap(
          const SeatAvatar(
            player: PlayerDto(
              seat: 1,
              name: 'Beta',
              connected: true,
              eliminated: false,
              avatar: 'detective',
            ),
          ),
        ),
      );

      final icon = tester.widget<DoodleIcon>(find.byType(DoodleIcon));
      expect(icon.doodle, equals(Doodle.clock));
    });
  });

  group('CardPile', () {
    testWidgets('draws on tap and stamps the penalty', (tester) async {
      var draws = 0;
      await tester.pumpWidget(
        _wrap(CardPile(count: 3, penalty: 5, onDraw: () => draws++)),
      );

      expect(find.text('3'), findsOneWidget);
      expect(find.text('-5'), findsOneWidget);
      await tester.tap(find.byType(CardPile));
      await tester.pump();
      expect(draws, equals(1));
    });

    testWidgets('refuses to draw once the pile is empty', (tester) async {
      var draws = 0;
      await tester.pumpWidget(
        _wrap(CardPile(count: 0, penalty: 5, onDraw: () => draws++)),
      );

      expect(find.text('Pile empty'), findsOneWidget);
      await tester.tap(find.byType(CardPile));
      await tester.pump();
      expect(draws, equals(0));
    });

    testWidgets('shows FREE instead of the penalty while a free draw is banked',
        (tester) async {
      await tester.pumpWidget(
        _wrap(const CardPile(count: 3, penalty: 5, freeDraws: 1)),
      );

      expect(find.text('FREE'), findsOneWidget);
      expect(find.text('-5'), findsNothing);
    });

    testWidgets('a banked free draw counts the covered card on the pile face',
        (tester) async {
      await tester.pumpWidget(
        _wrap(const CardPile(count: 3, penalty: 5, freeDraws: 1)),
      );
      expect(find.text('4'), findsOneWidget);
      expect(find.text('3'), findsNothing);
    });

    testWidgets('an empty pile with a banked free draw shows the FREE ghost',
        (tester) async {
      await tester.pumpWidget(
        _wrap(const CardPile(count: 0, penalty: 5, freeDraws: 1)),
      );

      expect(find.text('1'), findsOneWidget);
      expect(find.text('FREE'), findsOneWidget);
      expect(find.text('-5'), findsNothing);
    });

    testWidgets('the count pops bigger when a free draw lands', (tester) async {
      // The face counts the covered free card: 3 + 1 = 4.
      double countScale() => tester
          .widget<Transform>(
            find
                .ancestor(
                  of: find.text('4'),
                  matching: find.byType(Transform),
                )
                .first,
          )
          .transform
          .getMaxScaleOnAxis();

      await tester.pumpWidget(
        _wrap(const CardPile(count: 3, penalty: 5)),
      );
      // No token yet, so the face shows the bare count.
      expect(find.text('3'), findsOneWidget);
      await tester.pumpWidget(
        _wrap(const CardPile(count: 3, penalty: 5, freeDraws: 1, popTick: 1)),
      );
      final before = countScale();

      // Mid-animation the count is visibly larger; after it settles the
      // scale returns to ~1.
      await tester.pump(const Duration(milliseconds: 150));
      expect(countScale(), greaterThan(before));

      await tester.pump(const Duration(milliseconds: 1200));
      expect(countScale(), closeTo(1.0, 0.01));
    });
  });

  group('SeatSheet', () {
    testWidgets(
        'offers stat placeholders and the flag for a bot seat too '
        '(dev tables run bots-only)', (tester) async {
      await tester.pumpWidget(
        _wrap(
          const SeatSheet(
            player: PlayerDto(
              seat: 2,
              name: '',
              connected: true,
              eliminated: false,
              bot: true,
            ),
          ),
        ),
      );

      expect(find.text('Career'), findsOneWidget);
      expect(find.text('—'), findsWidgets);
      expect(find.text('Report player'), findsOneWidget);
    });

    testWidgets('shows career stats and the flag for a human seat',
        (tester) async {
      await tester.pumpWidget(
        _wrap(
          SeatSheet(
            player: const PlayerDto(
              seat: 1,
              name: 'Beta',
              connected: true,
              eliminated: false,
              accountId: 'acc-1',
            ),
            api: _StubApiClient(),
          ),
        ),
      );
      await tester.pumpAndSettle();

      expect(find.text('Beta'), findsOneWidget);
      expect(find.text('Seat 1'), findsOneWidget);
      expect(find.text('42'), findsOneWidget);
      expect(find.text('Report player'), findsOneWidget);
    });

    testWidgets('never offers to flag your own seat', (tester) async {
      await tester.pumpWidget(
        _wrap(
          SeatSheet(
            player: const PlayerDto(
              seat: 1,
              name: 'Beta',
              connected: true,
              eliminated: false,
              accountId: 'acc-1',
            ),
            isLocal: true,
            api: _StubApiClient(),
          ),
        ),
      );
      await tester.pumpAndSettle();

      expect(find.text('Report player'), findsNothing);
    });
  });
}
