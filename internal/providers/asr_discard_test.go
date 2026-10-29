package providers

// Contract tests for ASR.Discard, called on every Converse Turn switch: buffered
// hypotheses and undelivered audio may be dropped, but a Provider failure that
// has not surfaced yet must still reach the next Push.

import (
	"testing"

	"github.com/wray-lee/parley/internal/core"
	"github.com/wray-lee/parley/internal/protocol"
)

type discardCase struct {
	name  string
	build func(*fakeTransport) (core.ASR, *socketASR)
}

func discardCases() []discardCase {
	return []discardCase{
		{"deepgram", func(transport *fakeTransport) (core.ASR, *socketASR) {
			asr := NewDeepgramASR(DeepgramOptions{APIKey: "test-key", dial: transport.dial})
			return asr, &asr.socketASR
		}},
		{"paraformer", func(transport *fakeTransport) (core.ASR, *socketASR) {
			asr := NewParaformerASR(ParaformerOptions{APIKey: "test-key", dial: transport.dial})
			return asr, &asr.socketASR
		}},
		{"whisperlivekit", func(transport *fakeTransport) (core.ASR, *socketASR) {
			asr := NewWhisperLiveKitASR(WhisperLiveKitOptions{dial: transport.dial})
			return asr, &asr.socketASR
		}},
	}
}

func TestASRDiscardKeepsAnUnsurfacedFailureAndDropsEvents(t *testing.T) {
	for _, tc := range discardCases() {
		t.Run(tc.name, func(t *testing.T) {
			transport := &fakeTransport{status: 401, err: errHandshake}
			asr, state := tc.build(transport)
			asr.Open(protocol.Direction{Src: "ja", Tgt: "zh"})
			waitFor(t, "the handshake failure", func() bool {
				state.mu.Lock()
				defer state.mu.Unlock()
				return state.failure != nil
			})
			state.mu.Lock()
			want := state.failure.Error()
			state.deliverLocked(core.ASREvent{Type: "stable", Text: "stale"})
			state.mu.Unlock()

			asr.Discard()
			events, err := asr.Push(nil, 0)
			if len(events) != 0 {
				t.Fatalf("events after Discard = %+v, want none", events)
			}
			if err == nil || err.Error() != want {
				t.Fatalf("failure after Discard = %v, want %q", err, want)
			}
		})
	}
}

func TestASRDiscardClearsAudioQueuedWhileDialing(t *testing.T) {
	for _, tc := range discardCases() {
		t.Run(tc.name, func(t *testing.T) {
			transport := &fakeTransport{hold: make(chan struct{})}
			asr, state := tc.build(transport)
			defer close(transport.hold)
			defer asr.Close()
			asr.Open(protocol.Direction{Src: "ja", Tgt: "zh"})
			waitFor(t, "the dial attempt", func() bool {
				transport.mu.Lock()
				defer transport.mu.Unlock()
				return transport.calls == 1
			})
			if _, err := asr.Push([]int16{1, 2, 3}, 20); err != nil {
				t.Fatal(err)
			}
			state.mu.Lock()
			queued := len(state.pendingAudio)
			state.mu.Unlock()
			if queued == 0 {
				t.Fatal("audio pushed during the dial was not queued")
			}

			asr.Discard()
			state.mu.Lock()
			queued = len(state.pendingAudio)
			state.mu.Unlock()
			if queued != 0 {
				t.Fatalf("pendingAudio after Discard = %d, want 0", queued)
			}
		})
	}
}
