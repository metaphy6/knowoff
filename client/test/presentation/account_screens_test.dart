import 'dart:convert';
import 'dart:typed_data';
import 'package:file_selector/file_selector.dart';
import 'package:knowoff_client/presentation/widgets/service_avatar_upload.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/data/api_client.dart';
import 'package:knowoff_client/data/auth_service.dart';
import 'package:knowoff_client/l10n/app_localizations.dart';
import 'package:knowoff_client/presentation/screens/account_screens.dart';
import 'package:knowoff_client/presentation/widgets/ko_ui.dart';
import 'package:knowoff_client/presentation/widgets/service_notices.dart';
import 'package:knowoff_client/presentation/widgets/service_components.dart';

class _Api extends ApiClient {
  _Api()
      : super(
            baseUrl: 'http://test', auth: AuthService(baseUrl: 'http://test'));
  bool failProfile = false;
  int profileCalls = 0;
  int walletCalls = 0;
  String nickname = 'Unconvincing Alibi';
  String avatar = 'default';
  String? pass;
  int? conversion;
  String? feedback;
  String? report;
  int uploads = 0;
  @override
  Future<void> uploadAvatar(List<int> bytes, String filename) async {
    uploads++;
  }

  @override
  Future<Map<String, dynamic>> getPublicProfile(String id) => getProfile();
  @override
  Future<void> createReport(
      {required String reportType,
      String? targetAccountID,
      String? targetMediaID,
      required String reason,
      String? description}) async {
    report = "$reportType:$targetAccountID:$reason";
  }

  @override
  Future<Map<String, dynamic>> getProfile() async {
    profileCalls++;
    if (failProfile) throw StateError('offline');
    return {
      'nickname': nickname,
      'avatar': avatar,
      'level': 3,
      'xp': 450,
      'overall_points': 1800,
      'non_converted_points': 600,
      'matches_played': 12,
      'matches_won_nower': 5,
      'matches_won_donower': 3,
      'correct_votes': 14,
      'votes_cast': 20,
      'donower_survivals': 2,
      'donower_matches': 4,
      'week_winner_titles': 2,
      'weekly_podiums': 3,
      'pokes_sent': 18,
      'contributor_credits': ['Core pack']
    };
  }

  @override
  Future<void> updateNickname(String value) async => nickname = value;
  @override
  Future<void> updateAvatar(String value) async => avatar = value;
  @override
  Future<Map<String, dynamic>> getWallet() async {
    walletCalls++;
    return {'noin': 1234, 'non_converted_points': 600};
  }

  @override
  Future<Map<String, dynamic>> getStoreCatalog() async => {
        'points_to_noin': 100,
        'play_pass_prices': {'day_1': 250, 'day_3': 600, 'day_7': 1200},
        'unlock_prices': {
          'custom_avatar': 1000,
          'poke_style': 400,
          'theme_pack': 1500
        },
        'noin_bundles': [
          {'id': 'noin_500', 'size': 500},
          {'id': 'noin_1200', 'size': 1200},
          {'id': 'noin_3000', 'size': 3000}
        ],
        'premium_yearly_discount_pct': 20
      };
  @override
  Future<void> purchasePlayPass(String type) async => pass = type;
  @override
  Future<Map<String, dynamic>> convertPoints(int points) async {
    conversion = points;
    return {'noin_granted': points ~/ 100};
  }

  @override
  Future<Map<String, dynamic>> getLeaderboard() async => {
        'own': {'rank': 3, 'points': 900},
        'top': [
          {'account_id': 'abc123456789', 'rank': 1, 'points': 1500},
          {'account_id': 'def123456789', 'rank': 1, 'points': 1500}
        ]
      };
  @override
  Future<List<dynamic>> getNotices() async => [
        {
          'type': 'announcement',
          'title': 'Pack arrival',
          'body': 'Fresh suspicious evidence.'
        }
      ];
  @override
  Future<void> createFeedback(
          {required String type,
          required String title,
          required String message,
          Map<String, dynamic>? contextSnapshot}) async =>
      feedback = '$type:$title:$message';
}

Future<void> _pump(WidgetTester tester, Widget screen,
    {double width = 800,
    double height = 1000,
    Locale locale = const Locale('en'),
    double textScale = 1}) async {
  tester.view.reset();
  tester.view.physicalSize = Size(width, height);
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.reset);
  await tester.pumpWidget(MaterialApp(
      locale: locale,
      builder: (context, child) => MediaQuery(
          data: MediaQuery.of(context)
              .copyWith(textScaler: TextScaler.linear(textScale)),
          child: child!),
      theme: knowoffTheme(),
      localizationsDelegates: AppLocalizations.localizationsDelegates,
      supportedLocales: AppLocalizations.supportedLocales,
      home: screen));
  await tester.pumpAndSettle();
}

class _OversizeImage extends XFile {
  _OversizeImage() : super('large.png');
  bool read = false;
  @override
  Future<int> length() async => 2 * 1024 * 1024 + 1;
  @override
  Future<Uint8List> readAsBytes() async {
    read = true;
    return Uint8List(0);
  }
}

void main() {
  testWidgets('refreshing a cached profile keeps an unfinished nickname',
      (tester) async {
    final api = _Api();
    await _pump(tester, ProfileScreen(api: api));
    final field = find.byKey(const Key('profile-nickname'));
    await tester.enterText(field, 'Unfinished Alibi');
    await tester.tap(find.byKey(const Key('profile-refresh')));
    await tester.pumpAndSettle();
    expect(api.profileCalls, 2);
    expect(tester.widget<TextFormField>(field).controller!.text,
        'Unfinished Alibi');
    api.failProfile = true;
    await tester.tap(find.byKey(const Key('profile-refresh')));
    await tester.pumpAndSettle();
    api.failProfile = false;
    await tester.tap(find.text('Retry'));
    await tester.pumpAndSettle();
    expect(api.profileCalls, 4);
    expect(tester.widget<TextFormField>(field).controller!.text,
        'Unfinished Alibi');
    expect(tester.takeException(), isNull);
  });

  for (final size in [
    const Size(320, 640),
    const Size(430, 932),
    const Size(834, 1194),
    const Size(1440, 900),
  ]) {
    testWidgets('profile tasks use a deliberate workspace at $size',
        (tester) async {
      final api = _Api();
      await _pump(tester, ProfileScreen(api: api),
          width: size.width, height: size.height);
      final editor = tester.getRect(find.byKey(const Key('profile-editor')));
      final statistics =
          tester.getRect(find.byKey(const Key('profile-statistics')));
      if (size.width >= 600) {
        expect(statistics.left, greaterThan(editor.right));
        expect(statistics.top, editor.top);
      } else {
        expect(statistics.top, greaterThan(editor.bottom));
      }
      final field = find.byKey(const Key('profile-nickname'));
      await tester.ensureVisible(field);
      await tester.enterText(field, 'Device Alibi');
      final save = find.byKey(const Key('profile-save'));
      await tester.ensureVisible(save);
      await tester.tap(save);
      await tester.pumpAndSettle();
      expect(api.nickname, 'Device Alibi');
      expect(tester.takeException(), isNull);
    });

    testWidgets('store keeps wallet context beside catalog at $size',
        (tester) async {
      final api = _Api();
      await _pump(tester, StoreScreen(api: api),
          width: size.width, height: size.height);
      final wallet = tester.getRect(find.byKey(const Key('store-wallet')));
      final catalog = tester.getRect(find.byKey(const Key('store-catalog')));
      if (size.width >= 600) {
        expect(catalog.left, greaterThan(wallet.right));
        expect(catalog.top, wallet.top);
      } else {
        expect(catalog.top, greaterThan(wallet.bottom));
      }
      final buy = find.byKey(const Key('pass-day_1'));
      await tester.ensureVisible(buy);
      await tester.tap(buy);
      await tester.pumpAndSettle();
      expect(api.pass, 'day_1');
      expect(api.walletCalls, 2);
      expect(tester.takeException(), isNull);
    });
  }

  testWidgets('profile retry recovers and nickname save updates API',
      (tester) async {
    final api = _Api()..failProfile = true;
    await _pump(tester, ProfileScreen(api: api));
    api.failProfile = false;
    await tester.tap(find.text('Retry'));
    await tester.pumpAndSettle();
    expect(api.profileCalls, 2);
    await tester.enterText(
        find.byKey(const Key('profile-nickname')), '  New Alibi  ');
    await tester.tap(find.byKey(const Key('profile-save')));
    await tester.pumpAndSettle();
    expect(api.nickname, 'New Alibi');
  });

  testWidgets('store pass refreshes wallet and validates conversion',
      (tester) async {
    final api = _Api();
    await _pump(tester, StoreScreen(api: api));
    await tester.ensureVisible(find.byKey(const Key('pass-day_1')));
    await tester.tap(find.byKey(const Key('pass-day_1')));
    await tester.pumpAndSettle();
    expect(api.pass, 'day_1');
    expect(api.walletCalls, 2);
    await tester.ensureVisible(find.byKey(const Key('convert-open')));
    await tester.tap(find.byKey(const Key('convert-open')));
    await tester.pumpAndSettle();
    await tester.enterText(find.byKey(const Key('convert-input')), '150');
    await tester.tap(find.byKey(const Key('convert-submit')));
    await tester.pumpAndSettle();
    expect(api.conversion, isNull);
    expect(find.text('Must be a multiple of 100.'), findsOneWidget);
    await tester.enterText(find.byKey(const Key('convert-input')), '300');
    await tester.tap(find.byKey(const Key('convert-submit')));
    await tester.pumpAndSettle();
    expect(api.conversion, 300);
    expect(find.text('You received 3 Noin'), findsOneWidget);
    await tester.binding.handlePopRoute();
    await tester.pumpAndSettle();
    expect(api.walletCalls, 3);
    expect(find.text('500 Noin'), findsOneWidget);
  });

  testWidgets('feedback refuses empty fields and sends trimmed values',
      (tester) async {
    final api = _Api();
    await _pump(tester, FeedbackScreen(api: api));
    await tester.tap(find.byKey(const Key('feedback-submit')));
    await tester.pumpAndSettle();
    expect(api.feedback, isNull);
    await tester.enterText(find.byKey(const Key('feedback-title')), '  Idea  ');
    await tester.enterText(
        find.byKey(const Key('feedback-message')), '  Better bluff  ');
    await tester.tap(find.byKey(const Key('feedback-submit')));
    await tester.pumpAndSettle();
    expect(api.feedback, 'bug:Idea:Better bluff');
  });

  testWidgets('preset avatar update uses server preset and refreshes profile',
      (tester) async {
    final api = _Api();
    await _pump(tester, ProfileScreen(api: api));
    await tester.ensureVisible(find.byKey(const Key('avatar-detective')));
    await tester.tap(find.byKey(const Key('avatar-detective')));
    await tester.pumpAndSettle();
    expect(api.avatar, 'detective');
    expect(
        tester
            .widget<DoodleIcon>(find.byKey(const Key('profile-avatar')))
            .doodle,
        Doodle.incognito);
    expect(api.profileCalls, 2);
  });

  testWidgets(
      'notices localize with English fallback and maintenance countdown',
      (tester) async {
    await _pump(
        tester,
        Scaffold(
            body: ServiceNoticeTile(notice: {
          'type': 'maintenance',
          'title': const {'en': 'Maintenance window', 'tr': 'Bakım'},
          'body': const {'en': 'Back shortly'},
          'maintenance_start': DateTime.now()
              .add(const Duration(hours: 2))
              .toUtc()
              .toIso8601String(),
        })));
    expect(find.text('Maintenance window'), findsOneWidget);
    expect(find.text('Back shortly'), findsOneWidget);
    expect(find.textContaining('Maintenance in'), findsOneWidget);
    expect(
        serviceNoticeText({'en': 'Fallback'}, const Locale('fr')), 'Fallback');
    await tester.pumpWidget(const SizedBox.shrink());
  });

  testWidgets('public profiles hide owner balance and editing controls',
      (tester) async {
    final api = _Api();
    await _pump(tester, ProfileScreen(api: api, accountId: 'other-account'));
    expect(find.text('Non-Converted Points'), findsNothing);
    expect(find.byKey(const Key('profile-nickname')), findsNothing);
    expect(find.text('Report player'), findsOneWidget);
  });

  testWidgets('report requires reason and sends chosen account target',
      (tester) async {
    final api = _Api();
    await _pump(
        tester,
        Builder(
            builder: (context) => Scaffold(
                body: KoButton(
                    label: 'Report',
                    onPressed: () => showDialog<bool>(
                        context: context,
                        builder: (_) => ServiceReportDialog(
                            api: api,
                            reportType: 'conduct',
                            accountId: 'other-account'))))));
    await tester.tap(find.text('Report'));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('report-submit')));
    await tester.pumpAndSettle();
    expect(api.report, isNull);
    await tester.enterText(
        find.byKey(const Key('report-reason')), '  Harassment  ');
    await tester.tap(find.byKey(const Key('report-submit')));
    await tester.pumpAndSettle();
    expect(api.report, 'conduct:other-account:Harassment');
  });

  testWidgets(
      'avatar picker cancellation and oversize never upload or read oversized bytes',
      (tester) async {
    final api = _Api();
    await _pump(
        tester,
        Scaffold(
            body: ServiceAvatarUpload(api: api, pickImage: () async => null)));
    await tester.tap(find.byKey(const Key('avatar-pick')));
    await tester.pumpAndSettle();
    expect(api.uploads, 0);
    expect(find.byKey(const Key('avatar-upload')), findsNothing);
    final large = _OversizeImage();
    await tester.pumpWidget(const SizedBox.shrink());
    await _pump(
        tester,
        Scaffold(
            body: ServiceAvatarUpload(api: api, pickImage: () async => large)));
    await tester.tap(find.byKey(const Key('avatar-pick')));
    await tester.pumpAndSettle();
    expect(large.read, isFalse);
    expect(api.uploads, 0);
    expect(find.text('Choose a JPEG or PNG within the size limits.'),
        findsOneWidget);
  });

  testWidgets('avatar selection awaits explicit upload and reports success',
      (tester) async {
    final api = _Api();
    final image = XFile.fromData(
        base64Decode(
            'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aZ1sAAAAASUVORK5CYII='),
        name: 'avatar.png',
        path: 'avatar.png',
        mimeType: 'image/png');
    await _pump(
        tester,
        Scaffold(
            body: ServiceAvatarUpload(api: api, pickImage: () async => image)));
    await tester.tap(find.byKey(const Key('avatar-pick')));
    await tester.runAsync(() async {
      await Future<void>.delayed(const Duration(milliseconds: 100));
    });
    await tester.pumpAndSettle();
    expect(api.uploads, 0);
    expect(find.byKey(const Key('avatar-upload')), findsOneWidget);
    await tester.tap(find.byKey(const Key('avatar-upload')));
    await tester.pumpAndSettle();
    expect(api.uploads, 1);
    expect(find.text('Image uploaded for review.'), findsOneWidget);
  });

  for (final size in [const Size(360, 800), const Size(1440, 900)]) {
    for (final locale in [const Locale('en'), const Locale('en', 'XA')]) {
      testWidgets('account acceptance sweep $size $locale at 2x text',
          (tester) async {
        final api = _Api();
        for (final screen in [
          ProfileScreen(api: api),
          LeaderboardScreen(api: api),
          StoreScreen(api: api),
          NoticeInboxScreen(api: api),
          FeedbackScreen(api: api)
        ]) {
          await _pump(tester, screen,
              width: size.width,
              height: size.height,
              locale: locale,
              textScale: 2);
          expect(tester.takeException(), isNull,
              reason: '${screen.runtimeType} initial layout');
          final scroll = find.byType(SingleChildScrollView).first;
          await tester.drag(scroll, const Offset(0, -1800));
          await tester.pumpAndSettle();
          expect(tester.takeException(), isNull,
              reason: '${screen.runtimeType} scrolled layout');
        }
      });
    }
  }

  for (final width in [320.0, 1440.0]) {
    testWidgets('account pages have no overflow at width $width',
        (tester) async {
      final api = _Api();
      for (final screen in [
        ProfileScreen(api: api),
        LeaderboardScreen(api: api),
        StoreScreen(api: api),
        NoticeInboxScreen(api: api),
        FeedbackScreen(api: api)
      ]) {
        await _pump(tester, screen, width: width);
        expect(tester.takeException(), isNull);
      }
    });
  }
}
