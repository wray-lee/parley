import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:flutter/foundation.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:parley/platform/device_host.dart';
import 'package:parley/session/listen_session.dart';
import 'package:parley/transport/device_host_transport.dart';

import 'fakes.dart';

/// A real loopback WebSocket server: the transport's inner socket is the same
/// web_socket_channel implementation the app uses on device.
class TestServer {
  TestServer._(this._server, this.url);

  static Future<TestServer> start() async {
    final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    final url = Uri.parse('ws://127.0.0.1:${server.port}/ws');
    final instance = TestServer._(server, url);
    server.transform(WebSocketTransformer()).listen(instance._accept);
    return instance;
  }

  final HttpServer _server;
  final Uri url;

  /// What the next accepted socket hears; the default stands in for a server
  /// that is not a Parley host (the transport tests only need a frame).
  String? greeting;
  final List<WebSocket> sockets = [];
  final List<Object?> received = [];
  int closed = 0;

  void _accept(WebSocket socket) {
    final number = sockets.length + 1;
    sockets.add(socket);
    socket.add(greeting ?? 'hello-$number');
    socket.listen(
      received.add,
      onDone: () => closed++,
      onError: (_) => closed++,
    );
  }

  Future<void> close() async {
    for (final socket in sockets) {
      await socket.close();
    }
    await _server.close(force: true);
  }
}

class FakeDeviceHost implements DeviceHost {
  FakeDeviceHost(this.endpoint);

  final Uri endpoint;
  int starts = 0;
  int stops = 0;
  final List<Map<String, String>> environments = [];
  Object? startError;

  @override
  Future<HostEndpoint> start(Map<String, String> environment) async {
    starts++;
    environments.add({...environment});
    final error = startError;
    if (error != null) throw error;
    final base = endpoint.replace(queryParameters: {'token': 'token-$starts'});
    return HostEndpoint(
      wsUri: base,
      baseUri: Uri.parse('http://127.0.0.1:${endpoint.port}/'),
    );
  }

  @override
  Future<void> stop() async => stops++;
}

/// Holds the end of `start` so a test can pause the app between the host
/// starting and the transport opening its socket.
class GatedDeviceHost extends FakeDeviceHost {
  GatedDeviceHost(super.endpoint);

  final Completer<void> started = Completer<void>();
  final Completer<void> release = Completer<void>();

  @override
  Future<HostEndpoint> start(Map<String, String> environment) async {
    final endpoint = await super.start(environment);
    if (!started.isCompleted) started.complete();
    await release.future;
    return endpoint;
  }
}

Future<int> closedPort() async {
  final socket = await ServerSocket.bind(InternetAddress.loopbackIPv4, 0);
  final port = socket.port;
  await socket.close();
  return port;
}

Future<void> eventually(bool Function() condition) async {
  final deadline = DateTime.now().add(const Duration(seconds: 5));
  while (!condition()) {
    if (DateTime.now().isAfter(deadline)) {
      fail('condition was not met within 5s');
    }
    await Future<void>.delayed(const Duration(milliseconds: 10));
  }
}

/// A real hello so a [ListenSession] reaches `ready` against the loopback.
String deviceHostHello() => jsonEncode({
  'type': 'hello',
  'pairs': [
    {
      'a': 'ja',
      'b': 'zh',
      'directions': [
        {'src': 'ja', 'tgt': 'zh'},
        {'src': 'zh', 'tgt': 'ja'},
      ],
      'converse': true,
    },
  ],
  'packs': <dynamic>[],
  'presetsInUse': <dynamic>[],
  'demo': false,
});

void main() {
  late TestServer server;

  setUp(() async {
    server = await TestServer.start();
  });

  tearDown(() => server.close());

  test('connect starts the host, forwards hello and carries frames', () async {
    final host = FakeDeviceHost(server.url);
    final transport = DeviceHostTransport(
      host: host,
      environment: () => {'LLM_API_KEY': 'llm-secret'},
    );
    addTearDown(transport.close);
    final text = <String>[];
    transport.text.listen(text.add);

    await transport.connect();
    await eventually(() => text.isNotEmpty);

    expect(text, ['hello-1']);
    expect(host.starts, 1);
    expect(host.environments.single, {'LLM_API_KEY': 'llm-secret'});
    expect(
      transport.serverBase,
      Uri.parse('http://127.0.0.1:${server.url.port}/'),
    );

    transport.sendText('ping');
    await eventually(() => server.received.contains('ping'));
  });

  test('concurrent connects share one host start', () async {
    final host = FakeDeviceHost(server.url);
    final transport = DeviceHostTransport(
      host: host,
      environment: () => const {},
    );
    addTearDown(transport.close);
    final text = <String>[];
    transport.text.listen(text.add);

    await Future.wait([transport.connect(), transport.resume()]);
    await eventually(() => text.isNotEmpty);

    expect(host.starts, 1);
    expect(text, ['hello-1']);
  });

  test(
    'a pause between host start and socket connect abandons that host',
    () async {
      final host = GatedDeviceHost(server.url);
      final transport = DeviceHostTransport(
        host: host,
        environment: () => const {},
      );
      addTearDown(transport.close);
      final text = <String>[];
      transport.text.listen(text.add);

      final connecting = transport.connect();
      await host.started.future;

      final pausing = transport.suspend();
      host.release.complete();
      await pausing;
      await connecting;
      // The stale connect stops the host it started (Stop is idempotent) and
      // never marks itself connected.
      expect(host.stops, 2);
      expect(transport.serverBase, isNull);
      expect(text, isEmpty);

      await transport.resume();
      await eventually(() => text.isNotEmpty);
      expect(text, ['hello-1']);
      expect(host.starts, 2);
      transport.sendText('ping');
      await eventually(() => server.received.contains('ping'));
    },
  );

  test('restart stops the host and reconnects with a fresh endpoint', () async {
    final host = FakeDeviceHost(server.url);
    final transport = DeviceHostTransport(
      host: host,
      environment: () => const {},
    );
    addTearDown(transport.close);
    final text = <String>[];
    transport.text.listen(text.add);

    await transport.connect();
    await eventually(() => text.length == 1);
    await transport.restart();
    await eventually(() => text.length == 2);

    expect(text, ['hello-1', 'hello-2']);
    expect(host.starts, 2);
    expect(host.stops, 1);
  });

  test('suspend stops the host and resume starts a new one', () async {
    final host = FakeDeviceHost(server.url);
    final transport = DeviceHostTransport(
      host: host,
      environment: () => const {},
    );
    addTearDown(transport.close);
    final text = <String>[];
    transport.text.listen(text.add);

    await transport.connect();
    await eventually(() => text.length == 1);

    await transport.suspend();
    expect(host.stops, 1);
    await eventually(() => server.closed == 1);

    await transport.resume();
    await eventually(() => text.length == 2);
    expect(text, ['hello-1', 'hello-2']);
    expect(host.starts, 2);
  });

  test('close stops the host and closes the stream', () async {
    final host = FakeDeviceHost(server.url);
    final transport = DeviceHostTransport(
      host: host,
      environment: () => const {},
    );
    var closed = false;
    transport.text.listen(null, onDone: () => closed = true);

    await transport.connect();
    await transport.close();

    expect(host.stops, 1);
    expect(closed, isTrue);
    await eventually(() => server.closed == 1);
  });

  test('a host start failure propagates and leaves nothing open', () async {
    final host = FakeDeviceHost(server.url)
      ..startError = const DeviceHostException(
        'The on-device host could not start.',
      );
    final transport = DeviceHostTransport(
      host: host,
      environment: () => const {},
    );
    addTearDown(transport.close);

    await expectLater(transport.connect(), throwsA(isA<DeviceHostException>()));
    expect(host.stops, 0);
  });

  test('a failed handshake is an explicit, token-free error', () async {
    final host = FakeDeviceHost(
      Uri.parse('ws://127.0.0.1:${await closedPort()}/ws'),
    );
    final transport = DeviceHostTransport(
      host: host,
      environment: () => const {},
    );
    addTearDown(transport.close);

    await expectLater(
      transport.connect(),
      throwsA(
        isA<DeviceHostException>().having(
          (error) => error.message,
          'message',
          'The on-device host connection failed.',
        ),
      ),
    );
    expect(host.stops, 1);
    expect(transport.serverBase, isNull);
  });

  test('Provider keys never reach the debug log', () async {
    final logs = <String?>[];
    final original = debugPrint;
    debugPrint = (message, {wrapWidth}) => logs.add(message);
    addTearDown(() => debugPrint = original);

    final host = FakeDeviceHost(server.url);
    final transport = DeviceHostTransport(
      host: host,
      environment: () => {'LLM_API_KEY': 'sentinel-secret-value'},
    );
    addTearDown(transport.close);

    await transport.connect();
    await eventually(() => logs.isNotEmpty);

    expect(logs.join('\n'), isNot(contains('sentinel-secret-value')));
    expect(logs.join('\n'), contains('PARLEY host ws://127.0.0.1:'));
  });

  // A Settings Save stops the host, then starts a new one. When that start
  // rejects the transport goes quiet: no socket, no done, no error. A Session
  // that trusted its stale `ready` used to Start into the void.
  Future<
    ({
      FakeDeviceHost host,
      DeviceHostTransport transport,
      ListenSession session,
      FakeAudioCapture audio,
      FakeTtsPlayer ttsPlayer,
    })
  >
  readySession() async {
    server.greeting = deviceHostHello();
    final host = FakeDeviceHost(server.url);
    final transport = DeviceHostTransport(
      host: host,
      environment: () => const {},
    );
    final audio = FakeAudioCapture();
    final ttsPlayer = FakeTtsPlayer();
    final session = ListenSession(
      transport: transport,
      audio: audio,
      ttsPlayer: ttsPlayer,
    );
    addTearDown(transport.close);
    addTearDown(session.dispose);
    await session.connect();
    await eventually(() => session.status == SessionStatus.ready);
    return (
      host: host,
      transport: transport,
      session: session,
      audio: audio,
      ttsPlayer: ttsPlayer,
    );
  }

  const missingRegion =
      'Azure TTS requires AZURE_SPEECH_REGION (for example eastus).';

  test(
    'a failed Settings restart fails the Session with the host error',
    () async {
      final s = await readySession();
      // What Settings Save does with an invalid environment: the host start
      // rejects before any socket exists.
      s.host.startError = const DeviceHostException(missingRegion);
      await expectLater(
        s.transport.restart(),
        throwsA(isA<DeviceHostException>()),
      );
      await eventually(() => s.session.status == SessionStatus.failed);

      expect(s.session.error, missingRegion);
      expect(server.received, isEmpty);
    },
  );

  test(
    'a failed Settings restart fails a listening Session and stops the mic',
    () async {
      final s = await readySession();
      await s.session.start();
      expect(s.session.status, SessionStatus.listening);
      expect(s.audio.started, isTrue);

      s.host.startError = const DeviceHostException(missingRegion);
      await expectLater(
        s.transport.restart(),
        throwsA(isA<DeviceHostException>()),
      );
      await eventually(() => s.session.status == SessionStatus.failed);

      expect(s.session.error, missingRegion);
      expect(s.audio.started, isFalse);
    },
  );

  test(
    'a successful Settings restart stops a listening Session at ready',
    () async {
      final s = await readySession();
      await s.session.start();
      expect(s.session.status, SessionStatus.listening);
      expect(s.audio.started, isTrue);

      await s.transport.restart();
      await eventually(() => s.session.status == SessionStatus.ready);

      expect(s.session.error, isNull);
      expect(s.audio.started, isFalse);
      expect(s.ttsPlayer.stops, greaterThanOrEqualTo(1));
    },
  );

  test('resume reports the Device Host error, not Connection lost', () async {
    final s = await readySession();
    await s.transport.suspend();
    s.host.startError = const DeviceHostException(missingRegion);

    await s.transport.resume();
    await eventually(() => s.session.status == SessionStatus.failed);

    expect(s.session.error, missingRegion);
  });

  test('a failed restart keeps a non-host error generic', () async {
    final s = await readySession();
    s.host.startError = StateError(
      'start failed at ws://127.0.0.1:9/ws?token=SECRET123',
    );
    await expectLater(s.transport.restart(), throwsA(isA<StateError>()));
    await eventually(() => s.session.status == SessionStatus.failed);

    expect(s.session.error, 'Connection lost.');
    expect(s.session.error, isNot(contains('SECRET123')));
  });
}
