import 'package:fake_async/fake_async.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:parley/audio/tts_system.dart';

import 'fakes.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  test('speaks one Segment at a time and reports a start/end pair', () async {
    final engine = FakeSystemTtsEngine();
    final reports = <String>[];
    final errors = <Object>[];
    final speaker = SystemTtsSpeaker(
      engine: engine,
      onPlayback: (segId, state) => reports.add('$segId:$state'),
      onError: errors.add,
    );

    speaker.speak(1, 'one');
    speaker.speak(2, 'two', language: 'ja');
    await pumpEventQueue();

    expect(engine.spoken, ['one']);
    expect(engine.maxActive, 1);
    expect(reports, ['1:start']);

    engine.complete(0);
    await pumpEventQueue();
    expect(reports, ['1:start', '1:end', '2:start']);
    expect(engine.spoken, ['one', 'two']);
    expect(engine.languages, [null, 'ja']);
    expect(engine.maxActive, 1);

    engine.complete(1);
    await pumpEventQueue();
    expect(reports, ['1:start', '1:end', '2:start', '2:end']);
    expect(errors, isEmpty);
  });

  test('an engine error ends the utterance, reports it and moves on', () async {
    final engine = FakeSystemTtsEngine();
    final reports = <String>[];
    final errors = <Object>[];
    final speaker = SystemTtsSpeaker(
      engine: engine,
      onPlayback: (segId, state) => reports.add('$segId:$state'),
      onError: errors.add,
    );

    speaker.speak(1, 'one');
    speaker.speak(2, 'two');
    await pumpEventQueue();
    engine.fail(StateError('engine died'));
    await pumpEventQueue();

    expect(errors.single, isA<StateError>());
    expect(reports, ['1:start', '1:end', '2:start']);
    expect(engine.spoken, ['one', 'two']);
  });

  test('stop cancels the current utterance and drops the queue', () async {
    final engine = FakeSystemTtsEngine();
    final reports = <String>[];
    final speaker = SystemTtsSpeaker(
      engine: engine,
      onPlayback: (segId, state) => reports.add('$segId:$state'),
      onError: (_) {},
    );

    speaker.speak(1, 'one');
    speaker.speak(2, 'two');
    await pumpEventQueue();
    await speaker.stop();

    expect(reports, ['1:start', '1:end']);
    expect(engine.stops, 1);
    expect(engine.spoken, ['one']);

    // A late engine completion must not report the end twice or revive seg 2.
    engine.complete(0);
    await pumpEventQueue();
    expect(reports, ['1:start', '1:end']);
    expect(engine.spoken, ['one']);
  });

  test('the watchdog ends an utterance the engine never completes', () {
    fakeAsync((async) {
      final engine = FakeSystemTtsEngine();
      final reports = <String>[];
      final errors = <Object>[];
      final speaker = SystemTtsSpeaker(
        engine: engine,
        onPlayback: (segId, state) => reports.add('$segId:$state'),
        onError: errors.add,
        watchdog: (_) => const Duration(seconds: 3),
      );

      speaker.speak(1, 'one');
      speaker.speak(2, 'two');
      async.flushMicrotasks();
      expect(engine.spoken, ['one']);
      expect(reports, ['1:start']);

      async.elapse(const Duration(seconds: 3));
      async.flushMicrotasks();
      expect(reports, ['1:start', '1:end', '2:start']);
      expect(engine.spoken, ['one', 'two']);
      // A missing completion is a TTS failure: the chip must appear rather
      // than the user silently getting no audio.
      expect(errors, hasLength(1));

      async.elapse(const Duration(seconds: 3));
      async.flushMicrotasks();
      expect(reports, ['1:start', '1:end', '2:start', '2:end']);
      expect(errors, hasLength(2));
    });
  });

  test(
    'reset drops a previous Session without reporting its playback end',
    () async {
      final engine = FakeSystemTtsEngine();
      final reports = <String>[];
      final speaker = SystemTtsSpeaker(
        engine: engine,
        onPlayback: (segId, state) => reports.add('$segId:$state'),
        onError: (_) {},
      );

      speaker.speak(1, 'old');
      await pumpEventQueue();
      speaker.reset();
      speaker.speak(1, 'new');
      await pumpEventQueue();

      expect(reports, ['1:start', '1:start']);
      expect(engine.spoken, ['old', 'new']);

      // The old utterance's late completion is ignored, not reported as the
      // new Session's seg 1.
      engine.complete(0);
      await pumpEventQueue();
      expect(reports, ['1:start', '1:start']);

      engine.complete(1);
      await pumpEventQueue();
      expect(reports, ['1:start', '1:start', '1:end']);
    },
  );

  test('empty text is never spoken', () async {
    final engine = FakeSystemTtsEngine();
    final speaker = SystemTtsSpeaker(
      engine: engine,
      onPlayback: (_, _) {},
      onError: (_) {},
    );

    speaker.speak(1, '   ');
    await pumpEventQueue();
    expect(engine.spoken, isEmpty);
  });

  test('the watchdog follows the core estimate plus its grace', () {
    expect(systemSpeechWatchdog('abc'), const Duration(milliseconds: 3540));
    expect(
      systemSpeechWatchdog('x' * 200),
      const Duration(milliseconds: 31500),
    );
  });

  test('an engine that refuses the utterance reports an error', () async {
    const channel = MethodChannel('flutter_tts');
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(channel, (call) async {
          return call.method == 'speak' ? 0 : 1;
        });
    addTearDown(
      () => TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
          .setMockMethodCallHandler(channel, null),
    );

    final reports = <String>[];
    final errors = <Object>[];
    final speaker = SystemTtsSpeaker(
      engine: FlutterTtsEngine(),
      onPlayback: (segId, state) => reports.add('$segId:$state'),
      onError: errors.add,
    );

    speaker.speak(1, 'one');
    await pumpEventQueue();

    expect(errors.single, isA<StateError>());
    expect(reports, ['1:start', '1:end']);
  });
}
