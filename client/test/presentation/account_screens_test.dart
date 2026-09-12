import 'dart:convert';
import 'dart:async';
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

class _StoreFixtureAuth extends AuthService {
  _StoreFixtureAuth() : super(baseUrl: 'http://test');
  @override
  String get accountId => 'fixture-owner';
  @override
  Future<void> ensureSession() async {}
}

class _Api extends ApiClient {
  _Api() : super(baseUrl: 'http://test', auth: _StoreFixtureAuth());
  bool failProfile = false, currentWeekWinner = false;
  int profileCalls = 0;
  int walletCalls = 0;
  String nickname = 'Unconvincing Alibi';
  String avatar = 'default';
  String? pass;
  int? conversion;
  String? feedback;
  String? report;
  int uploads = 0;
  Object? avatarUploadError;
  Completer<void>? pendingAvatar;
  Uint8List? avatarBytes;
  int avatarReads = 0;
  int avatarCatalogReads = 0;
  @override
  Future<Uint8List?> getAvatarImage(String accountID) async {
    avatarReads++;
    return avatarBytes;
  }

  bool avatarOwned = true;
  bool avatarAvailable = true;
  int avatarPurchases = 0;
  @override
  Future<void> purchaseUnlock(String type, {String value = ''}) async {
    avatarPurchases++;
    avatarOwned = true;
  }

  @override
  Future<void> uploadAvatar(List<int> bytes, String filename) async {
    uploads++;
    if (pendingAvatar case final pending?) await pending.future;
    if (avatarUploadError case final error?) throw error;
  }

  @override
  Future<Map<String, dynamic>> getPublicProfile(String id) => getProfile();
  @override
  Future<void> createReport({
    required String reportType,
    String? targetAccountID,
    String? targetMediaID,
    required String reason,
    String? description,
  }) async {
    report = "$reportType:$targetAccountID:$reason";
  }

  @override
  Future<Map<String, dynamic>> getProfile() async {
    profileCalls++;
    if (failProfile) throw StateError('offline');
    return {
      'account_id': 'fixture-owner',
      'avatar_revision': profileCalls,
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
      'current_week_winner': currentWeekWinner,
      'weekly_podiums': 3,
      'pokes_sent': 18,
      'contributor_credits': ['Core pack'],
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
  Future<Map<String, dynamic>> getStoreCatalog() async {
    avatarCatalogReads++;
    return {
      'custom_avatar_available': avatarAvailable,
      'custom_avatar_owned': avatarOwned,
      'points_to_noin': 100,
      'play_pass_prices': {'day_1': 250, 'day_3': 600, 'day_7': 1200},
      'unlock_prices': {
        'custom_avatar': 1000,
        'poke_style': 400,
        'theme_pack': 1500,
      },
      'noin_bundles': [
        {'id': 'noin_500', 'size': 500},
        {'id': 'noin_1200', 'size': 1200},
        {'id': 'noin_3000', 'size': 3000},
      ],
      'premium_yearly_discount_pct': 20,
    };
  }

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
      {'account_id': 'def123456789', 'rank': 1, 'points': 1500},
    ],
  };
  @override
  Future<List<dynamic>> getNotices() async => [
    {
      'type': 'announcement',
      'title': 'Pack arrival',
      'body': 'Fresh suspicious evidence.',
    },
  ];
  @override
  Future<void> createFeedback({
    required String type,
    required String title,
    required String message,
    Map<String, dynamic>? contextSnapshot,
  }) async => feedback = '$type:$title:$message';
}

Future<void> _pump(
  WidgetTester tester,
  Widget screen, {
  double width = 800,
  double height = 1000,
  Locale locale = const Locale('en'),
  double textScale = 1,
}) async {
  tester.view.reset();
  tester.view.physicalSize = Size(width, height);
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.reset);
  await tester.pumpWidget(
    MaterialApp(
      locale: locale,
      builder: (context, child) => MediaQuery(
        data: MediaQuery.of(
          context,
        ).copyWith(textScaler: TextScaler.linear(textScale)),
        child: child!,
      ),
      theme: knowoffTheme(),
      localizationsDelegates: AppLocalizations.localizationsDelegates,
      supportedLocales: AppLocalizations.supportedLocales,
      home: screen,
    ),
  );
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
  testWidgets(
    'stale avatar picker cannot call replacement API or release its busy state',
    (tester) async {
      final first = _Api();
      final second = _Api();
      final oldPick = Completer<XFile?>();
      final newPick = Completer<XFile?>();
      Widget screen(_Api api, Completer<XFile?> pick) => Scaffold(
        body: ServiceAvatarUpload(
          key: const Key('same-avatar'),
          api: api,
          pickImage: () => pick.future,
        ),
      );
      await _pump(tester, screen(first, oldPick));
      await tester.tap(find.byKey(const Key('avatar-pick')));
      await tester.pump();
      await _pump(tester, screen(second, newPick));
      await tester.tap(find.byKey(const Key('avatar-pick')));
      await tester.pump();
      oldPick.complete(
        XFile.fromData(
          base64Decode(
            'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aZ1sAAAAASUVORK5CYII=',
          ),
          name: 'old.png',
          path: 'old.png',
        ),
      );
      await tester.runAsync(() async {
        await Future<void>.delayed(const Duration(milliseconds: 100));
      });
      await tester.pump();
      expect(
        second.avatarCatalogReads,
        0,
        reason: 'old picker cannot invoke replacement API',
      );
      expect(
        tester.widget<KoButton>(find.byKey(const Key('avatar-pick'))).onPressed,
        isNull,
        reason: 'old finally cannot clear new picker busy state',
      );
      newPick.complete(null);
      await tester.pumpAndSettle();
      expect(tester.takeException(), isNull);
    },
  );

  testWidgets(
    'current Week Winner title uses server flag rather than lifetime count',
    (t) async {
      final api = _Api();
      await _pump(t, ProfileScreen(api: api));
      expect(
        find.byKey(const Key('profile-current-week-winner')),
        findsNothing,
      );
      api.currentWeekWinner = true;
      await t.tap(find.byKey(const Key('profile-refresh')));
      await t.pumpAndSettle();
      expect(
        find.byKey(const Key('profile-current-week-winner')),
        findsOneWidget,
      );
      api.currentWeekWinner = false;
      await t.tap(find.byKey(const Key('profile-refresh')));
      await t.pumpAndSettle();
      expect(
        find.byKey(const Key('profile-current-week-winner')),
        findsNothing,
      );
    },
  );

  testWidgets('refreshing a cached profile keeps an unfinished nickname', (
    tester,
  ) async {
    final api = _Api();
    await _pump(tester, ProfileScreen(api: api));
    final field = find.byKey(const Key('profile-nickname'));
    await tester.enterText(field, 'Unfinished Alibi');
    await tester.tap(find.byKey(const Key('profile-refresh')));
    await tester.pumpAndSettle();
    expect(api.profileCalls, 2);
    expect(
      tester.widget<TextFormField>(field).controller!.text,
      'Unfinished Alibi',
    );
    api.failProfile = true;
    await tester.tap(find.byKey(const Key('profile-refresh')));
    await tester.pumpAndSettle();
    api.failProfile = false;
    await tester.tap(find.text('Retry'));
    await tester.pumpAndSettle();
    expect(api.profileCalls, 4);
    expect(
      tester.widget<TextFormField>(field).controller!.text,
      'Unfinished Alibi',
    );
    expect(tester.takeException(), isNull);
  });

  for (final size in [
    const Size(320, 640),
    const Size(430, 932),
    const Size(834, 1194),
    const Size(1440, 900),
  ]) {
    testWidgets('profile tasks use a deliberate workspace at $size', (
      tester,
    ) async {
      final api = _Api();
      await _pump(
        tester,
        ProfileScreen(api: api),
        width: size.width,
        height: size.height,
      );
      final editor = tester.getRect(find.byKey(const Key('profile-editor')));
      final statistics = tester.getRect(
        find.byKey(const Key('profile-statistics')),
      );
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

    testWidgets('store keeps wallet context beside catalog at $size', (
      tester,
    ) async {
      final api = _Api();
      await _pump(
        tester,
        StoreScreen(api: api),
        width: size.width,
        height: size.height,
      );
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

  testWidgets('profile retry recovers and nickname save updates API', (
    tester,
  ) async {
    final api = _Api()..failProfile = true;
    await _pump(tester, ProfileScreen(api: api));
    api.failProfile = false;
    await tester.tap(find.text('Retry'));
    await tester.pumpAndSettle();
    expect(api.profileCalls, 2);
    await tester.enterText(
      find.byKey(const Key('profile-nickname')),
      '  New Alibi  ',
    );
    await tester.tap(find.byKey(const Key('profile-save')));
    await tester.pumpAndSettle();
    expect(api.nickname, 'New Alibi');
  });

  testWidgets('store pass refreshes wallet and validates conversion', (
    tester,
  ) async {
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
    expect(find.byKey(const Key('store-native-billing')), findsOneWidget);
    expect(
      find.byKey(const Key('native-buy-coins')),
      findsNothing,
      reason: 'a legacy bundle-size preview is not a configured native offer',
    );
  });

  testWidgets('feedback refuses empty fields and sends trimmed values', (
    tester,
  ) async {
    final api = _Api();
    await _pump(tester, FeedbackScreen(api: api));
    await tester.tap(find.byKey(const Key('feedback-submit')));
    await tester.pumpAndSettle();
    expect(api.feedback, isNull);
    await tester.enterText(find.byKey(const Key('feedback-title')), '  Idea  ');
    await tester.enterText(
      find.byKey(const Key('feedback-message')),
      '  Better bluff  ',
    );
    await tester.tap(find.byKey(const Key('feedback-submit')));
    await tester.pumpAndSettle();
    expect(api.feedback, 'bug:Idea:Better bluff');
  });

  testWidgets(
    'approved custom avatar uses authenticated bytes and missing image falls back',
    (tester) async {
      final api = _Api()
        ..avatar = 'custom'
        ..avatarBytes = base64Decode(
          'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aZ1sAAAAASUVORK5CYII=',
        );
      await _pump(tester, ProfileScreen(api: api));
      expect(find.byKey(const Key('profile-custom-avatar')), findsOneWidget);
      expect(api.avatarReads, 1);
      api.avatarBytes = null;
      await tester.tap(find.byKey(const Key('profile-refresh')));
      await tester.pumpAndSettle();
      expect(find.byKey(const Key('profile-custom-avatar')), findsNothing);
      expect(find.byKey(const Key('profile-avatar')), findsOneWidget);
    },
  );

  testWidgets('preset avatar update uses server preset and refreshes profile', (
    tester,
  ) async {
    final api = _Api();
    await _pump(tester, ProfileScreen(api: api));
    await tester.ensureVisible(find.byKey(const Key('avatar-detective')));
    await tester.tap(find.byKey(const Key('avatar-detective')));
    await tester.pumpAndSettle();
    expect(api.avatar, 'detective');
    expect(
      tester.widget<DoodleIcon>(find.byKey(const Key('profile-avatar'))).doodle,
      Doodle.incognito,
    );
    expect(api.profileCalls, 2);
  });

  testWidgets(
    'notices localize with English fallback and maintenance countdown',
    (tester) async {
      await _pump(
        tester,
        Scaffold(
          body: ServiceNoticeTile(
            notice: {
              'type': 'maintenance',
              'title': const {'en': 'Maintenance window', 'tr': 'Bakım'},
              'body': const {'en': 'Back shortly'},
              'maintenance_start': DateTime.now()
                  .add(const Duration(hours: 2))
                  .toUtc()
                  .toIso8601String(),
            },
          ),
        ),
      );
      expect(find.text('Maintenance window'), findsOneWidget);
      expect(find.text('Back shortly'), findsOneWidget);
      expect(find.textContaining('Maintenance in'), findsOneWidget);
      expect(
        serviceNoticeText({'en': 'Fallback'}, const Locale('fr')),
        'Fallback',
      );
      await tester.pumpWidget(const SizedBox.shrink());
    },
  );

  testWidgets('public profiles hide owner balance and editing controls', (
    tester,
  ) async {
    final api = _Api();
    await _pump(tester, ProfileScreen(api: api, accountId: 'other-account'));
    expect(find.text('Non-Converted Points'), findsNothing);
    expect(find.byKey(const Key('profile-nickname')), findsNothing);
    expect(find.text('Report player'), findsOneWidget);
  });

  testWidgets('report requires reason and sends chosen account target', (
    tester,
  ) async {
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
                accountId: 'other-account',
              ),
            ),
          ),
        ),
      ),
    );
    await tester.tap(find.text('Report'));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('report-submit')));
    await tester.pumpAndSettle();
    expect(api.report, isNull);
    await tester.enterText(
      find.byKey(const Key('report-reason')),
      '  Harassment  ',
    );
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
          body: ServiceAvatarUpload(api: api, pickImage: () async => null),
        ),
      );
      await tester.tap(find.byKey(const Key('avatar-pick')));
      await tester.pumpAndSettle();
      expect(api.uploads, 0);
      expect(find.byKey(const Key('avatar-upload')), findsNothing);
      final large = _OversizeImage();
      await tester.pumpWidget(const SizedBox.shrink());
      await _pump(
        tester,
        Scaffold(
          body: ServiceAvatarUpload(api: api, pickImage: () async => large),
        ),
      );
      await tester.tap(find.byKey(const Key('avatar-pick')));
      await tester.pumpAndSettle();
      expect(large.read, isFalse);
      expect(api.uploads, 0);
      expect(
        find.text('Choose a JPEG or PNG within the size limits.'),
        findsOneWidget,
      );
    },
  );

  testWidgets('avatar selection awaits explicit upload and reports success', (
    tester,
  ) async {
    final api = _Api();
    final image = XFile.fromData(
      base64Decode(
        'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aZ1sAAAAASUVORK5CYII=',
      ),
      name: 'avatar.png',
      path: 'avatar.png',
      mimeType: 'image/png',
    );
    await _pump(
      tester,
      Scaffold(
        body: ServiceAvatarUpload(api: api, pickImage: () async => image),
      ),
    );
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
    expect(find.text('Avatar approved and updated.'), findsOneWidget);
  });

  testWidgets('failed avatar upload preserves selected bytes and paid ownership', (
    tester,
  ) async {
    final api = _Api()
      ..avatarUploadError = const ApiException(422, code: 'avatar.flagged');
    final image = XFile.fromData(
      base64Decode(
        'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aZ1sAAAAASUVORK5CYII=',
      ),
      name: 'avatar.png',
      path: 'avatar.png',
    );
    await _pump(
      tester,
      Scaffold(
        body: SingleChildScrollView(
          child: ServiceAvatarUpload(api: api, pickImage: () async => image),
        ),
      ),
    );
    await tester.tap(find.byKey(const Key('avatar-pick')));
    await tester.runAsync(() async {
      await Future<void>.delayed(const Duration(milliseconds: 100));
    });
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('avatar-upload')));
    await tester.pumpAndSettle();
    expect(find.text('avatar.png'), findsOneWidget);
    expect(find.byKey(const Key('avatar-unlock')), findsNothing);
    expect(api.avatarPurchases, 0);
    expect(
      find.text(
        'This image was not approved. Choose another image; your unlock remains yours.',
      ),
      findsOneWidget,
    );
    api.avatarUploadError = null;
    api.pendingAvatar = Completer<void>();
    await tester.tap(find.byKey(const Key('avatar-upload')));
    await tester.pump();
    await tester.pumpWidget(const SizedBox.shrink());
    api.pendingAvatar!.complete();
    await tester.pumpAndSettle();
    expect(tester.takeException(), isNull);
  });

  testWidgets('avatar requires explicit permanent unlock and cancel is free', (
    tester,
  ) async {
    final api = _Api()..avatarOwned = false;
    final image = XFile.fromData(
      base64Decode(
        'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aZ1sAAAAASUVORK5CYII=',
      ),
      name: 'avatar.png',
      path: 'avatar.png',
    );
    await _pump(
      tester,
      Scaffold(
        body: SingleChildScrollView(
          child: ServiceAvatarUpload(api: api, pickImage: () async => image),
        ),
      ),
    );
    await tester.tap(find.byKey(const Key('avatar-pick')));
    await tester.runAsync(() async {
      await Future<void>.delayed(const Duration(milliseconds: 100));
    });
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('avatar-upload')), findsNothing);
    await tester.tap(find.byKey(const Key('avatar-unlock')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('avatar-unlock-cancel')));
    await tester.pumpAndSettle();
    expect(api.avatarPurchases, 0);
    expect(api.uploads, 0);
    await tester.tap(find.byKey(const Key('avatar-unlock')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('avatar-unlock-confirm')));
    await tester.pumpAndSettle();
    expect(api.avatarPurchases, 1);
    expect(api.uploads, 0);
    await tester.tap(find.byKey(const Key('avatar-upload')));
    await tester.pumpAndSettle();
    expect(api.uploads, 1);
  });
  testWidgets('disabled avatar screening never offers a new purchase or upload', (
    tester,
  ) async {
    final api = _Api()
      ..avatarOwned = false
      ..avatarAvailable = false;
    final image = XFile.fromData(
      base64Decode(
        'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aZ1sAAAAASUVORK5CYII=',
      ),
      name: 'avatar.png',
      path: 'avatar.png',
    );
    await _pump(
      tester,
      Scaffold(
        body: SingleChildScrollView(
          child: ServiceAvatarUpload(api: api, pickImage: () async => image),
        ),
      ),
    );
    await tester.tap(find.byKey(const Key('avatar-pick')));
    await tester.runAsync(() async {
      await Future<void>.delayed(const Duration(milliseconds: 100));
    });
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('avatar-unlock')), findsNothing);
    expect(find.byKey(const Key('avatar-upload')), findsNothing);
    expect(api.avatarPurchases, 0);
    expect(api.uploads, 0);
    expect(
      find.text(
        'Avatar screening is unavailable. Your image and unlock are unchanged.',
      ),
      findsOneWidget,
    );
  });

  for (final size in [const Size(360, 800), const Size(1440, 900)]) {
    for (final locale in [const Locale('en'), const Locale('en', 'XA')]) {
      testWidgets('account acceptance sweep $size $locale at 2x text', (
        tester,
      ) async {
        final api = _Api();
        for (final screen in [
          ProfileScreen(api: api),
          LeaderboardScreen(api: api),
          StoreScreen(api: api),
          NoticeInboxScreen(api: api),
          FeedbackScreen(api: api),
        ]) {
          await _pump(
            tester,
            screen,
            width: size.width,
            height: size.height,
            locale: locale,
            textScale: 2,
          );
          expect(
            tester.takeException(),
            isNull,
            reason: '${screen.runtimeType} initial layout',
          );
          final scroll = find.byType(SingleChildScrollView).first;
          await tester.drag(scroll, const Offset(0, -1800));
          await tester.pumpAndSettle();
          expect(
            tester.takeException(),
            isNull,
            reason: '${screen.runtimeType} scrolled layout',
          );
        }
      });
    }
  }

  for (final width in [320.0, 1440.0]) {
    testWidgets('account pages have no overflow at width $width', (
      tester,
    ) async {
      final api = _Api();
      for (final screen in [
        ProfileScreen(api: api),
        LeaderboardScreen(api: api),
        StoreScreen(api: api),
        NoticeInboxScreen(api: api),
        FeedbackScreen(api: api),
      ]) {
        await _pump(tester, screen, width: width);
        expect(tester.takeException(), isNull);
      }
    });
  }
}
