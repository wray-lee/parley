import 'dart:convert';
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:parley/model/mode.dart';
import 'package:parley/session/listen_session.dart';

import 'fakes.dart';

Map<String, dynamic> hello({bool converse = true}) => {
  'type': 'hello',
  'pairs': [
    {
      'a': 'ja',
      'b': 'zh',
      'directions': [
        {'src': 'ja', 'tgt': 'zh'},
        if (converse) {'src': 'zh', 'tgt': 'ja'},
      ],
      'converse': converse,
    },
  ],
  'packs': <dynamic>[],
  'presetsInUse': <dynamic>[],
  'demo': false,
};

ListenSession sessionFor(FakeTransport transport) {
  final session = ListenSession(
    transport: transport,
    audio: FakeAudioCapture(),
    ttsPlayer: FakeTtsPlayer(),
  );
  addTearDown(session.dispose);
  return session;
}

Future<ListenSession> startedConverse(FakeTransport transport) async {
  final session = sessionFor(transport);
  await session.connect();
  transport.emit(hello());
  await pumpEventQueue();
  final pair = session.pairs.first;
  session
    ..selectMode(SessionMode.converse)
    ..selectPair(pair)
    ..selectDirection(pair.direction('zh', 'ja'));
  await session.start();
  transport.emit({'type': 'turn', 'who': 'me', 'auto': false});
  await pumpEventQueue();
  return session;
}

List<Map<String, dynamic>> sent(FakeTransport transport) => transport.sentText
    .map((text) => jsonDecode(text) as Map<String, dynamic>)
    .toList(growable: false);

void main() {
  test('Converse starts with me mode and my Direction', () async {
    final transport = FakeTransport();
    final session = sessionFor(transport);
    await session.connect();
    transport.emit(hello());
    await pumpEventQueue();
    final pair = session.pairs.first;
    session
      ..selectMode(SessionMode.converse)
      ..selectPair(pair)
      ..selectDirection(pair.direction('zh', 'ja'));

    await session.start();

    final start = sent(transport)
        .firstWhere((message) => message['type'] == 'start');
    expect(start['mode'], 'converse');
    expect(start['src'], 'zh');
    expect(start['tgt'], 'ja');
    expect(session.meDirection?.src, 'zh');
    expect(session.themDirection?.src, 'ja');
    expect(session.isConverse, isTrue);
  });

  test('Converse refuses a one-way pair without sending start', () async {
    final transport = FakeTransport();
    final session = sessionFor(transport);
    await session.connect();
    transport.emit(hello(converse: false));
    await pumpEventQueue();
    final pair = session.pairs.first;
    session
      ..selectMode(SessionMode.converse)
      ..selectPair(pair)
      ..selectDirection(pair.direction('ja', 'zh'));

    await session.start();

    expect(session.error, contains('one way only'));
    expect(
      sent(transport).where((message) => message['type'] == 'start'),
      isEmpty,
    );
  });

  test('a Tap takes the Turn and the server turn event updates it', () async {
    final transport = FakeTransport();
    final session = await startedConverse(transport);

    session.takeTurn('them');
    expect(sent(transport).last, {'type': 'turn', 'who': 'them'});

    transport.emit({'type': 'turn', 'who': 'them', 'auto': false});
    await pumpEventQueue();
    expect(session.turn, 'them');
    expect(session.turnAuto, isFalse);
    expect(session.turnCue, 1);
    expect(session.turnCueWho, 'them');
  });

  test('push-to-talk sends the Turn before ptt down, then ptt up', () async {
    final transport = FakeTransport();
    final session = await startedConverse(transport);
    final before = transport.sentText.length;

    session.beginPushToTalk('them');
    session.endPushToTalk();

    final messages = sent(transport).sublist(before);
    expect(messages, [
      {'type': 'turn', 'who': 'them'},
      {'type': 'ptt', 'state': 'down'},
      {'type': 'ptt', 'state': 'up'},
    ]);
  });

  test('the auto-switch after my speech is marked auto and cues', () async {
    final transport = FakeTransport();
    final session = await startedConverse(transport);

    transport.emit({'type': 'turn', 'who': 'them', 'auto': true});
    await pumpEventQueue();

    expect(session.turn, 'them');
    expect(session.turnAuto, isTrue);
    expect(session.turnCue, 1);
  });

  test('Gating follows my playback in Converse and its 500 ms tail', () async {
    final transport = FakeTransport();
    final session = await startedConverse(transport);

    transport.emit({'type': 'tts_begin', 'seg_id': 1, 'format': 'wav'});
    transport.emitBinary(Uint8List.fromList([1, 2, 3, 4]));
    transport.emit({'type': 'tts_end', 'seg_id': 1});
    await Future<void>.delayed(const Duration(milliseconds: 10));
    expect(session.gated, isTrue, reason: 'my speech the app is playing');

    await Future<void>.delayed(const Duration(milliseconds: 600));
    expect(session.gated, isFalse, reason: 'the tail released the mic');
  });

  test('Stop then Start clears the previous Session live state', () async {
    final transport = FakeTransport();
    final session = await startedConverse(transport);
    transport.emit({'type': 'asr_partial', 'text': 'stale partial'});
    transport.emit({'type': 'hold', 'held': true, 'merges': 4});
    transport.emit({'type': 'error', 'provider': 'asr', 'message': 'asr down'});
    await pumpEventQueue();
    expect(session.live.text, 'stale partial');
    expect(session.held, isTrue);
    expect(session.merges, 4);

    await session.stop();
    await session.start();
    transport.emit({'type': 'turn', 'who': 'me', 'auto': false});
    await pumpEventQueue();

    expect(session.live.isEmpty, isTrue);
    expect(session.held, isFalse);
    expect(session.merges, 0);
    expect(session.inlineError, isNull);

    // The next Partial starts its Stable Prefix from scratch, not from the
    // previous Session's hypothesis.
    transport.emit({'type': 'asr_partial', 'text': 'stale partial again'});
    await pumpEventQueue();
    expect(session.live.stable, isEmpty);
  });

  test('Listen never reports Gating', () async {
    final transport = FakeTransport();
    final session = sessionFor(transport);
    await session.connect();
    transport.emit(hello());
    await pumpEventQueue();
    final pair = session.pairs.first;
    session
      ..selectPair(pair)
      ..selectDirection(pair.direction('ja', 'zh'));
    await session.start();

    transport.emit({'type': 'tts_begin', 'seg_id': 1, 'format': 'wav'});
    transport.emitBinary(Uint8List.fromList([1, 2, 3, 4]));
    transport.emit({'type': 'tts_end', 'seg_id': 1});
    await Future<void>.delayed(const Duration(milliseconds: 20));

    expect(session.gated, isFalse);
  });
}
