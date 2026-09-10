import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/data/api_client.dart';
import 'package:knowoff_client/data/auth_service.dart';
import 'package:knowoff_client/l10n/app_localizations.dart';
import 'package:knowoff_client/presentation/screens/community_screens.dart';
import 'package:knowoff_client/presentation/widgets/ko_ui.dart';

class _Api extends ApiClient {
  _Api()
      : super(
            baseUrl: 'https://game.example',
            auth: AuthService(baseUrl: 'https://game.example'));
  String? connectedCode;
  Object? pairingError;
  Object? challengeError;
  Object? submitError;
  bool ownApproved = false;
  int maxBytes = 2000;
  bool empty = false;
  bool submitted = false;
  bool voted = false;
  bool closed = false;
  String? winnerId;
  List<Map<String, dynamic>>? entries;
  int reads = 0;
  Completer<Map<String, dynamic>>? nextLoad;
  String? entryContent;
  String? acceptedTerms;
  String? votedId;
  @override
  Future<void> connectPortal(String code) async {
    if (pairingError != null) throw pairingError!;
    connectedCode = code;
  }

  @override
  Future<Map<String, dynamic>> getActiveChallenge() async {
    reads++;
    if (nextLoad != null) {
      final pending = nextLoad!;
      nextLoad = null;
      return pending.future;
    }
    if (challengeError != null) throw challengeError!;
    if (empty) return {};
    return {
      'topic': {
        'id': 'topic-1',
        'week_start': '2026-09-07',
        'week_end': '2026-09-13',
        'closed_at': closed ? '2026-09-13T23:59:00Z' : null,
        'winner_entry_id': winnerId,
        'nown': {
          'type': 'text',
          'content': 'When your calendar schedules free time'
        }
      },
      'entries': entries ??
          [
            {
              'id': ownApproved ? 'entry-1' : 'entry-2',
              'account_id': 'player-2',
              'nickname': 'Meeting Survivor',
              'entry_type': 'text',
              'content': 'This meeting could have been a nap.',
              'vote_count': 3
            }
          ],
      'own_entry': submitted || ownApproved
          ? {
              'id': 'entry-1',
              'entry_type': 'text',
              'content': entryContent,
              'status': 'submitted'
            }
          : null,
      'voted_entry_id': voted ? 'entry-2' : null,
      'terms': {
        'version': 'v1',
        'title': 'Contribution terms',
        'body':
            'Perpetual, non-exclusive license; commercial use and modification permitted.'
      },
      'intake_remaining': 99,
      'max_text_bytes': maxBytes,
      'can_submit': !submitted && !closed,
      'can_vote': !voted && !closed,
    };
  }

  @override
  Future<Map<String, dynamic>> submitChallengeEntry(
      {required String topicId,
      required String content,
      required String termsVersion}) async {
    if (submitError != null) throw submitError!;
    entryContent = content;
    acceptedTerms = termsVersion;
    submitted = true;
    return {
      'entry': {
        'id': 'entry-1',
        'status': 'submitted',
        'content': content,
        'entry_type': 'text'
      }
    };
  }

  @override
  Future<void> voteChallenge(
      {required String topicId, required String entryId}) async {
    votedId = entryId;
    voted = true;
  }
}

Future<void> _pump(WidgetTester tester, Widget screen,
    {Size size = const Size(1440, 1200),
    double scale = 1,
    Locale locale = const Locale('en')}) async {
  tester.view.physicalSize = size;
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.resetPhysicalSize);
  addTearDown(tester.view.resetDevicePixelRatio);
  await tester.pumpWidget(MaterialApp(
    theme: knowoffTheme(),
    locale: locale,
    localizationsDelegates: AppLocalizations.localizationsDelegates,
    supportedLocales: AppLocalizations.supportedLocales,
    builder: (context, child) => MediaQuery(
        data: MediaQuery.of(context).copyWith(
            textScaler: TextScaler.linear(scale), disableAnimations: true),
        child: child!),
    home: screen,
  ));
  await tester.pumpAndSettle();
}

Future<void> _tap(WidgetTester tester, String key) async {
  final finder = find.byKey(Key(key));
  await tester.ensureVisible(finder);
  await tester.tap(finder);
  await tester.pumpAndSettle();
}

void main() {
  for (final tied in [false, true]) {
    testWidgets('closed challenge shows the server winner when tied=$tied',
        (tester) async {
      final winner = tied ? 'entry-3' : 'entry-2';
      final api = _Api()
        ..closed = true
        ..winnerId = winner
        ..entries = [
          {
            'id': 'entry-2',
            'nickname': 'First player',
            'content': 'First entry',
            'vote_count': 3
          },
          {
            'id': 'entry-3',
            'nickname': 'Second player',
            'content': 'Second entry',
            'vote_count': tied ? 3 : 1
          },
        ];
      await _pump(tester, WeeklyChallengeScreen(api: api));
      expect(find.text('The week is closed. Meet your Week Winner.'),
          findsOneWidget);
      expect(find.byKey(Key('challenge-winner-$winner')), findsOneWidget);
      expect(find.text('Week Winner'), findsOneWidget);
      expect(
          find.byKey(Key('challenge-winner-${tied ? 'entry-2' : 'entry-3'}')),
          findsNothing);
      expect(find.byKey(const Key('challenge-content')), findsNothing);
      expect(
          tester
              .widget<KoButton>(find.byKey(const Key('challenge-vote-entry-2')))
              .onPressed,
          isNull);
    });
  }

  testWidgets('closed challenge explicitly reports no winner', (tester) async {
    final api = _Api()
      ..closed = true
      ..entries = [];
    await _pump(tester, WeeklyChallengeScreen(api: api));
    expect(find.text('This week is closed with no winner.'), findsOneWidget);
    expect(find.text('Week Winner'), findsNothing);
    expect(
        find.text(
            'The evidence is being checked. Approved entries will appear here.'),
        findsNothing);
  });

  testWidgets('forbidden challenge read explains account availability',
      (tester) async {
    final api = _Api()
      ..challengeError = const ApiException(403, code: 'challenge_forbidden');
    await _pump(tester, WeeklyChallengeScreen(api: api));
    expect(
        find.text(
            'This account cannot participate in the challenge right now.'),
        findsOneWidget);
    api.challengeError = null;
    await _tap(tester, 'challenge-refresh');
    expect(find.byKey(const Key('challenge-content')), findsOneWidget);
  });

  testWidgets(
      'forbidden submission preserves draft and explains account availability',
      (tester) async {
    final api = _Api()
      ..submitError = const ApiException(403, code: 'challenge_forbidden');
    await _pump(tester, WeeklyChallengeScreen(api: api));
    await tester.enterText(
        find.byKey(const Key('challenge-content')), 'My careful draft');
    await _tap(tester, 'challenge-terms');
    await _tap(tester, 'challenge-submit');
    await _tap(tester, 'challenge-confirm');
    expect(
        find.text(
            'This account cannot participate in the challenge right now.'),
        findsOneWidget);
    expect(
        tester
            .widget<TextFormField>(find.byKey(const Key('challenge-content')))
            .controller!
            .text,
        'My careful draft');
    expect(api.entryContent, isNull);
  });

  testWidgets('an older refresh cannot undo a successful entry submission',
      (tester) async {
    final api = _Api();
    await _pump(tester, WeeklyChallengeScreen(api: api));
    final stale = await api.getActiveChallenge();
    final pending = Completer<Map<String, dynamic>>();
    api.nextLoad = pending;
    await tester.enterText(
        find.byKey(const Key('challenge-content')), 'My final entry');
    await _tap(tester, 'challenge-terms');
    await _tap(tester, 'challenge-refresh');
    await _tap(tester, 'challenge-submit');
    await _tap(tester, 'challenge-confirm');
    pending.complete(stale);
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('challenge-own-entry')), findsOneWidget);
    expect(find.byKey(const Key('challenge-content')), findsNothing);
  });

  testWidgets('own approved entry cannot receive the player vote',
      (tester) async {
    final api = _Api()..ownApproved = true;
    await _pump(tester, WeeklyChallengeScreen(api: api));
    expect(
        tester
            .widget<KoButton>(find.byKey(const Key('challenge-vote-entry-1')))
            .onPressed,
        isNull);
    expect(api.votedId, isNull);
  });

  testWidgets('server UTF-8 limit blocks oversized accented entry',
      (tester) async {
    final api = _Api()..maxBytes = 3;
    await _pump(tester, WeeklyChallengeScreen(api: api));
    await tester.enterText(find.byKey(const Key('challenge-content')), 'éé');
    await _tap(tester, 'challenge-terms');
    await _tap(tester, 'challenge-submit');
    expect(find.byKey(const Key('challenge-confirm')), findsNothing);
    expect(api.entryContent, isNull);
    expect(
        find.text(
            'Add an entry within the text limit. Accented characters may use more space.'),
        findsOneWidget);
  });

  testWidgets('stale terms preserve the entry and require fresh consent',
      (tester) async {
    final api = _Api()
      ..submitError = const ApiException(409, code: 'terms_outdated');
    await _pump(tester, WeeklyChallengeScreen(api: api));
    await tester.enterText(find.byKey(const Key('challenge-content')),
        'My carefully drafted entry');
    await _tap(tester, 'challenge-terms');
    await _tap(tester, 'challenge-submit');
    await _tap(tester, 'challenge-confirm');
    expect(api.entryContent, isNull);
    expect(
        tester
            .widget<CheckboxListTile>(find.byKey(const Key('challenge-terms')))
            .value,
        isFalse);
    expect(
        tester
            .widget<TextFormField>(find.byKey(const Key('challenge-content')))
            .controller!
            .text,
        'My carefully drafted entry');
    expect(
        find.text(
            'The contribution terms changed. Read the updated terms and accept them before trying again.'),
        findsOneWidget);
  });

  testWidgets('pairing requires a valid code and explicit browser confirmation',
      (tester) async {
    final api = _Api();
    Uri? opened;
    await _pump(
        tester,
        ContributorConnectScreen(
            api: api,
            openBrowser: (uri) async {
              opened = uri;
              return true;
            }));
    await _tap(tester, 'portal-open');
    expect(opened.toString(), 'https://game.example/portal/login');
    await _tap(tester, 'portal-connect');
    expect(api.connectedCode, isNull);
    await tester.enterText(find.byKey(const Key('portal-code')), 'abcd efgh');
    await _tap(tester, 'portal-confirm-browser');
    await _tap(tester, 'portal-connect');
    expect(api.connectedCode, 'ABCD-EFGH');
    expect(find.byKey(const Key('portal-connected')), findsOneWidget);
  });

  testWidgets('expired pairing preserves code and allows retry',
      (tester) async {
    final api = _Api()..pairingError = const ApiException(400);
    await _pump(tester, ContributorConnectScreen(api: api));
    await tester.enterText(find.byKey(const Key('portal-code')), 'ABCD-EFGH');
    await _tap(tester, 'portal-confirm-browser');
    await _tap(tester, 'portal-connect');
    expect(
        find.text(
            'That code has expired or was already used. Open a fresh portal sign-in page and try its new code.'),
        findsOneWidget);
    expect(
        tester
            .widget<TextFormField>(find.byKey(const Key('portal-code')))
            .controller!
            .text,
        'ABCD-EFGH');
    api.pairingError = null;
    await _tap(tester, 'portal-connect');
    expect(api.connectedCode, 'ABCD-EFGH');
  });

  testWidgets('challenge empty state and refresh recover after a failed load',
      (tester) async {
    final api = _Api()..challengeError = StateError('offline');
    await _pump(tester, WeeklyChallengeScreen(api: api));
    expect(find.text('Retry'), findsOneWidget);
    api.challengeError = null;
    api.empty = true;
    await tester.tap(find.text('Retry'));
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('challenge-empty')), findsOneWidget);
  });

  testWidgets('entry requires terms and immutable submission confirmation',
      (tester) async {
    final api = _Api();
    await _pump(tester, WeeklyChallengeScreen(api: api));
    await tester.enterText(find.byKey(const Key('challenge-content')),
        'A calendar invite to cancel a calendar invite.');
    await _tap(tester, 'challenge-submit');
    expect(api.entryContent, isNull);
    await _tap(tester, 'challenge-terms');
    await _tap(tester, 'challenge-submit');
    expect(api.entryContent, isNull);
    await _tap(tester, 'challenge-confirm');
    expect(api.entryContent, 'A calendar invite to cancel a calendar invite.');
    expect(api.acceptedTerms, 'v1');
    expect(find.byKey(const Key('challenge-own-entry')), findsOneWidget);
    expect(find.byKey(const Key('challenge-content')), findsNothing);
  });

  testWidgets('one immutable vote needs confirmation and disables later votes',
      (tester) async {
    final api = _Api();
    await _pump(tester, WeeklyChallengeScreen(api: api));
    await _tap(tester, 'challenge-vote-entry-2');
    expect(api.votedId, isNull);
    await _tap(tester, 'challenge-confirm');
    expect(api.votedId, 'entry-2');
    expect(
        tester
            .widget<KoButton>(find.byKey(const Key('challenge-vote-entry-2')))
            .onPressed,
        isNull);
  });

  testWidgets('live tally refresh preserves a draft and stops after disposal',
      (tester) async {
    final api = _Api();
    await _pump(tester, WeeklyChallengeScreen(api: api));
    await tester.enterText(
        find.byKey(const Key('challenge-content')), 'Unfinished genius');
    await tester.pump(const Duration(seconds: 16));
    await tester.pump();
    expect(api.reads, greaterThan(1));
    expect(
        tester
            .widget<TextFormField>(find.byKey(const Key('challenge-content')))
            .controller!
            .text,
        'Unfinished genius');
    await tester.pumpWidget(const SizedBox());
    final reads = api.reads;
    await tester.pump(const Duration(seconds: 30));
    expect(api.reads, reads);
  });

  for (final size in [
    const Size(320, 568),
    const Size(430, 932),
    const Size(834, 1194),
    const Size(1440, 900)
  ]) {
    testWidgets('community screens fit $size with large pseudo-localized text',
        (tester) async {
      final api = _Api();
      await _pump(tester, WeeklyChallengeScreen(api: api),
          size: size, scale: 2, locale: const Locale('en', 'XA'));
      expect(tester.takeException(), isNull);
      api.closed = true;
      api.winnerId = 'entry-2';
      await _tap(tester, 'challenge-refresh');
      expect(find.byKey(const Key('challenge-winner-entry-2')), findsOneWidget);
      expect(tester.takeException(), isNull);
      await _pump(tester, ContributorConnectScreen(api: api),
          size: size, scale: 2, locale: const Locale('en', 'XA'));
      expect(tester.takeException(), isNull);
    });
  }
}
