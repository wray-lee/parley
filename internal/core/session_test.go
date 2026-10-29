package core_test

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/wray-lee/parley"
	"github.com/wray-lee/parley/internal/core"
	"github.com/wray-lee/parley/internal/protocol"
	"github.com/wray-lee/parley/internal/providers"
)

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
	clock   *fakeClock
}

func newFakeClock(start int64) *fakeClock { return &fakeClock{now: start} }

func (c *fakeClock) Now() int64 { return c.now }

func (c *fakeClock) AfterFunc(ms int64, fn func()) core.Timer {
	c.seq++
	timer := &fakeTimer{at: c.now + ms, seq: c.seq, fn: fn, clock: c}
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
		c.remove(due)
		c.now = due.at
		due.fn()
	}
	c.now = target
}

func (c *fakeClock) remove(target *fakeTimer) {
	for i, timer := range c.timers {
		if timer == target {
			c.timers = append(c.timers[:i], c.timers[i+1:]...)
			return
		}
	}
}

type asrMark struct {
	at     int64
	events []core.ASREvent
}

// scriptedASR replays marks at audio offsets, like the oracle's test double.
type scriptedASR struct {
	name        string
	acceptsLive bool
	revisable   bool
	marks       []asrMark
	fired       []bool
	opened      int
}

func (a *scriptedASR) Name() string      { return a.name }
func (a *scriptedASR) AcceptsLive() bool { return a.acceptsLive }
func (a *scriptedASR) Revisable() bool   { return a.revisable }
func (a *scriptedASR) NewStream() core.ASR {
	return &scriptedASR{name: a.name, acceptsLive: a.acceptsLive, revisable: a.revisable, marks: a.marks}
}
func (a *scriptedASR) Discard() { a.fired = make([]bool, len(a.marks)) }
func (a *scriptedASR) Open(protocol.Direction) {
	a.opened++
	a.fired = make([]bool, len(a.marks))
}
func (a *scriptedASR) Close() {}
func (a *scriptedASR) Push(_ []int16, audioMS int64) ([]core.ASREvent, error) {
	var events []core.ASREvent
	for index, mark := range a.marks {
		if !a.fired[index] && audioMS >= mark.at {
			a.fired[index] = true
			events = append(events, mark.events...)
		}
	}
	return events, nil
}

type scriptedTranslator struct {
	name    string
	answers [][]string
	calls   []protocol.TranslatorRequest
}

func (t *scriptedTranslator) Name() string { return t.name }

func (t *scriptedTranslator) Translate(request protocol.TranslatorRequest, onChunk func(string)) error {
	t.calls = append(t.calls, request)
	if len(t.answers) == 0 {
		return nil
	}
	answer := t.answers[0]
	t.answers = t.answers[1:]
	for _, chunk := range answer {
		onChunk(chunk)
	}
	return nil
}

type scriptedTTS struct {
	name    string
	request []protocol.TTSRequest
}

func (t *scriptedTTS) Name() string { return t.name }
func (t *scriptedTTS) Synthesize(request protocol.TTSRequest) (protocol.Audio, error) {
	t.request = append(t.request, request)
	return protocol.Audio{Format: "wav", Data: []byte("RIFF")}, nil
}

func stable(text string) core.ASREvent { return core.ASREvent{Type: "stable", Text: text} }
func partial(text string) core.ASREvent {
	return core.ASREvent{Type: "partial", Text: text}
}

func segmentsOf(events []protocol.Event) []*protocol.SegmentEvent {
	var segments []*protocol.SegmentEvent
	for _, event := range events {
		if segment, ok := event.(*protocol.SegmentEvent); ok {
			segments = append(segments, segment)
		}
	}
	return segments
}

type harness struct {
	session *core.Session
	clock   *fakeClock
	events  []protocol.Event
	logs    []protocol.SegmentLog
}

func newHarness(t *testing.T, asr core.ASR, translator core.Translator, tts core.TTS) *harness {
	t.Helper()
	return newHarnessWithConfig(t, asr, translator, tts,
		map[string]core.PairConfig{"zh->ja": {ClauseLength: 12, Unit: "char"}})
}

func newHarnessWithConfig(t *testing.T, asr core.ASR, translator core.Translator, tts core.TTS, pairConfig map[string]core.PairConfig) *harness {
	t.Helper()
	registry, err := core.LoadRegistry(parley.Assets, func(string) {})
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	forced, err := fs.ReadFile(parley.Assets, "prompts/forced_translation.txt")
	if err != nil {
		t.Fatalf("forced instruction: %v", err)
	}
	if asr == nil {
		asr = &scriptedASR{name: "scripted"}
	}
	if translator == nil {
		translator = &scriptedTranslator{name: "scripted"}
	}
	if tts == nil {
		tts = &scriptedTTS{name: "scripted"}
	}
	h := &harness{clock: newFakeClock(1000)}
	h.session = core.NewSession(core.SessionConfig{
		Providers:         core.Providers{ASR: asr, Translator: translator, TTS: tts},
		Clock:             h.clock,
		Registry:          registry,
		PairConfig:        pairConfig,
		ForcedInstruction: strings.TrimSpace(string(forced)),
		Emit:              func(event protocol.Event) { h.events = append(h.events, event) },
		Log:               func(record protocol.SegmentLog) { h.logs = append(h.logs, record) },
	})
	return h
}

func (h *harness) feed(ms int) {
	for elapsed := 0; elapsed < ms; elapsed += 20 {
		pcm := make([]int16, 320)
		for i := range pcm {
			pcm[i] = 3000 // above the speech RMS floor
		}
		h.session.HandleAudio(pcm)
		h.clock.Advance(20)
	}
}

func (h *harness) silence(ms int) {
	for elapsed := 0; elapsed < ms; elapsed += 20 {
		h.session.HandleAudio(make([]int16, 320))
		h.clock.Advance(20)
	}
}

func TestDemoListenSessionCommitsASegment(t *testing.T) {
	sample, err := providers.LoadSample(parley.Assets, "placeholder")
	if err != nil {
		t.Fatalf("sample: %v", err)
	}
	registry, err := core.LoadRegistry(parley.Assets, func(string) {})
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	clock := newFakeClock(1000)
	var events []protocol.Event
	var logs []protocol.SegmentLog
	session := core.NewSession(core.SessionConfig{
		Providers: core.Providers{
			ASR: providers.NewMockASR(sample), Translator: providers.NewMockTranslator(sample.ReferenceTranslations), TTS: providers.NewMockTTS(),
		},
		Clock:      clock,
		Registry:   registry,
		PairConfig: map[string]core.PairConfig{"zh->ja": {ClauseLength: 12, Unit: "char"}},
		Emit:       func(event protocol.Event) { events = append(events, event) },
		Log:        func(record protocol.SegmentLog) { logs = append(logs, record) },
	})
	hello := session.Hello()
	if !hello.Demo {
		t.Fatal("mock providers must report demo=true")
	}

	session.HandleMessage(protocol.ClientMessage{Type: "start", Mode: "listen", Src: "zh", Tgt: "ja"})
	session.HandleMessage(protocol.ClientMessage{Type: "sample", Name: "placeholder"})
	for elapsed := 0; elapsed < 1600; elapsed += 20 {
		pcm := make([]int16, 320)
		for i := range pcm {
			pcm[i] = 6000
		}
		session.HandleAudio(pcm)
		clock.Advance(20)
		session.Idle()
	}
	for elapsed := 0; elapsed < 500; elapsed += 20 {
		session.HandleAudio(make([]int16, 320))
		clock.Advance(20)
		session.Idle()
	}

	if len(logs) != 1 {
		t.Fatalf("logs = %d, want 1: %+v", len(logs), logs)
	}
	var segment *protocol.SegmentEvent
	for _, event := range events {
		if candidate, ok := event.(*protocol.SegmentEvent); ok {
			segment = candidate
		}
	}
	if segment == nil {
		t.Fatalf("no segment event: %+v", events)
	}
	if segment.Src != "你好，这是演示。" {
		t.Fatalf("source = %q", segment.Src)
	}
	if segment.Tgt != "[DEMO] こんにちは、これはデモです。" {
		t.Fatalf("translation = %q", segment.Tgt)
	}
	if !segment.Demo || !segment.Speak || segment.SegID != 1 {
		t.Fatalf("segment = %+v", segment)
	}
	if logs[0].Direction != (protocol.Direction{Src: "zh", Tgt: "ja"}) || logs[0].FirstToken == nil {
		t.Fatalf("log = %+v", logs[0])
	}
	ttsChunks := 0
	for _, event := range events {
		if _, ok := event.(*protocol.TTSChunkEvent); ok {
			ttsChunks++
		}
	}
	if ttsChunks != 1 {
		t.Fatalf("tts chunks = %d, want 1", ttsChunks)
	}
}

func TestListenSilenceCutUsesSpeechEnd(t *testing.T) {
	asr := &scriptedASR{name: "scripted", marks: []asrMark{{at: 20, events: []core.ASREvent{partial("one"), stable("one")}}}}
	translator := &scriptedTranslator{name: "scripted", answers: [][]string{{"translated-one"}}}
	h := newHarness(t, asr, translator, nil)
	h.session.HandleMessage(protocol.ClientMessage{Type: "start", Mode: "listen", Src: "zh", Tgt: "ja"})
	h.session.HandleMessage(protocol.ClientMessage{Type: "sample", Name: "placeholder"})
	h.feed(20)
	h.silence(360)
	h.session.Idle()

	if len(h.logs) != 1 {
		t.Fatalf("logs = %d, want 1 (%+v)", len(h.logs), h.events)
	}
	if h.logs[0].SpeechEnd != 1000 {
		t.Fatalf("speech_end = %d, want 1000", h.logs[0].SpeechEnd)
	}
	if h.logs[0].ASRFinalAt == nil || *h.logs[0].ASRFinalAt != 1360 {
		t.Fatalf("asr_final_at = %v, want 1360", h.logs[0].ASRFinalAt)
	}
	if h.logs[0].FirstToken == nil || *h.logs[0].FirstToken != 1380 {
		t.Fatalf("first_token = %v, want 1380", h.logs[0].FirstToken)
	}
	var segment *protocol.SegmentEvent
	for _, event := range h.events {
		if candidate, ok := event.(*protocol.SegmentEvent); ok {
			segment = candidate
		}
	}
	if segment == nil || segment.LatencyMS != 380 {
		t.Fatalf("segment = %+v", segment)
	}
}

func TestSilenceCutWaitsForTheLateFinal(t *testing.T) {
	asr := &scriptedASR{name: "scripted", marks: []asrMark{
		{at: 20, events: []core.ASREvent{partial("127.5")}},
		{at: 440, events: []core.ASREvent{stable("127.5")}},
	}}
	h := newHarness(t, asr, nil, nil)
	startListen(h)

	h.feed(20)
	h.silence(360)
	h.session.Idle()
	if got := len(segmentsOf(h.events)); got != 0 {
		t.Fatalf("cut before the late final: %d segments", got)
	}

	h.silence(60)
	h.session.Idle()
	segments := segmentsOf(h.events)
	if len(segments) != 1 || segments[0].Src != "127.5" {
		t.Fatalf("segments = %+v", segments)
	}
	if segments[0].LatencyMS >= 950 {
		t.Fatalf("cut at the deadline, not on the final: latency=%d", segments[0].LatencyMS)
	}
}

func TestSilenceCutCatchupExpiresWithStableTextOnly(t *testing.T) {
	asr := &scriptedASR{name: "scripted", marks: []asrMark{
		{at: 20, events: []core.ASREvent{partial("hello world")}},
		{at: 40, events: []core.ASREvent{stable("hello")}},
	}}
	h := newHarness(t, asr, nil, nil)
	startListen(h)

	h.feed(40)
	h.silence(360)
	h.session.Idle()
	if got := len(segmentsOf(h.events)); got != 0 {
		t.Fatalf("cut before the wait: %d segments", got)
	}
	h.silence(560)
	h.session.Idle()
	if got := len(segmentsOf(h.events)); got != 0 {
		t.Fatalf("cut before 600 ms: %d segments", got)
	}
	h.silence(20)
	h.session.Idle()
	segments := segmentsOf(h.events)
	if len(segments) != 1 || segments[0].Src != "hello" {
		t.Fatalf("segments = %+v", segments)
	}
}

func TestRevisableAgreementMakesThePrefixStable(t *testing.T) {
	asr := &scriptedASR{name: "scripted", revisable: true, marks: []asrMark{
		{at: 20, events: []core.ASREvent{partial("hello")}},
		{at: 40, events: []core.ASREvent{partial("hello")}},
	}}
	h := newHarness(t, asr, nil, nil)
	startListen(h)

	h.feed(200)
	h.session.Idle()
	if got := len(segmentsOf(h.events)); got != 0 {
		t.Fatalf("agreed Partials cut before a Cut: %d segments", got)
	}
	h.session.HandleMessage(protocol.ClientMessage{Type: "ptt", State: "up"})
	h.session.Idle()
	segments := segmentsOf(h.events)
	if len(segments) != 1 || segments[0].Src != "hello" {
		t.Fatalf("segments = %+v", segments)
	}
}

func TestSingleRevisableHypothesisNeverBecomesStable(t *testing.T) {
	asr := &scriptedASR{name: "scripted", revisable: true, marks: []asrMark{
		{at: 20, events: []core.ASREvent{partial("hello")}},
	}}
	h := newHarness(t, asr, nil, nil)
	startListen(h)

	h.feed(200)
	h.session.HandleMessage(protocol.ClientMessage{Type: "ptt", State: "up"})
	h.session.Idle()
	if got := len(segmentsOf(h.events)); got != 0 {
		t.Fatalf("single hypothesis became stable: %d segments", got)
	}
}

func TestRollingContextCarriesTheLastCommittedPair(t *testing.T) {
	asr := &scriptedASR{name: "scripted", marks: []asrMark{
		{at: 20, events: []core.ASREvent{partial("one"), stable("one")}},
		{at: 440, events: []core.ASREvent{partial("two"), stable("two")}},
	}}
	translator := &scriptedTranslator{name: "scripted", answers: [][]string{{"translated-one"}, {"translated-two"}}}
	h := newHarness(t, asr, translator, nil)
	h.session.HandleMessage(protocol.ClientMessage{Type: "start", Mode: "listen", Src: "zh", Tgt: "ja"})
	h.session.HandleMessage(protocol.ClientMessage{Type: "sample", Name: "placeholder"})
	h.feed(20)
	h.silence(400)
	h.session.Idle()
	h.feed(20)
	h.silence(400)
	h.session.Idle()

	if len(translator.calls) != 2 {
		t.Fatalf("translator calls = %d, want 2", len(translator.calls))
	}
	second := translator.calls[1]
	if second.Text != "two" || !strings.Contains(second.User, "原文：one\n译文：translated-one") {
		t.Fatalf("second request = %+v", second)
	}
	if !strings.Contains(second.User, "[新段落]\ntwo") {
		t.Fatalf("second request tail = %q", second.User)
	}
}

func TestUnknownDirectionAndPackAreProtocolErrors(t *testing.T) {
	h := newHarness(t, nil, nil, nil)
	h.session.HandleMessage(protocol.ClientMessage{Type: "start", Mode: "listen", Src: "en", Tgt: "fr"})
	if len(h.events) != 1 {
		t.Fatalf("events = %+v", h.events)
	}
	if err := h.events[0].(*protocol.ErrorEvent); err.Message != "unknown direction: en->fr" {
		t.Fatalf("error = %+v", err)
	}

	h.session.HandleMessage(protocol.ClientMessage{Type: "start", Mode: "listen", Src: "zh", Tgt: "ja", Packs: []string{"nope"}})
	last := h.events[len(h.events)-1].(*protocol.StateEvent)
	if last.State != "listening" {
		t.Fatalf("session must still start: %+v", h.events)
	}
	packError := h.events[len(h.events)-2].(*protocol.ErrorEvent)
	if packError.Message != "unknown pack: nope" {
		t.Fatalf("error = %+v", packError)
	}
}

func TestMockAsrRefusesLiveAudioOnce(t *testing.T) {
	sample, err := providers.LoadSample(parley.Assets, "placeholder")
	if err != nil {
		t.Fatalf("sample: %v", err)
	}
	h := newHarness(t, providers.NewMockASR(sample), providers.NewMockTranslator(nil), providers.NewMockTTS())
	h.session.HandleMessage(protocol.ClientMessage{Type: "start", Mode: "listen", Src: "zh", Tgt: "ja"})
	h.session.HandleAudio(make([]int16, 320))
	h.session.HandleAudio(make([]int16, 320))
	if len(h.events) != 2 {
		t.Fatalf("events = %+v", h.events)
	}
	errEvent := h.events[1].(*protocol.ErrorEvent)
	if errEvent.Provider != "asr" || errEvent.Fallback == "" {
		t.Fatalf("error = %+v", errEvent)
	}
}

func TestConverseTurnsRouteAudioAndSpeakOnlyMe(t *testing.T) {
	asr := &scriptedASR{
		name: "scripted", acceptsLive: true,
		marks: []asrMark{{at: 20, events: []core.ASREvent{stable("from me")}}},
	}
	translator := &scriptedTranslator{name: "scripted", answers: [][]string{{"translated-me"}, {"translated-them"}}}
	tts := &scriptedTTS{name: "scripted"}
	h := newHarness(t, asr, translator, tts)
	h.session.HandleMessage(protocol.ClientMessage{Type: "start", Mode: "converse", Src: "zh", Tgt: "ja"})

	if len(h.events) != 2 {
		t.Fatalf("events = %+v", h.events)
	}
	if turn, ok := h.events[1].(*protocol.TurnEvent); !ok || turn.Who != "me" {
		t.Fatalf("turn = %+v", h.events[1])
	}
	h.feed(20)
	h.silence(360)
	h.session.Idle()

	h.session.HandleMessage(protocol.ClientMessage{Type: "turn", Who: "them"})
	h.session.HandleMessage(protocol.ClientMessage{Type: "playback", SegID: 1, State: "end"})
	h.clock.Advance(500) // the gating tail
	h.feed(20)
	h.silence(360)
	h.session.Idle()

	segments := segmentsOf(h.events)
	if len(segments) != 2 {
		t.Fatalf("segments = %+v", segments)
	}
	if !segments[0].Speak || segments[0].Direction.Src != "zh" {
		t.Fatalf("me segment = %+v", segments[0])
	}
	if segments[1].Speak || segments[1].Direction.Src != "ja" {
		t.Fatalf("them segment = %+v", segments[1])
	}
	if len(tts.request) != 1 || tts.request[0].Text != "translated-me" {
		t.Fatalf("tts = %+v", tts.request)
	}
}
