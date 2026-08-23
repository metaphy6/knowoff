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

    // A `Container` builds its own `DecoratedBox` carrying the identical
    // decoration object. Counting both would report one painted gradient as
    // two, so the container's decoration is skipped once on the way down.
    void visit(Element element, Decoration? skip) {
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
      var handedDown = skip;
      if (w is Container) {
        decoration = w.decoration;
        handedDown = decoration;
      } else if (w is DecoratedBox) {
        if (identical(w.decoration, skip)) {
          decoration = null;
          handedDown = null;
        } else {
          decoration = w.decoration;
        }
      }

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

      element.visitChildren((child) => visit(child, handedDown));
    }

    visit(root, null);
    return violations;
  }
}
