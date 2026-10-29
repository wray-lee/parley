package trace

// Recorder for the Go-only Session Traces. Behaviour that v0.1 deliberately
// changes — core-owned Gating for system TTS, Listen never gating — cannot be
// recorded by the frozen JavaScript oracle, so these traces are recorded from
// the Go core itself (meta.oracle = "go") and replayed by the generic runner.
// Rewrite with PARLEY_RECORD_TRACES=1 go test ./internal/trace -run TestRecord.

import (
	"encoding/base64"
	"encoding/json"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/wray-lee/parley"
	"github.com/wray-lee/parley/internal/core"
	"github.com/wray-lee/parley/internal/protocol"
)

type traceRecorder struct {
	t              *testing.T
	session        *core.Session
	clock          *fakeClock
	lines          []any
	translatorCall int
}

func (r *traceRecorder) input(kind string, fields map[string]any) {
	record := map[string]any{"type": "input", "kind": kind}
	for key, value := range fields {
		record[key] = value
	}
	r.lines = append(r.lines, record)
}

func (r *traceRecorder) output(kind string, fields map[string]any) {
	record := map[string]any{"type": "output", "kind": kind}
	for key, value := range fields {
		record[key] = value
	}
	r.lines = append(r.lines, record)
}

func (r *traceRecorder) client(message protocol.ClientMessage) {
	r.input("client", map[string]any{"message": message})
	r.session.HandleMessage(message)
}

func (r *traceRecorder) audio(ms int64, amplitude float64) {
	r.input("audio", map[string]any{"ms": ms, "amplitude": amplitude})
	r.session.HandleAudio(pcmFrame(ms, amplitude))
}

func (r *traceRecorder) advance(ms int64) {
	r.input("advance", map[string]any{"ms": ms})
	r.clock.Advance(ms)
}

func (r *traceRecorder) idle() {
	r.input("idle", nil)
	r.session.Idle()
}

func (r *traceRecorder) finish(name string) []byte {
	var b strings.Builder
	for _, line := range r.lines {
		encoded, err := json.Marshal(line)
		if err != nil {
			r.t.Fatalf("marshal trace line: %v", err)
		}
		b.Write(encoded)
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

// recordingASR records one asr input per Push, exactly like the JS recorder.
type recordingASR struct {
	inner core.ASR
	rec   *traceRecorder
}

func (a *recordingASR) Name() string                      { return a.inner.Name() }
func (a *recordingASR) AcceptsLive() bool                 { return a.inner.AcceptsLive() }
func (a *recordingASR) Revisable() bool                   { return a.inner.Revisable() }
func (a *recordingASR) Open(direction protocol.Direction) { a.inner.Open(direction) }
func (a *recordingASR) Close()                            { a.inner.Close() }
func (a *recordingASR) NewStream() core.ASR {
	return &recordingASR{inner: a.inner.NewStream(), rec: a.rec}
}
func (a *recordingASR) Discard() { a.inner.Discard() }
func (a *recordingASR) Push(pcm []int16, audioMS int64) ([]core.ASREvent, error) {
	events, err := a.inner.Push(pcm, audioMS)
	if err != nil {
		a.rec.input("provider_error", map[string]any{
			"provider": "asr",
			"error":    map[string]any{"message": err.Error()},
		})
		return events, err
	}
	list := make([]map[string]any, 0, len(events))
	for _, event := range events {
		item := map[string]any{"type": event.Type, "text": event.Text}
		if event.Glue != nil {
			item["glue"] = *event.Glue
		}
		list = append(list, item)
	}
	a.rec.input("asr", map[string]any{"events": list})
	return events, nil
}

type recordingTranslator struct {
	inner   core.Translator
	rec     *traceRecorder
	answers [][]string
}

func (t *recordingTranslator) Name() string { return t.inner.Name() }
func (t *recordingTranslator) Translate(request protocol.TranslatorRequest, onChunk func(string)) error {
	call := t.rec.translatorCall
	t.rec.translatorCall++
	t.rec.output("request", map[string]any{"provider": "translator", "request": request})
	answer := []string{}
	if call < len(t.answers) {
		answer = t.answers[call]
	}
	for _, chunk := range answer {
		t.rec.input("translator", map[string]any{"call": call, "chunk": chunk})
		onChunk(chunk)
	}
	return nil
}

// scriptedStreamASR replays a per-stream, per-push script of stable texts.
// scriptedStreamASR scripts Provider output per Direction slot: the base
// stream is slot 0 and each NewStream takes the next slot. A reopen keeps its
// slot and its script position, so the same Direction's output continues
// across Turns instead of restarting.
type scriptedStreamASR struct {
	name   string
	shared *scriptedStreamShared
	slot   int
}

type scriptedStreamShared struct {
	slots   int
	pushes  []int
	scripts [][]string
}

func (a *scriptedStreamASR) Name() string      { return a.name }
func (a *scriptedStreamASR) AcceptsLive() bool { return true }
func (a *scriptedStreamASR) Revisable() bool   { return false }
func (a *scriptedStreamASR) NewStream() core.ASR {
	a.shared.slots++
	return &scriptedStreamASR{name: a.name, shared: a.shared, slot: a.shared.slots}
}
func (a *scriptedStreamASR) Open(protocol.Direction) {
	for len(a.shared.pushes) <= a.slot {
		a.shared.pushes = append(a.shared.pushes, 0)
	}
}
func (a *scriptedStreamASR) Close()   {}
func (a *scriptedStreamASR) Discard() {}
func (a *scriptedStreamASR) Push([]int16, int64) ([]core.ASREvent, error) {
	push := a.shared.pushes[a.slot]
	a.shared.pushes[a.slot]++
	if a.slot >= len(a.shared.scripts) || push >= len(a.shared.scripts[a.slot]) {
		return nil, nil
	}
	text := a.shared.scripts[a.slot][push]
	if text == "" {
		return nil, nil
	}
	return []core.ASREvent{{Type: "stable", Text: text}}, nil
}

type systemTTS struct{}

func (systemTTS) Name() string { return "system" }
func (systemTTS) Synthesize(protocol.TTSRequest) (protocol.Audio, error) {
	return protocol.Audio{}, nil
}

// recordingTTS records the request output and the audio response input, like
// the JavaScript recorder. Only audio TTS Providers are called at all.
type recordingTTS struct {
	inner core.TTS
	rec   *traceRecorder
}

func (t *recordingTTS) Name() string { return t.inner.Name() }
func (t *recordingTTS) Synthesize(request protocol.TTSRequest) (protocol.Audio, error) {
	t.rec.output("request", map[string]any{"provider": "tts", "request": request})
	audio, err := t.inner.Synthesize(request)
	if err != nil {
		t.rec.input("provider_error", map[string]any{
			"provider": "tts",
			"error":    map[string]any{"message": err.Error()},
		})
		return audio, err
	}
	t.rec.input("tts", map[string]any{"response": map[string]any{
		"format": audio.Format,
		"data":   map[string]any{"$b64": base64.StdEncoding.EncodeToString(audio.Data)},
	}})
	return audio, nil
}

// toneTTS produces a silent WAV of durMS so the playback estimate is the audio
// duration, exactly like the JavaScript oracle.
type toneTTS struct{ durMS int64 }

func (t toneTTS) Name() string { return "scripted" }
func (t toneTTS) Synthesize(protocol.TTSRequest) (protocol.Audio, error) {
	bytes := make([]byte, 44+int(t.durMS)*32)
	copy(bytes[0:4], "RIFF")
	copy(bytes[8:12], "WAVE")
	copy(bytes[12:16], "fmt ")
	bytes[20] = 1
	bytes[22] = 1
	putUint32(bytes[24:], 16000)
	putUint32(bytes[28:], 32000)
	bytes[32] = 2
	bytes[34] = 16
	copy(bytes[36:40], "data")
	putUint32(bytes[40:], uint32(len(bytes)-44))
	return protocol.Audio{Format: "wav", Data: bytes}, nil
}

func putUint32(dst []byte, value uint32) {
	dst[0] = byte(value)
	dst[1] = byte(value >> 8)
	dst[2] = byte(value >> 16)
	dst[3] = byte(value >> 24)
}

func newRecordSession(t *testing.T, name string, tts core.TTS, scripts [][]string, answers [][]string) (*traceRecorder, *core.Session) {
	t.Helper()
	registry, err := core.LoadRegistry(parley.Assets, func(string) {})
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	forced, err := fs.ReadFile(parley.Assets, "prompts/forced_translation.txt")
	if err != nil {
		t.Fatalf("forced instruction: %v", err)
	}
	clock := newFakeClock(1000)
	rec := &traceRecorder{t: t, clock: clock}
	innerASR := &scriptedStreamASR{name: "scripted", shared: &scriptedStreamShared{scripts: scripts}}
	asr := &recordingASR{inner: innerASR, rec: rec}
	translator := &recordingTranslator{inner: &dummyTranslator{}, rec: rec, answers: answers}
	tts = &recordingTTS{inner: tts, rec: rec}
	rec.lines = append(rec.lines, map[string]any{
		"type": "meta", "format": 2, "name": name, "oracle": "go", "clockStart": clock.Now(),
		"registry": registry, "pairConfig": map[string]core.PairConfig{},
		"forcedInstruction": strings.TrimSpace(string(forced)),
		"providers": map[string]any{
			"asr":        map[string]any{"name": asr.Name(), "acceptsLive": asr.AcceptsLive(), "revisable": asr.Revisable()},
			"translator": map[string]any{"name": translator.Name()},
			"tts":        map[string]any{"name": tts.Name()},
		},
	})
	session := core.NewSession(core.SessionConfig{
		Providers:         core.Providers{ASR: asr, Translator: translator, TTS: tts},
		Clock:             clock,
		Registry:          registry,
		ForcedInstruction: strings.TrimSpace(string(forced)),
		Emit:              func(event protocol.Event) { rec.output("event", map[string]any{"event": event}) },
		Log:               func(record protocol.SegmentLog) { rec.output("log", map[string]any{"record": record}) },
	})
	rec.session = session
	return rec, session
}

type dummyTranslator struct{}

func (dummyTranslator) Name() string { return "scripted" }
func (dummyTranslator) Translate(protocol.TranslatorRequest, func(string)) error {
	return nil
}

// TestRecordConverseSystemGatingTrace records the v0.1 Converse Gating trace:
// system TTS is Gated too, frames are withheld through playback and the 500 ms
// tail, the watchdog releases at estimateSpeechMs(text) + 1.5 s, and a client
// cancel or error ends the Gate.
func TestRecordConverseSystemGatingTrace(t *testing.T) {
	const name = "converse-system-gating"
	rec, _ := newRecordSession(t, name, systemTTS{},
		[][]string{
			{"one", "three", "five", "six"}, // me stream, per push
			{"two", "", "", ""},             // them stream, per push
		},
		[][]string{{"translated-one"}, {"translated-two"}, {"translated-three"}, {"translated-five"}, {"translated-six"}},
	)

	rec.client(protocol.ClientMessage{Type: "start", Mode: "converse", Src: "zh", Tgt: "ja"})

	// Segment 1 (me): system TTS, so the Gate starts from the Segment itself.
	rec.client(protocol.ClientMessage{Type: "ptt", State: "down"})
	rec.audio(20, 0.2)
	rec.client(protocol.ClientMessage{Type: "ptt", State: "up"})
	rec.idle()

	// Playback: frames are withheld, including 499 ms of the tail.
	rec.audio(20, 0.2)
	rec.advance(20)
	rec.client(protocol.ClientMessage{Type: "playback", SegID: 1, State: "end"})
	rec.advance(499)
	rec.audio(20, 0.2)
	rec.advance(1)

	// Segment 2 (them): subtitle-only after the tail released the mic.
	rec.client(protocol.ClientMessage{Type: "ptt", State: "down"})
	rec.audio(20, 0.2)
	rec.client(protocol.ClientMessage{Type: "ptt", State: "up"})
	rec.idle()

	// Segment 3 (me): no playback end; the watchdog releases at estimate + 1.5 s.
	rec.client(protocol.ClientMessage{Type: "turn", Who: "me"})
	rec.client(protocol.ClientMessage{Type: "ptt", State: "down"})
	rec.audio(20, 0.2)
	rec.client(protocol.ClientMessage{Type: "ptt", State: "up"})
	rec.idle()
	rec.audio(20, 0.2) // withheld
	watchdog := core.EstimateSpeechMS("translated-three") + 1500
	rec.advance(watchdog - 1)
	rec.audio(20, 0.2) // still withheld
	rec.advance(1)     // watchdog ends playback, auto-switches to them, starts the tail
	rec.advance(500)   // tail over
	rec.audio(20, 0.2) // the first released frame (them stream)

	// Segment 4 (me): a client cancel ends the Gate.
	rec.client(protocol.ClientMessage{Type: "turn", Who: "me"})
	rec.client(protocol.ClientMessage{Type: "ptt", State: "down"})
	rec.audio(20, 0.2)
	rec.client(protocol.ClientMessage{Type: "ptt", State: "up"})
	rec.idle()
	rec.client(protocol.ClientMessage{Type: "playback", SegID: 4, State: "cancel"})
	rec.advance(499)
	rec.audio(20, 0.2) // withheld through the tail
	rec.advance(1)
	rec.audio(20, 0.2) // released

	// The cancel auto-switched to them; take the Turn back for the last Segment.
	rec.client(protocol.ClientMessage{Type: "turn", Who: "me"})
	rec.client(protocol.ClientMessage{Type: "ptt", State: "down"})
	rec.audio(20, 0.2)
	rec.client(protocol.ClientMessage{Type: "ptt", State: "up"})
	rec.idle()
	rec.client(protocol.ClientMessage{Type: "playback", SegID: 5, State: "error"})
	rec.advance(499)
	rec.audio(20, 0.2) // withheld through the tail
	rec.advance(1)
	rec.audio(20, 0.2) // released

	got := rec.finish(name)
	path := "../../spec/traces/" + name + ".jsonl"
	if os.Getenv("PARLEY_RECORD_TRACES") == "1" {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("write trace: %v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read trace: %v (run PARLEY_RECORD_TRACES=1 go test ./internal/trace -run TestRecord)", err)
	}
	if string(got) != strings.ReplaceAll(string(want), "\r\n", "\n") {
		t.Fatalf("recorded trace drifted; rerun with PARLEY_RECORD_TRACES=1\n%s", firstLineDiff(string(got), string(want)))
	}
}

func firstLineDiff(got, want string) string {
	gotLines, wantLines := strings.Split(got, "\n"), strings.Split(want, "\n")
	for index := 0; index < len(gotLines) && index < len(wantLines); index++ {
		if gotLines[index] != wantLines[index] {
			return "line " + strconv.Itoa(index+1) + ":\n got: " + gotLines[index] + "\nwant: " + wantLines[index]
		}
	}
	return "line count " + strconv.Itoa(len(gotLines)) + " vs " + strconv.Itoa(len(wantLines))
}

// TestRecordListenNeverGatesTrace records the v0.1 Listen trace: microphone
// frames reach ASR while audio TTS plays, the app's own words are dropped as an
// Echo Suspect, and the same text commits once the echo tail has passed.
func TestRecordListenNeverGatesTrace(t *testing.T) {
	const name = "listen-never-gates"
	rec, _ := newRecordSession(t, name, toneTTS{durMS: 100},
		[][]string{
			{"one", "translated-one", "second", "translated-one"},
		},
		[][]string{{"translated-one"}, {"translated-two"}, {"translated-one"}},
	)
	rec.client(protocol.ClientMessage{Type: "start", Mode: "listen", Src: "zh", Tgt: "ja"})

	// Segment 1 starts audio playback.
	rec.client(protocol.ClientMessage{Type: "ptt", State: "down"})
	rec.audio(20, 0.2)
	rec.client(protocol.ClientMessage{Type: "ptt", State: "up"})
	rec.idle()

	// While TTS plays: the mic keeps flowing. The own words are an Echo
	// Suspect; a real phrase still commits.
	rec.client(protocol.ClientMessage{Type: "ptt", State: "down"})
	rec.audio(20, 0.2) // "translated-one" -> dropped
	rec.audio(20, 0.2) // "second" -> committed
	rec.client(protocol.ClientMessage{Type: "ptt", State: "up"})
	rec.idle()

	// End the first playback; one echo tail later the same text is real.
	rec.client(protocol.ClientMessage{Type: "playback", SegID: 1, State: "end"})
	rec.advance(500)
	rec.client(protocol.ClientMessage{Type: "ptt", State: "down"})
	rec.audio(20, 0.2)
	rec.client(protocol.ClientMessage{Type: "ptt", State: "up"})
	rec.idle()

	got := rec.finish(name)
	path := "../../spec/traces/" + name + ".jsonl"
	if os.Getenv("PARLEY_RECORD_TRACES") == "1" {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("write trace: %v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read trace: %v (run PARLEY_RECORD_TRACES=1 go test ./internal/trace -run TestRecord)", err)
	}
	if string(got) != strings.ReplaceAll(string(want), "\r\n", "\n") {
		t.Fatalf("recorded trace drifted; rerun with PARLEY_RECORD_TRACES=1\n%s", firstLineDiff(string(got), string(want)))
	}
}
