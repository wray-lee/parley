package core_test

import (
	"testing"

	"github.com/wray-lee/parley/internal/core"
)

func frames(ms, amplitude, sampleRate int) []int16 {
	count := sampleRate / 1000 * ms
	pcm := make([]int16, count)
	for i := range pcm {
		pcm[i] = int16(amplitude)
	}
	return pcm
}

func TestCutPolicySilenceAfterSpeech(t *testing.T) {
	policy := core.NewCutPolicy(core.PairConfig{})
	if speech, cut := policy.Feed(frames(350, 0, 16000)); speech || cut {
		t.Fatalf("silence without speech: speech=%v cut=%v", speech, cut)
	}
	policy.Feed(frames(20, 3000, 16000))
	cutAt := -1
	for elapsed := 0; elapsed < 1000; elapsed += 20 {
		if _, cut := policy.Feed(frames(20, 0, 16000)); cut {
			cutAt = elapsed + 20
			break
		}
	}
	if cutAt < 340 || cutAt > 360 {
		t.Fatalf("silence cut at %d ms, want 350±10", cutAt)
	}
}

func TestCutPolicyClauseLengthInCharacters(t *testing.T) {
	policy := core.NewCutPolicy(core.PairConfig{ClauseLength: 12, Unit: "char"})
	if policy.Stable("你好，这是演示。", 1000) {
		t.Fatal("8 characters must not reach a 12-character clause")
	}
	if !policy.Stable("你好，这是演示。今天开会。", 1000) {
		t.Fatal("13 characters must reach a 12-character clause")
	}
}

func TestCutPolicyClauseLengthInWords(t *testing.T) {
	policy := core.NewCutPolicy(core.PairConfig{ClauseLength: 3, Unit: "word"})
	if policy.Stable("one two", 1000) {
		t.Fatal("2 words must not reach a 3-word clause")
	}
	if !policy.Stable("one two three", 1000) {
		t.Fatal("3 words must reach a 3-word clause")
	}
}

func TestCutPolicyAccumulation(t *testing.T) {
	policy := core.NewCutPolicy(core.PairConfig{})
	policy.Stable("x", 1000)
	if policy.Accumulated(3499) {
		t.Fatal("accumulated too early")
	}
	if !policy.Accumulated(3500) {
		t.Fatal("accumulated at 2500 ms")
	}
	policy.Reset()
	if policy.Accumulated(4000) {
		t.Fatal("reset must clear the accumulation clock")
	}
}
