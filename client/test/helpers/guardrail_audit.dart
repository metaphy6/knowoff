import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/presentation/widgets/guardrail_audit.dart';

/// Pumps [widget] and returns any guardrail violations found in its tree.
Future<List<String>> auditWidget(WidgetTester tester, Widget widget) async {
  await tester.pumpWidget(widget);
  final violations = <String>[];
  tester.binding.rootElement?.visitChildren((element) {
    violations.addAll(GuardrailAudit.auditElement(element));
  });
  return violations;
}
