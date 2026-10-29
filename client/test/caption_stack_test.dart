import 'package:material_ui/material_ui.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:parley/model/protocol.dart';
import 'package:parley/theme.dart';
import 'package:parley/ui/caption_stack.dart';

void main() {
  testWidgets(
    'caption stack: DEMO banner, source above translation, newest low',
    (tester) async {
      tester.view.physicalSize = const Size(420, 780);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.reset);

      const segments = [
        Segment(
          segId: 1,
          source: 'First source.',
          translation: 'First translation.',
          demo: true,
        ),
        Segment(
          segId: 2,
          source: 'Second source.',
          translation: 'Second translation.',
          demo: true,
        ),
        Segment(
          segId: 3,
          source: 'Third source.',
          translation: 'Third translation.',
          demo: true,
        ),
      ];

      await tester.pumpWidget(
        MaterialApp(
          theme: parleyTheme(Brightness.dark),
          home: const Scaffold(
            body: CaptionStack(segments: segments, demo: true),
          ),
        ),
      );

      expect(find.text('DEMO — sample audio, mock providers'), findsOneWidget);

      final sourceY = tester.getTopLeft(find.text('Third source.')).dy;
      final translationY = tester
          .getTopLeft(find.text('Third translation.'))
          .dy;
      expect(
        sourceY,
        lessThan(translationY),
        reason: 'the source line sits above its translation',
      );

      expect(
        tester.getTopLeft(find.text('First translation.')).dy,
        lessThan(tester.getTopLeft(find.text('Third translation.')).dy),
        reason: 'the newest Segment is the lowest one',
      );

      final newest = tester.widget<Text>(find.text('Third translation.'));
      expect(newest.style?.fontSize, 28);

      await expectLater(
        find.byType(CaptionStack),
        matchesGoldenFile('goldens/caption_stack.png'),
      );
    },
  );
}
