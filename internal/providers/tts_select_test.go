package providers_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wray-lee/parley/internal/providers"
)

func bundledVoices(t *testing.T) providers.Voices {
	t.Helper()
	voices, err := providers.LoadVoices(os.DirFS(filepath.Join("..", "..")))
	if err != nil {
		t.Fatalf("LoadVoices: %v", err)
	}
	return voices
}

// Mirrors tests/contract/tts-loader.test.js: TTS_PROVIDER names resolve by
// convention, azure asks for its env key and cosyvoice fails clearly.
func TestSelectTTSResolvesTheAdaptersByConvention(t *testing.T) {
	voices := bundledVoices(t)

	system, err := providers.SelectTTS("system", providers.TTSOptions{}, voices)
	if err != nil || system.Name() != "system" {
		t.Fatalf("system = %v, %v", system, err)
	}

	edge, err := providers.SelectTTS("edge", providers.TTSOptions{}, voices)
	if err != nil || edge.Name() != "edge" {
		t.Fatalf("edge = %v, %v", edge, err)
	}

	kokoro, err := providers.SelectTTS("kokoro", providers.TTSOptions{KokoroURL: "http://127.0.0.1:8880"}, voices)
	if err != nil || kokoro.Name() != "kokoro" {
		t.Fatalf("kokoro = %v, %v", kokoro, err)
	}

	if _, err := providers.SelectTTS("azure", providers.TTSOptions{}, voices); err == nil ||
		!strings.Contains(err.Error(), "AZURE_SPEECH_KEY") {
		t.Fatalf("azure without a key = %v", err)
	}

	if _, err := providers.SelectTTS("cosyvoice", providers.TTSOptions{}, voices); err == nil ||
		!strings.Contains(err.Error(), "not implemented in v1") ||
		!strings.Contains(err.Error(), "kokoro") || !strings.Contains(err.Error(), "system") {
		t.Fatalf("cosyvoice = %v", err)
	}

	mock, err := providers.SelectTTS("mock", providers.TTSOptions{}, voices)
	if err != nil || mock.Name() != "mock" {
		t.Fatalf("mock = %v, %v", mock, err)
	}

	if _, err := providers.SelectTTS("no-such-provider", providers.TTSOptions{}, voices); err == nil {
		t.Fatal("an unknown TTS_PROVIDER must fail loudly")
	}
}
