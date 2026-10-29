// Echo-filter seam: drop ASR hypotheses that match our own played speech. The
// Session owns each spoken Segment's playback lifetime; this module only
// matches normalised text and forgets an entry one tail after it ends. Ported
// from core/echo.js.
package core

import (
	"strings"
	"unicode"
	"unicode/utf16"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"golang.org/x/text/unicode/norm"
)

const echoThreshold = 0.8

type echoEntry struct {
	text  string
	until int64
}

// EchoFilter remembers the text the app spoke per Segment.
type EchoFilter struct {
	entries   map[int]echoEntry
	threshold float64
	tailMS    int64
}

// NewEchoFilter builds an EchoFilter with the 0.8 threshold and Gating tail.
func NewEchoFilter() *EchoFilter {
	return &EchoFilter{entries: make(map[int]echoEntry), threshold: echoThreshold, tailMS: gatingTailMS}
}

// Remember starts an entry alive for the whole playback.
func (f *EchoFilter) Remember(segID int, text string) {
	f.entries[segID] = echoEntry{text: normalizeEcho(text), until: maxInt64}
}

// End expires an entry one tail after playback ended.
func (f *EchoFilter) End(segID int, now int64) {
	if entry, ok := f.entries[segID]; ok {
		entry.until = now + f.tailMS
		f.entries[segID] = entry
	}
}

// IsEcho reports whether a hypothesis matches any live spoken text.
func (f *EchoFilter) IsEcho(text string, now int64) bool {
	hypothesis := normalizeEcho(text)
	if hypothesis == "" {
		return false
	}
	for segID, entry := range f.entries {
		if now >= entry.until {
			delete(f.entries, segID)
			continue
		}
		if bestEchoRatio(hypothesis, entry.text) > f.threshold {
			return true
		}
	}
	return false
}

// Reset forgets every entry.
func (f *EchoFilter) Reset() {
	f.entries = make(map[int]echoEntry)
}

const maxInt64 = int64(^uint64(0) >> 1)

// normalizeEcho is core/echo.js: NFKC, lowercase, then keep only letters and
// numbers.
func normalizeEcho(text string) string {
	var b strings.Builder
	for _, r := range cases.Lower(language.Und).String(norm.NFKC.String(text)) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// bestEchoRatio is the highest 1 - distance / longer length over windows of
// roughly the hypothesis's length. Lengths and windows are UTF-16 code units,
// like JavaScript's String.length and slice.
func bestEchoRatio(hypothesis, speech string) float64 {
	if hypothesis == "" || speech == "" {
		return 0
	}
	if strings.Contains(speech, hypothesis) {
		return 1
	}
	h := utf16.Encode([]rune(hypothesis))
	s := utf16.Encode([]rune(speech))
	shortest := len(h) - 2
	if shortest < 1 {
		shortest = 1
	}
	longest := len(h) + 2
	if longest > len(s) {
		longest = len(s)
	}
	best := 0.0
	// Clamp like core/echo.js: a hypothesis a few characters longer than the
	// spoken text still matches a window that covers the whole speech.
	for length := minInt(shortest, len(s)); length <= longest; length++ {
		for i := 0; i+length <= len(s); i++ {
			longer := len(h)
			if length > longer {
				longer = length
			}
			ratio := 1 - float64(editDistance(h, s[i:i+length]))/float64(longer)
			if ratio > best {
				best = ratio
			}
		}
	}
	return best
}

func editDistance(a, b []uint16) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		row := make([]int, len(b)+1)
		row[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			row[j] = minInt(prev[j]+1, minInt(row[j-1]+1, prev[j-1]+cost))
		}
		prev = row
	}
	return prev[len(b)]
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
