import 'package:flutter/foundation.dart' show kIsWeb;

/// Server endpoint settings. Both values are injectable at build or launch
/// time, so no host URL or token lives in the UI or in tracked files. An empty
/// PARLEY_URL on a native build selects the embedded Device Host.
class AppConfig {
  const AppConfig({required this.baseUrl, this.token = ''});

  factory AppConfig.fromEnvironment() {
    const override = String.fromEnvironment('PARLEY_URL');
    return AppConfig(
      // The web build is served same-origin by the Go server. A native build
      // with no build-time URL runs the embedded Device Host instead.
      baseUrl: override.isNotEmpty ? override : (kIsWeb ? Uri.base.origin : ''),
      token: const String.fromEnvironment('PARLEY_TOKEN'),
    );
  }

  final String baseUrl;
  final String token;

  bool get hasServer => baseUrl.isNotEmpty;

  Uri get baseUri => Uri.parse(baseUrl);

  Uri get wsUri => wsUriFor(token);

  /// The web client asks for the access token after a failed connect; it only
  /// ever rides the WebSocket query, never a UI field or a tracked file.
  Uri wsUriFor(String token) {
    final base = baseUri;
    return base.replace(
      scheme: base.scheme == 'https' ? 'wss' : 'ws',
      path: '/ws',
      queryParameters: token.isEmpty ? null : {'token': token},
    );
  }
}
