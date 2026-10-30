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
  String get converseMe => '私';

  @override
  String get converseThem => '相手';

  @override
  String get converseYourTurn => 'あなたの番です';

  @override
  String get converseTapToTake => 'タップで交代';

  @override
  String get converseHoldToTalk => '長押しで話す';

  @override
  String get converseMicPaused => '発話中はマイクを停止';

  @override
  String get converseAuto => '自動';

  @override
  String get converseManual => '手動';

  @override
  String get converseAutoUnavailable =>
      '自動ターンには言語タグの検証に合格したプロバイダーとペアが必要です。パッドを使ってください。';

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

  @override
  String get micPrimer => 'Parley がマイクを使うのはセッション中だけです。';

  @override
  String get micDenied => 'マイクへのアクセスがオフです。設定で許可してから、もう一度開始してください。';

  @override
  String get micInsecure =>
      'マイクには HTTPS が必要です。https:// または localhost で Parley を開いてください。';

  @override
  String get openSettings => '設定を開く';

  @override
  String get listenEarphonesHint => 'ハウリング防止のためイヤホンを推奨します。';

  @override
  String get listenLoudspeaker => 'スピーカー';

  @override
  String get listenLoudspeakerOn => 'スピーカー出力がオンです。';

  @override
  String get asrReconnecting => 'ASR が切断されました — 再接続中…';

  @override
  String get errorNextAuth => '設定で API キーを確認してください。';

  @override
  String get errorNextQuota => 'クォータの回復を待つか追加してから、もう一度お試しください。';

  @override
  String get errorNextNetwork => 'ネットワーク接続とベース URL を確認してください。';

  @override
  String get errorNextModel => '設定でモデル名とベース URL を確認してください。';

  @override
  String get errorNextCors => 'プロバイダーがブラウザからのリクエストを拒否しています。サーバーホスト経由で接続してください。';

  @override
  String get errorNextTimeout => 'もう一度お試しください。軽いモデルに切り替えるのも有効です。';

  @override
  String get errorNextGeneric => 'もう一度お試しください。続く場合は設定を確認してください。';

  @override
  String get onboardingPairTitle => 'ペア';

  @override
  String get onboardingPairBody => 'Parley の使い方を選びます。';

  @override
  String get onboardingTryTitle => '試す';

  @override
  String get onboardingTryBody => 'キーなしで開始するか、自分のキーを使うか、サーバーホストに接続します。';

  @override
  String get onboardingDemo => 'デモ';

  @override
  String get onboardingDemoBody => '同梱のサンプル音声。キーは不要です。';

  @override
  String get onboardingKeys => '自分のキーを使う';

  @override
  String get onboardingKeysBody => '設定でプロバイダーのキーを貼り付けます。';

  @override
  String get onboardingServer => '自分のサーバーに接続';

  @override
  String get onboardingServerBody => 'このビルドが指定するサーバーホストを使います。';

  @override
  String get onboardingNoDemo => 'このサーバーにはデモ音声がありません。';

  @override
  String get onboardingAudioTitle => '音声を確認';

  @override
  String get onboardingAudioBody =>
      'Parley がマイクを使うのはセッション中だけです。選んだ出力で翻訳サンプルが聞こえます。';

  @override
  String get onboardingEarphones => 'イヤホン';

  @override
  String get onboardingLoudspeaker => 'スピーカー';

  @override
  String get onboardingNext => '次へ';

  @override
  String get onboardingBack => '戻る';

  @override
  String get onboardingFinish => 'セットアップを完了';

  @override
  String get onboardingStartDemo => 'デモを開始';

  @override
  String get onboardingTestSound => 'テスト音を再生';

  @override
  String get audioTestPhrase => 'Parley はここで翻訳を読み上げます。';

  @override
  String get audioTestFailed => 'テスト音を再生できませんでした。端末の音量と出力を確認してください。';

  @override
  String get converseFirstUseTitle => '対面モード';

  @override
  String get converseFirstUseBody =>
      'スマートフォンを二人の間に置きます。それぞれの半分がその話者の言語で表示され、相手側は 180 度回転します。半分をタップしてターンを取ります。長押しで話します。';

  @override
  String get gotIt => '了解';

  @override
  String get localNetworkDenied =>
      'ローカルネットワークへのアクセスが拒否されました。Android の設定 > 権限で許可してから、もう一度保存してください。';

  @override
  String get accessTokenTitle => 'アクセストークン';

  @override
  String get accessTokenPrompt =>
      'サーバーがアクセストークンを要求しました。トークンはこのタブのメモリにのみ保持されます。';

  @override
  String get accessTokenConnect => '接続';

  @override
  String get accessTokenCancel => 'キャンセル';
}
