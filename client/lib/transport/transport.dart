import 'dart:typed_data';

/// The one seam between the client and whatever serves the wire protocol:
/// a remote Server Host over `web_socket_channel`, or the on-device Go host
/// behind the same interface. The UI never sees a host URL.
abstract interface class Transport {
  /// JSON events from the server.
  Stream<String> get text;

  /// Binary frames from the server (`tts_chunk` audio).
  Stream<Uint8List> get binary;

  Future<void> connect();
  void sendText(String message);
  void sendBinary(Uint8List data);
  Future<void> close();
}
