import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';

import 'package:parley/audio/audio_capture_api.dart';
import 'package:parley/audio/tts_player_api.dart';
import 'package:parley/audio/tts_system.dart';
import 'package:parley/model/protocol.dart';
import 'package:parley/platform/first_run_store_api.dart';
import 'package:parley/platform/local_network_permission_api.dart';
import 'package:parley/platform/mic_permission_api.dart';
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
  int stops = 0;

  @override
  Future<void> play(String format, Uint8List data) async {
    playedFormats.add(format);
    playedData.add(data);
    final failure = error;
    if (failure != null) throw failure;
  }

  @override
  Future<void> stop() async {
    stops++;
  }

  @override
  Future<void> dispose() async {
    disposed = true;
  }
}

class FakeMicPermission implements MicPermission {
  FakeMicPermission([this.access = MicAccess.granted]);

  MicAccess access;
  int requests = 0;
  int settingsOpened = 0;

  @override
  Future<MicAccess> request() async {
    requests++;
    return access;
  }

  @override
  Future<bool> openSettings() async {
    settingsOpened++;
    return true;
  }
}

class FakeFirstRunStore implements FirstRunStore {
  FakeFirstRunStore([Set<String>? seen]) : values = {...?seen};

  final Set<String> values;

  @override
  Future<bool> seen(String flag) async => values.contains(flag);

  @override
  Future<void> markSeen(String flag) async => values.add(flag);
}

class FakeLocalNetworkPermission implements LocalNetworkPermission {
  FakeLocalNetworkPermission([this.access = LocalNetworkAccess.granted]);

  LocalNetworkAccess access;
  int requests = 0;

  @override
  Future<LocalNetworkAccess> request() async {
    requests++;
    return access;
  }
}

/// A system speech engine whose utterances end only when the test says so.
class FakeSystemTtsEngine implements SystemTtsEngine {
  final List<String> spoken = [];
  final List<String?> languages = [];
  final List<Completer<void>> pending = [];
  int stops = 0;
  bool disposed = false;
  int _active = 0;
  int maxActive = 0;

  @override
  Future<void> speak(String text, {String? language}) {
    spoken.add(text);
    languages.add(language);
    _active++;
    if (_active > maxActive) maxActive = _active;
    final completer = Completer<void>();
    pending.add(completer);
    return completer.future.whenComplete(() => _active--);
  }

  void complete([int index = 0]) {
    if (!pending[index].isCompleted) pending[index].complete();
  }

  void fail(Object error, [int index = 0]) {
    if (!pending[index].isCompleted) pending[index].completeError(error);
  }

  @override
  Future<void> stop() async {
    stops++;
  }

  @override
  Future<void> dispose() async {
    disposed = true;
  }
}
