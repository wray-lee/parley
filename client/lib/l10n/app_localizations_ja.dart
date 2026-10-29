// ignore: unused_import
import 'package:intl/intl.dart' as intl;

import 'app_localizations.dart';

// ignore_for_file: type=lint

/// The translations for Japanese (`ja`).
class AppLocalizationsJa extends AppLocalizations {
  AppLocalizationsJa([String locale = 'ja']) : super(locale);

  @override
  String get modeListen => '聞く';

  @override
  String get modeConverse => '会話';

  @override
  String startPairListen(String source, String target) {
    return '相手は $source を話し、私は $target を読みます';
  }

  @override
  String startPairConverse(String me, String them) {
    return '私 $me ⇄ 相手 $them';
  }

  @override
  String get swapLanguages => '言語を入れ替える';

  @override
  String get oneWayPair => '一方通行のペア — 聞くのみ';

  @override
  String get listenOnlyPairReason => 'この言語ペアは一方通行のため、会話は使えません。聞くを使ってください。';

  @override
  String get domainPacks => 'ドメインパック';

  @override
  String get noDomainPacks => 'この方向のドメインパックはありません。';

  @override
  String get providersTitle => 'プロバイダー';

  @override
  String get providerDemo => 'デモ（モックプロバイダー）';

  @override
  String presetTrainsOnData(String preset) {
    return '$preset：無料枠ではプロンプトが学習に使われる可能性があります。';
  }

  @override
  String get startListen => '聞くを開始';

  @override
  String get startConverse => '会話を開始';

  @override
  String get stop => '停止';

  @override
  String get converseNotInBuild => '対面の会話画面はこのビルドにはまだ含まれていません。聞くを使ってください。';

  @override
  String get captionsEmpty => '字幕がここに表示されます。';

  @override
  String get connecting => 'サーバーに接続しています…';

  @override
  String get connected => '接続済み';

  @override
  String get listening => '聞き取り中';

  @override
  String get stopped => '停止済み';

  @override
  String get connectionFailed => '接続に失敗しました';

  @override
  String get retry => '再試行';

  @override
  String get demoBanner => 'デモ — サンプル音声、モックプロバイダー';

  @override
  String get demoUseMyKeys => '自分のキーを使う';

  @override
  String get demoMicOff => 'デモモードではマイクはオフです。モック ASR は同梱サンプルのみを再生します。';

  @override
  String get demoMicOffShort => 'デモではマイクオフ';

  @override
  String get demoSamples => 'デモサンプル';

  @override
  String get sampleMissing => 'この方向に合う同梱サンプルがありません。';

  @override
  String get settings => '設定';

  @override
  String get settingsClose => '閉じる';

  @override
  String get settingsSave => '保存';

  @override
  String get settingsSaved => '保存しました。';

  @override
  String get settingsSaveFailed => 'ハイライトされた項目を修正してください。';

  @override
  String get settingsSaveRestartsHost => '保存するとローカルホストが再起動します。';

  @override
  String get settingsSpeech => '音声認識';

  @override
  String get settingsTranslation => '翻訳';

  @override
  String get settingsVoice => '音声';

  @override
  String get settingsPreset => 'プリセット';

  @override
  String get settingsBaseUrl => 'ベース URL';

  @override
  String get settingsModel => 'モデル';

  @override
  String get settingsExtraBody => '追加リクエストフィールド（JSON）';

  @override
  String get settingsAdvanced => '詳細';

  @override
  String get settingsApiKey => 'API キー';

  @override
  String get settingsRegion => 'リージョン';

  @override
  String get settingsKokoroUrl => 'Kokoro URL';

  @override
  String settingsKeySaved(String mask) {
    return '保存済み — $mask';
  }

  @override
  String get settingsKeyMissing => 'キー未保存';

  @override
  String get settingsKeyChange => '変更';

  @override
  String get settingsMockNoKey => 'モックにはキーが不要です。';

  @override
  String get settingsWebReadOnly =>
      'これらのプロバイダーはサーバーで設定されています。このブラウザーはキーを受け取らず、サーバー URL 欄もありません。';

  @override
  String get settingsWebProviders => 'サーバーのプロバイダー';

  @override
  String get settingsWebPreset => '翻訳プリセット';

  @override
  String get settingsWebVoice => '音声';

  @override
  String get settingsWebSpeech => '音声認識';

  @override
  String get settingsServerDefault => 'サーバーの既定値';

  @override
  String get settingsVoiceSystem => 'システム音声';

  @override
  String get settingsWebNoKeys => 'キーはサーバーに留まります。';

  @override
  String settingsLastError(String message) {
    return '前回のセッション：$message';
  }

  @override
  String get providerMock => 'モック';

  @override
  String get providerSystem => 'システム';

  @override
  String get providerAzure => 'Azure';

  @override
  String get providerKokoro => 'Kokoro';

  @override
  String get providerDeepgram => 'Deepgram';

  @override
  String get providerParaformer => 'Paraformer';

  @override
  String get errorNeedsBaseUrl => 'カスタムプリセットにはベース URL が必要です。';

  @override
  String get errorInvalidUrl =>
      'ベース URL は完全な http:// または https:// URL である必要があります。';

  @override
  String get errorNotHttp => 'ベース URL は http:// または https:// で始まる必要があります。';

  @override
  String get errorPlainHttp =>
      '平文 http は localhost またはプライベート LAN の IPv4 アドレスにのみ許可されます。それ以外のホストには https:// を使ってください。';

  @override
  String get errorCredentials => 'ベース URL に認証情報を含めることはできません。';

  @override
  String get errorQuery => 'ベース URL にクエリやフラグメントを含めることはできません。';

  @override
  String get errorNeedsModel => 'カスタムプリセットにはモデルが必要です。';

  @override
  String get errorInvalidJson => '追加リクエストフィールドは有効な JSON である必要があります。';

  @override
  String get errorNotObject => '追加リクエストフィールドは JSON オブジェクトである必要があります。';

  @override
  String get languageEnglish => '英語';

  @override
  String get languageChinese => '中国語';

  @override
  String get languageJapanese => '日本語';

  @override
  String get settingsKeyClear => '消去';

  @override
  String get settingsPresetCustom => 'カスタム（ベース URL + モデル）';

  @override
  String get exitFocusMode => '集中モードを終了';

  @override
  String get focusMode => '集中モード';

  @override
  String get jumpToLive => '最新へ移動';

  @override
  String get finishing => '仕上げ中…';

  @override
  String get stillListening => 'まだ聞き取り中…';

  @override
  String get skip => 'スキップ';

  @override
  String get takingLonger => '通常より時間がかかっています';

  @override
  String get pauseCaptions => '字幕を一時停止';

  @override
  String get resumeCaptions => '字幕を再開';

  @override
  String get more => 'その他';

  @override
  String get captionSize => '字幕サイズ';

  @override
  String get audioUnavailable => '音声を利用できません — 字幕のみ表示';
}
