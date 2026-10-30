import 'presets.dart';
import 'settings_controller.dart';

/// The embedded host reads the Server Host's environment keys, so the app sends
/// exactly those through `start`. Empty values are omitted: a provider without
/// its key falls back to the mock, which is what makes a keyless app Demo Mode.
Map<String, String> hostEnvironment(Settings values, PresetCatalog presets) {
  final environment = <String, String>{};
  void put(String key, String value) {
    if (value.isNotEmpty) environment[key] = value;
  }

  final preset = presets[values.translatorPreset];
  final translatorConfigured =
      values.llmKey.isNotEmpty ||
      values.translatorBaseUrl.isNotEmpty ||
      values.translatorModel.isNotEmpty ||
      values.translatorExtraBody.isNotEmpty ||
      preset?.requiresKey == false;
  if (translatorConfigured) {
    put('LLM_PRESET', values.translatorPreset);
    put('LLM_BASE_URL', values.translatorBaseUrl);
    put('LLM_MODEL', values.translatorModel);
    put('LLM_EXTRA_BODY', values.translatorExtraBody);
    put('LLM_API_KEY', values.llmKey);
  }

  switch (values.asrProvider) {
    case 'deepgram':
      if (values.deepgramKey.isNotEmpty) {
        put('ASR_PROVIDER', 'deepgram');
        put('DEEPGRAM_API_KEY', values.deepgramKey);
      }
    case 'paraformer':
      if (values.paraformerKey.isNotEmpty) {
        put('ASR_PROVIDER', 'paraformer');
        put('DASHSCOPE_API_KEY', values.paraformerKey);
      }
  }

  switch (values.ttsProvider) {
    case 'system':
      put('TTS_PROVIDER', 'system');
    case 'azure':
      if (values.azureKey.isNotEmpty) {
        put('TTS_PROVIDER', 'azure');
        put('AZURE_SPEECH_KEY', values.azureKey);
        put('AZURE_SPEECH_REGION', values.azureRegion);
      }
    case 'kokoro':
      if (values.kokoroUrl.isNotEmpty) {
        put('TTS_PROVIDER', 'kokoro');
        put('KOKORO_URL', values.kokoroUrl);
      }
  }
  return environment;
}
