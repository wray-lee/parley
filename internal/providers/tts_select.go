// TTS Provider selection: TTS_PROVIDER names one adapter, exactly like the
// JavaScript providers/index.js convention. Unknown names fail loudly.

package providers

import (
	"fmt"

	"github.com/wray-lee/parley/internal/core"
)

// TTSOptions carries the real TTS Provider settings of the Server Host.
type TTSOptions struct {
	KokoroURL   string
	AzureKey    string
	AzureRegion string
	// Warn receives adapter warnings such as a missing exact-language voice.
	Warn func(string)
}

// SelectTTS builds the TTS Provider named by TTS_PROVIDER.
func SelectTTS(name string, options TTSOptions, voices Voices) (core.TTS, error) {
	warn := options.Warn
	if warn == nil {
		warn = func(string) {}
	}
	switch name {
	case "mock":
		return NewMockTTS(), nil
	case "system":
		return NewSystemTTS(), nil
	case "kokoro":
		return NewKokoroTTS(KokoroOptions{KokoroURL: options.KokoroURL, Voices: voices.Kokoro, Warn: warn}), nil
	case "edge":
		return NewEdgeTTS(EdgeOptions{Voices: voices.Edge, Warn: warn}), nil
	case "azure":
		return NewAzureTTS(AzureOptions{
			AzureKey: options.AzureKey, AzureRegion: options.AzureRegion,
			Voices: voices.Azure, Warn: warn,
		})
	case "cosyvoice":
		return nil, fmt.Errorf(
			"CosyVoice TTS is not implemented in v1. " +
				"Set TTS_PROVIDER=kokoro, edge, azure, system or mock.",
		)
	}
	return nil, fmt.Errorf(
		"TTS_PROVIDER %q is not supported; use mock, system, kokoro, edge, azure or cosyvoice",
		name,
	)
}
