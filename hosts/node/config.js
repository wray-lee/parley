// Environment surface from the spec. Unset Providers mean mock, so a fresh
// clone with no .env is Demo Mode.

export function loadConfig(env = process.env) {
  const hasTranslatorConfig = Boolean(env.LLM_PRESET || env.LLM_BASE_URL || env.LLM_API_KEY);
  return {
    port: Number.parseInt(env.PORT ?? '3000', 10),
    accessToken: env.ACCESS_TOKEN ?? '',
    publicDomain: env.PUBLIC_DOMAIN ?? '',
    asr: env.ASR_PROVIDER ?? 'mock',
    translator: hasTranslatorConfig ? 'openai' : 'mock',
    tts: env.TTS_PROVIDER ?? 'mock',
    llm: {
      preset: env.LLM_PRESET ?? '',
      baseUrl: env.LLM_BASE_URL ?? '',
      apiKey: env.LLM_API_KEY ?? '',
      model: env.LLM_MODEL ?? '',
      extraBody: env.LLM_EXTRA_BODY ?? '',
    },
    asrOptions: {
      whisperlivekitUrl: env.WHISPERLIVEKIT_URL ?? '',
      deepgramApiKey: env.DEEPGRAM_API_KEY ?? '',
      dashscopeApiKey: env.DASHSCOPE_API_KEY ?? '',
    },
    ttsOptions: {
      kokoroUrl: env.KOKORO_URL ?? '',
      azureKey: env.AZURE_SPEECH_KEY ?? '',
      azureRegion: env.AZURE_SPEECH_REGION ?? '',
    },
  };
}
