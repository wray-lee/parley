// The Server Host's Demo Mode plays more than one bundled sample. The client
// picks a chip from samples/manifest.json and sends `sample {name}`; the host
// selects that sample here before the Session reopens its mock ASR. Each
// connection gets its own Session copy of the book, so one Demo Session never
// switches another's sample. The JS oracle ignores the name (it pins one
// sample per connection), so this stays host-side.

package providers

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"sync"

	"github.com/wray-lee/parley/internal/core"
	"github.com/wray-lee/parley/internal/protocol"
)

// SampleBook holds every bundled sample, keyed by id, plus the selection the
// current Demo Session plays.
type SampleBook struct {
	mu      sync.RWMutex
	samples map[string]Sample
	current string
}

// LoadSampleBook reads samples/*.json (not the manifest) from fsys.
func LoadSampleBook(fsys fs.FS) (*SampleBook, error) {
	entries, err := fs.ReadDir(fsys, "samples")
	if err != nil {
		return nil, fmt.Errorf("read samples: %w", err)
	}
	book := &SampleBook{samples: map[string]Sample{}}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".json") || name == "manifest.json" {
			continue
		}
		data, err := fs.ReadFile(fsys, "samples/"+name)
		if err != nil {
			return nil, fmt.Errorf("read sample %s: %w", name, err)
		}
		var sample Sample
		if err := json.Unmarshal(data, &sample); err != nil {
			return nil, fmt.Errorf("parse sample %s: %w", name, err)
		}
		id := sample.ID
		if id == "" {
			id = strings.TrimSuffix(name, ".json")
			sample.ID = id
		}
		book.samples[id] = sample
	}
	if len(book.samples) == 0 {
		return nil, fmt.Errorf("no samples under samples/")
	}
	if _, ok := book.samples["placeholder"]; ok {
		book.current = "placeholder"
	} else {
		ids := make([]string, 0, len(book.samples))
		for id := range book.samples {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		book.current = ids[0]
	}
	return book, nil
}

// Session returns a copy that shares the immutable sample map but keeps its
// own selection, so concurrent Demo Sessions cannot switch each other's
// sample. The receiver is the template; callers never select on it.
func (b *SampleBook) Session() *SampleBook {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return &SampleBook{samples: b.samples, current: b.current}
}

// Select switches the sample the next ASR Open replays. An empty name keeps
// the current sample: that is the v0.0.3 `sample` message, whose name the
// oracle ignores. An unknown name keeps the current sample and reports false
// (the Host turns that into an explicit protocol error).
func (b *SampleBook) Select(name string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if name == "" {
		return true
	}
	if _, ok := b.samples[name]; !ok {
		return false
	}
	b.current = name
	return true
}

// Current returns the selected sample.
func (b *SampleBook) Current() Sample {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.samples[b.current]
}

// SampleASR replays whichever sample is selected when Open is called.
type SampleASR struct {
	book       *SampleBook
	transcript []TranscriptEntry
	fired      []firedLine
}

type firedLine struct{ partial, stable bool }

func NewSampleASR(book *SampleBook) *SampleASR { return &SampleASR{book: book} }

func (a *SampleASR) Name() string      { return "mock" }
func (a *SampleASR) AcceptsLive() bool { return false }
func (a *SampleASR) Revisable() bool   { return false }
func (a *SampleASR) Close()            {}

func (a *SampleASR) NewStream() core.ASR { return NewSampleASR(a.book) }

func (a *SampleASR) Discard() { a.fired = nil }

func (a *SampleASR) Open(protocol.Direction) {
	a.transcript = a.book.Current().TimedTranscript
	a.fired = make([]firedLine, len(a.transcript))
}

func (a *SampleASR) Push(_ []int16, audioMS int64) ([]core.ASREvent, error) {
	if a.fired == nil {
		a.Open(protocol.Direction{})
	}
	events := []core.ASREvent{}
	for index, entry := range a.transcript {
		state := &a.fired[index]
		if !state.partial && audioMS >= entry.StartMS {
			state.partial = true
			events = append(events, core.ASREvent{Type: "partial", Text: entry.Text})
		}
		if !state.stable && audioMS >= entry.EndMS {
			state.stable = true
			events = append(events, core.ASREvent{Type: "stable", Text: entry.Text})
		}
	}
	return events, nil
}

// SampleTranslator returns the selected sample's reference translation with
// the DEMO marker, read at Translate time so a switch takes effect at once.
type SampleTranslator struct{ book *SampleBook }

func NewSampleTranslator(book *SampleBook) *SampleTranslator {
	return &SampleTranslator{book: book}
}

func (t *SampleTranslator) Name() string { return "mock" }

func (t *SampleTranslator) Translate(request protocol.TranslatorRequest, onChunk func(string)) error {
	text, ok := t.book.Current().ReferenceTranslations[request.Text]
	if !ok {
		text = request.Text
	}
	onChunk(demoMarker + text)
	return nil
}
