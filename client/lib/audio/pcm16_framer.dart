import 'dart:typed_data';

/// Cuts an arbitrary PCM16 byte stream into exact 20 ms mono frames at 16 kHz:
/// 320 samples, 640 bytes. `record` chunk boundaries vary; the wire contract
/// does not.
class Pcm16Framer {
  static const int sampleRate = 16000;
  static const int frameSamples = 320;
  static const int frameBytes = frameSamples * 2;

  final BytesBuilder _pending = BytesBuilder(copy: true);

  int get pendingBytes => _pending.length;

  List<Uint8List> add(List<int> chunk) {
    _pending.add(chunk);
    final bytes = _pending.takeBytes();
    final frames = <Uint8List>[];
    var offset = 0;
    while (bytes.length - offset >= frameBytes) {
      frames.add(Uint8List.sublistView(bytes, offset, offset + frameBytes));
      offset += frameBytes;
    }
    if (offset < bytes.length) {
      _pending.add(bytes.sublist(offset));
    }
    return frames;
  }

  void reset() => _pending.clear();
}
