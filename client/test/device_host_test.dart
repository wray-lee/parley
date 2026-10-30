import 'dart:convert';

import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:parley/platform/device_host_api.dart';
import 'package:parley/platform/device_host_io.dart';
import 'package:parley/settings/host_environment.dart';
import 'package:parley/settings/presets.dart';
import 'package:parley/settings/settings_controller.dart';

const catalog = PresetCatalog(
  defaultPreset: 'groq',
  presets: {
    'groq': TranslatorPreset(
      id: 'groq',
      baseUrl: 'https://api.groq.com/openai/v1',
      model: 'llama-3.3-70b-versatile',
      trainsOnData: false,
      requiresKey: true,
      note: '',
    ),
    'ollama': TranslatorPreset(
      id: 'ollama',
      baseUrl: 'http://localhost:11434/v1',
      model: 'llama3.1',
      trainsOnData: false,
      requiresKey: false,
      note: '',
    ),
    'custom': TranslatorPreset(
      id: 'custom',
      baseUrl: '',
      model: '',
      trainsOnData: false,
      requiresKey: false,
      note: '',
    ),
  },
);

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  group('hostEnvironment', () {
    test('a keyless app sends only the keyless voice choice', () {
      expect(hostEnvironment(const Settings(), catalog), {
        'TTS_PROVIDER': 'system',
      });
    });

    test('a Provider key and its provider travel under the server keys', () {
      final env = hostEnvironment(
        const Settings(llmKey: 'llm-secret', deepgramKey: 'asr-secret'),
        catalog,
      );

      expect(env, {
        'LLM_PRESET': 'groq',
        'LLM_API_KEY': 'llm-secret',
        'ASR_PROVIDER': 'deepgram',
        'DEEPGRAM_API_KEY': 'asr-secret',
        'TTS_PROVIDER': 'system',
      });
    });

    test('a keyless custom or ollama preset is still configured', () {
      expect(
        hostEnvironment(
          const Settings(
            translatorPreset: 'custom',
            translatorBaseUrl: 'http://10.0.2.2:9000/v1',
            translatorModel: 'local',
          ),
          catalog,
        ),
        {
          'LLM_PRESET': 'custom',
          'LLM_BASE_URL': 'http://10.0.2.2:9000/v1',
          'LLM_MODEL': 'local',
          'TTS_PROVIDER': 'system',
        },
      );
      expect(
        hostEnvironment(const Settings(translatorPreset: 'ollama'), catalog),
        {'LLM_PRESET': 'ollama', 'TTS_PROVIDER': 'system'},
      );
    });

    test('Paraformer, Azure and Kokoro map to their server keys', () {
      expect(
        hostEnvironment(
          const Settings(
            asrProvider: 'paraformer',
            paraformerKey: 'dash-secret',
            ttsProvider: 'azure',
            azureKey: 'azure-secret',
            azureRegion: 'japaneast',
            kokoroUrl: 'http://localhost:8880',
          ),
          catalog,
        ),
        {
          'ASR_PROVIDER': 'paraformer',
          'DASHSCOPE_API_KEY': 'dash-secret',
          'TTS_PROVIDER': 'azure',
          'AZURE_SPEECH_KEY': 'azure-secret',
          'AZURE_SPEECH_REGION': 'japaneast',
        },
      );
      expect(
        hostEnvironment(
          const Settings(
            ttsProvider: 'kokoro',
            kokoroUrl: 'http://localhost:8880',
          ),
          catalog,
        ),
        {'TTS_PROVIDER': 'kokoro', 'KOKORO_URL': 'http://localhost:8880'},
      );
    });

    test('a provider without its key falls back to Demo Mode', () {
      final env = hostEnvironment(
        const Settings(asrProvider: 'deepgram', ttsProvider: 'azure'),
        catalog,
      );
      expect(env.containsKey('ASR_PROVIDER'), isFalse);
      expect(env.containsKey('TTS_PROVIDER'), isFalse);
      expect(env, isEmpty);
    });
  });

  group('parseStartResult', () {
    test('splits the loopback URL from the per-launch token', () {
      final endpoint = parseStartResult(
        '{"url":"ws://127.0.0.1:4567/ws","token":"0123456789abcdef0123456789abcdef"}',
      );

      expect(endpoint.wsUri.scheme, 'ws');
      expect(endpoint.wsUri.host, '127.0.0.1');
      expect(endpoint.wsUri.port, 4567);
      expect(endpoint.wsUri.path, '/ws');
      expect(
        endpoint.wsUri.queryParameters['token'],
        '0123456789abcdef0123456789abcdef',
      );
      expect(endpoint.baseUri, Uri.parse('http://127.0.0.1:4567/'));
    });

    test('an unreadable result is an explicit error, never a silent URL', () {
      expect(
        () => parseStartResult('not json'),
        throwsA(isA<DeviceHostException>()),
      );
      expect(
        () => parseStartResult('{"url": 1}'),
        throwsA(isA<DeviceHostException>()),
      );
    });
  });

  test('redactSecrets removes every configured secret from a message', () {
    expect(
      redactSecrets('failed for llm-secret and asr-secret', const [
        'llm-secret',
        '',
        'asr-secret',
      ]),
      'failed for <redacted> and <redacted>',
    );
  });

  group('PlatformDeviceHost', () {
    const channel = MethodChannel('app.parley/core');
    final messenger =
        TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;

    tearDown(() => messenger.setMockMethodCallHandler(channel, null));

    test('start sends the environment and parses the endpoint', () async {
      final calls = <MethodCall>[];
      messenger.setMockMethodCallHandler(channel, (call) async {
        calls.add(call);
        return '{"url":"ws://127.0.0.1:4321/ws","token":"tok"}';
      });

      final endpoint = await PlatformDeviceHost().start({'LLM_API_KEY': 'k'});

      expect(calls.single.method, 'start');
      expect(jsonDecode(calls.single.arguments as String), {
        'LLM_API_KEY': 'k',
      });
      expect(endpoint.wsUri.toString(), 'ws://127.0.0.1:4321/ws?token=tok');
    });

    test('start redacts config values from a native error message', () async {
      messenger.setMockMethodCallHandler(channel, (call) async {
        throw PlatformException(code: 'start', message: 'boom k-secret');
      });

      await expectLater(
        PlatformDeviceHost().start({'LLM_API_KEY': 'k-secret'}),
        throwsA(
          isA<DeviceHostException>().having(
            (error) => error.message,
            'message',
            allOf(contains('boom'), isNot(contains('k-secret'))),
          ),
        ),
      );
    });

    test('a missing native host is an explicit error', () async {
      await expectLater(
        PlatformDeviceHost().start(const {}),
        throwsA(
          isA<DeviceHostException>().having(
            (error) => error.message,
            'message',
            contains('not available'),
          ),
        ),
      );
    });

    test('stop calls the channel and survives a missing native host', () async {
      final calls = <MethodCall>[];
      messenger.setMockMethodCallHandler(channel, (call) async {
        calls.add(call);
        return null;
      });

      await PlatformDeviceHost().stop();
      expect(calls.single.method, 'stop');

      messenger.setMockMethodCallHandler(channel, null);
      await PlatformDeviceHost().stop();
    });
  });
}
