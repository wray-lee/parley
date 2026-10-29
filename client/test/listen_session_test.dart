import 'dart:convert';

import 'package:flutter/foundation.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:parley/model/protocol.dart';
import 'package:parley/session/listen_session.dart';
import 'package:web_socket_channel/web_socket_channel.dart';

import 'fakes.dart';

DemoClip testClip() => DemoClip.fromJson({
  'id': 'placeholder',
  'duration_ms': 100,
  'audio': [
    {'start_ms': 0, 'end_ms': 50, 'amplitude': 0.2},
  ],
  'src': 'zh',
  'tgt': 'ja',
});

Map<String, dynamic> helloMessage({String? tts}) => {
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
  'tts': ?tts,
};

Map<String, dynamic> segment(
  int segId,
  String translation, {
  bool speak = true,
}) => {
  'type': 'segment',
  'seg_id': segId,
  'src': 'source $segId',
  'tgt': translation,
  'direction': {'src': 'zh', 'tgt': 'ja'},
  'latency_ms': 10,
  'speak': speak,
  'demo': true,
};

/// Captures everything the session debugPrints for the rest of the test.
List<String> captureDebugPrint() {
  final logs = <String>[];
  final original = debugPrint;
  debugPrint = (String? message, {int? wrapWidth}) {
    if (message != null) logs.add(message);
  };
  addTearDown(() => debugPrint = original);
  return logs;
}

({
  FakeTransport transport,
  FakeAudioCapture audio,
  FakeTtsPlayer tts,
  FakeSystemTtsEngine system,
  ListenSession session,
})
makeSession() {
  final transport = FakeTransport();
  final audio = FakeAudioCapture();
  final tts = FakeTtsPlayer();
  final system = FakeSystemTtsEngine();
  final session = ListenSession(
    transport: transport,
    audio: audio,
    ttsPlayer: tts,
    systemTtsEngine: system,
    loadDemoClip: () async => testClip(),
  );
  return (
    transport: transport,
    audio: audio,
    tts: tts,
    system: system,
    session: session,
  );
}

List<String> playbackOf(FakeTransport transport) => transport.sentText
    .map((message) => jsonDecode(message) as Map<String, dynamic>)
    .where((message) => message['type'] == 'playback')
    .map((message) => '${message['seg_id']}:${message['state']}')
    .toList();

void main() {
  test(
    'hello fills pairs; demo Start sends start + sample and streams frames',
    () async {
      final s = makeSession();
      await s.session.connect();
      s.transport.emit(helloMessage());
      await pumpEventQueue();

      expect(s.session.demo, isTrue);
      expect(s.session.status, SessionStatus.ready);
      expect(s.session.pairs.single.label, 'ja ↔ zh');

      await s.session.start();
      expect(jsonDecode(s.transport.sentText[0]), {
        'type': 'start',
        'mode': 'listen',
        'src': 'zh',
        'tgt': 'ja',
        'packs': <dynamic>[],
      });
      expect(jsonDecode(s.transport.sentText[1]), {
        'type': 'sample',
        'name': 'placeholder',
      });
      expect(s.audio.demo?.id, 'placeholder');

      s.audio.frame();
      expect(s.transport.sentBinary.single.length, 640);
    },
  );

  test('segments render in seg_id order and are never revised', () async {
    final s = makeSession();
    await s.session.connect();
    s.transport.emit(helloMessage());
    s.transport.emit(segment(2, 'second'));
    s.transport.emit(segment(1, 'first'));
    s.transport.emit(segment(1, 'revised'));
    await pumpEventQueue();

    expect(s.session.segments.map((segment) => segment.translation).toList(), [
      'first',
      'second',
    ]);
  });

  test(
    'TTS chunks play in seg_id order even when later audio arrives first',
    () async {
      final s = makeSession();
      await s.session.connect();
      s.transport.emit(helloMessage());
      s.transport.emit(segment(1, 'first'));
      s.transport.emit(segment(2, 'second'));

      s.transport.emit({'type': 'tts_begin', 'seg_id': 2, 'format': 'wav'});
      s.transport.emitBinary(Uint8List.fromList([2]));
      s.transport.emit({'type': 'tts_end', 'seg_id': 2});
      await pumpEventQueue();
      expect(s.tts.playedData, isEmpty);

      s.transport.emit({'type': 'tts_begin', 'seg_id': 1, 'format': 'wav'});
      s.transport.emitBinary(Uint8List.fromList([1]));
      s.transport.emit({'type': 'tts_end', 'seg_id': 1});
      await pumpEventQueue();

      expect(s.tts.playedData.map((data) => data.single).toList(), [1, 2]);
      final playback = s.transport.sentText
          .map((raw) => jsonDecode(raw) as Map<String, dynamic>)
          .where((message) => message['type'] == 'playback')
          .map((message) => '${message['seg_id']}:${message['state']}')
          .toList();
      expect(playback, ['1:start', '1:end', '2:start', '2:end']);
    },
  );

  test('each TTS chunk of a Segment plays as its own buffer', () async {
    final s = makeSession();
    await s.session.connect();
    s.transport.emit(helloMessage());
    s.transport.emit({'type': 'tts_begin', 'seg_id': 1, 'format': 'mp3'});
    s.transport.emitBinary(Uint8List.fromList([1]));
    s.transport.emitBinary(Uint8List.fromList([2]));
    s.transport.emit({'type': 'tts_end', 'seg_id': 1});
    await pumpEventQueue();

    expect(s.tts.playedData.map((data) => data.toList()).toList(), [
      [1],
      [2],
    ]);
    expect(s.tts.playedFormats, ['mp3', 'mp3']);
  });

  test('hello.tts=system speaks each spoken Segment itself', () async {
    final s = makeSession();
    await s.session.connect();
    s.transport.emit(helloMessage(tts: 'system'));
    await pumpEventQueue();
    await s.session.start();
    expect(s.session.tts, 'system');

    s.transport.emit(segment(1, 'first'));
    s.transport.emit(segment(2, 'silent', speak: false));
    s.transport.emit(segment(3, 'third'));
    await pumpEventQueue();

    expect(s.system.spoken, ['first']);
    expect(s.system.languages, ['ja']);
    expect(playbackOf(s.transport), ['1:start']);

    s.system.complete(0);
    await pumpEventQueue();
    expect(s.system.spoken, ['first', 'third']);
    expect(playbackOf(s.transport), ['1:start', '1:end', '3:start']);

    s.system.complete(1);
    await pumpEventQueue();
    expect(playbackOf(s.transport), ['1:start', '1:end', '3:start', '3:end']);
    expect(s.session.segments.map((segment) => segment.translation).toList(), [
      'first',
      'silent',
      'third',
    ]);
  });

  test('an audio TTS Provider never touches the system engine', () async {
    final s = makeSession();
    await s.session.connect();
    s.transport.emit(helloMessage(tts: 'audio'));
    await pumpEventQueue();
    await s.session.start();
    s.transport.emit(segment(1, 'first'));
    await pumpEventQueue();

    expect(s.session.tts, 'audio');
    expect(s.system.spoken, isEmpty);
    expect(playbackOf(s.transport), isEmpty);
  });

  test('a system TTS error keeps the transcript and the chip stays', () async {
    final s = makeSession();
    await s.session.connect();
    s.transport.emit(helloMessage(tts: 'system'));
    await pumpEventQueue();
    await s.session.start();
    s.transport.emit(segment(1, 'first'));
    await pumpEventQueue();

    s.system.fail(StateError('engine offline'));
    await pumpEventQueue();
    expect(s.session.audioError, contains('engine offline'));
    expect(s.session.providerErrors['tts'], contains('engine offline'));
    expect(s.session.segments.single.translation, 'first');

    s.transport.emit(segment(2, 'second'));
    await pumpEventQueue();
    expect(s.session.audioError, isNotNull);
    expect(s.system.spoken, ['first', 'second']);
  });

  test('Stop cancels system speech and never reports its end twice', () async {
    final s = makeSession();
    await s.session.connect();
    s.transport.emit(helloMessage(tts: 'system'));
    await pumpEventQueue();
    await s.session.start();
    s.transport.emit(segment(1, 'first'));
    await pumpEventQueue();

    await s.session.stop();
    expect(s.system.stops, greaterThanOrEqualTo(1));
    expect(playbackOf(s.transport), ['1:start', '1:end']);

    s.system.complete(0);
    await pumpEventQueue();
    expect(playbackOf(s.transport), ['1:start', '1:end']);
    expect(s.system.spoken, ['first']);
  });

  test('a server TTS error raises the subtitles-only state', () async {
    final s = makeSession();
    await s.session.connect();
    s.transport.emit(helloMessage());
    s.transport.emit({
      'type': 'error',
      'provider': 'tts',
      'kind': 'protocol',
      'message': 'voice missing',
      'fallback': 'subtitles only',
    });
    await pumpEventQueue();

    expect(s.session.audioError, contains('voice missing'));
    expect(s.session.providerErrors['tts'], contains('voice missing'));
  });

  test('Stop then Start begins a new Session: seg_id restarts at 1', () async {
    final s = makeSession();
    await s.session.connect();
    s.transport.emit(helloMessage());
    await s.session.start();

    // Session 1 reaches seg 1, then is cut off mid-seg 2.
    s.transport.emit(segment(1, 'first session'));
    s.transport.emit({'type': 'tts_begin', 'seg_id': 1, 'format': 'wav'});
    s.transport.emitBinary(Uint8List.fromList([1]));
    s.transport.emit({'type': 'tts_end', 'seg_id': 1});
    s.transport.emit({'type': 'tts_begin', 'seg_id': 2, 'format': 'wav'});
    s.transport.emitBinary(Uint8List.fromList([2]));
    await pumpEventQueue();
    expect(s.session.segments.single.translation, 'first session');
    expect(s.tts.playedData.map((data) => data.toList()).toList(), [
      [1],
    ]);

    await s.session.stop();
    await s.session.start();

    // The server numbers a new Session from 1 again.
    s.transport.emit(segment(1, 'second session'));
    s.transport.emit({'type': 'tts_begin', 'seg_id': 1, 'format': 'wav'});
    s.transport.emitBinary(Uint8List.fromList([9]));
    s.transport.emit({'type': 'tts_end', 'seg_id': 1});
    await pumpEventQueue();

    expect(s.session.segments.map((segment) => segment.translation).toList(), [
      'second session',
    ]);
    expect(s.tts.playedData.map((data) => data.toList()).toList(), [
      [1],
      [9],
    ]);
  });

  test(
    'a Session cut off mid-clip does not reuse that clip after restart',
    () async {
      final s = makeSession();
      await s.session.connect();
      s.transport.emit(helloMessage());
      await s.session.start();
      s.transport.emit({'type': 'tts_begin', 'seg_id': 1, 'format': 'wav'});
      s.transport.emitBinary(Uint8List.fromList([1]));
      await pumpEventQueue();

      await s.session.stop();
      await s.session.start();
      s.transport.emit({'type': 'tts_begin', 'seg_id': 1, 'format': 'wav'});
      s.transport.emitBinary(Uint8List.fromList([9]));
      s.transport.emit({'type': 'tts_end', 'seg_id': 1});
      await pumpEventQueue();

      expect(s.tts.playedData.map((data) => data.toList()).toList(), [
        [9],
      ]);
    },
  );

  test('a playback failure leaves committed captions visible', () async {
    final s = makeSession();
    s.tts.error = StateError('no audio device');
    await s.session.connect();
    s.transport.emit(helloMessage());
    s.transport.emit(segment(1, 'first'));
    s.transport.emit({'type': 'tts_begin', 'seg_id': 1, 'format': 'wav'});
    s.transport.emitBinary(Uint8List.fromList([1]));
    s.transport.emit({'type': 'tts_end', 'seg_id': 1});
    await pumpEventQueue();

    expect(s.session.segments.single.translation, 'first');
    expect(s.session.audioError, contains('Audio playback failed'));
  });

  test('a connect failure surfaces a generic, token-free error', () async {
    final logs = captureDebugPrint();
    final s = makeSession();
    // What web_socket_channel 3.0.3's IO backend throws on a rejected
    // handshake: the message carries the full URI, token included.
    s.transport.connectError = WebSocketChannelException.from(
      "WebSocketException: Connection to "
      "'http://127.0.0.1:3999/ws?token=SECRET123#' was not upgraded to "
      'websocket',
    );
    await s.session.connect();

    expect(s.session.status, SessionStatus.failed);
    expect(s.session.error, 'Connection failed.');
    expect(logs.join('\n'), isNot(contains('SECRET123')));
    expect(logs.join('\n'), isNot(contains('ws?token=')));
  });

  test('a lost connection surfaces a generic, token-free error', () async {
    final logs = captureDebugPrint();
    final s = makeSession();
    await s.session.connect();
    s.transport.emit(helloMessage());
    s.transport.addError(
      WebSocketChannelException.from(
        "WebSocketException: Connection to "
        "'http://127.0.0.1:3999/ws?token=SECRET123#' was lost",
      ),
    );
    await pumpEventQueue();

    expect(s.session.status, SessionStatus.failed);
    expect(s.session.error, 'Connection lost.');
    expect(logs.join('\n'), isNot(contains('SECRET123')));
    expect(logs.join('\n'), isNot(contains('ws?token=')));
  });

  test('a server error is shown with its fallback', () async {
    final s = makeSession();
    await s.session.connect();
    s.transport.emit({
      'type': 'error',
      'provider': 'asr',
      'kind': 'protocol',
      'message': 'Mock ASR only accepts bundled samples.',
      'fallback': 'Play the bundled sample.',
    });
    await pumpEventQueue();

    expect(s.session.error, contains('Mock ASR only accepts bundled samples.'));
    expect(s.session.error, contains('Play the bundled sample.'));
  });
}
