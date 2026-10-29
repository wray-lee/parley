import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';

import 'package:parley/audio/audio_capture_api.dart';
import 'package:parley/audio/tts_player_api.dart';
import 'package:parley/model/protocol.dart';
import 'package:parley/transport/transport.dart';

class FakeTransport implements Transport {
  // One controller, like the real WebSocket: text and binary keep their order
  // relative to each other.
  final StreamController<dynamic> _events =
      StreamController<dynamic>.broadcast();

  final List<String> sentText = [];
  final List<Uint8List> sentBinary = [];
  Object? connectError;

  @override
  Stream<String> get text =>
      _events.stream.where((e) => e is String).cast<String>();

  @override
  Stream<Uint8List> get binary => _events.stream
      .where((e) => e is List<int>)
      .map((e) => Uint8List.fromList(e));

  @override
  Future<void> connect() async {
    final error = connectError;
    if (error != null) throw error;
  }

  @override
  void sendText(String message) => sentText.add(message);

  @override
  void sendBinary(Uint8List data) => sentBinary.add(data);

  @override
  Future<void> close() => _events.close();

  void emit(Map<String, dynamic> message) => _events.add(jsonEncode(message));

  void emitBinary(Uint8List data) => _events.add(data);

  void addError(Object error) => _events.addError(error);
}

class FakeAudioCapture implements AudioCapture {
  bool permission = true;
  bool started = false;
  DemoClip? demo;
  void Function(Uint8List frame)? _onFrame;

  @override
  Future<bool> start(
    void Function(Uint8List frame) onFrame, {
    DemoClip? demo,
  }) async {
    _onFrame = onFrame;
    this.demo = demo;
    started = true;
    return permission;
  }

  @override
  Future<void> stop() async {
    started = false;
  }

  void frame({int bytes = 640}) => _onFrame?.call(Uint8List(bytes));
}

class FakeTtsPlayer implements TtsPlayer {
  final List<Uint8List> playedData = [];
  final List<String> playedFormats = [];
  Object? error;
  bool disposed = false;

  @override
  Future<void> play(String format, Uint8List data) async {
    playedFormats.add(format);
    playedData.add(data);
    final failure = error;
    if (failure != null) throw failure;
  }

  @override
  Future<void> stop() async {}

  @override
  Future<void> dispose() async {
    disposed = true;
  }
}
