// Session Trace replayer: the Go core must reproduce the frozen JavaScript
// oracle's outputs on the fake clock. Format: spec/traces/README.md.

package trace

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"reflect"
	"strings"

	"github.com/wray-lee/parley/internal/core"
	"github.com/wray-lee/parley/internal/protocol"
)

// Meta is the first line of a trace.
type Meta struct {
	Type              string                     `json:"type"`
	Format            int                        `json:"format"`
	Name              string                     `json:"name"`
	ClockStart        int64                      `json:"clockStart"`
	Registry          core.Registry              `json:"registry"`
	PairConfig        map[string]core.PairConfig `json:"pairConfig"`
	ForcedInstruction string                     `json:"forcedInstruction"`
	Providers         struct {
		ASR struct {
			Name        string `json:"name"`
			AcceptsLive bool   `json:"acceptsLive"`
			Revisable   bool   `json:"revisable"`
		} `json:"asr"`
		Translator struct {
			Name string `json:"name"`
		} `json:"translator"`
		TTS struct {
			Name string `json:"name"`
		} `json:"tts"`
	} `json:"providers"`
}

// ReplayFile replays one trace file and fails on any output difference.
func ReplayFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return Replay(string(data))
}

// Replay replays one trace and fails on any output difference.
func Replay(text string) error {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) == 0 {
		return fmt.Errorf("empty trace")
	}
	var meta Meta
	if err := json.Unmarshal([]byte(lines[0]), &meta); err != nil {
		return fmt.Errorf("parse meta: %w", err)
	}
	if meta.Type != "meta" {
		return fmt.Errorf("trace must start with a meta record")
	}
	if meta.Format != 1 {
		return fmt.Errorf("unsupported trace format %d", meta.Format)
	}

	var inputs []map[string]any
	asrResponses := [][]core.ASREvent{}
	translatorCalls := [][]string{}
	ttsResponses := []protocol.Audio{}
	expected := []any{}

	for _, line := range lines[1:] {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return fmt.Errorf("parse record: %w", err)
		}
		switch record["type"] {
		case "input":
			inputs = append(inputs, record)
			switch record["kind"] {
			case "asr":
				asrResponses = append(asrResponses, decodeASREvents(record["events"]))
			case "translator":
				call := int(record["call"].(float64))
				for len(translatorCalls) <= call {
					translatorCalls = append(translatorCalls, nil)
				}
				translatorCalls[call] = append(translatorCalls[call], record["chunk"].(string))
			case "tts":
				responses, err := decodeTTSResponse(record["response"])
				if err != nil {
					return err
				}
				ttsResponses = append(ttsResponses, responses...)
			}
		case "output":
			delete(record, "type")
			expected = append(expected, record)
		}
	}

	clock := newFakeClock(meta.ClockStart)
	actual := []any{}
	push := func(kind string, payload map[string]any) {
		payload["kind"] = kind
		actual = append(actual, payload)
	}

	session := core.NewSession(core.SessionConfig{
		Providers: core.Providers{
			ASR:        &traceASR{name: meta.Providers.ASR.Name, acceptsLive: meta.Providers.ASR.AcceptsLive, revisable: meta.Providers.ASR.Revisable, responses: asrResponses},
			Translator: &traceTranslator{name: meta.Providers.Translator.Name, calls: translatorCalls, push: push},
			TTS:        &traceTTS{name: meta.Providers.TTS.Name, responses: ttsResponses, push: push},
		},
		Clock:             clock,
		Registry:          &meta.Registry,
		PairConfig:        meta.PairConfig,
		ForcedInstruction: meta.ForcedInstruction,
		Emit:              func(event protocol.Event) { push("event", map[string]any{"event": event}) },
		Log:               func(record protocol.SegmentLog) { push("log", map[string]any{"record": record}) },
	})

	for _, input := range inputs {
		switch input["kind"] {
		case "client":
			raw, _ := json.Marshal(input["message"])
			var message protocol.ClientMessage
			if err := json.Unmarshal(raw, &message); err != nil {
				return err
			}
			session.HandleMessage(message)
		case "audio":
			session.HandleAudio(pcmFrame(int64(input["ms"].(float64)), input["amplitude"].(float64)))
		case "advance":
			clock.Advance(int64(input["ms"].(float64)))
		case "idle":
			session.Idle()
		}
	}

	var actualValue any
	raw, err := json.Marshal(actual)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, &actualValue); err != nil {
		return err
	}
	actualValue = normalize(actualValue)
	expectedValue := normalize(any(expected))
	if !reflect.DeepEqual(actualValue, expectedValue) {
		actualPretty, _ := json.MarshalIndent(actualValue, "", "  ")
		expectedPretty, _ := json.MarshalIndent(expectedValue, "", "  ")
		return fmt.Errorf("trace %s: replayed outputs differ\nactual:\n%s\nexpected:\n%s", meta.Name, actualPretty, expectedPretty)
	}
	return nil
}

func decodeASREvents(value any) []core.ASREvent {
	if value == nil {
		return nil
	}
	events := []core.ASREvent{}
	for _, item := range value.([]any) {
		event := item.(map[string]any)
		converted := core.ASREvent{Type: event["type"].(string), Text: event["text"].(string)}
		if glue, ok := event["glue"].(string); ok {
			converted.Glue = &glue
		}
		events = append(events, converted)
	}
	return events
}

func decodeTTSResponse(value any) ([]protocol.Audio, error) {
	if value == nil {
		return nil, nil
	}
	response := value.(map[string]any)
	encoded, ok := response["data"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("tts response without binary data")
	}
	data, err := base64.StdEncoding.DecodeString(encoded["$b64"].(string))
	if err != nil {
		return nil, err
	}
	return []protocol.Audio{{Format: response["format"].(string), Data: data}}, nil
}

// normalize turns the trace's {"$b64": ...} encoding into the base64 string
// encoding/json uses for []byte.
func normalize(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		if len(typed) == 1 {
			if encoded, ok := typed["$b64"].(string); ok {
				return encoded
			}
		}
		for key, child := range typed {
			typed[key] = normalize(child)
		}
		return typed
	case []any:
		for index, child := range typed {
			typed[index] = normalize(child)
		}
		return typed
	}
	return value
}

func pcmFrame(ms int64, amplitude float64) []int16 {
	count := int(math.Round(16000 * float64(ms) / 1000))
	pcm := make([]int16, count)
	if amplitude == 0 {
		return pcm
	}
	for i := range pcm {
		value := math.Sin(2*math.Pi*220*float64(i)/16000) * amplitude * 32767
		pcm[i] = int16(math.Round(value))
	}
	return pcm
}

type traceASR struct {
	name        string
	acceptsLive bool
	revisable   bool
	responses   [][]core.ASREvent
	index       int
}

func (a *traceASR) Name() string            { return a.name }
func (a *traceASR) AcceptsLive() bool       { return a.acceptsLive }
func (a *traceASR) Revisable() bool         { return a.revisable }
func (a *traceASR) Open(protocol.Direction) {}
func (a *traceASR) Close()                  {}
func (a *traceASR) Push([]int16, int64) ([]core.ASREvent, error) {
	if a.index >= len(a.responses) {
		return nil, nil
	}
	events := a.responses[a.index]
	a.index++
	return events, nil
}

type traceTranslator struct {
	name  string
	calls [][]string
	index int
	push  func(string, map[string]any)
}

func (t *traceTranslator) Name() string { return t.name }

func (t *traceTranslator) Translate(request protocol.TranslatorRequest, onChunk func(string)) error {
	t.push("request", map[string]any{"provider": "translator", "request": request})
	var chunks []string
	if t.index < len(t.calls) {
		chunks = t.calls[t.index]
	}
	t.index++
	for _, chunk := range chunks {
		onChunk(chunk)
	}
	return nil
}

type traceTTS struct {
	name      string
	responses []protocol.Audio
	index     int
	push      func(string, map[string]any)
}

func (t *traceTTS) Name() string { return t.name }

func (t *traceTTS) Synthesize(request protocol.TTSRequest) (protocol.Audio, error) {
	t.push("request", map[string]any{"provider": "tts", "request": request})
	if t.index >= len(t.responses) {
		return protocol.Audio{}, fmt.Errorf("trace has no tts response for call %d", t.index)
	}
	audio := t.responses[t.index]
	t.index++
	return audio, nil
}

type fakeClock struct {
	now    int64
	seq    int
	timers []*fakeTimer
}

type fakeTimer struct {
	at      int64
	seq     int
	fn      func()
	stopped bool
}

func newFakeClock(start int64) *fakeClock { return &fakeClock{now: start} }

func (c *fakeClock) Now() int64 { return c.now }

func (c *fakeClock) AfterFunc(ms int64, fn func()) core.Timer {
	c.seq++
	timer := &fakeTimer{at: c.now + ms, seq: c.seq, fn: fn}
	c.timers = append(c.timers, timer)
	return timer
}

func (t *fakeTimer) Stop() { t.stopped = true }

func (c *fakeClock) Advance(ms int64) {
	target := c.now + ms
	for {
		var due *fakeTimer
		for _, timer := range c.timers {
			if timer.stopped || timer.at > target {
				continue
			}
			if due == nil || timer.at < due.at || (timer.at == due.at && timer.seq < due.seq) {
				due = timer
			}
		}
		if due == nil {
			break
		}
		for i, timer := range c.timers {
			if timer == due {
				c.timers = append(c.timers[:i], c.timers[i+1:]...)
				break
			}
		}
		c.now = due.at
		due.fn()
	}
	c.now = target
}
