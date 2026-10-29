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
  String get converseNotInBuild => '面对面对话界面尚未包含在此版本中；请使用聆听。';

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
}
