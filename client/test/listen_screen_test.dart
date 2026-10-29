import 'package:material_ui/material_ui.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:parley/l10n/parley_localizations.dart';
import 'package:parley/model/protocol.dart';
import 'package:parley/session/listen_session.dart';
import 'package:parley/theme.dart';
import 'package:parley/ui/caption_stack.dart';
import 'package:parley/ui/listen_screen.dart';
import 'package:parley/ui/live_line.dart';
import 'package:web_socket_channel/web_socket_channel.dart';

import 'fakes.dart';

Map<String, dynamic> helloMessage() => {
  'type': 'hello',
  'pairs': [
    {
      'a': 'ja',
      'b': 'zh',
      'directions': [
        {'src': 'ja', 'tgt': 'zh'},
        {'src': 'zh', 'tgt': 'ja'},
      ],
      'converse': true,
    },
  ],
  'packs': <dynamic>[],
  'presetsInUse': <dynamic>[],
  'demo': true,
};

Map<String, dynamic> segmentMessage(int segId) => {
  'type': 'segment',
  'seg_id': segId,
  'src': 'Source $segId.',
  'tgt': 'Translation $segId.',
  'direction': {'src': 'ja', 'tgt': 'zh'},
  'latency_ms': 399,
  'speak': false,
  'demo': true,
};

Future<ListenSession> connectedSession(FakeTransport transport) async {
  final session = ListenSession(
    transport: transport,
    audio: FakeAudioCapture(),
    ttsPlayer: FakeTtsPlayer(),
  );
  addTearDown(session.dispose);
  await session.connect();
  transport.emit(helloMessage());
  return session;
}

Future<void> pumpScreen(
  WidgetTester tester,
  ListenSession session, {
  bool advanced = false,
  Future<void> Function(bool on)? wakeLock,
  Brightness brightness = Brightness.dark,
  double textScale = 1,
}) async {
  await tester.pumpWidget(
    MaterialApp(
      theme: parleyTheme(brightness),
      home: Builder(
        builder: (context) => MediaQuery(
          data: MediaQuery.of(context)
              .copyWith(textScaler: TextScaler.linear(textScale)),
          child: ListenScreen(
            session: session,
            advanced: advanced,
            wakeLock: wakeLock ?? (_) async {},
          ),
        ),
      ),
    ),
  );
  await tester.pump();
}

void main() {
  testWidgets('a connect failure shows the generic error banner', (
    tester,
  ) async {
    // The IO backend's exception carries the full URI, token included; the
    // banner must never show it.
    final transport = FakeTransport()
      ..connectError = WebSocketChannelException.from(
        "WebSocketException: Connection to "
        "'http://127.0.0.1:3999/ws?token=SECRET123#' was not upgraded to "
        'websocket',
      );
    final session = ListenSession(
      transport: transport,
      audio: FakeAudioCapture(),
      ttsPlayer: FakeTtsPlayer(),
    );
    addTearDown(session.dispose);
    await session.connect();

    await tester.pumpWidget(
      MaterialApp(
        theme: parleyTheme(Brightness.dark),
        home: ListenScreen(session: session),
      ),
    );

    expect(find.text('Connection failed.'), findsOneWidget);
    expect(find.textContaining('SECRET123'), findsNothing);
    expect(find.textContaining('WebSocketException'), findsNothing);
    expect(find.text('Start Listen'), findsOneWidget);
  });

  testWidgets('a TTS failure shows the persistent subtitles-only chip', (
    tester,
  ) async {
    final transport = FakeTransport();
    final session = await connectedSession(transport);
    transport.emit({
      'type': 'error',
      'provider': 'tts',
      'kind': 'protocol',
      'message': 'voice missing',
      'fallback': 'subtitles only',
    });
    await tester.pump();
    await pumpScreen(tester, session);

    expect(find.text('Audio unavailable — subtitles only'), findsOneWidget);
    // The technical message stays in Settings, not in the chip.
    expect(find.textContaining('voice missing'), findsNothing);

    transport.emit(segmentMessage(1));
    await tester.pumpAndSettle();
    expect(session.segments, hasLength(1));
    expect(find.text('Audio unavailable — subtitles only'), findsOneWidget);
    expect(find.text('Translation 1.'), findsOneWidget);
  });

  testWidgets('a connected demo shows the pair and enables Start', (
    tester,
  ) async {
    final transport = FakeTransport();
    final session = ListenSession(
      transport: transport,
      audio: FakeAudioCapture(),
      ttsPlayer: FakeTtsPlayer(),
      loadDemoClip: () async => DemoClip.fromJson({
        'id': 'placeholder',
        'duration_ms': 100,
        'audio': <dynamic>[],
        'src': 'zh',
        'tgt': 'ja',
      }),
    );
    addTearDown(session.dispose);
    await session.connect();
    transport.emit({
      'type': 'hello',
      'pairs': [
        {
          'a': 'ja',
          'b': 'zh',
          'directions': [
            {'src': 'ja', 'tgt': 'zh'},
            {'src': 'zh', 'tgt': 'ja'},
          ],
          'converse': true,
        },
      ],
      'demo': true,
    });
    await tester.pumpWidget(
      MaterialApp(
        theme: parleyTheme(Brightness.dark),
        home: ListenScreen(session: session),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.text('ja ↔ zh'), findsOneWidget);
    final button = tester.widget<FilledButton>(find.byType(FilledButton));
    expect(button.onPressed, isNotNull);
  });

  testWidgets('Aa steps captions 24/28/34/40', (tester) async {
    final transport = FakeTransport();
    final session = await connectedSession(transport);
    transport.emit(segmentMessage(1));
    await tester.pump();
    await pumpScreen(tester, session);

    double captionSize() =>
        tester.widget<Text>(find.text('Translation 1.')).style!.fontSize!;

    expect(captionSize(), 28);
    final steps = <double>[];
    for (var i = 0; i < 4; i++) {
      await tester.tap(find.byKey(const Key('caption-size')));
      await tester.pump();
      steps.add(captionSize());
    }
    expect(steps, [34, 40, 24, 28]);
  });

  testWidgets('captions are never clamped at 200% text', (tester) async {
    final transport = FakeTransport();
    final session = await connectedSession(transport);
    transport.emit(segmentMessage(1));
    await tester.pump();
    await pumpScreen(tester, session, textScale: 2);

    final caption = tester.widget<Text>(find.text('Translation 1.'));
    expect(caption.style!.fontSize, 28);
    expect(caption.maxLines, isNull);
    expect(caption.overflow, isNull);

    final source = tester.widget<Text>(find.text('Source 1.'));
    expect(source.maxLines, isNull);

    final scaler = MediaQuery.textScalerOf(
      tester.element(find.text('Translation 1.')),
    );
    expect(scaler.scale(28), 56, reason: 'the OS scale applies on top');
    expect(tester.takeException(), isNull);
  });

  testWidgets('focus mode hides source and chrome, keeps DEMO and awake', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(420, 900);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);

    final transport = FakeTransport();
    final session = await connectedSession(transport);
    transport.emit(segmentMessage(1));
    await tester.pump();
    final awake = <bool>[];
    await pumpScreen(tester, session, wakeLock: (on) async => awake.add(on));

    expect(find.text('Parley'), findsOneWidget);
    expect(find.text('Start Listen'), findsOneWidget);

    await tester.tap(find.byKey(const Key('focus-mode')));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 300));

    expect(awake, [true]);
    expect(find.text('Parley'), findsNothing);
    expect(find.text('Start Listen'), findsNothing);
    expect(find.text('Source 1.'), findsNothing);
    expect(find.text('Translation 1.'), findsOneWidget);
    expect(
      tester.widget<Text>(find.text('Translation 1.')).style!.fontSize,
      34,
      reason: 'Focus mode enlarges the caption',
    );
    expect(find.text('DEMO — sample audio, mock providers'), findsOneWidget);
    expect(find.byKey(const Key('exit-focus')), findsOneWidget);

    await tester.tap(find.byType(CaptionStack));
    await tester.pump();
    expect(find.byKey(const Key('jump-to-live')), findsOneWidget);

    await tester.tap(find.byKey(const Key('jump-to-live')));
    await tester.pump();
    expect(find.byKey(const Key('jump-to-live')), findsNothing);

    await tester.tap(find.byKey(const Key('exit-focus')));
    await tester.pump();
    expect(awake, [true, false]);
    expect(find.text('Parley'), findsOneWidget);
  });

  testWidgets('Advanced in the ⋯ menu reveals latency chips', (tester) async {
    final transport = FakeTransport();
    final session = await connectedSession(transport);
    transport.emit(segmentMessage(1));
    transport.emit({'type': 'hold', 'held': true, 'merges': 2});
    await tester.pump();

    await pumpScreen(tester, session);
    expect(find.text('399 ms'), findsNothing);
    expect(find.text(' · 2 merges'), findsNothing);

    // Fixed pumps, not pumpAndSettle: a Held line pulses forever after 1.5 s.
    await tester.tap(find.byKey(const Key('more-menu')));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 300));
    await tester.tap(find.byType(CheckedPopupMenuItem<String>));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 300));
    expect(find.text('399 ms'), findsOneWidget);
    expect(find.text(' · 2 merges'), findsOneWidget);
  });

  testWidgets('meets the accessibility guidelines in dark and light', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(420, 900);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    final handle = tester.ensureSemantics();

    final transport = FakeTransport();
    final session = await connectedSession(transport);
    transport.emit(segmentMessage(1));
    await tester.pump();

    for (final brightness in [Brightness.dark, Brightness.light]) {
      await pumpScreen(tester, session, brightness: brightness);
      await tester.pumpAndSettle();
      await expectLater(tester, meetsGuideline(androidTapTargetGuideline));
      await expectLater(tester, meetsGuideline(iOSTapTargetGuideline));
      await expectLater(tester, meetsGuideline(labeledTapTargetGuideline));
      await expectLater(tester, meetsGuideline(textContrastGuideline));
    }
    handle.dispose();
  });

  testWidgets('the Listen surface follows the UI locale', (tester) async {
    final transport = FakeTransport();
    final session = await connectedSession(transport);
    await tester.pumpWidget(
      MaterialApp(
        theme: parleyTheme(Brightness.dark),
        locale: const Locale('zh'),
        localizationsDelegates: parleyLocalizationDelegates,
        supportedLocales: parleySupportedLocales,
        home: ListenScreen(session: session),
      ),
    );
    await tester.pump();

    expect(find.text('已连接'), findsOneWidget);
    expect(find.text('开始聆听'), findsOneWidget);
    expect(find.text('字幕会显示在这里。'), findsOneWidget);
    expect(find.byTooltip('专注模式'), findsOneWidget);
    expect(find.byTooltip('字幕大小'), findsOneWidget);

    await tester.pumpWidget(
      MaterialApp(
        theme: parleyTheme(Brightness.dark),
        locale: const Locale('ja'),
        localizationsDelegates: parleyLocalizationDelegates,
        supportedLocales: parleySupportedLocales,
        home: ListenScreen(session: session),
      ),
    );
    await tester.pump();

    expect(find.text('接続済み'), findsOneWidget);
    expect(find.text('聞くを開始'), findsOneWidget);
    expect(find.text('字幕がここに表示されます。'), findsOneWidget);
    expect(find.byTooltip('集中モード'), findsOneWidget);
    expect(find.byTooltip('字幕サイズ'), findsOneWidget);
  });

  testWidgets('the live line follows the UI locale', (tester) async {
    await tester.pumpWidget(
      MaterialApp(
        theme: parleyTheme(Brightness.dark),
        locale: const Locale('zh'),
        localizationsDelegates: parleyLocalizationDelegates,
        supportedLocales: parleySupportedLocales,
        home: const Scaffold(
          body: LiveLine(caption: LiveCaption(stable: '你好')),
        ),
      ),
    );
    await tester.pump(const Duration(seconds: 6));
    await tester.pump();

    expect(find.text('比平时更久'), findsOneWidget);
    expect(find.text('重试'), findsOneWidget);
    expect(find.text('跳过'), findsOneWidget);
  });
}
