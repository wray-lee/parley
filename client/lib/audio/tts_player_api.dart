import 'dart:typed_data';

/// Plays one TTS audio chunk (WAV or MP3). Completion means the sound finished
/// or failed; [TtsQueue] serialises calls, so one player never overlaps.
abstract interface class TtsPlayer {
  Future<void> play(String format, Uint8List data);
  Future<void> stop();
  Future<void> dispose();
}
