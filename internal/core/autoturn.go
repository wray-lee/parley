// Auto Turn: a capability-gated, pair-restricted decision made at the Segment
// Cut from Provider-reported language tags. No language codes or text
// heuristics live here; the pair and the tags are compared generically.
//
// S3 (spec/../.scratch/v01/spikes/S3.md) found a reproducible cross-Turn
// language-ID error in Deepgram Nova-3 multi for zh<->ja, so v0.1 ships
// Manual-only: an adapter must list both Directions of the pair in
// ValidatedPairs before Auto can be used, and the client opts in.

package core

import "github.com/wray-lee/parley/internal/protocol"

// autoTurnMinTags is the agreeing-tag rule chosen by S3: the first two
// agreeing non-punctuation tags decide; three gain no accuracy.
const autoTurnMinTags = 2

// turnProvenance rides a Segment's log so a decision or an override is
// auditable after the fact.
type turnProvenance struct {
	auto     bool
	override bool
	language string
}

// streaming reports whether the declaration can carry the Cut decision.
func (c LanguageCapability) streaming() bool {
	if !c.StreamingLanguageID || len(c.Languages) == 0 {
		return false
	}
	return c.Granularity == "word" || c.Granularity == "token" || c.Granularity == "turn"
}

// supports is the pair gate: both languages are in the Provider's set and the
// exact Direction passed its S3 acceptance run.
func (c LanguageCapability) supports(pair protocol.Direction) bool {
	if !c.streaming() || !containsLanguage(c.Languages, pair.Src) ||
		!containsLanguage(c.Languages, pair.Tgt) {
		return false
	}
	for _, validated := range c.ValidatedPairs {
		if validated == pair {
			return true
		}
	}
	return false
}

func containsLanguage(languages []string, want string) bool {
	for _, language := range languages {
		if language == want {
			return true
		}
	}
	return false
}

// autoCapability returns the adapter declaration and whether it can stream
// language tags at all. Pair validation is checked by supports.
func (s *Session) autoCapability() (LanguageCapability, bool) {
	auto, ok := s.asr.(AutoASR)
	if !ok {
		return LanguageCapability{}, false
	}
	capability := auto.LanguageCapability()
	return capability, capability.streaming()
}

// pairAutoAvailable applies the S3 amendment: the Provider must declare the
// tags and both Directions of the pair must have passed the acceptance run,
// because a tag in either language routes to its Direction.
func (s *Session) pairAutoAvailable(pair protocol.Direction) bool {
	capability, ok := s.autoCapability()
	return ok && capability.supports(pair) &&
		capability.supports(protocol.Direction{Src: pair.Tgt, Tgt: pair.Src})
}

// autoTurnHello reports capability at connection time. The pair is not chosen
// yet, so this is conservative: every Converse pair in the registry must pass
// the full gate, or Auto stays unavailable with a note. The pair is checked
// again at start.
func (s *Session) autoTurnHello() protocol.AutoTurn {
	capability, ok := s.autoCapability()
	if !ok {
		return protocol.AutoTurn{}
	}
	converse := 0
	for _, pair := range s.registry.Pairs {
		if !pair.Converse {
			continue
		}
		converse++
		for _, direction := range pair.Directions {
			if !capability.supports(direction) {
				return protocol.AutoTurn{}
			}
		}
	}
	if converse == 0 {
		return protocol.AutoTurn{}
	}
	return protocol.AutoTurn{Available: true}
}

// autoTurnFloor is the pair-level confidence floor: the stricter of the two
// Directions' configured floors, so a Segment's uncertainty cannot depend on
// the Direction the previous Turn left behind.
func (s *Session) autoTurnFloor() float64 {
	floor := s.pairConfig[s.start.Src+"->"+s.start.Tgt].AutoTurnConfidenceFloor
	reverse := s.pairConfig[s.start.Tgt+"->"+s.start.Src].AutoTurnConfidenceFloor
	if reverse > floor {
		return reverse
	}
	return floor
}

// decideAutoTurn aggregates the pending Segment's tags. A tag outside the
// Language Pair is unknown, never the other language. Word/token Providers
// need autoTurnMinTags agreeing tags; a turn Provider reports one tag at the
// final, so one tag at or above the pair floor decides. No winner leaves the Turn
// where it is and marks the pads.
func (s *Session) decideAutoTurn(current *Direction) (next *Direction, who, language string, uncertain bool) {
	next, who = current, s.turn
	if !s.autoEnabled || !s.autoAvailable || len(s.tags) == 0 || s.ownSpeechPending > 0 {
		return next, who, "", false
	}
	capability, _ := s.autoCapability()
	floor := s.autoTurnFloor()
	if capability.Granularity == "turn" {
		language = s.turnTag(floor, capability.LanguageConfidence)
	} else {
		language = s.agreeingTag(floor, capability.LanguageConfidence)
	}
	if language == "" {
		return next, who, "", true
	}
	for candidateWho, candidate := range s.directions {
		if candidate.Src == language {
			return candidate, candidateWho, language, false
		}
	}
	return next, who, "", true
}

// turnTag takes the last final from a turn-granularity Provider; only a
// reported confidence at or above the configured pair floor counts.
func (s *Session) turnTag(floor float64, confidenceReported bool) string {
	last := s.tags[len(s.tags)-1]
	if last.Language != s.start.Src && last.Language != s.start.Tgt {
		return ""
	}
	if !confidenceReported || floor <= 0 || last.Confidence < floor {
		return ""
	}
	return last.Language
}

// agreeingTag is the word/token rule: the first language to reach
// autoTurnMinTags tags at or above the floor wins.
func (s *Session) agreeingTag(floor float64, confidenceReported bool) string {
	counts := make(map[string]int)
	for _, tag := range s.tags {
		if tag.Language != s.start.Src && tag.Language != s.start.Tgt {
			continue
		}
		if confidenceReported && floor > 0 && tag.Confidence < floor {
			continue
		}
		counts[tag.Language]++
		if counts[tag.Language] >= autoTurnMinTags {
			return tag.Language
		}
	}
	return ""
}

// applyAutoTurn decides at the Cut and applies the Turn for this Segment. It
// adds no clock read and no queued work: the decision rides the result that
// cut the Segment (0 ms added to T1).
func (s *Session) applyAutoTurn(current *Direction) (*Direction, turnProvenance) {
	next, who, language, uncertain := s.decideAutoTurn(current)
	if language != "" {
		s.turn = who
		s.direction = next
		s.cut = NewCutPolicy(s.pairConfig[next.Src+"->"+next.Tgt])
		s.emit(&protocol.TurnEvent{Type: "turn", Who: who, Auto: true, Language: language})
		if s.hold.Held() != nil {
			s.enqueue(func() error {
				s.forceHeld()
				return nil
			})
		}
		return next, turnProvenance{auto: true, language: language}
	}
	if uncertain {
		s.emit(&protocol.TurnEvent{Type: "turn", Who: s.turn, Auto: true, Uncertain: true})
	}
	return current, turnProvenance{}
}
