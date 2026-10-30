import 'package:material_ui/material_ui.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:parley/model/protocol.dart';
import 'package:parley/theme.dart';
import 'package:parley/ui/caption_stack.dart';

const first = Segment(
  segId: 1,
  source: 'First source.',
  translation: 'First translation.',
  demo: true,
);
const second = Segment(
  segId: 2,
  source: 'Second source.',
  translation: 'Second translation.',
  demo: true,
);
const third = Segment(
  segId: 3,
  source: 'Third source.',
  translation: 'Third translation.',
  demo: true,
);
const segments = [first, second, third];

Future<void> pumpSurface(
  WidgetTester tester,
  Widget child, {
  Brightness brightness = Brightness.dark,
  double textScale = 1,
  bool highContrast = false,
}) async {
  await tester.pumpWidget(
    MaterialApp(
      theme: parleyTheme(brightness),
      home: Builder(
        builder: (context) => MediaQuery(
          data: MediaQuery.of(context).copyWith(
            textScaler: TextScaler.linear(textScale),
            highContrast: highContrast,
          ),
          child: Scaffold(body: child),
        ),
      ),
    ),
  );
}

void main() {
  testWidgets(
    'caption stack: DEMO banner, source above translation, newest low',
    (tester) async {
      tester.view.physicalSize = const Size(420, 780);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.reset);

      await pumpSurface(
        tester,
        const CaptionStack(segments: segments, demo: true),
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
    },
  );

  testWidgets('caption column keeps 24dp margins at 600dp width', (
    tester,
  ) async {
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);

    tester.view.physicalSize = const Size(599, 700);
    await pumpSurface(
      tester,
      const CaptionStack(
        segments: [first],
        live: LiveCaption(stable: 'Hello'),
      ),
    );
    expect(tester.getTopLeft(find.text('First translation.')).dx, 16);
    expect(tester.getTopLeft(find.byKey(const Key('live-partial'))).dx, 16);

    tester.view.physicalSize = const Size(600, 700);
    await pumpSurface(
      tester,
      const CaptionStack(
        segments: [first],
        live: LiveCaption(stable: 'Hello'),
      ),
    );
    expect(tester.getTopLeft(find.text('First translation.')).dx, 24);
    expect(tester.getTopLeft(find.byKey(const Key('live-partial'))).dx, 24);
  });

  testWidgets(
    'Partial: stable prefix upright, tail greyed, hidden from readers',
    (tester) async {
      final handle = tester.ensureSemantics();
      await pumpSurface(
        tester,
        const CaptionStack(
          segments: [first],
          live: LiveCaption(stable: 'Hello', provisional: ' wor'),
        ),
      );

      expect(
        tester.getTopLeft(find.text('First translation.')).dy,
        lessThan(tester.getTopLeft(find.byKey(const Key('live-partial'))).dy),
        reason: 'Committed Segments sit above the live line',
      );
      final rich = tester.widget<Text>(find.byKey(const Key('live-partial')));
      final spans = (rich.textSpan! as TextSpan).children!.cast<TextSpan>();
      expect(spans[0].text, 'Hello');
      expect(spans[0].style!.color, ParleyColors.dark.ink);
      expect(spans[1].text, ' wor');
      expect(spans[1].style!.color, ParleyColors.dark.provisional);

      expect(find.bySemanticsLabel('Hello wor'), findsNothing);
      final committed = find.bySemanticsLabel('First translation.');
      expect(committed, findsOneWidget);
      expect(
        tester
            .getSemantics(committed)
            .getSemanticsData()
            .flagsCollection
            .isLiveRegion,
        isTrue,
        reason: 'only Committed Segments are a polite live region',
      );
      handle.dispose();
    },
  );

  testWidgets('Hold: glyph, then still listening after 1.5 s, then finishing', (
    tester,
  ) async {
    final handle = tester.ensureSemantics();
    await pumpSurface(
      tester,
      const CaptionStack(
        segments: [first],
        held: true,
        live: LiveCaption(stable: 'まだ話しています'),
      ),
    );

    expect(find.text('still listening…'), findsNothing);
    await tester.pump(const Duration(milliseconds: 1499));
    expect(find.text('still listening…'), findsNothing);

    await tester.pump(const Duration(milliseconds: 1));
    expect(find.text('still listening…'), findsOneWidget);
    expect(find.bySemanticsLabel('still listening…'), findsNothing);

    await tester.pump(const Duration(milliseconds: 1000));
    expect(find.text('finishing…'), findsOneWidget);
    expect(find.bySemanticsLabel('finishing…'), findsNothing);
    expect(find.bySemanticsLabel('まだ話しています'), findsNothing);
    handle.dispose();
  });

  testWidgets('Hold merges are hidden from readers even under Advanced', (
    tester,
  ) async {
    final handle = tester.ensureSemantics();
    await pumpSurface(
      tester,
      const CaptionStack(
        segments: [first],
        held: true,
        merges: 2,
        advanced: true,
        live: LiveCaption(stable: 'まだ話しています'),
      ),
    );
    await tester.pump(const Duration(milliseconds: 1600));

    expect(find.textContaining('2 merges'), findsOneWidget);
    expect(find.bySemanticsLabel(RegExp('merges')), findsNothing);
    expect(find.bySemanticsLabel(RegExp('still listening')), findsNothing);
    handle.dispose();
  });

  testWidgets('slow translation says so after five seconds with Retry/Skip', (
    tester,
  ) async {
    var skipped = 0;
    await pumpSurface(
      tester,
      CaptionStack(
        segments: const [first],
        live: const LiveCaption(stable: 'Hello'),
        onSkipLive: () => skipped++,
      ),
    );

    expect(find.text('taking longer than usual'), findsNothing);
    await tester.pump(const Duration(seconds: 5));
    expect(find.text('taking longer than usual'), findsOneWidget);
    expect(find.text('Retry'), findsOneWidget);
    expect(find.text('Skip'), findsOneWidget);

    await tester.tap(find.text('Retry'));
    await tester.pump();
    expect(find.text('taking longer than usual'), findsNothing);

    await tester.pump(const Duration(seconds: 5));
    expect(find.text('taking longer than usual'), findsOneWidget);

    await tester.tap(find.text('Skip'));
    expect(skipped, 1);
  });

  testWidgets('reduce motion stops the pulse', (tester) async {
    await pumpSurface(
      tester,
      const CaptionStack(
        segments: [first],
        held: true,
        live: LiveCaption(stable: 'まだ話しています'),
      ),
    );
    await tester.pump(const Duration(milliseconds: 1600));
    expect(find.text('still listening…'), findsOneWidget);
    expect(
      tester.binding.transientCallbackCount,
      greaterThan(0),
      reason: 'the pulsing ellipsis animates by default',
    );

    await pumpSurface(
      tester,
      const MediaQuery(
        data: MediaQueryData(disableAnimations: true),
        child: CaptionStack(
          segments: [first],
          held: true,
          live: LiveCaption(stable: 'まだ話しています'),
        ),
      ),
    );
    await tester.pump(const Duration(milliseconds: 1600));
    expect(find.text('still listening…'), findsOneWidget);
    expect(tester.binding.transientCallbackCount, 0);
  });

  testWidgets('an inline error keeps the earlier Segments', (tester) async {
    await pumpSurface(
      tester,
      const CaptionStack(
        segments: [first, second],
        inlineError: '[translator] request timed out — Retry or switch preset.',
      ),
    );

    expect(find.text('First translation.'), findsOneWidget);
    expect(find.text('Second translation.'), findsOneWidget);
    expect(find.textContaining('request timed out'), findsOneWidget);
  });

  testWidgets('latency chips are shown only under Advanced', (tester) async {
    const timed = Segment(
      segId: 4,
      source: 'Source',
      translation: 'Translation',
      demo: true,
      latencyMs: 399,
    );

    await pumpSurface(tester, const CaptionStack(segments: [timed]));
    expect(find.text('399 ms'), findsNothing);

    await pumpSurface(
      tester,
      const CaptionStack(segments: [timed], showLatency: true),
    );
    expect(find.text('399 ms'), findsOneWidget);
  });

  testWidgets('OS high contrast switches muted and provisional text to ink', (
    tester,
  ) async {
    await pumpSurface(
      tester,
      const CaptionStack(
        segments: segments,
        live: LiveCaption(stable: 'Stable', provisional: ' tail'),
      ),
      highContrast: true,
    );

    expect(
      tester.widget<Text>(find.text('First source.')).style!.color,
      ParleyColors.dark.ink,
      reason: 'the source line is muted text',
    );
    expect(
      tester.widget<Text>(find.text('First translation.')).style!.color,
      ParleyColors.dark.ink,
      reason: 'older Segments are muted text',
    );
    final partial = tester.widget<Text>(find.byKey(const Key('live-partial')));
    final spans = (partial.textSpan! as TextSpan).children!.cast<TextSpan>();
    expect(spans.last.style!.color, ParleyColors.dark.ink);
  });

  testWidgets('each Segment style carries its own language locale', (
    tester,
  ) async {
    const japanese = Segment(
      segId: 1,
      source: '直',
      translation: '直',
      demo: false,
      direction: DirectionRef(src: 'ja', tgt: 'zh'),
    );
    await pumpSurface(tester, const CaptionStack(segments: [japanese]));

    final texts = tester.widgetList<Text>(find.text('直')).toList();
    expect(texts, hasLength(2));
    expect(texts.first.style!.locale, const Locale('ja'));
    expect(texts.last.style!.locale, const Locale('zh'));
  });

  testWidgets('goldens: dark, light and 200% text', (tester) async {
    tester.view.physicalSize = const Size(420, 780);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    const surface = CaptionStack(
      segments: segments,
      demo: true,
      live: LiveCaption(stable: 'Hello', provisional: ' wor'),
    );

    await pumpSurface(tester, surface);
    expect(
      tester.widget<Text>(find.text('Third translation.')).style!.color,
      ParleyColors.dark.ink,
    );
    await expectLater(
      find.byType(Scaffold),
      matchesGoldenFile('goldens/caption_dark.png'),
    );

    await pumpSurface(tester, surface, brightness: Brightness.light);
    await tester.pumpAndSettle();
    expect(
      tester.widget<Text>(find.text('Third translation.')).style!.color,
      ParleyColors.light.ink,
    );
    await expectLater(
      find.byType(Scaffold),
      matchesGoldenFile('goldens/caption_light.png'),
    );

    await pumpSurface(tester, surface, textScale: 2);
    await tester.pumpAndSettle();
    await expectLater(
      find.byType(Scaffold),
      matchesGoldenFile('goldens/caption_text_200.png'),
    );
  });
}
