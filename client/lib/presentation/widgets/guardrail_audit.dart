import 'package:flutter/widgets.dart';

/// Guardrail audit helper.
///
/// Walks an element tree looking for performance-affecting rendering choices
/// that violate the design matrix:
///   * blurred shadows,
///   * BackdropFilter / ImageFiltered blur,
///   * more than one gradient,
///   * Opacity-based translucency.
///
/// The helper is invoked from tests via `test/helpers/guardrail_audit.dart`,
/// which pumps a widget and supplies the root element. This file does not
/// import `flutter_test` so it stays safe for production compilation.
class GuardrailAudit {
  static List<String> auditElement(Element root) {
    final violations = <String>[];
    var gradientCount = 0;

    void visit(Element element) {
      final w = element.widget;

      if (w is BackdropFilter) {
        violations.add('BackdropFilter blur found');
      }
      if (w is ImageFiltered) {
        violations.add('ImageFiltered blur found');
      }
      if (w is Opacity) {
        violations.add('Opacity translucency found (${w.opacity})');
      }

      Decoration? decoration;
      if (w is Container) decoration = w.decoration;
      if (w is DecoratedBox) decoration = w.decoration;

      if (decoration is BoxDecoration) {
        if (decoration.gradient != null) {
          gradientCount++;
          if (gradientCount > 1) {
            violations.add('More than one gradient found');
          }
        }
        for (final shadow in decoration.boxShadow ?? <BoxShadow>[]) {
          if (shadow.blurRadius != 0) {
            violations.add('Blurred shadow found: $shadow');
          }
        }
      }

      element.visitChildren(visit);
    }

    visit(root);
    return violations;
  }
}
