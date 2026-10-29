// Gating seam: in Converse the microphone is withheld while the app's own
// speech is playing and for a short tail after the last Segment finishes. The
// Session owns playback state; this module only remembers when the tail ends.
// Ported from core/gating.js.
package core

import "unicode/utf16"

const gatingTailMS = 500

// textEstimateMinMS and textEstimatePerCharMS mirror app/tts-speaker.js
// estimateSpeechMs, the timeout backstop for clients whose TTS never reports an
// end.
const (
	textEstimateMinMS     = 1500
	textEstimatePerCharMS = 180
	textEstimateMaxMS     = 30000
)

// EstimateSpeechMS estimates how long speaking text takes, in milliseconds. It
// counts UTF-16 code units, like app/tts-speaker.js estimateSpeechMs
// (JavaScript String.length), so astral characters cost two units.
func EstimateSpeechMS(text string) int64 {
	estimate := int64(textEstimateMinMS + textEstimatePerCharMS*len(utf16.Encode([]rune(text))))
	if estimate > textEstimateMaxMS {
		return textEstimateMaxMS
	}
	return estimate
}

// Gating remembers the end of the tail that follows playback.
type Gating struct{ tailUntil int64 }

// End starts the tail at now.
func (g *Gating) End(now int64) {
	if until := now + gatingTailMS; until > g.tailUntil {
		g.tailUntil = until
	}
}

// Blocked reports whether the tail still withholds the microphone.
func (g *Gating) Blocked(now int64) bool { return now < g.tailUntil }

// Reset clears the tail.
func (g *Gating) Reset() { g.tailUntil = 0 }
