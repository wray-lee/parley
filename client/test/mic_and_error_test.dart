import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:parley/model/protocol.dart';
import 'package:parley/platform/mic_permission_api.dart';
import 'package:parley/session/listen_session.dart';

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
  'tts': 'audio',
};

Map<String, dynamic> segment(int segId) => {
  'type': 'segment',
  'seg_id': segId,
  'src': 'source $segId',
  'tgt': 'translation $segId',
  'direction': {'src': 'ja', 'tgt': 'zh'},
  'latency_ms': 10,
  'speak': false,
  'demo': false,
};

Map<String, dynamic> error({
  String provider = 'translator',
  String kind = 'auth',
}) => {
  'type': 'error',
  'provider': provider,
  'kind': kind,
  'message': 'Provider said no',
  'fallback': 'Do the kind-specific thing',
};

({
  FakeTransport transport,
  FakeAudioCapture audio,
  FakeMicPermission mic,
  ListenSession session,
})
make({MicAccess access = MicAccess.granted, bool demo = false}) {
  final transport = FakeTransport();
  final audio = FakeAudioCapture();
  final mic = FakeMicPermission(access);
  final session = ListenSession(
    transport: transport,
    audio: audio,
    ttsPlayer: FakeTtsPlayer(),
    micPermission: mic,
    loadDemoClip: () async => DemoClip.fromJson({
      'id': 'placeholder',
      'duration_ms': 40,
      'audio': <dynamic>[],
      'src': 'ja',
      'tgt': 'zh',
    }),
  );
  return (transport: transport, audio: audio, mic: mic, session: session);
}

Future<
  ({
    FakeTransport transport,
    FakeAudioCapture audio,
    FakeMicPermission mic,
    ListenSession session,
  })
>
liveSession({MicAccess access = MicAccess.granted}) async {
  final s = make(access: access);
  await s.session.connect();
  s.transport.emit(hello());
  await pumpEventQueue();
  return s;
}

List<Map<String, dynamic>> sent(FakeTransport transport) => transport.sentText
    .map((m) => jsonDecode(m) as Map<String, dynamic>)
    .toList();

void main() {
  test(
    'the microphone is asked for at Start, not at launch or hello',
    () async {
      final s = make();
      await s.session.connect();
      s.transport.emit(hello());
      await pumpEventQueue();
      expect(s.mic.requests, 0);

      await s.session.start();
      expect(s.mic.requests, 1);
      expect(s.session.micRecovery, isNull);
      expect(s.audio.started, isTrue);
      expect(sent(s.transport).first['type'], 'start');
    },
  );

  test('Demo Mode starts without asking for the microphone', () async {
    final s = make(demo: true);
    await s.session.connect();
    s.transport.emit(hello(demo: true));
    await pumpEventQueue();

    await s.session.start();
    expect(s.mic.requests, 0);
    expect(s.audio.started, isTrue);
    expect(s.audio.demo, isNotNull);
  });

  test(
    'a denied microphone keeps the Session ready and offers recovery',
    () async {
      final s = await liveSession(access: MicAccess.denied);

      await s.session.start();
      expect(s.session.micRecovery, MicAccess.denied);
      expect(s.session.status, SessionStatus.ready);
      expect(s.audio.started, isFalse);
      expect(sent(s.transport).where((m) => m['type'] == 'start'), isEmpty);

      await s.session.openMicSettings();
      expect(s.mic.settingsOpened, 1);
    },
  );

  test('an insecure page reports HTTPS, not a bare denial', () async {
    final s = await liveSession(access: MicAccess.insecure);

    await s.session.start();
    expect(s.session.micRecovery, MicAccess.insecure);
    expect(s.audio.started, isFalse);
  });

  test('a later granted Start clears the recovery card', () async {
    final s = await liveSession(access: MicAccess.denied);
    await s.session.start();
    expect(s.session.micRecovery, MicAccess.denied);

    s.mic.access = MicAccess.granted;
    await s.session.start();
    expect(s.session.micRecovery, isNull);
    expect(s.session.status, SessionStatus.listening);
  });

  test('every error kind is kept with the committed text', () async {
    for (final kind in [
      'auth',
      'quota',
      'network',
      'model',
      'cors',
      'timeout',
    ]) {
      final s = await liveSession();
      await s.session.start();
      s.transport.emit(segment(1));
      await pumpEventQueue();
      s.transport.emit(error(kind: kind));
      await pumpEventQueue();

      expect(s.session.segments.map((x) => x.segId), [1], reason: kind);
      expect(s.session.inlineErrorKind, kind);
      expect(s.session.inlineError, contains('Provider said no'));
      expect(s.session.status, SessionStatus.listening, reason: kind);
    }
  });

  test('a translator protocol error points at the model', () async {
    final s = await liveSession();
    await s.session.start();
    s.transport.emit(error(provider: 'translator', kind: 'protocol'));
    await pumpEventQueue();

    expect(s.session.inlineErrorKind, 'model');
    expect(s.session.asrReconnecting, isFalse);
  });

  test(
    'an ASR failure keeps the Session alive and shows reconnecting',
    () async {
      final s = await liveSession();
      await s.session.start();
      s.transport.emit(segment(1));
      await pumpEventQueue();

      s.transport.emit(error(provider: 'asr', kind: 'unreachable'));
      await pumpEventQueue();
      expect(s.session.asrReconnecting, isTrue);
      expect(s.session.status, SessionStatus.listening);
      expect(s.session.segments.map((x) => x.segId), [1]);

      s.transport.emit({
        'type': 'asr_partial',
        'text': 'still here',
        'direction': {'src': 'ja', 'tgt': 'zh'},
      });
      await pumpEventQueue();
      expect(s.session.asrReconnecting, isFalse);
    },
  );

  test(
    'a TTS failure keeps the subtitles-only chip and the transcript',
    () async {
      final s = await liveSession();
      await s.session.start();
      s.transport.emit(segment(1));
      await pumpEventQueue();

      s.transport.emit(error(provider: 'tts', kind: 'quota'));
      await pumpEventQueue();
      expect(s.session.audioError, isNotNull);
      expect(s.session.inlineError, isNull);
      expect(s.session.segments.map((x) => x.segId), [1]);
    },
  );
}
