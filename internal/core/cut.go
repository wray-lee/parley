// Cut policy: closes a Segment on energy-VAD silence, accumulated confirmed
// speech, or the Direction's clause length. Session feeds it audio, Stable
// Prefix text and the clock; PTT suspension is the Session's job.
// shortcut: energy VAD only; upgrade to Silero via onnxruntime if false Cuts become frequent.

package core

import (
	"math"
	"strings"
)

// PairConfig is the per-Direction Cut configuration from config/pairs.json.
type PairConfig struct {
	ClauseLength int    `json:"clauseLength"`
	Unit         string `json:"unit"`
}

const (
	defaultSilenceMS      = 350
	defaultAccumulationMS = 2500
	speechRMS             = 0.01
	samplesPerMS          = 16 // PCM16 16 kHz, fixed by the protocol
)

// CutPolicy decides when a Segment ends.
type CutPolicy struct {
	silenceMS      int64
	accumulationMS int64
	clauseLength   int
	unit           string

	silence     int64
	heardSpeech bool
	stableSince *int64
}

// NewCutPolicy builds a policy for one Direction.
func NewCutPolicy(config PairConfig) *CutPolicy {
	return &CutPolicy{
		silenceMS:      defaultSilenceMS,
		accumulationMS: defaultAccumulationMS,
		clauseLength:   config.ClauseLength,
		unit:           config.Unit,
	}
}

// Feed adds one PCM16 frame and reports speech energy and whether silence Cut.
func (c *CutPolicy) Feed(pcm []int16) (speech, cut bool) {
	ms := int64(len(pcm)) / samplesPerMS
	if rms(pcm) > speechRMS {
		c.silence = 0
		c.heardSpeech = true
		return true, false
	}
	if !c.heardSpeech {
		return false, false
	}
	c.silence += ms
	if c.silence >= c.silenceMS {
		c.Reset()
		return false, true
	}
	return false, false
}

// Stable starts the accumulation clock on the first confirmed text after the
// last Cut and reports whether the clause length is reached.
func (c *CutPolicy) Stable(text string, now int64) bool {
	if c.stableSince == nil {
		c.stableSince = &now
	}
	return c.clauseReached(text)
}

// Accumulated reports whether confirmed speech has lasted the accumulation limit.
func (c *CutPolicy) Accumulated(now int64) bool {
	return c.stableSince != nil && now-*c.stableSince >= c.accumulationMS
}

// Reset clears all counters after a Cut.
func (c *CutPolicy) Reset() {
	c.silence = 0
	c.heardSpeech = false
	c.stableSince = nil
}

func (c *CutPolicy) clauseReached(text string) bool {
	if c.clauseLength <= 0 || (c.unit != "char" && c.unit != "word") {
		return false
	}
	if c.unit == "word" {
		return len(strings.Fields(text)) >= c.clauseLength
	}
	return len([]rune(text)) >= c.clauseLength
}

func rms(pcm []int16) float64 {
	if len(pcm) == 0 {
		return 0
	}
	sum := 0.0
	for _, sample := range pcm {
		value := float64(sample) / 32768
		sum += value * value
	}
	return math.Sqrt(sum / float64(len(pcm)))
}
