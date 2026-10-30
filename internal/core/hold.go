// Hold policy: ⏸ detection and the Held Segment's state. The Session owns
// translation, timers and logging; this module only tracks what is Held and
// builds the Forced Translation request from the prompt-asset instruction.

package core

import "strings"

const (
	holdMark    = "⏸"
	holdLimitMS = 2500
)

// pendingSegment is one Segment waiting to be translated. A Held Segment keeps
// its identity while later Cuts merge into it.
type pendingSegment struct {
	source      string
	speechEnd   int64
	held        bool
	merges      int
	forced      bool
	direction   *Direction
	finalizedAt int64
	heldAt      int64
	// Auto Turn provenance for the Segment log.
	turnAuto     bool
	turnOverride bool
	detected     string
}

// HoldPolicy tracks the Held Segment.
type HoldPolicy struct {
	instruction string
	held        *pendingSegment
}

// NewHoldPolicy builds the Hold policy for one Session.
func NewHoldPolicy(instruction string) *HoldPolicy {
	return &HoldPolicy{instruction: instruction}
}

// Inspect reports whether translated text is the Hold marker.
func (h *HoldPolicy) Inspect(text string) bool {
	return strings.TrimSpace(text) == holdMark
}

// Held returns the Held Segment or nil.
func (h *HoldPolicy) Held() *pendingSegment { return h.held }

// MarkHeld starts a Held Segment on its first ⏸.
func (h *HoldPolicy) MarkHeld(source string, speechEnd, now int64, direction *Direction, finalizedAt int64) *pendingSegment {
	if h.held == nil {
		h.held = &pendingSegment{
			source: source, speechEnd: speechEnd, held: true, direction: direction,
			finalizedAt: finalizedAt, heldAt: now,
		}
	}
	return h.held
}

// Merge appends a later Cut to the Held Segment.
func (h *HoldPolicy) Merge(source string, speechEnd, finalizedAt int64) *pendingSegment {
	if h.held == nil {
		return nil
	}
	h.held.source += " " + source
	h.held.speechEnd = speechEnd
	h.held.merges++
	h.held.finalizedAt = finalizedAt
	return h.held
}

// MSLeft is the time left before the Forced Translation.
func (h *HoldPolicy) MSLeft(now int64) int64 {
	if h.held == nil {
		return 0
	}
	left := h.held.heldAt + holdLimitMS - now
	if left < 0 {
		return 0
	}
	return left
}

// Expired reports whether the Hold limit has passed.
func (h *HoldPolicy) Expired(now int64) bool {
	return h.held != nil && !h.held.forced && now-h.held.heldAt >= holdLimitMS
}

// Force marks the Held Segment for a Forced Translation.
func (h *HoldPolicy) Force() *pendingSegment {
	if h.held == nil {
		return nil
	}
	h.held.forced = true
	return h.held
}

// WithInstruction adds the prompt-asset instruction to a Forced Translation.
func (h *HoldPolicy) WithInstruction(request TranslatorPrompt) TranslatorPrompt {
	if h.instruction != "" {
		request.System += "\n" + h.instruction
	}
	return request
}

// Reset forgets the Held Segment.
func (h *HoldPolicy) Reset() { h.held = nil }
