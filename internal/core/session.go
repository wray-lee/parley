// The deep module: one Session is driven by protocol client messages, audio
// frames, an injected clock and injected Providers, and emits protocol events.
// No host built-ins here.

package core

import (
	"errors"
	"strings"
	"unicode"

	"github.com/wray-lee/parley/internal/protocol"
)

const (
	contextWindow = 8
	// Spec amendment: when silence Cuts but the ASR has not confirmed the latest
	// Partial yet, wait up to 600 ms for the Stable Prefix to catch up (a late
	// final). Other Cut triggers never wait.
	cutCatchupMS = 600
	// shortcut: 3 s bounded ASR drain after stop; raise it if a Provider needs
	// longer to flush its final line.
	asrDrainMS     = 3000
	asrDrainPollMS = 50
)

// ASREvent is one hypothesis event from an ASR Provider.
type ASREvent struct {
	Type string
	Text string
	// Glue nil means the default single space; a non-nil empty string means
	// concatenate without a space (successive prefixes of one hypothesis).
	Glue *string
}

// ASR is the Provider seam for speech recognition.
type ASR interface {
	Name() string
	AcceptsLive() bool
	Open(direction protocol.Direction)
	Push(pcm []int16, audioMS int64) ([]ASREvent, error)
	Close()
}

// Translator is the Provider seam for translation. Translate emits one chunk
// at a time through onChunk.
type Translator interface {
	Name() string
	Translate(request protocol.TranslatorRequest, onChunk func(string)) error
}

// TTS is the Provider seam for speech synthesis.
type TTS interface {
	Name() string
	Synthesize(request protocol.TTSRequest) (protocol.Audio, error)
}

// Providers are the three injected Provider seams.
type Providers struct {
	ASR        ASR
	Translator Translator
	TTS        TTS
}

// Timer is one scheduled callback.
type Timer interface{ Stop() }

// Clock is the injected time source and scheduler.
type Clock interface {
	Now() int64
	AfterFunc(ms int64, callback func()) Timer
}

// SessionConfig wires one Session.
type SessionConfig struct {
	Providers         Providers
	Clock             Clock
	Registry          *Registry
	PairConfig        map[string]PairConfig
	ForcedInstruction string
	Emit              func(protocol.Event)
	Log               func(protocol.SegmentLog)
}

// Session is one translation Session. Methods must be called from one
// goroutine; Idle drains queued commit work at the caller's clock time.
type Session struct {
	cfg        SessionConfig
	asr        ASR
	translator Translator
	tts        TTS
	registry   *Registry
	pairConfig map[string]PairConfig
	queue      []func() error

	running          bool
	mode             string
	start            protocol.Direction
	direction        *Direction
	packs            []string
	input            string
	audioMS          int64
	stable           string
	partial          string
	lastSpeechAt     *int64
	ptt              bool
	segID            int
	contexts         map[string][]ContextPair
	mockMicErrorSent bool

	cut           *CutPolicy
	cutWait       Timer
	drain         Timer
	drainDeadline int64
}

// NewSession builds a Session.
func NewSession(cfg SessionConfig) *Session {
	if cfg.Emit == nil {
		cfg.Emit = func(protocol.Event) {}
	}
	if cfg.Log == nil {
		cfg.Log = func(protocol.SegmentLog) {}
	}
	return &Session{
		cfg:        cfg,
		asr:        cfg.Providers.ASR,
		translator: cfg.Providers.Translator,
		tts:        cfg.Providers.TTS,
		registry:   cfg.Registry,
		pairConfig: cfg.PairConfig,
		contexts:   make(map[string][]ContextPair),
	}
}

func (s *Session) emit(event protocol.Event) { s.cfg.Emit(event) }

func (s *Session) emitLog(record protocol.SegmentLog) { s.cfg.Log(record) }

func (s *Session) emitError(provider string, err error) {
	event := &protocol.ErrorEvent{Type: "error", Provider: provider, Kind: "protocol", Message: err.Error()}
	var providerError *protocol.ProviderError
	if errors.As(err, &providerError) {
		event.Kind = providerError.Kind
		event.Message = providerError.Message
		event.Fallback = providerError.Fallback
	}
	s.emit(event)
}

func (s *Session) enqueue(work func() error) {
	s.queue = append(s.queue, work)
}

// Idle runs queued commit work. A trace's `idle` input calls it so commit
// output sees the clock at that point, exactly like the JavaScript oracle.
func (s *Session) Idle() {
	for len(s.queue) > 0 {
		work := s.queue[0]
		s.queue = s.queue[1:]
		if err := work(); err != nil {
			s.emitError("session", err)
		}
	}
}

// Hello is the handshake payload for a new connection.
func (s *Session) Hello() protocol.Hello {
	hello := protocol.Hello{
		Type:         "hello",
		Pairs:        append([]protocol.Pair{}, s.registry.Pairs...),
		Packs:        make([]protocol.PackRef, 0, len(s.registry.Packs)),
		PresetsInUse: []protocol.Preset{},
		Demo:         s.demo(),
	}
	if hello.Pairs == nil {
		hello.Pairs = []protocol.Pair{}
	}
	for _, pack := range s.registry.Packs {
		hello.Packs = append(hello.Packs, protocol.PackRef{Name: pack.Name, Src: pack.Src, Tgt: pack.Tgt})
	}
	return hello
}

func (s *Session) demo() bool {
	return s.asr.Name() == "mock" || s.translator.Name() == "mock" || s.tts.Name() == "mock"
}

func directionRef(direction *Direction) protocol.Direction {
	return protocol.Direction{Src: direction.Src, Tgt: direction.Tgt}
}

func sameDirection(a, b protocol.Direction) bool {
	return a.Src != "" && a.Tgt != "" && a.Src == b.Src && a.Tgt == b.Tgt
}

func (s *Session) converse() bool { return s.mode == "converse" }

// HandleMessage applies one client protocol message.
func (s *Session) HandleMessage(message protocol.ClientMessage) {
	switch message.Type {
	case "start":
		s.startMessage(message)
	case "sample":
		if !s.running {
			return
		}
		s.input = "sample"
		s.audioMS = 0
		s.resetAgreement()
		s.asr.Open(directionRef(s.direction))
	case "input":
		if message.Source == "sample" {
			s.input = "sample"
		} else {
			s.input = "live"
		}
	case "turn":
		// Converse Turns arrive with the session parity ticket.
	case "playback":
		// Playback gating arrives with the Gating parity ticket.
	case "ptt":
		s.ptt = message.State == "down"
		if s.ptt {
			s.cancelCutWait()
		} else {
			s.closeSegment(s.cfg.Clock.Now(), true)
		}
	case "stop":
		s.running = false
		s.asr.Close()
		s.drainAfterClose()
	}
}

func (s *Session) startMessage(message protocol.ClientMessage) {
	direction := s.registry.FindDirection(message.Src, message.Tgt)
	if direction == nil {
		s.emit(&protocol.ErrorEvent{
			Type: "error", Kind: "protocol",
			Message: "unknown direction: " + message.Src + "->" + message.Tgt,
		})
		return
	}
	mode := message.Mode
	if mode == "" {
		mode = "listen"
	}
	if mode == "converse" {
		s.emit(&protocol.ErrorEvent{
			Type: "error", Kind: "protocol",
			Message:  "converse mode is not implemented in this build",
			Fallback: "Start in Listen mode; Converse support arrives in a later build.",
		})
		return
	}

	packs := []string{}
	for _, name := range message.Packs {
		if s.registry.HasPack(name) {
			packs = append(packs, name)
		} else {
			s.emit(&protocol.ErrorEvent{Type: "error", Kind: "protocol", Message: "unknown pack: " + name})
		}
	}

	s.cancelDrain()
	s.running = true
	s.mode = mode
	s.start = protocol.Direction{Src: direction.Src, Tgt: direction.Tgt}
	s.packs = packs
	s.direction = direction
	s.input = "live"
	s.audioMS = 0
	s.stable = ""
	s.resetAgreement()
	s.lastSpeechAt = nil
	s.ptt = false
	s.segID = 0
	s.mockMicErrorSent = false
	s.contexts = make(map[string][]ContextPair)
	s.cut = NewCutPolicy(s.pairConfig[direction.Src+"->"+direction.Tgt])
	s.asr.Open(directionRef(direction))
	s.emit(&protocol.StateEvent{Type: "state", State: "listening"})
}

// HandleAudio feeds one PCM16 microphone frame.
func (s *Session) HandleAudio(pcm []int16) {
	if !s.running || s.direction == nil {
		return
	}
	if !s.asr.AcceptsLive() && s.input == "live" {
		if !s.mockMicErrorSent {
			s.mockMicErrorSent = true
			s.emit(&protocol.ErrorEvent{
				Type:     "error",
				Provider: "asr",
				Kind:     "protocol",
				Message:  "Mock ASR only accepts bundled samples. Configure a real ASR provider to use the microphone.",
				Fallback: "Play the bundled sample, or set ASR_PROVIDER to a real provider.",
			})
		}
		return
	}

	s.audioMS += int64(len(pcm)) / 16
	events, ok := s.pushASR(pcm)
	if !ok {
		return
	}
	s.handleASREvents(events)

	speech, cut := s.cut.Feed(pcm)
	now := s.cfg.Clock.Now()
	if speech {
		s.lastSpeechAt = &now
		s.cancelCutWait() // speech resumed: normal accumulation continues
	}
	if s.ptt {
		return // automatic Cuts are suspended while PTT is held
	}
	if cut {
		if s.cutWait != nil {
			return // already waiting for the ASR
		}
		if s.asrPending() {
			s.armCutWait()
		} else {
			s.closeSegment(now, true)
		}
	} else if s.cut.Accumulated(now) {
		s.closeSegment(now, false)
	}
}

func (s *Session) pushASR(pcm []int16) ([]ASREvent, bool) {
	events, err := s.asr.Push(pcm, s.audioMS)
	if err != nil {
		s.emitError("asr", err)
		return nil, false
	}
	return events, true
}

func isPunctuationOrSpace(r rune) bool {
	return unicode.IsPunct(r) || unicode.IsSpace(r)
}

func (s *Session) handleASREvents(events []ASREvent) {
	for _, event := range events {
		switch event.Type {
		case "partial":
			s.partial = event.Text
			// A Partial is display-only: never translated and never committed.
			s.emit(&protocol.ASAPartialEvent{Type: "asr_partial", Text: event.Text, Direction: directionRef(s.direction)})
		case "stable":
			glue := " "
			if event.Glue != nil {
				glue = *event.Glue
			}
			s.addStable(event.Text, glue)
		}
	}
}

func (s *Session) addStable(text, glue string) {
	if text == "" {
		return
	}
	if s.stable == "" {
		s.stable = text
	} else {
		s.stable = s.stable + glue + text
	}
	now := s.cfg.Clock.Now()
	clauseCut := s.cut.Stable(s.stable, now)
	if !s.ptt && clauseCut {
		s.closeSegment(now, false)
	} else if s.cutWait != nil && !s.asrPending() {
		s.closeSegment(now, true)
	}
}

// asrPending reports whether the latest Partial has a tail the Stable Prefix
// does not cover yet (ignoring spacing and punctuation).
func (s *Session) asrPending() bool {
	strip := func(text string) string {
		var b strings.Builder
		for _, r := range strings.ToLower(text) {
			if isPunctuationOrSpace(r) {
				continue
			}
			b.WriteRune(r)
		}
		return b.String()
	}
	partial := strip(s.partial)
	return partial != "" && !strings.HasSuffix(strip(s.stable), partial)
}

func (s *Session) resetAgreement() { s.partial = "" }

func (s *Session) closeSegment(now int64, utteranceEnd bool) {
	s.cancelCutWait()
	if s.cut == nil {
		return
	}
	direction := s.direction
	source := strings.TrimSpace(s.stable)
	speechEnd := now
	if s.lastSpeechAt != nil {
		speechEnd = *s.lastSpeechAt
	}
	s.cut.Reset()
	s.stable = ""
	s.lastSpeechAt = nil
	if utteranceEnd {
		s.resetAgreement()
	}
	if source == "" {
		return
	}
	finalizedAt := now
	s.enqueue(func() error {
		s.commit(source, speechEnd, direction, finalizedAt)
		return nil
	})
}

func (s *Session) commit(source string, speechEnd int64, direction *Direction, finalizedAt int64) {
	ref := directionRef(direction)
	key := ref.Src + "\x00" + ref.Tgt
	context := s.contexts[key]
	glossary := SelectGlossary(s.registry, ref.Src, ref.Tgt, s.packs)
	prompt := BuildTranslatorRequest(direction.Prompt, context, source, glossary)

	var translated strings.Builder
	var firstToken *int64
	err := s.translator.Translate(protocol.TranslatorRequest{
		System: prompt.System, User: prompt.User, Text: source, Direction: ref,
	}, func(chunk string) {
		if firstToken == nil {
			at := s.cfg.Clock.Now()
			firstToken = &at
		}
		translated.WriteString(chunk)
	})
	if err != nil {
		s.emitError("translator", err)
		return
	}
	text := strings.TrimSpace(translated.String())

	s.segID++
	segID := s.segID
	speak := !s.converse() || sameDirection(ref, s.start)
	s.emit(&protocol.SegmentEvent{
		Type: "segment", SegID: segID, Src: source, Tgt: text, Direction: ref,
		LatencyMS: s.cfg.Clock.Now() - speechEnd, Speak: speak, Demo: s.demo(),
	})

	next := append(context, ContextPair{Src: source, Tgt: text})
	if len(next) > contextWindow {
		next = next[len(next)-contextWindow:]
	}
	s.contexts[key] = next

	var ttsFirstChunk *int64
	if speak {
		audio, err := s.tts.Synthesize(protocol.TTSRequest{Text: text, SegID: segID, Direction: ref})
		if err != nil {
			s.emitError("tts", err)
		} else {
			at := s.cfg.Clock.Now()
			ttsFirstChunk = &at
			s.emit(&protocol.TTSBeginEvent{Type: "tts_begin", SegID: segID, Format: audio.Format})
			s.emit(&protocol.TTSChunkEvent{Type: "tts_chunk", SegID: segID, Data: audio.Data})
			s.emit(&protocol.TTSEndEvent{Type: "tts_end", SegID: segID})
		}
	}

	s.emitLog(protocol.SegmentLog{
		SegID: segID, Direction: ref, SpeechEnd: speechEnd, ASRFinal: source,
		ASRFinalAt: &finalizedAt, FirstToken: firstToken, TTSFirstChunk: ttsFirstChunk,
		Held: false, Merges: 0, Forced: false, ForcedFail: false, EchoSuspect: 0,
	})
}

func (s *Session) armCutWait() {
	s.cancelCutWait()
	s.cutWait = s.cfg.Clock.AfterFunc(cutCatchupMS, func() {
		s.cutWait = nil
		s.closeSegment(s.cfg.Clock.Now(), true)
	})
}

func (s *Session) cancelCutWait() {
	if s.cutWait != nil {
		s.cutWait.Stop()
		s.cutWait = nil
	}
}

func (s *Session) cancelDrain() {
	if s.drain != nil {
		s.drain.Stop()
		s.drain = nil
	}
}

// After stop the Provider flushes its final line only on a later push, so keep
// polling for a bounded window and let the normal Cut commit it.
func (s *Session) drainASR() {
	s.drain = nil
	if s.cfg.Clock.Now() >= s.drainDeadline {
		s.closeSegment(s.cfg.Clock.Now(), true)
		return
	}
	events, ok := s.pushASR(nil)
	if !ok {
		s.closeSegment(s.cfg.Clock.Now(), true)
		return
	}
	s.handleASREvents(events)
	s.closeSegment(s.cfg.Clock.Now(), true) // commit each flushed batch; no-op when empty
	s.drain = s.cfg.Clock.AfterFunc(asrDrainPollMS, s.drainASR)
}

func (s *Session) drainAfterClose() {
	s.cancelDrain()
	s.drainDeadline = s.cfg.Clock.Now() + asrDrainMS
	s.drainASR()
}

// Close stops the Session and its timers; the host calls it when the
// WebSocket closes.
func (s *Session) Close() {
	s.running = false
	s.cancelDrain()
	s.cancelCutWait()
	s.asr.Close()
}
