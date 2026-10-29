import 'dart:convert';

import 'package:flutter/services.dart' show AssetBundle, rootBundle;

/// One translation Preset from `config/presets.json` (bundled as an asset).
class TranslatorPreset {
  const TranslatorPreset({
    required this.id,
    required this.baseUrl,
    required this.model,
    required this.trainsOnData,
    required this.requiresKey,
    required this.note,
  });

  factory TranslatorPreset.fromJson(String id, Map<String, dynamic> json) =>
      TranslatorPreset(
        id: id,
        baseUrl: json['baseUrl'] as String? ?? '',
        model: json['model'] as String? ?? '',
        trainsOnData: json['trainsOnData'] == true,
        requiresKey: json['requiresKey'] == true,
        note: json['note'] as String? ?? '',
      );

  final String id;
  final String baseUrl;
  final String model;
  final bool trainsOnData;
  final bool requiresKey;
  final String note;
}

class PresetCatalog {
  const PresetCatalog({required this.defaultPreset, required this.presets});

  factory PresetCatalog.fromJson(Map<String, dynamic> json) {
    final presets = (json['PRESETS'] as Map<String, dynamic>).map(
      (id, value) => MapEntry(
        id,
        TranslatorPreset.fromJson(id, value as Map<String, dynamic>),
      ),
    );
    return PresetCatalog(
      defaultPreset: json['DEFAULT_PRESET'] as String? ?? 'groq',
      presets: presets,
    );
  }

  static const assetPath = 'assets/config/presets.json';

  final String defaultPreset;
  final Map<String, TranslatorPreset> presets;

  TranslatorPreset? operator [](String id) => presets[id];

  static Future<PresetCatalog> load([AssetBundle? bundle]) async {
    final text = await (bundle ?? rootBundle).loadString(assetPath);
    return PresetCatalog.fromJson(jsonDecode(text) as Map<String, dynamic>);
  }
}
