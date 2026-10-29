package providers_test

import (
	"testing"

	"github.com/wray-lee/parley/internal/protocol"
	"github.com/wray-lee/parley/internal/providers"
)

// Mirrors tests/contract/tts-system.test.js: the system Provider sends text only.
func TestSystemTTSReturnsTextAndNoAudio(t *testing.T) {
	tts := providers.NewSystemTTS()
	if tts.Name() != "system" {
		t.Fatalf("name = %q", tts.Name())
	}
	audio, err := tts.Synthesize(protocol.TTSRequest{
		Text: "こんにちは。", SegID: 1, Direction: protocol.Direction{Src: "zh", Tgt: "ja"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if audio.Format != "text" || len(audio.Data) != 0 {
		t.Fatalf("audio = %+v", audio)
	}
}
