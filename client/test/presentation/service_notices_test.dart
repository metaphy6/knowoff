import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/data/api_client.dart';
import 'package:knowoff_client/data/auth_service.dart';
import 'package:knowoff_client/l10n/app_localizations.dart';
import 'package:knowoff_client/presentation/widgets/service_notices.dart';

class _Notices extends ApiClient {
  _Notices()
    : super(
        baseUrl: 'http://test',
        auth: AuthService(baseUrl: 'http://test'),
      );
  List<dynamic> notices = [];
  bool fail = false;
  int calls = 0;
  Completer<List<dynamic>>? pending;
  @override
  Future<List<dynamic>> getNotices() async {
    calls++;
    if (pending != null) return pending!.future;
    if (fail) throw StateError('unavailable');
    return notices;
  }
}

void main() {
  testWidgets(
    'disposing a notice with a stalled request cancels timeout work',
    (tester) async {
      final api = _Notices()..pending = Completer<List<dynamic>>();
      await tester.pumpWidget(MaterialApp(home: ServiceNoticeBanner(api: api)));
      await tester.pumpWidget(const SizedBox.shrink());
      expect(tester.takeException(), isNull);
    },
  );

  testWidgets(
    'live notices refresh, preserve dismissal and show new reminders',
    (tester) async {
      final api = _Notices();
      final changes = ChangeNotifier();
      await tester.pumpWidget(
        MaterialApp(
          localizationsDelegates: AppLocalizations.localizationsDelegates,
          supportedLocales: AppLocalizations.supportedLocales,
          home: Scaffold(
            body: SingleChildScrollView(
              child: ServiceNoticeBanner(api: api, changes: changes),
            ),
          ),
        ),
      );
      await tester.pumpAndSettle();
      expect(find.byType(ServiceNoticeTile), findsNothing);
      final notice = <String, dynamic>{
        'id': 'maintenance-1',
        'type': 'maintenance',
        'title': 'Scheduled maintenance',
        'body': 'Finish your current match.',
        'reminder_stage': 0,
      };
      api.notices = [notice];
      changes.notifyListeners();
      await tester.pumpAndSettle();
      expect(find.text('Scheduled maintenance'), findsOneWidget);
      await tester.tap(find.byIcon(Icons.close));
      await tester.pumpAndSettle();
      changes.notifyListeners();
      await tester.pumpAndSettle();
      expect(find.byType(ServiceNoticeTile), findsNothing);
      api.notices = [
        {...notice, 'reminder_stage': 1},
      ];
      changes.notifyListeners();
      await tester.pumpAndSettle();
      expect(find.text('Scheduled maintenance'), findsOneWidget);
      api.notices = [];
      changes.notifyListeners();
      await tester.pumpAndSettle();
      expect(find.byType(ServiceNoticeTile), findsNothing);
      api.fail = true;
      changes.notifyListeners();
      await tester.pumpAndSettle();
      expect(tester.takeException(), isNull);
      api.fail = false;
      api.notices = [
        {...notice, 'reminder_stage': 2},
      ];
      await tester.pump(const Duration(seconds: 30));
      await tester.pumpAndSettle();
      expect(find.text('Scheduled maintenance'), findsOneWidget);
      final beforeBackground = api.calls;
      tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.inactive);
      changes.notifyListeners();
      await tester.pump(const Duration(seconds: 31));
      expect(api.calls, beforeBackground);
      tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
      await tester.pumpAndSettle();
      expect(api.calls, beforeBackground + 1);
      final calls = api.calls;
      await tester.pumpWidget(const SizedBox.shrink());
      changes.notifyListeners();
      expect(api.calls, calls);
      changes.dispose();
    },
  );
}
