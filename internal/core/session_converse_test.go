package core_test

import (
	"testing"

	"github.com/wray-lee/parley/internal/core"
	"github.com/wray-lee/parley/internal/protocol"
	"github.com/wray-lee/parley/internal/providers"
)

// streamedASR is the Converse double: one independent stream per Direction
// slot, scripted in first-open order. Each Open starts a new connection like
// the socket adapters; a read from a replaced connection cannot surface.
type streamedASR struct {
	name        string
	acceptsLive bool
	revisable   bool
	shared      *streamedShared
	slot        int
	current     *streamedStream
}

type streamedShared struct {
	opened  int
	slots   int
	streams []*streamedStream
	scripts [][]asrMark
	pushes  int
}

type streamedStream struct {
	direction string
	marks     []asrMark
	pushes    int
	discards  int
	opens     int
	conn      *streamedConn
}

// streamedConn is the buffer a socket fills between Pushes.
type streamedConn struct {
	fired []bool
	stale []core.ASREvent
}

func newStreamedASR(scripts ...[]asrMark) *streamedASR {
	return &streamedASR{
		name:        "scripted",
		acceptsLive: true,
		shared:      &streamedShared{scripts: scripts},
	}
}

func (a *streamedASR) Name() string      { return a.name }
func (a *streamedASR) AcceptsLive() bool { return a.acceptsLive }
func (a *streamedASR) Revisable() bool   { return a.revisable }
func (a *streamedASR) NewStream() core.ASR {
	a.shared.slots++
	return &streamedASR{name: a.name, acceptsLive: a.acceptsLive, revisable: a.revisable,
		shared: a.shared, slot: a.shared.slots}
}
func (a *streamedASR) Open(direction protocol.Direction) {
	for len(a.shared.streams) <= a.slot {
		a.shared.streams = append(a.shared.streams, nil)
	}
	stream := a.shared.streams[a.slot]
	if stream == nil {
		stream = &streamedStream{}
		if a.slot < len(a.shared.scripts) {
			stream.marks = a.shared.scripts[a.slot]
		}
		a.shared.streams[a.slot] = stream
	}
	a.shared.opened++
	stream.direction = direction.Src
	stream.opens++
	stream.conn = &streamedConn{fired: make([]bool, len(stream.marks))}
	a.current = stream
}

// deliverLate models a socket read on conn: the adapter drops it once Open
// replaced that connection.
func (a *streamedASR) deliverLate(conn *streamedConn, events ...core.ASREvent) {
	if a.current != nil && a.current.conn == conn {
		a.current.conn.stale = append(a.current.conn.stale, events...)
	}
}

func (a *streamedASR) Close() {}
func (a *streamedASR) Discard() {
	if a.current == nil || a.current.conn == nil {
		return
	}
	a.current.discards++
	a.current.conn.fired = make([]bool, len(a.current.marks))
	a.current.conn.stale = nil
}
func (a *streamedASR) Push(_ []int16, audioMS int64) ([]core.ASREvent, error) {
	if a.current == nil || a.current.conn == nil {
		return nil, nil
	}
	a.current.pushes++
	a.shared.pushes++
	if len(a.current.conn.stale) > 0 {
		events := a.current.conn.stale
		a.current.conn.stale = nil
		return events, nil
	}
	var events []core.ASREvent
	for index, mark := range a.current.marks {
		if !a.current.conn.fired[index] && audioMS >= mark.at {
			a.current.conn.fired[index] = true
			events = append(events, mark.events...)
		}
	}
	return events, nil
}

// wavDuration returns a WAV whose byte rate honours ms, so the core's playback
// estimate matches the JavaScript oracle's audioDurationMs.
func wavDuration(ms int64) []byte {
	data := make([]byte, int(ms)*32) // 16000 Hz * 2 bytes = 32000 B/s
	out := make([]byte, 44+len(data))
	copy(out[0:4], "RIFF")
	copy(out[8:12], "WAVE")
	copy(out[12:16], "fmt ")
	out[20] = 1
	out[22] = 1
	putUint32(out[24:], 16000)
	putUint32(out[28:], 32000) // byte rate
	out[32] = 2
	out[34] = 16
	copy(out[36:40], "data")
	putUint32(out[40:], uint32(len(data)))
	return out
}

func putUint32(dst []byte, value uint32) {
	dst[0] = byte(value)
	dst[1] = byte(value >> 8)
	dst[2] = byte(value >> 16)
	dst[3] = byte(value >> 24)
}

type wavTTS struct {
	name     string
	duration int64
	requests []protocol.TTSRequest
}

func (t *wavTTS) Name() string { return t.name }
func (t *wavTTS) Synthesize(request protocol.TTSRequest) (protocol.Audio, error) {
	t.requests = append(t.requests, request)
	return protocol.Audio{Format: "wav", Data: wavDuration(t.duration)}, nil
}

func startConverse(h *harness, src, tgt string) {
	h.session.HandleMessage(protocol.ClientMessage{Type: "start", Mode: "converse", Src: src, Tgt: tgt})
}

func speechFrame() []int16 {
	pcm := make([]int16, 320)
	for i := range pcm {
		pcm[i] = 3000 // above the speech RMS floor
	}
	return pcm
}

func turnEvents(events []protocol.Event) []*protocol.TurnEvent {
	var turns []*protocol.TurnEvent
	for _, event := range events {
		if turn, ok := event.(*protocol.TurnEvent); ok {
			turns = append(turns, turn)
		}
	}
	return turns
}

func hasAutoTurn(events []protocol.Event, who string) bool {
	for _, turn := range turnEvents(events) {
		if turn.Who == who && turn.Auto {
			return true
		}
	}
	return false
}

// A Converse Session opens one ASR stream per Direction at start; a Turn change
// routes audio to the incoming Direction and reopens that stream, like the
// oracle's asr.open on every Turn.
func TestConverseOpensBothDirectionStreamsAtStart(t *testing.T) {
	asr := newStreamedASR()
	h := newHarness(t, asr, &scriptedTranslator{name: "scripted"}, &scriptedTTS{name: "scripted"})
	startConverse(h, "zh", "ja")

	if asr.shared.opened != 2 {
		t.Fatalf("opened %d streams at start, want 2", asr.shared.opened)
	}
	if len(asr.shared.streams) != 2 ||
		asr.shared.streams[0].direction != "zh" || asr.shared.streams[1].direction != "ja" {
		t.Fatalf("stream directions = %+v, want zh then ja", asr.shared.streams)
	}

	h.feed(20)
	if asr.shared.streams[0].pushes != 1 || asr.shared.streams[1].pushes != 0 {
		t.Fatalf("pushes before the Turn = %d/%d, want 1/0",
			asr.shared.streams[0].pushes, asr.shared.streams[1].pushes)
	}

	h.session.HandleMessage(protocol.ClientMessage{Type: "turn", Who: "them"})
	if asr.shared.streams[0].opens != 1 || asr.shared.streams[1].opens != 2 {
		t.Fatalf("stream opens after the Turn = %d/%d, want 1/2",
			asr.shared.streams[0].opens, asr.shared.streams[1].opens)
	}
	h.feed(20)
	if asr.shared.streams[0].pushes != 1 || asr.shared.streams[1].pushes != 1 {
		t.Fatalf("pushes after the Turn = %d/%d, want 1/1",
			asr.shared.streams[0].pushes, asr.shared.streams[1].pushes)
	}
}

// The Go half of tests/converse.test.js "Turn me/them routes audio to its
// Direction; only my Direction is spoken".
func TestConverseTurnRoutesAudioAndSpeaksOnlyMe(t *testing.T) {
	asr := newStreamedASR(
		[]asrMark{{at: 20, events: []core.ASREvent{stable("from me")}}},
		[]asrMark{{at: 20, events: []core.ASREvent{stable("from them")}}},
	)
	translator := &scriptedTranslator{name: "scripted", answers: [][]string{{"translated-me"}, {"translated-them"}}}
	tts := &wavTTS{name: "scripted", duration: 100}
	h := newHarness(t, asr, translator, tts)
	startConverse(h, "zh", "ja")

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
		t.Fatalf("segments = %+v, want 2", segments)
	}
	if segments[0].Direction != (protocol.Direction{Src: "zh", Tgt: "ja"}) || !segments[0].Speak {
		t.Fatalf("my Segment = %+v, want zh->ja speak", segments[0])
	}
	if segments[1].Direction != (protocol.Direction{Src: "ja", Tgt: "zh"}) || segments[1].Speak {
		t.Fatalf("their Segment = %+v, want ja->zh subtitle-only", segments[1])
	}
	if len(tts.requests) != 1 || tts.requests[0].Text != "translated-me" {
		t.Fatalf("TTS requests = %+v, want only my text", tts.requests)
	}
	if got := translator.calls; len(got) != 2 || got[0].System == got[1].System {
		t.Fatalf("translator prompts = %+v, want one per Direction", got)
	}
}

// The Go half of tests/converse.test.js "Gating withholds live mic frames while
// own speech plays and through the 500 ms tail".
func TestConverseGatingWithholdsFramesAndTail(t *testing.T) {
	asr := newStreamedASR(
		[]asrMark{{at: 20, events: []core.ASREvent{stable("question")}}},
		nil,
	)
	h := newHarness(t, asr, &scriptedTranslator{name: "scripted", answers: [][]string{{"translated-one"}}},
		&wavTTS{name: "scripted", duration: 100})
	startConverse(h, "zh", "ja")
	h.feed(20)
	h.silence(360)
	h.session.Idle()

	pushes := asr.shared.pushes
	h.session.HandleAudio(speechFrame())
	if asr.shared.pushes != pushes {
		t.Fatal("mic frames are withheld while own speech plays")
	}

	h.session.HandleMessage(protocol.ClientMessage{Type: "playback", SegID: 1, State: "end"})
	if !hasAutoTurn(h.events, "them") {
		t.Fatal("my spoken output ending switches the Turn to them")
	}

	h.clock.Advance(499)
	h.session.HandleAudio(speechFrame())
	if asr.shared.pushes != pushes {
		t.Fatal("the tail still blocks the mic")
	}

	h.clock.Advance(1)
	h.session.HandleAudio(speechFrame())
	if asr.shared.pushes != pushes+1 {
		t.Fatalf("pushes after the tail = %d, want %d", asr.shared.pushes, pushes+1)
	}
}

// The Go half of tests/converse.test.js "without a playback report the estimate
// is audio duration + 1.5 s, then the tail".
func TestConverseWatchdogReleasesAtAudioDurationPlus1500(t *testing.T) {
	asr := newStreamedASR(
		[]asrMark{{at: 20, events: []core.ASREvent{stable("question")}}},
		nil,
	)
	h := newHarness(t, asr, &scriptedTranslator{name: "scripted", answers: [][]string{{"translated-one"}}},
		&wavTTS{name: "scripted", duration: 100})
	startConverse(h, "zh", "ja")
	h.feed(20)
	h.silence(360)
	h.session.Idle()
	pushes := asr.shared.pushes

	h.session.HandleAudio(speechFrame())
	if asr.shared.pushes != pushes {
		t.Fatal("gated before the estimate fires")
	}

	h.clock.Advance(1599) // 100 ms audio + 1500 ms estimate, minus 1
	h.session.HandleAudio(speechFrame())
	if asr.shared.pushes != pushes {
		t.Fatal("still gated during the estimated playback")
	}

	h.clock.Advance(1)
	if !hasAutoTurn(h.events, "them") {
		t.Fatal("the estimate also finishes the spoken output and switches the Turn")
	}
	h.session.HandleAudio(speechFrame())
	if asr.shared.pushes != pushes {
		t.Fatal("the tail follows the estimate")
	}

	h.clock.Advance(500)
	h.session.HandleAudio(speechFrame())
	if asr.shared.pushes != pushes+1 {
		t.Fatalf("pushes after estimate + tail = %d, want %d", asr.shared.pushes, pushes+1)
	}
}

// System TTS has no audio to measure, so the Converse gate uses the text
// estimate: estimateSpeechMs(text) + 1.5 s.
func TestSystemTTSGatesConverseAtEstimateSpeechMsPlus1500(t *testing.T) {
	asr := newStreamedASR(
		[]asrMark{{at: 20, events: []core.ASREvent{stable("question")}}},
		nil,
	)
	h := newHarness(t, asr, &scriptedTranslator{name: "scripted", answers: [][]string{{"translated-one"}}},
		providers.NewSystemTTS())
	startConverse(h, "zh", "ja")
	h.feed(20)
	h.silence(360)
	h.session.Idle()
	if events := ttsEvents(h.events); len(events) != 0 {
		t.Fatalf("system TTS emitted audio events: %+v", events)
	}

	pushes := asr.shared.pushes
	h.session.HandleAudio(speechFrame())
	if asr.shared.pushes != pushes {
		t.Fatal("system TTS still gates Converse from the Segment")
	}

	watchdog := core.EstimateSpeechMS("translated-one") + 1500
	h.clock.Advance(watchdog - 1)
	h.session.HandleAudio(speechFrame())
	if asr.shared.pushes != pushes {
		t.Fatal("still gated before estimateSpeechMs + 1.5 s")
	}
	h.clock.Advance(1)
	if !hasAutoTurn(h.events, "them") {
		t.Fatal("the system-TTS watchdog releases at estimateSpeechMs + 1.5 s")
	}
}

// Listen never gates, even while TTS plays; the echo filter still drops the
// app's own words, and forgets them one tail after playback ends.
func TestListenNeverGatesAndEchoSuspectExpires(t *testing.T) {
	asr := newStreamedASR(
		[]asrMark{
			{at: 20, events: []core.ASREvent{stable("one")}},
			{at: 400, events: []core.ASREvent{stable("translated-one")}},  // own voice
			{at: 2800, events: []core.ASREvent{stable("translated-one")}}, // after expiry
		},
	)
	h := newHarness(t, asr, &scriptedTranslator{name: "scripted", answers: [][]string{{"translated-one"}, {"translated-one"}}},
		&wavTTS{name: "scripted", duration: 100})
	h.session.HandleMessage(protocol.ClientMessage{Type: "start", Mode: "listen", Src: "zh", Tgt: "ja"})
	h.feed(20)
	h.silence(360)
	h.session.Idle()
	if len(segmentsOf(h.events)) != 1 {
		t.Fatalf("segments = %+v, want the first Segment", segmentsOf(h.events))
	}

	pushes := asr.shared.pushes
	h.session.HandleAudio(speechFrame())
	if asr.shared.pushes != pushes+1 {
		t.Fatal("Listen pushes mic frames while TTS plays")
	}

	h.session.HandleMessage(protocol.ClientMessage{Type: "playback", SegID: 1, State: "end"})
	h.clock.Advance(500)
	h.feed(2400) // past the echo tail; the same text is no longer an Echo Suspect
	h.silence(360)
	h.session.Idle()

	segments := segmentsOf(h.events)
	if len(segments) != 2 {
		t.Fatalf("segments = %+v, want the post-expiry Segment", segments)
	}
	if segments[1].Src != "translated-one" {
		t.Fatalf("second Segment source = %q", segments[1].Src)
	}
	last := h.logs[len(h.logs)-1]
	if last.EchoSuspect != 1 {
		t.Fatalf("echo_suspect = %d, want 1 (the drop during playback)", last.EchoSuspect)
	}
}

// A playback cancel or error also ends the gate and releases the tail.
func TestConversePlaybackCancelAndErrorEndTheGate(t *testing.T) {
	for _, state := range []string{"cancel", "error"} {
		t.Run(state, func(t *testing.T) {
			asr := newStreamedASR(
				[]asrMark{{at: 20, events: []core.ASREvent{stable("question")}}},
				nil,
			)
			h := newHarness(t, asr, &scriptedTranslator{name: "scripted", answers: [][]string{{"translated-one"}}},
				&wavTTS{name: "scripted", duration: 100})
			startConverse(h, "zh", "ja")
			h.feed(20)
			h.silence(360)
			h.session.Idle()
			pushes := asr.shared.pushes

			h.session.HandleMessage(protocol.ClientMessage{Type: "playback", SegID: 1, State: state})
			h.session.HandleAudio(speechFrame())
			if asr.shared.pushes != pushes {
				t.Fatalf("%s did not start the 500 ms tail", state)
			}
			h.clock.Advance(500)
			h.session.HandleAudio(speechFrame())
			if asr.shared.pushes != pushes+1 {
				t.Fatalf("%s left the gate closed past the tail", state)
			}
		})
	}
}

// A final the socket delivers after the switch-out belongs to the ended Turn;
// switching back must drop it, or it commits under the wrong Turn.
func TestTurnChangeDropsLateHypothesesFromBothStreams(t *testing.T) {
	asr := newStreamedASR()
	h := newHarness(t, asr, &scriptedTranslator{name: "scripted"}, &scriptedTTS{name: "scripted"})
	startConverse(h, "zh", "ja")

	h.session.HandleMessage(protocol.ClientMessage{Type: "turn", Who: "them"})
	if asr.shared.streams[0].discards != 1 {
		t.Fatalf("outgoing stream discards = %d, want 1", asr.shared.streams[0].discards)
	}
	// Delivered asynchronously after the switch-out, so the me stream's next
	// Push would return it.
	asr.deliverLate(asr.shared.streams[0].conn, stable("ghost"))

	h.session.HandleMessage(protocol.ClientMessage{Type: "turn", Who: "me"})
	h.feed(20)
	h.silence(360)
	h.session.Idle()

	if segments := segmentsOf(h.events); len(segments) != 0 {
		t.Fatalf("segments = %+v, want the late final dropped", segments)
	}
}

// The window the oracle closes by reopening the stream: a final from the
// previous tenure can arrive after the switch-in and before the new Turn's
// first Push, so clearing a buffer once at switch-in is not enough.
func TestTurnChangeDropsALateFinalDeliveredAfterSwitchIn(t *testing.T) {
	asr := newStreamedASR()
	h := newHarness(t, asr, &scriptedTranslator{name: "scripted"}, &scriptedTTS{name: "scripted"})
	startConverse(h, "zh", "ja")

	h.session.HandleMessage(protocol.ClientMessage{Type: "turn", Who: "them"})
	oldConn := asr.shared.streams[0].conn
	h.session.HandleMessage(protocol.ClientMessage{Type: "turn", Who: "me"})
	if asr.shared.streams[0].opens != 2 {
		t.Fatalf("incoming stream opens = %d, want 2 (reopened at switch-in)",
			asr.shared.streams[0].opens)
	}
	// The me socket from the first tenure delivers its final only now.
	asr.deliverLate(oldConn, stable("ghost"))

	h.feed(20)
	h.silence(360)
	h.session.Idle()

	if segments := segmentsOf(h.events); len(segments) != 0 {
		t.Fatalf("segments = %+v, want the late final dropped", segments)
	}
}

// Stop must drain the stream of the current Turn, not the me stream: the
// current Provider flushes its final on the next Push after Close.
func TestConverseStopDrainsTheCurrentTurnStream(t *testing.T) {
	asr := newStreamedASR(
		[]asrMark{{at: 0, events: []core.ASREvent{stable("wrong")}}},
		[]asrMark{{at: 0, events: []core.ASREvent{stable("their-final")}}},
	)
	h := newHarness(t, asr, &scriptedTranslator{name: "scripted", answers: [][]string{{"translated"}}},
		&wavTTS{name: "scripted", duration: 100})
	startConverse(h, "zh", "ja")
	h.session.HandleMessage(protocol.ClientMessage{Type: "turn", Who: "them"})
	h.session.HandleMessage(protocol.ClientMessage{Type: "stop"})
	h.session.Idle()

	segments := segmentsOf(h.events)
	if len(segments) != 1 {
		t.Fatalf("segments = %+v, want the drained final", segments)
	}
	if segments[0].Src != "their-final" ||
		segments[0].Direction != (protocol.Direction{Src: "ja", Tgt: "zh"}) {
		t.Fatalf("drained Segment = %+v, want the them stream's final", segments[0])
	}
	if asr.shared.streams[0].pushes != 0 {
		t.Fatalf("me stream pushes = %d, want 0 (Stop must not drain it)", asr.shared.streams[0].pushes)
	}
}

// A client playback start re-arms the estimate watchdog, so the original
// deadline does not release the Gate while the Segment is still playing.
func TestConversePlaybackStartRearmsTheWatchdog(t *testing.T) {
	asr := newStreamedASR([]asrMark{{at: 20, events: []core.ASREvent{stable("one")}}})
	h := newHarness(t, asr, &scriptedTranslator{name: "scripted", answers: [][]string{{"translated-one"}}},
		&wavTTS{name: "scripted", duration: 100})
	startConverse(h, "zh", "ja")
	h.feed(20)
	h.silence(360)
	h.session.Idle()

	h.clock.Advance(1000)
	h.session.HandleMessage(protocol.ClientMessage{Type: "playback", SegID: 1, State: "start"})
	h.clock.Advance(700) // past the original 100 ms + 1.5 s deadline

	pushes := asr.shared.pushes
	h.session.HandleAudio(speechFrame())
	if asr.shared.pushes != pushes {
		t.Fatal("playback start must re-arm the estimate watchdog")
	}
	if hasAutoTurn(h.events, "them") {
		t.Fatal("the original watchdog must not release the Gate after playback start")
	}

	h.clock.Advance(900) // 1.6 s past the re-armed start
	if !hasAutoTurn(h.events, "them") {
		t.Fatal("the re-armed watchdog releases at estimate + 1.5 s")
	}
}

// In Listen the estimate watchdog also ends the echo entry: without any
// playback report the app's own words stop matching after audio + 1.5 s + tail.
func TestListenEchoExpiresWithoutAPlaybackEnd(t *testing.T) {
	asr := newStreamedASR([]asrMark{
		{at: 20, events: []core.ASREvent{stable("one")}},
		{at: 5000, events: []core.ASREvent{stable("translated-one")}}, // own voice
	})
	h := newHarness(t, asr, &scriptedTranslator{name: "scripted", answers: [][]string{{"translated-one"}, {"translated-two"}}},
		&wavTTS{name: "scripted", duration: 100})
	h.session.HandleMessage(protocol.ClientMessage{Type: "start", Mode: "listen", Src: "zh", Tgt: "ja"})
	h.feed(20)
	h.silence(360)
	h.session.Idle()
	if len(segmentsOf(h.events)) != 1 {
		t.Fatalf("segments = %+v, want the first Segment", segmentsOf(h.events))
	}

	// No playback message: the watchdog ends the entry at 100 ms + 1.5 s and
	// the 500 ms echo tail runs out before the feedback arrives.
	h.clock.Advance(100 + 1500 + 500)
	h.feed(5000)
	h.silence(360)
	h.session.Idle()

	segments := segmentsOf(h.events)
	if len(segments) != 2 {
		t.Fatalf("segments = %+v, want the post-expiry Segment", segments)
	}
	if segments[1].Src != "translated-one" {
		t.Fatalf("second Segment source = %q, want the expired match to commit", segments[1].Src)
	}
	if last := h.logs[len(h.logs)-1]; last.EchoSuspect != 0 {
		t.Fatalf("echo_suspect = %d, want 0 after expiry", last.EchoSuspect)
	}
}
