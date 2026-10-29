package core

import (
	"strings"
	"testing"
)

// The oracle applies Unicode NFKC before lowercasing: halfwidth katakana
// composes, the fi ligature expands and a circled digit becomes ASCII.
func TestNormalizeEchoAppliesNFKC(t *testing.T) {
	for input, want := range map[string]string{
		"ｶﾞ":  "ガ",
		"ﬁle": "file",
		"①":   "1",
	} {
		if got := normalizeEcho(input); got != want {
			t.Fatalf("normalizeEcho(%q) = %q, want %q", input, got, want)
		}
	}
}

// core/echo.js measures string length and windows in UTF-16 code units. Seven
// astral characters plus three differing letters give 1-3/17 > 0.8; counting
// runes would give 1-3/10 = 0.7 and miss the Echo Suspect.
func TestBestEchoRatioCountsUTF16CodeUnits(t *testing.T) {
	spoken := strings.Repeat("𠮟", 7) + "abc"
	hypothesis := strings.Repeat("𠮟", 7) + "xyz"
	if got := bestEchoRatio(hypothesis, spoken); got <= echoThreshold {
		t.Fatalf("astral ratio = %v, want above %v like the oracle", got, echoThreshold)
	}
}

func TestNormalizeEchoKeepsLettersAndNumbers(t *testing.T) {
	if got := normalizeEcho("Hello, World! 123"); got != "helloworld123" {
		t.Fatalf("normalizeEcho = %q", got)
	}
	if got := normalizeEcho("ＡＢＣ１２３"); got != "abc123" {
		t.Fatalf("fullwidth normalizeEcho = %q", got)
	}
	// JavaScript's toLowerCase uses the full Unicode mapping, so a word-final
	// sigma becomes ς; Go's simple strings.ToLower would keep σ.
	if got := normalizeEcho("ΟΣ"); got != "ος" {
		t.Fatalf("final sigma normalizeEcho = %q, want %q", got, "ος")
	}
}

func TestBestEchoRatio(t *testing.T) {
	if got := bestEchoRatio(normalizeEcho("world"), normalizeEcho("hello world")); got != 1 {
		t.Fatalf("substring ratio = %v, want 1", got)
	}
	// One wrong character in a ten-character phrase is 0.9, above the threshold.
	if got := bestEchoRatio(normalizeEcho("helloworlt"), normalizeEcho("helloworld")); got <= 0.8 {
		t.Fatalf("one-edit ratio = %v, want above 0.8", got)
	}
	if got := bestEchoRatio(normalizeEcho("completely different"), normalizeEcho("hello world")); got >= 0.8 {
		t.Fatalf("unrelated ratio = %v, want below 0.8", got)
	}
	// The window start clamps to the spoken length, like core/echo.js: an ASR
	// hypothesis a few characters longer than the spoken text still matches.
	if got := bestEchoRatio(normalizeEcho("abcdefghijklmnopqrstuvw"), normalizeEcho("abcdefghijklmnopqrst")); got <= 0.8 {
		t.Fatalf("longer-hypothesis ratio = %v, want above 0.8", got)
	}
}

func TestEchoEntryExpiresOneTailAfterPlayback(t *testing.T) {
	filter := NewEchoFilter()
	filter.Remember(1, "Hello, world!")
	if !filter.IsEcho("hello world", 1000) {
		t.Fatal("a live entry must match")
	}
	filter.End(1, 1000)
	if !filter.IsEcho("hello world", 1000+gatingTailMS-1) {
		t.Fatal("the entry lives through the tail")
	}
	if filter.IsEcho("hello world", 1000+gatingTailMS) {
		t.Fatal("the entry expires at the end of the tail")
	}
}
