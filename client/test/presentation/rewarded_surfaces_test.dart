import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/data/rewarded_session.dart';
import 'package:knowoff_client/l10n/app_localizations.dart';
import 'package:knowoff_client/presentation/theme/knowoff_theme.dart';
import 'package:knowoff_client/presentation/widgets/rewarded_lifecycle.dart';
import 'package:knowoff_client/presentation/widgets/rewarded_offer.dart';
import '../data/rewarded_ads_test.dart' show Claims, Platform, unit, match, at;
import '../data/rewarded_session_test.dart' show Harness;

void main() {
  testWidgets(
    'ad lifecycle defers background mount and disposes replaced owners',
    (t) async {
      final first = Harness(), second = Harness();
      t.binding.handleAppLifecycleStateChanged(AppLifecycleState.paused);
      await t.pumpWidget(
        RewardedLifecycle(session: first.session, child: const SizedBox()),
      );
      await t.pumpAndSettle();
      expect(first.platform.forms, 0);
      t.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
      await t.pump();
      await t.pumpAndSettle();
      expect(first.platform.forms, 1);
      final pending = first.session.offer(
        const RewardedMatchCandidate(
          matchId: match,
          accountId: 'owner',
          generation: 1,
        ),
      );
      await t.pumpAndSettle();
      await pending;
      expect(first.session.ready, true);
      t.binding.handleAppLifecycleStateChanged(AppLifecycleState.inactive);
      await t.pumpAndSettle();
      expect(first.session.ready, true);
      t.binding.handleAppLifecycleStateChanged(AppLifecycleState.paused);
      await t.pumpAndSettle();
      expect(first.session.ready, false);
      expect(first.platform.ad.disposals, 1);
      await t.pumpWidget(
        RewardedLifecycle(session: second.session, child: const SizedBox()),
      );
      await t.pumpAndSettle();
      expect(second.platform.forms, 0);
      t.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
      await t.pump();
      await t.pumpAndSettle();
      expect(first.platform.forms, 2);
      expect(first.session.supported, false);
      expect(second.platform.forms, 1);
      await t.pumpWidget(const SizedBox());
      expect(second.session.supported, false);
      first.identity.dispose();
      second.identity.dispose();
    },
  );

  for (final locale in const [
    Locale('en'),
    Locale('tr'),
    Locale('ar'),
    Locale('en', 'XA'),
  ]) {
    testWidgets('reward offer is scoped, explicit and localized $locale', (
      t,
    ) async {
      t.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
      t.view.physicalSize = const Size(360, 1000);
      t.view.devicePixelRatio = 1;
      addTearDown(t.view.resetPhysicalSize);
      addTearDown(t.view.resetDevicePixelRatio);
      final claims = Claims(),
          platform = Platform()..privacyOptionsRequired = true;
      final identity = ValueNotifier<({String? accountId, int generation})>((
        accountId: claims.accountId,
        generation: claims.sessionGeneration,
      ));
      final session = RewardedSessionController(
        claims,
        platform,
        identityChanges: identity,
        sdkAdUnit: unit,
        now: () => at,
      );
      final candidate = RewardedMatchCandidate(
        matchId: match,
        accountId: claims.accountId!,
        generation: claims.sessionGeneration,
      );
      await t.pumpWidget(
        RewardedLifecycle(
          session: session,
          child: MaterialApp(
            locale: locale,
            localizationsDelegates: AppLocalizations.localizationsDelegates,
            supportedLocales: AppLocalizations.supportedLocales,
            theme: knowoffTheme(),
            home: MediaQuery(
              data: const MediaQueryData(textScaler: TextScaler.linear(2)),
              child: Scaffold(
                body: SingleChildScrollView(
                  child: Column(
                    children: [
                      RewardedOffer(session: session, candidate: candidate),
                      RewardedPrivacyButton(session: session),
                    ],
                  ),
                ),
              ),
            ),
          ),
        ),
      );
      await t.pumpAndSettle();
      expect(claims.issues, 0);
      expect(platform.loads, 0);
      expect(find.byKey(const Key('reward-watch-ad')), findsNothing);
      expect(find.byKey(const Key('reward-privacy-options')), findsOneWidget);
      final preparing = session.offer(candidate);
      await t.pumpAndSettle();
      await preparing;
      expect(
        session.ready,
        true,
        reason:
            'status=${session.status} failed=${session.failed} forms=${platform.forms} issues=${claims.issues} loads=${platform.loads} identity=${session.identity}',
      );
      expect(find.byKey(const Key('reward-watch-ad')), findsOneWidget);
      expect(platform.ad.shows, 0);
      expect(t.takeException(), isNull);
      await t.ensureVisible(find.byKey(const Key('reward-watch-ad')));
      await t.tap(find.byKey(const Key('reward-watch-ad')));
      await t.pumpAndSettle();
      expect(platform.ad.shows, 1);
      expect(claims.checks, 1);
      platform.ad.closed?.call();
      await t.pumpAndSettle();
      claims.sessionGeneration++;
      identity.value = (
        accountId: claims.accountId,
        generation: claims.sessionGeneration,
      );
      await t.pumpAndSettle();
      expect(find.byKey(const Key('reward-watch-ad')), findsNothing);
      await t.pumpWidget(const SizedBox());
      identity.dispose();
    });
  }
}
