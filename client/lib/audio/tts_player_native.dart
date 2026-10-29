import 'dart:async';
import 'dart:typed_data';

import 'package:flutter_soloud/flutter_soloud.dart';

import 'tts_player_api.dart';

/// Native TTS playback through flutter_soloud, one buffer stream per chunk.
/// The API and the completion event are the ones spike S1 verified on Android.
class SoloudTtsPlayer implements TtsPlayer {
  static const Duration _timeout = Duration(seconds: 10);

  SoundHandle? _handle;

  @override
  Future<void> play(String format, Uint8List data) async {
    await SoLoud.instance.init();
    final source = SoLoud.instance.setBufferStream(
      maxBufferSizeDuration: const Duration(seconds: 30),
      bufferingTimeNeeds: 0.05,
      sampleRate: format == 'wav' ? 16000 : 24000,
      channels: Channels.mono,
      format: BufferType.auto,
    );
    final done = Completer<void>();
    SoundHandle? handle;
    final sub = source.soundEvents.listen((event) {
      if (event.event == SoundEventType.handleIsNoMoreValid &&
          event.handle == handle &&
          !done.isCompleted) {
        done.complete();
      }
    });
    try {
      SoLoud.instance.addAudioDataStream(source, data);
      handle = SoLoud.instance.play(source);
      _handle = handle;
      SoLoud.instance.setDataIsEnded(source);
      await done.future.timeout(_timeout);
    } finally {
      await sub.cancel();
      // An interrupted or stopped clip may settle after the next one started;
      // never clear the newer handle.
      if (_handle == handle) _handle = null;
      await SoLoud.instance.disposeSource(source);
    }
  }

  @override
  Future<void> stop() async {
    final handle = _handle;
    if (handle != null) await SoLoud.instance.stop(handle);
  }

  @override
  Future<void> dispose() => SoLoud.instance.deinitAsync();
}

TtsPlayer createTtsPlayer() => SoloudTtsPlayer();
