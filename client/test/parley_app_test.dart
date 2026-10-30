import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';
import 'package:parley/model/protocol.dart';
import 'package:parley/platform/mic_permission_api.dart';
import 'package:parley/session/listen_session.dart';
import 'package:parley/ui/converse_screen.dart';
import 'package:parley/ui/parley_app.dart';

import 'fakes.dart';

Map<String, dynamic> converseHello() => {
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
  'demo': false,
};

void main() {
  testWidgets(
    'read-only web keeps Settings reachable but drops the Demo key CTA',
    (tester) async {
      final transport = FakeTransport();
      final session = ListenSession(
        transport: transport,
        audio: FakeAudioCapture(),
        ttsPlayer: FakeTtsPlayer(),
        loadSamples: () async => const [],
      );

      await tester.pumpWidget(
        ParleyApp(createSession: () => session, readOnlySettings: true),
      );
      await tester.pump();
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
        'packs': <dynamic>[],
        'presetsInUse': [
          {'id': 'gemini', 'trainsOnData': true},
        ],
        'demo': true,
      });
      await tester.pumpAndSettle();

      expect(find.byKey(const Key('demo-banner')), findsOneWidget);
      expect(find.text('Use my keys'), findsNothing);
      expect(find.byKey(const Key('open-settings')), findsOneWidget);

      await tester.tap(find.byKey(const Key('open-settings')));
      await tester.pumpAndSettle();
      expect(find.byKey(const Key('settings-read-only')), findsOneWidget);
    },
  );

  testWidgets('choosing Converse starts it and shows the Converse surface', (
    tester,
  ) async {
    final transport = FakeTransport();
    final session = ListenSession(
      transport: transport,
      audio: FakeAudioCapture(),
      ttsPlayer: FakeTtsPlayer(),
    );

    await tester.pumpWidget(
      ParleyApp(createSession: () => session, readOnlySettings: true),
    );
    await tester.pump();
    transport.emit(converseHello());
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('mode-converse')));
    await tester.pump();
    await tester.tap(find.text('Start Converse'));
    await tester.pump();
    await tester.pump();

    expect(find.byType(ConverseScreen), findsOneWidget);
    final start = transport.sentText
        .map((text) => jsonDecode(text) as Map<String, dynamic>)
        .firstWhere((message) => message['type'] == 'start');
    expect(start['mode'], 'converse');
    expect(start['src'], 'zh');
    expect(start['tgt'], 'ja');
  });

  testWidgets('a denied microphone surfaces a recovery card after Start', (
    tester,
  ) async {
    final transport = FakeTransport();
    final mic = FakeMicPermission(MicAccess.denied);
    final session = ListenSession(
      transport: transport,
      audio: FakeAudioCapture(),
      ttsPlayer: FakeTtsPlayer(),
      micPermission: mic,
    );

    await tester.pumpWidget(
      ParleyApp(createSession: () => session, readOnlySettings: true),
    );
    await tester.pump();
    transport.emit(converseHello());
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('start-button')));
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('mic-recovery-card')), findsOneWidget);
    await tester.tap(find.byKey(const Key('mic-recovery-settings')));
    expect(mic.settingsOpened, 1);
  });

  testWidgets('a fresh install finishes onboarding on the Demo and starts', (
    tester,
  ) async {
    final transport = FakeTransport();
    final firstRun = FakeFirstRunStore();
    final audio = FakeAudioCapture();
    final session = ListenSession(
      transport: transport,
      audio: audio,
      ttsPlayer: FakeTtsPlayer(),
      loadDemoClip: () async => DemoClip.fromJson({
        'id': 'placeholder',
        'duration_ms': 20,
        'audio': <dynamic>[],
        'src': 'ja',
        'tgt': 'zh',
      }),
    );

    await tester.pumpWidget(
      ParleyApp(
        createSession: () => session,
        readOnlySettings: true,
        firstRun: firstRun,
        showOnboarding: true,
      ),
    );
    await tester.pump();
    transport.emit({...converseHello(), 'demo': true});
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('onboarding-pair-0')), findsOneWidget);
    await tester.tap(find.byKey(const Key('onboarding-next')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('onboarding-next')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('onboarding-finish')));
    await tester.pumpAndSettle();

    expect(firstRun.values, contains('onboarding'));
    expect(audio.started, isTrue);
    expect(find.byKey(const Key('demo-banner')), findsOneWidget);
  });

  testWidgets('leaving the foreground ends the running Session', (
    tester,
  ) async {
    final transport = FakeTransport();
    final firstRun = FakeFirstRunStore();
    final audio = FakeAudioCapture();
    final session = ListenSession(
      transport: transport,
      audio: audio,
      ttsPlayer: FakeTtsPlayer(),
      loadDemoClip: () async => DemoClip.fromJson({
        'id': 'placeholder',
        'duration_ms': 20,
        'audio': <dynamic>[],
        'src': 'ja',
        'tgt': 'zh',
      }),
    );

    await tester.pumpWidget(
      ParleyApp(
        createSession: () => session,
        readOnlySettings: true,
        firstRun: firstRun,
        showOnboarding: true,
      ),
    );
    await tester.pump();
    transport.emit({...converseHello(), 'demo': true});
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('onboarding-next')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('onboarding-next')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('onboarding-finish')));
    await tester.pumpAndSettle();
    expect(audio.started, isTrue);

    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.paused);
    await tester.pump();

    expect(audio.started, isFalse);
    expect(session.status, SessionStatus.stopped);
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
  });
}
