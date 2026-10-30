package core_test

import (
	"testing"

	"github.com/wray-lee/parley/internal/core"
	"github.com/wray-lee/parley/internal/protocol"
)

// autoScriptedASR is the Auto Turn double: it declares a capability and serves
// the one multilingual stream, while its embedded scriptedASR supplies the
// scripted marks.
type autoScriptedASR struct {
	*scriptedASR
	capability core.LanguageCapability
	autoOpens  int
	hints      []protocol.Direction
	newStreams int
}

func (a *autoScriptedASR) LanguageCapability() core.LanguageCapability {
	return a.capability
}

func (a *autoScriptedASR) OpenAuto(pair protocol.Direction) {
	a.autoOpens++
	a.hints = append(a.hints, pair)
	a.scriptedASR.Open(pair)
}

func (a *autoScriptedASR) NewStream() core.ASR {
	a.newStreams++
	return a.scriptedASR.NewStream()
}

func autoCapability(validated ...protocol.Direction) core.LanguageCapability {
	return core.LanguageCapability{
		StreamingLanguageID: true,
		Granularity:         "word",
		LanguageConfidence:  true,
		Languages:           []string{"zh", "ja"},
		ValidatedPairs:      validated,
	}
}

func tag(language string, confidence float64) core.LanguageTag {
	return core.LanguageTag{Language: language, Confidence: confidence}
}

func tagged(text string, tags ...core.LanguageTag) core.ASREvent {
	return core.ASREvent{Type: "stable", Text: text, Tags: tags}
}

func setAuto(h *harness, enabled bool) {
	h.session.HandleMessage(protocol.ClientMessage{Type: "autoTurn", Enabled: &enabled})
}

func startAuto(t *testing.T, asr *autoScriptedASR) *harness {
	t.Helper()
	h := newHarness(t, asr, nil, nil)
	startConverse(h, "zh", "ja")
	setAuto(h, true)
	return h
}

func TestAutoTurnCapabilityLessProviderStaysManual(t *testing.T) {
	h := newHarness(t, &scriptedASR{name: "scripted", acceptsLive: true}, nil, nil)
	hello := h.session.Hello()
	if hello.AutoTurn.Available || hello.AutoTurn.Enabled {
		t.Fatalf("hello.autoTurn = %+v, want unavailable in Manual", hello.AutoTurn)
	}

	startConverse(h, "zh", "ja")
	setAuto(h, true)
	h.feed(20)
	h.silence(360)
	h.session.Idle()

	if turns := turnEvents(h.events); len(turns) != 1 || turns[0].Who != "me" || turns[0].Auto {
		t.Fatalf("turn events = %+v, want only the initial manual me", turns)
	}
}

func TestAutoTurnMissingPairLanguageStaysManual(t *testing.T) {
	capability := autoCapability(protocol.Direction{Src: "zh", Tgt: "ja"})
	capability.Languages = []string{"zh", "fr"}
	asr := &autoScriptedASR{scriptedASR: &scriptedASR{name: "scripted", acceptsLive: true}, capability: capability}
	h := newHarness(t, asr, nil, nil)
	if hello := h.session.Hello(); hello.AutoTurn.Available {
		t.Fatalf("hello.autoTurn = %+v, want unavailable when ja is unsupported", hello.AutoTurn)
	}

	startConverse(h, "zh", "ja")
	setAuto(h, true)
	if asr.autoOpens != 0 {
		t.Fatalf("auto streams = %d, want none", asr.autoOpens)
	}
}

func TestAutoTurnIsOptInAndOpensOneStreamPerSession(t *testing.T) {
	asr := &autoScriptedASR{
		scriptedASR: &scriptedASR{name: "scripted", acceptsLive: true},
		capability: autoCapability(
			protocol.Direction{Src: "zh", Tgt: "ja"},
			protocol.Direction{Src: "ja", Tgt: "zh"},
		),
	}
	h := newHarness(t, asr, nil, nil)
	hello := h.session.Hello()
	if !hello.AutoTurn.Available || hello.AutoTurn.Enabled {
		t.Fatalf("hello.autoTurn = %+v, want available and opt-in (S3 no-go)", hello.AutoTurn)
	}

	startConverse(h, "zh", "ja")
	if asr.autoOpens != 0 {
		t.Fatal("start must stay Manual until the client opts in")
	}
	setAuto(h, true)
	if asr.autoOpens != 1 || asr.hints[0] != (protocol.Direction{Src: "zh", Tgt: "ja"}) {
		t.Fatalf("auto opens = %d hints = %+v", asr.autoOpens, asr.hints)
	}

	h.session.HandleMessage(protocol.ClientMessage{Type: "turn", Who: "them"})
	h.session.HandleMessage(protocol.ClientMessage{Type: "turn", Who: "me"})
	if asr.autoOpens != 1 {
		t.Fatalf("a Turn change reopened the auto stream (%d opens)", asr.autoOpens)
	}

	setAuto(h, false)
	if streams := asr.newStreams; streams != 2 {
		t.Fatalf("manual streams after opting out = %d, want the two Direction streams", streams)
	}
}

func TestAutoTurnFlipsAtTheCutWithTagsAlreadyInHand(t *testing.T) {
	asr := &autoScriptedASR{
		scriptedASR: &scriptedASR{name: "scripted", acceptsLive: true, marks: []asrMark{
			{at: 20, events: []core.ASREvent{tagged("hi", tag("ja", 0.9), tag("ja", 0.8))}},
		}},
		capability: autoCapability(
			protocol.Direction{Src: "zh", Tgt: "ja"},
			protocol.Direction{Src: "ja", Tgt: "zh"},
		),
	}
	h := startAuto(t, asr)
	h.feed(20)
	h.silence(360)
	h.session.Idle()

	turns := turnEvents(h.events)
	if len(turns) != 2 {
		t.Fatalf("turn events = %+v, want the initial me and the auto them", turns)
	}
	if last := turns[len(turns)-1]; !last.Auto || last.Who != "them" || last.Language != "ja" {
		t.Fatalf("auto turn = %+v", last)
	}
	segments := segmentsOf(h.events)
	if len(segments) != 1 {
		t.Fatalf("segments = %+v", segments)
	}
	if got := segments[0].Direction; got != (protocol.Direction{Src: "ja", Tgt: "zh"}) {
		t.Fatalf("Segment direction = %+v, want the detected Direction", got)
	}
	if len(h.logs) != 1 || !h.logs[0].TurnAuto || h.logs[0].DetectedLanguage != "ja" {
		t.Fatalf("logs = %+v, want turn_auto detected_language=ja", h.logs)
	}
}

func TestAutoTurnUncertainSegmentsKeepTheTurn(t *testing.T) {
	asr := &autoScriptedASR{
		scriptedASR: &scriptedASR{name: "scripted", acceptsLive: true, marks: []asrMark{
			{at: 20, events: []core.ASREvent{tagged("a", tag("fr", 0.9), tag("fr", 0.9))}},
			{at: 400, events: []core.ASREvent{tagged("b", tag("ja", 0.5), tag("ja", 0.5))}},
			{at: 780, events: []core.ASREvent{tagged("c", tag("ja", 0.9))}},
		}},
		capability: autoCapability(
			protocol.Direction{Src: "zh", Tgt: "ja"},
			protocol.Direction{Src: "ja", Tgt: "zh"},
		),
	}
	config := map[string]core.PairConfig{
		"zh->ja": {ClauseLength: 100, Unit: "char", AutoTurnConfidenceFloor: 0.8},
		"ja->zh": {ClauseLength: 100, Unit: "char", AutoTurnConfidenceFloor: 0.8},
	}
	h := newHarnessWithConfig(t, asr, nil, nil, config)
	startConverse(h, "zh", "ja")
	setAuto(h, true)

	for round := 0; round < 3; round++ {
		h.feed(20)
		h.silence(360)
		h.session.Idle()
		// My spoken Segment gates the mic until the client reports playback
		// end and the tail passes; otherwise the next round never reaches ASR.
		for _, segment := range segmentsOf(h.events) {
			if segment.Speak {
				h.session.HandleMessage(protocol.ClientMessage{
					Type: "playback", SegID: segment.SegID, State: "end",
				})
			}
		}
		h.clock.Advance(520) // the 500 ms Gating tail plus a frame
	}

	uncertain := 0
	for _, turn := range turnEvents(h.events) {
		if turn.Uncertain {
			uncertain++
		}
		if turn.Who != "me" || turn.Auto && turn.Language != "" {
			t.Fatalf("Turn flipped on an undecided Segment: %+v", turn)
		}
	}
	if uncertain != 3 {
		t.Fatalf("uncertain turns = %d, want out-of-pair, low-confidence and short", uncertain)
	}
	segments := segmentsOf(h.events)
	if len(segments) != 3 {
		t.Fatalf("segments = %d", len(segments))
	}
	for _, segment := range segments {
		if segment.Direction != (protocol.Direction{Src: "zh", Tgt: "ja"}) {
			t.Fatalf("Segment direction = %+v, want the kept Turn", segment.Direction)
		}
	}
}

func TestAutoTurnOverrideCancelsThePendingSwitchAndIsLogged(t *testing.T) {
	asr := &autoScriptedASR{
		scriptedASR: &scriptedASR{name: "scripted", acceptsLive: true, marks: []asrMark{
			{at: 20, events: []core.ASREvent{tagged("hi", tag("ja", 0.9), tag("ja", 0.9))}},
		}},
		capability: autoCapability(
			protocol.Direction{Src: "zh", Tgt: "ja"},
			protocol.Direction{Src: "ja", Tgt: "zh"},
		),
	}
	h := startAuto(t, asr)
	h.feed(20) // tags say them, but no Cut yet

	h.session.HandleMessage(protocol.ClientMessage{Type: "turn", Who: "me"})
	h.session.Idle()

	turns := turnEvents(h.events)
	last := turns[len(turns)-1]
	if !last.Override || last.Who != "me" || last.Auto {
		t.Fatalf("override turn = %+v", last)
	}
	if len(h.logs) != 1 || !h.logs[0].TurnOverride {
		t.Fatalf("logs = %+v, want the override logged", h.logs)
	}
	if got := h.logs[0].Direction; got != (protocol.Direction{Src: "zh", Tgt: "ja"}) {
		t.Fatalf("overridden Segment direction = %+v", got)
	}

	h.silence(360)
	h.session.Idle()
	if segments := segmentsOf(h.events); len(segments) != 1 {
		t.Fatalf("segments = %d, want the pending text committed once", len(segments))
	}
}

// A tag in either language routes to its Direction, so the capability must
// cover both Directions of the Converse pair, not only the started one.
func TestAutoTurnRequiresBothDirectionsValidated(t *testing.T) {
	for name, validated := range map[string][]protocol.Direction{
		"none":                   {},
		"start direction only":   {{Src: "zh", Tgt: "ja"}},
		"reverse direction only": {{Src: "ja", Tgt: "zh"}},
	} {
		t.Run(name, func(t *testing.T) {
			asr := &autoScriptedASR{
				scriptedASR: &scriptedASR{name: "scripted", acceptsLive: true, marks: []asrMark{
					{at: 20, events: []core.ASREvent{tagged("hi", tag("ja", 0.9), tag("ja", 0.9))}},
				}},
				capability: autoCapability(validated...),
			}
			h := newHarness(t, asr, nil, nil)
			if hello := h.session.Hello(); hello.AutoTurn.Available {
				t.Fatalf("hello.autoTurn = %+v, want unavailable without both Directions", hello.AutoTurn)
			}
			startConverse(h, "zh", "ja")
			setAuto(h, true)
			if asr.autoOpens != 0 {
				t.Fatalf("auto streams = %d, want none", asr.autoOpens)
			}
			h.feed(20)
			h.silence(360)
			h.session.Idle()

			for _, turn := range turnEvents(h.events) {
				if turn.Auto || turn.Who != "me" {
					t.Fatalf("turn event = %+v, want only the manual me", turn)
				}
			}
		})
	}
}

// The confidence floor belongs to the pair, not to whichever Direction the
// previous Turn left behind: an asymmetric config must not change a Segment's
// uncertainty.
func TestAutoTurnFloorIsPairLevel(t *testing.T) {
	asr := &autoScriptedASR{
		scriptedASR: &scriptedASR{name: "scripted", acceptsLive: true, marks: []asrMark{
			{at: 20, events: []core.ASREvent{tagged("hi", tag("ja", 0.9), tag("ja", 0.9))}},
			{at: 400, events: []core.ASREvent{tagged("hmm", tag("ja", 0.5), tag("ja", 0.5))}},
		}},
		capability: autoCapability(
			protocol.Direction{Src: "zh", Tgt: "ja"},
			protocol.Direction{Src: "ja", Tgt: "zh"},
		),
	}
	config := map[string]core.PairConfig{
		"zh->ja": {ClauseLength: 100, Unit: "char", AutoTurnConfidenceFloor: 0.8},
	}
	h := newHarnessWithConfig(t, asr, nil, nil, config)
	startConverse(h, "zh", "ja")
	setAuto(h, true)

	h.feed(20)
	h.silence(360)
	h.session.Idle() // strong ja tags flip the Turn to them
	h.feed(20)
	h.silence(360)
	h.session.Idle() // the same low-confidence tags stay below the pair floor

	turns := turnEvents(h.events)
	last := turns[len(turns)-1]
	if !last.Uncertain || last.Language != "" || last.Who != "them" {
		t.Fatalf("last turn = %+v, want uncertain them", last)
	}
}

// A turn-granularity Provider reports one tag per final, so one tag above the
// pair floor decides; word/token Providers still need two agreeing tags.
func TestAutoTurnTurnGranularityDecidesOnOneConfidentTag(t *testing.T) {
	capability := autoCapability(
		protocol.Direction{Src: "zh", Tgt: "ja"},
		protocol.Direction{Src: "ja", Tgt: "zh"},
	)
	capability.Granularity = "turn"
	asr := &autoScriptedASR{
		scriptedASR: &scriptedASR{name: "scripted", acceptsLive: true, marks: []asrMark{
			{at: 20, events: []core.ASREvent{tagged("hi", tag("ja", 0.9))}},
			{at: 400, events: []core.ASREvent{tagged("hmm", tag("ja", 0.5))}},
		}},
		capability: capability,
	}
	config := map[string]core.PairConfig{
		"zh->ja": {ClauseLength: 100, Unit: "char", AutoTurnConfidenceFloor: 0.8},
		"ja->zh": {ClauseLength: 100, Unit: "char", AutoTurnConfidenceFloor: 0.8},
	}
	h := newHarnessWithConfig(t, asr, nil, nil, config)
	if hello := h.session.Hello(); !hello.AutoTurn.Available {
		t.Fatalf("hello.autoTurn = %+v, want available for a validated turn Provider", hello.AutoTurn)
	}
	startConverse(h, "zh", "ja")
	setAuto(h, true)
	if asr.autoOpens != 1 {
		t.Fatalf("auto streams = %d, want one", asr.autoOpens)
	}

	h.feed(20)
	h.silence(360)
	h.session.Idle()
	turns := turnEvents(h.events)
	last := turns[len(turns)-1]
	if !last.Auto || last.Who != "them" || last.Language != "ja" {
		t.Fatalf("auto turn = %+v, want one confident turn tag to decide", last)
	}

	h.feed(20)
	h.silence(360)
	h.session.Idle()
	turns = turnEvents(h.events)
	last = turns[len(turns)-1]
	if !last.Uncertain || last.Language != "" || last.Who != "them" {
		t.Fatalf("low-confidence turn tag = %+v, want uncertain them", last)
	}
}
