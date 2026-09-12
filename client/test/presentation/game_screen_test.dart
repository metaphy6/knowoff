import 'dart:async';
import 'dart:ui' as ui;

import 'package:flutter/material.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/rendering.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/core/config/app_config.dart';
import 'package:knowoff_client/core/config/client_config.dart';
import 'package:knowoff_client/core/text/v2_session.dart';
import 'package:knowoff_client/l10n/app_localizations.dart';
import 'package:knowoff_client/presentation/screens/text_match_screen.dart';
import 'package:knowoff_client/presentation/screens/text_play_screen.dart';
import 'package:knowoff_client/presentation/widgets/game_surfaces.dart';
import 'package:knowoff_client/presentation/widgets/ko_ui.dart';
import 'package:qr_flutter/qr_flutter.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../core/network/text_reducer_test.dart' show fixture;
import '../core/network/text_session_test.dart'
    show FakeTextTransport, hello, admit;
import 'safety_screens_test.dart' show SafetyApi;

Map<String, dynamic> _wire(String name, {int size = 4, int cards = 4}) {
  final wire = fixture('snapshot-$name');
  wire['contract']['original_size'] = size;
  for (var seat = 4; seat < size; seat++) {
    wire['seats'].add({'seat': seat, 'connected': true, 'eliminated': false});
    if (wire['scores'] != null) wire['scores'].add({'seat': seat, 'points': 0});
  }
  if (wire['phase'] == 'play' && wire['private']['hand'].isNotEmpty) {
    wire['private']['hand'] = [
      for (var i = 1; i <= cards; i++)
        {
          'copy_id': 'copy-$i',
          'content': {
            'content_id': 'content-$i',
            'revision': 1,
            'text': 'Retained card $i',
          },
        },
    ];
  }
  return wire;
}

Future<(TextSession, FakeTextTransport)> _pump(
  WidgetTester t,
  Map<String, dynamic> wire, {
  Size size = const Size(430, 932),
  Locale locale = const Locale('en'),
  double scale = 1,
  bool reduced = true,
}) async {
  t.view.physicalSize = size;
  t.view.devicePixelRatio = 1;
  addTearDown(t.view.reset);
  final transport = FakeTextTransport();
  final session = TextSession(
    transport: transport,
    tokenLoader: () async => 'token',
    now: () => DateTime.fromMillisecondsSinceEpoch(0),
  );
  addTearDown(session.dispose);
  await session.connect();
  await t.pump();
  transport.emit('hello', hello());
  await session.control('room_create', {});
  admit(transport);
  transport.emit('snapshot', wire);
  expect(session.snapshot, isNotNull);
  await t.pumpWidget(
    MaterialApp(
      theme: knowoffTheme(),
      locale: locale,
      localizationsDelegates: AppLocalizations.localizationsDelegates,
      supportedLocales: AppLocalizations.supportedLocales,
      builder: (context, child) => MediaQuery(
        data: MediaQuery.of(context).copyWith(
          textScaler: TextScaler.linear(scale),
          disableAnimations: reduced,
        ),
        child: child!,
      ),
      home: Scaffold(
        body: SingleChildScrollView(
          child: AnimatedBuilder(
            animation: session,
            builder: (_, _) => session.snapshot == null
                ? const Text('No private state')
                : TextMatchView(
                    snapshot: session.snapshot!,
                    onAction: (action) => unawaited(session.act(action)),
                    onRematch: () => unawaited(session.control('rematch', {})),
                    serverNowMS: session.serverNowMS,
                    historyPageSize: 8,
                    maxTextBytes: 512,
                    authoredChatAllowed: true,
                    busy: session.reducer!.pendingRequest != null,
                  ),
          ),
        ),
      ),
    ),
  );
  await t.pumpAndSettle();
  return (session, transport);
}

Future<void> _tap(WidgetTester t, Finder target) async {
  await t.ensureVisible(target);
  await t.pump();
  await t.tap(target);
  await t.pumpAndSettle();
}

List<Map<String, dynamic>> _actions(FakeTextTransport t) =>
    t.sent.where((f) => f['type'] == 'action').toList();

// All original screen test families now mount the actual v2 recipient session.
void main() {
  for (final name in [
    'nower',
    'secret_scale',
    'make_room',
    'bad_bargains',
    'top_that',
  ]) {
    for (final players in [4, 6]) {
      for (final viewport in [
        const Size(320, 640),
        const Size(430, 932),
        const Size(834, 1194),
        const Size(1440, 900),
      ]) {
        testWidgets(
          '$name $players seats at $viewport keeps every retained card and board reachable',
          (t) async {
            final (s, transport) = await _pump(
              t,
              _wire(name, size: players),
              size: viewport,
            );
            expect(s.snapshot!.json['seats'], hasLength(players));
            final hand = [
              for (var i = 1; i <= 4; i++) find.byKey(Key('text-hand-copy-$i')),
            ];
            for (final card in hand) {
              await t.ensureVisible(card);
              await t.pumpAndSettle();
              final rect = t.getRect(card);
              expect(rect.width, greaterThan(100));
              expect(rect.left, greaterThanOrEqualTo(0));
              expect(rect.right, lessThanOrEqualTo(viewport.width));
              expect(card.hitTestable(), findsOneWidget);
            }
            expect(find.byKey(const Key('text-public-board')), findsOneWidget);
            expect(find.byType(Image), findsNothing);
            expect(find.byIcon(Icons.play_arrow), findsNothing);
            expect(_actions(transport), isEmpty);
            expect(t.takeException(), isNull);
          },
        );
      }
    }
  }

  for (final locale in [const Locale('ar'), const Locale('en', 'XA')]) {
    for (final name in [
      'nower',
      'secret_scale',
      'make_room',
      'bad_bargains',
      'top_that',
      'knowoff',
      'result-falling',
      'verdict-begun-only',
    ]) {
      testWidgets('$name fits 320px and 2x $locale with reduced motion', (
        t,
      ) async {
        await _pump(
          t,
          _wire(name),
          size: const Size(320, 640),
          locale: locale,
          scale: 2,
        );
        final buttons = find.byType(KoButton);
        for (final element in buttons.evaluate().toList()) {
          final target = find.byWidget(element.widget);
          await t.ensureVisible(target);
          await t.pumpAndSettle();
          final rect = t.getRect(target);
          expect(rect.left, greaterThanOrEqualTo(0));
          expect(rect.right, lessThanOrEqualTo(320));
          expect(rect.height, greaterThanOrEqualTo(48));
        }
        expect(t.takeException(), isNull);
        expect(t.binding.transientCallbackCount, 0);
      });
    }
  }

  for (final width in [360.0, 834.0, 1440.0]) {
    testWidgets(
      'stable copies retain identity and selection through seat update and rotation at $width',
      (t) async {
        final wire = _wire('nower');
        final (s, transport) = await _pump(t, wire, size: Size(width, 1000));
        final card = find.byKey(const Key('text-hand-copy-2'));
        await t.ensureVisible(card);
        await t.pumpAndSettle();
        final unselected = t.getRect(card);
        final element = t.element(card);
        await t.tap(card);
        await t.pumpAndSettle();
        final before = t.getRect(card);
        expect(
          before,
          unselected,
          reason: 'selection alone must not move the hand',
        );
        expect(_actions(transport), isEmpty);
        wire['cursor']['recipient_seq'] = 2;
        wire['seats'][3]['connected'] = false;
        transport.emit('snapshot', wire);
        await t.pumpAndSettle();
        expect(t.element(card), same(element));
        expect(t.getRect(card).size, before.size);
        expect(t.getRect(card).left, greaterThanOrEqualTo(0));
        expect(t.getRect(card).right, lessThanOrEqualTo(width));
        expect(t.takeException(), isNull);
        expect(t.widget<KoButton>(card).color, KoColors.lime);
        t.view.physicalSize = Size(1000, width);
        await t.pumpAndSettle();
        expect(t.widget<KoButton>(card).color, KoColors.lime);
        expect(s.snapshot!.hand.map((c) => c.copyID), [
          'copy-1',
          'copy-2',
          'copy-3',
          'copy-4',
        ]);
        expect(_actions(transport), isEmpty);
      },
    );
    testWidgets(
      'public evidence preserves hand order but invalidates stale board selection at $width',
      (t) async {
        final wire = _wire('nower');
        final (s, transport) = await _pump(t, wire, size: Size(width, 1000));
        await _tap(t, find.byKey(const Key('text-hand-copy-2')));
        final event = fixture('public-history-page')['events'][0];
        event['actor']['seat'] = 3;
        wire['history'] = [event];
        wire['cursor']['evidence_seq'] = 1;
        wire['cursor']['recipient_seq'] = 2;
        wire['board']['revision'] = 1;
        transport.emit('snapshot', wire);
        await t.pumpAndSettle();
        expect(s.snapshot!.hand.map((c) => c.copyID), [
          'copy-1',
          'copy-2',
          'copy-3',
          'copy-4',
        ]);
        expect(find.byKey(const Key('text-confirm')), findsNothing);
        expect(_actions(transport), isEmpty);
        final l = AppLocalizations.of(t.element(find.byType(TextMatchView)));
        expect(find.text('${l.textCount}: 1'), findsOneWidget);
      },
    );
  }

  for (final input in ['touch', 'keyboard', 'semantics']) {
    testWidgets(
      '$input selection and confirm send exactly one current copy and no optimistic spend',
      (t) async {
        final semantics = t.ensureSemantics();

        final (s, transport) = await _pump(t, _wire('nower'));
        Future<void> activate(String key) async {
          final target = find.byKey(Key(key));
          await t.ensureVisible(target);
          await t.pump();
          if (input == 'semantics') {
            final node = t.getSemantics(target);
            node.owner!.performAction(node.id, SemanticsAction.tap);
          } else if (input == 'keyboard') {
            final gesture = find
                .descendant(of: target, matching: find.byType(GestureDetector))
                .first;
            Focus.of(t.element(gesture)).requestFocus();
            await t.pump();
            await t.sendKeyEvent(LogicalKeyboardKey.enter);
          } else {
            await t.tap(target);
          }
          await t.pumpAndSettle();
        }

        await activate('text-hand-copy-2');
        expect(_actions(transport), isEmpty);
        await activate('text-confirm');
        expect(_actions(transport).single['payload']['action'], {
          'kind': 'respond',
          'copy_id': 'copy-2',
        });
        expect(s.snapshot!.hand, hasLength(4));
        expect(find.byKey(const Key('text-confirm')), findsNothing);
        expect(
          t
              .widget<KoButton>(find.byKey(const Key('text-hand-copy-2')))
              .onPressed,
          isNull,
        );
        await t.pumpWidget(const SizedBox.shrink());
        semantics.dispose();
      },
    );
  }

  testWidgets('off-turn local inspection never schedules an automatic move', (
    t,
  ) async {
    final wire = _wire('nower');
    wire['current_seat'] = 1;
    wire['private']['capabilities'] = ['poke', 'chat'];
    final (_, transport) = await _pump(t, wire);
    expect(
      t.widget<KoButton>(find.byKey(const Key('text-hand-copy-1'))).onPressed,
      isNull,
    );
    wire['current_seat'] = 0;
    wire['cursor']['recipient_seq'] = 2;
    wire['private']['capabilities'] = ['respond', 'draw', 'poke', 'chat'];
    transport.emit('snapshot', wire);
    await t.pumpAndSettle();
    expect(_actions(transport), isEmpty);
    expect(find.byKey(const Key('text-confirm')), findsNothing);
  });

  testWidgets(
    'next round clears stale local selection while retained copies stay readable',
    (t) async {
      final wire = _wire('nower');
      final (_, transport) = await _pump(t, wire);
      await _tap(t, find.byKey(const Key('text-hand-copy-1')));
      wire['round'] = 2;
      wire['phase_id'] = 'round-two';
      wire['cursor']['recipient_seq'] = 2;
      transport.emit('snapshot', wire);
      await t.pumpAndSettle();
      expect(find.byKey(const Key('text-confirm')), findsNothing);
      expect(find.text('Retained card 1'), findsOneWidget);
      expect(_actions(transport), isEmpty);
    },
  );

  testWidgets(
    'role seal has no concealed semantics and keeps footprint while held',
    (t) async {
      final semantics = t.ensureSemantics();

      await _pump(t, _wire('nower'));
      final l = AppLocalizations.of(t.element(find.byType(TextMatchView)));
      final seal = find.byType(GameRoleSeal);
      await t.ensureVisible(seal);
      await t.pump();
      final before = t.getRect(seal);
      expect(find.text(l.roleNower), findsNothing);
      final gesture = await t.startGesture(t.getCenter(seal));
      await t.pump();
      expect(find.text(l.roleNower), findsOneWidget);
      expect(t.getRect(seal), before);
      await gesture.up();
      await t.pump();
      expect(find.text(l.roleNower), findsNothing);
      expect(find.bySemanticsLabel(l.roleNower), findsNothing);
      semantics.dispose();
    },
  );

  for (final name in ['donower', 'eliminated']) {
    testWidgets(
      '$name never mounts private Nown or an unauthorized active hand',
      (t) async {
        final handle = t.ensureSemantics();

        final (s, _) = await _pump(t, _wire(name));
        expect(find.byKey(const Key('text-private-nown')), findsNothing);
        expect(find.text('A silent family dinner'), findsNothing);
        expect(
          find.bySemanticsLabel(RegExp('A silent family dinner')),
          findsNothing,
        );
        if (s.snapshot!.eliminated) {
          expect(find.byType(GameRoleSeal), findsNothing);
          expect(find.byKey(const Key('text-hand-copy-1')), findsNothing);
          expect(find.byKey(const Key('text-confirm')), findsNothing);
        }
        handle.dispose();
      },
    );
  }

  testWidgets(
    'background drops private hand, role, focus and pending selection before resume',
    (t) async {
      final handle = t.ensureSemantics();

      final (s, transport) = await _pump(t, _wire('nower'));
      await _tap(t, find.byKey(const Key('text-hand-copy-1')));
      await t.enterText(
        find.byKey(const Key('text-authored-chat')),
        'private draft',
      );
      s.background();
      t.binding.handleAppLifecycleStateChanged(AppLifecycleState.paused);
      await t.pump();
      expect(find.text('Retained card 1'), findsNothing);
      expect(find.text('A silent family dinner'), findsNothing);
      expect(find.bySemanticsLabel(RegExp('Retained card')), findsNothing);
      expect(find.byType(EditableText), findsNothing);
      expect(s.reducer!.pendingRequest, isNull);
      t.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
      await s.resume();
      await t.pump();
      transport.emit('hello', hello());
      final fresh = _wire('nower');
      fresh['cursor']['stream_epoch'] = 'new-epoch';
      transport.emit('snapshot', fresh);
      await t.pumpAndSettle();
      expect(find.text('Retained card 1'), findsOneWidget);
      expect(find.byKey(const Key('text-confirm')), findsNothing);
      expect(_actions(transport), isEmpty);
      handle.dispose();
    },
  );

  testWidgets(
    'runoff uses only server candidates and pending vote prevents duplicate click',
    (t) async {
      final (_, transport) = await _pump(t, _wire('runoff'));
      final l = AppLocalizations.of(t.element(find.byType(TextMatchView)));
      final buttons = find.widgetWithText(KoButton, l.textVote);
      expect(buttons, findsNWidgets(2));
      await _tap(t, buttons.last);
      expect(_actions(transport).single['payload']['action'], {
        'kind': 'vote',
        'target_seat': 2,
      });
      for (final button in t.widgetList<KoButton>(buttons)) {
        expect(button.onPressed, isNull);
      }
    },
  );

  testWidgets(
    'result animation finishes without inventing role until server reveal boundary',
    (t) async {
      final (_, transport) = await _pump(
        t,
        _wire('result-falling'),
        reduced: false,
      );
      final l = AppLocalizations.of(t.element(find.byType(TextMatchView)));
      expect(find.byKey(const Key('text-ballot-role-poster')), findsNothing);
      await t.pump(const Duration(seconds: 8));
      await t.pumpAndSettle();
      expect(find.byKey(const Key('text-ballot-role-poster')), findsNothing);
      expect(find.text(l.roleDonower), findsNothing);
      final next = _wire('result');
      next['cursor']['recipient_seq'] = 2;
      transport.emit('snapshot', next);
      await t.pumpAndSettle();
      expect(find.byKey(const Key('text-ballot-role-poster')), findsOneWidget);
      expect(find.text(l.roleDonower), findsOneWidget);
      final rect = t.getRect(find.byKey(const Key('text-ballot-role-poster')));
      await t.pump(const Duration(seconds: 5));
      expect(t.getRect(find.byKey(const Key('text-ballot-role-poster'))), rect);
    },
  );

  testWidgets(
    'verdict exposes only begun Nowns and explicit same-table rematch',
    (t) async {
      final (s, transport) = await _pump(t, _wire('verdict-begun-only'));
      final l = AppLocalizations.of(t.element(find.byType(TextMatchView)));
      expect(s.snapshot!.json['verdict_nowns'], hasLength(1));
      expect(find.text('A silent family dinner'), findsOneWidget);
      expect(find.byKey(const Key('text-final-scores')), findsOneWidget);
      await _tap(t, find.widgetWithText(KoButton, l.textRematch));
      expect(transport.sent.last['type'], 'rematch');
      expect(transport.sent.last['payload'], isEmpty);
      expect(_actions(transport), isEmpty);
    },
  );

  for (final size in [4, 6]) {
    testWidgets(
      '$size attributed public table cards retain exact seat and chronological history',
      (t) async {
        final wire = _wire('nower', size: size);
        final board = <Map<String, dynamic>>[];
        final history = <Map<String, dynamic>>[];
        for (var seat = 0; seat < size; seat++) {
          final card = {
            'copy_id': 'public-$seat',
            'content': {
              'content_id': 'public-content-$seat',
              'revision': 1,
              'text': 'Public evidence $seat',
            },
          };
          final actor = {'kind': 'seat', 'seat': seat};
          board.add({'card': card, 'actor': actor});
          history.add({
            'event_id': 'public-event-$seat',
            'evidence_seq': seat + 1,
            'round': 1,
            'phase': 'play',
            'phase_id': 'phase-a',
            'actor': actor,
            'kind': 'respond',
            'cards': [card],
            'before_revision': seat,
            'after_revision': seat + 1,
            'reason': 'player',
            'server_time_ms': 1000,
          });
        }
        wire['board']['cards'] = board;
        wire['board']['revision'] = size;
        wire['history'] = history;
        wire['cursor']['evidence_seq'] = size;
        final (session, _) = await _pump(t, wire, size: const Size(360, 800));
        final public = find.byKey(const Key('text-public-board'));
        final l = AppLocalizations.of(t.element(find.byType(TextMatchView)));
        for (var seat = 0; seat < size; seat++) {
          expect(
            find.descendant(
              of: public,
              matching: find.text('Public evidence $seat'),
            ),
            findsOneWidget,
          );
          expect(
            find.descendant(
              of: public,
              matching: find.text(l.seatNumberLabel(seat + 1)),
            ),
            findsOneWidget,
          );
        }
        expect(
          session.snapshot!.json['history']
              .map((e) => e['evidence_seq'])
              .toList(),
          List.generate(size, (i) => i + 1),
        );
        final first = find.descendant(
          of: public,
          matching: find.text('Public evidence 0'),
        );
        final last = find.descendant(
          of: public,
          matching: find.text('Public evidence ${size - 1}'),
        );
        expect(t.getTopLeft(first).dy, lessThan(t.getTopLeft(last).dy));
        expect(t.takeException(), isNull);
      },
    );
  }

  testWidgets(
    'round one is explicit and a zero-round snapshot cannot replace evidence',
    (t) async {
      final (_, transport) = await _pump(t, _wire('nower'));
      final l = AppLocalizations.of(t.element(find.byType(TextMatchView)));
      expect(find.textContaining('${l.textRound} 1'), findsWidgets);
      final invalid = _wire('nower');
      invalid['round'] = 0;
      invalid['cursor']['recipient_seq'] = 2;
      transport.emit('snapshot', invalid);
      await t.pumpAndSettle();
      expect(find.text('No private state'), findsOneWidget);
      expect(find.textContaining('${l.textRound} 0'), findsNothing);
    },
  );

  testWidgets(
    'long authored text remains complete and reachable at enlarged phone scale',
    (t) async {
      final wire = _wire('nower', cards: 1);
      final text = List.filled(18, 'A very convincing alibi').join(' ');
      wire['private']['hand'][0]['content']['text'] = text;
      await _pump(t, wire, size: const Size(360, 800), scale: 2);
      final card = find.byKey(const Key('text-hand-copy-1'));
      await t.ensureVisible(card);
      await t.pumpAndSettle();
      expect(t.widget<KoButton>(card).label, text);
      expect(find.text(text), findsOneWidget);
      expect(t.getRect(card).width, lessThanOrEqualTo(360));
      expect(t.takeException(), isNull);
    },
  );

  for (final field in ['asset_ref', 'signed_url', 'type']) {
    testWidgets(
      'obsolete $field playable metadata never mounts media or private content',
      (t) async {
        final (_, transport) = await _pump(t, _wire('nower'));
        final invalid = _wire('nower');
        invalid['cursor']['recipient_seq'] = 2;
        invalid['private']['hand'][0]['content'][field] =
            'https://invalid.example/private.webp';
        transport.emit('snapshot', invalid);
        await t.pumpAndSettle();
        expect(find.byType(Image), findsNothing);
        expect(find.text('Retained card 1'), findsNothing);
        expect(find.text('No private state'), findsOneWidget);
      },
    );
  }

  testWidgets(
    'text lobby exposes clipboard for the same public join URL as its QR',
    (t) async {
      SharedPreferences.setMockInitialValues({});
      await AppConfig.initialize(ClientConfig.defaultConfig());
      final transport = FakeTextTransport();
      final session = TextSession(
        transport: transport,
        tokenLoader: () async => 'token',
      );
      addTearDown(session.dispose);
      await session.connect();
      await t.pump();
      transport.emit('hello', hello());
      await session.control('room_create', {});
      admit(transport);
      await t.pumpWidget(
        MaterialApp(
          theme: knowoffTheme(),
          localizationsDelegates: AppLocalizations.localizationsDelegates,
          supportedLocales: AppLocalizations.supportedLocales,
          home: TextPlayScreen(session: session, api: SafetyApi()),
        ),
      );
      await t.pumpAndSettle();
      expect(find.byType(QrImageView), findsOneWidget);
      expect(find.byKey(const Key('text-copy-room-link')), findsOneWidget);
    },
  );
  for (final web in [true, false]) {
    testWidgets(
      'lobby QR and clipboard agree on the actual app route web=$web',
      (tester) async {
        String? copied;
        tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
          SystemChannels.platform,
          (call) async {
            if (call.method == 'Clipboard.setData') {
              copied = (call.arguments as Map)['text'] as String;
            }
            return null;
          },
        );
        addTearDown(
          () => tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
            SystemChannels.platform,
            null,
          ),
        );
        await tester.pumpWidget(
          MaterialApp(
            localizationsDelegates: AppLocalizations.localizationsDelegates,
            supportedLocales: AppLocalizations.supportedLocales,
            home: Scaffold(
              body: TextRoomShare(
                code: 'ABC123',
                web: web,
                base: Uri.parse(
                  'https://play.example.test/game/?unrelated=1#/profile',
                ),
              ),
            ),
          ),
        );
        await tester.pumpAndSettle();
        final link = web
            ? 'https://play.example.test/game/#/join/ABC123'
            : 'knowoff://join/ABC123';
        await tester.tap(find.byKey(const Key('text-copy-room-link')));
        expect(copied, link);
        final qrPaint = tester.widget<CustomPaint>(
          find.descendant(
            of: find.byType(QrImageView),
            matching: find.byWidgetPredicate(
              (widget) => widget is CustomPaint && widget.painter is QrPainter,
            ),
          ),
        );
        // Compare the rendered QR modules with a QR encoding the copied URL.
        // QrImageView intentionally keeps its source data private.
        await tester.runAsync(() async {
          final actual = await (qrPaint.painter! as QrPainter).toImage(190);
          final expected = await QrPainter(
            data: copied!,
            version: QrVersions.auto,
            gapless: true,
          ).toImage(190);
          try {
            final actualBytes = await actual.toByteData(
              format: ui.ImageByteFormat.rawRgba,
            );
            final expectedBytes = await expected.toByteData(
              format: ui.ImageByteFormat.rawRgba,
            );
            expect(actualBytes, isNotNull);
            expect(
              listEquals(
                actualBytes!.buffer.asUint8List(),
                expectedBytes!.buffer.asUint8List(),
              ),
              isTrue,
              reason: 'The displayed QR must encode the copied app join URL',
            );
          } finally {
            actual.dispose();
            expected.dispose();
          }
        });
      },
    );
  }
}
