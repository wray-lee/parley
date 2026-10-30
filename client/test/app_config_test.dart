import 'package:flutter_test/flutter_test.dart';
import 'package:parley/app_config.dart';

void main() {
  test('the WebSocket URI carries the access token the web prompt got', () {
    const config = AppConfig(baseUrl: 'https://parley.example');
    expect(config.wsUri.toString(), 'wss://parley.example/ws');
    expect(
      config.wsUriFor('secret').toString(),
      'wss://parley.example/ws?token=secret',
    );
    expect(config.wsUriFor('').queryParameters, isEmpty);
    expect(config.wsUriFor('a b').queryParameters['token'], 'a b');
  });
}
