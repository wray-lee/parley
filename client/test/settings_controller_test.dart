import 'dart:convert';
import 'dart:io';

import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:parley/settings/presets.dart';
import 'package:parley/settings/secret_store.dart';
import 'package:parley/settings/settings_controller.dart';

PresetCatalog catalog() => const PresetCatalog(
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
    'custom': TranslatorPreset(
      id: 'custom',
      baseUrl: '',
      model: '',
      trainsOnData: false,
      requiresKey: true,
      note: '',
    ),
  },
);

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  test('the bundled presets asset matches config/presets.json', () async {
    final asset = await rootBundle.loadString(PresetCatalog.assetPath);
    final source = File('../config/presets.json').readAsStringSync();
    expect(jsonDecode(asset), jsonDecode(source));
  });

  test('a field edit never restarts the host; Save restarts it once', () async {
    var restarts = 0;
    final store = MemorySecretStore();
    final controller = SettingsController(
      store: store,
      presets: catalog(),
      restartHost: () async => restarts++,
    );
    await controller.load();

    controller.update(
      controller.values.copyWith(
        translatorPreset: 'custom',
        translatorBaseUrl: 'https://gateway.example.com/v1',
        translatorModel: 'my-model',
      ),
    );
    expect(restarts, 0);

    expect(await controller.save(), isTrue);
    expect(restarts, 1);
    expect(await store.read(SettingsController.storageKey), isNotNull);
  });

  test('an invalid Base URL blocks Save, reports the shared code and restarts nothing', () async {
    var restarts = 0;
    final controller = SettingsController(
      store: MemorySecretStore(),
      presets: catalog(),
      restartHost: () async => restarts++,
    );
    await controller.load();
    controller.update(
      controller.values.copyWith(
        translatorPreset: 'custom',
        translatorBaseUrl: 'gateway.example.com/v1',
        translatorModel: '',
      ),
    );

    expect(await controller.save(), isFalse);
    expect(controller.errors['baseUrl'], 'invalidUrl');
    expect(controller.errors['model'], 'needsModel');
    expect(restarts, 0);
  });

  test(
    'keys go to the store, stay masked and never enter the settings JSON',
    () async {
      final logs = <String>[];
      final original = debugPrint;
      debugPrint = (String? message, {int? wrapWidth}) {
        if (message != null) logs.add(message);
      };
      addTearDown(() => debugPrint = original);

      final store = MemorySecretStore();
      final controller = SettingsController(store: store, presets: catalog());
      await controller.load();
      controller.update(
        controller.values.copyWith(
          llmKey: 'sk-llm-1234abcd',
          deepgramKey: 'dg-5678efgh',
          azureKey: 'az-9999wxyz',
        ),
      );
      expect(await controller.save(), isTrue);

      expect(
        await store.read(SettingsController.llmSecretKey),
        'sk-llm-1234abcd',
      );
      expect(
        await store.read(SettingsController.deepgramSecretKey),
        'dg-5678efgh',
      );
      expect(
        await store.read(SettingsController.azureSecretKey),
        'az-9999wxyz',
      );

      final blob = await store.read(SettingsController.storageKey) ?? '';
      expect(blob.contains('1234abcd'), isFalse);
      expect(blob.contains('5678efgh'), isFalse);

      expect(controller.maskedLlmKey, '••••abcd');
      expect(controller.maskedDeepgramKey, '••••efgh');
      expect(controller.maskedLlmKey.contains('sk-llm'), isFalse);
      for (final entry in logs) {
        expect(entry.contains('1234abcd'), isFalse);
        expect(entry.contains('5678efgh'), isFalse);
      }
    },
  );

  test(
    'load restores non-secret settings and secrets from the store',
    () async {
      final store = MemorySecretStore({
        SettingsController.storageKey: jsonEncode({
          'translatorPreset': 'custom',
          'translatorBaseUrl': 'https://gateway.example.com/v1',
          'translatorModel': 'm',
          'translatorExtraBody': '{"top_k":20}',
          'asrProvider': 'paraformer',
          'ttsProvider': 'kokoro',
          'azureRegion': '',
          'kokoroUrl': 'http://localhost:8880',
        }),
        SettingsController.llmSecretKey: 'sk-llm-abcd',
      });
      final controller = SettingsController(store: store, presets: catalog());
      await controller.load();

      expect(controller.values.translatorPreset, 'custom');
      expect(
        controller.values.translatorBaseUrl,
        'https://gateway.example.com/v1',
      );
      expect(controller.values.asrProvider, 'paraformer');
      expect(controller.values.ttsProvider, 'kokoro');
      expect(controller.values.llmKey, 'sk-llm-abcd');
      expect(controller.maskedLlmKey, '••••abcd');
    },
  );

  test('corrupt storage falls back to defaults and says so', () async {
    final controller = SettingsController(
      store: MemorySecretStore({SettingsController.storageKey: 'not json'}),
      presets: catalog(),
    );
    await controller.load();
    expect(controller.values.translatorPreset, 'groq');
    expect(controller.hostError, contains('could not be loaded'));
  });

  test('Save stores trimmed and normalized values, including keys', () async {
    final store = MemorySecretStore();
    final controller = SettingsController(store: store, presets: catalog());
    await controller.load();
    controller.update(
      controller.values.copyWith(
        translatorPreset: 'custom',
        translatorBaseUrl: '  https://gateway.example.com/v1/  ',
        translatorModel: '  my-model  ',
        translatorExtraBody: '  {"temperature": 0}  ',
        azureRegion: '  eastasia  ',
        kokoroUrl: '  http://127.0.0.1:8880  ',
        llmKey: '  sk-llm-1234abcd  ',
        deepgramKey: '  dg-5678efgh  ',
        paraformerKey: '  pf-0000  ',
        azureKey: '  az-9999wxyz  ',
      ),
    );

    expect(await controller.save(), isTrue);
    final blob = jsonDecode(
      (await store.read(SettingsController.storageKey))!,
    ) as Map<String, dynamic>;
    expect(blob['translatorBaseUrl'], 'https://gateway.example.com/v1');
    expect(blob['translatorModel'], 'my-model');
    expect(blob['translatorExtraBody'], '{"temperature": 0}');
    expect(blob['azureRegion'], 'eastasia');
    expect(blob['kokoroUrl'], 'http://127.0.0.1:8880');
    expect(
      controller.values.translatorBaseUrl,
      'https://gateway.example.com/v1',
    );
    expect(
      await store.read(SettingsController.llmSecretKey),
      'sk-llm-1234abcd',
    );
    expect(
      await store.read(SettingsController.deepgramSecretKey),
      'dg-5678efgh',
    );
    expect(await store.read(SettingsController.azureSecretKey), 'az-9999wxyz');
  });

  test(
    'a keystore write failure is a message, not an unhandled error',
    () async {
      final controller = SettingsController(
        store: _FailingStore(),
        presets: catalog(),
      );
      await controller.load();
      controller.update(controller.values.copyWith(llmKey: 'sk-llm-1234abcd'));

      expect(await controller.save(), isFalse);
      expect(controller.hostError, contains('could not be saved'));
      expect(controller.hostError, contains('keystore locked'));
    },
  );

  test('an unreadable keystore reports the fallback to defaults', () async {
    final controller = SettingsController(
      store: _UnreadableStore(),
      presets: catalog(),
    );
    await controller.load();

    expect(controller.values.translatorPreset, 'groq');
    expect(controller.hostError, contains('could not be loaded'));
  });
}

class _FailingStore extends MemorySecretStore {
  @override
  Future<void> write(String key, String? value) async {
    throw StateError('keystore locked');
  }
}

class _UnreadableStore extends MemorySecretStore {
  @override
  Future<String?> read(String key) async {
    throw StateError('keystore locked');
  }
}
