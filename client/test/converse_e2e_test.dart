import 'dart:async';
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:parley/audio/audio_capture_api.dart';
import 'package:parley/audio/tts_player_api.dart';
import 'package:parley/model/mode.dart';
import 'package:parley/model/protocol.dart';
import 'package:parley/session/listen_session.dart';
import 'package:parley/transport/web_socket_transport.dart';

/// Runs a Converse Demo Session against a live Server Host:
/// `flutter test test/converse_e2e_test.dart --dart-define=PARLEY_E2E_URL=http://127.0.0.1:PORT`
/// The Demo sample is my speech; the core speaks it, gates the mic, then
/// auto-switches to them. A second replay becomes their subtitles only.
const e2eUrl = String.fromEnvironment('PARLEY_E2E_URL');

class ReplayAudioCapture implements AudioCapture {
  void Function(Uint8List frame)? _onFrame;
  DemoClip? _clip;
  Timer? _timer;
  int _offsetMs = 0;

  @override
  Future<bool> start(
    void Function(Uint8List frame) onFrame, {
    DemoClip? demo,
  }) async {
    _onFrame = onFrame;
    _clip = demo;
    replay();
    return true;
  }

  void replay() {
    _timer?.cancel();
    _offsetMs = 0;
    final clip = _clip;
    if (clip == null) return;
    _timer = Timer.periodic(const Duration(milliseconds: 20), (timer) {
      if (_offsetMs >= clip.durationMs) {
        timer.cancel();
        _timer = null;
        return;
      }
      _onFrame?.call(Uint8List.view(clip.frame(_offsetMs, 320).buffer));
      _offsetMs += 20;
    });
  }

  @override
  Future<void> stop() async {
    _timer?.cancel();
    _timer = null;
  }
}

class BlockingTtsPlayer implements TtsPlayer {
  final List<Uint8List> played = [];
  Completer<void>? _playing;

  int get playCount => played.length;

  @override
  Future<void> play(String format, Uint8List data) {
    played.add(data);
    final done = Completer<void>();
    _playing = done;
    return done.future;
  }

  void finish() {
    final done = _playing;
    _playing = null;
    if (done != null && !done.isCompleted) done.complete();
  }

  @override
  Future<void> stop() async => finish();

  @override
  Future<void> dispose() async {}
}

Future<void> waitFor(
  bool Function() condition, {
  Duration timeout = const Duration(seconds: 30),
  required String reason,
}) async {
  final deadline = DateTime.now().add(timeout);
  while (!condition()) {
    if (DateTime.now().isAfter(deadline)) fail('timed out waiting for $reason');
    await Future<void>.delayed(const Duration(milliseconds: 50));
  }
}

void main() {
  test(
    'Converse Demo against a live Server Host: speak, gate, auto-switch, subtitles',
    () async {
      final base = Uri.parse(e2eUrl);
      final ws = base.replace(
        scheme: 'ws',
        path: '/ws',
        queryParameters: const <String, String>{},
      );
      final transport = WebSocketTransport(ws);
      final player = BlockingTtsPlayer();
      final audio = ReplayAudioCapture();
      final session = ListenSession(
        transport: transport,
        audio: audio,
        ttsPlayer: player,
        serverBase: base,
      );
      addTearDown(session.dispose);
      await session.connect();
      await waitFor(
        () => session.pairs.isNotEmpty,
        reason: 'hello with a Language Pair',
      );
      final pair = session.pairs.first;
      // ignore: avoid_print
      print('PARLEY e2e pair=${pair.label} converse=${pair.converse}');
      expect(pair.converse, isTrue);
      session
        ..selectMode(SessionMode.converse)
        ..selectPair(pair)
        ..selectDirection(pair.direction('zh', 'ja')!)
        ..selectSample('placeholder');

      await session.start();
      await waitFor(() => session.turn == 'me', reason: 'the initial me Turn');
      await waitFor(
        () => session.segments.isNotEmpty,
        reason: 'my committed Segment',
      );

      final mine = session.segments.first;
      // ignore: avoid_print
      print(
        'PARLEY e2e segment1 dir=${mine.direction?.src}->${mine.direction?.tgt} '
        'tgt="${mine.translation}"',
      );
      expect(mine.direction?.src, 'zh');
      expect(mine.direction?.tgt, 'ja', reason: 'my Direction is spoken');

      await waitFor(
        () => player.playCount == 1,
        reason: 'my spoken Segment to play',
      );
      expect(
        session.gated,
        isTrue,
        reason: 'the mic is withheld while I speak',
      );
      // ignore: avoid_print
      print('PARLEY e2e gated=true while my Segment plays');

      player.finish();
      await waitFor(
        () => session.turn == 'them',
        reason: 'the auto-switch after my speech',
      );
      expect(session.turnAuto, isTrue);
      expect(session.turnCue, 1);
      // ignore: avoid_print
      print(
        'PARLEY e2e auto-switch turn=${session.turn} '
        'auto=${session.turnAuto} cue=${session.turnCue}',
      );

      // Their speech: same bundled sample replayed on the them Turn. Its
      // Direction is subtitle-only, so no second clip reaches the player.
      audio.replay();
      await waitFor(
        () => session.segments.length >= 2,
        reason: 'their subtitle Segment',
      );
      final theirs = session.segments[1];
      // ignore: avoid_print
      print(
        'PARLEY e2e segment2 dir=${theirs.direction?.src}->${theirs.direction?.tgt} '
        'tgt="${theirs.translation}"',
      );
      expect(theirs.direction?.src, 'ja');
      expect(theirs.direction?.tgt, 'zh');
      await Future<void>.delayed(const Duration(milliseconds: 300));
      expect(player.playCount, 1, reason: 'their Direction is subtitles only');
      expect(session.gated, isFalse);

      await session.stop();
    },
    skip: e2eUrl.isEmpty ? 'PARLEY_E2E_URL is not set' : false,
    timeout: const Timeout(Duration(minutes: 3)),
  );
}
