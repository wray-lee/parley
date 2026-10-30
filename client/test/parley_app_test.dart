import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';
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
}
