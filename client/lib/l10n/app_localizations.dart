import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:intl/intl.dart' as intl;

import 'app_localizations_en.dart';
import 'app_localizations_ja.dart';
import 'app_localizations_zh.dart';

// ignore_for_file: type=lint

/// Callers can lookup localized strings with an instance of AppLocalizations
/// returned by `AppLocalizations.of(context)`.
///
/// Applications need to include `AppLocalizations.delegate()` in their app's
/// `localizationDelegates` list, and the locales they support in the app's
/// `supportedLocales` list. For example:
///
/// ```dart
/// import 'l10n/app_localizations.dart';
///
/// return MaterialApp(
///   localizationsDelegates: AppLocalizations.localizationsDelegates,
///   supportedLocales: AppLocalizations.supportedLocales,
///   home: MyApplicationHome(),
/// );
/// ```
///
/// ## Update pubspec.yaml
///
/// Please make sure to update your pubspec.yaml to include the following
/// packages:
///
/// ```yaml
/// dependencies:
///   # Internationalization support.
///   flutter_localizations:
///     sdk: flutter
///   intl: any # Use the pinned version from flutter_localizations
///
///   # Rest of dependencies
/// ```
///
/// ## iOS Applications
///
/// iOS applications define key application metadata, including supported
/// locales, in an Info.plist file that is built into the application bundle.
/// To configure the locales supported by your app, you’ll need to edit this
/// file.
///
/// First, open your project’s ios/Runner.xcworkspace Xcode workspace file.
/// Then, in the Project Navigator, open the Info.plist file under the Runner
/// project’s Runner folder.
///
/// Next, select the Information Property List item, select Add Item from the
/// Editor menu, then select Localizations from the pop-up menu.
///
/// Select and expand the newly-created Localizations item then, for each
/// locale your application supports, add a new item and select the locale
/// you wish to add from the pop-up menu in the Value field. This list should
/// be consistent with the languages listed in the AppLocalizations.supportedLocales
/// property.
abstract class AppLocalizations {
  AppLocalizations(String locale)
    : localeName = intl.Intl.canonicalizedLocale(locale.toString());

  final String localeName;

  static AppLocalizations of(BuildContext context) {
    return Localizations.of<AppLocalizations>(context, AppLocalizations)!;
  }

  static const LocalizationsDelegate<AppLocalizations> delegate =
      _AppLocalizationsDelegate();

  /// A list of this localizations delegate along with the default localizations
  /// delegates.
  ///
  /// Returns a list of localizations delegates containing this delegate along with
  /// GlobalMaterialLocalizations.delegate, GlobalCupertinoLocalizations.delegate,
  /// and GlobalWidgetsLocalizations.delegate.
  ///
  /// Additional delegates can be added by appending to this list in
  /// MaterialApp. This list does not have to be used at all if a custom list
  /// of delegates is preferred or required.
  static const List<LocalizationsDelegate<dynamic>> localizationsDelegates =
      <LocalizationsDelegate<dynamic>>[
        delegate,
        GlobalMaterialLocalizations.delegate,
        GlobalCupertinoLocalizations.delegate,
        GlobalWidgetsLocalizations.delegate,
      ];

  /// A list of this localizations delegate's supported locales.
  static const List<Locale> supportedLocales = <Locale>[
    Locale('en'),
    Locale('ja'),
    Locale('zh'),
  ];

  /// No description provided for @modeListen.
  ///
  /// In en, this message translates to:
  /// **'Listen'**
  String get modeListen;

  /// No description provided for @modeConverse.
  ///
  /// In en, this message translates to:
  /// **'Converse'**
  String get modeConverse;

  /// No description provided for @startPairListen.
  ///
  /// In en, this message translates to:
  /// **'They speak {source} → I read {target}'**
  String startPairListen(String source, String target);

  /// No description provided for @startPairConverse.
  ///
  /// In en, this message translates to:
  /// **'Me {me} ⇄ Them {them}'**
  String startPairConverse(String me, String them);

  /// No description provided for @swapLanguages.
  ///
  /// In en, this message translates to:
  /// **'Swap languages'**
  String get swapLanguages;

  /// No description provided for @oneWayPair.
  ///
  /// In en, this message translates to:
  /// **'One-way pair — Listen only.'**
  String get oneWayPair;

  /// No description provided for @listenOnlyPairReason.
  ///
  /// In en, this message translates to:
  /// **'This Language Pair translates one way only, so Converse is unavailable. Start Listen instead.'**
  String get listenOnlyPairReason;

  /// No description provided for @domainPacks.
  ///
  /// In en, this message translates to:
  /// **'Domain Packs'**
  String get domainPacks;

  /// No description provided for @noDomainPacks.
  ///
  /// In en, this message translates to:
  /// **'No Domain Packs for this direction.'**
  String get noDomainPacks;

  /// No description provided for @providersTitle.
  ///
  /// In en, this message translates to:
  /// **'Providers'**
  String get providersTitle;

  /// No description provided for @providerDemo.
  ///
  /// In en, this message translates to:
  /// **'Demo (mock Providers)'**
  String get providerDemo;

  /// No description provided for @presetTrainsOnData.
  ///
  /// In en, this message translates to:
  /// **'{preset}: its free tier may train on your prompts.'**
  String presetTrainsOnData(String preset);

  /// No description provided for @startListen.
  ///
  /// In en, this message translates to:
  /// **'Start Listen'**
  String get startListen;

  /// No description provided for @startConverse.
  ///
  /// In en, this message translates to:
  /// **'Start Converse'**
  String get startConverse;

  /// No description provided for @stop.
  ///
  /// In en, this message translates to:
  /// **'Stop'**
  String get stop;

  /// No description provided for @converseMe.
  ///
  /// In en, this message translates to:
  /// **'Me'**
  String get converseMe;

  /// No description provided for @converseThem.
  ///
  /// In en, this message translates to:
  /// **'Them'**
  String get converseThem;

  /// No description provided for @converseYourTurn.
  ///
  /// In en, this message translates to:
  /// **'Your turn'**
  String get converseYourTurn;

  /// No description provided for @converseTapToTake.
  ///
  /// In en, this message translates to:
  /// **'Tap to take the turn'**
  String get converseTapToTake;

  /// No description provided for @converseHoldToTalk.
  ///
  /// In en, this message translates to:
  /// **'Hold to talk'**
  String get converseHoldToTalk;

  /// No description provided for @converseMicPaused.
  ///
  /// In en, this message translates to:
  /// **'Mic paused while speaking'**
  String get converseMicPaused;

  /// No description provided for @captionsEmpty.
  ///
  /// In en, this message translates to:
  /// **'Captions land here.'**
  String get captionsEmpty;

  /// No description provided for @connecting.
  ///
  /// In en, this message translates to:
  /// **'Connecting to the server…'**
  String get connecting;

  /// No description provided for @connected.
  ///
  /// In en, this message translates to:
  /// **'Connected'**
  String get connected;

  /// No description provided for @listening.
  ///
  /// In en, this message translates to:
  /// **'Listening'**
  String get listening;

  /// No description provided for @stopped.
  ///
  /// In en, this message translates to:
  /// **'Stopped'**
  String get stopped;

  /// No description provided for @connectionFailed.
  ///
  /// In en, this message translates to:
  /// **'Connection failed'**
  String get connectionFailed;

  /// No description provided for @retry.
  ///
  /// In en, this message translates to:
  /// **'Retry'**
  String get retry;

  /// No description provided for @demoBanner.
  ///
  /// In en, this message translates to:
  /// **'DEMO — sample audio, mock providers'**
  String get demoBanner;

  /// No description provided for @demoUseMyKeys.
  ///
  /// In en, this message translates to:
  /// **'Use my keys'**
  String get demoUseMyKeys;

  /// No description provided for @demoMicOff.
  ///
  /// In en, this message translates to:
  /// **'Microphone off in Demo Mode: the mock ASR only plays bundled samples.'**
  String get demoMicOff;

  /// No description provided for @demoMicOffShort.
  ///
  /// In en, this message translates to:
  /// **'Mic off in Demo'**
  String get demoMicOffShort;

  /// No description provided for @demoSamples.
  ///
  /// In en, this message translates to:
  /// **'Demo samples'**
  String get demoSamples;

  /// No description provided for @sampleMissing.
  ///
  /// In en, this message translates to:
  /// **'No bundled sample matches this direction.'**
  String get sampleMissing;

  /// No description provided for @settings.
  ///
  /// In en, this message translates to:
  /// **'Settings'**
  String get settings;

  /// No description provided for @settingsClose.
  ///
  /// In en, this message translates to:
  /// **'Close'**
  String get settingsClose;

  /// No description provided for @settingsSave.
  ///
  /// In en, this message translates to:
  /// **'Save'**
  String get settingsSave;

  /// No description provided for @settingsSaved.
  ///
  /// In en, this message translates to:
  /// **'Saved.'**
  String get settingsSaved;

  /// No description provided for @settingsSaveFailed.
  ///
  /// In en, this message translates to:
  /// **'Fix the highlighted fields.'**
  String get settingsSaveFailed;

  /// No description provided for @settingsSaveRestartsHost.
  ///
  /// In en, this message translates to:
  /// **'Saving restarts the local host.'**
  String get settingsSaveRestartsHost;

  /// No description provided for @settingsSpeech.
  ///
  /// In en, this message translates to:
  /// **'Speech recognition'**
  String get settingsSpeech;

  /// No description provided for @settingsTranslation.
  ///
  /// In en, this message translates to:
  /// **'Translation'**
  String get settingsTranslation;

  /// No description provided for @settingsVoice.
  ///
  /// In en, this message translates to:
  /// **'Voice'**
  String get settingsVoice;

  /// No description provided for @settingsPreset.
  ///
  /// In en, this message translates to:
  /// **'Preset'**
  String get settingsPreset;

  /// No description provided for @settingsBaseUrl.
  ///
  /// In en, this message translates to:
  /// **'Base URL'**
  String get settingsBaseUrl;

  /// No description provided for @settingsModel.
  ///
  /// In en, this message translates to:
  /// **'Model'**
  String get settingsModel;

  /// No description provided for @settingsExtraBody.
  ///
  /// In en, this message translates to:
  /// **'Extra request fields (JSON)'**
  String get settingsExtraBody;

  /// No description provided for @settingsAdvanced.
  ///
  /// In en, this message translates to:
  /// **'Advanced'**
  String get settingsAdvanced;

  /// No description provided for @settingsApiKey.
  ///
  /// In en, this message translates to:
  /// **'API key'**
  String get settingsApiKey;

  /// No description provided for @settingsRegion.
  ///
  /// In en, this message translates to:
  /// **'Region'**
  String get settingsRegion;

  /// No description provided for @settingsKokoroUrl.
  ///
  /// In en, this message translates to:
  /// **'Kokoro URL'**
  String get settingsKokoroUrl;

  /// No description provided for @settingsKeySaved.
  ///
  /// In en, this message translates to:
  /// **'Saved — {mask}'**
  String settingsKeySaved(String mask);

  /// No description provided for @settingsKeyMissing.
  ///
  /// In en, this message translates to:
  /// **'No key saved'**
  String get settingsKeyMissing;

  /// No description provided for @settingsKeyChange.
  ///
  /// In en, this message translates to:
  /// **'Change'**
  String get settingsKeyChange;

  /// No description provided for @settingsMockNoKey.
  ///
  /// In en, this message translates to:
  /// **'Mock needs no key.'**
  String get settingsMockNoKey;

  /// No description provided for @settingsWebReadOnly.
  ///
  /// In en, this message translates to:
  /// **'These Providers are configured on the server. This browser never sees keys, and there is no server-URL field.'**
  String get settingsWebReadOnly;

  /// No description provided for @settingsWebProviders.
  ///
  /// In en, this message translates to:
  /// **'Server Providers'**
  String get settingsWebProviders;

  /// No description provided for @settingsWebPreset.
  ///
  /// In en, this message translates to:
  /// **'Translation Preset'**
  String get settingsWebPreset;

  /// No description provided for @settingsWebVoice.
  ///
  /// In en, this message translates to:
  /// **'Voice'**
  String get settingsWebVoice;

  /// No description provided for @settingsWebSpeech.
  ///
  /// In en, this message translates to:
  /// **'Speech recognition'**
  String get settingsWebSpeech;

  /// No description provided for @settingsServerDefault.
  ///
  /// In en, this message translates to:
  /// **'Server default'**
  String get settingsServerDefault;

  /// No description provided for @settingsVoiceSystem.
  ///
  /// In en, this message translates to:
  /// **'System voice'**
  String get settingsVoiceSystem;

  /// No description provided for @settingsWebNoKeys.
  ///
  /// In en, this message translates to:
  /// **'Keys stay on the server.'**
  String get settingsWebNoKeys;

  /// No description provided for @settingsLastError.
  ///
  /// In en, this message translates to:
  /// **'Last session: {message}'**
  String settingsLastError(String message);

  /// No description provided for @providerMock.
  ///
  /// In en, this message translates to:
  /// **'Mock'**
  String get providerMock;

  /// No description provided for @providerSystem.
  ///
  /// In en, this message translates to:
  /// **'System'**
  String get providerSystem;

  /// No description provided for @providerAzure.
  ///
  /// In en, this message translates to:
  /// **'Azure'**
  String get providerAzure;

  /// No description provided for @providerKokoro.
  ///
  /// In en, this message translates to:
  /// **'Kokoro'**
  String get providerKokoro;

  /// No description provided for @providerDeepgram.
  ///
  /// In en, this message translates to:
  /// **'Deepgram'**
  String get providerDeepgram;

  /// No description provided for @providerParaformer.
  ///
  /// In en, this message translates to:
  /// **'Paraformer'**
  String get providerParaformer;

  /// No description provided for @errorNeedsBaseUrl.
  ///
  /// In en, this message translates to:
  /// **'The custom Preset needs a Base URL.'**
  String get errorNeedsBaseUrl;

  /// No description provided for @errorInvalidUrl.
  ///
  /// In en, this message translates to:
  /// **'Base URL must be a full http:// or https:// URL.'**
  String get errorInvalidUrl;

  /// No description provided for @errorNotHttp.
  ///
  /// In en, this message translates to:
  /// **'Base URL must start with http:// or https://.'**
  String get errorNotHttp;

  /// No description provided for @errorPlainHttp.
  ///
  /// In en, this message translates to:
  /// **'Plain http is allowed only for localhost or a private LAN IPv4 address; use https:// for other hosts.'**
  String get errorPlainHttp;

  /// No description provided for @errorCredentials.
  ///
  /// In en, this message translates to:
  /// **'Base URL must not include credentials.'**
  String get errorCredentials;

  /// No description provided for @errorQuery.
  ///
  /// In en, this message translates to:
  /// **'Base URL must not include a query or fragment.'**
  String get errorQuery;

  /// No description provided for @errorNeedsModel.
  ///
  /// In en, this message translates to:
  /// **'The custom Preset needs a Model.'**
  String get errorNeedsModel;

  /// No description provided for @errorInvalidJson.
  ///
  /// In en, this message translates to:
  /// **'Extra request fields must be valid JSON.'**
  String get errorInvalidJson;

  /// No description provided for @errorNotObject.
  ///
  /// In en, this message translates to:
  /// **'Extra request fields must be a JSON object.'**
  String get errorNotObject;

  /// No description provided for @languageEnglish.
  ///
  /// In en, this message translates to:
  /// **'English'**
  String get languageEnglish;

  /// No description provided for @languageChinese.
  ///
  /// In en, this message translates to:
  /// **'中文'**
  String get languageChinese;

  /// No description provided for @languageJapanese.
  ///
  /// In en, this message translates to:
  /// **'日本語'**
  String get languageJapanese;

  /// No description provided for @settingsKeyClear.
  ///
  /// In en, this message translates to:
  /// **'Clear'**
  String get settingsKeyClear;

  /// No description provided for @settingsPresetCustom.
  ///
  /// In en, this message translates to:
  /// **'Custom (Base URL + Model)'**
  String get settingsPresetCustom;

  /// No description provided for @exitFocusMode.
  ///
  /// In en, this message translates to:
  /// **'Exit focus mode'**
  String get exitFocusMode;

  /// No description provided for @focusMode.
  ///
  /// In en, this message translates to:
  /// **'Focus mode'**
  String get focusMode;

  /// No description provided for @jumpToLive.
  ///
  /// In en, this message translates to:
  /// **'Jump to live'**
  String get jumpToLive;

  /// No description provided for @finishing.
  ///
  /// In en, this message translates to:
  /// **'finishing…'**
  String get finishing;

  /// No description provided for @stillListening.
  ///
  /// In en, this message translates to:
  /// **'still listening…'**
  String get stillListening;

  /// No description provided for @skip.
  ///
  /// In en, this message translates to:
  /// **'Skip'**
  String get skip;

  /// No description provided for @takingLonger.
  ///
  /// In en, this message translates to:
  /// **'taking longer than usual'**
  String get takingLonger;

  /// No description provided for @pauseCaptions.
  ///
  /// In en, this message translates to:
  /// **'Pause captions'**
  String get pauseCaptions;

  /// No description provided for @resumeCaptions.
  ///
  /// In en, this message translates to:
  /// **'Resume captions'**
  String get resumeCaptions;

  /// No description provided for @more.
  ///
  /// In en, this message translates to:
  /// **'More'**
  String get more;

  /// No description provided for @captionSize.
  ///
  /// In en, this message translates to:
  /// **'Caption size'**
  String get captionSize;

  /// No description provided for @audioUnavailable.
  ///
  /// In en, this message translates to:
  /// **'Audio unavailable — subtitles only'**
  String get audioUnavailable;

  /// No description provided for @micPrimer.
  ///
  /// In en, this message translates to:
  /// **'Parley listens only while a Session runs.'**
  String get micPrimer;

  /// No description provided for @micDenied.
  ///
  /// In en, this message translates to:
  /// **'Microphone access is off. Allow it in Settings, then Start again.'**
  String get micDenied;

  /// No description provided for @micInsecure.
  ///
  /// In en, this message translates to:
  /// **'The microphone needs HTTPS. Open Parley over https:// or on localhost.'**
  String get micInsecure;

  /// No description provided for @openSettings.
  ///
  /// In en, this message translates to:
  /// **'Open Settings'**
  String get openSettings;

  /// No description provided for @listenEarphonesHint.
  ///
  /// In en, this message translates to:
  /// **'Earphones recommended to avoid feedback.'**
  String get listenEarphonesHint;

  /// No description provided for @listenLoudspeaker.
  ///
  /// In en, this message translates to:
  /// **'Loudspeaker'**
  String get listenLoudspeaker;

  /// No description provided for @listenLoudspeakerOn.
  ///
  /// In en, this message translates to:
  /// **'Loudspeaker voice is on.'**
  String get listenLoudspeakerOn;

  /// No description provided for @asrReconnecting.
  ///
  /// In en, this message translates to:
  /// **'ASR disconnected — reconnecting…'**
  String get asrReconnecting;

  /// No description provided for @errorNextAuth.
  ///
  /// In en, this message translates to:
  /// **'Check the API key in Settings.'**
  String get errorNextAuth;

  /// No description provided for @errorNextQuota.
  ///
  /// In en, this message translates to:
  /// **'Wait for the quota to reset or top up, then try again.'**
  String get errorNextQuota;

  /// No description provided for @errorNextNetwork.
  ///
  /// In en, this message translates to:
  /// **'Check the network connection and the Base URL.'**
  String get errorNextNetwork;

  /// No description provided for @errorNextModel.
  ///
  /// In en, this message translates to:
  /// **'Check the model name and Base URL in Settings.'**
  String get errorNextModel;

  /// No description provided for @errorNextCors.
  ///
  /// In en, this message translates to:
  /// **'The Provider blocks browser requests; connect through your Server Host.'**
  String get errorNextCors;

  /// No description provided for @errorNextTimeout.
  ///
  /// In en, this message translates to:
  /// **'Try again, or choose a lighter model.'**
  String get errorNextTimeout;

  /// No description provided for @errorNextGeneric.
  ///
  /// In en, this message translates to:
  /// **'Try again; if it persists, check Settings.'**
  String get errorNextGeneric;

  /// No description provided for @onboardingPairTitle.
  ///
  /// In en, this message translates to:
  /// **'Pair'**
  String get onboardingPairTitle;

  /// No description provided for @onboardingPairBody.
  ///
  /// In en, this message translates to:
  /// **'Choose how you will use Parley.'**
  String get onboardingPairBody;

  /// No description provided for @onboardingTryTitle.
  ///
  /// In en, this message translates to:
  /// **'Try it'**
  String get onboardingTryTitle;

  /// No description provided for @onboardingTryBody.
  ///
  /// In en, this message translates to:
  /// **'Start keyless, paste your own keys, or use your Server Host.'**
  String get onboardingTryBody;

  /// No description provided for @onboardingDemo.
  ///
  /// In en, this message translates to:
  /// **'Demo'**
  String get onboardingDemo;

  /// No description provided for @onboardingDemoBody.
  ///
  /// In en, this message translates to:
  /// **'Bundled sample audio, no key needed.'**
  String get onboardingDemoBody;

  /// No description provided for @onboardingKeys.
  ///
  /// In en, this message translates to:
  /// **'Use my keys'**
  String get onboardingKeys;

  /// No description provided for @onboardingKeysBody.
  ///
  /// In en, this message translates to:
  /// **'Paste Provider keys in Settings.'**
  String get onboardingKeysBody;

  /// No description provided for @onboardingServer.
  ///
  /// In en, this message translates to:
  /// **'Connect to my server'**
  String get onboardingServer;

  /// No description provided for @onboardingServerBody.
  ///
  /// In en, this message translates to:
  /// **'Use the Server Host this build points at.'**
  String get onboardingServerBody;

  /// No description provided for @onboardingNoDemo.
  ///
  /// In en, this message translates to:
  /// **'This server has no Demo audio.'**
  String get onboardingNoDemo;

  /// No description provided for @onboardingAudioTitle.
  ///
  /// In en, this message translates to:
  /// **'Check audio'**
  String get onboardingAudioTitle;

  /// No description provided for @onboardingAudioBody.
  ///
  /// In en, this message translates to:
  /// **'Parley listens only while a Session runs. You will hear the translated sample through the output you choose.'**
  String get onboardingAudioBody;

  /// No description provided for @onboardingEarphones.
  ///
  /// In en, this message translates to:
  /// **'Earphones'**
  String get onboardingEarphones;

  /// No description provided for @onboardingLoudspeaker.
  ///
  /// In en, this message translates to:
  /// **'Loudspeaker'**
  String get onboardingLoudspeaker;

  /// No description provided for @onboardingNext.
  ///
  /// In en, this message translates to:
  /// **'Next'**
  String get onboardingNext;

  /// No description provided for @onboardingBack.
  ///
  /// In en, this message translates to:
  /// **'Back'**
  String get onboardingBack;

  /// No description provided for @onboardingFinish.
  ///
  /// In en, this message translates to:
  /// **'Finish setup'**
  String get onboardingFinish;

  /// No description provided for @onboardingStartDemo.
  ///
  /// In en, this message translates to:
  /// **'Start the demo'**
  String get onboardingStartDemo;

  /// No description provided for @onboardingTestSound.
  ///
  /// In en, this message translates to:
  /// **'Play test sound'**
  String get onboardingTestSound;

  /// No description provided for @audioTestPhrase.
  ///
  /// In en, this message translates to:
  /// **'Parley will speak your translations here.'**
  String get audioTestPhrase;

  /// No description provided for @audioTestFailed.
  ///
  /// In en, this message translates to:
  /// **'The test sound failed. Check the device volume and output.'**
  String get audioTestFailed;

  /// No description provided for @converseFirstUseTitle.
  ///
  /// In en, this message translates to:
  /// **'Face to face'**
  String get converseFirstUseTitle;

  /// No description provided for @converseFirstUseBody.
  ///
  /// In en, this message translates to:
  /// **'Set the phone between you. Each half is in one speaker\'s language, and the far half is turned for them. Tap a half to take the Turn; hold it to talk.'**
  String get converseFirstUseBody;

  /// No description provided for @gotIt.
  ///
  /// In en, this message translates to:
  /// **'Got it'**
  String get gotIt;

  /// No description provided for @localNetworkDenied.
  ///
  /// In en, this message translates to:
  /// **'Local network access is denied. Allow it in Android Settings > Permissions, then save again.'**
  String get localNetworkDenied;
}

class _AppLocalizationsDelegate
    extends LocalizationsDelegate<AppLocalizations> {
  const _AppLocalizationsDelegate();

  @override
  Future<AppLocalizations> load(Locale locale) {
    return SynchronousFuture<AppLocalizations>(lookupAppLocalizations(locale));
  }

  @override
  bool isSupported(Locale locale) =>
      <String>['en', 'ja', 'zh'].contains(locale.languageCode);

  @override
  bool shouldReload(_AppLocalizationsDelegate old) => false;
}

AppLocalizations lookupAppLocalizations(Locale locale) {
  // Lookup logic when only language code is specified.
  switch (locale.languageCode) {
    case 'en':
      return AppLocalizationsEn();
    case 'ja':
      return AppLocalizationsJa();
    case 'zh':
      return AppLocalizationsZh();
  }

  throw FlutterError(
    'AppLocalizations.delegate failed to load unsupported locale "$locale". This is likely '
    'an issue with the localizations generation tool. Please file an issue '
    'on GitHub with a reproducible sample app and the gen-l10n configuration '
    'that was used.',
  );
}
