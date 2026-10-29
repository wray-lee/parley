package core

// The Converse Gate withholds live microphone frames for as long as an echo
// entry is live (playback pending plus one tail), so no public input can reach
// addStable with a matching hypothesis in Converse. This white-box test drives
// the same seam the trace runner drives — beginOwnSpeech then a Stable — to
// prove the Converse echo drop and its expiry.

import (
	"testing"

	"github.com/wray-lee/parley/internal/protocol"
)

type echoClock struct{ now int64 }

func (c *echoClock) Now() int64                    { return c.now }
func (c *echoClock) AfterFunc(int64, func()) Timer { return echoTimer{} }

type echoTimer struct{}

func (echoTimer) Stop() {}

type echoTTS struct{ name string }

func (t echoTTS) Name() string { return t.name }
func (t echoTTS) Synthesize(protocol.TTSRequest) (protocol.Audio, error) {
	return protocol.Audio{}, nil
}

func TestConverseEchoSuspectIsDroppedAndExpires(t *testing.T) {
	clock := &echoClock{now: 1000}
	session := &Session{
		cfg:       SessionConfig{Clock: clock, Emit: func(protocol.Event) {}},
		mode:      "converse",
		input:     "live",
		direction: &Direction{Src: "zh", Tgt: "ja"},
		playbacks: make(map[int]*playback),
		gating:    &Gating{},
		echo:      NewEchoFilter(),
		cut:       NewCutPolicy(PairConfig{ClauseLength: 1000, Unit: "char"}),
		tts:       echoTTS{name: "system"},
	}
	session.beginOwnSpeech(1, "hello world", nil)

	session.addStable("hello world", " ")
	if session.echoSuspects != 1 {
		t.Fatalf("echo_suspect = %d, want 1", session.echoSuspects)
	}
	if session.stable != "" {
		t.Fatalf("stable = %q, want the Echo Suspect dropped", session.stable)
	}

	session.HandleMessage(protocol.ClientMessage{Type: "playback", SegID: 1, State: "end"})
	clock.now += gatingTailMS
	session.addStable("hello world", " ")
	if session.stable != "hello world" {
		t.Fatalf("stable = %q, want the match to commit after the tail", session.stable)
	}
}
