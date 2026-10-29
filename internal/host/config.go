// Environment surface from the spec. Unset Providers mean mock, so a fresh
// clone with no .env is Demo Mode. Secrets stay in memory and are never logged.

package host

import "strconv"

// Config is the v0.0.2 environment contract of the Server Host.
type Config struct {
	Port               int
	AccessToken        string
	PublicDomain       string
	ASRProvider        string
	TranslatorProvider string
	TTSProvider        string
	LLM                LLMConfig
	ASROptions         ASROptions
	TTSOptions         TTSOptions
}

// LLMConfig carries the OpenAI-compatible translator settings.
type LLMConfig struct {
	Preset    string
	BaseURL   string
	APIKey    string
	Model     string
	ExtraBody string
}

// ASROptions carries the real ASR Provider settings.
type ASROptions struct {
	WhisperLiveKitURL string
	DeepgramAPIKey    string
	DashscopeAPIKey   string
}

// TTSOptions carries the real TTS Provider settings.
type TTSOptions struct {
	KokoroURL   string
	AzureKey    string
	AzureRegion string
}

// LoadConfig reads the environment. getenv is injected for tests.
func LoadConfig(getenv func(string) string) Config {
	hasTranslatorConfig := false
	for _, key := range []string{"LLM_PRESET", "LLM_BASE_URL", "LLM_API_KEY"} {
		if getenv(key) != "" {
			hasTranslatorConfig = true
		}
	}
	port, err := strconv.Atoi(getenv("PORT"))
	if err != nil || port < 0 {
		port = 3000
	}
	translator := "mock"
	if hasTranslatorConfig {
		translator = "openai"
	}
	asr := getenv("ASR_PROVIDER")
	if asr == "" {
		asr = "mock"
	}
	tts := getenv("TTS_PROVIDER")
	if tts == "" {
		tts = "mock"
	}
	return Config{
		Port:               port,
		AccessToken:        getenv("ACCESS_TOKEN"),
		PublicDomain:       getenv("PUBLIC_DOMAIN"),
		ASRProvider:        asr,
		TranslatorProvider: translator,
		TTSProvider:        tts,
		LLM: LLMConfig{
			Preset:    getenv("LLM_PRESET"),
			BaseURL:   getenv("LLM_BASE_URL"),
			APIKey:    getenv("LLM_API_KEY"),
			Model:     getenv("LLM_MODEL"),
			ExtraBody: getenv("LLM_EXTRA_BODY"),
		},
		ASROptions: ASROptions{
			WhisperLiveKitURL: getenv("WHISPERLIVEKIT_URL"),
			DeepgramAPIKey:    getenv("DEEPGRAM_API_KEY"),
			DashscopeAPIKey:   getenv("DASHSCOPE_API_KEY"),
		},
		TTSOptions: TTSOptions{
			KokoroURL:   getenv("KOKORO_URL"),
			AzureKey:    getenv("AZURE_SPEECH_KEY"),
			AzureRegion: getenv("AZURE_SPEECH_REGION"),
		},
	}
}
