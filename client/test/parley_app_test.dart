import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';
import 'package:parley/session/listen_session.dart';
import 'package:parley/ui/parley_app.dart';

import 'fakes.dart';

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
}
