import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:parley/model/protocol.dart';
import 'package:parley/session/listen_session.dart';

import 'fakes.dart';

MaterialPair makeHello() => (
  pairs: [
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
  packs: [
    {'name': 'conference', 'src': 'ja', 'tgt': 'zh'},
    {'name': 'conference', 'src': 'zh', 'tgt': 'ja'},
  ],
  presets: [
    {'id': 'groq', 'trainsOnData': false},
  ],
);

typedef MaterialPair = ({
  List<Map<String, dynamic>> pairs,
  List<Map<String, dynamic>> packs,
  List<Map<String, dynamic>> presets,
});

({FakeTransport transport, ListenSession session}) make() {
  final transport = FakeTransport();
  final session = ListenSession(
    transport: transport,
    audio: FakeAudioCapture(),
    ttsPlayer: FakeTtsPlayer(),
  );
  return (transport: transport, session: session);
}

List<Map<String, dynamic>> sent(FakeTransport transport) => [
  for (final message in transport.sentText)
    jsonDecode(message) as Map<String, dynamic>,
];

void main() {
  test('hello carries packs, presets and the TTS mode', () async {
    final (:transport, :session) = make();
    addTearDown(session.dispose);
    await session.connect();
    final hello = makeHello();
    transport.emit({
      'type': 'hello',
      'pairs': hello.pairs,
      'packs': hello.packs,
      'presetsInUse': hello.presets,
      'tts': 'system',
      'demo': false,
    });
    await Future<void>.delayed(Duration.zero);

    expect(session.packs.length, 2);
    expect(session.packs.first.name, 'conference');
    expect(session.presetsInUse.single.id, 'groq');
    expect(session.tts, 'system');
  });

  test('start sends the selected Direction and Domain Packs', () async {
    final (:transport, :session) = make();
    addTearDown(session.dispose);
    await session.connect();
    final hello = makeHello();
    transport.emit({
      'type': 'hello',
      'pairs': hello.pairs,
      'packs': hello.packs,
      'presetsInUse': hello.presets,
      'demo': false,
    });
    await Future<void>.delayed(Duration.zero);

    final pair = session.pairs.single;
    session
      ..selectPair(pair)
      ..selectDirection(pair.directions[1])
      ..selectPacks({'conference'});
    await session.start();

    final start = sent(transport)
        .lastWhere((message) => message['type'] == 'start');
    expect(start['src'], 'zh');
    expect(start['tgt'], 'ja');
    expect(start['packs'], ['conference']);
  });

  test('a Provider error is kept for the Settings card', () async {
    final (:transport, :session) = make();
    addTearDown(session.dispose);
    await session.connect();
    transport.emit({
      'type': 'error',
      'provider': 'translator',
      'kind': 'auth',
      'message': 'Bad key',
      'fallback': 'Check Settings.',
    });
    await Future<void>.delayed(Duration.zero);

    expect(session.providerErrors['translator'], contains('Bad key'));
    expect(session.providerErrors['translator'], contains('Check Settings.'));
  });

  test('a one-way Direction has no reverse', () {
    const pair = LanguagePair(
      a: 'ja',
      b: 'zh',
      directions: [DirectionRef(src: 'ja', tgt: 'zh')],
      converse: false,
    );
    expect(pair.direction('ja', 'zh')?.tgt, 'zh');
    expect(pair.reverseOf(const DirectionRef(src: 'ja', tgt: 'zh')), isNull);
  });
}
