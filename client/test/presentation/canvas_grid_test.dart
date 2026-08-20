import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/presentation/theme/ko_canvas_grid.dart';

void main() {
  testWidgets('KoCanvasGridPainter paints without error', (tester) async {
    await tester.pumpWidget(
      const MaterialApp(
        home: CustomPaint(
          size: Size(200, 200),
          painter: KoCanvasGridPainter(),
        ),
      ),
    );
    expect(
      find.byWidgetPredicate(
        (w) => w is CustomPaint && w.painter is KoCanvasGridPainter,
      ),
      findsOneWidget,
    );
  });
}
