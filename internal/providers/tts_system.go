// System TTS: the OS or browser speaks, so the Server Host sends only text.
// The Session recognises `name == "system"` and emits only `segment {speak:true}`.
// Ported from core/providers/tts-system.js.

package providers

import "github.com/wray-lee/parley/internal/protocol"

// SystemTTS is the text-only TTS Provider.
type SystemTTS struct{}

// NewSystemTTS builds the text-only Provider.
func NewSystemTTS() *SystemTTS { return &SystemTTS{} }

// Name is the Provider name the Session special-cases.
func (s *SystemTTS) Name() string { return "system" }

// Synthesize returns text and no audio.
func (s *SystemTTS) Synthesize(protocol.TTSRequest) (protocol.Audio, error) {
	return protocol.Audio{Format: "text"}, nil
}
