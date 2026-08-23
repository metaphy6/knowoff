import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/data/models/game_state_dto.dart';
import 'package:knowoff_client/l10n/app_localizations.dart';
import 'package:knowoff_client/presentation/theme/knowoff_tokens.dart';
import 'package:knowoff_client/presentation/widgets/hand_fan.dart';
import 'package:knowoff_client/presentation/widgets/ko_meters.dart';
import 'package:knowoff_client/presentation/widgets/seat_tile.dart';
import 'package:knowoff_client/presentation/widgets/vote_board.dart';

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
      expect(find.textContaining('-5 pts each'), findsOneWidget);
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
  });
}
