/// Server endpoint settings. Both values are injectable at build or launch
/// time, so no host URL or token lives in the UI or in tracked files.
class AppConfig {
  const AppConfig({required this.baseUrl, this.token = ''});

  factory AppConfig.fromEnvironment() => const AppConfig(
    baseUrl: String.fromEnvironment(
      'PARLEY_URL',
      defaultValue: 'http://localhost:3000',
    ),
    token: String.fromEnvironment('PARLEY_TOKEN'),
  );

  final String baseUrl;
  final String token;

  Uri get baseUri => Uri.parse(baseUrl);

  Uri get wsUri {
    final base = baseUri;
    return base.replace(
      scheme: base.scheme == 'https' ? 'wss' : 'ws',
      path: '/ws',
      queryParameters: token.isEmpty ? null : {'token': token},
    );
  }
}
