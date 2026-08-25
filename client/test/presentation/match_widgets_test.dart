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
    testWidgets('lists only living opponents and hides the tally while blind',
        (tester) async {
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

      expect(find.text('Alpha'), findsNothing);
      expect(find.text('Beta'), findsOneWidget);
      expect(find.text('Delta'), findsNothing);
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

    testWidgets('locks every row once a vote is cast', (tester) async {
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

    testWidgets('reveals the tally after the window closes', (tester) async {
      await tester.pumpWidget(
        _wrap(
          const VoteBoard(
            players: _players,
            localSeat: 0,
            votedSeat: 1,
            tally: {'1': 2, '2': 0},
            eliminatedSeat: 1,
          ),
        ),
      );

      expect(find.text('2'), findsOneWidget);
      expect(find.text('0'), findsOneWidget);
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
  });

  group('KoVoteBudget', () {
    testWidgets('draws one pip per voting and crosses out spent ones',
        (tester) async {
      await tester.pumpWidget(
        _wrap(
          const KoVoteBudget(remaining: 1, total: 3, label: 'Votes left'),
        ),
      );

      expect(find.byIcon(Icons.close), findsNWidgets(2));
    });

    testWidgets('draws nothing until the server reports a budget',
        (tester) async {
      await tester.pumpWidget(
        _wrap(
          const KoVoteBudget(remaining: 0, total: 2, label: 'Votes left'),
        ),
      );

      expect(find.text('Votes left'), findsNothing);
      expect(find.byIcon(Icons.close), findsNothing);
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
  });

  group('SeatSheet', () {
    testWidgets('explains a bot seat instead of fetching a profile',
        (tester) async {
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

      expect(find.textContaining('Backfill bot'), findsOneWidget);
      expect(find.text('Report player'), findsNothing);
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
