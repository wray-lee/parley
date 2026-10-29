package core_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/wray-lee/parley/internal/core"
	"github.com/wray-lee/parley/internal/protocol"
	"github.com/wray-lee/parley/internal/providers"
)

func ttsEvents(events []protocol.Event) []protocol.Event {
	var found []protocol.Event
	for _, event := range events {
		switch event.(type) {
		case *protocol.TTSBeginEvent, *protocol.TTSChunkEvent, *protocol.TTSEndEvent:
			found = append(found, event)
		}
	}
	return found
}

// The Go half of tests/contract/tts-system.test.js: text-only TTS emits no
// tts_* frames, and the Segment still asks the client to speak it.
func TestSystemTTSKeepsTheSpeakFlagAndEmitsNoAudio(t *testing.T) {
	asr := &scriptedASR{name: "scripted", marks: []asrMark{
		{at: 20, events: []core.ASREvent{stable("one")}},
	}}
	translator := &scriptedTranslator{name: "scripted", answers: [][]string{{"translated-one"}}}
	h := newHarness(t, asr, translator, providers.NewSystemTTS())
	startListen(h)

	h.feed(20)
	h.silence(360)
	h.session.Idle()

	segments := segmentsOf(h.events)
	if len(segments) != 1 || !segments[0].Speak || segments[0].Tgt != "translated-one" {
		t.Fatalf("segments = %+v", segments)
	}
	if events := ttsEvents(h.events); len(events) != 0 {
		t.Fatalf("system TTS emitted audio events: %+v", events)
	}
	if len(h.logs) != 1 || h.logs[0].TTSFirstChunk != nil {
		t.Fatalf("log = %+v", h.logs)
	}
}

type failingTTS struct{ calls int }

func (f *failingTTS) Name() string { return "failing" }

func (f *failingTTS) Synthesize(protocol.TTSRequest) (protocol.Audio, error) {
	f.calls++
	return protocol.Audio{}, errors.New("tts provider exploded")
}

// A TTS failure keeps the committed subtitle, emits the protocol error and does
// not block the next Commit.
func TestTTSFailureKeepsSubtitlesAndCommitProgress(t *testing.T) {
	asr := &scriptedASR{name: "scripted", marks: []asrMark{
		{at: 20, events: []core.ASREvent{stable("one")}},
		{at: 420, events: []core.ASREvent{stable("two")}},
	}}
	translator := &scriptedTranslator{name: "scripted", answers: [][]string{
		{"translated-one"}, {"translated-two"},
	}}
	tts := &failingTTS{}
	h := newHarness(t, asr, translator, tts)
	startListen(h)

	h.feed(20)
	h.silence(360)
	h.session.Idle()
	h.feed(20)
	h.silence(360)
	h.session.Idle()

	segments := segmentsOf(h.events)
	if len(segments) != 2 || segments[0].Tgt != "translated-one" || segments[1].Tgt != "translated-two" {
		t.Fatalf("segments = %+v", segments)
	}
	if tts.calls != 2 {
		t.Fatalf("TTS calls = %d, want 2 (the failure must not block the next Commit)", tts.calls)
	}

	var errorsSeen int
	for _, event := range h.events {
		if eventError, ok := event.(*protocol.ErrorEvent); ok {
			errorsSeen++
			if eventError.Provider != "tts" || eventError.Kind != "protocol" ||
				!strings.Contains(eventError.Message, "tts provider exploded") {
				t.Fatalf("error = %+v", eventError)
			}
		}
	}
	if errorsSeen != 2 {
		t.Fatalf("error events = %d, want 2", errorsSeen)
	}
	if events := ttsEvents(h.events); len(events) != 0 {
		t.Fatalf("a failed TTS must not emit audio events: %+v", events)
	}
	if len(h.logs) != 2 || h.logs[0].TTSFirstChunk != nil || h.logs[1].TTSFirstChunk != nil {
		t.Fatalf("logs = %+v", h.logs)
	}
}
