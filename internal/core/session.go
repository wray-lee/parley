// The deep module: one Session is driven by protocol client messages, audio
// frames, an injected clock and injected Providers, and emits protocol events.
// No host built-ins here.

package core

import (
	"errors"
	"regexp"
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
	// StartMS and EndMS are the audio span the Provider reported. The Session
	// ignores them; contract tests compare them with the fixtures.
	StartMS int64
	EndMS   int64
}

// ASR is the Provider seam for speech recognition.
type ASR interface {
	Name() string
	AcceptsLive() bool
	// Revisable reports that the Provider emits only revisable hypotheses, so
	// the Session derives the Stable Prefix by agreement.
	Revisable() bool
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
	directions       map[string]*Direction
	turn             string
	direction        *Direction
	revisable        bool
	packs            []string
	input            string
	audioMS          int64
	stable           string
	partial          string
	lastHypothesis   string
	committedPrefix  string
	lastSpeechAt     *int64
	ptt              bool
	segID            int
	contexts         map[string][]ContextPair
	mockMicErrorSent bool

	cut           *CutPolicy
	cutWait       Timer
	drain         Timer
	drainDeadline int64
	hold          *HoldPolicy
	holdTimer     Timer
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
		revisable:  cfg.Providers.ASR.Revisable(),
		contexts:   make(map[string][]ContextPair),
		hold:       NewHoldPolicy(cfg.ForcedInstruction),
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
		TTS:          "audio",
	}
	if s.tts.Name() == "system" {
		hello.TTS = "system"
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
		s.setTurn(message.Who)
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
		s.clearHoldTimer()
		s.hold.Reset()
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
	var directions map[string]*Direction
	if mode == "converse" {
		reverse := s.registry.FindDirection(direction.Tgt, direction.Src)
		if reverse == nil {
			s.emit(&protocol.ErrorEvent{
				Type: "error", Kind: "protocol",
				Message: "converse needs both directions: " + direction.Src + "->" + direction.Tgt +
					" and " + direction.Tgt + "->" + direction.Src,
			})
			return
		}
		directions = map[string]*Direction{"me": direction, "them": reverse}
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
	s.directions = directions
	s.turn = ""
	if mode == "converse" {
		s.turn = "me"
	}
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
	s.clearHoldTimer()
	s.hold.Reset()
	s.cut = NewCutPolicy(s.pairConfig[direction.Src+"->"+direction.Tgt])
	s.asr.Open(directionRef(direction))
	s.emit(&protocol.StateEvent{Type: "state", State: "listening"})
	if mode == "converse" {
		s.emit(&protocol.TurnEvent{Type: "turn", Who: "me"})
	}
}

// setTurn switches the audio Direction in Converse. A Held Segment never
// merges into another Turn's Direction, so it is forced now.
func (s *Session) setTurn(who string) {
	if !s.converse() || s.directions[who] == nil || s.turn == who {
		return
	}
	s.closeSegment(s.cfg.Clock.Now(), true)
	s.turn = who
	s.direction = s.directions[who]
	s.audioMS = 0
	s.resetAgreement()
	s.cut = NewCutPolicy(s.pairConfig[s.direction.Src+"->"+s.direction.Tgt])
	if s.hold.Held() != nil {
		s.enqueue(func() error {
			s.forceHeld()
			return nil
		})
	}
	s.asr.Open(directionRef(s.direction))
	s.emit(&protocol.TurnEvent{Type: "turn", Who: who})
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
		// The JavaScript adapters deliver an error event alongside whatever
		// hypotheses arrived in the same push; the Session keeps running.
		s.emitError("asr", err)
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
			s.handlePartial(event.Text)
		case "stable":
			glue := " "
			if event.Glue != nil {
				glue = *event.Glue
			}
			s.addStable(event.Text, glue)
		}
	}
}

// handlePartial stores a display-only hypothesis. A revisable ASR only emits
// hypotheses, so the Stable Prefix is the agreed common prefix not yet added.
func (s *Session) handlePartial(text string) {
	s.partial = text
	if s.revisable {
		common := commonPrefix(s.lastHypothesis, text)
		stable := ""
		if runes := []rune(common); len(runes) > len([]rune(s.committedPrefix)) {
			stable = string(runes[len([]rune(s.committedPrefix)):])
		}
		s.lastHypothesis = text
		if stable != "" {
			s.committedPrefix = common
			s.addStable(stable, "") // successive prefixes of one hypothesis
		}
	}
	s.emit(&protocol.ASAPartialEvent{Type: "asr_partial", Text: text, Direction: directionRef(s.direction)})
}

func commonPrefix(a, b string) string {
	left, right := []rune(a), []rune(b)
	index := 0
	for index < len(left) && index < len(right) && left[index] == right[index] {
		index++
	}
	return string(left[:index])
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

func (s *Session) resetAgreement() {
	s.partial = ""
	s.lastHypothesis = ""
	s.committedPrefix = ""
}

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
	if held := s.hold.Held(); held != nil && sameDirection(directionRef(held.direction), directionRef(direction)) {
		// A Held Segment is merged with this Cut and retried.
		s.translate(s.hold.Merge(source, speechEnd, finalizedAt), false)
		return
	}
	if s.hold.Held() != nil {
		// A Segment of another Direction never merges into the Held one.
		s.translate(s.hold.Force(), true)
	}
	s.translate(&pendingSegment{
		source: source, speechEnd: speechEnd, direction: direction, finalizedAt: finalizedAt,
	}, false)
}

func (s *Session) forceHeld() {
	if s.hold.Held() == nil || s.hold.Held().forced {
		return
	}
	s.translate(s.hold.Force(), true)
}

func (s *Session) translate(pending *pendingSegment, forced bool) {
	if pending == nil {
		return
	}
	direction := pending.direction
	ref := directionRef(direction)
	key := ref.Src + "\x00" + ref.Tgt
	context := s.contexts[key]
	glossary := SelectGlossary(s.registry, ref.Src, ref.Tgt, s.packs)
	request := BuildTranslatorRequest(direction.Prompt, context, pending.source, glossary)
	if forced {
		request = s.hold.WithInstruction(request)
	}

	var translated strings.Builder
	var firstToken *int64
	err := s.translator.Translate(protocol.TranslatorRequest{
		System: request.System, User: request.User, Text: pending.source, Direction: ref,
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

	// A reasoning model can leak its scratchpad into the content stream before
	// the answer. Drop everything through the last closing "think" tag before
	// Hold, Commit, TTS or Rolling Context see it.
	text := translated.String()
	leak := thinkLeakPattern.FindString(text)
	if leak != "" {
		text = text[len(leak):]
	}
	// Models may pad the answer; trim once here so Hold, Commit, TTS and
	// Rolling Context all see the same text.
	text = strings.TrimSpace(text)
	if leak != "" && text == "" {
		s.emit(&protocol.ErrorEvent{
			Type: "error", Provider: "translator", Kind: "protocol",
			Message:  "translator returned no text after leaked reasoning (" + s.translator.Name() + ")",
			Fallback: "Switch LLM_PRESET, or check the endpoint and model with LLM_BASE_URL / LLM_MODEL.",
		})
		return
	}
	if s.hold.Inspect(text) {
		s.handleHold(pending, forced, firstToken, direction)
		return
	}
	s.commitTranslation(pending, text, firstToken, direction, key, context)
}

func (s *Session) handleHold(pending *pendingSegment, forced bool, firstToken *int64, direction *Direction) {
	if forced || pending.forced {
		// A Forced Translation that still returns ⏸ commits the source only.
		s.commitSourceOnly(pending, firstToken, direction)
		return
	}
	if s.hold.Held() == nil {
		s.hold.MarkHeld(pending.source, pending.speechEnd, s.cfg.Clock.Now(), direction, pending.finalizedAt)
		s.armHoldTimer()
	}
	held := s.hold.Held()
	s.emit(&protocol.HoldEvent{
		Type: "hold", Held: true, Merges: held.merges, Direction: directionRef(direction),
	})
	if s.hold.Expired(s.cfg.Clock.Now()) {
		// The retry returned ⏸ after the limit: force it now.
		s.clearHoldTimer()
		s.translate(s.hold.Force(), true)
	}
}

func (s *Session) commitSourceOnly(pending *pendingSegment, firstToken *int64, direction *Direction) {
	ref := directionRef(direction)
	s.segID++
	segID := s.segID
	s.emit(&protocol.SegmentEvent{
		Type: "segment", SegID: segID, Src: pending.source, Tgt: "", Direction: ref,
		LatencyMS: s.cfg.Clock.Now() - pending.speechEnd, Speak: false, Demo: s.demo(),
	})
	s.emitLog(protocol.SegmentLog{
		SegID: segID, Direction: ref, SpeechEnd: pending.speechEnd, ASRFinal: pending.source,
		ASRFinalAt: &pending.finalizedAt, FirstToken: firstToken,
		Held: true, Merges: pending.merges, Forced: true, ForcedFail: true, EchoSuspect: 0,
	})
	s.clearHoldTimer()
	s.hold.Reset()
}

func (s *Session) commitTranslation(pending *pendingSegment, text string, firstToken *int64, direction *Direction, key string, context []ContextPair) {
	ref := directionRef(direction)
	s.segID++
	segID := s.segID
	speak := !s.converse() || sameDirection(ref, s.start)
	s.emit(&protocol.SegmentEvent{
		Type: "segment", SegID: segID, Src: pending.source, Tgt: text, Direction: ref,
		LatencyMS: s.cfg.Clock.Now() - pending.speechEnd, Speak: speak, Demo: s.demo(),
	})

	next := append(context, ContextPair{Src: pending.source, Tgt: text})
	if len(next) > contextWindow {
		next = next[len(next)-contextWindow:]
	}
	s.contexts[key] = next
	s.clearHoldTimer()
	s.hold.Reset()

	var ttsFirstChunk *int64
	// A system TTS is text-only: the client speaks the Segment itself, so the
	// host emits no audio events (as in the JavaScript oracle).
	if speak && s.tts.Name() != "system" {
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
		SegID: segID, Direction: ref, SpeechEnd: pending.speechEnd, ASRFinal: pending.source,
		ASRFinalAt: &pending.finalizedAt, FirstToken: firstToken, TTSFirstChunk: ttsFirstChunk,
		Held: pending.held, Merges: pending.merges, Forced: pending.forced, ForcedFail: false, EchoSuspect: 0,
	})
}

// thinkLeakPattern matches reasoning-model scratchpad up to its last closing
// "think" tag.
var thinkLeakPattern = regexp.MustCompile(`(?is)^.*</[^>]*think[^>]*>`)

func (s *Session) armHoldTimer() {
	if s.holdTimer != nil || s.hold.Held() == nil {
		return
	}
	s.holdTimer = s.cfg.Clock.AfterFunc(s.hold.MSLeft(s.cfg.Clock.Now()), func() {
		s.holdTimer = nil
		s.enqueue(func() error {
			s.forceHeld()
			return nil
		})
	})
}

func (s *Session) clearHoldTimer() {
	if s.holdTimer != nil {
		s.holdTimer.Stop()
		s.holdTimer = nil
	}
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
	s.clearHoldTimer()
	s.hold.Reset()
	s.asr.Close()
}
