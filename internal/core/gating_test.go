package core

import (
	"testing"

	"github.com/wray-lee/parley/internal/protocol"
)

func TestEstimateSpeechMS(t *testing.T) {
	if got := EstimateSpeechMS("abc"); got != 1500+3*180 {
		t.Fatalf("EstimateSpeechMS = %d", got)
	}
	// An astral character is two UTF-16 code units, like JavaScript's
	// String.length; counting runes would understate the estimate by 180 ms.
	if got := EstimateSpeechMS("𠮟"); got != 1500+2*180 {
		t.Fatalf("EstimateSpeechMS(astral) = %d, want 1860", got)
	}
	long := ""
	for len(long) < 500 {
		long += "a"
	}
	if got := EstimateSpeechMS(long); got != 30000 {
		t.Fatalf("EstimateSpeechMS cap = %d, want 30000", got)
	}
}

// countingClock counts the watchdogs a Session arms.
type countingClock struct {
	now  int64
	arms int
}

func (c *countingClock) Now() int64 { return c.now }
func (c *countingClock) AfterFunc(int64, func()) Timer {
	c.arms++
	return echoTimer{}
}

// A Listen system-TTS entry is unarmed (the oracle's beginOwnSpeech arms only
// gating entries) and stays unarmed when the client reports playback start.
func TestListenSystemTTSPlaybackStartDoesNotArmTheWatchdog(t *testing.T) {
	clock := &countingClock{now: 1000}
	session := &Session{
		cfg:       SessionConfig{Clock: clock, Emit: func(protocol.Event) {}},
		mode:      "listen",
		playbacks: make(map[int]*playback),
		gating:    &Gating{},
		echo:      NewEchoFilter(),
		tts:       echoTTS{name: "system"},
	}
	session.beginOwnSpeech(1, "hello world", nil)
	session.HandleMessage(protocol.ClientMessage{Type: "playback", SegID: 1, State: "start"})
	if clock.arms != 0 {
		t.Fatalf("watchdogs armed = %d, want 0 for a Listen system TTS", clock.arms)
	}
}

// postedClock models the production clock: a fired callback is posted onto the
// session loop, so Stop cannot recall it.
type postedClock struct {
	now       int64
	callbacks []func()
}

func (c *postedClock) Now() int64 { return c.now }
func (c *postedClock) AfterFunc(_ int64, callback func()) Timer {
	c.callbacks = append(c.callbacks, callback)
	return postedTimer{}
}

type postedTimer struct{}

func (postedTimer) Stop() {}

// A watchdog callback posted at the first deadline can run after a playback
// start re-armed the entry; it must not release the Gate early.
func TestPlaybackStartIgnoresAStalePostedWatchdog(t *testing.T) {
	clock := &postedClock{now: 1000}
	session := &Session{
		cfg:       SessionConfig{Clock: clock, Emit: func(protocol.Event) {}},
		mode:      "converse",
		playbacks: make(map[int]*playback),
		gating:    &Gating{},
		echo:      NewEchoFilter(),
		tts:       echoTTS{name: "scripted"},
	}
	session.beginOwnSpeech(1, "hello world", nil)
	if len(clock.callbacks) != 1 {
		t.Fatalf("watchdogs armed = %d, want 1", len(clock.callbacks))
	}
	stale := clock.callbacks[0]
	session.HandleMessage(protocol.ClientMessage{Type: "playback", SegID: 1, State: "start"})
	if len(clock.callbacks) != 2 {
		t.Fatalf("watchdogs armed after playback start = %d, want 2", len(clock.callbacks))
	}
	stale()
	if session.playbacks[1] == nil || session.playbacks[1].timer == nil {
		t.Fatal("the stale posted callback released the re-armed playback")
	}
}

// A watchdog posted before stop must not end a Segment that reuses the same
// seg_id after the restart.
func TestStaleWatchdogDoesNotEndAReusedSegmentID(t *testing.T) {
	clock := &postedClock{now: 1000}
	session := &Session{
		cfg:       SessionConfig{Clock: clock, Emit: func(protocol.Event) {}},
		mode:      "converse",
		playbacks: make(map[int]*playback),
		gating:    &Gating{},
		echo:      NewEchoFilter(),
		tts:       echoTTS{name: "scripted"},
	}
	session.beginOwnSpeech(1, "hello world", nil)
	stale := clock.callbacks[0]
	session.resetPlayback()
	session.beginOwnSpeech(1, "again", nil)
	stale()
	if session.playbacks[1] == nil {
		t.Fatal("the stale posted callback ended the restarted Segment")
	}
}

func TestGatingTail(t *testing.T) {
	var gating Gating
	if gating.Blocked(1000) {
		t.Fatal("an untouched gate must be open")
	}
	gating.End(1000)
	if !gating.Blocked(1499) {
		t.Fatal("the 500 ms tail must block")
	}
	if gating.Blocked(1500) {
		t.Fatal("the tail ends at 500 ms")
	}
	gating.Reset()
	if gating.Blocked(1000) {
		t.Fatal("reset opens the gate")
	}
}
