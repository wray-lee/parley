package host_test

import (
	"testing"

	"github.com/wray-lee/parley/internal/host"
)

func env(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestLoadConfigDefaultsToDemoMode(t *testing.T) {
	config := host.LoadConfig(env(map[string]string{}))
	if config.Port != 3000 || config.ASRProvider != "mock" || config.TranslatorProvider != "mock" || config.TTSProvider != "mock" {
		t.Fatalf("config = %+v", config)
	}
	if config.AccessToken != "" {
		t.Fatal("token must default to empty")
	}
}

func TestLoadConfigPassesLLMExtraBodyUntouched(t *testing.T) {
	config := host.LoadConfig(env(map[string]string{"LLM_API_KEY": "k", "LLM_EXTRA_BODY": `{"top_k":20}`}))
	if config.LLM.ExtraBody != `{"top_k":20}` {
		t.Fatalf("extra body = %q", config.LLM.ExtraBody)
	}
}

func TestLoadConfigEnvironmentSelectsProvidersAndPort(t *testing.T) {
	config := host.LoadConfig(env(map[string]string{
		"PORT": "4242", "ACCESS_TOKEN": "secret", "PUBLIC_DOMAIN": "parley.example.com",
		"ASR_PROVIDER": "deepgram", "TTS_PROVIDER": "azure",
		"LLM_BASE_URL": "https://llm.example.com/v1", "LLM_MODEL": "m", "LLM_EXTRA_BODY": `{"a":1}`,
		"DEEPGRAM_API_KEY": "dg", "AZURE_SPEECH_KEY": "az", "AZURE_SPEECH_REGION": "japaneast",
		"WHISPERLIVEKIT_URL": "ws://w", "DASHSCOPE_API_KEY": "ds", "KOKORO_URL": "http://k",
		"LLM_PRESET": "groq",
	}))
	if config.Port != 4242 || config.AccessToken != "secret" || config.PublicDomain != "parley.example.com" {
		t.Fatalf("config = %+v", config)
	}
	if config.ASRProvider != "deepgram" || config.TTSProvider != "azure" || config.TranslatorProvider != "openai" {
		t.Fatalf("providers = %+v", config)
	}
	if config.LLM.Model != "m" || config.LLM.ExtraBody != `{"a":1}` || config.ASROptions.DashscopeAPIKey != "ds" || config.TTSOptions.AzureRegion != "japaneast" {
		t.Fatalf("options = %+v", config)
	}
}

func TestLoadConfigBadPortFallsBack(t *testing.T) {
	config := host.LoadConfig(env(map[string]string{"PORT": "not-a-port"}))
	if config.Port != 3000 {
		t.Fatalf("port = %d", config.Port)
	}
}
