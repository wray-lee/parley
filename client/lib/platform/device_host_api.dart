import 'dart:convert';

/// The Device Host endpoint: a loopback WebSocket URL that already carries the
/// per-launch token, plus the HTTP origin the Demo sample files live under.
class HostEndpoint {
  const HostEndpoint({required this.wsUri, required this.baseUri});

  final Uri wsUri;
  final Uri baseUri;
}

/// The seam over the native embedded host. The platform side only knows
/// `start`/`stop`; audio, events and TTS chunks all travel over the socket.
abstract interface class DeviceHost {
  Future<HostEndpoint> start(Map<String, String> environment);

  Future<void> stop();
}

class DeviceHostException implements Exception {
  const DeviceHostException(this.message);

  final String message;

  @override
  String toString() => message;
}

/// Parses gomobile's `{"url":...,"token":...}` without ever putting the token
/// in a message of its own.
HostEndpoint parseStartResult(String raw) {
  try {
    final json = jsonDecode(raw);
    if (json is! Map<String, dynamic>) throw const FormatException();
    final url = Uri.parse(json['url'] as String);
    final token = json['token'] as String;
    if ((url.scheme != 'ws' && url.scheme != 'wss') || token.isEmpty) {
      throw const FormatException();
    }
    return HostEndpoint(
      wsUri: url.replace(queryParameters: {'token': token}),
      baseUri: Uri(
        scheme: url.scheme == 'wss' ? 'https' : 'http',
        host: url.host,
        port: url.hasPort ? url.port : null,
        path: '/',
      ),
    );
  } on DeviceHostException {
    rethrow;
  } catch (_) {
    throw const DeviceHostException(
      'The on-device host returned an unreadable endpoint.',
    );
  }
}

/// Provider keys stay out of every message the app shows or logs.
String redactSecrets(String message, Iterable<String> secrets) {
  var redacted = message;
  for (final secret in secrets) {
    if (secret.isNotEmpty) {
      redacted = redacted.replaceAll(secret, '<redacted>');
    }
  }
  return redacted;
}
