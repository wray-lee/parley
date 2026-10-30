import 'dart:async';
import 'dart:typed_data';

import 'package:flutter/foundation.dart' show debugPrint, kDebugMode;

import '../platform/device_host_api.dart';
import 'transport.dart';
import 'web_socket_transport.dart';

/// The native app's Transport: starts the embedded host, then speaks the same
/// wire protocol over its loopback WebSocket. The outer streams stay stable
/// across [restart], so one Session object survives a Settings Save and a
/// foreground round trip.
class DeviceHostTransport implements Transport {
  DeviceHostTransport({
    required DeviceHost host,
    required Map<String, String> Function() environment,
  }) : _host = host,
       _environment = environment;

  final DeviceHost _host;
  final Map<String, String> Function() _environment;
  final StreamController<String> _text = StreamController<String>.broadcast();
  final StreamController<Uint8List> _binary =
      StreamController<Uint8List>.broadcast();
  WebSocketTransport? _inner;
  StreamSubscription<String>? _textSub;
  StreamSubscription<Uint8List>? _binarySub;
  HostEndpoint? _endpoint;
  bool _connected = false;

  /// Host lifetime operations run one at a time, so a start can never race a
  /// stop (mobile.Start rejects a second host as "already running").
  Future<void> _ops = Future.value();
  bool _pausing = false;

  /// One connect at a time, and a counter suspend/restart/close bump so a
  /// connect still awaiting the host or the handshake abandons its host
  /// instead of overwriting the fields of a newer lifetime.
  Future<void>? _connecting;
  int _connectingGeneration = -1;
  int _generation = 0;

  Future<T> _enqueue<T>(Future<T> Function() action) {
    final result = _ops.then((_) => action());
    _ops = result.then((_) {}, onError: (_) {});
    return result;
  }

  /// Where `/samples` lives while the host is running; null otherwise.
  Uri? get serverBase => _endpoint?.baseUri;

  @override
  Stream<String> get text => _text.stream;

  @override
  Stream<Uint8List> get binary => _binary.stream;

  @override
  Future<void> connect() {
    if (_connected && !_pausing) return Future.value();
    final pending = _connecting;
    if (pending != null) {
      if (_connectingGeneration == _generation) return pending;
      // A suspended connect is cleaning up; start a fresh one behind it.
      return pending.then((_) {}, onError: (_) {}).then((_) => connect());
    }
    final generation = _generation;
    late final Future<void> tracked;
    tracked =
        _enqueue(() async {
          if (_connected) return;
          await _connect(generation);
        }).whenComplete(() {
          if (identical(_connecting, tracked)) _connecting = null;
        });
    _connecting = tracked;
    _connectingGeneration = generation;
    return tracked;
  }

  Future<void> _connect(int generation) async {
    final endpoint = await _host.start(_environment());
    if (generation != _generation) {
      // The app paused while the host was starting: stop the host this connect
      // started so the next resume binds a fresh port and token.
      await _host.stop();
      return;
    }
    // The per-launch token is loopback-only and this line is compiled out of
    // release builds; the conformance harness needs it to attach over adb.
    if (kDebugMode) debugPrint('PARLEY host ${endpoint.wsUri}');
    final inner = WebSocketTransport(endpoint.wsUri);
    try {
      await inner.connect();
    } catch (_) {
      // Closing a channel whose handshake failed hangs in web_socket_channel
      // 3.0.3, so leave the dead socket alone.
      if (generation != _generation) return; // the queued suspend stops it
      await _host.stop();
      // Never surface the socket error: its message carries the loopback URI,
      // token included.
      throw const DeviceHostException('The on-device host connection failed.');
    }
    if (generation != _generation) {
      // A pause/restart/close invalidated this connect: close the socket and
      // stop the host it started instead of marking it connected (Stop is
      // idempotent).
      try {
        await inner.close();
      } catch (_) {
        // The handshake raced the host shutdown.
      }
      await _host.stop();
      return;
    }
    _endpoint = endpoint;
    _inner = inner;
    _textSub = inner.text.listen(_text.add, onError: _text.addError);
    _binarySub = inner.binary.listen(_binary.add, onError: _binary.addError);
    _connected = true;
  }

  @override
  void sendText(String message) {
    if (_connected) _inner?.sendText(message);
  }

  @override
  void sendBinary(Uint8List data) {
    if (_connected) _inner?.sendBinary(data);
  }

  /// Settings Save: new keys must reach a new host process, so stop and start.
  Future<void> restart() {
    final generation = ++_generation;
    return _enqueue(() async {
      await _disconnect();
      await _host.stop();
      if (generation != _generation) return;
      try {
        await _connect(generation);
      } catch (error) {
        // The settings route sees this throw, but a live Session only hears
        // the outer streams; without this it would stay "listening" with no
        // host behind it.
        _text.addError(error);
        rethrow;
      }
    });
  }

  /// Foreground-only lifetime: S2 stops the host when the app leaves the
  /// foreground and starts a new one on return.
  Future<void> suspend() {
    _pausing = true;
    _generation++;
    return _enqueue(() async {
      try {
        await _disconnect();
        await _host.stop();
      } finally {
        _pausing = false;
      }
    });
  }

  Future<void> resume() async {
    // A pause may still be queued; connect() then starts a new host after it.
    if (_connected && !_pausing) return;
    try {
      await connect();
    } catch (error) {
      // The Session's subscription ends the Session; a Device Host error
      // keeps its redacted cause, anything else falls back to "Connection
      // lost." so the socket URI and token never reach the banner.
      _text.addError(error);
    }
  }

  @override
  Future<void> close() {
    _generation++;
    return _enqueue(() async {
      await _disconnect();
      await _host.stop();
      await _text.close();
      await _binary.close();
    });
  }

  Future<void> _disconnect() async {
    _connected = false;
    await _textSub?.cancel();
    await _binarySub?.cancel();
    _textSub = null;
    _binarySub = null;
    final inner = _inner;
    _inner = null;
    _endpoint = null;
    if (inner != null) {
      try {
        await inner.close();
      } catch (_) {
        // The socket may already be gone; the host stop below is the real
        // cleanup.
      }
    }
  }
}
