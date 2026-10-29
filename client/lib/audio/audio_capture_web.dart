import 'dart:async';
import 'dart:typed_data';

import 'package:record/record.dart';

import '../model/protocol.dart';
import 'audio_capture_api.dart';
import 'pcm16_framer.dart';

/// Browser capture. `audio_session` has no web implementation, so the browser
/// owns routing; frames still arrive as PCM16 16 kHz through `record`.
class DeviceAudioCapture implements AudioCapture {
  final AudioRecorder _recorder = AudioRecorder();
  final Pcm16Framer _framer = Pcm16Framer();
  StreamSubscription<Uint8List>? _subscription;
  Timer? _demoTimer;

  @override
  Future<bool> start(
    void Function(Uint8List frame) onFrame, {
    DemoClip? demo,
  }) async {
    await stop();
    if (demo != null) {
      _startDemo(demo, onFrame);
      return true;
    }
    if (!await _recorder.hasPermission()) return false;
    final stream = await _recorder.startStream(
      const RecordConfig(
        encoder: AudioEncoder.pcm16bits,
        sampleRate: Pcm16Framer.sampleRate,
        numChannels: 1,
      ),
    );
    _subscription = stream.listen((chunk) {
      for (final frame in _framer.add(chunk)) {
        onFrame(frame);
      }
    });
    return true;
  }

  void _startDemo(DemoClip clip, void Function(Uint8List frame) onFrame) {
    var offsetMs = 0;
    _demoTimer = Timer.periodic(const Duration(milliseconds: 20), (timer) {
      if (offsetMs >= clip.durationMs) {
        timer.cancel();
        _demoTimer = null;
        return;
      }
      final pcm = clip.frame(offsetMs, Pcm16Framer.frameSamples);
      offsetMs += 20;
      onFrame(Uint8List.view(pcm.buffer));
    });
  }

  @override
  Future<void> stop() async {
    _demoTimer?.cancel();
    _demoTimer = null;
    await _subscription?.cancel();
    _subscription = null;
    _framer.reset();
    if (await _recorder.isRecording()) await _recorder.stop();
  }
}

AudioCapture createAudioCapture() => DeviceAudioCapture();
