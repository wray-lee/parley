import 'dart:convert';

import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';
import 'package:parley/l10n/parley_localizations.dart';
import 'package:parley/model/mode.dart';
import 'package:parley/session/listen_session.dart';
import 'package:parley/theme.dart';
import 'package:parley/ui/converse_screen.dart';

import 'fakes.dart';

Map<String, dynamic> hello({bool demo = false}) => {
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
  'demo': demo,
};

Map<String, dynamic> segment(
  int segId,
  String source,
  String translation,
  String src,
  String tgt,
) => {
  'type': 'segment',
  'seg_id': segId,
  'src': source,
  'tgt': translation,
  'direction': {'src': src, 'tgt': tgt},
  'latency_ms': 10,
  'speak': tgt == 'ja',
  'demo': false,
};

/// A connected Converse Session with me speaking zh -> ja (them read ja),
/// not started yet.
Future<ListenSession> connectedSession(
  WidgetTester tester,
  FakeTransport transport, {
  bool demo = false,
}) async {
  final session = ListenSession(
    transport: transport,
    audio: FakeAudioCapture(),
    ttsPlayer: FakeTtsPlayer(),
  );
  addTearDown(session.dispose);
  await session.connect();
  transport.emit(hello(demo: demo));
  await tester.pump();
  final pair = session.pairs.first;
  session
    ..selectMode(SessionMode.converse)
    ..selectPair(pair)
    ..selectDirection(pair.direction('zh', 'ja'));
  return session;
}

/// Starts a Converse Session with me speaking zh -> ja (them read ja).
Future<ListenSession> startedSession(
  WidgetTester tester,
  FakeTransport transport, {
  bool withSegments = false,
}) async {
  final session = await connectedSession(tester, transport);
  await session.start();
  transport.emit({'type': 'turn', 'who': 'me', 'auto': false});
  if (withSegments) {
    transport.emit(
      segment(1, 'Good morning.', 'Good morning. How are you?', 'zh', 'ja'),
    );
    transport.emit(
      segment(2, 'I am fine.', 'I am fine, thank you.', 'ja', 'zh'),
    );
  }
  await tester.pump();
  return session;
}

Future<void> pumpConverse(
  WidgetTester tester,
  ListenSession session, {
  Size size = const Size(420, 780),
  double textScale = 1,
  bool accessibleNavigation = false,
  bool disableAnimations = false,
  TargetPlatform platform = TargetPlatform.android,
  bool? isWeb,
  VoidCallback? onUseKeys,
}) async {
  tester.view.physicalSize = size;
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.reset);
  await tester.pumpWidget(
    MaterialApp(
      theme: parleyTheme(Brightness.dark),
      localizationsDelegates: parleyLocalizationDelegates,
      supportedLocales: parleySupportedLocales,
      home: Builder(
        builder: (context) => MediaQuery(
          data: MediaQuery.of(context).copyWith(
            textScaler: TextScaler.linear(textScale),
            accessibleNavigation: accessibleNavigation,
            disableAnimations: disableAnimations,
          ),
          child: ConverseScreen(
            session: session,
            wakeLock: (_) async {},
            platform: platform,
            isWeb: isWeb,
            onUseKeys: onUseKeys,
          ),
        ),
      ),
    ),
  );
  await tester.pump();
}

List<Map<String, dynamic>> sent(FakeTransport transport) => transport.sentText
    .map((text) => jsonDecode(text) as Map<String, dynamic>)
    .toList(growable: false);

void main() {
  testWidgets('phone portrait splits, and only the far half rotates 180°', (
    tester,
  ) async {
    final transport = FakeTransport();
    final session = await startedSession(tester, transport);
    await pumpConverse(tester, session);

    expect(find.byKey(const Key('converse-split')), findsOneWidget);
    expect(find.byKey(const Key('converse-stacked')), findsNothing);
    final rotation = tester.widget<RotatedBox>(
      find.byKey(const Key('converse-rotation')),
    );
    expect(rotation.quarterTurns, 2);
    expect(
      find.descendant(
        of: find.byKey(const Key('converse-rotation')),
        matching: find.byKey(const Key('half-them')),
      ),
      findsOneWidget,
      reason: 'the far half belongs to them',
    );
  });

  testWidgets('large text, landscape, desktop and screen readers stack', (
    tester,
  ) async {
    final transport = FakeTransport();
    final session = await startedSession(tester, transport);

    await pumpConverse(tester, session, textScale: 1.6);
    expect(find.byKey(const Key('converse-stacked')), findsOneWidget);
    expect(find.byKey(const Key('converse-rotation')), findsNothing);

    await pumpConverse(tester, session, size: const Size(780, 420));
    expect(find.byKey(const Key('converse-stacked')), findsOneWidget);
    expect(find.byKey(const Key('converse-rotation')), findsNothing);

    // Landscape at 2x text still fits without clipped or rotated controls.
    await pumpConverse(
      tester,
      session,
      size: const Size(780, 420),
      textScale: 2,
    );
    expect(find.byKey(const Key('converse-stacked')), findsOneWidget);
    expect(find.byKey(const Key('converse-rotation')), findsNothing);

    await pumpConverse(tester, session, platform: TargetPlatform.linux);
    expect(find.byKey(const Key('converse-stacked')), findsOneWidget);
    expect(find.byKey(const Key('converse-rotation')), findsNothing);

    await pumpConverse(tester, session, isWeb: true);
    expect(find.byKey(const Key('converse-stacked')), findsOneWidget);
    expect(find.byKey(const Key('converse-rotation')), findsNothing);

    await pumpConverse(tester, session, accessibleNavigation: true);
    expect(find.byKey(const Key('converse-stacked')), findsOneWidget);
    expect(find.byKey(const Key('converse-rotation')), findsNothing);
  });

  testWidgets('landscape keeps the Turn pad clear of a live Partial and Hold', (
    tester,
  ) async {
    final transport = FakeTransport();
    final session = await startedSession(tester, transport);
    await pumpConverse(tester, session, size: const Size(780, 420));

    transport.emit({
      'type': 'asr_partial',
      'text': '今日はいい天気ですね。散歩に行きましょうか。駅まで歩いて十分くらいです。一緒にコーヒーを買って、公園で休みましょう。',
    });
    transport.emit({'type': 'hold', 'held': true, 'merges': 3});
    await tester.pump();
    await tester.pump();

    expect(tester.takeException(), isNull);
    expect(find.byKey(const Key('converse-stacked')), findsOneWidget);
    expect(find.byKey(const Key('live-partial')), findsOneWidget);
    final half = tester.getRect(find.byKey(const Key('half-me')));
    final pad = tester.getRect(find.byKey(const Key('pad-me')));
    expect(pad.top, greaterThanOrEqualTo(half.top));
    expect(pad.bottom, lessThanOrEqualTo(half.bottom));
  });

  testWidgets('a live Partial never clips the pad at large text on a phone', (
    tester,
  ) async {
    final transport = FakeTransport();
    final session = await startedSession(tester, transport);

    for (final scale in [1.6, 2.0]) {
      await pumpConverse(
        tester,
        session,
        size: const Size(360, 640),
        textScale: scale,
      );
      transport.emit({
        'type': 'asr_partial',
        'text': '今日はいい天気ですね。散歩に行きましょうか。駅まで歩いて十分くらいです。コーヒーも買いたいですね。',
      });
      await tester.pump();
      await tester.pump();

      expect(tester.takeException(), isNull, reason: 'text scale $scale');
      expect(
        find.byKey(const Key('live-partial')),
        findsOneWidget,
        reason: 'text scale $scale',
      );
      expect(find.byKey(const Key('converse-stacked')), findsOneWidget);
      final half = tester.getRect(find.byKey(const Key('half-me')));
      final pad = tester.getRect(find.byKey(const Key('pad-me')));
      expect(pad.top, greaterThanOrEqualTo(half.top));
      expect(pad.bottom, lessThanOrEqualTo(half.bottom));
    }
  });

  testWidgets('Gating and a Turn cue in landscape keep both pads whole', (
    tester,
  ) async {
    final transport = FakeTransport();
    final session = await startedSession(tester, transport);
    await pumpConverse(
      tester,
      session,
      size: const Size(780, 420),
      textScale: 2,
    );

    transport.emit({'type': 'tts_begin', 'seg_id': 1, 'format': 'wav'});
    transport.emitBinary(Uint8List.fromList([1, 2, 3, 4]));
    transport.emit({'type': 'tts_end', 'seg_id': 1});
    transport.emit({'type': 'turn', 'who': 'them', 'auto': true});
    transport.emit({
      'type': 'asr_partial',
      'text': '今日はいい天気ですね。散歩に行きましょうか。駅まで歩いて十分くらいです。一緒にコーヒーを買って、公園で休みましょう。',
    });
    transport.emit({'type': 'hold', 'held': true, 'merges': 3});
    await tester.pump();
    await tester.pump();

    expect(session.gated, isTrue);
    expect(find.byKey(const Key('turn-cue-them')), findsOneWidget);
    expect(find.byIcon(Icons.mic_off_outlined), findsNWidgets(2));
    expect(tester.takeException(), isNull);
    for (final who in ['me', 'them']) {
      final half = tester.getRect(find.byKey(Key('half-$who')));
      final pad = tester.getRect(find.byKey(Key('pad-$who')));
      expect(pad.top, greaterThanOrEqualTo(half.top), reason: who);
      expect(pad.bottom, lessThanOrEqualTo(half.bottom), reason: who);
    }

    // Let the 500 ms Gating tail fire before the test ends.
    await tester.pump(const Duration(milliseconds: 600));
  });

  testWidgets('a tap takes the Turn and sends turn {who}', (tester) async {
    final transport = FakeTransport();
    final session = await startedSession(tester, transport);
    await pumpConverse(tester, session);

    await tester.tap(find.byKey(const Key('pad-them')));
    await tester.pump();
    expect(sent(transport).last, {'type': 'turn', 'who': 'them'});

    final before = transport.sentText.length;
    await tester.tap(find.byKey(const Key('pad-me')));
    await tester.pump();
    expect(
      transport.sentText.length,
      before,
      reason: 'I already hold the Turn, so a tap sends nothing',
    );
  });

  testWidgets('a hold is push-to-talk and cuts on release', (tester) async {
    final transport = FakeTransport();
    final session = await startedSession(tester, transport);
    await pumpConverse(tester, session);
    final before = transport.sentText.length;

    final gesture = await tester.startGesture(
      tester.getCenter(find.byKey(const Key('pad-them'))),
    );
    await tester.pump(const Duration(milliseconds: 600));
    expect(sent(transport).sublist(before), [
      {'type': 'turn', 'who': 'them'},
      {'type': 'ptt', 'state': 'down'},
    ]);

    await gesture.up();
    await tester.pump();
    expect(sent(transport).last, {'type': 'ptt', 'state': 'up'});
  });

  testWidgets('each half is labelled by speaker for screen readers', (
    tester,
  ) async {
    final handle = tester.ensureSemantics();
    final transport = FakeTransport();
    final session = await startedSession(tester, transport, withSegments: true);
    await pumpConverse(tester, session);

    // Each half's label is scoped to that speaker's language.
    expect(find.bySemanticsLabel(RegExp('我 · 中文')), findsOneWidget);
    expect(find.bySemanticsLabel(RegExp('相手 · 日本語')), findsOneWidget);
    expect(find.text('I am fine, thank you.'), findsOneWidget);
    expect(find.text('Good morning. How are you?'), findsOneWidget);
    handle.dispose();
  });

  testWidgets('Gating is visible while my speech plays, then releases', (
    tester,
  ) async {
    final transport = FakeTransport();
    final session = await startedSession(tester, transport);
    await pumpConverse(tester, session);

    transport.emit({'type': 'tts_begin', 'seg_id': 1, 'format': 'wav'});
    transport.emitBinary(Uint8List.fromList([1, 2, 3, 4]));
    transport.emit({'type': 'tts_end', 'seg_id': 1});
    await tester.pump();
    await tester.pump();
    await tester.pump();

    expect(session.gated, isTrue);
    expect(find.byIcon(Icons.mic_off_outlined), findsNWidgets(2));

    await tester.pump(const Duration(milliseconds: 600));
    await tester.pump();
    expect(find.byIcon(Icons.mic_off_outlined), findsNothing);
  });

  testWidgets('the Turn change is a 250 ms cue with a haptic', (tester) async {
    final calls = <MethodCall>[];
    tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
      SystemChannels.platform,
      (call) async {
        calls.add(call);
        return null;
      },
    );
    addTearDown(
      () => tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
        SystemChannels.platform,
        null,
      ),
    );
    final transport = FakeTransport();
    final session = await startedSession(tester, transport);
    await pumpConverse(tester, session);

    transport.emit({'type': 'turn', 'who': 'them', 'auto': true});
    await tester.pump();
    await tester.pump();

    expect(find.byKey(const ValueKey('cue-them')), findsOneWidget);
    final switcher = tester.widget<AnimatedSwitcher>(
      find.byKey(const Key('turn-cue-them')),
    );
    expect(switcher.duration, const Duration(milliseconds: 250));
    expect(
      calls.any((call) => call.method == 'HapticFeedback.vibrate'),
      isTrue,
    );

    await tester.pump(const Duration(milliseconds: 1100));
    await tester.pump(const Duration(milliseconds: 300));
    expect(find.byKey(const ValueKey('cue-them')), findsNothing);
  });

  testWidgets('disabled animations remove the Turn motion', (tester) async {
    final transport = FakeTransport();
    final session = await startedSession(tester, transport);
    await pumpConverse(tester, session, disableAnimations: true);

    transport.emit({'type': 'turn', 'who': 'them', 'auto': true});
    await tester.pump();
    await tester.pump();

    final switcher = tester.widget<AnimatedSwitcher>(
      find.byKey(const Key('turn-cue-them')),
    );
    expect(switcher.duration, Duration.zero);
    final pad = tester.widget<AnimatedContainer>(
      find.byKey(const Key('pad-surface-them')),
    );
    expect(pad.duration, Duration.zero);
  });

  testWidgets('a Session that never started offers Start, not a bare spinner', (
    tester,
  ) async {
    final transport = FakeTransport();
    final session = await connectedSession(tester, transport);
    await pumpConverse(tester, session);

    expect(find.byType(CircularProgressIndicator), findsNothing);
    expect(session.status, SessionStatus.ready);
    expect(find.byKey(const Key('session-bar')), findsOneWidget);

    await tester.tap(find.byKey(const Key('session-bar')));
    await tester.pump();
    await tester.pump();
    expect(session.status, SessionStatus.listening);
    expect(find.byKey(const Key('converse-split')), findsOneWidget);
  });

  testWidgets('Stop leaves a Start bar and Start begins a new Session', (
    tester,
  ) async {
    final transport = FakeTransport();
    final session = await startedSession(tester, transport);
    await pumpConverse(tester, session);

    await tester.tap(find.byKey(const Key('session-bar')));
    await tester.pump();
    expect(session.status, SessionStatus.stopped);
    expect(sent(transport).last, {'type': 'stop'});
    expect(find.byKey(const Key('converse-split')), findsOneWidget);
    expect(find.byKey(const Key('session-bar')), findsOneWidget);

    await tester.tap(find.byKey(const Key('session-bar')));
    await tester.pump();
    await tester.pump();
    expect(session.status, SessionStatus.listening);
    expect(
      sent(transport).where((message) => message['type'] == 'start').length,
      2,
      reason: 'the bar starts a new Session after Stop',
    );
  });

  testWidgets('a cancelled hold releases push-to-talk', (tester) async {
    final transport = FakeTransport();
    final session = await startedSession(tester, transport);
    await pumpConverse(tester, session);
    final before = transport.sentText.length;

    final gesture = await tester.startGesture(
      tester.getCenter(find.byKey(const Key('pad-them'))),
    );
    await tester.pump(const Duration(milliseconds: 600));
    expect(sent(transport).sublist(before), [
      {'type': 'turn', 'who': 'them'},
      {'type': 'ptt', 'state': 'down'},
    ]);

    await gesture.cancel();
    await tester.pump();
    final ups = sent(
      transport,
    ).where((message) => message['type'] == 'ptt' && message['state'] == 'up');
    expect(ups.length, 1, reason: 'a cancelled hold sends exactly one ptt up');
  });

  testWidgets('a TTS failure raises the subtitles-only chip', (tester) async {
    final transport = FakeTransport();
    final session = await startedSession(tester, transport);
    await pumpConverse(tester, session);

    transport.emit({
      'type': 'error',
      'provider': 'tts',
      'message': 'voice down',
    });
    await tester.pump();
    await tester.pump();

    expect(session.audioError, isNotNull);
    expect(find.byKey(const Key('audio-unavailable')), findsOneWidget);
  });

  testWidgets('provider errors and Hold show on the active half', (
    tester,
  ) async {
    final transport = FakeTransport();
    final session = await startedSession(tester, transport);
    await pumpConverse(tester, session);

    transport.emit({'type': 'error', 'provider': 'asr', 'message': 'asr down'});
    await tester.pump();
    await tester.pump();
    expect(find.textContaining('asr down'), findsOneWidget);

    transport.emit({'type': 'hold', 'held': true, 'merges': 2});
    await tester.pump();
    await tester.pump();
    expect(find.byIcon(Icons.pause_rounded), findsOneWidget);
    expect(find.textContaining('2 merges'), findsOneWidget);
  });

  testWidgets('each half follows new Segments', (tester) async {
    final transport = FakeTransport();
    final session = await startedSession(tester, transport);
    await pumpConverse(tester, session);

    for (var i = 1; i <= 8; i++) {
      transport.emit(segment(i, 'source $i', 'translation $i', 'ja', 'zh'));
      await tester.pump();
    }
    await tester.pump();

    final half = tester.getRect(find.byKey(const Key('half-me')));
    final latest = tester.getRect(find.text('translation 8'));
    expect(latest.top, greaterThanOrEqualTo(half.top));
    expect(
      latest.bottom,
      lessThanOrEqualTo(half.bottom),
      reason: 'the newest Segment stays visible in its half',
    );
  });

  testWidgets('Demo Mode keeps its banner on the Converse surface', (
    tester,
  ) async {
    final transport = FakeTransport();
    final session = await connectedSession(tester, transport, demo: true);
    await pumpConverse(tester, session, onUseKeys: () {});

    expect(find.byKey(const Key('demo-banner')), findsOneWidget);
    expect(find.byKey(const Key('demo-use-keys')), findsOneWidget);
  });

  testWidgets('goldens: split and stacked Converse', (tester) async {
    final transport = FakeTransport();
    final session = await startedSession(tester, transport, withSegments: true);

    await pumpConverse(tester, session);
    await expectLater(
      find.byType(ConverseScreen),
      matchesGoldenFile('goldens/converse_split.png'),
    );

    await pumpConverse(tester, session, textScale: 2);
    expect(find.byKey(const Key('converse-stacked')), findsOneWidget);
    await expectLater(
      find.byType(ConverseScreen),
      matchesGoldenFile('goldens/converse_stacked.png'),
    );
  });
}
