import 'dart:async';
import 'dart:ui' show Tristate;
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/data/api_client.dart';
import 'package:knowoff_client/data/auth_service.dart';
import 'package:knowoff_client/l10n/app_localizations.dart';
import 'package:knowoff_client/presentation/screens/safety_screens.dart';
import 'package:knowoff_client/presentation/widgets/ko_ui.dart';

const first = '11111111-1111-4111-8111-111111111111';
const second = '22222222-2222-4222-8222-222222222222';

class SafetyApi extends ApiClient {
  SafetyApi()
    : super(
        baseUrl: 'https://game.example',
        auth: AuthService(baseUrl: 'https://game.example'),
      );
  bool available = true,
      accepted = false,
      failAccept = false,
      failBlock = false;
  String version = 'terms-v2', support = 'https://support.example/help';
  String expectedMatch = 'match-123';
  int reads = 0, accepts = 0, blocks = 0, reports = 0;
  bool failBlockRead = false, failAuth = false;
  @override
  Future<Map<String, dynamic>> getSafetyHelp() async => {
    'support_url': support,
    'privacy_url': '',
  };
  Completer<void>? blockPending;
  Completer<void>? textReportPending;
  final textReports = <Map<String, dynamic>>[];
  @override
  Future<void> createTextReport({
    required String matchID,
    required String contentID,
    required int revision,
    required String reason,
  }) async {
    textReports.add({
      'match_id': matchID,
      'content_id': contentID,
      'revision': revision,
      'reason': reason,
    });
    await textReportPending?.future;
  }

  final roomReads = <String>[];
  Map<int, Completer<Map<String, dynamic>>> roomPending = {};
  @override
  Future<Map<String, dynamic>> getRoomSeatIdentity(
    String roomID,
    int seat,
  ) async {
    roomReads.add('$roomID/$seat');
    return roomPending[seat]?.future ??
        {
          'account_id': seat == 0 ? second : first,
          'nickname': 'Lobby $seat',
          'current_week_winner': seat == 1,
        };
  }

  String? unblocked, reportTarget;
  final pages = <String>[];
  Completer<Map<String, dynamic>>? identity;
  @override
  Future<Map<String, dynamic>> getSafety() async {
    reads++;
    if (failAuth) throw const AuthSessionException('auth.restore_required');
    return {
      'terms': {
        'available': available,
        'version': available ? version : '',
        'body': available ? 'Published terms body' : '',
        'accepted': accepted,
      },
      'support_url': support,
      'privacy_url': '',
    };
  }

  @override
  Future<void> acceptUserTerms(String value) async {
    accepts++;
    expect(value, version);
    if (failAccept) throw const ApiException(409, code: 'terms.changed');
    accepted = true;
  }

  @override
  Future<Map<String, dynamic>> getBlocks({String after = ''}) async {
    if (failBlockRead) throw const ApiException(503);
    pages.add(after);
    return {
      'account_ids': after.isEmpty ? [first] : [second],
      'next_cursor': after.isEmpty ? first : '',
    };
  }

  @override
  Future<void> unblockAccount(String id) async {
    unblocked = id;
  }

  @override
  Future<void> blockAccount(String id) async {
    blocks++;
    expect(id, first);
    await blockPending?.future;
    if (failBlock) throw const ApiException(503);
  }

  @override
  Future<Map<String, dynamic>> getMatchSeatIdentity(
    String matchID,
    int seat,
  ) async {
    expect(matchID, expectedMatch);
    expect(seat, 2);
    return identity?.future ??
        {'account_id': first, 'nickname': 'Public Player'};
  }

  @override
  Future<void> createReport({
    required String reportType,
    String? targetAccountID,
    String? targetMediaID,
    required String reason,
    String? description,
  }) async {
    expect(reportType, 'conduct');
    expect(reason, 'Conduct concern');
    expect(targetMediaID, isNull);
    expect(description, isNull);
    reports++;
    reportTarget = targetAccountID;
  }
}

Future<void> show(
  WidgetTester t,
  Widget child, {
  Locale locale = const Locale('en'),
  double scale = 1,
}) async {
  await t.pumpWidget(
    MaterialApp(
      locale: locale,
      localizationsDelegates: AppLocalizations.localizationsDelegates,
      supportedLocales: AppLocalizations.supportedLocales,
      theme: knowoffTheme(),
      builder: (context, child) => MediaQuery(
        data: MediaQuery.of(context).copyWith(
          textScaler: TextScaler.linear(scale),
          disableAnimations: true,
        ),
        child: child!,
      ),
      home: child,
    ),
  );
  await t.pumpAndSettle();
}

Future<void> tap(WidgetTester t, String key) async {
  await t.ensureVisible(find.byKey(Key(key)));
  await t.tap(find.byKey(Key(key)));
  await t.pumpAndSettle();
}

void main() {
  testWidgets('Arabic large-text reporting confirmation remains readable', (
    t,
  ) async {
    await show(
      t,
      Scaffold(
        body: SingleChildScrollView(
          child: TextContentReport(onReport: (_) async {}),
        ),
      ),
      locale: const Locale('ar'),
      scale: 2,
    );
    await tap(t, 'text-report-open');
    expect(t.takeException(), isNull);
    await tap(t, 'text-report-reason-content.inappropriate');
    await tap(t, 'text-report-send');
    expect(find.byKey(const Key('text-report-success')), findsOneWidget);
    expect(t.takeException(), isNull);
  });
  testWidgets(
    'text report requires confirmation and preserves uncertain retry',
    (t) async {
      final semantics = t.ensureSemantics();
      try {
        final pending = Completer<void>();
        final reasons = <String>[];
        await show(
          t,
          Scaffold(
            body: SingleChildScrollView(
              child: TextContentReport(
                onReport: (reason) async {
                  reasons.add(reason);
                  if (reasons.length == 1) await pending.future;
                },
              ),
            ),
          ),
        );
        await tap(t, 'text-report-open');
        expect(reasons, isEmpty);
        await tap(t, 'text-report-reason-content.inappropriate');
        expect(
          t
              .getSemantics(
                find.byKey(
                  const Key('text-report-reason-content.inappropriate'),
                ),
              )
              .flagsCollection
              .isSelected,
          Tristate.isTrue,
        );
        await t.tap(find.byKey(const Key('text-report-send')));
        await t.pump();
        await t.tap(find.byKey(const Key('text-report-send')));
        await t.pump();
        expect(reasons, ['content.inappropriate']);
        expect(find.byKey(const Key('text-report-success')), findsNothing);
        pending.completeError(
          const ApiException(400, code: 'report.target_unavailable'),
        );
        await t.pumpAndSettle();
        expect(find.byKey(const Key('text-report-error')), findsOneWidget);
        expect(find.textContaining('target_unavailable'), findsNothing);
        expect(
          find.byKey(const Key('text-report-reason-content.incorrect')),
          findsNothing,
        );
        await tap(t, 'text-report-send');
        expect(reasons, ['content.inappropriate', 'content.inappropriate']);
        expect(find.byKey(const Key('text-report-success')), findsOneWidget);
        expect(find.byKey(const Key('text-report-send')), findsNothing);
        expect(t.takeException(), isNull);
      } finally {
        semantics.dispose();
      }
    },
  );

  testWidgets(
    'late report completion cannot restore removed private controls',
    (t) async {
      final pending = Completer<void>();
      await show(
        t,
        Scaffold(body: TextContentReport(onReport: (_) => pending.future)),
      );
      await tap(t, 'text-report-open');
      await tap(t, 'text-report-reason-content.incorrect');
      await t.tap(find.byKey(const Key('text-report-send')));
      await t.pump();
      await show(t, const Scaffold(body: Text('Authoritative sync')));
      pending.complete();
      await t.pumpAndSettle();
      expect(find.byKey(const Key('text-report-success')), findsNothing);
      expect(t.takeException(), isNull);
    },
  );

  testWidgets('locked saved identity retains public configured recovery help', (
    t,
  ) async {
    final api = SafetyApi()..failAuth = true;
    final opened = <Uri>[];
    await show(
      t,
      SafetyScreen(
        api: api,
        openLink: (u) async {
          opened.add(u);
          return true;
        },
      ),
    );
    expect(find.byKey(const Key('safety-error')), findsOneWidget);
    expect(find.byKey(const Key('safety-accept')), findsNothing);
    expect(api.pages, isEmpty);
    await tap(t, 'safety-support');
    expect(opened.single.toString(), api.support);
    expect(api.accepts, 0);
    expect(api.blocks, 0);
  });

  testWidgets('a block-list outage cannot hide published terms and support', (
    t,
  ) async {
    final api = SafetyApi()..failBlockRead = true;
    await show(t, SafetyScreen(api: api));
    expect(find.text('Published terms body'), findsOneWidget);
    expect(find.byKey(const Key('safety-support')), findsOneWidget);
    expect(find.byKey(const Key('safety-error')), findsOneWidget);
  });
  test('malformed acceptance never grants authored chat', () {
    for (final terms in [
      null,
      {},
      {'accepted': true},
      {'available': false, 'accepted': true, 'version': 'v1', 'body': 'body'},
      {'available': true, 'accepted': true, 'version': '', 'body': 'body'},
      {'available': true, 'accepted': true, 'version': 'v1', 'body': ''},
    ]) {
      expect(safetyTermsAccepted({'terms': terms}), isFalse);
    }
  });
  testWidgets('terms require explicit version acceptance and server reread', (
    t,
  ) async {
    final api = SafetyApi();
    await show(t, SafetyScreen(api: api));
    expect(api.accepts, 0);
    expect(find.text('Published terms body'), findsOneWidget);
    await tap(t, 'safety-accept');
    expect(api.accepts, 1);
    expect(api.reads, 2);
    expect(find.byKey(const Key('safety-accepted')), findsOneWidget);
    expect(find.byKey(const Key('safety-accept')), findsNothing);
  });
  testWidgets('unpublished and rejected terms never become accepted', (
    t,
  ) async {
    final api = SafetyApi()..available = false;
    await show(t, SafetyScreen(api: api));
    expect(find.byKey(const Key('safety-accept')), findsNothing);
    expect(api.accepts, 0);
    api.available = true;
    api.failAccept = true;
    await tap(t, 'safety-refresh');
    await tap(t, 'safety-accept');
    expect(find.byKey(const Key('safety-accepted')), findsNothing);
    expect(find.byKey(const Key('safety-error')), findsOneWidget);
    expect(api.accepts, 1);
  });
  testWidgets(
    'private blocks page only on request and unblock after explicit tap',
    (t) async {
      final api = SafetyApi();
      await show(t, SafetyScreen(api: api));
      expect(api.pages, ['']);
      expect(api.unblocked, isNull);
      await tap(t, 'safety-blocks-next');
      expect(api.pages, ['', first]);
      expect(find.text(first), findsNothing);
      expect(find.text(second), findsOneWidget);
      await tap(t, 'safety-unblock-$second');
      expect(api.unblocked, second);
    },
  );
  testWidgets('only configured credential-free HTTPS links can launch', (
    t,
  ) async {
    final api = SafetyApi();
    final opened = <Uri>[];
    await show(
      t,
      SafetyScreen(
        api: api,
        openLink: (u) async {
          opened.add(u);
          return false;
        },
      ),
    );
    await tap(t, 'safety-support');
    expect(opened.single.toString(), api.support);
    expect(find.byKey(const Key('safety-error')), findsOneWidget);
    for (final bad in [
      '',
      'javascript:alert(1)',
      'http://support.example',
      'https://user:pass@support.example',
    ]) {
      api.support = bad;
      await tap(t, 'safety-refresh');
      expect(find.byKey(const Key('safety-support')), findsNothing);
    }
  });
  testWidgets('match report resolves only public identity and needs no terms', (
    t,
  ) async {
    final api = SafetyApi()..available = false;
    await show(t, PlayerSafetyScreen(api: api, matchID: 'match-123', seat: 2));
    expect(find.text('Public Player'), findsOneWidget);
    expect(api.reads, 0);
    expect(api.blocks, 0);
    await tap(t, 'safety-report');
    await t.enterText(
      find.byKey(const Key('report-reason')),
      'Conduct concern',
    );
    await tap(t, 'report-submit');
    expect(api.reports, 1);
    expect(api.reportTarget, first);
    expect(api.accepts, 0);
  });
  testWidgets('failed block preserves target and permits explicit retry', (
    t,
  ) async {
    final api = SafetyApi()..failBlock = true;
    await show(t, PlayerSafetyScreen(api: api, matchID: 'match-123', seat: 2));
    await tap(t, 'safety-block');
    expect(api.blocks, 1);
    expect(find.text('Public Player'), findsOneWidget);
    expect(find.byKey(const Key('safety-error')), findsOneWidget);
  });
  testWidgets('late identity lookup cannot reopen a departed screen', (
    t,
  ) async {
    final api = SafetyApi()..identity = Completer<Map<String, dynamic>>();
    await t.pumpWidget(
      MaterialApp(
        localizationsDelegates: AppLocalizations.localizationsDelegates,
        supportedLocales: AppLocalizations.supportedLocales,
        home: PlayerSafetyScreen(api: api, matchID: 'match-123', seat: 2),
      ),
    );
    await t.pump();
    await t.pumpWidget(const MaterialApp(home: Text('Left')));
    api.identity!.complete({'account_id': first, 'nickname': 'Late Player'});
    await t.pumpAndSettle();
    expect(t.takeException(), isNull);
    expect(find.text('Late Player'), findsNothing);
  });
  testWidgets('Arabic large-text safety remains usable without overflow', (
    t,
  ) async {
    await t.binding.setSurfaceSize(const Size(390, 844));
    addTearDown(() => t.binding.setSurfaceSize(null));
    await show(
      t,
      SafetyScreen(api: SafetyApi()),
      locale: const Locale('ar'),
      scale: 2,
    );
    await t.ensureVisible(find.byKey(const Key('safety-accept')));
    await t.pumpAndSettle();
    expect(t.takeException(), isNull);
  });
}
