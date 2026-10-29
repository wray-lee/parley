import 'dart:typed_data';

import '../model/protocol.dart';

/// Produces 20 ms PCM16 16 kHz frames for the session: the microphone on a
/// device, or the bundled Demo clip's synthesised waveform in Demo Mode.
abstract interface class AudioCapture {
  /// Returns false when the microphone is unavailable (permission or device).
  Future<bool> start(void Function(Uint8List frame) onFrame, {DemoClip? demo});

  Future<void> stop();
}
