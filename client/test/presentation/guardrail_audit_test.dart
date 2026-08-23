import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/presentation/theme/knowoff_tokens.dart';

import '../helpers/guardrail_audit.dart';

void main() {
  testWidgets('GuardrailAudit catches blur and extra gradient', (tester) async {
    final violations = await auditWidget(
      tester,
      MaterialApp(
        home: Scaffold(
          body: Column(
            children: [
              Container(
                decoration: const BoxDecoration(
                  gradient: LinearGradient(colors: [Colors.red, Colors.blue]),
                ),
              ),
              Container(
                decoration: const BoxDecoration(
                  gradient:
                      LinearGradient(colors: [Colors.green, Colors.yellow]),
                ),
              ),
              Container(
                decoration: const BoxDecoration(
                  boxShadow: [
                    BoxShadow(
                      color: Colors.black,
                      blurRadius: 4,
                      offset: Offset(2, 2),
                    ),
                  ],
                ),
              ),
            ],
          ),
        ),
      ),
    );

    expect(violations.any((v) => v.startsWith('Blurred shadow found')), isTrue);
    expect(violations, contains('More than one gradient found'));
  });

  testWidgets('GuardrailAudit reports zero violations for compliant widgets',
      (tester) async {
    final violations = await auditWidget(
      tester,
      const MaterialApp(
        home: Scaffold(
          body: SizedBox.shrink(),
        ),
      ),
    );

    expect(violations, isEmpty);
  });

  testWidgets("GuardrailAudit counts a Container's gradient exactly once",
      (tester) async {
    final violations = await auditWidget(
      tester,
      MaterialApp(
        home: Scaffold(
          body: Container(
            decoration: const BoxDecoration(
              gradient: KoColors.revealGradient,
            ),
          ),
        ),
      ),
    );

    expect(violations, isEmpty);
  });

  testWidgets('GuardrailAudit still catches two separate gradient containers',
      (tester) async {
    final violations = await auditWidget(
      tester,
      MaterialApp(
        home: Scaffold(
          body: Column(
            children: <Widget>[
              Container(
                decoration: const BoxDecoration(
                  gradient: KoColors.revealGradient,
                ),
              ),
              Container(
                decoration: const BoxDecoration(
                  gradient: KoColors.revealGradient,
                ),
              ),
            ],
          ),
        ),
      ),
    );

    expect(violations, contains('More than one gradient found'));
  });
}
