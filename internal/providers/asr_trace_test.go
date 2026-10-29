package providers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wray-lee/parley/internal/core"
)

// The recorded traces replay the scripts the adapters produced on the JS side.
// Replaying the same fixtures through the Go adapters must yield that same
// event stream, and internal/trace replays the traces to the same Stable
// Prefix and Cut outputs.

type traceASREvent struct {
	Type string  `json:"type"`
	Text string  `json:"text"`
	Glue *string `json:"glue"`
}

func readTraceASREvents(t *testing.T, name string) []traceASREvent {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "spec", "traces", name+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var events []traceASREvent
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var record struct {
			Type   string          `json:"type"`
			Kind   string          `json:"kind"`
			Events []traceASREvent `json:"events"`
		}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("parse trace line: %v", err)
		}
		if record.Type == "input" && record.Kind == "asr" {
			events = append(events, record.Events...)
		}
	}
	return events
}

func TestASRAdaptersProduceTheEventsTheirTracesRecorded(t *testing.T) {
	cases := []struct {
		name   string
		trace  string
		replay func(t *testing.T) []core.ASREvent
	}{
		{
			name:  "deepgram",
			trace: "asr-deepgram-listen",
			replay: func(t *testing.T) []core.ASREvent {
				fixture := loadDeepgramFixture(t, "live-ja-context")
				transport := &fakeTransport{}
				asr := startDeepgram(t, fixture, transport)
				var events []core.ASREvent
				for _, frame := range fixture.Frames {
					asr.receive(frame.Data)
					batch, err := asr.Push(nil, frame.AtMS)
					if err != nil {
						t.Fatal(err)
					}
					events = append(events, batch...)
				}
				return events
			},
		},
		{
			name:  "paraformer",
			trace: "asr-paraformer-listen",
			replay: func(t *testing.T) []core.ASREvent {
				fixture := loadParaformerFixture(t, "interim-final")
				transport := &fakeTransport{}
				asr := startParaformer(t, fixture, transport)
				var events []core.ASREvent
				for _, frame := range fixture.Frames {
					asr.receive(frame.Data)
					batch, err := asr.Push(nil, frame.AtMS)
					if err != nil {
						t.Fatal(err)
					}
					events = append(events, batch...)
				}
				return events
			},
		},
		{
			name:  "whisperlivekit",
			trace: "asr-whisperlivekit-listen",
			replay: func(t *testing.T) []core.ASREvent {
				fixture := loadWhisperLiveKitFixture(t, "full-mode")
				transport := &fakeTransport{}
				asr := startWhisperLiveKit(t, fixture, transport)
				var events []core.ASREvent
				for _, frame := range fixture.Frames {
					asr.receive(frame.Data)
					batch, err := asr.Push(make([]int16, 160), frame.AtMS)
					if err != nil {
						t.Fatal(err)
					}
					events = append(events, batch...)
				}
				return events
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			want := readTraceASREvents(t, testCase.trace)
			got := testCase.replay(t)
			if len(got) != len(want) {
				t.Fatalf("got %d events, trace recorded %d", len(got), len(want))
			}
			for index := range want {
				actual, expected := got[index], want[index]
				if actual.Type != expected.Type || actual.Text != expected.Text {
					t.Fatalf("event %d = %s %q, trace = %s %q", index, actual.Type, actual.Text, expected.Type, expected.Text)
				}
				if (actual.Glue == nil) != (expected.Glue == nil) {
					t.Fatalf("event %d glue present = %v, trace = %v", index, actual.Glue != nil, expected.Glue != nil)
				}
				if actual.Glue != nil && *actual.Glue != *expected.Glue {
					t.Fatalf("event %d glue = %q, trace = %q", index, *actual.Glue, *expected.Glue)
				}
			}
		})
	}
}
