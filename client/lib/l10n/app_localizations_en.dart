// ignore: unused_import
import 'package:intl/intl.dart' as intl;

import 'app_localizations.dart';

// ignore_for_file: type=lint

/// The translations for English (`en`).
class AppLocalizationsEn extends AppLocalizations {
  AppLocalizationsEn([String locale = 'en']) : super(locale);

  @override
  String get modeListen => 'Listen';

  @override
  String get modeConverse => 'Converse';

  @override
  String startPairListen(String source, String target) {
    return 'They speak $source → I read $target';
  }

  @override
  String startPairConverse(String me, String them) {
    return 'Me $me ⇄ Them $them';
  }

  @override
  String get swapLanguages => 'Swap languages';

  @override
  String get oneWayPair => 'One-way pair — Listen only.';

  @override
  String get listenOnlyPairReason =>
      'This Language Pair translates one way only, so Converse is unavailable. Start Listen instead.';

  @override
  String get domainPacks => 'Domain Packs';

  @override
  String get noDomainPacks => 'No Domain Packs for this direction.';

  @override
  String get providersTitle => 'Providers';

  @override
  String get providerDemo => 'Demo (mock Providers)';

  @override
  String presetTrainsOnData(String preset) {
    return '$preset: its free tier may train on your prompts.';
  }

  @override
  String get startListen => 'Start Listen';

  @override
  String get startConverse => 'Start Converse';

  @override
  String get stop => 'Stop';

  @override
  String get converseMe => 'Me';

  @override
  String get converseThem => 'Them';

  @override
  String get converseYourTurn => 'Your turn';

  @override
  String get converseTapToTake => 'Tap to take the turn';

  @override
  String get converseHoldToTalk => 'Hold to talk';

  @override
  String get converseMicPaused => 'Mic paused while speaking';

  @override
  String get converseAuto => 'Auto';

  @override
  String get converseManual => 'Manual';

  @override
  String get converseAutoUnavailable =>
      'Auto Turn needs a provider and pair that passed language-tag checks. Use the pads.';

  @override
  String get captionsEmpty => 'Captions land here.';

  @override
  String get connecting => 'Connecting to the server…';

  @override
  String get connected => 'Connected';

  @override
  String get listening => 'Listening';

  @override
  String get stopped => 'Stopped';

  @override
  String get connectionFailed => 'Connection failed';

  @override
  String get retry => 'Retry';

  @override
  String get demoBanner => 'DEMO — sample audio, mock providers';

  @override
  String get demoUseMyKeys => 'Use my keys';

  @override
  String get demoMicOff =>
      'Microphone off in Demo Mode: the mock ASR only plays bundled samples.';

  @override
  String get demoMicOffShort => 'Mic off in Demo';

  @override
  String get demoSamples => 'Demo samples';

  @override
  String get sampleMissing => 'No bundled sample matches this direction.';

  @override
  String get settings => 'Settings';

  @override
  String get settingsClose => 'Close';

  @override
  String get settingsSave => 'Save';

  @override
  String get settingsSaved => 'Saved.';

  @override
  String get settingsSaveFailed => 'Fix the highlighted fields.';

  @override
  String get settingsSaveRestartsHost => 'Saving restarts the local host.';

  @override
  String get settingsSpeech => 'Speech recognition';

  @override
  String get settingsTranslation => 'Translation';

  @override
  String get settingsVoice => 'Voice';

  @override
  String get settingsPreset => 'Preset';

  @override
  String get settingsBaseUrl => 'Base URL';

  @override
  String get settingsModel => 'Model';

  @override
  String get settingsExtraBody => 'Extra request fields (JSON)';

  @override
  String get settingsAdvanced => 'Advanced';

  @override
  String get settingsApiKey => 'API key';

  @override
  String get settingsRegion => 'Region';

  @override
  String get settingsKokoroUrl => 'Kokoro URL';

  @override
  String settingsKeySaved(String mask) {
    return 'Saved — $mask';
  }

  @override
  String get settingsKeyMissing => 'No key saved';

  @override
  String get settingsKeyChange => 'Change';

  @override
  String get settingsMockNoKey => 'Mock needs no key.';

  @override
  String get settingsWebReadOnly =>
      'These Providers are configured on the server. This browser never sees keys, and there is no server-URL field.';

  @override
  String get settingsWebProviders => 'Server Providers';

  @override
  String get settingsWebPreset => 'Translation Preset';

  @override
  String get settingsWebVoice => 'Voice';

  @override
  String get settingsWebSpeech => 'Speech recognition';

  @override
  String get settingsServerDefault => 'Server default';

  @override
  String get settingsVoiceSystem => 'System voice';

  @override
  String get settingsWebNoKeys => 'Keys stay on the server.';

  @override
  String settingsLastError(String message) {
    return 'Last session: $message';
  }

  @override
  String get providerMock => 'Mock';

  @override
  String get providerSystem => 'System';

  @override
  String get providerAzure => 'Azure';

  @override
  String get providerKokoro => 'Kokoro';

  @override
  String get providerDeepgram => 'Deepgram';

  @override
  String get providerParaformer => 'Paraformer';

  @override
  String get errorNeedsBaseUrl => 'The custom Preset needs a Base URL.';

  @override
  String get errorInvalidUrl =>
      'Base URL must be a full http:// or https:// URL.';

  @override
  String get errorNotHttp => 'Base URL must start with http:// or https://.';

  @override
  String get errorPlainHttp =>
      'Plain http is allowed only for localhost or a private LAN IPv4 address; use https:// for other hosts.';

  @override
  String get errorCredentials => 'Base URL must not include credentials.';

  @override
  String get errorQuery => 'Base URL must not include a query or fragment.';

  @override
  String get errorNeedsModel => 'The custom Preset needs a Model.';

  @override
  String get errorInvalidJson => 'Extra request fields must be valid JSON.';

  @override
  String get errorNotObject => 'Extra request fields must be a JSON object.';

  @override
  String get languageEnglish => 'English';

  @override
  String get languageChinese => '中文';

  @override
  String get languageJapanese => '日本語';

  @override
  String get settingsKeyClear => 'Clear';

  @override
  String get settingsPresetCustom => 'Custom (Base URL + Model)';

  @override
  String get exitFocusMode => 'Exit focus mode';

  @override
  String get focusMode => 'Focus mode';

  @override
  String get jumpToLive => 'Jump to live';

  @override
  String get finishing => 'finishing…';

  @override
  String get stillListening => 'still listening…';

  @override
  String get skip => 'Skip';

  @override
  String get takingLonger => 'taking longer than usual';

  @override
  String get pauseCaptions => 'Pause captions';

  @override
  String get resumeCaptions => 'Resume captions';

  @override
  String get more => 'More';

  @override
  String get captionSize => 'Caption size';

  @override
  String get audioUnavailable => 'Audio unavailable — subtitles only';

  @override
  String get micPrimer => 'Parley listens only while a Session runs.';

  @override
  String get micDenied =>
      'Microphone access is off. Allow it in Settings, then Start again.';

  @override
  String get micInsecure =>
      'The microphone needs HTTPS. Open Parley over https:// or on localhost.';

  @override
  String get openSettings => 'Open Settings';

  @override
  String get listenEarphonesHint => 'Earphones recommended to avoid feedback.';

  @override
  String get listenLoudspeaker => 'Loudspeaker';

  @override
  String get listenLoudspeakerOn => 'Loudspeaker voice is on.';

  @override
  String get asrReconnecting => 'ASR disconnected — reconnecting…';

  @override
  String get errorNextAuth => 'Check the API key in Settings.';

  @override
  String get errorNextQuota =>
      'Wait for the quota to reset or top up, then try again.';

  @override
  String get errorNextNetwork =>
      'Check the network connection and the Base URL.';

  @override
  String get errorNextModel => 'Check the model name and Base URL in Settings.';

  @override
  String get errorNextCors =>
      'The Provider blocks browser requests; connect through your Server Host.';

  @override
  String get errorNextTimeout => 'Try again, or choose a lighter model.';

  @override
  String get errorNextGeneric => 'Try again; if it persists, check Settings.';

  @override
  String get onboardingPairTitle => 'Pair';

  @override
  String get onboardingPairBody => 'Choose how you will use Parley.';

  @override
  String get onboardingTryTitle => 'Try it';

  @override
  String get onboardingTryBody =>
      'Start keyless, paste your own keys, or use your Server Host.';

  @override
  String get onboardingDemo => 'Demo';

  @override
  String get onboardingDemoBody => 'Bundled sample audio, no key needed.';

  @override
  String get onboardingKeys => 'Use my keys';

  @override
  String get onboardingKeysBody => 'Paste Provider keys in Settings.';

  @override
  String get onboardingServer => 'Connect to my server';

  @override
  String get onboardingServerBody =>
      'Use the Server Host this build points at.';

  @override
  String get onboardingNoDemo => 'This server has no Demo audio.';

  @override
  String get onboardingAudioTitle => 'Check audio';

  @override
  String get onboardingAudioBody =>
      'Parley listens only while a Session runs. You will hear the translated sample through the output you choose.';

  @override
  String get onboardingEarphones => 'Earphones';

  @override
  String get onboardingLoudspeaker => 'Loudspeaker';

  @override
  String get onboardingNext => 'Next';

  @override
  String get onboardingBack => 'Back';

  @override
  String get onboardingFinish => 'Finish setup';

  @override
  String get onboardingStartDemo => 'Start the demo';

  @override
  String get onboardingTestSound => 'Play test sound';

  @override
  String get audioTestPhrase => 'Parley will speak your translations here.';

  @override
  String get audioTestFailed =>
      'The test sound failed. Check the device volume and output.';

  @override
  String get converseFirstUseTitle => 'Face to face';

  @override
  String get converseFirstUseBody =>
      'Set the phone between you. Each half is in one speaker\'s language, and the far half is turned for them. Tap a half to take the Turn; hold it to talk.';

  @override
  String get gotIt => 'Got it';

  @override
  String get localNetworkDenied =>
      'Local network access is denied. Allow it in Android Settings > Permissions, then save again.';
}
