package core_test

import (
	"testing"

	"github.com/wray-lee/parley/internal/core"
	"github.com/wray-lee/parley/internal/protocol"
)

// failingASR emits one stable hypothesis and an error in the same push, like a
// socket adapter that fails while a final is buffered.
type failingASR struct {
	pushed bool
}

func (a *failingASR) Name() string            { return "failing" }
func (a *failingASR) AcceptsLive() bool       { return true }
func (a *failingASR) Revisable() bool         { return false }
func (a *failingASR) Open(protocol.Direction) {}
func (a *failingASR) Close()                  {}
func (a *failingASR) NewStream() core.ASR     { return &failingASR{} }
func (a *failingASR) Discard()                {}
func (a *failingASR) Push([]int16, int64) ([]core.ASREvent, error) {
	if a.pushed {
		return nil, nil
	}
	a.pushed = true
	return []core.ASREvent{stable("one")}, &protocol.ProviderError{
		Kind: "unreachable", Message: "Deepgram connection closed (code 1006)", Fallback: "use mock",
	}
}

func TestASRErrorKeepsTheSessionAliveAndKeepsThatPushHypotheses(t *testing.T) {
	h := newHarness(t, &failingASR{},
		&scriptedTranslator{name: "scripted", answers: [][]string{{"translated"}}},
		&scriptedTTS{name: "system"})
	h.session.HandleMessage(protocol.ClientMessage{Type: "start", Mode: "listen", Src: "zh", Tgt: "ja"})

	h.feed(100)    // the failing push delivers stable "one" plus the error
	h.silence(400) // a silence Cut commits the segment, so the Session kept running
	h.session.Idle()

	var sawError bool
	for _, event := range h.events {
		if failure, ok := event.(*protocol.ErrorEvent); ok {
			sawError = true
			if failure.Provider != "asr" || failure.Kind != "unreachable" || failure.Fallback != "use mock" {
				t.Fatalf("error event = %+v", failure)
			}
		}
	}
	if !sawError {
		t.Fatal("the ASR error must reach the client")
	}
	segments := segmentsOf(h.events)
	if len(segments) != 1 || segments[0].Src != "one" {
		t.Fatalf("segments = %+v; the Session must keep running after an ASR error", segments)
	}
}
