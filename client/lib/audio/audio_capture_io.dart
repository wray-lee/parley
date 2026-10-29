import 'dart:async';
import 'dart:typed_data';

import 'package:audio_session/audio_session.dart';
import 'package:record/record.dart';

import '../model/protocol.dart';
import 'audio_capture_api.dart';
import 'pcm16_framer.dart';

/// Device capture. `audio_session` is the sole owner of the platform audio
/// session (spike S1): `record` must not manage it on iOS, and the session is
/// configured once, after the plugin is initialised.
class DeviceAudioCapture implements AudioCapture {
  final AudioRecorder _recorder = AudioRecorder();
  final Pcm16Framer _framer = Pcm16Framer();
  StreamSubscription<Uint8List>? _subscription;
  Timer? _demoTimer;
  bool _sessionReady = false;

  static final RecordConfig _config = RecordConfig(
    encoder: AudioEncoder.pcm16bits,
    sampleRate: Pcm16Framer.sampleRate,
    numChannels: 1,
    autoGain: true,
    echoCancel: true,
    noiseSuppress: true,
    androidConfig: AndroidRecordConfig(
      audioSource: AndroidAudioSource.voiceCommunication,
      audioManagerMode: AudioManagerMode.modeInCommunication,
      speakerphone: true,
      manageBluetooth: true,
    ),
    iosConfig: const IosRecordConfig(),
  );

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
    await _prepareSession();
    final stream = await _recorder.startStream(_config);
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

  Future<void> _prepareSession() async {
    if (_sessionReady) return;
    final session = await AudioSession.instance;
    await _recorder.ios?.manageAudioSession(false);
    await session.configure(
      AudioSessionConfiguration(
        avAudioSessionCategory: AVAudioSessionCategory.playAndRecord,
        avAudioSessionCategoryOptions:
            AVAudioSessionCategoryOptions.defaultToSpeaker |
            AVAudioSessionCategoryOptions.allowBluetooth |
            AVAudioSessionCategoryOptions.allowBluetoothA2dp,
        avAudioSessionMode: AVAudioSessionMode.voiceChat,
        avAudioSessionRouteSharingPolicy:
            AVAudioSessionRouteSharingPolicy.defaultPolicy,
        androidAudioAttributes: const AndroidAudioAttributes(
          contentType: AndroidAudioContentType.speech,
          usage: AndroidAudioUsage.voiceCommunication,
          flags: AndroidAudioFlags.none,
        ),
        androidAudioFocusGainType: AndroidAudioFocusGainType.gain,
        androidWillPauseWhenDucked: false,
      ),
    );
    await session.setActive(true);
    _sessionReady = true;
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
