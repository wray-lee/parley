package providers_test

import (
	"testing"
	"testing/fstest"

	"github.com/wray-lee/parley/internal/protocol"
	"github.com/wray-lee/parley/internal/providers"
)

func sampleJSON(id, text, translation string) []byte {
	return []byte(`{
		"id": "` + id + `",
		"src": "zh",
		"tgt": "ja",
		"timed_transcript": [{"start_ms": 0, "end_ms": 100, "text": "` + text + `"}],
		"reference_translations": {"` + text + `": "` + translation + `"}
	}`)
}

func book(t *testing.T) *providers.SampleBook {
	t.Helper()
	fsys := fstest.MapFS{
		"samples/manifest.json": {Data: []byte(`{"samples":[]}`)},
		"samples/alpha.json":    {Data: sampleJSON("alpha", "alpha text", "alpha translation")},
		"samples/beta.json":     {Data: sampleJSON("beta", "beta text", "beta translation")},
	}
	loaded, err := providers.LoadSampleBook(fsys)
	if err != nil {
		t.Fatal(err)
	}
	return loaded
}

func TestSampleBookSwitchesTheMockASRTranscript(t *testing.T) {
	book := book(t)
	asr := providers.NewSampleASR(book)

	asr.Open(protocol.Direction{})
	events, err := asr.Push(nil, 100)
	if err != nil || len(events) < 2 || events[1].Text != "alpha text" {
		t.Fatalf("alpha events = %+v, %v", events, err)
	}

	if !book.Select("beta") {
		t.Fatal("beta is not in the book")
	}
	asr.Open(protocol.Direction{})
	events, err = asr.Push(nil, 100)
	if err != nil || len(events) < 2 || events[1].Text != "beta text" {
		t.Fatalf("beta events = %+v, %v", events, err)
	}
}

func TestSampleBookSwitchesTheMockTranslatorReferences(t *testing.T) {
	book := book(t)
	translator := providers.NewSampleTranslator(book)

	var got string
	if err := translator.Translate(protocol.TranslatorRequest{Text: "alpha text"}, func(chunk string) { got = chunk }); err != nil {
		t.Fatal(err)
	}
	if got != "[DEMO] alpha translation" {
		t.Fatalf("alpha translation = %q", got)
	}

	book.Select("beta")
	if err := translator.Translate(protocol.TranslatorRequest{Text: "beta text"}, func(chunk string) { got = chunk }); err != nil {
		t.Fatal(err)
	}
	if got != "[DEMO] beta translation" {
		t.Fatalf("beta translation = %q", got)
	}
}

func TestSampleBookRejectsUnknownNames(t *testing.T) {
	book := book(t)
	if book.Select("nope") {
		t.Fatal("unknown sample selected")
	}
	if book.Select("../manifest") {
		t.Fatal("path-like sample selected")
	}
	if book.Current().ID != "alpha" {
		t.Fatalf("current = %q", book.Current().ID)
	}
}

func TestSampleSelectionsArePerSession(t *testing.T) {
	book := book(t)
	first := book.Session()
	second := book.Session()

	if !first.Select("beta") {
		t.Fatal("beta is not in the book")
	}
	// An empty name is the v0.0.3 `sample` message: keep the current sample.
	if !second.Select("") {
		t.Fatal("an empty name must keep the current sample")
	}
	if !book.Select("") {
		t.Fatal("an empty name on the template must not fail")
	}

	firstASR := providers.NewSampleASR(first)
	secondASR := providers.NewSampleASR(second)
	firstASR.Open(protocol.Direction{})
	secondASR.Open(protocol.Direction{})

	events, err := firstASR.Push(nil, 100)
	if err != nil || len(events) < 2 || events[1].Text != "beta text" {
		t.Fatalf("first session events = %+v, %v", events, err)
	}
	events, err = secondASR.Push(nil, 100)
	if err != nil || len(events) < 2 || events[1].Text != "alpha text" {
		t.Fatalf("second session events = %+v, %v", events, err)
	}
	if book.Current().ID != "alpha" {
		t.Fatalf("template selection changed to %q", book.Current().ID)
	}
}
