import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:parley/audio/pcm16_framer.dart';

void main() {
  test('cuts arbitrary chunks into exact 640-byte 20 ms frames', () {
    final framer = Pcm16Framer();
    final frames = <Uint8List>[
      ...framer.add(Uint8List(100)),
      ...framer.add(Uint8List(2048)),
      ...framer.add(Uint8List(129)),
    ];
    expect(frames, hasLength(3));
    expect(
      frames.every((frame) => frame.length == Pcm16Framer.frameBytes),
      isTrue,
    );
    expect(framer.pendingBytes, 2277 - 3 * Pcm16Framer.frameBytes);
  });

  test('a single 20 ms frame comes out whole and leaves no remainder', () {
    final framer = Pcm16Framer();
    final frames = framer.add(Uint8List(Pcm16Framer.frameBytes));
    expect(frames.single.length, 640);
    expect(framer.pendingBytes, 0);
  });
}
