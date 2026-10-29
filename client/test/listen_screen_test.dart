import 'package:material_ui/material_ui.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:parley/model/protocol.dart';
import 'package:parley/session/listen_session.dart';
import 'package:parley/theme.dart';
import 'package:parley/ui/listen_screen.dart';
import 'package:web_socket_channel/web_socket_channel.dart';

import 'fakes.dart';

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
}
