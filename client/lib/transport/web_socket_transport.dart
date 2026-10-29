import 'dart:async';
import 'dart:typed_data';

import 'package:web_socket_channel/web_socket_channel.dart';

import 'transport.dart';

/// WebSocket transport for a Server Host, a web origin or a loopback Device
/// Host. The URL (and its token) is built by the caller; nothing here knows a
/// host name.
class WebSocketTransport implements Transport {
  WebSocketTransport(this.uri) : _channel = WebSocketChannel.connect(uri);

  final Uri uri;
  final WebSocketChannel _channel;
  late final Stream<dynamic> _events = _channel.stream.asBroadcastStream();

  @override
  Stream<String> get text => _events.where((e) => e is String).cast<String>();

  @override
  Stream<Uint8List> get binary => _events
      .where((e) => e is List<int>)
      .map((e) => Uint8List.fromList(e as List<int>));

  @override
  Future<void> connect() => _channel.ready;

  @override
  void sendText(String message) => _channel.sink.add(message);

  @override
  void sendBinary(Uint8List data) => _channel.sink.add(data);

  @override
  Future<void> close() => _channel.sink.close();
}
