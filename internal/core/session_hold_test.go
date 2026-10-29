package core_test

import (
	"strings"
	"testing"

	"github.com/wray-lee/parley/internal/core"
	"github.com/wray-lee/parley/internal/protocol"
)

func startListen(h *harness) {
	h.session.HandleMessage(protocol.ClientMessage{Type: "start", Mode: "listen", Src: "zh", Tgt: "ja"})
	h.session.HandleMessage(protocol.ClientMessage{Type: "sample", Name: "placeholder"})
}

func holdEvents(events []protocol.Event) []*protocol.HoldEvent {
	var holds []*protocol.HoldEvent
	for _, event := range events {
		if hold, ok := event.(*protocol.HoldEvent); ok {
			holds = append(holds, hold)
		}
	}
	return holds
}

func TestHoldMergesCutsIntoOneCommittedSegment(t *testing.T) {
	asr := &scriptedASR{name: "scripted", marks: []asrMark{
		{at: 20, events: []core.ASREvent{stable("part one")}},
		{at: 420, events: []core.ASREvent{stable("part two")}},
	}}
	translator := &scriptedTranslator{name: "scripted", answers: [][]string{{"⏸"}, {"merged"}}}
	h := newHarness(t, asr, translator, nil)
	startListen(h)

	h.feed(20)
	h.silence(360)
	h.session.Idle()
	h.feed(20)
	h.silence(360)
	h.session.Idle()

	segments := segmentsOf(h.events)
	if len(segments) != 1 || segments[0].Src != "part one part two" {
		t.Fatalf("segments = %+v", segments)
	}
	if len(h.logs) != 1 || !h.logs[0].Held || h.logs[0].Merges != 1 || h.logs[0].Forced {
		t.Fatalf("log = %+v", h.logs)
	}
	if len(translator.calls) != 2 || translator.calls[1].Text != "part one part two" {
		t.Fatalf("translator calls = %+v", translator.calls)
	}
	holds := holdEvents(h.events)
	if len(holds) != 1 || holds[0].Merges != 0 || holds[0].Type != "hold" {
		t.Fatalf("holds = %+v", holds)
	}
}

func TestHoldLimitForcesTranslationAt2500ms(t *testing.T) {
	asr := &scriptedASR{name: "scripted", marks: []asrMark{
		{at: 20, events: []core.ASREvent{stable("hold me")}},
	}}
	translator := &scriptedTranslator{name: "scripted", answers: [][]string{{"⏸"}, {"forced"}}}
	h := newHarness(t, asr, translator, nil)
	startListen(h)
	h.feed(20)
	h.silence(360)
	h.session.Idle()

	h.clock.Advance(2499)
	h.session.Idle()
	if len(translator.calls) != 1 {
		t.Fatalf("forced before the 2.5 s limit: %+v", translator.calls)
	}

	h.clock.Advance(1)
	h.session.Idle()
	if len(translator.calls) != 2 {
		t.Fatalf("forced translation did not run at 2.5 s: %+v", translator.calls)
	}
	if !strings.HasSuffix(translator.calls[1].System, "\nTranslate the current segment now, even if it is incomplete. Do not answer with ⏸.") {
		t.Fatalf("forced request lacks the instruction: %q", translator.calls[1].System)
	}
	segments := segmentsOf(h.events)
	if len(segments) != 1 || segments[0].Tgt != "forced" {
		t.Fatalf("segments = %+v", segments)
	}
	if len(h.logs) != 1 || !h.logs[0].Forced || h.logs[0].ForcedFail || h.logs[0].Merges != 0 {
		t.Fatalf("log = %+v", h.logs)
	}
}

func TestMergedCommitClearsTheHoldTimerBeforeTheNextHold(t *testing.T) {
	asr := &scriptedASR{name: "scripted", marks: []asrMark{
		{at: 20, events: []core.ASREvent{stable("part one")}},
		{at: 420, events: []core.ASREvent{stable("part two")}},
		{at: 820, events: []core.ASREvent{stable("part three")}},
	}}
	translator := &scriptedTranslator{name: "scripted", answers: [][]string{{"⏸"}, {"merged"}, {"⏸"}, {"forced"}}}
	h := newHarness(t, asr, translator, nil)
	startListen(h)

	h.feed(20)
	h.silence(360)
	h.session.Idle() // Held at 1380: its timer is due at 3880.
	h.feed(20)
	h.silence(360)
	h.session.Idle() // Merged and committed at 1760: the oracle releases the Hold here.
	h.feed(20)
	h.silence(360)
	h.session.Idle() // Held again at 2140: its own timer is due at 4640.

	h.clock.Advance(2499)
	h.session.Idle()
	if len(translator.calls) != 3 {
		t.Fatalf("a committed Hold left its timer armed and forced the next Segment early")
	}
	h.clock.Advance(1)
	h.session.Idle()
	if len(translator.calls) != 4 || !strings.HasSuffix(translator.calls[3].System, "Do not answer with ⏸.") {
		t.Fatalf("the new Hold did not force at 2.5 s: %+v", translator.calls)
	}
}

func TestForcedFailureCommitsSourceOnlyAndTheSessionContinues(t *testing.T) {
	asr := &scriptedASR{name: "scripted", marks: []asrMark{
		{at: 20, events: []core.ASREvent{stable("first")}},
		{at: 420, events: []core.ASREvent{stable("second")}},
	}}
	translator := &scriptedTranslator{name: "scripted", answers: [][]string{{"⏸"}, {"⏸"}, {"final"}}}
	tts := &scriptedTTS{name: "scripted"}
	h := newHarness(t, asr, translator, tts)
	startListen(h)

	h.feed(20)
	h.silence(360)
	h.session.Idle()
	h.clock.Advance(2500)
	h.session.Idle()

	segments := segmentsOf(h.events)
	if len(segments) != 1 || segments[0].Src != "first" || segments[0].Tgt != "" || segments[0].Speak {
		t.Fatalf("source-only segment = %+v", segments)
	}
	if len(h.logs) != 1 || !h.logs[0].ForcedFail || !h.logs[0].Forced || !h.logs[0].Held {
		t.Fatalf("log = %+v", h.logs)
	}
	if len(tts.request) != 0 {
		t.Fatalf("source-only Segment must not speak: %+v", tts.request)
	}

	h.feed(20)
	h.silence(360)
	h.session.Idle()
	segments = segmentsOf(h.events)
	if len(segments) != 2 || segments[1].SegID != 2 || segments[1].Src != "second" || segments[1].Tgt != "final" {
		t.Fatalf("segments after the forced failure = %+v", segments)
	}
}

func TestAccumulatedConfirmedSpeechCutsAt2500ms(t *testing.T) {
	asr := &scriptedASR{name: "scripted", marks: []asrMark{
		{at: 20, events: []core.ASREvent{stable("x")}},
	}}
	h := newHarnessWithConfig(t, asr, nil, nil, map[string]core.PairConfig{})
	startListen(h)

	h.feed(2500)
	h.session.Idle()
	if len(segmentsOf(h.events)) != 0 {
		t.Fatal("accumulated Cut fired before 2500 ms")
	}
	h.feed(20)
	h.session.Idle()
	segments := segmentsOf(h.events)
	if len(segments) != 1 || segments[0].Src != "x" {
		t.Fatalf("segments = %+v", segments)
	}
}

func TestClauseLengthCutsInSession(t *testing.T) {
	for _, test := range []struct {
		name   string
		config core.PairConfig
		text   string
	}{
		{name: "characters", config: core.PairConfig{ClauseLength: 3, Unit: "char"}, text: "abc"},
		{name: "words", config: core.PairConfig{ClauseLength: 2, Unit: "word"}, text: "two words"},
	} {
		t.Run(test.name, func(t *testing.T) {
			asr := &scriptedASR{name: "scripted", marks: []asrMark{
				{at: 20, events: []core.ASREvent{stable(test.text)}},
			}}
			h := newHarnessWithConfig(t, asr, nil, nil, map[string]core.PairConfig{"zh->ja": test.config})
			startListen(h)
			h.feed(20)
			h.session.Idle()
			segments := segmentsOf(h.events)
			if len(segments) != 1 || segments[0].Src != test.text {
				t.Fatalf("segments = %+v", segments)
			}
		})
	}

	asr := &scriptedASR{name: "scripted", marks: []asrMark{
		{at: 20, events: []core.ASREvent{stable("abcdef")}},
	}}
	h := newHarnessWithConfig(t, asr, nil, nil, map[string]core.PairConfig{})
	startListen(h)
	h.feed(20)
	h.session.Idle()
	if len(segmentsOf(h.events)) != 0 {
		t.Fatal("a Direction with no pair config must never Cut on clause length")
	}
}

func TestPTTReleaseCutsImmediatelyAndSuspendsAutomaticCuts(t *testing.T) {
	asr := &scriptedASR{name: "scripted", marks: []asrMark{
		{at: 20, events: []core.ASREvent{stable("abcdef")}},
	}}
	h := newHarnessWithConfig(t, asr, nil, nil,
		map[string]core.PairConfig{"zh->ja": {ClauseLength: 3, Unit: "char"}})
	startListen(h)

	h.session.HandleMessage(protocol.ClientMessage{Type: "ptt", State: "down"})
	h.feed(1000)
	h.session.Idle()
	if len(segmentsOf(h.events)) != 0 {
		t.Fatal("automatic Cuts must be suspended while PTT is held")
	}
	h.session.HandleMessage(protocol.ClientMessage{Type: "ptt", State: "up"})
	h.session.Idle()
	segments := segmentsOf(h.events)
	if len(segments) != 1 || segments[0].Src != "abcdef" {
		t.Fatalf("segments = %+v", segments)
	}
}

func TestRollingContextKeepsExactlyTheLastEightPairs(t *testing.T) {
	marks := make([]asrMark, 10)
	for index := range marks {
		marks[index] = asrMark{
			at:     20 + 380*int64(index),
			events: []core.ASREvent{stable("s" + string(rune('0'+index)))},
		}
	}
	answers := make([][]string, 10)
	for index := range answers {
		answers[index] = []string{"t" + string(rune('0'+index))}
	}
	translator := &scriptedTranslator{name: "scripted", answers: answers}
	h := newHarness(t, &scriptedASR{name: "scripted", marks: marks}, translator, nil)
	startListen(h)

	for index := 0; index < 10; index++ {
		h.feed(20)
		h.silence(360)
		h.session.Idle()
	}

	if len(segmentsOf(h.events)) != 10 || len(translator.calls) != 10 {
		t.Fatalf("segments = %d calls = %d", len(segmentsOf(h.events)), len(translator.calls))
	}
	wantSecond := "[已译上下文]\n原文：s0\n译文：t0\n[新段落]\ns1"
	if translator.calls[1].User != wantSecond {
		t.Fatalf("request 2 = %q", translator.calls[1].User)
	}
	wantThird := "[已译上下文]\n原文：s0\n译文：t0\n原文：s1\n译文：t1\n[新段落]\ns2"
	if translator.calls[2].User != wantThird {
		t.Fatalf("request 3 = %q", translator.calls[2].User)
	}
	last := translator.calls[9].User
	if strings.Count(last, "原文：") != 8 {
		t.Fatalf("request 10 carries %d pairs: %q", strings.Count(last, "原文："), last)
	}
	if !strings.Contains(last, "原文：s1") || strings.Contains(last, "原文：s0") {
		t.Fatalf("oldest pair not dropped: %q", last)
	}
}

func TestCommitOnceEmitsEachSegmentExactlyOnce(t *testing.T) {
	asr := &scriptedASR{name: "scripted", marks: []asrMark{
		{at: 20, events: []core.ASREvent{stable("one")}},
		{at: 420, events: []core.ASREvent{stable("two")}},
	}}
	translator := &scriptedTranslator{name: "scripted", answers: [][]string{{"uno"}, {"dos"}}}
	h := newHarness(t, asr, translator, nil)
	startListen(h)
	h.feed(20)
	h.silence(360)
	h.session.Idle()
	h.feed(20)
	h.silence(360)
	h.session.Idle()

	segments := segmentsOf(h.events)
	if len(segments) != 2 || segments[0].SegID != 1 || segments[1].SegID != 2 {
		t.Fatalf("segments = %+v", segments)
	}
	for _, segment := range segments {
		occurrences := 0
		for _, candidate := range segments {
			if candidate.SegID == segment.SegID {
				occurrences++
			}
		}
		if occurrences != 1 {
			t.Fatalf("seg_id %d emitted %d times", segment.SegID, occurrences)
		}
	}
}

func TestStopDropsTheHeldSegmentAndItsTimer(t *testing.T) {
	asr := &scriptedASR{name: "scripted", marks: []asrMark{
		{at: 20, events: []core.ASREvent{stable("held text")}},
	}}
	translator := &scriptedTranslator{name: "scripted", answers: [][]string{{"⏸"}, {"late"}}}
	h := newHarness(t, asr, translator, nil)
	startListen(h)
	h.feed(20)
	h.silence(360)
	h.session.Idle()
	if len(holdEvents(h.events)) != 1 {
		t.Fatalf("expected a Held Segment: %+v", h.events)
	}

	h.session.HandleMessage(protocol.ClientMessage{Type: "stop"})
	h.clock.Advance(3000)
	h.session.Idle()
	if len(segmentsOf(h.events)) != 0 {
		t.Fatalf("stop must not commit or force the Held Segment: %+v", segmentsOf(h.events))
	}
	if len(translator.calls) != 1 {
		t.Fatalf("the Hold timer fired after stop: %+v", translator.calls)
	}
}

func TestPackGlossaryReachesTheTranslatorRequest(t *testing.T) {
	asr := &scriptedASR{name: "scripted", marks: []asrMark{
		{at: 20, events: []core.ASREvent{stable("预算")}},
	}}
	translator := &scriptedTranslator{name: "scripted", answers: [][]string{{"translation"}}}
	h := newHarness(t, asr, translator, nil)
	h.session.HandleMessage(protocol.ClientMessage{
		Type: "start", Mode: "listen", Src: "zh", Tgt: "ja", Packs: []string{"conference"},
	})
	h.session.HandleMessage(protocol.ClientMessage{Type: "sample", Name: "placeholder"})
	h.feed(20)
	h.silence(360)
	h.session.Idle()

	if len(translator.calls) != 1 || !strings.Contains(translator.calls[0].System, "预算 → 予算") {
		t.Fatalf("translator request lacks the conference glossary: %+v", translator.calls)
	}
}
