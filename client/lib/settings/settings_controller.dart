import 'dart:convert';

import 'package:flutter/foundation.dart';

import 'presets.dart';
import 'secret_store.dart';
import 'validation.dart';

/// Non-secret settings plus the keys held only in memory and secure storage.
class Settings {
  const Settings({
    this.translatorPreset = 'groq',
    this.translatorBaseUrl = '',
    this.translatorModel = '',
    this.translatorExtraBody = '',
    this.asrProvider = 'deepgram',
    this.ttsProvider = 'system',
    this.azureRegion = '',
    this.kokoroUrl = '',
    this.llmKey = '',
    this.deepgramKey = '',
    this.paraformerKey = '',
    this.azureKey = '',
  });

  factory Settings.fromJson(Map<String, dynamic> json) => Settings(
    translatorPreset: json['translatorPreset'] as String? ?? 'groq',
    translatorBaseUrl: json['translatorBaseUrl'] as String? ?? '',
    translatorModel: json['translatorModel'] as String? ?? '',
    translatorExtraBody: json['translatorExtraBody'] as String? ?? '',
    asrProvider: json['asrProvider'] as String? ?? 'deepgram',
    ttsProvider: json['ttsProvider'] as String? ?? 'system',
    azureRegion: json['azureRegion'] as String? ?? '',
    kokoroUrl: json['kokoroUrl'] as String? ?? '',
  );

  final String translatorPreset;
  final String translatorBaseUrl;
  final String translatorModel;
  final String translatorExtraBody;
  final String asrProvider;
  final String ttsProvider;
  final String azureRegion;
  final String kokoroUrl;

  // Secrets are never serialized with the non-secret settings.
  final String llmKey;
  final String deepgramKey;
  final String paraformerKey;
  final String azureKey;

  Settings copyWith({
    String? translatorPreset,
    String? translatorBaseUrl,
    String? translatorModel,
    String? translatorExtraBody,
    String? asrProvider,
    String? ttsProvider,
    String? azureRegion,
    String? kokoroUrl,
    String? llmKey,
    String? deepgramKey,
    String? paraformerKey,
    String? azureKey,
  }) => Settings(
    translatorPreset: translatorPreset ?? this.translatorPreset,
    translatorBaseUrl: translatorBaseUrl ?? this.translatorBaseUrl,
    translatorModel: translatorModel ?? this.translatorModel,
    translatorExtraBody: translatorExtraBody ?? this.translatorExtraBody,
    asrProvider: asrProvider ?? this.asrProvider,
    ttsProvider: ttsProvider ?? this.ttsProvider,
    azureRegion: azureRegion ?? this.azureRegion,
    kokoroUrl: kokoroUrl ?? this.kokoroUrl,
    llmKey: llmKey ?? this.llmKey,
    deepgramKey: deepgramKey ?? this.deepgramKey,
    paraformerKey: paraformerKey ?? this.paraformerKey,
    azureKey: azureKey ?? this.azureKey,
  );

  /// The non-secret half, the only part written to storage as one JSON blob.
  Map<String, Object?> toJson() => {
    'translatorPreset': translatorPreset,
    'translatorBaseUrl': translatorBaseUrl,
    'translatorModel': translatorModel,
    'translatorExtraBody': translatorExtraBody,
    'asrProvider': asrProvider,
    'ttsProvider': ttsProvider,
    'azureRegion': azureRegion,
    'kokoroUrl': kokoroUrl,
  };

  /// The shape `needsLocalNetworkPermission` reads.
  Map<String, String> toMap() => {
    'mode': 'device',
    'translatorPreset': translatorPreset,
    'translatorBaseUrl': translatorBaseUrl,
    'translatorModel': translatorModel,
    'translatorExtraBody': translatorExtraBody,
    'asrProvider': asrProvider,
    'ttsProvider': ttsProvider,
    'azureRegion': azureRegion,
    'kokoroUrl': kokoroUrl,
    'serverUrl': '',
  };
}

/// One editable Settings form. A field edit only updates memory; [save]
/// validates through the shared case table, writes secure storage and only
/// then restarts the embedded host (the restart callback is wired by ticket
/// 17; null means this host has nothing to restart).
class SettingsController extends ChangeNotifier {
  SettingsController({
    required this.store,
    PresetCatalog? presets,
    this.restartHost,
  }) : presets =
           presets ?? const PresetCatalog(defaultPreset: 'groq', presets: {}) {
    values = Settings(translatorPreset: this.presets.defaultPreset);
  }

  static const storageKey = 'parley.settings.v1';
  static const llmSecretKey = 'parley.secret.llm';
  static const deepgramSecretKey = 'parley.secret.asr.deepgram';
  static const paraformerSecretKey = 'parley.secret.asr.paraformer';
  static const azureSecretKey = 'parley.secret.tts.azure';

  final SecretStore store;
  final PresetCatalog presets;
  final Future<void> Function()? restartHost;

  late Settings values;
  final Map<String, String> errors = {};
  bool saved = false;
  String? hostError;

  Future<void> load() async {
    try {
      final raw = await store.read(storageKey);
      if (raw != null) {
        values = Settings.fromJson(jsonDecode(raw) as Map<String, dynamic>);
      }
      values = values.copyWith(
        llmKey: await store.read(llmSecretKey) ?? '',
        deepgramKey: await store.read(deepgramSecretKey) ?? '',
        paraformerKey: await store.read(paraformerSecretKey) ?? '',
        azureKey: await store.read(azureSecretKey) ?? '',
      );
    } catch (error) {
      // A corrupt or unavailable keystore keeps the defaults; it must not
      // stop the app from starting, but the user has to see why their saved
      // settings are gone before a later Save overwrites them.
      hostError = 'Saved settings could not be loaded: $error';
      values = Settings(translatorPreset: presets.defaultPreset);
    }
    notifyListeners();
  }

  /// A field edit, never a save.
  void update(Settings next) {
    values = next;
    saved = false;
    notifyListeners();
  }

  Future<bool> save() async {
    errors.clear();
    hostError = null;
    var baseUrl = values.translatorBaseUrl;
    var model = values.translatorModel;
    var extraBody = values.translatorExtraBody;
    try {
      baseUrl = normalizeBaseUrl(baseUrl, preset: values.translatorPreset);
    } on SettingsValidationError catch (error) {
      errors['baseUrl'] = error.code;
    }
    try {
      model = normalizeModel(model, preset: values.translatorPreset);
    } on SettingsValidationError catch (error) {
      errors['model'] = error.code;
    }
    try {
      extraBody = normalizeExtraBody(extraBody);
    } on SettingsValidationError catch (error) {
      errors['extraBody'] = error.code;
    }
    if (errors.isNotEmpty) {
      saved = false;
      notifyListeners();
      return false;
    }
    // Store what the shared fixture says Save stores: normalized URL/model
    // and trimmed fields, never the raw text with its surrounding spaces.
    values = values.copyWith(
      translatorBaseUrl: baseUrl,
      translatorModel: model,
      translatorExtraBody: extraBody,
      azureRegion: values.azureRegion.trim(),
      kokoroUrl: values.kokoroUrl.trim(),
      llmKey: values.llmKey.trim(),
      deepgramKey: values.deepgramKey.trim(),
      paraformerKey: values.paraformerKey.trim(),
      azureKey: values.azureKey.trim(),
    );
    try {
      await store.write(storageKey, jsonEncode(values.toJson()));
      await _writeSecret(llmSecretKey, values.llmKey);
      await _writeSecret(deepgramSecretKey, values.deepgramKey);
      await _writeSecret(paraformerSecretKey, values.paraformerKey);
      await _writeSecret(azureSecretKey, values.azureKey);
    } catch (error) {
      // A keystore failure must surface as a message, not an unhandled
      // async error or a silent "Saved".
      saved = false;
      hostError = 'Settings could not be saved: $error';
      notifyListeners();
      return false;
    }
    saved = true;
    notifyListeners();
    try {
      await restartHost?.call();
    } catch (error) {
      hostError = 'Host restart failed: $error';
      notifyListeners();
      return false;
    }
    return true;
  }

  Future<void> _writeSecret(String key, String value) =>
      store.write(key, value.isEmpty ? null : value);

  String get maskedLlmKey => maskedSecret(values.llmKey);
  String get maskedDeepgramKey => maskedSecret(values.deepgramKey);
  String get maskedParaformerKey => maskedSecret(values.paraformerKey);
  String get maskedAzureKey => maskedSecret(values.azureKey);
}
