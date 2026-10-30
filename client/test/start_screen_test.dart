import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:material_ui/material_ui.dart';
import 'package:parley/l10n/parley_localizations.dart';
import 'package:parley/model/mode.dart';
import 'package:parley/model/protocol.dart';
import 'package:parley/session/listen_session.dart';
import 'package:parley/theme.dart';
import 'package:parley/ui/start_screen.dart';

import 'fakes.dart';

Map<String, dynamic> hello({
  bool converse = true,
  bool demo = false,
  List<Map<String, dynamic>> directions = const [
    {'src': 'ja', 'tgt': 'zh'},
    {'src': 'zh', 'tgt': 'ja'},
  ],
  List<Map<String, dynamic>>? pairs,
}) => {
  'type': 'hello',
  'pairs':
      pairs ??
      [
        {'a': 'ja', 'b': 'zh', 'directions': directions, 'converse': converse},
      ],
  'packs': [
    {'name': 'conference', 'src': 'ja', 'tgt': 'zh'},
    {'name': 'conference', 'src': 'zh', 'tgt': 'ja'},
  ],
  'presetsInUse': <dynamic>[],
  'demo': demo,
};

Widget wrap(Widget child) => MaterialApp(
  theme: parleyTheme(Brightness.dark),
  localizationsDelegates: parleyLocalizationDelegates,
  supportedLocales: parleySupportedLocales,
  home: child,
);

Future<({FakeTransport transport, ListenSession session})> makeSession({
  required Map<String, dynamic> helloMessage,
  List<SampleRef> samples = const [],
  Uri? serverBase,
  http.Client? httpClient,
}) async {
  final transport = FakeTransport();
  final session = ListenSession(
    transport: transport,
    audio: FakeAudioCapture(),
    ttsPlayer: FakeTtsPlayer(),
    serverBase: serverBase,
    httpClient: httpClient,
    loadSamples: () async => samples,
  );
  await session.connect();
  transport.emit(helloMessage);
  return (transport: transport, session: session);
}

List<Map<String, dynamic>> sent(FakeTransport transport) => [
  for (final message in transport.sentText)
    jsonDecode(message) as Map<String, dynamic>,
];

void main() {
  testWidgets('Start reads the pair and its Domain Pack from hello', (
    tester,
  ) async {
    final (:transport, :session) = await makeSession(helloMessage: hello());
    addTearDown(session.dispose);

    await tester.pumpWidget(
      wrap(StartScreen(session: session, onStart: () {}, onSettings: () {})),
    );
    await tester.pumpAndSettle();

    expect(find.text('日本語 ↔ 中文'), findsOneWidget);
    expect(find.byKey(const Key('pack-conference')), findsOneWidget);
    expect(find.text('They speak 日本語 → I read 中文'), findsOneWidget);
    expect(
      tester
          .widget<FilledButton>(find.byKey(const Key('start-button')))
          .onPressed,
      isNotNull,
    );
  });

  testWidgets('swap flips the Direction shown on Start', (tester) async {
    final (:transport, :session) = await makeSession(helloMessage: hello());
    addTearDown(session.dispose);

    await tester.pumpWidget(
      wrap(StartScreen(session: session, onStart: () {}, onSettings: () {})),
    );
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('swap-languages')));
    await tester.pump();

    expect(find.text('They speak 中文 → I read 日本語'), findsOneWidget);
  });

  testWidgets('Converse starts my Direction, and swap flips it', (
    tester,
  ) async {
    final (:transport, :session) = await makeSession(helloMessage: hello());
    addTearDown(session.dispose);

    await tester.pumpWidget(
      wrap(
        StartScreen(
          session: session,
          onStart: () => session.start(),
          onSettings: () {},
        ),
      ),
    );
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('mode-converse')));
    await tester.pump();
    await tester.tap(find.byKey(const Key('start-button')));
    await tester.pumpAndSettle();

    var start = sent(transport).lastWhere((m) => m['type'] == 'start');
    expect(start['mode'], 'converse');
    expect(start['src'], 'zh', reason: 'me -> them, the reverse of Listen');
    expect(start['tgt'], 'ja');
    expect(session.meDirection?.src, 'zh');

    await session.stop();
    session
      ..selectMode(SessionMode.listen)
      ..selectPair(session.pairs.first);
    await tester.pumpWidget(
      wrap(
        StartScreen(
          session: session,
          onStart: () => session.start(),
          onSettings: () {},
        ),
      ),
    );
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('mode-converse')));
    await tester.pump();
    await tester.tap(find.byKey(const Key('swap-languages')));
    await tester.pump();
    await tester.tap(find.byKey(const Key('start-button')));
    await tester.pumpAndSettle();

    start = sent(transport).lastWhere((m) => m['type'] == 'start');
    expect(start['src'], 'ja');
    expect(start['tgt'], 'zh');
  });

  testWidgets('the Demo sample is my speech in Converse', (tester) async {
    final client = MockClient(
      (request) async => http.Response(
        jsonEncode({
          'id': 'placeholder',
          'duration_ms': 100,
          'audio': [
            {'start_ms': 0, 'end_ms': 50, 'amplitude': 0.2},
          ],
          'src': 'zh',
          'tgt': 'ja',
        }),
        200,
      ),
    );
    final (:transport, :session) = await makeSession(
      helloMessage: hello(demo: true),
      serverBase: Uri.parse('http://localhost:3000'),
      httpClient: client,
      samples: const [SampleRef(id: 'placeholder', src: 'zh', tgt: 'ja')],
    );
    addTearDown(session.dispose);

    await tester.pumpWidget(
      wrap(
        StartScreen(
          session: session,
          onStart: () => session.start(),
          onSettings: () {},
        ),
      ),
    );
    await tester.pumpAndSettle();
    // Listen first orients the sample as their speech (swapped); Converse
    // re-orients it as my speech, so Start must send zh -> ja.
    await tester.tap(find.byKey(const Key('mode-converse')));
    await tester.pump();
    await tester.tap(find.byKey(const Key('start-button')));
    await tester.pumpAndSettle();

    final start = sent(transport).lastWhere((m) => m['type'] == 'start');
    expect(start['mode'], 'converse');
    expect(start['src'], 'zh');
    expect(start['tgt'], 'ja');
    expect(
      sent(transport).lastWhere((m) => m['type'] == 'sample')['name'],
      'placeholder',
    );
  });

  testWidgets('a one-way pair is disabled in Converse with a reason', (
    tester,
  ) async {
    final (:transport, :session) = await makeSession(
      helloMessage: hello(
        converse: false,
        directions: const [
          {'src': 'ja', 'tgt': 'zh'},
        ],
      ),
    );
    addTearDown(session.dispose);

    await tester.pumpWidget(
      wrap(StartScreen(session: session, onStart: () {}, onSettings: () {})),
    );
    await tester.tap(find.byKey(const Key('mode-converse')));
    await tester.pump();

    final chip = tester.widget<ChoiceChip>(find.byKey(const Key('pair-0')));
    expect(chip.onSelected, isNull);
    expect(find.byKey(const Key('listen-only-reason')), findsOneWidget);
    expect(
      tester
          .widget<FilledButton>(find.byKey(const Key('start-button')))
          .onPressed,
      isNull,
    );
  });

  testWidgets('a one-way pair never offers a swap that would strand it', (
    tester,
  ) async {
    final (:transport, :session) = await makeSession(
      helloMessage: hello(
        converse: false,
        directions: const [
          {'src': 'ja', 'tgt': 'zh'},
        ],
      ),
    );
    addTearDown(session.dispose);

    await tester.pumpWidget(
      wrap(StartScreen(session: session, onStart: () {}, onSettings: () {})),
    );
    await tester.pump();

    final swap = tester.widget<IconButton>(
      find.byKey(const Key('swap-languages')),
    );
    expect(swap.onPressed, isNull);
    expect(swap.tooltip, 'One-way pair — Listen only.');
    // Listen still works in the forward Direction.
    expect(
      tester
          .widget<FilledButton>(find.byKey(const Key('start-button')))
          .onPressed,
      isNotNull,
    );
  });

  testWidgets('every disabled one-way chip carries the Converse reason', (
    tester,
  ) async {
    final (:transport, :session) = await makeSession(
      helloMessage: hello(
        pairs: [
          {
            'a': 'zh',
            'b': 'ja',
            'directions': [
              {'src': 'zh', 'tgt': 'ja'},
              {'src': 'ja', 'tgt': 'zh'},
            ],
            'converse': true,
          },
          {
            'a': 'ja',
            'b': 'zh',
            'directions': [
              {'src': 'ja', 'tgt': 'zh'},
            ],
            'converse': false,
          },
        ],
      ),
    );
    addTearDown(session.dispose);

    await tester.pumpWidget(
      wrap(StartScreen(session: session, onStart: () {}, onSettings: () {})),
    );
    await tester.tap(find.byKey(const Key('mode-converse')));
    await tester.pump();

    final chip = tester.widget<ChoiceChip>(find.byKey(const Key('pair-1')));
    expect(chip.onSelected, isNull);
    expect(
      find.byTooltip(
        'This Language Pair translates one way only, so Converse is '
        'unavailable. Start Listen instead.',
      ),
      findsOneWidget,
    );
  });

  testWidgets('without a key editor the Demo banner hides its CTA', (
    tester,
  ) async {
    final (:transport, :session) = await makeSession(
      helloMessage: hello(demo: true),
    );
    addTearDown(session.dispose);

    await tester.pumpWidget(
      wrap(StartScreen(session: session, onStart: () {}, onSettings: () {})),
    );
    await tester.pumpAndSettle();

    expect(find.byKey(const Key('demo-banner')), findsOneWidget);
    expect(find.text('Use my keys'), findsNothing);
    expect(find.byKey(const Key('open-settings')), findsOneWidget);
  });

  testWidgets(
    'Demo Mode keeps the banner, explains the mic and lists samples',
    (tester) async {
      var settingsOpened = 0;
      final (:transport, :session) = await makeSession(
        helloMessage: hello(demo: true),
        samples: const [
          SampleRef(id: 'zh-numbers', src: 'zh', tgt: 'ja'),
          SampleRef(id: 'placeholder', src: 'zh', tgt: 'ja'),
        ],
      );
      addTearDown(session.dispose);

      await tester.pumpWidget(
        wrap(
          StartScreen(
            session: session,
            onStart: () {},
            onSettings: () {},
            onUseKeys: () => settingsOpened++,
          ),
        ),
      );
      await tester.pumpAndSettle();

      expect(find.byKey(const Key('demo-banner')), findsOneWidget);
      expect(find.byKey(const Key('mic-off-demo')), findsOneWidget);
      expect(find.byKey(const Key('sample-placeholder')), findsOneWidget);
      expect(find.byKey(const Key('sample-zh-numbers')), findsOneWidget);
      // The default Direction follows the server's bundled placeholder sample.
      expect(find.text('They speak 中文 → I read 日本語'), findsOneWidget);

      await tester.tap(find.text('Use my keys'));
      expect(settingsOpened, 1);
    },
  );

  testWidgets('Start sends the chosen Direction, Domain Packs and sample', (
    tester,
  ) async {
    final client = MockClient((request) async {
      expect(request.url.path, '/samples/zh-numbers.json');
      return http.Response(
        jsonEncode({
          'id': 'zh-numbers',
          'duration_ms': 100,
          'audio': [
            {'start_ms': 0, 'end_ms': 50, 'amplitude': 0.2},
          ],
          'src': 'zh',
          'tgt': 'ja',
        }),
        200,
      );
    });
    final (:transport, :session) = await makeSession(
      helloMessage: hello(demo: true),
      serverBase: Uri.parse('http://localhost:3000'),
      httpClient: client,
      samples: const [
        SampleRef(id: 'placeholder', src: 'zh', tgt: 'ja'),
        SampleRef(id: 'zh-numbers', src: 'zh', tgt: 'ja'),
      ],
    );
    addTearDown(session.dispose);

    await tester.pumpWidget(
      wrap(
        StartScreen(
          session: session,
          onStart: () => session.start(),
          onSettings: () {},
        ),
      ),
    );
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('pack-conference')));
    await tester.tap(find.byKey(const Key('sample-zh-numbers')));
    await tester.pump();
    await tester.tap(find.byKey(const Key('start-button')));
    await tester.pumpAndSettle();

    final start = sent(transport).lastWhere((m) => m['type'] == 'start');
    expect(start['src'], 'zh');
    expect(start['tgt'], 'ja');
    expect(start['packs'], ['conference']);
    expect(
      sent(transport).lastWhere((m) => m['type'] == 'sample')['name'],
      'zh-numbers',
    );
  });

  testWidgets('settings are exposed from Start', (tester) async {
    var opened = 0;
    final (:transport, :session) = await makeSession(helloMessage: hello());
    addTearDown(session.dispose);

    await tester.pumpWidget(
      wrap(
        StartScreen(
          session: session,
          onStart: () {},
          onSettings: () => opened++,
        ),
      ),
    );
    await tester.tap(find.byKey(const Key('open-settings')));
    expect(opened, 1);
  });
}
