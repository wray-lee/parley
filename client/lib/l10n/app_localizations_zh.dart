// ignore: unused_import
import 'package:intl/intl.dart' as intl;

import 'app_localizations.dart';

// ignore_for_file: type=lint

/// The translations for Chinese (`zh`).
class AppLocalizationsZh extends AppLocalizations {
  AppLocalizationsZh([String locale = 'zh']) : super(locale);

  @override
  String get modeListen => '聆听';

  @override
  String get modeConverse => '对话';

  @override
  String startPairListen(String source, String target) {
    return '对方说 $source → 我读 $target';
  }

  @override
  String startPairConverse(String me, String them) {
    return '我 $me ⇄ 对方 $them';
  }

  @override
  String get swapLanguages => '交换语言';

  @override
  String get oneWayPair => '单向语言对 — 仅聆听';

  @override
  String get listenOnlyPairReason => '该语言对只能单向翻译，因此无法使用对话模式。请改用聆听。';

  @override
  String get domainPacks => '领域包';

  @override
  String get noDomainPacks => '该方向没有领域包。';

  @override
  String get providersTitle => '服务提供方';

  @override
  String get providerDemo => '演示（模拟服务）';

  @override
  String presetTrainsOnData(String preset) {
    return '$preset：其免费套餐可能会使用你的提示词进行训练。';
  }

  @override
  String get startListen => '开始聆听';

  @override
  String get startConverse => '开始对话';

  @override
  String get stop => '停止';

  @override
  String get converseMe => '我';

  @override
  String get converseThem => '对方';

  @override
  String get converseYourTurn => '轮到你';

  @override
  String get converseTapToTake => '点按轮到我';

  @override
  String get converseHoldToTalk => '长按说话';

  @override
  String get converseMicPaused => '说话时麦克风暂停';

  @override
  String get converseAuto => '自动';

  @override
  String get converseManual => '手动';

  @override
  String get converseAutoUnavailable => '自动轮换需要语言标签验证通过的服务商和语言对。请使用两侧按钮。';

  @override
  String get captionsEmpty => '字幕会显示在这里。';

  @override
  String get connecting => '正在连接服务器…';

  @override
  String get connected => '已连接';

  @override
  String get listening => '聆听中';

  @override
  String get stopped => '已停止';

  @override
  String get connectionFailed => '连接失败';

  @override
  String get retry => '重试';

  @override
  String get demoBanner => '演示 — 示例音频，模拟服务';

  @override
  String get demoUseMyKeys => '使用我的密钥';

  @override
  String get demoMicOff => '演示模式已关闭麦克风：模拟 ASR 只播放内置示例。';

  @override
  String get demoMicOffShort => '演示中麦克风已关闭';

  @override
  String get demoSamples => '演示示例';

  @override
  String get sampleMissing => '没有与该方向匹配的内置示例。';

  @override
  String get settings => '设置';

  @override
  String get settingsClose => '关闭';

  @override
  String get settingsSave => '保存';

  @override
  String get settingsSaved => '已保存。';

  @override
  String get settingsSaveFailed => '请修正高亮的字段。';

  @override
  String get settingsSaveRestartsHost => '保存后会重启本地主机。';

  @override
  String get settingsSpeech => '语音识别';

  @override
  String get settingsTranslation => '翻译';

  @override
  String get settingsVoice => '语音';

  @override
  String get settingsPreset => '预设';

  @override
  String get settingsBaseUrl => '基础 URL';

  @override
  String get settingsModel => '模型';

  @override
  String get settingsExtraBody => '额外请求字段（JSON）';

  @override
  String get settingsAdvanced => '高级';

  @override
  String get settingsApiKey => 'API 密钥';

  @override
  String get settingsRegion => '区域';

  @override
  String get settingsKokoroUrl => 'Kokoro URL';

  @override
  String settingsKeySaved(String mask) {
    return '已保存 — $mask';
  }

  @override
  String get settingsKeyMissing => '未保存密钥';

  @override
  String get settingsKeyChange => '更改';

  @override
  String get settingsMockNoKey => '模拟无需密钥。';

  @override
  String get settingsWebReadOnly => '这些服务由服务器配置。此浏览器不会看到密钥，也没有服务器 URL 字段。';

  @override
  String get settingsWebProviders => '服务器服务';

  @override
  String get settingsWebPreset => '翻译预设';

  @override
  String get settingsWebVoice => '语音';

  @override
  String get settingsWebSpeech => '语音识别';

  @override
  String get settingsServerDefault => '服务器默认';

  @override
  String get settingsVoiceSystem => '系统语音';

  @override
  String get settingsWebNoKeys => '密钥保留在服务器上。';

  @override
  String settingsLastError(String message) {
    return '上次会话：$message';
  }

  @override
  String get providerMock => '模拟';

  @override
  String get providerSystem => '系统';

  @override
  String get providerAzure => 'Azure';

  @override
  String get providerKokoro => 'Kokoro';

  @override
  String get providerDeepgram => 'Deepgram';

  @override
  String get providerParaformer => 'Paraformer';

  @override
  String get errorNeedsBaseUrl => '自定义预设需要基础 URL。';

  @override
  String get errorInvalidUrl => '基础 URL 必须是完整的 http:// 或 https:// URL。';

  @override
  String get errorNotHttp => '基础 URL 必须以 http:// 或 https:// 开头。';

  @override
  String get errorPlainHttp =>
      '明文 http 仅允许用于 localhost 或私有局域网 IPv4 地址；其他主机请使用 https://。';

  @override
  String get errorCredentials => '基础 URL 不得包含凭据。';

  @override
  String get errorQuery => '基础 URL 不得包含查询参数或片段。';

  @override
  String get errorNeedsModel => '自定义预设需要模型。';

  @override
  String get errorInvalidJson => '额外请求字段必须是有效的 JSON。';

  @override
  String get errorNotObject => '额外请求字段必须是 JSON 对象。';

  @override
  String get languageEnglish => '英语';

  @override
  String get languageChinese => '中文';

  @override
  String get languageJapanese => '日语';

  @override
  String get settingsKeyClear => '清除';

  @override
  String get settingsPresetCustom => '自定义（基础 URL + 模型）';

  @override
  String get exitFocusMode => '退出专注模式';

  @override
  String get focusMode => '专注模式';

  @override
  String get jumpToLive => '跳转到最新';

  @override
  String get finishing => '正在收尾…';

  @override
  String get stillListening => '仍在聆听…';

  @override
  String get skip => '跳过';

  @override
  String get takingLonger => '比平时更久';

  @override
  String get pauseCaptions => '暂停字幕';

  @override
  String get resumeCaptions => '继续字幕';

  @override
  String get more => '更多';

  @override
  String get captionSize => '字幕大小';

  @override
  String get audioUnavailable => '音频不可用 — 仅显示字幕';

  @override
  String get micPrimer => 'Parley 只在会话进行时使用麦克风。';

  @override
  String get micDenied => '麦克风权限已关闭。请在设置中允许，然后重新开始。';

  @override
  String get micInsecure => '麦克风需要 HTTPS。请通过 https:// 或 localhost 打开 Parley。';

  @override
  String get openSettings => '打开设置';

  @override
  String get listenEarphonesHint => '建议使用耳机以避免啸叫。';

  @override
  String get listenLoudspeaker => '扬声器';

  @override
  String get listenLoudspeakerOn => '扬声器输出已开启。';

  @override
  String get asrReconnecting => 'ASR 已断开 — 正在重连…';

  @override
  String get errorNextAuth => '请在设置中检查 API 密钥。';

  @override
  String get errorNextQuota => '等待配额恢复或充值，然后重试。';

  @override
  String get errorNextNetwork => '请检查网络连接和基础 URL。';

  @override
  String get errorNextModel => '请在设置中检查模型名称和基础 URL。';

  @override
  String get errorNextCors => '服务商拒绝了浏览器请求，请通过服务器主机连接。';

  @override
  String get errorNextTimeout => '请重试，或换用更轻量的模型。';

  @override
  String get errorNextGeneric => '请重试；若仍然失败，请检查设置。';

  @override
  String get onboardingPairTitle => '配对';

  @override
  String get onboardingPairBody => '选择你将如何使用 Parley。';

  @override
  String get onboardingTryTitle => '试用';

  @override
  String get onboardingTryBody => '无需密钥直接开始，使用自己的密钥，或连接服务器主机。';

  @override
  String get onboardingDemo => '演示';

  @override
  String get onboardingDemoBody => '内置示例音频，无需密钥。';

  @override
  String get onboardingKeys => '使用我的密钥';

  @override
  String get onboardingKeysBody => '在设置中粘贴服务商密钥。';

  @override
  String get onboardingServer => '连接我的服务器';

  @override
  String get onboardingServerBody => '使用此构建所指向的服务器主机。';

  @override
  String get onboardingNoDemo => '此服务器没有演示音频。';

  @override
  String get onboardingAudioTitle => '检查音频';

  @override
  String get onboardingAudioBody => 'Parley 只在会话进行时使用麦克风。你将通过所选输出听到翻译示例。';

  @override
  String get onboardingEarphones => '耳机';

  @override
  String get onboardingLoudspeaker => '扬声器';

  @override
  String get onboardingNext => '下一步';

  @override
  String get onboardingBack => '返回';

  @override
  String get onboardingFinish => '完成设置';

  @override
  String get onboardingStartDemo => '开始演示';

  @override
  String get onboardingTestSound => '播放测试音';

  @override
  String get audioTestPhrase => 'Parley 会在这里朗读你的翻译。';

  @override
  String get audioTestFailed => '测试音播放失败。请检查设备音量和输出。';

  @override
  String get converseFirstUseTitle => '面对面';

  @override
  String get converseFirstUseBody =>
      '把手机放在两人之间。每一半显示该说话者的语言，远端的一半会旋转 180 度。轻点一半取得话轮，按住说话。';

  @override
  String get gotIt => '知道了';

  @override
  String get localNetworkDenied => '本地网络访问被拒绝。请在 Android 设置 > 权限中允许，然后再次保存。';
}
