//go:build live

// Live TTS checks, ported from scripts/live-check-tts-azure.mjs plus a local
// Kokoro probe. They need a real key or a running Kokoro service and never run
// in CI: every test skips cleanly when the environment is not configured.
// Secrets are only read into the adapter, never printed.

package providers_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wray-lee/parley/internal/protocol"
	"github.com/wray-lee/parley/internal/providers"
)

func liveVoices(t *testing.T) providers.Voices {
	t.Helper()
	voices, err := providers.LoadVoices(os.DirFS(filepath.Join("..", "..")))
	if err != nil {
		t.Fatalf("LoadVoices: %v", err)
	}
	return voices
}

func TestLiveAzureTTS(t *testing.T) {
	key := os.Getenv("AZURE_SPEECH_KEY")
	region := os.Getenv("AZURE_SPEECH_REGION")
	if key == "" || region == "" {
		t.Skip("set AZURE_SPEECH_KEY and AZURE_SPEECH_REGION to check the live Azure TTS")
	}
	tts, err := providers.SelectTTS("azure", providers.TTSOptions{
		AzureKey: key, AzureRegion: region,
	}, liveVoices(t))
	if err != nil {
		t.Fatal(err)
	}
	audio, err := tts.Synthesize(protocol.TTSRequest{
		Text: "こんにちは。", SegID: 1, Direction: protocol.Direction{Src: "zh", Tgt: "ja"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if audio.Format != "mp3" || len(audio.Data) == 0 {
		t.Fatalf("Azure returned %s audio with %d bytes", audio.Format, len(audio.Data))
	}
}

func TestLiveKokoroTTS(t *testing.T) {
	url := os.Getenv("KOKORO_URL")
	if url == "" {
		t.Skip("set KOKORO_URL (for example http://127.0.0.1:8880) to check the live Kokoro TTS")
	}
	tts, err := providers.SelectTTS("kokoro", providers.TTSOptions{KokoroURL: url}, liveVoices(t))
	if err != nil {
		t.Fatal(err)
	}
	audio, err := tts.Synthesize(protocol.TTSRequest{
		Text: "こんにちは。", SegID: 1, Direction: protocol.Direction{Src: "zh", Tgt: "ja"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if audio.Format != "mp3" || len(audio.Data) == 0 {
		t.Fatalf("Kokoro returned %s audio with %d bytes", audio.Format, len(audio.Data))
	}
}
